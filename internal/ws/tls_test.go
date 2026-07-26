package ws

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func selfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

func startTLSEchoServer(t *testing.T, opts UpgradeOptions) (addr string, pool *x509.CertPool) {
	t.Helper()
	cert, pool := selfSignedCert(t)
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				handleEcho(c, opts)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		wg.Wait()
	})
	return l.Addr().String(), pool
}

func TestDialTLSEcho(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{})
	res, err := Dial(dialCtx(t), "wss://"+addr+"/", DialOptions{TLSConfig: &tls.Config{RootCAs: pool}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteMessage(OpText, []byte("secure")); err != nil {
		t.Fatal(err)
	}
	op, p, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if op != OpText || string(p) != "secure" {
		t.Errorf("got op=%v %q, want TEXT \"secure\"", op, p)
	}
	if _, ok := res.Conn.Underlying().(*tls.Conn); !ok {
		t.Errorf("Underlying = %T, want *tls.Conn", res.Conn.Underlying())
	}
}

func TestDialTLSSchemeAliases(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{})
	for _, scheme := range []string{"wss", "https", "WSS", "HTTPS"} {
		t.Run(scheme, func(t *testing.T) {
			res, err := Dial(dialCtx(t), scheme+"://"+addr+"/", DialOptions{TLSConfig: &tls.Config{RootCAs: pool}})
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			_ = res.Conn.Close()
		})
	}
}

func TestDialTLSDoesNotMutateCallerConfig(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{})
	cfg := &tls.Config{RootCAs: pool}
	res, err := Dial(dialCtx(t), "wss://"+addr+"/", DialOptions{TLSConfig: cfg})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if cfg.ServerName != "" {
		t.Errorf("caller's TLSConfig was mutated: ServerName = %q", cfg.ServerName)
	}
}

func TestDialTLSSetsServerNameFromHost(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{})
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Dial(dialCtx(t), "wss://localhost:"+port+"/", DialOptions{TLSConfig: &tls.Config{RootCAs: pool}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	state := res.Conn.Underlying().(*tls.Conn).ConnectionState()
	if state.ServerName != "localhost" {
		t.Errorf("negotiated ServerName = %q, want localhost", state.ServerName)
	}
}

func TestDialTLSHonoursExplicitServerName(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{})
	cfg := &tls.Config{RootCAs: pool, ServerName: "localhost"}
	res, err := Dial(dialCtx(t), "wss://"+addr+"/", DialOptions{TLSConfig: cfg})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	state := res.Conn.Underlying().(*tls.Conn).ConnectionState()
	if state.ServerName != "localhost" {
		t.Errorf("ServerName = %q, want localhost", state.ServerName)
	}
}

func TestDialTLSUntrustedCertificate(t *testing.T) {
	addr, _ := startTLSEchoServer(t, UpgradeOptions{})
	res, err := Dial(dialCtx(t), "wss://"+addr+"/", DialOptions{})
	if err == nil {
		if res != nil && res.Conn != nil {
			_ = res.Conn.Close()
		}
		t.Fatal("expected certificate verification to fail")
	}
	if res != nil {
		t.Errorf("res = %+v, want nil", res)
	}
	var certErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if !errors.As(err, &certErr) && !errors.As(err, &hostErr) {
		t.Logf("verification error (accepted): %v", err)
	}
}

func TestDialTLSInsecureSkipVerify(t *testing.T) {
	addr, _ := startTLSEchoServer(t, UpgradeOptions{})
	res, err := Dial(dialCtx(t), "wss://"+addr+"/",
		DialOptions{TLSConfig: &tls.Config{InsecureSkipVerify: true}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteMessage(OpText, []byte("skip")); err != nil {
		t.Fatal(err)
	}
	_, p, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(p) != "skip" {
		t.Errorf("echo = %q", p)
	}
}

func TestDialTLSAgainstPlainServer(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()
	res, err := Dial(dialCtx(t), "wss://"+addr+"/",
		DialOptions{TLSConfig: &tls.Config{InsecureSkipVerify: true}, DialTimeout: 3 * time.Second})
	if err == nil {
		if res != nil && res.Conn != nil {
			_ = res.Conn.Close()
		}
		t.Fatal("expected TLS handshake failure against a plaintext server")
	}
}

func TestDialTLSDeflateNegotiated(t *testing.T) {
	addr, pool := startTLSEchoServer(t, UpgradeOptions{AcceptDeflate: true})
	res, err := Dial(dialCtx(t), "wss://"+addr+"/",
		DialOptions{TLSConfig: &tls.Config{RootCAs: pool}, OfferDeflate: true})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if !res.Extensions.Negotiated {
		t.Fatal("expected permessage-deflate to be negotiated over TLS")
	}
	payload := strings.Repeat("compress-me ", 500)
	if err := res.Conn.WriteMessage(OpText, []byte(payload)); err != nil {
		t.Fatal(err)
	}
	_, got, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Error("deflate roundtrip over TLS mismatched")
	}
}

func TestUpgradeResponseContents(t *testing.T) {
	tests := []struct {
		name        string
		reqHeaders  http.Header
		opts        UpgradeOptions
		wantHeaders map[string]string
		absent      []string
		wantSub     string
		wantExt     bool
	}{
		{
			name: "minimal",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="},
			},
			wantHeaders: map[string]string{
				"Upgrade":              "websocket",
				"Connection":           "Upgrade",
				"Sec-WebSocket-Accept": "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=",
			},
			absent: []string{"Sec-WebSocket-Protocol", "Sec-WebSocket-Extensions"},
		},
		{
			name: "subprotocol selected",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key":      {"dGhlIHNhbXBsZSBub25jZQ=="},
				"Sec-Websocket-Protocol": {"chat, superchat"},
			},
			opts:        UpgradeOptions{Subprotocols: []string{"superchat"}},
			wantHeaders: map[string]string{"Sec-WebSocket-Protocol": "superchat"},
			wantSub:     "superchat",
		},
		{
			name: "subprotocol offered but none match",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key":      {"dGhlIHNhbXBsZSBub25jZQ=="},
				"Sec-Websocket-Protocol": {"mqtt"},
			},
			opts:   UpgradeOptions{Subprotocols: []string{"chat"}},
			absent: []string{"Sec-WebSocket-Protocol"},
		},
		{
			name: "deflate accepted",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key":        {"dGhlIHNhbXBsZSBub25jZQ=="},
				"Sec-Websocket-Extensions": {"permessage-deflate; client_no_context_takeover"},
			},
			opts: UpgradeOptions{AcceptDeflate: true},
			wantHeaders: map[string]string{
				"Sec-WebSocket-Extensions": "permessage-deflate; client_no_context_takeover",
			},
			wantExt: true,
		},
		{
			name: "deflate offered but not accepted",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key":        {"dGhlIHNhbXBsZSBub25jZQ=="},
				"Sec-Websocket-Extensions": {"permessage-deflate"},
			},
			opts:   UpgradeOptions{AcceptDeflate: false},
			absent: []string{"Sec-WebSocket-Extensions"},
		},
		{
			name: "accept deflate but client did not offer",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="},
			},
			opts:   UpgradeOptions{AcceptDeflate: true},
			absent: []string{"Sec-WebSocket-Extensions"},
		},
		{
			name: "extra headers included and handshake headers filtered",
			reqHeaders: http.Header{
				"Upgrade": {"websocket"}, "Connection": {"Upgrade"},
				"Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="},
			},
			opts: UpgradeOptions{ExtraHeaders: http.Header{
				"X-Server":   {"tracto"},
				"Set-Cookie": {"a=b"},
				"Upgrade":    {"bogus"},
				"Connection": {"bogus"},
			}},
			wantHeaders: map[string]string{
				"X-Server":   "tracto",
				"Set-Cookie": "a=b",
				"Upgrade":    "websocket",
				"Connection": "Upgrade",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			req := &http.Request{Method: http.MethodGet, Header: tt.reqHeaders}

			type result struct {
				res *UpgradeResult
				err error
			}
			resc := make(chan result, 1)
			go func() {
				r, err := Upgrade(server, bufio.NewReader(server), req, tt.opts)
				resc <- result{r, err}
			}()

			resp, err := http.ReadResponse(bufio.NewReader(client), &http.Request{Method: "GET"})
			if err != nil {
				t.Fatalf("ReadResponse: %v", err)
			}
			got := <-resc
			if got.err != nil {
				t.Fatalf("Upgrade: %v", got.err)
			}
			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Errorf("status = %d, want 101", resp.StatusCode)
			}
			for k, want := range tt.wantHeaders {
				if v := resp.Header.Get(k); v != want {
					t.Errorf("header %s = %q, want %q", k, v, want)
				}
			}
			for _, k := range tt.absent {
				if v := resp.Header.Get(k); v != "" {
					t.Errorf("header %s = %q, want absent", k, v)
				}
			}
			if got.res.Subprotocol != tt.wantSub {
				t.Errorf("Subprotocol = %q, want %q", got.res.Subprotocol, tt.wantSub)
			}
			if got.res.Extensions.Negotiated != tt.wantExt {
				t.Errorf("Extensions.Negotiated = %v, want %v", got.res.Extensions.Negotiated, tt.wantExt)
			}
			if got.res.Conn == nil {
				t.Error("Conn is nil")
			} else if got.res.Conn.isClient {
				t.Error("server-side Conn has isClient = true")
			}
		})
	}
}
