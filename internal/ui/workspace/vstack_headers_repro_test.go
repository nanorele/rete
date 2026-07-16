package workspace

import (
	"image"
	"testing"
	"time"
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type vstackRig struct {
	r    input.Router
	ops  *op.Ops
	tab  *RequestTab
	th   *material.Theme
	win  *app.Window
	now  time.Time
	size image.Point
}

func newVStackRig() *vstackRig {
	tab := NewRequestTab("T1")
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
	tab.AddHeader("Authorization", "secret")
	tab.LayoutMode = LayoutModeVert
	tab.HeadersExpanded = true

	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	return &vstackRig{
		ops:  new(op.Ops),
		tab:  tab,
		th:   th,
		win:  new(app.Window),
		now:  time.Unix(1700000000, 0),
		size: image.Pt(800, 600),
	}
}

func (rig *vstackRig) frame() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.tab.Layout(gtx, rig.th, rig.win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	rig.r.Frame(rig.ops)
}

func (rig *vstackRig) gtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
	}
}

func (rig *vstackRig) paneTop() int {
	return 37 + 1 + 30
}

func (rig *vstackRig) paneH() int {
	return int(rig.tab.VStackRatio*rig.tab.stackedSplitExtent(rig.gtx()) + 0.5)
}

func (rig *vstackRig) splitDividerY() int {
	return rig.paneTop() + rig.paneH() + 2
}

func (rig *vstackRig) headersSliderY() int {
	return rig.paneTop() + rig.tab.reqPaneAboveHeadersPx(rig.gtx()) + rig.tab.headersRenderH + 2
}

func pointerPress(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Position: f32.Pt(float32(x), float32(y)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerMove(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), float32(y)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerMoveF(x int, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse}
}

func pointerRelease(x, y int) pointer.Event {
	return pointer.Event{Kind: pointer.Release, Position: f32.Pt(float32(x), float32(y)), Source: pointer.Mouse}
}

func (rig *vstackRig) drag(x, y0, y1 int) {
	rig.r.Queue(pointerPress(x, y0))
	rig.frame()
	steps := 4
	for i := 1; i <= steps; i++ {
		y := y0 + (y1-y0)*i/steps
		rig.r.Queue(pointerMove(x, y))
		rig.frame()
	}
	rig.r.Queue(pointerRelease(x, y1))
	rig.frame()
	rig.frame()
}

func near(got, want, tol int) bool {
	d := got - want
	return d >= -tol && d <= tol
}

func TestVStackSplitToMinKeepsHeadersHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 200
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Fatalf("setup: headers should render at 200, got %d", got)
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)

	if got := rig.tab.HeadersAbsHeight; !near(got, 200, 2) {
		t.Errorf("split-to-min clobbered stored headers height: got %d, want ~200", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Errorf("split-to-min squeezed rendered headers: got %d, want ~200", got)
	}
	wantMin := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, wantMin, 3) {
		t.Errorf("request pane should stop at headers+request header (%dpx), got %dpx", wantMin, got)
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+300)

	if got := rig.tab.HeadersAbsHeight; !near(got, 200, 2) {
		t.Errorf("headers height lost after restoring the split: got %d, want ~200", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 200, 2) {
		t.Errorf("headers should render at their stored height again: got %d, want ~200", got)
	}
}

func TestHeadersSliderWorksAtMinRequestPane(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 200
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)
	paneBefore := rig.paneH()

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+100)
	if got := rig.tab.HeadersAbsHeight; !near(got, 300, 3) {
		t.Errorf("slider down at min pane should grow headers to ~300 by pushing the split, got %d", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 300, 3) {
		t.Errorf("rendered headers should follow the slider, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+100, 4) {
		t.Errorf("request pane should grow with headers (editor stays 0): pane %d, want ~%d", got, paneBefore+100)
	}

	paneGrown := rig.paneH()
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-160)
	if got := rig.tab.HeadersAbsHeight; !near(got, 140, 3) {
		t.Errorf("slider up should shrink headers to ~140, got %d", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 140, 3) {
		t.Errorf("rendered headers should follow the slider, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneGrown-160, 4) {
		t.Errorf("split must follow the slider up while the editor is collapsed: pane %d, want ~%d", got, paneGrown-160)
	}
}

func TestHeadersSliderPushesRequestDown(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+50)
	if got := rig.tab.HeadersAbsHeight; !near(got, 150, 2) {
		t.Errorf("slider down should grow headers to ~150, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+50, 3) {
		t.Errorf("growing headers must push Request down (shrinking Body): pane %d, want ~%d", got, paneBefore+50)
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-30)
	if got := rig.tab.HeadersAbsHeight; !near(got, 120, 2) {
		t.Errorf("slider up should shrink headers to ~120, got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+20, 3) {
		t.Errorf("shrinking headers must pull Request up (growing Body): pane %d, want ~%d", got, paneBefore+20)
	}
}

func TestHeadersSliderClampedByResponseMin(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	gtx := rig.gtx()
	extent := int(rig.tab.stackedSplitExtent(gtx))
	paneBefore := rig.paneH()
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+2000)
	if got := extent - rig.paneH(); !near(got, 120, 3) {
		t.Errorf("response pane should keep its 120px minimum, got %d", got)
	}
	maxHeaders := 100 + (extent - 120 - paneBefore)
	if got := rig.tab.HeadersAbsHeight; !near(got, maxHeaders, 5) {
		t.Errorf("slider down must stop when the response pane hits its minimum: got %d, want ~%d", got, maxHeaders)
	}

	before := rig.tab.HeadersAbsHeight
	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-30)
	if got := rig.tab.HeadersAbsHeight; !near(got, before-30, 2) {
		t.Errorf("slider up must respond immediately after an over-drag: got %d, want ~%d", got, before-30)
	}
}

func TestHeadersSliderCollapsesAtZero(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-200)
	if rig.tab.HeadersExpanded {
		t.Errorf("dragging the headers slider to zero must collapse the headers section")
	}
	if got := rig.tab.HeadersAbsHeight; got < 60 {
		t.Errorf("stored headers height must stay usable for re-expanding, got %d", got)
	}
}

func TestSlowSplitDragCollapsesRequest(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.splitDividerY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	for i := 1; i <= 300; i++ {
		rig.r.Queue(pointerMove(400, y-i))
		rig.frame()
	}
	rig.r.Queue(pointerRelease(400, y-300))
	rig.frame()
	rig.frame()

	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("a slow 1px-per-frame drag to the top must still collapse the request body")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("pane should hug the collapsed minimum: %d, want ~%d", got, want)
	}
}
