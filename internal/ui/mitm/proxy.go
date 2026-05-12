package mitm

import (
	"bufio"
	"bytes"
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

const (
	maxCaptureBody  = 1 << 20  // 1 MiB shown in inspector
	maxBodyForward  = 64 << 20 // 64 MiB hard cap for full forwarding
)

type Proxy struct {
	Store *Store

	mu       sync.Mutex
	listener net.Listener
	addr     string
	running  atomic.Bool
	wg       sync.WaitGroup

	connMu sync.Mutex
	conns  map[net.Conn]struct{}
}

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
	// proxy-visible work happens — we don't decrypt the payload.
	// BytesIn/BytesOut keep updating so the inspector still reflects
	// live traffic.
	now := time.Now()
	p.Store.Update(func() {
		flow.StatusCode = 200
		flow.Status = "200 Connection Established"
		flow.Ended = now
	})

	in, out := bridge(c, dst)
	p.Store.Update(func() {
		flow.BytesIn = in
		flow.BytesOut = out
		flow.TunnelClosed = true
	})
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
