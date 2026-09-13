package workspace

import (
	"strings"
	"testing"
	"time"
)

func TestDoubleClickPastLineEndDoesNotGrabNextLine(t *testing.T) {
	resp := "{\n  \"avatar\": \"\",\n  \"next\": 1\n}\n"
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}

	line2Start := strings.Index(resp, "  \"next\"")
	nlOff := line2Start - 1

	x := 300
	y := 4 + rig.v.lastLineHeight + rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)

	s, e := rig.v.selStart, rig.v.selEnd
	if s > e {
		s, e = e, s
	}
	if e > nlOff {
		t.Errorf("double-click past line end selected across the newline: [%d,%d) = %q",
			s, e, resp[s:e])
	}
}

func TestDoubleClickOnIndentSelectsIndentOnly(t *testing.T) {
	resp := "{\n    \"avatar\": \"\",\n    \"next\": 1\n}\n"
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}

	x := 7
	y := 4 + rig.v.lastLineHeight + rig.v.lastLineHeight/2
	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+80*time.Millisecond)

	s, e := rig.v.selStart, rig.v.selEnd
	if s > e {
		s, e = e, s
	}
	if s != e {
		if got := resp[s:e]; strings.ContainsAny(got, "\n\r") {
			t.Errorf("double-click on indent spaces crossed a line break: [%d,%d) = %q", s, e, got)
		}
	}
}
