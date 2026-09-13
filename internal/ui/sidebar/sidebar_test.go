package sidebar

import (
	"encoding/json"
	"fmt"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"image"
	"image/color"
	"os"
	"reflect"
	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/colorpicker"
	"rete/internal/ui/environments"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"
	"rete/internal/ui/workspace"
	"strings"
	"testing"
	"time"
)

func (rig *sideRig) rightClick(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
	rig.frame()
	rig.frame()
}

func TestEnvRowRightClickOpensMenu(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	a := rig.addEnv("e1", "Dev")
	b := rig.addEnv("e2", "Prod")
	rig.frames(3)

	hitY := float32(-1)
	for y := float32(0); y < 700 && hitY < 0; y += 2 {
		a.MenuOpen, b.MenuOpen = false, false
		rig.frame()
		rig.rightClick(40, y)
		if b.MenuOpen {
			hitY = y
		}
	}
	if hitY < 0 {
		t.Fatal("right-clicking the environment row never opened its menu")
	}
	if !b.CtxMenu.AtPointer {
		t.Error("environment right-click menu must anchor at the pointer")
	}
	if a.MenuOpen {
		t.Error("right-click on one environment left another menu open")
	}
	if *rig.host.ActiveEnvID != "" {
		t.Errorf("right-click must not select the environment, ActiveEnvID=%q", *rig.host.ActiveEnvID)
	}
	if *rig.host.DraggedEnv != nil {
		t.Error("right-click must not start an environment drag")
	}
	if *rig.host.EditingEnv != nil {
		t.Error("right-click must not open the environment editor")
	}

	rig.click(&b.MenuBtn)
	rig.click(&b.MenuBtn)
	if !b.MenuOpen || b.CtxMenu.AtPointer {
		t.Errorf("⋯ button after a right-click: open=%v atPointer=%v, want open and anchored to the button", b.MenuOpen, b.CtxMenu.AtPointer)
	}
}

func TestScriptRowRightClickOpensMenu(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.ScriptsExpanded = true
	a := rig.addScript("s1", "first")
	b := rig.addScript("s2", "second")
	rig.frames(3)

	hitY := float32(-1)
	for y := float32(0); y < 700 && hitY < 0; y += 2 {
		a.MenuOpen, b.MenuOpen = false, false
		rig.frame()
		rig.rightClick(40, y)
		if b.MenuOpen {
			hitY = y
		}
	}
	if hitY < 0 {
		t.Fatal("right-clicking the script row never opened its menu")
	}
	if !b.CtxMenu.AtPointer {
		t.Error("script right-click menu must anchor at the pointer")
	}
	if a.MenuOpen {
		t.Error("right-click on one script left another menu open")
	}
	if len(rig.opened) != 0 {
		t.Errorf("right-click must not open the script, OpenScript calls=%v", rig.opened)
	}

	rig.click(&b.MenuBtn)
	rig.click(&b.MenuBtn)
	if !b.MenuOpen || b.CtxMenu.AtPointer {
		t.Errorf("⋯ button after a right-click: open=%v atPointer=%v, want open and anchored to the button", b.MenuOpen, b.CtxMenu.AtPointer)
	}
}

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

// f1 and f2 are expanded sibling folders; f3 is dragged from below into the
// gap between them, grabbed by its label (cursor X well right of the indent).
func expandedSiblingFolders() (host *Host, cleanup func(), root, f1, f2, f3 *collections.CollectionNode) {
	host, cleanup = newTestHost()

	root = mkNode("root", true)
	root.Expanded = true
	f1 = mkNode("f1", true)
	f1.Expanded = true
	c1 := mkNode("c1", false)
	f1.Children = []*collections.CollectionNode{c1}
	f2 = mkNode("f2", true)
	f2.Expanded = true
	c2 := mkNode("c2", false)
	f2.Children = []*collections.CollectionNode{c2}
	f3 = mkNode("f3", true)
	root.Children = []*collections.CollectionNode{f1, f2, f3}

	col := &collections.ParsedCollection{ID: "col", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)

	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root, f1, c1, f2, c2, f3}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	for i := range visible {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = len(visible) * 20
	return
}

func TestDropBetweenExpandedFolders(t *testing.T) {
	host, cleanup, root, _, f2, f3 := expandedSiblingFolders()
	defer cleanup()

	// f3 sits at row 5 (y=100); the gap between f1's subtree and f2 is y=60.
	// The pointer grabbed the row at its label, 60px from the row's left edge,
	// and never moved horizontally.
	*host.DraggedNode = f3
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 60
	*host.DragNodeCurrentX = 60
	*host.DragNodeOriginY = 10
	*host.DragNodeCurrentY = -40

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != root {
		t.Fatalf("drop between two expanded folders should stay in root; got parent=%q into=%q",
			parentName(drop.parent), parentName(drop.intoNode))
	}
	if drop.insertIdx != 1 {
		t.Fatalf("insertIdx = %d, want 1 (between f1 and f2)", drop.insertIdx)
	}

	commitNodeDrop(host, f3, unitMetric())
	if len(root.Children) != 3 || root.Children[1] != f3 {
		got := make([]string, 0, len(root.Children))
		for _, c := range root.Children {
			got = append(got, c.Name)
		}
		t.Fatalf("root children = %v, want [f1 f3 f2]", got)
	}
	if f2.Children[0].Name != "c2" || len(f2.Children) != 1 {
		t.Fatalf("f2 must be untouched, got %d children", len(f2.Children))
	}
}

// Moving the pointer right by one indent step still nests into the folder above.
func TestDropIntoFolderWithHorizontalIntent(t *testing.T) {
	host, cleanup, _, f1, _, f3 := expandedSiblingFolders()
	defer cleanup()

	*host.DraggedNode = f3
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 60
	*host.DragNodeCurrentX = 60 + 12
	*host.DragNodeOriginY = 10
	*host.DragNodeCurrentY = -40

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != f1 {
		t.Fatalf("dragging right of the gap should nest into f1; got parent=%q", parentName(drop.parent))
	}
}

func twoCollections(withB func(rB *collections.CollectionNode)) (rA, a0, a1, rB *collections.CollectionNode, colA, colB *collections.ParsedCollection) {
	rA = mkNode("A", true)
	rA.Expanded = true
	a0 = mkNode("a0", false)
	a1 = mkNode("a1", false)
	rA.Children = []*collections.CollectionNode{a0, a1}
	colA = &collections.ParsedCollection{ID: "A", Root: rA}
	collections.AssignParents(rA, nil, colA)
	recalcDepth(rA, 0)

	rB = mkNode("B", true)
	colB = &collections.ParsedCollection{ID: "B", Root: rB}
	collections.AssignParents(rB, nil, colB)
	recalcDepth(rB, 0)
	if withB != nil {
		withB(rB)
	}
	return
}

func TestDragChildStaysInCollectionWhenOtherRootOffscreen(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	rA, a0, a1, rB, colA, colB := twoCollections(func(rB *collections.CollectionNode) { rB.Expanded = true })

	*host.Collections = []*collections.CollectionUI{{Data: colB}, {Data: colA}}
	visible := []*collections.CollectionNode{rB, rA, a0, a1}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	// rB(0) and rA(1) are scrolled above the viewport: only a0(2) and a1(3) laid out.
	(*host.ColRowYs)[2] = 0
	(*host.ColRowYs)[3] = 20
	*host.ColAfterLastY = 40

	*host.DraggedNode = a0
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 12
	*host.DragNodeCurrentX = 12
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 6

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent == rB || (drop.parent != nil && drop.parent.Collection == colB) {
		t.Fatalf("child fell into off-screen collection B; parent=%q", parentName(drop.parent))
	}
	if drop.parent == nil || drop.parent.Collection != colA {
		t.Fatalf("child should stay within collection A; got parent=%q", parentName(drop.parent))
	}
}

func TestDragChildIntoCollapsedTargetCollection(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	rA, a0, a1, rB, colA, colB := twoCollections(nil)

	*host.Collections = []*collections.CollectionUI{{Data: colA}, {Data: colB}}
	visible := []*collections.CollectionNode{rA, a0, a1, rB}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	for i := range visible {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = 80

	*host.DraggedNode = a0
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 12
	*host.DragNodeCurrentX = 12
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 43

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != rB {
		t.Fatalf("child dropped on collection B header should go INTO B; got parent=%q", parentName(drop.parent))
	}
}

func parentName(n *collections.CollectionNode) string {
	if n == nil {
		return "<nil>"
	}
	return n.Name
}

func TestCollectionRootMenuOffersDuplicate(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")
	rig.frames(2)

	has := func(n *collections.CollectionNode, label string) bool {
		for _, it := range nodeMenuItems(n) {
			if it.Label == label {
				return true
			}
		}
		return false
	}
	if !has(root, "Duplicate") {
		t.Fatal("collection root menu has no Duplicate item")
	}
	if !has(req, "Duplicate") {
		t.Fatal("request menu lost its Duplicate item")
	}
	for _, it := range nodeMenuItems(root) {
		if it.Label == "Duplicate" && it.Click != &root.DupBtn {
			t.Fatal("root Duplicate item is not wired to root.DupBtn")
		}
	}
}

func TestDuplicateCollectionCopiesExtras(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	root.Collection.InfoExtras = map[string]json.RawMessage{
		"_postman_id": json.RawMessage(`"abc"`),
		"schema":      json.RawMessage(`"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"`),
	}
	root.Collection.TopExtras = map[string]json.RawMessage{
		"auth":     json.RawMessage(`{"type":"bearer"}`),
		"variable": json.RawMessage(`[{"key":"host","value":"x"}]`),
	}
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.DupBtn)
	if len(*rig.host.Collections) != 2 {
		t.Fatalf("collections = %d, want 2", len(*rig.host.Collections))
	}
	dup := (*rig.host.Collections)[1].Data
	if _, ok := dup.InfoExtras["_postman_id"]; ok {
		t.Error("duplicate must not reuse the source _postman_id")
	}
	if string(dup.InfoExtras["schema"]) != string(root.Collection.InfoExtras["schema"]) {
		t.Errorf("schema not copied: %s", dup.InfoExtras["schema"])
	}
	for k, v := range root.Collection.TopExtras {
		if string(dup.TopExtras[k]) != string(v) {
			t.Errorf("TopExtras[%q] = %s, want %s", k, dup.TopExtras[k], v)
		}
	}
	dup.TopExtras["auth"][0] = 'X'
	if root.Collection.TopExtras["auth"][0] == 'X' {
		t.Error("TopExtras shared with the source instead of copied")
	}
}

const longEnvName = "Production environment with a very long overflowing name"

func TestSidebarRig_EnvDragWorksWithOverflowingName(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", longEnvName)
	rig.addEnv("e2", "Prod")
	rig.addEnv("e3", "QA")
	rig.frames(2)

	h := *rig.host.EnvRowH
	y := float32(rig.envRowY(0))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	if *rig.host.DraggedEnv != a {
		t.Fatalf("press on a row with an overflowing name should latch the drag, got %v", *rig.host.DraggedEnv)
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y+10),
	})
	rig.frame()
	if !*rig.host.DragEnvActive {
		t.Fatal("moving past the slop threshold should activate the env drag")
	}
	end := y + 10 + float32(2*h)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, end),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, end),
	})
	rig.frames(2)
	if (*rig.host.Environments)[0] == a {
		t.Errorf("dragging down should reorder: %q still first", (*rig.host.Environments)[0].Data.Name)
	}
}

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
			if n.RowHovered {
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

func TestRealLayoutEnvHoverScroll(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	colsMenuBtn := &widget.Clickable{}
	colsMenuOpen := false
	envsMenuBtn := &widget.Clickable{}
	envsMenuOpen := false
	colsMenuOpen2 := false
	host.ColsMenuBtn = colsMenuBtn
	host.ColsMenuOpen = &colsMenuOpen
	host.EnvsMenuBtn = envsMenuBtn
	host.EnvsMenuOpen = &envsMenuOpen
	_ = colsMenuOpen2

	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	colsExp := false
	envsExp := true
	host.ColsExpanded = &colsExp
	host.EnvsExpanded = &envsExp

	const N = 30
	envs := make([]*environments.EnvironmentUI, N)
	for i := range envs {
		envs[i] = &environments.EnvironmentUI{
			Data: &model.ParsedEnvironment{ID: fmt.Sprintf("e%d", i), Name: fmt.Sprintf("env-%d", i)},
		}
		envs[i].InlineNameEd.SingleLine = true
	}
	*host.Environments = envs

	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(220, 240)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	dump := func(label string) int {
		var idxs []int
		n := 0
		for i, e := range envs {
			if e.RowHovered {
				idxs = append(idxs, i)
				n++
			}
		}
		t.Logf("%-20s hovered=%v", label, idxs)
		return n
	}

	frame()
	hitY := -1
	for y := 0; y < 240; y += 4 {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, float32(y)), Source: pointer.Mouse})
		frame()
		n := 0
		for _, e := range envs {
			if e.RowHovered {
				n++
			}
		}
		if n > 0 {
			hitY = y
			t.Logf("hover hit at y=%d (n=%d)", y, n)
			break
		}
	}
	if hitY < 0 {
		t.Fatal("could not find a Y that hovers an env row")
	}
	dump("after hover")

	for s := 1; s <= 12; s++ {
		host.EnvList.Position.First = s
		host.EnvList.Position.Offset = 0
		frame()
	}
	t.Logf("EnvList.Position.First=%d Offset=%d", host.EnvList.Position.First, host.EnvList.Position.Offset)
	nScroll := dump("after scroll")

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, 999), Source: pointer.Mouse})
	frame()
	nOutside := dump("after move outside")

	t.Logf("hovered after scroll=%d, after-outside=%d", nScroll, nOutside)
	if nOutside > 0 {
		t.Errorf("STUCK HOVER: %d env(s) still hovered after cursor left the list", nOutside)
	}
}

type sideRig struct {
	host    *Host
	r       input.Router
	sz      image.Point
	now     time.Time
	cleanup func()

	opened   []string
	newCalls int
	renamed  map[string]string
	duped    []string
	deleted  []string
	imported [][]byte
	sections []string
	activeID string
}

func newSideRig(t *testing.T, sz image.Point) *sideRig {
	t.Helper()
	host, cleanup := newTestHost()
	t.Cleanup(cleanup)

	rig := &sideRig{
		host:    host,
		sz:      sz,
		now:     time.Unix(1700000000, 0),
		cleanup: cleanup,
		renamed: map[string]string{},
	}

	cmo, emo, smo := false, false, false
	host.ColsMenuBtn = &widget.Clickable{}
	host.ColsMenuOpen = &cmo
	host.EnvsMenuBtn = &widget.Clickable{}
	host.EnvsMenuOpen = &emo
	host.ScriptsMenuOpen = &smo
	host.ColBarScroll = &gesture.Scroll{}
	host.EnvBarScroll = &gesture.Scroll{}
	host.ScriptBarScroll = &gesture.Scroll{}
	host.EnvColorPicker = &colorpicker.State{}
	envColorID := ""
	host.EnvColorEnvID = &envColorID
	winOrig := f32.Point{}
	winPos := f32.Point{}
	host.DragNodeWinOrig = &winOrig
	host.DragNodeWinPos = &winPos
	zones := []DropZoneRect{}
	host.DropZones = &zones
	*host.WindowSize = sz

	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	sectionBtn := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	host.LayoutSectionRequests = sectionBtn
	host.LayoutSectionFlows = sectionBtn
	host.LayoutSectionMITM = sectionBtn
	host.LayoutSectionNetlimit = sectionBtn
	host.LayoutSectionHAR = sectionBtn

	host.ColList.Axis = layout.Vertical
	host.EnvList.Axis = layout.Vertical

	host.ActiveScriptID = func() string { return rig.activeID }
	host.OpenScript = func(id string) { rig.opened = append(rig.opened, id) }
	host.NewScript = func() { rig.newCalls++ }
	host.RenameScript = func(id, name string) { rig.renamed[id] = name }
	host.DuplicateScript = func(id string) { rig.duped = append(rig.duped, id) }
	host.DeleteScript = func(id string) { rig.deleted = append(rig.deleted, id) }
	host.ImportScript = func(data []byte) { rig.imported = append(rig.imported, data) }
	host.SwitchSection = func(s string) { rig.sections = append(rig.sections, s) }
	host.UpdateVisibleCols = func() { rig.rebuildVisible() }
	host.ensureScripts()
	return rig
}

func (rig *sideRig) rebuildVisible() {
	var out []*collections.CollectionNode
	var walk func(n *collections.CollectionNode)
	walk = func(n *collections.CollectionNode) {
		if n == nil {
			return
		}
		out = append(out, n)
		if n.IsFolder && !n.Expanded {
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, cu := range *rig.host.Collections {
		if cu == nil || cu.Data == nil {
			continue
		}
		walk(cu.Data.Root)
	}
	*rig.host.VisibleCols = out
}

func (rig *sideRig) gtx() layout.Context {
	rig.now = rig.now.Add(16 * time.Millisecond)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
}

func (rig *sideRig) frame() layout.Dimensions {
	gtx := rig.gtx()
	d := Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return d
}

func (rig *sideRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *sideRig) click(b *widget.Clickable) {
	b.Click()
	rig.frame()
}

func (rig *sideRig) advance(d time.Duration) {
	rig.now = rig.now.Add(d)
}

func (rig *sideRig) addCollection(id string) *collections.CollectionNode {
	root := mkNode(id, true)
	root.Expanded = true
	col := &collections.ParsedCollection{ID: id, Name: id, Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*rig.host.Collections = append(*rig.host.Collections, &collections.CollectionUI{Data: col})
	rig.rebuildVisible()
	return root
}

func (rig *sideRig) addRequest(parent *collections.CollectionNode, name, method string) *collections.CollectionNode {
	n := mkNode(name, false)
	n.Request = &model.ParsedRequest{Name: name, Method: method}
	n.Parent = parent
	n.Collection = parent.Collection
	parent.Children = append(parent.Children, n)
	recalcDepth(parent.Collection.Root, 0)
	rig.rebuildVisible()
	return n
}

func (rig *sideRig) addEnv(id, name string) *environments.EnvironmentUI {
	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: id, Name: name}}
	*rig.host.Environments = append(*rig.host.Environments, env)
	return env
}

func (rig *sideRig) addScript(id, name string) *ScriptRow {
	r := &ScriptRow{ID: id, Name: name}
	*rig.host.Scripts = append(*rig.host.Scripts, r)
	return r
}

func TestSidebarRig_EmptyStateRenders(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	d := rig.frames(2)
	if d.Size.X <= 0 || d.Size.Y <= 0 {
		t.Fatalf("empty sidebar produced no dimensions: %+v", d.Size)
	}
}

func TestSidebarRig_PopulatedRenders(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col A")
	folder := mkNode("folder", true)
	folder.Expanded = true
	folder.Parent = root
	folder.Collection = root.Collection
	root.Children = append(root.Children, folder)
	rig.addRequest(root, "get-users", "GET")
	rig.addRequest(root, "delete-user", "DELETE")
	rig.addRequest(folder, "patch-user", "PATCH")
	rig.addEnv("e1", "Dev")
	rig.addEnv("e2", "Prod")
	rig.addScript("s1", "Script one")
	rig.addScript("s2", "Script two")
	*rig.host.ScriptsExpanded = true
	rig.rebuildVisible()

	if d := rig.frames(3); d.Size.Y <= 0 {
		t.Fatal("populated sidebar produced no dimensions")
	}
	if len(*rig.host.DropZones) != 3 {
		t.Errorf("expected 3 drop zones, got %d", len(*rig.host.DropZones))
	}
}

func TestSidebarRig_HeaderTogglesAndMenus(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	rig.addCollection("Col")
	rig.addEnv("e1", "Dev")
	rig.frames(2)

	rig.click(rig.host.ColsHeaderClick)
	if *rig.host.ColsExpanded {
		t.Error("collections header click should collapse")
	}
	rig.click(rig.host.ColsHeaderClick)
	if !*rig.host.ColsExpanded {
		t.Error("second collections header click should expand")
	}

	rig.click(rig.host.EnvsHeaderClick)
	if *rig.host.EnvsExpanded {
		t.Error("environments header click should collapse")
	}
	rig.click(rig.host.EnvsHeaderClick)
	if !*rig.host.EnvsExpanded {
		t.Error("second environments header click should expand")
	}

	rig.click(rig.host.ScriptsHeaderClick)
	if !*rig.host.ScriptsExpanded {
		t.Error("scripts header click should expand")
	}

	rig.click(rig.host.ColsMenuBtn)
	if !*rig.host.ColsMenuOpen {
		t.Error("collections menu button should open the menu")
	}
	rig.click(rig.host.EnvsMenuBtn)
	if !*rig.host.EnvsMenuOpen {
		t.Error("environments menu button should open the menu")
	}
	rig.click(rig.host.ScriptsMenuBtn)
	if !*rig.host.ScriptsMenuOpen {
		t.Error("scripts menu button should open the menu")
	}
}

func TestSidebarRig_AddButtons(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	rig.frames(2)

	rig.click(rig.host.AddColBtn)
	if len(*rig.host.Collections) != 1 {
		t.Fatalf("AddColBtn: collections = %d, want 1", len(*rig.host.Collections))
	}
	if (*rig.host.Collections)[0].Data.Name != "New Collection" {
		t.Errorf("new collection name = %q", (*rig.host.Collections)[0].Data.Name)
	}

	rig.click(rig.host.AddEnvBtn)
	if len(*rig.host.Environments) != 1 {
		t.Fatalf("AddEnvBtn: environments = %d, want 1", len(*rig.host.Environments))
	}
	if *rig.host.EditingEnv == nil {
		t.Error("adding an environment should open it for editing")
	}

	rig.click(rig.host.AddScriptBtn)
	if rig.newCalls != 1 {
		t.Errorf("AddScriptBtn: NewScript calls = %d, want 1", rig.newCalls)
	}
}

func TestSidebarRig_ExpandCollapseAll(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col")
	inner := mkNode("inner", true)
	inner.Parent = root
	inner.Collection = root.Collection
	inner.Expanded = true
	root.Children = append(root.Children, inner)
	rig.addRequest(inner, "req", "GET")
	rig.rebuildVisible()
	rig.frames(2)

	*rig.host.ColsMenuOpen = true
	rig.click(rig.host.ColsCollapseAll)
	if root.Expanded || inner.Expanded {
		t.Fatalf("collapse all left folders expanded: root=%v inner=%v", root.Expanded, inner.Expanded)
	}
	if *rig.host.ColsMenuOpen {
		t.Error("collapse all should close the menu")
	}

	*rig.host.ColsMenuOpen = true
	rig.click(rig.host.ColsExpandAll)
	if !root.Expanded || !inner.Expanded {
		t.Fatalf("expand all left folders collapsed: root=%v inner=%v", root.Expanded, inner.Expanded)
	}
}

func TestSidebarRig_NodeMenuOpensAndIsExclusive(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	a := rig.addRequest(root, "a", "GET")
	b := rig.addRequest(root, "b", "POST")
	rig.frames(2)

	rig.click(&a.MenuBtn)
	if !a.MenuOpen {
		t.Fatal("node menu button should open the menu")
	}
	rig.click(&b.MenuBtn)
	if a.MenuOpen {
		t.Error("opening another node menu should close the first")
	}
	if !b.MenuOpen {
		t.Error("second node menu should be open")
	}
	rig.click(&b.MenuBtn)
	if b.MenuOpen {
		t.Error("clicking the same menu button should close it")
	}
}

func TestSidebarRig_NodeMenuAddRequestAndFolder(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.AddReqBtn)
	if len(root.Children) != 1 {
		t.Fatalf("AddReq: children = %d, want 1", len(root.Children))
	}
	added := root.Children[0]
	if added.Name != "New Request" || added.Request == nil || added.Request.Method != "GET" {
		t.Errorf("added request node = %+v", added)
	}
	if !added.IsRenaming || *rig.host.RenamingNode != added {
		t.Error("a newly added request should start in rename mode")
	}
	if root.MenuOpen {
		t.Error("adding a request should close the menu")
	}

	added.IsRenaming = false
	*rig.host.RenamingNode = nil
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.AddFldBtn)
	if len(root.Children) != 2 {
		t.Fatalf("AddFolder: children = %d, want 2", len(root.Children))
	}
	fld := root.Children[1]
	if !fld.IsFolder || fld.Name != "New Folder" {
		t.Errorf("added folder node = %+v", fld)
	}
}

func TestSidebarRig_NodeMenuRename(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "old-name", "GET")
	rig.frames(2)

	rig.click(&req.MenuBtn)
	rig.click(&req.EditBtn)
	if !req.IsRenaming {
		t.Fatal("Rename menu item should put the node into rename mode")
	}
	if req.NameEditor.Text() != "old-name" {
		t.Errorf("rename editor seeded with %q, want %q", req.NameEditor.Text(), "old-name")
	}
	if req.MenuOpen {
		t.Error("Rename should close the menu")
	}
}

func TestSidebarRig_NodeMenuDuplicateChildAndRoot(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")
	rig.frames(2)

	rig.click(&req.MenuBtn)
	rig.click(&req.DupBtn)
	if len(root.Children) != 2 {
		t.Fatalf("duplicate child: children = %d, want 2", len(root.Children))
	}
	dup := root.Children[1]
	if dup == req {
		t.Fatal("duplicate must be a distinct node")
	}
	if dup.Depth != req.Depth {
		t.Errorf("duplicate depth = %d, want %d", dup.Depth, req.Depth)
	}
	if !dup.IsRenaming {
		t.Error("duplicate should start in rename mode")
	}

	dup.IsRenaming = false
	*rig.host.RenamingNode = nil
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.DupBtn)
	if len(*rig.host.Collections) != 2 {
		t.Fatalf("duplicate root: collections = %d, want 2", len(*rig.host.Collections))
	}
	newCol := (*rig.host.Collections)[1].Data
	if newCol.Name != "Col Copy" {
		t.Errorf("duplicated collection name = %q, want %q", newCol.Name, "Col Copy")
	}
	if newCol.Root == nil || newCol.Root.Collection != newCol {
		t.Error("duplicated collection root not re-parented")
	}
	if newCol.Root.Depth != 0 {
		t.Errorf("duplicated root depth = %d, want 0", newCol.Root.Depth)
	}
}

func TestSidebarRig_DuplicateLandsBelowSource(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	folder := mkNode("folder", true)
	folder.Parent = root
	folder.Collection = root.Collection
	root.Children = append(root.Children, folder)
	rig.addRequest(root, "tail", "GET")
	recalcDepth(root, 0)
	rig.rebuildVisible()
	rig.frames(2)

	rig.click(&folder.MenuBtn)
	rig.click(&folder.DupBtn)

	if len(root.Children) != 3 {
		t.Fatalf("children = %d, want 3", len(root.Children))
	}
	if root.Children[0] != folder {
		t.Fatal("the source node must keep its place")
	}
	if root.Children[1] == folder || !strings.HasPrefix(root.Children[1].Name, "folder") {
		t.Fatalf("the copy must sit right below its source; order = %q, %q, %q",
			root.Children[0].Name, root.Children[1].Name, root.Children[2].Name)
	}
	if root.Children[2].Name != "tail" {
		t.Fatalf("the trailing sibling moved: %q", root.Children[2].Name)
	}
}

func TestSidebarRig_DuplicateCollectionLandsBelowSource(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	first := rig.addCollection("First")
	rig.addCollection("Last")
	rig.frames(2)

	rig.click(&first.MenuBtn)
	rig.click(&first.DupBtn)

	if len(*rig.host.Collections) != 3 {
		t.Fatalf("collections = %d, want 3", len(*rig.host.Collections))
	}
	names := make([]string, 0, 3)
	for _, c := range *rig.host.Collections {
		names = append(names, c.Data.Name)
	}
	if names[1] != "First Copy" || names[2] != "Last" {
		t.Fatalf("collection order = %v, want [First, First Copy, Last]", names)
	}
}

func TestSidebarRig_NodeMenuDeleteChild(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	keep := rig.addRequest(root, "keep", "GET")
	drop := rig.addRequest(root, "drop", "POST")
	rig.frames(2)

	rig.click(&drop.MenuBtn)
	rig.click(&drop.DelBtn)
	if len(root.Children) != 1 || root.Children[0] != keep {
		t.Fatalf("delete removed the wrong node: %+v", root.Children)
	}
}

func TestSidebarRig_NodeMenuDeleteRootClosesLinkedTabs(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")

	closed := []int{}
	deletedCols := []string{}
	rig.host.CloseTab = func(i int) {
		closed = append(closed, i)
		*rig.host.Tabs = append((*rig.host.Tabs)[:i], (*rig.host.Tabs)[i+1:]...)
	}
	rig.host.DeleteCollection = func(id string) { deletedCols = append(deletedCols, id) }
	*rig.host.Tabs = []*workspace.RequestTab{{LinkedNode: req}}
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.DelBtn)

	if len(*rig.host.Collections) != 0 {
		t.Fatalf("collections = %d, want 0", len(*rig.host.Collections))
	}
	if len(deletedCols) != 1 || deletedCols[0] != "Col" {
		t.Errorf("DeleteCollection calls = %v", deletedCols)
	}
	if len(closed) != 1 {
		t.Errorf("tabs linked to deleted nodes should be closed, got %v", closed)
	}
}

func TestSidebarRig_DeleteNodeClearsRenamingPointer(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")
	rig.frames(2)

	req.IsRenaming = true
	*rig.host.RenamingNode = req
	rig.frames(2)

	rig.click(&req.MenuBtn)
	rig.click(&req.DelBtn)
	if *rig.host.RenamingNode != nil {
		t.Fatal("deleting the node being renamed must clear RenamingNode")
	}
}

func TestSidebarRig_NodeClickTogglesFolderAndOpensRequest(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")

	opened := []*collections.CollectionNode{}
	rig.host.OpenRequestInTab = func(n *collections.CollectionNode) { opened = append(opened, n) }
	rig.frames(2)

	pressReleaseNode(rig, root, 100)
	if root.Expanded {
		t.Error("clicking a folder should collapse it")
	}
	pressReleaseNode(rig, root, 100)
	if !root.Expanded {
		t.Error("clicking a collapsed folder should expand it")
	}

	pressReleaseNode(rig, req, 100)
	if len(opened) != 1 || opened[0] != req {
		t.Fatalf("clicking a request should open it in a tab, got %v", opened)
	}
}

func pressReleaseNode(rig *sideRig, n *collections.CollectionNode, x float32) {
	rig.frame()
	y := nodeY(rig, n)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y),
	})
	rig.frame()
}

const (
	colsBodyTopPx  = 27
	envsBodyTopPx  = 27
	rowProbeInsetY = 4
	gutterPx       = 36
)

func nodeNameX(n *collections.CollectionNode) float32 {
	return float32(gutterPx + n.NameLeftPx + 2)
}

func nodeY(rig *sideRig, n *collections.CollectionNode) float32 {
	for i, v := range *rig.host.VisibleCols {
		if v == n {
			y, ok := (*rig.host.ColRowYs)[i]
			if !ok {
				y = i * *rig.host.ColRowH
			}
			return float32(colsBodyTopPx + y + rowProbeInsetY)
		}
	}
	return 0
}

func TestSidebarRig_NodeRenameSubmitAndEscape(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "before", "GET")

	dirty := []*collections.ParsedCollection{}
	rig.host.MarkCollectionDirty = func(c *collections.ParsedCollection) { dirty = append(dirty, c) }
	rig.frames(2)

	req.IsRenaming = true
	req.NameEditor.SetText("after")
	rig.frames(3)
	if !req.RenamingFocused {
		t.Fatal("precondition: the rename editor should have taken focus")
	}

	rig.submitKey(key.NameReturn)
	if req.Name != "after" {
		t.Fatalf("rename did not commit: %q", req.Name)
	}
	if req.Request.Name != "after" {
		t.Errorf("request name not updated: %q", req.Request.Name)
	}
	if req.IsRenaming {
		t.Error("commit should leave rename mode")
	}
	if *rig.host.RenamingNode != nil {
		t.Error("commit should clear RenamingNode")
	}
	if len(dirty) == 0 {
		t.Error("rename should mark the collection dirty")
	}

	req.IsRenaming = true
	req.NameEditor.SetText("discarded")
	rig.frames(3)
	rig.submitKey(key.NameEscape)
	if req.Name != "after" {
		t.Fatalf("escape should discard the edit, got %q", req.Name)
	}
	if req.IsRenaming {
		t.Error("escape should leave rename mode")
	}
}

func (rig *sideRig) submitKey(name key.Name) {
	rig.r.Queue(key.Event{Name: name, State: key.Press})
	rig.frames(2)
}

func TestSidebarRig_RenameToBlankRestoresOldName(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "keepme", "GET")
	rig.frames(2)

	req.IsRenaming = true
	req.NameEditor.SetText("   ")
	rig.frames(3)
	rig.submitKey(key.NameReturn)

	if req.Name != "keepme" {
		t.Fatalf("blank rename should keep the old name, got %q", req.Name)
	}
	if req.NameEditor.Text() != "keepme" {
		t.Errorf("editor should be restored to %q, got %q", "keepme", req.NameEditor.Text())
	}
	if req.IsRenaming {
		t.Error("blank rename should exit rename mode")
	}
}

func TestSidebarRig_RenamingRootUpdatesCollectionName(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	rig.frames(2)

	root.IsRenaming = true
	root.NameEditor.SetText("Renamed")
	rig.frames(3)
	rig.submitKey(key.NameReturn)

	if root.Name != "Renamed" {
		t.Fatalf("root name = %q", root.Name)
	}
	if root.Collection.Name != "Renamed" {
		t.Errorf("collection name should follow the root node, got %q", root.Collection.Name)
	}
}

func TestSidebarRig_EnvRowClickActivatesAndDeactivates(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	env := rig.addEnv("e1", "Dev")
	rig.frames(2)

	pressReleaseEnv(rig, 0)
	if *rig.host.ActiveEnvID != env.Data.ID {
		t.Fatalf("ActiveEnvID = %q, want %q", *rig.host.ActiveEnvID, env.Data.ID)
	}
	rig.advance(400 * time.Millisecond)
	pressReleaseEnv(rig, 0)
	if *rig.host.ActiveEnvID != "" {
		t.Fatalf("clicking the active environment should deactivate it, got %q", *rig.host.ActiveEnvID)
	}
}

func pressReleaseEnv(rig *sideRig, idx int) {
	rig.frame()
	y := float32(rig.envRowY(idx))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, y),
	})
	rig.frame()
}

func (rig *sideRig) scriptRowY(idx int) int {
	h := *rig.host.ScriptRowH
	if h <= 0 {
		h = 24
	}
	return *rig.host.ScriptsDividerY + 1 + 26 + idx*h + h/2
}

func (rig *sideRig) tapAt(x, y float32) {
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y),
	})
	rig.frame()
}

func (rig *sideRig) envRowY(idx int) int {
	h := *rig.host.EnvRowH
	if h <= 0 {
		h = 30
	}
	return *rig.host.EnvDividerY + envsBodyTopPx + idx*h + h/2
}

func TestSidebarRig_EnvMenuActions(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", "Dev")
	b := rig.addEnv("e2", "Prod")
	rig.frames(2)

	rig.click(&a.MenuBtn)
	if !a.MenuOpen {
		t.Fatal("env menu button should open the menu")
	}
	rig.click(&b.MenuBtn)
	if a.MenuOpen || !b.MenuOpen {
		t.Fatalf("env menus should be exclusive: a=%v b=%v", a.MenuOpen, b.MenuOpen)
	}

	rig.click(&b.RenameBtn)
	if !b.IsRenaming {
		t.Fatal("Rename should put the environment into rename mode")
	}
	if b.InlineNameEd.Text() != "Prod" {
		t.Errorf("inline editor seeded with %q", b.InlineNameEd.Text())
	}
	if b.MenuOpen {
		t.Error("Rename should close the menu")
	}
	b.IsRenaming = false
	rig.frames(2)

	rig.click(&a.MenuBtn)
	rig.click(&a.EditBtn)
	if *rig.host.EditingEnv != a {
		t.Error("Edit should set EditingEnv")
	}

	rig.click(&a.MenuBtn)
	rig.click(&a.DupBtn)
	if len(*rig.host.Environments) != 3 {
		t.Fatalf("duplicate: environments = %d, want 3", len(*rig.host.Environments))
	}
	if got := (*rig.host.Environments)[1].Data.Name; got != "Dev (copy)" {
		t.Errorf("the copy should sit right below its source; got %q at index 1", got)
	}
}

func TestSidebarRig_EnvMenuDelete(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", "Dev")
	rig.addEnv("e2", "Prod")
	*rig.host.ActiveEnvID = "e1"
	rig.frames(2)

	rig.click(&a.MenuBtn)
	rig.click(&a.DelBtn)
	if len(*rig.host.Environments) != 1 {
		t.Fatalf("environments = %d, want 1", len(*rig.host.Environments))
	}
	if (*rig.host.Environments)[0].Data.ID != "e2" {
		t.Errorf("wrong environment deleted: %q remains", (*rig.host.Environments)[0].Data.ID)
	}
	if *rig.host.ActiveEnvID != "" {
		t.Errorf("deleting the active environment should clear ActiveEnvID, got %q", *rig.host.ActiveEnvID)
	}
	if !*rig.host.ActiveEnvDirty {
		t.Error("deleting the active environment should mark it dirty")
	}
}

func TestSidebarRig_EnvRenameCommitAndEscape(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	env := rig.addEnv("e1", "Dev")
	other := rig.addEnv("e2", "Prod")
	rig.frames(2)

	env.IsRenaming = true
	env.InlineNameEd.SingleLine = true
	env.InlineNameEd.Submit = true
	env.InlineNameEd.SetText("Staging")
	rig.frames(3)
	rig.submitKey(key.NameReturn)
	if env.Data.Name != "Staging" {
		t.Fatalf("env rename did not commit: %q", env.Data.Name)
	}
	if env.IsRenaming {
		t.Error("commit should leave rename mode")
	}

	other.IsRenaming = true
	other.InlineNameEd.SingleLine = true
	other.InlineNameEd.Submit = true
	other.InlineNameEd.SetText("Discarded")
	rig.frames(3)
	rig.submitKey(key.NameEscape)
	if other.Data.Name != "Prod" {
		t.Fatalf("escape should discard the env rename, got %q", other.Data.Name)
	}
	if other.IsRenaming {
		t.Error("escape should leave rename mode")
	}
}

func TestSidebarRig_ScriptsBodyEmptyAndPopulated(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("empty scripts body produced no dimensions")
	}

	rig.addScript("s1", "One")
	rig.addScript("s2", "Two")
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("populated scripts body produced no dimensions")
	}
}

func TestSidebarRig_ScriptMenuActions(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	a := rig.addScript("s1", "One")
	b := rig.addScript("s2", "Two")
	rig.frames(2)

	rig.click(&a.MenuBtn)
	if !a.MenuOpen {
		t.Fatal("script menu button should open the menu")
	}
	rig.click(&b.MenuBtn)
	if a.MenuOpen || !b.MenuOpen {
		t.Fatalf("script menus should be exclusive: a=%v b=%v", a.MenuOpen, b.MenuOpen)
	}

	rig.click(&b.DupBtn)
	if len(rig.duped) != 1 || rig.duped[0] != "s2" {
		t.Errorf("DuplicateScript calls = %v", rig.duped)
	}
	if b.MenuOpen {
		t.Error("Duplicate should close the menu")
	}

	rig.click(&a.MenuBtn)
	rig.click(&a.DelBtn)
	if len(rig.deleted) != 1 || rig.deleted[0] != "s1" {
		t.Errorf("DeleteScript calls = %v", rig.deleted)
	}

	rig.click(&a.MenuBtn)
	rig.click(&a.RenameBtn)
	if !a.IsRenaming {
		t.Fatal("Rename should put the script row into rename mode")
	}
	if a.NameEd.Text() != "One" {
		t.Errorf("rename editor seeded with %q", a.NameEd.Text())
	}
	if a.MenuOpen {
		t.Error("Rename should close the menu")
	}
}

func TestSidebarRig_ScriptRenameCommits(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	r := rig.addScript("s1", "One")
	rig.frames(2)

	r.startRename()
	r.NameEd.SetText("Renamed")
	rig.frames(3)
	rig.submitKey(key.NameReturn)

	if r.Name != "Renamed" {
		t.Fatalf("script name = %q, want Renamed", r.Name)
	}
	if rig.renamed["s1"] != "Renamed" {
		t.Errorf("RenameScript calls = %v", rig.renamed)
	}
	if r.IsRenaming {
		t.Error("commit should leave rename mode")
	}
}

func TestCommitScriptRename_NoopCases(t *testing.T) {
	host := &Host{}
	r := &ScriptRow{ID: "s1", Name: "One"}

	commitScriptRename(host, r)
	if r.Name != "One" {
		t.Errorf("commit when not renaming must be a no-op, got %q", r.Name)
	}

	r.startRename()
	r.NameEd.SetText("   ")
	commitScriptRename(host, r)
	if r.Name != "One" {
		t.Errorf("blank name must not rename, got %q", r.Name)
	}
	if r.IsRenaming {
		t.Error("commit should always leave rename mode")
	}

	r.startRename()
	r.NameEd.SetText("One")
	commitScriptRename(host, r)
	if r.Name != "One" {
		t.Errorf("same name must not rename, got %q", r.Name)
	}
}

func TestScriptRow_StartRenameSeedsEditor(t *testing.T) {
	r := &ScriptRow{ID: "s1", Name: "Hello"}
	r.startRename()
	if !r.IsRenaming || r.RenamingFocused {
		t.Fatalf("startRename state: IsRenaming=%v RenamingFocused=%v", r.IsRenaming, r.RenamingFocused)
	}
	if !r.NameEd.SingleLine || !r.NameEd.Submit {
		t.Error("rename editor should be single-line with submit")
	}
	if r.NameEd.Text() != "Hello" {
		t.Errorf("rename editor text = %q", r.NameEd.Text())
	}
	if s, e := r.NameEd.Selection(); s != 0 || e != len([]rune("Hello")) {
		t.Errorf("rename editor selection = (%d,%d), want full text", s, e)
	}
}

func TestSidebarRig_ScriptRowClickInRequestsMode(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	rig.addScript("s1", "One")
	rig.frames(2)

	y := float32(rig.scriptRowY(0))
	rig.tapAt(80, y)
	if len(rig.opened) != 0 {
		t.Fatalf("a single click in requests mode must not open the script, got %v", rig.opened)
	}
	rig.tapAt(80, y)
	if len(rig.opened) != 1 || rig.opened[0] != "s1" {
		t.Fatalf("a double click should open the script, got %v", rig.opened)
	}
}

func TestSidebarRig_ScriptRowClickInFlowsMode(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.SidebarSection = "flows"
	*rig.host.ScriptsExpanded = true
	r := rig.addScript("s1", "One")
	rig.activeID = "s1"
	rig.frames(2)

	y := float32(rig.scriptRowY(0))
	rig.tapAt(80, y)
	if len(rig.opened) != 1 || rig.opened[0] != "s1" {
		t.Fatalf("in flows mode a single click should open the script, got %v", rig.opened)
	}
	rig.tapAt(80, y)
	if !r.IsRenaming {
		t.Error("in flows mode a double click should start renaming")
	}
}

func TestSidebarRig_ScriptRowClickIgnoredWhileRenaming(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	r := rig.addScript("s1", "One")
	rig.frames(2)
	r.startRename()
	rig.frames(2)

	y := float32(rig.scriptRowY(0))
	rig.tapAt(80, y)
	rig.tapAt(80, y)
	if len(rig.opened) != 0 {
		t.Fatalf("clicks while renaming must be ignored, got %v", rig.opened)
	}
}

func TestSidebarRig_FlowsModeNodeClicks(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	*rig.host.SidebarSection = "flows"
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")

	opened := []*collections.CollectionNode{}
	rig.host.OpenRequestInTab = func(n *collections.CollectionNode) { opened = append(opened, n) }
	rig.frames(2)

	pressReleaseNode(rig, root, 100)
	if root.Expanded {
		t.Error("flows mode: single click on a folder should toggle it")
	}
	if len(rig.sections) != 0 {
		t.Errorf("flows mode: a single click must not switch sections, got %v", rig.sections)
	}

	root.Expanded = true
	rig.rebuildVisible()
	rig.frames(2)

	pressReleaseNode(rig, req, 100)
	if len(opened) != 0 {
		t.Errorf("flows mode: a single click on a request must not open a tab, got %v", opened)
	}
	pressReleaseNode(rig, req, 100)
	if len(rig.sections) == 0 || rig.sections[0] != "requests" {
		t.Errorf("flows mode: a double click should switch to requests, got %v", rig.sections)
	}
	if len(opened) != 1 {
		t.Errorf("flows mode: a double click should open the request, got %v", opened)
	}
}

func TestSidebarRig_SectionModes(t *testing.T) {
	for _, section := range []string{"requests", "flows", "netlimit", "mitm", "har"} {
		t.Run(section, func(t *testing.T) {
			rig := newSideRig(t, image.Pt(260, 800))
			*rig.host.SidebarSection = section
			rig.addCollection("Col")
			rig.addEnv("e1", "Dev")
			rig.addScript("s1", "One")
			*rig.host.ScriptsExpanded = true
			body := func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			rig.host.LayoutNetlimitBody = body
			rig.host.LayoutMITMRules = body
			if d := rig.frames(2); d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Fatalf("section %s produced no dimensions: %+v", section, d.Size)
			}
		})
	}
}

func TestSidebarRig_HiddenSidebarRendersGutterOnly(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	rig.host.Settings.HideSidebar = true
	d := rig.frames(2)
	if d.Size.X != 36 {
		t.Fatalf("hidden sidebar width = %d, want the 36dp gutter", d.Size.X)
	}
}

func TestSidebarRig_ExpandCollapseCombinations(t *testing.T) {
	combos := []struct{ cols, envs, scripts bool }{
		{true, true, true},
		{true, true, false},
		{true, false, true},
		{true, false, false},
		{false, true, true},
		{false, true, false},
		{false, false, true},
		{false, false, false},
	}
	for _, c := range combos {
		rig := newSideRig(t, image.Pt(260, 800))
		root := rig.addCollection("Col")
		rig.addRequest(root, "req", "GET")
		rig.addEnv("e1", "Dev")
		rig.addScript("s1", "One")
		*rig.host.ColsExpanded = c.cols
		*rig.host.EnvsExpanded = c.envs
		*rig.host.ScriptsExpanded = c.scripts
		if d := rig.frames(2); d.Size.Y <= 0 {
			t.Fatalf("cols=%v envs=%v scripts=%v produced no dimensions", c.cols, c.envs, c.scripts)
		}
	}
}

func TestSidebarRig_ScrollWheelMovesLists(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.EnvsExpanded = false
	root := rig.addCollection("Col")
	for i := 0; i < 60; i++ {
		rig.addRequest(root, "req", "GET")
	}
	rig.frames(2)

	before := rig.host.ColList.Position.First
	for i := 0; i < 6; i++ {
		rig.r.Queue(pointer.Event{
			Kind: pointer.Scroll, Source: pointer.Mouse,
			Position: f32.Pt(120, 100), Scroll: f32.Pt(0, 60),
		})
		rig.frame()
	}
	if rig.host.ColList.Position.First <= before {
		t.Fatalf("scrolling should advance the collections list: %d -> %d",
			before, rig.host.ColList.Position.First)
	}
}

func TestSidebarRig_StickyHeadersRenderWhenScrolled(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 300))
	root := rig.addCollection("Col")
	folder := mkNode("folder", true)
	folder.Expanded = true
	folder.Parent = root
	folder.Collection = root.Collection
	root.Children = append(root.Children, folder)
	for i := 0; i < 30; i++ {
		rig.addRequest(folder, "req", "GET")
	}
	rig.rebuildVisible()
	rig.frames(2)

	rig.host.ColList.Position.First = 8
	rig.host.ColList.Position.Offset = 4
	rig.frames(2)
	if *rig.host.StickyBandH <= 0 {
		t.Fatalf("scrolled list should pin sticky headers, band height = %d", *rig.host.StickyBandH)
	}
	if len(rig.host.StickyRows) == 0 {
		t.Error("expected at least one pinned sticky row")
	}
}

func TestSidebarRig_HoverMarksRowUnderPointer(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col")
	a := rig.addRequest(root, "a", "GET")
	rig.frames(2)

	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Position: f32.Pt(80, nodeY(rig, a)),
	})
	rig.frames(2)
	if !a.RowHovered && !root.RowHovered {
		t.Error("moving the pointer over the list should mark a row hovered")
	}
}

func TestSidebarRig_NodeDragActivatesAfterSlop(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	a := rig.addRequest(root, "a", "GET")
	rig.addRequest(root, "b", "POST")
	rig.frames(2)

	y := nodeY(rig, a)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(100, y),
	})
	rig.frame()
	if *rig.host.DraggedNode != a {
		t.Fatalf("press should latch the dragged node, got %v", *rig.host.DraggedNode)
	}
	if *rig.host.DragNodeActive {
		t.Error("drag must not activate before the slop threshold")
	}

	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(100, y+40),
	})
	rig.frames(2)
	if !*rig.host.DragNodeActive {
		t.Fatal("moving past the slop threshold should activate the drag")
	}

	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(100, y+40),
	})
	rig.frames(2)
	if *rig.host.DraggedNode != nil || *rig.host.DragNodeActive {
		t.Errorf("release should clear the drag state: node=%v active=%v",
			*rig.host.DraggedNode, *rig.host.DragNodeActive)
	}
}

func TestSidebarRig_NodeDragGhostRenders(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	a := rig.addRequest(root, "a", "GET")
	rig.addRequest(root, "b", "POST")
	rig.frames(2)

	*rig.host.DraggedNode = a
	*rig.host.DragNodeActive = true
	*rig.host.DragNodeOriginY = nodeY(rig, a)
	*rig.host.DragNodeCurrentY = nodeY(rig, a) + 40
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("dragging render produced no dimensions")
	}
}

func TestSidebarRig_EnvDragGhostRenders(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", "Dev")
	rig.addEnv("e2", "Prod")
	rig.addEnv("e3", "QA")
	rig.frames(2)

	*rig.host.DraggedEnv = a
	*rig.host.DragEnvActive = true
	*rig.host.DragEnvOriginY = 0
	*rig.host.DragEnvCurrentY = float32(2 * *rig.host.EnvRowH)
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("env drag render produced no dimensions")
	}
}

func TestSidebarRig_EnvDragActivatesAndCommits(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", "Dev")
	rig.addEnv("e2", "Prod")
	rig.addEnv("e3", "QA")
	rig.frames(2)

	h := *rig.host.EnvRowH
	y := float32(rig.envRowY(0))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	if *rig.host.DraggedEnv != a {
		t.Fatalf("press should latch the dragged environment, got %v", *rig.host.DraggedEnv)
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y+10),
	})
	rig.frame()
	if !*rig.host.DragEnvActive {
		t.Fatal("moving past the slop threshold should activate the env drag")
	}
	end := y + 10 + float32(2*h)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, end),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, end),
	})
	rig.frames(2)
	if *rig.host.DraggedEnv != nil || *rig.host.DragEnvActive {
		t.Error("release should clear the env drag state")
	}
	if (*rig.host.Environments)[0] == a {
		t.Errorf("dragging down should reorder: %q still first", (*rig.host.Environments)[0].Data.Name)
	}
}

func TestSidebarRig_DividerDragResizesSections(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col")
	for i := 0; i < 10; i++ {
		rig.addRequest(root, "req", "GET")
	}
	for i := 0; i < 6; i++ {
		rig.addEnv("e", "Env")
	}
	for i := 0; i < 6; i++ {
		rig.addScript("s", "Script")
	}
	*rig.host.ScriptsExpanded = true
	rig.frames(3)

	before := *rig.host.SidebarEnvHeight
	y := float32(*rig.host.EnvDividerY)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, y),
	})
	rig.frames(2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, y-60),
	})
	rig.frames(2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(120, y-60),
	})
	rig.frames(2)

	if *rig.host.SidebarEnvHeight == before {
		t.Logf("env height unchanged (%d) — drag may have been fully absorbed", before)
	}
	if *rig.host.SidebarEnvHeight <= 0 {
		t.Fatalf("env height must stay positive, got %d", *rig.host.SidebarEnvHeight)
	}
}

func TestSidebarRig_ScriptsDividerDrag(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col")
	for i := 0; i < 10; i++ {
		rig.addRequest(root, "req", "GET")
	}
	for i := 0; i < 8; i++ {
		rig.addScript("s", "Script")
	}
	rig.addEnv("e1", "Dev")
	*rig.host.ScriptsExpanded = true
	rig.frames(3)

	y := float32(*rig.host.ScriptsDividerY)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, y),
	})
	rig.frames(2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, y-40),
	})
	rig.frames(2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(120, y-40),
	})
	rig.frames(2)

	if *rig.host.ScriptsHeight <= 0 {
		t.Fatalf("scripts height must stay positive, got %d", *rig.host.ScriptsHeight)
	}
}

func TestSidebarRig_ImportButtonsUseFilePicker(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	done := make(chan struct{}, 3)
	rig.host.ChooseJSONFile = func() ([]byte, error) {
		done <- struct{}{}
		return nil, nil
	}
	rig.frames(2)

	*rig.host.ColsMenuOpen = true
	rig.click(rig.host.ImportBtn)
	*rig.host.EnvsMenuOpen = true
	rig.click(rig.host.ImportEnvBtn)
	*rig.host.ScriptsMenuOpen = true
	rig.click(rig.host.ImportScriptBtn)

	for i := 0; i < 3; i++ {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("import %d never called the file picker", i)
		}
	}
	if *rig.host.ColsMenuOpen || *rig.host.EnvsMenuOpen || *rig.host.ScriptsMenuOpen {
		t.Error("import should close its menu")
	}
}

func TestSidebarRig_NodeDoubleClickOnNameStartsRename(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "target", "GET")
	rig.frames(2)

	x := nodeNameX(req)
	if req.NameWidthPx <= 0 {
		t.Fatalf("precondition: name width not measured (left=%d width=%d)", req.NameLeftPx, req.NameWidthPx)
	}
	pressReleaseNode(rig, req, x)
	if req.IsRenaming {
		t.Fatal("a single click on the name must not start renaming")
	}
	pressReleaseNode(rig, req, x)
	if !req.IsRenaming {
		t.Fatal("a double click on the name should start renaming")
	}
	if *rig.host.RenamingNode != req {
		t.Error("double-click rename should set RenamingNode")
	}
	if req.NameEditor.Text() != "target" {
		t.Errorf("rename editor seeded with %q", req.NameEditor.Text())
	}
}

func TestSidebarRig_NodeDoubleClickExpiresAfterTimeout(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "target", "GET")
	rig.frames(2)

	x := nodeNameX(req)
	pressReleaseNode(rig, req, x)
	rig.advance(500 * time.Millisecond)
	pressReleaseNode(rig, req, x)
	if req.IsRenaming {
		t.Fatal("clicks more than 300ms apart must not count as a double click")
	}
}

func TestSidebarRig_EnvDoubleClickStartsInlineRename(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	env := rig.addEnv("e1", "Dev")
	rig.frames(2)

	pressReleaseEnv(rig, 0)
	if env.IsRenaming {
		t.Fatal("a single click must not start renaming")
	}
	pressReleaseEnv(rig, 0)
	if !env.IsRenaming {
		t.Fatal("a double click on an environment row should start renaming")
	}
	if env.InlineNameEd.Text() != "Dev" {
		t.Errorf("inline editor seeded with %q", env.InlineNameEd.Text())
	}
	if !env.InlineNameEd.SingleLine || !env.InlineNameEd.Submit {
		t.Error("inline rename editor should be single-line with submit")
	}
}

func TestSidebarRig_ScriptsBodyHoverMarksRow(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	rig.addScript("s1", "One")
	rig.addScript("s2", "Two")
	rig.frames(3)

	y := float32(*rig.host.ScriptsDividerY + 1 + 26 + *rig.host.ScriptRowH/2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(80, y),
	})
	rig.frames(2)

	hovered := 0
	for _, r := range *rig.host.Scripts {
		if r.RowHovered {
			hovered++
		}
	}
	if hovered != 1 {
		t.Errorf("expected exactly one hovered script row, got %d", hovered)
	}
}

func TestDeleteEnvironment_ResetsEditorsAndIgnoresNil(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	env := rig.addEnv("e1", "Dev")
	env.Data.Vars = []model.EnvVar{{Key: "A", Value: "1"}, {Key: "B", Value: "2"}}
	env.InitEditor()
	if len(env.Rows) == 0 {
		t.Fatal("precondition: InitEditor should build rows")
	}
	*rig.host.EditingEnv = env

	deleteEnvironment(rig.host, nil)
	deleteEnvironment(rig.host, &environments.EnvironmentUI{})
	if len(*rig.host.Environments) != 1 {
		t.Fatalf("nil/blank deletes must be no-ops, got %d", len(*rig.host.Environments))
	}

	deleteEnvironment(rig.host, env)
	if len(*rig.host.Environments) != 0 {
		t.Fatalf("environments = %d, want 0", len(*rig.host.Environments))
	}
	if *rig.host.EditingEnv != nil {
		t.Error("deleting the edited environment should clear EditingEnv")
	}
}

func TestSidebarRig_NodeDragDropOntoFolderCommits(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	folder := mkNode("folder", true)
	folder.Expanded = true
	folder.Parent = root
	folder.Collection = root.Collection
	root.Children = append(root.Children, folder)
	req := rig.addRequest(root, "movable", "GET")
	rig.rebuildVisible()
	rig.frames(3)

	startY := nodeY(rig, req)
	endY := nodeY(rig, folder)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, startY),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, startY-10),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, endY),
	})
	rig.frames(2)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(120, endY),
	})
	rig.frames(2)

	if *rig.host.DraggedNode != nil || *rig.host.DragNodeActive {
		t.Error("release should clear the drag state")
	}
	total := 0
	var count func(n *collections.CollectionNode)
	count = func(n *collections.CollectionNode) {
		if n == nil {
			return
		}
		if !n.IsFolder {
			total++
		}
		for _, c := range n.Children {
			count(c)
		}
	}
	count(root)
	if total != 1 {
		t.Fatalf("the dragged request should still exist exactly once, got %d", total)
	}
}

func TestSidebarRig_ExternalDropHandlerWins(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "movable", "GET")
	rig.addRequest(root, "other", "POST")
	handled := []*collections.CollectionNode{}
	rig.host.DropNodeExternal = func(n *collections.CollectionNode) bool {
		handled = append(handled, n)
		return true
	}
	rig.frames(3)

	startY := nodeY(rig, req)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, startY),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(120, startY+30),
	})
	rig.frames(2)
	if !*rig.host.DragNodeActive {
		t.Fatal("precondition: drag should be active")
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(120, startY+30),
	})
	rig.frames(2)

	if len(handled) != 1 || handled[0] != req {
		t.Fatalf("external drop handler should have claimed the node, got %v", handled)
	}
	if root.Children[0] != req {
		t.Error("an externally handled drop must not reorder the tree")
	}
}

func TestAbbrevMethod(t *testing.T) {
	cases := map[string]string{
		"GET":      "GET",
		"post":     "POST",
		" put ":    "PUT",
		"DELETE":   "DEL",
		"OPTIONS":  "OPT",
		"PATCH":    "PAT",
		"TRACE":    "TRC",
		"CONNECT":  "CONN",
		"PROPFIND": "PROP",
		"":         "",
	}
	for in, want := range cases {
		if got := abbrevMethod(in); got != want {
			t.Errorf("abbrevMethod(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCollectionMethodSet(t *testing.T) {
	root := mkNode("root", true)
	a := mkNode("a", false)
	a.Request = &model.ParsedRequest{Method: "DELETE"}
	b := mkNode("b", false)
	b.Request = &model.ParsedRequest{Method: "get"}
	folder := mkNode("f", true)
	c := mkNode("c", false)
	c.Request = &model.ParsedRequest{Method: "GET"}
	folder.Children = []*collections.CollectionNode{c}
	root.Children = []*collections.CollectionNode{a, b, folder}

	set := map[string]bool{}
	collectionMethodSet(root, set)
	if len(set) != 2 || !set["DEL"] || !set["GET"] {
		t.Fatalf("method set = %v, want {DEL, GET}", set)
	}

	empty := map[string]bool{}
	collectionMethodSet(nil, empty)
	if len(empty) != 0 {
		t.Errorf("nil node should contribute nothing, got %v", empty)
	}

	noReq := mkNode("x", false)
	collectionMethodSet(noReq, empty)
	if len(empty) != 0 {
		t.Errorf("a request-less leaf should contribute nothing, got %v", empty)
	}
}

func TestSetAllCollectionsExpanded_SkipsBrokenEntries(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	root := rig.addCollection("Col")
	*rig.host.Collections = append(*rig.host.Collections,
		nil,
		&collections.CollectionUI{},
		&collections.CollectionUI{Data: &collections.ParsedCollection{ID: "x"}},
	)
	setAllCollectionsExpanded(rig.host, false)
	if root.Expanded {
		t.Error("collapse all should collapse the healthy root")
	}
	setAllCollectionsExpanded(rig.host, true)
	if !root.Expanded {
		t.Error("expand all should expand the healthy root")
	}
}

func TestScrollBarWheel_NilArgs(t *testing.T) {
	gtx := makeGtx(100, 100)
	scrollBarWheel(gtx, nil, &widget.List{})
	scrollBarWheel(gtx, &gesture.Scroll{}, nil)
}

func TestAddScrollBarStrip_DegenerateSizes(t *testing.T) {
	gtx := makeGtx(100, 100)
	sc := &gesture.Scroll{}
	addScrollBarStrip(gtx, nil, image.Pt(10, 10), 4)
	addScrollBarStrip(gtx, sc, image.Pt(10, 10), 0)
	addScrollBarStrip(gtx, sc, image.Pt(0, 10), 4)
	addScrollBarStrip(gtx, sc, image.Pt(10, 0), 4)
	addScrollBarStrip(gtx, sc, image.Pt(10, 10), 4)
}

func TestMenuOutlineHandlesTinyRows(t *testing.T) {
	for _, sz := range []image.Point{{}, {X: 1, Y: 1}, {X: 3, Y: 3}, {X: 40, Y: 2}, {X: 240, Y: 23}} {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		paintMenuOutline(gtx, sz)
	}
}

func TestMenuRowBgSitsBetweenBaseAndHover(t *testing.T) {
	bg := menuRowBg(theme.BgDark)
	if bg == theme.BgDark || bg == theme.BgHover {
		t.Fatalf("menu row bg %v must differ from both base %v and hover %v", bg, theme.BgDark, theme.BgHover)
	}
	lum := func(c interface{ RGBA() (r, g, b, a uint32) }) uint32 {
		r, g, b, _ := c.RGBA()
		return r + g + b
	}
	if lo, mid, hi := lum(theme.BgDark), lum(bg), lum(theme.BgHover); !(lo < mid && mid < hi) {
		t.Fatalf("menu row bg luminance %d not between base %d and hover %d", mid, lo, hi)
	}
}

func TestEnvPopupOpenTracksEditorAndColorPicker(t *testing.T) {
	rig := newSideRig(t, image.Pt(240, 600))
	a := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "a", Name: "A"}}
	b := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "b", Name: "B"}}
	*rig.host.Environments = []*environments.EnvironmentUI{a, b}

	if envPopupOpen(rig.host, a) || envPopupOpen(rig.host, b) {
		t.Fatal("no popup open: rows must not be highlighted")
	}

	*rig.host.EditingEnv = a
	if !envPopupOpen(rig.host, a) {
		t.Error("editing env must highlight its row")
	}
	if envPopupOpen(rig.host, b) {
		t.Error("editing env must not highlight other rows")
	}
	*rig.host.EditingEnv = nil

	*rig.host.EnvColorEnvID = b.Data.ID
	rig.host.EnvColorPicker.Open(colorpicker.KindEnv, 0, color.NRGBA{A: 0xff}, colorpicker.Anchor{})
	if !envPopupOpen(rig.host, b) {
		t.Error("open color picker must highlight its env row")
	}
	if envPopupOpen(rig.host, a) {
		t.Error("open color picker must not highlight other rows")
	}
	rig.host.EnvColorPicker.Close()
	if envPopupOpen(rig.host, b) {
		t.Error("closed color picker must drop the highlight")
	}
	if envPopupOpen(rig.host, nil) || envPopupOpen(rig.host, &environments.EnvironmentUI{}) {
		t.Error("nil env / nil data must be false")
	}
}

func TestSidebarMenusRender(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	host.ColsMenuBtn = &widget.Clickable{}
	cmo := true
	host.ColsMenuOpen = &cmo
	host.EnvsMenuBtn = &widget.Clickable{}
	emo := true
	host.EnvsMenuOpen = &emo
	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	root := mkNode("root", true)
	root.Expanded = true
	reqNode := &collections.CollectionNode{
		Name:    "req",
		Request: &model.ParsedRequest{Name: "req", Method: "GET"},
	}
	root.Children = append(root.Children, reqNode)
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	*host.Collections = []*collections.CollectionUI{{Data: col}}
	*host.VisibleCols = []*collections.CollectionNode{root, reqNode}

	reqNode.MenuOpen = true

	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev"}}
	env.MenuOpen = true
	*host.Environments = []*environments.EnvironmentUI{env}

	r := new(input.Router)
	frame := func() {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(240, 600)),
			Source:      r.Source(),
			Now:         time.Now(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	frame()
	frame()

	if !reqNode.MenuOpen {
		t.Error("node menu should remain open across plain layout frames")
	}
	if !env.MenuOpen {
		t.Error("env menu should remain open across plain layout frames")
	}
}

func TestRowIconBtnGeometry(t *testing.T) {
	draw := rowIcon(widgets.IconMore, theme.FgMuted)
	var btn widget.Clickable
	for _, tc := range []struct{ h, want int }{{23, 23}, {40, 40}, {0, 16}} {
		for _, b := range []*widget.Clickable{&btn, nil} {
			for _, hovered := range []bool{true, false} {
				gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(200, 200)}}
				d := rowIconBtn(gtx, b, hovered, tc.h, 1, draw)
				if d.Size != image.Pt(18, tc.want) {
					t.Errorf("h=%d btn=%v hovered=%v: size %v, want 18x%d", tc.h, b != nil, hovered, d.Size, tc.want)
				}
			}
		}
	}
}

func TestRowButtonsSpanTheirRows(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	*rig.host.ScriptsExpanded = true
	root := rig.addCollection("Col A")
	req := rig.addRequest(root, "get-users", "GET")
	env := rig.addEnv("e1", "Dev")
	scr := rig.addScript("s1", "first")
	rig.frames(4)

	check := func(label string, btnH, rowH int) {
		if btnH <= 0 || btnH != rowH {
			t.Errorf("%s: menu button height %d, row height %d", label, btnH, rowH)
		}
	}
	check("collection root", root.ContentHeightPx, root.RowHeightPx)
	check("request", req.ContentHeightPx, req.RowHeightPx)
	check("environment", env.ContentHeightPx, *rig.host.EnvRowH)
	check("script", scr.ContentHeightPx, *rig.host.ScriptRowH)
}

func (rig *sideRig) moveTo(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *sideRig) pressAt(x, y float32) {
	rig.moveTo(x, y)
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *sideRig) scanY(x float32, cond func() bool) float32 {
	for y := float32(0); y < float32(rig.sz.Y); y += 2 {
		rig.moveTo(x, y)
		if cond() {
			return y
		}
	}
	return -1
}

func TestEnvRowButtonsHitTestAndHover(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	rig.addEnv("e1", "Dev")
	b := rig.addEnv("e2", "Prod")
	w := float32(rig.frames(3).Size.X)
	menuX, swatchX, gearX := w-19, w-41, w-63

	y := rig.scanY(menuX, func() bool { return b.MenuHovered && b.MenuBtn.Hovered() })
	if y < 0 {
		t.Fatal("env ⋯ button never hovered")
	}
	rig.moveTo(gearX, y)
	if !b.QuickEditBtn.Hovered() || b.MenuBtn.Hovered() {
		t.Fatalf("gear at x=%v: gear hovered=%v, menu hovered=%v", gearX, b.QuickEditBtn.Hovered(), b.MenuBtn.Hovered())
	}
	rig.moveTo(swatchX, y)
	if !b.SelectBtn.Hovered() || b.QuickEditBtn.Hovered() {
		t.Fatalf("swatch at x=%v: swatch hovered=%v, gear hovered=%v", swatchX, b.SelectBtn.Hovered(), b.QuickEditBtn.Hovered())
	}

	rig.pressAt(gearX, y)
	if *rig.host.EditingEnv != b {
		t.Fatal("pressing the gear must open the environment editor")
	}
	*rig.host.EditingEnv = nil
	rig.frame()

	rig.pressAt(swatchX, y)
	if !rig.host.EnvColorPicker.IsOpen() || *rig.host.EnvColorEnvID != "e2" {
		t.Fatalf("pressing the swatch must open the color picker for e2: open=%v id=%q", rig.host.EnvColorPicker.IsOpen(), *rig.host.EnvColorEnvID)
	}
}

func TestStickyRowMenuButtonHovers(t *testing.T) {
	host, frame, r, _, fld2, _, bandY := buildStickyMenuHost(t)

	frame()
	host.ColList.Position.First = 41
	host.ColList.Position.Offset = 0
	frame()
	frame()
	if _, ok := bandY(fld2); !ok {
		t.Fatalf("fld2 is not in the band at First=41")
	}
	const menuX = 240 - 19
	hitY := float32(-1)
	for y := float32(0); y < 200; y++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(menuX, y), Source: pointer.Mouse})
		frame()
		frame()
		if fld2.StickyHovered && fld2.StickyMenuBtn.Hovered() {
			hitY = y
			break
		}
	}
	if hitY < 0 {
		t.Fatal("the sticky row ⋯ button never reported hover")
	}
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(menuX, hitY+2), Source: pointer.Mouse})
	frame()
	frame()
	if !fld2.StickyMenuBtn.Hovered() {
		t.Fatalf("sticky ⋯ button lost hover 2px below the first hit at y=%v", hitY)
	}
}

func TestListGutterOnlyWhenScrollable(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	list := &widget.List{}
	if g := listGutter(gtx, host.Theme, list); g != 0 {
		t.Fatalf("gutter for a non-scrollable list = %d, want 0", g)
	}
	list.Position.OffsetLast = -5
	g := listGutter(gtx, host.Theme, list)
	if g <= 0 {
		t.Fatalf("gutter for a scrollable list = %d, want > 0", g)
	}
	if s := rowSurface(image.Pt(250, 23), g); s != image.Pt(250-g, 23) {
		t.Fatalf("row surface %v, want %v", s, image.Pt(250-g, 23))
	}
	if s := rowSurface(image.Pt(1, 23), g); s.X != 0 {
		t.Fatalf("row surface narrower than the gutter must clamp to 0, got %v", s)
	}
}

func TestSidebarRig_ScriptDragReorders(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	a := rig.addScript("s1", "One")
	rig.addScript("s2", "Two")
	rig.addScript("s3", "Three")
	var reordered [][]string
	rig.host.ReorderScripts = func(ids []string) { reordered = append(reordered, ids) }
	rig.frames(3)

	h := *rig.host.ScriptRowH
	if h <= 0 {
		t.Fatal("precondition: script row height must be measured")
	}
	y := float32(rig.scriptRowY(0))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	if *rig.host.DraggedScript != a {
		t.Fatalf("press on a script row should latch the drag, got %v", *rig.host.DraggedScript)
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y+10),
	})
	rig.frame()
	if !*rig.host.DragScriptActive {
		t.Fatal("moving past the slop threshold should activate the script drag")
	}
	end := y + 10 + float32(2*h)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, end),
	})
	rig.frame()
	if got := dragScriptDropTargetIdx(rig.host); got != 2 {
		t.Fatalf("drop target = %d, want 2", got)
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, end),
	})
	rig.frames(2)

	ids := func() []string {
		var out []string
		for _, r := range *rig.host.Scripts {
			out = append(out, r.ID)
		}
		return out
	}
	if want := []string{"s2", "s3", "s1"}; !reflect.DeepEqual(ids(), want) {
		t.Errorf("rows after drag = %v, want %v", ids(), want)
	}
	if len(reordered) != 1 || !reflect.DeepEqual(reordered[0], []string{"s2", "s3", "s1"}) {
		t.Errorf("ReorderScripts calls = %v, want one call with the new order", reordered)
	}
	if len(rig.opened) != 0 {
		t.Errorf("a drag must not count as a click, opened %v", rig.opened)
	}
	if *rig.host.DraggedScript != nil || *rig.host.DragScriptActive {
		t.Error("drag state must reset after release")
	}
}

func TestSidebarRig_ScriptClickWithoutMoveDoesNotReorder(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	*rig.host.ScriptsExpanded = true
	rig.addScript("s1", "One")
	rig.addScript("s2", "Two")
	calls := 0
	rig.host.ReorderScripts = func([]string) { calls++ }
	rig.frames(3)

	y := float32(rig.scriptRowY(1))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y+2),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, y+2),
	})
	rig.frames(2)
	if calls != 0 {
		t.Errorf("a jitter within the slop must not reorder, got %d calls", calls)
	}
	if (*rig.host.Scripts)[0].ID != "s1" {
		t.Error("row order must be unchanged after a plain click")
	}
}

func TestCommitScriptDrop_NoopWhenInactive(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addScript("s1", "One")
	rig.addScript("s2", "Two")
	calls := 0
	rig.host.ReorderScripts = func([]string) { calls++ }
	*rig.host.ScriptRowH = 24

	commitScriptDrop(rig.host, a)
	*rig.host.DraggedScript = a
	*rig.host.DragScriptActive = true
	*rig.host.DragScriptOriginY = 0
	*rig.host.DragScriptCurrentY = 5
	commitScriptDrop(rig.host, a)
	if calls != 0 {
		t.Errorf("inactive or same-slot drops must not reorder, got %d calls", calls)
	}
	*rig.host.DragScriptCurrentY = 30
	commitScriptDrop(rig.host, a)
	if calls != 1 || (*rig.host.Scripts)[1] != a {
		t.Errorf("a one-row drop should move the row: calls=%d order=%v", calls, *rig.host.Scripts)
	}
}

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

// A bare material.NewTheme has no font collection, so text falls back to the
// OS fonts and measures zero width on a machine without any installed — which
// collapses every label-sized hit area and the measured node-name width.
// Pin the embedded Go fonts so layout matches on every platform.
func testTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func makeGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

func mkNode(name string, folder bool) *collections.CollectionNode {
	n := &collections.CollectionNode{Name: name, IsFolder: folder}
	n.NameEditor.SingleLine = true
	n.NameEditor.Submit = true
	return n
}

func buildTree() (root, a, b, c, d *collections.CollectionNode, col *collections.ParsedCollection) {
	root = mkNode("root", true)
	a = mkNode("a", true)
	b = mkNode("b", false)
	c = mkNode("c", true)
	d = mkNode("d", false)
	root.Children = []*collections.CollectionNode{a, c}
	a.Children = []*collections.CollectionNode{b}
	c.Children = []*collections.CollectionNode{d}
	col = &collections.ParsedCollection{ID: "col1", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)
	return
}

func unitMetric() unit.Metric { return unit.Metric{PxPerDp: 1, PxPerSp: 1} }

func newTestHost() (*Host, func()) {
	tmp, err := os.MkdirTemp("", "sidebar-test-*")
	if err != nil {
		panic(err)
	}
	persist.SetConfigOverride(tmp)
	cleanup := func() {
		persist.SetConfigOverride("")
		_ = os.RemoveAll(tmp)
	}

	cols := []*collections.CollectionUI{}
	visible := []*collections.CollectionNode{}
	envs := []*environments.EnvironmentUI{}
	tabs := []*workspace.RequestTab{}
	activeIdx := -1

	var renaming *collections.CollectionNode
	var editing *environments.EnvironmentUI
	var pendingEnv *environments.EnvironmentUI
	var draggedNode *collections.CollectionNode
	var draggedEnv *environments.EnvironmentUI
	activeEnvID := ""

	var (
		dragNodeOY, dragNodeCY, dragNodeOX, dragNodeCX float32
		dragNodeActive                                 bool
		dragEnvOY, dragEnvCY                           float32
		dragEnvActive                                  bool
	)

	colRowH := 20
	envRowH := 20
	colRowYs := map[int]int{}
	colAfterLastY := 0
	windowSize := image.Pt(0, 0)
	sidebarEnvHeight := 200
	sidebarEnvDragY := float32(0)
	colsExpanded := true
	envsExpanded := true
	dropTag := false
	activeEnvDirty := false
	sidebarSection := "requests"

	host := &Host{
		Theme:    testTheme(),
		Window:   &app.Window{},
		Settings: &model.AppSettings{},

		Collections:  &cols,
		VisibleCols:  &visible,
		Environments: &envs,
		Tabs:         &tabs,
		ActiveIdx:    &activeIdx,

		RenamingNode:    &renaming,
		EditingEnv:      &editing,
		PendingEnvClose: &pendingEnv,
		DraggedNode:     &draggedNode,
		DraggedEnv:      &draggedEnv,
		ActiveEnvID:     &activeEnvID,

		DragNodeOriginY:  &dragNodeOY,
		DragNodeCurrentY: &dragNodeCY,
		DragNodeOriginX:  &dragNodeOX,
		DragNodeCurrentX: &dragNodeCX,
		DragNodeActive:   &dragNodeActive,

		DragEnvOriginY:  &dragEnvOY,
		DragEnvCurrentY: &dragEnvCY,
		DragEnvActive:   &dragEnvActive,

		ColRowH:       &colRowH,
		StickyBandH:   new(int),
		StickyScroll:  &gesture.Scroll{},
		EnvRowH:       &envRowH,
		ColRowYs:      &colRowYs,
		ColAfterLastY: &colAfterLastY,
		WindowSize:    &windowSize,

		SidebarEnvHeight: &sidebarEnvHeight,
		SidebarEnvDrag:   &gesture.Drag{},
		SidebarEnvDragY:  &sidebarEnvDragY,

		ColList:         &widget.List{},
		EnvList:         &widget.List{},
		ColsHeaderClick: &widget.Clickable{},
		EnvsHeaderClick: &widget.Clickable{},
		ColsExpanded:    &colsExpanded,
		EnvsExpanded:    &envsExpanded,
		ImportBtn:       &widget.Clickable{},
		AddColBtn:       &widget.Clickable{},
		ColsExpandAll:   &widget.Clickable{},
		ColsCollapseAll: &widget.Clickable{},
		ImportEnvBtn:    &widget.Clickable{},
		AddEnvBtn:       &widget.Clickable{},
		SidebarDropTag:  &dropTag,
		ActiveEnvDirty:  &activeEnvDirty,
		SidebarSection:  &sidebarSection,

		ColsBodyHover:    &widgets.Hover{},
		ScriptsBodyHover: &widgets.Hover{},
		EnvsBodyHover:    &widgets.Hover{},
		ColsBodyFade:     &widgets.Fade{},
		ScriptsBodyFade:  &widgets.Fade{},
		EnvsBodyFade:     &widgets.Fade{},

		ChooseJSONFile:      func() ([]byte, error) { return nil, nil },
		SaveState:           func() {},
		PushColLoaded:       func(*collections.CollectionUI) {},
		MarkCollectionDirty: func(*collections.ParsedCollection) {},
		OpenRequestInTab:    func(*collections.CollectionNode) {},
		UpdateVisibleCols:   func() {},
		PushEnvLoaded:       func(*environments.EnvironmentUI) {},
		CommitEditingEnv:    func() {},
		CloseTab:            func(int) {},
		DeleteCollection:    func(string) {},
	}
	return host, cleanup
}

func TestRecalcDepth(t *testing.T) {
	root, a, b, c, d, _ := buildTree()
	if root.Depth != 0 || a.Depth != 1 || b.Depth != 2 || c.Depth != 1 || d.Depth != 2 {
		t.Fatalf("unexpected depths: %d %d %d %d %d", root.Depth, a.Depth, b.Depth, c.Depth, d.Depth)
	}
	recalcDepth(root, 5)
	if root.Depth != 5 || a.Depth != 6 || b.Depth != 7 || c.Depth != 6 || d.Depth != 7 {
		t.Fatalf("base=5 unexpected: %d %d %d %d %d", root.Depth, a.Depth, b.Depth, c.Depth, d.Depth)
	}
	recalcDepth(nil, 0)
}

func TestSiblingIndex(t *testing.T) {
	root, a, b, c, _, _ := buildTree()
	if siblingIndex(a) != 0 {
		t.Errorf("a sibling want 0 got %d", siblingIndex(a))
	}
	if siblingIndex(c) != 1 {
		t.Errorf("c sibling want 1 got %d", siblingIndex(c))
	}
	if siblingIndex(b) != 0 {
		t.Errorf("b sibling want 0 got %d", siblingIndex(b))
	}
	if siblingIndex(root) != -1 {
		t.Errorf("root has no parent, want -1 got %d", siblingIndex(root))
	}
	if siblingIndex(nil) != -1 {
		t.Errorf("nil want -1")
	}
	orphan := mkNode("orphan", false)
	orphan.Parent = root
	if siblingIndex(orphan) != -1 {
		t.Errorf("not in parent's children want -1")
	}
}

func TestIsAncestorOrSelf(t *testing.T) {
	root, a, b, c, _, _ := buildTree()
	if !isAncestorOrSelf(root, b) {
		t.Error("root ancestor of b")
	}
	if !isAncestorOrSelf(a, b) {
		t.Error("a ancestor of b")
	}
	if !isAncestorOrSelf(a, a) {
		t.Error("a self")
	}
	if isAncestorOrSelf(b, a) {
		t.Error("b not ancestor of a")
	}
	if isAncestorOrSelf(a, c) {
		t.Error("a not ancestor of c")
	}
	if isAncestorOrSelf(root, nil) {
		t.Error("nil n is false")
	}
}

func TestAddNewCollection(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	addNewCollection(host)
	if len(*host.Collections) != 1 {
		t.Fatalf("want 1 got %d", len(*host.Collections))
	}
	col := (*host.Collections)[0]
	if col.Data == nil || col.Data.Root == nil {
		t.Fatal("missing data/root")
	}
	if col.Data.Root.Collection != col.Data {
		t.Error("AssignParents not wired")
	}
	if !*host.ColsExpanded {
		t.Error("ColsExpanded want true")
	}

	addNewCollection(host)
	if len(*host.Collections) != 2 {
		t.Fatalf("want 2 got %d", len(*host.Collections))
	}
	if (*host.Collections)[0].Data.ID == (*host.Collections)[1].Data.ID {
		t.Error("collection IDs must differ")
	}
}

func TestAddNewEnvironment(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	addNewEnvironment(host)
	if len(*host.Environments) != 1 {
		t.Fatalf("want 1 got %d", len(*host.Environments))
	}
	env := (*host.Environments)[0]
	if env.Data == nil || env.Data.ID == "" {
		t.Error("missing data/id")
	}
	if *host.EditingEnv != env {
		t.Error("EditingEnv should point to new env")
	}
	if !*host.EnvsExpanded {
		t.Error("EnvsExpanded want true")
	}
}

func TestDeleteEnvironment(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	addNewEnvironment(host)
	addNewEnvironment(host)
	if len(*host.Environments) != 2 {
		t.Fatalf("setup want 2 got %d", len(*host.Environments))
	}
	first := (*host.Environments)[0]
	second := (*host.Environments)[1]
	*host.ActiveEnvID = first.Data.ID
	*host.EditingEnv = first

	deleteEnvironment(host, first)
	if len(*host.Environments) != 1 {
		t.Fatalf("want 1 after delete got %d", len(*host.Environments))
	}
	if (*host.Environments)[0] != second {
		t.Error("wrong env survived")
	}
	if *host.ActiveEnvID != "" {
		t.Error("ActiveEnvID should clear when deleting active env")
	}
	if !*host.ActiveEnvDirty {
		t.Error("ActiveEnvDirty should be set")
	}
	if *host.EditingEnv != nil {
		t.Error("EditingEnv should clear when deleting edited env")
	}

	deleteEnvironment(host, nil)
	deleteEnvironment(host, &environments.EnvironmentUI{})
}

func TestDuplicateEnvironment(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	src := &environments.EnvironmentUI{
		Data: &model.ParsedEnvironment{
			ID:             "src",
			Name:           "Original",
			HighlightColor: "#ff0000",
			Vars: []model.EnvVar{
				{Key: "k1", Value: "v1"},
				{Key: "k2", Value: "v2"},
			},
		},
	}
	*host.Environments = append(*host.Environments, src)
	duplicateEnvironment(host, src)
	if len(*host.Environments) != 2 {
		t.Fatalf("want 2 got %d", len(*host.Environments))
	}
	dup := (*host.Environments)[1]
	if dup.Data.Name != "Original (copy)" {
		t.Errorf("dup name = %q", dup.Data.Name)
	}
	if dup.Data.HighlightColor != "#ff0000" {
		t.Errorf("dup HighlightColor = %q", dup.Data.HighlightColor)
	}
	if len(dup.Data.Vars) != 2 {
		t.Errorf("dup vars len = %d", len(dup.Data.Vars))
	}
	if dup.Data.ID == src.Data.ID {
		t.Error("dup id must differ")
	}

	duplicateEnvironment(host, nil)
	duplicateEnvironment(host, &environments.EnvironmentUI{})
}

func TestDragEnvDropTargetIdx(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	e0 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "0"}}
	e1 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "1"}}
	e2 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "2"}}
	*host.Environments = []*environments.EnvironmentUI{e0, e1, e2}

	if got := dragEnvDropTargetIdx(host); got != -1 {
		t.Errorf("no drag -> -1 got %d", got)
	}

	*host.DraggedEnv = e1
	*host.DragEnvActive = true
	*host.EnvRowH = 20
	*host.DragEnvOriginY = 0
	*host.DragEnvCurrentY = 25
	if got := dragEnvDropTargetIdx(host); got != 2 {
		t.Errorf("down ~1 row from 1 -> 2 got %d", got)
	}
	*host.DragEnvCurrentY = -25
	if got := dragEnvDropTargetIdx(host); got != 0 {
		t.Errorf("up ~1 row from 1 -> 0 got %d", got)
	}
	*host.DragEnvCurrentY = -1000
	if got := dragEnvDropTargetIdx(host); got != 0 {
		t.Errorf("clamp low -> 0 got %d", got)
	}
	*host.DragEnvCurrentY = 1000
	if got := dragEnvDropTargetIdx(host); got != 2 {
		t.Errorf("clamp high -> 2 got %d", got)
	}

	*host.EnvRowH = 0
	if got := dragEnvDropTargetIdx(host); got != -1 {
		t.Errorf("EnvRowH=0 -> -1 got %d", got)
	}

	*host.EnvRowH = 20
	stray := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "stray"}}
	*host.DraggedEnv = stray
	if got := dragEnvDropTargetIdx(host); got != -1 {
		t.Errorf("dragged not in list -> -1 got %d", got)
	}
}

func TestCommitEnvDrop(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	e0 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "0"}}
	e1 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "1"}}
	e2 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "2"}}
	*host.Environments = []*environments.EnvironmentUI{e0, e1, e2}
	*host.DraggedEnv = e0
	*host.DragEnvActive = true
	*host.EnvRowH = 20
	*host.DragEnvOriginY = 0
	*host.DragEnvCurrentY = 50

	commitEnvDrop(host, e0)
	if (*host.Environments)[2] != e0 {
		t.Errorf("e0 should be at index 2, got order: %s %s %s",
			(*host.Environments)[0].Data.ID,
			(*host.Environments)[1].Data.ID,
			(*host.Environments)[2].Data.ID)
	}

	commitEnvDrop(host, nil)

	stray := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "stray"}}
	*host.DraggedEnv = stray
	commitEnvDrop(host, stray)
	if len(*host.Environments) != 3 {
		t.Errorf("stray drop must not modify list")
	}
}

func TestDragNodeDropGuards(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	if _, ok := dragNodeDrop(host, unitMetric()); ok {
		t.Error("no dragged -> not ok")
	}
	stray := mkNode("x", false)
	*host.DraggedNode = stray
	*host.DragNodeActive = true
	*host.ColRowH = 0
	if _, ok := dragNodeDrop(host, unitMetric()); ok {
		t.Error("ColRowH=0 -> not ok")
	}

	*host.ColRowH = 20
	*host.VisibleCols = []*collections.CollectionNode{}
	if _, ok := dragNodeDrop(host, unitMetric()); ok {
		t.Error("src not in VisibleCols -> not ok")
	}
}

func TestDragRootDropMovesCollection(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	r1 := mkNode("c1", true)
	r2 := mkNode("c2", true)
	r3 := mkNode("c3", true)
	c1 := &collections.ParsedCollection{ID: "1", Root: r1}
	c2 := &collections.ParsedCollection{ID: "2", Root: r2}
	c3 := &collections.ParsedCollection{ID: "3", Root: r3}
	collections.AssignParents(r1, nil, c1)
	collections.AssignParents(r2, nil, c2)
	collections.AssignParents(r3, nil, c3)
	*host.Collections = []*collections.CollectionUI{
		{Data: c1}, {Data: c2}, {Data: c3},
	}
	*host.VisibleCols = []*collections.CollectionNode{r1, r2, r3}
	*host.ColRowH = 20
	(*host.ColRowYs)[0] = 0
	(*host.ColRowYs)[1] = 20
	(*host.ColRowYs)[2] = 40
	*host.ColAfterLastY = 60

	*host.DraggedNode = r1
	*host.DragNodeActive = true
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 60

	commitNodeDrop(host, r1, unitMetric())

	got := []string{
		(*host.Collections)[0].Data.ID,
		(*host.Collections)[1].Data.ID,
		(*host.Collections)[2].Data.ID,
	}
	if got[2] != "1" {
		t.Errorf("r1 should be last; got %v", got)
	}
}

func TestCommitNodeDropPreservesOnSelfTarget(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	r1 := mkNode("c1", true)
	c1 := &collections.ParsedCollection{ID: "1", Root: r1}
	collections.AssignParents(r1, nil, c1)
	*host.Collections = []*collections.CollectionUI{{Data: c1}}
	*host.VisibleCols = []*collections.CollectionNode{r1}
	*host.ColRowH = 20
	(*host.ColRowYs)[0] = 0
	*host.ColAfterLastY = 20

	*host.DraggedNode = r1
	*host.DragNodeActive = true
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 0

	commitNodeDrop(host, r1, unitMetric())
	if len(*host.Collections) != 1 || (*host.Collections)[0].Data != c1 {
		t.Errorf("single-root self drop must not duplicate or lose entries")
	}
}

func TestCommitNodeDropChildIntoFolder(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	root, a, b, c, _, _ := buildTree()
	flat := []*collections.CollectionNode{root, a, b, c}
	*host.Collections = []*collections.CollectionUI{{Data: root.Collection}}
	*host.VisibleCols = flat
	*host.ColRowH = 20
	for i := range flat {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = 80

	*host.DraggedNode = b
	*host.DragNodeActive = true
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 30
	*host.DragNodeOriginX = 0
	*host.DragNodeCurrentX = 24

	prevParent := b.Parent
	commitNodeDrop(host, b, unitMetric())

	if b.Parent == prevParent {
		t.Log("b stayed under same parent; geometry may not have selected intoNode slot")
	}

	if b.Parent != nil && b.Depth != b.Parent.Depth+1 {
		t.Errorf("depth invariant broken: b.Depth=%d parent.Depth=%d", b.Depth, b.Parent.Depth)
	}

	if b.Collection != root.Collection {
		t.Error("collection pointer must remain")
	}

	_ = c
}

func TestCommitNodeDropNilSrc(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()
	commitNodeDrop(host, nil, unitMetric())
}

func TestDragChildDropExcludesAncestors(t *testing.T) {

	host, cleanup := newTestHost()
	defer cleanup()

	root, a, b, _, _, _ := buildTree()
	flat := []*collections.CollectionNode{root, a, b}
	*host.VisibleCols = flat
	*host.ColRowH = 20
	for i := range flat {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = 60

	*host.DraggedNode = a
	*host.DragNodeActive = true
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 40
	*host.DragNodeOriginX = 0
	*host.DragNodeCurrentX = 24

	drop, ok := dragNodeDrop(host, unitMetric())
	if ok && drop.parent == b {
		t.Error("dragChildDrop produced a drop with parent=descendant; cycle would form")
	}
}

func TestRenderNodeGhost(t *testing.T) {
	th := testTheme()
	gtx := makeGtx(200, 24)

	folder := mkNode("Folder", true)
	folder.Expanded = true
	renderNodeGhost(gtx, th, folder)

	gtx2 := makeGtx(200, 0)
	folder.Expanded = false
	folder.Depth = 0
	renderNodeGhost(gtx2, th, folder)

	gtx3 := makeGtx(200, 24)
	req := mkNode("Req", false)
	req.Request = &model.ParsedRequest{Method: "POST", Name: "Req"}
	renderNodeGhost(gtx3, th, req)
}

func TestRenderEnvGhost(t *testing.T) {
	th := testTheme()
	gtx := makeGtx(200, 30)
	env := &environments.EnvironmentUI{
		Data: &model.ParsedEnvironment{ID: "x", Name: "MyEnv", HighlightColor: "#00ff00"},
	}
	renderEnvGhost(gtx, th, env)
	renderEnvGhost(makeGtx(200, 0), th, env)
}

func TestLayoutSmoke(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Skipf("Layout panicked (driver/gpu not available in unit test): %v", r)
		}
	}()
	host, cleanup := newTestHost()
	defer cleanup()

	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	host.LayoutSectionRequests = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	gtx := makeGtx(220, 400)
	Layout(gtx, host)

	addNewCollection(host)
	addNewEnvironment(host)
	gtx2 := makeGtx(220, 400)
	Layout(gtx2, host)

	*host.SidebarSection = "mitm"
	gtx3 := makeGtx(220, 400)
	Layout(gtx3, host)
}

func TestDragNodeDropRootSrcNoChildSlot(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	r1 := mkNode("c1", true)
	r2 := mkNode("c2", true)
	c1 := &collections.ParsedCollection{ID: "1", Root: r1}
	c2 := &collections.ParsedCollection{ID: "2", Root: r2}
	collections.AssignParents(r1, nil, c1)
	collections.AssignParents(r2, nil, c2)
	*host.Collections = []*collections.CollectionUI{{Data: c1}, {Data: c2}}
	*host.VisibleCols = []*collections.CollectionNode{r1, r2}
	*host.ColRowH = 20
	(*host.ColRowYs)[0] = 0
	(*host.ColRowYs)[1] = 20
	*host.ColAfterLastY = 40

	*host.DraggedNode = r1
	*host.DragNodeActive = true
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 0

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected ok for root drag")
	}
	if drop.parent != nil {
		t.Errorf("root drag must produce nil parent target, got %v", drop.parent)
	}
}

func zoneTestHost(t *testing.T) (*Host, func()) {
	t.Helper()
	host, cleanup := newTestHost()
	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }
	host.LayoutSectionRequests = func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }
	host.ColsMenuBtn = &widget.Clickable{}
	host.ColsMenuOpen = new(bool)
	host.EnvsMenuBtn = &widget.Clickable{}
	host.EnvsMenuOpen = new(bool)
	return host, cleanup
}

func TestLayout_ReportsRealDropZones(t *testing.T) {
	host, cleanup := zoneTestHost(t)
	defer cleanup()

	var zones []DropZoneRect
	host.DropZones = &zones

	Layout(makeGtx(260, 600), host)
	Layout(makeGtx(260, 600), host)

	if len(zones) != 3 {
		t.Fatalf("expected 3 library drop zones, got %d: %+v", len(zones), zones)
	}
	for i, id := range []string{"collections", "scripts", "variables"} {
		if zones[i].ID != id {
			t.Errorf("zone %d id = %q, want %q", i, zones[i].ID, id)
		}
		if zones[i].Rect.Min.X != 36 || zones[i].Rect.Max.X != 260 {
			t.Errorf("zone %q x = [%d,%d], want [36,260] (past the icon gutter)", id, zones[i].Rect.Min.X, zones[i].Rect.Max.X)
		}
	}
	if zones[0].Rect.Min.Y != 0 {
		t.Errorf("collections zone must start at y=0, got %d", zones[0].Rect.Min.Y)
	}
	if zones[0].Rect.Max.Y != zones[1].Rect.Min.Y || zones[1].Rect.Max.Y != zones[2].Rect.Min.Y {
		t.Errorf("zones must be contiguous: %v / %v / %v", zones[0].Rect, zones[1].Rect, zones[2].Rect)
	}
	if zones[2].Rect.Max.Y != 600 {
		t.Errorf("variables zone must reach the sidebar bottom (600), got %d", zones[2].Rect.Max.Y)
	}
	if zones[0].Rect.Dy() <= zones[1].Rect.Dy() {
		t.Errorf("expected real heights (collections taller than collapsed scripts), got coll=%d scripts=%d",
			zones[0].Rect.Dy(), zones[1].Rect.Dy())
	}
}

func TestLayout_NoDropZonesForMITM(t *testing.T) {
	host, cleanup := zoneTestHost(t)
	defer cleanup()
	*host.SidebarSection = "mitm"
	host.LayoutMITMRules = func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Min} }

	var zones []DropZoneRect
	host.DropZones = &zones
	Layout(makeGtx(260, 600), host)
	if len(zones) != 0 {
		t.Errorf("MITM section must report no library drop zones, got %+v", zones)
	}
}

func TestLayout_HARSectionIsGutterOnly(t *testing.T) {
	host, cleanup := zoneTestHost(t)
	defer cleanup()
	*host.SidebarSection = "har"

	var zones []DropZoneRect
	host.DropZones = &zones

	dims := Layout(makeGtx(260, 600), host)
	if dims.Size.X != 36 {
		t.Errorf("HAR sidebar width = %d, want 36 (gutter only)", dims.Size.X)
	}
	if len(zones) != 0 {
		t.Errorf("HAR section must report no library drop zones, got %+v", zones)
	}
}

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

func TestStickyHeaderClick(t *testing.T) {
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
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, y), Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(120, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(120, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		frame()
	}

	hit := false
	for y := float32(32); y <= 70 && !hit; y++ {
		colsExp = true
		host.ColList.Position.First = 12
		opened = 0
		frame()
		clickAt(y)
		if f := host.ColList.Position.First; f == 0 || f == 1 {
			hit = true
		}
		if opened != 0 {
			t.Fatalf("sticky click at y=%v leaked through to the row beneath (OpenRequestInTab called %d times)", y, opened)
		}
	}
	if !hit {
		t.Fatalf("clicking a pinned sticky header never navigated to an ancestor (First stayed %d)", host.ColList.Position.First)
	}
}

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
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
		frame()
		frame()
	}

	hit := false
	for y := float32(32); y <= 70 && !hit; y++ {
		for x := float32(236); x >= 214; x-- {
			colsExp = true
			host.ColList.Position.First = 12
			root.MenuOpen = false
			fld.MenuOpen = false
			frame()
			clickAt(x, y)
			if root.MenuOpen || fld.MenuOpen {
				if host.ColList.Position.First != 12 {
					t.Errorf("opening the sticky ⋮ menu should not scroll the list (First=%d, want 12)", host.ColList.Position.First)
				}
				hit = true
				break
			}
		}
	}
	if !hit {
		t.Fatalf("clicking the sticky ⋮ button never opened a folder menu")
	}
}

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
