package environments

import (
	"image"
	"testing"
	"time"

	"tracto/internal/model"

	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

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
	if s, _ := ed.Selection(); s != 17 {
		t.Errorf("Ctrl+Left again: caret = %d, want 17 (start of \"green-api\")", s)
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
