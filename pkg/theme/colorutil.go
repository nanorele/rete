package theme

import (
	"image/color"
	"math"
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
		s := float64(v) / 255
		if s <= 0.03928 {
			return float32(s / 12.92)
		}
		return float32(math.Pow((s+0.055)/1.055, 2.4))
	}
	return 0.2126*chan01(c.R) + 0.7152*chan01(c.G) + 0.0722*chan01(c.B)
}

// Levels taken from VS Code Dark+, where findMatchBackground (#515C6A) sits a
// fixed ~2.4:1 above the editor background whatever token it lands on, and
// findMatchHighlightBackground is the same tone made translucent. Fixing the
// level against the background rather than against the glyphs is what keeps
// every match the same brightness; only the hue follows the text.
const (
	searchFillBgRatio   = 2.4
	searchFillMinText   = 2.0
	searchFillDesat     = 0.4
	searchFillInactiveA = 110
)

// SearchFill derives a find-match background from the colour of the glyphs it
// covers instead of from the accent, so a match carries the hue of the token it
// sits on. active picks the current match's solid fill over the translucent one
// used for the rest.
func SearchFill(text, bg color.NRGBA, active bool) color.NRGBA {
	yb := RelLuminance(bg)
	dark := yb < 0.5

	var target float32
	if dark {
		target = (yb+0.05)*searchFillBgRatio - 0.05
	} else {
		target = (yb+0.05)/searchFillBgRatio - 0.05
	}

	// A token whose own luminance sits on top of that level would disappear
	// into its highlight, so step the fill further away from the background.
	yt := RelLuminance(text)
	for i := 0; i < 8 && contrastOf(yt, target) < searchFillMinText; i++ {
		if dark {
			target = (yt+0.05)/searchFillMinText - 0.05
		} else {
			target = (yt+0.05)*searchFillMinText - 0.05
		}
	}
	target = clamp01(target)

	tone := Mix(text, GreyAt(yt), searchFillDesat)
	fill := toneAtLuminance(tone, target)
	if !active {
		fill.A = searchFillInactiveA
	}
	return fill
}

func contrastOf(a, b float32) float32 {
	if a < b {
		a, b = b, a
	}
	return (a + 0.05) / (b + 0.05)
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// toneAtLuminance walks c along black → c → white, which is monotone in
// luminance, until it lands on target. Solving numerically rather than scaling
// the channels keeps it exact: sRGB mixing shifts luminance enough that a
// single closed-form scale lands well off the mark for saturated colours.
func toneAtLuminance(c color.NRGBA, target float32) color.NRGBA {
	black := color.NRGBA{A: 255}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	at := func(t float32) color.NRGBA {
		if t <= 0.5 {
			return Mix(black, c, t*2)
		}
		return Mix(c, white, (t-0.5)*2)
	}
	lo, hi := float32(0), float32(1)
	out := c
	for i := 0; i < 20; i++ {
		mid := (lo + hi) / 2
		out = at(mid)
		if RelLuminance(out) < target {
			lo = mid
		} else {
			hi = mid
		}
	}
	return out
}

// GreyAt returns the neutral colour with the given relative luminance.
func GreyAt(y float32) color.NRGBA {
	var s float64
	switch {
	case y <= 0:
		s = 0
	case y >= 1:
		s = 1
	case float64(y) <= 0.0031308:
		s = float64(y) * 12.92
	default:
		s = 1.055*math.Pow(float64(y), 1.0/2.4) - 0.055
	}
	v := uint8(s*255 + 0.5)
	return color.NRGBA{R: v, G: v, B: v, A: 255}
}

func ContrastRatio(a, b color.NRGBA) float32 {
	la := RelLuminance(a)
	lb := RelLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func ContrastOn(bg color.NRGBA) color.NRGBA {
	return BestTextOn(bg)
}

func BestTextOn(bgs ...color.NRGBA) color.NRGBA {
	black := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	blackMin, whiteMin := float32(99), float32(99)
	for _, bg := range bgs {
		if r := ContrastRatio(bg, black); r < blackMin {
			blackMin = r
		}
		if r := ContrastRatio(bg, white); r < whiteMin {
			whiteMin = r
		}
	}
	if blackMin >= whiteMin {
		return black
	}
	return white
}

func AdjustForContrast(fg, bg color.NRGBA, minRatio float32) color.NRGBA {
	if ContrastRatio(fg, bg) >= minRatio {
		return fg
	}
	toward := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	if RelLuminance(bg) < 0.5 {
		toward = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	}
	out := fg
	lo, hi := float32(0), float32(1)
	for i := 0; i < 16; i++ {
		mid := (lo + hi) / 2
		cand := Mix(fg, toward, mid)
		cand.A = fg.A
		if ContrastRatio(cand, bg) >= minRatio {
			out = cand
			hi = mid
		} else {
			lo = mid
		}
	}
	return out
}

func Composite(fg, bg color.NRGBA) color.NRGBA {
	a := float32(fg.A) / 255
	mix := func(f, b uint8) uint8 {
		return uint8(float32(f)*a + float32(b)*(1-a))
	}
	return color.NRGBA{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B), A: 255}
}

func ContrastBgFor(bg, text color.NRGBA, minRatio float32) color.NRGBA {
	if ContrastRatio(bg, text) >= minRatio {
		return bg
	}
	toward := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	if RelLuminance(bg) <= RelLuminance(text) {
		toward = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	}
	out := bg
	lo, hi := float32(0), float32(1)
	for i := 0; i < 16; i++ {
		mid := (lo + hi) / 2
		cand := Mix(bg, toward, mid)
		if ContrastRatio(cand, text) >= minRatio {
			out = cand
			hi = mid
		} else {
			lo = mid
		}
	}
	return out
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
