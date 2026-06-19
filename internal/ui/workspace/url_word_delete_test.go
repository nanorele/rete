package workspace

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
	"github.com/nanorele/gio/widget/material"
)

type urlKeyRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	tab *RequestTab
}

func newURLKeyRig(text string) *urlKeyRig {
	rig := &urlKeyRig{
		ops: new(op.Ops),
		th:  material.NewTheme(),
		tab: &RequestTab{},
	}
	rig.tab.URLInput.SingleLine = true
	rig.tab.URLInput.SetText(text)
	n := utf8.RuneCountInString(text)
	rig.tab.URLInput.SetCaret(n, n)
	return rig
}

func (rig *urlKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	rig.tab.handleURLWordDelete(gtx)
	for {
		if _, ok := rig.tab.URLInput.Update(gtx); !ok {
			break
		}
	}
	material.Editor(rig.th, &rig.tab.URLInput, "").Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *urlKeyRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *urlKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestURLCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(utf8.RuneCountInString("https://example.com/path"), utf8.RuneCountInString("https://example.com/path"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "https://example.com/" {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "https://example." {
		t.Fatalf("Ctrl+Backspace should delete the slash separator and preceding word, got %q", got)
	}
}

func TestURLCtrlBackspaceDeletesSelection(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(0, utf8.RuneCountInString("https://example.com/path"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "" {
		t.Fatalf("Ctrl+Backspace with a selection should delete it, got %q", got)
	}
}

func TestURLCtrlBackspaceTreatsVarAsWord(t *testing.T) {
	rig := newURLKeyRig("http://{{host}}")
	rig.focus()
	rig.tab.URLInput.SetCaret(utf8.RuneCountInString("http://{{host}}"), utf8.RuneCountInString("http://{{host}}"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "http://" {
		t.Fatalf("Ctrl+Backspace should delete whole {{var}} as one word, got %q", got)
	}
}

func TestURLCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newURLKeyRig("https://example.com/path")
	rig.focus()
	rig.tab.URLInput.SetCaret(0, 0)

	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.tab.URLInput.Text(); got != "://example.com/path" {
		t.Fatalf("Ctrl+Delete should delete the leading word, got %q", got)
	}
}
