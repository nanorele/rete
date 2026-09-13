package apptest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/image/math/fixed"
	"image"
	"math/rand"
	"os"
	"path/filepath"
	"rete/internal/model"
	"rete/internal/persist"
	. "rete/internal/ui"
	"rete/internal/ui/collections"
	"rete/internal/ui/environments"
	"rete/internal/ui/flow"
	harui "rete/internal/ui/har"
	"rete/internal/ui/mitm"
	"rete/internal/ui/sidebar"
	"rete/internal/ui/widgets"
	"rete/internal/ui/workspace"
	"rete/pkg/fontsubset"
	"rete/pkg/syntax"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
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

	ui.LayoutApp(gtx)
	ui.LayoutContent(gtx)

	ui.TabBar.TabCtxMenuIdx = 0
	ui.CloseTab(0)
	ui.LayoutContent(gtx)

	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"), workspace.NewRequestTab("T2"))
	ui.ActiveIdx = 0

	keep := 0
	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		if i != keep {
			ui.CloseTab(i)
		}
	}
	ui.LayoutContent(gtx)

	ui.CloseAllSidebarMenus()
}

func TestAppUIHelpers(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	env := &model.ParsedEnvironment{ID: "e1", Name: "E1", Vars: []model.EnvVar{{Key: "k", Value: "v"}}}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})
	ui.ActiveEnvID = "e1"
	ui.SetActiveEnvDirty(true)

	ui.RefreshActiveEnv()
	if ui.ActiveEnvVarsMap()["k"] != "v" {
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

	ui.OpenRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab to be opened, got %d", len(ui.Tabs))
	}

	ui.OpenRequestInTab(col.Root)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected still 1 tab, got %d", len(ui.Tabs))
	}
}

func TestFlushSaves(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.SetSaveNeeded(true)
	ui.FlushSaveState()
	ui.WaitBackgroundSaves()

	col := &collections.ParsedCollection{ID: "c1", Root: &collections.CollectionNode{}}
	ui.MarkCollectionDirty(col)
	ui.FlushCollectionSavesSync()
	if len(ui.DirtyCollectionsMap()) != 0 {
		t.Errorf("dirty collections not cleared")
	}
}

func TestImportDroppedData(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	colJSON := `{"info": {"name": "Dropped Col"}, "item": [{"name":"req"}]}`
	ui.ImportDroppedData([]byte(colJSON))
	select {
	case c := <-ui.ColLoadedChan:
		if c.Data.Name != "Dropped Col" {
			t.Errorf("expected Dropped Col, got %s", c.Data.Name)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("collection not imported: timeout")
	}

	envJSON := `{"name": "Dropped Env", "values": [{"key":"k","value":"v"}]}`
	ui.ImportDroppedData([]byte(envJSON))
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
	wrap := tab
	ui.Tabs = append(ui.Tabs, wrap)

	ui.RevealLinkedNode(wrap)
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
	ui.RelinkTabs()
	if tab.PendingColID != "col1" {
		t.Errorf("expected PendingColID to be preserved")
	}
	tab.LinkedNode = nil

	ui.RelinkTabs()
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

	ui.RelinkTabs()
	if tab.LinkedNode == nil {
		t.Errorf("tab not relinked, PendingColID was %s", tab.PendingColID)
	} else if tab.LinkedNode.Name != "Target" {
		t.Errorf("relinked to wrong node: %s", tab.LinkedNode.Name)
	}

	tab2 := workspace.NewRequestTab("test2")
	tab2.PendingColID = "col1"
	tab2.PendingNodePath = []int{99}
	ui.Tabs = append(ui.Tabs, tab2)
	ui.RelinkTabs()
	if tab2.LinkedNode != nil {
		t.Errorf("expected no link for invalid path")
	}

	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: &collections.ParsedCollection{ID: "col-nil-root"}})
	tab3 := workspace.NewRequestTab("test3")
	tab3.PendingColID = "col-nil-root"
	tab3.PendingNodePath = []int{0}
	ui.Tabs = append(ui.Tabs, tab3)
	ui.RelinkTabs()
	if tab3.LinkedNode != nil {
		t.Errorf("expected no link for nil root collection")
	}
}

func TestScheduleCollectionFlush(t *testing.T) {
	ui := NewAppUI()
	col := &collections.ParsedCollection{ID: "c1"}
	ui.MarkCollectionDirty(col)
	if _, ok := ui.DirtyCollectionsMap()["c1"]; !ok {
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

	snap := ui.BuildStateSnapshot()
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
	snap2 := ui.BuildStateSnapshot()
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
	ui.LayoutContent(gtx)
	if len(ui.Tabs) != 1 {
		t.Errorf("expected 1 tab auto-created")
	}

	ui.SaveStateSync()

	ui.MarkCollectionDirty(nil)
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
	if !ui.SaveNeeded() {
		t.Fatalf("expected saveNeeded=true after loading legacy state.json with mono_font")
	}

	ui.SaveStateSync()
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
	ui.SetActiveEnvDirty(true)
	ui.SetSaveNeeded(true)

	ui.LayoutApp(gtx)
	ui.LayoutContent(gtx)

	widgets.GlobalVarHover = &widgets.VarHoverState{Name: "k", Pos: f32.Pt(10, 10)}
	ui.LayoutApp(gtx)

	ui.VarPopup.Name = "k"
	ui.SetActiveEnvVars(map[string]string{"k": "v"})
	ui.LayoutApp(gtx)
}

func filtersContainKey(ui *AppUI, name key.Name) bool {
	for _, f := range ui.ContentKeyFilters() {
		if kf, ok := f.(key.Filter); ok && kf.Name == name && kf.Required == key.ModShortcut {
			return true
		}
	}
	return false
}

func undoFiltersContainKey(ui *AppUI, name key.Name) bool {
	for _, f := range ui.FlowUndoFilters() {
		if kf, ok := f.(key.Filter); ok && kf.Name == name && kf.Required == key.ModShortcut {
			return true
		}
	}
	return false
}

func TestContentKeyFiltersUndoOnlyInFlows(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	for _, section := range []string{"collections", "flows"} {
		ui.SidebarSection = section
		if filtersContainKey(ui, "Z") || filtersContainKey(ui, "Y") {
			t.Errorf("%s: Ctrl+Z/Y must never be drained before the content (it would steal undo from focused editors)", section)
		}
		for _, n := range []key.Name{"S", "W", "F", key.NameReturn} {
			if !filtersContainKey(ui, n) {
				t.Errorf("%s: Ctrl+%s must remain a window-level filter", section, string(n))
			}
		}
	}

	ui.SidebarSection = "collections"
	if undoFiltersContainKey(ui, "Z") || undoFiltersContainKey(ui, "Y") {
		t.Error("the scenario undo fallback must exist only in the flows section")
	}
	ui.SidebarSection = "flows"
	if !undoFiltersContainKey(ui, "Z") || !undoFiltersContainKey(ui, "Y") {
		t.Error("the flows section must keep a Ctrl+Z/Y fallback for the scenario")
	}
}

type flowKeysRig struct {
	t  *testing.T
	ui *AppUI
	r  input.Router
}

func newFlowKeysRig(t *testing.T) *flowKeysRig {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "flows"
	rig := &flowKeysRig{t: t, ui: ui}
	rig.frame(nil)
	rig.frame(nil)
	return rig
}

func (rig *flowKeysRig) frame(before func(gtx layout.Context)) {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 800)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	if before != nil {
		before(gtx)
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(gtx.Ops)
}

func (rig *flowKeysRig) focus(tag any) {
	rig.frame(func(gtx layout.Context) { gtx.Execute(key.FocusCmd{Tag: tag}) })
	rig.frame(nil)
	if !rig.r.Source().Focused(tag) {
		rig.t.Fatal("setup: target did not take focus")
	}
}

func (rig *flowKeysRig) press(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press}, key.Event{Name: name, Modifiers: mods, State: key.Release})
	rig.frame(nil)
	rig.frame(nil)
}

func TestFlowBodyEditorOwnsUndoInsideTheApp(t *testing.T) {
	rig := newFlowKeysRig(t)
	ed := rig.ui.Flow
	n := flow.NewNode(flow.KindRequest, 120, 120)
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	rig.frame(nil)
	rig.frame(nil)
	nodes := len(ed.Scenario.Nodes)

	body := n.CanvasBodyEditor()
	rig.focus(body)
	rig.r.Queue(key.EditEvent{Text: "hello"})
	rig.frame(nil)
	rig.frame(nil)
	if got := body.Text(); got != "hello" {
		t.Fatalf("setup: typing must reach the body, got %q", got)
	}

	rig.press("Z", key.ModShortcut)
	if got := body.Text(); got != "" {
		t.Errorf("Ctrl+Z with the node body focused must undo the text, got %q", got)
	}
	if len(ed.Scenario.Nodes) != nodes {
		t.Errorf("Ctrl+Z with the node body focused must not undo the scenario, nodes = %d", len(ed.Scenario.Nodes))
	}
	rig.press("Y", key.ModShortcut)
	if got := body.Text(); got != "hello" {
		t.Errorf("Ctrl+Y with the node body focused must redo the text, got %q", got)
	}

	rig.frame(func(gtx layout.Context) { gtx.Execute(key.FocusCmd{Tag: nil}) })
	rig.frame(nil)
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, flow.NewNode(flow.KindDelay, 500, 120))
	rig.frame(nil)
	if rig.r.Source().Focused(body) {
		t.Fatal("setup: body must be blurred")
	}
	ed.PushHistory()
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, flow.NewNode(flow.KindDelay, 700, 120))
	before := len(ed.Scenario.Nodes)
	rig.press("Z", key.ModShortcut)
	if len(ed.Scenario.Nodes) != before-1 {
		t.Errorf("Ctrl+Z with nothing focused must fall back to the scenario undo, nodes = %d", len(ed.Scenario.Nodes))
	}
	if got := body.Text(); got != "hello" {
		t.Errorf("the scenario undo must not touch the body text, got %q", got)
	}
}

// savedTab writes the app state the way a real shutdown would and reads back the
// single tab it persisted.
func savedTab(t *testing.T, ui *AppUI) persist.TabState {
	t.Helper()
	if !ui.SaveNeeded() {
		t.Fatal("toggling a pane did not schedule a state save")
	}
	ui.SaveStateSync()
	ui.WaitBackgroundSaves()
	state := persist.Load()
	if len(state.Tabs) != 1 {
		t.Fatalf("persisted tabs = %d, want 1", len(state.Tabs))
	}
	return state.Tabs[0]
}

func TestHTTPPaneCollapsePersistsInBothLayouts(t *testing.T) {
	for _, lm := range []struct {
		name string
		mode int
	}{
		{"vertical", workspace.LayoutModeVert},
		{"horizontal", workspace.LayoutModeHoriz},
	} {
		t.Run(lm.name, func(t *testing.T) {
			rig := newAppSliderRig(t)
			tab := rig.ui.Tabs[0]
			tab.LayoutMode = lm.mode
			for i := 0; i < 4; i++ {
				rig.frame()
			}
			rig.ui.SetSaveNeeded(false)

			tab.ReqCollapseBtn.Click()
			tab.RespCollapseBtn.Click()
			tab.ViewGeneratedBtn.Click()
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if !tab.ReqBodyCollapsed || !tab.RespBodyCollapsed || tab.HeadersExpanded {
				t.Fatalf("panes did not collapse: req=%v resp=%v headersExpanded=%v",
					tab.ReqBodyCollapsed, tab.RespBodyCollapsed, tab.HeadersExpanded)
			}

			ts := savedTab(t, rig.ui)
			if !ts.ReqCollapsed || !ts.RespCollapsed || ts.HeadersExpanded {
				t.Fatalf("saved state lost the collapse flags: req=%v resp=%v headers=%v",
					ts.ReqCollapsed, ts.RespCollapsed, ts.HeadersExpanded)
			}

			restored := workspace.TabFromState(ts)
			if !restored.ReqBodyCollapsed || !restored.RespBodyCollapsed || restored.HeadersExpanded {
				t.Fatalf("restart reopened the panes: req=%v resp=%v headers=%v",
					restored.ReqBodyCollapsed, restored.RespBodyCollapsed, restored.HeadersExpanded)
			}

			rig.ui.SetSaveNeeded(false)
			tab.ReqCollapseBtn.Click()
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if tab.ReqBodyCollapsed {
				t.Fatal("second click must expand the request pane again")
			}
			if ts := savedTab(t, rig.ui); ts.ReqCollapsed {
				t.Error("expanding the request pane was not persisted")
			}
		})
	}
}

func TestWSPaneCollapsePersists(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.Method = workspace.MethodWS
	tab.URLInput.SetText("wss://echo.test/ws")
	tab.EnsureWS()
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	rig.ui.SetSaveNeeded(false)

	tab.WS.ComposeCollapseBtn.Click()
	tab.WS.MessagesCollapseBtn.Click()
	tab.WS.HeadersCollapseBtn.Click()
	tab.WS.OptionsBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	s := tab.WS
	if !s.ComposeCollapsed || !s.MessagesCollapsed || !s.HeadersCollapsed || !s.OptionsExpanded {
		t.Fatalf("ws panes did not toggle: compose=%v messages=%v headers=%v options=%v",
			s.ComposeCollapsed, s.MessagesCollapsed, s.HeadersCollapsed, s.OptionsExpanded)
	}

	ts := savedTab(t, rig.ui)
	if ts.WS == nil {
		t.Fatal("ws state not persisted")
	}
	if !ts.WS.ComposeCollapsed || !ts.WS.MessagesCollapsed || !ts.WS.HeadersCollapsed || !ts.WS.OptionsExpanded {
		t.Fatalf("saved ws state lost the collapse flags: %+v", ts.WS)
	}

	restored := workspace.TabFromState(ts).WS
	if restored == nil {
		t.Fatal("ws session not restored")
	}
	if !restored.ComposeCollapsed || !restored.MessagesCollapsed || !restored.HeadersCollapsed || !restored.OptionsExpanded {
		t.Errorf("restart reopened the ws panes: %+v", restored)
	}
}

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "rete-test")
	persist.SetConfigOverride(configPath)

	t.Cleanup(func() {
		persist.SetConfigOverride("")
	})

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tempDir)
	case "darwin":
		t.Setenv("HOME", tempDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}

func dropGtx(sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
	}
}

func dropFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func libraryTestUI(t *testing.T) *AppUI {
	t.Helper()
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.SidebarWidth = 260
	ui.Drop.TopY = 30
	ui.SetSidebarZones([]sidebar.DropZoneRect{
		{ID: "collections", Rect: image.Rect(36, 0, 260, 320)},
		{ID: "scripts", Rect: image.Rect(36, 320, 260, 430)},
		{ID: "variables", Rect: image.Rect(36, 430, 260, 570)},
	})
	ui.RebuildDropZones(dropGtx(image.Pt(1000, 700)))
	return ui
}

func TestDropPipeline_CollectionsZone(t *testing.T) {
	ui := libraryTestUI(t)

	p := dropFile(t, "c.json", `{"info":{"name":"Dropped Coll"},"item":[{"name":"R"}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 100))
	ui.DrainDroppedFiles()

	select {
	case col := <-ui.ColLoadedChan:
		if col == nil || col.Data == nil || col.Data.Name != "Dropped Coll" {
			t.Fatalf("collection zone imported wrong data: %+v", col)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the collections zone must import a collection")
	}
}

func TestDropPipeline_VariablesZone(t *testing.T) {
	ui := libraryTestUI(t)

	p := dropFile(t, "e.json", `{"name":"Dropped Env","values":[{"key":"k","value":"v"}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 500))
	ui.DrainDroppedFiles()

	select {
	case env := <-ui.EnvLoadedChan:
		if env == nil || env.Data == nil || env.Data.Name != "Dropped Env" {
			t.Fatalf("variables zone imported wrong data: %+v", env)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the variables zone must import an environment")
	}
}

func TestDropPipeline_ScriptsZone(t *testing.T) {
	ui := libraryTestUI(t)

	before := len(flow.ListScenarios())
	p := dropFile(t, "s.json", `{"name":"Dropped Script","nodes":[{"id":"n1","kind":1,"x":80,"y":200}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 400))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(flow.ListScenarios()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the scripts zone must import a scenario")
}

func TestDropPipeline_HARZoneLoadsArchive(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.Drop.TopY = 30
	ui.RebuildDropZones(dropGtx(image.Pt(1000, 700)))

	p := dropFile(t, "drop.har", harTestDoc)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(500, 400))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the HAR zone must load the archive")
}

func TestDropPipeline_OutsideZonesFallsBackToHAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()

	p := dropFile(t, "drop.har", harTestDoc)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(9999, 9999))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("HAR section drop must still load even with no zone hit")
}

func TestHarPrettyShared(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "p.har", nil)
	ui.HARView.Pretty = true

	sz := image.Pt(1100, 620)
	render := func() {
		gtx := dropGtx(sz)
		for i := 0; i < 2; i++ {
			gtx.Ops = new(op.Ops)
			ui.LayoutHARSection(gtx)
		}
	}

	ui.HARView.TopTab = harui.TabRequests
	ui.HARView.SelReq = 0
	ui.HARView.InspTab = 1
	render()
	if !strings.Contains(ui.HARView.ReqViewerKey, "pretty=1") {
		t.Errorf("requests viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.ReqViewerKey)
	}

	ui.HARView.TopTab = harui.TabFiles
	ui.HARView.SelFile = 0
	render()
	if !strings.Contains(ui.HARView.FileViewerKey, "pretty=1") {
		t.Errorf("files viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.FileViewerKey)
	}
}

func TestHarRunEntry_CarriesLangHint(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	ui.HARRunEntry(&ui.HARView.Doc.Entries[0])
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.ReqLangHint != syntax.LangJSON {
		t.Errorf("ReqLangHint = %v, want LangJSON so the request tab keeps the HAR colouring", rt.ReqLangHint)
	}
}

func TestEmojiFontMetadata(t *testing.T) {
	b, err := LoadEmbeddedTTF("NotoColorEmoji.ttf")
	if err != nil {
		t.Fatalf("load NotoColorEmoji: %v", err)
	}
	face, err := opentype.Parse(b)
	if err != nil {
		t.Fatalf("parse NotoColorEmoji: %v", err)
	}
	fnt := face.Font()
	t.Logf("Family: %q  Style: %v  Weight: %v", fnt.Typeface, fnt.Style, fnt.Weight)
	if fnt.Typeface != widgets.EmojiTypeface {
		t.Errorf("emoji font typeface = %q, want %q", fnt.Typeface, widgets.EmojiTypeface)
	}
}

func buildShaper(t *testing.T) (*text.Shaper, []string) {
	var fonts []font.FontFace
	var faceNames []string

	addTextFont := func(name string) {
		b, err := LoadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		stripped, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		face, err := opentype.Parse(stripped)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := face.Font()
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}
	addJBM := func(name string, style font.Style, weight font.Weight) {
		b, err := LoadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		stripped, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		face, err := opentype.Parse(stripped)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := font.Font{Typeface: widgets.MonoFamilyName, Style: style, Weight: weight}
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}
	addEmojiFont := func(name string) {
		b, err := LoadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := face.Font()
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}

	addTextFont("Inter-Regular.ttf")
	addTextFont("Inter-Bold.ttf")
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)
	addEmojiFont("NotoColorEmoji.ttf")

	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(fonts)), faceNames
}

const facebits = 16
const sizebits = 16
const gidbits = 64 - facebits - sizebits

func faceIdxFromGlyph(id uint64) int {
	return int(id >> (gidbits + sizebits))
}

const (
	interRegularIdx = 0
	interBoldIdx    = 1
	jbmRegularIdx   = 2
	jbmBoldIdx      = 3
	jbmItalicIdx    = 4
	jbmBoldItIdx    = 5
	emojiFaceIdx    = 6
)

func TestEmojiShapingPureEmojisUseEmojiFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	t.Logf("Font collection:")
	for i, n := range faceNames {
		t.Logf("  face[%d] = %s", i, n)
	}

	cases := []struct {
		name string
		s    string
	}{
		{"😀", "\U0001F600"},
		{"🚀", "\U0001F680"},
		{"🎉", "\U0001F389"},
		{"👍", "\U0001F44D"},
		{"🇺🇸 flag", "\U0001F1FA\U0001F1F8"},
		{"🇯🇵 flag", "\U0001F1EF\U0001F1F5"},
		{"👨‍💻 ZWJ", "\U0001F468‍\U0001F4BB"},
		{"👍🏻 skin", "\U0001F44D\U0001F3FB"},
		{"🏳️‍🌈 rainbow", "\U0001F3F3️‍\U0001F308"},
		{"🙂", "\U0001F642"},
		{"🤔", "\U0001F914"},
		{"🔥", "\U0001F525"},
		{"🌍", "\U0001F30D"},
		{"👩‍🔬", "\U0001F469‍\U0001F52C"},
		{"👨‍👩‍👧‍👦 family", "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466"},
		{"🤦", "\U0001F926"},
		{"🐱", "\U0001F431"},
		{"🍎", "\U0001F34E"},
	}
	pxPerEm := fixed.I(20)
	queries := []font.Font{
		{Typeface: "Inter," + widgets.EmojiTypeface},
		{Typeface: widgets.MonoTypeface},
		{Typeface: ""},
	}
	for _, q := range queries {
		t.Run(string(q.Typeface), func(t *testing.T) {
			for _, tc := range cases {
				shaper.LayoutString(text.Parameters{
					PxPerEm:  pxPerEm,
					MaxWidth: 1 << 20,
					Locale:   system.Locale{Language: "en", Direction: system.LTR},
					Font:     q,
				}, tc.s)
				var advance fixed.Int26_6
				faces := map[int]int{}
				for {
					g, ok := shaper.NextGlyph()
					if !ok {
						break
					}
					if g.Advance == 0 && g.Runes == 0 {
						continue
					}
					advance += g.Advance
					faces[faceIdxFromGlyph(uint64(g.ID))]++
				}
				t.Logf("  %-25s faces=%v advance=%v", tc.name, faces, advance)
				if advance == 0 {
					t.Errorf("    zero advance for %q", tc.s)
				}
				for idx, cnt := range faces {
					if idx != emojiFaceIdx && cnt > 0 {
						t.Errorf("    %s: face[%d]=%s used (want emoji face[%d])",
							tc.name, idx, faceNames[idx], emojiFaceIdx)
					}
				}
			}
		})
	}
}

func TestDigitsAndTextStayInTextFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	cases := []struct {
		name        string
		s           string
		query       font.Font
		wantTextIdx int
	}{
		{"digits/Inter", "1234567890", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"hash/Inter", "#", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"asterisk/Inter", "*", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"latin/Inter", "Hello", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"cyrillic/Inter", "Привет", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"digits/Mono", "1234567890", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"hash/Mono", "#", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"asterisk/Mono", "*", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"latin/Mono", "Hello", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"digits/Empty", "1234567890", font.Font{}, interRegularIdx},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shaper.LayoutString(text.Parameters{
				PxPerEm:  fixed.I(20),
				MaxWidth: 1 << 20,
				Locale:   system.Locale{Language: "en", Direction: system.LTR},
				Font:     tc.query,
			}, tc.s)
			var adv fixed.Int26_6
			faces := map[int]int{}
			for {
				g, ok := shaper.NextGlyph()
				if !ok {
					break
				}
				if g.Advance == 0 && g.Runes == 0 {
					continue
				}
				adv += g.Advance
				faces[faceIdxFromGlyph(uint64(g.ID))]++
			}
			t.Logf("%s faces=%v advance=%v", tc.name, faces, adv)
			if adv == 0 {
				t.Errorf("%q: zero advance", tc.s)
			}
			if n := faces[emojiFaceIdx]; n > 0 {
				t.Errorf("%q: %d glyphs went through NotoColorEmoji — should be %s",
					tc.s, n, faceNames[tc.wantTextIdx])
			}
		})
	}
}

func TestDualUseBMPEmojiNowGoToEmojiFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	cases := []struct {
		name string
		s    string
	}{
		{"heart ❤", "❤"},
		{"warning ⚠", "⚠"},
		{"sun ☀", "☀"},
		{"snowman ☃", "☃"},
		{"snowflake ❄", "❄"},
		{"lightning ⚡", "⚡"},
		{"white square ⬜", "⬜"},
		{"black square ⬛", "⬛"},
		{"telephone ☎", "☎"},
		{"airplane ✈", "✈"},
		{"hot bev ☕", "☕"},
		{"copyright ©", "©"},
		{"registered ®", "®"},
		{"tm ™", "™"},
		{"star ⭐", "⭐"},
	}
	queries := []font.Font{
		{Typeface: "Inter," + widgets.EmojiTypeface},
		{Typeface: widgets.MonoTypeface},
		{Typeface: "Inter," + widgets.EmojiTypeface, Weight: font.Bold},
		{Typeface: widgets.MonoTypeface, Style: font.Italic},
		{Typeface: ""},
	}
	for _, q := range queries {
		t.Run(string(q.Typeface)+"/"+q.Weight.String()+"/"+q.Style.String(), func(t *testing.T) {
			for _, tc := range cases {
				shaper.LayoutString(text.Parameters{
					PxPerEm:  fixed.I(20),
					MaxWidth: 1 << 20,
					Locale:   system.Locale{Language: "en", Direction: system.LTR},
					Font:     q,
				}, tc.s)
				var adv fixed.Int26_6
				faces := map[int]int{}
				for {
					g, ok := shaper.NextGlyph()
					if !ok {
						break
					}
					if g.Advance == 0 && g.Runes == 0 {
						continue
					}
					adv += g.Advance
					faces[faceIdxFromGlyph(uint64(g.ID))]++
				}
				t.Logf("  %-20s faces=%v advance=%v", tc.name, faces, adv)
				if adv == 0 {
					t.Errorf("%s: zero advance", tc.name)
				}
				for idx, cnt := range faces {
					if idx != emojiFaceIdx && cnt > 0 {
						t.Errorf("%s: face[%d]=%s used (want NotoColorEmoji)",
							tc.name, idx, faceNames[idx])
					}
				}
			}
		})
	}
}

func TestNonEmojiUnicodeStillWorks(t *testing.T) {
	shaper, _ := buildShaper(t)
	cases := []struct {
		name string
		s    string
	}{
		{"latin", "Hello world"},
		{"cyrillic", "Привет мир"},
		{"greek", "Γειά σου κόσμε"},
		{"punctuation", "[]{}—«»…"},
		{"numbers", "1234567890"},
		{"latin+emoji", "hi 🚀"},
	}
	for _, tc := range cases {
		shaper.LayoutString(text.Parameters{
			PxPerEm:  fixed.I(20),
			MaxWidth: 1 << 20,
			Locale:   system.Locale{Language: "en", Direction: system.LTR},
			Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
		}, tc.s)
		var adv fixed.Int26_6
		var gc int
		for {
			g, ok := shaper.NextGlyph()
			if !ok {
				break
			}
			gc++
			adv += g.Advance
		}
		t.Logf("%-20s glyphs=%d advance=%v", tc.name, gc, adv)
		if adv == 0 {
			t.Errorf("%q produced zero advance", tc.s)
		}
	}
}

type envKeysRig struct {
	t   *testing.T
	ui  *AppUI
	env *environments.EnvironmentUI
	r   input.Router
}

func newEnvKeysRig(t *testing.T) *envKeysRig {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
		ID:   "env1",
		Name: "Test Env",
		Vars: []model.EnvVar{{Key: "k1", Value: "v1"}},
	}}
	env.InitEditor()
	ui.Environments = append(ui.Environments, env)
	ui.EditingEnv = env
	rig := &envKeysRig{t: t, ui: ui, env: env}
	rig.frame(nil)
	rig.frame(nil)
	return rig
}

func (rig *envKeysRig) frame(before func(gtx layout.Context)) {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 800)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	if before != nil {
		before(gtx)
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(gtx.Ops)
}

func (rig *envKeysRig) focus(ed *widget.Editor) {
	rig.frame(func(gtx layout.Context) { gtx.Execute(key.FocusCmd{Tag: ed}) })
	rig.frame(nil)
	if !rig.r.Source().Focused(ed) {
		rig.t.Fatal("setup: editor did not take focus")
	}
}

func (rig *envKeysRig) press(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame(nil)
	rig.frame(nil)
}

func TestEnvEditorCtrlEnterSavesAndBlurs(t *testing.T) {
	rig := newEnvKeysRig(t)
	row := rig.env.Rows[0]
	rig.focus(&row.KeyEditor)
	row.KeyEditor.SetText("renamed")
	rig.frame(nil)

	rig.press(key.NameReturn, key.ModShortcut)

	if rig.r.Source().Focused(&row.KeyEditor) {
		t.Error("Ctrl+Enter must leave the field")
	}
	if got := rig.env.Data.Vars[0].Key; got != "renamed" {
		t.Errorf("Ctrl+Enter must save the rename, got %q", got)
	}
	if rig.ui.EditingEnv == nil {
		t.Error("env editor must stay open")
	}
}

func TestEnvEditorCtrlSSavesAndBlurs(t *testing.T) {
	rig := newEnvKeysRig(t)
	row := rig.env.Rows[0]
	rig.focus(&row.ValEditor)
	row.ValEditor.SetText("changed")
	rig.frame(nil)

	rig.press("S", key.ModShortcut)

	if rig.r.Source().Focused(&row.ValEditor) {
		t.Error("Ctrl+S must leave the field")
	}
	if got := rig.env.Data.Vars[0].Value; got != "changed" {
		t.Errorf("Ctrl+S must save the value, got %q", got)
	}
}

func TestEnvEditorEnterBlursWithoutSaving(t *testing.T) {
	rig := newEnvKeysRig(t)
	row := rig.env.Rows[0]
	rig.focus(&row.KeyEditor)
	row.KeyEditor.SetText("renamed")
	rig.frame(nil)

	rig.press(key.NameReturn, 0)

	if rig.r.Source().Focused(&row.KeyEditor) {
		t.Error("Enter must leave the field")
	}
	if got := rig.env.Data.Vars[0].Key; got != "k1" {
		t.Errorf("plain Enter must not commit, got %q", got)
	}
	if !rig.env.EditorDirty() {
		t.Error("the edit must stay pending in the editor")
	}
}

func TestEnvEditorNameFieldEnterBlurs(t *testing.T) {
	rig := newEnvKeysRig(t)
	rig.focus(&rig.env.NameEditor)
	rig.press(key.NameReturn, 0)
	if rig.r.Source().Focused(&rig.env.NameEditor) {
		t.Error("Enter in the name field must leave it")
	}
}

func TestMonoFontShapesAsterisksUniformly(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	sh := ui.Theme.Shaper
	for _, txt := range []string{"***", "*****", "->", "!="} {
		sh.LayoutString(text.Parameters{
			Font:     widgets.MonoFont,
			PxPerEm:  fixed.I(14),
			MaxWidth: 1 << 20,
			Locale:   system.Locale{Language: "EN", Direction: system.LTR},
		}, txt)
		var glyphs []text.Glyph
		for {
			g, ok := sh.NextGlyph()
			if !ok {
				break
			}
			glyphs = append(glyphs, g)
		}
		if len(glyphs) != len([]rune(txt)) {
			t.Errorf("%q: %d glyphs for %d runes, glyphs must not merge into ligatures", txt, len(glyphs), len([]rune(txt)))
		}
		if txt[0] != '*' {
			continue
		}
		for _, g := range glyphs {
			if g.ID != glyphs[0].ID || g.Offset != glyphs[0].Offset {
				t.Errorf("%q: every asterisk must use the same glyph: %v", txt, glyphs)
				break
			}
		}
	}
}

func TestEnvEditor(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	env := &model.ParsedEnvironment{
		ID:   "env1",
		Name: "Test Env",
		Vars: []model.EnvVar{{Key: "k1", Value: "v1"}},
	}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})
	ui.EditingEnv = ui.Environments[0]
	ui.EditingEnv.InitEditor()

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}

	ui.LayoutEnvEditor(gtx)

	ui.EditingEnv.AddBtn.Click()
	ui.LayoutEnvEditor(gtx)
	if len(ui.EditingEnv.Rows) != 2 {
		t.Errorf("expected 2 rows after add, got %d", len(ui.EditingEnv.Rows))
	}

	ui.EditingEnv.NameEditor.SetText("Updated Env")
	ui.EditingEnv.Rows[0].KeyEditor.SetText("newKey")
	ui.EditingEnv.SaveBtn.Click()
	ui.LayoutEnvEditor(gtx)

	if ui.EditingEnv == nil {
		t.Errorf("expected editing mode to remain open after save")
	}
	if env.Name != "Updated Env" {
		t.Errorf("expected name to be updated, got %s", env.Name)
	}
	if env.Vars[0].Key != "newKey" {
		t.Errorf("expected var key to be updated")
	}

	ui.EditingEnv.Rows[0].DelBtn.Click()
	ui.LayoutEnvEditor(gtx)
	if len(ui.EditingEnv.Rows) != 1 {
		t.Errorf("expected 1 row after delete, got %d", len(ui.EditingEnv.Rows))
	}

	ui.EditingEnv.BackBtn.Click()
	ui.LayoutEnvEditor(gtx)
	if ui.EditingEnv != nil {
		t.Errorf("expected editing mode to be closed after back")
	}
}

func TestEnvEditor_Discard(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &model.ParsedEnvironment{ID: "e1", Name: "E1"}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})
	ui.EditingEnv = ui.Environments[0]
	ui.EditingEnv.InitEditor()

	gtx := layout.Context{Ops: new(op.Ops)}
	ui.EditingEnv.BackBtn.Click()
	ui.LayoutEnvEditor(gtx)

	if ui.EditingEnv != nil {
		t.Errorf("expected editing mode closed")
	}
}

func TestSaveVarPopup(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &model.ParsedEnvironment{ID: "e1", Name: "E1"}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})

	ui.VarPopup.EnvID = "e1"
	ui.VarPopup.Name = "newVar"
	ui.VarPopup.Editor.SetText("val")
	ui.SaveVarPopup()

	if len(env.Vars) != 1 || env.Vars[0].Key != "newVar" || env.Vars[0].Value != "val" {
		t.Errorf("var not saved to env")
	}

	ui.VarPopup.Editor.SetText("newVal")
	ui.SaveVarPopup()
	if env.Vars[0].Value != "newVal" {
		t.Errorf("var not updated")
	}
}

func TestSaveVarPopup_NoEnvironmentAutoCreates(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()

	ui.VarPopup.EnvID = ""
	ui.ActiveEnvID = ""
	ui.VarPopup.Name = "token"
	ui.VarPopup.Editor.SetText("secret")
	ui.SaveVarPopup()

	if len(ui.Environments) != 1 {
		t.Fatalf("expected a default environment to be auto-created, got %d", len(ui.Environments))
	}
	env := ui.Environments[0].Data
	if ui.ActiveEnvID != env.ID {
		t.Errorf("auto-created environment should become active, got %q", ui.ActiveEnvID)
	}
	if len(env.Vars) != 1 || env.Vars[0].Key != "token" || env.Vars[0].Value != "secret" {
		t.Errorf("variable not saved into default environment: %+v", env.Vars)
	}

	ui.SetActiveEnvDirty(true)
	ui.RefreshActiveEnv()
	if ui.ActiveEnvVarsMap()["token"] != "secret" {
		t.Errorf("variable should resolve after auto-create, got %q", ui.ActiveEnvVarsMap()["token"])
	}
}

func TestSaveVarPopup_NoEnvironmentEmptyValueNoop(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()

	ui.VarPopup.EnvID = ""
	ui.ActiveEnvID = ""
	ui.VarPopup.Name = "token"
	ui.VarPopup.Editor.SetText("")
	ui.SaveVarPopup()

	if len(ui.Environments) != 0 {
		t.Errorf("empty value with no environment should not create one, got %d", len(ui.Environments))
	}
}

func TestActiveEnvSnapshot(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	if snap := ui.ActiveEnvSnapshot(); snap != nil {
		t.Errorf("expected nil snapshot when activeEnvVars nil")
	}
	ui.SetActiveEnvVars(map[string]string{"k": "v", "x": "y"})
	snap := ui.ActiveEnvSnapshot()
	if len(snap) != 2 || snap["k"] != "v" || snap["x"] != "y" {
		t.Errorf("snapshot mismatch: %v", snap)
	}

	snap["k"] = "MUT"
	if ui.ActiveEnvVarsMap()["k"] != "v" {
		t.Errorf("snapshot should be independent copy")
	}
}

func TestRefreshActiveEnv_EmptyValuesAndMissingEnv(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &model.ParsedEnvironment{
		ID:   "e1",
		Name: "E1",
		Vars: []model.EnvVar{
			{Key: "ok", Value: "v"},
			{Key: "also", Value: "v2"},
			{Key: "empty", Value: ""},
		},
	}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})
	ui.ActiveEnvID = "e1"
	ui.SetActiveEnvDirty(true)
	ui.RefreshActiveEnv()
	if _, ok := ui.ActiveEnvVarsMap()["ok"]; !ok {
		t.Errorf("var with value missing")
	}
	if _, ok := ui.ActiveEnvVarsMap()["also"]; !ok {
		t.Errorf("var with value missing")
	}
	if _, ok := ui.ActiveEnvVarsMap()["empty"]; ok {
		t.Errorf("empty-value var should be excluded")
	}

	ui.SetActiveEnvVars(map[string]string{"sentinel": "1"})
	ui.SetActiveEnvDirty(false)
	ui.RefreshActiveEnv()
	if ui.ActiveEnvVarsMap()["sentinel"] != "1" {
		t.Errorf("expected no-op when not dirty")
	}

	ui.ActiveEnvID = "missing"
	ui.SetActiveEnvDirty(true)
	ui.RefreshActiveEnv()
	if ui.ActiveEnvVarsMap() != nil {
		t.Errorf("expected nil when no matching env")
	}
}

func TestNewVariableResolvesAfterEditorCommit(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	env := &model.ParsedEnvironment{ID: "e1", Name: "E1"}
	envUI := &environments.EnvironmentUI{Data: env}
	envUI.InitEditor()
	ui.Environments = append(ui.Environments, envUI)
	ui.EditingEnv = envUI
	ui.ActiveEnvID = "e1"

	envUI.Rows = append(envUI.Rows, &environments.EnvVarRow{})
	envUI.Rows[0].KeyEditor.SetText("base")
	envUI.Rows[0].ValEditor.SetText("http://api")

	ui.CommitEditingEnv()
	ui.RefreshActiveEnv()

	if got := ui.ActiveEnvVarsMap()["base"]; got != "http://api" {
		t.Fatalf("a freshly added {{base}} must resolve to its value; got %q", got)
	}
}

func TestApplySharedLayout(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Tabs = nil

	newTab := workspace.NewRequestTab("x")
	original := newTab.SplitRatio
	ui.ApplySharedLayout(newTab)
	if newTab.SplitRatio != original {
		t.Errorf("no tabs: nothing to share (was %v, now %v)", original, newTab.SplitRatio)
	}

	src := workspace.NewRequestTab("src")
	src.SplitRatio = 0.42
	src.VStackRatio = 0.31
	src.LayoutMode = 1
	src.HeaderKeyW = 0.7
	ui.Tabs = []*workspace.RequestTab{src}
	ui.ActiveIdx = 0

	dst := workspace.NewRequestTab("dst")
	ui.ApplySharedLayout(dst)
	if dst.SplitRatio != 0.42 || dst.VStackRatio != 0.31 || dst.LayoutMode != 1 || dst.HeaderKeyW != 0.7 {
		t.Errorf("layout not inherited: %+v", dst)
	}

	ui.ApplySharedLayout(src)
	if src.SplitRatio != 0.42 {
		t.Errorf("self-inherit should be no-op")
	}

	ui.ActiveIdx = 99
	dst2 := workspace.NewRequestTab("dst2")
	ui.ApplySharedLayout(dst2)
	if dst2.SplitRatio != 0.42 || dst2.VStackRatio != 0.31 {
		t.Errorf("shared layout must apply even without a valid active tab: %+v", dst2)
	}
}

func TestSharedLayoutPropagatesToExistingTabs(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()

	a := workspace.NewRequestTab("a")
	b := workspace.NewRequestTab("b")
	c := workspace.NewRequestTab("c")
	ui.Tabs = []*workspace.RequestTab{a, b, c}
	ui.ActiveIdx = 1
	ui.SyncLayoutPrefs()

	b.SplitRatio = 0.37
	b.VStackRatio = 0.62
	b.LayoutMode = 2
	b.HeaderKeyW = 0.44
	b.ReqBodyCollapsed = true
	ui.SyncLayoutPrefs()

	for _, tab := range []*workspace.RequestTab{a, c} {
		if tab.SplitRatio != 0.37 || tab.VStackRatio != 0.62 || tab.LayoutMode != 2 || tab.HeaderKeyW != 0.44 || !tab.ReqBodyCollapsed {
			t.Errorf("tab %q did not receive the resize: %+v", tab.Title, tab)
		}
	}

	ui.ActiveIdx = 0
	a.LayoutMode = 1
	ui.SyncLayoutPrefs()
	if b.LayoutMode != 1 || c.LayoutMode != 1 {
		t.Errorf("resize from another active tab must propagate too: b=%d c=%d", b.LayoutMode, c.LayoutMode)
	}
}

func TestUpdateVisibleCols_DeepNesting(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	leaf := &collections.CollectionNode{Name: "leaf", Request: &model.ParsedRequest{}}
	folder := &collections.CollectionNode{Name: "f", IsFolder: true, Expanded: true, Depth: 1, Children: []*collections.CollectionNode{leaf}}
	leaf.Parent = folder
	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true, Depth: 0, Children: []*collections.CollectionNode{folder}}
	folder.Parent = root
	col := &collections.ParsedCollection{ID: "c1", Root: root}
	root.Collection = col
	folder.Collection = col
	leaf.Collection = col
	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: col})
	ui.UpdateVisibleCols()
	if len(ui.VisibleCols) != 3 {
		t.Errorf("expected 3 visible nodes when all expanded, got %d", len(ui.VisibleCols))
	}

	folder.Expanded = false
	ui.UpdateVisibleCols()
	if len(ui.VisibleCols) != 2 {
		t.Errorf("expected 2 visible nodes (root,folder), got %d", len(ui.VisibleCols))
	}

	root.Expanded = false
	ui.UpdateVisibleCols()
	if len(ui.VisibleCols) < 1 {
		t.Errorf("expected at least root visible, got %d", len(ui.VisibleCols))
	}
}

func TestCloseTab_BoundaryAndOutOfRange(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Tabs = []*workspace.RequestTab{
		workspace.NewRequestTab("a"),
		workspace.NewRequestTab("b"),
		workspace.NewRequestTab("c"),
	}
	ui.ActiveIdx = 2

	before := len(ui.Tabs)
	ui.CloseTab(-1)
	ui.CloseTab(99)
	if len(ui.Tabs) != before {
		t.Errorf("out-of-range close should be no-op")
	}

	ui.CloseTab(2)
	if len(ui.Tabs) != 2 || ui.ActiveIdx != 1 {
		t.Errorf("after close last: tabs=%d active=%d", len(ui.Tabs), ui.ActiveIdx)
	}

	ui.CloseTab(0)
	if len(ui.Tabs) != 1 || ui.ActiveIdx != 0 {
		t.Errorf("after close first w/ active=1: tabs=%d active=%d", len(ui.Tabs), ui.ActiveIdx)
	}

	ui.CloseTab(0)
	if len(ui.Tabs) != 0 {
		t.Errorf("expected empty tabs")
	}
	if ui.ActiveIdx != -1 {
		t.Errorf("expected ActiveIdx=-1 after closing only tab, got %d", ui.ActiveIdx)
	}
}

func TestMITMLayoutSection_BasicSmoke(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 600)),
		Now:         time.Now(),
	}

	ui.LayoutMITMSection(gtx)

	ui.MITM.HelpOpen = true
	ui.LayoutMITMSection(gtx)
}

func TestPushChannelsAndImportInvalid(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.ImportDroppedData([]byte("not even json"))
	select {
	case <-ui.ColLoadedChan:
		t.Errorf("garbage should not push collection")
	case <-ui.EnvLoadedChan:
		t.Errorf("garbage should not push environment")
	case <-time.After(300 * time.Millisecond):

	}
}

func TestMITMAllViewsSmoke(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "mitm"
	st := &ui.MITM
	st.Ensure()

	// seed a forward flow, a reverse flow, and a tunnel flow
	f1 := st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcForward, Method: "GET",
		Host: "example.com", Port: "443", Path: "/a?x=1", URL: "https://example.com/a?x=1",
		StatusCode: 200, Status: "200 OK", Started: time.Now(),
		ReqHeaders:  [][2]string{{"Host", "example.com"}, {"Cookie", "sid=1"}},
		RespHeaders: [][2]string{{"Content-Type", "text/html"}, {"Set-Cookie", "a=b"}},
		ReqBody:     []byte("k=v"), RespBody: []byte("<html>hi</html>")})
	st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcReverse, TargetDomain: "rev.example.com",
		Method: "POST", Host: "rev.example.com", Path: "/", StatusCode: 500, Started: time.Now()})
	st.Store.Add(&mitm.Flow{Kind: mitm.FlowTunnel, Method: "CONNECT", Host: "tls.example.com", Port: "443", Started: time.Now()})
	st.Selected = f1.ID
	st.Store.SetAnnotation(f1.ID, "red", "note")

	// seed reverse target, MR/scope/intercept rules, a WS message, a held item
	st.Proxy.Targets.Add(&mitm.Target{Domain: "shop.example.com"})
	st.Proxy.MR.Add(mitm.MatchReplaceRule{Enabled: true, Type: mitm.MRResponse, Area: mitm.MRHeader, Pattern: "X-Frame-Options"})
	st.Proxy.ScopeR.Add(mitm.ScopeRule{Enabled: true, Kind: mitm.ScopeInclude, Field: "host", Pattern: "example"})
	st.Proxy.IRules.Add(mitm.HeldRequest, mitm.InterceptCond{Enabled: true, Field: mitm.CondMethod, Value: "GET"})
	st.Proxy.WS.Add(&mitm.WSMessage{FlowID: f1.ID, URL: "wss://x/y", ToServer: true, Opcode: 0x1, Payload: []byte("ping")})

	gtx := func() layout.Context {
		return layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1280, 800)), Now: time.Now()}
	}

	// expand every accordion section + open help
	st.SecTargetsOpen, st.SecTLSOpen, st.SecIRulesOpen, st.SecMROpen, st.SecScopeOpen = true, true, true, true, true
	st.HelpOpen = true
	ui.LayoutMITMSidebar(gtx())

	// render each view, each inspector tab + render mode + section tab
	for _, view := range []string{mitm.ViewHistory, mitm.ViewIntercept, mitm.ViewWebSockets} {
		st.View = view
		for _, tab := range []int{0, 1} {
			st.ActTab = tab
			for _, rm := range []int{0, 1, 2, 3} {
				st.RenderMode = rm
				for _, sec := range []int{0, 1, 2, 3} {
					st.SecTab = sec
					ui.LayoutMITMSection(gtx())
				}
			}
		}
	}

	// overlays
	st.CtxOpen = true
	st.CtxFlowID = f1.ID
	ui.LayoutMITMSection(gtx())
	st.CtxOpen = false
	st.AnnotateOpen = true
	st.AnnotateFlowID = f1.ID
	ui.LayoutMITMSection(gtx())
	st.AnnotateOpen = false
	st.ClearConfirmOpen = true
	ui.LayoutMITMSection(gtx())
	st.ClearConfirmOpen = false

	// collapsed inspector
	st.InspectorCollapsed = true
	ui.LayoutMITMSection(gtx())
}

func findGtx(r *input.Router) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 800)),
		Source:      r.Source(),
	}
}

func findRig(t *testing.T) (*AppUI, *workspace.RequestTab) {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	if len(ui.Tabs) == 0 {
		t.Fatal("expected a request tab to exist")
	}
	tab := ui.Tabs[ui.ActiveIdx]
	tab.RespEditor.SetText(`{"name":"value"}`)
	return ui, tab
}

// Ctrl+F used to fall through to the active request tab from every section that
// is not the HAR viewer, so pressing it over MITM opened a search box on a pane
// that was not on screen — invisible, unclosable, and waiting to pop up the next
// time the workspace came back.
func TestFindShortcut_InertInSectionsWithoutText(t *testing.T) {
	for _, section := range []string{"mitm", "netlimit", "flows"} {
		// mitm has its own search now; what must never happen from any of
		// these sections is the shortcut reaching the hidden request tab
		t.Run(section, func(t *testing.T) {
			ui, tab := findRig(t)
			ui.SidebarSection = section

			var r input.Router
			ui.FindShortcut(findGtx(&r))

			if tab.RespSearch.Open || tab.ReqSearch.Open {
				t.Errorf("Ctrl+F in the %s section opened a search on the hidden request tab", section)
			}
		})
	}
}

func TestFindShortcut_OpensInTheWorkspace(t *testing.T) {
	ui, tab := findRig(t)
	ui.SidebarSection = "requests"

	var r input.Router
	ui.FindShortcut(findGtx(&r))

	if !tab.RespSearch.Open {
		t.Error("Ctrl+F in the workspace must open the response search")
	}
}

func TestFindShortcut_GoesToTheHARViewer(t *testing.T) {
	ui, tab := findRig(t)
	ui.SidebarSection = "har"

	var r input.Router
	ui.FindShortcut(findGtx(&r))

	if tab.RespSearch.Open || tab.ReqSearch.Open {
		t.Error("Ctrl+F in the HAR section must not reach the request tab")
	}
}

type embeddedFont struct {
	name string
	data []byte
}

func textFontPayloads(t *testing.T) []embeddedFont {
	files := []struct{ label, file string }{
		{"Inter-Regular", "Inter-Regular.ttf"},
		{"Inter-Bold", "Inter-Bold.ttf"},
		{"JetBrainsMono-Regular", "JetBrainsMono-Regular.ttf"},
		{"JetBrainsMono-Bold", "JetBrainsMono-Bold.ttf"},
		{"JetBrainsMono-Italic", "JetBrainsMono-Italic.ttf"},
		{"JetBrainsMono-BoldItalic", "JetBrainsMono-BoldItalic.ttf"},
	}
	out := make([]embeddedFont, 0, len(files))
	for _, f := range files {
		b, err := LoadEmbeddedTTF(f.file)
		if err != nil {
			t.Fatalf("load %s: %v", f.file, err)
		}
		out = append(out, embeddedFont{f.label, b})
	}
	return out
}

func TestSubsetRemovesEmojiCoverage(t *testing.T) {
	emojisToCheck := []rune{
		0x00A9,
		0x00AE,
		0x2122,
		0x2600,
		0x2603,
		0x2614,
		0x2615,
		0x2618,
		0x2620,
		0x26A0,
		0x26A1,
		0x26C4,
		0x26FD,
		0x2705,
		0x2708,
		0x2728,
		0x2744,
		0x274C,
		0x2753,
		0x2757,
		0x2764,
		0x2B1C,
		0x2B50,
		0x2B55,
	}
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("SubsetEmoji: %v", err)
			}
			face, err := opentype.Parse(out)
			if err != nil {
				t.Fatalf("Parse subsetted: %v", err)
			}
			fnt := face.Face().Font
			for _, r := range emojisToCheck {
				if gid, ok := fnt.Cmap.Lookup(r); ok && gid != 0 {
					t.Errorf("U+%04X still mapped to glyph %d after subset", r, gid)
				}
			}
		})
	}
}

func TestSubsetKeepsTextCoverage(t *testing.T) {
	mustCover := []rune{
		'0', '1', '5', '9',
		'#', '*',
		'A', 'M', 'z',
		'(', ')', ',', '.', ' ',
		'я', 'А', 'Ё',
		'α', 'Ω',
	}
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("SubsetEmoji: %v", err)
			}
			face, err := opentype.Parse(out)
			if err != nil {
				t.Fatalf("Parse subsetted: %v", err)
			}
			fnt := face.Face().Font
			for _, r := range mustCover {
				gid, ok := fnt.Cmap.Lookup(r)
				if !ok || gid == 0 {
					t.Errorf("U+%04X (%q) lost coverage (gid=%d ok=%v)", r, string(r), gid, ok)
				}
			}
		})
	}
}

func TestSubsetRoundTripParses(t *testing.T) {
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("subset: %v", err)
			}
			if _, err := opentype.Parse(out); err != nil {
				t.Fatalf("opentype.Parse: %v", err)
			}
		})
	}
}

func TestSubsetIdempotent(t *testing.T) {
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			once, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("first subset: %v", err)
			}
			twice, err := fontsubset.SubsetEmoji(once)
			if err != nil {
				t.Fatalf("second subset: %v", err)
			}
			if len(twice) > len(once) {
				t.Errorf("size grew on re-subset: %d → %d", len(once), len(twice))
			}
			face, err := opentype.Parse(twice)
			if err != nil {
				t.Fatalf("parse twice: %v", err)
			}
			f := face.Face().Font
			if gid, ok := f.Cmap.Lookup(0x2764); ok && gid != 0 {
				t.Errorf("❤ reappeared after re-subset")
			}
			if gid, ok := f.Cmap.Lookup('0'); !ok || gid == 0 {
				t.Errorf("digit 0 lost on re-subset")
			}
		})
	}
}

const harRunDoc = `{
  "log": {
    "version": "1.2",
    "entries": [
      {"request": {"method": "POST", "url": "https://api.example.com/v1/users?q=1",
        "headers": [{"name":"Content-Type","value":"application/json"},{"name":":authority","value":"api.example.com"},{"name":"Content-Length","value":"9"}],
        "postData": {"mimeType":"application/json","text":"{\"a\":1}"}},
        "response": {"status": 200, "content": {"mimeType":"application/json","text":"{}"}}},
      {"request": {"method": "GET", "url": "https://example.com/socket",
        "headers": [{"name":"Upgrade","value":"websocket"}]},
        "response": {"status": 101},
        "_webSocketMessages": [
          {"type":"send","time":1,"opcode":1,"data":"{\"hi\":1}"},
          {"type":"receive","time":2,"opcode":1,"data":"pong"}
        ]}
    ]
  }
}`

func TestHarRunEntry_HTTP(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	before := len(ui.Tabs)
	ui.HARRunEntry(&ui.HARView.Doc.Entries[0])
	if len(ui.Tabs) != before+1 {
		t.Fatalf("expected a new tab; tabs %d→%d", before, len(ui.Tabs))
	}
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.Method != "POST" {
		t.Errorf("method = %q", rt.Method)
	}
	if got := rt.URLInput.Text(); got != "https://api.example.com/v1/users?q=1" {
		t.Errorf("url = %q", got)
	}
	if got := rt.ReqEditor.Text(); got != `{"a":1}` {
		t.Errorf("body = %q", got)
	}
	if ui.SidebarSection != "requests" {
		t.Errorf("section = %q, want requests", ui.SidebarSection)
	}
	if !rt.URLSubmitted {
		t.Error("URLSubmitted must be set so the request auto-runs")
	}
	hdrs := harTabHeaderNames(rt)
	if hdrs[":authority"] || hdrs["content-length"] {
		t.Errorf("must skip pseudo/recomputed headers, got %v", hdrs)
	}
	if !hdrs["content-type"] {
		t.Errorf("Content-Type header should carry over, got %v", hdrs)
	}
}

func TestHarRunEntry_WebSocket(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	ui.HARRunEntry(&ui.HARView.Doc.Entries[1])
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.Method != workspace.MethodWS {
		t.Errorf("ws method = %q, want %q", rt.Method, workspace.MethodWS)
	}
	if got := rt.URLInput.Text(); got != "wss://example.com/socket" {
		t.Errorf("ws url = %q, want wss://example.com/socket", got)
	}
}

func harTabHeaderNames(rt *workspace.RequestTab) map[string]bool {
	out := map[string]bool{}
	for _, h := range rt.Headers {
		out[strings.ToLower(h.Key.Text())] = true
	}
	return out
}

func TestHarWSURL(t *testing.T) {
	cases := map[string]string{
		"https://x/s": "wss://x/s",
		"http://x/s":  "ws://x/s",
		"wss://x/s":   "wss://x/s",
	}
	for in, want := range cases {
		if got := HarWSURL(in); got != want {
			t.Errorf("HarWSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHarSkipHeader(t *testing.T) {
	for _, n := range []string{":authority", ":method", "Content-Length", "host"} {
		if !HarSkipHeader(n) {
			t.Errorf("%q should be skipped", n)
		}
	}
	for _, n := range []string{"Content-Type", "Accept", "Authorization"} {
		if HarSkipHeader(n) {
			t.Errorf("%q should NOT be skipped", n)
		}
	}
}

func TestRouteDroppedFiles_HAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()

	dir := t.TempDir()
	p := filepath.Join(dir, "drop.har")
	if err := os.WriteFile(p, []byte(harRunDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	ui.OnOSFilesDropped([]string{p}, f32.Point{})
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ui.HARView.Doc == nil {
		t.Fatal("dropping a .har in the HAR section must load it")
	}
	if len(ui.HARView.Doc.Entries) != 2 {
		t.Errorf("loaded entries = %d, want 2", len(ui.HARView.Doc.Entries))
	}
}

const harPagesDoc = `{
  "log": {
    "version": "1.2",
    "pages": [
      {"id":"page_1","title":"Home","startedDateTime":"2024-01-01T10:00:00Z"},
      {"id":"page_2","title":"About","startedDateTime":"2024-01-01T10:01:00Z"}
    ],
    "entries": [
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/a"},"response":{"status":200,"content":{"mimeType":"text/html","text":"a"}}},
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/b"},"response":{"status":200,"content":{"mimeType":"text/css","text":"b"}}},
      {"pageref":"page_2","request":{"method":"GET","url":"https://x/c"},"response":{"status":200,"content":{"mimeType":"application/json","text":"c"}}}
    ]
  }
}`

func TestHARSection_PagesTabRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harPagesDoc), "p.har", nil)
	ui.HARView.TopTab = harui.TabPages

	var r input.Router
	if d := layoutHARTwice(&r, image.Pt(1100, 620), ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("pages tab failed to render")
	}
}

const harEmptyEntriesDoc = `{"log":{"version":"1.2","entries":[]}}`

func TestHARTable_HasResizableColumns(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	if ui.HARView.Table == nil {
		t.Fatal("HAR view must build a shared table model in ensure()")
	}
	if got := len(ui.HARView.Table.Columns()); got != 7 {
		t.Errorf("HAR table columns = %d, want 7", got)
	}
}

func TestHARSection_EmptyEntriesAndLoadedRender(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harEmptyEntriesDoc), "empty.har", nil)
	ui.HARView.TopTab = harui.TabRequests

	var r input.Router
	sz := image.Pt(1100, 620)
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("empty-entries requests view failed to render")
	}

	ui.HARView.ApplyLoad([]byte(harTestDoc), "x.har", nil)
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("loaded requests view failed to render")
	}
}

func TestHARSection_NarrowSplitRendersClipped(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "x.har", nil)
	ui.HARView.TopTab = harui.TabRequests
	ui.HARView.SplitRatio = 0.8

	var r input.Router
	if d := layoutHARTwice(&r, image.Pt(700, 500), ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("narrow-split requests view failed to render")
	}
}

const harTestDoc = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "Firefox", "version": "126.0"},
    "pages": [{"id": "p1", "title": "t", "startedDateTime": "2024-01-01T10:00:00Z"}],
    "entries": [
      {"startedDateTime": "2024-01-01T10:00:00.1Z", "request": {"method": "GET", "url": "https://example.com/app.js"},
        "response": {"status": 200, "content": {"mimeType": "application/javascript", "text": "code"}}},
      {"startedDateTime": "2024-01-01T10:00:00.2Z", "request": {"method": "GET", "url": "https://example.com/empty"},
        "response": {"status": 204, "content": {"mimeType": "text/plain"}}}
    ]
  }
}`

func harTestUI(t *testing.T) *AppUI {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "har"
	return ui
}

func harTestGtx(r *input.Router, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Source:      r.Source(),
	}
}

func layoutHARTwice(r *input.Router, sz image.Point, w func(layout.Context) layout.Dimensions) layout.Dimensions {
	var dims layout.Dimensions
	for i := 0; i < 2; i++ {
		gtx := harTestGtx(r, sz)
		dims = w(gtx)
		r.Frame(gtx.Ops)
	}
	return dims
}

func TestHARSection_EmptyStateRenders(t *testing.T) {
	ui := harTestUI(t)
	var r input.Router
	sz := image.Pt(1000, 600)

	dims := layoutHARTwice(&r, sz, ui.LayoutHARSection)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("empty HAR section produced no dimensions: %+v", dims.Size)
	}
}

func TestHARSection_AllTabsRenderWhenLoaded(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	if ui.HARView.Doc == nil {
		t.Fatal("precondition: doc must load")
	}

	var r input.Router
	sz := image.Pt(1100, 620)
	for _, tab := range []int{harui.TabRequests, harui.TabFiles, harui.TabPages, harui.TabInfo} {
		ui.HARView.TopTab = tab
		dims := layoutHARTwice(&r, sz, ui.LayoutHARSection)
		if dims.Size.X <= 0 || dims.Size.Y <= 0 {
			t.Errorf("tab %d produced no dimensions", tab)
		}
	}
}

func TestHARSection_RequestsTabSelectionRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	ui.HARView.TopTab = harui.TabRequests

	var r input.Router
	sz := image.Pt(1100, 620)

	ui.HARView.InspTab = 1
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("response inspector failed to render")
	}

	ui.HARView.SelReq = 999
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("out-of-range selection broke rendering")
	}
}

func TestHARSection_FilesTabPreviewRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	ui.HARView.TopTab = harui.TabFiles

	var r input.Router
	sz := image.Pt(1100, 620)
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("files tab failed to render")
	}

	ui.HARView.SelFile = -1
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("files tab with no selection failed to render")
	}
}

func TestHARSection_WebSocketAndPrettyRender(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "ws.har", nil)

	var r input.Router
	sz := image.Pt(1100, 620)

	ui.HARView.SelReq = 1
	ui.HARView.InspTab = 1
	ui.HARView.Pretty = true
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("websocket inspector failed to render")
	}

	ui.HARView.TopTab = harui.TabFiles
	ui.HARView.Pretty = true
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("files pretty view failed to render")
	}
}

func TestHARSection_RoutingThroughLayoutApp(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)

	var r input.Router
	sz := image.Pt(1200, 700)
	for i := 0; i < 2; i++ {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Source:      r.Source(),
			Now:         time.Unix(1700000000, 0),
		}
		ui.LayoutApp(gtx)
		r.Frame(gtx.Ops)
	}
}

func TestHeadersSliderReopenFromCollapsedSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.Headers = nil
	tab.HeadersAbsHeight = 16
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.05
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	tab.HeadersAbsHeight = 16
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.press(x, float32(sliderY))
	pos := float32(sliderY)
	for i := 0; i < 40; i++ {
		pos -= 1
		rig.move(x, pos)
	}
	if tab.HeadersExpanded {
		t.Fatalf("headers did not collapse after dragging up")
	}

	var stores []int
	var expanded []bool
	for i := 0; i < 60; i++ {
		pos += 1
		rig.move(x, pos)
		h := -1
		if tab.HeadersExpanded {
			h = tab.HeadersAbsHeight
		}
		stores = append(stores, h)
		expanded = append(expanded, tab.HeadersExpanded)
	}
	rig.release(x, pos)
	t.Logf("stores=%v", stores)

	opened := false
	prev := -1
	for i, h := range stores {
		if h >= 0 {
			opened = true
			if prev >= 0 {
				step := h - prev
				if step < 0 {
					t.Fatalf("stored height rolled back on step %d: %d -> %d", i, prev, h)
				}
				if step > 2 {
					t.Fatalf("stored height jumped on step %d: %d -> %d", i, prev, h)
				}
			}
			prev = h
		} else if opened {
			t.Fatalf("headers re-collapsed on step %d while dragging down", i)
		}
	}
	if !opened {
		t.Fatalf("headers never reopened after 60px of downward drag")
	}
}

// TestLazyFontsRenderMultilingualResponse rasterizes a response body mixing
// Latin, Cyrillic, CJK, RTL and color emoji through the real app shaper, so a
// deferred face that fails to load shows up as missing ink rather than only as
// a shaping-level difference.
func TestLazyFontsRenderMultilingualResponse(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	if len(ui.Tabs) == 0 {
		ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("multilang")}
		ui.ActiveIdx = 0
	}

	sz := image.Pt(900, 620)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	shot := func(body string) image.Image {
		ui.Tabs[ui.ActiveIdx].RespEditor.SetText(body)
		for i := 0; i < 2; i++ {
			ui.LayoutApp(renderGtx(new(op.Ops), sz))
		}
		ops := new(op.Ops)
		ui.LayoutApp(renderGtx(ops, sz))
		if err := win.Frame(ops); err != nil {
			t.Fatalf("frame: %v", err)
		}
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		if err := win.Screenshot(img); err != nil {
			t.Fatalf("screenshot: %v", err)
		}
		return img
	}

	blank := shot("{\n  \"a\": 1\n}")
	filled := shot("{\n  \"ru\": \"Привет\",\n  \"zh\": \"你好世界\",\n  \"he\": \"שלום\",\n  \"th\": \"สวัสดี\",\n  \"emoji\": \"🙂🚀🎉\"\n}")

	if ink(filled) <= ink(blank) {
		t.Fatalf("multilingual body drew no additional ink (blank=%d filled=%d)", ink(blank), ink(filled))
	}
	baseHues, emojiHues := distinctHues(blank), distinctHues(filled)
	if emojiHues < baseHues+4 {
		t.Errorf("color emoji bitmaps added no hues: plain body %d, multilingual body %d", baseHues, emojiHues)
	}
}

func renderGtx(ops *op.Ops, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Now:         time.Unix(1700000000, 0),
	}
}

func ink(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 0x90 || g>>8 > 0x90 || bl>>8 > 0x90 {
				n++
			}
		}
	}
	return n
}

// distinctHues counts strongly saturated colors, which monochrome text cannot
// produce but color emoji bitmaps do.
func distinctHues(img image.Image) int {
	b := img.Bounds()
	seen := map[uint32]bool{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r32, g32, b32, _ := img.At(x, y).RGBA()
			r, g, bl := r32>>8, g32>>8, b32>>8
			maxc, minc := max3(r, g, bl), min3(r, g, bl)
			if maxc-minc < 0x50 {
				continue
			}
			seen[(r>>5)<<10|(g>>5)<<5|(bl>>5)] = true
		}
	}
	return len(seen)
}

func max3(a, b, c uint32) uint32 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c uint32) uint32 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

type lazyProbe struct {
	shaper *text.Shaper
	loaded map[string]int
	eager  int
}

func newLazyProbe(t *testing.T) *lazyProbe {
	t.Helper()
	eager := AppFontCollection()
	p := &lazyProbe{loaded: map[string]int{}, eager: len(eager)}
	lazy := AppLazyFontFaces()
	wrapped := make([]text.LazyFace, len(lazy))
	for i, lf := range lazy {
		name := string(lf.Typeface)
		load := lf.Load
		wrapped[i] = text.LazyFace{
			Typeface: lf.Typeface,
			Ranges:   lf.Ranges,
			Load: func() (font.FontFace, error) {
				p.loaded[name]++
				return load()
			},
		}
	}
	p.shaper = text.NewShaper(
		text.NoSystemFonts(),
		text.WithCollection(eager),
		text.WithLazyCollection(wrapped),
	)
	return p
}

func (p *lazyProbe) shape(s string, dir system.TextDirection) (advance fixed.Int26_6, primaryGlyphs int) {
	p.shaper.LayoutString(text.Parameters{
		PxPerEm:  fixed.I(20),
		MaxWidth: 1 << 20,
		Locale:   system.Locale{Language: "en", Direction: dir},
		Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
	}, s)
	for {
		g, ok := p.shaper.NextGlyph()
		if !ok {
			break
		}
		if g.Advance == 0 && g.Runes == 0 {
			continue
		}
		advance += g.Advance
		if faceIdxFromGlyph(uint64(g.ID)) < p.eager {
			primaryGlyphs++
		}
	}
	return advance, primaryGlyphs
}

func TestLazyFontsStayUnloadedForLatinAndCyrillic(t *testing.T) {
	p := newLazyProbe(t)
	for _, s := range []string{
		`{"name":"item-1","ok":true,"score":12.5}`,
		"Привет, мир! Ёжик — тест.",
		"Grüße, naïve café — ½ ± 3°C",
		"ΑΒΓΔ αβγδ",
	} {
		if adv, _ := p.shape(s, system.LTR); adv == 0 {
			t.Fatalf("zero advance for %q", s)
		}
	}
	if len(p.loaded) != 0 {
		t.Fatalf("lazy faces loaded for Latin/Cyrillic/Greek text: %v", p.loaded)
	}
}

func TestLazyFontsLoadOnlyClaimedScript(t *testing.T) {
	cases := []struct {
		name string
		s    string
		dir  system.TextDirection
		want string
	}{
		{"Hebrew", "שלום", system.RTL, "Noto Sans Hebrew"},
		{"Arabic", "مرحبا", system.RTL, "Noto Sans Arabic"},
		{"Thai", "สวัสดี", system.LTR, "Noto Sans Thai"},
		{"Devanagari", "नमस्ते", system.LTR, "Noto Sans Devanagari"},
		{"Bengali", "ওহে", system.LTR, "Noto Sans Bengali"},
		{"Tamil", "வணக்கம்", system.LTR, "Noto Sans Tamil"},
		{"Telugu", "హలో", system.LTR, "Noto Sans Telugu"},
		{"Kannada", "ಹಲೋ", system.LTR, "Noto Sans Kannada"},
		{"Malayalam", "ഹലോ", system.LTR, "Noto Sans Malayalam"},
		{"Gujarati", "નમસ્તે", system.LTR, "Noto Sans Gujarati"},
		{"Gurmukhi", "ਸਤਿਸ੍ਰੀ", system.LTR, "Noto Sans Gurmukhi"},
		{"Sinhala", "ආයුබෝවන්", system.LTR, "Noto Sans Sinhala"},
		{"Georgian", "გამარჯობა", system.LTR, "Noto Sans Georgian"},
		{"Armenian", "Բարեւ", system.LTR, "Noto Sans Armenian"},
		{"Khmer", "សួស្តី", system.LTR, "Noto Sans Khmer"},
		{"Lao", "ສະບາຍດີ", system.LTR, "Noto Sans Lao"},
		{"Myanmar", "မင်္ဂလာပါ", system.LTR, "Noto Sans Myanmar"},
		{"Ethiopic", "ሰላም", system.LTR, "Noto Sans Ethiopic"},
		{"Han", "你好世界", system.LTR, "Noto Sans CJK SC"},
		{"Japanese", "こんにちは", system.LTR, "Noto Sans CJK SC"},
		{"Korean", "안녕하세요", system.LTR, "Noto Sans CJK SC"},
		{"Emoji", "🙂🚀🎉", system.LTR, "Noto Color Emoji"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newLazyProbe(t)
			adv, primary := p.shape(tc.s, tc.dir)
			if adv == 0 {
				t.Fatalf("%s: zero advance", tc.name)
			}
			if primary > 0 {
				t.Errorf("%s: %d glyph(s) fell back to a primary face (tofu)", tc.name, primary)
			}
			if p.loaded[tc.want] != 1 {
				t.Fatalf("%s: want %q loaded once, got %v", tc.name, tc.want, p.loaded)
			}
			if len(p.loaded) != 1 {
				t.Errorf("%s: extra faces loaded: %v", tc.name, p.loaded)
			}
		})
	}
}

func TestLazyFontsLoadAllWhenRuneIsUnclaimed(t *testing.T) {
	p := newLazyProbe(t)
	// Runic is claimed by no range and covered by no embedded face; the
	// safety net must still consult every deferred face before giving up.
	p.shape("ᚠᚢᚦ", system.LTR)
	if got, want := len(p.loaded), len(AppLazyFontFaces()); got != want {
		t.Fatalf("unclaimed rune loaded %d faces, want all %d: %v", got, want, p.loaded)
	}
}

func TestLazyFontsMatchEagerCoverage(t *testing.T) {
	eager := AppFontCollection()
	full := append([]font.FontFace{}, eager...)
	for _, lf := range AppLazyFontFaces() {
		ff, err := lf.Load()
		if err != nil {
			t.Fatalf("load %s: %v", lf.Typeface, err)
		}
		full = append(full, ff)
	}
	eagerShaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(full))

	samples := []struct {
		s   string
		dir system.TextDirection
	}{
		{"שלום", system.RTL}, {"مرحبا", system.RTL}, {"สวัสดี", system.LTR},
		{"नमस्ते", system.LTR}, {"你好世界", system.LTR}, {"안녕하세요", system.LTR},
		{"🙂🚀", system.LTR}, {"Привет", system.LTR}, {"hello", system.LTR},
	}
	for _, sm := range samples {
		p := newLazyProbe(t)
		lazyAdv, _ := p.shape(sm.s, sm.dir)

		var eagerAdv fixed.Int26_6
		eagerShaper.LayoutString(text.Parameters{
			PxPerEm:  fixed.I(20),
			MaxWidth: 1 << 20,
			Locale:   system.Locale{Language: "en", Direction: sm.dir},
			Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
		}, sm.s)
		for {
			g, ok := eagerShaper.NextGlyph()
			if !ok {
				break
			}
			if g.Advance == 0 && g.Runes == 0 {
				continue
			}
			eagerAdv += g.Advance
		}
		if lazyAdv != eagerAdv {
			t.Errorf("%q: lazy advance %v != eager advance %v", sm.s, lazyAdv, eagerAdv)
		}
	}
}

func buildFullShaper(t *testing.T) (*text.Shaper, int) {
	var fonts []font.FontFace

	parse := func(b []byte) opentype.Face {
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return face
	}
	subset := func(name string) []byte {
		b, err := LoadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		s, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		return s
	}
	addUI := func(name string) {
		face := parse(subset(name))
		fonts = append(fonts, font.FontFace{Font: face.Font(), Face: face})
	}
	addJBM := func(name string, style font.Style, weight font.Weight) {
		face := parse(subset(name))
		fn := font.Font{Typeface: widgets.MonoFamilyName, Style: style, Weight: weight}
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
	}

	addUI("Inter-Regular.ttf")
	addUI("Inter-Bold.ttf")
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)

	emoji := parse(mustEmbed(t, "NotoColorEmoji.ttf"))
	fonts = append(fonts, font.FontFace{Font: emoji.Font(), Face: emoji})

	firstFallback := len(fonts)
	for _, name := range FallbackFontFiles() {
		face := parse(mustEmbed(t, name))
		fonts = append(fonts, font.FontFace{Font: face.Font(), Face: face})
	}

	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(fonts)), firstFallback
}

func mustEmbed(t *testing.T, name string) []byte {
	b, err := LoadEmbeddedTTF(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return b
}

func TestScriptFallbackCoverage(t *testing.T) {
	shaper, firstFallback := buildFullShaper(t)

	cases := []struct {
		name string
		s    string
		dir  system.TextDirection
	}{
		{"Hebrew", "שלום", system.RTL},
		{"Arabic", "مرحبا", system.RTL},
		{"Thai", "สวัสดี", system.LTR},
		{"Devanagari", "नमस्ते", system.LTR},
		{"Bengali", "ওহে", system.LTR},
		{"Tamil", "வணக்கம்", system.LTR},
		{"Telugu", "హలో", system.LTR},
		{"Kannada", "ಹಲೋ", system.LTR},
		{"Malayalam", "ഹലോ", system.LTR},
		{"Gujarati", "નમસ્તે", system.LTR},
		{"Gurmukhi", "ਸਤਿਸ੍ਰੀ", system.LTR},
		{"Sinhala", "ආයුබෝවන්", system.LTR},
		{"Georgian", "გამარჯობა", system.LTR},
		{"Armenian", "Բարեւ", system.LTR},
		{"Khmer", "សួស្តី", system.LTR},
		{"Lao", "ສະບາຍດີ", system.LTR},
		{"Myanmar", "မင်္ဂလာပါ", system.LTR},
		{"Ethiopic", "ሰላም", system.LTR},
		{"Han", "你好世界", system.LTR},
		{"Japanese", "こんにちは", system.LTR},
		{"Korean", "안녕하세요", system.LTR},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shaper.LayoutString(text.Parameters{
				PxPerEm:  fixed.I(20),
				MaxWidth: 1 << 20,
				Locale:   system.Locale{Language: "en", Direction: tc.dir},
				Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
			}, tc.s)

			var adv fixed.Int26_6
			seen := map[int]int{}
			primary := 0
			for {
				g, ok := shaper.NextGlyph()
				if !ok {
					break
				}
				if g.Advance == 0 && g.Runes == 0 {
					continue
				}
				adv += g.Advance
				idx := faceIdxFromGlyph(uint64(g.ID))
				seen[idx]++
				if idx < firstFallback {
					primary++
				}
			}
			t.Logf("%-12s faces=%v advance=%v", tc.name, seen, adv)
			if adv == 0 {
				t.Fatalf("%s: zero advance", tc.name)
			}
			if primary > 0 {
				t.Errorf("%s: %d glyph(s) resolved to primary faces (idx<%d) = tofu/.notdef; script not covered: %v",
					tc.name, primary, firstFallback, seen)
			}
		})
	}
}

func TestNetlimitSectionWiring(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(1024, 768)),
	}

	ui.SidebarSection = "netlimit"
	ui.LayoutApp(gtx)

	ui.LayoutNetlimitBody(gtx)
	ui.LayoutNetlimitSection(gtx)
	ui.WireNetTitlebar()

	if !ui.Net.Started() {
		t.Fatal("netlimit manager not started")
	}
}

func respHTMLLike() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		if i%12 == 5 {
			for j := 0; j < 70; j++ {
				fmt.Fprintf(&b, `<meta name="tag%d" content="value value" />`, j)
			}
		} else {
			fmt.Fprintf(&b, `<div class="row item-%d"> <span>content word here</span></div>`, i)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (rig *appSliderRig) findSideSplit(t *testing.T, y, from, to int) int {
	tab := rig.ui.Tabs[0]
	for x := from; x < to; x++ {
		before := tab.SplitRatio
		rig.press(x, float32(y))
		rig.movePt(float32(x+8), float32(y))
		changed := tab.SplitRatio != before
		rig.release(x+8, float32(y))
		if changed {
			tab.SplitRatio = before
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return x
		}
	}
	t.Fatalf("side split divider not found in x [%d,%d) at y=%d", from, to, y)
	return 0
}

func newSideRespRig(t *testing.T) *appSliderRig {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.LayoutMode = workspace.LayoutModeHoriz
	tab.WrapEnabled = true
	tab.PreviewEnabled = true
	tab.RespEditor.SetText(respHTMLLike())
	for i := 0; i < 5; i++ {
		rig.frame()
	}
	return rig
}

func (rig *appSliderRig) dragSideSplit(t *testing.T, steps int) {
	tab := rig.ui.Tabs[0]
	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	r0 := tab.SplitRatio
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < steps; i++ {
		pos -= 2
		rig.movePt(pos, float32(y))
	}
	for i := 0; i < steps; i++ {
		pos += 2
		rig.movePt(pos, float32(y))
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	t.Logf("split drag: x=%d ratio %.4f -> %.4f", x0, r0, tab.SplitRatio)
}

func TestSideSplitDragKeepsResponseBottom(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	for i := 0; i < 5; i++ {
		tab.RespEditor.SetScrollY(1 << 30)
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < 100; i++ {
		if i < 50 {
			pos -= 2
		} else {
			pos += 2
		}
		rig.movePt(pos, float32(y))
		if g := tab.RespEditor.BottomGap(); g != 0 {
			t.Fatalf("step %d: response left the bottom mid-drag, gap=%dpx", i, g)
		}
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Errorf("after side-split drag at bottom the response drifted: gap=%dpx (scrollY=%d)",
			g, tab.RespEditor.GetScrollY())
	}
}

func TestSideSplitSlowDragKeepsBottomStable(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	for i := 0; i < 5; i++ {
		tab.RespEditor.SetScrollY(1 << 30)
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	prevY := tab.RespEditor.GetScrollY()
	for i := 0; i < 60; i++ {
		if i < 30 {
			pos -= 1
		} else {
			pos += 1
		}
		rig.movePt(pos, float32(y))
		stepY := tab.RespEditor.GetScrollY()
		for k := 0; k < 3; k++ {
			rig.frame()
			if g := tab.RespEditor.BottomGap(); g != 0 {
				t.Fatalf("slow step %d idle %d: gap=%dpx (scrollY %d -> %d)",
					i, k, g, prevY, tab.RespEditor.GetScrollY())
			}
			if yNow := tab.RespEditor.GetScrollY(); yNow != stepY {
				t.Fatalf("slow step %d idle %d: scrollY oscillated %d -> %d with no geometry change",
					i, k, stepY, yNow)
			}
		}
		prevY = tab.RespEditor.GetScrollY()
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if g := tab.RespEditor.BottomGap(); g != 0 {
		t.Errorf("after slow drag at bottom: gap=%dpx", g)
	}
}

func TestSideSplitSlowDragKeepsMidStable(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	tab.RespEditor.SetScrollY(tab.RespEditor.GetScrollBounds().Max.Y / 2)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off0 := tab.RespEditor.TopContentOffset()
	if off0 == 0 {
		t.Fatalf("setup: expected mid-document")
	}

	y := 400
	x0 := rig.findSideSplit(t, y, 300, 900)
	rig.press(x0, float32(y))
	pos := float32(x0)
	for i := 0; i < 60; i++ {
		if i < 30 {
			pos -= 1
		} else {
			pos += 1
		}
		rig.movePt(pos, float32(y))
		for k := 0; k < 3; k++ {
			rig.frame()
			off := tab.RespEditor.TopContentOffset()
			if d := off - off0; d < -220 || d > 220 {
				t.Fatalf("slow step %d idle %d: top content moved %d bytes (%d -> %d)", i, k, d, off0, off)
			}
		}
	}
	rig.release(int(pos), float32(y))
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off1 := tab.RespEditor.TopContentOffset()
	if d := off1 - off0; d < -220 || d > 220 {
		t.Errorf("slow round-trip moved response content: top offset %d -> %d", off0, off1)
	}
}

func TestSideSplitDragKeepsResponseMidPosition(t *testing.T) {
	rig := newSideRespRig(t)
	tab := rig.ui.Tabs[0]

	tab.RespEditor.SetScrollY(tab.RespEditor.GetScrollBounds().Max.Y / 2)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	off0 := tab.RespEditor.TopContentOffset()
	if off0 == 0 {
		t.Fatalf("setup: expected mid-document")
	}

	rig.dragSideSplit(t, 50)

	off1 := tab.RespEditor.TopContentOffset()
	if d := off1 - off0; d < -200 || d > 200 {
		t.Errorf("round-trip side-split drag moved response content: top offset %d -> %d (delta %d bytes)",
			off0, off1, d)
	}
}

type scriptDragRig struct {
	t  *testing.T
	ui *AppUI
	r  input.Router
	sz image.Point
}

func newScriptDragRig(t *testing.T, names ...string) *scriptDragRig {
	t.Helper()
	setupTestConfigDir(t)
	dir := persist.FlowsDir()
	for i, name := range names {
		data := fmt.Sprintf(`{"id":"s%d","name":%q,"nodes":[]}`, i+1, name)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("s%d.json", i+1)), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ScriptsExpanded = true
	rig := &scriptDragRig{t: t, ui: ui, sz: image.Pt(900, 600)}
	rig.frames(3)
	return rig
}

func (rig *scriptDragRig) frame(events ...pointer.Event) *op.Ops {
	ops := new(op.Ops)
	for _, e := range events {
		rig.r.Queue(e)
	}
	rig.ui.LayoutApp(layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	})
	rig.r.Frame(ops)
	return ops
}

func (rig *scriptDragRig) frames(n int) {
	for i := 0; i < n; i++ {
		rig.frame()
	}
}

func (rig *scriptDragRig) rowY(idx int) float32 {
	h := rig.ui.ScriptRowH()
	if h <= 0 {
		rig.t.Fatal("script row height must be measured before driving the drag")
	}
	titleBarH := 30
	return float32(titleBarH + rig.ui.ScriptsDivY() + 1 + 26 + idx*h + h/2)
}

func (rig *scriptDragRig) rowIDs() []string {
	var out []string
	for _, r := range rig.ui.ScriptRows {
		out = append(out, r.ID)
	}
	return out
}

func press(x, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}
}

func dragTo(x, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(x, y)}
}

func release(x, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(x, y)}
}

func TestScriptDragReordersAndPersists(t *testing.T) {
	rig := newScriptDragRig(t, "One", "Two", "Three")
	before := rig.rowIDs()
	if len(before) != 3 {
		t.Fatalf("expected 3 script rows, got %v", before)
	}
	first := before[0]
	h := float32(rig.ui.ScriptRowH())
	x := float32(100)
	y := rig.rowY(0)

	rig.frame(press(x, y))
	rig.frame(dragTo(x, y+10))
	rig.frame(dragTo(x, y+10+2*h))
	rig.frame(release(x, y+10+2*h))
	rig.frames(2)

	after := rig.rowIDs()
	if after[2] != first || after[0] == first {
		t.Fatalf("dragging the first row two rows down should make it last: before %v after %v", before, after)
	}
	var stored []string
	for _, inf := range flow.ListScenarios() {
		stored = append(stored, inf.ID)
	}
	if fmt.Sprint(stored) != fmt.Sprint(after) {
		t.Fatalf("persisted order %v must match the sidebar order %v", stored, after)
	}

	if err := flow.RenameScenario(after[1], "Renamed"); err != nil {
		t.Fatal(err)
	}
	rig.frames(2)
	if got := rig.rowIDs(); fmt.Sprint(got) != fmt.Sprint(after) {
		t.Errorf("renaming (which rewrites the file) must not move the row: %v, want %v", got, after)
	}
}

func TestScriptClickWithoutDragKeepsOrder(t *testing.T) {
	rig := newScriptDragRig(t, "One", "Two")
	before := rig.rowIDs()
	x := float32(100)
	y := rig.rowY(1)
	rig.frame(press(x, y))
	rig.frame(dragTo(x, y+2))
	rig.frame(release(x, y+2))
	rig.frames(2)
	if got := rig.rowIDs(); fmt.Sprint(got) != fmt.Sprint(before) {
		t.Errorf("a click with sub-slop jitter must not reorder: %v, want %v", got, before)
	}
	if _, err := os.Stat(filepath.Join(persist.FlowsDir(), "order.json")); err == nil {
		t.Error("no order file should be written without a real drag")
	}
}

func TestSplitDragAppliesToAllTabs(t *testing.T) {
	rig := newAppSliderRig(t)
	active := rig.ui.Tabs[0]

	other := workspace.NewRequestTab("T2")
	other.Method = "GET"
	other.URLInput.SetText("http://other.example.com")
	other.AddHeader("X-Other", "1")
	rig.ui.Tabs = append(rig.ui.Tabs, other)

	for i := 0; i < 4; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)

	before := other.VStackRatio
	rig.press(x, float32(divY))
	rig.move(x, float32(divY-40))
	rig.release(x, float32(divY-40))

	if active.VStackRatio == before {
		t.Fatalf("split drag did not move the active tab ratio (%v)", active.VStackRatio)
	}
	if other.VStackRatio != active.VStackRatio {
		t.Errorf("split ratio not shared: active=%v other=%v", active.VStackRatio, other.VStackRatio)
	}

	beforeH := other.HeadersAbsHeight
	rig.press(x, float32(sliderY))
	rig.move(x, float32(sliderY+40))
	rig.release(x, float32(sliderY+40))

	if active.HeadersAbsHeight == beforeH {
		t.Fatalf("headers drag did not resize the active tab (%d)", active.HeadersAbsHeight)
	}
	if other.HeadersAbsHeight != active.HeadersAbsHeight {
		t.Errorf("headers height not shared: active=%d other=%d", active.HeadersAbsHeight, other.HeadersAbsHeight)
	}

	active.LayoutMode = workspace.LayoutModeHoriz
	rig.frame()
	if other.LayoutMode != workspace.LayoutModeHoriz {
		t.Errorf("layout mode not shared: other=%d", other.LayoutMode)
	}
}

func (rig *appSliderRig) findSidebarHandle(t *testing.T, y int) int {
	for x := 100; x < 700; x++ {
		before := rig.ui.SidebarWidth
		rig.press(x, float32(y))
		rig.move(x+8, float32(y))
		changed := rig.ui.SidebarWidth != before
		rig.release(x+8, float32(y))
		if changed {
			rig.ui.SidebarWidth = before
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return x
		}
	}
	t.Fatalf("sidebar width handle not found")
	return 0
}

func TestSidebarWidthDragSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	y := 400
	x := rig.findSidebarHandle(t, y)
	t.Logf("handle at x=%d width=%d", x, rig.ui.SidebarWidth)

	rig.press(x, float32(y))
	pos := float32(x)
	var widths []int
	for i := 0; i < 40; i++ {
		pos += 1
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	steady := len(widths)
	for i := 0; i < 160; i++ {
		pos -= 1
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	past := len(widths)
	tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
	for i := 0; i < 18; i++ {
		pos += tremor[i%len(tremor)]
		rig.movePt(pos, float32(y))
		widths = append(widths, rig.ui.SidebarWidth)
	}
	rig.release(int(pos), float32(y))
	t.Logf("widths=%v", widths)

	for i := 1; i < steady; i++ {
		if widths[i] < widths[i-1] {
			t.Fatalf("width rolled back while growing on step %d: %d -> %d", i, widths[i-1], widths[i])
		}
	}
	for i := steady + 1; i < past; i++ {
		if widths[i] > widths[i-1] {
			t.Fatalf("width rolled back while shrinking on step %d: %d -> %d", i, widths[i-1], widths[i])
		}
	}
	min := widths[past-1]
	for i := past; i < len(widths); i++ {
		if widths[i] != min {
			t.Fatalf("tremor past the min moved sidebar on step %d: %d -> %d", i, min, widths[i])
		}
	}
}

func TestSidebarEnvSectionDragSmooth(t *testing.T) {
	rig := newAppSliderRig(t)
	rig.ui.ColsExpanded = true
	rig.ui.EnvsExpanded = true
	rig.ui.ScriptsExpanded = true
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	x := 150
	divY := -1
	base := rig.ui.EnvDivY()
	if base <= 0 {
		t.Fatalf("env divider offset not recorded")
	}
	for off := 10; off < 60; off++ {
		y := base + off
		if y >= rig.sz.Y {
			break
		}
		rig.press(x, float32(y))
		grabbed := rig.ui.SidebarEnvDrag.Pressed()
		rig.release(x, float32(y))
		rig.ui.ColsExpanded = true
		rig.ui.EnvsExpanded = true
		rig.ui.ScriptsExpanded = true
		rig.frame()
		if grabbed {
			divY = y
			break
		}
	}
	if divY < 0 {
		t.Fatalf("env divider not found near offset %d", base)
	}
	t.Logf("env divider at y=%d envH=%d", divY, rig.ui.SidebarEnvHeight)

	rig.press(x, float32(divY))
	if !rig.ui.SidebarEnvDrag.Pressed() {
		t.Fatalf("env drag did not grab at y=%d", divY)
	}
	pos := float32(divY)
	var heights []int
	for i := 0; i < 50; i++ {
		pos -= 1
		rig.movePt(float32(x), pos)
		heights = append(heights, rig.ui.SidebarEnvHeight)
	}
	grow := len(heights)
	for i := 0; i < 50; i++ {
		pos += 1
		rig.movePt(float32(x), pos)
		heights = append(heights, rig.ui.SidebarEnvHeight)
	}
	rig.release(x, pos)
	t.Logf("heights=%v", heights)

	for i := 1; i < grow; i++ {
		if heights[i] < heights[i-1] {
			t.Fatalf("env height rolled back while growing on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
		if heights[i]-heights[i-1] > 2 {
			t.Fatalf("env height jumped on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
	}
	for i := grow + 1; i < len(heights); i++ {
		if heights[i] > heights[i-1] {
			t.Fatalf("env height rolled back while shrinking on step %d: %d -> %d", i, heights[i-1], heights[i])
		}
	}
}

type dropGapRig struct {
	ui  *AppUI
	r   input.Router
	sz  image.Point
	now time.Time
}

func newDropGapRig(t *testing.T) (*dropGapRig, *collections.CollectionNode) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	folder := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, IsFolder: true, Expanded: true}
	}
	root := folder("API")
	f1, f2, f3 := folder("folder 1"), folder("folder 2"), folder("folder 3")
	c1, c2 := folder("child 1"), folder("child 2")
	f1.Children = []*collections.CollectionNode{c1}
	f2.Children = []*collections.CollectionNode{c2}
	root.Children = []*collections.CollectionNode{f1, f2, f3}
	col := &collections.ParsedCollection{ID: "c1", Name: "API", Root: root}
	collections.AssignParents(root, nil, col)
	var depth func(n *collections.CollectionNode, d int)
	depth = func(n *collections.CollectionNode, d int) {
		n.Depth = d
		for _, c := range n.Children {
			depth(c, d+1)
		}
	}
	depth(root, 0)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	return &dropGapRig{ui: ui, sz: image.Pt(1100, 800), now: time.Unix(1700000000, 0)}, root
}

func (rig *dropGapRig) frame() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(ops)
}

func (rig *dropGapRig) send(ev pointer.Event) {
	ev.Source = pointer.Mouse
	rig.r.Queue(ev)
	rig.frame()
}

// rowCenters walks the sidebar with hover moves and reports each visible
// node's on-screen center Y, found the same way a user finds a row: by
// pointing at it.
func (rig *dropGapRig) rowCenters(t *testing.T, x float32) map[string]float32 {
	t.Helper()
	tops := map[string]float32{}
	bottoms := map[string]float32{}
	for y := float32(20); y < 500; y += 2 {
		rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y)})
		for _, n := range rig.ui.VisibleCols {
			if n.RowHovered {
				if _, seen := tops[n.Name]; !seen {
					tops[n.Name] = y
				}
				bottoms[n.Name] = y
			}
		}
	}
	centers := map[string]float32{}
	for name, top := range tops {
		centers[name] = (top + bottoms[name]) / 2
	}
	return centers
}

// problems.md #2: with folder 1 and folder 2 both expanded, folder 3 dragged
// into the gap between them always fell inside one of them; it must be
// possible to drop it between the expanded folders.
func TestSidebarRealDragBetweenExpandedFolders(t *testing.T) {
	rig, root := newDropGapRig(t)
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	const x = 120
	centers := rig.rowCenters(t, x)
	for _, name := range []string{"API", "folder 1", "child 1", "folder 2", "child 2", "folder 3"} {
		if _, ok := centers[name]; !ok {
			t.Fatalf("row %q not found by hover scan; got %v", name, centers)
		}
	}

	// Grab folder 3 by its label and pull it straight up into the boundary
	// between child 1 (end of folder 1's subtree) and folder 2, without any
	// horizontal travel.
	src := centers["folder 3"]
	dst := (centers["child 1"] + centers["folder 2"]) / 2
	rig.send(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, src), Buttons: pointer.ButtonPrimary})
	for y := src; y > dst; y -= 4 {
		rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary})
	}
	rig.send(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, dst), Buttons: pointer.ButtonPrimary})
	rig.send(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, dst)})
	rig.frame()

	names := make([]string, 0, len(root.Children))
	for _, c := range root.Children {
		names = append(names, c.Name)
	}
	if len(names) != 3 || names[0] != "folder 1" || names[1] != "folder 3" || names[2] != "folder 2" {
		t.Fatalf("root children = %v, want [folder 1, folder 3, folder 2]", names)
	}
	f3 := root.Children[1]
	if f3.Depth != 1 || f3.Parent != root {
		t.Fatalf("folder 3 nested wrongly: depth=%d parent=%q", f3.Depth, f3.Parent.Name)
	}
	if len(root.Children[0].Children) != 1 || len(root.Children[2].Children) != 1 {
		t.Fatalf("sibling folders must be untouched: f1=%d f2=%d children",
			len(root.Children[0].Children), len(root.Children[2].Children))
	}
}

func TestSidebarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	col := &collections.ParsedCollection{
		ID:   "c1",
		Name: "C1",
		Root: &collections.CollectionNode{
			Name:     "R1",
			IsFolder: true,
			Expanded: true,
			Children: []*collections.CollectionNode{
				{
					Name: "Child",
					Request: &model.ParsedRequest{
						Method: "GET",
					},
				},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col

	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: col})
	ui.UpdateVisibleCols()

	env := &model.ParsedEnvironment{
		ID:   "e1",
		Name: "E1",
	}
	ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: env})

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(300, 768)),
		Now:         time.Now(),
	}

	ui.LayoutSidebar(gtx)

	ui.ColsExpanded = false
	ui.LayoutSidebar(gtx)
	ui.ColsExpanded = true

	node := ui.VisibleCols[1]
	node.MenuOpen = true
	ui.LayoutSidebar(gtx)
	node.MenuOpen = false

	ui.ActiveEnvID = "e1"
	ui.LayoutSidebar(gtx)
	ui.ActiveEnvID = ""
	ui.LayoutSidebar(gtx)

	ui.EditingEnv = ui.Environments[0]
	ui.LayoutSidebar(gtx)
}

func TestSidebar_FolderCreation(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	col := &collections.ParsedCollection{
		ID: "c1",
		Root: &collections.CollectionNode{
			Name:     "Root",
			IsFolder: true,
		},
	}
	col.Root.Collection = col
	ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: col})
	ui.UpdateVisibleCols()

	newNode := collections.CloneNode(col.Root, nil)
	if newNode.Name != "Root Copy" {
		t.Errorf("expected Root Copy, got %s", newNode.Name)
	}
}

type appSliderRig struct {
	ui    *AppUI
	r     input.Router
	sz    image.Point
	now   time.Time
	scale float32
}

func newAppSliderRig(t *testing.T) *appSliderRig {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("T")}
	ui.ActiveIdx = 0
	tab := ui.Tabs[0]
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
	tab.AddHeader("Authorization", "secret")
	tab.LayoutMode = workspace.LayoutModeVert
	tab.HeadersExpanded = true
	tab.HeadersAbsHeight = 100
	return &appSliderRig{ui: ui, sz: image.Pt(1100, 800), now: time.Unix(1700000000, 0)}
}

func (rig *appSliderRig) frame() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	scale := rig.scale
	if scale <= 0 {
		scale = 1
	}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.sz),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(ops)
}

func (rig *appSliderRig) press(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) move(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) movePt(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) release(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(float32(x), y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *appSliderRig) findHeadersSlider(t *testing.T, x int) int {
	tab := rig.ui.Tabs[0]
	for y := 80; y < 700; y++ {
		before := tab.HeadersAbsHeight
		rig.press(x, float32(y))
		rig.move(x, float32(y+8))
		changed := tab.HeadersAbsHeight != before
		rig.release(x, float32(y+8))
		if changed {
			tab.HeadersAbsHeight = 100
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return y
		}
		if tab.HeadersAbsHeight != before {
			tab.HeadersAbsHeight = 100
			for i := 0; i < 3; i++ {
				rig.frame()
			}
		}
	}
	t.Fatalf("headers slider not found")
	return 0
}

func TestHeadersSliderFullAppNoJitter(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.008
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	t.Logf("slider found at y=%d, ratio=%.4f stored=%d", sliderY, tab.VStackRatio, tab.HeadersAbsHeight)

	rig.press(x, float32(sliderY))
	var ratios []float32
	var stores []int
	pos := float32(sliderY)
	for i := 1; i <= 25; i++ {
		pos += 1
		rig.move(x, pos)
		ratios = append(ratios, tab.VStackRatio)
		stores = append(stores, tab.HeadersAbsHeight)
	}
	for i := 1; i <= 25; i++ {
		pos -= 1
		rig.move(x, pos)
		ratios = append(ratios, tab.VStackRatio)
		stores = append(stores, tab.HeadersAbsHeight)
	}
	rig.release(x, pos)
	t.Logf("stores=%v", stores)
	t.Logf("ratios(x1e4)=%v", func() []int {
		out := make([]int, len(ratios))
		for i, r := range ratios {
			out[i] = int(r * 10000)
		}
		return out
	}())

	for i := 1; i < 25; i++ {
		if stores[i] < stores[i-1] {
			t.Fatalf("stored jitter on down step %d: %d -> %d", i, stores[i-1], stores[i])
		}
		if ratios[i] < ratios[i-1]-0.0001 {
			t.Fatalf("ratio jitter on down step %d: %.5f -> %.5f", i, ratios[i-1], ratios[i])
		}
	}
	for i := 26; i < 50; i++ {
		if stores[i] > stores[i-1] {
			t.Fatalf("stored jitter on up step %d: %d -> %d", i, stores[i-1], stores[i])
		}
		if ratios[i] > ratios[i-1]+0.0001 {
			t.Fatalf("ratio jitter on up step %d: %.5f -> %.5f", i, ratios[i-1], ratios[i])
		}
	}
}

func (rig *appSliderRig) findMainSplit(t *testing.T, x, from, to int) int {
	tab := rig.ui.Tabs[0]
	for y := from; y < to; y++ {
		before := tab.VStackRatio
		beforeH := tab.HeadersAbsHeight
		beforeExp := tab.HeadersExpanded
		beforeCol := tab.ReqBodyCollapsed
		rig.press(x, float32(y))
		rig.move(x, float32(y+8))
		ratioChanged := tab.VStackRatio != before
		otherChanged := tab.HeadersAbsHeight != beforeH || tab.HeadersExpanded != beforeExp
		rig.release(x, float32(y+8))
		if ratioChanged || otherChanged || tab.ReqBodyCollapsed != beforeCol {
			tab.VStackRatio = before
			tab.HeadersAbsHeight = beforeH
			tab.HeadersExpanded = beforeExp
			tab.ReqBodyCollapsed = beforeCol
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if ratioChanged && !otherChanged {
				return y
			}
		}
	}
	t.Fatalf("main split divider not found in [%d,%d)", from, to)
	return 0
}

func TestMainSplitNoJitterNearCollapse(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.008
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)
	t.Logf("slider=%d divider=%d ratio=%.4f", sliderY, divY, tab.VStackRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var collapsed []bool
	pos := float32(divY)
	for i := 1; i <= 20; i++ {
		pos -= 1
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	for i := 1; i <= 20; i++ {
		pos += 1
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	tremor := []float32{0.4, -0.3, 0.45, -0.4, 0.35, -0.45}
	for i := 0; i < 24; i++ {
		pos += tremor[i%len(tremor)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < 20; i++ {
		if ratios[i] > ratios[i-1] {
			t.Fatalf("ratio jitter on up step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	for i := 21; i < 40; i++ {
		if ratios[i] < ratios[i-1] {
			t.Fatalf("ratio jitter on down step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	base := ratios[40]
	flips := 0
	for i := 41; i < len(ratios); i++ {
		if ratios[i] != ratios[i-1] {
			flips++
		}
		if diff := ratios[i] - base; diff < -25 || diff > 25 {
			t.Fatalf("tremor moved the split too far on step %d: %d vs base %d", i, ratios[i], base)
		}
	}
	if flips > 4 {
		t.Fatalf("split oscillated %d times under pointer tremor", flips)
	}
	for i := 41; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			t.Fatalf("collapse state flapped under tremor at step %d", i)
		}
	}
}

func TestMainSplitSlowShrinkUserState(t *testing.T) {
	rig := newAppSliderRig(t)
	rig.sz.X, rig.sz.Y = 1920, 1040
	tab := rig.ui.Tabs[0]
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com/api")
	tab.ReqEditor.SetText("")
	tab.Headers = nil
	tab.HeadersExpanded = true
	tab.HeadersAbsHeight = 16
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.031
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 900
	divY := rig.findMainSplit(t, x, 100, 900)
	t.Logf("divider=%d ratio=%.4f minRatio=%.4f", divY, tab.VStackRatio, minRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var drawn []int
	var collapsed []bool
	pos := float32(divY)
	steps := []float32{0.4, 0.7, 0.3, 1.1, 0.5, 0.9, 0.4, 1.6}
	for i := 0; i < 80; i++ {
		pos -= steps[i%len(steps)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		drawn = append(drawn, tab.PaneDrawnH)
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("drawn=%v", drawn)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < len(ratios); i++ {
		if ratios[i] > ratios[i-1] {
			t.Errorf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
		if drawn[i] > drawn[i-1] {
			t.Errorf("drawn pane grew on step %d: %d -> %d", i, drawn[i-1], drawn[i])
		}
	}
	changes := 0
	for i := 1; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			changes++
		}
	}
	if changes > 1 {
		t.Errorf("collapse state flapped %d times", changes)
	}
}

func TestMainSplitSlowShrinkMatrix(t *testing.T) {
	for _, sc := range []float32{1, 1.25, 1.5} {
		for _, headers := range []bool{true, false} {
			t.Run(fmt.Sprintf("scale=%.2f headers=%v", sc, headers), func(t *testing.T) {
				rig := newAppSliderRig(t)
				rig.scale = sc
				tab := rig.ui.Tabs[0]
				tab.HeadersExpanded = headers
				for i := 0; i < 4; i++ {
					rig.frame()
				}

				tab.VStackRatio = 0.01
				for i := 0; i < 3; i++ {
					rig.frame()
				}
				minRatio := tab.VStackRatio
				tab.VStackRatio = minRatio + 0.047
				for i := 0; i < 3; i++ {
					rig.frame()
				}

				x := 700
				divY := rig.findMainSplit(t, x, 100, 750)
				t.Logf("divider=%d ratio=%.4f minRatio=%.4f", divY, tab.VStackRatio, minRatio)

				rig.press(x, float32(divY))
				var ratios []int
				var drawn []int
				var collapsed []bool
				pos := float32(divY)
				steps := []float32{0.4, 0.7, 0.3, 1.1, 0.5, 0.9, 0.4, 1.6}
				for i := 0; i < 90; i++ {
					pos -= steps[i%len(steps)]
					rig.move(x, pos)
					ratios = append(ratios, int(tab.VStackRatio*10000))
					drawn = append(drawn, tab.PaneDrawnH)
					collapsed = append(collapsed, tab.ReqBodyCollapsed)
				}
				steady := len(ratios)
				tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
				for i := 0; i < 18; i++ {
					pos += tremor[i%len(tremor)]
					rig.move(x, pos)
					ratios = append(ratios, int(tab.VStackRatio*10000))
					drawn = append(drawn, tab.PaneDrawnH)
					collapsed = append(collapsed, tab.ReqBodyCollapsed)
				}
				rig.release(x, pos)
				t.Logf("ratios(x1e4)=%v", ratios)
				t.Logf("drawn=%v", drawn)
				t.Logf("collapsed=%v", collapsed)

				for i := 1; i < steady; i++ {
					if ratios[i] > ratios[i-1] {
						t.Errorf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
					}
					if drawn[i] > drawn[i-1] {
						t.Errorf("drawn pane grew on step %d: %d -> %d", i, drawn[i-1], drawn[i])
					}
				}
				for i := steady; i < len(ratios); i++ {
					if ratios[i] != ratios[steady-1] {
						t.Errorf("tremor past the limit moved the split on step %d: %d -> %d", i, ratios[steady-1], ratios[i])
					}
					if drawn[i] != drawn[steady-1] {
						t.Errorf("tremor past the limit moved drawn pane on step %d: %d -> %d", i, drawn[steady-1], drawn[i])
					}
				}
				changes := 0
				for i := 1; i < len(collapsed); i++ {
					if collapsed[i] != collapsed[i-1] {
						changes++
					}
				}
				if changes > 1 {
					t.Errorf("collapse state flapped %d times", changes)
				}
				if !collapsed[len(collapsed)-1] {
					t.Errorf("request body did not collapse")
				}
			})
		}
	}
}

func TestMainSplitSlowShrinkToCollapse(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.047
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	divY := rig.findMainSplit(t, x, sliderY+30, 700)
	t.Logf("slider=%d divider=%d ratio=%.4f minRatio=%.4f", sliderY, divY, tab.VStackRatio, minRatio)

	rig.press(x, float32(divY))
	var ratios []int
	var collapsed []bool
	pos := float32(divY)
	for i := 0; i < 120; i++ {
		pos -= 0.4
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	steady := len(ratios)
	tremor := []float32{1.2, -1.2, 0.8, -1.1, 1.3, -1.2}
	for i := 0; i < 18; i++ {
		pos += tremor[i%len(tremor)]
		rig.move(x, pos)
		ratios = append(ratios, int(tab.VStackRatio*10000))
		collapsed = append(collapsed, tab.ReqBodyCollapsed)
	}
	rig.release(x, pos)
	t.Logf("ratios(x1e4)=%v", ratios)
	t.Logf("collapsed=%v", collapsed)

	for i := 1; i < steady; i++ {
		if ratios[i] > ratios[i-1] {
			t.Fatalf("ratio rolled back on step %d: %d -> %d", i, ratios[i-1], ratios[i])
		}
	}
	for i := steady; i < len(ratios); i++ {
		if ratios[i] != ratios[steady-1] {
			t.Fatalf("tremor past the limit moved the split on step %d: %d -> %d", i, ratios[steady-1], ratios[i])
		}
	}
	changes := 0
	for i := 1; i < len(collapsed); i++ {
		if collapsed[i] != collapsed[i-1] {
			changes++
		}
	}
	if changes > 1 {
		t.Fatalf("collapse state flapped %d times during slow shrink", changes)
	}
	if !collapsed[len(collapsed)-1] {
		t.Fatalf("request body did not collapse after 48px slow shrink")
	}
}

func TestPaths(t *testing.T) {
	setupTestConfigDir(t)

	cfgPath := persist.ConfigDir()
	if !strings.HasSuffix(cfgPath, "rete-test") {
		t.Errorf("expected config path to end with rete-test, got %s", cfgPath)
	}

	stateFile := persist.StateFilePath()
	if !strings.HasSuffix(stateFile, "state.json") {
		t.Errorf("expected state file to end with state.json, got %s", stateFile)
	}

	colDir := persist.CollectionsDir()
	if !strings.HasSuffix(colDir, "collections") {
		t.Errorf("expected collections dir to end with collections, got %s", colDir)
	}

	envDir := persist.EnvironmentsDir()
	if !strings.HasSuffix(envDir, "environments") {
		t.Errorf("expected environments dir to end with environments, got %s", envDir)
	}
}

func TestLoadStateEmpty(t *testing.T) {
	setupTestConfigDir(t)

	state := persist.Load()
	if len(state.Tabs) != 0 {
		t.Errorf("expected empty state")
	}
}

func TestCollectionsRawAndLoad(t *testing.T) {
	setupTestConfigDir(t)

	cols := collections.LoadAll()
	if len(cols) != 0 {
		t.Errorf("expected 0 collections initially")
	}

	_, err := persist.SaveCollectionRaw([]byte("invalid json"))
	if err != nil {
		t.Errorf("unexpected error on save raw: %v", err)
	}

	cols = collections.LoadAll()
	if len(cols) != 0 {
		t.Errorf("expected 0 collections after invalid save")
	}

	validJSON := `{"info": {"name": "Raw Col"}, "item": []}`
	id, err := persist.SaveCollectionRaw([]byte(validJSON))
	if err != nil || id == "" {
		t.Errorf("failed to save raw")
	}

	cols = collections.LoadAll()
	if len(cols) != 1 {
		t.Errorf("expected 1 collection, got %d", len(cols))
	} else if cols[0].Name != "Raw Col" {
		t.Errorf("expected Raw Col, got %s", cols[0].Name)
	}
}

func TestEnvironmentRawAndLoad(t *testing.T) {
	setupTestConfigDir(t)

	envs := environments.LoadAll()
	if len(envs) != 0 {
		t.Errorf("expected 0 envs initially")
	}

	validJSON := `{"name": "Raw Env", "values": []}`
	id, err := persist.SaveEnvironmentRaw([]byte(validJSON))
	if err != nil || id == "" {
		t.Errorf("failed to save raw env")
	}

	envs = environments.LoadAll()
	if len(envs) != 1 {
		t.Errorf("expected 1 env, got %d", len(envs))
	} else if envs[0].Name != "Raw Env" {
		t.Errorf("expected Raw Env, got %s", envs[0].Name)
	}
}

func TestSaveEnvironmentAndCollection(t *testing.T) {
	setupTestConfigDir(t)

	env := &model.ParsedEnvironment{
		ID:   "env1",
		Name: "Test Env",
		Vars: []model.EnvVar{
			{Key: "k1", Value: "v1"},
		},
	}
	err := persist.SaveEnvironment(env)
	if err != nil {
		t.Errorf("failed to save environment: %v", err)
	}

	envs := environments.LoadAll()
	if len(envs) != 1 || envs[0].ID != "env1" || envs[0].Name != "Test Env" {
		t.Errorf("failed to load saved environment")
	}

	col := &collections.ParsedCollection{
		ID:   "col1",
		Name: "Test Col",
		Root: &collections.CollectionNode{
			Name:     "Test Col",
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{
					Name: "Req1",
					Request: &model.ParsedRequest{
						Method: "GET",
						URL:    "http://example.com",
					},
				},
			},
		},
	}

	err = collections.SaveToFile(col)
	if err != nil {
		t.Errorf("failed to save collection: %v", err)
	}

	cols := collections.LoadAll()
	if len(cols) != 1 || cols[0].ID != "col1" || cols[0].Name != "Test Col" {
		t.Errorf("failed to load saved collection")
	}
	if len(cols[0].Root.Children) != 1 || cols[0].Root.Children[0].Name != "Req1" {
		t.Errorf("collection children not saved properly")
	}
}

func TestSnapshotCollection_EmptyNodes(t *testing.T) {
	col := &collections.ParsedCollection{
		ID:   "c1",
		Name: "C1",
		Root: &collections.CollectionNode{
			Name: "Root",
			Children: []*collections.CollectionNode{
				{Name: "Empty Folder", IsFolder: true},
				{Name: "Nil Req", Request: nil},
			},
		},
	}
	id, data := collections.Snapshot(col)
	if id != "c1" || len(data) == 0 {
		t.Errorf("snapshot returned empty data")
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("snapshot output is not valid JSON: %v", err)
	}
	items, _ := parsed["item"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestWriteCollectionFile_Error(t *testing.T) {
	setupTestConfigDir(t)

	err := persist.WriteCollectionFile("id", nil)
	if err != nil {
		t.Errorf("expected no error for nil ext")
	}
}

func TestStateErrors(t *testing.T) {
	tempDir := setupTestConfigDir(t)

	collections.LoadAll()
	environments.LoadAll()

	_ = os.MkdirAll(filepath.Join(tempDir, "rete"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "rete", "state.json"), []byte("invalid"), 0644)
	persist.Load()

	_ = os.MkdirAll(filepath.Join(tempDir, "rete", "collections"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "rete", "collections", "bad.json"), []byte("invalid"), 0644)
	collections.LoadAll()

	_ = os.MkdirAll(filepath.Join(tempDir, "rete", "collections", "subdir"), 0755)
	collections.LoadAll()
}

func TestGetConfigPath_Error(t *testing.T) {

	t.Setenv("AppData", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	path := persist.ConfigDir()
	if path == "" {
		t.Errorf("expected at least a fallback path")
	}
}

func TestSnapshotCollection(t *testing.T) {
	col := &collections.ParsedCollection{
		ID:   "test-col-id",
		Name: "Test Col",
		Root: &collections.CollectionNode{
			Name:     "Test Col",
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{
					Name:     "Folder 1",
					IsFolder: true,
					Children: []*collections.CollectionNode{
						{
							Name: "Request A",
							Request: &model.ParsedRequest{
								Method:   "POST",
								URL:      "http://api.example.com",
								Body:     "{\"foo\": \"bar\"}",
								BodyType: model.BodyRaw,
								Headers: map[string]string{
									"Content-Type": "application/json",
									"Auth":         "Bearer token",
								},
							},
						},
					},
				},
				{
					Name: "Request B",
					Request: &model.ParsedRequest{
						Method: "GET",
						URL:    "http://api.example.com/b",
					},
				},
			},
		},
	}

	id, data := collections.Snapshot(col)
	if id != "test-col-id" {
		t.Errorf("expected id test-col-id, got %s", id)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty snapshot data")
	}
	var ext model.ExtCollection
	if err := json.Unmarshal(data, &ext); err != nil {
		t.Fatalf("snapshot output is not valid model.ExtCollection JSON: %v", err)
	}
	if ext.Info.Name != "Test Col" {
		t.Errorf("expected name Test Col, got %s", ext.Info.Name)
	}

	if len(ext.Item) != 2 {
		t.Fatalf("expected 2 root items, got %d", len(ext.Item))
	}

	folderItem := ext.Item[0]
	if folderItem.Name != "Folder 1" {
		t.Errorf("expected folder name Folder 1")
	}
	if len(folderItem.Item) != 1 {
		t.Fatalf("expected 1 child in folder, got %d", len(folderItem.Item))
	}
	if len(folderItem.Request) > 0 {
		t.Errorf("expected no request for folder")
	}

	reqAItem := folderItem.Item[0]
	if reqAItem.Name != "Request A" {
		t.Errorf("expected Request A")
	}
	if len(reqAItem.Request) == 0 {
		t.Fatalf("expected request bytes")
	}

	var reqA model.ExtRequest
	if err := json.Unmarshal(reqAItem.Request, &reqA); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}
	if reqA.Method != "POST" {
		t.Errorf("expected POST, got %s", reqA.Method)
	}
	if reqA.URL != "http://api.example.com" {
		t.Errorf("expected url, got %v", reqA.URL)
	}
	if reqA.Body.Mode != "raw" || reqA.Body.Raw != "{\"foo\": \"bar\"}" {
		t.Errorf("unexpected body: %+v", reqA.Body)
	}

	reqBItem := ext.Item[1]
	if reqBItem.Name != "Request B" {
		t.Errorf("expected Request B")
	}
	if len(reqBItem.Request) == 0 {
		t.Fatalf("expected request bytes")
	}
}

func TestSnapshotCollection_Nil(t *testing.T) {
	id, ext := collections.Snapshot(nil)
	if id != "" || ext != nil {
		t.Errorf("expected empty results for nil")
	}

	id, ext = collections.Snapshot(&collections.ParsedCollection{})
	if id != "" || ext != nil {
		t.Errorf("expected empty results for missing root")
	}

	id, ext = collections.Snapshot(&collections.ParsedCollection{Root: &collections.CollectionNode{}})
	if id != "" || ext != nil {
		t.Errorf("expected empty results for missing id")
	}
}

func randStickyName(rng *rand.Rand) string {
	const al = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6+rng.Intn(8))
	for i := range b {
		b[i] = al[rng.Intn(len(al))]
	}
	return string(b)
}

func stickyDepthOf(n *collections.CollectionNode) int {
	d := 0
	for p := n.Parent; p != nil; p = p.Parent {
		d++
	}
	return d
}

func buildRandomStickyTree(rng *rand.Rand, maxDepths []int) *collections.CollectionNode {
	root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
	var grow func(parent *collections.CollectionNode, depth, maxDepth int)
	grow = func(parent *collections.CollectionNode, depth, maxDepth int) {
		for i, n := 0, 2+rng.Intn(5); i < n; i++ {
			parent.Children = append(parent.Children,
				&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
		}
		if depth >= maxDepth {
			return
		}
		for i, n := 0, 1+rng.Intn(2); i < n; i++ {
			f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			parent.Children = append(parent.Children, f)
			grow(f, depth+1, maxDepth)
		}
	}
	for _, md := range maxDepths {
		f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		root.Children = append(root.Children, f)
		grow(f, 1, md)
	}
	return root
}

func buildShortTailStickyTree(rng *rand.Rand) *collections.CollectionNode {
	root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
	for s := 0; s < 5; s++ {
		d1 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		root.Children = append(root.Children, d1)
		depth := 2 + rng.Intn(3)
		cur := d1
		for d := 0; d < depth; d++ {
			sub := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			cur.Children = append(cur.Children, sub)
			for i, n := 0, 1+rng.Intn(2); i < n; i++ {
				sub.Children = append(sub.Children,
					&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
			}
			cur = sub
		}
		for i := 0; i < 2+rng.Intn(3); i++ {
			d1.Children = append(d1.Children,
				&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
		}
	}
	return root
}

func stickyIsAncestorOrSelf(a, n *collections.CollectionNode) bool {
	for p := n; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

func TestStickyBandPinsOnlyAncestors(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			root := buildShortTailStickyTree(rng)
			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.UpdateVisibleCols()

			var band []string
			sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
			defer func() { sidebar.DebugSticky = nil }()
			var solidH int
			sidebar.DebugBandSolid = func(s int) { solidH = s }
			defer func() { sidebar.DebugBandSolid = nil }()
			var drawn int
			sidebar.DebugBandGeom = func(n []string, _ []int, _ int) { drawn = len(n) }
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
			now := time.Unix(1700000000, 0)
			r := new(input.Router)
			frame := func() {
				ops := new(op.Ops)
				ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
				r.Frame(ops)
			}
			for i := 0; i < 3; i++ {
				frame()
			}
			ui.ColList.Position.First = 0
			ui.ColList.Position.Offset = 0
			frame()

			const rowH = 24
			nameToNode := map[string]*collections.CollectionNode{}
			for _, n := range ui.VisibleCols {
				nameToNode[n.Name] = n
			}

			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				band = band[:0]
				solidH, drawn = 0, 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()

				first := ui.ColList.Position.First
				if first < 0 || first >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[first]
				for _, nm := range band {
					n := nameToNode[nm]
					if n == nil {
						continue
					}
					if !stickyIsAncestorOrSelf(n, top) {
						t.Fatalf("step %d: band pins %q which is NOT an ancestor of the top row %q (band=%v First=%d Off=%d) — the band jumped ahead and would cover the next sibling",
							step, nm, top.Name, band, first, ui.ColList.Position.Offset)
					}
				}
				if drawn > 0 && solidH > drawn*rowH+rowH {
					t.Fatalf("step %d: opaque band %dpx overshoots its %d drawn rows (band=%v First=%d)", step, solidH, drawn, band, first)
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
		})
	}
}

func TestStickyNoFlickerOnExit(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 100, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			runStickyFlickerScroll(t, seed)
		})
	}
}

func runStickyFlickerScroll(t *testing.T, seed int64) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	ui.Settings.StickyMaxLines = 64

	rng := rand.New(rand.NewSource(seed))
	root := buildRandomStickyTree(rng, []int{1, 1, 2, 3, 1, 4, 2, 5})
	col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	var solidH int
	sidebar.DebugBandSolid = func(s int) { solidH = s }
	defer func() { sidebar.DebugBandSolid = nil }()
	var drawn int
	sidebar.DebugBandGeom = func(names []string, _ []int, _ int) { drawn = len(names) }
	defer func() { sidebar.DebugBandGeom = nil }()

	sz := image.Pt(900, 520)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	const rowH = 24
	const gapTol = 12
	seen := map[string]map[int]bool{}
	gapWorst, gapDesc := 0, ""

	step := 0
	for ; step < 4000; step++ {
		before := ui.ColList.Position.First + ui.ColList.Position.Offset
		band = band[:0]
		solidH, drawn = 0, 0
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()

		for _, nm := range band {
			if seen[nm] == nil {
				seen[nm] = map[int]bool{}
			}
			seen[nm][step] = true
		}
		if rows := drawn; rows > 0 {
			if over := solidH - (rows*rowH + gapTol); over > gapWorst {
				gapWorst = over
				gapDesc = fmt.Sprintf("step %d solidH=%d rows=%d band=%v", step, solidH, rows, band)
			}
		}
		if ui.ColList.Position.First+ui.ColList.Position.Offset == before {
			break
		}
	}

	const maxGap = 10
	var flick []string
	worstGap := 0
	for nm, set := range seen {
		lo, hi := 1<<30, -1
		for s := range set {
			if s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		run := 0
		folderWorst := 0
		for s := lo; s <= hi; s++ {
			if set[s] {
				run = 0
			} else {
				run++
				if run > folderWorst {
					folderWorst = run
				}
			}
		}
		if folderWorst > worstGap {
			worstGap = folderWorst
		}
		if folderWorst > maxGap {
			flick = append(flick, fmt.Sprintf("%q dropped out of the band for %d consecutive frames within [%d..%d]", nm, folderWorst, lo, hi))
		}
	}
	t.Logf("seed %d: %d frames, worst single-folder flicker gap = %d frame(s)", seed, step, worstGap)
	if len(flick) > 0 {
		sort.Strings(flick)
		for _, f := range flick {
			t.Log(f)
		}
		t.Fatalf("sticky band flickered (gap > %d frame) for %d folder(s) — a sustained drop, not a 1-frame boundary blip (seed %d)", maxGap, len(flick), seed)
	}
	if gapWorst > 0 {
		t.Fatalf("sticky band opaque fill exceeded its pinned rows by %dpx (empty space): %s", gapWorst, gapDesc)
	}
}

func TestStickySeamlessTransition(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 4; i++ {
				f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
				f.Children = append(f.Children, mkReq())
				root.Children = append(root.Children, f)
			}
			d1 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			d2 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			d3 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 5; i++ {
				d3.Children = append(d3.Children, mkReq())
			}
			d2.Children = append(d2.Children, d3)
			for i := 0; i < 4; i++ {
				d2.Children = append(d2.Children, mkReq())
			}
			d1.Children = append(d1.Children, d2)
			for i := 0; i < 4; i++ {
				d1.Children = append(d1.Children, mkReq())
			}
			root.Children = append(root.Children, d1)
			last := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 6; i++ {
				last.Children = append(last.Children, mkReq())
			}
			root.Children = append(root.Children, last)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.UpdateVisibleCols()

			depthOf := func(n *collections.CollectionNode) int { return stickyDepthOf(n) }
			d1Idx, d2Idx, d3Idx := -1, -1, -1
			for i, n := range ui.VisibleCols {
				switch n {
				case d1:
					d1Idx = i
				case d2:
					d2Idx = i
				case d3:
					d3Idx = i
				}
			}

			var band []string
			sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
			defer func() { sidebar.DebugSticky = nil }()
			var names []string
			var ys []int
			var bottom int
			sidebar.DebugBandGeom = func(n []string, y []int, b int) {
				names = append(names[:0], n...)
				ys = append(ys[:0], y...)
				bottom = b
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
			now := time.Unix(1700000000, 0)
			r := new(input.Router)
			frame := func() {
				ops := new(op.Ops)
				ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
				r.Frame(ops)
			}
			for i := 0; i < 3; i++ {
				frame()
			}
			ui.ColList.Position.First = 0
			ui.ColList.Position.Offset = 0
			frame()

			const rowH = 24
			inName := func(s string) bool {
				for _, n := range names {
					if n == s {
						return true
					}
				}
				return false
			}
			gaps, worst := 0, ""
			d1Checked, d2Checked := false, false
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				band = band[:0]
				names, ys, bottom = names[:0], ys[:0], 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()

				f := ui.ColList.Position.First
				if f < 0 || f >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[f]
				visible := 0
				for _, y := range ys {
					if y+rowH > 2 && y < bottom {
						visible++
					}
				}
				if depthOf(top) >= 2 && visible < 2 {
					gaps++
					if worst == "" {
						worst = fmt.Sprintf("step %d First=%d(%q) depth=%d Off=%d visible=%d ys=%v",
							step, f, top.Name, depthOf(top), ui.ColList.Position.Offset, visible, ys)
					}
				}
				if !d1Checked && f == d1Idx && d2Idx >= 0 && d3Idx >= 0 && ui.ColList.Position.Offset > 0 {
					d1Checked = true
					if !inName(d2.Name) || !inName(d3.Name) {
						t.Fatalf("deep descent lags: top is on the parent %q but its nested subfolders %q/%q are not both in the band (names=%v) — they would dock a node (or two) later",
							d1.Name, d2.Name, d3.Name, names)
					}
				}
				if !d2Checked && f == d2Idx && d3Idx >= 0 && ui.ColList.Position.Offset > 0 {
					d2Checked = true
					if !inName(d3.Name) {
						t.Fatalf("descent late: top is on %q but its first-child subfolder %q is not in the band (names=%v)",
							d2.Name, d3.Name, names)
					}
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			if gaps > 0 {
				t.Fatalf("band collapsed to root-only in %d frame(s) (the sibling-swap gap): %s", gaps, worst)
			}
			if !d1Checked || !d2Checked {
				t.Fatalf("test never exercised the descent (d1Checked=%v d2Checked=%v; idx d1=%d d2=%d d3=%d)", d1Checked, d2Checked, d1Idx, d2Idx, d3Idx)
			}
		})
	}
}

func TestStickyNonFirstSubfolderSwap(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			p := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			a := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			b := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 2+rng.Intn(3); i++ {
				a.Children = append(a.Children, mkReq())
			}
			for i := 0; i < 2+rng.Intn(3); i++ {
				b.Children = append(b.Children, mkReq())
			}
			p.Children = append(p.Children, a, b)
			for i := 0; i < 2; i++ {
				p.Children = append(p.Children, mkReq())
			}
			root.Children = append(root.Children, p)
			q := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 6; i++ {
				q.Children = append(q.Children, mkReq())
			}
			root.Children = append(root.Children, q)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.UpdateVisibleCols()

			aIdx, bIdx := -1, -1
			for i, n := range ui.VisibleCols {
				switch n {
				case a:
					aIdx = i
				case b:
					bIdx = i
				}
			}

			var names []string
			var bottom int
			sidebar.DebugBandGeom = func(n []string, _ []int, bt int) {
				names = append(names[:0], n...)
				bottom = bt
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
			now := time.Unix(1700000000, 0)
			r := new(input.Router)
			frame := func() {
				ops := new(op.Ops)
				ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
				r.Frame(ops)
			}
			for i := 0; i < 3; i++ {
				frame()
			}
			ui.ColList.Position.First = 0
			ui.ColList.Position.Offset = 0
			frame()

			rowH := ui.ColRowH()
			if rowH <= 0 {
				rowH = 24
			}
			depthOf := func(n *collections.CollectionNode) int { return stickyDepthOf(n) }
			bVisible := false
			dips, dipDesc := 0, ""
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				names, bottom = names[:0], 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()
				f := ui.ColList.Position.First
				if f < 0 || f >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[f]
				for _, nm := range names {
					if nm == b.Name {
						bVisible = true
					}
				}
				if depthOf(top) >= 3 && bottom < 3*rowH-2 {
					dips++
					if dipDesc == "" {
						dipDesc = fmt.Sprintf("step %d First=%d(%q) depth=%d bottom=%d names=%v", step, f, top.Name, depthOf(top), bottom, names)
					}
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			if aIdx < 0 || bIdx < 0 {
				t.Fatalf("could not locate subfolders A=%d B=%d", aIdx, bIdx)
			}
			if !bVisible {
				t.Fatal("the non-first subfolder B never appeared in the band")
			}
			if dips > 0 {
				t.Fatalf("band dipped below depth-2 while inside a depth-2 subtree in %d frame(s) (subfolder swap not smooth): %s", dips, dipDesc)
			}
		})
	}
}

func TestStickyDeepChainSlidesInNotPops(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			folder := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			}
			root := folder()
			w := folder()
			sib := folder()
			for i := 0; i < 4+rng.Intn(3); i++ {
				sib.Children = append(sib.Children, mkReq())
			}
			succ := folder()
			inner := folder()
			deep := folder()
			for i := 0; i < 3+rng.Intn(3); i++ {
				deep.Children = append(deep.Children, mkReq())
			}
			inner.Children = append(inner.Children, deep)
			for i := 0; i < 2+rng.Intn(2); i++ {
				inner.Children = append(inner.Children, mkReq())
			}
			succ.Children = append(succ.Children, inner)
			for i := 0; i < 2+rng.Intn(2); i++ {
				succ.Children = append(succ.Children, mkReq())
			}
			w.Children = append(w.Children, sib, succ)
			root.Children = append(root.Children, w)
			tail := folder()
			for i := 0; i < 6; i++ {
				tail.Children = append(tail.Children, mkReq())
			}
			root.Children = append(root.Children, tail)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.UpdateVisibleCols()

			var names []string
			sidebar.DebugBandGeom = func(n []string, _ []int, _ int) {
				names = append(names[:0], n...)
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 520)
			now := time.Unix(1700000000, 0)
			r := new(input.Router)
			frame := func() {
				ops := new(op.Ops)
				ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
				r.Frame(ops)
			}
			for i := 0; i < 3; i++ {
				frame()
			}
			ui.ColList.Position.First = 0
			ui.ColList.Position.Offset = 0
			frame()

			seen := map[string]bool{}
			prev := -1
			worstJump, jumpDesc := 0, ""
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				names = names[:0]
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()
				cur := len(names)
				for _, nm := range names {
					seen[nm] = true
				}
				if prev >= 0 && cur-prev > worstJump {
					worstJump = cur - prev
					f := ui.ColList.Position.First
					nm := "?"
					if f < len(ui.VisibleCols) {
						nm = ui.VisibleCols[f].Name
					}
					jumpDesc = fmt.Sprintf("step %d First=%d(%q) rows %d->%d names=%v", step, f, nm, prev, cur, names)
				}
				prev = cur
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			for _, n := range []*collections.CollectionNode{succ, inner, deep} {
				if !seen[n.Name] {
					t.Fatalf("deep-chain folder %q never appeared in the band", n.Name)
				}
			}
			if worstJump > 2 {
				t.Fatalf("sticky band grew by %d rows in a single frame (deep chain popped instead of staggering in): %s", worstJump, jumpDesc)
			}
		})
	}
}

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
	ui.UpdateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	var solidH int
	sidebar.DebugBandSolid = func(s int) { solidH = s }
	defer func() { sidebar.DebugBandSolid = nil }()
	var drawn int
	sidebar.DebugBandGeom = func(names []string, _ []int, _ int) { drawn = len(names) }
	defer func() { sidebar.DebugBandGeom = nil }()

	sz := image.Pt(900, 520)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	const rowH = 24
	const tol = 12
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

func TestStickyBandDoesNotLagList(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	cur := root
	for _, nm := range []string{"A", "B", "C", "D"} {
		f := &collections.CollectionNode{Name: nm, IsFolder: true, Expanded: true}
		cur.Children = append(cur.Children, f)
		cur = f
	}
	for i := 0; i < 40; i++ {
		cur.Children = append(cur.Children, &collections.CollectionNode{
			Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"},
		})
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	ancestorsOf := func(n *collections.CollectionNode) []string {
		var out []string
		for p := n.Parent; p != nil; p = p.Parent {
			out = append([]string{p.Name}, out...)
		}
		return out
	}

	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.ColRowH() > 0 {
			return ui.ColRowH()
		}
		return 24
	}
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}
	expectedBand := func(i int) []string {
		n := ui.VisibleCols[i]
		out := ancestorsOf(n)
		if (n.IsFolder || n.Depth == 0) && n.Expanded &&
			i+1 < len(ui.VisibleCols) && ui.VisibleCols[i+1].Parent == n {
			out = append(out, n.Name)
		}
		return out
	}

	absScroll := func(ui *AppUI) int {
		p := ui.ColList.Position.Offset
		for i := 0; i < ui.ColList.Position.First && i < len(ui.VisibleCols); i++ {
			h := ui.VisibleCols[i].RowHeightPx
			if h <= 0 {
				h = ui.ColRowH()
			}
			p += h
		}
		return p
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         now,
			Source:      r.Source(),
		}
		ui.LayoutApp(gtx)
		r.Frame(ops)
	}

	frame()

	maxAnc := 0
	prevEffective := absScroll(ui) - reserve
	streak, worst := 0, 0
	for step := 0; step < 80; step++ {
		r.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(120, 400),
			Scroll:   f32.Pt(0, 6),
		})
		now = now.Add(16 * time.Millisecond)
		frame()

		first := ui.ColList.Position.First
		if first < 0 || first >= len(ui.VisibleCols) {
			continue
		}
		_ = rowUnderBand
		if bandH > 0 {
			want := expectedBand(first)
			if len(want) > maxAnc {
				maxAnc = len(want)
			}
			if fmt.Sprint(lastRendered) == fmt.Sprint(want) {
				streak = 0
			} else {
				streak++
				if streak > worst {
					worst = streak
				}
				if streak > 6 {
					t.Fatalf("step %d: sticky band did not match the top row's scope for %d frames — First=%d\n  rendered = %v\n  reach-up expects = %v",
						step, streak, first, lastRendered, want)
				}
			}
		}
		effective := absScroll(ui) - reserve
		if effective < prevEffective-2 {
			t.Fatalf("step %d: content scrolled BACKWARDS — First=%d Offset=%d reserve=%d: effective %d -> %d",
				step, first, ui.ColList.Position.Offset, reserve, prevEffective, effective)
		}
		prevEffective = effective
	}

	if maxAnc < 4 {
		t.Fatalf("scroll only ever reached %d pinned ancestors; the test never exercised the deep boundaries", maxAnc)
	}
}

func TestStickyBandDoesNotLurchAtSiblingFolder(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	for _, fn := range []string{"fldA", "fldB", "fldC", "fldD"} {
		f := &collections.CollectionNode{Name: fn, IsFolder: true, Expanded: true}
		for i := 0; i < 25; i++ {
			f.Children = append(f.Children, &collections.CollectionNode{
				Name: fmt.Sprintf("%s-req-%d", fn, i), Request: &model.ParsedRequest{Method: "GET"},
			})
		}
		root.Children = append(root.Children, f)
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	absScroll := func(ui *AppUI) int {
		p := ui.ColList.Position.Offset
		for i := 0; i < ui.ColList.Position.First && i < len(ui.VisibleCols); i++ {
			h := ui.VisibleCols[i].RowHeightPx
			if h <= 0 {
				h = ui.ColRowH()
			}
			p += h
		}
		return p
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         now,
			Source:      r.Source(),
		}
		ui.LayoutApp(gtx)
		r.Frame(ops)
	}

	frame()

	const delta = 6
	forwardMax := delta + 4
	prevEffective := absScroll(ui) - reserve
	seen := map[string]bool{}
	for step := 0; step < 320; step++ {
		r.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(120, 400),
			Scroll:   f32.Pt(0, delta),
		})
		now = now.Add(16 * time.Millisecond)
		frame()

		if len(lastRendered) > 0 {
			seen[lastRendered[len(lastRendered)-1]] = true
		}
		_ = bandH
		effective := absScroll(ui) - reserve
		if effective < prevEffective-4 {
			t.Fatalf("step %d: content scrolled BACKWARDS: effective %d -> %d (First=%d Offset=%d band=%v)",
				step, prevEffective, effective, ui.ColList.Position.First, ui.ColList.Position.Offset, lastRendered)
		}
		if effective > prevEffective+forwardMax {
			t.Fatalf("step %d: content LURCHED FORWARD: effective %d -> %d (+%d > scroll %d) (First=%d Offset=%d band=%v)",
				step, prevEffective, effective, effective-prevEffective, delta,
				ui.ColList.Position.First, ui.ColList.Position.Offset, lastRendered)
		}
		prevEffective = effective
	}

	for _, fn := range []string{"fldB", "fldC"} {
		if !seen[fn] {
			t.Fatalf("scroll never pinned %q as innermost; sibling boundaries not exercised (seen=%v)", fn, seen)
		}
	}
}

func TestStickySiblingPushOutSlidesNotVanish(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	for _, fn := range []string{"A", "B", "C", "D", "E"} {
		f := &collections.CollectionNode{Name: fn, IsFolder: true, Expanded: true}
		for i := 0; i < 10; i++ {
			f.Children = append(f.Children, &collections.CollectionNode{
				Name: fmt.Sprintf("%s-r%d", fn, i), Request: &model.ParsedRequest{Method: "GET"}})
		}
		root.Children = append(root.Children, f)
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	rowH := ui.ColRowH()
	if rowH <= 0 {
		rowH = 24
	}
	twoRow := 2*rowH + 4

	type fr struct {
		deepest string
		h       int
	}
	var frames []fr
	for step := 0; step < 900; step++ {
		before := ui.ColList.Position.First*10000 + ui.ColList.Position.Offset
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		d := ""
		if len(band) > 0 {
			d = band[len(band)-1]
		}
		frames = append(frames, fr{d, bandH})
		if ui.ColList.Position.First*10000+ui.ColList.Position.Offset == before {
			break
		}
	}

	siblings := map[string]bool{"A": true, "B": true, "C": true, "D": true, "E": true}
	slidOut := 0
	run := 0
	minHInRun := 1 << 30
	prevDeepest := ""
	flush := func(next string) {
		if siblings[prevDeepest] && run >= 2 && minHInRun < twoRow-rowH/2 && siblings[next] && next != prevDeepest {
			slidOut++
		}
	}
	for _, f := range frames {
		if siblings[f.deepest] && f.h > twoRow {
			t.Fatalf("band extended past two rows (%d > %d) for top-level folder %q — it grew instead of pushing out", f.h, twoRow, f.deepest)
		}
		if f.deepest == prevDeepest {
			run++
			if f.h < minHInRun {
				minHInRun = f.h
			}
		} else {
			flush(f.deepest)
			prevDeepest = f.deepest
			run = 1
			minHInRun = f.h
		}
	}
	if slidOut < 2 {
		t.Fatalf("only %d sibling-folder transitions showed the leaving folder sliding out; expected the push-out at several boundaries", slidOut)
	}
}

func TestStickyNoJumpAtNestedSubfolder(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	f1 := &collections.CollectionNode{Name: "F1", IsFolder: true, Expanded: true}
	f1.Children = []*collections.CollectionNode{mkReq("f1-r0"), mkReq("f1-r1"), mkReq("f1-r2")}
	f2 := &collections.CollectionNode{Name: "F2sub", IsFolder: true, Expanded: true}
	for i := 0; i < 40; i++ {
		f2.Children = append(f2.Children, mkReq(fmt.Sprintf("f2-r%d", i)))
	}
	f1.Children = append(f1.Children, f2, mkReq("f1-after"))
	root.Children = []*collections.CollectionNode{f1}
	for i := 0; i < 10; i++ {
		root.Children = append(root.Children, mkReq(fmt.Sprintf("root-r%d", i)))
	}
	c := &collections.ParsedCollection{ID: "nest", Name: "root", Root: root}
	collections.AssignParents(root, nil, c)
	ui.Collections = []*collections.CollectionUI{{Data: c}}
	ui.UpdateVisibleCols()

	absScroll := func(ui *AppUI) int {
		p := ui.ColList.Position.Offset
		for i := 0; i < ui.ColList.Position.First && i < len(ui.VisibleCols); i++ {
			h := ui.VisibleCols[i].RowHeightPx
			if h <= 0 {
				h = ui.ColRowH()
			}
			p += h
		}
		return p
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	frame()

	const delta = 6
	forwardMax := delta + 4
	prevEffective := absScroll(ui) - reserve
	sawSub := false
	for step := 0; step < 90; step++ {
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 400), Scroll: f32.Pt(0, delta)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if len(lastRendered) > 0 && lastRendered[len(lastRendered)-1] == "F2sub" {
			sawSub = true
		}
		if reserve > bandH {
			t.Fatalf("step %d: reserve=%d exceeds bandH=%d", step, reserve, bandH)
		}
		effective := absScroll(ui) - reserve
		if effective < prevEffective-4 {
			t.Fatalf("step %d: content scrolled BACKWARDS at subfolder: effective %d -> %d (First=%d Off=%d reserve=%d band=%v)",
				step, prevEffective, effective, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, lastRendered)
		}
		if effective > prevEffective+forwardMax {
			t.Fatalf("step %d: content LURCHED FORWARD at subfolder: effective %d -> %d (+%d > scroll %d) (First=%d Off=%d reserve=%d band=%v)",
				step, prevEffective, effective, effective-prevEffective, delta, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, lastRendered)
		}
		prevEffective = effective
	}
	if !sawSub {
		t.Fatal("scroll never pinned F2sub as the innermost header; the nested-subfolder boundary was not exercised")
	}
}

func TestStickyRealCollectionScrollTopToBottom(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/rete/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.UpdateVisibleCols()
	t.Logf("collections=%d visible nodes=%d", len(cols), len(ui.VisibleCols))
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small to exercise scrolling (%d nodes)", len(ui.VisibleCols))
	}

	absScroll := func() int {
		p := ui.ColList.Position.Offset
		for i := 0; i < ui.ColList.Position.First && i < len(ui.VisibleCols); i++ {
			h := ui.VisibleCols[i].RowHeightPx
			if h <= 0 {
				h = ui.ColRowH()
			}
			p += h
		}
		return p
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	const delta = 6
	const tol = 6
	topName := func() (string, int, bool) {
		f := ui.ColList.Position.First
		if f < 0 || f >= len(ui.VisibleCols) {
			return "?", -1, false
		}
		n := ui.VisibleCols[f]
		entering := (n.IsFolder || n.Depth == 0) && n.Expanded &&
			f+1 < len(ui.VisibleCols) && ui.VisibleCols[f+1].Parent == n
		return n.Name, n.Depth, entering
	}

	type jerk struct{ msg string }
	scrollDir := func(dir string, sign int) (int, []jerk) {
		prevEff := absScroll() - reserve
		stall, frames := 0, 0
		var jerks []jerk
		for step := 0; step < 6000; step++ {
			r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, float32(sign*delta))})
			now = now.Add(16 * time.Millisecond)
			before := absScroll()
			frame()
			frames++
			eff := absScroll() - reserve
			d := eff - prevEff
			name, depth, entering := topName()
			var bad string
			if sign > 0 {
				if d < -tol {
					bad = fmt.Sprintf("BACKWARDS d=%d", d)
				} else if d > delta+tol {
					bad = fmt.Sprintf("FORWARD-LURCH d=%d (>scroll %d)", d, delta)
				}
			} else {
				if d > tol {
					bad = fmt.Sprintf("FORWARD d=%d", d)
				} else if d < -(delta + tol) {
					bad = fmt.Sprintf("BACKWARD-LURCH d=%d (>scroll %d)", d, delta)
				}
			}
			if bad != "" {
				jerks = append(jerks, jerk{fmt.Sprintf("%s step %d: %s top=%q depth=%d entering=%v First=%d Off=%d reserve=%d bandH=%d band=%v",
					dir, step, bad, name, depth, entering, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, bandH, lastRendered)})
			}
			prevEff = eff
			if absScroll() == before {
				if stall++; stall >= 3 {
					break
				}
			} else {
				stall = 0
			}
		}
		return frames, jerks
	}

	downFrames, downJerks := scrollDir("down", +1)
	if ui.ColList.Position.First == 0 {
		t.Fatal("scrolling down never advanced past the first node")
	}
	t.Logf("scrolled to bottom in %d frames (First=%d); %d jerks", downFrames, ui.ColList.Position.First, len(downJerks))
	upFrames, upJerks := scrollDir("up", -1)
	t.Logf("scrolled back toward top in %d frames (First=%d Off=%d); %d jerks", upFrames, ui.ColList.Position.First, ui.ColList.Position.Offset, len(upJerks))

	all := append(downJerks, upJerks...)
	if len(all) > 0 {
		const show = 25
		for i, j := range all {
			if i >= show {
				t.Logf("... and %d more", len(all)-show)
				break
			}
			t.Log(j.msg)
		}
		t.Fatalf("real collection scroll produced %d content jerks (see above)", len(all))
	}
}

func TestStickyRealCollectionBandMatchesContent(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/rete/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.UpdateVisibleCols()
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small (%d nodes)", len(ui.VisibleCols))
	}

	ancestorsOf := func(n *collections.CollectionNode) []string {
		var out []string
		for p := n.Parent; p != nil; p = p.Parent {
			out = append([]string{p.Name}, out...)
		}
		return out
	}
	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.ColRowH() > 0 {
			return ui.ColRowH()
		}
		return 24
	}
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}
	expectedBand := func(i int) []string {
		n := ui.VisibleCols[i]
		out := ancestorsOf(n)
		if (n.IsFolder || n.Depth == 0) && n.Expanded &&
			i+1 < len(ui.VisibleCols) && ui.VisibleCols[i+1].Parent == n {
			out = append(out, n.Name)
		}
		return out
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	const maxStreak = 10
	var mism []string
	checks, streak, worst, mismTotal := 0, 0, 0, 0
	for step := 0; step < 1400; step++ {
		before := ui.ColList.Position.Offset + ui.ColList.Position.First
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH <= 0 {
			streak = 0
			continue
		}
		_ = rowUnderBand
		first := ui.ColList.Position.First
		want := expectedBand(first)
		checks++
		if fmt.Sprint(lastRendered) == fmt.Sprint(want) {
			streak = 0
		} else {
			streak++
			mismTotal++
			if streak > worst {
				worst = streak
			}
			if len(mism) < 20 {
				mism = append(mism, fmt.Sprintf("step %d (streak %d): band=%v but top row %q reach-up chain is %v (First=%d Off=%d bandH=%d)",
					step, streak, lastRendered, ui.VisibleCols[first].Name, want, ui.ColList.Position.First, ui.ColList.Position.Offset, bandH))
			}
		}
		if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
			break
		}
	}
	t.Logf("compared %d frames; %d mismatched (all transitions); worst consecutive streak = %d", checks, mismTotal, worst)
	if worst > maxStreak {
		for _, m := range mism {
			t.Log(m)
		}
		t.Fatalf("sticky band lagged the content for %d consecutive frames (> %d) — a persistent lag, not a transition", worst, maxStreak)
	}
}

func TestStickyRealCollectionNoDuplicateUnderBand(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/rete/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.UpdateVisibleCols()
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small (%d nodes)", len(ui.VisibleCols))
	}

	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.ColRowH() > 0 {
			return ui.ColRowH()
		}
		return 24
	}
	rowTop := func(i int) int {
		y := -ui.ColList.Position.Offset
		for j := ui.ColList.Position.First; j < i && j < len(ui.VisibleCols); j++ {
			y += rowH(j)
		}
		return y
	}
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	var dups []string
	for step := 0; step < 1400; step++ {
		before := ui.ColList.Position.Offset + ui.ColList.Position.First
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH <= 0 || len(lastRendered) == 0 {
			if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
				break
			}
			continue
		}
		ui_ := rowUnderBand(bandH)
		under := ui.VisibleCols[ui_]
		isFolderUnder := (under.IsFolder || under.Depth == 0) && under.Expanded &&
			ui_+1 < len(ui.VisibleCols) && ui.VisibleCols[ui_+1].Parent == under
		peek := rowTop(ui_) + rowH(ui_) - bandH
		if isFolderUnder && lastRendered[len(lastRendered)-1] == under.Name && peek > 2 {
			if len(dups) < 20 {
				dups = append(dups, fmt.Sprintf("step %d: %q pinned AND its real row peeks %dpx below the band (First=%d Off=%d bandH=%d band=%v)",
					step, under.Name, peek, ui.ColList.Position.First, ui.ColList.Position.Offset, bandH, lastRendered))
			}
		}
		if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
			break
		}
	}
	if len(dups) > 0 {
		for _, d := range dups {
			t.Log(d)
		}
		t.Fatalf("sticky header duplicated with its list row in %d+ frames (no smooth transition)", len(dups))
	}
}

func loadRealCollections(t *testing.T) []*collections.CollectionUI {
	t.Helper()
	var paths []string
	if p := os.Getenv("STICKY_COLLECTION"); p != "" {
		paths = []string{p}
	} else {
		dir := filepath.Join(os.Getenv("APPDATA"), "rete", "collections")
		matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		paths = matches
	}
	var out []*collections.CollectionUI
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		col, err := collections.ParseCollection(bytes.NewReader(data), filepath.Base(p))
		if err != nil || col == nil || col.Root == nil {
			continue
		}
		out = append(out, &collections.CollectionUI{Data: col})
	}
	return out
}

func TestStickyMaxLinesSetting(t *testing.T) {
	for _, limit := range []int{1, 2, 3, 4} {
		setupTestConfigDir(t)
		ui := NewAppUI()
		ui.Window = new(app.Window)
		ui.Tabs = nil
		ui.SidebarSection = "requests"
		ui.ColsExpanded = true
		ui.Settings.StickyMaxLines = limit

		rng := rand.New(rand.NewSource(int64(limit) * 7))
		root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		cur := root
		for d := 0; d < 7; d++ {
			sub := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			cur.Children = append(cur.Children, sub)
			for i := 0; i < 5; i++ {
				sub.Children = append(sub.Children, &collections.CollectionNode{Name: randStickyName(rng)})
			}
			cur = sub
		}
		col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
		collections.AssignParents(root, nil, col)
		ui.Collections = []*collections.CollectionUI{{Data: col}}
		ui.UpdateVisibleCols()

		worst := 0
		sidebar.DebugSticky = func(_ int, names []string) {
			if len(names) > worst {
				worst = len(names)
			}
		}
		defer func() { sidebar.DebugSticky = nil }()

		sz := image.Pt(900, 600)
		now := time.Unix(1700000000, 0)
		r := new(input.Router)
		frame := func() {
			ops := new(op.Ops)
			ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
			r.Frame(ops)
		}
		for i := 0; i < 3; i++ {
			frame()
		}
		ui.ColList.Position.First = 0
		ui.ColList.Position.Offset = 0
		frame()
		for step := 0; step < 400; step++ {
			before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
			r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
			now = now.Add(16 * time.Millisecond)
			frame()
			if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
				break
			}
		}
		if worst > limit {
			t.Fatalf("StickyMaxLines=%d but the band pinned %d rows", limit, worst)
		}
		if worst == 0 {
			t.Fatalf("StickyMaxLines=%d: band never pinned any row (test did not exercise it)", limit)
		}
	}
}

func TestStickyScrollThroughBand(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	rng := rand.New(rand.NewSource(7))
	root := buildRandomStickyTree(rng, []int{2, 3, 4, 2, 3})
	col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()
	total := len(ui.VisibleCols)
	if total < 40 {
		t.Skipf("tree too small (%d nodes)", total)
	}

	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 600)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.LayoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	maxFirst := 0
	sawBand := false
	for step := 0; step < 2000; step++ {
		before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 8)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH > 0 {
			sawBand = true
		}
		if ui.ColList.Position.First > maxFirst {
			maxFirst = ui.ColList.Position.First
		}
		if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
			break
		}
	}
	if !sawBand {
		t.Skip("band never rendered; cannot exercise scroll-through-band")
	}
	if maxFirst < total-25 {
		t.Fatalf("scrolling over the band stalled the list at First=%d of %d nodes (band swallowed scroll?)", maxFirst, total)
	}
}

func TestTabBarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	ui.Tabs = nil
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"))
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T2 long title for multi words test"))
	ui.ActiveIdx = 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.LayoutTabBar(gtx)

	ui.Tabs[1].TabBtn.Click()
	ui.LayoutTabBar(gtx)
	if ui.ActiveIdx != 1 {
		t.Errorf("expected tab 1 active, got %d", ui.ActiveIdx)
	}

	ui.TabBar.AddTabBtn.Click()
	ui.LayoutContent(gtx)
	if len(ui.Tabs) != 3 {
		t.Errorf("expected 3 tabs after add, got %d", len(ui.Tabs))
	}

	ui.CloseTab(0)
	if len(ui.Tabs) != 2 {
		t.Errorf("expected 2 tabs after close")
	}

	_ = material.NewTheme()
}

func TestTabBar_Dragging(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"), workspace.NewRequestTab("T2"))

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(800, 100)),
	}

	ui.TabBar.TabDragging = true
	ui.TabBar.TabDragIdx = 0
	ui.TabBar.TabDragCurrentX = 100
	ui.TabBar.TabDragCurrentY = 50
	ui.LayoutTabBar(gtx)
}

func TestTabBarWrapping(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.Tabs = nil
	for i := 0; i < 20; i++ {
		ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("Tab"))
	}

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 800)),
		Now:         time.Now(),
	}

	ui.LayoutTabBar(gtx)
}

func TestTitleBarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 30)),
	}

	ui.LayoutTitleBar(gtx)

	ui.TitleBar.BtnMinimize.Click()
	ui.LayoutTitleBar(gtx)

	ui.TitleBar.BtnMaximize.Click()
	ui.LayoutTitleBar(gtx)
	if !ui.TitleBar.Maximized {
		t.Errorf("expected maximized")
	}

	ui.TitleBar.BtnMaximize.Click()
	ui.LayoutTitleBar(gtx)
	if ui.TitleBar.Maximized {
		t.Errorf("expected unmaximized")
	}

	ui.TitleBar.BtnClose.Click()
	ui.LayoutTitleBar(gtx)

	ui.TitleBar.Maximized = false
	ui.SettingsOpen = false
	ui.TitleBar.SettingsBtn.Click()
	ui.LayoutTitleBar(gtx)
	if !ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=true after click")
	}
	if ui.SettingsState == nil {
		t.Errorf("expected SettingsState to be initialized after first open")
	}
	ui.TitleBar.SettingsBtn.Click()
	ui.LayoutTitleBar(gtx)
	if ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=false after second click")
	}
}

func TestTitleBarSettingsButtonHitArea(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win
	ui.SettingsOpen = false

	var router input.Router
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 30)),
		Source:      router.Source(),
	}

	ui.LayoutTitleBar(gtx)
	router.Frame(ops)

	router.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(110, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(110, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)

	ops.Reset()
	gtx.Ops = ops
	ui.LayoutTitleBar(gtx)

	if !ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=true after pointer click in button area")
	}

	router.Frame(ops)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(600, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(600, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	ops.Reset()
	gtx.Ops = ops
	prev := ui.SettingsOpen
	ui.LayoutTitleBar(gtx)
	if ui.SettingsOpen != prev {
		t.Errorf("click in drag area should not toggle SettingsOpen")
	}
}
