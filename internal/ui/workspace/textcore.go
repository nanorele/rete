package workspace

import (
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"sort"
	"time"
	"tracto/internal/ui/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"unicode"
	"unicode/utf8"
)

type textCore struct {
	text       []byte
	lineStarts []int

	chunkHeights      []int
	chunkHeightsWrap  bool
	chunkHeightsWidth int

	wrapPlans []wrapPlan

	scrollY int
	scrollX int

	maxLineWidth int

	highlightStart int
	highlightEnd   int

	searchSpans []matchSpan

	selStart   int
	selEnd     int
	dragActive bool

	Scroller  gesture.Scroll
	ScrollerH gesture.Scroll
	Drag      gesture.Drag
	Click     gesture.Click

	lastLineHeight int
	lastTotalH     int
	lastViewportH  int
	descOvershoot  int
	lineBox        int

	tokens        []syntax.Token
	tokensLang    syntax.Lang
	tokensTxt     int
	tokensDirty   bool
	tokensChanged time.Time

	layoutShaper *text.Shaper
	layoutFont   font.Font
	layoutSize   unit.Sp
	layoutInnerW int
}

func (v *textCore) spansForChunk(chunkStart, chunkEnd int, sp theme.SyntaxPalette, bracketCycle bool) []widgets.ColoredSpan {
	if len(v.tokens) == 0 || chunkStart >= chunkEnd {
		return nil
	}
	first := sort.Search(len(v.tokens), func(i int) bool {
		return v.tokens[i].End > chunkStart
	})
	if first >= len(v.tokens) || v.tokens[first].Start >= chunkEnd {
		return nil
	}
	out := make([]widgets.ColoredSpan, 0, 16)
	for i := first; i < len(v.tokens); i++ {
		t := v.tokens[i]
		if t.Start >= chunkEnd {
			break
		}
		s, e := t.Start, t.End
		if s < chunkStart {
			s = chunkStart
		}
		if e > chunkEnd {
			e = chunkEnd
		}
		if s >= e {
			continue
		}
		out = append(out, widgets.ColoredSpan{
			Start: s - chunkStart,
			End:   e - chunkStart,
			Color: sp.ColorForToken(t.Kind, t.Depth, bracketCycle),
		})
	}
	return out
}

func (v *textCore) SelectedText() string {
	if v.selStart == v.selEnd {
		return ""
	}
	s, e := v.selStart, v.selEnd
	if s > e {
		s, e = e, s
	}
	if s < 0 {
		s = 0
	}
	if e > len(v.text) {
		e = len(v.text)
	}
	return string(v.text[s:e])
}

func (v *textCore) invalidateChunkHeights() {
	v.chunkHeights = v.chunkHeights[:0]
}

func (v *textCore) padChunkHeights() {
	for len(v.chunkHeights) < len(v.lineStarts) {
		v.chunkHeights = append(v.chunkHeights, 0)
	}
	if len(v.chunkHeights) > len(v.lineStarts) {
		v.chunkHeights = v.chunkHeights[:len(v.lineStarts)]
	}
}

func (v *textCore) padWrapPlans() {
	for len(v.wrapPlans) < len(v.lineStarts) {
		v.wrapPlans = append(v.wrapPlans, wrapPlan{})
	}
	if len(v.wrapPlans) > len(v.lineStarts) {
		v.wrapPlans = v.wrapPlans[:len(v.lineStarts)]
	}
}

func (v *textCore) invalidateWrapPlansFrom(line int) {
	if line < 0 {
		line = 0
	}
	for i := line; i < len(v.wrapPlans); i++ {
		v.wrapPlans[i].valid = false
	}
}

func (v *textCore) invalidateAllWrapPlans() {
	for i := range v.wrapPlans {
		v.wrapPlans[i].valid = false
	}
}

func (v *textCore) ensureWrapPlan(
	line, absStart, absEnd int,
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	gtx layout.Context,
	innerW, lineHeight int,
) *wrapPlan {
	p := &v.wrapPlans[line]
	if p.valid && p.width == innerW && p.lineH == lineHeight {
		return p
	}
	glyphs := widgets.ShapeChunkForWrap(shaper, fnt, size, gtx, v.text[absStart:absEnd], innerW)
	points := widgets.WrapLineStarts(glyphs)
	totalSubLines := len(points)
	if totalSubLines < 1 {
		totalSubLines = 1
	}

	if totalSubLines <= subLinesPerWrapChunk {
		p.starts = append(p.starts[:0], 0)
		p.subLines = append(p.subLines[:0], totalSubLines)
	} else {
		p.starts = p.starts[:0]
		p.subLines = p.subLines[:0]
		for i := 0; i < totalSubLines; i += subLinesPerWrapChunk {
			p.starts = append(p.starts, points[i])
			end := i + subLinesPerWrapChunk
			if end > totalSubLines {
				end = totalSubLines
			}
			p.subLines = append(p.subLines, end-i)
		}
	}
	p.height = totalSubLines * lineHeight
	p.width = innerW
	p.lineH = lineHeight
	p.valid = true
	return p
}

func (v *textCore) Text() string { return string(v.text) }

func (v *textCore) Bytes() []byte { return v.text }

func (v *textCore) Len() int { return len(v.text) }

func (v *textCore) Selection() (int, int) {
	return v.highlightStart, v.highlightEnd
}

func (v *textCore) SetSearchSpans(spans []matchSpan) { v.searchSpans = spans }

func (v *textCore) SetScrollCaret(bool) {}

func (v *textCore) GetScrollY() int { return v.scrollY }

func (v *textCore) SetScrollY(y int) {
	v.scrollY = y
	v.clampScroll()
}

func (v *textCore) GetScrollX() int { return v.scrollX }

func (v *textCore) SetScrollX(x int) {
	v.scrollX = x
	if v.scrollX < 0 {
		v.scrollX = 0
	}
}

func (v *textCore) GetMaxLineWidth() int { return v.maxLineWidth }

func (v *textCore) GetScrollBounds() image.Rectangle {
	if v.lastLineHeight == 0 {
		return image.Rectangle{}
	}
	totalH := v.lastTotalH
	if totalH <= 0 {
		totalH = len(v.lineStarts) * v.lastLineHeight
	}
	return image.Rectangle{Max: image.Point{Y: totalH}}
}

func (v *textCore) clampScroll() {
	if v.scrollY < 0 {
		v.scrollY = 0
	}
	if v.lastTotalH > 0 && v.lastViewportH > 0 {
		maxY := v.lastTotalH - v.lastViewportH
		if maxY < 0 {
			maxY = 0
		}
		if v.scrollY > maxY {
			v.scrollY = maxY
		}
	}
	if v.scrollX < 0 {
		v.scrollX = 0
	}
}

func (v *textCore) lineForByteOffset(off int) int {
	lo, hi := 0, len(v.lineStarts)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if v.lineStarts[mid] <= off {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return lo
}

func (v *textCore) rebuildLineStartsFrom(startIdx int) {
	for len(v.lineStarts) > 0 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *textCore) appendLineStartsFrom(startIdx int) {
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	for len(v.lineStarts) > 1 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *textCore) scanChunks(from int) {
	for i := from; i < len(v.text); i++ {
		if v.text[i] == '\n' && i+1 <= len(v.text) {
			v.lineStarts = append(v.lineStarts, i+1)
		}
	}
}

func (v *textCore) coordToByteOffset(
	gtx layout.Context,
	posX, posY int,
	advance fixed.Int26_6,
	exactLineH, viewportW int,
	wrap bool,
) int {
	if advance <= 0 || exactLineH <= 0 || len(v.lineStarts) == 0 {
		return 0
	}
	yDoc := posY + v.scrollY
	if yDoc < 0 {
		yDoc = 0
	}

	accum := 0
	chunkIdx := len(v.chunkHeights) - 1
	for i, h := range v.chunkHeights {
		if h <= 0 {
			h = v.estimateChunkHeight(i, exactLineH, advance, viewportW, wrap)
		}
		if accum+h > yDoc {
			chunkIdx = i
			break
		}
		accum += h
	}
	if chunkIdx < 0 || chunkIdx >= len(v.lineStarts) {
		return len(v.text)
	}
	chunkStart, chunkEnd := v.lineBounds(chunkIdx)
	chunkText := v.text[chunkStart:chunkEnd]

	if !wrap {
		chunkRunes := utf8.RuneCount(chunkText)
		col := int(fixed.I(posX+v.scrollX) / advance)
		if col < 0 {
			col = 0
		}
		if col > chunkRunes {
			col = chunkRunes
		}
		return chunkStart + runeIdxToByte(chunkText, col)
	}

	yWithin := yDoc - accum
	if yWithin < 0 {
		yWithin = 0
	}
	wrapLine := yWithin / exactLineH
	clickX := posX
	if clickX < 0 {
		clickX = 0
	}
	glyphs := widgets.ShapeChunkForWrap(v.layoutShaper, v.layoutFont, v.layoutSize, gtx, chunkText, viewportW)
	return chunkStart + widgets.ByteOffInWrap(glyphs, clickX, wrapLine)
}

func (v *textCore) estimateChunkHeight(line, lineHeight int, advance fixed.Int26_6, viewportW int, wrap bool) int {
	if !wrap || advance <= 0 || viewportW <= 0 {
		return lineHeight
	}
	if line < 0 || line >= len(v.lineStarts) {
		return lineHeight
	}
	start := v.lineStarts[line]
	var end int
	if line+1 < len(v.lineStarts) {
		end = v.lineStarts[line+1]
	} else {
		end = len(v.text)
	}
	if end <= start {
		return lineHeight
	}
	chunkRunes := utf8.RuneCount(v.text[start:end])
	if chunkRunes <= 0 {
		return lineHeight
	}
	charsPerLine := charsPerLineFor(viewportW, advance)
	subLines := (chunkRunes + charsPerLine - 1) / charsPerLine
	if subLines < 1 {
		subLines = 1
	}
	return subLines * lineHeight
}

func (v *textCore) firstChunkAtFn(y, lineH int, advance fixed.Int26_6, viewportW int, wrap bool) (int, int) {
	if y <= 0 {
		return 0, 0
	}
	accum := 0
	for i, h := range v.chunkHeights {
		if h <= 0 {
			h = v.estimateChunkHeight(i, lineH, advance, viewportW, wrap)
		}
		if accum+h > y {
			return i, accum
		}
		accum += h
	}
	return len(v.chunkHeights), accum
}

func (v *textCore) lineBounds(n int) (int, int) {
	start := v.lineStarts[n]
	var end int
	if n+1 < len(v.lineStarts) {
		end = v.lineStarts[n+1]
	} else {
		end = len(v.text)
	}
	if end > start && v.text[end-1] == '\n' {
		end--
	}
	if end > start && v.text[end-1] == '\r' {
		end--
	}
	return start, end
}

func (v *textCore) wordBoundsAt(byteOff int) (int, int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff >= len(v.text) {
		byteOff = len(v.text)
		if byteOff == 0 {
			return 0, 0
		}
		prev, _ := utf8.DecodeLastRune(v.text[:byteOff])
		if widgets.IsSeparator(prev) {
			return byteOff, byteOff
		}
		start := byteOff
		for start > 0 {
			r, sz := utf8.DecodeLastRune(v.text[:start])
			if widgets.IsSeparator(r) {
				break
			}
			start -= sz
		}
		return start, byteOff
	}
	r, sz := utf8.DecodeRune(v.text[byteOff:])

	if widgets.IsSeparator(r) {
		if unicode.IsSpace(r) {
			start := byteOff
			for start > 0 {
				prev, psz := utf8.DecodeLastRune(v.text[:start])
				if !unicode.IsSpace(prev) {
					break
				}
				start -= psz
			}
			end := byteOff
			for end < len(v.text) {
				next, nsz := utf8.DecodeRune(v.text[end:])
				if !unicode.IsSpace(next) {
					break
				}
				end += nsz
			}
			return start, end
		}
		return byteOff, byteOff + sz
	}

	start := byteOff
	for start > 0 {
		prev, psz := utf8.DecodeLastRune(v.text[:start])
		if widgets.IsSeparator(prev) {
			break
		}
		start -= psz
	}
	end := byteOff
	for end < len(v.text) {
		next, nsz := utf8.DecodeRune(v.text[end:])
		if widgets.IsSeparator(next) {
			break
		}
		end += nsz
	}
	return start, end
}

func (v *textCore) sourceLineBoundsAt(byteOff int) (int, int) {
	if byteOff < 0 {
		byteOff = 0
	}
	if byteOff > len(v.text) {
		byteOff = len(v.text)
	}
	start := byteOff
	for start > 0 && v.text[start-1] != '\n' {
		start--
	}
	end := byteOff
	for end < len(v.text) && v.text[end] != '\n' {
		end++
	}
	if end > start && v.text[end-1] == '\r' {
		end--
	}
	return start, end
}

func (v *textCore) SelectAll() {
	v.selStart = 0
	v.selEnd = len(v.text)
	v.dragActive = false
}

func (v *textCore) moveCaret(newPos int, extend bool) {
	if newPos < 0 {
		newPos = 0
	}
	if newPos > len(v.text) {
		newPos = len(v.text)
	}
	if extend {
		v.selEnd = newPos
	} else {
		v.selStart = newPos
		v.selEnd = newPos
	}
	v.dragActive = false
}

func (v *textCore) charLeft(off int) int {
	if off <= 0 {
		return 0
	}
	_, sz := utf8.DecodeLastRune(v.text[:off])
	return off - sz
}

func (v *textCore) charRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	_, sz := utf8.DecodeRune(v.text[off:])
	return off + sz
}

func (v *textCore) wordLeft(off int) int {
	if off <= 0 {
		return 0
	}
	i := off
	for i > 0 {
		r, sz := utf8.DecodeLastRune(v.text[:i])
		if !widgets.IsSeparator(r) {
			break
		}
		i -= sz
	}
	for i > 0 {
		r, sz := utf8.DecodeLastRune(v.text[:i])
		if widgets.IsSeparator(r) {
			break
		}
		i -= sz
	}
	return i
}

func (v *textCore) wordRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	i := off
	for i < len(v.text) {
		r, sz := utf8.DecodeRune(v.text[i:])
		if widgets.IsSeparator(r) {
			break
		}
		i += sz
	}
	for i < len(v.text) {
		r, sz := utf8.DecodeRune(v.text[i:])
		if !widgets.IsSeparator(r) {
			break
		}
		i += sz
	}
	return i
}

func (v *textCore) columnAt(off int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if off <= lineStart {
		return 0
	}
	return utf8.RuneCount(v.text[lineStart:off])
}

func (v *textCore) offsetAtColumn(lineStart, col int) int {
	_, lineEnd := v.sourceLineBoundsAt(lineStart)
	if col <= 0 {
		return lineStart
	}
	off := lineStart
	runes := 0
	for off < lineEnd && runes < col {
		_, sz := utf8.DecodeRune(v.text[off:lineEnd])
		off += sz
		runes++
	}
	return off
}

func (v *textCore) lineUp(off, col int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if lineStart == 0 {
		return 0
	}
	prevLineStart, _ := v.sourceLineBoundsAt(lineStart - 1)
	return v.offsetAtColumn(prevLineStart, col)
}

func (v *textCore) lineDown(off, col int) int {
	_, lineEnd := v.sourceLineBoundsAt(off)
	nextLineStart := lineEnd
	if nextLineStart < len(v.text) && v.text[nextLineStart] == '\r' {
		nextLineStart++
	}
	if nextLineStart < len(v.text) && v.text[nextLineStart] == '\n' {
		nextLineStart++
	}
	if nextLineStart >= len(v.text) {
		return len(v.text)
	}
	return v.offsetAtColumn(nextLineStart, col)
}

func (v *textCore) visualXAt(off int, gtx layout.Context, viewportW int) int {
	line := v.lineForByteOffset(off)
	chunkStart, chunkEnd := v.lineBounds(line)
	chunkText := v.text[chunkStart:chunkEnd]
	inChunkByte := off - chunkStart
	if inChunkByte < 0 {
		inChunkByte = 0
	}
	glyphs := widgets.ShapeChunkForWrap(v.layoutShaper, v.layoutFont, v.layoutSize, gtx, chunkText, viewportW)
	x, _ := widgets.CaretXYInWrap(glyphs, inChunkByte)
	return x
}

func (v *textCore) wrapLineMoveX(off, prefX, dir int, gtx layout.Context, viewportW int) int {
	line := v.lineForByteOffset(off)
	chunkStart, chunkEnd := v.lineBounds(line)
	chunkText := v.text[chunkStart:chunkEnd]
	glyphs := widgets.ShapeChunkForWrap(v.layoutShaper, v.layoutFont, v.layoutSize, gtx, chunkText, viewportW)
	_, subLine := widgets.CaretXYInWrap(glyphs, off-chunkStart)
	maxSub := widgets.WrapMaxLine(glyphs)

	if dir < 0 {
		if subLine > 0 {
			return chunkStart + widgets.ByteOffInWrap(glyphs, prefX, subLine-1)
		}
		if line == 0 {
			return 0
		}
		prevStart, prevEnd := v.lineBounds(line - 1)
		prevText := v.text[prevStart:prevEnd]
		prevGlyphs := widgets.ShapeChunkForWrap(v.layoutShaper, v.layoutFont, v.layoutSize, gtx, prevText, viewportW)
		lastSub := widgets.WrapMaxLine(prevGlyphs)
		return prevStart + widgets.ByteOffInWrap(prevGlyphs, prefX, lastSub)
	}
	if subLine < maxSub {
		return chunkStart + widgets.ByteOffInWrap(glyphs, prefX, subLine+1)
	}
	if line+1 >= len(v.lineStarts) {
		return len(v.text)
	}
	nextStart, nextEnd := v.lineBounds(line + 1)
	nextText := v.text[nextStart:nextEnd]
	nextGlyphs := widgets.ShapeChunkForWrap(v.layoutShaper, v.layoutFont, v.layoutSize, gtx, nextText, viewportW)
	return nextStart + widgets.ByteOffInWrap(nextGlyphs, prefX, 0)
}

func (v *textCore) ensureCaretVisible() {
	if v.lastLineHeight == 0 {
		return
	}
	line := v.lineForByteOffset(v.selEnd)
	caretY := 0
	for i := 0; i < line; i++ {
		if i < len(v.chunkHeights) && v.chunkHeights[i] > 0 {
			caretY += v.chunkHeights[i]
		} else {
			caretY += v.lastLineHeight
		}
	}
	chunkH := v.lastLineHeight
	if line < len(v.chunkHeights) && v.chunkHeights[line] > 0 {
		chunkH = v.chunkHeights[line]
	}
	if caretY < v.scrollY {
		v.scrollY = caretY
	} else if v.lastViewportH > 0 && caretY+chunkH > v.scrollY+v.lastViewportH {
		v.scrollY = caretY + chunkH - v.lastViewportH
	}
	v.clampScroll()
}

func (v *textCore) paintHighlight(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	chunkH, yOff int,
	advance fixed.Int26_6,
	wrap bool,
	viewportW int,
	col color.NRGBA,
	rangeStart, rangeEnd int,
	glyphs []widgets.WrapGlyph,
) {
	if advance <= 0 {
		return
	}
	descPad := v.descOvershoot
	hStartByte := rangeStart - chunkStart
	if hStartByte < 0 {
		hStartByte = 0
	}
	maxEndByte := chunkEnd - chunkStart
	hEndByte := rangeEnd - chunkStart
	if hEndByte > maxEndByte {
		hEndByte = maxEndByte
	}
	if hEndByte <= hStartByte {
		return
	}
	continuesPastChunk := rangeEnd > chunkEnd

	if !wrap {
		chunkText := v.text[chunkStart:chunkEnd]
		hStart := byteToRuneIdx(chunkText, hStartByte)
		hEnd := byteToRuneIdx(chunkText, hEndByte)
		if hEnd <= hStart {
			return
		}
		colToPx := func(c int) int {
			return (advance * fixed.Int26_6(c)).Round()
		}
		x1 := colToPx(hStart) - v.scrollX
		x2 := colToPx(hEnd) - v.scrollX
		cellH := chunkH
		if v.lineBox > cellH {
			cellH = v.lineBox
		}
		bottom := yOff + cellH
		if !continuesPastChunk {
			bottom += descPad
		}
		r := image.Rect(x1, yOff, x2, bottom)
		paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
		return
	}

	startX, startWL := widgets.CaretXYInWrap(glyphs, hStartByte)
	endX, endWL := widgets.CaretXYInWrap(glyphs, hEndByte)
	if endWL < startWL || (endWL == startWL && endX <= startX) {
		return
	}

	subLineH := v.lastLineHeight
	if subLineH < 1 {
		return
	}
	chunkBottom := yOff + chunkH
	fullWidth := viewportW

	for wl := startWL; wl <= endWL; wl++ {
		y1 := yOff + wl*subLineH
		if y1 >= chunkBottom {
			break
		}
		y2 := y1 + subLineH
		if wl == endWL {
			isChunkLastSubLine := y1+2*subLineH > chunkBottom
			if continuesPastChunk || isChunkLastSubLine {
				y2 = chunkBottom
			}
		}
		if y2 > chunkBottom {
			y2 = chunkBottom
		}
		x1 := 0
		x2 := fullWidth
		if wl == startWL {
			x1 = startX
		}
		if wl == endWL {
			x2 = endX
		}
		if wl == endWL && !continuesPastChunk {
			if box := y1 + v.lineBox; box > y2 {
				y2 = box
			}
			y2 += descPad
		}
		r := image.Rect(x1, y1, x2, y2)
		paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
	}
}
