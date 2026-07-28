package workspace

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/io/key"
)

func bigPrettyLines(n int) string {
	var b strings.Builder
	b.Grow(n * 24)
	for i := 0; i < n; i++ {
		b.WriteString(`      "id": 123456,`)
		b.WriteByte('\n')
	}
	return b.String()
}

func TestJumpToEndPaintsOnlyTheViewport(t *testing.T) {
	rig := newRespRig(bigPrettyLines(200000), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	rig.click(40, 40, 10*time.Millisecond)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	at := 50 * time.Millisecond
	rig.r.Queue(key.Event{
		Name:      key.NameEnd,
		Modifiers: key.ModShortcut,
		State:     key.Press,
	})
	rig.frame(time.Unix(1700000000, 0).Add(at))
	runtime.ReadMemStats(&after)

	if rig.v.scrollY <= 0 {
		t.Fatalf("ctrl+end did not scroll: scrollY=%d totalH=%d", rig.v.scrollY, rig.v.lastTotalH)
	}
	alloc := after.TotalAlloc - before.TotalAlloc
	if alloc > 32<<20 {
		t.Errorf("jump-to-end frame allocated %.1fMB; it must paint only the viewport", float64(alloc)/(1<<20))
	}
}

func TestJumpToEndKeepsScrollAnchorConsistent(t *testing.T) {
	rig := newRespRig(bigPrettyLines(50000), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	rig.click(40, 40, 10*time.Millisecond)

	rig.r.Queue(key.Event{Name: key.NameEnd, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame(time.Unix(1700000000, 0).Add(50 * time.Millisecond))

	v := rig.v
	wantLine, wantAccum := v.firstChunkAtFn(v.scrollY, v.lastLineHeight, 0, rig.size.X, true)
	if wantLine == 0 && v.scrollY > v.lastLineHeight {
		t.Fatalf("anchor collapsed to line 0 at scrollY=%d", v.scrollY)
	}
	if gap := v.scrollY - wantAccum; gap < 0 || gap > v.lastLineHeight {
		t.Errorf("anchor %d px off from scrollY=%d (line %d)", gap, v.scrollY, wantLine)
	}
}
