package ui

import (
	"image"
	"strings"
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

func TestShortFingerprint(t *testing.T) {
	if got := shortFingerprint(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	short := "ab:cd:ef"
	if got := shortFingerprint(short); got != short {
		t.Errorf("short passthrough: got %q want %q", got, short)
	}

	bd := strings.Repeat("a", 17)
	if got := shortFingerprint(bd); got != bd {
		t.Errorf("len17 passthrough: got %q", got)
	}

	in := "0123456789abcdef00"
	got := shortFingerprint(in)
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in %q", got)
	}
	if !strings.HasPrefix(got, in[:8]) || !strings.HasSuffix(got, in[len(in)-8:]) {
		t.Errorf("expected first 8 and last 8 chars in %q", got)
	}
}

func TestGenLabel(t *testing.T) {
	if genLabel(nil) != "Generate CA" {
		t.Errorf("nil CA should give Generate CA")
	}

	ca, err := mitm.GenerateCA()
	if err != nil {
		t.Skipf("GenerateCA failed: %v", err)
	}
	if genLabel(ca) != "Regenerate" {
		t.Errorf("non-nil CA should give Regenerate")
	}
}

func TestChromeEdgeAndFirefoxSteps(t *testing.T) {
	for _, installed := range []bool{true, false} {
		steps := chromeEdgeSteps(installed)
		if len(steps) == 0 {
			t.Errorf("expected chromeEdgeSteps non-empty for %v", installed)
		}
		joined := strings.ToLower(strings.Join(steps, " "))
		if installed && !strings.Contains(joined, "trusted") {
			t.Errorf("installed=true should mention trusted")
		}
		if !installed && !strings.Contains(joined, "install") {
			t.Errorf("installed=false should mention install")
		}
	}
	ff := firefoxSteps()
	if len(ff) < 3 {
		t.Errorf("firefoxSteps too short: %d", len(ff))
	}
	if !strings.Contains(strings.ToLower(strings.Join(ff, " ")), "firefox") {
		t.Errorf("firefoxSteps should mention firefox")
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1024 * 1024, "1.0M"},
		{1536, "1.5K"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q want %q", c.n, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	f := &mitm.Flow{}
	if got := humanDuration(f); got != "-" {
		t.Errorf("zero Started: got %q want -", got)
	}
	now := time.Now()
	f = &mitm.Flow{Started: now.Add(-500 * time.Microsecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "µs") {
		t.Errorf("expected microseconds suffix, got %q", got)
	}
	f = &mitm.Flow{Started: now.Add(-50 * time.Millisecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "ms") {
		t.Errorf("expected ms, got %q", got)
	}
	f = &mitm.Flow{Started: now.Add(-2 * time.Second), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "s") {
		t.Errorf("expected s, got %q", got)
	}

	f = &mitm.Flow{Started: time.Now().Add(-10 * time.Millisecond)}
	if got := humanDuration(f); got == "-" {
		t.Errorf("live flow should compute, got %q", got)
	}
}

func TestTunnelStatusText(t *testing.T) {
	if got := tunnelStatusText(&mitm.Flow{}); got != "…" {
		t.Errorf("empty: got %q", got)
	}
	if got := tunnelStatusText(&mitm.Flow{Status: "200 OK"}); got != "200 OK" {
		t.Errorf("status only: got %q", got)
	}
	got := tunnelStatusText(&mitm.Flow{Status: "200 OK", Error: "boom"})
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "boom") {
		t.Errorf("status+err: got %q", got)
	}

}

func TestMITMStatusLine(t *testing.T) {
	now := time.Now()
	f := &mitm.Flow{Status: "200 OK", ReqSize: 100, RespSize: 200, Started: now.Add(-20 * time.Millisecond), Ended: now}
	got := mitmStatusLine(f)
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "req") || !strings.Contains(got, "resp") {
		t.Errorf("status line missing components: %q", got)
	}

	f2 := &mitm.Flow{}
	got2 := mitmStatusLine(f2)
	if got2 != "-" {
		t.Errorf("expected just '-' for empty flow, got %q", got2)
	}
}

func TestMITMFindByID(t *testing.T) {
	s := mitm.NewStore()
	if got := mitmFindByID(s, 1); got != nil {
		t.Errorf("empty store: expected nil")
	}
	added := s.Add(&mitm.Flow{Method: "GET", Host: "h"})
	if got := mitmFindByID(s, added.ID); got == nil || got.ID != added.ID {
		t.Errorf("expected to find by ID %d", added.ID)
	}
	if got := mitmFindByID(s, 9999); got != nil {
		t.Errorf("expected nil for missing ID")
	}
}

func TestActiveEnvSnapshot(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	if snap := ui.activeEnvSnapshot(); snap != nil {
		t.Errorf("expected nil snapshot when activeEnvVars nil")
	}
	ui.activeEnvVars = map[string]string{"k": "v", "x": "y"}
	snap := ui.activeEnvSnapshot()
	if len(snap) != 2 || snap["k"] != "v" || snap["x"] != "y" {
		t.Errorf("snapshot mismatch: %v", snap)
	}

	snap["k"] = "MUT"
	if ui.activeEnvVars["k"] != "v" {
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
	ui.activeEnvDirty = true
	ui.refreshActiveEnv()
	if _, ok := ui.activeEnvVars["ok"]; !ok {
		t.Errorf("var with value missing")
	}
	if _, ok := ui.activeEnvVars["also"]; !ok {
		t.Errorf("var with value missing")
	}
	if _, ok := ui.activeEnvVars["empty"]; ok {
		t.Errorf("empty-value var should be excluded")
	}

	ui.activeEnvVars = map[string]string{"sentinel": "1"}
	ui.activeEnvDirty = false
	ui.refreshActiveEnv()
	if ui.activeEnvVars["sentinel"] != "1" {
		t.Errorf("expected no-op when not dirty")
	}

	ui.ActiveEnvID = "missing"
	ui.activeEnvDirty = true
	ui.refreshActiveEnv()
	if ui.activeEnvVars != nil {
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

	ui.commitEditingEnv()
	ui.refreshActiveEnv()

	if got := ui.activeEnvVars["base"]; got != "http://api" {
		t.Fatalf("a freshly added {{base}} must resolve to its value; got %q", got)
	}
}

func TestInheritActiveTabLayout(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Tabs = nil

	newTab := workspace.NewRequestTab("x")
	ui.inheritActiveTabLayout(newTab)

	src := workspace.NewRequestTab("src")
	src.SplitRatio = 0.42
	src.VStackRatio = 0.31
	src.LayoutMode = 1
	src.HeaderSplitRatio = 0.7
	ui.Tabs = []*workspace.RequestTab{src}
	ui.ActiveIdx = 0

	dst := workspace.NewRequestTab("dst")
	ui.inheritActiveTabLayout(dst)
	if dst.SplitRatio != 0.42 || dst.VStackRatio != 0.31 || dst.HeaderSplitRatio != 0.7 {
		t.Errorf("layout not inherited: %+v", dst)
	}

	ui.inheritActiveTabLayout(src)
	if src.SplitRatio != 0.42 {
		t.Errorf("self-inherit should be no-op")
	}

	ui.ActiveIdx = 99
	dst2 := workspace.NewRequestTab("dst2")
	original := dst2.SplitRatio
	ui.inheritActiveTabLayout(dst2)
	if dst2.SplitRatio != original {
		t.Errorf("out-of-range ActiveIdx must not modify dst (was %v, now %v)", original, dst2.SplitRatio)
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
	ui.updateVisibleCols()
	if len(ui.VisibleCols) != 3 {
		t.Errorf("expected 3 visible nodes when all expanded, got %d", len(ui.VisibleCols))
	}

	folder.Expanded = false
	ui.updateVisibleCols()
	if len(ui.VisibleCols) != 2 {
		t.Errorf("expected 2 visible nodes (root,folder), got %d", len(ui.VisibleCols))
	}

	root.Expanded = false
	ui.updateVisibleCols()
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
	ui.closeTab(-1)
	ui.closeTab(99)
	if len(ui.Tabs) != before {
		t.Errorf("out-of-range close should be no-op")
	}

	ui.closeTab(2)
	if len(ui.Tabs) != 2 || ui.ActiveIdx != 1 {
		t.Errorf("after close last: tabs=%d active=%d", len(ui.Tabs), ui.ActiveIdx)
	}

	ui.closeTab(0)
	if len(ui.Tabs) != 1 || ui.ActiveIdx != 0 {
		t.Errorf("after close first w/ active=1: tabs=%d active=%d", len(ui.Tabs), ui.ActiveIdx)
	}

	ui.closeTab(0)
	if len(ui.Tabs) != 0 {
		t.Errorf("expected empty tabs")
	}
	if ui.ActiveIdx != -1 {
		t.Errorf("expected ActiveIdx=-1 after closing only tab, got %d", ui.ActiveIdx)
	}
}

func TestConsumeStartupFlags_NoAdmin(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.MITM.Ensure()

	ui.MITMAutoStart = true
	ui.MITMAutoInstallCA = true
	ui.MITMAutoRemoveCA = true
	ui.consumeStartupFlags()
	if ui.MITMAutoStart || ui.MITMAutoInstallCA || ui.MITMAutoRemoveCA {
		t.Errorf("startup flags should be consumed (reset to false) regardless of admin state")
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

	ui.layoutMITMSection(gtx)

	ui.MITM.HelpOpen = true
	ui.layoutMITMSection(gtx)
}

func TestPushChannelsAndImportInvalid(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.importDroppedData([]byte("not even json"))
	select {
	case <-ui.ColLoadedChan:
		t.Errorf("garbage should not push collection")
	case <-ui.EnvLoadedChan:
		t.Errorf("garbage should not push environment")
	case <-time.After(300 * time.Millisecond):

	}
}
