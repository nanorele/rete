package workspace

import (
	"strings"
	"testing"

	"github.com/nanorele/gio/io/key"
)

// problems.md #3: with the caret right before the value of "phoneNumber",
// Ctrl+Right used to land on the next line between `"` and `force` instead of
// stopping at the end of the current line.
func TestCtrlRightKeepsCaretOnLine(t *testing.T) {
	body := "{\n    \"phoneNumber\": 123456789,\n\t\"force\": true\n}"
	rig := newEditorKeyRig()
	rig.v.SetText(body)

	valueStart := strings.Index(body, "123456789")
	lineEnd := strings.Index(body[valueStart:], "\n") + valueStart

	rig.focus()
	rig.v.selStart, rig.v.selEnd = valueStart, valueStart
	rig.pressKey(key.NameRightArrow, key.ModShortcut)

	caret := rig.v.selEnd
	if caret > lineEnd {
		t.Fatalf("Ctrl+Right jumped past the line break: caret=%d (%q), line ends at %d",
			caret, body[caret:min(caret+5, len(body))], lineEnd)
	}
	if caret <= valueStart {
		t.Fatalf("Ctrl+Right did not advance: caret=%d", caret)
	}

	// The next Ctrl+Right crosses the break and lands on the first word of the
	// following line.
	rig.pressKey(key.NameRightArrow, key.ModShortcut)
	caret = rig.v.selEnd
	if want := strings.Index(body, "force"); caret != want {
		t.Fatalf("second Ctrl+Right: caret=%d (%q), want %d (start of \"force\")",
			caret, body[caret:min(caret+5, len(body))], want)
	}
}
