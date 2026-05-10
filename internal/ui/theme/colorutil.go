package theme

import (
	"image/color"
	"strconv"
	"strings"
)

func Shade(c color.NRGBA, amt float32) color.NRGBA {
	f := func(v uint8) uint8 {
		x := float32(v)
		if amt < 0 {
			x = x * (1 + amt)
		} else {
			x = x + (255-x)*amt
		}
		if x < 0 {
			x = 0
		}
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return color.NRGBA{R: f(c.R), G: f(c.G), B: f(c.B), A: c.A}
}

func Mix(a, b color.NRGBA, t float32) color.NRGBA {
	mf := func(av, bv uint8) uint8 {
		return uint8(float32(av)*(1-t) + float32(bv)*t)
	}
	return color.NRGBA{R: mf(a.R, b.R), G: mf(a.G, b.G), B: mf(a.B, b.B), A: 255}
}

func WithAlpha(c color.NRGBA, a uint8) color.NRGBA {
	return color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

func RelLuminance(c color.NRGBA) float32 {
	chan01 := func(v uint8) float32 {
		s := float32(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		x := (s + 0.055) / 1.055
		return x * x * x
	}
	return 0.2126*chan01(c.R) + 0.7152*chan01(c.G) + 0.0722*chan01(c.B)
}

func ContrastOn(bg color.NRGBA) color.NRGBA {
	if RelLuminance(bg) > 0.45 {
		return color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	}
	return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
}

func ParseHex(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{
		R: uint8((v >> 16) & 0xFF),
		G: uint8((v >> 8) & 0xFF),
		B: uint8(v & 0xFF),
		A: 255,
	}, true
}

func HexFromColor(c color.NRGBA) string {
	const hex = "0123456789abcdef"
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	out[1] = hex[c.R>>4]
	out[2] = hex[c.R&0x0F]
	out[3] = hex[c.G>>4]
	out[4] = hex[c.G&0x0F]
	out[5] = hex[c.B>>4]
	out[6] = hex[c.B&0x0F]
	return string(out)
}
