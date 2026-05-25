package ws

import (
	"bytes"
	"testing"
)

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
