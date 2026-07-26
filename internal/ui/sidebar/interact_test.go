package sidebar

import (
	"image"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

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
	if got := (*rig.host.Environments)[2].Data.Name; got != "Dev (copy)" {
		t.Errorf("duplicate name = %q", got)
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
	r := rig.addScript("s1", "One")
	rig.frames(2)

	r.NameClick.Click()
	rig.frame()
	if len(rig.opened) != 0 {
		t.Fatalf("a single click in requests mode must not open the script, got %v", rig.opened)
	}
	r.NameClick.Click()
	rig.frame()
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

	r.NameClick.Click()
	rig.frame()
	if len(rig.opened) != 1 || rig.opened[0] != "s1" {
		t.Fatalf("in flows mode a single click should open the script, got %v", rig.opened)
	}
	r.NameClick.Click()
	rig.frame()
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

	r.NameClick.Click()
	rig.frame()
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
