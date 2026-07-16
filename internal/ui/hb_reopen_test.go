package ui

import (
	"testing"
)

func TestHeadersSliderReopenFromCollapsedSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.Headers = nil
	tab.HeadersAbsHeight = 16
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.05
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	tab.HeadersAbsHeight = 16
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.press(x, float32(sliderY))
	pos := float32(sliderY)
	for i := 0; i < 40; i++ {
		pos -= 1
		rig.move(x, pos)
	}
	if tab.HeadersExpanded {
		t.Fatalf("headers did not collapse after dragging up")
	}

	var stores []int
	var expanded []bool
	for i := 0; i < 60; i++ {
		pos += 1
		rig.move(x, pos)
		h := -1
		if tab.HeadersExpanded {
			h = tab.HeadersAbsHeight
		}
		stores = append(stores, h)
		expanded = append(expanded, tab.HeadersExpanded)
	}
	rig.release(x, pos)
	t.Logf("stores=%v", stores)

	opened := false
	prev := -1
	for i, h := range stores {
		if h >= 0 {
			opened = true
			if prev >= 0 {
				step := h - prev
				if step < 0 {
					t.Fatalf("stored height rolled back on step %d: %d -> %d", i, prev, h)
				}
				if step > 2 {
					t.Fatalf("stored height jumped on step %d: %d -> %d", i, prev, h)
				}
			}
			prev = h
		} else if opened {
			t.Fatalf("headers re-collapsed on step %d while dragging down", i)
		}
	}
	if !opened {
		t.Fatalf("headers never reopened after 60px of downward drag")
	}
}
