package wsproto

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/pierrec/lz4/v4"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	Version           = 10
	HeaderLen         = 10
	CompressThreshold = 32
	maxBodyLen        = 1<<24 - 1
)

var (
	ErrShortHeader   = errors.New("frame shorter than 10-byte header")
	ErrTruncatedBody = errors.New("declared body length exceeds frame")
	ErrBodyTooLarge  = errors.New("body exceeds 24-bit length limit")
)

type Frame struct {
	Cmd     uint8
	Seq     int16
	Opcode  int16
	Payload any
}

type Meta struct {
	Version uint8
	Cmd     uint8
	Seq     int16
	Opcode  int16
	Cof     uint8
	BodyLen int
	RawLen  int
}

func (m Meta) Compressed() bool { return m.Cof > 0 }

func Encode(f Frame) ([]byte, Meta, error) {
	body, err := msgpack.Marshal(f.Payload)
	if err != nil {
		return nil, Meta{}, err
	}
	rawLen := len(body)
	cof := byte(0)
	if len(body) > CompressThreshold {
		dst := make([]byte, lz4.CompressBlockBound(len(body)))
		var c lz4.Compressor
		n, cerr := c.CompressBlock(body, dst)
		if cerr != nil {
			return nil, Meta{}, cerr
		}
		if n > 0 && n < len(body) {
			ratio := min(int(math.Ceil(float64(rawLen)/float64(n))), 255)
			if ratio > 0 {
				cof = byte(ratio)
				body = dst[:n]
			}
		}
	}
	if len(body) > maxBodyLen {
		return nil, Meta{}, ErrBodyTooLarge
	}
	out := make([]byte, HeaderLen+len(body))
	out[0] = Version
	out[1] = f.Cmd
	binary.BigEndian.PutUint16(out[2:], uint16(f.Seq))
	binary.BigEndian.PutUint16(out[4:], uint16(f.Opcode))
	out[6] = cof
	out[7] = byte(len(body) >> 16)
	out[8] = byte(len(body) >> 8)
	out[9] = byte(len(body))
	copy(out[HeaderLen:], body)

	meta := Meta{
		Version: Version,
		Cmd:     f.Cmd,
		Seq:     f.Seq,
		Opcode:  f.Opcode,
		Cof:     cof,
		BodyLen: len(body),
		RawLen:  rawLen,
	}
	return out, meta, nil
}

func Decode(raw []byte) (any, Meta, error) {
	if len(raw) < HeaderLen {
		return nil, Meta{}, ErrShortHeader
	}
	m := Meta{
		Version: raw[0],
		Cmd:     raw[1],
		Seq:     int16(binary.BigEndian.Uint16(raw[2:])),
		Opcode:  int16(binary.BigEndian.Uint16(raw[4:])),
		Cof:     raw[6],
	}
	length := int(raw[7])<<16 | int(raw[8])<<8 | int(raw[9])
	if HeaderLen+length > len(raw) {
		return nil, m, ErrTruncatedBody
	}
	m.BodyLen = length
	body := raw[HeaderLen : HeaderLen+length]

	if m.Cof > 0 {
		dst := make([]byte, length*int(m.Cof))
		n, err := lz4.UncompressBlock(body, dst)
		if err != nil {
			return nil, m, err
		}
		body = dst[:n]
	}
	m.RawLen = len(body)

	if m.Version != Version {
		return nil, m, fmt.Errorf("unsupported proto version %d (want %d)", m.Version, Version)
	}

	var payload any
	if len(body) > 0 {
		if err := msgpack.Unmarshal(body, &payload); err != nil {
			return nil, m, err
		}
	}
	return payload, m, nil
}

func MarshalJSON(v any) (string, error) {
	b, err := json.MarshalIndent(normalize(v), "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func normalize(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalize(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[fmt.Sprint(k)] = normalize(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalize(val)
		}
		return out
	default:
		return v
	}
}
