package apptest

import (
	"testing"

	"tracto/internal/ui/workspace"
)

func TestSplitDragAppliesToAllTabs(t *testing.T) {
	rig := newAppSliderRig(t)
	active := rig.ui.Tabs[0]

	other := workspace.NewRequestTab("T2")
	other.Method = "GET"
	other.URLInput.SetText("http://other.example.com")
	other.AddHeader("X-Other", "1")
	rig.ui.Tabs = append(rig.ui.Tabs, other)

	for i := 0; i < 4; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)

	before := other.VStackRatio
	rig.press(x, float32(divY))
	rig.move(x, float32(divY-40))
	rig.release(x, float32(divY-40))

	if active.VStackRatio == before {
		t.Fatalf("split drag did not move the active tab ratio (%v)", active.VStackRatio)
	}
	if other.VStackRatio != active.VStackRatio {
		t.Errorf("split ratio not shared: active=%v other=%v", active.VStackRatio, other.VStackRatio)
	}

	beforeH := other.HeadersAbsHeight
	rig.press(x, float32(sliderY))
	rig.move(x, float32(sliderY+40))
	rig.release(x, float32(sliderY+40))

	if active.HeadersAbsHeight == beforeH {
		t.Fatalf("headers drag did not resize the active tab (%d)", active.HeadersAbsHeight)
	}
	if other.HeadersAbsHeight != active.HeadersAbsHeight {
		t.Errorf("headers height not shared: active=%d other=%d", active.HeadersAbsHeight, other.HeadersAbsHeight)
	}

	active.LayoutMode = workspace.LayoutModeHoriz
	rig.frame()
	if other.LayoutMode != workspace.LayoutModeHoriz {
		t.Errorf("layout mode not shared: other=%d", other.LayoutMode)
	}
}
