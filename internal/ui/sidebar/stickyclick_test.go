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

func TestStickyHeaderClick(t *testing.T) {
	// Overlay sticky model: the band is painted on top of the list and, in this
	// gio build, ANY opaque pointer area inside the on-top band blocks input to the
	// whole list beneath it (only pass-through ops are safe). Pinned headers are
	// therefore visual-only for now; click-to-navigate needs a bounded-hit-area
	// approach and is tracked as a follow-up. Skipped until then.
	t.Skip("pinned sticky headers are visual-only in the overlay model (see comment)")
	host, cleanup := newTestHost()
	defer cleanup()

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

	opened := 0
	host.OpenRequestInTab = func(*collections.CollectionNode) { opened++ }

	root := mkNode("root", true)
	root.Expanded = true
	fld := mkNode("fld", true)
	fld.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	root.Children = append(root.Children, fld)
	const N = 40
	reqs := make([]*collections.CollectionNode, 0, N)
	for i := 0; i < N; i++ {
		n := &collections.CollectionNode{Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"}}
		fld.Children = append(fld.Children, n)
		reqs = append(reqs, n)
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root, fld}
	visible = append(visible, reqs...)
	*host.VisibleCols = visible

	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(240, 320)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	host.ColList.Position.First = 12
	frame()

	clickAt := func(y float32) {
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(120, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(120, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		frame()
	}

	hit := false
	for y := float32(29); y <= 44; y++ {
		host.ColList.Position.First = 12
		opened = 0
		frame()
		clickAt(y)
		if host.ColList.Position.First == 1 {
			hit = true
			break
		}
	}
	if !hit {
		t.Fatalf("clicking the top sticky header never scrolled to reveal root's contents (First stayed %d)", host.ColList.Position.First)
	}
	if opened != 0 {
		t.Errorf("sticky click leaked through to the row beneath (OpenRequestInTab called %d times)", opened)
	}
}
