package ws

type Opcode byte

const (
	OpContinuation Opcode = 0x0
	OpText         Opcode = 0x1
	OpBinary       Opcode = 0x2
	OpClose        Opcode = 0x8
	OpPing         Opcode = 0x9
	OpPong         Opcode = 0xA
)

func (o Opcode) IsControl() bool { return o&0x8 != 0 }

func (o Opcode) IsData() bool { return o == OpText || o == OpBinary || o == OpContinuation }

func (o Opcode) String() string {
	switch o {
	case OpContinuation:
		return "CONT"
	case OpText:
		return "TEXT"
	case OpBinary:
		return "BIN"
	case OpClose:
		return "CLOSE"
	case OpPing:
		return "PING"
	case OpPong:
		return "PONG"
	default:
		return "OP?"
	}
}
