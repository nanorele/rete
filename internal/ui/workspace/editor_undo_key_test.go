package workspace

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type editorKeyRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *RequestEditor
}

func newEditorKeyRig() *editorKeyRig {
	return &editorKeyRig{
		ops:    new(op.Ops),
		shaper: material.NewTheme().Shaper,
		v:      NewRequestEditor(),
	}
}

func (rig *editorKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 300)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	RequestEditorStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(14),
		Wrap:     true,
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *editorKeyRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *editorKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestRequestEditorCtrlZKeyUndo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("setup: got %q", got)
	}

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)

	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}
}

func TestRequestEditorCtrlYKeyRedo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}

	rig.pressKey("Y", key.ModShortcut)
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("Ctrl+Y should redo to %q, got %q", "base text", got)
	}
}

func TestRequestEditorCtrlShiftZKeyRedo(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.v.Replace(4, 4, " text")

	rig.focus()
	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "base" {
		t.Fatalf("Ctrl+Z should undo to %q, got %q", "base", got)
	}

	rig.pressKey("Z", key.ModShortcut|key.ModShift)
	if got := rig.v.Text(); got != "base text" {
		t.Fatalf("Ctrl+Shift+Z should redo to %q, got %q", "base text", got)
	}
}

func TestRequestEditorTypeThenCtrlZ(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("base")
	rig.focus()
	before := rig.v.Text()

	rig.r.Queue(key.EditEvent{Text: " typed"})
	rig.frame()
	if rig.v.Text() == before {
		t.Fatalf("typing via EditEvent had no effect; still %q", before)
	}

	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != before {
		t.Fatalf("Ctrl+Z after typing should undo to %q, got %q", before, got)
	}
}
