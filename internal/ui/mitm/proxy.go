package mitm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultAddr = "127.0.0.1:8888"

// interceptDialRoots, if non-nil, replaces the system root pool used by
// the intercept transport when verifying upstream TLS. Tests set this
// to trust an httptest.NewTLSServer cert; production leaves it nil.
var interceptDialRoots *x509.CertPool

const (
	maxCaptureBody  = 1 << 20  // 1 MiB shown in inspector
	maxBodyForward  = 64 << 20 // 64 MiB hard cap for full forwarding
)

type Proxy struct {
	Store *Store

	mu        sync.Mutex
	listener  net.Listener
	addr      string
	running   atomic.Bool
	wg        sync.WaitGroup
	ca        *CA
	intercept atomic.Bool

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
}

// SetCA installs the CA used for HTTPS interception. Pass nil to disable
// the CA entirely (interception is then forced off).
func (p *Proxy) SetCA(ca *CA) {
	p.mu.Lock()
	p.ca = ca
	p.mu.Unlock()
	if ca == nil {
		p.intercept.Store(false)
	}
}

func (p *Proxy) CA() *CA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.ca
}

// SetIntercept toggles HTTPS MITM. Has no effect unless a CA is installed.
func (p *Proxy) SetIntercept(on bool) {
	if p.CA() == nil {
		p.intercept.Store(false)
		return
	}
	p.intercept.Store(on)
}

func (p *Proxy) Intercepting() bool { return p.intercept.Load() }

func (p *Proxy) trackConn(c net.Conn) {
	p.connMu.Lock()
	if p.conns == nil {
		p.conns = make(map[net.Conn]struct{})
	}
	p.conns[c] = struct{}{}
	p.connMu.Unlock()
}

func (p *Proxy) untrackConn(c net.Conn) {
	p.connMu.Lock()
	delete(p.conns, c)
	p.connMu.Unlock()
}

func (p *Proxy) closeAllConns() {
	p.connMu.Lock()
	list := make([]net.Conn, 0, len(p.conns))
	for c := range p.conns {
		list = append(list, c)
	}
	p.conns = nil
	p.connMu.Unlock()
	for _, c := range list {
		_ = c.Close()
	}
}

func NewProxy(store *Store) *Proxy {
	return &Proxy{Store: store}
}

func (p *Proxy) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.addr
}

func (p *Proxy) Running() bool { return p.running.Load() }

func (p *Proxy) Start(addr string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.running.Load() {
		return errors.New("proxy already running")
	}
	if addr == "" {
		addr = DefaultAddr
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	p.listener = l
	p.addr = l.Addr().String()
	p.running.Store(true)
	p.wg.Add(1)
	go p.serve(l)
	return nil
}

func (p *Proxy) Stop() {
	p.mu.Lock()
	l := p.listener
	p.listener = nil
	p.mu.Unlock()
	if l != nil {
		_ = l.Close()
	}
	p.closeAllConns()
	p.running.Store(false)
	p.wg.Wait()
	if p.Store != nil {
		p.Store.MarkAllEnded()
	}
}

func (p *Proxy) serve(l net.Listener) {
	defer p.wg.Done()
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go p.handleConn(c)
	}
}

func (p *Proxy) handleConn(c net.Conn) {
	p.trackConn(c)
	defer func() {
		p.untrackConn(c)
		_ = c.Close()
	}()
	_ = c.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	_ = c.SetReadDeadline(time.Time{})

	if req.Method == http.MethodConnect {
		p.handleConnect(c, req)
		return
	}
	p.handleHTTP(c, br, req)
}

func (p *Proxy) handleConnect(c net.Conn, req *http.Request) {
	host, port, err := splitHostPort(req.Host, "443")
	if err != nil {
		writeStatus(c, 400, "Bad CONNECT request")
		return
	}
	target := net.JoinHostPort(host, port)

	flow := p.Store.Add(&Flow{
		Kind:       FlowTunnel,
		ClientAddr: c.RemoteAddr().String(),
		Scheme:     "https",
		Method:     "CONNECT",
		Host:       host,
		Port:       port,
		URL:        target,
		Version:    req.Proto,
		ReqHeaders: collectHeaders(req.Header),
	})
	defer p.markEnded(flow)

	dst, err := net.DialTimeout("tcp", target, 15*time.Second)
	if err != nil {
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 502
			flow.Status = "502 Bad Gateway"
			flow.Ended = time.Now()
		})
		writeStatus(c, 502, "CONNECT dial failed: "+err.Error())
		return
	}
	p.trackConn(dst)
	defer func() {
		p.untrackConn(dst)
		_ = dst.Close()
	}()

	if _, err := io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.TunnelClosed = true
		})
		return
	}

	// Stamp Ended at handshake completion. The TCP tunnel will stay
	// open as long as the browser holds keep-alive, but no further
	// proxy-visible work happens unless we intercept TLS. In all cases
	// the per-tunnel "duration" we report is the time to establish.
	now := time.Now()
	p.Store.Update(func() {
		flow.StatusCode = 200
		flow.Status = "200 Connection Established"
		flow.Ended = now
	})

	if p.intercept.Load() && p.ca != nil {
		// Don't need the upstream TCP — interceptHTTPS opens its own TLS
		// connections per request via http.Client.
		_ = dst.Close()
		p.untrackConn(dst)
		p.interceptHTTPS(c, host, port, flow)
		return
	}

	in, out := bridge(c, dst)
	p.Store.Update(func() {
		flow.BytesIn = in
		flow.BytesOut = out
		flow.TunnelClosed = true
	})
}

// interceptHTTPS terminates TLS on the client side using a leaf cert
// signed by our root CA, then loops reading plaintext HTTP/1.1 requests
// from the client and forwarding each to the real upstream over TLS.
// Each forwarded request is captured as its own HTTP-kind Flow so the
// inspector shows real headers and bodies.
func (p *Proxy) interceptHTTPS(client net.Conn, host, port string, parent *Flow) {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
		GetCertificate: func(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
			sni := hi.ServerName
			if sni == "" {
				sni = host
			}
			return p.ca.LeafFor(sni)
		},
	}
	tlsConn := tls.Server(client, cfg)
	defer func() {
		_ = tlsConn.Close()
		p.Store.Update(func() {
			parent.TunnelClosed = true
		})
	}()
	if err := tlsConn.Handshake(); err != nil {
		p.Store.Update(func() {
			parent.Error = "tls handshake: " + err.Error()
		})
		return
	}

	upstreamHost := tlsConn.ConnectionState().ServerName
	if upstreamHost == "" {
		upstreamHost = host
	}
	target := net.JoinHostPort(upstreamHost, port)

	// One transport per intercepted CONNECT — gives us connection reuse
	// to the upstream while keeping cert verification strict.
	transport := &http.Transport{
		Proxy: nil,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			d := &net.Dialer{Timeout: 15 * time.Second}
			raw, err := d.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			tc := tls.Client(raw, &tls.Config{
				ServerName: upstreamHost,
				RootCAs:    interceptDialRoots,
				MinVersion: tls.VersionTLS12,
				NextProtos: []string{"http/1.1"},
			})
			if err := tc.HandshakeContext(ctx); err != nil {
				_ = raw.Close()
				return nil, err
			}
			return tc, nil
		},
		ForceAttemptHTTP2:     false,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       30 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}
	defer transport.CloseIdleConnections()
	cl := &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 0,
	}

	br := bufio.NewReader(tlsConn)
	for {
		_ = tlsConn.SetReadDeadline(time.Now().Add(60 * time.Second))
		req, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		_ = tlsConn.SetReadDeadline(time.Time{})
		req.URL.Scheme = "https"
		// Use host:port so http.Transport dials the right port (the
		// real upstream may be on something other than 443, e.g. tests).
		req.URL.Host = target

		flow := &Flow{
			Kind:       FlowHTTP,
			ClientAddr: client.RemoteAddr().String(),
			Scheme:     "https",
			Method:     req.Method,
			Host:       upstreamHost,
			Port:       port,
			Path:       req.URL.RequestURI(),
			URL:        "https://" + target + req.URL.RequestURI(),
			Version:    req.Proto,
			ReqHeaders: collectHeaders(req.Header),
		}
		body, _ := readLimited(req.Body, maxCaptureBody)
		_ = req.Body.Close()
		flow.ReqBody = body
		flow.ReqSize = int64(len(body))
		p.Store.Add(flow)

		if !p.proxyOneIntercepted(cl, tlsConn, target, req, body, flow) {
			return
		}
	}
}

// proxyOneIntercepted forwards a single intercepted HTTPS request to the
// upstream and writes the response back over the already-terminated TLS
// connection. Returns false if the connection should be torn down (e.g.
// the client requested Connection: close, or write to client failed).
func (p *Proxy) proxyOneIntercepted(cl *http.Client, tlsConn *tls.Conn, target string, req *http.Request, body []byte, flow *Flow) bool {
	defer p.markEnded(flow)

	out, err := http.NewRequest(req.Method, req.URL.String(), bytes.NewReader(body))
	if err != nil {
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 500
			flow.Status = "500 Internal Proxy Error"
		})
		return false
	}
	out.Header = req.Header.Clone()
	stripHopByHop(out.Header)
	out.Host = req.Host
	out.ContentLength = int64(len(body))

	resp, err := cl.Do(out)
	if err != nil {
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 502
			flow.Status = "502 Bad Gateway"
		})
		_, _ = io.WriteString(tlsConn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	fullBody, _ := readLimited(resp.Body, maxBodyForward)
	captured := fullBody
	if int64(len(captured)) > maxCaptureBody {
		captured = captured[:maxCaptureBody]
	}
	p.Store.Update(func() {
		flow.Status = resp.Status
		flow.StatusCode = resp.StatusCode
		flow.RespHeaders = collectHeaders(resp.Header)
		flow.RespBody = captured
		flow.RespSize = int64(len(fullBody))
	})

	stripHopByHop(resp.Header)
	resp.Header.Set("Content-Length", strconv.Itoa(len(fullBody)))
	if _, err := fmt.Fprintf(tlsConn, "HTTP/1.1 %s\r\n", resp.Status); err != nil {
		return false
	}
	if err := resp.Header.Write(tlsConn); err != nil {
		return false
	}
	if _, err := io.WriteString(tlsConn, "\r\n"); err != nil {
		return false
	}
	if _, err := tlsConn.Write(fullBody); err != nil {
		return false
	}
	if strings.EqualFold(req.Header.Get("Connection"), "close") {
		return false
	}
	return true
}

func (p *Proxy) handleHTTP(c net.Conn, br *bufio.Reader, req *http.Request) {
	// A real proxy request uses absolute-form in the request-line
	// (RFC 7230 §5.3.2): "GET http://example.com/foo HTTP/1.1".
	// Origin-form ("GET /foo") with only a Host header means the client
	// is hitting the proxy address directly (e.g. typed into a browser
	// URL bar). Forwarding such a request would loop right back into
	// ourselves and exhaust ephemeral ports.
	if !req.URL.IsAbs() || req.URL.Host == "" {
		p.serveDirectInfo(c)
		return
	}
	if sameHost(req.URL.Host, p.addr) {
		p.serveDirectInfo(c)
		return
	}
	host, port, _ := splitHostPort(req.URL.Host, "80")

	flow := &Flow{
		Kind:       FlowHTTP,
		ClientAddr: c.RemoteAddr().String(),
		Scheme:     req.URL.Scheme,
		Method:     req.Method,
		Host:       host,
		Port:       port,
		Path:       req.URL.RequestURI(),
		URL:        req.URL.String(),
		Version:    req.Proto,
		ReqHeaders: collectHeaders(req.Header),
	}

	body, _ := readLimited(req.Body, maxCaptureBody)
	_ = req.Body.Close()
	flow.ReqBody = body
	flow.ReqSize = int64(len(body))
	p.Store.Add(flow)
	defer p.markEnded(flow)

	stripHopByHop(req.Header)
	req.RequestURI = ""
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))

	cl := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       60 * time.Second,
	}
	resp, err := cl.Do(req)
	if err != nil {
		p.Store.Update(func() {
			flow.Error = err.Error()
			flow.StatusCode = 502
			flow.Status = "502 Bad Gateway"
			flow.Ended = time.Now()
		})
		writeStatus(c, 502, "upstream error: "+err.Error())
		return
	}
	defer func() { _ = resp.Body.Close() }()

	fullBody, _ := readLimited(resp.Body, maxBodyForward)
	captured := fullBody
	if int64(len(captured)) > maxCaptureBody {
		captured = captured[:maxCaptureBody]
	}
	p.Store.Update(func() {
		flow.Status = resp.Status
		flow.StatusCode = resp.StatusCode
		flow.RespHeaders = collectHeaders(resp.Header)
		flow.RespBody = captured
		flow.RespSize = int64(len(fullBody))
		flow.Ended = time.Now()
	})

	stripHopByHop(resp.Header)
	resp.Header.Set("Content-Length", strconv.Itoa(len(fullBody)))
	_, _ = fmt.Fprintf(c, "HTTP/1.1 %s\r\n", resp.Status)
	_ = resp.Header.Write(c)
	_, _ = io.WriteString(c, "\r\n")
	_, _ = c.Write(fullBody)
	_ = br
}

func writeStatus(c net.Conn, code int, msg string) {
	_, _ = fmt.Fprintf(c, "HTTP/1.1 %d %s\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		code, http.StatusText(code), len(msg), msg)
}

func (p *Proxy) markEnded(flow *Flow) {
	if p.Store == nil || flow == nil {
		return
	}
	p.Store.Update(func() {
		if flow.Ended.IsZero() {
			flow.Ended = time.Now()
		}
	})
}

func (p *Proxy) serveDirectInfo(c net.Conn) {
	body := "Tracto MITM Proxy\n\n" +
		"This endpoint is an HTTP proxy, not a website.\n" +
		"Configure your client to use http://" + p.addr + " as the HTTP/HTTPS proxy.\n"
	_, _ = fmt.Fprintf(c,
		"HTTP/1.1 421 Misdirected Request\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
		len(body), body)
}

func sameHost(a, b string) bool {
	ha, pa, err := net.SplitHostPort(a)
	if err != nil {
		ha = a
	}
	hb, pb, err := net.SplitHostPort(b)
	if err != nil {
		hb = b
	}
	if pa != "" && pb != "" && pa != pb {
		return false
	}
	return canonicalHost(ha) == canonicalHost(hb)
}

func canonicalHost(h string) string {
	h = strings.ToLower(strings.TrimSpace(h))
	switch h {
	case "localhost", "::1", "[::1]", "0.0.0.0":
		return "127.0.0.1"
	}
	return h
}

func bridge(a, b net.Conn) (in, out int64) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(b, a)
		atomic.AddInt64(&out, n)
		if tcp, ok := b.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(a, b)
		atomic.AddInt64(&in, n)
		if tcp, ok := a.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		}
	}()
	wg.Wait()
	return in, out
}

var hopByHop = []string{
	"Connection",
	"Proxy-Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
}

func stripHopByHop(h http.Header) {
	if c := h.Get("Connection"); c != "" {
		for _, k := range strings.Split(c, ",") {
			h.Del(strings.TrimSpace(k))
		}
	}
	for _, k := range hopByHop {
		h.Del(k)
	}
}

func collectHeaders(h http.Header) [][2]string {
	out := make([][2]string, 0, len(h))
	for k, vs := range h {
		for _, v := range vs {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

func splitHostPort(hostport, defaultPort string) (host, port string, err error) {
	host, port, err = net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
		port = defaultPort
		err = nil
	}
	return
}

func readLimited(r io.Reader, max int64) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	lr := io.LimitReader(r, max)
	return io.ReadAll(lr)
}
