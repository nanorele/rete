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

func TestStickyHeaderMenuButton(t *testing.T) {
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
	host.OpenRequestInTab = func(*collections.CollectionNode) {}

	root := mkNode("root", true)
	root.Expanded = true
	fld := mkNode("fld", true)
	fld.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	root.Children = append(root.Children, fld)
	const N = 40
	for i := 0; i < N; i++ {
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
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(240, 320)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	clickAt := func(x, y float32) {
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		frame()
	}

	// Scroll deep so root + fld become sticky, then click the "..." button on the
	// top sticky row (root): its menu should open and the list should scroll so
	// root is the first visible row.
	hit := false
	for y := float32(29); y <= 46 && !hit; y++ {
		for x := float32(238); x >= 218; x-- {
			host.ColList.Position.First = 12
			root.MenuOpen = false
			fld.MenuOpen = false
			frame()
			clickAt(x, y)
			if root.MenuOpen {
				if host.ColList.Position.First != 0 {
					t.Errorf("sticky menu opened but list did not scroll root to top (First=%d)", host.ColList.Position.First)
				}
				hit = true
				break
			}
		}
	}
	if !hit {
		t.Fatalf("clicking the sticky '...' button never opened the folder menu")
	}
}
