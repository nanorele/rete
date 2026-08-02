package workspace

import (
	"fmt"
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
)

func scratchGtx(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 400)),
		Now:         time.Unix(1700000000, 0),
	}
}

// scratchBody builds lines of deliberately different lengths, so two chunks
// never produce the same glyph geometry. WrapGlyph stores only positions, so
// equal-width lines would mask a buffer being reused underneath a live slice.
func scratchBody() string {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "%s%s\n", strings.Repeat("w", 3+i*11), strings.Repeat(".il ", i%5))
	}
	return b.String()
}

func scratchSetup(v *textCore) (layout.Context, *text.Shaper, font.Font) {
	shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
	fnt := font.Font{Typeface: "Go Mono"}
	v.layoutShaper, v.layoutFont, v.layoutSize = shaper, fnt, unit.Sp(13)
	v.lastLineHeight = 16
	return scratchGtx(new(op.Ops)), shaper, fnt
}

// TestScratchBuffersIndependent pins the invariant the two glyph scratch
// buffers exist for: a slice produced for painting stays valid while the
// hit-test helpers shape other chunks, as happens when a caret or selection
// bound is resolved in the middle of painting a chunk. With one shared buffer
// the painted glyphs silently become another chunk's.
func TestScratchBuffersIndependent(t *testing.T) {
	v := NewResponseViewer()
	v.SetText(scratchBody())
	gtx, shaper, fnt := scratchSetup(&v.textCore)

	// Paint a long line, exactly as paintChunk does.
	const paintLine = 40
	chunkStart, chunkEnd := v.lineStarts[paintLine], v.lineStarts[paintLine+1]
	v.paintScratch = widgets.ShapeChunkForWrapInto(v.paintScratch, shaper, fnt, v.layoutSize,
		gtx, v.text[chunkStart:chunkEnd], 600)
	painted := v.paintScratch
	if len(painted) < 100 {
		t.Fatalf("paint path produced %d glyphs, expected a long line", len(painted))
	}
	before := append([]widgets.WrapGlyph(nil), painted...)
	wantX, wantLine := widgets.CaretXYInWrap(painted, 300)

	// Run every hit-test entry point on shorter, differently shaped chunks
	// while the painted slice is still live.
	for line := 0; line < 12; line++ {
		s, e := v.lineStarts[line], v.lineStarts[line+1]
		v.wrapCaretXY(line, s, e, s+3, gtx, 600)
		v.wrapByteAt(line, s, e, 120, 0, gtx, 600)
		v.wrapMaxLineOf(line, s, e, gtx, 600)
	}

	if len(painted) != len(before) {
		t.Fatalf("painted slice length changed: %d, want %d", len(painted), len(before))
	}
	for i := range painted {
		if painted[i] != before[i] {
			t.Fatalf("hit-test overwrote painted glyph %d of %d", i, len(painted))
		}
	}
	if gotX, gotLine := widgets.CaretXYInWrap(painted, 300); gotX != wantX || gotLine != wantLine {
		t.Fatalf("caret over painted glyphs = (%d,%d), want (%d,%d)", gotX, gotLine, wantX, wantLine)
	}
	if len(v.paintScratch) > 0 && len(v.hitScratch) > 0 &&
		&v.paintScratch[0] == &v.hitScratch[0] {
		t.Fatal("paint and hit-test scratch share a backing array")
	}
}

// TestScratchBuffersIndependentEditor is the same invariant for the request
// editor, which has its own textCore and its own pair of buffers.
func TestScratchBuffersIndependentEditor(t *testing.T) {
	e := NewRequestEditor()
	e.SetText(scratchBody())
	gtx, shaper, fnt := scratchSetup(&e.textCore)

	const paintLine = 45
	s0, e0 := e.lineStarts[paintLine], e.lineStarts[paintLine+1]
	e.paintScratch = widgets.ShapeChunkForWrapInto(e.paintScratch, shaper, fnt, e.layoutSize,
		gtx, e.text[s0:e0], 300)
	painted := e.paintScratch
	if len(painted) < 100 {
		t.Fatalf("paint path produced %d glyphs, expected a long line", len(painted))
	}
	before := append([]widgets.WrapGlyph(nil), painted...)

	for line := 0; line < 15; line++ {
		s, en := e.lineStarts[line], e.lineStarts[line+1]
		e.wrapCaretXY(line, s, en, s+7, gtx, 300)
		e.wrapByteAt(line, s, en, 55, 0, gtx, 300)
	}

	for i := range painted {
		if painted[i] != before[i] {
			t.Fatalf("hit-test overwrote painted glyph %d of %d", i, len(painted))
		}
	}
}
