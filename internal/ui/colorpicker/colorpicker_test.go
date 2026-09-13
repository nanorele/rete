package colorpicker

import (
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"image/color"
	"math"
	"testing"
	"time"
)

func TestHSVtoRGBRoundTrip(t *testing.T) {
	cases := []color.NRGBA{
		{R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 255, G: 0, B: 0, A: 255},
		{R: 0, G: 255, B: 0, A: 255},
		{R: 0, G: 0, B: 255, A: 255},
		{R: 255, G: 255, B: 0, A: 255},
		{R: 0, G: 255, B: 255, A: 255},
		{R: 255, G: 0, B: 255, A: 255},
		{R: 14, G: 99, B: 156, A: 255},
		{R: 128, G: 64, B: 32, A: 255},
		{R: 200, G: 200, B: 200, A: 255},
		{R: 1, G: 1, B: 1, A: 255},
		{R: 254, G: 253, B: 252, A: 255},
		{R: 127, G: 127, B: 127, A: 255},
		{R: 128, G: 128, B: 128, A: 255},
	}
	for _, c := range cases {
		h, s, v := rgbToHSV(c)
		back := hsvToRGB(h, s, v)
		if !near(c, back, 1) {
			t.Errorf("round-trip lost color: %+v -> H=%.2f S=%.3f V=%.3f -> %+v", c, h, s, v, back)
		}
	}
}

func TestHSVtoRGBKnownValues(t *testing.T) {
	cases := []struct {
		name    string
		h, s, v float32
		want    color.NRGBA
	}{
		{"pure red", 0, 1, 1, color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
		{"pure green", 120, 1, 1, color.NRGBA{R: 0, G: 255, B: 0, A: 255}},
		{"pure blue", 240, 1, 1, color.NRGBA{R: 0, G: 0, B: 255, A: 255}},
		{"black", 0, 0, 0, color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{"white", 0, 0, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		{"50% gray", 0, 0, 0.5, color.NRGBA{R: 128, G: 128, B: 128, A: 255}},
		{"hue wraps 360 -> 0", 360, 1, 1, color.NRGBA{R: 255, G: 0, B: 0, A: 255}},
		{"negative hue", -60, 1, 1, color.NRGBA{R: 255, G: 0, B: 255, A: 255}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := hsvToRGB(tc.h, tc.s, tc.v)
			if !near(got, tc.want, 1) {
				t.Errorf("hsvToRGB(%.0f,%.2f,%.2f) = %+v, want %+v", tc.h, tc.s, tc.v, got, tc.want)
			}
		})
	}
}

func TestRGBtoHSVKnownValues(t *testing.T) {
	cases := []struct {
		name   string
		in     color.NRGBA
		wantH  float32
		wantS  float32
		wantV  float32
		hueTol float32
		satTol float32
		valTol float32
	}{
		{"pure red", color.NRGBA{R: 255, A: 255}, 0, 1, 1, 0.5, 0.01, 0.01},
		{"pure green", color.NRGBA{G: 255, A: 255}, 120, 1, 1, 0.5, 0.01, 0.01},
		{"pure blue", color.NRGBA{B: 255, A: 255}, 240, 1, 1, 0.5, 0.01, 0.01},
		{"gray returns S=0", color.NRGBA{R: 128, G: 128, B: 128, A: 255}, 0, 0, 0.5, 1, 0.01, 0.01},
		{"black", color.NRGBA{A: 255}, 0, 0, 0, 1, 0.01, 0.01},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, s, v := rgbToHSV(tc.in)
			if absF(h-tc.wantH) > tc.hueTol {
				t.Errorf("H: got %.2f, want %.2f", h, tc.wantH)
			}
			if absF(s-tc.wantS) > tc.satTol {
				t.Errorf("S: got %.3f, want %.3f", s, tc.wantS)
			}
			if absF(v-tc.wantV) > tc.valTol {
				t.Errorf("V: got %.3f, want %.3f", v, tc.wantV)
			}
		})
	}
}

func TestHSVtoRGBRangeClamps(t *testing.T) {
	c := hsvToRGB(720, 1, 1)
	if !near(c, color.NRGBA{R: 255, A: 255}, 1) {
		t.Errorf("hue overflow 720°: got %+v, want red", c)
	}
	c = hsvToRGB(-180, 1, 1)
	if !near(c, color.NRGBA{G: 255, B: 255, A: 255}, 1) {
		t.Errorf("negative hue: got %+v", c)
	}
}

func TestHSVStableUnderRepeatedRoundTrip(t *testing.T) {
	originals := []color.NRGBA{
		{R: 50, G: 100, B: 200, A: 255},
		{R: 250, G: 200, B: 100, A: 255},
		{R: 200, G: 200, B: 200, A: 255},
		{R: 17, G: 89, B: 137, A: 255},
	}
	for _, orig := range originals {
		c := orig
		for i := 0; i < 5; i++ {
			h, s, v := rgbToHSV(c)
			c = hsvToRGB(h, s, v)
		}
		if !near(orig, c, 2) {
			t.Errorf("color drifted after 5 round-trips: %+v -> %+v", orig, c)
		}
	}
}

func near(a, b color.NRGBA, tol int) bool {
	return absInt(int(a.R)-int(b.R)) <= tol &&
		absInt(int(a.G)-int(b.G)) <= tol &&
		absInt(int(a.B)-int(b.B)) <= tol &&
		absInt(int(a.A)-int(b.A)) <= tol
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func absF(x float32) float32 {
	return float32(math.Abs(float64(x)))
}

func makeTimeoutChan() <-chan time.Time {
	return time.After(2 * time.Second)
}

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
	if c.A != 255 {
		t.Errorf("alpha = %d", c.A)
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

	for hue := float32(0); hue < 360; hue += 60 {
		a := hsvToRGB(hue, 1, 1)
		b := hsvToRGB(hue+59.999, 1, 1)

		_ = a
		_ = b
	}

	c := hsvToRGB(30, 1, 1)
	if c.R != 255 || c.B != 0 {
		t.Errorf("hue 30: %+v", c)
	}

	c = hsvToRGB(60, 1, 1)
	if c.R != 255 || c.G != 255 || c.B != 0 {
		t.Errorf("hue 60 (yellow): %+v", c)
	}

	c = hsvToRGB(180, 1, 1)
	if c.R != 0 || c.G != 255 || c.B != 255 {
		t.Errorf("hue 180 (cyan): %+v", c)
	}

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

	if p.H == 0 && p.S == 0 && p.V == 0 {
		t.Logf("note: Close happened to leave zero HSV (color was near-black)")
	}
}

func TestStateColorBeforeOpen(t *testing.T) {
	var p State

	c := p.Color()
	if c.R != 0 || c.G != 0 || c.B != 0 || c.A != 255 {
		t.Errorf("default Color() should be opaque black, got %+v", c)
	}
}

func TestKindConstantsStable(t *testing.T) {

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
		x, y         int
		wantS, wantV float32
	}{
		{0, 0, 0, 1},
		{w - 1, 0, 1, 1},
		{0, h - 1, 0, 0},
		{w - 1, h - 1, 1, 0},
		{(w - 1) / 2, (h - 1) / 2, 0.5, 0.5},
		{-50, -50, 0, 1},
		{w + 99, h + 99, 1, 0},
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

	s, v := clampSVMath(0, 0, 1, 1)
	if s != 0 || v != 0 {
		t.Errorf("zero-size picker: s=%v v=%v want 0,0", s, v)
	}
	s, v = clampSVMath(50, 50, 1, 1)
	if s != 0 || v != 0 {
		t.Errorf("zero-size picker w/ overflow input: s=%v v=%v", s, v)
	}
}

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

	if got := clampHueMath(0, 1); got != 0 {
		t.Errorf("svW=1 should yield 0, got %v", got)
	}
}

func TestHueCursorXMappingRoundTrip(t *testing.T) {

	const svW = 240
	for hue := float32(0); hue <= 360; hue += 30 {
		x := int(hue / 360 * float32(svW-1))
		back := clampHueMath(x, svW)

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

const (
	cpWidth    = 240
	cpInnerPad = 10
	cpSVW      = cpWidth - 2*cpInnerPad
	cpSVH      = 140
	cpSVMinX   = cpInnerPad
	cpSVMinY   = cpInnerPad
	cpHueMinX  = cpInnerPad
	cpHueMinY  = cpInnerPad + cpSVH + 6
	cpHueH     = 14
	cpRowY     = cpHueMinY + cpHueH + 6
	cpCloseW   = 64
	cpCloseMin = cpInnerPad + cpSVW - cpCloseW
	cpPreviewH = 22
)

type cpRig struct {
	th     *material.Theme
	p      *State
	r      input.Router
	metric unit.Metric
	sz     image.Point
	now    time.Time
}

func newCPRig(t *testing.T) *cpRig {
	t.Helper()
	rig := &cpRig{
		th:     material.NewTheme(),
		p:      &State{},
		metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		sz:     image.Pt(400, 400),
		now:    time.Unix(1700000000, 0),
	}
	rig.p.Open(KindSyntax, 0, color.NRGBA{R: 128, G: 64, B: 32, A: 255}, Anchor{})
	return rig
}

func (rig *cpRig) frame() layout.Dimensions {
	rig.now = rig.now.Add(16 * time.Millisecond)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      rig.metric,
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
	dims := Render(gtx, rig.th, rig.p)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *cpRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *cpRig) press(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *cpRig) drag(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *cpRig) release(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frames(2)
}

func (rig *cpRig) hover(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
}

func TestRender_GeometryMatchesTestConstants(t *testing.T) {
	rig := newCPRig(t)
	dims := rig.frames(2)
	if dims.Size.X != cpWidth {
		t.Fatalf("card width = %d, want %d", dims.Size.X, cpWidth)
	}
	wantH := cpInnerPad + cpSVH + 6 + cpHueH + 6 + cpPreviewH + cpInnerPad
	if dims.Size.Y != wantH {
		t.Fatalf("card height = %d, want %d", dims.Size.Y, wantH)
	}
}

func TestRender_SVPressSetsSaturationAndValue(t *testing.T) {
	cases := []struct {
		name         string
		x, y         float32
		wantS, wantV float32
	}{
		{"top-left", cpSVMinX, cpSVMinY, 0, 1},
		{"bottom-right", cpSVMinX + cpSVW - 1, cpSVMinY + cpSVH - 1, 1, 0},
		{"middle", cpSVMinX + 110, cpSVMinY + 70, 110.0 / (cpSVW - 1), 1 - 70.0/(cpSVH-1)},
		{"top-right", cpSVMinX + cpSVW - 1, cpSVMinY, 1, 1},
		{"bottom-left", cpSVMinX, cpSVMinY + cpSVH - 1, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newCPRig(t)
			rig.frames(2)
			rig.p.S, rig.p.V = -1, -1
			rig.press(tc.x, tc.y)
			rig.release(tc.x, tc.y)
			if absF(rig.p.S-tc.wantS) > 0.01 {
				t.Errorf("S = %v, want %v", rig.p.S, tc.wantS)
			}
			if absF(rig.p.V-tc.wantV) > 0.01 {
				t.Errorf("V = %v, want %v", rig.p.V, tc.wantV)
			}
		})
	}
}

func TestRender_SVPressLeavesHueUnchanged(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	before := rig.p.H
	rig.press(cpSVMinX+40, cpSVMinY+90)
	rig.release(cpSVMinX+40, cpSVMinY+90)
	if rig.p.H != before {
		t.Errorf("SV press changed H: %v -> %v", before, rig.p.H)
	}
}

func TestRender_SVDragClampsPastEdges(t *testing.T) {
	cases := []struct {
		name         string
		x, y         float32
		wantS, wantV float32
	}{
		{"past-top-left", -5000, -5000, 0, 1},
		{"past-bottom-right", 5000, 5000, 1, 0},
		{"past-left-only", -5000, cpSVMinY + 70, 0, 1 - 70.0/(cpSVH-1)},
		{"past-bottom-only", cpSVMinX + 110, 5000, 110.0 / (cpSVW - 1), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newCPRig(t)
			rig.frames(2)
			rig.press(cpSVMinX+110, cpSVMinY+70)
			rig.drag(tc.x, tc.y)
			rig.release(tc.x, tc.y)
			if absF(rig.p.S-tc.wantS) > 0.01 {
				t.Errorf("S = %v, want %v", rig.p.S, tc.wantS)
			}
			if absF(rig.p.V-tc.wantV) > 0.01 {
				t.Errorf("V = %v, want %v", rig.p.V, tc.wantV)
			}
			if rig.p.S < 0 || rig.p.S > 1 || rig.p.V < 0 || rig.p.V > 1 {
				t.Errorf("clamped drag escaped [0,1]: S=%v V=%v", rig.p.S, rig.p.V)
			}
		})
	}
}

func TestRender_SVDragTracksPointer(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	rig.press(cpSVMinX+10, cpSVMinY+10)
	first := [2]float32{rig.p.S, rig.p.V}
	rig.drag(cpSVMinX+180, cpSVMinY+120)
	if rig.p.S <= first[0] {
		t.Errorf("dragging right must raise S: %v -> %v", first[0], rig.p.S)
	}
	if rig.p.V >= first[1] {
		t.Errorf("dragging down must lower V: %v -> %v", first[1], rig.p.V)
	}
	rig.release(cpSVMinX+180, cpSVMinY+120)
}

func TestRender_HuePressSetsHue(t *testing.T) {
	cases := []struct {
		name  string
		x     float32
		wantH float32
	}{
		{"left-edge", cpHueMinX, 0},
		{"right-edge", cpHueMinX + cpSVW - 1, 360},
		{"middle", cpHueMinX + 110, 110.0 / (cpSVW - 1) * 360},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newCPRig(t)
			rig.frames(2)
			rig.p.H = -1
			y := float32(cpHueMinY + cpHueH/2)
			rig.press(tc.x, y)
			rig.release(tc.x, y)
			if absF(rig.p.H-tc.wantH) > 1 {
				t.Errorf("H = %v, want %v", rig.p.H, tc.wantH)
			}
		})
	}
}

func TestRender_HuePressLeavesSVUnchanged(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	beforeS, beforeV := rig.p.S, rig.p.V
	y := float32(cpHueMinY + cpHueH/2)
	rig.press(cpHueMinX+50, y)
	rig.release(cpHueMinX+50, y)
	if rig.p.S != beforeS || rig.p.V != beforeV {
		t.Errorf("hue press changed SV: %v/%v -> %v/%v", beforeS, beforeV, rig.p.S, rig.p.V)
	}
}

func TestRender_HueDragClampsPastEdges(t *testing.T) {
	cases := []struct {
		name  string
		x     float32
		wantH float32
	}{
		{"past-left", -5000, 0},
		{"past-right", 5000, 360},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newCPRig(t)
			rig.frames(2)
			y := float32(cpHueMinY + cpHueH/2)
			rig.press(cpHueMinX+110, y)
			rig.drag(tc.x, y)
			rig.release(tc.x, y)
			if absF(rig.p.H-tc.wantH) > 1 {
				t.Errorf("H = %v, want %v", rig.p.H, tc.wantH)
			}
			if rig.p.H < 0 || rig.p.H > 360 {
				t.Errorf("clamped hue drag escaped [0,360]: %v", rig.p.H)
			}
		})
	}
}

func TestRender_HueDragChangesPreviewColor(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	rig.p.S, rig.p.V = 1, 1
	y := float32(cpHueMinY + cpHueH/2)
	rig.press(cpHueMinX+2, y)
	rig.frame()
	red := rig.p.Color()
	rig.drag(cpHueMinX+cpSVW/3, y)
	green := rig.p.Color()
	rig.release(cpHueMinX+cpSVW/3, y)
	if red == green {
		t.Fatalf("hue drag did not change the color: %+v", red)
	}
	if red.R < 200 || red.G > 40 {
		t.Errorf("left end of the hue strip should be red, got %+v", red)
	}
	if green.G < 200 {
		t.Errorf("one third across the hue strip should be green, got %+v", green)
	}
}

func TestRender_CloseButtonHoverAndClick(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	if rig.p.CloseBtn.Hovered() {
		t.Fatal("precondition: close button must not start hovered")
	}

	cx := float32(cpCloseMin + cpCloseW/2)
	cy := float32(cpRowY + cpPreviewH/2)
	rig.hover(cx, cy)
	if !rig.p.CloseBtn.Hovered() {
		t.Fatalf("close button not hovered after moving to (%v,%v)", cx, cy)
	}
	rig.frame()

	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(cx, cy), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(cx, cy), Source: pointer.Mouse})
	if !rig.p.CloseBtn.Clicked(layout.Context{Source: rig.r.Source(), Now: rig.now}) {
		t.Error("close button reported no click after press+release on it")
	}

	rig.hover(1, 1)
	rig.frame()
	if rig.p.CloseBtn.Hovered() {
		t.Error("close button still hovered after the pointer left it")
	}
}

func TestRender_CloseButtonProgrammaticClick(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	rig.p.CloseBtn.Click()
	if !rig.p.CloseBtn.Clicked(layout.Context{Source: rig.r.Source(), Now: rig.now}) {
		t.Error("programmatic Click() produced no click event")
	}
}

func TestRender_TinyMetricForcesMinimumBorder(t *testing.T) {
	rig := newCPRig(t)
	rig.metric = unit.Metric{PxPerDp: 0.3, PxPerSp: 0.3}
	rig.sz = image.Pt(200, 200)
	dims := rig.frames(2)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("tiny metric produced no card: %+v", dims.Size)
	}
	if dims.Size.X >= cpWidth {
		t.Errorf("tiny metric should shrink the card, got width %d", dims.Size.X)
	}
}

func TestRender_DegenerateMetricNoPanicOnPress(t *testing.T) {
	rig := newCPRig(t)
	rig.metric = unit.Metric{PxPerDp: 0.004, PxPerSp: 0.004}
	rig.sz = image.Pt(8, 8)
	rig.frames(2)
	before := [3]float32{rig.p.H, rig.p.S, rig.p.V}
	rig.press(0, 0)
	rig.drag(4, 4)
	rig.release(4, 4)
	if rig.p.H != before[0] || rig.p.S != before[1] || rig.p.V != before[2] {
		t.Logf("degenerate metric altered HSV: %v -> %v/%v/%v", before, rig.p.H, rig.p.S, rig.p.V)
	}
	if rig.p.S < 0 || rig.p.S > 1 || rig.p.V < 0 || rig.p.V > 1 {
		t.Errorf("degenerate metric escaped [0,1]: S=%v V=%v", rig.p.S, rig.p.V)
	}
}

func TestRender_DragThenSecondPressRetargets(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	rig.press(cpSVMinX+20, cpSVMinY+20)
	rig.release(cpSVMinX+20, cpSVMinY+20)
	svS, svV := rig.p.S, rig.p.V

	y := float32(cpHueMinY + cpHueH/2)
	rig.press(cpHueMinX+150, y)
	rig.release(cpHueMinX+150, y)
	if rig.p.S != svS || rig.p.V != svV {
		t.Errorf("hue press clobbered SV: %v/%v -> %v/%v", svS, svV, rig.p.S, rig.p.V)
	}
	if absF(rig.p.H-150.0/(cpSVW-1)*360) > 1 {
		t.Errorf("second press did not land on the hue strip: H=%v", rig.p.H)
	}
}

func TestRender_PressOutsideAnyControlIsIgnored(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	before := [3]float32{rig.p.H, rig.p.S, rig.p.V}
	rig.press(390, 390)
	rig.release(390, 390)
	rig.press(2, 2)
	rig.release(2, 2)
	if rig.p.H != before[0] || rig.p.S != before[1] || rig.p.V != before[2] {
		t.Errorf("press outside the controls changed HSV: %v -> %v/%v/%v", before, rig.p.H, rig.p.S, rig.p.V)
	}
}

func TestRender_SecondaryButtonPressIgnored(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	before := [3]float32{rig.p.H, rig.p.S, rig.p.V}
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(cpSVMinX+100, cpSVMinY+100), Buttons: pointer.ButtonSecondary, Source: pointer.Mouse})
	rig.frames(2)
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(cpSVMinX+100, cpSVMinY+100), Source: pointer.Mouse})
	rig.frames(2)
	if rig.p.H != before[0] || rig.p.S != before[1] || rig.p.V != before[2] {
		t.Errorf("secondary-button press changed HSV: %v -> %v/%v/%v", before, rig.p.H, rig.p.S, rig.p.V)
	}
}

func TestRender_FullInteractionRoundTripsThroughColor(t *testing.T) {
	rig := newCPRig(t)
	rig.frames(2)
	y := float32(cpHueMinY + cpHueH/2)
	rig.press(float32(cpHueMinX)+float32(int(120.0/360*(cpSVW-1))), y)
	rig.release(float32(cpHueMinX)+float32(int(120.0/360*(cpSVW-1))), y)
	rig.press(cpSVMinX+cpSVW-1, cpSVMinY)
	rig.release(cpSVMinX+cpSVW-1, cpSVMinY)

	got := rig.p.Color()
	h, s, v := rgbToHSV(got)
	if absF(s-1) > 0.01 || absF(v-1) > 0.01 {
		t.Errorf("top-right corner should be fully saturated and bright: S=%v V=%v", s, v)
	}
	if absF(h-120) > 2 {
		t.Errorf("hue should stay near 120 after the SV press, got %v", h)
	}
}

func TestHSVToRGB_HueSectorIndexClampsToFive(t *testing.T) {
	c := hsvToRGB(float32(-1e-30), 1, 1)
	want := color.NRGBA{R: 255, G: 0, B: 0, A: 255}
	if !near(c, want, 1) {
		t.Errorf("hue rounding to exactly 360 must stay red, got %+v", c)
	}
	for _, s := range []float32{0.25, 0.5, 1} {
		got := hsvToRGB(float32(-1e-30), s, 1)
		ref := hsvToRGB(0, s, 1)
		if !near(got, ref, 1) {
			t.Errorf("s=%v: hue 360 %+v != hue 0 %+v", s, got, ref)
		}
	}
}

func TestRGBToHSVDenseRoundTrip(t *testing.T) {
	for r := 0; r < 256; r += 7 {
		for g := 0; g < 256; g += 11 {
			for b := 0; b < 256; b += 13 {
				orig := color.NRGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
				h, s, v := rgbToHSV(orig)
				if h < 0 || h >= 360 {
					t.Fatalf("%+v produced hue %v outside [0,360)", orig, h)
				}
				back := hsvToRGB(h, s, v)
				if !near(orig, back, 2) {
					t.Fatalf("round-trip lost %+v -> %+v", orig, back)
				}
			}
		}
	}
}

func makeRenderGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

func TestRender_FreshStateSyntax(t *testing.T) {
	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	var p State
	p.Open(KindSyntax, 0, color.NRGBA{R: 255, G: 128, B: 64, A: 255}, Anchor{X: 100, Y: 200})

	dims := Render(gtx, th, &p)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("Render returned non-positive size: %+v", dims.Size)
	}
}

func TestRender_FreshStateTheme(t *testing.T) {
	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	var p State
	p.Open(KindTheme, 3, color.NRGBA{R: 50, G: 60, B: 200, A: 255}, Anchor{X: 0, Y: 0})

	dims := Render(gtx, th, &p)
	if dims.Size.X != 240 {

		t.Errorf("expected width 240, got %d", dims.Size.X)
	}
}

func TestRender_KindEnv(t *testing.T) {
	th := material.NewTheme()
	gtx := makeRenderGtx(500, 500)
	var p State
	p.Open(KindEnv, 1, color.NRGBA{R: 10, G: 200, B: 30, A: 255}, Anchor{})
	_ = Render(gtx, th, &p)
}

func TestRender_MultipleFramesNoPanic(t *testing.T) {
	th := material.NewTheme()
	var p State
	p.Open(KindSyntax, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255}, Anchor{X: 50, Y: 80})

	for range 5 {
		gtx := makeRenderGtx(400, 400)
		_ = Render(gtx, th, &p)
	}
}

func TestRender_VariousAnchors(t *testing.T) {
	th := material.NewTheme()
	anchors := []Anchor{
		{X: 0, Y: 0},
		{X: 100, Y: 200},
		{X: -50, Y: -50},
		{X: 9999, Y: 9999},
		{X: float32(math.NaN()), Y: 0},
		{X: float32(math.Inf(1)), Y: 0},
		{X: 0, Y: float32(math.Inf(-1))},
	}
	for i, a := range anchors {
		var p State
		p.Open(KindSyntax, i, color.NRGBA{R: 100, G: 100, B: 100, A: 255}, a)
		gtx := makeRenderGtx(400, 400)

		_ = Render(gtx, th, &p)
	}
}

func TestRender_SmallConstraints(t *testing.T) {

	th := material.NewTheme()
	var p State
	p.Open(KindSyntax, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 255}, Anchor{})

	for _, sz := range []image.Point{
		{X: 1, Y: 1},
		{X: 10, Y: 10},
		{X: 50, Y: 50},
		{X: 100, Y: 100},
	} {
		gtx := makeRenderGtx(sz.X, sz.Y)
		_ = Render(gtx, th, &p)
	}
}

func TestRender_HighDPI(t *testing.T) {

	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 2, PxPerSp: 2},
		Constraints: layout.Exact(image.Pt(1000, 1000)),
	}
	var p State
	p.Open(KindTheme, 0, color.NRGBA{R: 80, G: 80, B: 80, A: 255}, Anchor{})
	dims := Render(gtx, th, &p)
	if dims.Size.X != 480 {
		t.Errorf("at PxPerDp=2, expected width 480, got %d", dims.Size.X)
	}
}

func TestRender_HSVExtremes(t *testing.T) {

	th := material.NewTheme()
	cases := []struct {
		name    string
		h, s, v float32
	}{
		{"top-left (S=0,V=1)", 0, 0, 1},
		{"top-right (S=1,V=1)", 180, 1, 1},
		{"bottom-left (S=0,V=0)", 0, 0, 0},
		{"bottom-right (S=1,V=0)", 240, 1, 0},
		{"mid (S=0.6,V=0.6) light-ring branch", 90, 0.4, 0.7},
		{"H=360 wraps", 360, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := makeRenderGtx(400, 400)
			p := State{Kind: KindSyntax, H: tc.h, S: tc.s, V: tc.v}
			_ = Render(gtx, th, &p)
		})
	}
}

func TestRender_LightRingBranch(t *testing.T) {

	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	p := State{Kind: KindSyntax, H: 200, S: 0.3, V: 0.9}
	_ = Render(gtx, th, &p)
}

func TestRender_NaNHSVDoesNotPanic(t *testing.T) {

	th := material.NewTheme()
	done := make(chan struct{})
	go func() {
		defer close(done)
		gtx := makeRenderGtx(400, 400)
		p := State{
			Kind: KindSyntax,
			H:    float32(math.NaN()),
			S:    float32(math.NaN()),
			V:    float32(math.NaN()),
		}
		_ = Render(gtx, th, &p)
	}()
	select {
	case <-done:
	case <-makeTimeoutChan():
		t.Fatal("Render hung on NaN HSV")
	}
}

func TestRender_InfHSV(t *testing.T) {

	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	p := State{
		Kind: KindSyntax,
		H:    float32(math.Inf(1)),
		S:    1,
		V:    1,
	}
	_ = Render(gtx, th, &p)

	gtx = makeRenderGtx(400, 400)
	p.H = float32(math.Inf(-1))
	_ = Render(gtx, th, &p)
}

func TestRender_TwoFramesAccumulateOps(t *testing.T) {

	th := material.NewTheme()
	var p State
	p.Open(KindSyntax, 0, color.NRGBA{R: 100, G: 50, B: 200, A: 255}, Anchor{})

	gtx1 := makeRenderGtx(400, 400)
	_ = Render(gtx1, th, &p)

	gtx2 := makeRenderGtx(400, 400)
	_ = Render(gtx2, th, &p)

	gtx3 := makeRenderGtx(400, 400)
	_ = Render(gtx3, th, &p)
	_ = Render(gtx3, th, &p)
}

func TestRender_ClosedStateStillRenders(t *testing.T) {

	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	var p State
	dims := Render(gtx, th, &p)
	if dims.Size.X <= 0 {
		t.Errorf("Render on zero State returned bad size: %+v", dims.Size)
	}
}
