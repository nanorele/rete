package environments

import (
	"bytes"
	"errors"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"io"
	"path/filepath"
	"rete/internal/model"
	"rete/internal/persist"
	"strings"
	"testing"
	"time"
)

func setupEnvConfig(t *testing.T) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rete-test")
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

func TestParseEnvironment(t *testing.T) {
	jsonStr := `
	{
		"name": "Test Environment",
		"values": [
			{
				"key": "API_URL",
				"value": "http://example.com",
				"enabled": true
			},
			{
				"key": "TOKEN",
				"value": "secret",
				"enabled": false
			}
		]
	}`

	env, err := ParseEnvironment(strings.NewReader(jsonStr), "env1")
	if err != nil {
		t.Fatalf("ParseEnvironment error: %v", err)
	}

	if env.ID != "env1" {
		t.Errorf("expected ID env1, got %s", env.ID)
	}
	if env.Name != "Test Environment" {
		t.Errorf("expected Test Environment, got %s", env.Name)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(env.Vars))
	}

	if env.Vars[0].Key != "API_URL" || env.Vars[0].Value != "http://example.com" {
		t.Errorf("unexpected var 0: %+v", env.Vars[0])
	}
	if env.Vars[1].Key != "TOKEN" || env.Vars[1].Value != "secret" {
		t.Errorf("unexpected var 1: %+v", env.Vars[1])
	}

	_, err = ParseEnvironment(strings.NewReader("invalid"), "env2")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	jsonWithValues := `{"name": "", "values": [{"key":"k","value":"v"}]}`
	envEmpty, _ := ParseEnvironment(strings.NewReader(jsonWithValues), "env3")
	if envEmpty.Name != "Imported Environment" {
		t.Errorf("expected Imported Environment, got %s", envEmpty.Name)
	}
}

func TestParseEnvironment_Errors(t *testing.T) {
	_, err := ParseEnvironment(strings.NewReader("invalid"), "env1")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	_, err = ParseEnvironment(strings.NewReader("{}"), "env2")
	if err == nil {
		t.Errorf("expected error for empty json object")
	}
}

func TestEnvironmentUI_InitEditor(t *testing.T) {
	env := &model.ParsedEnvironment{
		Name: "Init Test",
		Vars: []model.EnvVar{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		},
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()

	if ui.NameEditor.Text() != "Init Test" {
		t.Errorf("expected NameEditor to be Init Test, got %s", ui.NameEditor.Text())
	}
	if len(ui.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(ui.Rows))
	}

	if ui.Rows[0].KeyEditor.Text() != "k1" || ui.Rows[0].ValEditor.Text() != "v1" {
		t.Errorf("unexpected row 0: %s %s", ui.Rows[0].KeyEditor.Text(), ui.Rows[0].ValEditor.Text())
	}
	if ui.Rows[1].KeyEditor.Text() != "k2" || ui.Rows[1].ValEditor.Text() != "v2" {
		t.Errorf("unexpected row 1: %s %s", ui.Rows[1].KeyEditor.Text(), ui.Rows[1].ValEditor.Text())
	}
	if ui.List.Axis != 1 {
		t.Errorf("expected List.Axis to be 1")
	}
}

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return 0, errors.New("read fail") }

func TestParseEnvironment_NoEnabledField(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(env.Vars))
	}
	if env.Vars[0].Key != "k" || env.Vars[0].Value != "v" {
		t.Errorf("unexpected var: %+v", env.Vars[0])
	}
}

func TestParseEnvironment_LegacyEnabledFieldIgnored(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"a","value":"1","enabled":false},
		{"key":"b","value":"2","enabled":true}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars, got %d", len(env.Vars))
	}
	if env.Vars[0].Key != "a" || env.Vars[0].Value != "1" {
		t.Errorf("legacy enabled:false must not drop or alter the var: %+v", env.Vars[0])
	}
	if env.Vars[1].Key != "b" || env.Vars[1].Value != "2" {
		t.Errorf("unexpected var: %+v", env.Vars[1])
	}
}

func TestParseEnvironment_HighlightColorPresent(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}],"highlight_color":"#aabbcc"}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "#aabbcc" {
		t.Errorf("expected highlight_color=#aabbcc, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_HighlightColorMissing(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "" {
		t.Errorf("expected empty HighlightColor when absent, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_HighlightColorEmptyString(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}],"highlight_color":""}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "" {
		t.Errorf("expected empty HighlightColor, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_KeyOrderPreserved(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"z","value":"1"},
		{"key":"a","value":"2"},
		{"key":"m","value":"3"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"z", "a", "m"}
	if len(env.Vars) != len(want) {
		t.Fatalf("expected %d vars, got %d", len(want), len(env.Vars))
	}
	for i, k := range want {
		if env.Vars[i].Key != k {
			t.Errorf("at index %d: expected key %q, got %q", i, k, env.Vars[i].Key)
		}
	}
}

func TestParseEnvironment_DuplicateKeysKept(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"k","value":"first"},
		{"key":"k","value":"second"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars (duplicates kept), got %d", len(env.Vars))
	}
	if env.Vars[0].Value != "first" || env.Vars[1].Value != "second" {
		t.Errorf("duplicates not preserved in order: %+v", env.Vars)
	}
}

func TestParseEnvironment_EmptyKeyKept(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"","value":"v1"},
		{"key":"k","value":"v2"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 2 {
		t.Errorf("expected empty-key entries to be kept, got %d vars", len(env.Vars))
	}
	if env.Vars[0].Key != "" {
		t.Errorf("expected empty key preserved, got %q", env.Vars[0].Key)
	}
}

func TestParseEnvironment_EmptyValuesArrayWithName(t *testing.T) {
	jsonStr := `{"name":"OnlyName","values":[]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "OnlyName" {
		t.Errorf("expected Name=OnlyName, got %q", env.Name)
	}
	if len(env.Vars) != 0 {
		t.Errorf("expected 0 vars, got %d", len(env.Vars))
	}
}

func TestParseEnvironment_EmptyNameWithValues(t *testing.T) {
	jsonStr := `{"values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "Imported Environment" {
		t.Errorf("expected fallback name, got %q", env.Name)
	}
}

func TestParseEnvironment_BothEmptyReturnsError(t *testing.T) {
	jsonStr := `{"name":"","values":[]}`
	_, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err == nil {
		t.Errorf("expected error for empty name and empty values")
	}
}

func TestParseEnvironment_MalformedJSON(t *testing.T) {
	cases := []string{
		`{`,
		`{"name":}`,
		`{"name":"x","values":"notarray"}`,
		`{"name":"x","values":[{"key":1}]}`,
	}
	for _, c := range cases {
		_, err := ParseEnvironment(strings.NewReader(c), "id")
		if err == nil {
			t.Errorf("expected error for malformed JSON: %q", c)
		}
	}
}

func TestParseEnvironment_ReaderError(t *testing.T) {
	_, err := ParseEnvironment(failingReader{}, "id")
	if err == nil {
		t.Errorf("expected error when reader fails")
	}
}

func TestParseEnvironment_IDPropagated(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	cases := []string{"id-1", "", "long-uuid-1234-5678", "with spaces"}
	for _, id := range cases {
		env, err := ParseEnvironment(strings.NewReader(jsonStr), id)
		if err != nil {
			t.Fatalf("unexpected error for id %q: %v", id, err)
		}
		if env.ID != id {
			t.Errorf("expected ID=%q, got %q", id, env.ID)
		}
	}
}

func TestParseEnvironment_BytesReader(t *testing.T) {
	data := []byte(`{"name":"B","values":[{"key":"k","value":"v"}]}`)
	env, err := ParseEnvironment(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "B" {
		t.Errorf("expected Name=B, got %q", env.Name)
	}
}

func TestHighlightColor_NilEnv(t *testing.T) {
	c := HighlightColor(nil)
	if c.A == 0 {
		t.Errorf("expected non-transparent accent color for nil env")
	}
}

func TestParseEnvironment_EmptyReturnsUnexpectedEOF(t *testing.T) {
	_, err := ParseEnvironment(strings.NewReader(`{"name":"","values":[]}`), "id")
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}

type wordJumpRig struct {
	r    input.Router
	th   *material.Theme
	ui   *EnvironmentUI
	host *EditorHost
	now  time.Time
}

func newWordJumpRig(t *testing.T) *wordJumpRig {
	t.Helper()
	setupEnvConfig(t)
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	env := &model.ParsedEnvironment{
		ID:   "envWJ",
		Name: "WordJump",
		Vars: []model.EnvVar{{Key: "token", Value: "alpha beta gamma"}},
	}
	ui := &EnvironmentUI{Data: env}
	ui.InitEditor()
	return &wordJumpRig{
		r:    input.Router{},
		th:   th,
		ui:   ui,
		host: &EditorHost{Theme: th},
		now:  time.Unix(1700000000, 0),
	}
}

func (rg *wordJumpRig) frame(execute func(gtx layout.Context)) {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         rg.now,
		Source:      rg.r.Source(),
	}
	if execute != nil {
		execute(gtx)
	}
	rg.ui.LayoutEditor(gtx, rg.host)
	rg.r.Frame(gtx.Ops)
}

func (rg *wordJumpRig) keyPress(name key.Name, mods key.Modifiers) {
	rg.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rg.frame(nil)
}

func TestLayoutEditor_CtrlArrowMovesByWord(t *testing.T) {
	rg := newWordJumpRig(t)
	rg.frame(nil)
	ed := &rg.ui.Rows[0].ValEditor
	rg.frame(func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: ed})
	})
	ed.SetCaret(16, 16)
	rg.frame(nil)

	rg.keyPress(key.NameLeftArrow, key.ModShortcut)
	if s, e := ed.Selection(); s != 11 || e != 11 {
		t.Errorf("Ctrl+Left: caret = (%d,%d), want (11,11)", s, e)
	}

	rg.keyPress(key.NameRightArrow, key.ModShortcut)
	if s, e := ed.Selection(); s != 16 || e != 16 {
		t.Errorf("Ctrl+Right: caret = (%d,%d), want (16,16)", s, e)
	}
}

func TestLayoutEditor_CtrlArrowStopsAtURLPunctuation(t *testing.T) {
	rg := newWordJumpRig(t)
	rg.frame(nil)
	ed := &rg.ui.Rows[0].ValEditor
	ed.SetText("https://3400.api.green-api.com/")
	rg.frame(func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: ed})
	})
	ed.SetCaret(31, 31)
	rg.frame(nil)

	rg.keyPress(key.NameLeftArrow, key.ModShortcut)
	if s, _ := ed.Selection(); s != 27 {
		t.Errorf("Ctrl+Left from end: caret = %d, want 27 (start of \"com/\")", s)
	}

	rg.keyPress(key.NameLeftArrow, key.ModShortcut)
	if s, _ := ed.Selection(); s != 23 {
		t.Errorf("Ctrl+Left again: caret = %d, want 23 (start of \"api\", '-' separates words)", s)
	}

	ed.SetCaret(0, 0)
	rg.frame(nil)
	rg.keyPress(key.NameRightArrow, key.ModShortcut)
	if s, _ := ed.Selection(); s != 5 {
		t.Errorf("Ctrl+Right from start: caret = %d, want 5 (end of \"https\")", s)
	}
}

func TestLayoutEditor_CtrlShiftArrowExtendsByWord(t *testing.T) {
	rg := newWordJumpRig(t)
	rg.frame(nil)
	ed := &rg.ui.Rows[0].ValEditor
	rg.frame(func(gtx layout.Context) {
		gtx.Execute(key.FocusCmd{Tag: ed})
	})
	ed.SetCaret(16, 16)
	rg.frame(nil)

	rg.keyPress(key.NameLeftArrow, key.ModShortcut|key.ModShift)
	s, e := ed.Selection()
	lo, hi := s, e
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo != 11 || hi != 16 {
		t.Errorf("Ctrl+Shift+Left: selection = (%d,%d), want covering [11,16]", s, e)
	}
}
