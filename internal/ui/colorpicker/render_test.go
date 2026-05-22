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
