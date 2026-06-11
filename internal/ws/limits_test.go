package ws

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"testing"
)

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
