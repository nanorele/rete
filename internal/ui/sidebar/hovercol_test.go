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

func TestRealLayoutColHoverScroll(t *testing.T) {
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

	const N = 30
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

	const winH = 240
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(220, winH)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	listHovered := func() bool {
		for _, n := range append([]*collections.CollectionNode{root}, nodes...) {
			if ent, _ := peekHover(&n.Hover); ent {
				return true
			}
		}
		return false
	}

	host.ColList.Position.First = 1
	frame()

	hitY := -1
	for y := 0; y < winH && hitY < 0; y += 2 {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, float32(y)), Source: pointer.Mouse})
		frame()
		if root.StickyClick.Hovered() || listHovered() {
			hitY = y
		}
	}
	if hitY < 0 {
		t.Fatal("no Y hovers the band or a collection row")
	}

	for s := 2; s <= 13; s++ {
		host.ColList.Position.First = s
		host.ColList.Position.Offset = 0
		frame()
	}

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, 999), Source: pointer.Mouse})
	frame()
	frame()
	if listHovered() {
		t.Error("STUCK HOVER: a collection list row stayed hovered after the cursor left")
	}
	if root.StickyClick.Hovered() {
		t.Error("STUCK HOVER: the sticky band row stayed hovered after the cursor left")
	}
}
