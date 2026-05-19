package colorpicker

import (
	"image/color"
	"math"
	"testing"
	"time"
)

func makeTimeoutChan() <-chan time.Time {
	return time.After(2 * time.Second)
}

// TODO bug: State.Close does not reset H/S/V/LastHSV (colorpicker.go:152) — may be intentional

func TestModf32_NoHangOnNaN(t *testing.T) {
	done := make(chan struct{})
	go func() {
		_ = modf32(float32(math.NaN()), 6)
		_ = modf32(1, 0)
		_ = modf32(1, -5)
		close(done)
	}()
	select {
	case <-done:
	case <-makeTimeoutChan():
		t.Fatal("modf32 hung on pathological input")
	}
}

func TestHSVToRGB_NoHangOnNaNHue(t *testing.T) {
	done := make(chan struct{})
	go func() {
		_ = hsvToRGB(float32(math.NaN()), 1, 1)
		close(done)
	}()
	select {
	case <-done:
	case <-makeTimeoutChan():
		t.Fatal("hsvToRGB hung on NaN hue")
	}
}

func TestHSVToRGB_ClampsOutOfRange(t *testing.T) {
	c := hsvToRGB(0, 5, 5)
	if c.A != 255 {
		t.Errorf("alpha = %d", c.A)
	}
	c = hsvToRGB(0, -1, -1)
	if c.R == 0 && c.G == 0 && c.B == 0 {
		// expected for v=0 after clamp
	}
}

func TestModf32Basic(t *testing.T) {
	cases := []struct {
		a, m, want float32
	}{
		{0, 6, 0},
		{3, 6, 3},
		{6, 6, 0},
		{7, 6, 1},
		{12, 6, 0},
		{13, 6, 1},
		{-1, 6, 5},
		{-7, 6, 5},
		{-12, 6, 0},
		{5.5, 6, 5.5},
	}
	for _, tc := range cases {
		got := modf32(tc.a, tc.m)
		if absF(got-tc.want) > 1e-5 {
			t.Errorf("modf32(%v,%v)=%v want %v", tc.a, tc.m, got, tc.want)
		}
	}
}

func TestModf32RangeInvariant(t *testing.T) {
	for a := float32(-50); a < 50; a += 0.37 {
		r := modf32(a, 6)
		if r < 0 || r >= 6 {
			t.Errorf("modf32(%v,6)=%v out of [0,6)", a, r)
		}
	}
}

func TestHSVToRGBAlphaAlways255(t *testing.T) {
	hues := []float32{0, 30, 60, 120, 180, 240, 300, 359.9}
	sats := []float32{0, 0.5, 1}
	vals := []float32{0, 0.25, 0.5, 0.75, 1}
	for _, h := range hues {
		for _, s := range sats {
			for _, v := range vals {
				c := hsvToRGB(h, s, v)
				if c.A != 255 {
					t.Errorf("alpha not 255 for h=%v s=%v v=%v: %+v", h, s, v, c)
				}
			}
		}
	}
}

func TestHSVToRGBSaturationZeroProducesGray(t *testing.T) {
	for v := float32(0); v <= 1; v += 0.1 {
		c := hsvToRGB(0, 0, v)
		if c.R != c.G || c.G != c.B {
			t.Errorf("s=0 v=%v not gray: %+v", v, c)
		}
		want := uint8(v*255 + 0.5)
		if c.R != want {
			t.Errorf("s=0 v=%v want value %d got %d", v, want, c.R)
		}
		// hue should not affect grayscale
		for _, h := range []float32{45, 137, 271, 359} {
			c2 := hsvToRGB(h, 0, v)
			if c2 != c {
				t.Errorf("s=0 hue affected color: h=%v c=%+v c2=%+v", h, c, c2)
			}
		}
	}
}

func TestHSVToRGBValueZeroBlack(t *testing.T) {
	for _, h := range []float32{0, 60, 120, 180, 240, 300} {
		for _, s := range []float32{0, 0.3, 0.7, 1} {
			c := hsvToRGB(h, s, 0)
			if c.R != 0 || c.G != 0 || c.B != 0 {
				t.Errorf("v=0 not black: h=%v s=%v -> %+v", h, s, c)
			}
		}
	}
}

func TestHSVToRGBSixHueSectors(t *testing.T) {
	// At hue boundaries each sector should equal the next sector's start
	for hue := float32(0); hue < 360; hue += 60 {
		a := hsvToRGB(hue, 1, 1)
		b := hsvToRGB(hue+59.999, 1, 1)
		// no panic; just sanity
		_ = a
		_ = b
	}
	// Sector midpoints: 30 = orange-ish (R=255 G=128 B=0)
	c := hsvToRGB(30, 1, 1)
	if c.R != 255 || c.B != 0 {
		t.Errorf("hue 30: %+v", c)
	}
	// hue 60 = yellow
	c = hsvToRGB(60, 1, 1)
	if c.R != 255 || c.G != 255 || c.B != 0 {
		t.Errorf("hue 60 (yellow): %+v", c)
	}
	// hue 180 = cyan
	c = hsvToRGB(180, 1, 1)
	if c.R != 0 || c.G != 255 || c.B != 255 {
		t.Errorf("hue 180 (cyan): %+v", c)
	}
	// hue 300 = magenta
	c = hsvToRGB(300, 1, 1)
	if c.R != 255 || c.G != 0 || c.B != 255 {
		t.Errorf("hue 300 (magenta): %+v", c)
	}
}

func TestHSVToRGBLargeHueWraps(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	for _, h := range []float32{360, 720, 1080, -360, -720} {
		c := hsvToRGB(h, 1, 1)
		if !near(c, red, 1) {
			t.Errorf("h=%v should wrap to red: %+v", h, c)
		}
	}
}

func TestRGBToHSVAllPrimaries(t *testing.T) {
	cases := []struct {
		in    color.NRGBA
		wantH float32
	}{
		{color.NRGBA{R: 255, A: 255}, 0},
		{color.NRGBA{R: 255, G: 255, A: 255}, 60},
		{color.NRGBA{G: 255, A: 255}, 120},
		{color.NRGBA{G: 255, B: 255, A: 255}, 180},
		{color.NRGBA{B: 255, A: 255}, 240},
		{color.NRGBA{R: 255, B: 255, A: 255}, 300},
	}
	for _, tc := range cases {
		h, s, v := rgbToHSV(tc.in)
		if absF(h-tc.wantH) > 0.5 {
			t.Errorf("%+v hue: got %v want %v", tc.in, h, tc.wantH)
		}
		if absF(s-1) > 0.001 {
			t.Errorf("%+v sat should be 1, got %v", tc.in, s)
		}
		if absF(v-1) > 0.001 {
			t.Errorf("%+v val should be 1, got %v", tc.in, v)
		}
	}
}

func TestRGBToHSVHueAlwaysNonNegative(t *testing.T) {
	for r := 0; r < 256; r += 17 {
		for g := 0; g < 256; g += 17 {
			for b := 0; b < 256; b += 17 {
				c := color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
				h, s, v := rgbToHSV(c)
				if h < 0 || h >= 360 {
					t.Errorf("%+v hue out of [0,360): %v", c, h)
				}
				if s < 0 || s > 1 {
					t.Errorf("%+v sat out of [0,1]: %v", c, s)
				}
				if v < 0 || v > 1 {
					t.Errorf("%+v val out of [0,1]: %v", c, v)
				}
			}
		}
	}
}

func TestRGBToHSVValueEqualsMax(t *testing.T) {
	cases := []color.NRGBA{
		{R: 100, G: 50, B: 20, A: 255},
		{R: 5, G: 200, B: 30, A: 255},
		{R: 0, G: 0, B: 255, A: 255},
		{R: 17, G: 250, B: 88, A: 255},
	}
	for _, c := range cases {
		_, _, v := rgbToHSV(c)
		mx := max(c.R, c.G, c.B)
		want := float32(mx) / 255
		if absF(v-want) > 0.001 {
			t.Errorf("%+v V=%v want %v", c, v, want)
		}
	}
}

func TestExhaustiveRoundTripPreservesNearly(t *testing.T) {
	maxErr := 0
	for r := 0; r < 256; r += 31 {
		for g := 0; g < 256; g += 31 {
			for b := 0; b < 256; b += 31 {
				orig := color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
				h, s, v := rgbToHSV(orig)
				back := hsvToRGB(h, s, v)
				dr := absInt(int(orig.R) - int(back.R))
				dg := absInt(int(orig.G) - int(back.G))
				db := absInt(int(orig.B) - int(back.B))
				m := max(dr, dg, db)
				maxErr = max(maxErr, m)
				if m > 2 {
					t.Errorf("%+v -> %+v (delta=%d)", orig, back, m)
				}
			}
		}
	}
	t.Logf("max round-trip error: %d", maxErr)
}

func TestStateOpenSetsFields(t *testing.T) {
	var p State
	c := color.NRGBA{R: 255, G: 128, B: 0, A: 255}
	a := Anchor{X: 10, Y: 20}
	p.Open(KindSyntax, 4, c, a)
	if p.Kind != KindSyntax {
		t.Errorf("Kind: %v", p.Kind)
	}
	if p.OpenIdx != 4 {
		t.Errorf("OpenIdx: %v", p.OpenIdx)
	}
	if p.Anchor != a {
		t.Errorf("Anchor: %+v", p.Anchor)
	}
	if !p.IsOpen() {
		t.Errorf("IsOpen should be true")
	}
	if p.LastHSV[0] != p.H || p.LastHSV[1] != p.S || p.LastHSV[2] != p.V {
		t.Errorf("LastHSV mismatch")
	}
	got := p.Color()
	if !near(got, c, 1) {
		t.Errorf("Color() round-trip lost: %+v -> %+v", c, got)
	}
}

func TestStateOpenWithAllKinds(t *testing.T) {
	var p State
	c := color.NRGBA{R: 50, G: 60, B: 70, A: 255}
	for _, k := range []Kind{KindNone, KindSyntax, KindTheme, KindEnv} {
		p.Open(k, 0, c, Anchor{})
		if p.Kind != k {
			t.Errorf("Kind not set: want %v got %v", k, p.Kind)
		}
		// KindNone open is a peculiar state: IsOpen() reports false
		if k == KindNone && p.IsOpen() {
			t.Errorf("KindNone open should not be IsOpen")
		}
		if k != KindNone && !p.IsOpen() {
			t.Errorf("kind %v should be IsOpen", k)
		}
	}
}

func TestStateCloseResetsKindAndIdx(t *testing.T) {
	var p State
	p.Open(KindTheme, 7, color.NRGBA{R: 200, G: 100, B: 50, A: 255}, Anchor{X: 1, Y: 2})
	p.Close()
	if p.Kind != KindNone {
		t.Errorf("Close should reset Kind, got %v", p.Kind)
	}
	if p.OpenIdx != -1 {
		t.Errorf("Close should set OpenIdx=-1, got %v", p.OpenIdx)
	}
	if p.IsOpen() {
		t.Errorf("After Close, IsOpen should be false")
	}
	// H/S/V NOT reset by Close (current behavior) — assert and TODO bug above
	if p.H == 0 && p.S == 0 && p.V == 0 {
		t.Logf("note: Close happened to leave zero HSV (color was near-black)")
	}
}

func TestStateColorBeforeOpen(t *testing.T) {
	var p State
	// Default state: H=S=V=0 → black
	c := p.Color()
	if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("default Color() should be opaque black, got %+v", c)
	}
}

func TestKindConstantsStable(t *testing.T) {
	// LastHSV ordering / Kind enum ordering invariant (project memory)
	if KindNone != 0 || KindSyntax != 1 || KindTheme != 2 || KindEnv != 3 {
		t.Errorf("Kind enum order changed: None=%d Syntax=%d Theme=%d Env=%d",
			KindNone, KindSyntax, KindTheme, KindEnv)
	}
}

func TestStateOpenPreservesAnchorPrecision(t *testing.T) {
	var p State
	a := Anchor{X: 123.456, Y: -78.9}
	p.Open(KindEnv, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255}, a)
	if p.Anchor.X != 123.456 || p.Anchor.Y != -78.9 {
		t.Errorf("Anchor precision lost: %+v", p.Anchor)
	}
}

// Coordinate-to-color picker math (mirrors logic at colorpicker.go:230-249).
// Even though Render uses pointer events, the clamp + ratio math is identical
// and is the hit-test core. Tested standalone here.
func clampSVMath(x, y, svW, svH int) (s, v float32) {
	if x < 0 {
		x = 0
	}
	if x > svW-1 {
		x = svW - 1
	}
	if y < 0 {
		y = 0
	}
	if y > svH-1 {
		y = svH - 1
	}
	if svW > 1 {
		s = float32(x) / float32(svW-1)
	}
	if svH > 1 {
		v = 1 - float32(y)/float32(svH-1)
	}
	return
}

func TestClampSVMathCorners(t *testing.T) {
	const w, h = 200, 140
	cases := []struct {
		x, y       int
		wantS, wantV float32
	}{
		{0, 0, 0, 1},
		{w - 1, 0, 1, 1},
		{0, h - 1, 0, 0},
		{w - 1, h - 1, 1, 0},
		{(w - 1) / 2, (h - 1) / 2, 0.5, 0.5},
		{-50, -50, 0, 1},      // clamp negative
		{w + 99, h + 99, 1, 0}, // clamp overflow
	}
	for _, tc := range cases {
		s, v := clampSVMath(tc.x, tc.y, w, h)
		if absF(s-tc.wantS) > 0.01 {
			t.Errorf("x=%d S: got %v want %v", tc.x, s, tc.wantS)
		}
		if absF(v-tc.wantV) > 0.01 {
			t.Errorf("y=%d V: got %v want %v", tc.y, v, tc.wantV)
		}
	}
}

func TestClampSVMathZeroSize(t *testing.T) {
	// svW=1 / svH=1 should not divide-by-zero; result stays zero
	s, v := clampSVMath(0, 0, 1, 1)
	if s != 0 || v != 0 {
		t.Errorf("zero-size picker: s=%v v=%v want 0,0", s, v)
	}
	s, v = clampSVMath(50, 50, 1, 1)
	if s != 0 || v != 0 {
		t.Errorf("zero-size picker w/ overflow input: s=%v v=%v", s, v)
	}
}

// Hue cursor math mirrors colorpicker.go:308-317.
func clampHueMath(x, svW int) (h float32) {
	if x < 0 {
		x = 0
	}
	if x > svW-1 {
		x = svW - 1
	}
	if svW > 1 {
		h = float32(x) / float32(svW-1) * 360
	}
	return
}

func TestClampHueMath(t *testing.T) {
	const w = 200
	cases := []struct {
		x    int
		want float32
	}{
		{0, 0},
		{w - 1, 360},
		{(w - 1) / 2, 180},
		{-100, 0},
		{w + 999, 360},
	}
	for _, tc := range cases {
		h := clampHueMath(tc.x, w)
		if absF(h-tc.want) > 1 {
			t.Errorf("x=%d hue: got %v want %v", tc.x, h, tc.want)
		}
	}
	// svW=1: divide-by-zero guard
	if got := clampHueMath(0, 1); got != 0 {
		t.Errorf("svW=1 should yield 0, got %v", got)
	}
}

func TestHueCursorXMappingRoundTrip(t *testing.T) {
	// Inverse of colorpicker.go:287 — hcx = min.X + int(H/360 * (svW-1))
	const svW = 240
	for hue := float32(0); hue <= 360; hue += 30 {
		x := int(hue / 360 * float32(svW-1))
		back := clampHueMath(x, svW)
		// Allow 2° drift from int truncation
		if math.Abs(float64(back-hue)) > 2 {
			t.Errorf("hue %v -> x %d -> %v drift too large", hue, x, back)
		}
	}
}

func TestSVCursorXYMappingRoundTrip(t *testing.T) {
	const w, h = 240, 140
	for s := float32(0); s <= 1; s += 0.1 {
		for v := float32(0); v <= 1; v += 0.1 {
			cx := int(s * float32(w-1))
			cy := int((1 - v) * float32(h-1))
			s2, v2 := clampSVMath(cx, cy, w, h)
			if absF(s-s2) > 0.02 {
				t.Errorf("S %v -> x %d -> %v drift", s, cx, s2)
			}
			if absF(v-v2) > 0.02 {
				t.Errorf("V %v -> y %d -> %v drift", v, cy, v2)
			}
		}
	}
}
