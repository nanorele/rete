package ui

import (
	"image"
	"image/color"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/semantic"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"golang.org/x/image/math/fixed"
)

type coloredSpan struct {
	Start, End int
	Color      color.NRGBA
}

func paintColoredText(
	gtx layout.Context,
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	txt string,
	spans []coloredSpan,
	defaultColor color.NRGBA,
	wrap bool,
	maxW int,
) layout.Dimensions {
	cs := gtx.Constraints
	textSize := fixed.I(gtx.Sp(size))

	params := text.Parameters{
		Font:    fnt,
		PxPerEm: textSize,
		Locale:  gtx.Locale,
	}
	if wrap {
		params.WrapPolicy = text.WrapGraphemes
		params.MaxWidth = maxW
	} else {
		params.MaxLines = 1
		params.MaxWidth = 1 << 24
	}
	shaper.LayoutString(params, txt)

	m := op.Record(gtx.Ops)
	viewport := image.Rectangle{Max: cs.Max}
	semantic.LabelOp(txt).Add(gtx.Ops)

	var (
		lineGlyphs []text.Glyph
		lineColors []color.NRGBA
		first      = true
		baseline   int
		bounds     image.Rectangle
		byteIdx    int
		spanIdx    int
	)

	colorAtByte := func(b int) color.NRGBA {
		for spanIdx < len(spans) && spans[spanIdx].End <= b {
			spanIdx++
		}
		if spanIdx < len(spans) && b >= spans[spanIdx].Start && b < spans[spanIdx].End {
			return spans[spanIdx].Color
		}
		return defaultColor
	}

	flushLine := func() {
		if len(lineGlyphs) == 0 {
			return
		}
		i := 0
		for i < len(lineGlyphs) {
			j := i + 1
			curCol := lineColors[i]
			for j < len(lineGlyphs) && lineColors[j] == curCol {
				j++
			}
			runOff := f32.Point{
				X: fixedToFloat(lineGlyphs[i].X),
				Y: float32(lineGlyphs[i].Y),
			}.Sub(layout.FPt(viewport.Min))
			t := op.Affine(f32.AffineId().Offset(runOff)).Push(gtx.Ops)
			path := shaper.Shape(lineGlyphs[i:j])
			outline := clip.Outline{Path: path}.Op().Push(gtx.Ops)
			paint.ColorOp{Color: curCol}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
			outline.Pop()
			t.Pop()
			i = j
		}
		lineGlyphs = lineGlyphs[:0]
		lineColors = lineColors[:0]
	}

	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		logicalBounds := image.Rectangle{
			Min: image.Pt(g.X.Floor(), int(g.Y)-g.Ascent.Ceil()),
			Max: image.Pt((g.X + g.Advance).Ceil(), int(g.Y)+g.Descent.Ceil()),
		}
		if first {
			first = false
			baseline = int(g.Y)
			bounds = logicalBounds
		} else {
			if logicalBounds.Min.X < bounds.Min.X {
				bounds.Min.X = logicalBounds.Min.X
			}
			if logicalBounds.Min.Y < bounds.Min.Y {
				bounds.Min.Y = logicalBounds.Min.Y
			}
			if logicalBounds.Max.X > bounds.Max.X {
				bounds.Max.X = logicalBounds.Max.X
			}
			if logicalBounds.Max.Y > bounds.Max.Y {
				bounds.Max.Y = logicalBounds.Max.Y
			}
		}

		col := colorAtByte(byteIdx)
		lineGlyphs = append(lineGlyphs, g)
		lineColors = append(lineColors, col)

		for r := uint16(0); r < g.Runes; r++ {
			if byteIdx >= len(txt) {
				break
			}
			_, sz := utf8.DecodeRuneInString(txt[byteIdx:])
			byteIdx += sz
		}

		if g.Flags&text.FlagLineBreak != 0 {
			flushLine()
		}
	}
	flushLine()

	call := m.Stop()
	clipStack := clip.Rect(viewport).Push(gtx.Ops)
	call.Add(gtx.Ops)
	dims := layout.Dimensions{Size: bounds.Size()}
	dims.Size = cs.Constrain(dims.Size)
	dims.Baseline = dims.Size.Y - baseline
	clipStack.Pop()
	return dims
}

func fixedToFloat(i fixed.Int26_6) float32 {
	return float32(i) / 64.0
}

type wrapGlyph struct {
	byteStart int
	byteEnd   int
	x         fixed.Int26_6
	advance   fixed.Int26_6
	line      int
	isBreak   bool
}

func shapeChunkForWrap(
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	gtx layout.Context,
	chunkText []byte,
	maxW int,
) []wrapGlyph {
	if len(chunkText) == 0 || shaper == nil {
		return nil
	}
	if maxW < 1 {
		maxW = 1
	}
	shaper.LayoutString(text.Parameters{
		Font:       fnt,
		PxPerEm:    fixed.I(gtx.Sp(size)),
		Locale:     gtx.Locale,
		WrapPolicy: text.WrapGraphemes,
		MaxWidth:   maxW,
	}, string(chunkText))

	out := make([]wrapGlyph, 0, 64)
	line := 0
	byteAccum := 0
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		runeBytes := 0
		for r := uint16(0); r < g.Runes && byteAccum+runeBytes < len(chunkText); r++ {
			_, sz := utf8.DecodeRune(chunkText[byteAccum+runeBytes:])
			runeBytes += sz
		}
		out = append(out, wrapGlyph{
			byteStart: byteAccum,
			byteEnd:   byteAccum + runeBytes,
			x:         g.X,
			advance:   g.Advance,
			line:      line,
			isBreak:   g.Flags&text.FlagLineBreak != 0,
		})
		byteAccum += runeBytes
		if g.Flags&text.FlagLineBreak != 0 {
			line++
		}
	}
	return out
}

func caretXYInWrap(glyphs []wrapGlyph, byteOff int) (xPx, subLine int) {
	if len(glyphs) == 0 {
		return 0, 0
	}
	if byteOff <= 0 {
		return glyphs[0].x.Round(), glyphs[0].line
	}
	for _, g := range glyphs {
		if byteOff <= g.byteStart {
			return g.x.Round(), g.line
		}
	}
	last := glyphs[len(glyphs)-1]
	return (last.x + last.advance).Round(), last.line
}

func byteOffInWrap(glyphs []wrapGlyph, posX, targetLine int) int {
	if len(glyphs) == 0 {
		return 0
	}
	if targetLine < 0 {
		targetLine = 0
	}
	firstIdx, lastIdx := -1, -1
	for i, g := range glyphs {
		if g.line == targetLine {
			if firstIdx < 0 {
				firstIdx = i
			}
			lastIdx = i
		} else if g.line > targetLine {
			break
		}
	}
	if firstIdx < 0 {
		return glyphs[len(glyphs)-1].byteEnd
	}
	posXf := fixed.I(posX)
	for i := firstIdx; i <= lastIdx; i++ {
		g := glyphs[i]
		if posXf < g.x+g.advance/2 {
			return g.byteStart
		}
	}
	return glyphs[lastIdx].byteEnd
}

func wrapMaxLine(glyphs []wrapGlyph) int {
	if len(glyphs) == 0 {
		return 0
	}
	return glyphs[len(glyphs)-1].line
}
