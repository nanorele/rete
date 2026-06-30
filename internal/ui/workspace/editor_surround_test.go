package workspace

import (
	"testing"

	"github.com/nanorele/gio/io/key"
)

func TestRequestEditorQuoteWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 6
	rig.v.selEnd = 11

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()

	if got := rig.v.Text(); got != "hello \"world\"" {
		t.Fatalf("quote should wrap selection, got %q", got)
	}
	if rig.v.selStart != 12 || rig.v.selEnd != 12 {
		t.Fatalf("caret should stay collapsed where it was (end of word), got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorQuoteWrapsBackwardSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 11
	rig.v.selEnd = 6

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()

	if got := rig.v.Text(); got != "hello \"world\"" {
		t.Fatalf("quote should wrap a right-to-left selection, got %q", got)
	}
	if rig.v.selStart != 12 || rig.v.selEnd != 12 {
		t.Fatalf("caret should land after the word regardless of selection direction, got [%d,%d]", rig.v.selStart, rig.v.selEnd)
	}
}

func TestRequestEditorBracketWrapsSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "abc"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 3

	rig.r.Queue(key.EditEvent{Text: "("})
	rig.frame()
	if got := rig.v.Text(); got != "(abc)" {
		t.Fatalf("paren should wrap selection, got %q", got)
	}
}

func TestRequestEditorWrapUndoIsSingleStep(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "abc"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 3

	rig.r.Queue(key.EditEvent{Text: "["})
	rig.frame()
	if got := rig.v.Text(); got != "[abc]" {
		t.Fatalf("setup: got %q", got)
	}

	rig.pressKey("Z", key.ModShortcut)
	if got := rig.v.Text(); got != "abc" {
		t.Fatalf("single undo should revert the wrap, got %q", got)
	}
}

func TestRequestEditorTypingWithoutSelectionUnchanged(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "ab"})
	rig.frame()
	rig.v.selStart = 2
	rig.v.selEnd = 2

	rig.r.Queue(key.EditEvent{Text: "\""})
	rig.frame()
	if got := rig.v.Text(); got != "ab\"" {
		t.Fatalf("typing a quote with no selection should insert a single quote, got %q", got)
	}
}

func TestRequestEditorNonPairReplacesSelection(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello"})
	rig.frame()
	rig.v.selStart = 0
	rig.v.selEnd = 5

	rig.r.Queue(key.EditEvent{Text: "x"})
	rig.frame()
	if got := rig.v.Text(); got != "x" {
		t.Fatalf("a non-pair char should replace the selection, got %q", got)
	}
}
