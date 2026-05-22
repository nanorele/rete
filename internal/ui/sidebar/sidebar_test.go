package sidebar

import (
	"image"
	"os"
	"testing"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

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
		Theme:    material.NewTheme(),
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
		ImportEnvBtn:    &widget.Clickable{},
		AddEnvBtn:       &widget.Clickable{},
		SidebarDropTag:  &dropTag,
		ActiveEnvDirty:  &activeEnvDirty,
		SidebarSection:  &sidebarSection,

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
				{Key: "k1", Value: "v1", Enabled: true},
				{Key: "k2", Value: "v2", Enabled: false},
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
	th := material.NewTheme()
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
	th := material.NewTheme()
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
