package ui

import (
	"testing"
)

func (rig *appSliderRig) findMainSplit(t *testing.T, x, from, to int) int {
	tab := rig.ui.Tabs[0]
	for y := from; y < to; y++ {
		before := tab.VStackRatio
		beforeH := tab.HeadersAbsHeight
		beforeExp := tab.HeadersExpanded
		beforeCol := tab.ReqBodyCollapsed
		rig.press(x, float32(y))
		rig.move(x, float32(y+8))
		ratioChanged := tab.VStackRatio != before
		otherChanged := tab.HeadersAbsHeight != beforeH || tab.HeadersExpanded != beforeExp
		rig.release(x, float32(y+8))
		if ratioChanged || otherChanged || tab.ReqBodyCollapsed != beforeCol {
			tab.VStackRatio = before
			tab.HeadersAbsHeight = beforeH
			tab.HeadersExpanded = beforeExp
			tab.ReqBodyCollapsed = beforeCol
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if ratioChanged && !otherChanged {
				return y
			}
		}
	}
	t.Fatalf("main split divider not found in [%d,%d)", from, to)
	return 0
}

func TestMainSplitNoJitterNearCollapse(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.008
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)
	t.Logf("slider=%d divider=%d ratio=%.4f", sliderY, divY, tab.VStackRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var collapsed []bool
	pos := float32(divY)
	for i := 1; i <= 20; i++ {
		pos -= 1
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	for i := 1; i <= 20; i++ {
		pos += 1
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	tremor := []float32{0.4, -0.3, 0.45, -0.4, 0.35, -0.45}
	for i := 0; i < 24; i++ {
		pos += tremor[i%len(tremor)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < 20; i++ {
		if ratios[i] > ratios[i-1] {
			t.Fatalf("ratio jitter on up step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	for i := 21; i < 40; i++ {
		if ratios[i] < ratios[i-1] {
			t.Fatalf("ratio jitter on down step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	base := ratios[40]
	flips := 0
	for i := 41; i < len(ratios); i++ {
		if ratios[i] != ratios[i-1] {
			flips++
		}
		if diff := ratios[i] - base; diff < -25 || diff > 25 {
			t.Fatalf("tremor moved the split too far on step %d: %d vs base %d", i, ratios[i], base)
		}
	}
	if flips > 4 {
		t.Fatalf("split oscillated %d times under pointer tremor", flips)
	}
	for i := 41; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			t.Fatalf("collapse state flapped under tremor at step %d", i)
		}
	}
}
