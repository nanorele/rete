package ws

import (
	"bufio"
	"bytes"
	"compress/flate"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseExtensionsTable(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   ExtParams
	}{
		{"empty", "", ExtParams{}},
		{"whitespace only", "   ", ExtParams{}},
		{"unrelated extension", "x-webkit-deflate-frame", ExtParams{}},
		{"plain", "permessage-deflate", ExtParams{Negotiated: true}},
		{"case insensitive name", "PerMessage-Deflate", ExtParams{Negotiated: true}},
		{"leading whitespace", "  permessage-deflate  ", ExtParams{Negotiated: true}},
		{
			"server no context",
			"permessage-deflate; server_no_context_takeover",
			ExtParams{Negotiated: true, ServerNoContextTakeover: true},
		},
		{
			"client no context",
			"permessage-deflate; client_no_context_takeover",
			ExtParams{Negotiated: true, ClientNoContextTakeover: true},
		},
		{
			"both no context",
			"permessage-deflate; server_no_context_takeover; client_no_context_takeover",
			ExtParams{Negotiated: true, ServerNoContextTakeover: true, ClientNoContextTakeover: true},
		},
		{
			"window bits",
			"permessage-deflate; server_max_window_bits=10; client_max_window_bits=9",
			ExtParams{Negotiated: true, ServerMaxWindowBits: 10, ClientMaxWindowBits: 9},
		},
		{
			"quoted window bits",
			`permessage-deflate; server_max_window_bits="12"`,
			ExtParams{Negotiated: true, ServerMaxWindowBits: 12},
		},
		{
			"valueless client_max_window_bits",
			"permessage-deflate; client_max_window_bits",
			ExtParams{Negotiated: true},
		},
		{
			"non numeric window bits ignored",
			"permessage-deflate; server_max_window_bits=abc",
			ExtParams{Negotiated: true},
		},
		{
			"uppercase params",
			"permessage-deflate; SERVER_NO_CONTEXT_TAKEOVER",
			ExtParams{Negotiated: true, ServerNoContextTakeover: true},
		},
		{
			"unknown params ignored",
			"permessage-deflate; foo=bar; baz",
			ExtParams{Negotiated: true},
		},
		{
			"skips leading unrelated extension",
			"mux; max-channels=4, permessage-deflate; server_no_context_takeover",
			ExtParams{Negotiated: true, ServerNoContextTakeover: true},
		},
		{
			"empty list entries skipped",
			", , permessage-deflate",
			ExtParams{Negotiated: true},
		},
		{
			"first deflate offer wins",
			"permessage-deflate; client_no_context_takeover, permessage-deflate",
			ExtParams{Negotiated: true, ClientNoContextTakeover: true},
		},
		{
			"extra whitespace around params",
			"permessage-deflate ;  server_max_window_bits = 11 ",
			ExtParams{Negotiated: true, ServerMaxWindowBits: 11},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseExtensions(tt.header); got != tt.want {
				t.Errorf("ParseExtensions(%q) =\n  %+v\nwant\n  %+v", tt.header, got, tt.want)
			}
		})
	}
}

func TestOfferExtensionsIsParseable(t *testing.T) {
	got := ParseExtensions(OfferExtensions())
	if !got.Negotiated {
		t.Fatalf("own offer %q does not parse as negotiated", OfferExtensions())
	}
}

func TestDeflateInflateTable(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		noContext bool
	}{
		{"empty", nil, true},
		{"single byte", []byte("a"), true},
		{"short text", []byte("hello world"), true},
		{"highly repetitive", bytes.Repeat([]byte("ab"), 5000), true},
		{"binary with nulls", append(make([]byte, 100), []byte("tail")...), true},
		{"unicode", []byte(strings.Repeat("日本語テキスト", 100)), true},
		{"context takeover empty", nil, false},
		{"context takeover text", []byte("hello world"), false},
		{"context takeover large", bytes.Repeat([]byte("xyz"), 4000), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := NewDeflater(tt.noContext)
			if err != nil {
				t.Fatal(err)
			}
			i := NewInflater(tt.noContext)
			comp, err := d.Deflate(tt.payload)
			if err != nil {
				t.Fatalf("Deflate: %v", err)
			}
			got, err := i.Inflate(comp)
			if err != nil {
				t.Fatalf("Inflate: %v", err)
			}
			if !bytes.Equal(got, tt.payload) && !(len(got) == 0 && len(tt.payload) == 0) {
				t.Errorf("roundtrip mismatch: got %d bytes, want %d", len(got), len(tt.payload))
			}
		})
	}
}

func TestDeflateContextTakeoverAcrossManyMessages(t *testing.T) {
	d, err := NewDeflater(false)
	if err != nil {
		t.Fatal(err)
	}
	i := NewInflater(false)
	msg := bytes.Repeat([]byte("repeated-block-"), 200)
	var firstSize, lastSize int
	for n := range 8 {
		comp, err := d.Deflate(msg)
		if err != nil {
			t.Fatalf("message %d: Deflate: %v", n, err)
		}
		got, err := i.Inflate(comp)
		if err != nil {
			t.Fatalf("message %d: Inflate: %v", n, err)
		}
		if !bytes.Equal(got, msg) {
			t.Fatalf("message %d: payload mismatch", n)
		}
		if n == 0 {
			firstSize = len(comp)
		}
		lastSize = len(comp)
	}
	if lastSize > firstSize {
		t.Errorf("context takeover made compression worse: first=%d last=%d", firstSize, lastSize)
	}
}

func TestDeflateNoContextIsStateless(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	msg := bytes.Repeat([]byte("stateless-"), 200)
	first, err := d.Deflate(msg)
	if err != nil {
		t.Fatal(err)
	}
	for n := range 4 {
		got, err := d.Deflate(msg)
		if err != nil {
			t.Fatalf("message %d: %v", n, err)
		}
		if !bytes.Equal(got, first) {
			t.Errorf("message %d: no_context_takeover output differs from first message", n)
		}
	}
	if d.history != nil {
		t.Errorf("history retained despite no_context_takeover: %d bytes", len(d.history))
	}
}

func TestInflateNoContextIsStateless(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	i := NewInflater(true)
	msg := []byte("independent message")
	comp, err := d.Deflate(msg)
	if err != nil {
		t.Fatal(err)
	}
	for n := range 4 {
		got, err := i.Inflate(comp)
		if err != nil {
			t.Fatalf("message %d: %v", n, err)
		}
		if !bytes.Equal(got, msg) {
			t.Errorf("message %d: got %q, want %q", n, got, msg)
		}
	}
	if i.history != nil {
		t.Errorf("history retained despite no_context_takeover: %d bytes", len(i.history))
	}
}

func TestInflateCorruptPayload(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{"all ones", []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{"invalid block type", []byte{0x07}},
		{"random bytes", []byte{0x12, 0x9A, 0x44, 0x7F, 0x01, 0xC3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			i := NewInflater(true)
			out, err := i.Inflate(tt.in)
			if err == nil {
				t.Fatalf("expected error, got %d bytes", len(out))
			}
			if out != nil {
				t.Errorf("out = %v, want nil on error", out)
			}
		})
	}
}

func TestInflateTruncatedPayload(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := d.Deflate(bytes.Repeat([]byte("truncate me "), 200))
	if err != nil {
		t.Fatal(err)
	}
	if len(comp) < 8 {
		t.Fatalf("compressed payload too small to truncate: %d", len(comp))
	}
	for _, cut := range []int{1, len(comp) / 4, len(comp) / 2, len(comp) - 1} {
		i := NewInflater(true)
		out, err := i.Inflate(comp[:cut])
		if err == nil && len(out) == 0 {
			continue
		}
		if err == nil && !bytes.HasPrefix(bytes.Repeat([]byte("truncate me "), 200), out) {
			t.Errorf("cut=%d: silently produced non-prefix output of %d bytes", cut, len(out))
		}
	}
}

func TestInflateRejectsEmptyPayload(t *testing.T) {
	for _, in := range [][]byte{nil, {}} {
		i := NewInflater(true)
		out, err := i.Inflate(in)
		if err == nil {
			t.Errorf("Inflate(%v) = %q, want error: a bare sync marker is not a valid message", in, out)
		}
	}
}

func TestInflateAcceptsDeflatedEmptyMessage(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	comp, err := d.Deflate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(comp) == 0 {
		t.Fatal("Deflate(nil) produced an empty payload, which Inflate cannot decode")
	}
	out, err := NewInflater(true).Inflate(comp)
	if err != nil {
		t.Fatalf("Inflate: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("out = %q, want empty", out)
	}
}

func TestDeflateDoesNotMutateCallerPayload(t *testing.T) {
	d, err := NewDeflater(false)
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("original"), 100)
	orig := append([]byte{}, payload...)
	if _, err := d.Deflate(payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(payload, orig) {
		t.Error("Deflate mutated the caller's payload")
	}
}

func TestDeflateOutputIsIndependentOfInternalBuffer(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	first, err := d.Deflate([]byte("first message payload"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot := append([]byte{}, first...)
	if _, err := d.Deflate(bytes.Repeat([]byte("second and much longer payload "), 50)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, snapshot) {
		t.Error("earlier Deflate result was clobbered by a later call")
	}
}

func TestDeflateStripsSyncTail(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{
		[]byte("a"),
		[]byte("hello world hello world"),
		bytes.Repeat([]byte("q"), 5000),
	} {
		out, err := d.Deflate(payload)
		if err != nil {
			t.Fatal(err)
		}
		if len(out) >= 4 && bytes.Equal(out[len(out)-4:], syncTail[:]) {
			t.Errorf("payload len %d: output still ends with the 00 00 ff ff sync tail", len(payload))
		}
	}
}

func TestAppendHistory(t *testing.T) {
	tests := []struct {
		name        string
		historyLen  int
		freshLen    int
		wantLen     int
		wantTailOf  string
		checkPrefix bool
	}{
		{"empty into empty", 0, 0, 0, "", false},
		{"small into empty", 0, 100, 100, "fresh", false},
		{"small into small", 100, 100, 200, "fresh", false},
		{"fills exactly", flateHistorySize - 10, 10, flateHistorySize, "fresh", false},
		{"overflows window", flateHistorySize, 100, flateHistorySize, "fresh", false},
		{"fresh exactly window", 500, flateHistorySize, flateHistorySize, "fresh", false},
		{"fresh larger than window", 500, flateHistorySize + 1000, flateHistorySize, "fresh", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := bytes.Repeat([]byte{'H'}, tt.historyLen)
			fresh := make([]byte, tt.freshLen)
			for i := range fresh {
				fresh[i] = byte('a' + i%26)
			}
			got := appendHistory(history, fresh)
			if len(got) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.freshLen > 0 && len(got) > 0 {
				want := fresh[len(fresh)-min(len(fresh), len(got)):]
				if !bytes.HasSuffix(got, want) {
					t.Error("history does not end with the most recent bytes")
				}
			}
		})
	}
}

func TestAppendHistoryDoesNotAliasFresh(t *testing.T) {
	fresh := make([]byte, flateHistorySize+100)
	for i := range fresh {
		fresh[i] = byte(i)
	}
	got := appendHistory(nil, fresh)
	fresh[len(fresh)-1] = 0xFF
	if got[len(got)-1] == 0xFF {
		t.Error("appendHistory aliased the caller's slice")
	}
}

func TestReassemblerResetAfterCompleteMessage(t *testing.T) {
	var r Reassembler
	for n := range 3 {
		if _, ready, err := r.Step(Header{Opcode: OpText}, []byte("a")); err != nil || ready {
			t.Fatalf("round %d: ready=%v err=%v", n, ready, err)
		}
		asm, ready, err := r.Step(Header{Opcode: OpContinuation, FIN: true}, []byte("b"))
		if err != nil || !ready {
			t.Fatalf("round %d: ready=%v err=%v", n, ready, err)
		}
		if string(asm.Payload) != "ab" || asm.Opcode != OpText {
			t.Fatalf("round %d: got op=%v %q", n, asm.Opcode, asm.Payload)
		}
		if r.open {
			t.Fatalf("round %d: reassembler still open", n)
		}
	}
}

func TestReassemblerCompressedFragmentedMessage(t *testing.T) {
	var r Reassembler
	if _, ready, err := r.Step(Header{Opcode: OpBinary, RSV1: true}, []byte("x")); err != nil || ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	asm, ready, err := r.Step(Header{Opcode: OpContinuation, FIN: true}, []byte("y"))
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if !asm.Compressed {
		t.Error("Compressed flag lost across fragments")
	}
	if asm.Opcode != OpBinary {
		t.Errorf("Opcode = %v, want BIN", asm.Opcode)
	}
	if string(asm.Payload) != "xy" {
		t.Errorf("payload = %q, want xy", asm.Payload)
	}
}

func TestReassemblerPayloadIsCloned(t *testing.T) {
	var r Reassembler
	if _, _, err := r.Step(Header{Opcode: OpText}, []byte("first")); err != nil {
		t.Fatal(err)
	}
	asm, ready, err := r.Step(Header{Opcode: OpContinuation, FIN: true}, []byte("second"))
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	snapshot := string(asm.Payload)
	if _, _, err := r.Step(Header{Opcode: OpText}, []byte("later message data")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.Step(Header{Opcode: OpContinuation, FIN: true}, []byte("more")); err != nil {
		t.Fatal(err)
	}
	if string(asm.Payload) != snapshot {
		t.Errorf("earlier payload mutated: %q, want %q", asm.Payload, snapshot)
	}
}

func TestReassemblerControlFramesArePassThrough(t *testing.T) {
	tests := []struct {
		name string
		op   Opcode
	}{{"ping", OpPing}, {"pong", OpPong}, {"close", OpClose}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Reassembler
			asm, ready, err := r.Step(Header{FIN: true, Opcode: tt.op}, []byte("ctl"))
			if err != nil || !ready {
				t.Fatalf("ready=%v err=%v", ready, err)
			}
			if !asm.Control {
				t.Error("Control flag not set")
			}
			if asm.Opcode != tt.op {
				t.Errorf("Opcode = %v, want %v", asm.Opcode, tt.op)
			}
			if string(asm.Payload) != "ctl" {
				t.Errorf("payload = %q", asm.Payload)
			}
			if r.open {
				t.Error("control frame opened the reassembler")
			}
		})
	}
}

func TestReassemblerUnfragmentedMessageDoesNotBuffer(t *testing.T) {
	var r Reassembler
	asm, ready, err := r.Step(Header{FIN: true, Opcode: OpText, RSV1: true}, []byte("solo"))
	if err != nil || !ready {
		t.Fatalf("ready=%v err=%v", ready, err)
	}
	if !asm.Compressed {
		t.Error("Compressed flag not propagated from RSV1")
	}
	if r.open || r.buf.Len() != 0 {
		t.Error("unfragmented message left state behind")
	}
}

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

func TestDeflateInflateRoundtripNoContext(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatalf("NewDeflater: %v", err)
	}
	defer d.Close()
	in := NewInflater(true)
	defer in.Close()

	for _, payload := range [][]byte{
		[]byte("hello"),
		bytes.Repeat([]byte("AB"), 1000),
		[]byte("permessage-deflate test payload with some redundancy permessage-deflate"),
	} {
		compressed, err := d.Deflate(payload)
		if err != nil {
			t.Fatalf("Deflate: %v", err)
		}
		got, err := in.Inflate(compressed)
		if err != nil {
			t.Fatalf("Inflate: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("roundtrip mismatch: got %q want %q", got, payload)
		}
	}
}

func TestDeflateInflateRoundtripContextTakeover(t *testing.T) {
	d, err := NewDeflater(false)
	if err != nil {
		t.Fatalf("NewDeflater: %v", err)
	}
	defer d.Close()
	in := NewInflater(false)
	defer in.Close()

	msgs := []string{"one two three", "one two three four", "one two three four five"}
	for _, m := range msgs {
		compressed, err := d.Deflate([]byte(m))
		if err != nil {
			t.Fatalf("Deflate: %v", err)
		}
		got, err := in.Inflate(compressed)
		if err != nil {
			t.Fatalf("Inflate: %v", err)
		}
		if string(got) != m {
			t.Errorf("got %q want %q", got, m)
		}
	}
}

func TestParseExtensions(t *testing.T) {
	cases := []struct {
		in   string
		want ExtParams
	}{
		{"", ExtParams{}},
		{"permessage-deflate", ExtParams{Negotiated: true}},
		{"permessage-deflate; client_no_context_takeover", ExtParams{Negotiated: true, ClientNoContextTakeover: true}},
		{"permessage-deflate; server_no_context_takeover; client_no_context_takeover", ExtParams{Negotiated: true, ServerNoContextTakeover: true, ClientNoContextTakeover: true}},
		{"permessage-deflate; client_max_window_bits=10", ExtParams{Negotiated: true, ClientMaxWindowBits: 10}},
		{"other-ext, permessage-deflate", ExtParams{Negotiated: true}},
		{"deflate-stream", ExtParams{}},
	}
	for _, c := range cases {
		got := ParseExtensions(c.in)
		if got != c.want {
			t.Errorf("ParseExtensions(%q) = %+v want %+v", c.in, got, c.want)
		}
	}
}

func silentServer(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = io.Copy(io.Discard, c)
			}()
		}
	}()
	return l
}

func TestDialHandshakeTimeout(t *testing.T) {
	l := silentServer(t)
	start := time.Now()
	_, err := Dial(context.Background(), "ws://"+l.Addr().String(), DialOptions{DialTimeout: 300 * time.Millisecond})
	if err == nil {
		t.Fatal("expected handshake timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dial blocked for %v despite timeout", elapsed)
	}
}

func TestDialCancelDuringHandshake(t *testing.T) {
	l := silentServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Dial(ctx, "ws://"+l.Addr().String(), DialOptions{DialTimeout: 30 * time.Second})
	if err == nil {
		t.Fatal("expected error after context cancel")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("dial blocked for %v despite cancel", elapsed)
	}
}

func TestReadUnmaskedTextFrame(t *testing.T) {
	raw := []byte{0x81, 0x05, 'H', 'e', 'l', 'l', 'o'}
	hdr, payload, err := ReadFrame(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !hdr.FIN || hdr.Opcode != OpText {
		t.Errorf("hdr=%+v", hdr)
	}
	if string(payload) != "Hello" {
		t.Errorf("payload=%q", payload)
	}
}

func TestReadMaskedTextFrame(t *testing.T) {
	raw := []byte{0x81, 0x85, 0x37, 0xFA, 0x21, 0x3D, 0x7F, 0x9F, 0x4D, 0x51, 0x58}
	hdr, payload, err := ReadFrame(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !hdr.FIN || hdr.Opcode != OpText || !hdr.Masked {
		t.Errorf("hdr=%+v", hdr)
	}
	if string(payload) != "Hello" {
		t.Errorf("payload=%q", payload)
	}
}

func TestRoundtrip16BitLength(t *testing.T) {
	payload := bytes.Repeat([]byte{'a'}, 300)
	var buf bytes.Buffer
	hdr := Header{FIN: true, Opcode: OpBinary, Length: uint64(len(payload))}
	if err := WriteFrame(&buf, hdr, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, gotPayload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if got.Opcode != OpBinary || !got.FIN {
		t.Errorf("hdr=%+v", got)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload mismatch")
	}
}

func TestRoundtrip64BitLength(t *testing.T) {
	payload := bytes.Repeat([]byte{'b'}, 70000)
	var buf bytes.Buffer
	hdr := Header{FIN: true, Opcode: OpBinary, Length: uint64(len(payload))}
	if err := WriteFrame(&buf, hdr, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, gotPayload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if got.Length != uint64(len(payload)) {
		t.Errorf("len=%d want %d", got.Length, len(payload))
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload mismatch")
	}
}

func TestRoundtripMasked(t *testing.T) {
	payload := []byte("client-to-server-payload")
	var buf bytes.Buffer
	hdr := Header{FIN: true, Opcode: OpText, Masked: true, MaskKey: [4]byte{0xAA, 0xBB, 0xCC, 0xDD}, Length: uint64(len(payload))}
	if err := WriteFrame(&buf, hdr, payload); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	got, gotPayload, err := ReadFrame(&buf)
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if !got.Masked {
		t.Errorf("expected mask bit set")
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Errorf("payload=%q want %q", gotPayload, payload)
	}
}

func TestControlFrameTooLong(t *testing.T) {
	raw := []byte{0x89, 0x7E, 0x00, 0xC8}
	raw = append(raw, bytes.Repeat([]byte{'x'}, 200)...)
	if _, _, err := ReadFrame(bytes.NewReader(raw)); err != ErrControlFrameTooLong {
		t.Errorf("expected ErrControlFrameTooLong, got %v", err)
	}
}

func TestControlFrameNotFinal(t *testing.T) {
	raw := []byte{0x09, 0x00}
	if _, _, err := ReadFrame(bytes.NewReader(raw)); err != ErrControlFrameNotFinal {
		t.Errorf("expected ErrControlFrameNotFinal, got %v", err)
	}
}

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

func startEchoServer(t *testing.T, opts UpgradeOptions) (addr string, stop func()) {
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
			go handleEcho(c, opts)
		}
	}()
	return l.Addr().String(), func() {
		_ = l.Close()
		wg.Wait()
	}
}

func handleEcho(c net.Conn, opts UpgradeOptions) {
	defer c.Close()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	res, err := Upgrade(c, br, req, opts)
	if err != nil {
		return
	}
	defer res.Conn.Close()
	for {
		op, payload, err := res.Conn.ReadMessage()
		if err != nil {
			return
		}
		switch op {
		case OpText, OpBinary:
			if err := res.Conn.WriteMessage(op, payload); err != nil {
				return
			}
		case OpPing:
			if err := res.Conn.WriteMessage(OpPong, payload); err != nil {
				return
			}
		case OpClose:
			_ = res.Conn.WriteMessage(OpClose, payload)
			return
		}
	}
}

func TestEchoPlainText(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	for _, want := range []string{"hello", "world", "third message"} {
		if err := res.Conn.WriteMessage(OpText, []byte(want)); err != nil {
			t.Fatalf("Write: %v", err)
		}
		op, payload, err := res.Conn.ReadMessage()
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if op != OpText || string(payload) != want {
			t.Errorf("got op=%v %q want OpText %q", op, payload, want)
		}
	}
}

func TestEchoDeflate(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{AcceptDeflate: true})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{OfferDeflate: true})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if !res.Extensions.Negotiated {
		t.Fatalf("expected deflate to be negotiated")
	}
	payload := bytes.Repeat([]byte("ABCD"), 500)
	if err := res.Conn.WriteMessage(OpText, payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	op, got, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpText {
		t.Errorf("op=%v", op)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch")
	}
}

func TestEchoPingPong(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteMessage(OpPing, []byte("pingdata")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	op, payload, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpPong || string(payload) != "pingdata" {
		t.Errorf("got op=%v %q", op, payload)
	}
}

func TestEchoClose(t *testing.T) {
	addr, stop := startEchoServer(t, UpgradeOptions{})
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := Dial(ctx, "ws://"+addr+"/", DialOptions{})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer res.Conn.Close()
	if err := res.Conn.WriteClose(CloseNormal, "bye"); err != nil {
		t.Fatalf("WriteClose: %v", err)
	}
	op, payload, err := res.Conn.ReadMessage()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if op != OpClose {
		t.Errorf("op=%v", op)
	}
	code, reason := ParseClosePayload(payload)
	if code != CloseNormal || reason != "bye" {
		t.Errorf("got code=%d reason=%q", code, reason)
	}
}

func TestReadFrameRejectsHugeLength(t *testing.T) {
	raw := []byte{0x82, 0x7F}
	var ext [8]byte
	binary.BigEndian.PutUint64(ext[:], 1<<50)
	raw = append(raw, ext[:]...)
	if _, _, err := ReadFrame(bytes.NewReader(raw)); err != ErrMessageTooLarge {
		t.Errorf("expected ErrMessageTooLarge, got %v", err)
	}
}

func TestReadFrameRejectsJustOverLimit(t *testing.T) {
	raw := []byte{0x82, 0x7F}
	var ext [8]byte
	binary.BigEndian.PutUint64(ext[:], MaxMessageSize+1)
	raw = append(raw, ext[:]...)
	if _, _, err := ReadFrame(bytes.NewReader(raw)); err != ErrMessageTooLarge {
		t.Errorf("expected ErrMessageTooLarge, got %v", err)
	}
}

func TestReassemblerRejectsOversizedFragments(t *testing.T) {
	var r Reassembler
	first := Header{Opcode: OpBinary, Length: 1024}
	if _, _, err := r.Step(first, make([]byte, 1024)); err != nil {
		t.Fatalf("first fragment: %v", err)
	}
	chunk := make([]byte, 1<<20)
	var err error
	for i := 0; i < (MaxMessageSize>>20)+1; i++ {
		cont := Header{Opcode: OpContinuation, Length: uint64(len(chunk))}
		if _, _, err = r.Step(cont, chunk); err != nil {
			break
		}
	}
	if err != ErrMessageTooLarge {
		t.Errorf("expected ErrMessageTooLarge, got %v", err)
	}
	if r.open {
		t.Errorf("reassembler must reset after overflow")
	}
}

func TestInflateRejectsDecompressionBomb(t *testing.T) {
	var comp bytes.Buffer
	fw, err := flate.NewWriter(&comp, flate.BestCompression)
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	zeros := make([]byte, 1<<20)
	for written := 0; written <= MaxMessageSize; written += len(zeros) {
		if _, err := fw.Write(zeros); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if err := fw.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	payload := comp.Bytes()
	if len(payload) >= 4 && bytes.Equal(payload[len(payload)-4:], syncTail[:]) {
		payload = payload[:len(payload)-4]
	}
	inf := NewInflater(true)
	if _, err := inf.Inflate(payload); err != ErrMessageTooLarge {
		t.Errorf("expected ErrMessageTooLarge, got %v", err)
	}
}

func TestInflateUnderLimitStillWorks(t *testing.T) {
	df, err := NewDeflater(true)
	if err != nil {
		t.Fatalf("NewDeflater: %v", err)
	}
	msg := bytes.Repeat([]byte("hello world "), 1000)
	payload, err := df.Deflate(msg)
	if err != nil {
		t.Fatalf("Deflate: %v", err)
	}
	inf := NewInflater(true)
	out, err := inf.Inflate(payload)
	if err != nil {
		t.Fatalf("Inflate: %v", err)
	}
	if !bytes.Equal(out, msg) {
		t.Errorf("roundtrip mismatch")
	}
}

func TestReassembleFragmentedText(t *testing.T) {
	var r Reassembler
	_, ready, err := r.Step(Header{Opcode: OpText, FIN: false, Length: 3}, []byte("hel"))
	if err != nil || ready {
		t.Fatalf("first frame: ready=%v err=%v", ready, err)
	}
	_, ready, err = r.Step(Header{Opcode: OpContinuation, FIN: false, Length: 3}, []byte("lo "))
	if err != nil || ready {
		t.Fatalf("middle: ready=%v err=%v", ready, err)
	}
	asm, ready, err := r.Step(Header{Opcode: OpContinuation, FIN: true, Length: 5}, []byte("world"))
	if err != nil || !ready {
		t.Fatalf("final: ready=%v err=%v", ready, err)
	}
	if asm.Opcode != OpText {
		t.Errorf("opcode=%v", asm.Opcode)
	}
	if !bytes.Equal(asm.Payload, []byte("hello world")) {
		t.Errorf("payload=%q", asm.Payload)
	}
}

func TestReassembleControlInterleaved(t *testing.T) {
	var r Reassembler
	_, _, err := r.Step(Header{Opcode: OpBinary, FIN: false, Length: 2}, []byte{0x01, 0x02})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	asm, ready, err := r.Step(Header{Opcode: OpPing, FIN: true, Length: 4}, []byte("ping"))
	if err != nil || !ready || !asm.Control {
		t.Fatalf("ping not surfaced: ready=%v ctrl=%v err=%v", ready, asm.Control, err)
	}
	asm, ready, err = r.Step(Header{Opcode: OpContinuation, FIN: true, Length: 2}, []byte{0x03, 0x04})
	if err != nil || !ready {
		t.Fatalf("final: %v %v", ready, err)
	}
	if !bytes.Equal(asm.Payload, []byte{0x01, 0x02, 0x03, 0x04}) {
		t.Errorf("payload=%v", asm.Payload)
	}
}

func TestReassembleUnexpectedContinuation(t *testing.T) {
	var r Reassembler
	_, _, err := r.Step(Header{Opcode: OpContinuation, FIN: true, Length: 1}, []byte("x"))
	if err != ErrUnexpectedContinuation {
		t.Errorf("expected ErrUnexpectedContinuation, got %v", err)
	}
}

func TestReassembleUnexpectedDataFrame(t *testing.T) {
	var r Reassembler
	_, _, _ = r.Step(Header{Opcode: OpText, FIN: false, Length: 1}, []byte("a"))
	_, _, err := r.Step(Header{Opcode: OpText, FIN: true, Length: 1}, []byte("b"))
	if err != ErrUnexpectedDataFrame {
		t.Errorf("expected ErrUnexpectedDataFrame, got %v", err)
	}
}

func TestReassembleSingleFrameCompressed(t *testing.T) {
	var r Reassembler
	asm, ready, err := r.Step(Header{Opcode: OpText, FIN: true, RSV1: true, Length: 5}, []byte("xyzab"))
	if err != nil || !ready || !asm.Compressed {
		t.Fatalf("ready=%v compressed=%v err=%v", ready, asm.Compressed, err)
	}
}

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
				"X-Server":   {"rete"},
				"Set-Cookie": {"a=b"},
				"Upgrade":    {"bogus"},
				"Connection": {"bogus"},
			}},
			wantHeaders: map[string]string{
				"X-Server":   "rete",
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
