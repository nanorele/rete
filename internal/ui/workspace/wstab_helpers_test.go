package workspace

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"tracto/internal/ws"

	"github.com/nanorele/gio/app"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024*1024 - 1, "1024.0K"},
		{1024 * 1024, "1.0M"},
		{3 * 1024 * 1024, "3.0M"},
	}
	for _, c := range cases {
		if got := humanBytes(c.n); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestPreviewPayload(t *testing.T) {
	closePayload := func(code ws.CloseCode, reason string) []byte {
		p := []byte{byte(code >> 8), byte(code)}
		return append(p, reason...)
	}

	cases := []struct {
		name string
		p    []byte
		op   ws.Opcode
		want string
	}{
		{"text", []byte("hello"), ws.OpText, "hello"},
		{"binary hex", []byte{0xde, 0xad}, ws.OpBinary, "dead"},
		{"invalid utf8 as text", []byte{0xff, 0xfe}, ws.OpText, "fffe"},
		{"close with reason", closePayload(ws.CloseNormal, "bye"), ws.OpClose, `code=1000 "bye"`},
		{"close without reason", closePayload(ws.CloseGoingAway, ""), ws.OpClose, "code=1001"},
		{"close too short", []byte{1}, ws.OpClose, ""},
		{"close empty", nil, ws.OpClose, ""},
	}
	for _, c := range cases {
		if got := previewPayload(c.p, c.op); got != c.want {
			t.Errorf("%s: previewPayload = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPreviewPayloadTruncates(t *testing.T) {
	bin := make([]byte, 100)
	got := previewPayload(bin, ws.OpBinary)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("long binary preview must be elided: %q", got)
	}
	if want := hex.EncodeToString(bin[:64]) + "…"; got != want {
		t.Errorf("binary preview = %q, want the first 64 bytes", got)
	}

	long := strings.Repeat("a", 300)
	got = previewPayload([]byte(long), ws.OpText)
	if got != long[:256]+"…" {
		t.Errorf("long text preview = %q, want the first 256 bytes plus an ellipsis", got)
	}
}

func TestPreviewPayloadTruncatesOnRuneBoundary(t *testing.T) {
	s := strings.Repeat("a", 255) + "日本語"
	got := previewPayload([]byte(s), ws.OpText)
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncation, got %q", got)
	}
	body := strings.TrimSuffix(got, "…")
	if !strings.HasPrefix(s, body) {
		t.Errorf("truncated preview %q is not a prefix of the payload", body)
	}
	for _, r := range body {
		if r == '�' {
			t.Errorf("truncation split a multi-byte rune: %q", body)
		}
	}
}

func TestDetailText(t *testing.T) {
	closePayload := []byte{0x03, 0xe8, 'b', 'y', 'e'}

	cases := []struct {
		name  string
		msg   WSDisplayMessage
		asHex bool
		want  string
	}{
		{"text", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, false, "hi"},
		{"text as hex", WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("hi")}, true, "68 69"},
		{"binary invalid utf8", WSDisplayMessage{Opcode: ws.OpBinary, Payload: []byte{0xff}}, false, "ff"},
		{"close", WSDisplayMessage{Opcode: ws.OpClose, Payload: closePayload}, false, "code=1000\nreason=bye"},
		{"close as hex", WSDisplayMessage{Opcode: ws.OpClose, Payload: closePayload}, true, hexDump(closePayload)},
		{"empty", WSDisplayMessage{Opcode: ws.OpText}, false, ""},
	}
	for _, c := range cases {
		if got := detailText(c.msg, c.asHex); got != c.want {
			t.Errorf("%s: detailText = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetailTextForProtoMessages(t *testing.T) {
	m := WSDisplayMessage{
		Opcode:  ws.OpBinary,
		Payload: []byte{1, 2},
		Proto:   &ProtoView{Cmd: 7, Seq: 3, Opcode: 9, RawLen: 40, JSON: `{"a":1}`},
	}
	got := detailText(m, false)
	for _, want := range []string{"cmd=7", "seq=3", "opcode=9", "uncompressed", `{"a":1}`} {
		if !strings.Contains(got, want) {
			t.Errorf("proto detail = %q, want it to mention %q", got, want)
		}
	}

	m.Proto.Cof = 2
	m.Proto.BodyLen = 20
	if got := detailText(m, false); !strings.Contains(got, "lz4 cof=2") {
		t.Errorf("compressed proto detail = %q, want the lz4 line", got)
	}

	m.Proto.DecodeErr = "bad msgpack"
	if got := detailText(m, false); !strings.Contains(got, "decode error: bad msgpack") {
		t.Errorf("failed proto detail = %q", got)
	}

	if got := detailText(m, true); got != hexDump(m.Payload) {
		t.Errorf("hex mode must ignore the proto view: %q", got)
	}
}

func TestPreviewProto(t *testing.T) {
	p := &ProtoView{Cmd: 1, Seq: 2, Opcode: 3, JSON: "{\n  \"a\": 1\n}"}
	got := previewProto(p)
	if strings.Contains(got, "\n") {
		t.Errorf("preview must collapse whitespace: %q", got)
	}
	if !strings.Contains(got, `{ "a": 1 }`) {
		t.Errorf("preview = %q", got)
	}

	p.Cof = 4
	if got := previewProto(p); !strings.Contains(got, "lz4") {
		t.Errorf("compressed preview = %q, want an lz4 marker", got)
	}

	p.DecodeErr = "boom"
	if got := previewProto(p); !strings.Contains(got, "boom") {
		t.Errorf("failed preview = %q", got)
	}

	if got := previewProto(&ProtoView{Cmd: 5}); got != "cmd=5 seq=0 op=0" {
		t.Errorf("preview with no body = %q", got)
	}
}

func TestDirString(t *testing.T) {
	if got := dirString(ws.DirOut); !strings.HasPrefix(got, "OUT") {
		t.Errorf("dirString(DirOut) = %q", got)
	}
	if got := dirString(ws.DirIn); !strings.HasPrefix(got, "IN") {
		t.Errorf("dirString(DirIn) = %q", got)
	}
}

func TestFormatNegotiated(t *testing.T) {
	s := newWSSession()
	if got := s.formatNegotiated(); got != "" {
		t.Errorf("a closed session must report nothing, got %q", got)
	}

	s.setState(WSStateOpen)
	if got := s.formatNegotiated(); got != "" {
		t.Errorf("an open session with no negotiation must report nothing, got %q", got)
	}

	s.setConnInfo(nil, "chat", ws.ExtParams{Negotiated: true})
	got := s.formatNegotiated()
	if !strings.Contains(got, "subprotocol=chat") || !strings.Contains(got, "deflate") {
		t.Errorf("formatNegotiated = %q", got)
	}
}

func TestRefreshDetailTracksSelection(t *testing.T) {
	s := newWSSession()
	s.Selected = -1
	s.refreshDetail()
	if s.DetailSrcID != -1 {
		t.Errorf("DetailSrcID = %d, want -1 with nothing selected", s.DetailSrcID)
	}

	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("first")})
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("second")})
	s.Selected = 1
	s.refreshDetail()
	if s.DetailEditor.Text() != "second" {
		t.Errorf("DetailEditor = %q, want second", s.DetailEditor.Text())
	}
	if s.DetailSrcID != 1 {
		t.Errorf("DetailSrcID = %d, want 1", s.DetailSrcID)
	}

	s.DetailHex = true
	s.refreshDetail()
	if s.DetailEditor.Text() != hexDump([]byte("second")) {
		t.Errorf("switching to hex must re-render: %q", s.DetailEditor.Text())
	}

	s.Selected = 99
	s.refreshDetail()
	if s.Selected != -1 || s.DetailSrcID != -1 {
		t.Errorf("an out-of-range selection must reset: sel=%d src=%d", s.Selected, s.DetailSrcID)
	}
}

func TestWSDebouncerTriggerCoalesces(t *testing.T) {
	d := newWSDebouncer(new(app.Window))
	d.trigger()
	if !d.armed.Load() {
		t.Fatal("the first trigger must arm the debouncer")
	}
	d.trigger()
	if !d.armed.Load() {
		t.Error("a second trigger while armed must stay armed")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && d.armed.Load() {
		time.Sleep(2 * time.Millisecond)
	}
	if d.armed.Load() {
		t.Error("the debouncer never disarmed")
	}
	d.trigger()

	var nilD *wsDebouncer
	nilD.trigger()
	(&wsDebouncer{}).trigger()
}

func TestAttachWSWindowIsIdempotent(t *testing.T) {
	tab := NewRequestTab("t")
	win := new(app.Window)
	tab.AttachWSWindow(win)
	s := tab.EnsureWS()
	if s.notify == nil {
		t.Fatal("AttachWSWindow must install a notifier")
	}
	first := s.notify
	tab.AttachWSWindow(new(app.Window))
	if s.notify != first {
		t.Error("AttachWSWindow must not replace an existing notifier")
	}
	s.appendMessage(WSDisplayMessage{Payload: []byte("x")})
}

func TestWSSendProtoOverConnection(t *testing.T) {
	received := make(chan []byte, 4)
	url := startWSEcho(t, ws.UpgradeOptions{}, func(conn *ws.Conn) {
		for {
			op, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if op == ws.OpBinary {
				cp := make([]byte, len(payload))
				copy(cp, payload)
				select {
				case received <- cp:
				default:
				}
				_ = conn.WriteMessage(ws.OpBinary, payload)
			}
		}
	})

	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	s.UseMsgpackProto = true
	s.ProtoCmdEditor.SetText("5")
	s.ProtoSeqEditor.SetText("11")
	s.ProtoOpcodeEditor.SetText("2")

	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	s.ComposerEditor.SetText(`{"hello":"world"}`)
	tab.SendFromComposer()

	select {
	case <-received:
	case <-time.After(10 * time.Second):
		t.Fatal("the server never received the encoded frame")
	}

	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirIn && m.Proto != nil {
				return true
			}
		}
		return false
	})
	for _, m := range wsMessages(s) {
		if m.Dir == ws.DirOut && m.Proto != nil {
			if m.Proto.Cmd != 5 || m.Proto.Seq != 11 || m.Proto.Opcode != 2 {
				t.Errorf("outgoing proto header = %+v, want cmd=5 seq=11 op=2", m.Proto)
			}
			if !strings.Contains(m.Proto.JSON, "hello") {
				t.Errorf("outgoing proto JSON = %q", m.Proto.JSON)
			}
		}
	}
	tab.MarkClosed()
}

func TestWSSendProtoRejectsInvalidJSON(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, echoUntilClosed)
	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	tab.WSSendProto(`{"broken":`)
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if strings.HasPrefix(m.Error, "JSON parse: ") {
				return true
			}
		}
		return false
	})
	tab.MarkClosed()
}

func TestWSSendProtoEmptyPayloadIsAllowed(t *testing.T) {
	url := startWSEcho(t, ws.UpgradeOptions{}, echoUntilClosed)
	tab := NewRequestTab("t")
	tab.URLInput.SetText(url)
	s := tab.EnsureWS()
	tab.WSConnect(context.Background(), nil, nil, nil)
	waitWS(t, s, func() bool { return s.State() == WSStateOpen })

	tab.WSSendProto("   ")
	waitWS(t, s, func() bool {
		for _, m := range s.Messages {
			if m.Dir == ws.DirOut && m.Proto != nil {
				return true
			}
		}
		return false
	})
	for _, m := range wsMessages(s) {
		if m.Error != "" {
			t.Errorf("an empty proto payload must not error: %q", m.Error)
		}
	}
	tab.MarkClosed()
}
