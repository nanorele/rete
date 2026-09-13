package sidebar

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestNodeRightClickOpensMenu(t *testing.T) {
	host, nodes, cleanup := setupScrollableHost(t)
	defer cleanup()
	host.ColList.Axis = layout.Vertical

	const winW, winH = 220, 240
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(winW, winH)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}
	rightClick := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
		frame()
		frame()
	}
	anyOpen := func() int {
		for i, n := range *host.VisibleCols {
			if n.MenuOpen {
				return i
			}
		}
		return -1
	}

	host.ColList.Position.First = 8
	host.ColList.Position.Offset = 0
	frame()
	frame()
	band := *host.StickyBandH
	if band <= 0 {
		t.Fatalf("expected a sticky band, got %d", band)
	}

	var rowY int
	for y := 0; y < winH-band && rowY == 0; y++ {
		for _, n := range nodes {
			n.MenuOpen = false
		}
		rightClick(40, float32(y))
		if idx := anyOpen(); idx > 0 {
			rowY = y
			n := (*host.VisibleCols)[idx]
			if !n.CtxMenu.AtPointer {
				t.Errorf("right-click menu must anchor at the pointer")
			}
			if host.ColList.Position.First != 8 {
				t.Errorf("right-click must not scroll the list (First=%d)", host.ColList.Position.First)
			}
			if *host.DraggedNode != nil {
				t.Errorf("right-click must not start a node drag")
			}
		}
	}
	if rowY == 0 {
		t.Fatal("right-clicking a list row never opened its menu")
	}

	for _, n := range *host.VisibleCols {
		n.MenuOpen = false
	}
	root := (*host.VisibleCols)[0]
	hitBand := false
	for y := 0; y < winH && !hitBand; y++ {
		rightClick(40, float32(y))
		if root.MenuOpen {
			hitBand = true
			if !root.CtxMenu.AtPointer {
				t.Errorf("sticky-band right-click menu must anchor at the pointer")
			}
		}
	}
	if !hitBand {
		t.Fatal("right-clicking the sticky band row never opened the root menu")
	}
}

// A node menu is an overlay above the list; the scrollbar overlay must not
// cover it, so a press on the menu surface where the bar runs must reach the
// menu (and not start a scrollbar drag).
func TestNodeMenuAboveScrollbar(t *testing.T) {
	host, nodes, cleanup := setupScrollableHost(t)
	defer cleanup()
	host.ColList.Axis = layout.Vertical

	const winW, winH = 220, 240
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(winW, winH)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}
	press := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y+20), Source: pointer.Mouse})
		frame()
	}
	release := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
	}

	host.ColList.Position.First = 8
	host.ColList.Position.Offset = 0
	frame()
	frame()

	barX := float32(winW - 2)
	bodyTop := -1
	for y := 0; y < winH-40 && bodyTop < 0; y += 2 {
		press(barX, float32(y))
		if host.ColList.Scrollbar.Dragging() {
			bodyTop = y
		}
		release(barX, float32(y+20))
	}
	if bodyTop < 0 {
		t.Fatal("could not locate the scrollbar drag area")
	}

	host.ColList.Position.First = 8
	host.ColList.Position.Offset = 0
	first := (*host.VisibleCols)[8]
	first.MenuOpen = true
	first.CtxMenu.AtPointer = false
	frame()
	frame()
	if !first.MenuOpen {
		t.Fatal("menu closed unexpectedly")
	}
	rowH := first.RowHeightPx
	if rowH <= 0 {
		t.Fatalf("row height unknown")
	}
	menuY := float32(bodyTop + *host.StickyBandH + rowH + 12)
	press(barX, menuY)
	if host.ColList.Scrollbar.Dragging() {
		t.Errorf("scrollbar drag started through an open node menu at y=%v", menuY)
	}
	release(barX, menuY+20)
	_ = nodes
}
