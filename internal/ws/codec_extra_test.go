package ws

import (
	"bytes"
	"strings"
	"testing"
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
