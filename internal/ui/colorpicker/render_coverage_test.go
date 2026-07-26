package colorpicker

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

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
