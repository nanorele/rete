package workspace

import "testing"

func TestZeroValueEditor_TypingIsRendered(t *testing.T) {
	v := &RequestEditor{}
	v.Insert(0, "a")
	if len(v.lineStarts) == 0 {
		t.Fatalf("lineStarts empty after typing into a fresh editor: text=%q would never render", v.text)
	}
	s, e := v.lineBounds(0)
	if got := string(v.text[s:e]); got != "a" {
		t.Fatalf("line 0 does not cover typed text: got %q, want %q", got, "a")
	}
}

func TestNewRequestTab_BodyEditorReady(t *testing.T) {
	tab := NewRequestTab("T")
	tab.ReqEditor.Insert(0, "x")
	if len(tab.ReqEditor.lineStarts) == 0 {
		t.Fatalf("new request body editor not initialized: typed text would be invisible")
	}
}
