package environments

import (
	"image"
	"path/filepath"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/persist"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func setupEnvConfig(t *testing.T) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tracto-test")
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })
}

func makeGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
}

func TestCommit_NilSafe(t *testing.T) {

	var nilUI *EnvironmentUI
	nilUI.Commit(nil)

	ui := &EnvironmentUI{}
	ui.Commit(nil)
}

func TestCommit_WritesNameAndVars(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envA", Name: "Old"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	ui.NameEditor.SetText("New Name")
	ui.Rows = append(ui.Rows,
		&EnvVarRow{},
		&EnvVarRow{},
	)
	ui.Rows[0].KeyEditor.SetText(" key1 ")
	ui.Rows[0].ValEditor.SetText("v1")
	ui.Rows[1].KeyEditor.SetText("key2")
	ui.Rows[1].ValEditor.SetText("v2")

	called := false
	ui.Commit(func() { called = true })

	if env.Name != "New Name" {
		t.Errorf("expected Name=New Name, got %q", env.Name)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(env.Vars))
	}
	if env.Vars[0].Key != "key1" {
		t.Errorf("expected key trimmed to %q, got %q", "key1", env.Vars[0].Key)
	}
	if !called {
		t.Errorf("expected onDirty callback invoked")
	}
}

func TestCommit_SkipsEmptyKeys(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envB", Name: "X"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	ui.Rows = append(ui.Rows, &EnvVarRow{}, &EnvVarRow{}, &EnvVarRow{})
	ui.Rows[0].KeyEditor.SetText("   ")
	ui.Rows[0].ValEditor.SetText("v1")
	ui.Rows[1].KeyEditor.SetText("real")
	ui.Rows[1].ValEditor.SetText("v2")
	ui.Rows[2].KeyEditor.SetText("")
	ui.Rows[2].ValEditor.SetText("v3")

	ui.Commit(nil)

	if len(env.Vars) != 1 {
		t.Fatalf("expected 1 var after skipping empties, got %d", len(env.Vars))
	}
	if env.Vars[0].Key != "real" {
		t.Errorf("expected the 'real' key kept, got %q", env.Vars[0].Key)
	}
}

func TestCommit_HighlightColorParsing(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envC", Name: "X", HighlightColor: "#aaaaaa"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	ui.ColorEditor.SetText("#ff0000")
	ui.Commit(nil)
	if env.HighlightColor != "#ff0000" {
		t.Errorf("expected #ff0000, got %q", env.HighlightColor)
	}

	ui.ColorEditor.SetText("notahex")
	ui.Commit(nil)
	if env.HighlightColor != "#ff0000" {
		t.Errorf("expected #ff0000 preserved on invalid input, got %q", env.HighlightColor)
	}

	ui.ColorEditor.SetText("")
	ui.Commit(nil)
	if env.HighlightColor != "" {
		t.Errorf("expected cleared HighlightColor, got %q", env.HighlightColor)
	}
}

func TestCommit_VarsResetEachCall(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{
		ID:   "envD",
		Name: "X",
		Vars: []model.EnvVar{
			{Key: "stale", Value: "old"},
		},
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	ui.Rows[0].KeyEditor.SetText("")
	ui.Commit(nil)
	if len(env.Vars) != 0 {
		t.Errorf("expected vars cleared when no non-empty rows, got %d", len(env.Vars))
	}
}

func TestLayoutEditor_NilSafe(t *testing.T) {
	var nilUI *EnvironmentUI
	dims := nilUI.LayoutEditor(makeGtx(), &EditorHost{Theme: material.NewTheme()})
	if dims.Size.X != 0 || dims.Size.Y != 0 {
		t.Errorf("expected zero dims for nil receiver, got %+v", dims)
	}
}

func TestLayoutEditor_SmokeRender(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{
		ID:   "envE",
		Name: "Smoke",
		Vars: []model.EnvVar{
			{Key: "a", Value: "1"},
			{Key: "b", Value: "2"},
		},
		HighlightColor: "#abcdef",
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	host := &EditorHost{Theme: material.NewTheme()}
	dims := ui.LayoutEditor(makeGtx(), host)
	if dims.Size.X == 0 && dims.Size.Y == 0 {
		t.Errorf("expected non-zero layout dims for smoke render")
	}
}

func TestLayoutEditor_AddButton(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envF", Name: "X"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	host := &EditorHost{Theme: material.NewTheme()}
	ui.LayoutEditor(makeGtx(), host)

	startRows := len(ui.Rows)
	ui.AddBtn.Click()
	ui.LayoutEditor(makeGtx(), host)
	if len(ui.Rows) != startRows+1 {
		t.Errorf("expected rows to grow by 1 after AddBtn click, got %d→%d", startRows, len(ui.Rows))
	}
}

func TestLayoutEditor_DeleteRow(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{
		ID:   "envG",
		Name: "X",
		Vars: []model.EnvVar{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
			{Key: "k3", Value: "v3"},
		},
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	host := &EditorHost{Theme: material.NewTheme()}
	ui.LayoutEditor(makeGtx(), host)

	if len(ui.Rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(ui.Rows))
	}

	ui.Rows[1].DelBtn.Click()
	ui.LayoutEditor(makeGtx(), host)
	if len(ui.Rows) != 2 {
		t.Errorf("expected 2 rows after delete, got %d", len(ui.Rows))
	}
	if ui.Rows[0].KeyEditor.Text() != "k1" || ui.Rows[1].KeyEditor.Text() != "k3" {
		t.Errorf("unexpected row order after delete: %q, %q",
			ui.Rows[0].KeyEditor.Text(), ui.Rows[1].KeyEditor.Text())
	}
}

func TestLayoutEditor_DeleteLastRow(t *testing.T) {

	setupEnvConfig(t)
	env := &model.ParsedEnvironment{
		ID:   "envH",
		Name: "X",
		Vars: []model.EnvVar{
			{Key: "only", Value: "v"},
		},
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	host := &EditorHost{Theme: material.NewTheme()}
	ui.LayoutEditor(makeGtx(), host)
	ui.Rows[0].DelBtn.Click()
	ui.LayoutEditor(makeGtx(), host)
	if len(ui.Rows) != 0 {
		t.Errorf("expected 0 rows after deleting last, got %d", len(ui.Rows))
	}
}

func TestLayoutEditor_ColorResetClears(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envI", Name: "X", HighlightColor: "#112233"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	if ui.ColorEditor.Text() != "#112233" {
		t.Fatalf("setup: expected color editor to mirror data, got %q", ui.ColorEditor.Text())
	}

	host := &EditorHost{Theme: material.NewTheme()}
	ui.ColorReset.Click()
	ui.LayoutEditor(makeGtx(), host)

	if ui.ColorEditor.Text() != "" {
		t.Errorf("expected ColorEditor cleared, got %q", ui.ColorEditor.Text())
	}
	if env.HighlightColor != "" {
		t.Errorf("expected Data.HighlightColor cleared, got %q", env.HighlightColor)
	}
}

func TestLayoutEditor_SaveButtonCommits(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envJ", Name: "OldName"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()
	ui.NameEditor.SetText("Saved")

	dirtyHits := 0
	host := &EditorHost{Theme: material.NewTheme(), OnDirty: func() { dirtyHits++ }}

	ui.SaveBtn.Click()
	ui.LayoutEditor(makeGtx(), host)

	if env.Name != "Saved" {
		t.Errorf("expected Name=Saved, got %q", env.Name)
	}
	if dirtyHits != 1 {
		t.Errorf("expected OnDirty called once, got %d", dirtyHits)
	}
}

func TestLayoutEditor_BackButtonCommitsAndCloses(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envK", Name: "OldName"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()
	ui.NameEditor.SetText("BackSaved")

	closed := false
	dirtyHits := 0
	host := &EditorHost{
		Theme:   material.NewTheme(),
		OnClose: func() { closed = true },
		OnDirty: func() { dirtyHits++ },
	}

	ui.BackBtn.Click()
	dims := ui.LayoutEditor(makeGtx(), host)

	if env.Name != "BackSaved" {
		t.Errorf("expected commit on back, got Name=%q", env.Name)
	}
	if !closed {
		t.Errorf("expected OnClose invoked")
	}
	if dirtyHits != 1 {
		t.Errorf("expected OnDirty called once, got %d", dirtyHits)
	}
	if dims.Size.X != 0 || dims.Size.Y != 0 {
		t.Errorf("expected zero dims when back closes early, got %+v", dims)
	}
}

func TestHighlightColor_Valid(t *testing.T) {
	env := &model.ParsedEnvironment{HighlightColor: "#102030"}
	c := HighlightColor(env)
	if c.R != 0x10 || c.G != 0x20 || c.B != 0x30 {
		t.Errorf("expected RGB 10/20/30, got %v", c)
	}
}

func TestHighlightColor_InvalidFallsBackToAccent(t *testing.T) {
	env := &model.ParsedEnvironment{HighlightColor: "garbage"}
	c := HighlightColor(env)
	if c.A == 0 {
		t.Errorf("expected non-transparent fallback color, got %v", c)
	}
}

func TestHighlightColor_EmptyFallsBackToAccent(t *testing.T) {
	env := &model.ParsedEnvironment{HighlightColor: ""}
	c := HighlightColor(env)
	if c.A == 0 {
		t.Errorf("expected non-transparent fallback, got %v", c)
	}
}

func TestLoadAll_EmptyDir(t *testing.T) {
	setupEnvConfig(t)
	got := LoadAll()
	if got != nil {
		t.Errorf("expected nil slice for empty dir, got %v", got)
	}
}

func TestLoadAll_RoundTrip(t *testing.T) {
	setupEnvConfig(t)

	envs := []*model.ParsedEnvironment{
		{ID: "id1", Name: "First", Vars: []model.EnvVar{{Key: "a", Value: "1"}}},
		{ID: "id2", Name: "Second", HighlightColor: "#abcdef"},
	}
	for _, e := range envs {
		if err := persist.SaveEnvironment(e); err != nil {
			t.Fatalf("SaveEnvironment(%s): %v", e.ID, err)
		}
	}

	loaded := LoadAll()
	if len(loaded) != 2 {
		t.Fatalf("expected 2 envs loaded, got %d", len(loaded))
	}

	names := map[string]bool{}
	for _, e := range loaded {
		names[e.Name] = true
	}
	if !names["First"] || !names["Second"] {
		t.Errorf("expected both envs round-tripped, got %v", names)
	}
}

func TestInitEditor_TruncatesExtraRows(t *testing.T) {

	env := &model.ParsedEnvironment{
		Name: "trunc",
		Vars: []model.EnvVar{{Key: "k1", Value: "v1"}},
	}
	ui := &EnvironmentUI{
		Data: env,
		Rows: []*EnvVarRow{{}, {}, {}},
	}
	ui.InitEditor()
	if len(ui.Rows) != 1 {
		t.Errorf("expected rows truncated to 1, got %d", len(ui.Rows))
	}
}

func TestInitEditor_ReusesExistingRows(t *testing.T) {
	env := &model.ParsedEnvironment{
		Name: "reuse",
		Vars: []model.EnvVar{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		},
	}
	pre := &EnvVarRow{}
	pre.KeyEditor.SetText("stale")
	ui := &EnvironmentUI{Data: env, Rows: []*EnvVarRow{pre}}
	ui.InitEditor()
	if len(ui.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(ui.Rows))
	}
	if ui.Rows[0] != pre {
		t.Errorf("expected first row pointer reused")
	}
	if ui.Rows[0].KeyEditor.Text() != "k1" {
		t.Errorf("expected reused row updated to k1, got %q", ui.Rows[0].KeyEditor.Text())
	}
}

func TestCommit_AddedVariablePersistsWithValue(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envNew", Name: "New"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	host := &EditorHost{Theme: material.NewTheme()}
	ui.AddBtn.Click()
	ui.LayoutEditor(makeGtx(), host)
	if len(ui.Rows) != 1 {
		t.Fatalf("expected 1 row after add, got %d", len(ui.Rows))
	}
	ui.Rows[0].KeyEditor.SetText("api")
	ui.Rows[0].ValEditor.SetText("http://example.com")

	ui.Commit(nil)

	if len(env.Vars) != 1 || env.Vars[0].Key != "api" || env.Vars[0].Value != "http://example.com" {
		t.Fatalf("unexpected committed vars: %+v", env.Vars)
	}

	loaded := LoadAll()
	var got *model.ParsedEnvironment
	for _, e := range loaded {
		if e.ID == "envNew" {
			got = e
		}
	}
	if got == nil {
		t.Fatalf("environment not persisted to disk")
	}
	if len(got.Vars) != 1 || got.Vars[0].Key != "api" || got.Vars[0].Value != "http://example.com" {
		t.Fatalf("reloaded var wrong (a value-bearing var must survive round-trip): %+v", got.Vars)
	}
}

func TestLayoutEditor_BackReturnsEarlyBeforeOtherEvents(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envBack", Name: "X"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	closed := 0
	host := &EditorHost{Theme: material.NewTheme(), OnClose: func() { closed++ }}

	ui.BackBtn.Click()
	ui.AddBtn.Click()
	dims := ui.LayoutEditor(makeGtx(), host)

	if closed != 1 {
		t.Errorf("expected OnClose invoked exactly once, got %d", closed)
	}
	if len(ui.Rows) != 0 {
		t.Errorf("back must return before the AddBtn handler runs, so no row is added; got %d rows", len(ui.Rows))
	}
	if dims.Size.X != 0 || dims.Size.Y != 0 {
		t.Errorf("expected zero dims when back closes early, got %+v", dims)
	}
}

func TestLayoutEditor_BackProcessedOncePerFrame(t *testing.T) {
	setupEnvConfig(t)
	env := &model.ParsedEnvironment{ID: "envBack2", Name: "X"}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	closed := 0
	host := &EditorHost{Theme: material.NewTheme(), OnClose: func() { closed++ }}

	ui.BackBtn.Click()
	ui.BackBtn.Click()
	ui.LayoutEditor(makeGtx(), host)

	if closed != 1 {
		t.Errorf("multiple queued back clicks must collapse to a single close per frame, got %d", closed)
	}
}
