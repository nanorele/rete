package ui

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestAppUILayouts(t *testing.T) {
	setupTestConfigDir(t) // Reuse from state_test.go
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	// Set up some data to exercise loops
	ui.Tabs = nil // Clear default tab
	tab := NewRequestTab("Test")
	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 0

	col := &ParsedCollection{
		ID:   "col1",
		Name: "Collection 1",
		Root: &CollectionNode{
			Name:     "Root",
			IsFolder: true,
			Expanded: true,
			Children: []*CollectionNode{
				{Name: "Node1"},
			},
		},
	}
	col.Root.Children[0].Collection = col
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	ui.updateVisibleCols()

	env := &ParsedEnvironment{
		ID:   "env1",
		Name: "Env 1",
		Vars: []EnvVar{{Key: "k", Value: "v", Enabled: true}},
	}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.layoutApp(gtx)
	ui.layoutContent(gtx)

	// Test Popup closure
	ui.TabCtxMenuOpen = true
	ui.layoutApp(gtx)
	if !ui.TabCtxMenuOpen {
		t.Errorf("expected still open")
	}

	// Test GlobalVarHover
	GlobalVarHover = &VarHoverState{Name: "k", Pos: f32.Point{X: 10, Y: 10}}
	ui.activeEnvVars = map[string]string{"k": "v"}
	ui.layoutApp(gtx)

	// Test GlobalVarClick
	GlobalVarClick = &VarHoverState{Name: "k", Range: struct{ Start, End int }{0, 1}}
	ui.layoutApp(gtx)
	if !ui.VarPopupOpen {
		t.Errorf("expected var popup open")
	}
	if ui.VarPopupName != "k" {
		t.Errorf("expected var name k, got %s", ui.VarPopupName)
	}

	ui.closeAllSidebarMenus()
	
	ui.revealLinkedNode(tab)
	
	ui.markCollectionDirty(col)
	ui.flushCollectionSavesSync()
}

func TestAppUIHelpers(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win
	
	// Set up environment for refreshActiveEnv
	env := &ParsedEnvironment{ID: "e1", Name: "E1", Vars: []EnvVar{{Key: "k", Value: "v", Enabled: true}}}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})
	ui.ActiveEnvID = "e1"
	ui.activeEnvDirty = true
	
	ui.refreshActiveEnv()
	if ui.activeEnvVars["k"] != "v" {
		t.Errorf("expected active env var k=v")
	}

	ui.Tabs = nil // Clear default tab
	req := &ParsedRequest{
		Name: "Req",
		URL:  "http://example.com",
	}
	col := &ParsedCollection{
		Root: &CollectionNode{
			Request: req,
		},
	}
	col.Root.Collection = col

	ui.openRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab to be opened, got %d", len(ui.Tabs))
	}

	// Try opening again, should switch to it
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
	
	col := &ParsedCollection{ID: "c1", Root: &CollectionNode{}}
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
	
	// Test collection import
	colJSON := `{"info": {"name": "Dropped Col"}, "item": [{"name":"req"}]}`
	ui.importDroppedData([]byte(colJSON))
	select {
	case c := <-ui.ColLoadedChan:
		if c.Data.Name != "Dropped Col" {
			t.Errorf("expected Dropped Col, got %s", c.Data.Name)
		}
	default:
		t.Errorf("collection not imported")
	}
	
	envJSON := `{"name": "Dropped Env", "values": [{"key":"k","value":"v"}]}`
	ui.importDroppedData([]byte(envJSON))
	// It should fail collection parsing now and proceed to environment
	select {
	case e := <-ui.EnvLoadedChan:
		if e.Data.Name != "Dropped Env" {
			t.Errorf("expected Dropped Env, got %s", e.Data.Name)
		}
	default:
		// Check ColLoadedChan in case it was misparsed
		select {
		case c := <-ui.ColLoadedChan:
			t.Errorf("misparsed as collection: %s", c.Data.Name)
		default:
			t.Errorf("environment not imported")
		}
	}
}

func TestRelinkTabs(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	tab := NewRequestTab("test")
	tab.pendingColID = "col1"
	tab.pendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab)
	
	col := &ParsedCollection{
		ID: "col1",
		Root: &CollectionNode{
			IsFolder: true,
			Children: []*CollectionNode{
				{Name: "Target", Request: &ParsedRequest{}},
			},
		},
	}
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	
	ui.relinkTabs()
	if tab.LinkedNode == nil {
		t.Errorf("tab not relinked, pendingColID was %s", tab.pendingColID)
	} else if tab.LinkedNode.Name != "Target" {
		t.Errorf("relinked to wrong node: %s", tab.LinkedNode.Name)
	}
}

func TestBuildStateSnapshot(t *testing.T) {
	ui := NewAppUI()
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.addHeader("H1", "V1")
	tab.SplitRatio = 0.4
	tab.SaveToFilePath = "some/path"
	tab.LinkedNode = &CollectionNode{
		Name: "node1",
		Collection: &ParsedCollection{ID: "col1"},
	}
	// nodePathFrom needs parent links
	root := &CollectionNode{Name: "root", IsFolder: true, Children: []*CollectionNode{tab.LinkedNode}}
	tab.LinkedNode.Parent = root
	tab.LinkedNode.Collection.Root = root

	ui.Tabs = append(ui.Tabs, tab)
	ui.ActiveIdx = 1 // NewAppUI might add a default tab at 0
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
}
