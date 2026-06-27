package wsproto

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/pierrec/lz4/v4"
	"github.com/vmihailenco/msgpack/v5"
)

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
