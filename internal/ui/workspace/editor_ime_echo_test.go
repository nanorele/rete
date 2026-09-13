package workspace

import (
	"image"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
)

func (rig *editorKeyRig) typeLikeWindow(s string) {
	start, end := rig.v.normSel()
	startRune := utf8.RuneCount(rig.v.text[:start])
	endRune := utf8.RuneCount(rig.v.text[:end])
	echo := startRune + utf8.RuneCountInString(s)
	rig.r.Queue(
		key.EditEvent{Range: key.Range{Start: startRune, End: endRune}, Text: s},
		key.SnippetEvent{Start: 0, End: echo},
		key.SelectionEvent{Start: echo, End: echo},
	)
	rig.frame()
}

func TestRequestEditorQuoteWrapIgnoresWindowSelectionEcho(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("test")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 4
	rig.frame()

	rig.typeLikeWindow("\"")
	if got := rig.v.Text(); got != "\"test\"" {
		t.Fatalf("first quote: got %q", got)
	}
	if rig.v.selStart != 1 || rig.v.selEnd != 5 {
		t.Fatalf("selection must stay on the word despite the window's caret echo, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.typeLikeWindow("\"")
	if got := rig.v.Text(); got != "\"\"test\"\"" {
		t.Fatalf("second quote should wrap again, got %q", got)
	}
	if rig.v.selStart != 2 || rig.v.selEnd != 6 {
		t.Fatalf("selection after second wrap: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.typeLikeWindow("(")
	if got := rig.v.Text(); got != "\"\"(test)\"\"" {
		t.Fatalf("paren after quotes: got %q", got)
	}
}

func TestRequestEditorPlainTypingStillFollowsWindowSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("ab")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 2, 2
	rig.frame()

	rig.typeLikeWindow("c")
	if got := rig.v.Text(); got != "abc" {
		t.Fatalf("got %q", got)
	}
	if rig.v.selStart != 3 || rig.v.selEnd != 3 {
		t.Fatalf("caret after plain insert: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}

	rig.r.Queue(key.SelectionEvent{Start: 1, End: 1})
	rig.frame()
	if rig.v.selStart != 1 || rig.v.selEnd != 1 {
		t.Fatalf("a later, unrelated selection event must still apply, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorFocusMoveCollapsesSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("hello world")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 5
	rig.frame()

	other := new(int)
	rig.r.Source().Execute(key.FocusCmd{Tag: other})
	rig.frame()
	rig.frame()

	if rig.v.selStart != rig.v.selEnd {
		t.Fatalf("selection must collapse when another widget takes focus, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorWindowDeactivationKeepsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("hello world")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 5
	rig.frame()

	rig.r.Queue(key.FocusEvent{Focus: false})
	rig.frame()
	rig.r.Queue(key.FocusEvent{Focus: true})
	rig.frame()

	if rig.v.selStart != 0 || rig.v.selEnd != 5 {
		t.Fatalf("losing window focus must not drop the selection, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

type blurRespRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
}

func newBlurRespRig(txt string) *blurRespRig {
	rig := &blurRespRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
	}
	rig.v.SetText(txt)
	return rig
}

func (rig *blurRespRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 300)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	ResponseViewerStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     true,
		Padding:  unit.Dp(4),
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func TestResponseViewerFocusMoveCollapsesSelection(t *testing.T) {
	rig := newBlurRespRig("hello world")
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(10, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
	rig.r.Queue(key.Event{Name: "A", Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("setup: Ctrl+A should select the text")
	}

	rig.r.Queue(key.FocusEvent{Focus: false})
	rig.frame()
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("window deactivation must keep the response selection")
	}

	other := new(int)
	rig.r.Source().Execute(key.FocusCmd{Tag: other})
	rig.frame()
	rig.frame()
	if rig.v.selStart != rig.v.selEnd {
		t.Fatalf("selection must collapse when focus moves elsewhere, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorClosingBracketWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.v.SetText("test")
	rig.focus()
	rig.v.selStart, rig.v.selEnd = 0, 4
	rig.frame()

	rig.typeLikeWindow("\"")
	rig.typeLikeWindow("}")
	if got := rig.v.Text(); got != "\"{test}\"" {
		t.Fatalf("a closing bracket over a selection must wrap it, got %q", got)
	}
	if rig.v.selStart != 2 || rig.v.selEnd != 6 {
		t.Fatalf("selection after wrap: got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
	rig.typeLikeWindow(")")
	rig.typeLikeWindow("]")
	rig.typeLikeWindow(">")
	if got := rig.v.Text(); got != "\"{([<test>])}\"" {
		t.Fatalf("closing chars must wrap like their openers, got %q", got)
	}
}
