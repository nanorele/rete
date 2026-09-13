package sidebar

import (
	"fmt"
	"image"
	"testing"

	"rete/internal/model"
	"rete/internal/ui/collections"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

type menuDraw struct {
	name        string
	anchorY, rh int
}

func buildStickyMenuHost(t *testing.T) (host *Host, frame func(), r *input.Router, fld, fld2 *collections.CollectionNode, draws *[]menuDraw, bandY func(*collections.CollectionNode) (int, bool)) {
	t.Helper()
	var cleanup func()
	host, cleanup = newTestHost()
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
	host.ColList.Axis = layout.Vertical

	root := mkNode("root", true)
	root.Expanded = true
	fld = mkNode("fld", true)
	fld.Expanded = true
	fld2 = mkNode("fld2", true)
	fld2.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	root.Children = []*collections.CollectionNode{fld, fld2}
	for i := 0; i < 40; i++ {
		fld.Children = append(fld.Children, &collections.CollectionNode{Name: fmt.Sprintf("a-%d", i), Request: &model.ParsedRequest{Method: "GET"}})
		fld2.Children = append(fld2.Children, &collections.CollectionNode{Name: fmt.Sprintf("b-%d", i), Request: &model.ParsedRequest{Method: "GET"}})
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root, fld}
	visible = append(visible, fld.Children...)
	visible = append(visible, fld2)
	visible = append(visible, fld2.Children...)
	*host.VisibleCols = visible

	var names []string
	var ys []int
	DebugBandGeom = func(n []string, y []int, _ int) { names, ys = n, y }
	t.Cleanup(func() { DebugBandGeom = nil })
	d := &[]menuDraw{}
	DebugNodeMenu = func(name string, y, rh int) { *d = append(*d, menuDraw{name, y, rh}) }
	t.Cleanup(func() { DebugNodeMenu = nil })

	r = new(input.Router)
	frame = func() {
		*d = (*d)[:0]
		names, ys = nil, nil
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(240, 700)), Source: r.Source()}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}
	bandY = func(n *collections.CollectionNode) (int, bool) {
		for i, nm := range names {
			if nm == n.Name {
				return ys[i], true
			}
		}
		return 0, false
	}
	return host, frame, r, fld, fld2, d, bandY
}

func TestStickyIncomingRowMenuDrawnOnce(t *testing.T) {
	host, frame, _, fld, fld2, draws, bandY := buildStickyMenuHost(t)

	frame()
	host.ColList.Position.First = 41
	host.ColList.Position.Offset = 0
	frame()
	frame()
	y2, ok := bandY(fld2)
	if !ok {
		t.Fatalf("fld2 is not an incoming sticky line at First=41")
	}
	if _, ok := bandY(fld); !ok {
		t.Fatalf("fld is not pinned in the band at First=41")
	}

	fld2.MenuOpen = true
	frame()
	if len(*draws) != 1 {
		t.Fatalf("menu for the incoming sticky row drawn %d times: %v", len(*draws), *draws)
	}
	if (*draws)[0].anchorY != y2 {
		t.Fatalf("menu anchored at %d, want the band row y %d: %v", (*draws)[0].anchorY, y2, *draws)
	}
	fld2.MenuOpen = false

	fld.MenuOpen = true
	frame()
	if len(*draws) != 1 {
		t.Fatalf("menu for the docked sticky row drawn %d times: %v", len(*draws), *draws)
	}
	if y, _ := bandY(fld); (*draws)[0].anchorY != y {
		t.Fatalf("docked menu anchored at %d, want %d", (*draws)[0].anchorY, y)
	}
	fld.MenuOpen = false

	req := fld2.Children[2]
	req.MenuOpen = true
	frame()
	if len(*draws) != 1 || (*draws)[0].name != req.Name {
		t.Fatalf("menu for a plain list row drawn %d times: %v", len(*draws), *draws)
	}
	if (*draws)[0].anchorY <= y2 {
		t.Fatalf("plain row menu anchored at %d, expected below the band row at %d", (*draws)[0].anchorY, y2)
	}
}

func TestStickyRowContextMenuAnchorsToBand(t *testing.T) {
	host, frame, r, _, fld2, draws, bandY := buildStickyMenuHost(t)

	frame()
	host.ColList.Position.First = 41
	host.ColList.Position.Offset = 0
	frame()
	frame()
	y2, ok := bandY(fld2)
	if !ok {
		t.Fatalf("fld2 is not an incoming sticky line at First=41")
	}

	hitY := float32(-1)
	for y := float32(0); y < 200; y++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(100, y), Source: pointer.Mouse})
		frame()
		if fld2.StickyClick.Hovered() {
			hitY = y
			break
		}
	}
	if hitY < 0 {
		t.Fatal("never hovered the fld2 sticky row")
	}
	r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(100, hitY), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(100, hitY), Source: pointer.Mouse})
	frame()
	if !fld2.MenuOpen || !fld2.CtxMenu.AtPointer {
		t.Fatalf("right-click on the sticky row did not open its context menu: open=%v atPointer=%v", fld2.MenuOpen, fld2.CtxMenu.AtPointer)
	}
	if len(*draws) != 1 {
		t.Fatalf("context menu drawn %d times: %v", len(*draws), *draws)
	}
	if (*draws)[0].anchorY != y2 {
		t.Fatalf("context menu anchored at %d, want band row y %d", (*draws)[0].anchorY, y2)
	}
	if p := fld2.CtxMenu.Pos; p.Y < 0 || p.Y >= 40 {
		t.Fatalf("context menu pointer offset %v is not relative to the sticky row", p)
	}
}
