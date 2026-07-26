package wsproto

import "encoding/binary"

func validateMsgpack(b []byte) error {
	pending := 1
	i := 0
	for pending > 0 {
		if i >= len(b) {
			return ErrBadMsgpack
		}
		c := b[i]
		i++
		pending--

		var skip, elems int
		switch {
		case c <= 0x7f, c >= 0xe0:
		case c >= 0x80 && c <= 0x8f:
			elems = 2 * int(c&0x0f)
		case c >= 0x90 && c <= 0x9f:
			elems = int(c & 0x0f)
		case c >= 0xa0 && c <= 0xbf:
			skip = int(c & 0x1f)
		default:
			switch c {
			case 0xc0, 0xc2, 0xc3:
			case 0xc1:
				return ErrBadMsgpack
			case 0xc4, 0xc5, 0xc6, 0xd9, 0xda, 0xdb:
				n, adv, err := readLen(b, i, lenBytes(c))
				if err != nil {
					return err
				}
				i += adv
				skip = n
			case 0xc7, 0xc8, 0xc9:
				n, adv, err := readLen(b, i, lenBytes(c))
				if err != nil {
					return err
				}
				i += adv
				skip = n + 1
			case 0xca:
				skip = 4
			case 0xcb:
				skip = 8
			case 0xcc, 0xd0:
				skip = 1
			case 0xcd, 0xd1:
				skip = 2
			case 0xce, 0xd2:
				skip = 4
			case 0xcf, 0xd3:
				skip = 8
			case 0xd4:
				skip = 2
			case 0xd5:
				skip = 3
			case 0xd6:
				skip = 5
			case 0xd7:
				skip = 9
			case 0xd8:
				skip = 17
			case 0xdc, 0xdd:
				n, adv, err := readLen(b, i, lenBytes(c))
				if err != nil {
					return err
				}
				i += adv
				elems = n
			case 0xde, 0xdf:
				n, adv, err := readLen(b, i, lenBytes(c))
				if err != nil {
					return err
				}
				i += adv
				if n > (len(b)-i)/2 {
					return ErrBadMsgpack
				}
				elems = 2 * n
			default:
				return ErrBadMsgpack
			}
		}

		if skip < 0 || skip > len(b)-i {
			return ErrBadMsgpack
		}
		i += skip

		if elems < 0 || elems > len(b)-i {
			return ErrBadMsgpack
		}
		pending += elems
	}
	if i != len(b) {
		return ErrBadMsgpack
	}
	return nil
}

func lenBytes(c byte) int {
	switch c {
	case 0xc4, 0xc7, 0xd9:
		return 1
	case 0xc5, 0xc8, 0xda, 0xdc, 0xde:
		return 2
	}
	return 4
}

func readLen(b []byte, i, width int) (int, int, error) {
	if len(b)-i < width {
		return 0, 0, ErrBadMsgpack
	}
	var n uint64
	switch width {
	case 1:
		n = uint64(b[i])
	case 2:
		n = uint64(binary.BigEndian.Uint16(b[i:]))
	default:
		n = uint64(binary.BigEndian.Uint32(b[i:]))
	}
	if n > uint64(len(b)) {
		return 0, 0, ErrBadMsgpack
	}
	return int(n), width, nil
}
