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

func htmlLikeText() string {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "<div id=%d>", i)
		for j := 0; j < 60; j++ {
			b.WriteString(" content word")
		}
		b.WriteString("</div>\n")
	}
	return b.String()
}

func (rig *respRig) bottomGap() int {
	return rig.v.lastTotalH - rig.v.lastViewportH - rig.v.scrollY
}

func (rig *respRig) topAnchor() (int, int) {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	innerW := rig.size.X - 2*4
	return rig.v.scrollAnchor(rig.v.lastLineHeight, adv, innerW, rig.wrap)
}

func TestResizeRoundTripReturnsToSamePlace(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}
	rig.v.SetScrollY(rig.v.lastTotalH / 2)
	rig.frame(now)
	rig.frame(now)

	v := rig.v
	startLine, startSub := rig.topAnchor()
	if startLine == 0 && startSub == 0 {
		t.Fatalf("setup: expected mid-document, top at 0/0")
	}

	for w := 392; w >= 320; w -= 8 {
		rig.size.X = w
		rig.frame(now)
	}
	for w := 328; w <= 400; w += 8 {
		rig.size.X = w
		rig.frame(now)
	}
	rig.frame(now)
	rig.frame(now)

	endLine, endSub := rig.topAnchor()
	if endLine != startLine || endSub < startSub-1 || endSub > startSub+1 {
		t.Errorf("round-trip drag moved visible content: top %d/%d -> %d/%d (scrollY %d, lineH %d)",
			startLine, startSub, endLine, endSub, v.scrollY, v.lastLineHeight)
	}
}

func (rig *respRig) subRowRem() int {
	line, sub := rig.topAnchor()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
	}
	adv := measureCharAdvance(rig.shaper, font.Font{}, unit.Sp(13), gtx)
	innerW := rig.size.X - 2*4
	return rig.v.scrollY - rig.v.scrollYForAnchor(line, sub, rig.v.lastLineHeight, adv, innerW, rig.wrap)
}

func TestResizePreservesSubRowOffset(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}

	rig.v.SetScrollY(20*rig.v.lastLineHeight + 7)
	rig.frame(now)
	if rem := rig.subRowRem(); rem != 7 {
		t.Fatalf("setup: sub-row remainder = %d, want 7", rem)
	}

	rig.size.X = 360
	rig.frame(now)
	rig.frame(now)

	if rem := rig.subRowRem(); rem != 7 {
		t.Errorf("resize snapped the view to a whole row: sub-row remainder %d, want 7 (scrollY=%d lineH=%d)",
			rem, rig.v.scrollY, rig.v.lastLineHeight)
	}
}

func TestResizeAtBottomStaysAtBottom(t *testing.T) {
	rig := newRespRig(htmlLikeText(), true)
	rig.size.X = 400
	now := time.Unix(1700000000, 0)
	for i := 0; i < 3; i++ {
		rig.frame(now)
	}
	for i := 0; i < 5; i++ {
		rig.v.SetScrollY(1 << 30)
		rig.frame(now)
	}
	v := rig.v
	if v.scrollY <= 0 {
		t.Fatalf("setup: scrollY=%d", v.scrollY)
	}
	if g := rig.bottomGap(); g != 0 {
		t.Fatalf("setup: not at bottom, gap=%d", g)
	}

	for w := 392; w >= 320; w -= 8 {
		rig.size.X = w
		rig.frame(now)
		if g := rig.bottomGap(); g < 0 || g > v.lastLineHeight {
			t.Fatalf("width %d: viewer left the end mid-drag, gap=%dpx (lineH=%d)", w, g, v.lastLineHeight)
		}
	}
	rig.frame(now)
	rig.frame(now)

	if g := rig.bottomGap(); g < 0 || g > v.lastLineHeight {
		t.Errorf("after resizing at bottom the viewer drifted %dpx from the end (lineH=%d totalH=%d viewH=%d scrollY=%d)",
			g, v.lastLineHeight, v.lastTotalH, v.lastViewportH, v.scrollY)
	}
}
