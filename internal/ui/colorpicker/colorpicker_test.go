package colorpicker

import (
	"image/color"
	"math"
	"testing"
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
		name string
		h, s, v float32
		want color.NRGBA
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
		name    string
		in      color.NRGBA
		wantH   float32
		wantS   float32
		wantV   float32
		hueTol  float32
		satTol  float32
		valTol  float32
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
