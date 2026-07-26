package workspace

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"tracto/internal/ws"
)

func TestWSStateString(t *testing.T) {
	cases := []struct {
		st   WSState
		want string
	}{
		{WSStateIdle, "Idle"},
		{WSStateConnecting, "Connecting"},
		{WSStateOpen, "Open"},
		{WSStateClosing, "Closing"},
		{WSStateClosed, "Closed"},
		{WSState(99), "?"},
	}
	for _, c := range cases {
		if got := c.st.String(); got != c.want {
			t.Errorf("WSState(%d).String() = %q, want %q", c.st, got, c.want)
		}
	}
}

func TestTrimSpaceLocal(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"   ", ""},
		{"\t\n\r ", ""},
		{"a", "a"},
		{"  a  ", "a"},
		{"\ta b\n", "a b"},
		{"a  ", "a"},
		{"  a", "a"},
	}
	for _, c := range cases {
		if got := trimSpaceLocal(c.in); got != c.want {
			t.Errorf("trimSpaceLocal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	for b := 0; b < 256; b++ {
		want := b == ' ' || b == '\t' || b == '\n' || b == '\r'
		if got := isSpaceByte(byte(b)); got != want {
			t.Errorf("isSpaceByte(%d) = %v, want %v", b, got, want)
		}
	}
}

func TestWSSessionStateAndStatus(t *testing.T) {
	s := newWSSession()
	if s.State() != WSStateIdle {
		t.Errorf("fresh state = %v, want Idle", s.State())
	}
	s.setState(WSStateOpen)
	if s.State() != WSStateOpen {
		t.Errorf("state = %v, want Open", s.State())
	}
	s.setStatus("Connected", false)
	if s.StatusText() != "Connected" || s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}
	s.setStatus("Boom", true)
	if s.StatusText() != "Boom" || !s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}
}

func TestWSSessionConnInfoAndGetConn(t *testing.T) {
	s := newWSSession()
	if s.getConn() != nil {
		t.Error("a fresh session must have no connection")
	}
	if s.Subprotocol() != "" {
		t.Errorf("Subprotocol = %q, want empty", s.Subprotocol())
	}
	ext := ws.ExtParams{Negotiated: true, ServerNoContextTakeover: true}
	s.setConnInfo(nil, "chat", ext)
	if s.Subprotocol() != "chat" {
		t.Errorf("Subprotocol = %q", s.Subprotocol())
	}
	if got := s.NegotiatedExtensions(); got != ext {
		t.Errorf("NegotiatedExtensions = %+v, want %+v", got, ext)
	}
}

func TestWSSubprotocolListDropsBlanks(t *testing.T) {
	s := newWSSession()
	s.AddSubprotocol("  chat  ")
	s.AddSubprotocol("")
	s.AddSubprotocol("\t\n")
	s.AddSubprotocol("json")
	got := s.SubprotocolList()
	if len(got) != 2 || got[0] != "chat" || got[1] != "json" {
		t.Errorf("SubprotocolList = %#v, want [chat json]", got)
	}
}

func TestWSMessageAppendersAndClear(t *testing.T) {
	s := newWSSession()
	s.sessionCount = 3
	s.appendMessage(WSDisplayMessage{Payload: []byte("a"), Session: 3})
	s.appendError("bad things")
	s.appendNote(3, "note")
	if len(s.Messages) != 3 {
		t.Fatalf("Messages = %d, want 3", len(s.Messages))
	}
	if s.Messages[1].Error != "bad things" || s.Messages[1].Session != 3 {
		t.Errorf("error entry = %+v", s.Messages[1])
	}
	if s.Messages[2].Note != "note" {
		t.Errorf("note entry = %+v", s.Messages[2])
	}
	s.ClearMessages()
	if len(s.Messages) != 0 {
		t.Errorf("ClearMessages left %d entries", len(s.Messages))
	}
}

func TestWSSessionMarkClosedIsIdempotent(t *testing.T) {
	s := newWSSession()
	var cancels int
	s.cancel = func() { cancels++ }
	s.markClosed()
	s.markClosed()
	if cancels != 1 {
		t.Errorf("cancel called %d times, want exactly 1", cancels)
	}
}

func TestWSMenuOpenAndClose(t *testing.T) {
	tab := NewRequestTab("t")
	if tab.WSMenuOpen() {
		t.Error("a tab with no WS session must report no open menu")
	}
	tab.CloseWSMenus()

	s := tab.EnsureWS()
	s.OpcodeMenuOpen = true
	if !tab.WSMenuOpen() {
		t.Error("WSMenuOpen must see the opcode menu")
	}
	s.OpcodeMenuOpen = false
	s.FilterMenuOpen = true
	if !tab.WSMenuOpen() {
		t.Error("WSMenuOpen must see the filter menu")
	}
	tab.CloseWSMenus()
	if tab.WSMenuOpen() {
		t.Error("CloseWSMenus must close both menus")
	}
}

func TestParseHexInput(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"00ff", "\x00\xff", false},
		{"00 ff", "\x00\xff", false},
		{"00:ff-01", "\x00\xff\x01", false},
		{"0x00ff", "\x00\xff", false},
		{"00\n ff\t", "\x00\xff", false},
		{"0,0,f,f", "\x00\xff", false},
		{"zz", "", true},
		{"abc", "", true},
	}
	for _, c := range cases {
		got, err := parseHexInput(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseHexInput(%q) = %x, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseHexInput(%q): %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("parseHexInput(%q) = %x, want %x", c.in, got, c.want)
		}
	}
}

func TestHexDump(t *testing.T) {
	if got := hexDump(nil); got != "" {
		t.Errorf("hexDump(nil) = %q, want empty", got)
	}
	if got := hexDump([]byte{0x00, 0x0f, 0xa5}); got != "00 0f a5" {
		t.Errorf("hexDump = %q", got)
	}
	long := make([]byte, 20)
	for i := range long {
		long[i] = byte(i)
	}
	got := hexDump(long)
	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("hexDump wrapped into %d lines, want 2: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "07  08") {
		t.Errorf("hexDump must double-space the 8-byte group boundary: %q", lines[0])
	}
	if len(strings.Fields(lines[1])) != 4 {
		t.Errorf("second line = %q, want the remaining 4 bytes", lines[1])
	}
}

func TestFormatDialError(t *testing.T) {
	base := errors.New("dial tcp: refused")
	if got := formatDialError(base, nil); got != "Dial failed: dial tcp: refused" {
		t.Errorf("nil result = %q", got)
	}
	if got := formatDialError(base, &ws.DialResult{}); !strings.HasPrefix(got, "Dial failed: ") {
		t.Errorf("result without a response = %q", got)
	}

	mk := func(status string, code int, ct string, body string) string {
		res := &ws.DialResult{
			Response:     &http.Response{Status: status, StatusCode: code, Header: http.Header{}},
			ResponseBody: []byte(body),
		}
		if ct != "" {
			res.Response.Header.Set("Content-Type", ct)
		}
		return formatDialError(base, res)
	}

	if got := mk("200 OK", 200, "text/html; charset=utf-8", ""); !strings.Contains(got, "returned HTML") {
		t.Errorf("html hint missing: %q", got)
	}
	if got := mk("200 OK", 200, "application/json", ""); !strings.Contains(got, "returned JSON") {
		t.Errorf("json hint missing: %q", got)
	}
	if got := mk("403 Forbidden", 403, "text/plain", ""); !strings.Contains(got, "refused the upgrade") {
		t.Errorf("4xx hint missing: %q", got)
	}
	if got := mk("200 OK", 200, "text/plain", ""); strings.Contains(got, " — ") {
		t.Errorf("a plain 200 must get no hint: %q", got)
	}
	if got := mk("500 Err", 500, "", "  boom  "); !strings.HasSuffix(got, "\nboom") {
		t.Errorf("body must be trimmed and appended: %q", got)
	}
	long := mk("500 Err", 500, "", strings.Repeat("x", 400))
	body := long[strings.Index(long, "\n")+1:]
	if len([]rune(body)) != 241 || !strings.HasSuffix(body, "…") {
		t.Errorf("long body must be truncated to 240 chars plus an ellipsis, got %d runes", len([]rune(body)))
	}
}

func TestSuffixFromExt(t *testing.T) {
	cases := []struct {
		name string
		res  ws.DialResult
		want []string
		none bool
	}{
		{name: "empty", res: ws.DialResult{}, none: true},
		{name: "subprotocol", res: ws.DialResult{Subprotocol: "chat"}, want: []string{"subprotocol=chat"}},
		{
			name: "deflate",
			res:  ws.DialResult{Extensions: ws.ExtParams{Negotiated: true}},
			want: []string{"permessage-deflate"},
		},
		{
			name: "deflate with takeovers",
			res: ws.DialResult{Extensions: ws.ExtParams{
				Negotiated:              true,
				ServerNoContextTakeover: true,
				ClientNoContextTakeover: true,
			}},
			want: []string{"server_no_context_takeover", "client_no_context_takeover"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := suffixFromExt(&c.res)
			if c.none {
				if got != "" {
					t.Errorf("suffixFromExt = %q, want empty", got)
				}
				return
			}
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("suffixFromExt = %q, want it to mention %q", got, w)
				}
			}
		})
	}
}

func TestIsNormalCloseErr(t *testing.T) {
	ctx := context.Background()
	normal := []error{
		nil,
		context.Canceled,
		ws.ErrConnClosed,
		io.EOF,
		io.ErrUnexpectedEOF,
		net.ErrClosed,
		fmt.Errorf("wrapped: %w", io.EOF),
		errors.New("read tcp: use of closed network connection"),
		errors.New("read tcp: connection reset by peer"),
		errors.New("write tcp: broken pipe"),
	}
	for _, err := range normal {
		if !isNormalCloseErr(ctx, err) {
			t.Errorf("isNormalCloseErr(%v) = false, want true", err)
		}
	}
	if isNormalCloseErr(ctx, errors.New("protocol error: bad opcode")) {
		t.Error("a protocol error must not count as a normal close")
	}

	done, cancel := context.WithCancel(context.Background())
	cancel()
	if !isNormalCloseErr(done, errors.New("protocol error: bad opcode")) {
		t.Error("any error must count as normal once the context is done")
	}
}

func TestIsAbnormalCloseCode(t *testing.T) {
	for _, c := range []ws.CloseCode{ws.CloseNormal, ws.CloseGoingAway, ws.CloseNoStatusRcvd} {
		if isAbnormalCloseCode(c) {
			t.Errorf("close code %d must be treated as normal", c)
		}
	}
	for _, c := range []ws.CloseCode{ws.CloseProtocolError, ws.CloseInternalErr, ws.CloseCode(4999)} {
		if !isAbnormalCloseCode(c) {
			t.Errorf("close code %d must be treated as abnormal", c)
		}
	}
}

func TestFormatPeerClose(t *testing.T) {
	if got := formatPeerClose(ws.CloseNormal, ""); got != "Closed by peer (code=1000)" {
		t.Errorf("formatPeerClose = %q", got)
	}
	if got := formatPeerClose(ws.CloseGoingAway, "bye"); got != "Closed by peer (code=1001, reason=bye)" {
		t.Errorf("formatPeerClose = %q", got)
	}
}

func TestWSSendWithoutConnectionReportsError(t *testing.T) {
	sends := []struct {
		name string
		fn   func(*RequestTab)
	}{
		{"text", func(tab *RequestTab) { tab.WSSendText("hi") }},
		{"binary", func(tab *RequestTab) { tab.WSSendBinary([]byte{1}) }},
		{"ping", func(tab *RequestTab) { tab.WSSendPing() }},
		{"proto", func(tab *RequestTab) { tab.WSSendProto(`{"a":1}`) }},
	}
	for _, c := range sends {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			c.fn(tab)
			s := tab.EnsureWS()
			if len(s.Messages) != 1 || s.Messages[0].Error != "Not connected" {
				t.Errorf("messages = %+v, want a single 'Not connected' error", s.Messages)
			}
		})
	}
}

func TestSendFromComposerRoutesByMode(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		proto   bool
		opText  bool
		wantErr string
	}{
		{"text mode", "hello", false, true, "Not connected"},
		{"binary mode", "00ff", false, false, "Not connected"},
		{"bad hex", "zz", false, false, "Hex parse: "},
		{"proto mode", `{"a":1}`, true, false, "Not connected"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			s := tab.EnsureWS()
			s.ComposerEditor.SetText(c.text)
			s.UseMsgpackProto = c.proto
			s.OpcodeText = c.opText
			tab.SendFromComposer()
			if len(s.Messages) != 1 {
				t.Fatalf("messages = %+v, want exactly 1", s.Messages)
			}
			if !strings.HasPrefix(s.Messages[0].Error, c.wantErr) {
				t.Errorf("error = %q, want prefix %q", s.Messages[0].Error, c.wantErr)
			}
		})
	}
}

func TestWSSendProtoRejectsBadHeaderFieldsAndJSON(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*WSSession)
		json   string
		prefix string
	}{
		{"bad cmd", func(s *WSSession) { s.ProtoCmdEditor.SetText("300") }, "{}", "cmd: "},
		{"bad seq", func(s *WSSession) { s.ProtoSeqEditor.SetText("99999") }, "{}", "seq: "},
		{"bad opcode", func(s *WSSession) { s.ProtoOpcodeEditor.SetText("abc") }, "{}", "opcode: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newWSSession()
			c.setup(s)
			_, _, _, err := s.protoHeaderFields()
			if err == nil {
				t.Fatal("protoHeaderFields must reject the value")
			}
			if !strings.HasPrefix(err.Error(), c.prefix) {
				t.Errorf("error = %q, want prefix %q", err, c.prefix)
			}
		})
	}

	s := newWSSession()
	cmd, seq, op, err := s.protoHeaderFields()
	if err != nil {
		t.Fatalf("default proto fields: %v", err)
	}
	if cmd != 0 || seq != 0 || op != 0 {
		t.Errorf("default proto fields = %d/%d/%d, want zeros", cmd, seq, op)
	}
}

func TestWSConnectRejectsBadURLs(t *testing.T) {
	cases := []struct {
		name string
		url  string
		env  map[string]string
		want string
	}{
		{"empty", "   ", nil, "URL is empty"},
		{"unresolved", "ws://{{host}}/s", nil, "unresolved variables"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			tab.URLInput.SetText(c.url)
			tab.WSConnect(context.Background(), nil, c.env, nil)
			s := tab.EnsureWS()
			if len(s.Messages) != 1 || !strings.Contains(s.Messages[0].Error, c.want) {
				t.Fatalf("messages = %+v, want an error mentioning %q", s.Messages, c.want)
			}
			if s.State() != WSStateIdle {
				t.Errorf("state = %v, want Idle after a rejected URL", s.State())
			}
		})
	}
}

func TestWSConnectIgnoredWhenAlreadyOpen(t *testing.T) {
	for _, st := range []WSState{WSStateConnecting, WSStateOpen} {
		tab := NewRequestTab("t")
		tab.URLInput.SetText("ws://127.0.0.1:1/s")
		s := tab.EnsureWS()
		s.setState(st)
		tab.WSConnect(context.Background(), nil, nil, nil)
		if len(s.Messages) != 0 {
			t.Errorf("state %v: WSConnect must be a no-op, got %+v", st, s.Messages)
		}
	}
}

func TestWSDisconnectNoopWhenNotConnected(t *testing.T) {
	tab := NewRequestTab("t")
	tab.WSDisconnect()

	s := tab.EnsureWS()
	for _, st := range []WSState{WSStateIdle, WSStateClosed, WSStateClosing} {
		s.setState(st)
		s.setStatus("keep", false)
		tab.WSDisconnect()
		if s.State() != st {
			t.Errorf("state %v changed to %v", st, s.State())
		}
		if s.StatusText() != "keep" {
			t.Errorf("status = %q, want untouched", s.StatusText())
		}
	}
}

func TestWSConnectHandshakeFailureReportsBanner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(404)
		_, _ = w.Write([]byte("<html>nope</html>"))
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("ws://" + strings.TrimPrefix(srv.URL, "http://") + "/s")
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)

	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if !s.StatusIsError() {
		t.Errorf("status = %q, want an error flag", s.StatusText())
	}
	msgs := wsMessages(s)
	if len(msgs) == 0 || !strings.Contains(msgs[0].Error, "Handshake rejected") {
		t.Errorf("messages = %+v, want a handshake rejection", msgs)
	}
}

func wsMessages(s *WSSession) []WSDisplayMessage {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	out := make([]WSDisplayMessage, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func waitWS(t *testing.T, s *WSSession, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		s.sessionMu.Lock()
		ok := cond()
		s.sessionMu.Unlock()
		if ok {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within the deadline (state=%v status=%q msgs=%d)",
		s.State(), s.StatusText(), len(wsMessages(s)))
}

func startWSEcho(t *testing.T, opts ws.UpgradeOptions, handle func(*ws.Conn)) string {
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
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					return
				}
				res, err := ws.Upgrade(c, br, req, opts)
				if err != nil {
					return
				}
				defer func() { _ = res.Conn.Close() }()
				handle(res.Conn)
			}(c)
		}
	}()
	t.Cleanup(func() {
		_ = l.Close()
		wg.Wait()
	})
	return "ws://" + l.Addr().String() + "/socket"
}

func echoUntilClosed(conn *ws.Conn) {
	for {
		op, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		switch op {
		case ws.OpText, ws.OpBinary:
			if err := conn.WriteMessage(op, payload); err != nil {
				return
			}
		case ws.OpClose:
			_ = conn.WriteClose(ws.CloseNormal, "")
			return
		}
	}
}

func TestWSConnectSendReceiveDisconnect(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{Subprotocols: []string{"chat"}}, echoUntilClosed)

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	s.AddSubprotocol("chat")

	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })
	if s.Subprotocol() != "chat" {
		t.Errorf("Subprotocol = %q, want chat", s.Subprotocol())
	}
	if s.StatusText() != "Connected" || s.StatusIsError() {
		t.Errorf("status = %q/%v", s.StatusText(), s.StatusIsError())
	}

	tab.WSSendText("hello")
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && string(m.Payload) == "hello" {
				return true
			}
		}
		return false
	})

	tab.WSSendBinary([]byte{1, 2, 3})
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && m.Opcode == ws.OpBinary && len(m.Payload) == 3 {
				return true
			}
		}
		return false
	})

	tab.WSDisconnect()
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if s.StatusIsError() {
		t.Errorf("a clean disconnect must not flag an error: %q", s.StatusText())
	}
}

func TestWSAutoPongOnPing(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		_ = conn.WriteMessage(ws.OpPing, []byte("hi"))
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool {
		var sawPing, sawPong bool
		for _, m := range s.Messages {
			if m.Opcode == ws.OpPing && m.Dir == ws.DirIn {
				sawPing = true
			}
			if m.Opcode == ws.OpPong && m.Dir == ws.DirOut && m.Note == "auto-pong" {
				sawPong = true
			}
		}
		return sawPing && sawPong
	})
	tab.MarkClosed()
}

func TestWSPeerCloseIsReported(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		_ = conn.WriteClose(ws.CloseProtocolError, "bad frame")
		time.Sleep(50 * time.Millisecond)
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if strings.Contains(m.Note, "Closed by peer") && strings.Contains(m.Note, "bad frame") {
				return true
			}
		}
		return false
	})
	waitWS(t, s, func() bool { return s.State() == WSStateClosed })
	if !s.StatusIsError() {
		t.Errorf("an abnormal peer close must flag an error, status=%q", s.StatusText())
	}
}

func TestMarkClosedStopsEverything(t *testing.T) {
	tab := NewRequestTab("t")
	s := tab.EnsureWS()
	var cancelled bool
	s.cancel = func() { cancelled = true }
	r := tab.EnsureRun()
	var runCancelled bool
	r.cancel = func() { runCancelled = true }

	tab.MarkClosed()
	if !tab.Closed.Load() {
		t.Error("MarkClosed must set the Closed flag")
	}
	if !cancelled {
		t.Error("MarkClosed must cancel the WS session")
	}
	if !runCancelled {
		t.Error("MarkClosed must stop an in-flight run")
	}

	tab.MarkClosed()
}

func TestMarkClosedRemovesResponseFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/resp.bin"
	if err := os.WriteFile(path, []byte("body"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := NewRequestTab("t")
	tab.respFile = path
	tab.MarkClosed()
	if _, err := os.Stat(path); err == nil {
		t.Error("MarkClosed must delete the spilled response file")
	}
	if tab.respFile != "" {
		t.Errorf("respFile = %q, want cleared", tab.respFile)
	}
}
