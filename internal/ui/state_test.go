package ui

import (
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
)

func node(name string, folder, expanded bool, depth int, children ...*collections.CollectionNode) *collections.CollectionNode {
	n := &collections.CollectionNode{
		Name:     name,
		IsFolder: folder,
		Expanded: expanded,
		Depth:    depth,
		Children: children,
	}
	for _, c := range children {
		c.Parent = n
	}
	return n
}

func visibleNames(ui *AppUI) []string {
	out := make([]string, 0, len(ui.VisibleCols))
	for _, n := range ui.VisibleCols {
		out = append(out, n.Name)
	}
	return out
}

func eqNames(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUpdateVisibleCols(t *testing.T) {
	cases := []struct {
		name string
		root *collections.CollectionNode
		want []string
	}{
		{
			"collapsed root hides children",
			node("root", true, false, 0, node("a", false, false, 1)),
			[]string{"root"},
		},
		{
			"expanded root shows children",
			node("root", true, true, 0, node("a", false, false, 1), node("b", false, false, 1)),
			[]string{"root", "a", "b"},
		},
		{
			"collapsed folder hides its subtree",
			node("root", true, true, 0,
				node("fld", true, false, 1, node("deep", false, false, 2))),
			[]string{"root", "fld"},
		},
		{
			"expanded folder shows subtree",
			node("root", true, true, 0,
				node("fld", true, true, 1, node("deep", false, false, 2))),
			[]string{"root", "fld", "deep"},
		},
		{
			"expanded request leaf does not recurse",
			node("root", true, true, 0,
				node("req", false, true, 1, node("hidden", false, false, 2))),
			[]string{"root", "req"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ui := &AppUI{}
			ui.Collections = []*collections.CollectionUI{
				{Data: &collections.ParsedCollection{ID: "c1", Root: c.root}},
			}
			ui.updateVisibleCols()
			if got := visibleNames(ui); !eqNames(got, c.want) {
				t.Errorf("visible = %v, want %v", got, c.want)
			}
		})
	}
}

func TestUpdateVisibleCols_MultipleCollections(t *testing.T) {
	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{
		{Data: &collections.ParsedCollection{ID: "c1", Root: node("r1", true, true, 0, node("a", false, false, 1))}},
		{Data: &collections.ParsedCollection{ID: "c2", Root: node("r2", true, false, 0, node("b", false, false, 1))}},
	}
	ui.updateVisibleCols()
	want := []string{"r1", "a", "r2"}
	if got := visibleNames(ui); !eqNames(got, want) {
		t.Errorf("visible = %v, want %v", got, want)
	}
}

func TestUpdateVisibleCols_RepeatedCallsStable(t *testing.T) {
	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{
		{Data: &collections.ParsedCollection{ID: "c1", Root: node("root", true, true, 0,
			node("a", false, false, 1), node("b", false, false, 1))}},
	}
	ui.updateVisibleCols()
	first := visibleNames(ui)
	ui.updateVisibleCols()
	ui.updateVisibleCols()
	if got := visibleNames(ui); !eqNames(got, first) {
		t.Errorf("visible drifted across calls: %v then %v", first, got)
	}
}

func TestUpdateVisibleCols_ShrinkAfterCollapse(t *testing.T) {
	root := node("root", true, true, 0, node("a", false, false, 1), node("b", false, false, 1))
	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{
		{Data: &collections.ParsedCollection{ID: "c1", Root: root}},
	}
	ui.updateVisibleCols()
	if len(ui.VisibleCols) != 3 {
		t.Fatalf("len(VisibleCols) = %d, want 3", len(ui.VisibleCols))
	}
	root.Expanded = false
	ui.updateVisibleCols()
	if got := visibleNames(ui); !eqNames(got, []string{"root"}) {
		t.Errorf("visible = %v, want [root]", got)
	}
	for _, n := range ui.VisibleCols {
		if n == nil {
			t.Error("VisibleCols contains nil after shrink")
		}
	}
}

func envUI(id string, vars ...model.EnvVar) *environments.EnvironmentUI {
	return &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: id, Name: id, Vars: vars}}
}

func TestRefreshActiveEnv(t *testing.T) {
	ui := &AppUI{}
	ui.Environments = []*environments.EnvironmentUI{
		envUI("e1", model.EnvVar{Key: "host", Value: "a.com"}, model.EnvVar{Key: "empty", Value: ""}),
		envUI("e2", model.EnvVar{Key: "host", Value: "b.com"}),
	}
	ui.ActiveEnvID = "e1"

	ui.refreshActiveEnv()
	if ui.activeEnvVars != nil {
		t.Fatal("refreshActiveEnv ran while not dirty")
	}

	ui.activeEnvDirty = true
	ui.refreshActiveEnv()
	if ui.activeEnvVars["host"] != "a.com" {
		t.Errorf("host = %q, want a.com", ui.activeEnvVars["host"])
	}
	if _, ok := ui.activeEnvVars["empty"]; ok {
		t.Error("empty-valued var was included in active env vars")
	}
	if ui.activeEnvDirty {
		t.Error("refreshActiveEnv left dirty flag set")
	}

	ui.ActiveEnvID = "e2"
	ui.activeEnvDirty = true
	ui.refreshActiveEnv()
	if ui.activeEnvVars["host"] != "b.com" {
		t.Errorf("host = %q, want b.com", ui.activeEnvVars["host"])
	}

	ui.ActiveEnvID = "missing"
	ui.activeEnvDirty = true
	ui.refreshActiveEnv()
	if ui.activeEnvVars != nil {
		t.Errorf("activeEnvVars = %v, want nil for unknown env", ui.activeEnvVars)
	}
}

func TestActiveEnvSnapshot(t *testing.T) {
	ui := &AppUI{}
	if ui.activeEnvSnapshot() != nil {
		t.Error("snapshot of nil vars should be nil")
	}

	ui.activeEnvVars = map[string]string{"a": "1", "b": "2"}
	snap := ui.activeEnvSnapshot()
	if len(snap) != 2 || snap["a"] != "1" || snap["b"] != "2" {
		t.Fatalf("snapshot = %v, want {a:1 b:2}", snap)
	}

	snap["a"] = "mutated"
	delete(snap, "b")
	if ui.activeEnvVars["a"] != "1" || ui.activeEnvVars["b"] != "2" {
		t.Errorf("mutating snapshot changed source: %v", ui.activeEnvVars)
	}
}

func TestSetSidebarSection_HARHidesAndRestores(t *testing.T) {
	ui := &AppUI{}
	ui.SidebarSection = "requests"
	ui.Settings.HideSidebar = false

	ui.SetSidebarSection("har")
	if !ui.Settings.HideSidebar {
		t.Error("entering har did not hide sidebar")
	}
	if ui.SidebarSection != "har" {
		t.Errorf("SidebarSection = %q, want har", ui.SidebarSection)
	}

	ui.SetSidebarSection("requests")
	if ui.Settings.HideSidebar {
		t.Error("leaving har did not restore sidebar visibility")
	}
	if ui.sidebarHideSavedSet {
		t.Error("saved-flag still set after restore")
	}
}

func TestSetSidebarSection_PreservesHiddenPreference(t *testing.T) {
	ui := &AppUI{}
	ui.SidebarSection = "requests"
	ui.Settings.HideSidebar = true

	ui.SetSidebarSection("har")
	if !ui.Settings.HideSidebar {
		t.Error("har should keep sidebar hidden")
	}
	ui.SetSidebarSection("requests")
	if !ui.Settings.HideSidebar {
		t.Error("user preference for hidden sidebar was lost")
	}
}

func TestSetSidebarSection_ReentryDoesNotClobberSaved(t *testing.T) {
	ui := &AppUI{}
	ui.SidebarSection = "requests"
	ui.Settings.HideSidebar = false

	ui.SetSidebarSection("har")
	ui.SetSidebarSection("har")
	ui.SetSidebarSection("requests")
	if ui.Settings.HideSidebar {
		t.Error("re-entering har clobbered the saved sidebar preference")
	}
}

func TestSetSidebarSection_NonHARTransitions(t *testing.T) {
	ui := &AppUI{}
	ui.SidebarSection = "requests"
	ui.Settings.HideSidebar = false
	ui.SetSidebarSection("flows")
	if ui.Settings.HideSidebar {
		t.Error("non-har transition should not hide sidebar")
	}
	if ui.hideSidebar() {
		t.Error("hideSidebar() disagrees with Settings.HideSidebar")
	}
}

func TestEnsureDefaultEnvironment(t *testing.T) {
	ui := &AppUI{}
	env := ui.ensureDefaultEnvironment()
	if env == nil {
		t.Fatal("ensureDefaultEnvironment returned nil")
	}
	if env.Name != "Default" {
		t.Errorf("name = %q, want Default", env.Name)
	}
	if env.ID == "" {
		t.Error("created environment has empty ID")
	}
	if len(ui.Environments) != 1 {
		t.Fatalf("len(Environments) = %d, want 1", len(ui.Environments))
	}
	if !ui.EnvsExpanded {
		t.Error("EnvsExpanded not set after creating default env")
	}

	again := ui.ensureDefaultEnvironment()
	if again != env {
		t.Error("ensureDefaultEnvironment created a second environment")
	}
	if len(ui.Environments) != 1 {
		t.Errorf("len(Environments) = %d, want 1", len(ui.Environments))
	}
}
