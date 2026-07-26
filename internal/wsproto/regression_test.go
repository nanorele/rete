package wsproto

import (
	"bytes"
	"encoding/binary"
	"errors"
	"reflect"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

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
