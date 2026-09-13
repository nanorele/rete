package workspace

import (
	"strings"
	"testing"
	"time"
)

func TestViewerDoubleClickSelectsURLPathSegment(t *testing.T) {
	const url = "{{apiUrl}}/waInstance{{idInstance}}/getStatusInstance/{{apiTokenInstance}}"
	v := NewResponseViewer()
	v.SetText(url)

	cases := []struct {
		word string
		off  int
	}{
		{"getStatusInstance", 3},
		{"waInstance", 2},
		{"apiUrl", 1},
		{"idInstance", 0},
		{"apiTokenInstance", 8},
	}
	for _, c := range cases {
		at := strings.Index(url, c.word) + c.off
		s, e := v.wordBoundsAt(at)
		if got := url[s:e]; got != c.word {
			t.Errorf("wordBoundsAt(%d) = %q, want %q", at, got, c.word)
		}
	}
	slash := strings.Index(url, "/getStatusInstance")
	if s, e := v.wordBoundsAt(slash); url[s:e] != "/" {
		t.Errorf("double-click on the slash must select just the slash, got %q", url[s:e])
	}

	rig := newRespRig("x/getStatusInstance/y", true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "getStatusInstance" {
		t.Errorf("double-click in the viewer selected %q, want getStatusInstance", got)
	}
}
