package apptest

import (
	"image"
	"testing"
	"time"

	. "rete/internal/ui"
	"rete/internal/ui/flow"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

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
