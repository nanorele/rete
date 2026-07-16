package workspace

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func resizeRigText() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		b.WriteString(fmt.Sprintf("line %02d: ", i))
		b.WriteString(strings.Repeat("abcdefghij ", 10))
		b.WriteByte('\n')
	}
	return b.String()
}

func (rig *respRig) topSourceLine() int {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	pad := 4
	innerW := rig.size.X - 2*pad
	line, _ := rig.v.firstChunkAtFn(rig.v.scrollY, rig.v.lastLineHeight, adv, innerW, rig.wrap)
	return line
}

func TestResizeKeepsResponseTextAnchored(t *testing.T) {
	cases := []struct {
		name    string
		startW  int
		endW    int
		scrollY int
	}{
		{"narrow", 400, 250, 15 * 18},
		{"widen", 400, 620, 15 * 18},
		{"narrow-unaligned", 400, 250, 15*18 + 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRespRig(resizeRigText(), true)
			rig.size.X = tc.startW
			now := time.Unix(1700000000, 0)
			for i := 0; i < 3; i++ {
				rig.frame(now)
			}

			rig.v.scrollY = tc.scrollY
			rig.frame(now)

			before := rig.topSourceLine()
			if before == 0 {
				t.Fatalf("setup: expected to be scrolled past the first line, got top line %d", before)
			}

			rig.size.X = tc.endW
			rig.frame(now)
			rig.frame(now)

			after := rig.topSourceLine()
			if after != before {
				t.Errorf("response text jumped on resize %d->%d: top source line was %d, became %d",
					tc.startW, tc.endW, before, after)
			}
		})
	}
}
