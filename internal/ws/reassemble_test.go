package ws

import (
	"bytes"
	"testing"
)

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
