package workspace

import (
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/pointer"
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
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/pkg/syntax"
	"unicode"
	"unicode/utf8"
)

const scrollbarFadeDur = 100 * time.Millisecond

type textCore struct {
	text       []byte
	lineStarts []int

	chunkHeights      []int
	chunkHeightsWrap  bool
	chunkHeightsWidth int

	wrapPlans []*wrapPlan

	scrollY int
	scrollX int

	maxLineWidth int

	highlightStart int
	highlightEnd   int

	searchSpans []matchSpan

	revealStart   int
	revealEnd     int
	revealPending bool
	revealInset   int
	revealY       int
	revealYValid  bool

	selStart   int
	selEnd     int
	dragActive bool

	lastClickTime   time.Time
	lastClickEvTime time.Duration
	lastClickPos    image.Point
	multiClickN     int

	Scroller  gesture.Scroll
	ScrollerH gesture.Scroll
	Drag      gesture.Drag
	Click     gesture.Click

	scrollbarHover widgets.Hover
	scrollbarFade  widgets.Fade

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

	noWrapCache  colCache
	wrapScratch  []int
	paintScratch []widgets.WrapGlyph
	hitScratch   []widgets.WrapGlyph
	monoAdvance  fixed.Int26_6
}

type colCache struct {
	start   int
	end     int
	textLen int
	runes   int
	col     int
	byteOff int
	valid   bool
}

func colPx(advance fixed.Int26_6, col int) int {
	return int((int64(advance)*int64(col) + 32) >> 6)
}

func colAtPx(advance fixed.Int26_6, px int) int {
	if advance <= 0 {
		return 0
	}
	return int(int64(px) << 6 / int64(advance))
}

func (v *textCore) noWrapPaintWindow(
	chunkStart, chunkEnd, innerW int,
	advance fixed.Int26_6,
) (paintStart, paintEnd, xOff, totalCols int) {
	if advance <= 0 || innerW <= 0 || chunkEnd-chunkStart <= longLineThresholdBytes {
		return chunkStart, chunkEnd, 0, 0
	}
	c := &v.noWrapCache
	if !c.valid || c.start != chunkStart || c.end != chunkEnd || c.textLen != len(v.text) {
		*c = colCache{
			start:   chunkStart,
			end:     chunkEnd,
			textLen: len(v.text),
			runes:   utf8.RuneCount(v.text[chunkStart:chunkEnd]),
			valid:   true,
		}
	}
	txt := v.text[chunkStart:chunkEnd]
	firstCol := colAtPx(advance, v.scrollX)
	if firstCol > c.runes {
		firstCol = c.runes
	}
	lastCol := firstCol + colAtPx(advance, innerW) + 2
	if lastCol > c.runes {
		lastCol = c.runes
	}
	if firstCol < c.col {
		c.col, c.byteOff = 0, 0
	}
	off, n := c.byteOff, c.col
	for n < firstCol && off < len(txt) {
		_, sz := utf8.DecodeRune(txt[off:])
		if sz < 1 {
			sz = 1
		}
		off += sz
		n++
	}
	c.col, c.byteOff = n, off
	end, m := off, n
	for m < lastCol && end < len(txt) {
		_, sz := utf8.DecodeRune(txt[end:])
		if sz < 1 {
			sz = 1
		}
		end += sz
		m++
	}
	return chunkStart + off, chunkStart + end, colPx(advance, firstCol), c.runes
}

func (v *textCore) needsChunkGlyphs(chunkStart, chunkEnd int, hasHL, hasSel bool) bool {
	if hasHL && v.highlightEnd > chunkStart && v.highlightStart < chunkEnd {
		return true
	}
	if hasSel {
		s, e := v.selStart, v.selEnd
		if s > e {
			s, e = e, s
		}
		if e > chunkStart && s < chunkEnd {
			return true
		}
	}
	if len(v.searchSpans) > 0 {
		i := sort.Search(len(v.searchSpans), func(i int) bool { return v.searchSpans[i].end > chunkStart })
		if i < len(v.searchSpans) && v.searchSpans[i].start < chunkEnd {
			return true
		}
	}
	return false
}

func (v *textCore) spansForChunk(chunkStart, chunkEnd int, sp theme.SyntaxPalette, bracketCycle bool) []widgets.ColoredSpan {
	if len(v.tokens) == 0 || chunkStart >= chunkEnd {
		return nil
	}
	first := sort.Search(len(v.tokens), func(i int) bool {
		return int(v.tokens[i].End()) > chunkStart
	})
	if first >= len(v.tokens) || int(v.tokens[first].Start) >= chunkEnd {
		return nil
	}
	out := make([]widgets.ColoredSpan, 0, 16)
	for i := first; i < len(v.tokens); i++ {
		t := v.tokens[i]
		if int(t.Start) >= chunkEnd {
			break
		}
		s, e := int(t.Start), int(t.End())
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

// tokenColorAt reports the colour the glyphs at off are painted in, so a search
// highlight can be derived from the text it covers instead of from the accent.
func (v *textCore) tokenColorAt(off int, sp theme.SyntaxPalette, bracketCycle bool, def color.NRGBA) color.NRGBA {
	if len(v.tokens) == 0 {
		return def
	}
	i := sort.Search(len(v.tokens), func(i int) bool { return int(v.tokens[i].End()) > off })
	if i < len(v.tokens) && int(v.tokens[i].Start) <= off {
		return sp.ColorForToken(v.tokens[i].Kind, v.tokens[i].Depth, bracketCycle)
	}
	return def
}

func clipSpansToVars(spans []widgets.ColoredSpan, chunk []byte) []widgets.ColoredSpan {
	var vars []matchSpan
	for idx := 0; idx < len(chunk); {
		s := bytesIndex(chunk[idx:], "{{")
		if s == -1 {
			break
		}
		s += idx
		rel := bytesIndex(chunk[s+2:], "}}")
		if rel == -1 {
			break
		}
		e := s + 2 + rel + 2
		vars = append(vars, matchSpan{start: s, end: e})
		idx = e
	}
	if len(vars) == 0 {
		return spans
	}
	res := make([]widgets.ColoredSpan, 0, len(spans)+len(vars))
	for _, span := range spans {
		segStart := span.Start
		for _, vr := range vars {
			if vr.end <= segStart || vr.start >= span.End {
				continue
			}
			if vr.start > segStart {
				res = append(res, widgets.ColoredSpan{Start: segStart, End: vr.start, Color: span.Color})
			}
			if vr.end > segStart {
				segStart = vr.end
			}
		}
		if segStart < span.End {
			res = append(res, widgets.ColoredSpan{Start: segStart, End: span.End, Color: span.Color})
		}
	}
	return res
}

func (v *textCore) resolveClickCount(now time.Time, evTime time.Duration, pos image.Point) int {
	const interval = 500 * time.Millisecond
	const slop = 5
	dx := pos.X - v.lastClickPos.X
	dy := pos.Y - v.lastClickPos.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	var within bool
	if evTime != 0 && v.lastClickEvTime != 0 {
		d := evTime - v.lastClickEvTime
		within = d >= 0 && d <= interval
	} else {
		within = !v.lastClickTime.IsZero() && now.Sub(v.lastClickTime) <= interval
	}
	if within && dx <= slop && dy <= slop {
		v.multiClickN++
	} else {
		v.multiClickN = 1
	}
	v.lastClickTime = now
	v.lastClickEvTime = evTime
	v.lastClickPos = pos
	return v.multiClickN
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
		v.wrapPlans = append(v.wrapPlans, nil)
	}
	if len(v.wrapPlans) > len(v.lineStarts) {
		v.wrapPlans = v.wrapPlans[:len(v.lineStarts)]
	}
}

// invalidateWrapPlansFrom drops plans whose content may have changed, so it
// also resets the mono-scan progress; invalidateAllWrapPlans is for geometry
// changes and keeps it.
func (v *textCore) invalidateWrapPlansFrom(line int) {
	if line < 0 {
		line = 0
	}
	for i := line; i < len(v.wrapPlans); i++ {
		if v.wrapPlans[i] != nil {
			v.wrapPlans[i].valid = false
			v.wrapPlans[i].scanned = 0
		}
	}
}

func (v *textCore) invalidateAllWrapPlans() {
	for i := range v.wrapPlans {
		if v.wrapPlans[i] != nil {
			v.wrapPlans[i].valid = false
		}
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
	if v.wrapPlans[line] == nil {
		v.wrapPlans[line] = &wrapPlan{}
	}
	p := v.wrapPlans[line]
	span := absEnd - absStart
	reusable := p.valid && p.width == innerW && p.lineH == lineHeight
	if reusable && p.covered == span {
		return p
	}

	if cpl := v.monoColumns(p, absStart, absEnd, innerW); cpl > 0 {
		buildMonoPlan(p, span, cpl, lineHeight)
		p.width = innerW
		p.lineH = lineHeight
		p.valid = true
		return p
	}

	totalSubLines := 0
	pending := 0
	pos := absStart
	if reusable && p.covered < span && len(p.starts) > 0 {
		last := len(p.starts) - 1
		pos = absStart + p.starts[last]
		totalSubLines = p.subTotal - p.subLines[last]
		p.starts = p.starts[:last]
		p.subLines = p.subLines[:last]
	} else {
		p.starts = p.starts[:0]
		p.subLines = p.subLines[:0]
	}

	for pos < absEnd {
		winEnd := pos + wrapShapeWindowBytes
		if winEnd >= absEnd {
			winEnd = absEnd
		} else {
			winEnd = runeBoundaryAt(v.text, winEnd)
		}
		points := widgets.WrapLineStartsFor(shaper, fnt, size, gtx, v.text[pos:winEnd], innerW, v.wrapScratch)
		v.wrapScratch = points

		emit := len(points)
		nextPos := winEnd
		if winEnd < absEnd && emit > 1 {
			if adv := pos + points[emit-1]; adv > pos {
				emit--
				nextPos = adv
			}
		}
		for i := 0; i < emit; i++ {
			if pending == 0 {
				p.starts = append(p.starts, pos+points[i]-absStart)
				p.subLines = append(p.subLines, 0)
			}
			pending++
			p.subLines[len(p.subLines)-1] = pending
			totalSubLines++
			if pending == subLinesPerWrapChunk {
				pending = 0
			}
		}
		if nextPos <= pos {
			break
		}
		pos = nextPos
	}

	if totalSubLines < 1 {
		totalSubLines = 1
		p.starts = append(p.starts[:0], 0)
		p.subLines = append(p.subLines[:0], 1)
	}
	p.covered = span
	p.subTotal = totalSubLines
	p.height = totalSubLines * lineHeight
	p.width = innerW
	p.lineH = lineHeight
	p.valid = true
	return p
}

// monoColumns reports the fixed column count a line of this span wraps into,
// or 0 when the exact shaper has to run. Column arithmetic reproduces the
// shaper's grapheme wrapping only for a monospaced face over printable ASCII
// with no spaces — gio trims trailing whitespace at a break, which shifts the
// following line. That restriction still covers what makes wrapping expensive:
// multi-megabyte minified payloads on one logical line.
func (v *textCore) monoColumns(p *wrapPlan, absStart, absEnd, innerW int) int {
	if v.monoAdvance <= 0 || innerW <= 0 {
		return 0
	}
	span := absEnd - absStart
	if span < p.scanned {
		p.scanned = 0
		p.mono = true
	}
	if p.scanned == 0 {
		p.mono = true
	}
	if p.mono && p.scanned < span {
		if !plainASCIIRun(v.text[absStart+p.scanned : absEnd]) {
			p.mono = false
		}
		p.scanned = span
	}
	if !p.mono {
		return 0
	}
	return charsPerLineFor(innerW, v.monoAdvance)
}

func plainASCIIRun(b []byte) bool {
	for _, c := range b {
		if c <= ' ' || c >= 0x7F {
			return false
		}
	}
	return true
}

func buildMonoPlan(p *wrapPlan, span, charsPerLine, lineHeight int) {
	total := (span + charsPerLine - 1) / charsPerLine
	if total < 1 {
		total = 1
	}
	p.starts = p.starts[:0]
	p.subLines = p.subLines[:0]
	for i := 0; i < total; i += subLinesPerWrapChunk {
		n := subLinesPerWrapChunk
		if i+n > total {
			n = total - i
		}
		p.starts = append(p.starts, i*charsPerLine)
		p.subLines = append(p.subLines, n)
	}
	p.covered = span
	p.subTotal = total
	p.height = total * lineHeight
}

func measureMonoAdvance(shaper *text.Shaper, fnt font.Font, size unit.Sp, gtx layout.Context) fixed.Int26_6 {
	var want fixed.Int26_6
	for i, s := range [...]string{"M", "i", "W", "0", ".", "|"} {
		shaper.LayoutString(text.Parameters{
			Font:    fnt,
			PxPerEm: fixed.I(gtx.Sp(size)),
			Locale:  gtx.Locale,
		}, s)
		g, ok := shaper.NextGlyph()
		if !ok {
			return 0
		}
		if i == 0 {
			want = g.Advance
		} else if g.Advance != want {
			return 0
		}
	}
	return want
}

func runeBoundaryAt(text []byte, i int) int {
	if i >= len(text) {
		return len(text)
	}
	for i > 0 && !utf8.RuneStart(text[i]) {
		i--
	}
	return i
}

func (v *textCore) wrapPlanFor(line, chunkStart, chunkEnd int, gtx layout.Context, viewportW, lineH int) *wrapPlan {
	if chunkEnd-chunkStart < longLineThresholdBytes || line < 0 || line >= len(v.wrapPlans) || lineH <= 0 {
		return nil
	}
	p := v.ensureWrapPlan(line, chunkStart, chunkEnd, v.layoutShaper, v.layoutFont, v.layoutSize, gtx, viewportW, lineH)
	if len(p.starts) == 0 {
		return nil
	}
	return p
}

func planSubForWrapLine(p *wrapPlan, wrapLine int) (int, int) {
	sub0 := 0
	for i, n := range p.subLines {
		if wrapLine < sub0+n {
			return i, sub0
		}
		if i < len(p.subLines)-1 {
			sub0 += n
		}
	}
	return len(p.starts) - 1, sub0
}

func planSubForByte(p *wrapPlan, rel int) (int, int) {
	subIdx := 0
	for i := 1; i < len(p.starts); i++ {
		if p.starts[i] > rel {
			break
		}
		subIdx = i
	}
	sub0 := 0
	for i := 0; i < subIdx; i++ {
		sub0 += p.subLines[i]
	}
	return subIdx, sub0
}

func planSubBounds(p *wrapPlan, subIdx, chunkStart, chunkEnd int) (int, int) {
	subStart := chunkStart + p.starts[subIdx]
	subEnd := chunkEnd
	if subIdx+1 < len(p.starts) {
		subEnd = chunkStart + p.starts[subIdx+1]
	}
	return subStart, subEnd
}

func planTotalSubLines(p *wrapPlan) int {
	if p.subTotal < 1 {
		return 1
	}
	return p.subTotal
}

func (v *textCore) wrapCaretXY(line, chunkStart, chunkEnd, off int, gtx layout.Context, viewportW int) (int, int) {
	rel := off - chunkStart
	if rel < 0 {
		rel = 0
	}
	if plan := v.wrapPlanFor(line, chunkStart, chunkEnd, gtx, viewportW, v.lastLineHeight); plan != nil {
		subIdx, sub0 := planSubForByte(plan, rel)
		subStart, subEnd := planSubBounds(plan, subIdx, chunkStart, chunkEnd)
		glyphs := v.shapeChunkScratch(gtx, v.text[subStart:subEnd], viewportW)
		x, sl := widgets.CaretXYInWrap(glyphs, rel-plan.starts[subIdx])
		return x, sub0 + sl
	}
	glyphs := v.shapeChunkScratch(gtx, v.text[chunkStart:chunkEnd], viewportW)
	return widgets.CaretXYInWrap(glyphs, rel)
}

func (v *textCore) wrapByteAt(line, chunkStart, chunkEnd, prefX, wrapLine int, gtx layout.Context, viewportW int) int {
	if plan := v.wrapPlanFor(line, chunkStart, chunkEnd, gtx, viewportW, v.lastLineHeight); plan != nil {
		subIdx, sub0 := planSubForWrapLine(plan, wrapLine)
		subStart, subEnd := planSubBounds(plan, subIdx, chunkStart, chunkEnd)
		glyphs := v.shapeChunkScratch(gtx, v.text[subStart:subEnd], viewportW)
		return subStart + widgets.ByteOffInWrap(glyphs, prefX, wrapLine-sub0)
	}
	glyphs := v.shapeChunkScratch(gtx, v.text[chunkStart:chunkEnd], viewportW)
	return chunkStart + widgets.ByteOffInWrap(glyphs, prefX, wrapLine)
}

func (v *textCore) wrapMaxLineOf(line, chunkStart, chunkEnd int, gtx layout.Context, viewportW int) int {
	if plan := v.wrapPlanFor(line, chunkStart, chunkEnd, gtx, viewportW, v.lastLineHeight); plan != nil {
		return planTotalSubLines(plan) - 1
	}
	glyphs := v.shapeChunkScratch(gtx, v.text[chunkStart:chunkEnd], viewportW)
	return widgets.WrapMaxLine(glyphs)
}

// shapeChunkScratch shapes chunkText into the hit-test glyph buffer. The
// result is only valid until the next shapeChunkScratch call on the same
// textCore; painting uses paintScratch and is unaffected.
func (v *textCore) shapeChunkScratch(gtx layout.Context, chunkText []byte, viewportW int) []widgets.WrapGlyph {
	v.hitScratch = widgets.ShapeChunkForWrapInto(v.hitScratch, v.layoutShaper, v.layoutFont, v.layoutSize, gtx, chunkText, viewportW)
	return v.hitScratch
}

func (v *textCore) Text() string { return string(v.text) }

func (v *textCore) Bytes() []byte { return v.text }

func (v *textCore) Len() int { return len(v.text) }

func (v *textCore) Selection() (int, int) {
	return v.highlightStart, v.highlightEnd
}

func (v *textCore) SetSearchSpans(spans []matchSpan) { v.searchSpans = spans }

func (v *textCore) SetScrollCaret(bool) {}

func (v *textCore) LayoutScrollbarHover(gtx layout.Context, keepVisible bool) layout.Dimensions {
	on := v.scrollbarHover.Update(gtx.Source) || keepVisible
	v.scrollbarFade.Update(gtx, on, scrollbarFadeDur)
	size := gtx.Constraints.Max
	if size.X > 0 && size.Y > 0 {
		pass := pointer.PassOp{}.Push(gtx.Ops)
		cl := clip.Rect{Max: size}.Push(gtx.Ops)
		v.scrollbarHover.Add(gtx.Ops)
		cl.Pop()
		pass.Pop()
	}
	return layout.Dimensions{}
}

func (v *textCore) ScrollbarFade() float32 { return v.scrollbarFade.Value() }

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

func (v *textCore) scrollAnchor(lineH int, adv fixed.Int26_6, width int, wrap bool) (int, int) {
	if lineH <= 0 || v.scrollY <= 0 {
		return 0, 0
	}
	accum := 0
	for i := range v.chunkHeights {
		h := v.chunkHeights[i]
		if h <= 0 {
			h = v.estimateChunkHeight(i, lineH, adv, width, wrap)
		}
		if accum+h > v.scrollY {
			return i, (v.scrollY - accum) / lineH
		}
		accum += h
	}
	return len(v.chunkHeights), 0
}

func (v *textCore) scrollYForAnchor(line, sub, lineH int, adv fixed.Int26_6, width int, wrap bool) int {
	if lineH <= 0 {
		return v.scrollY
	}
	y := 0
	for i := 0; i < line && i < len(v.chunkHeights); i++ {
		h := v.chunkHeights[i]
		if h <= 0 {
			h = v.estimateChunkHeight(i, lineH, adv, width, wrap)
		}
		y += h
	}
	return y + sub*lineH
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
		col := int((fixed.I(posX+v.scrollX) + advance/2) / advance)
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
	if plan := v.wrapPlanFor(chunkIdx, chunkStart, chunkEnd, gtx, viewportW, exactLineH); plan != nil {
		subIdx, sub0 := planSubForWrapLine(plan, wrapLine)
		subStart, subEnd := planSubBounds(plan, subIdx, chunkStart, chunkEnd)
		glyphs := v.shapeChunkScratch(gtx, v.text[subStart:subEnd], viewportW)
		return subStart + widgets.ByteOffInWrap(glyphs, clickX, wrapLine-sub0)
	}
	glyphs := v.shapeChunkScratch(gtx, chunkText, viewportW)
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
	x, _ := v.wrapCaretXY(line, chunkStart, chunkEnd, off, gtx, viewportW)
	return x
}

func (v *textCore) wrapLineMoveX(off, prefX, dir int, gtx layout.Context, viewportW int) int {
	line := v.lineForByteOffset(off)
	chunkStart, chunkEnd := v.lineBounds(line)
	_, subLine := v.wrapCaretXY(line, chunkStart, chunkEnd, off, gtx, viewportW)
	maxSub := v.wrapMaxLineOf(line, chunkStart, chunkEnd, gtx, viewportW)

	if dir < 0 {
		if subLine > 0 {
			return v.wrapByteAt(line, chunkStart, chunkEnd, prefX, subLine-1, gtx, viewportW)
		}
		if line == 0 {
			return 0
		}
		prevStart, prevEnd := v.lineBounds(line - 1)
		lastSub := v.wrapMaxLineOf(line-1, prevStart, prevEnd, gtx, viewportW)
		return v.wrapByteAt(line-1, prevStart, prevEnd, prefX, lastSub, gtx, viewportW)
	}
	if subLine < maxSub {
		return v.wrapByteAt(line, chunkStart, chunkEnd, prefX, subLine+1, gtx, viewportW)
	}
	if line+1 >= len(v.lineStarts) {
		return len(v.text)
	}
	nextStart, nextEnd := v.lineBounds(line + 1)
	return v.wrapByteAt(line+1, nextStart, nextEnd, prefX, 0, gtx, viewportW)
}

// requestReveal defers scrolling [start,end) into view until the next layout,
// where the wrap plan, line height and viewport extent are all known. Resolving
// it earlier would have to guess the sub-line a wrapped match sits on and the
// column it starts at, which is what made search jump to the wrong place.
func (v *textCore) requestReveal(start, end int) {
	if end < start {
		start, end = end, start
	}
	v.revealStart = start
	v.revealEnd = end
	v.revealPending = true
}

func (v *textCore) cancelReveal() {
	v.revealPending = false
	v.revealYValid = false
}

// SetRevealInset reserves a band at the top and bottom of the viewport that the
// search panel floats over, so a match sitting under the panel still counts as
// needing to be scrolled into the clear.
func (v *textCore) SetRevealInset(px int) { v.revealInset = px }

// ClearSearchCaret drops the current match's highlight without moving the
// viewport. Closing the search box has to call it, otherwise the last match
// stays visibly selected in a viewer that no longer has a search in it.
func (v *textCore) ClearSearchCaret() {
	hs, he := v.highlightStart, v.highlightEnd
	v.highlightStart, v.highlightEnd = 0, 0
	if he > hs && v.selStart == hs && v.selEnd == he {
		v.selStart, v.selEnd = 0, 0
	}
	v.cancelReveal()
}

// RevealScreenY reports where the last revealed match landed, measured from the
// top of the viewport. The search panel uses it to step out of the way when the
// document is too short to scroll the match clear of it.
func (v *textCore) RevealScreenY() (int, bool) { return v.revealY, v.revealYValid }

func (v *textCore) applyReveal(gtx layout.Context, advance fixed.Int26_6, lineH, innerW, innerH int, wrap bool) {
	if !v.revealPending || lineH <= 0 || len(v.lineStarts) == 0 {
		return
	}
	v.revealPending = false

	start, end := v.revealStart, v.revealEnd
	if start > len(v.text) {
		start = len(v.text)
	}
	if end > len(v.text) {
		end = len(v.text)
	}

	line := v.lineForByteOffset(start)
	top := 0
	for i := 0; i < line && i < len(v.chunkHeights); i++ {
		h := v.chunkHeights[i]
		if h <= 0 {
			h = v.estimateChunkHeight(i, lineH, advance, innerW, wrap)
		}
		top += h
	}

	chunkStart, chunkEnd := v.lineBounds(line)
	if wrap {
		_, sub := v.wrapCaretXY(line, chunkStart, chunkEnd, start, gtx, innerW)
		top += sub * lineH
	}

	inset := v.revealInset
	if max := (innerH - lineH) / 3; inset > max {
		inset = max
	}
	if inset < 0 {
		inset = 0
	}
	if innerH <= 0 {
		v.scrollY = top
	} else if top < v.scrollY+inset || top+lineH > v.scrollY+innerH-inset {
		v.scrollY = top - (innerH-lineH)/2
	}
	v.clampScroll()
	v.revealY = top - v.scrollY
	v.revealYValid = true

	if wrap || innerW <= 0 || advance <= 0 {
		return
	}
	if end > chunkEnd {
		end = chunkEnd
	}
	v.revealColumns(colPx(advance, v.columnAt(start)), colPx(advance, v.columnAt(end)), innerW, advance)
}

func (v *textCore) revealColumns(x1, x2, innerW int, advance fixed.Int26_6) {
	margin := colPx(advance, 4)
	if margin > innerW/4 {
		margin = innerW / 4
	}
	if x1 >= v.scrollX+margin && x2 <= v.scrollX+innerW-margin {
		return
	}
	if x2-x1 >= innerW-2*margin {
		v.scrollX = x1 - margin
	} else {
		v.scrollX = (x1+x2)/2 - innerW/2
	}
	if v.scrollX < 0 {
		v.scrollX = 0
	}
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
