package workspace

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/image/math/fixed"
)

func makeTestGtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
}

func TestResponseSelection_DefaultsZero(t *testing.T) {
	v := NewResponseViewer()
	s, e := v.Selection()
	if s != 0 || e != 0 {
		t.Errorf("default Selection = (%d,%d), want (0,0)", s, e)
	}
}

func TestResponseSelection_SetCaretSyncsBoth(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world")
	v.SetCaret(2, 7)
	s, e := v.Selection()
	if s != 2 || e != 7 {
		t.Errorf("Selection = (%d,%d), want (2,7)", s, e)
	}
	if got := v.SelectedText(); got != "llo w" {
		t.Errorf("SelectedText must reflect the selection set via SetCaret, got %q", got)
	}
}

func TestSetScrollCaret_NoPanicNoEffect(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollCaret(true)
	v.SetScrollCaret(false)
}

func TestSetScrollY_NegativeClamps(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollY(-100)
	if v.GetScrollY() != 0 {
		t.Errorf("expected clamp to 0, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_ClampsToContentBounds(t *testing.T) {
	v := NewResponseViewer()
	v.lastTotalH = 1000
	v.lastViewportH = 200
	v.SetScrollY(5000)
	if v.GetScrollY() != 800 {
		t.Errorf("expected clamp to 800, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_ZeroContentMaxClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.lastTotalH = 50
	v.lastViewportH = 200
	v.SetScrollY(100)
	if v.GetScrollY() != 0 {
		t.Errorf("expected clamp to 0 when viewport>total, got %d", v.GetScrollY())
	}
}

func TestSetScrollY_NoClampWhenLayoutUnseeded(t *testing.T) {
	v := NewResponseViewer()

	v.SetScrollY(123)
	if v.GetScrollY() != 123 {
		t.Errorf("without layout info, scrollY should be kept; got %d", v.GetScrollY())
	}
}

func TestSetScrollX_NegativeClamps(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollX(-50)
	if v.GetScrollX() != 0 {
		t.Errorf("SetScrollX(-50) = %d, want 0", v.GetScrollX())
	}
}

func TestSetScrollX_PositivePreserved(t *testing.T) {
	v := NewResponseViewer()
	v.SetScrollX(42)
	if v.GetScrollX() != 42 {
		t.Errorf("SetScrollX(42) = %d, want 42", v.GetScrollX())
	}
}

func TestLineForByteOffset(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nbb\nccc\nd")

	cases := []struct {
		off  int
		want int
	}{
		{0, 0},
		{1, 0},
		{2, 1},
		{3, 1},
		{5, 2},
		{7, 2},
		{9, 3},
		{12, 3},
	}
	for _, tc := range cases {
		if got := v.lineForByteOffset(tc.off); got != tc.want {
			t.Errorf("lineForByteOffset(%d) = %d, want %d (lineStarts=%v)", tc.off, got, tc.want, v.lineStarts)
		}
	}
}

func TestLineForByteOffset_Empty(t *testing.T) {
	v := NewResponseViewer()
	if got := v.lineForByteOffset(0); got != 0 {
		t.Errorf("empty: got %d", got)
	}
	if got := v.lineForByteOffset(999); got != 0 {
		t.Errorf("empty out-of-range: got %d", got)
	}
}

func TestLineForByteOffset_NegativeReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nb")
	if got := v.lineForByteOffset(-100); got != 0 {
		t.Errorf("negative offset: got %d", got)
	}
}

func TestMoveCaret_NoExtendCollapsesSelection(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.selStart, v.selEnd = 1, 4
	v.moveCaret(2, false)
	if v.selStart != 2 || v.selEnd != 2 {
		t.Errorf("moveCaret(no-extend) should collapse to (2,2); got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ExtendKeepsAnchor(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.selStart, v.selEnd = 1, 1
	v.moveCaret(4, true)
	if v.selStart != 1 || v.selEnd != 4 {
		t.Errorf("moveCaret(extend) should keep anchor and move end; got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ClampsOutOfRange(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.moveCaret(-5, false)
	if v.selStart != 0 || v.selEnd != 0 {
		t.Errorf("negative not clamped to 0; got (%d,%d)", v.selStart, v.selEnd)
	}
	v.moveCaret(1000, false)
	if v.selStart != 3 || v.selEnd != 3 {
		t.Errorf("over-end not clamped; got (%d,%d)", v.selStart, v.selEnd)
	}
}

func TestMoveCaret_ResetsDragActive(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.dragActive = true
	v.moveCaret(1, false)
	if v.dragActive {
		t.Errorf("dragActive should be reset")
	}
}

func TestCharLeft(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aбc")
	cases := []struct{ in, want int }{
		{0, 0},
		{1, 0},
		{3, 1},
		{4, 3},
		{-5, 0},
	}
	for _, tc := range cases {
		if got := v.charLeft(tc.in); got != tc.want {
			t.Errorf("charLeft(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCharRight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aбc")
	cases := []struct{ in, want int }{
		{0, 1},
		{1, 3},
		{3, 4},
		{4, 4},
		{100, 4},
	}
	for _, tc := range cases {
		if got := v.charRight(tc.in); got != tc.want {
			t.Errorf("charRight(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestWordLeft(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("foo bar  baz")

	if got := v.wordLeft(11); got != 9 {
		t.Errorf("wordLeft(11)=%d, want 9", got)
	}

	if got := v.wordLeft(9); got != 4 {
		t.Errorf("wordLeft(9)=%d, want 4", got)
	}
	if got := v.wordLeft(0); got != 0 {
		t.Errorf("wordLeft(0)=%d, want 0", got)
	}
	if got := v.wordLeft(-3); got != 0 {
		t.Errorf("wordLeft(-3)=%d, want 0", got)
	}
}

func TestWordRight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("foo bar  baz")

	if got := v.wordRight(0); got != 4 {
		t.Errorf("wordRight(0)=%d, want 4", got)
	}

	if got := v.wordRight(4); got != 9 {
		t.Errorf("wordRight(4)=%d, want 9", got)
	}
	if got := v.wordRight(12); got != 12 {
		t.Errorf("wordRight(12)=%d, want 12", got)
	}
	if got := v.wordRight(999); got != 12 {
		t.Errorf("wordRight past-end should clamp; got %d", got)
	}
}

func TestColumnAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nпривет")
	if got := v.columnAt(0); got != 0 {
		t.Errorf("columnAt(0)=%d, want 0", got)
	}
	if got := v.columnAt(2); got != 2 {
		t.Errorf("columnAt(2)=%d, want 2", got)
	}
	if got := v.columnAt(3); got != 3 {
		t.Errorf("columnAt(3)=%d, want 3 (end of first line)", got)
	}

	if got := v.columnAt(4); got != 0 {
		t.Errorf("columnAt(4)=%d, want 0 (start of 'привет')", got)
	}

	if got := v.columnAt(6); got != 1 {
		t.Errorf("columnAt(6)=%d, want 1", got)
	}

	if got := v.columnAt(16); got != 6 {
		t.Errorf("columnAt(16)=%d, want 6", got)
	}
}

func TestOffsetAtColumn(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nпривет")

	if got := v.offsetAtColumn(0, 0); got != 0 {
		t.Errorf("offsetAtColumn(0,0)=%d, want 0", got)
	}
	if got := v.offsetAtColumn(0, 2); got != 2 {
		t.Errorf("offsetAtColumn(0,2)=%d, want 2", got)
	}
	if got := v.offsetAtColumn(0, 99); got != 3 {
		t.Errorf("offsetAtColumn(0,99) should clamp to lineEnd=3, got %d", got)
	}
	if got := v.offsetAtColumn(0, -1); got != 0 {
		t.Errorf("offsetAtColumn(0,-1)=%d, want 0", got)
	}

	if got := v.offsetAtColumn(4, 1); got != 6 {
		t.Errorf("offsetAtColumn(4,1) for 'п' should advance 2 bytes; got %d", got)
	}
	if got := v.offsetAtColumn(4, 6); got != 16 {
		t.Errorf("offsetAtColumn(4,6) should walk all 6 runes; got %d", got)
	}
}

func TestLineUp_FromSecondLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij")

	if got := v.lineUp(8, 2); got != 2 {
		t.Errorf("lineUp(8,2)=%d, want 2", got)
	}
}

func TestLineUp_FromFirstLineReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij")
	if got := v.lineUp(3, 3); got != 0 {
		t.Errorf("lineUp from first line should be 0; got %d", got)
	}
}

func TestLineDown(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcde\nfghij\nklmno")

	if got := v.lineDown(2, 2); got != 8 {
		t.Errorf("lineDown(2,2)=%d, want 8", got)
	}
}

func TestLineDown_FromLastLineGoesToEOF(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\ncd")
	if got := v.lineDown(4, 1); got != 5 {
		t.Errorf("lineDown from last line should clamp to len(text)=5; got %d", got)
	}
}

func TestLineDown_CRLFConsumed(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\r\ncd")

	if got := v.lineDown(0, 1); got != 5 {
		t.Errorf("lineDown across CRLF; got %d, want 5", got)
	}
}

func TestVisualXAt_NilShaperReturnsZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello\nworld")
	if got := v.visualXAt(3, makeTestGtx(), 200); got != 0 {
		t.Errorf("with nil shaper, visualXAt should degrade to 0; got %d", got)
	}
}

func TestWrapLineMoveX_NoShaper_BoundaryBehavior(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("aa\nbb\ncc")
	gtx := makeTestGtx()

	if got := v.wrapLineMoveX(0, 0, +1, gtx, 100); got != 3 {
		t.Errorf("wrapLineMoveX down: got %d, want 3 (next line start)", got)
	}

	if got := v.wrapLineMoveX(7, 0, +1, gtx, 100); got != len(v.text) {
		t.Errorf("wrapLineMoveX down from last line; got %d, want %d", got, len(v.text))
	}

	if got := v.wrapLineMoveX(0, 0, -1, gtx, 100); got != 0 {
		t.Errorf("wrapLineMoveX up from line 0; got %d, want 0", got)
	}

	if got := v.wrapLineMoveX(3, 0, -1, gtx, 100); got != 0 {
		t.Errorf("wrapLineMoveX up from line 1; got %d, want 0", got)
	}
}

func TestEnsureCaretVisible_NoopWithoutLayout(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello")
	v.selEnd = 2
	v.scrollY = 999
	v.ensureCaretVisible()
	if v.scrollY != 999 {
		t.Errorf("without lastLineHeight, ensureCaretVisible must be a no-op; got scrollY=%d", v.scrollY)
	}
}

func TestEnsureCaretVisible_ScrollsUp(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4")
	v.lastLineHeight = 10
	v.lastViewportH = 25
	v.lastTotalH = 50
	v.padChunkHeights()
	v.scrollY = 30
	v.selEnd = 0
	v.ensureCaretVisible()
	if v.scrollY != 0 {
		t.Errorf("expected scrollY snapped up to 0, got %d", v.scrollY)
	}
}

func TestEnsureCaretVisible_ScrollsDown(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4")
	v.lastLineHeight = 10
	v.lastViewportH = 20
	v.lastTotalH = 50
	v.padChunkHeights()
	v.scrollY = 0

	v.selEnd = len(v.text)
	v.ensureCaretVisible()
	if v.scrollY != 30 {
		t.Errorf("expected scrollY=30, got %d", v.scrollY)
	}
}

func TestEnsureCaretVisible_UsesPerChunkHeightWhenAvailable(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.lastLineHeight = 10
	v.lastViewportH = 20
	v.lastTotalH = 100
	v.padChunkHeights()

	v.chunkHeights[0] = 40
	v.scrollY = 0
	v.selEnd = 7
	v.ensureCaretVisible()

	if v.scrollY != 40 {
		t.Errorf("expected scrollY=40 reflecting tall chunkHeights[0]=40, got %d", v.scrollY)
	}
}

func TestScrollToByteOffset_NoopWithoutLayout(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello")
	v.scrollY = 42
	v.scrollToByteOffset(2)
	if v.scrollY != 42 {
		t.Errorf("without lastLineHeight, scrollToByteOffset must be no-op; got %d", v.scrollY)
	}
}

func TestScrollToByteOffset_CentersTargetLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3\nL4\nL5\nL6")
	v.lastLineHeight = 10
	v.lastViewportH = 40
	v.lastTotalH = 70
	v.padChunkHeights()

	v.scrollToByteOffset(12)
	if v.scrollY != 20 {
		t.Errorf("expected centered scrollY=20, got %d", v.scrollY)
	}
}

func TestScrollToByteOffset_ClampsToMax(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3")
	v.lastLineHeight = 10
	v.lastViewportH = 5
	v.lastTotalH = 40
	v.padChunkHeights()

	v.scrollToByteOffset(len(v.text))
	if v.scrollY != 28 {
		t.Errorf("expected scrollY=28, got %d", v.scrollY)
	}
}

func TestScrollToByteOffset_NegativeTargetClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.lastLineHeight = 10
	v.lastViewportH = 100
	v.lastTotalH = 10
	v.padChunkHeights()

	v.scrollToByteOffset(0)
	if v.scrollY != 0 {
		t.Errorf("expected clamp to 0, got %d", v.scrollY)
	}
}

func TestScrollToByteOffset_UsesPerChunkHeight(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.lastLineHeight = 10
	v.lastViewportH = 0
	v.lastTotalH = 100
	v.padChunkHeights()
	v.chunkHeights[0] = 100
	v.chunkHeights[1] = 5

	v.scrollToByteOffset(6)
	if v.scrollY != 105 {
		t.Errorf("expected scrollY=105, got %d", v.scrollY)
	}
}

func TestFirstChunkAtFn_ZeroY(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	idx, acc := v.firstChunkAtFn(0, 10, fixed.I(8), 200, false)
	if idx != 0 || acc != 0 {
		t.Errorf("y=0 should return (0,0); got (%d,%d)", idx, acc)
	}
}

func TestFirstChunkAtFn_MidContent(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2\nL3")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}

	idx, acc := v.firstChunkAtFn(15, 10, fixed.I(8), 200, false)
	if idx != 1 || acc != 10 {
		t.Errorf("y=15: got (%d,%d), want (1,10)", idx, acc)
	}
}

func TestFirstChunkAtFn_BeyondLast(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	idx, acc := v.firstChunkAtFn(9999, 10, fixed.I(8), 200, false)
	if idx != len(v.chunkHeights) || acc != 20 {
		t.Errorf("beyond last: got (%d,%d), want (%d,%d)", idx, acc, len(v.chunkHeights), 20)
	}
}

func TestFirstChunkAtFn_ZeroHeightFallsBackToEstimate(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("L0\nL1\nL2")
	v.padChunkHeights()

	idx, acc := v.firstChunkAtFn(15, 10, fixed.I(8), 200, false)
	if idx != 1 || acc != 10 {
		t.Errorf("estimate path: got (%d,%d), want (1,10)", idx, acc)
	}
}

func TestCoordToByteOffset_GuardsReturnZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc")
	v.padChunkHeights()
	gtx := makeTestGtx()
	if got := v.coordToByteOffset(gtx, 0, 0, 0, 10, 100, false); got != 0 {
		t.Errorf("zero advance must return 0; got %d", got)
	}
	if got := v.coordToByteOffset(gtx, 0, 0, fixed.I(8), 0, 100, false); got != 0 {
		t.Errorf("zero lineHeight must return 0; got %d", got)
	}
	empty := NewResponseViewer()

	empty.lineStarts = empty.lineStarts[:0]
	if got := empty.coordToByteOffset(gtx, 0, 0, fixed.I(8), 10, 100, false); got != 0 {
		t.Errorf("empty lineStarts must return 0; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_FirstLineCol(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 24, 0, adv, 10, 200, false); got != 3 {
		t.Errorf("col 3 expected byte 3; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_BeyondLineClampsToEnd(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("ab\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 1000, 0, adv, 10, 200, false); got != 2 {
		t.Errorf("clamp to line end: got %d, want 2", got)
	}
}

func TestCoordToByteOffset_NoWrap_NegativeXClampsToCol0(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)
	if got := v.coordToByteOffset(gtx, -50, 0, adv, 10, 200, false); got != 0 {
		t.Errorf("negative X should clamp to 0; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_NegativeYClampsToZero(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	v.scrollY = 5
	if got := v.coordToByteOffset(gtx, 8, -100, adv, 10, 200, false); got != 1 {
		t.Errorf("negative Y -> first line, col 1 -> byte 1; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_PicksSecondLine(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abc\nXYZW")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 16, 15, adv, 10, 200, false); got != 6 {
		t.Errorf("line1 col2 expected byte 6; got %d", got)
	}
}

func TestCoordToByteOffset_NoWrap_BeyondLastChunkReturnsTextEnd(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("a\nb")
	v.padChunkHeights()

	gtx := makeTestGtx()
	adv := fixed.I(8)

	v.chunkHeights = v.chunkHeights[:0]
	if got := v.coordToByteOffset(gtx, 0, 0, adv, 10, 200, false); got != len(v.text) {
		t.Errorf("empty chunkHeights -> chunkIdx=-1 path; got %d, want %d", got, len(v.text))
	}
}

func TestCoordToByteOffset_Wrap_NilShaperReturnsChunkStart(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world\nXYZ")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 10, 15, adv, 10, 200, true); got != 12 {
		t.Errorf("wrap+nil shaper: expected chunkStart=12; got %d", got)
	}
}

func TestBodyTypeRowMinWidth(t *testing.T) {
	tab := NewRequestTab("t")
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
	got := tab.bodyTypeRowMinWidth(gtx, th)
	expected := computeBodyTypeRowMinWidth(gtx, th, tab.BodyType.String())
	if got != expected {
		t.Errorf("bodyTypeRowMinWidth mismatch: %d vs computed %d", got, expected)
	}
	if got <= 0 {
		t.Errorf("bodyTypeRowMinWidth should be positive; got %d", got)
	}
}
