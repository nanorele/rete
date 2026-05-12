package widgets

import (
	"image"
	"image/color"
	"strings"
	"time"
	"tracto/internal/ui/theme"
	"unicode"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/image/math/fixed"
)

func MeasureTextWidth(gtx layout.Context, th *material.Theme, size unit.Sp, fnt font.Font, str string) int {
	th.Shaper.LayoutString(text.Parameters{
		Font:     fnt,
		PxPerEm:  fixed.I(gtx.Sp(size)),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, str)

	var maxW fixed.Int26_6
	for {
		g, ok := th.Shaper.NextGlyph()
		if !ok {
			break
		}
		if right := g.X + g.Advance; right > maxW {
			maxW = right
		}
	}
	return maxW.Ceil()
}

type widthCacheKey struct {
	pxPerEm  int
	typeface string
	text     string
}

const widthCacheLimit = 2048

var widthCache = make(map[widthCacheKey]int, 512)

func MeasureTextWidthCached(gtx layout.Context, th *material.Theme, size unit.Sp, fnt font.Font, str string) int {
	if str == "" {
		return 0
	}
	key := widthCacheKey{gtx.Sp(size), string(fnt.Typeface), str}
	if w, ok := widthCache[key]; ok {
		return w
	}
	w := MeasureTextWidth(gtx, th, size, fnt, str)
	if len(widthCache) >= widthCacheLimit {
		widthCache = make(map[widthCacheKey]int, 512)
	}
	widthCache[key] = w
	return w
}

const MonoTypeface font.Typeface = "JetBrains Mono"

var MonoFont = font.Font{Typeface: MonoTypeface}

func MonoLabel(th *material.Theme, size unit.Sp, txt string) material.LabelStyle {
	l := material.Label(th, size, txt)
	l.Font.Typeface = MonoTypeface
	return l
}

func MonoButton(th *material.Theme, btn *widget.Clickable, txt string) material.ButtonStyle {
	b := material.Button(th, btn, txt)
	b.Font.Typeface = MonoTypeface
	return b
}

type cachedMetrics struct {
	pxPerEm int
	height  int
	spacing int
}

var metricsCache [16]cachedMetrics
var metricsLRU uint32

func LineMetrics(gtx layout.Context, th *material.Theme, textSize unit.Sp) (int, int) {
	pxPerEm := gtx.Sp(textSize)
	for i := range metricsCache {
		if metricsCache[i].pxPerEm == pxPerEm && metricsCache[i].height > 0 {
			return metricsCache[i].height, metricsCache[i].spacing
		}
	}

	th.Shaper.LayoutString(text.Parameters{
		Font:     MonoFont,
		PxPerEm:  fixed.I(pxPerEm),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, "A")

	var lineHeight int
	if g, ok := th.Shaper.NextGlyph(); ok {
		lineHeight = (g.Ascent + g.Descent).Ceil()
	}
	if lineHeight == 0 {
		lineHeight = gtx.Dp(unit.Dp(15))
	}

	th.Shaper.LayoutString(text.Parameters{
		Font:     MonoFont,
		PxPerEm:  fixed.I(pxPerEm),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, "A\nA")
	var firstY, lastY int32
	firstGlyph := true
	for {
		g, ok := th.Shaper.NextGlyph()
		if !ok {
			break
		}
		if firstGlyph {
			firstY = g.Y
			firstGlyph = false
		}
		lastY = g.Y
	}
	lineSpacing := int(lastY - firstY)
	if lineSpacing <= 0 {
		lineSpacing = int(float64(lineHeight) * 1.2)
	}

	for i := range metricsCache {
		if metricsCache[i].pxPerEm == 0 {
			metricsCache[i] = cachedMetrics{pxPerEm, lineHeight, lineSpacing}
			return lineHeight, lineSpacing
		}
	}
	idx := metricsLRU % uint32(len(metricsCache))
	metricsLRU++
	metricsCache[idx] = cachedMetrics{pxPerEm, lineHeight, lineSpacing}
	return lineHeight, lineSpacing
}

type VarHoverState struct {
	Name   string
	Pos    f32.Point
	Editor any
	Range  struct{ Start, End int }
}

type VarClickTag struct {
	ed    *widget.Editor
	start int
}

type FieldFallbackClickTag struct {
	ed *widget.Editor
}

func CaretIndexAtX(gtx layout.Context, th *material.Theme, textSize unit.Sp, s string, x int) int {
	if x <= 0 || s == "" {
		return 0
	}
	th.Shaper.LayoutString(text.Parameters{
		Font:     MonoFont,
		PxPerEm:  fixed.I(gtx.Sp(textSize)),
		MaxWidth: 1 << 24,
		Locale:   gtx.Locale,
	}, s)
	target := fixed.I(x)
	runeIdx := 0
	for {
		g, ok := th.Shaper.NextGlyph()
		if !ok {
			break
		}
		if g.Advance > 0 {
			mid := g.X + g.Advance/2
			if target < mid {
				return runeIdx
			}
		}
		runeIdx += int(g.Runes)
	}
	return runeIdx
}

func HandleFieldFallbackClick(gtx layout.Context, th *material.Theme, ed *widget.Editor,
	finalSize image.Point, editorRect image.Rectangle, scrollX int, textSize unit.Sp) {
	tag := FieldFallbackClickTag{ed: ed}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	stack := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	event.Op(gtx.Ops, tag)
	stack.Pop()
	pass.Pop()

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		pe, ok := ev.(pointer.Event)
		if !ok || pe.Kind != pointer.Press {
			continue
		}
		if !pe.Buttons.Contain(pointer.ButtonPrimary) {
			continue
		}
		x, y := int(pe.Position.X), int(pe.Position.Y)
		if x >= editorRect.Min.X && x < editorRect.Max.X &&
			y >= editorRect.Min.Y && y < editorRect.Max.Y {
			continue
		}
		textX := x - editorRect.Min.X + scrollX
		if textX < 0 {
			textX = 0
		}
		caretPos := CaretIndexAtX(gtx, th, textSize, ed.Text(), textX)
		ed.SetCaret(caretPos, caretPos)
		gtx.Execute(key.FocusCmd{Tag: ed})
	}
}

var GlobalVarClick *VarHoverState
var GlobalVarHover *VarHoverState
var GlobalPointerPos f32.Point

type hScrollState struct {
	scroller    gesture.Scroll
	thumbDrag   gesture.Drag
	dragLastX   float32
	scrollX     int
	prevCaret   int
	prevTextLen int
	initialized bool
	lastSeen    time.Time
}

var editorHScrolls = make(map[*widget.Editor]*hScrollState)

const hScrollMaxAge = 5 * time.Minute
const hScrollCleanupThreshold = 64

func GetHScroll(ed *widget.Editor) *hScrollState {
	s, ok := editorHScrolls[ed]
	if !ok {
		s = &hScrollState{}
		editorHScrolls[ed] = s
		if len(editorHScrolls) > hScrollCleanupThreshold {
			cutoff := time.Now().Add(-hScrollMaxAge)
			for k, v := range editorHScrolls {
				if k != ed && v.lastSeen.Before(cutoff) {
					delete(editorHScrolls, k)
				}
			}
		}
	}
	s.lastSeen = time.Now()
	return s
}

func UpdateHScroll(gtx layout.Context, ed *widget.Editor, viewW, contentW int) (scrollX, maxScroll int, addGesture func()) {
	s := GetHScroll(ed)
	maxScroll = contentW - viewW
	if maxScroll < 0 {
		maxScroll = 0
	}

	if maxScroll > 0 {
		dx := s.scroller.Update(gtx.Metric, gtx.Source, gtx.Now, gesture.Horizontal,
			pointer.ScrollRange{Min: -s.scrollX, Max: maxScroll - s.scrollX},
			pointer.ScrollRange{},
		)
		s.scrollX += dx
	}

	if maxScroll > 0 && viewW > 0 && contentW > 0 {
		thumbW := viewW * viewW / contentW
		if minW := gtx.Dp(unit.Dp(14)); thumbW < minW {
			thumbW = minW
		}
		if thumbW > viewW {
			thumbW = viewW
		}
		travel := viewW - thumbW
		for {
			ev, ok := s.thumbDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
			if !ok {
				break
			}
			switch ev.Kind {
			case pointer.Press:
				s.dragLastX = ev.Position.X
			case pointer.Drag:
				delta := ev.Position.X - s.dragLastX
				s.dragLastX = ev.Position.X
				if travel > 0 {
					s.scrollX += int(delta * float32(maxScroll) / float32(travel))
				}
			}
		}
	}

	caretX := int(ed.CaretCoords().X)
	_, caret := ed.CaretPos()
	textLen := ed.Len()
	if !s.initialized || caret != s.prevCaret || textLen != s.prevTextLen {
		s.initialized = true
		s.prevCaret = caret
		s.prevTextLen = textLen
		if viewW > 0 {
			caretPad := gtx.Dp(unit.Dp(2))
			if caretX < s.scrollX {
				s.scrollX = caretX - caretPad
			}
			if caretX > s.scrollX+viewW-caretPad {
				s.scrollX = caretX - viewW + caretPad
			}
		}
	}

	if s.scrollX < 0 {
		s.scrollX = 0
	}
	if s.scrollX > maxScroll {
		s.scrollX = maxScroll
	}

	scrollX = s.scrollX
	addGesture = func() {
		s.scroller.Add(gtx.Ops)
	}
	return scrollX, maxScroll, addGesture
}

func ResetEditorHScroll(ed *widget.Editor) {
	delete(editorHScrolls, ed)
}

func TextFieldOverlay(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, env map[string]string, frozenWidth int, textSize unit.Sp) layout.Dimensions {
	ed.SingleLine = true
	ed.Submit = true
	pX := gtx.Dp(unit.Dp(4))
	pY := gtx.Dp(unit.Dp(6))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}
	viewW := max(textWidth-(pX*2), 0)

	edGtx := gtx
	edGtx.Constraints.Min.X = viewW
	edGtx.Constraints.Max.X = 1 << 24
	edGtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y-(pY*2), 0)

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)

	lineHeight, lineSpacing := LineMetrics(gtx, th, textSize)

	type varRectInfo struct {
		name       string
		rect       image.Rectangle
		start, end int
	}
	var varRects []varRectInfo

	if ed.Len() >= 4 {
		textStr := ed.Text()
		if strings.Contains(textStr, "{{") {
			padY := gtx.Dp(unit.Dp(2))

			lineStarts := []int{0}
			for i := 0; i < len(textStr); i++ {
				if textStr[i] == '\n' {
					lineStarts = append(lineStarts, i+1)
				}
			}

			totalHeight := len(lineStarts)*lineSpacing + lineHeight
			cl := clip.Rect{
				Min: image.Pt(0, -padY),
				Max: image.Pt(1<<24, totalHeight+padY),
			}.Push(gtx.Ops)

			cornerR := gtx.Dp(unit.Dp(3))
			idx := 0
			for idx < len(textStr) {
				start := strings.Index(textStr[idx:], "{{")
				if start == -1 {
					break
				}
				start += idx
				end := strings.Index(textStr[start+2:], "}}")
				if end == -1 {
					break
				}
				end = start + 2 + end + 2

				varName := strings.TrimSpace(textStr[start+2 : end-2])

				lineIdx := 0
				for lineIdx+1 < len(lineStarts) && lineStarts[lineIdx+1] <= start {
					lineIdx++
				}
				lineStart := lineStarts[lineIdx]

				pWidth := MeasureTextWidthCached(gtx, th, textSize, MonoFont, textStr[lineStart:start])
				vWidth := MeasureTextWidthCached(gtx, th, textSize, MonoFont, textStr[start:end])

				bgColor := theme.VarMissing
				if _, ok := env[varName]; ok {
					bgColor = theme.VarFound
				}

				yOff := lineIdx * lineSpacing
				rect := image.Rect(pWidth, yOff-padY, pWidth+vWidth, yOff+lineHeight+padY)
				paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))

				varRects = append(varRects, varRectInfo{
					name:  varName,
					rect:  rect,
					start: start,
					end:   end,
				})

				idx = end
			}
			cl.Pop()
		}
	}

	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	e.Font = MonoFont
	HandleEditorShortcuts(gtx, ed)
	dims := e.Layout(edGtx)
	call := macro.Stop()

	contentW := dims.Size.X
	scrollX, _, addGesture := UpdateHScroll(gtx, ed, viewW, contentW)

	finalWidth := availWidth
	naturalH := dims.Size.Y + (pY * 2)
	finalHeight := naturalH
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}
	extraY := 0
	if finalHeight > naturalH {
		extraY = (finalHeight - naturalH) / 2
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, theme.BgField, rect.Op(gtx.Ops))

	if drawBorder {
		borderColor := theme.Border
		if gtx.Focused(ed) {
			borderColor = theme.Accent
		}
		PaintBorder1px(gtx, finalSize, borderColor)
	}

	gestureClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	addGesture()
	gestureClip.Pop()

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	scrollOffset := op.Offset(image.Pt(-scrollX, extraY)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	scrollOffset.Pop()
	textClip.Pop()

	DrawHScrollbar(gtx, ed, contentW, scrollX, finalSize, viewW, pX, 1)

	editorRect := image.Rect(pX, pY+extraY, pX+viewW, pY+extraY+dims.Size.Y)
	HandleFieldFallbackClick(gtx, th, ed, finalSize, editorRect, scrollX, textSize)

	if len(varRects) > 0 {
		macroClick := op.Record(gtx.Ops)
		op.Offset(image.Point{X: pX, Y: pY}).Add(gtx.Ops)
		for _, vr := range varRects {
			tag := VarClickTag{ed: ed, start: vr.start}
			vrLocal := vr.rect
			stack := clip.Rect(vrLocal).Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			event.Op(gtx.Ops, tag)
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: tag,
					Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						originX := GlobalPointerPos.X - pe.Position.X
						originY := GlobalPointerPos.Y - pe.Position.Y
						GlobalVarClick = &VarHoverState{
							Name:   vr.name,
							Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
							Editor: ed,
							Range:  struct{ Start, End int }{vr.start, vr.end},
						}
					}
				case pointer.Enter:
					originX := GlobalPointerPos.X - pe.Position.X
					originY := GlobalPointerPos.Y - pe.Position.Y
					GlobalVarHover = &VarHoverState{
						Name:   vr.name,
						Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
						Editor: ed,
						Range:  struct{ Start, End int }{vr.start, vr.end},
					}
				case pointer.Leave:
					if GlobalVarHover != nil &&
						GlobalVarHover.Editor == ed &&
						GlobalVarHover.Range.Start == vr.start {
						GlobalVarHover = nil
					}
				}
			}
			stack.Pop()
		}
		callClick := macroClick.Stop()

		textClipClick := clip.Rect{Max: finalSize}.Push(gtx.Ops)
		clickOffset := op.Offset(image.Pt(-scrollX, extraY)).Push(gtx.Ops)
		callClick.Add(gtx.Ops)
		clickOffset.Pop()
		textClipClick.Pop()
	}

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + pY}
}

func TextField(gtx layout.Context, th *material.Theme, ed *widget.Editor, hint string, drawBorder bool, env map[string]string, frozenWidth int, textSize unit.Sp) layout.Dimensions {
	ed.SingleLine = true
	ed.Submit = true
	p := gtx.Dp(unit.Dp(4))

	availWidth := gtx.Constraints.Max.X
	if availWidth <= 0 {
		return layout.Dimensions{}
	}

	textWidth := availWidth
	if frozenWidth > 0 {
		textWidth = frozenWidth
	}
	viewW := max(textWidth-(p*2), 0)

	edGtx := gtx
	edGtx.Constraints.Min.X = viewW
	edGtx.Constraints.Max.X = 1 << 24
	edGtx.Constraints.Min.Y = max(gtx.Constraints.Min.Y-(p*2), 0)

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)

	lineHeight, lineSpacing := LineMetrics(gtx, th, textSize)

	type varRectInfo struct {
		name       string
		rect       image.Rectangle
		start, end int
	}
	var varRects []varRectInfo

	if ed.Len() >= 4 {
		textStr := ed.Text()
		if strings.Contains(textStr, "{{") {
			padY := gtx.Dp(unit.Dp(2))

			lineStarts := []int{0}
			for i := 0; i < len(textStr); i++ {
				if textStr[i] == '\n' {
					lineStarts = append(lineStarts, i+1)
				}
			}

			totalHeight := len(lineStarts)*lineSpacing + lineHeight
			cl := clip.Rect{
				Min: image.Pt(0, -padY),
				Max: image.Pt(1<<24, totalHeight+padY),
			}.Push(gtx.Ops)

			cornerR := gtx.Dp(unit.Dp(3))
			idx := 0
			for idx < len(textStr) {
				start := strings.Index(textStr[idx:], "{{")
				if start == -1 {
					break
				}
				start += idx
				end := strings.Index(textStr[start+2:], "}}")
				if end == -1 {
					break
				}
				end = start + 2 + end + 2

				varName := strings.TrimSpace(textStr[start+2 : end-2])

				lineIdx := 0
				for lineIdx+1 < len(lineStarts) && lineStarts[lineIdx+1] <= start {
					lineIdx++
				}
				lineStart := lineStarts[lineIdx]

				pWidth := MeasureTextWidthCached(gtx, th, textSize, MonoFont, textStr[lineStart:start])
				vWidth := MeasureTextWidthCached(gtx, th, textSize, MonoFont, textStr[start:end])

				bgColor := theme.VarMissing
				if _, ok := env[varName]; ok {
					bgColor = theme.VarFound
				}

				yOff := lineIdx * lineSpacing
				rect := image.Rect(pWidth, yOff-padY, pWidth+vWidth, yOff+lineHeight+padY)
				paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))

				varRects = append(varRects, varRectInfo{
					name:  varName,
					rect:  rect,
					start: start,
					end:   end,
				})

				idx = end
			}
			cl.Pop()
		}
	}

	e := material.Editor(th, ed, hint)
	e.TextSize = textSize
	e.Font = MonoFont
	HandleEditorShortcuts(gtx, ed)
	dims := e.Layout(edGtx)
	call := macro.Stop()

	contentW := dims.Size.X
	scrollX, _, addGesture := UpdateHScroll(gtx, ed, viewW, contentW)

	finalWidth := availWidth
	finalHeight := dims.Size.Y + (p * 2)
	if finalHeight < gtx.Constraints.Min.Y {
		finalHeight = gtx.Constraints.Min.Y
	}
	if finalHeight > gtx.Constraints.Max.Y {
		finalHeight = gtx.Constraints.Max.Y
	}

	finalSize := image.Point{X: finalWidth, Y: finalHeight}
	rect := clip.UniformRRect(image.Rectangle{Max: finalSize}, 2)
	paint.FillShape(gtx.Ops, theme.BgField, rect.Op(gtx.Ops))

	if drawBorder {
		borderColor := theme.Border
		if gtx.Focused(ed) {
			borderColor = theme.Accent
		}
		PaintBorder1px(gtx, finalSize, borderColor)
	}

	gestureClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	addGesture()
	gestureClip.Pop()

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	scrollOffset := op.Offset(image.Pt(-scrollX, 0)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	scrollOffset.Pop()
	textClip.Pop()

	DrawHScrollbar(gtx, ed, contentW, scrollX, finalSize, viewW, p, 1)

	editorRect := image.Rect(p, p, p+viewW, p+dims.Size.Y)
	HandleFieldFallbackClick(gtx, th, ed, finalSize, editorRect, scrollX, textSize)

	if len(varRects) > 0 {
		macroClick := op.Record(gtx.Ops)
		op.Offset(image.Point{X: p, Y: p}).Add(gtx.Ops)
		for _, vr := range varRects {
			tag := VarClickTag{ed: ed, start: vr.start}
			vrLocal := vr.rect
			stack := clip.Rect(vrLocal).Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			event.Op(gtx.Ops, tag)
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: tag,
					Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						originX := GlobalPointerPos.X - pe.Position.X
						originY := GlobalPointerPos.Y - pe.Position.Y
						GlobalVarClick = &VarHoverState{
							Name:   vr.name,
							Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
							Editor: ed,
							Range:  struct{ Start, End int }{vr.start, vr.end},
						}
					}
				case pointer.Enter:
					originX := GlobalPointerPos.X - pe.Position.X
					originY := GlobalPointerPos.Y - pe.Position.Y
					GlobalVarHover = &VarHoverState{
						Name:   vr.name,
						Pos:    f32.Pt(originX+float32(vrLocal.Min.X), originY+float32(vrLocal.Max.Y)),
						Editor: ed,
						Range:  struct{ Start, End int }{vr.start, vr.end},
					}
				case pointer.Leave:
					if GlobalVarHover != nil &&
						GlobalVarHover.Editor == ed &&
						GlobalVarHover.Range.Start == vr.start {
						GlobalVarHover = nil
					}
				}
			}
			stack.Pop()
		}
		callClick := macroClick.Stop()

		textClipClick := clip.Rect{Max: finalSize}.Push(gtx.Ops)
		clickOffset := op.Offset(image.Pt(-scrollX, 0)).Push(gtx.Ops)
		callClick.Add(gtx.Ops)
		clickOffset.Pop()
		textClipClick.Pop()
	}

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline + p}
}

func SquareBtn(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return SquareBtnSized(gtx, clk, ic, th, 28, 16)
}

type ScrollLabel struct {
	scroller gesture.Scroll
	scrollX  int
}

func (s *ScrollLabel) Layout(gtx layout.Context, th *material.Theme, lbl material.LabelStyle) layout.Dimensions {
	natW := MeasureTextWidthCached(gtx, th, lbl.TextSize, lbl.Font, lbl.Text)
	viewW := gtx.Constraints.Max.X
	if natW <= viewW {
		s.scrollX = 0
		return lbl.Layout(gtx)
	}
	maxX := natW - viewW
	if maxX < 0 {
		maxX = 0
	}
	if s.scrollX > maxX {
		s.scrollX = maxX
	}
	if s.scrollX < 0 {
		s.scrollX = 0
	}
	sx := s.scroller.Update(gtx.Metric, gtx.Source, gtx.Now, gesture.Horizontal,
		pointer.ScrollRange{Min: -s.scrollX, Max: maxX - s.scrollX},
		pointer.ScrollRange{},
	)
	s.scrollX += sx
	if s.scrollX > maxX {
		s.scrollX = maxX
	}
	if s.scrollX < 0 {
		s.scrollX = 0
	}

	macro := op.Record(gtx.Ops)
	natGtx := gtx
	natGtx.Constraints.Min.X = natW
	natGtx.Constraints.Max.X = natW
	dim := lbl.Layout(natGtx)
	call := macro.Stop()

	cl := clip.Rect{Max: image.Pt(viewW, dim.Size.Y)}.Push(gtx.Ops)
	s.scroller.Add(gtx.Ops)
	off := op.Offset(image.Pt(-s.scrollX, 0)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	off.Pop()
	cl.Pop()

	return layout.Dimensions{Size: image.Pt(viewW, dim.Size.Y)}
}

func InlineRenameField(gtx layout.Context, th *material.Theme, ed *widget.Editor) layout.Dimensions {
	ed.SingleLine = true
	pad := gtx.Dp(unit.Dp(4))

	availWidth := gtx.Constraints.Max.X
	if availWidth < gtx.Constraints.Min.X {
		availWidth = gtx.Constraints.Min.X
	}
	if availWidth <= 0 {
		return layout.Dimensions{}
	}
	viewW := availWidth - 2*pad
	if viewW < 0 {
		viewW = 0
	}

	edGtx := gtx
	edGtx.Constraints.Min.X = viewW
	edGtx.Constraints.Max.X = 1 << 24

	macro := op.Record(gtx.Ops)
	op.Offset(image.Point{X: pad, Y: 0}).Add(gtx.Ops)
	e := material.Editor(th, ed, "")
	e.TextSize = unit.Sp(12)
	dims := e.Layout(edGtx)
	call := macro.Stop()

	contentW := dims.Size.X
	scrollX, _, addGesture := UpdateHScroll(gtx, ed, viewW, contentW)

	finalSize := image.Pt(availWidth, dims.Size.Y)
	rect := image.Rectangle{Max: finalSize}
	paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(rect, 2).Op(gtx.Ops))
	borderC := theme.Border
	if gtx.Focused(ed) {
		borderC = theme.Accent
	}
	PaintBorder1px(gtx, finalSize, borderC)

	gestureClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	addGesture()
	gestureClip.Pop()

	textClip := clip.Rect{Max: finalSize}.Push(gtx.Ops)
	scrollOffset := op.Offset(image.Pt(-scrollX, 0)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	scrollOffset.Pop()
	textClip.Pop()

	DrawHScrollbar(gtx, ed, contentW, scrollX, finalSize, viewW, pad, 1)

	editorRect := image.Rect(pad, 0, pad+viewW, dims.Size.Y)
	HandleFieldFallbackClick(gtx, th, ed, finalSize, editorRect, scrollX, unit.Sp(12))

	return layout.Dimensions{Size: finalSize, Baseline: dims.Baseline}
}

func SquareBtnSlim(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme) layout.Dimensions {
	return SquareBtnSized(gtx, clk, ic, th, 24, 14)
}

func Bordered1px(gtx layout.Context, _ unit.Dp, color color.NRGBA, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	call.Add(gtx.Ops)
	PaintBorder1px(gtx, dims.Size, color)
	return dims
}

func DrawHScrollbar(gtx layout.Context, ed *widget.Editor, contentW, scrollX int, boxSize image.Point, viewW, padX, marginBottom int) {
	span := contentW - viewW
	if span <= 0 || viewW <= 0 {
		return
	}
	h := gtx.Dp(unit.Dp(4))
	if h < 3 {
		h = 3
	}
	if boxSize.Y <= h+marginBottom {
		return
	}
	trackW := viewW
	if trackW > boxSize.X-2*padX {
		trackW = boxSize.X - 2*padX
	}
	if trackW <= 0 {
		return
	}
	trackY := boxSize.Y - h - marginBottom

	thumbW := trackW * viewW / contentW
	if minW := gtx.Dp(unit.Dp(14)); thumbW < minW {
		thumbW = minW
	}
	if thumbW > trackW {
		thumbW = trackW
	}

	posOffset := (trackW - thumbW) * scrollX / span
	if posOffset < 0 {
		posOffset = 0
	}
	if max := trackW - thumbW; posOffset > max {
		posOffset = max
	}

	hitH := h + gtx.Dp(unit.Dp(4))
	if hitH < h {
		hitH = h
	}
	hitY := trackY + h/2 - hitH/2
	if hitY < 0 {
		hitY = 0
	}
	if hitY+hitH > boxSize.Y {
		hitY = boxSize.Y - hitH
	}
	state := GetHScroll(ed)
	hitClip := clip.Rect{
		Min: image.Pt(padX, hitY),
		Max: image.Pt(padX+trackW, hitY+hitH),
	}.Push(gtx.Ops)
	pointer.CursorPointer.Add(gtx.Ops)
	state.thumbDrag.Add(gtx.Ops)
	hitClip.Pop()

	thumb := image.Rect(
		padX+posOffset, trackY,
		padX+posOffset+thumbW, trackY+h,
	)
	r := h / 2
	paint.FillShape(gtx.Ops, theme.EditorScroll, clip.UniformRRect(thumb, r).Op(gtx.Ops))
}

func PaintBorder1px(gtx layout.Context, sz image.Point, color color.NRGBA) {
	if sz.X <= 0 || sz.Y <= 0 {
		return
	}
	paint.FillShape(gtx.Ops, color, clip.Rect{Max: image.Pt(sz.X, 1)}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(0, sz.Y-1), Max: sz}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Max: image.Pt(1, sz.Y)}.Op())
	paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(sz.X-1, 0), Max: sz}.Op())
}

func SquareBtnSized(gtx layout.Context, clk *widget.Clickable, ic *widget.Icon, th *material.Theme, dpBox, dpIcon int) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(float32(dpBox)))
		gtx.Constraints.Min = image.Point{X: size, Y: size}
		gtx.Constraints.Max = gtx.Constraints.Min

		rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
		bg := theme.BgField
		if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
		PaintBorder1px(gtx, gtx.Constraints.Min, theme.Border)
		pointer.CursorPointer.Add(gtx.Ops)

		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{X: gtx.Dp(unit.Dp(float32(dpIcon))), Y: gtx.Dp(unit.Dp(float32(dpIcon)))}
			return ic.Layout(gtx, th.Fg)
		})
	})
}

func MenuOption(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, icon *widget.Icon) layout.Dimensions {
	return MenuOptionStyled(gtx, th, clk, title, icon, th.Fg, th.Fg, false)
}

func MenuOptionDanger(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, icon *widget.Icon) layout.Dimensions {
	return MenuOptionStyled(gtx, th, clk, title, icon, theme.Danger, theme.Danger, true)
}

func MenuOptionStyled(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title string, icon *widget.Icon, iconCol color.NRGBA, textCol color.NRGBA, bold bool) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(150)
		if clk.Hovered() {
			paint.FillShape(gtx.Ops, theme.BgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
		}
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
					return icon.Layout(gtx, iconCol)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), title)
					lbl.Color = textCol
					if bold {
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func IsSeparator(r rune) bool {

	return unicode.IsSpace(r) || strings.ContainsRune(".,:;!?()[]{}\"'`", r)
}

func MoveWord(s string, pos int, dir int) int {
	if dir > 0 {
		if pos < 0 {
			pos = 0
		}
		bi := 0
		ri := 0
		for ri < pos && bi < len(s) {
			_, size := utf8.DecodeRuneInString(s[bi:])
			bi += size
			ri++
		}
		for bi < len(s) {
			r, size := utf8.DecodeRuneInString(s[bi:])
			if !IsSeparator(r) {
				break
			}
			bi += size
			ri++
		}
		for bi < len(s) {
			r, size := utf8.DecodeRuneInString(s[bi:])
			if IsSeparator(r) {
				break
			}
			bi += size
			ri++
		}
		return ri
	}
	if pos <= 0 {
		return 0
	}
	bi := 0
	ri := 0
	for ri < pos && bi < len(s) {
		_, size := utf8.DecodeRuneInString(s[bi:])
		bi += size
		ri++
	}
	for bi > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:bi])
		if size == 0 || !IsSeparator(r) {
			break
		}
		bi -= size
		ri--
	}
	for bi > 0 {
		r, size := utf8.DecodeLastRuneInString(s[:bi])
		if size == 0 || IsSeparator(r) {
			break
		}
		bi -= size
		ri--
	}
	return ri
}

func HandleEditorShortcuts(gtx layout.Context, ed *widget.Editor) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: ed, Name: key.NameLeftArrow, Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: ed, Name: key.NameRightArrow, Required: key.ModShortcut, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}

		extend := e.Modifiers.Contain(key.ModShift)
		start, end := ed.Selection()
		switch e.Name {
		case key.NameLeftArrow:
			newPos := MoveWord(ed.Text(), end, -1)
			if extend {
				ed.SetCaret(start, newPos)
			} else {
				ed.SetCaret(newPos, newPos)
			}
		case key.NameRightArrow:
			newPos := MoveWord(ed.Text(), end, 1)
			if extend {
				ed.SetCaret(start, newPos)
			} else {
				ed.SetCaret(newPos, newPos)
			}
		}
	}
}
