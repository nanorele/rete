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

func buildDeepFolderHost(t *testing.T) (*Host, func(), *input.Router, *collections.CollectionNode, *collections.CollectionNode, int) {
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
	host.UpdateVisibleCols = func() {}

	root := mkNode("root", true)
	root.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	for i := 0; i < 10; i++ {
		root.Children = append(root.Children, &collections.CollectionNode{Name: fmt.Sprintf("top-%d", i), Request: &model.ParsedRequest{Method: "GET"}})
	}
	fld := mkNode("fld", true)
	fld.Expanded = true
	root.Children = append(root.Children, fld)
	for i := 0; i < 40; i++ {
		fld.Children = append(fld.Children, &collections.CollectionNode{Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"}})
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root}
	visible = append(visible, root.Children[:10]...)
	visible = append(visible, fld)
	visible = append(visible, fld.Children...)
	*host.VisibleCols = visible

	fldIdx := 11
	if visible[fldIdx] != fld {
		t.Fatalf("expected fld at index %d", fldIdx)
	}

	r := new(input.Router)
	return host, func() {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(240, 320)), Source: r.Source()}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}, r, root, fld, fldIdx
}

func clickBand(r *input.Router, frame func(), x, y float32) {
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	frame()
	frame()
}

func TestStickyFolderScrollThenCollapse(t *testing.T) {
	host, frame, r, root, fld, fldIdx := buildDeepFolderHost(t)

	host.ColList.Position.First = fldIdx + 20
	frame()
	if !fld.StickyClick.Hovered() && *host.StickyBandH <= 0 {
		t.Fatal("band did not render while scrolled deep into the folder")
	}

	clickBand(r, frame, 120, 60)
	if !fld.Expanded {
		t.Fatal("first folder click collapsed the folder; it should only scroll")
	}
	if root.Expanded == false {
		t.Fatal("first folder click collapsed the root")
	}
	if host.ColList.Position.First != fldIdx-1 {
		t.Fatalf("first folder click should dock the folder near the top (First=%d, want %d)", host.ColList.Position.First, fldIdx-1)
	}

	clickBand(r, frame, 120, 60)
	if fld.Expanded {
		t.Fatal("second folder click should collapse the folder")
	}
	if !root.Expanded {
		t.Fatal("second folder click collapsed the root too")
	}
}

func TestStickyCollectionCollapsesImmediately(t *testing.T) {
	host, frame, r, root, fld, fldIdx := buildDeepFolderHost(t)

	host.ColList.Position.First = fldIdx + 20
	frame()

	clickBand(r, frame, 120, 36)
	if root.Expanded {
		t.Fatal("clicking the collection sticky header should collapse it immediately")
	}
	if !fld.Expanded {
		t.Fatal("collapsing the collection should not collapse the inner folder")
	}
}
