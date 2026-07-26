package workspace

import (
	"image"
	"testing"
	"time"

	"tracto/internal/ws"
)

func newWSRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("wss://echo.test/socket")
	rig.size = image.Pt(1200, 800)
	s := rig.tab.EnsureWS()
	s.OptionsExpanded = true
	s.AddSubprotocol("chat")
	s.ComposerEditor.SetText("hello")
	return rig
}

func TestWSToggleButtons(t *testing.T) {
	cases := []struct {
		name  string
		click func(*WSSession)
		check func(*WSSession) bool
	}{
		{"options", func(s *WSSession) { s.OptionsBtn.Click() }, func(s *WSSession) bool { return !s.OptionsExpanded }},
		{"deflate", func(s *WSSession) { s.OfferDeflateBtn.Click() }, func(s *WSSession) bool { return !s.OfferDeflate }},
		{"msgpack", func(s *WSSession) { s.MsgpackProtoBtn.Click() }, func(s *WSSession) bool { return s.UseMsgpackProto }},
		{"insecure", func(s *WSSession) { s.InsecureBtn.Click() }, func(s *WSSession) bool { return s.InsecureSkipVerify }},
		{"tracto ca", func(s *WSSession) { s.UseTractoCABtn.Click() }, func(s *WSSession) bool { return s.UseTractoCA }},
		{"headers collapse", func(s *WSSession) { s.HeadersCollapseBtn.Click() }, func(s *WSSession) bool { return s.HeadersCollapsed }},
		{"composer wrap", func(s *WSSession) { s.ComposerWrapBtn.Click() }, func(s *WSSession) bool { return !s.ComposerWrap }},
		{"opcode menu", func(s *WSSession) { s.OpcodeMenuBtn.Click() }, func(s *WSSession) bool { return s.OpcodeMenuOpen }},
		{"filter menu", func(s *WSSession) { s.FilterMenuBtn.Click() }, func(s *WSSession) bool { return s.FilterMenuOpen }},
		{"hide ping", func(s *WSSession) { s.FilterPingBtn.Click() }, func(s *WSSession) bool { return s.Filter.HidePing }},
		{"hide pong", func(s *WSSession) { s.FilterPongBtn.Click() }, func(s *WSSession) bool { return s.Filter.HidePong }},
		{"hide close", func(s *WSSession) { s.FilterCloseBtn.Click() }, func(s *WSSession) bool { return s.Filter.HideClose }},
		{"detail hex", func(s *WSSession) { s.DetailHexBtn.Click() }, func(s *WSSession) bool { return s.DetailHex }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newWSRig()
			s := rig.tab.EnsureWS()
			rig.frame()
			c.click(s)
			rig.frame()
			rig.frame()
			if !c.check(s) {
				t.Errorf("%s button did not take effect", c.name)
			}
		})
	}
}

func TestWSOpcodeChoicesSwitchMode(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.OpcodeMenuOpen = true
	rig.frame()

	s.OpcodeBinChoice.Click()
	rig.frame()
	rig.frame()
	if s.OpcodeText {
		t.Error("picking BIN must clear OpcodeText")
	}
	if s.OpcodeMenuOpen {
		t.Error("picking an opcode must close the menu")
	}

	s.OpcodeMenuOpen = true
	rig.frame()
	s.OpcodeTextChoice.Click()
	rig.frame()
	rig.frame()
	if !s.OpcodeText {
		t.Error("picking TEXT must set OpcodeText")
	}
}

func TestWSDetailTextButtonResetsHex(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.DetailHex = true
	rig.frame()
	s.DetailTextBtn.Click()
	rig.frame()
	rig.frame()
	if s.DetailHex {
		t.Error("the TEXT button must turn hex mode off")
	}
}

func TestWSAddAndDeleteSubprotocols(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()

	before := len(s.Subprotocols)
	s.AddSubprotoBtn.Click()
	rig.frame()
	rig.frame()
	if len(s.Subprotocols) != before+1 {
		t.Fatalf("subprotocols = %d, want %d", len(s.Subprotocols), before+1)
	}
	if !s.OptionsExpanded {
		t.Error("adding a subprotocol must expand the options area")
	}

	s.Subprotocols[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(s.Subprotocols) != before {
		t.Errorf("subprotocols = %d after delete, want %d", len(s.Subprotocols), before)
	}
}

func TestWSHeaderAddAndDelete(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.HeadersCollapsed = true
	rig.frame()

	before := len(rig.tab.Headers)
	s.HeadersAddBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Headers) != before+1 {
		t.Fatalf("headers = %d, want %d", len(rig.tab.Headers), before+1)
	}
	if s.HeadersCollapsed {
		t.Error("adding a header must expand the headers area")
	}

	rig.tab.Headers[0].DelBtn.Click()
	rig.frame()
	rig.frame()
	if len(rig.tab.Headers) != before {
		t.Errorf("headers = %d after delete, want %d", len(rig.tab.Headers), before)
	}
}

func TestWSClearButtonDropsMessagesAndSelection(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("a")})
	s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("b")})
	s.Selected = 1
	rig.frame()

	s.ClearBtn.Click()
	rig.frame()
	rig.frame()
	if len(wsMessages(s)) != 0 {
		t.Errorf("Clear left %d messages", len(wsMessages(s)))
	}
	if s.Selected != -1 {
		t.Errorf("Selected = %d, want -1 after Clear", s.Selected)
	}
}

func TestWSDisconnectButtonUsesHostHook(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.setState(WSStateOpen)
	var called int
	rig.tab.WSHost.OnDisconnect = func(*RequestTab) { called++ }
	rig.frame()

	s.DisconnectBtn.Click()
	rig.frame()
	rig.frame()
	if called != 1 {
		t.Errorf("OnDisconnect called %d times, want 1", called)
	}
	if s.State() != WSStateOpen {
		t.Errorf("the host hook must own the teardown, state = %v", s.State())
	}
}

func TestWSPingButtonWithoutConnectionReportsError(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()
	s.PingBtn.Click()
	rig.frame()
	rig.frame()
	msgs := wsMessages(s)
	if len(msgs) != 1 || msgs[0].Error != "Not connected" {
		t.Errorf("messages = %+v, want a single 'Not connected' error", msgs)
	}
}

func TestWSComposerSendIgnoredWhileDisconnected(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	rig.frame()
	s.ComposerSendBtn.Click()
	rig.frame()
	rig.frame()
	if len(wsMessages(s)) != 0 {
		t.Errorf("Send must be inert while disconnected, got %+v", wsMessages(s))
	}
}

func TestWSCopyButtonsRunWithoutPanic(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	s.DetailEditor.SetText("detail")
	rig.frame()
	s.ComposerCopyBtn.Click()
	rig.frame()
	s.DetailCopyBtn.Click()
	rig.frame()
	rig.frame()
}

func TestWSMessageListRendersEveryOpcode(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	now := time.Unix(1700000000, 0)
	s.Messages = []WSDisplayMessage{
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpText, Payload: []byte("out")},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpBinary, Payload: []byte{0, 1, 2}},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpPing, Payload: []byte("p")},
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpPong, Payload: []byte("p")},
		{Time: now, Dir: ws.DirIn, Opcode: ws.OpClose, Payload: []byte{0x03, 0xe8, 'x'}},
		{Time: now, Note: "Connected"},
		{Time: now, Error: "Read: boom"},
		{Time: now, Dir: ws.DirOut, Opcode: ws.OpBinary, Payload: []byte{9},
			Proto: &ProtoView{Cmd: 1, Seq: 2, Opcode: 3, JSON: `{"k":1}`}},
	}

	for _, sel := range []int{-1, 0, 4, 5, 6, 7} {
		s.Selected = sel
		for i := 0; i < 2; i++ {
			rig.frame()
		}
	}

	s.Filter.HidePing = true
	s.Filter.HidePong = true
	s.Filter.HideClose = true
	s.Selected = 0
	for i := 0; i < 2; i++ {
		rig.frame()
	}
}

func TestWSLayoutCollapsedSectionsAndSizes(t *testing.T) {
	sizes := []image.Point{{X: 1400, Y: 900}, {X: 700, Y: 500}, {X: 380, Y: 300}}
	for _, sz := range sizes {
		rig := newWSRig()
		rig.size = sz
		s := rig.tab.EnsureWS()
		s.appendMessage(WSDisplayMessage{Opcode: ws.OpText, Payload: []byte("x")})
		s.Selected = 0
		for _, collapse := range []bool{false, true} {
			s.HeadersCollapsed = collapse
			s.ComposeCollapsed = collapse
			s.MessagesCollapsed = collapse
			for i := 0; i < 2; i++ {
				rig.frame()
			}
		}
	}
}

func TestWSStatusBarReflectsSessionState(t *testing.T) {
	rig := newWSRig()
	s := rig.tab.EnsureWS()
	for _, st := range []WSState{WSStateIdle, WSStateConnecting, WSStateOpen, WSStateClosing, WSStateClosed} {
		s.setState(st)
		s.setStatus(st.String(), st == WSStateClosed)
		for i := 0; i < 2; i++ {
			rig.frame()
		}
	}
	s.setState(WSStateOpen)
	s.setConnInfo(nil, "chat", ws.ExtParams{Negotiated: true})
	rig.frame()
	if got := s.formatNegotiated(); got == "" {
		t.Error("an open negotiated session must describe its handshake")
	}
}
