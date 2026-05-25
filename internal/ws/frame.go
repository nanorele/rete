package ws

import (
	"encoding/binary"
	"errors"
	"io"
)

var (
	ErrControlFrameTooLong  = errors.New("ws: control frame payload > 125 bytes")
	ErrControlFrameNotFinal = errors.New("ws: control frame must be FIN=1")
	ErrInvalidOpcode        = errors.New("ws: invalid opcode in current state")
	ErrUnmaskedFromClient   = errors.New("ws: client frame must be masked")
	ErrMaskedFromServer     = errors.New("ws: server frame must not be masked")
)

type Header struct {
	FIN     bool
	RSV1    bool
	RSV2    bool
	RSV3    bool
	Opcode  Opcode
	Masked  bool
	MaskKey [4]byte
	Length  uint64
}

func ReadFrame(r io.Reader) (Header, []byte, error) {
	var hdr Header
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return hdr, nil, err
	}
	hdr.FIN = b[0]&0x80 != 0
	hdr.RSV1 = b[0]&0x40 != 0
	hdr.RSV2 = b[0]&0x20 != 0
	hdr.RSV3 = b[0]&0x10 != 0
	hdr.Opcode = Opcode(b[0] & 0x0F)
	hdr.Masked = b[1]&0x80 != 0
	l := uint64(b[1] & 0x7F)
	switch l {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return hdr, nil, err
		}
		hdr.Length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			return hdr, nil, err
		}
		hdr.Length = binary.BigEndian.Uint64(ext[:])
	default:
		hdr.Length = l
	}
	if hdr.Opcode.IsControl() {
		if !hdr.FIN {
			return hdr, nil, ErrControlFrameNotFinal
		}
		if hdr.Length > 125 {
			return hdr, nil, ErrControlFrameTooLong
		}
	}
	if hdr.Masked {
		if _, err := io.ReadFull(r, hdr.MaskKey[:]); err != nil {
			return hdr, nil, err
		}
	}
	payload := make([]byte, hdr.Length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return hdr, nil, err
	}
	if hdr.Masked {
		applyMask(payload, hdr.MaskKey)
	}
	return hdr, payload, nil
}

func WriteFrame(w io.Writer, hdr Header, payload []byte) error {
	var head [14]byte
	head[0] = byte(hdr.Opcode) & 0x0F
	if hdr.FIN {
		head[0] |= 0x80
	}
	if hdr.RSV1 {
		head[0] |= 0x40
	}
	if hdr.RSV2 {
		head[0] |= 0x20
	}
	if hdr.RSV3 {
		head[0] |= 0x10
	}
	n := 2
	l := uint64(len(payload))
	switch {
	case l < 126:
		head[1] = byte(l)
	case l <= 0xFFFF:
		head[1] = 126
		binary.BigEndian.PutUint16(head[2:4], uint16(l))
		n += 2
	default:
		head[1] = 127
		binary.BigEndian.PutUint64(head[2:10], l)
		n += 8
	}
	if hdr.Masked {
		head[1] |= 0x80
		copy(head[n:n+4], hdr.MaskKey[:])
		n += 4
	}
	if _, err := w.Write(head[:n]); err != nil {
		return err
	}
	if !hdr.Masked || len(payload) == 0 {
		if len(payload) > 0 {
			if _, err := w.Write(payload); err != nil {
				return err
			}
		}
		return nil
	}
	masked := make([]byte, len(payload))
	copy(masked, payload)
	applyMask(masked, hdr.MaskKey)
	_, err := w.Write(masked)
	return err
}

func applyMask(p []byte, key [4]byte) {
	for i := range p {
		p[i] ^= key[i&3]
	}
}
