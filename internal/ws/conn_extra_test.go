package ws

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func clientConn(t *testing.T, r io.Reader) *Conn {
	t.Helper()
	return &Conn{br: bufio.NewReader(r), isClient: true}
}

func serverConn(t *testing.T, r io.Reader) *Conn {
	t.Helper()
	return &Conn{br: bufio.NewReader(r), isClient: false}
}

func frameBytes(t *testing.T, hdr Header, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteFrame(&buf, hdr, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	return buf.Bytes()
}

func TestOpcodeString(t *testing.T) {
	tests := []struct {
		op   Opcode
		want string
	}{
		{OpContinuation, "CONT"},
		{OpText, "TEXT"},
		{OpBinary, "BIN"},
		{OpClose, "CLOSE"},
		{OpPing, "PING"},
		{OpPong, "PONG"},
		{0x3, "OP?"},
		{0x7, "OP?"},
		{0xB, "OP?"},
		{0xF, "OP?"},
	}
	for _, tt := range tests {
		if got := tt.op.String(); got != tt.want {
			t.Errorf("Opcode(%#x).String() = %q, want %q", byte(tt.op), got, tt.want)
		}
	}
}

func TestOpcodeClassification(t *testing.T) {
	tests := []struct {
		op        Opcode
		isControl bool
		isData    bool
	}{
		{OpContinuation, false, true},
		{OpText, false, true},
		{OpBinary, false, true},
		{0x3, false, false},
		{0x7, false, false},
		{OpClose, true, false},
		{OpPing, true, false},
		{OpPong, true, false},
		{0xB, true, false},
		{0xF, true, false},
	}
	for _, tt := range tests {
		if got := tt.op.IsControl(); got != tt.isControl {
			t.Errorf("Opcode(%#x).IsControl() = %v, want %v", byte(tt.op), got, tt.isControl)
		}
		if got := tt.op.IsData(); got != tt.isData {
			t.Errorf("Opcode(%#x).IsData() = %v, want %v", byte(tt.op), got, tt.isData)
		}
	}
}

func TestDirString(t *testing.T) {
	if got := DirOut.String(); got != "OUT" {
		t.Errorf("DirOut.String() = %q, want OUT", got)
	}
	if got := DirIn.String(); got != "IN" {
		t.Errorf("DirIn.String() = %q, want IN", got)
	}
	if got := Dir(99).String(); got != "IN" {
		t.Errorf("Dir(99).String() = %q, want IN", got)
	}
}

func TestClosePayloadRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		code   CloseCode
		reason string
	}{
		{"normal", CloseNormal, "bye"},
		{"going away no reason", CloseGoingAway, ""},
		{"protocol error", CloseProtocolError, "bad frame"},
		{"policy violation", ClosePolicyViolation, "nope"},
		{"too big", CloseMessageTooBig, strings.Repeat("x", 100)},
		{"unicode reason", CloseInvalidPayload, "причина"},
		{"max code", CloseCode(65535), "hi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := MakeClosePayload(tt.code, tt.reason)
			if len(p) != 2+len(tt.reason) {
				t.Fatalf("payload len = %d, want %d", len(p), 2+len(tt.reason))
			}
			code, reason := ParseClosePayload(p)
			if code != tt.code {
				t.Errorf("code = %d, want %d", code, tt.code)
			}
			if reason != tt.reason {
				t.Errorf("reason = %q, want %q", reason, tt.reason)
			}
		})
	}
}

func TestParseClosePayloadShort(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"nil", nil},
		{"empty", []byte{}},
		{"one byte", []byte{0x03}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, reason := ParseClosePayload(tt.in)
			if code != CloseNoStatusRcvd {
				t.Errorf("code = %d, want CloseNoStatusRcvd", code)
			}
			if reason != "" {
				t.Errorf("reason = %q, want empty", reason)
			}
		})
	}
}

func TestReadFrameTruncatedInput(t *testing.T) {
	full := frameBytes(t, Header{FIN: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{1, 2, 3, 4}},
		bytes.Repeat([]byte("payload"), 100))
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, len(full) - 1} {
		_, _, err := ReadFrame(bytes.NewReader(full[:n]))
		if err == nil {
			t.Errorf("prefix len %d: expected error", n)
			continue
		}
		if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("prefix len %d: err = %v, want EOF-ish", n, err)
		}
	}
}

func TestReadFrameTruncatedExtendedLength(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"16-bit length cut", []byte{0x81, 126, 0x01}},
		{"64-bit length cut", []byte{0x81, 127, 0, 0, 0}},
		{"mask key cut", []byte{0x81, 0x81, 0xAA, 0xBB}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrame(bytes.NewReader(tt.in))
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestReadFrameRSVBits(t *testing.T) {
	tests := []struct {
		name             string
		rsv1, rsv2, rsv3 bool
	}{
		{"none", false, false, false},
		{"rsv1", true, false, false},
		{"rsv2", false, true, false},
		{"rsv3", false, false, true},
		{"all", true, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := Header{FIN: true, RSV1: tt.rsv1, RSV2: tt.rsv2, RSV3: tt.rsv3, Opcode: OpBinary}
			raw := frameBytes(t, in, []byte("x"))
			got, payload, err := ReadFrame(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			if got.RSV1 != tt.rsv1 || got.RSV2 != tt.rsv2 || got.RSV3 != tt.rsv3 {
				t.Errorf("rsv roundtrip = %v/%v/%v, want %v/%v/%v",
					got.RSV1, got.RSV2, got.RSV3, tt.rsv1, tt.rsv2, tt.rsv3)
			}
			if string(payload) != "x" {
				t.Errorf("payload = %q", payload)
			}
		})
	}
}

func TestReadFrameControlValidation(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		wantErr error
	}{
		{"ping not final", []byte{0x09, 0x00}, ErrControlFrameNotFinal},
		{"close not final", []byte{0x08, 0x00}, ErrControlFrameNotFinal},
		{"pong not final", []byte{0x0A, 0x00}, ErrControlFrameNotFinal},
		{"ping 126 bytes", append([]byte{0x89, 126, 0x00, 126}, make([]byte, 126)...), ErrControlFrameTooLong},
		{"ping 65535 bytes", []byte{0x89, 126, 0xFF, 0xFF}, ErrControlFrameTooLong},
		{"ping 125 bytes ok", append([]byte{0x89, 125}, make([]byte, 125)...), nil},
		{"ping empty ok", []byte{0x89, 0x00}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ReadFrame(bytes.NewReader(tt.raw))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestWriteFrameLengthEncoding(t *testing.T) {
	tests := []struct {
		name       string
		size       int
		wantHeader int
		wantLenBy  byte
	}{
		{"zero", 0, 2, 0},
		{"125", 125, 2, 125},
		{"126 uses 16-bit", 126, 4, 126},
		{"65535 uses 16-bit", 65535, 4, 126},
		{"65536 uses 64-bit", 65536, 10, 127},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := frameBytes(t, Header{FIN: true, Opcode: OpBinary}, make([]byte, tt.size))
			if len(raw) != tt.wantHeader+tt.size {
				t.Fatalf("frame len = %d, want %d", len(raw), tt.wantHeader+tt.size)
			}
			if raw[1]&0x7F != tt.wantLenBy {
				t.Errorf("length byte = %d, want %d", raw[1]&0x7F, tt.wantLenBy)
			}
			if raw[1]&0x80 != 0 {
				t.Error("mask bit set on unmasked frame")
			}
		})
	}
}

func TestWriteFrameMaskedLengths(t *testing.T) {
	for _, size := range []int{0, 1, 125, 126, 65535, 65536} {
		payload := make([]byte, size)
		for i := range payload {
			payload[i] = byte(i)
		}
		orig := append([]byte{}, payload...)
		hdr := Header{FIN: true, Opcode: OpBinary, Masked: true, MaskKey: [4]byte{0xDE, 0xAD, 0xBE, 0xEF}}
		raw := frameBytes(t, hdr, payload)
		if !bytes.Equal(payload, orig) {
			t.Fatalf("size=%d: WriteFrame mutated caller payload", size)
		}
		if size > 0 && raw[1]&0x80 == 0 {
			t.Errorf("size=%d: mask bit not set", size)
		}
		got, decoded, err := ReadFrame(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("size=%d: %v", size, err)
		}
		if !got.Masked {
			t.Errorf("size=%d: Masked = false, want true", size)
		}
		if got.MaskKey != hdr.MaskKey {
			t.Errorf("size=%d: MaskKey = %v, want %v", size, got.MaskKey, hdr.MaskKey)
		}
		if !bytes.Equal(decoded, orig) {
			t.Errorf("size=%d: payload roundtrip mismatch", size)
		}
	}
}

func TestWriteFrameWriteErrors(t *testing.T) {
	tests := []struct {
		name   string
		hdr    Header
		body   []byte
		failAt int
	}{
		{"header write fails", Header{FIN: true, Opcode: OpText}, []byte("abc"), 0},
		{"payload write fails", Header{FIN: true, Opcode: OpText}, []byte("abc"), 1},
		{"masked payload write fails", Header{FIN: true, Opcode: OpText, Masked: true}, []byte("abc"), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &failWriter{failAfter: tt.failAt}
			if err := WriteFrame(w, tt.hdr, tt.body); err == nil {
				t.Fatal("expected write error")
			}
		})
	}
}

type failWriter struct {
	n         int
	failAfter int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.n >= w.failAfter {
		return 0, errors.New("write failed")
	}
	w.n++
	return len(p), nil
}

func TestWriteFrameEmptyMaskedPayloadOmitsBody(t *testing.T) {
	raw := frameBytes(t, Header{FIN: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{1, 2, 3, 4}}, nil)
	if len(raw) != 6 {
		t.Fatalf("frame len = %d, want 6 (2 header + 4 mask)", len(raw))
	}
	hdr, payload, err := ReadFrame(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !hdr.Masked || len(payload) != 0 {
		t.Errorf("hdr.Masked=%v len(payload)=%d", hdr.Masked, len(payload))
	}
}

func TestApplyMaskIsInvolution(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{{"empty", 0}, {"one", 1}, {"three", 3}, {"four", 4}, {"five", 5}, {"large", 1000}}
	key := [4]byte{0x12, 0x34, 0x56, 0x78}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.size)
			for i := range data {
				data[i] = byte(i * 7)
			}
			orig := append([]byte{}, data...)
			applyMask(data, key)
			if tt.size >= 4 && bytes.Equal(data, orig) {
				t.Error("mask was a no-op")
			}
			applyMask(data, key)
			if !bytes.Equal(data, orig) {
				t.Error("double mask did not restore original")
			}
		})
	}
}

func TestConnMaskingDirectionRules(t *testing.T) {
	tests := []struct {
		name     string
		isClient bool
		hdr      Header
		payload  []byte
		wantErr  error
	}{
		{
			name: "client rejects masked server frame", isClient: true,
			hdr:     Header{FIN: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{1, 2, 3, 4}},
			payload: []byte("hi"), wantErr: ErrMaskedFromServer,
		},
		{
			name: "client rejects masked control frame", isClient: true,
			hdr:     Header{FIN: true, Opcode: OpPing, Masked: true, MaskKey: [4]byte{1, 2, 3, 4}},
			payload: []byte("p"), wantErr: ErrMaskedFromServer,
		},
		{
			name: "client accepts unmasked server frame", isClient: true,
			hdr: Header{FIN: true, Opcode: OpText}, payload: []byte("hi"), wantErr: nil,
		},
		{
			name: "server rejects unmasked client data", isClient: false,
			hdr: Header{FIN: true, Opcode: OpText}, payload: []byte("hi"), wantErr: ErrUnmaskedFromClient,
		},
		{
			name: "server rejects unmasked client binary", isClient: false,
			hdr: Header{FIN: true, Opcode: OpBinary}, payload: []byte{1}, wantErr: ErrUnmaskedFromClient,
		},
		{
			name: "server accepts masked client data", isClient: false,
			hdr:     Header{FIN: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{9, 8, 7, 6}},
			payload: []byte("hi"), wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := frameBytes(t, tt.hdr, tt.payload)
			var c *Conn
			if tt.isClient {
				c = clientConn(t, bytes.NewReader(raw))
			} else {
				c = serverConn(t, bytes.NewReader(raw))
			}
			_, payload, err := c.ReadMessage()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && !bytes.Equal(payload, tt.payload) {
				t.Errorf("payload = %q, want %q", payload, tt.payload)
			}
		})
	}
}

func TestConnReadMessagePropagatesReassemblyErrors(t *testing.T) {
	tests := []struct {
		name    string
		frames  []Header
		bodies  [][]byte
		wantErr error
	}{
		{
			name:    "continuation without open message",
			frames:  []Header{{FIN: true, Opcode: OpContinuation}},
			bodies:  [][]byte{[]byte("orphan")},
			wantErr: ErrUnexpectedContinuation,
		},
		{
			name:    "two open data frames",
			frames:  []Header{{Opcode: OpText}, {Opcode: OpText}},
			bodies:  [][]byte{[]byte("a"), []byte("b")},
			wantErr: ErrUnexpectedDataFrame,
		},
		{
			name:    "rsv1 on continuation",
			frames:  []Header{{Opcode: OpText}, {FIN: true, RSV1: true, Opcode: OpContinuation}},
			bodies:  [][]byte{[]byte("a"), []byte("b")},
			wantErr: ErrUnexpectedRSV1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			for i, h := range tt.frames {
				if err := WriteFrame(&buf, h, tt.bodies[i]); err != nil {
					t.Fatal(err)
				}
			}
			c := clientConn(t, &buf)
			_, _, err := c.ReadMessage()
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestConnReadMessageFragmentedWithInterleavedControl(t *testing.T) {
	var buf bytes.Buffer
	write := func(h Header, p []byte) {
		if err := WriteFrame(&buf, h, p); err != nil {
			t.Fatal(err)
		}
	}
	write(Header{Opcode: OpText}, []byte("frag-"))
	write(Header{FIN: true, Opcode: OpPing}, []byte("ping1"))
	write(Header{Opcode: OpContinuation}, []byte("ment-"))
	write(Header{FIN: true, Opcode: OpPong}, []byte("pong1"))
	write(Header{FIN: true, Opcode: OpContinuation}, []byte("end"))

	c := clientConn(t, &buf)
	want := []struct {
		op      Opcode
		payload string
	}{
		{OpPing, "ping1"},
		{OpPong, "pong1"},
		{OpText, "frag-ment-end"},
	}
	for i, w := range want {
		op, p, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if op != w.op || string(p) != w.payload {
			t.Errorf("message %d: got op=%v %q, want op=%v %q", i, op, p, w.op, w.payload)
		}
	}
}

func TestConnReadMessageEOF(t *testing.T) {
	c := clientConn(t, bytes.NewReader(nil))
	op, p, err := c.ReadMessage()
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
	if op != 0 || p != nil {
		t.Errorf("got op=%v payload=%v, want zero values", op, p)
	}
}

func TestConnUnderlyingAndClose(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c, err := NewConn(a, nil, true, ExtParams{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Underlying() != a {
		t.Error("Underlying did not return the wrapped net.Conn")
	}
	if c.isDead() {
		t.Error("fresh conn reported dead")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !c.isDead() {
		t.Error("closed conn not reported dead")
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v, want nil", err)
	}
	if err := c.WriteMessage(OpText, []byte("x")); !errors.Is(err, ErrConnClosed) {
		t.Errorf("write after close err = %v, want ErrConnClosed", err)
	}
}

func TestConnCloseWithExtensions(t *testing.T) {
	a, b := net.Pipe()
	defer b.Close()
	c, err := NewConn(a, nil, true, ExtParams{Negotiated: true})
	if err != nil {
		t.Fatal(err)
	}
	if c.inflater == nil || c.deflater == nil {
		t.Fatal("expected inflater and deflater to be created")
	}
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := c.inflater.Close(); err != nil {
		t.Errorf("Inflater.Close: %v", err)
	}
	if err := c.deflater.Close(); err != nil {
		t.Errorf("Deflater.Close: %v", err)
	}
}

func TestNewConnCreatesReaderWhenNil(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c, err := NewConn(a, nil, false, ExtParams{})
	if err != nil {
		t.Fatal(err)
	}
	if c.br == nil {
		t.Error("expected bufio.Reader to be created")
	}
	if c.isClient {
		t.Error("isClient should be false")
	}
}

func TestNewConnContextTakeoverDirection(t *testing.T) {
	tests := []struct {
		name          string
		isClient      bool
		ext           ExtParams
		wantReaderNCT bool
		wantWriterNCT bool
	}{
		{"client both off", true, ExtParams{Negotiated: true}, false, false},
		{"client server_nct", true, ExtParams{Negotiated: true, ServerNoContextTakeover: true}, true, false},
		{"client client_nct", true, ExtParams{Negotiated: true, ClientNoContextTakeover: true}, false, true},
		{"server server_nct", false, ExtParams{Negotiated: true, ServerNoContextTakeover: true}, false, true},
		{"server client_nct", false, ExtParams{Negotiated: true, ClientNoContextTakeover: true}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			c, err := NewConn(a, nil, tt.isClient, tt.ext)
			if err != nil {
				t.Fatal(err)
			}
			if c.inflater.noContext != tt.wantReaderNCT {
				t.Errorf("inflater.noContext = %v, want %v", c.inflater.noContext, tt.wantReaderNCT)
			}
			if c.deflater.noContext != tt.wantWriterNCT {
				t.Errorf("deflater.noContext = %v, want %v", c.deflater.noContext, tt.wantWriterNCT)
			}
		})
	}
}

func TestConnWriteMessageFramesCorrectly(t *testing.T) {
	tests := []struct {
		name     string
		isClient bool
		op       Opcode
		payload  []byte
	}{
		{"client text", true, OpText, []byte("hello")},
		{"client empty", true, OpText, nil},
		{"client binary", true, OpBinary, []byte{0, 1, 2, 255}},
		{"client ping", true, OpPing, []byte("p")},
		{"server text", false, OpText, []byte("hello")},
		{"server close", false, OpClose, MakeClosePayload(CloseNormal, "bye")},
		{"large", true, OpBinary, bytes.Repeat([]byte("z"), 70000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			c, err := NewConn(a, nil, tt.isClient, ExtParams{})
			if err != nil {
				t.Fatal(err)
			}
			errc := make(chan error, 1)
			go func() { errc <- c.WriteMessage(tt.op, tt.payload) }()

			hdr, payload, err := ReadFrame(bufio.NewReader(b))
			if err != nil {
				t.Fatalf("ReadFrame: %v", err)
			}
			if err := <-errc; err != nil {
				t.Fatalf("WriteMessage: %v", err)
			}
			if hdr.Opcode != tt.op {
				t.Errorf("opcode = %v, want %v", hdr.Opcode, tt.op)
			}
			if !hdr.FIN {
				t.Error("FIN not set")
			}
			if hdr.RSV1 {
				t.Error("RSV1 set without deflate")
			}
			wantMask := tt.isClient && len(tt.payload) > 0
			if wantMask && !hdr.Masked {
				t.Error("client frame not masked")
			}
			if !tt.isClient && hdr.Masked {
				t.Error("server frame masked")
			}
			if !bytes.Equal(payload, tt.payload) && !(len(payload) == 0 && len(tt.payload) == 0) {
				t.Errorf("payload len = %d, want %d", len(payload), len(tt.payload))
			}
		})
	}
}

func TestConnWriteMessageSetsRSV1OnlyForDataFrames(t *testing.T) {
	tests := []struct {
		name     string
		op       Opcode
		payload  []byte
		wantRSV1 bool
	}{
		{"text compressed", OpText, bytes.Repeat([]byte("ab"), 200), true},
		{"binary compressed", OpBinary, bytes.Repeat([]byte("cd"), 200), true},
		{"empty text not compressed", OpText, nil, false},
		{"ping not compressed", OpPing, []byte("ping"), false},
		{"pong not compressed", OpPong, []byte("pong"), false},
		{"close not compressed", OpClose, MakeClosePayload(CloseNormal, ""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			c, err := NewConn(a, nil, true, ExtParams{Negotiated: true})
			if err != nil {
				t.Fatal(err)
			}
			errc := make(chan error, 1)
			go func() { errc <- c.WriteMessage(tt.op, tt.payload) }()
			hdr, _, err := ReadFrame(bufio.NewReader(b))
			if err != nil {
				t.Fatalf("ReadFrame: %v", err)
			}
			if err := <-errc; err != nil {
				t.Fatalf("WriteMessage: %v", err)
			}
			if hdr.RSV1 != tt.wantRSV1 {
				t.Errorf("RSV1 = %v, want %v", hdr.RSV1, tt.wantRSV1)
			}
		})
	}
}

func TestConnWriteMessageOnBrokenPipe(t *testing.T) {
	a, b := net.Pipe()
	_ = b.Close()
	c, err := NewConn(a, nil, true, ExtParams{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.WriteMessage(OpText, []byte("x")); err == nil {
		t.Fatal("expected write error on broken pipe")
	}
}

func TestConnConcurrentWritesAreSerialized(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c, err := NewConn(a, nil, true, ExtParams{})
	if err != nil {
		t.Fatal(err)
	}
	const n = 20
	errc := make(chan error, n)
	for i := range n {
		go func(i int) { errc <- c.WriteMessage(OpText, []byte{byte('a' + i)}) }(i)
	}
	br := bufio.NewReader(b)
	got := make(map[byte]int)
	for range n {
		_, payload, err := ReadFrame(br)
		if err != nil {
			t.Fatalf("ReadFrame: %v", err)
		}
		if len(payload) != 1 {
			t.Fatalf("interleaved frame: payload len %d", len(payload))
		}
		got[payload[0]]++
	}
	for range n {
		if err := <-errc; err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
	}
	if len(got) != n {
		t.Errorf("got %d distinct payloads, want %d", len(got), n)
	}
}

func TestConnReadMessageDecompresses(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	sc, err := NewConn(server, nil, false, ExtParams{Negotiated: true})
	if err != nil {
		t.Fatal(err)
	}
	cc, err := NewConn(client, nil, true, ExtParams{Negotiated: true})
	if err != nil {
		t.Fatal(err)
	}
	payloads := [][]byte{
		bytes.Repeat([]byte("context takeover "), 50),
		bytes.Repeat([]byte("context takeover "), 50),
		[]byte("short"),
		bytes.Repeat([]byte("different data "), 60),
	}
	errc := make(chan error, 1)
	go func() {
		for _, p := range payloads {
			if err := sc.WriteMessage(OpBinary, p); err != nil {
				errc <- err
				return
			}
		}
		errc <- nil
	}()
	for i, want := range payloads {
		op, got, err := cc.ReadMessage()
		if err != nil {
			t.Fatalf("message %d: %v", i, err)
		}
		if op != OpBinary {
			t.Errorf("message %d: op = %v", i, op)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("message %d: payload mismatch (%d vs %d bytes)", i, len(got), len(want))
		}
	}
	if err := <-errc; err != nil {
		t.Fatalf("writer: %v", err)
	}
}

func TestConnReadMessageInflateError(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c, err := NewConn(a, nil, true, ExtParams{Negotiated: true})
	if err != nil {
		t.Fatal(err)
	}
	raw := frameBytes(t, Header{FIN: true, RSV1: true, Opcode: OpText}, []byte{0xFF, 0xFF, 0xFF, 0xFF})
	c.br = bufio.NewReader(bytes.NewReader(raw))
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected inflate error on garbage deflate payload")
	}
}

func TestConnReadMessageDeadlineError(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c, err := NewConn(a, nil, true, ExtParams{})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.SetReadDeadline(time.Now().Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.ReadMessage(); err == nil {
		t.Fatal("expected read deadline error")
	}
}
