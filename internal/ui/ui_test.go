package ui

import (
	"context"
	"crypto/tls"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"os"
	"path/filepath"
	"rete/internal/har"
	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/colorpicker"
	"rete/internal/ui/environments"
	"rete/internal/ui/settings"
	"rete/internal/ui/tabbar"
	"rete/internal/ui/workspace"
	"testing"
	"time"
)

func TestHarSkipHeader(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{":authority", true},
		{":method", true},
		{":path", true},
		{"content-length", true},
		{"Content-Length", true},
		{"CONTENT-LENGTH", true},
		{"host", true},
		{"Host", true},
		{"content-type", false},
		{"Authorization", false},
		{"", false},
		{"x-content-length", false},
		{"hostname", false},
	}
	for _, c := range cases {
		if got := harSkipHeader(c.name); got != c.want {
			t.Errorf("harSkipHeader(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHarWSURL(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://example.com/socket", "wss://example.com/socket"},
		{"http://example.com/socket", "ws://example.com/socket"},
		{"wss://example.com/socket", "wss://example.com/socket"},
		{"ws://example.com/socket", "ws://example.com/socket"},
		{"", ""},
		{"example.com", "example.com"},
		{"HTTPS://example.com", "HTTPS://example.com"},
		{"https://", "wss://"},
	}
	for _, c := range cases {
		if got := harWSURL(c.raw); got != c.want {
			t.Errorf("harWSURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestHarRunTitle(t *testing.T) {
	cases := []struct {
		method, reqURL, want string
	}{
		{"GET", "https://example.com/a/b", "GET example.com"},
		{"POST", "https://example.com:8443/x?q=1", "POST example.com:8443"},
		{"GET", "wss://example.com/socket", "GET example.com"},
		{"GET", "not a url", "GET not a url"},
		{"", "https://example.com/a", "example.com"},
		{"GET", "", "GET"},
	}
	for _, c := range cases {
		e := &har.Entry{Request: har.Request{Method: c.method, URL: c.reqURL}}
		if got := harRunTitle(e, c.reqURL); got != c.want {
			t.Errorf("harRunTitle(%q, %q) = %q, want %q", c.method, c.reqURL, got, c.want)
		}
	}
}

func TestLoadEmbeddedTTF(t *testing.T) {
	b, err := loadEmbeddedTTF("Inter-Regular.ttf")
	if err != nil {
		t.Fatalf("loadEmbeddedTTF(Inter-Regular.ttf) error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("loadEmbeddedTTF returned empty font")
	}
	if _, err := loadEmbeddedTTF("DoesNotExist.ttf"); err == nil {
		t.Error("loadEmbeddedTTF(DoesNotExist.ttf) = nil error, want error")
	}
}

func TestEmbeddedFontsDecompress(t *testing.T) {
	for name := range embeddedFonts {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Errorf("loadEmbeddedTTF(%q) error: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("loadEmbeddedTTF(%q) decompressed to 0 bytes", name)
		}
	}
}

func TestFallbackFontFilesAreEmbedded(t *testing.T) {
	specs := append([]lazyFontSpec{emojiFontSpec}, fallbackFontSpecs...)
	for _, spec := range specs {
		if _, ok := embeddedFonts[spec.file]; !ok {
			t.Errorf("fallback font %q has no embedded entry", spec.file)
		}
	}
}

func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := filepath.Join(dir, "rete-test")
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
		reteCA       bool
		wantInsecure bool
	}{
		{"default", false, false, false},
		{"insecure", true, false, true},
		{"rete ca", false, true, false},
		{"insecure wins over ca", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(800, 600))
			rt := workspace.NewRequestTab("ws")
			s := rt.EnsureWS()
			s.InsecureSkipVerify = tc.insecure
			s.UseReteCA = tc.reteCA

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

func reqNode(name string, depth int) *collections.CollectionNode {
	return &collections.CollectionNode{
		Name:    name,
		Depth:   depth,
		Request: &model.ParsedRequest{Name: name},
	}
}

func colWithTree(id string, root *collections.CollectionNode) *collections.CollectionUI {
	pc := &collections.ParsedCollection{ID: id, Name: id, Root: root}
	var mark func(n *collections.CollectionNode)
	mark = func(n *collections.CollectionNode) {
		n.Collection = pc
		for _, c := range n.Children {
			c.Parent = n
			mark(c)
		}
	}
	mark(root)
	return &collections.CollectionUI{Data: pc}
}

func TestRelinkTabs_ResolvesPendingNode(t *testing.T) {
	leaf := reqNode("get-user", 1)
	root := node("root", true, true, 0, leaf)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.PendingColID = "c1"
	tab.PendingNodePath = []int{0}

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.relinkTabs()

	if tab.LinkedNode != leaf {
		t.Fatalf("LinkedNode = %v, want the leaf node", tab.LinkedNode)
	}
	if tab.PendingColID != "" {
		t.Errorf("PendingColID = %q, want cleared", tab.PendingColID)
	}
	if tab.PendingNodePath != nil {
		t.Errorf("PendingNodePath = %v, want nil", tab.PendingNodePath)
	}
}

func TestRelinkTabs_LeavesUnresolvable(t *testing.T) {
	cases := []struct {
		name    string
		colID   string
		path    []int
		wantCol string
	}{
		{"unknown collection", "nope", []int{0}, "nope"},
		{"path out of range", "c1", []int{5}, "c1"},
		{"path into missing child", "c1", []int{0, 3}, "c1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := node("root", true, true, 0, reqNode("get-user", 1))
			col := colWithTree("c1", root)

			tab := workspace.NewRequestTab("t")
			tab.PendingColID = c.colID
			tab.PendingNodePath = c.path

			ui := &AppUI{}
			ui.Collections = []*collections.CollectionUI{col}
			ui.Tabs = []*workspace.RequestTab{tab}
			ui.relinkTabs()

			if tab.LinkedNode != nil {
				t.Error("LinkedNode was set from an unresolvable path")
			}
			if tab.PendingColID != c.wantCol {
				t.Errorf("PendingColID = %q, want %q retained for a later retry", tab.PendingColID, c.wantCol)
			}
		})
	}
}

func TestRelinkTabs_SkipsFolderNodes(t *testing.T) {
	folder := node("fld", true, false, 1)
	root := node("root", true, true, 0, folder)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.PendingColID = "c1"
	tab.PendingNodePath = []int{0}

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.relinkTabs()

	if tab.LinkedNode != nil {
		t.Error("a folder node (no Request) was linked to a request tab")
	}
}

func TestRelinkTabs_IgnoresAlreadyLinkedAndUnpending(t *testing.T) {
	leaf := reqNode("a", 1)
	other := reqNode("b", 1)
	root := node("root", true, true, 0, leaf, other)
	col := colWithTree("c1", root)

	linked := workspace.NewRequestTab("linked")
	linked.LinkedNode = other
	linked.PendingColID = "c1"
	linked.PendingNodePath = []int{0}

	plain := workspace.NewRequestTab("plain")

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{linked, plain}
	ui.relinkTabs()

	if linked.LinkedNode != other {
		t.Error("relinkTabs overwrote an already-linked tab")
	}
	if plain.LinkedNode != nil {
		t.Error("relinkTabs linked a tab with no pending reference")
	}
}

func TestRevealLinkedNode_ExpandsAncestors(t *testing.T) {
	leaf := reqNode("deep", 2)
	folder := node("fld", true, false, 1, leaf)
	root := node("root", true, false, 0, folder)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.LinkedNode = leaf

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.revealLinkedNode(tab)

	if !folder.Expanded {
		t.Error("parent folder was not expanded")
	}
	if !root.Expanded {
		t.Error("root was not expanded")
	}
	if len(ui.VisibleCols) == 0 {
		t.Error("visible collections were not rebuilt after expanding")
	}
}

func TestRevealLinkedNode_NoopCases(t *testing.T) {
	ui := &AppUI{}
	ui.revealLinkedNode(nil)

	tab := workspace.NewRequestTab("t")
	ui.revealLinkedNode(tab)

	orphan := reqNode("orphan", 1)
	tab.LinkedNode = orphan
	ui.revealLinkedNode(tab)

	if len(ui.VisibleCols) != 0 {
		t.Error("revealLinkedNode rebuilt visible list for an unreachable node")
	}
}

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

func newTabsUI(titles ...string) *AppUI {
	ui := &AppUI{TabBar: tabbar.NewStrip()}
	for _, t := range titles {
		ui.Tabs = append(ui.Tabs, workspace.NewRequestTab(t))
	}
	return ui
}

func tabTitles(ui *AppUI) []string {
	out := make([]string, 0, len(ui.Tabs))
	for _, t := range ui.Tabs {
		out = append(out, t.Title)
	}
	return out
}

func TestCloseTab_OutOfRange(t *testing.T) {
	for _, idx := range []int{-1, 3, 99} {
		ui := newTabsUI("a", "b", "c")
		ui.ActiveIdx = 1
		ui.closeTab(idx)
		if len(ui.Tabs) != 3 {
			t.Errorf("closeTab(%d) removed a tab, want no-op", idx)
		}
		if ui.ActiveIdx != 1 {
			t.Errorf("closeTab(%d) ActiveIdx = %d, want 1", idx, ui.ActiveIdx)
		}
	}
}

func TestCloseTab_ActiveIdx(t *testing.T) {
	cases := []struct {
		name     string
		titles   []string
		active   int
		close    int
		wantTabs []string
		wantIdx  int
	}{
		{"last tab remaining", []string{"a"}, 0, 0, []string{}, -1},
		{"close before active", []string{"a", "b", "c"}, 2, 0, []string{"b", "c"}, 1},
		{"close active middle", []string{"a", "b", "c"}, 1, 1, []string{"a", "c"}, 0},
		{"close active first", []string{"a", "b", "c"}, 0, 0, []string{"b", "c"}, 0},
		{"close active last", []string{"a", "b", "c"}, 2, 2, []string{"a", "b"}, 1},
		{"close after active", []string{"a", "b", "c"}, 0, 2, []string{"a", "b"}, 0},
		{"close after active mid", []string{"a", "b", "c"}, 1, 2, []string{"a", "b"}, 1},
		{"two tabs close first active second", []string{"a", "b"}, 1, 0, []string{"b"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ui := newTabsUI(c.titles...)
			ui.ActiveIdx = c.active
			ui.closeTab(c.close)

			got := tabTitles(ui)
			if len(got) != len(c.wantTabs) {
				t.Fatalf("tabs = %v, want %v", got, c.wantTabs)
			}
			for i := range got {
				if got[i] != c.wantTabs[i] {
					t.Fatalf("tabs = %v, want %v", got, c.wantTabs)
				}
			}
			if ui.ActiveIdx != c.wantIdx {
				t.Errorf("ActiveIdx = %d, want %d", ui.ActiveIdx, c.wantIdx)
			}
		})
	}
}

func TestCloseTab_ActiveIdxAlwaysValid(t *testing.T) {
	for active := 0; active < 4; active++ {
		for closeIdx := 0; closeIdx < 4; closeIdx++ {
			ui := newTabsUI("a", "b", "c", "d")
			ui.ActiveIdx = active
			ui.closeTab(closeIdx)
			if len(ui.Tabs) == 0 {
				continue
			}
			if ui.ActiveIdx < 0 || ui.ActiveIdx >= len(ui.Tabs) {
				t.Errorf("active=%d close=%d: ActiveIdx = %d out of range (len=%d)",
					active, closeIdx, ui.ActiveIdx, len(ui.Tabs))
			}
		}
	}
}

func TestCloseTab_MarksSaveNeeded(t *testing.T) {
	ui := newTabsUI("a", "b")
	ui.ActiveIdx = 0
	ui.closeTab(1)
	if !ui.saveNeeded {
		t.Error("closeTab did not mark state save needed")
	}
}

func TestWindowPosSnapshotRoundTrip(t *testing.T) {
	ui := &AppUI{}
	if got := ui.buildStateSnapshot(); got.WindowXPx != nil || got.WindowYPx != nil {
		t.Fatalf("no position seen yet: must not persist one (%v,%v)", got.WindowXPx, got.WindowYPx)
	}

	ui.winXPx, ui.winYPx, ui.winPosSet = -1720, 0, true
	state := ui.buildStateSnapshot()
	if state.WindowXPx == nil || state.WindowYPx == nil {
		t.Fatalf("position not persisted: %+v", state)
	}
	if *state.WindowXPx != -1720 || *state.WindowYPx != 0 {
		t.Errorf("got %d,%d want -1720,0", *state.WindowXPx, *state.WindowYPx)
	}

	loaded := &AppUI{}
	loaded.applyWindowState(state)
	if !loaded.winPosSet || loaded.winXPx != -1720 || loaded.winYPx != 0 {
		t.Errorf("position not restored: set=%v %d,%d", loaded.winPosSet, loaded.winXPx, loaded.winYPx)
	}

	missing := &AppUI{}
	missing.applyWindowState(persist.AppState{WindowXPx: intPtrTest(10)})
	if missing.winPosSet {
		t.Errorf("a half-written position must be ignored")
	}
}

func intPtrTest(i int) *int { return &i }

func TestRestoreWindowPosWithoutHandle(t *testing.T) {
	ui := &AppUI{winPosSet: true, winXPx: 100, winYPx: 100}
	if ui.restoreWindowPos() {
		t.Errorf("restore must fail without a native window handle so the caller falls back to centering")
	}
}
