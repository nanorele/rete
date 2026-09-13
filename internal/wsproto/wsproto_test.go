package wsproto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/pierrec/lz4/v4"
	"github.com/vmihailenco/msgpack/v5"
	"math"
	"reflect"
	"strings"
	"testing"
)

func buildFrame(version, cmd byte, seq, opcode int16, cof byte, declaredLen int, body []byte) []byte {
	raw := make([]byte, HeaderLen+len(body))
	raw[0] = version
	raw[1] = cmd
	binary.BigEndian.PutUint16(raw[2:], uint16(seq))
	binary.BigEndian.PutUint16(raw[4:], uint16(opcode))
	raw[6] = cof
	raw[7] = byte(declaredLen >> 16)
	raw[8] = byte(declaredLen >> 8)
	raw[9] = byte(declaredLen)
	copy(raw[HeaderLen:], body)
	return raw
}

func mustMsgpack(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatalf("msgpack.Marshal(%#v): %v", v, err)
	}
	return b
}

func mustLZ4(t *testing.T, src []byte) []byte {
	t.Helper()
	dst := make([]byte, lz4.CompressBlockBound(len(src)))
	var c lz4.Compressor
	n, err := c.CompressBlock(src, dst)
	if err != nil {
		t.Fatalf("CompressBlock: %v", err)
	}
	if n == 0 {
		t.Fatal("CompressBlock: incompressible")
	}
	return dst[:n]
}

func TestDecodeTruncatedHeaderLengths(t *testing.T) {
	for n := 0; n < HeaderLen; n++ {
		raw := make([]byte, n)
		if n > 0 {
			raw[0] = Version
		}
		payload, meta, err := Decode(raw)
		if !errors.Is(err, ErrShortHeader) {
			t.Errorf("len=%d: err = %v, want ErrShortHeader", n, err)
		}
		if payload != nil {
			t.Errorf("len=%d: payload = %#v, want nil", n, payload)
		}
		if meta != (Meta{}) {
			t.Errorf("len=%d: meta = %+v, want zero", n, meta)
		}
	}
	if _, _, err := Decode(nil); !errors.Is(err, ErrShortHeader) {
		t.Errorf("nil frame: err = %v, want ErrShortHeader", err)
	}
}

func TestDecodeDeclaredLengthOverflow(t *testing.T) {
	body := mustMsgpack(t, "payload")
	tests := []struct {
		name        string
		declaredLen int
		body        []byte
		wantErr     error
	}{
		{"one byte over", len(body) + 1, body, ErrTruncatedBody},
		{"max uint24 with empty body", 1<<24 - 1, nil, ErrTruncatedBody},
		{"max uint24 with real body", 1<<24 - 1, body, ErrTruncatedBody},
		{"huge with one byte short", len(body) + 1, body, ErrTruncatedBody},
		{"exact fit decodes", len(body), body, nil},
		{"under-declared decodes prefix", len(body), append(append([]byte{}, body...), 0xFF), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildFrame(Version, 1, 0, 0, 0, tt.declaredLen, tt.body)
			_, meta, err := Decode(raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr != nil && meta.Version != Version {
				t.Errorf("meta.Version = %d, want header echoed back on error", meta.Version)
			}
		})
	}
}

func TestDecodeDeclaredLengthNeverExceedsFrame(t *testing.T) {
	for _, declared := range []int{1, 100, 1 << 8, 1 << 16, 1<<24 - 1} {
		raw := buildFrame(Version, 0, 0, 0, 0, declared, nil)
		if _, _, err := Decode(raw); !errors.Is(err, ErrTruncatedBody) {
			t.Errorf("declared=%d: err = %v, want ErrTruncatedBody", declared, err)
		}
	}
}

func TestDecodeCorruptCompressedPayload(t *testing.T) {
	good := mustMsgpack(t, map[string]any{"text": strings.Repeat("payload-", 200)})
	comp := mustLZ4(t, good)
	cof := byte((len(good) + len(comp) - 1) / len(comp))

	tests := []struct {
		name string
		cof  byte
		body []byte
	}{
		{"all ones", cof, bytes.Repeat([]byte{0xFF}, len(comp))},
		{"all zeroes", cof, make([]byte, len(comp))},
		{"head truncated", cof, comp[1:]},
		{"tail truncated", cof, comp[:len(comp)/2]},
		{"first byte flipped", cof, append([]byte{comp[0] ^ 0xFF}, comp[1:]...)},
		{"cof too small for buffer", 1, comp},
		{"single byte body", 255, []byte{0xFF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildFrame(Version, 0, 0, 0, tt.cof, len(tt.body), tt.body)
			payload, _, err := Decode(raw)
			if err == nil {
				t.Fatalf("expected error, got payload %#v", payload)
			}
			if payload != nil {
				t.Errorf("payload = %#v, want nil on error", payload)
			}
		})
	}
}

func TestDecodeCorruptCompressedNeverPanics(t *testing.T) {
	good := mustMsgpack(t, map[string]any{"k": strings.Repeat("vvvv", 300)})
	comp := mustLZ4(t, good)
	for i := range comp {
		for _, mask := range []byte{0x01, 0x80, 0xFF} {
			mutated := append([]byte{}, comp...)
			mutated[i] ^= mask
			raw := buildFrame(Version, 0, 0, 0, 255, len(mutated), mutated)
			_, _, _ = Decode(raw)
		}
	}
}

func TestDecodeZeroCofIsUncompressed(t *testing.T) {
	body := mustMsgpack(t, map[string]any{"a": "b"})
	raw := buildFrame(Version, 0, 0, 0, 0, len(body), body)
	payload, meta, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Compressed() {
		t.Errorf("Compressed() = true for cof=0")
	}
	if meta.RawLen != meta.BodyLen {
		t.Errorf("RawLen = %d, BodyLen = %d, want equal when uncompressed", meta.RawLen, meta.BodyLen)
	}
	if payload.(map[string]any)["a"] != "b" {
		t.Errorf("payload = %#v", payload)
	}
}

func TestDecodeCofWithEmptyBody(t *testing.T) {
	for _, cof := range []byte{1, 2, 128, 255} {
		raw := buildFrame(Version, 0, 0, 0, cof, 0, nil)
		payload, meta, err := Decode(raw)
		if err != nil {
			t.Fatalf("cof=%d: unexpected err %v", cof, err)
		}
		if payload != nil {
			t.Errorf("cof=%d: payload = %#v, want nil", cof, payload)
		}
		if meta.RawLen != 0 {
			t.Errorf("cof=%d: RawLen = %d, want 0", cof, meta.RawLen)
		}
		if !meta.Compressed() {
			t.Errorf("cof=%d: Compressed() = false", cof)
		}
	}
}

func TestDecodeVersionMismatch(t *testing.T) {
	body := mustMsgpack(t, "x")
	for _, v := range []byte{0, 1, 9, 11, 255} {
		raw := buildFrame(v, 0, 0, 0, 0, len(body), body)
		payload, meta, err := Decode(raw)
		if err == nil {
			t.Fatalf("version=%d: expected error", v)
		}
		if !strings.Contains(err.Error(), "unsupported proto version") {
			t.Errorf("version=%d: err = %v", v, err)
		}
		if payload != nil {
			t.Errorf("version=%d: payload = %#v, want nil", v, payload)
		}
		if meta.Version != v {
			t.Errorf("version=%d: meta.Version = %d", v, meta.Version)
		}
	}
}

func TestDecodeCorruptMsgpackBody(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{"truncated map", []byte{0x81, 0xa1, 0x6b}},
		{"truncated str", []byte{0xd9, 0x40, 0x61}},
		{"reserved byte", []byte{0xc1}},
		{"truncated array", []byte{0x93, 0x01}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := buildFrame(Version, 0, 0, 0, 0, len(tt.body), tt.body)
			payload, _, err := Decode(raw)
			if err == nil {
				t.Fatalf("expected error, got %#v", payload)
			}
		})
	}
}

func TestDecodeHeaderFieldsSurviveSignedWraparound(t *testing.T) {
	tests := []struct {
		name   string
		seq    int16
		opcode int16
	}{
		{"min", math.MinInt16, math.MinInt16},
		{"max", math.MaxInt16, math.MaxInt16},
		{"negative one", -1, -1},
		{"zero", 0, 0},
		{"mixed", -32768, 32767},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, meta, err := Encode(Frame{Cmd: 200, Seq: tt.seq, Opcode: tt.opcode, Payload: "z"})
			if err != nil {
				t.Fatal(err)
			}
			if meta.Seq != tt.seq || meta.Opcode != tt.opcode {
				t.Fatalf("encode meta = %+v", meta)
			}
			_, dmeta, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if dmeta.Seq != tt.seq {
				t.Errorf("Seq = %d, want %d", dmeta.Seq, tt.seq)
			}
			if dmeta.Opcode != tt.opcode {
				t.Errorf("Opcode = %d, want %d", dmeta.Opcode, tt.opcode)
			}
			if dmeta.Cmd != 200 {
				t.Errorf("Cmd = %d, want 200", dmeta.Cmd)
			}
		})
	}
}

func TestEncodeCompressionThreshold(t *testing.T) {
	tests := []struct {
		name     string
		size     int
		wantComp bool
	}{
		{"empty", 0, false},
		{"one byte", 1, false},
		{"body just under threshold", 30, false},
		{"body at threshold", 31, false},
		{"comfortably compressible", 4096, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.Repeat("a", tt.size)
			bodyLen := len(mustMsgpack(t, in))
			_, meta, err := Encode(Frame{Payload: in})
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantComp && bodyLen > CompressThreshold {
				t.Fatalf("test setup: msgpack body %d already over threshold %d", bodyLen, CompressThreshold)
			}
			if got := meta.Compressed(); got != tt.wantComp {
				t.Fatalf("Compressed() = %v (cof=%d body=%d raw=%d), want %v",
					got, meta.Cof, meta.BodyLen, meta.RawLen, tt.wantComp)
			}
		})
	}
}

func TestEncodeCompressionThresholdIsOnMarshaledBody(t *testing.T) {
	tests := []struct {
		name        string
		size        int
		wantAttempt bool
	}{
		{"body 32 bytes", 31, false},
		{"body 34 bytes", 32, true},
		{"body 40 bytes", 38, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.Repeat("a", tt.size)
			bodyLen := len(mustMsgpack(t, in))
			if got := bodyLen > CompressThreshold; got != tt.wantAttempt {
				t.Fatalf("msgpack body = %d bytes, threshold check = %v, want %v", bodyLen, got, tt.wantAttempt)
			}
			raw, meta, err := Encode(Frame{Payload: in})
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantAttempt && meta.Compressed() {
				t.Errorf("body %d <= threshold but compressed (cof=%d)", bodyLen, meta.Cof)
			}
			if meta.RawLen != bodyLen {
				t.Errorf("RawLen = %d, want marshaled length %d", meta.RawLen, bodyLen)
			}
			got, _, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got.(string) != in {
				t.Error("roundtrip mismatch")
			}
		})
	}
}

func TestEncodeCofAlwaysSizesDecompressBuffer(t *testing.T) {
	inputs := []string{
		strings.Repeat("a", 64),
		strings.Repeat("a", 1000),
		strings.Repeat("a", 100000),
		strings.Repeat("a", 1<<20),
		strings.Repeat("abcd1234", 5000),
		strings.Repeat("the quick brown fox ", 2000),
	}
	for _, in := range inputs {
		raw, meta, err := Encode(Frame{Payload: in})
		if err != nil {
			t.Fatalf("len=%d: %v", len(in), err)
		}
		if !meta.Compressed() {
			continue
		}
		if meta.BodyLen*int(meta.Cof) < meta.RawLen {
			t.Errorf("len=%d: cof=%d undersizes buffer (%d*%d < %d)",
				len(in), meta.Cof, meta.BodyLen, meta.Cof, meta.RawLen)
		}
		got, _, err := Decode(raw)
		if err != nil {
			t.Fatalf("len=%d: decode: %v", len(in), err)
		}
		if got.(string) != in {
			t.Errorf("len=%d: roundtrip mismatch", len(in))
		}
	}
}

func TestEncodeIncompressiblePayloadStaysRaw(t *testing.T) {
	body := make([]byte, 8192)
	for i := range body {
		body[i] = byte(i*7 + i*i*13)
	}
	seq := make([]byte, len(body))
	x := uint32(0x12345678)
	for i := range seq {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		seq[i] = byte(x)
	}
	raw, meta, err := Encode(Frame{Payload: seq})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Compressed() && meta.BodyLen >= meta.RawLen {
		t.Errorf("marked compressed but body (%d) >= raw (%d)", meta.BodyLen, meta.RawLen)
	}
	got, _, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.([]byte), seq) {
		t.Error("roundtrip mismatch on incompressible payload")
	}
}

func TestEncodeRejectsBodyOverUint24(t *testing.T) {
	big := make([]byte, maxBodyLen+1)
	x := uint32(0xDEADBEEF)
	for i := range big {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		big[i] = byte(x)
	}
	raw, meta, err := Encode(Frame{Payload: big})
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("err = %v, want ErrBodyTooLarge", err)
	}
	if raw != nil {
		t.Errorf("raw = %d bytes, want nil", len(raw))
	}
	if meta != (Meta{}) {
		t.Errorf("meta = %+v, want zero", meta)
	}
}

func TestEncodeUnserializablePayload(t *testing.T) {
	tests := []struct {
		name    string
		payload any
	}{
		{"channel", make(chan int)},
		{"func", func() {}},
		{"map with chan value", map[string]any{"c": make(chan int)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, meta, err := Encode(Frame{Payload: tt.payload})
			if err == nil {
				t.Fatalf("expected error, got %d bytes", len(raw))
			}
			if raw != nil {
				t.Errorf("raw = %v, want nil", raw)
			}
			if meta != (Meta{}) {
				t.Errorf("meta = %+v, want zero", meta)
			}
		})
	}
}

func TestEncodeUint24LengthEncoding(t *testing.T) {
	for _, size := range []int{0, 1, 255, 256, 65535, 65536, 70000} {
		raw, meta, err := Encode(Frame{Payload: make([]byte, size)})
		if err != nil {
			t.Fatalf("size=%d: %v", size, err)
		}
		declared := int(raw[7])<<16 | int(raw[8])<<8 | int(raw[9])
		if declared != len(raw)-HeaderLen {
			t.Errorf("size=%d: declared %d, actual %d", size, declared, len(raw)-HeaderLen)
		}
		if declared != meta.BodyLen {
			t.Errorf("size=%d: declared %d != meta.BodyLen %d", size, declared, meta.BodyLen)
		}
	}
}

func TestRoundTripPayloadShapes(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want any
	}{
		{"nil", nil, nil},
		{"empty string", "", ""},
		{"bool", true, true},
		{"negative int", int64(-42), int64(-42)},
		{"float", 3.5, 3.5},
		{"empty slice", []any{}, []any{}},
		{"nested map", map[string]any{"a": map[string]any{"b": int8(1)}}, nil},
		{"deep array", []any{[]any{[]any{"x"}}}, nil},
		{"unicode", "日本語テキスト", "日本語テキスト"},
		{"long unicode", strings.Repeat("日本語", 500), strings.Repeat("日本語", 500)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, _, err := Encode(Frame{Cmd: 3, Seq: 1, Opcode: 2, Payload: tt.in})
			if err != nil {
				t.Fatal(err)
			}
			got, _, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if tt.want != nil {
				if s, ok := tt.want.(string); ok {
					if got.(string) != s {
						t.Errorf("got %#v, want %#v", got, tt.want)
					}
					return
				}
			}
			if tt.in == nil && got != nil {
				t.Errorf("got %#v, want nil", got)
			}
		})
	}
}

func TestMetaCompressed(t *testing.T) {
	tests := []struct {
		cof  uint8
		want bool
	}{{0, false}, {1, true}, {2, true}, {255, true}}
	for _, tt := range tests {
		if got := (Meta{Cof: tt.cof}).Compressed(); got != tt.want {
			t.Errorf("cof=%d: Compressed() = %v, want %v", tt.cof, got, tt.want)
		}
	}
}

func TestMarshalJSONNormalizesNestedKeys(t *testing.T) {
	tests := []struct {
		name     string
		in       any
		contains []string
	}{
		{
			name:     "map[any]any keys stringified",
			in:       map[any]any{1: "one", true: "yes"},
			contains: []string{`"1": "one"`, `"true": "yes"`},
		},
		{
			name:     "nested map[any]any inside slice",
			in:       []any{map[any]any{int8(7): "seven"}},
			contains: []string{`"7": "seven"`},
		},
		{
			name:     "nested map[any]any inside map[string]any",
			in:       map[string]any{"outer": map[any]any{"k": "v"}},
			contains: []string{`"outer"`, `"k": "v"`},
		},
		{
			name:     "slice of scalars",
			in:       []any{int64(1), "two", nil},
			contains: []string{"1", `"two"`, "null"},
		},
		{
			name:     "deeply nested",
			in:       map[any]any{"a": []any{map[any]any{"b": []any{"c"}}}},
			contains: []string{`"a"`, `"b"`, `"c"`},
		},
		{
			name:     "scalar passthrough",
			in:       "plain",
			contains: []string{`"plain"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js, err := MarshalJSON(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(js, want) {
					t.Errorf("json missing %q:\n%s", want, js)
				}
			}
		})
	}
}

func TestMarshalJSONErrors(t *testing.T) {
	tests := []struct {
		name string
		in   any
	}{
		{"channel", make(chan int)},
		{"nan", math.NaN()},
		{"positive infinity", math.Inf(1)},
		{"nested channel in map", map[string]any{"c": make(chan int)}},
		{"nested channel in slice", []any{make(chan int)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := MarshalJSON(tt.in)
			if err == nil {
				t.Fatalf("expected error, got %q", s)
			}
			if s != "" {
				t.Errorf("string = %q, want empty on error", s)
			}
		})
	}
}

func TestDecodeEncodeRoundTripThroughReferenceFraming(t *testing.T) {
	tests := []struct {
		name    string
		payload any
	}{
		{"small map", map[string]any{"k": "v"}},
		{"compressible string", strings.Repeat("xyz-", 300)},
		{"nil", nil},
		{"array", []any{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, meta, err := Encode(Frame{Cmd: 5, Seq: -7, Opcode: 9, Payload: tt.payload})
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) != HeaderLen+meta.BodyLen {
				t.Fatalf("frame len %d != %d + %d", len(raw), HeaderLen, meta.BodyLen)
			}
			if raw[6] != meta.Cof {
				t.Errorf("header cof %d != meta.Cof %d", raw[6], meta.Cof)
			}
			_, dmeta, err := Decode(raw)
			if err != nil {
				t.Fatal(err)
			}
			if dmeta.Cmd != meta.Cmd || dmeta.Seq != meta.Seq || dmeta.Opcode != meta.Opcode {
				t.Errorf("meta mismatch: %+v vs %+v", dmeta, meta)
			}
			if dmeta.RawLen != meta.RawLen {
				t.Errorf("RawLen %d != %d", dmeta.RawLen, meta.RawLen)
			}
		})
	}
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, HeaderLen))
	good, _, _ := Encode(Frame{Cmd: 1, Seq: 2, Opcode: 3, Payload: map[string]any{"k": "v"}})
	f.Add(good)
	comp, _, _ := Encode(Frame{Payload: strings.Repeat("abcd", 500)})
	f.Add(comp)
	f.Fuzz(func(t *testing.T, raw []byte) {
		payload, meta, err := Decode(raw)
		if err != nil {
			return
		}
		if meta.Version != Version {
			t.Fatalf("accepted version %d", meta.Version)
		}
		if HeaderLen+meta.BodyLen > len(raw) {
			t.Fatalf("BodyLen %d exceeds frame %d", meta.BodyLen, len(raw))
		}
		if meta.BodyLen == 0 && payload != nil && !meta.Compressed() {
			t.Fatalf("empty body produced payload %#v", payload)
		}
	})
}

func frameWith(t *testing.T, version, cmd, cof byte, body []byte) []byte {
	t.Helper()
	raw := make([]byte, HeaderLen+len(body))
	raw[0] = version
	raw[1] = cmd
	raw[6] = cof
	raw[7] = byte(len(body) >> 16)
	raw[8] = byte(len(body) >> 8)
	raw[9] = byte(len(body))
	copy(raw[HeaderLen:], body)
	return raw
}

func TestDecodeRejectsHugeDeclaredMsgpackMap(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"map32 declaring 2^32-1 entries", []byte{0xdf, 0xff, 0xff, 0xff, 0xff}},
		{"array32 declaring 2^32-1 entries", []byte{0xdd, 0xff, 0xff, 0xff, 0xff}},
		{"fuzz-found map32 + array32", []byte{0xdf, 0x30, 0x30, 0xdc, 0x30}},
		{"map16 declaring 65535 entries", []byte{0xde, 0xff, 0xff}},
		{"array16 declaring 65535 entries", []byte{0xdc, 0xff, 0xff}},
		{"str32 declaring 2^32-1 bytes", []byte{0xdb, 0xff, 0xff, 0xff, 0xff}},
		{"bin32 declaring 2^32-1 bytes", []byte{0xc6, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := Decode(frameWith(t, Version, 0, 0, c.body))
			if err == nil {
				t.Fatal("a body declaring more elements than it can hold must be rejected")
			}
		})
	}
}

func TestDecodeRejectsOversizedDecompressTarget(t *testing.T) {
	body := bytes.Repeat([]byte{0x01}, 1024)
	_, _, err := Decode(frameWith(t, Version, 0, 255, body))
	if err == nil {
		t.Fatal("expected an error for an implausible decompressed size or a failed decompress")
	}
}

func TestDecodeChecksVersionBeforeAllocating(t *testing.T) {
	body := bytes.Repeat([]byte{0x01}, 1024)
	_, m, err := Decode(frameWith(t, Version+1, 0, 255, body))
	if err == nil {
		t.Fatal("a frame with an unsupported version must be rejected")
	}
	if m.RawLen != 0 {
		t.Errorf("RawLen = %d, want 0: the version must be checked before decompressing", m.RawLen)
	}
}

func TestDecodeRawLenLimitConstant(t *testing.T) {
	if MaxRawLen <= 0 || MaxRawLen > 1<<31 {
		t.Errorf("MaxRawLen = %d, want a sane positive bound", MaxRawLen)
	}
	if !errors.Is(ErrRawTooLarge, ErrRawTooLarge) {
		t.Error("ErrRawTooLarge must be a usable sentinel")
	}
}

func TestValidateMsgpackAcceptsRealPayloads(t *testing.T) {
	payloads := []any{
		nil,
		true,
		false,
		int64(0),
		int64(-1),
		int64(1 << 40),
		float64(1.5),
		"hello",
		"",
		"ünïcode ✓ 日本語",
		[]any{},
		[]any{int64(1), int64(2), int64(3)},
		map[string]any{},
		map[string]any{"a": int64(1), "b": "two"},
		map[string]any{"nested": []any{map[string]any{"deep": true}}},
		bytes.Repeat([]byte{0xAB}, 300),
		make([]byte, 70000),
	}
	for i, p := range payloads {
		b, err := msgpack.Marshal(p)
		if err != nil {
			t.Fatalf("marshal[%d]: %v", i, err)
		}
		if err := validateMsgpack(b); err != nil {
			t.Errorf("validateMsgpack rejected a valid payload[%d] (%T): %v", i, p, err)
		}
	}
}

func TestValidateMsgpackLargeCollections(t *testing.T) {
	big := make([]any, 5000)
	for i := range big {
		big[i] = int64(i)
	}
	b, err := msgpack.Marshal(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMsgpack(b); err != nil {
		t.Errorf("validateMsgpack rejected a large but valid array: %v", err)
	}

	m := map[string]any{}
	for i := 0; i < 2000; i++ {
		m[string(rune('a'+i%26))+string(rune('a'+i/26))] = int64(i)
	}
	b, err = msgpack.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateMsgpack(b); err != nil {
		t.Errorf("validateMsgpack rejected a large but valid map: %v", err)
	}
}

func TestValidateMsgpackRejectsMalformed(t *testing.T) {
	cases := []struct {
		name string
		body []byte
	}{
		{"truncated str8 length", []byte{0xd9}},
		{"str8 body short", []byte{0xd9, 0x05, 'a', 'b'}},
		{"never-used byte", []byte{0xc1}},
		{"trailing garbage", append(mustMarshal(t, "x"), 0x01)},
		{"fixarray missing element", []byte{0x91}},
		{"fixmap missing value", []byte{0x81, 0xa1, 'k'}},
		{"empty", []byte{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := validateMsgpack(c.body); err == nil {
				t.Errorf("validateMsgpack accepted malformed input % x", c.body)
			}
		})
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := msgpack.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEncodeDecodeRoundTripStillWorks(t *testing.T) {
	payloads := []any{
		map[string]any{"cmd": "hello", "n": int64(42)},
		[]any{int64(1), "two", true},
		"a short string",
		bytes.Repeat([]byte{0x7f}, 4096),
	}
	for _, p := range payloads {
		raw, _, err := Encode(Frame{Cmd: 1, Seq: 2, Opcode: 3, Payload: p})
		if err != nil {
			t.Fatalf("Encode: %v", err)
		}
		got, m, err := Decode(raw)
		if err != nil {
			t.Fatalf("Decode: %v", err)
		}
		if m.Version != Version {
			t.Errorf("version = %d, want %d", m.Version, Version)
		}
		if !reflect.DeepEqual(got, p) {
			t.Errorf("round trip changed the payload:\n got %#v\nwant %#v", got, p)
		}
	}
}

func TestValidateMsgpackNoPanicOnArbitraryBytes(t *testing.T) {
	for i := 0; i < 256; i++ {
		body := []byte{byte(i)}
		_ = validateMsgpack(body)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(i)*0x0101010101010101)
		_ = validateMsgpack(buf)
	}
}

func TestEncodeHeaderLayout(t *testing.T) {
	raw, meta, err := Encode(Frame{Cmd: 7, Seq: -3, Opcode: 1234, Payload: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < HeaderLen {
		t.Fatalf("frame too short: %d", len(raw))
	}
	if raw[0] != Version {
		t.Fatalf("version = %d, want %d", raw[0], Version)
	}
	if raw[1] != 7 {
		t.Fatalf("cmd = %d, want 7", raw[1])
	}
	if got := int16(binary.BigEndian.Uint16(raw[2:])); got != -3 {
		t.Fatalf("seq = %d, want -3", got)
	}
	if got := int16(binary.BigEndian.Uint16(raw[4:])); got != 1234 {
		t.Fatalf("opcode = %d, want 1234", got)
	}
	if raw[6] != 0 {
		t.Fatalf("cof = %d, want 0 (small payload uncompressed)", raw[6])
	}
	length := int(raw[7])<<16 | int(raw[8])<<8 | int(raw[9])
	if length != len(raw)-HeaderLen {
		t.Fatalf("uint24 length = %d, want %d", length, len(raw)-HeaderLen)
	}
	if meta.Cof != 0 || meta.BodyLen != length {
		t.Fatalf("meta mismatch: %+v", meta)
	}
}

func TestRoundTripSmall(t *testing.T) {
	in := map[string]any{"a": int8(1), "b": "two", "c": true}
	raw, _, err := Encode(Frame{Cmd: 1, Seq: 9, Opcode: 2, Payload: in})
	if err != nil {
		t.Fatal(err)
	}
	payload, meta, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Cmd != 1 || meta.Seq != 9 || meta.Opcode != 2 {
		t.Fatalf("header meta mismatch: %+v", meta)
	}
	if meta.Cof != 0 {
		t.Fatalf("small payload should not be compressed, cof=%d", meta.Cof)
	}
	m, ok := payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T", payload)
	}
	if m["b"] != "two" || m["c"] != true {
		t.Fatalf("payload mismatch: %#v", m)
	}
}

func TestRoundTripCompressed(t *testing.T) {
	in := map[string]any{"text": strings.Repeat("abcd1234", 200)}
	raw, meta, err := Encode(Frame{Cmd: 2, Seq: 100, Opcode: 5, Payload: in})
	if err != nil {
		t.Fatal(err)
	}
	if meta.Cof == 0 {
		t.Fatalf("repetitive payload should compress, cof=%d", meta.Cof)
	}
	if meta.BodyLen >= meta.RawLen {
		t.Fatalf("compressed body (%d) not smaller than raw (%d)", meta.BodyLen, meta.RawLen)
	}
	if int(meta.BodyLen)*int(meta.Cof) < meta.RawLen {
		t.Fatalf("cof %d too small to size decompress buffer (body=%d raw=%d)", meta.Cof, meta.BodyLen, meta.RawLen)
	}
	payload, dmeta, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if dmeta.Cof != meta.Cof {
		t.Fatalf("cof roundtrip mismatch: %d vs %d", dmeta.Cof, meta.Cof)
	}
	m := payload.(map[string]any)
	if m["text"] != in["text"] {
		t.Fatal("decompressed text mismatch")
	}
}

func TestDecodeMatchesReferenceFraming(t *testing.T) {
	body, err := msgpack.Marshal(map[string]any{"k": "v"})
	if err != nil {
		t.Fatal(err)
	}
	raw := make([]byte, HeaderLen+len(body))
	raw[0] = 10
	raw[1] = 42
	seq, opcode := int16(-1), int16(7)
	binary.BigEndian.PutUint16(raw[2:], uint16(seq))
	binary.BigEndian.PutUint16(raw[4:], uint16(opcode))
	raw[6] = 0
	raw[7] = byte(len(body) >> 16)
	raw[8] = byte(len(body) >> 8)
	raw[9] = byte(len(body))
	copy(raw[HeaderLen:], body)

	payload, meta, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Cmd != 42 || meta.Seq != -1 || meta.Opcode != 7 {
		t.Fatalf("meta mismatch: %+v", meta)
	}
	if payload.(map[string]any)["k"] != "v" {
		t.Fatalf("payload mismatch: %#v", payload)
	}
}

func TestDecodeReferenceCompressedBlock(t *testing.T) {
	body, err := msgpack.Marshal(map[string]any{"text": strings.Repeat("xyz-", 300)})
	if err != nil {
		t.Fatal(err)
	}
	comp := make([]byte, lz4.CompressBlockBound(len(body)))
	var c lz4.Compressor
	n, err := c.CompressBlock(body, comp)
	if err != nil || n == 0 {
		t.Fatalf("compress failed n=%d err=%v", n, err)
	}
	comp = comp[:n]
	cof := byte((len(body) + n - 1) / n)
	raw := make([]byte, HeaderLen+len(comp))
	raw[0] = 10
	raw[6] = cof
	raw[7] = byte(len(comp) >> 16)
	raw[8] = byte(len(comp) >> 8)
	raw[9] = byte(len(comp))
	copy(raw[HeaderLen:], comp)

	payload, meta, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.Compressed() {
		t.Fatal("expected compressed meta")
	}
	if payload.(map[string]any)["text"] != strings.Repeat("xyz-", 300) {
		t.Fatal("payload mismatch")
	}
}

func TestDecodeErrors(t *testing.T) {
	if _, _, err := Decode([]byte{1, 2, 3}); err != ErrShortHeader {
		t.Fatalf("short header err = %v", err)
	}
	raw := make([]byte, HeaderLen)
	raw[0] = 10
	raw[9] = 50
	if _, _, err := Decode(raw); err != ErrTruncatedBody {
		t.Fatalf("truncated err = %v", err)
	}
	bad := make([]byte, HeaderLen)
	bad[0] = 9
	if _, _, err := Decode(bad); err == nil {
		t.Fatal("expected version error")
	}
}

func TestMarshalJSON(t *testing.T) {
	js, err := MarshalJSON(map[string]any{"n": int64(5), "s": "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js, "\"s\": \"hi\"") {
		t.Fatalf("json missing field: %s", js)
	}
}

func TestEncodeEmptyPayload(t *testing.T) {
	raw, _, err := Encode(Frame{Cmd: 0, Seq: 0, Opcode: 0, Payload: nil})
	if err != nil {
		t.Fatal(err)
	}
	payload, _, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if payload != nil {
		t.Fatalf("nil payload decoded to %#v", payload)
	}
	if !bytes.Equal(raw[:1], []byte{Version}) {
		t.Fatal("version byte wrong")
	}
}
