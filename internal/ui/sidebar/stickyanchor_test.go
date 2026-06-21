package sidebar

import (
	"fmt"
	"image"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func stickyConfigureHost(host *Host) {
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
}

func stickyAnchorHost(t *testing.T, n int) (*Host, func(), []*collections.CollectionNode) {
	t.Helper()
	host, cleanup := newTestHost()
	stickyConfigureHost(host)

	root := mkNode("root", true)
	root.Expanded = true
	fld := mkNode("fld", true)
	fld.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	root.Children = append(root.Children, fld)
	reqs := make([]*collections.CollectionNode, 0, n)
	for i := 0; i < n; i++ {
		req := &collections.CollectionNode{Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"}}
		fld.Children = append(fld.Children, req)
		reqs = append(reqs, req)
	}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}

	visible := []*collections.CollectionNode{root, fld}
	visible = append(visible, reqs...)
	*host.VisibleCols = visible

	return host, cleanup, visible
}

func stickyFrame(host *Host, r *input.Router) {
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

func TestStickyBandStableAcrossOffset(t *testing.T) {
	host, cleanup, visible := stickyAnchorHost(t, 12)
	defer cleanup()

	r := new(input.Router)
	lastChild := len(visible) - 1

	var baseRows []string
	baseH := -1
	for _, off := range []int{0, 5, 10, 15} {
		host.ColList.Position.First = lastChild
		host.ColList.Position.Offset = off
		stickyFrame(host, r)

		rows := make([]string, len(host.StickyRows))
		for i, n := range host.StickyRows {
			rows[i] = n.Name
		}
		if baseRows == nil {
			baseRows = rows
			baseH = *host.StickyBandH
			continue
		}
		if fmt.Sprint(rows) != fmt.Sprint(baseRows) {
			t.Errorf("offset %d: sticky rows changed with offset: %v vs %v", off, rows, baseRows)
		}
		if *host.StickyBandH != baseH {
			t.Errorf("offset %d: sticky band height changed with offset: %d vs %d", off, *host.StickyBandH, baseH)
		}
	}
}

func TestStickyBandComposition(t *testing.T) {
	host, cleanup, visible := stickyAnchorHost(t, 12)
	defer cleanup()

	r := new(input.Router)

	firstChild := 2
	for _, first := range []int{firstChild, firstChild + 3, len(visible) - 1} {
		host.ColList.Position.First = first
		host.ColList.Position.Offset = 0
		stickyFrame(host, r)

		got := make([]string, len(host.StickyRows))
		for i, n := range host.StickyRows {
			got[i] = n.Name
		}
		want := []string{"root", "fld"}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("First=%d: sticky band = %v, want %v", first, got, want)
		}
		if len(host.StickyRows) > 0 {
			deepest := host.StickyRows[len(host.StickyRows)-1]
			if deepest != visible[first].Parent {
				t.Errorf("First=%d: innermost sticky header = %q, want parent %q",
					first, deepest.Name, visible[first].Parent.Name)
			}
		}
	}
}

func TestStickyMaxRowsKeepsInnermost(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()
	stickyConfigureHost(host)

	root := mkNode("root", true)
	root.Expanded = true
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}

	const depth = 20
	cur := root
	for i := 0; i < depth; i++ {
		f := mkNode(fmt.Sprintf("f%d", i), true)
		f.Expanded = true
		cur.Children = append(cur.Children, f)
		cur = f
	}
	leaf := &collections.CollectionNode{Name: "leaf", Request: &model.ParsedRequest{Method: "GET"}}
	cur.Children = append(cur.Children, leaf)
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}

	var visible []*collections.CollectionNode
	for n := root; n != nil; {
		visible = append(visible, n)
		if len(n.Children) > 0 {
			n = n.Children[0]
		} else {
			n = nil
		}
	}
	*host.VisibleCols = visible

	r := new(input.Router)
	leafIdx := len(visible) - 1
	host.ColList.Position.First = leafIdx
	host.ColList.Position.Offset = 0
	stickyFrame(host, r)

	if len(host.StickyRows) == 0 {
		t.Fatal("expected a non-empty sticky band for a deeply nested node")
	}
	if len(host.StickyRows) >= leafIdx {
		t.Fatalf("expected the band to be truncated below the full ancestor count (%d), got %d rows",
			leafIdx, len(host.StickyRows))
	}
	deepest := host.StickyRows[len(host.StickyRows)-1]
	if deepest != leaf.Parent {
		t.Errorf("truncated band dropped the innermost ancestor: bottom header = %q, want immediate parent %q",
			deepest.Name, leaf.Parent.Name)
	}
}

func TestStickyBandTracksScopeBoundary(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()
	stickyConfigureHost(host)

	root := mkNode("root", true)
	root.Expanded = true
	fldA := mkNode("A", true)
	fldA.Expanded = true
	fldB := mkNode("B", true)
	fldB.Expanded = true
	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	visible := []*collections.CollectionNode{root}
	const perFolder = 8
	for fi, fld := range []*collections.CollectionNode{fldA, fldB} {
		visible = append(visible, fld)
		for i := 0; i < perFolder; i++ {
			req := mkReq(fmt.Sprintf("%c%d", 'a'+fi, i))
			fld.Children = append(fld.Children, req)
			visible = append(visible, req)
		}
	}
	root.Children = []*collections.CollectionNode{fldA, fldB}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	*host.VisibleCols = visible

	r := new(input.Router)
	cases := []struct {
		first int
		want  *collections.CollectionNode
	}{
		{1, fldA},
		{2, fldA},
		{5, fldA},
		{10, fldB},
		{11, fldB},
		{14, fldB},
	}
	for _, tc := range cases {
		host.ColList.Position.First = tc.first
		host.ColList.Position.Offset = 0
		stickyFrame(host, r)
		if len(host.StickyRows) == 0 {
			t.Errorf("First=%d (%q): empty sticky band, want innermost %q",
				tc.first, visible[tc.first].Name, tc.want.Name)
			continue
		}
		deepest := host.StickyRows[len(host.StickyRows)-1]
		if deepest != tc.want {
			t.Errorf("First=%d (%q): innermost sticky header = %q, want %q",
				tc.first, visible[tc.first].Name, deepest.Name, tc.want.Name)
		}
	}
}
