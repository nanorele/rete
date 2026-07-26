package mitm

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"tracto/internal/ws"
)

func serveWSEcho(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{})
	if err != nil {
		return
	}
	conn := res.Conn
	for {
		op, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if op == ws.OpClose {
			_ = conn.WriteClose(ws.CloseNormal, "bye")
			return
		}
		if err := conn.WriteMessage(op, payload); err != nil {
			return
		}
	}
}

func TestInterceptWebSocket_EndToEnd(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	upstreamCert, err := ca.LeafFor("127.0.0.1")
	if err != nil {
		t.Fatalf("LeafFor: %v", err)
	}
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{*upstreamCert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go serveWSEcho(c)
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())

	store := NewStore()
	p := NewProxy(store)
	p.SetCA(ca)
	p.SetIntercept(true)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Stop()

	caPool := x509.NewCertPool()
	caPool.AddCert(ca.Cert)
	interceptDialRoots = caPool
	defer func() { interceptDialRoots = nil }()

	proxyConn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = proxyConn.Close() }()

	target := net.JoinHostPort(host, port)
	if _, err := proxyConn.Write([]byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n")); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	pbr := bufio.NewReader(proxyConn)
	connectResp, err := http.ReadResponse(pbr, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if connectResp.StatusCode != 200 {
		t.Fatalf("CONNECT status = %d", connectResp.StatusCode)
	}

	tlsConn := tls.Client(proxyConn, &tls.Config{ServerName: host, RootCAs: caPool})
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		t.Fatalf("tls handshake through proxy: %v", err)
	}

	handshake := "GET /socket HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := tlsConn.Write([]byte(handshake)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	tbr := bufio.NewReader(tlsConn)
	upgradeResp, err := http.ReadResponse(tbr, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d, want 101", upgradeResp.StatusCode)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x1, []byte("hello-mitm"), true)); err != nil {
		t.Fatalf("write text frame: %v", err)
	}
	op, payload, _, err := readWSFrame(tbr)
	if err != nil {
		t.Fatalf("read echo frame: %v", err)
	}
	if op != 0x1 || string(payload) != "hello-mitm" {
		t.Fatalf("echo mismatch: op=%x payload=%q", op, payload)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x2, []byte{9, 8, 7}, true)); err != nil {
		t.Fatalf("write binary frame: %v", err)
	}
	if op, _, _, err := readWSFrame(tbr); err != nil || op != 0x2 {
		t.Fatalf("binary echo: op=%x err=%v", op, err)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x8, []byte{0x03, 0xe8}, true)); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = tlsConn.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p.WS.Len() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	msgs := p.WS.Snapshot()
	if len(msgs) < 2 {
		t.Fatalf("expected captured WS frames, got %d", len(msgs))
	}

	var sawText, sawToServer, sawFromServer bool
	for _, m := range msgs {
		if m.Opcode == 0x1 && string(m.Payload) == "hello-mitm" {
			sawText = true
		}
		if m.ToServer {
			sawToServer = true
		} else {
			sawFromServer = true
		}
		if m.URL == "" || !strings.HasPrefix(m.URL, "wss://") {
			t.Errorf("captured frame has a bad URL: %q", m.URL)
		}
	}
	if !sawText {
		t.Errorf("the intercepted text frame was not captured: %+v", msgs)
	}
	if !sawToServer || !sawFromServer {
		t.Errorf("expected both directions captured: toServer=%v fromServer=%v", sawToServer, sawFromServer)
	}

	deadline = time.Now().Add(2 * time.Second)
	var wsFlow *Flow
	for time.Now().Before(deadline) {
		for _, f := range store.Snapshot() {
			if f.WebSocket {
				wsFlow = f
			}
		}
		if wsFlow != nil && wsFlow.TunnelClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if wsFlow == nil {
		t.Fatalf("no WebSocket flow recorded")
	}
	if wsFlow.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("ws flow status = %d, want 101", wsFlow.StatusCode)
	}
	if wsFlow.Method != "WS" {
		t.Errorf("ws flow method = %q, want WS", wsFlow.Method)
	}
}
