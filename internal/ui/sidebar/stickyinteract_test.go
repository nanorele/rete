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

func buildStickyInteractHost(t *testing.T) (*Host, func(), *input.Router, *collections.CollectionNode, *collections.CollectionNode, *bool) {
	t.Helper()
	host, cleanup := newTestHost()
	t.Cleanup(cleanup)

	host.ColsMenuBtn = &widget.Clickable{}
	cmo := false
	host.ColsMenuOpen = &cmo
	host.EnvsMenuBtn = &widget.Clickable{}
	emo := false
	host.EnvsMenuOpen = &emo
	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }
	colsExp := true
	envsExp := false
	host.ColsExpanded = &colsExp
	host.EnvsExpanded = &envsExp
	host.OpenRequestInTab = func(*collections.CollectionNode) {}

	root := mkNode("root", true)
	root.Expanded = true
	fld := mkNode("fld", true)
	fld.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	root.Children = append(root.Children, fld)
	for i := 0; i < 40; i++ {
		n := &collections.CollectionNode{Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"}}
		fld.Children = append(fld.Children, n)
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root, fld}
	visible = append(visible, fld.Children...)
	*host.VisibleCols = visible

	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(240, 320)), Source: r.Source()}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}
	return host, frame, r, root, fld, &colsExp
}

func TestStickyHeaderHover(t *testing.T) {
	host, frame, r, root, fld, _ := buildStickyInteractHost(t)

	host.ColList.Position.First = 12
	frame()

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 38), Source: pointer.Mouse})
	frame()
	if !root.StickyClick.Hovered() {
		t.Fatal("root sticky row not hovered while pointer is over it")
	}
	if fld.StickyClick.Hovered() {
		t.Fatal("fld sticky row hovered while pointer is over root")
	}

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 300), Source: pointer.Mouse})
	frame()
	if root.StickyClick.Hovered() {
		t.Fatal("root sticky row still hovered after the pointer left the band")
	}
}

func TestStickyHeaderChevronCollapse(t *testing.T) {
	host, frame, r, root, fld, colsExp := buildStickyInteractHost(t)

	collapsed := 0
	host.UpdateVisibleCols = func() { collapsed++ }

	clickAt := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		frame()
	}

	hit := false
	for y := float32(54); y <= 72 && !hit; y++ {
		*colsExp = true
		root.Expanded = true
		fld.Expanded = true
		host.ColList.Position.First = 12
		frame()
		clickAt(44, y)
		if !fld.Expanded {
			if !root.Expanded {
				t.Fatalf("chevron click at y=%v collapsed the root too, not just fld", y)
			}
			if collapsed == 0 {
				t.Errorf("collapsing via the sticky chevron did not refresh the visible list")
			}
			hit = true
		}
	}
	if !hit {
		t.Fatal("clicking the sticky chevron never collapsed the folder")
	}
}

func TestStickyBandForwardsScrollDelta(t *testing.T) {
	host, frame, r, _, _, _ := buildStickyInteractHost(t)

	host.ColList.Position.First = 12
	host.ColList.Position.Offset = 0
	frame()
	if *host.StickyBandH <= 0 {
		t.Fatal("band did not render at First=12")
	}

	forwarded := 0
	DebugStickyScroll = func(d, _, _ int) {
		if d != 0 {
			forwarded += d
		}
	}
	defer func() { DebugStickyScroll = nil }()

	for i := 0; i < 5; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 36), Source: pointer.Mouse})
		r.Queue(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(120, 36), Source: pointer.Mouse, Scroll: f32.Pt(0, 30)})
		frame()
	}
	if forwarded <= 0 {
		t.Fatalf("scrolling over the sticky band did not forward any scroll to the list (forwarded=%d)", forwarded)
	}
	if host.ColList.Position.Offset <= 0 {
		t.Fatalf("forwarded band scroll did not reach the list offset (Offset=%d)", host.ColList.Position.Offset)
	}
}
