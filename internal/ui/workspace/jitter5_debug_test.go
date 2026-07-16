package workspace

import (
	"testing"

	"github.com/nanorele/gio/unit"
)

func TestJitter5Debug(t *testing.T) {
	for _, scale := range []float32{1, 1.25} {
		for _, step := range []int{1, 3} {
			rig := newVStackRig()
			rig.tab.HeadersAbsHeight = 100
			gtx := rig.gtxScaled(scale)
			for i := 0; i < 2; i++ {
				rig.frameScaled(scale)
			}
			ext := rig.tab.stackedSplitExtent(gtx)
			rig.tab.VStackRatio = (float32(rig.tab.stackedReqPaneMinPx(gtx)) + 5) / ext
			for i := 0; i < 3; i++ {
				rig.frameScaled(scale)
			}
			paneOf := func() int { return int(rig.tab.VStackRatio*ext + 0.5) }
			editorOf := func() int {
				return paneOf() - rig.tab.reqPaneAboveHeadersPx(gtx) - rig.tab.headersRenderH - rig.tab.reqPaneBelowHeadersPx(gtx) + gtx.Dp(unit.Dp(3))
			}
			sliderY := rig.paneTopScaled(gtx) + rig.tab.reqPaneAboveHeadersPx(gtx) + rig.tab.headersRenderH + 2

			rig.r.Queue(pointerPress(400, sliderY))
			rig.frameScaled(scale)
			var panes, renders, editors []int
			for i := 1; i <= 12; i++ {
				rig.r.Queue(pointerMove(400, sliderY+i*step))
				rig.frameScaled(scale)
				panes = append(panes, paneOf())
				renders = append(renders, rig.tab.headersRenderH)
				editors = append(editors, editorOf())
			}
			rig.r.Queue(pointerRelease(400, sliderY+12*step))
			rig.frameScaled(scale)
			t.Logf("scale=%.2f step=%d down: panes=%v", scale, step, panes)
			t.Logf("scale=%.2f step=%d down: renders=%v", scale, step, renders)
			t.Logf("scale=%.2f step=%d down: editors=%v", scale, step, editors)
		}
	}
}
