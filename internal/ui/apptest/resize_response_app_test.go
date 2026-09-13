package apptest

import (
	"fmt"
	"strings"
	"testing"

	"rete/internal/ui/workspace"
)

func respHTMLLike() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		if i%12 == 5 {
			for j := 0; j < 70; j++ {
				fmt.Fprintf(&b, `<meta name="tag%d" content="value value" />`, j)
			}
		} else {
			fmt.Fprintf(&b, `<div class="row item-%d"> <span>content word here</span></div>`, i)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (rig *appSliderRig) findSideSplit(t *testing.T, y, from, to int) int {
	tab := rig.ui.Tabs[0]
	for x := from; x < to; x++ {
		before := tab.SplitRatio
		rig.press(x, float32(y))
		rig.movePt(float32(x+8), float32(y))
		changed := tab.SplitRatio != before
		rig.release(x+8, float32(y))
		if changed {
			tab.SplitRatio = before
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return x
		}
	}
	t.Fatalf("side split divider not found in x [%d,%d) at y=%d", from, to, y)
	return 0
}

func newSideRespRig(t *testing.T) *appSliderRig {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.LayoutMode = workspace.LayoutModeHoriz
	tab.WrapEnabled = true
	tab.PreviewEnabled = true
	tab.RespEditor.SetText(respHTMLLike())
	for i := 0; i < 5; i++ {
		rig.frame()
	}
	return rig
}

func (rig *appSliderRig) dragSideSplit(t *testing.T, steps int) {
	tab := rig.ui.Tabs[0]
	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	r0 := tab.SplitRatio
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < steps; i++ {
		pos -= 2
		rig.movePt(pos, float32(y))
	}
	for i := 0; i < steps; i++ {
		pos += 2
		rig.movePt(pos, float32(y))
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	t.Logf("split drag: x=%d ratio %.4f -> %.4f", x0, r0, tab.SplitRatio)
}

func TestSideSplitDragKeepsResponseBottom(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	for i := 0; i < 5; i++ {
		tab.RespEditor.SetScrollY(1 << 30)
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < 100; i++ {
		if i < 50 {
			pos -= 2
		} else {
			pos += 2
		}
		rig.movePt(pos, float32(y))
		if g := tab.RespEditor.BottomGap(); g != 0 {
			t.Fatalf("step %d: response left the bottom mid-drag, gap=%dpx", i, g)
		}
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Errorf("after side-split drag at bottom the response drifted: gap=%dpx (scrollY=%d)",
			g, tab.RespEditor.GetScrollY())
	}
}

func TestSideSplitSlowDragKeepsBottomStable(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	for i := 0; i < 5; i++ {
		tab.RespEditor.SetScrollY(1 << 30)
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	prevY := tab.RespEditor.GetScrollY()
	for i := 0; i < 60; i++ {
		if i < 30 {
			pos -= 1
		} else {
			pos += 1
		}
		rig.movePt(pos, float32(y))
		stepY := tab.RespEditor.GetScrollY()
		for k := 0; k < 3; k++ {
			rig.frame()
			if g := tab.RespEditor.BottomGap(); g != 0 {
				t.Fatalf("slow step %d idle %d: gap=%dpx (scrollY %d -> %d)",
					i, k, g, prevY, tab.RespEditor.GetScrollY())
			}
			if yNow := tab.RespEditor.GetScrollY(); yNow != stepY {
				t.Fatalf("slow step %d idle %d: scrollY oscillated %d -> %d with no geometry change",
					i, k, stepY, yNow)
			}
		}
		prevY = tab.RespEditor.GetScrollY()
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Errorf("after slow drag at bottom: gap=%dpx", g)
	}
}

func TestSideSplitSlowDragKeepsMidStable(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	tab.RespEditor.SetScrollY(tab.RespEditor.GetScrollBounds().Max.Y / 2)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off0 := tab.RespEditor.TopContentOffset()
	if off0 == 0 {
		t.Fatalf("setup: expected mid-document")
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < 60; i++ {
		if i < 30 {
			pos -= 1
		} else {
			pos += 1
		}
		rig.movePt(pos, float32(y))
		for k := 0; k < 3; k++ {
			rig.frame()
			off := tab.RespEditor.TopContentOffset()
			if d := off - off0; d < -220 || d > 220 {
				t.Fatalf("slow step %d idle %d: top content moved %d bytes (%d -> %d)", i, k, d, off0, off)
			}
		}
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off1 := tab.RespEditor.TopContentOffset()
	if d := off1 - off0; d < -220 || d > 220 {
		t.Errorf("slow round-trip moved response content: top offset %d -> %d", off0, off1)
	}
}

func TestSideSplitDragKeepsResponseMidPosition(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	tab.RespEditor.SetScrollY(tab.RespEditor.GetScrollBounds().Max.Y / 2)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off0 := tab.RespEditor.TopContentOffset()
	if off0 == 0 {
		t.Fatalf("setup: expected mid-document")
	}

	rig.dragSideSplit(t, 50)

	off1 := tab.RespEditor.TopContentOffset()
	if d := off1 - off0; d < -200 || d > 200 {
		t.Errorf("round-trip side-split drag moved response content: top offset %d -> %d (delta %d bytes)",
			off0, off1, d)
	}
}
