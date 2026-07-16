package workspace

import "testing"

func TestHeadersSliderOpensCollapsedHeaders(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	sliderY := rig.paneTop() + rig.tab.headersRowH + 2
	rig.drag(400, sliderY, sliderY+90)
	if !rig.tab.HeadersExpanded {
		t.Fatalf("dragging the slider down while headers are hidden must open the headers section")
	}
	if got := rig.tab.HeadersAbsHeight; !near(got, 90, 4) {
		t.Errorf("opened headers should follow the drag distance: got %d, want ~90", got)
	}
	if got := rig.tab.headersRenderH; !near(got, 90, 4) {
		t.Errorf("rendered headers should follow the drag: got %d", got)
	}
	if got := rig.paneH(); !near(got, paneBefore+91, 5) {
		t.Errorf("opening headers must push Request down: pane %d, want ~%d", got, paneBefore+91)
	}
}

func TestHeadersSliderNoJitter(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 20
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.headersSliderY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	prevPane := rig.paneH()
	for i := 1; i <= 60; i++ {
		rig.r.Queue(pointerMove(400, y+i))
		rig.frame()
		pane := rig.paneH()
		if pane < prevPane {
			t.Fatalf("pane jittered on step %d: %d -> %d", i, prevPane, pane)
		}
		stored := rig.tab.HeadersAbsHeight
		if render := rig.tab.headersRenderH; !near(render, stored, 1) {
			t.Fatalf("rendered headers diverged from stored on step %d: render %d, stored %d", i, render, stored)
		}
		prevPane = pane
	}
	rig.r.Queue(pointerRelease(400, y+60))
	rig.frame()

	if got := rig.tab.HeadersAbsHeight; !near(got, 80, 3) {
		t.Errorf("slow drag should grow headers 20->~80, got %d", got)
	}
}

func TestHeadersSliderNoJitterOnPointerTremor(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 20
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	y := rig.headersSliderY()
	rig.r.Queue(pointerPress(400, y))
	rig.frame()
	rig.r.Queue(pointerMoveF(400, float32(y)+5))
	rig.frame()
	basePane := rig.paneH()
	baseStored := rig.tab.HeadersAbsHeight

	tremor := []float32{0.4, -0.3, 0.45, -0.4, 0.3, -0.45, 0.4, -0.3}
	pos := float32(y) + 5
	for i := 0; i < 32; i++ {
		pos += tremor[i%len(tremor)]
		rig.r.Queue(pointerMoveF(400, pos))
		rig.frame()
		if got := rig.paneH(); !near(got, basePane, 1) {
			t.Fatalf("pane jittered on tremor step %d: %d vs base %d", i, got, basePane)
		}
		if got := rig.tab.HeadersAbsHeight; !near(got, baseStored, 1) {
			t.Fatalf("stored height jittered on tremor step %d: %d vs base %d", i, got, baseStored)
		}
	}
	rig.r.Queue(pointerRelease(400, int(pos)))
	rig.frame()
}

func TestWSHeadersSliderOpensCollapsedHeaders(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.HeadersCollapsed = true
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.wsPaneH()

	sliderY := rig.paneTop() + s.wsRowH + 1 + s.wsRowH + 2
	rig.drag(400, sliderY, sliderY+90)
	if s.HeadersCollapsed {
		t.Fatalf("dragging the slider down while WS headers are hidden must open the section")
	}
	if got := s.HeadersAbsHeight; !near(got, 90, 4) {
		t.Errorf("opened WS headers should follow the drag distance: got %d, want ~90", got)
	}
	if got := rig.wsPaneH(); !near(got, paneBefore+91, 5) {
		t.Errorf("opening WS headers must push Compose down: pane %d, want ~%d", got, paneBefore+91)
	}
}
