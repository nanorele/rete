package ui

import (
	"fmt"
	"image"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/sidebar"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

// TestStickyNoEmptyBandGap drives a tree containing both a 2-level-nested folder
// and several sibling 1-level folders and asserts the sticky band never becomes
// "twice as big with empty space on top". The OPAQUE band height (what actually
// covers list rows) must stay close to the number of pinned rows it reports.
//
// The bug: the entering slide-dock painted opaque background over the gap between
// the docked ancestors and the rising folder header. On a sibling transition that
// gap holds the PREVIOUS folder's tail rows (real content), so covering it produced
// a band ~twice as tall with an empty strip above the rising folder name. It only
// happened past the first folder, since the first folder's header docks directly
// under root with no tail rows in between.
func TestStickyNoEmptyBandGap(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	mkReq := func(n string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: n, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	const perFolder = 8
	n1 := &collections.CollectionNode{Name: "N1", IsFolder: true, Expanded: true}
	for i := 0; i < perFolder; i++ {
		n1.Children = append(n1.Children, mkReq(fmt.Sprintf("n1-%d", i)))
	}
	// N2 holds a depth-2 subfolder, so leaving it crosses two scope levels at once.
	n2 := &collections.CollectionNode{Name: "N2", IsFolder: true, Expanded: true}
	n2a := &collections.CollectionNode{Name: "N2a", IsFolder: true, Expanded: true}
	for i := 0; i < perFolder; i++ {
		n2a.Children = append(n2a.Children, mkReq(fmt.Sprintf("n2a-%d", i)))
	}
	n2.Children = append(n2.Children, n2a)
	n3 := &collections.CollectionNode{Name: "N3", IsFolder: true, Expanded: true}
	for i := 0; i < perFolder; i++ {
		n3.Children = append(n3.Children, mkReq(fmt.Sprintf("n3-%d", i)))
	}
	n4 := &collections.CollectionNode{Name: "N4", IsFolder: true, Expanded: true}
	for i := 0; i < perFolder; i++ {
		n4.Children = append(n4.Children, mkReq(fmt.Sprintf("n4-%d", i)))
	}
	root.Children = []*collections.CollectionNode{n1, n2, n3, n4}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	// The OPAQUE solid-band height (what actually occludes rows). The empty-space
	// bug is opaque fill exceeding the number of DRAWN rows (which now includes the
	// incoming folder sliding into a slot during a seamless transition).
	var solidH int
	sidebar.DebugBandSolid = func(s int) { solidH = s }
	defer func() { sidebar.DebugBandSolid = nil }()
	var drawn int
	sidebar.DebugBandGeom = func(names []string, _ []int, _ int) { drawn = len(names) }
	defer func() { sidebar.DebugBandGeom = nil }()

	// Tall viewport: the cols body is large enough that the maxRows truncation
	// (a separate tiny-sidebar edge) never engages, isolating the entering/leaving
	// transition behaviour the user reported.
	sz := image.Pt(900, 520)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.layoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	const rowH = 24
	const tol = 12 // 1px border + slight row-height slack
	worst := 0
	worstDesc := ""
	for step := 0; step < 200; step++ {
		band = band[:0]
		solidH, drawn = 0, 0
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 5)})
		now = now.Add(16 * time.Millisecond)
		frame()
		rows := drawn
		if rows == 0 {
			continue
		}
		allowed := rows*rowH + tol
		over := solidH - allowed
		if over > worst {
			worst = over
			f := ui.ColList.Position.First
			nm := "?"
			if f < len(ui.VisibleCols) {
				nm = ui.VisibleCols[f].Name
			}
			worstDesc = fmt.Sprintf("step %d First=%d(%q) Off=%d solidH=%d rows=%d(%v) allowed=%d", step, f, nm, ui.ColList.Position.Offset, solidH, rows, band, allowed)
		}
	}
	if worst > 0 {
		t.Errorf("sticky band grew %dpx beyond its pinned rows (empty-space gap): %s", worst, worstDesc)
	}
}
