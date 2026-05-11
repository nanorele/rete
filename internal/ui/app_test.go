package ui

import (
	"tracto/internal/ui/workspace"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestAppUILayouts(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	ui.Tabs = nil
	tab := workspace.NewRequestTab("Test")
	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.layoutApp(gtx)
	ui.layoutContent(gtx)

	ui.TabBar.TabCtxMenuIdx = 0
	ui.closeTab(0)
	ui.layoutContent(gtx)

	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"), workspace.NewRequestTab("T2"))
	ui.ActiveIdx = 0

	keep := 0
	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		if i != keep {
			ui.closeTab(i)
		}
	}
	ui.layoutContent(gtx)

	ui.closeAllSidebarMenus()
}

func TestAppUIHelpers(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	env := &model.ParsedEnvironment{ID: "e1", Name: "E1", Vars: []model.EnvVar{{Key: "k", Value: "v", Enabled: true}}}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})
	ui.ActiveEnvID = "e1"
	ui.activeEnvDirty = true

	ui.refreshActiveEnv()
	if ui.activeEnvVars["k"] != "v" {
		t.Errorf("expected active env var k=v")
	}

	ui.Tabs = nil
	req := &model.ParsedRequest{
		Name: "Req",
		URL:  "http://example.com",
	}
	col := &collections.ParsedCollection{
		Root: &collections.CollectionNode{
			Request: req,
		},
	}
	col.Root.Collection = col

	ui.openRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab to be opened, got %d", len(ui.Tabs))
	}

	ui.openRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected still 1 tab, got %d", len(ui.Tabs))
	}
}

func TestFlushSaves(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.saveNeeded = true
	ui.flushSaveState()

	col := &collections.ParsedCollection{ID: "c1", Root: &collections.CollectionNode{}}
	ui.dirtyCollections["c1"] = &dirtyCollection{col: col}
	ui.flushCollectionSavesSync()
	if len(ui.dirtyCollections) != 0 {
		t.Errorf("dirty collections not cleared")
	}
}

func TestImportDroppedData(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	colJSON := `{"info": {"name": "Dropped Col"}, "item": [{"name":"req"}]}`
	ui.importDroppedData([]byte(colJSON))
	select {
	case c := <-ui.ColLoadedChan:
		if c.Data.Name != "Dropped Col" {
			t.Errorf("expected Dropped Col, got %s", c.Data.Name)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("collection not imported: timeout")
	}

	envJSON := `{"name": "Dropped Env", "values": [{"key":"k","value":"v"}]}`
	ui.importDroppedData([]byte(envJSON))
	select {
	case e := <-ui.EnvLoadedChan:
		if e.Data.Name != "Dropped Env" {
			t.Errorf("expected Dropped Env, got %s", e.Data.Name)
		}
	case c := <-ui.ColLoadedChan:
		t.Errorf("misparsed as collection: %s", c.Data.Name)
	case <-time.After(2 * time.Second):
		t.Errorf("environment not imported: timeout")
	}
}

func TestRevealLinkedNode(t *testing.T) {
	ui := NewAppUI()
	col := &collections.ParsedCollection{
		ID: "col1",
		Root: &collections.CollectionNode{
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{Name: "Target", Request: &model.ParsedRequest{}},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col

	tab := workspace.NewRequestTab("test")
	tab.LinkedNode = col.Root.Children[0]
	ui.Tabs = append(ui.Tabs, tab)

	ui.revealLinkedNode(tab)
	if !col.Root.Expanded {
		t.Errorf("expected parent folder to be expanded")
	}
}

func TestRelinkTabs(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	tab := workspace.NewRequestTab("test")
	tab.PendingColID = "col1"
	tab.PendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab)

	tab.LinkedNode = &collections.CollectionNode{}
	ui.relinkTabs()
	if tab.PendingColID != "col1" {
		t.Errorf("expected PendingColID to be preserved")
	}
	tab.LinkedNode = nil

	ui.relinkTabs()
	if tab.LinkedNode != nil {
		t.Errorf("expected nil link")
	}

	col := &collections.ParsedCollection{
		ID: "col1",
		Root: &collections.CollectionNode{
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{Name: "Target", Request: &model.ParsedRequest{}},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col
	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: col})

	ui.relinkTabs()
	if tab.LinkedNode == nil {
		t.Errorf("tab not relinked, PendingColID was %s", tab.PendingColID)
	} else if tab.LinkedNode.Name != "Target" {
		t.Errorf("relinked to wrong node: %s", tab.LinkedNode.Name)
	}

	tab2 := workspace.NewRequestTab("test2")
	tab2.PendingColID = "col1"
	tab2.PendingNodePath = []int{99}
	ui.Tabs = append(ui.Tabs, tab2)
	ui.relinkTabs()
	if tab2.LinkedNode != nil {
		t.Errorf("expected no link for invalid path")
	}

	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: &collections.ParsedCollection{ID: "col-nil-root"}})
	tab3 := workspace.NewRequestTab("test3")
	tab3.PendingColID = "col-nil-root"
	tab3.PendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab3)
	ui.relinkTabs()
	if tab3.LinkedNode != nil {
		t.Errorf("expected no link for nil root collection")
	}
}

func TestScheduleCollectionFlush(t *testing.T) {
	ui := NewAppUI()
	col := &collections.ParsedCollection{ID: "c1"}
	ui.markCollectionDirty(col)
	if _, ok := ui.dirtyCollections["c1"]; !ok {
		t.Errorf("collection not marked dirty")
	}
}

func TestBuildStateSnapshot(t *testing.T) {
	ui := NewAppUI()
	tab := workspace.NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.AddHeader("H1", "V1")
	tab.SplitRatio = 0.4
	tab.SaveToFilePath = "some/path"
	tab.LinkedNode = &collections.CollectionNode{
		Name:       "node1",
		Collection: &collections.ParsedCollection{ID: "col1"},
	}

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Children: []*collections.CollectionNode{tab.LinkedNode}}
	tab.LinkedNode.Parent = root
	tab.LinkedNode.Collection.Root = root

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 1
	ui.ActiveEnvID = "env1"

	snap := ui.buildStateSnapshot()
	if snap.ActiveEnvID != "env1" {
		t.Errorf("expected active env env1")
	}
	if len(snap.Tabs) < 2 {
		t.Errorf("expected at least 2 tabs")
	}

	lastTab := snap.Tabs[len(snap.Tabs)-1]
	if lastTab.Method != "POST" || lastTab.URL != "http://example.com" {
		t.Errorf("tab state not captured correctly")
	}
	if lastTab.CollectionID != "col1" {
		t.Errorf("linked collection not captured")
	}

	tab2 := workspace.NewRequestTab("unlinked")
	tab2.LinkedNode = &collections.CollectionNode{
		Collection: &collections.ParsedCollection{ID: "col2"},
	}

	ui.Tabs = append(ui.Tabs, tab2)
	snap2 := ui.buildStateSnapshot()
	lastTab2 := snap2.Tabs[len(snap2.Tabs)-1]
	if lastTab2.CollectionID != "col2" {
		t.Errorf("expected collection ID col2")
	}
	if len(lastTab2.NodePath) != 0 {
		t.Errorf("expected empty node path for orphaned node")
	}
}

func TestAppUIStateLoad(t *testing.T) {
	setupTestConfigDir(t)

	state := persist.AppState{
		ActiveIdx: 0,
		Tabs: []persist.TabState{
			{Title: "Saved Tab", Method: "GET", URL: "http://saved.com"},
		},
	}
	data, _ := json.Marshal(state)
	_ = os.MkdirAll(filepath.Dir(persist.StateFilePath()), 0755)
	_ = os.WriteFile(persist.StateFilePath(), data, 0644)

	ui := NewAppUI()
	if len(ui.Tabs) != 1 || ui.Tabs[0].Title != "Saved Tab" {
		t.Errorf("expected 1 tab loaded from state, got %d (title=%s)", len(ui.Tabs), ui.Tabs[0].Title)
	}
}

func TestAppUI_ExtraPaths(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.Tabs = nil
	gtx := layout.Context{Ops: new(op.Ops)}
	ui.layoutContent(gtx)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab auto-created")
	}

	ui.saveStateSync()

	ui.markCollectionDirty(nil)
}

func TestAppUIStateLoad_Corrupted(t *testing.T) {
	_ = setupTestConfigDir(t)
	_ = os.MkdirAll(filepath.Dir(persist.StateFilePath()), 0755)
	_ = os.WriteFile(persist.StateFilePath(), []byte("invalid json"), 0644)

	ui := NewAppUI()

	if len(ui.Tabs) != 1 {
		t.Errorf("expected fallback to default tab")
	}
}

func TestAppUIStateLoad_LegacyMonoFontRewrites(t *testing.T) {
	_ = setupTestConfigDir(t)
	_ = os.MkdirAll(filepath.Dir(persist.StateFilePath()), 0755)
	legacy := `{"tabs":[],"active_idx":0,"settings":{"theme":"dark","mono_font":"Ubuntu Mono","ui_text_size":14}}`
	_ = os.WriteFile(persist.StateFilePath(), []byte(legacy), 0644)

	ui := NewAppUI()
	if !ui.saveNeeded {
		t.Fatalf("expected saveNeeded=true after loading legacy state.json with mono_font")
	}

	ui.saveStateSync()
	rewritten, err := os.ReadFile(persist.StateFilePath())
	if err != nil {
		t.Fatalf("read after flush: %v", err)
	}
	if strings.Contains(string(rewritten), "mono_font") {
		t.Errorf("state.json still contains 'mono_font' after rewrite:\n%s", rewritten)
	}
}

func TestAppUIStateLoad_NilWrap(t *testing.T) {
	_ = setupTestConfigDir(t)
	state := persist.AppState{
		Tabs: []persist.TabState{
			{Title: "Nil Wrap", ReqWrapEnabled: nil},
		},
	}
	data, _ := json.Marshal(state)
	_ = os.MkdirAll(filepath.Dir(persist.StateFilePath()), 0755)
	_ = os.WriteFile(persist.StateFilePath(), data, 0644)

	ui := NewAppUI()
	if !ui.Tabs[0].ReqWrapEnabled {
		t.Errorf("expected default true for nil ReqWrapEnabled")
	}
}

func TestAppUI_AllLayoutPaths(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}

	ui.TabBar.TabCtxMenuOpen = true
	ui.VarPopup.Open = true
	ui.activeEnvDirty = true
	ui.saveNeeded = true

	ui.layoutApp(gtx)
	ui.layoutContent(gtx)

	widgets.GlobalVarHover = &widgets.VarHoverState{Name: "k", Pos: f32.Pt(10, 10)}
	ui.layoutApp(gtx)

	ui.VarPopup.Name = "k"
	ui.activeEnvVars = map[string]string{"k": "v"}
	ui.layoutApp(gtx)
}
