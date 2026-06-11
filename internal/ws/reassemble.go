package ws

import (
	"bytes"
	"errors"
	"slices"
)

var (
	ErrUnexpectedContinuation = errors.New("ws: continuation frame without an open message")
	ErrUnexpectedDataFrame    = errors.New("ws: new data frame while another is open")
	ErrUnexpectedRSV1         = errors.New("ws: RSV1 set on continuation frame")
)

type Assembled struct {
	Opcode     Opcode
	Payload    []byte
	Compressed bool
	Control    bool
}

type Reassembler struct {
	opcode     Opcode
	compressed bool
	open       bool
	buf        bytes.Buffer
}

func (r *Reassembler) Step(hdr Header, payload []byte) (Assembled, bool, error) {
	if hdr.Opcode.IsControl() {
		return Assembled{Opcode: hdr.Opcode, Payload: payload, Control: true}, true, nil
	}
	if hdr.Opcode == OpContinuation {
		if !r.open {
			return Assembled{}, false, ErrUnexpectedContinuation
		}
		if hdr.RSV1 {
			return Assembled{}, false, ErrUnexpectedRSV1
		}
		if uint64(r.buf.Len())+uint64(len(payload)) > MaxMessageSize {
			r.reset()
			return Assembled{}, false, ErrMessageTooLarge
		}
		r.buf.Write(payload)
		if !hdr.FIN {
			return Assembled{}, false, nil
		}
		out := Assembled{
			Opcode:     r.opcode,
			Payload:    slices.Clone(r.buf.Bytes()),
			Compressed: r.compressed,
		}
		r.reset()
		return out, true, nil
	}
	if r.open {
		return Assembled{}, false, ErrUnexpectedDataFrame
	}
	if hdr.FIN {
		return Assembled{
			Opcode:     hdr.Opcode,
			Payload:    payload,
			Compressed: hdr.RSV1,
		}, true, nil
	}
	r.opcode = hdr.Opcode
	r.compressed = hdr.RSV1
	r.open = true
	r.buf.Reset()
	r.buf.Write(payload)
	return Assembled{}, false, nil
}

func (r *Reassembler) reset() {
	r.opcode = 0
	r.compressed = false
	r.open = false
	r.buf.Reset()
}
