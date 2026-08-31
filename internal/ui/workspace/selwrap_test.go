package workspace

import (
	"strings"
	"testing"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"tracto/internal/ui/widgets"
)

// A wrapped line whose last glyph sits at the right margin used to be filled to
// the viewport edge, showing selected empty space after the last character.
func TestWrapSelectionStopsAtLastGlyph(t *testing.T) {
	const innerW = 400
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	gtx := bigTextGtx()
	fnt := font.Font{Typeface: widgets.MonoTypeface}
	size := unit.Sp(12)

	chunk := []byte(strings.Repeat("abcdefghij0123456789", 40))
	glyphs := widgets.ShapeChunkForWrap(shaper, fnt, size, gtx, chunk, innerW)
	if widgets.WrapMaxLine(glyphs) < 2 {
		t.Fatalf("chunk did not wrap into enough sub-lines: %d", widgets.WrapMaxLine(glyphs)+1)
	}
	lineStarts := widgets.WrapLineStarts(glyphs)

	shortOfEdge := 0
	for wl := 0; wl < len(lineStarts)-1; wl++ {
		x := wrapSubLineEndX(glyphs, wl, innerW)
		wantX, wantLine := widgets.CaretXYInWrap(glyphs, lineStarts[wl+1])
		if wantLine != wl {
			t.Fatalf("sub-line %d: caret at the next line start reported line %d", wl, wantLine)
		}
		if x != wantX && wantX <= innerW {
			t.Errorf("sub-line %d fill ends at %d, want the last glyph end %d", wl, x, wantX)
		}
		if x < innerW {
			shortOfEdge++
		}
	}
	if shortOfEdge == 0 {
		t.Error("no wrapped sub-line stopped short of the viewport edge; the fill is still viewport-wide")
	}

	last := len(lineStarts) - 1
	if x := wrapSubLineEndX(glyphs, last, innerW); x <= 0 || x > innerW {
		t.Errorf("last sub-line fill ends at %d, want inside (0, %d]", x, innerW)
	}
}
