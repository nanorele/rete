package workspace

import (
	"testing"
	"time"
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func (rig *vstackRig) frameScaled(scale float32) {
	rig.now = rig.now.Add(16 * time.Millisecond)
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.tab.Layout(gtx, rig.th, rig.win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	rig.r.Frame(rig.ops)
}

func (rig *vstackRig) gtxScaled(scale float32) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
	}
}

func (rig *vstackRig) paneTopScaled(gtx layout.Context) int {
	urlRow := gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(28)) + gtx.Dp(unit.Dp(8))
	return urlRow + gtx.Dp(unit.Dp(1)) + rig.tab.layoutModeBarHeight(gtx)
}

func TestHeadersSliderSmoothAtFractionalScale(t *testing.T) {
	for _, scale := range []float32{1.25, 1.5} {
		rig := newVStackRig()
		rig.tab.HeadersAbsHeight = 20
		rig.tab.VStackRatio = 0.5
		for i := 0; i < 3; i++ {
			rig.frameScaled(scale)
		}
		gtx := rig.gtxScaled(scale)
		sliderY := rig.paneTopScaled(gtx) + rig.tab.reqPaneAboveHeadersPx(gtx) + rig.tab.headersRenderH + 2
		ext := rig.tab.stackedSplitExtent(gtx)
		paneOf := func() int { return int(rig.tab.VStackRatio*ext + 0.5) }
		startStored := rig.tab.HeadersAbsHeight

		rig.r.Queue(pointerPress(400, sliderY))
		rig.frameScaled(scale)
		prev := paneOf()
		for i := 1; i <= 40; i++ {
			rig.r.Queue(pointerMove(400, sliderY+i))
			rig.frameScaled(scale)
			pane := paneOf()
			if pane < prev {
				t.Fatalf("scale %.2f: pane jittered on down step %d: %d -> %d", scale, i, prev, pane)
			}
			prev = pane
		}
		if got := rig.tab.HeadersAbsHeight; got <= startStored {
			t.Fatalf("scale %.2f: drag did not register, stored still %d", scale, got)
		}
		grown := rig.tab.HeadersAbsHeight
		wantGrown := startStored + int(40/scale)
		if !near(grown, wantGrown, 3) {
			t.Errorf("scale %.2f: 40px drag should grow headers by ~%ddp, got %d -> %d", scale, wantGrown-startStored, startStored, grown)
		}

		base := sliderY + 40
		for i := 1; i <= 20; i++ {
			rig.r.Queue(pointerMove(400, base-i))
			rig.frameScaled(scale)
			pane := paneOf()
			if pane > prev {
				t.Fatalf("scale %.2f: pane jittered on up step %d: %d -> %d", scale, i, prev, pane)
			}
			prev = pane
		}
		rig.r.Queue(pointerRelease(400, base-20))
		rig.frameScaled(scale)
		if got := rig.tab.HeadersAbsHeight; got >= grown {
			t.Errorf("scale %.2f: up drag did not shrink headers, stored %d", scale, got)
		}
	}
}
