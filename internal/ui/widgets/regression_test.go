package widgets

import (
	"image"
	"testing"

	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
)

func shortcutRig(t *testing.T, ed *widget.Editor) *rig {
	t.Helper()
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frame()
	rg.focus(ed)
	return rg
}

func TestHandleEditorShortcuts_ExtendMovesCaretNotAnchor(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		caret       int
		anchor      int
		key         key.Name
		wantCaret   int
		wantAnchor  int
		wantSelText string
	}{
		{
			name: "shrink ltr selection from the left", text: "alpha beta gamma",
			caret: 10, anchor: 0, key: key.NameLeftArrow,
			wantCaret: 6, wantAnchor: 0, wantSelText: "alpha ",
		},
		{
			name: "grow ltr selection to the right", text: "alpha beta gamma",
			caret: 5, anchor: 0, key: key.NameRightArrow,
			wantCaret: 10, wantAnchor: 0, wantSelText: "alpha beta",
		},
		{
			name: "grow rtl selection to the left", text: "alpha beta gamma",
			caret: 11, anchor: 16, key: key.NameLeftArrow,
			wantCaret: 6, wantAnchor: 16, wantSelText: "beta gamma",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText(tc.text)
			rg := shortcutRig(t, ed)
			ed.SetCaret(tc.caret, tc.anchor)

			rg.keyPress(tc.key, key.ModShortcut|key.ModShift)

			caret, anchor := ed.Selection()
			if caret != tc.wantCaret {
				t.Errorf("caret = %d, want %d (the caret must move, not the anchor)", caret, tc.wantCaret)
			}
			if anchor != tc.wantAnchor {
				t.Errorf("anchor = %d, want %d (the anchor must stay put)", anchor, tc.wantAnchor)
			}
			if got := ed.SelectedText(); got != tc.wantSelText {
				t.Errorf("selection = %q, want %q", got, tc.wantSelText)
			}
		})
	}
}

func TestHandleEditorShortcuts_PlainMoveUsesCaretOrigin(t *testing.T) {
	cases := []struct {
		name   string
		caret  int
		anchor int
		key    key.Name
		want   int
	}{
		{"left from caret of a ltr selection", 10, 0, key.NameLeftArrow, 6},
		{"right from caret of a ltr selection", 6, 0, key.NameRightArrow, 10},
		{"left from caret of a rtl selection", 6, 16, key.NameLeftArrow, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText("alpha beta gamma")
			rg := shortcutRig(t, ed)
			ed.SetCaret(tc.caret, tc.anchor)

			rg.keyPress(tc.key, key.ModShortcut)

			caret, anchor := ed.Selection()
			if caret != tc.want || anchor != tc.want {
				t.Errorf("caret = (%d,%d), want (%d,%d) collapsed at the caret origin",
					caret, anchor, tc.want, tc.want)
			}
		})
	}
}
