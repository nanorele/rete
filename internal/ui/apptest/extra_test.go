package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"
	"time"
	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/mitm"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

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
