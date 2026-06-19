package workspace

import (
	"testing"

	"github.com/nanorele/gio/io/key"
)

func TestRequestEditorCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()
	if got := rig.v.Text(); got != "hello world" {
		t.Fatalf("setup: got %q", got)
	}

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.v.Text(); got != "hello " {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}
}

func TestRequestEditorCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello world"})
	rig.frame()

	rig.v.selStart = 0
	rig.v.selEnd = 0
	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.v.Text(); got != "world" {
		t.Fatalf("Ctrl+Delete should delete the leading word and separator, got %q", got)
	}
}

func TestRequestEditorPlainBackspaceStillDeletesChar(t *testing.T) {
	rig := newEditorKeyRig()
	rig.focus()

	rig.r.Queue(key.EditEvent{Text: "hello"})
	rig.frame()

	rig.pressKey(key.NameDeleteBackward, 0)
	if got := rig.v.Text(); got != "hell" {
		t.Fatalf("plain Backspace should delete a single char, got %q", got)
	}
}
