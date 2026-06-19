package widgets

import (
	"image"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type fieldKeyRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	ed  *widget.Editor
}

func newFieldKeyRig(text string) *fieldKeyRig {
	rig := &fieldKeyRig{
		ops: new(op.Ops),
		th:  material.NewTheme(),
		ed:  &widget.Editor{SingleLine: true},
	}
	rig.ed.SetText(text)
	return rig
}

func (rig *fieldKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	HandleEditorShortcuts(gtx, rig.ed)
	for {
		if _, ok := rig.ed.Update(gtx); !ok {
			break
		}
	}
	material.Editor(rig.th, rig.ed, "").Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *fieldKeyRig) focusEnd() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
	n := utf8.RuneCountInString(rig.ed.Text())
	rig.ed.SetCaret(n, n)
}

func (rig *fieldKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestFieldCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.ed.Text(); got != "hello " {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}
}

func TestFieldCtrlBackspaceDeletesSelection(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()
	rig.ed.SetCaret(0, utf8.RuneCountInString("hello world"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.ed.Text(); got != "" {
		t.Fatalf("Ctrl+Backspace with a selection should delete it, got %q", got)
	}
}

func TestFieldCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()
	rig.ed.SetCaret(0, 0)

	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.ed.Text(); got != " world" {
		t.Fatalf("Ctrl+Delete should delete the leading word, got %q", got)
	}
}
