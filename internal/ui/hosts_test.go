package ui

import (
	"context"
	"crypto/tls"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "tracto-test")
	persist.SetConfigOverride(cfg)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	switch {
	case os.Getenv("APPDATA") != "":
		t.Setenv("AppData", dir)
	default:
		t.Setenv("XDG_CONFIG_HOME", dir)
	}
	return cfg
}

type uiRig struct {
	ui  *AppUI
	r   input.Router
	sz  image.Point
	now time.Time
}

func newUIRig(t *testing.T, sz image.Point) *uiRig {
	t.Helper()
	isolateConfig(t)
	u := &AppUI{
		Theme:            material.NewTheme(),
		Window:           new(app.Window),
		SidebarSection:   "requests",
		SidebarWidth:     250,
		dirtyCollections: make(map[string]*dirtyCollection),
		Settings:         model.DefaultSettings(),
		windowSize:       sz,
	}
	u.rootCtx, u.rootCancel = context.WithCancel(context.Background())
	t.Cleanup(u.rootCancel)
	return &uiRig{ui: u, sz: sz, now: time.Unix(1700000000, 0)}
}

func (rig *uiRig) gtx() layout.Context {
	rig.now = rig.now.Add(16 * time.Millisecond)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
}

func (rig *uiRig) frame(fn func(gtx layout.Context)) layout.Context {
	gtx := rig.gtx()
	fn(gtx)
	rig.r.Frame(gtx.Ops)
	return gtx
}

func colWithID(id string) *collections.ParsedCollection {
	return &collections.ParsedCollection{
		ID:   id,
		Name: id,
		Root: &collections.CollectionNode{Name: id, IsFolder: true},
	}
}

func TestFlushCollectionSaves_EmptyClearsTimerFlag(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui
	ui.collectionFlushTimerSet = true

	ui.flushCollectionSaves()

	if ui.collectionFlushTimerSet {
		t.Error("flushCollectionSaves left the timer flag set with nothing dirty")
	}
}

func TestFlushCollectionSaves_WritesDueCollection(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	col := colWithID("due-col")
	ui.markCollectionDirty(col)
	ui.dirtyCollections[col.ID].last = time.Now().Add(-2 * collectionSaveDebounce)

	ui.flushCollectionSaves()
	ui.collectionSaveWG.Wait()

	if len(ui.dirtyCollections) != 0 {
		t.Errorf("dirtyCollections = %d entries, want the due entry drained", len(ui.dirtyCollections))
	}
	path := filepath.Join(persist.CollectionsDir(), col.ID+".json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("collection file not written to %s: %v", path, err)
	}
}

func TestFlushCollectionSaves_HoldsUndebouncedEntry(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	col := colWithID("fresh-col")
	ui.markCollectionDirty(col)

	ui.flushCollectionSaves()
	ui.collectionSaveWG.Wait()

	if len(ui.dirtyCollections) != 1 {
		t.Fatalf("dirtyCollections = %d entries, want the fresh entry retained", len(ui.dirtyCollections))
	}
	if !ui.collectionFlushTimerSet {
		t.Error("a pending entry did not reschedule the flush timer")
	}
	path := filepath.Join(persist.CollectionsDir(), col.ID+".json")
	if _, err := os.Stat(path); err == nil {
		t.Error("an undebounced collection was written to disk immediately")
	}
}

func TestFlushCollectionSaves_SkipsDeletedCollection(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	col := colWithID("gone-col")
	ui.markCollectionDirty(col)
	ui.dirtyCollections[col.ID].last = time.Now().Add(-2 * collectionSaveDebounce)
	ui.deletedCollections = map[string]struct{}{col.ID: {}}

	ui.flushCollectionSaves()
	ui.collectionSaveWG.Wait()

	if len(ui.dirtyCollections) != 0 {
		t.Errorf("deleted collection left %d dirty entries", len(ui.dirtyCollections))
	}
	path := filepath.Join(persist.CollectionsDir(), col.ID+".json")
	if _, err := os.Stat(path); err == nil {
		t.Error("a deleted collection was resurrected on disk")
	}
}

func TestFlushCollectionSaves_SkipsEmptySnapshot(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	rootless := &collections.ParsedCollection{ID: "no-root", Name: "no-root"}
	ui.markCollectionDirty(rootless)
	ui.dirtyCollections[rootless.ID].last = time.Now().Add(-2 * collectionSaveDebounce)

	ui.flushCollectionSaves()
	ui.collectionSaveWG.Wait()

	if len(ui.dirtyCollections) != 0 {
		t.Errorf("dirtyCollections = %d entries, want drained", len(ui.dirtyCollections))
	}
	path := filepath.Join(persist.CollectionsDir(), rootless.ID+".json")
	if _, err := os.Stat(path); err == nil {
		t.Error("a collection with an empty snapshot produced a file")
	}
}

func TestFlushCollectionSaves_MixedDueAndPending(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	due := colWithID("mixed-due")
	fresh := colWithID("mixed-fresh")
	ui.markCollectionDirty(due)
	ui.markCollectionDirty(fresh)
	ui.dirtyCollections[due.ID].last = time.Now().Add(-2 * collectionSaveDebounce)

	ui.flushCollectionSaves()
	ui.collectionSaveWG.Wait()

	if _, ok := ui.dirtyCollections[due.ID]; ok {
		t.Error("due collection was not drained")
	}
	if _, ok := ui.dirtyCollections[fresh.ID]; !ok {
		t.Error("fresh collection was drained too early")
	}
	if _, err := os.Stat(filepath.Join(persist.CollectionsDir(), due.ID+".json")); err != nil {
		t.Errorf("due collection file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(persist.CollectionsDir(), fresh.ID+".json")); err == nil {
		t.Error("fresh collection was written despite the debounce")
	}
}

func TestSaveEnvironmentAsync(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	ui.saveEnvironmentAsync(nil)
	ui.envSaveWG.Wait()

	env := &model.ParsedEnvironment{
		ID:   "env-1",
		Name: "Staging",
		Vars: []model.EnvVar{{Key: "host", Value: "stg.example.com"}},
	}
	ui.saveEnvironmentAsync(env)
	ui.envSaveWG.Wait()

	path := filepath.Join(persist.EnvironmentsDir(), env.ID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("environment not written to %s: %v", path, err)
	}
	if len(data) == 0 {
		t.Error("environment file is empty")
	}
}

func TestSaveEnvironmentAsync_ConcurrentWritesSettle(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	for i := 0; i < 8; i++ {
		env := &model.ParsedEnvironment{
			ID:   "env-multi",
			Name: "Env",
			Vars: []model.EnvVar{{Key: "n", Value: string(rune('a' + i))}},
		}
		ui.saveEnvironmentAsync(env)
	}
	ui.envSaveWG.Wait()

	if _, err := os.Stat(filepath.Join(persist.EnvironmentsDir(), "env-multi.json")); err != nil {
		t.Fatalf("environment file missing after concurrent writes: %v", err)
	}
}

func TestRenderColorPickerOverlay_ClampsIntoViewport(t *testing.T) {
	cases := []struct {
		name   string
		size   image.Point
		anchor colorpicker.Anchor
	}{
		{"top-left", image.Pt(800, 600), colorpicker.Anchor{X: 10, Y: 10}},
		{"bottom-right", image.Pt(800, 600), colorpicker.Anchor{X: 790, Y: 590}},
		{"far-off-right", image.Pt(800, 600), colorpicker.Anchor{X: 5000, Y: 20}},
		{"far-off-bottom", image.Pt(800, 600), colorpicker.Anchor{X: 20, Y: 5000}},
		{"negative", image.Pt(800, 600), colorpicker.Anchor{X: -400, Y: -400}},
		{"viewport-smaller-than-picker", image.Pt(100, 80), colorpicker.Anchor{X: 50, Y: 40}},
		{"zero-viewport", image.Pt(0, 0), colorpicker.Anchor{X: 0, Y: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, tc.size)
			p := &colorpicker.State{}
			p.Open(colorpicker.KindEnv, 0, rig.ui.Theme.Palette.ContrastBg, tc.anchor)

			rig.frame(func(gtx layout.Context) {
				rig.ui.renderColorPickerOverlay(gtx, p)
			})

			if !p.IsOpen() {
				t.Error("rendering alone closed the picker")
			}
		})
	}
}

func TestRenderColorPickerOverlay_BackdropPressOutsideCloses(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	p := &colorpicker.State{}
	p.Open(colorpicker.KindEnv, 0, rig.ui.Theme.Palette.ContrastBg, colorpicker.Anchor{X: 20, Y: 20})

	rig.frame(func(gtx layout.Context) { rig.ui.renderColorPickerOverlay(gtx, p) })

	rig.r.Queue(pointer.Event{
		Kind:     pointer.Press,
		Position: f32.Pt(700, 500),
		Buttons:  pointer.ButtonPrimary,
		Source:   pointer.Mouse,
	})
	rig.frame(func(gtx layout.Context) { rig.ui.renderColorPickerOverlay(gtx, p) })

	if p.IsOpen() {
		t.Error("a press on the backdrop outside the picker did not close it")
	}
}

func TestRenderColorPickerOverlay_PressInsidePickerKeepsOpen(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	p := &colorpicker.State{}
	p.Open(colorpicker.KindEnv, 0, rig.ui.Theme.Palette.ContrastBg, colorpicker.Anchor{X: 20, Y: 20})

	rig.frame(func(gtx layout.Context) { rig.ui.renderColorPickerOverlay(gtx, p) })

	rig.r.Queue(pointer.Event{
		Kind:     pointer.Press,
		Position: f32.Pt(40, 40),
		Buttons:  pointer.ButtonPrimary,
		Source:   pointer.Mouse,
	})
	rig.frame(func(gtx layout.Context) { rig.ui.renderColorPickerOverlay(gtx, p) })

	if !p.IsOpen() {
		t.Error("a press inside the picker body closed it")
	}
}

func TestLayoutColorPickerOverlay_UsesSettingsPicker(t *testing.T) {
	rig := newUIRig(t, image.Pt(1000, 700))
	rig.ui.SettingsState = settings.NewEditor(rig.ui.Settings)
	rig.ui.SettingsState.ColorPicker.Open(
		colorpicker.KindEnv, 0, rig.ui.Theme.Palette.ContrastBg,
		colorpicker.Anchor{X: 990, Y: 690},
	)

	rig.frame(func(gtx layout.Context) { rig.ui.layoutColorPickerOverlay(gtx) })

	if !rig.ui.SettingsState.ColorPicker.IsOpen() {
		t.Error("layoutColorPickerOverlay closed the settings picker")
	}

	rig.r.Queue(pointer.Event{
		Kind:     pointer.Press,
		Position: f32.Pt(20, 20),
		Buttons:  pointer.ButtonPrimary,
		Source:   pointer.Mouse,
	})
	rig.frame(func(gtx layout.Context) { rig.ui.layoutColorPickerOverlay(gtx) })

	if rig.ui.SettingsState.ColorPicker.IsOpen() {
		t.Error("backdrop press did not close the settings picker")
	}
}

func TestFlowHost_EnvOptionsAndVars(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.Environments = []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{
			{Key: "host", Value: "dev.example.com"},
			{Key: "blank", Value: ""},
		}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod", Vars: []model.EnvVar{
			{Key: "host", Value: "prod.example.com"},
		}}},
		nil,
		{Data: nil},
	}
	ui.activeEnvVars = map[string]string{"host": "active.example.com"}

	h := ui.flowHost()

	opts := h.EnvOptions()
	if len(opts) != 3 {
		t.Fatalf("EnvOptions len = %d, want 3 (active + 2 named)", len(opts))
	}
	if opts[0].ID != "" || opts[0].Name != "Active environment" {
		t.Errorf("first option = %+v, want the active-environment sentinel", opts[0])
	}
	if opts[1].ID != "e1" || opts[2].ID != "e2" {
		t.Errorf("named options = %v, want e1 then e2", []string{opts[1].ID, opts[2].ID})
	}

	if got := h.EnvVars(""); got["host"] != "active.example.com" {
		t.Errorf(`EnvVars("")["host"] = %q, want the active snapshot`, got["host"])
	}
	e1 := h.EnvVars("e1")
	if e1["host"] != "dev.example.com" {
		t.Errorf(`EnvVars("e1")["host"] = %q, want dev.example.com`, e1["host"])
	}
	if _, ok := e1["blank"]; ok {
		t.Error("EnvVars included an empty-valued variable")
	}
	if got := h.EnvVars("missing"); got != nil {
		t.Errorf("EnvVars(unknown) = %v, want nil", got)
	}
}

func TestFlowHost_ExternalDragGating(t *testing.T) {
	cases := []struct {
		name      string
		section   string
		active    bool
		node      *collections.CollectionNode
		wantDrag  bool
		wantLabel string
	}{
		{"armed", "flows", true, &collections.CollectionNode{Name: "req-a"}, true, "req-a"},
		{"wrong section", "requests", true, &collections.CollectionNode{Name: "req-a"}, false, ""},
		{"not dragging", "flows", false, &collections.CollectionNode{Name: "req-a"}, false, ""},
		{"no node", "flows", true, nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 800))
			ui := rig.ui
			ui.SidebarSection = tc.section
			ui.DragNodeActive = tc.active
			ui.DraggedNode = tc.node
			ui.DragNodeWinPos = f32.Pt(11, 22)

			h := ui.flowHost()
			if h.ExternalDrag != tc.wantDrag {
				t.Errorf("ExternalDrag = %v, want %v", h.ExternalDrag, tc.wantDrag)
			}
			if h.ExternalDragLabel != tc.wantLabel {
				t.Errorf("ExternalDragLabel = %q, want %q", h.ExternalDragLabel, tc.wantLabel)
			}
			if h.ExternalDragPos != ui.DragNodeWinPos {
				t.Errorf("ExternalDragPos = %v, want %v", h.ExternalDragPos, ui.DragNodeWinPos)
			}
		})
	}
}

func TestLayoutFlowSection_CreatesEditorLazily(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.SidebarSection = "flows"

	if ui.Flow != nil {
		t.Fatal("precondition: Flow must start nil")
	}
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })

	if ui.Flow == nil {
		t.Fatal("layoutFlowSection did not create the flow editor")
	}
	created := ui.Flow
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })
	if ui.Flow != created {
		t.Error("layoutFlowSection replaced the existing editor on a second frame")
	}
}

func TestDropNodeOnFlowCanvas_Guards(t *testing.T) {
	req := &collections.CollectionNode{
		Name:    "get-user",
		Request: &model.ParsedRequest{Name: "get-user"},
	}
	cases := []struct {
		name        string
		section     string
		makeEditor  bool
		editingEnv  bool
		settingsOpn bool
	}{
		{"wrong section", "requests", true, false, false},
		{"no editor", "flows", false, false, false},
		{"editing env", "flows", true, true, false},
		{"settings open", "flows", true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 800))
			ui := rig.ui
			ui.SidebarSection = "flows"
			if tc.makeEditor {
				rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })
			}
			ui.SidebarSection = tc.section
			if tc.editingEnv {
				ui.EditingEnv = &environments.EnvironmentUI{}
			}
			ui.SettingsOpen = tc.settingsOpn
			ui.DragNodeWinPos = f32.Pt(float32(rig.sz.X)/2, float32(rig.sz.Y)/2)

			if ui.dropNodeOnFlowCanvas(req) {
				t.Error("dropNodeOnFlowCanvas accepted a drop that should have been rejected")
			}
		})
	}
}

func TestDropNodeOnFlowCanvas_NilNode(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.SidebarSection = "flows"
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })

	if ui.dropNodeOnFlowCanvas(nil) {
		t.Error("dropNodeOnFlowCanvas accepted a nil node")
	}
}

func TestDropNodeOnFlowCanvas_AddsRequestNode(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.SidebarSection = "flows"
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })

	before := len(ui.Flow.Scenario.Nodes)
	ui.DragNodeWinPos = f32.Pt(float32(rig.sz.X)/2, float32(rig.sz.Y)/2)
	req := &collections.CollectionNode{
		Name:    "get-user",
		Request: &model.ParsedRequest{Name: "get-user", URL: "https://example.com/u"},
	}

	if !ui.dropNodeOnFlowCanvas(req) {
		t.Fatal("dropping a request onto the laid-out canvas was rejected")
	}
	if got := len(ui.Flow.Scenario.Nodes); got != before+1 {
		t.Errorf("node count = %d, want %d", got, before+1)
	}
}

func TestDropNodeOnFlowCanvas_OutsideCanvasRejected(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.SidebarSection = "flows"
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })
	rig.frame(func(gtx layout.Context) { ui.layoutFlowSection(gtx) })

	before := len(ui.Flow.Scenario.Nodes)
	ui.DragNodeWinPos = f32.Pt(-500, -500)
	req := &collections.CollectionNode{
		Name:    "get-user",
		Request: &model.ParsedRequest{Name: "get-user"},
	}

	if ui.dropNodeOnFlowCanvas(req) {
		t.Error("a drop far outside the canvas was accepted")
	}
	if got := len(ui.Flow.Scenario.Nodes); got != before {
		t.Errorf("node count changed to %d after a rejected drop, want %d", got, before)
	}
}

func TestDropHostReflectsUIState(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.SidebarSection = "har"
	ui.Settings.HideSidebar = true

	h := ui.dropHost()
	if h.Blocked {
		t.Error("dropHost reported blocked with no overlay open")
	}
	if h.SidebarSection != "har" {
		t.Errorf("SidebarSection = %q, want har", h.SidebarSection)
	}
	if !h.SidebarHidden {
		t.Error("SidebarHidden = false, want true")
	}

	ui.SettingsOpen = true
	if !ui.dropHost().Blocked {
		t.Error("dropHost not blocked while settings are open")
	}
	ui.SettingsOpen = false
	ui.EditingEnv = &environments.EnvironmentUI{}
	if !ui.dropHost().Blocked {
		t.Error("dropHost not blocked while editing an environment")
	}
}

func TestOnOSFilesDragged_TracksActiveState(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.initDropzones()

	rig.frame(func(gtx layout.Context) { ui.rebuildDropZones(gtx) })

	ui.onOSFilesDragged(f32.Pt(600, 400), true)
	rig.frame(func(gtx layout.Context) { ui.layoutDropOverlay(gtx) })

	ui.onOSFilesDragged(f32.Pt(0, 0), false)
	rig.frame(func(gtx layout.Context) { ui.layoutDropOverlay(gtx) })
}

func TestOnOSFilesDragged_BlockedWhileSettingsOpen(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	ui := rig.ui
	ui.initDropzones()
	ui.SettingsOpen = true

	ui.onOSFilesDragged(f32.Pt(600, 400), true)
	rig.frame(func(gtx layout.Context) { ui.layoutDropOverlay(gtx) })
}

func TestHarHandleSearchShortcut_NoDocIsNoop(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	ui := rig.ui
	ui.HARView.Ensure()

	rig.frame(func(gtx layout.Context) { ui.harHandleSearchShortcut(gtx) })

	if ui.HARView.Doc != nil {
		t.Error("harHandleSearchShortcut fabricated a document")
	}
}

func TestHarHandleSearchShortcut_TogglesRequestSearch(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	ui := rig.ui
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	if ui.HARView.Doc == nil {
		t.Fatalf("precondition: doc must parse; banner=%q", ui.HARView.Banner)
	}
	ui.HARView.SelReq = 0

	rig.frame(func(gtx layout.Context) { ui.layoutHARSection(gtx) })
	before := ui.HARView.BodySearch.Open

	rig.frame(func(gtx layout.Context) { ui.harHandleSearchShortcut(gtx) })

	if ui.HARView.BodySearch.Open == before {
		t.Errorf("BodySearch.Open stayed %v, want toggled", before)
	}
}

const harTestDoc = `{
  "log": {"version":"1.2","entries":[
    {"request":{"method":"GET","url":"https://example.com/a","headers":[]},
     "response":{"status":200,"headers":[],"content":{"mimeType":"text/plain","text":"hello"}}}
  ]}
}`

func TestCloseNetlimit_BeforeInitIsSafe(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	if ui.Net.Started() {
		t.Fatal("precondition: netlimit must not be started")
	}
	ui.closeNetlimit()
}

func TestCloseNetlimit_AfterInit(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui
	ui.initNetlimit()

	if !ui.Net.Started() {
		t.Fatal("initNetlimit did not start the manager")
	}
	ui.closeNetlimit()
	ui.closeNetlimit()
}

func TestSettingsHost_Wiring(t *testing.T) {
	rig := newUIRig(t, image.Pt(1000, 700))
	ui := rig.ui
	ui.SettingsOpen = true
	ui.SettingsState = settings.NewEditor(ui.Settings)

	h := ui.settingsHost()

	if h.Theme != ui.Theme {
		t.Error("settingsHost did not pass the app theme")
	}
	if h.Window != ui.Window {
		t.Error("settingsHost did not pass the app window")
	}
	if h.Current != &ui.Settings {
		t.Error("settingsHost Current does not alias the live settings")
	}
	if h.Open != &ui.SettingsOpen {
		t.Error("settingsHost Open does not alias the live open flag")
	}
	if h.OnSave == nil {
		t.Error("settingsHost OnSave is nil")
	}

	h.OnClose()
	if ui.SettingsOpen {
		t.Error("OnClose did not clear SettingsOpen")
	}
	if ui.SettingsState != nil {
		t.Error("OnClose did not discard the editor state")
	}
}

func TestSettingsHost_OnSaveMarksState(t *testing.T) {
	rig := newUIRig(t, image.Pt(1000, 700))
	ui := rig.ui

	ui.settingsHost().OnSave()

	if !ui.saveNeeded {
		t.Error("settingsHost OnSave did not mark state save needed")
	}
}

func TestBuildWSTLSConfig(t *testing.T) {
	cases := []struct {
		name         string
		insecure     bool
		tractoCA     bool
		wantInsecure bool
	}{
		{"default", false, false, false},
		{"insecure", true, false, true},
		{"tracto ca", false, true, false},
		{"insecure wins over ca", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(800, 600))
			rt := workspace.NewRequestTab("ws")
			s := rt.EnsureWS()
			s.InsecureSkipVerify = tc.insecure
			s.UseTractoCA = tc.tractoCA

			cfg := rig.ui.buildWSTLSConfig(rt)

			if cfg == nil {
				t.Fatal("buildWSTLSConfig returned nil")
			}
			if cfg.MinVersion != tls.VersionTLS12 {
				t.Errorf("MinVersion = %d, want TLS 1.2", cfg.MinVersion)
			}
			if cfg.InsecureSkipVerify != tc.wantInsecure {
				t.Errorf("InsecureSkipVerify = %v, want %v", cfg.InsecureSkipVerify, tc.wantInsecure)
			}
			if tc.wantInsecure && cfg.RootCAs != nil {
				t.Error("insecure config should short-circuit before installing a root pool")
			}
		})
	}
}

func TestTriggerWSAction_ConnectsWhenIdle(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui
	rt := workspace.NewRequestTab("ws")
	rt.URLInput.SetText("ws://127.0.0.1:1/never")
	s := rt.EnsureWS()

	if s.State() != workspace.WSStateIdle {
		t.Fatalf("precondition: state = %v, want Idle", s.State())
	}
	ui.triggerWSAction(rt)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st := s.State(); st != workspace.WSStateIdle {
			rt.WSDisconnect()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("triggerWSAction never moved the session out of Idle")
}

func TestTriggerWSAction_EmptyURLRecordsError(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui
	rt := workspace.NewRequestTab("ws")
	s := rt.EnsureWS()

	ui.triggerWSAction(rt)

	if s.State() != workspace.WSStateIdle {
		t.Errorf("state = %v, want Idle after an empty-URL connect", s.State())
	}
}

func TestWireWSHost_InstallsCallbacks(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	rt := workspace.NewRequestTab("ws")

	rig.ui.wireWSHost(rt)

	if rt.WSHost.OnConnect == nil {
		t.Error("wireWSHost left OnConnect nil")
	}
	if rt.WSHost.OnDisconnect == nil {
		t.Error("wireWSHost left OnDisconnect nil")
	}
	rt.WSHost.OnDisconnect(rt)
}

func TestHideSidebarHook(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	ui := rig.ui

	ui.Settings.HideSidebar = false
	if ui.HideSidebar() {
		t.Error("HideSidebar() = true with the setting off")
	}
	ui.Settings.HideSidebar = true
	if !ui.HideSidebar() {
		t.Error("HideSidebar() = false with the setting on")
	}
	if ui.HideSidebar() != ui.hideSidebar() {
		t.Error("HideSidebar hook disagrees with hideSidebar")
	}
}
