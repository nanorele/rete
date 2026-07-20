package apptest

import (
	"testing"
)

func (rig *appSliderRig) findSidebarHandle(t *testing.T, y int) int {
	for x := 100; x < 700; x++ {
		before := rig.ui.SidebarWidth
		rig.press(x, float32(y))
		rig.move(x+8, float32(y))
		changed := rig.ui.SidebarWidth != before
		rig.release(x+8, float32(y))
		if changed {
			rig.ui.SidebarWidth = before
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return x
		}
	}
	t.Fatalf("sidebar width handle not found")
	return 0
}

func TestSidebarWidthDragSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	y := 400
	x := rig.findSidebarHandle(t, y)
	t.Logf("handle at x=%d width=%d", x, rig.ui.SidebarWidth)

	rig.press(x, float32(y))
	pos := float32(x)
	var widths []int
	for i := 0; i < 40; i++ {
		pos += 1
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	steady := len(widths)
	for i := 0; i < 160; i++ {
		pos -= 1
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	past := len(widths)
	tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
	for i := 0; i < 18; i++ {
		pos += tremor[i%len(tremor)]
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	rig.release(int(pos), float32(y))
	t.Logf("widths=%v", widths)

	for i := 1; i < steady; i++ {
		if widths[i] < widths[i-1] {
			t.Fatalf("width rolled back while growing on step %d: %d -> %d", i, widths[i-1], widths[i])
		}
	}
	for i := steady + 1; i < past; i++ {
		if widths[i] > widths[i-1] {
			t.Fatalf("width rolled back while shrinking on step %d: %d -> %d", i, widths[i-1], widths[i])
		}
	}
	min := widths[past-1]
	for i := past; i < len(widths); i++ {
		if widths[i] != min {
			t.Fatalf("tremor past the min moved sidebar on step %d: %d -> %d", i, min, widths[i])
		}
	}
}

func TestSidebarEnvSectionDragSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	rig.ui.ColsExpanded = true
	rig.ui.EnvsExpanded = true
	rig.ui.ScriptsExpanded = true
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	x := 150
	divY := -1
	base := rig.ui.EnvDivY()
	if base <= 0 {
		t.Fatalf("env divider offset not recorded")
	}
	for off := 10; off < 60; off++ {
		y := base + off
		if y >= rig.sz.Y {
			break
		}
		rig.press(x, float32(y))
		grabbed := rig.ui.SidebarEnvDrag.Pressed()
		rig.release(x, float32(y))
		rig.ui.ColsExpanded = true
		rig.ui.EnvsExpanded = true
		rig.ui.ScriptsExpanded = true
		rig.frame()
		if grabbed {
			divY = y
			break
		}
	}
	if divY < 0 {
		t.Fatalf("env divider not found near offset %d", base)
	}
	t.Logf("env divider at y=%d envH=%d", divY, rig.ui.SidebarEnvHeight)

	rig.press(x, float32(divY))
	if !rig.ui.SidebarEnvDrag.Pressed() {
		t.Fatalf("env drag did not grab at y=%d", divY)
	}
	pos := float32(divY)
	var heights []int
	for i := 0; i < 50; i++ {
		pos -= 1
		rig.movePt(float32(x), pos)
		heights = append(heights, rig.ui.SidebarEnvHeight)
	}
	grow := len(heights)
	for i := 0; i < 50; i++ {
		pos += 1
		rig.movePt(float32(x), pos)
		heights = append(heights, rig.ui.SidebarEnvHeight)
	}
	rig.release(x, pos)
	t.Logf("heights=%v", heights)

	for i := 1; i < grow; i++ {
		if heights[i] < heights[i-1] {
			t.Fatalf("env height rolled back while growing on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
		if heights[i]-heights[i-1] > 2 {
			t.Fatalf("env height jumped on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
	}
	for i := grow + 1; i < len(heights); i++ {
		if heights[i] > heights[i-1] {
			t.Fatalf("env height rolled back while shrinking on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
	}
}
