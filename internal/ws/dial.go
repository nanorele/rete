package ws

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

var (
	ErrBadHandshake     = errors.New("ws: bad handshake response")
	ErrBadAcceptKey     = errors.New("ws: bad Sec-WebSocket-Accept")
	ErrBadScheme        = errors.New("ws: scheme must be ws or wss")
	ErrExtensionRefused = errors.New("ws: unexpected extension in response")
)

type DialOptions struct {
	TLSConfig    *tls.Config
	Subprotocols []string
	Headers      http.Header
	OfferDeflate bool
	DialTimeout  time.Duration
}

type DialResult struct {
	Conn         *Conn
	Response     *http.Response
	Subprotocol  string
	Extensions   ExtParams
	ResponseBody []byte
}

func Dial(ctx context.Context, target string, opts DialOptions) (*DialResult, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	var defaultPort string
	var useTLS bool
	switch strings.ToLower(u.Scheme) {
	case "ws", "http":
		defaultPort = "80"
	case "wss", "https":
		defaultPort = "443"
		useTLS = true
	default:
		return nil, ErrBadScheme
	}
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, defaultPort)
	}
	timeout := opts.DialTimeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	d := &net.Dialer{Timeout: timeout}
	rawConn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	netConn := rawConn
	if useTLS {
		tlsCfg := opts.TLSConfig
		if tlsCfg == nil {
			tlsCfg = &tls.Config{}
		}
		if tlsCfg.ServerName == "" {
			tlsCfg = tlsCfg.Clone()
			h, _, _ := net.SplitHostPort(host)
			tlsCfg.ServerName = h
		}
		tc := tls.Client(rawConn, tlsCfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			return nil, err
		}
		netConn = tc
	}

	stopWatch := context.AfterFunc(ctx, func() { _ = netConn.Close() })
	defer stopWatch()
	_ = netConn.SetDeadline(time.Now().Add(timeout))

	key, err := generateSecKey()
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}

	reqPath := u.RequestURI()
	if reqPath == "" {
		reqPath = "/"
	}
	hostHeader := u.Host

	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\n", reqPath)
	fmt.Fprintf(&b, "Host: %s\r\n", hostHeader)
	b.WriteString("Upgrade: websocket\r\n")
	b.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&b, "Sec-WebSocket-Key: %s\r\n", key)
	b.WriteString("Sec-WebSocket-Version: 13\r\n")
	if len(opts.Subprotocols) > 0 {
		fmt.Fprintf(&b, "Sec-WebSocket-Protocol: %s\r\n", strings.Join(opts.Subprotocols, ", "))
	}
	if opts.OfferDeflate {
		fmt.Fprintf(&b, "Sec-WebSocket-Extensions: %s\r\n", OfferExtensions())
	}
	for k, vs := range opts.Headers {
		if isHandshakeHeader(k) {
			continue
		}
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	if _, err := netConn.Write([]byte(b.String())); err != nil {
		_ = netConn.Close()
		return nil, err
	}

	br := bufio.NewReader(netConn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body := readHandshakeBody(resp.Body)
		_ = netConn.Close()
		return &DialResult{Response: resp, ResponseBody: body}, ErrBadHandshake
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") ||
		!tokenContains(resp.Header.Get("Connection"), "upgrade") {
		_ = netConn.Close()
		return &DialResult{Response: resp}, ErrBadHandshake
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != expectedAccept(key) {
		_ = netConn.Close()
		return &DialResult{Response: resp}, ErrBadAcceptKey
	}

	ext := ParseExtensions(resp.Header.Get("Sec-WebSocket-Extensions"))
	if ext.Negotiated && !opts.OfferDeflate {
		_ = netConn.Close()
		return &DialResult{Response: resp}, ErrExtensionRefused
	}

	c, err := NewConn(netConn, br, true, ext)
	if err != nil {
		_ = netConn.Close()
		return nil, err
	}
	if ctx.Err() != nil {
		_ = netConn.Close()
		return nil, ctx.Err()
	}
	_ = netConn.SetDeadline(time.Time{})
	return &DialResult{
		Conn:        c,
		Response:    resp,
		Subprotocol: resp.Header.Get("Sec-WebSocket-Protocol"),
		Extensions:  ext,
	}, nil
}

func generateSecKey() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf[:]), nil
}

func expectedAccept(clientKey string) string {
	h := sha1.New()
	h.Write([]byte(clientKey))
	h.Write([]byte(wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func tokenContains(value, token string) bool {
	for part := range strings.SplitSeq(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func readHandshakeBody(body io.ReadCloser) []byte {
	if body == nil {
		return nil
	}
	defer func() { _ = body.Close() }()
	buf := make([]byte, 4096)
	n, _ := io.ReadFull(io.LimitReader(body, int64(len(buf))), buf)
	return buf[:n]
}

func isHandshakeHeader(k string) bool {
	switch strings.ToLower(k) {
	case "host", "upgrade", "connection",
		"sec-websocket-key", "sec-websocket-version",
		"sec-websocket-protocol", "sec-websocket-extensions",
		"content-length":
		return true
	}
	return false
}
