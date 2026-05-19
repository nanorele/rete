package colorpicker

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

// makeRenderGtx builds a minimal layout.Context sufficient for calling Render.
// gtx.Source is left zero-valued — pointer events from Drag.Update will return
// (zero, false) on the first iteration, so the gesture branches inside Render
// (colorpicker.go:240-268, 318-336) are exercised only for the loop-entry
// condition. That's acceptable per the test-plan constraints.
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
		// width is hardcoded gtx.Dp(unit.Dp(240)) — with PxPerDp=1 this should be 240
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
		{X: -50, Y: -50},                          // negative anchor
		{X: 9999, Y: 9999},                        // far-offscreen anchor
		{X: float32(math.NaN()), Y: 0},            // NaN anchor
		{X: float32(math.Inf(1)), Y: 0},           // +Inf anchor
		{X: 0, Y: float32(math.Inf(-1))},          // -Inf anchor
	}
	for i, a := range anchors {
		var p State
		p.Open(KindSyntax, i, color.NRGBA{R: 100, G: 100, B: 100, A: 255}, a)
		gtx := makeRenderGtx(400, 400)
		// Anchor isn't currently consumed by Render itself (it's positioned by
		// the caller), but driving Render across all Anchor variants still
		// guards against any future regression that begins reading p.Anchor.
		_ = Render(gtx, th, &p)
	}
}

func TestRender_SmallConstraints(t *testing.T) {
	// Render hardcodes a 240×~205 dp card; with tiny gtx constraints the
	// inner card border math is independent of constraints, so this should
	// not panic — but it's a good probe for out-of-bounds drawing.
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
	// PxPerDp=2 → border = max(int(2*1), 1) = 2; should not break math.
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
	// Cursor X/Y formulas (colorpicker.go:225-226, 304) cast to int;
	// extreme HSV values (H=360, S=1, V=1 / S=0, V=0) shouldn't panic.
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
	// Specifically hit the dark-ring branch (colorpicker.go:229: V>0.6 && S<0.5).
	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	p := State{Kind: KindSyntax, H: 200, S: 0.3, V: 0.9}
	_ = Render(gtx, th, &p)
}

func TestRender_NaNHSVDoesNotPanic(t *testing.T) {
	// Probe: feed NaN into LastHSV / H/S/V to see if Render guards them.
	// hsvToRGB clamps S and V via clamp01 (which handles NaN via x != x),
	// and modf32 guards NaN hue. Cursor-position float→int conversion of
	// a NaN cursor offset is implementation-defined in Go (typically 0 on
	// amd64) but should not panic.
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
	// +/-Inf H should be handled by modf32 inside hsvToRGB.
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
	// Reuse the same State across two distinct gtx values to ensure
	// gesture.Drag.Add doesn't error on a second Add after the first frame.
	th := material.NewTheme()
	var p State
	p.Open(KindSyntax, 0, color.NRGBA{R: 100, G: 50, B: 200, A: 255}, Anchor{})

	gtx1 := makeRenderGtx(400, 400)
	_ = Render(gtx1, th, &p)

	gtx2 := makeRenderGtx(400, 400)
	_ = Render(gtx2, th, &p)

	// Same Ops, multiple Render calls (sharing op buffer)
	gtx3 := makeRenderGtx(400, 400)
	_ = Render(gtx3, th, &p)
	_ = Render(gtx3, th, &p)
}

func TestRender_ClosedStateStillRenders(t *testing.T) {
	// Render does not check p.IsOpen() — the caller decides. Verify Render
	// is well-defined even on a default-zero State (acts as black, default
	// fields, etc.).
	th := material.NewTheme()
	gtx := makeRenderGtx(400, 400)
	var p State // zero value — Kind=KindNone, H=S=V=0
	dims := Render(gtx, th, &p)
	if dims.Size.X <= 0 {
		t.Errorf("Render on zero State returned bad size: %+v", dims.Size)
	}
}

// TODO bug: colorpicker.go:225-226 — Render does NOT clamp p.S/p.V/p.H before
// computing cursor X/Y. If LastHSV / H,S,V contain NaN or Inf (which State.Open
// can produce when the caller passes a color whose rgbToHSV path is well-defined
// but downstream user code sets H/S/V directly), the int() conversion produces
// implementation-defined results and may draw the ring marker far outside the
// SV rect. Consider clamping with clamp01 before line 225/226 and modf32(_,360)
// before line 304.

// TODO bug: colorpicker.go:225-226 — when svW or svH is 1 (which only happens
// at PxPerDp ≈ 0.007, unrealistic but possible), `svW-1` and `svH-1` become 0
// and the multiplication `p.S*float32(0)` is well-defined as 0 — OK. But the
// cursor radius `r := border * 5` combined with hard-coded cardSize on tiny
// constraints will draw the ring partially outside the visible card. Probably
// cosmetic only.
