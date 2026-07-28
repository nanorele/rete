package workspace

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"tracto/internal/ui/widgets"

	"golang.org/x/image/math/fixed"
	"unicode/utf8"
)

func bigTextGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(900, 700)),
		Now:         time.Unix(1700000000, 0),
	}
}

func TestWrapPlanWindowingMatchesWholeLineShaping(t *testing.T) {
	const innerW = 892
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()

	for _, n := range []int{20 << 10, 100 << 10, 300 << 10} {
		line := strings.Repeat("abcdefghij0123456789", n/20)
		v := &textCore{lineStarts: []int{0}}
		v.text = []byte(line)
		v.padWrapPlans()

		want := widgets.WrapLineStarts(
			widgets.ShapeChunkForWrap(shaper, font.Font{}, unit.Sp(13), gtx, v.text, innerW),
		)
		p := v.ensureWrapPlan(0, 0, len(v.text), shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)

		if got := planTotalSubLines(p); got != len(want) {
			t.Errorf("n=%d: sub-lines = %d, want %d", n, got, len(want))
		}
		if p.height != len(want)*lineH {
			t.Errorf("n=%d: height = %d, want %d", n, p.height, len(want)*lineH)
		}
		for i, start := range p.starts {
			wantIdx := i * subLinesPerWrapChunk
			if wantIdx >= len(want) {
				t.Fatalf("n=%d: chunk %d has no matching wrap line", n, i)
			}
			if start != want[wantIdx] {
				t.Errorf("n=%d: chunk %d starts at byte %d, want %d", n, i, start, want[wantIdx])
			}
		}
	}
}

func TestWrapPlanHandlesLineBeyondFixedPointRange(t *testing.T) {
	const innerW = 892
	const lineH = 18
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()

	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(strings.Repeat("0123456789", 600000))
	v.padWrapPlans()

	p := v.ensureWrapPlan(0, 0, len(v.text), shaper, font.Font{}, unit.Sp(13), gtx, innerW, lineH)
	subLines := planTotalSubLines(p)
	if subLines < 10000 {
		t.Fatalf("6MB line wrapped into only %d sub-lines", subLines)
	}
	for i := range p.starts {
		start, end := planSubBounds(p, i, 0, len(v.text))
		if end-start > wrapShapeWindowBytes {
			t.Fatalf("sub-chunk %d spans %d bytes, over the %d-byte shaping window",
				i, end-start, wrapShapeWindowBytes)
		}
	}
}

func TestNoWrapPaintWindowTracksHorizontalScroll(t *testing.T) {
	const innerW = 400
	adv := fixedAdvance(8)

	line := strings.Repeat("abcdefghij", 20000)
	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(line)

	short := &textCore{lineStarts: []int{0}}
	short.text = []byte("hello world")
	s, e, x, cols := short.noWrapPaintWindow(0, len(short.text), innerW, adv)
	if s != 0 || e != len(short.text) || x != 0 || cols != 0 {
		t.Errorf("short chunk = (%d,%d,%d,%d), want the whole chunk", s, e, x, cols)
	}

	for _, scrollX := range []int{0, 800, 40000, 1_000_000, 40000, 0} {
		v.scrollX = scrollX
		start, end, xOff, totalCols := v.noWrapPaintWindow(0, len(v.text), innerW, adv)
		if totalCols != len(line) {
			t.Fatalf("scrollX=%d: totalCols = %d, want %d", scrollX, totalCols, len(line))
		}
		wantFirst := colAtPx(adv, scrollX)
		if wantFirst > len(line) {
			wantFirst = len(line)
		}
		if start != wantFirst {
			t.Errorf("scrollX=%d: window starts at byte %d, want %d", scrollX, start, wantFirst)
		}
		if xOff != colPx(adv, wantFirst) {
			t.Errorf("scrollX=%d: xOff = %d, want %d", scrollX, xOff, colPx(adv, wantFirst))
		}
		if span := end - start; span > colAtPx(adv, innerW)+3 {
			t.Errorf("scrollX=%d: painted %d bytes for a %d px viewport", scrollX, span, innerW)
		}
		if end > len(line) {
			t.Errorf("scrollX=%d: window end %d past line end %d", scrollX, end, len(line))
		}
	}
}

func TestNoWrapPaintWindowRespectsRuneBoundaries(t *testing.T) {
	const innerW = 400
	adv := fixedAdvance(8)

	line := strings.Repeat("привет-мир ", 3000)
	v := &textCore{lineStarts: []int{0}}
	v.text = []byte(line)
	if len(v.text) <= longLineThresholdBytes {
		t.Fatalf("test line too short: %d bytes", len(v.text))
	}

	for _, scrollX := range []int{0, 500, 5000} {
		v.scrollX = scrollX
		start, end, _, _ := v.noWrapPaintWindow(0, len(v.text), innerW, adv)
		if !utf8Boundary(v.text, start) || !utf8Boundary(v.text, end) {
			t.Errorf("scrollX=%d: window [%d,%d) splits a rune", scrollX, start, end)
		}
	}
}

func fixedAdvance(px int) fixed.Int26_6 { return fixed.I(px) }

func utf8Boundary(b []byte, i int) bool {
	return i == len(b) || utf8.RuneStart(b[i])
}
