package workspace

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"tracto/internal/ws"
	"tracto/internal/wsproto"

	"github.com/nanorele/gio/app"
)

type wsDebouncer struct {
	win   *app.Window
	armed atomic.Bool
}

func newWSDebouncer(win *app.Window) *wsDebouncer { return &wsDebouncer{win: win} }

func (d *wsDebouncer) trigger() {
	if d == nil || d.win == nil {
		return
	}
	if d.armed.Swap(true) {
		return
	}
	win := d.win
	time.AfterFunc(16*time.Millisecond, func() {
		d.armed.Store(false)
		win.Invalidate()
	})
}

func (t *RequestTab) EnsureWS() *WSSession {
	if t.WS == nil {
		t.WS = newWSSession()
	}
	return t.WS
}

// WSMenuOpen reports whether any websocket popup menu (opcode/filter) is open.
func (t *RequestTab) WSMenuOpen() bool {
	return t.WS != nil && (t.WS.OpcodeMenuOpen || t.WS.FilterMenuOpen)
}

// CloseWSMenus closes the websocket popup menus.
func (t *RequestTab) CloseWSMenus() {
	if t.WS != nil {
		t.WS.OpcodeMenuOpen = false
		t.WS.FilterMenuOpen = false
	}
}

func (t *RequestTab) AttachWSWindow(win *app.Window) {
	s := t.EnsureWS()
	if s.notify == nil {
		s.notify = newWSDebouncer(win)
	}
}

func (t *RequestTab) WSConnect(ctx context.Context, tlsCfg *tls.Config, env map[string]string, extraHeaders http.Header) {
	s := t.EnsureWS()
	if s.State() == WSStateConnecting || s.State() == WSStateOpen {
		return
	}
	raw := strings.TrimSpace(t.URLInput.Text())
	if raw == "" {
		s.appendError("URL is empty")
		return
	}
	url := processTemplate(raw, env)
	if strings.Contains(url, "{{") {
		s.appendError("URL has unresolved variables: " + url)
		return
	}
	s.sessionMu.Lock()
	s.sessionCount++
	session := s.sessionCount
	s.sessionMu.Unlock()

	s.setState(WSStateConnecting)
	s.setStatus("Connecting to "+url+"…", false)
	if s.notify != nil {
		s.notify.trigger()
	}

	dialCtx, cancel := context.WithCancel(ctx)
	s.sessionMu.Lock()
	s.cancel = cancel
	s.sessionMu.Unlock()

	handshakeHeaders := t.wsHandshakeHeaders(env, extraHeaders)
	if handshakeHeaders.Get("Origin") == "" {
		if origin := defaultOrigin(url); origin != "" {
			if handshakeHeaders == nil {
				handshakeHeaders = http.Header{}
			}
			handshakeHeaders.Set("Origin", origin)
		}
	}

	go func() {
		opts := ws.DialOptions{
			TLSConfig:    tlsCfg,
			Subprotocols: s.SubprotocolList(),
			Headers:      handshakeHeaders,
			OfferDeflate: s.OfferDeflate,
			DialTimeout:  15 * time.Second,
		}
		res, err := ws.Dial(dialCtx, url, opts)
		if err != nil {
			s.appendError(formatDialError(err, res))
			s.setState(WSStateClosed)
			s.setStatus("Connection failed", true)
			cancel()
			if s.notify != nil {
				s.notify.trigger()
			}
			return
		}
		if dialCtx.Err() != nil {
			_ = res.Conn.Close()
			s.setState(WSStateClosed)
			s.setStatus("Disconnected", false)
			if s.notify != nil {
				s.notify.trigger()
			}
			return
		}
		s.setConnInfo(res.Conn, res.Subprotocol, res.Extensions)
		s.setState(WSStateOpen)
		s.setStatus("Connected", false)
		s.appendNote(session, "Connected • status="+res.Response.Status+suffixFromExt(res))
		if s.notify != nil {
			s.notify.trigger()
		}
		s.readLoop(dialCtx, session)
	}()
}

func defaultOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := "https"
	if s := strings.ToLower(u.Scheme); s == "ws" || s == "http" {
		scheme = "http"
	}
	host := u.Host
	if h, p, err := net.SplitHostPort(host); err == nil {
		if (scheme == "https" && p == "443") || (scheme == "http" && p == "80") {
			host = h
		}
	}
	return scheme + "://" + host
}

func (t *RequestTab) wsHandshakeHeaders(env map[string]string, extra http.Header) http.Header {
	h := http.Header{}
	for _, it := range t.Headers {
		if it.IsGenerated {
			continue
		}
		k := strings.TrimSpace(processTemplate(it.Key.Text(), env))
		if k == "" {
			continue
		}
		h.Add(k, processTemplate(it.Value.Text(), env))
	}
	for k, vs := range extra {
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	if len(h) == 0 {
		return nil
	}
	return h
}

func formatDialError(err error, res *ws.DialResult) string {
	if res == nil || res.Response == nil {
		return "Dial failed: " + err.Error()
	}
	status := res.Response.Status
	ct := strings.ToLower(res.Response.Header.Get("Content-Type"))
	body := string(res.ResponseBody)
	body = strings.TrimSpace(body)
	hint := ""
	switch {
	case strings.HasPrefix(ct, "text/html"):
		hint = " — endpoint returned HTML, not a WebSocket upgrade"
	case strings.HasPrefix(ct, "application/json"):
		hint = " — endpoint returned JSON, not a WebSocket upgrade"
	case res.Response.StatusCode >= 400:
		hint = " — server refused the upgrade"
	}
	msg := "Handshake rejected: " + status + hint
	if body != "" {
		if len(body) > 240 {
			body = body[:240] + "…"
		}
		msg += "\n" + body
	}
	return msg
}

func suffixFromExt(res *ws.DialResult) string {
	var b strings.Builder
	if res.Subprotocol != "" {
		b.WriteString(" • subprotocol=")
		b.WriteString(res.Subprotocol)
	}
	if res.Extensions.Negotiated {
		b.WriteString(" • permessage-deflate")
		if res.Extensions.ServerNoContextTakeover {
			b.WriteString(" (server_no_context_takeover)")
		}
		if res.Extensions.ClientNoContextTakeover {
			b.WriteString(" (client_no_context_takeover)")
		}
	}
	return b.String()
}

func (t *RequestTab) WSDisconnect() {
	if t.WS == nil {
		return
	}
	s := t.WS
	if s.State() != WSStateOpen && s.State() != WSStateConnecting {
		return
	}
	s.setState(WSStateClosing)
	s.setStatus("Disconnecting…", false)
	s.sessionMu.Lock()
	conn := s.conn
	cancel := s.cancel
	s.sessionMu.Unlock()
	if conn != nil {
		_ = conn.WriteClose(ws.CloseNormal, "client closing")
	}
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close()
	}
}

func (t *RequestTab) WSSendText(text string) { t.wsSend(ws.OpText, []byte(text)) }

func (t *RequestTab) WSSendBinary(payload []byte) { t.wsSend(ws.OpBinary, payload) }

func (t *RequestTab) WSSendPing() {
	t.wsSend(ws.OpPing, []byte("ping "+time.Now().Format("15:04:05.000")))
}

func (t *RequestTab) wsSend(op ws.Opcode, payload []byte) {
	s := t.EnsureWS()
	conn := s.getConn()
	if s.State() != WSStateOpen || conn == nil {
		s.appendError("Not connected")
		return
	}
	if err := conn.WriteMessage(op, payload); err != nil {
		if isNormalCloseErr(context.Background(), err) {
			s.appendNote(s.sessionCount, "Connection closed")
			s.setStatus("Disconnected", false)
			return
		}
		s.appendError("Write failed: " + err.Error())
		return
	}
	s.appendMessage(WSDisplayMessage{
		Time:    time.Now(),
		Dir:     ws.DirOut,
		Opcode:  op,
		Payload: payload,
		Session: s.sessionCount,
	})
}

func (t *RequestTab) WSSendProto(jsonText string) {
	s := t.EnsureWS()
	conn := s.getConn()
	if s.State() != WSStateOpen || conn == nil {
		s.appendError("Not connected")
		return
	}
	cmd, seq, opcode, err := s.protoHeaderFields()
	if err != nil {
		s.appendError(err.Error())
		return
	}
	var payload any
	if txt := strings.TrimSpace(jsonText); txt != "" {
		if err := json.Unmarshal([]byte(txt), &payload); err != nil {
			s.appendError("JSON parse: " + err.Error())
			return
		}
	}
	raw, meta, err := wsproto.Encode(wsproto.Frame{Cmd: cmd, Seq: seq, Opcode: opcode, Payload: payload})
	if err != nil {
		s.appendError("Encode: " + err.Error())
		return
	}
	if err := conn.WriteMessage(ws.OpBinary, raw); err != nil {
		if isNormalCloseErr(context.Background(), err) {
			s.appendNote(s.sessionCount, "Connection closed")
			s.setStatus("Disconnected", false)
			return
		}
		s.appendError("Write failed: " + err.Error())
		return
	}
	view := &ProtoView{Cmd: cmd, Seq: seq, Opcode: opcode, Cof: meta.Cof, BodyLen: meta.BodyLen, RawLen: meta.RawLen}
	if js, jerr := wsproto.MarshalJSON(payload); jerr == nil {
		view.JSON = js
	} else {
		view.DecodeErr = jerr.Error()
	}
	s.appendMessage(WSDisplayMessage{
		Time:    time.Now(),
		Dir:     ws.DirOut,
		Opcode:  ws.OpBinary,
		Payload: raw,
		Session: s.sessionCount,
		Proto:   view,
	})
}

func (s *WSSession) protoHeaderFields() (uint8, int16, int16, error) {
	cmd, err := parseProtoInt(s.ProtoCmdEditor.Text(), 0, 255)
	if err != nil {
		return 0, 0, 0, errors.New("cmd: " + err.Error())
	}
	seq, err := parseProtoInt(s.ProtoSeqEditor.Text(), -32768, 32767)
	if err != nil {
		return 0, 0, 0, errors.New("seq: " + err.Error())
	}
	opcode, err := parseProtoInt(s.ProtoOpcodeEditor.Text(), -32768, 32767)
	if err != nil {
		return 0, 0, 0, errors.New("opcode: " + err.Error())
	}
	return uint8(cmd), int16(seq), int16(opcode), nil
}

func parseProtoInt(text string, lo, hi int) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, errors.New("not a number")
	}
	if n < lo || n > hi {
		return 0, errors.New("out of range")
	}
	return n, nil
}

func decodeProtoView(payload []byte) *ProtoView {
	data, meta, err := wsproto.Decode(payload)
	view := &ProtoView{Cmd: meta.Cmd, Seq: meta.Seq, Opcode: meta.Opcode, Cof: meta.Cof, BodyLen: meta.BodyLen, RawLen: meta.RawLen}
	if err != nil {
		view.DecodeErr = err.Error()
		return view
	}
	if js, jerr := wsproto.MarshalJSON(data); jerr == nil {
		view.JSON = js
	} else {
		view.DecodeErr = jerr.Error()
	}
	return view
}

func (s *WSSession) readLoop(ctx context.Context, session int) {
	conn := s.getConn()
	defer func() {
		s.setState(WSStateClosed)
		if conn != nil {
			_ = conn.Close()
		}
		if text, isErr := s.statusSnapshot(); text == "" || !isErr {
			s.setStatus("Disconnected", false)
		}
		if s.notify != nil {
			s.notify.trigger()
		}
	}()
	if conn == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			return
		}
		op, payload, err := conn.ReadMessage()
		if err != nil {
			if isNormalCloseErr(ctx, err) {
				s.appendNote(session, "Connection closed")
				s.setStatus("Disconnected", false)
				return
			}
			s.appendError("Read: " + err.Error())
			s.setStatus("Disconnected", true)
			return
		}
		in := WSDisplayMessage{
			Time:    time.Now(),
			Dir:     ws.DirIn,
			Opcode:  op,
			Payload: payload,
			Session: session,
		}
		if op == ws.OpBinary && s.UseMsgpackProto {
			in.Proto = decodeProtoView(payload)
		}
		s.appendMessage(in)
		if op == ws.OpClose {
			code, reason := ws.ParseClosePayload(payload)
			s.appendNote(session, formatPeerClose(code, reason))
			s.setStatus("Closed by peer", isAbnormalCloseCode(code))
			if conn != nil {
				_ = conn.WriteClose(ws.CloseNormal, "")
			}
			return
		}
		if op == ws.OpPing {
			if conn != nil {
				_ = conn.WriteMessage(ws.OpPong, payload)
				s.appendMessage(WSDisplayMessage{
					Time:    time.Now(),
					Dir:     ws.DirOut,
					Opcode:  ws.OpPong,
					Payload: payload,
					Session: session,
					Note:    "auto-pong",
				})
			}
		}
	}
}

func isNormalCloseErr(ctx context.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, ws.ErrConnClosed) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	if ctx.Err() != nil {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "use of closed network connection") {
		return true
	}
	if strings.Contains(msg, "connection reset by peer") {
		return true
	}
	if strings.Contains(msg, "broken pipe") {
		return true
	}
	return false
}

func isAbnormalCloseCode(code ws.CloseCode) bool {
	switch code {
	case ws.CloseNormal, ws.CloseGoingAway, ws.CloseNoStatusRcvd:
		return false
	}
	return true
}

func formatPeerClose(code ws.CloseCode, reason string) string {
	if reason == "" {
		return "Closed by peer (code=" + itoa(int(code)) + ")"
	}
	return "Closed by peer (code=" + itoa(int(code)) + ", reason=" + reason + ")"
}

func (s *WSSession) appendMessage(m WSDisplayMessage) {
	s.sessionMu.Lock()
	s.Messages = append(s.Messages, m)
	s.sessionMu.Unlock()
	if s.notify != nil {
		s.notify.trigger()
	}
}

func (s *WSSession) appendError(msg string) {
	s.sessionMu.Lock()
	s.Messages = append(s.Messages, WSDisplayMessage{
		Time:    time.Now(),
		Session: s.sessionCount,
		Error:   msg,
	})
	s.sessionMu.Unlock()
	if s.notify != nil {
		s.notify.trigger()
	}
}

func (s *WSSession) appendNote(session int, note string) {
	s.sessionMu.Lock()
	s.Messages = append(s.Messages, WSDisplayMessage{
		Time:    time.Now(),
		Session: session,
		Note:    note,
	})
	s.sessionMu.Unlock()
}

func (s *WSSession) ClearMessages() {
	s.sessionMu.Lock()
	s.Messages = nil
	s.sessionMu.Unlock()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
