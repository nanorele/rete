package workspace

import (
	"sync"
	"sync/atomic"
	"time"

	"tracto/internal/ws"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
)

const MethodWS = "WS"

type WSState int32

const (
	WSStateIdle WSState = iota
	WSStateConnecting
	WSStateOpen
	WSStateClosing
	WSStateClosed
)

func (s WSState) String() string {
	switch s {
	case WSStateIdle:
		return "Idle"
	case WSStateConnecting:
		return "Connecting"
	case WSStateOpen:
		return "Open"
	case WSStateClosing:
		return "Closing"
	case WSStateClosed:
		return "Closed"
	}
	return "?"
}

type WSDisplayMessage struct {
	Time    time.Time
	Dir     ws.Dir
	Opcode  ws.Opcode
	Payload []byte
	Session int
	Note    string
	Error   string
}

type WSSavedSend struct {
	Name    string
	Text    string
	Opcode  ws.Opcode
	SendBtn widget.Clickable
	EditBtn widget.Clickable
	DelBtn  widget.Clickable
	UseBtn  widget.Clickable
}

type WSSubprotoItem struct {
	Editor widget.Editor
	DelBtn widget.Clickable
}

type WSFilter struct {
	HidePing  bool
	HidePong  bool
	HideClose bool
}

type WSSession struct {
	DisconnectBtn widget.Clickable
	PingBtn       widget.Clickable
	ClearBtn      widget.Clickable

	Subprotocols       []*WSSubprotoItem
	SubprotosList      widget.List
	SubprotosAbsHeight int
	FitSubprotos       bool
	AddSubprotoBtn     widget.Clickable
	OptionsExpanded    bool
	OptionsBtn         widget.Clickable
	OfferDeflate       bool
	OfferDeflateBtn    widget.Clickable
	InsecureSkipVerify bool
	InsecureBtn        widget.Clickable
	UseTractoCA        bool
	UseTractoCABtn     widget.Clickable

	ComposerEditor    widget.Editor
	OpcodeText        bool
	OpcodeMenuBtn     widget.Clickable
	OpcodeMenuOpen    bool
	OpcodeTextChoice  widget.Clickable
	OpcodeBinChoice   widget.Clickable
	ComposerWrap      bool
	ComposerWrapBtn   widget.Clickable
	ComposerCopyBtn   widget.Clickable
	ComposerSendBtn   widget.Clickable

	SavedSends     []*WSSavedSend
	SavedSendsList widget.List

	Messages       []WSDisplayMessage
	MessagesList   widget.List
	Filter         WSFilter
	FilterMenuBtn  widget.Clickable
	FilterMenuOpen bool
	FilterPingBtn  widget.Clickable
	FilterPongBtn  widget.Clickable
	FilterCloseBtn widget.Clickable

	RowClicks       []*widget.Clickable
	Selected        int
	DetailHex       bool
	DetailTextBtn   widget.Clickable
	DetailHexBtn    widget.Clickable
	DetailCopyBtn   widget.Clickable
	DetailEditor    widget.Editor
	DetailSrcID     int
	DetailSrcHex    bool

	SplitRatio    float32
	SplitDrag     gesture.Drag
	SplitDragX    float32
	ComposerRatio float32

	state         atomic.Int32
	sessionCount  int
	sessionMu     sync.Mutex
	conn          *ws.Conn
	cancel        func()
	notify        *wsDebouncer
	closed        atomic.Bool
	statusText    string
	statusErr     bool
	subprotocol   string
	negotiatedExt ws.ExtParams
}

func newWSSession() *WSSession {
	s := &WSSession{
		OpcodeText:    true,
		OfferDeflate:  true,
		ComposerWrap:  true,
		SplitRatio:    0.4,
		ComposerRatio: 0.5,
		Selected:      -1,
		DetailSrcID:   -1,
	}
	s.ComposerEditor.Submit = false
	s.DetailEditor.ReadOnly = true
	s.SubprotosList.Axis = layout.Vertical
	s.MessagesList.Axis = layout.Vertical
	return s
}

func (s *WSSession) State() WSState { return WSState(s.state.Load()) }

func (s *WSSession) setState(st WSState) { s.state.Store(int32(st)) }

func (s *WSSession) AddSubprotocol(name string) {
	item := &WSSubprotoItem{}
	item.Editor.SingleLine = true
	item.Editor.SetText(name)
	s.Subprotocols = append(s.Subprotocols, item)
}

func (s *WSSession) SubprotocolList() []string {
	out := make([]string, 0, len(s.Subprotocols))
	for _, it := range s.Subprotocols {
		v := trimSpaceLocal(it.Editor.Text())
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (s *WSSession) AppendSavedSend(name, text string, op ws.Opcode) {
	s.SavedSends = append(s.SavedSends, &WSSavedSend{
		Name:   name,
		Text:   text,
		Opcode: op,
	})
}

func (s *WSSession) markClosed() {
	if s.closed.Swap(true) {
		return
	}
	s.sessionMu.Lock()
	cancel := s.cancel
	conn := s.conn
	s.sessionMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *WSSession) setStatus(text string, isErr bool) {
	s.sessionMu.Lock()
	s.statusText = text
	s.statusErr = isErr
	s.sessionMu.Unlock()
}

func (s *WSSession) statusSnapshot() (string, bool) {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.statusText, s.statusErr
}

func (s *WSSession) setConnInfo(conn *ws.Conn, sub string, ext ws.ExtParams) {
	s.sessionMu.Lock()
	s.conn = conn
	s.subprotocol = sub
	s.negotiatedExt = ext
	s.sessionMu.Unlock()
}

func (s *WSSession) getConn() *ws.Conn {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.conn
}

func (s *WSSession) StatusText() string  { t, _ := s.statusSnapshot(); return t }
func (s *WSSession) StatusIsError() bool { _, e := s.statusSnapshot(); return e }
func (s *WSSession) Subprotocol() string {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.subprotocol
}
func (s *WSSession) NegotiatedExtensions() ws.ExtParams {
	s.sessionMu.Lock()
	defer s.sessionMu.Unlock()
	return s.negotiatedExt
}

func trimSpaceLocal(s string) string {
	start, end := 0, len(s)
	for start < end && isSpaceByte(s[start]) {
		start++
	}
	for end > start && isSpaceByte(s[end-1]) {
		end--
	}
	return s[start:end]
}

func isSpaceByte(b byte) bool { return b == ' ' || b == '\t' || b == '\n' || b == '\r' }
