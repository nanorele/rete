package apptest

import (
	. "rete/internal/ui"

	"image"
	"testing"
	"time"

	"rete/internal/model"
	"rete/internal/ui/environments"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"golang.org/x/image/math/fixed"
)

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
