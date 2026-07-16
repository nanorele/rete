package sidebar

import (
	"fmt"
	"image"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func setupScrollableHost(t *testing.T) (*Host, []*collections.CollectionNode, func()) {
	host, cleanup := newTestHost()
	host.ColsMenuBtn = &widget.Clickable{}
	cmo := false
	host.ColsMenuOpen = &cmo
	host.EnvsMenuBtn = &widget.Clickable{}
	emo := false
	host.EnvsMenuOpen = &emo
	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	colsExp := true
	envsExp := false
	host.ColsExpanded = &colsExp
	host.EnvsExpanded = &envsExp

	const N = 40
	root := mkNode("root", true)
	root.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	nodes := make([]*collections.CollectionNode, 0, N)
	for i := 0; i < N; i++ {
		n := &collections.CollectionNode{
			Name:    fmt.Sprintf("req-%d", i),
			Request: &model.ParsedRequest{Name: fmt.Sprintf("req-%d", i), Method: "GET"},
		}
		root.Children = append(root.Children, n)
		nodes = append(nodes, n)
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root}
	visible = append(visible, nodes...)
	*host.VisibleCols = visible
	return host, nodes, cleanup
}

// The scrollbar thumb must stay visible on top of the sticky band, own its
// cursor (no text I-beam bleeding up from a renaming row's editor beneath it),
// and dragging it must scroll the list.
func TestSidebarScrollbarOverlay(t *testing.T) {
	host, nodes, cleanup := setupScrollableHost(t)
	defer cleanup()

	// A renaming node lays out an inline text field (CursorText) beneath the
	// scrollbar; the overlay must mask it.
	for _, n := range nodes {
		n.IsRenaming = true
		n.NameEditor.SingleLine = true
		n.NameEditor.SetText(n.Name + " a fairly long name to fill the row")
	}

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

	// Scroll down so a sticky band exists.
	host.ColList.Position.First = 8
	host.ColList.Position.Offset = 0
	frame()
	frame()
	if *host.StickyBandH <= 0 {
		t.Fatalf("expected a sticky band (StickyBandH>0), got %d", *host.StickyBandH)
	}

	barX := float32(winW - 2)

	// Find the list body's top (the cols header sits above it): the smallest y
	// where a press+drag on the bar engages the scrollbar drag.
	bodyTop := -1
	for y := 0; y < winH-40 && bodyTop < 0; y += 2 {
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(barX, float32(y)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(barX, float32(y+20)), Source: pointer.Mouse})
		frame()
		if host.ColList.Scrollbar.Dragging() {
			bodyTop = y
		}
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(barX, float32(y+20)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
		frame()
	}
	if bodyTop < 0 {
		t.Fatal("could not locate the scrollbar drag area")
	}
	// Re-establish the scrolled state + band after the probing above.
	host.ColList.Position.First = 8
	host.ColList.Position.Offset = 0
	frame()
	band := *host.StickyBandH
	if band <= 0 {
		t.Fatalf("expected a sticky band, got %d", band)
	}

	// Inside the sticky-band region, over the scrollbar: cursor must not be the
	// text I-beam and the thumb overlay must be reachable (on top of the band).
	bandY := float32(bodyTop + band/2)
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(barX, bandY), Source: pointer.Mouse})
	frame()
	if c := r.Cursor(); c == pointer.CursorText {
		t.Errorf("scrollbar over sticky band shows I-beam (CursorText)")
	}

	// Press inside the band region and drag down: only works if the scrollbar
	// overlay is on top of the sticky band. The list must scroll and the cursor
	// must never become the text I-beam.
	before := host.ColList.Position.First
	r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(barX, bandY), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	frame()
	for y := bodyTop + band; y <= winH-8; y += 10 {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(barX, float32(y)), Source: pointer.Mouse})
		frame()
		if c := r.Cursor(); c == pointer.CursorText {
			t.Errorf("scrollbar drag shows I-beam (CursorText) at y=%d", y)
		}
	}
	r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(barX, float32(winH-8)), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	frame()

	if host.ColList.Position.First <= before {
		t.Errorf("dragging the scrollbar (from the band region) did not scroll the list (First %d -> %d)", before, host.ColList.Position.First)
	}
}
