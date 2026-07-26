package ws

import (
	"bufio"
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func dialCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func startRawServer(t *testing.T, handle func(c net.Conn, req *http.Request, br *bufio.Reader)) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
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
				defer c.Close()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				handle(c, req, br)
			}()
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		wg.Wait()
	})
	return l.Addr().String()
}

func TestDialRejectsBadScheme(t *testing.T) {
	tests := []struct {
		name   string
		target string
	}{
		{"ftp", "ftp://example.invalid/"},
		{"file", "file:///tmp/x"},
		{"no scheme", "example.invalid:9/"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Dial(dialCtx(t), tt.target, DialOptions{})
			if !errors.Is(err, ErrBadScheme) {
				t.Fatalf("err = %v, want ErrBadScheme", err)
			}
			if res != nil {
				t.Errorf("res = %+v, want nil", res)
			}
		})
	}
}

func TestDialRejectsUnparsableURL(t *testing.T) {
	res, err := Dial(dialCtx(t), "ws://[::1", DialOptions{})
	if err == nil {
		t.Fatal("expected parse error")
	}
	if res != nil {
		t.Errorf("res = %+v, want nil", res)
	}
}

func TestDialNon101Response(t *testing.T) {
	tests := []struct {
		name   string
		status string
		body   string
	}{
		{"forbidden", "403 Forbidden", "nope"},
		{"not found", "404 Not Found", "missing"},
		{"server error", "500 Internal Server Error", "boom"},
		{"empty body", "401 Unauthorized", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				resp := "HTTP/1.1 " + tt.status + "\r\n" +
					"Content-Length: " + itoa(len(tt.body)) + "\r\n\r\n" + tt.body
				_, _ = c.Write([]byte(resp))
			})
			res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{})
			if !errors.Is(err, ErrBadHandshake) {
				t.Fatalf("err = %v, want ErrBadHandshake", err)
			}
			if res == nil || res.Response == nil {
				t.Fatal("expected DialResult carrying the response")
			}
			if res.Conn != nil {
				t.Error("Conn should be nil on failed handshake")
			}
			if string(res.ResponseBody) != tt.body {
				t.Errorf("ResponseBody = %q, want %q", res.ResponseBody, tt.body)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestDialRejectsMalformedUpgradeHeaders(t *testing.T) {
	tests := []struct {
		name       string
		upgrade    string
		connection string
		wantErr    error
	}{
		{"missing upgrade", "", "Upgrade", ErrBadHandshake},
		{"wrong upgrade token", "h2c", "Upgrade", ErrBadHandshake},
		{"missing connection", "websocket", "", ErrBadHandshake},
		{"wrong connection token", "websocket", "keep-alive", ErrBadHandshake},
		{"case insensitive ok", "WebSocket", "UPGRADE", nil},
		{"connection list ok", "websocket", "keep-alive, Upgrade", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				accept := expectedAccept(req.Header.Get("Sec-WebSocket-Key"))
				resp := "HTTP/1.1 101 Switching Protocols\r\n"
				if tt.upgrade != "" {
					resp += "Upgrade: " + tt.upgrade + "\r\n"
				}
				if tt.connection != "" {
					resp += "Connection: " + tt.connection + "\r\n"
				}
				resp += "Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
				_, _ = c.Write([]byte(resp))
				if tt.wantErr == nil {
					_, _ = c.Read(make([]byte, 1))
				}
			})
			res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{})
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil {
				if res.Conn == nil {
					t.Fatal("expected a Conn")
				}
				_ = res.Conn.Close()
				return
			}
			if res == nil || res.Response == nil {
				t.Fatal("expected DialResult carrying the response")
			}
		})
	}
}

func TestDialRejectsBadAcceptKey(t *testing.T) {
	tests := []struct {
		name   string
		accept string
	}{
		{"empty", ""},
		{"garbage", "not-a-real-accept-key"},
		{"key echoed back", "echo"},
		{"accept of wrong key", expectedAccept("AAAAAAAAAAAAAAAAAAAAAA==")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				accept := tt.accept
				if accept == "echo" {
					accept = req.Header.Get("Sec-WebSocket-Key")
				}
				resp := "HTTP/1.1 101 Switching Protocols\r\n" +
					"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
					"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
				_, _ = c.Write([]byte(resp))
			})
			res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{})
			if !errors.Is(err, ErrBadAcceptKey) {
				t.Fatalf("err = %v, want ErrBadAcceptKey", err)
			}
			if res.Conn != nil {
				t.Error("Conn should be nil")
			}
		})
	}
}

func TestDialRejectsUnofferedExtension(t *testing.T) {
	addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
		accept := expectedAccept(req.Header.Get("Sec-WebSocket-Key"))
		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n" +
			"Sec-WebSocket-Extensions: permessage-deflate\r\n\r\n"
		_, _ = c.Write([]byte(resp))
	})
	res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{OfferDeflate: false})
	if !errors.Is(err, ErrExtensionRefused) {
		t.Fatalf("err = %v, want ErrExtensionRefused", err)
	}
	if res.Conn != nil {
		t.Error("Conn should be nil")
	}
}

func TestDialRequestLineAndHeaders(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		opts        DialOptions
		wantPath    string
		wantHeaders map[string]string
		absent      []string
	}{
		{
			name:     "root path defaulted",
			target:   "ws://%s",
			opts:     DialOptions{},
			wantPath: "/",
		},
		{
			name:     "path with query preserved",
			target:   "ws://%s/chat?room=1&x=2",
			opts:     DialOptions{},
			wantPath: "/chat?room=1&x=2",
		},
		{
			name:        "subprotocols joined",
			target:      "ws://%s/",
			opts:        DialOptions{Subprotocols: []string{"chat", "superchat"}},
			wantPath:    "/",
			wantHeaders: map[string]string{"Sec-Websocket-Protocol": "chat, superchat"},
		},
		{
			name:        "deflate offer",
			target:      "ws://%s/",
			opts:        DialOptions{OfferDeflate: true},
			wantPath:    "/",
			wantHeaders: map[string]string{"Sec-Websocket-Extensions": OfferExtensions()},
		},
		{
			name:     "custom headers pass through",
			target:   "ws://%s/",
			opts:     DialOptions{Headers: http.Header{"Authorization": {"Bearer tok"}, "X-Trace": {"abc"}}},
			wantPath: "/",
			wantHeaders: map[string]string{
				"Authorization": "Bearer tok",
				"X-Trace":       "abc",
			},
		},
		{
			name:   "handshake headers cannot be overridden",
			target: "ws://%s/",
			opts: DialOptions{Headers: http.Header{
				"Upgrade":               {"bogus"},
				"Connection":            {"bogus"},
				"Sec-WebSocket-Key":     {"bogus"},
				"Sec-WebSocket-Version": {"7"},
				"Content-Length":        {"999"},
			}},
			wantPath: "/",
			wantHeaders: map[string]string{
				"Upgrade":               "websocket",
				"Connection":            "Upgrade",
				"Sec-Websocket-Version": "13",
			},
			absent: []string{"Content-Length"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got *http.Request
			var gotURI string
			done := make(chan struct{})
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				got, gotURI = req, req.RequestURI
				close(done)
				accept := expectedAccept(req.Header.Get("Sec-WebSocket-Key"))
				resp := "HTTP/1.1 101 Switching Protocols\r\n" +
					"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
					"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
				_, _ = c.Write([]byte(resp))
				_, _ = c.Read(make([]byte, 1))
			})
			target := strings.Replace(tt.target, "%s", addr, 1)
			res, err := Dial(dialCtx(t), target, DialOptions(tt.opts))
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer res.Conn.Close()
			<-done
			if gotURI != tt.wantPath {
				t.Errorf("request URI = %q, want %q", gotURI, tt.wantPath)
			}
			if got.Host != addr {
				t.Errorf("Host header = %q, want %q", got.Host, addr)
			}
			if v := got.Header.Get("Sec-WebSocket-Version"); v != "13" {
				t.Errorf("Sec-WebSocket-Version = %q, want 13", v)
			}
			if k := got.Header.Get("Sec-WebSocket-Key"); k == "" || k == "bogus" {
				t.Errorf("Sec-WebSocket-Key = %q, want generated key", k)
			}
			for k, want := range tt.wantHeaders {
				if v := got.Header.Get(k); v != want {
					t.Errorf("header %s = %q, want %q", k, v, want)
				}
			}
			for _, k := range tt.absent {
				if v := got.Header.Get(k); v == "999" {
					t.Errorf("header %s leaked value %q", k, v)
				}
			}
		})
	}
}

func TestDialReportsSubprotocol(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{Subprotocols: []string{"superchat"}})
	defer stop()
	res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{Subprotocols: []string{"chat", "superchat"}})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if res.Subprotocol != "superchat" {
		t.Errorf("Subprotocol = %q, want superchat", res.Subprotocol)
	}
}

func TestDialConnRefused(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{DialTimeout: 2 * time.Second})
	if err == nil {
		t.Fatal("expected dial error to a closed port")
	}
	if res != nil {
		t.Errorf("res = %+v, want nil", res)
	}
}

func TestDialAlreadyCancelledContext(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{DialTimeout: 2 * time.Second})
	if err == nil {
		if res != nil && res.Conn != nil {
			_ = res.Conn.Close()
		}
		t.Fatal("expected error for pre-cancelled context")
	}
}

func TestDialMalformedResponse(t *testing.T) {
	tests := []struct {
		name string
		resp string
	}{
		{"not http", "GARBAGE\r\n\r\n"},
		{"truncated status line", "HTTP/1.1\r\n"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				_, _ = c.Write([]byte(tt.resp))
			})
			res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{DialTimeout: 3 * time.Second})
			if err == nil {
				t.Fatal("expected error")
			}
			if res != nil {
				t.Errorf("res = %+v, want nil", res)
			}
		})
	}
}

func TestDialSchemeAliasesAndPortDefaults(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()
	for _, scheme := range []string{"ws", "http", "WS", "Http"} {
		t.Run(scheme, func(t *testing.T) {
			res, err := Dial(dialCtx(t), scheme+"://"+addr+"/", DialOptions{})
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer res.Conn.Close()
			if err := res.Conn.WriteMessage(OpText, []byte("ok")); err != nil {
				t.Fatal(err)
			}
			_, p, err := res.Conn.ReadMessage()
			if err != nil {
				t.Fatal(err)
			}
			if string(p) != "ok" {
				t.Errorf("echo = %q", p)
			}
		})
	}
}

func TestIsUpgrade(t *testing.T) {
	tests := []struct {
		name   string
		method string
		header http.Header
		want   bool
	}{
		{
			name:   "valid",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"k"}},
			want:   true,
		},
		{
			name:   "case insensitive",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"WebSocket"}, "Connection": {"keep-alive, upgrade"}, "Sec-Websocket-Key": {"k"}},
			want:   true,
		},
		{
			name:   "post rejected",
			method: http.MethodPost,
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"k"}},
			want:   false,
		},
		{
			name:   "missing upgrade",
			method: http.MethodGet,
			header: http.Header{"Connection": {"Upgrade"}, "Sec-Websocket-Key": {"k"}},
			want:   false,
		},
		{
			name:   "wrong upgrade value",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"h2c"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"k"}},
			want:   false,
		},
		{
			name:   "missing connection",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"websocket"}, "Sec-Websocket-Key": {"k"}},
			want:   false,
		},
		{
			name:   "connection without upgrade token",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"close"}, "Sec-Websocket-Key": {"k"}},
			want:   false,
		},
		{
			name:   "missing key",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}},
			want:   false,
		},
		{
			name:   "empty key",
			method: http.MethodGet,
			header: http.Header{"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {""}},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Method: tt.method, Header: tt.header}
			if got := IsUpgrade(req); got != tt.want {
				t.Errorf("IsUpgrade = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpgradeRejectsNonUpgradeRequest(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	req := &http.Request{Method: http.MethodGet, Header: http.Header{}}
	res, err := Upgrade(a, bufio.NewReader(a), req, UpgradeOptions{})
	if !errors.Is(err, ErrNotUpgrade) {
		t.Fatalf("err = %v, want ErrNotUpgrade", err)
	}
	if res != nil {
		t.Errorf("res = %+v, want nil", res)
	}
}

func TestUpgradeWriteFailure(t *testing.T) {
	a, b := net.Pipe()
	_ = a.Close()
	_ = b.Close()
	req := &http.Request{Method: http.MethodGet, Header: http.Header{
		"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"k"},
	}}
	res, err := Upgrade(a, bufio.NewReader(a), req, UpgradeOptions{})
	if err == nil {
		t.Fatal("expected write error on closed conn")
	}
	if res != nil {
		t.Errorf("res = %+v, want nil", res)
	}
}

func TestNegotiateSubprotocol(t *testing.T) {
	tests := []struct {
		name   string
		client string
		server []string
		want   string
	}{
		{"empty client", "", []string{"chat"}, ""},
		{"empty server", "chat", nil, ""},
		{"both empty", "", nil, ""},
		{"exact match", "chat", []string{"chat"}, "chat"},
		{"client order wins", "b, a", []string{"a", "b"}, "b"},
		{"whitespace trimmed", "  chat  ,  x", []string{"chat"}, "chat"},
		{"case insensitive returns server casing", "CHAT", []string{"Chat"}, "Chat"},
		{"no overlap", "x, y", []string{"a", "b"}, ""},
		{"second client entry matches", "x, superchat", []string{"superchat"}, "superchat"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := negotiateSubprotocol(tt.client, tt.server); got != tt.want {
				t.Errorf("negotiateSubprotocol(%q, %v) = %q, want %q", tt.client, tt.server, got, tt.want)
			}
		})
	}
}

func TestResponseExtensions(t *testing.T) {
	tests := []struct {
		name string
		ext  ExtParams
		want string
	}{
		{"plain", ExtParams{Negotiated: true}, "permessage-deflate"},
		{"server no context", ExtParams{Negotiated: true, ServerNoContextTakeover: true},
			"permessage-deflate; server_no_context_takeover"},
		{"client no context", ExtParams{Negotiated: true, ClientNoContextTakeover: true},
			"permessage-deflate; client_no_context_takeover"},
		{"both", ExtParams{Negotiated: true, ServerNoContextTakeover: true, ClientNoContextTakeover: true},
			"permessage-deflate; server_no_context_takeover; client_no_context_takeover"},
		{"window bits dropped", ExtParams{Negotiated: true, ServerMaxWindowBits: 10, ClientMaxWindowBits: 9},
			"permessage-deflate"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := responseExtensions(tt.ext); got != tt.want {
				t.Errorf("responseExtensions = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIsHandshakeHeader(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"Host", true},
		{"host", true},
		{"HOST", true},
		{"Upgrade", true},
		{"Connection", true},
		{"Sec-WebSocket-Key", true},
		{"sec-websocket-version", true},
		{"Sec-WebSocket-Protocol", true},
		{"Sec-WebSocket-Extensions", true},
		{"Content-Length", true},
		{"Authorization", false},
		{"Cookie", false},
		{"X-Custom", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isHandshakeHeader(tt.key); got != tt.want {
			t.Errorf("isHandshakeHeader(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestTokenContains(t *testing.T) {
	tests := []struct {
		value string
		token string
		want  bool
	}{
		{"Upgrade", "upgrade", true},
		{"upgrade", "upgrade", true},
		{"keep-alive, Upgrade", "upgrade", true},
		{"Upgrade, keep-alive", "upgrade", true},
		{"  Upgrade  ", "upgrade", true},
		{"keep-alive", "upgrade", false},
		{"", "upgrade", false},
		{"upgraded", "upgrade", false},
		{"a,b,c", "b", true},
	}
	for _, tt := range tests {
		if got := tokenContains(tt.value, tt.token); got != tt.want {
			t.Errorf("tokenContains(%q, %q) = %v, want %v", tt.value, tt.token, got, tt.want)
		}
	}
}

func TestExpectedAcceptKnownVector(t *testing.T) {
	if got := expectedAccept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Errorf("expectedAccept = %q, want RFC 6455 vector", got)
	}
}

func TestGenerateSecKeyIsRandomBase64(t *testing.T) {
	seen := make(map[string]bool)
	for range 32 {
		k, err := generateSecKey()
		if err != nil {
			t.Fatal(err)
		}
		if len(k) != 24 || !strings.HasSuffix(k, "==") {
			t.Fatalf("key %q is not 16-byte base64", k)
		}
		if seen[k] {
			t.Fatalf("duplicate key %q", k)
		}
		seen[k] = true
	}
}

func TestReadHandshakeBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"nil-safe empty", "", 0},
		{"short", "hello", 5},
		{"exactly 4096", strings.Repeat("x", 4096), 4096},
		{"truncated at 4096", strings.Repeat("x", 9000), 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := readCloser{strings.NewReader(tt.body)}
			got := readHandshakeBody(rc)
			if len(got) != tt.want {
				t.Errorf("len = %d, want %d", len(got), tt.want)
			}
		})
	}
	if got := readHandshakeBody(nil); got != nil {
		t.Errorf("readHandshakeBody(nil) = %v, want nil", got)
	}
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }

func TestCanHonourClientWindow(t *testing.T) {
	cases := map[int]bool{0: true, 8: false, 9: false, 14: false, 15: true, 16: true}
	for bits, want := range cases {
		if got := canHonourClientWindow(bits); got != want {
			t.Errorf("canHonourClientWindow(%d) = %v, want %v", bits, got, want)
		}
	}
}

func TestDialRejectsUnhonourableClientWindowBits(t *testing.T) {
	addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
		accept := expectedAccept(req.Header.Get("Sec-WebSocket-Key"))
		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + accept + "\r\n" +
			"Sec-WebSocket-Extensions: permessage-deflate; client_max_window_bits=9\r\n\r\n"
		_, _ = c.Write([]byte(resp))
	})
	res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{OfferDeflate: true})
	if !errors.Is(err, ErrWindowBits) {
		t.Fatalf("err = %v, want ErrWindowBits: compress/flate always emits 15-bit "+
			"back references, so a smaller window would silently corrupt what we send", err)
	}
	if res.Conn != nil {
		t.Error("Conn should be nil")
	}
}

func TestDialAcceptsFullClientWindowBits(t *testing.T) {
	for _, ext := range []string{
		"permessage-deflate",
		"permessage-deflate; client_max_window_bits",
		"permessage-deflate; client_max_window_bits=15",
		"permessage-deflate; server_max_window_bits=10",
	} {
		t.Run(ext, func(t *testing.T) {
			addr := startRawServer(t, func(c net.Conn, req *http.Request, br *bufio.Reader) {
				accept := expectedAccept(req.Header.Get("Sec-WebSocket-Key"))
				resp := "HTTP/1.1 101 Switching Protocols\r\n" +
					"Upgrade: websocket\r\nConnection: Upgrade\r\n" +
					"Sec-WebSocket-Accept: " + accept + "\r\n" +
					"Sec-WebSocket-Extensions: " + ext + "\r\n\r\n"
				_, _ = c.Write([]byte(resp))
			})
			res, err := Dial(dialCtx(t), "ws://"+addr+"/", DialOptions{OfferDeflate: true})
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			_ = res.Conn.Close()
		})
	}
}
