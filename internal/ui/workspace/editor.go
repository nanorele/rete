package workspace

import (
	"image"
	"image/color"
	"io"
	"os"
	"sort"
	"strings"
	"time"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"unicode"
	"unicode/utf8"

	"tracto/internal/ui/syntax"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"golang.org/x/image/math/fixed"
)

type RequestEditor struct {
	text       []byte
	lineStarts []int

	chunkHeights      []int
	chunkHeightsWrap  bool
	chunkHeightsWidth int

	scrollY int
	scrollX int

	maxLineWidth int

	highlightStart int
	highlightEnd   int

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

	imeStart       int
	imeEnd         int
	imeSentSnippet key.Snippet

	blinkStart time.Time

	undoStack       []editOp
	redoStack       []editOp
	suppressHistory bool

	dirty bool

	oversizeMsg string

	tokens     []syntax.Token
	tokensLang syntax.Lang
	tokensTxt  int

	layoutShaper *text.Shaper
	layoutFont   font.Font
	layoutSize   unit.Sp
	layoutInnerW int
}

const requestEditorTokenizeMaxBytes = 1 * 1024 * 1024

type editOp struct {
	pos       int
	deleted   []byte
	inserted  []byte
	selBefore int
	endBefore int
	selAfter  int
}

const requestEditorUndoLimit = 1000

const RequestBodyMaxBytes = 100 * 1024 * 1024

const requestEditorVarScanCutoff = 10 * 1024 * 1024

func NewRequestEditor() *RequestEditor {
	return &RequestEditor{
		lineStarts: []int{0},
	}
}

func (v *RequestEditor) spansForChunk(chunkStart, chunkEnd int, sp theme.SyntaxPalette, bracketCycle bool) []widgets.ColoredSpan {
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

func (v *RequestEditor) SetText(s string) bool {
	if len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Body exceeds 100 MB. Load from file instead."
		return false
	}
	v.oversizeMsg = ""
	if cap(v.text) < len(s) {
		v.text = make([]byte, 0, len(s))
	}
	v.text = append(v.text[:0], s...)
	v.rebuildLineStartsFrom(0)
	v.invalidateChunkHeights()
	v.padChunkHeights()
	v.lastTotalH = 0
	v.imeSentSnippet = key.Snippet{}
	v.scrollY = 0
	v.scrollX = 0
	v.maxLineWidth = 0
	v.highlightStart = 0
	v.highlightEnd = 0
	v.selStart = 0
	v.selEnd = 0
	v.dragActive = false
	v.undoStack = v.undoStack[:0]
	v.redoStack = v.redoStack[:0]
	return true
}

func (v *RequestEditor) IsOverSoftLimit() bool {
	return len(v.text) >= RequestBodyMaxBytes
}

func (v *RequestEditor) OversizeMsg() string { return v.oversizeMsg }

func (v *RequestEditor) DismissOversize() { v.oversizeMsg = "" }

func (v *RequestEditor) SizeBytes() int { return len(v.text) }

func (v *RequestEditor) LoadFromReader(r io.Reader) error {
	limited := io.LimitReader(r, int64(RequestBodyMaxBytes)+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if len(data) > RequestBodyMaxBytes {
		v.oversizeMsg = "File exceeds 100 MB; cannot load inline."
		return errBodyTooLarge
	}
	if !v.SetText(string(data)) {
		return errBodyTooLarge
	}
	return nil
}

func (v *RequestEditor) LoadFromFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if fi.Size() > int64(RequestBodyMaxBytes) {
		v.oversizeMsg = "File exceeds 100 MB; cannot load inline."
		return errBodyTooLarge
	}
	data, err := os.ReadFile(path)
	if err != nil {
		v.oversizeMsg = "Load failed: " + err.Error()
		return err
	}
	if !v.SetText(string(data)) {
		return errBodyTooLarge
	}
	return nil
}

var errBodyTooLarge = errBody("request body exceeds 100 MB; load directly via the request's file source instead")

type errBody string

func (e errBody) Error() string { return string(e) }

func (v *RequestEditor) SelectedText() string {
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

func (v *RequestEditor) Append(s string) bool {
	if s == "" {
		return true
	}
	if len(v.text)+len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Append rejected: would exceed 100 MB. Load from file instead."
		return false
	}
	startIdx := len(v.text)
	v.text = append(v.text, s...)
	if last := len(v.chunkHeights) - 1; last >= 0 {
		v.chunkHeights[last] = 0
	}
	v.appendLineStartsFrom(startIdx)
	v.padChunkHeights()
	v.lastTotalH = 0
	return true
}

func (v *RequestEditor) invalidateChunkHeights() {
	v.chunkHeights = v.chunkHeights[:0]
}

func (v *RequestEditor) invalidateChunkHeightsFrom(pos int) {
	idx := sort.Search(len(v.lineStarts), func(i int) bool {
		return v.lineStarts[i] > pos
	}) - 1
	if idx < 0 {
		idx = 0
	}
	if idx < len(v.chunkHeights) {
		v.chunkHeights = v.chunkHeights[:idx]
	}
}

func (v *RequestEditor) padChunkHeights() {
	for len(v.chunkHeights) < len(v.lineStarts) {
		v.chunkHeights = append(v.chunkHeights, 0)
	}
	if len(v.chunkHeights) > len(v.lineStarts) {
		v.chunkHeights = v.chunkHeights[:len(v.lineStarts)]
	}
}

func (v *RequestEditor) Insert(pos int, s string) {
	if s == "" {
		return
	}
	if len(v.text)+len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Paste rejected: would exceed 100 MB. Load from file instead."
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos > len(v.text) {
		pos = len(v.text)
	}
	selBefore, endBefore := v.selStart, v.selEnd
	v.text = append(v.text[:pos], append([]byte(s), v.text[pos:]...)...)

	shift := len(s)
	v.shiftRanges(pos, shift)

	v.invalidateChunkHeightsFrom(pos)
	v.rebuildLineStartsFrom(pos)
	v.maxLineWidth = 0
	v.padChunkHeights()
	v.lastTotalH = 0
	v.recordEdit(editOp{
		pos:       pos,
		deleted:   nil,
		inserted:  []byte(s),
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  pos + len(s),
	})
}

func (v *RequestEditor) DeleteRange(start, end int) {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	if start == end {
		return
	}
	selBefore, endBefore := v.selStart, v.selEnd
	deletedCopy := make([]byte, end-start)
	copy(deletedCopy, v.text[start:end])
	v.text = append(v.text[:start], v.text[end:]...)

	v.shiftRanges(end, -(end - start))

	v.invalidateChunkHeightsFrom(start)
	v.rebuildLineStartsFrom(start)
	v.maxLineWidth = 0
	v.padChunkHeights()
	v.lastTotalH = 0
	v.recordEdit(editOp{
		pos:       start,
		deleted:   deletedCopy,
		inserted:  nil,
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  start,
	})
}

func (v *RequestEditor) Replace(start, end int, s string) bool {
	if start > end {
		start, end = end, start
	}
	if start < 0 {
		start = 0
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	if len(v.text)-(end-start)+len(s) > RequestBodyMaxBytes {
		v.oversizeMsg = "Paste rejected: would exceed 100 MB. Load from file instead."
		return false
	}
	selBefore, endBefore := v.selStart, v.selEnd
	var deletedCopy []byte
	if end > start {
		deletedCopy = make([]byte, end-start)
		copy(deletedCopy, v.text[start:end])
	}
	v.suppressHistory = true
	func() {
		defer func() { v.suppressHistory = false }()
		v.DeleteRange(start, end)
		v.Insert(start, s)
	}()
	if len(deletedCopy) == 0 && s == "" {
		return true
	}
	v.recordEdit(editOp{
		pos:       start,
		deleted:   deletedCopy,
		inserted:  []byte(s),
		selBefore: selBefore,
		endBefore: endBefore,
		selAfter:  start + len(s),
	})
	return true
}

func (v *RequestEditor) recordEdit(op editOp) {
	v.dirty = true
	if v.suppressHistory {
		return
	}
	if n := len(v.undoStack); n > 0 && canMergeEdit(v.undoStack[n-1], op) {
		mergeEditInto(&v.undoStack[n-1], op)
	} else {
		v.undoStack = append(v.undoStack, op)
	}
	if len(v.undoStack) > requestEditorUndoLimit {
		v.undoStack = v.undoStack[len(v.undoStack)-requestEditorUndoLimit:]
	}
	v.redoStack = v.redoStack[:0]
}

func canMergeEdit(prev, op editOp) bool {
	prevIns := len(prev.inserted) > 0 && len(prev.deleted) == 0
	prevDel := len(prev.deleted) > 0 && len(prev.inserted) == 0
	opIns := len(op.inserted) > 0 && len(op.deleted) == 0
	opDel := len(op.deleted) > 0 && len(op.inserted) == 0

	noBreak := func(b []byte) bool {
		for _, c := range b {
			if c == '\n' || c == '\t' || c == ' ' {
				return false
			}
		}
		return true
	}

	switch {
	case prevIns && opIns:
		if !noBreak(prev.inserted) || !noBreak(op.inserted) {
			return false
		}
		return op.pos == prev.pos+len(prev.inserted)
	case prevDel && opDel:
		if !noBreak(prev.deleted) || !noBreak(op.deleted) {
			return false
		}
		if op.pos+len(op.deleted) == prev.pos {
			return true
		}
		if op.pos == prev.pos {
			return true
		}
	}
	return false
}

func mergeEditInto(prev *editOp, op editOp) {
	switch {
	case len(prev.inserted) > 0 && len(op.inserted) > 0:
		prev.inserted = append(prev.inserted, op.inserted...)
		prev.selAfter = op.selAfter
	case len(prev.deleted) > 0 && len(op.deleted) > 0:
		if op.pos+len(op.deleted) == prev.pos {
			prev.deleted = append(append([]byte{}, op.deleted...), prev.deleted...)
			prev.pos = op.pos
		} else {
			prev.deleted = append(prev.deleted, op.deleted...)
		}
		prev.selAfter = op.selAfter
	}
}

func (v *RequestEditor) Changed() bool {
	d := v.dirty
	v.dirty = false
	return d
}

func (v *RequestEditor) Undo() bool {
	if len(v.undoStack) == 0 {
		return false
	}
	op := v.undoStack[len(v.undoStack)-1]
	v.undoStack = v.undoStack[:len(v.undoStack)-1]
	v.suppressHistory = true
	func() {
		defer func() { v.suppressHistory = false }()
		if len(op.inserted) > 0 {
			v.DeleteRange(op.pos, op.pos+len(op.inserted))
		}
		if len(op.deleted) > 0 {
			v.Insert(op.pos, string(op.deleted))
		}
	}()
	v.selStart = op.selBefore
	v.selEnd = op.endBefore
	v.redoStack = append(v.redoStack, op)
	return true
}

func (v *RequestEditor) Redo() bool {
	if len(v.redoStack) == 0 {
		return false
	}
	op := v.redoStack[len(v.redoStack)-1]
	v.redoStack = v.redoStack[:len(v.redoStack)-1]
	v.suppressHistory = true
	func() {
		defer func() { v.suppressHistory = false }()
		if len(op.deleted) > 0 {
			v.DeleteRange(op.pos, op.pos+len(op.deleted))
		}
		if len(op.inserted) > 0 {
			v.Insert(op.pos, string(op.inserted))
		}
	}()
	caret := op.selAfter
	v.selStart = caret
	v.selEnd = caret
	v.undoStack = append(v.undoStack, op)
	return true
}

func (v *RequestEditor) normSel() (int, int) {
	if v.selStart <= v.selEnd {
		return v.selStart, v.selEnd
	}
	return v.selEnd, v.selStart
}

func (v *RequestEditor) pushIMEState(gtx layout.Context) {
	caretByte := v.selEnd
	caretRune := byteToRuneIdx(v.text, caretByte)
	selStartRune := byteToRuneIdx(v.text, v.selStart)
	selEndRune := caretRune

	gtx.Execute(key.SelectionCmd{
		Tag:   v,
		Range: key.Range{Start: selStartRune, End: selEndRune},
	})

	const window = 256
	startRune := caretRune - window
	if startRune < 0 {
		startRune = 0
	}
	endRune := caretRune + window
	totalRunes := utf8.RuneCount(v.text)
	if endRune > totalRunes {
		endRune = totalRunes
	}
	startByte := runeIdxToByte(v.text, startRune)
	endByte := runeIdxToByte(v.text, endRune)
	snip := key.Snippet{
		Range: key.Range{Start: startRune, End: endRune},
		Text:  string(v.text[startByte:endByte]),
	}
	if snip == v.imeSentSnippet {
		return
	}
	v.imeSentSnippet = snip
	gtx.Execute(key.SnippetCmd{Tag: v, Snippet: snip})
}

func (v *RequestEditor) shiftRanges(from, delta int) {
	adjust := func(off int) int {
		if off >= from {
			return off + delta
		}
		if delta < 0 && off > from+delta {
			return from + delta
		}
		return off
	}
	v.selStart = adjust(v.selStart)
	v.selEnd = adjust(v.selEnd)
	if v.highlightEnd > 0 {
		v.highlightStart = adjust(v.highlightStart)
		v.highlightEnd = adjust(v.highlightEnd)
	}
	if v.selStart < 0 {
		v.selStart = 0
	}
	if v.selEnd < 0 {
		v.selEnd = 0
	}
	if v.selStart > len(v.text) {
		v.selStart = len(v.text)
	}
	if v.selEnd > len(v.text) {
		v.selEnd = len(v.text)
	}
}

func (v *RequestEditor) Text() string { return string(v.text) }

func (v *RequestEditor) Bytes() []byte { return v.text }

func (v *RequestEditor) Len() int { return len(v.text) }

func (v *RequestEditor) Selection() (int, int) {
	return v.highlightStart, v.highlightEnd
}

func (v *RequestEditor) SetCaret(start, end int) {
	if start < 0 {
		start = 0
	}
	if end < 0 {
		end = 0
	}
	if start > len(v.text) {
		start = len(v.text)
	}
	if end > len(v.text) {
		end = len(v.text)
	}
	v.highlightStart = start
	v.highlightEnd = end
	v.scrollToByteOffset(start)
}

func (v *RequestEditor) SetScrollCaret(bool) {}

func (v *RequestEditor) GetScrollY() int { return v.scrollY }

func (v *RequestEditor) SetScrollY(y int) {
	v.scrollY = y
	v.clampScroll()
}

func (v *RequestEditor) GetScrollX() int { return v.scrollX }

func (v *RequestEditor) SetScrollX(x int) {
	v.scrollX = x
	if v.scrollX < 0 {
		v.scrollX = 0
	}
}

func (v *RequestEditor) GetMaxLineWidth() int { return v.maxLineWidth }

func (v *RequestEditor) GetScrollBounds() image.Rectangle {
	if v.lastLineHeight == 0 {
		return image.Rectangle{}
	}
	totalH := v.lastTotalH
	if totalH <= 0 {
		totalH = len(v.lineStarts) * v.lastLineHeight
	}
	return image.Rectangle{Max: image.Point{Y: totalH}}
}

func (v *RequestEditor) clampScroll() {
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

func (v *RequestEditor) scrollToByteOffset(off int) {
	if v.lastLineHeight == 0 {
		return
	}
	line := v.lineForByteOffset(off)
	target := line * v.lastLineHeight
	if v.lastViewportH > 0 {
		target -= v.lastViewportH / 2
	}
	v.scrollY = target
	v.clampScroll()
}

func (v *RequestEditor) lineForByteOffset(off int) int {
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

func (v *RequestEditor) rebuildLineStartsFrom(startIdx int) {
	for len(v.lineStarts) > 0 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *RequestEditor) appendLineStartsFrom(startIdx int) {
	if len(v.lineStarts) == 0 {
		v.lineStarts = append(v.lineStarts, 0)
	}
	for len(v.lineStarts) > 1 && v.lineStarts[len(v.lineStarts)-1] > startIdx {
		v.lineStarts = v.lineStarts[:len(v.lineStarts)-1]
	}
	v.scanChunks(v.lineStarts[len(v.lineStarts)-1])
}

func (v *RequestEditor) scanChunks(from int) {
	lastBreak := from
	for i := from; i < len(v.text); i++ {
		if v.text[i] == '\n' {
			if i+1 <= len(v.text) {
				v.lineStarts = append(v.lineStarts, i+1)
			}
			lastBreak = i + 1
		} else if i-lastBreak >= chunkMaxBytes {
			breakAt := i
			for breakAt > lastBreak && (v.text[breakAt]&0xC0) == 0x80 {
				breakAt--
			}
			if breakAt == lastBreak {
				breakAt = i
				for breakAt < len(v.text) && (v.text[breakAt]&0xC0) == 0x80 {
					breakAt++
				}
				if breakAt >= len(v.text) {
					return
				}
			}
			v.lineStarts = append(v.lineStarts, breakAt)
			lastBreak = breakAt
		}
	}
}

type RequestEditorStyle struct {
	Viewer         *RequestEditor
	Shaper         *text.Shaper
	Font           font.Font
	TextSize       unit.Sp
	Color          color.NRGBA
	HighlightColor color.NRGBA
	SelectionColor color.NRGBA
	Wrap           bool
	ReadOnly       bool
	Padding        unit.Dp
	Env            map[string]string

	Lang syntax.Lang

	Syntax       theme.SyntaxPalette
	BracketCycle bool
}

func (s RequestEditorStyle) Layout(gtx layout.Context) layout.Dimensions {
	v := s.Viewer

	size := gtx.Constraints.Max
	if size.X <= 0 || size.Y <= 0 {
		event.Op(gtx.Ops, v)
		return layout.Dimensions{Size: size}
	}

	tokenizing := s.Lang != syntax.LangPlain && len(v.text) <= requestEditorTokenizeMaxBytes
	if tokenizing {
		if s.Lang != v.tokensLang || len(v.text) != v.tokensTxt {
			v.tokens = syntax.Tokenize(s.Lang, v.text)
			v.tokensLang = s.Lang
			v.tokensTxt = len(v.text)
		}
	} else if v.tokens != nil {
		v.tokens = nil
		v.tokensLang = syntax.LangPlain
		v.tokensTxt = 0
	}

	pad := 0
	if s.Padding > 0 {
		pad = gtx.Dp(s.Padding)
	}
	if pad*2 >= size.X || pad*2 >= size.Y {
		pad = 0
	}
	innerW := size.X - 2*pad
	innerH := size.Y - 2*pad

	v.layoutShaper = s.Shaper
	v.layoutFont = s.Font
	v.layoutSize = s.TextSize
	v.layoutInnerW = innerW

	lineHeight := gtx.Sp(s.TextSize) * 7 / 5
	if lineHeight <= 0 {
		lineHeight = 14
	}
	v.lastLineHeight = lineHeight
	v.lastViewportH = innerH

	if s.Wrap != v.chunkHeightsWrap || (s.Wrap && v.chunkHeightsWidth != innerW) {
		v.invalidateChunkHeights()
		v.chunkHeightsWrap = s.Wrap
		v.chunkHeightsWidth = innerW
		v.maxLineWidth = 0
		v.scrollX = 0
	}
	v.padChunkHeights()

	textColorMacro := op.Record(gtx.Ops)
	paint.ColorOp{Color: s.Color}.Add(gtx.Ops)
	textColor := textColorMacro.Stop()

	charAdv := measureCharAdvance(s.Shaper, s.Font, s.TextSize, gtx)
	exactLineH := measureLineHeight(s.Shaper, s.Font, s.TextSize, textColor, gtx)
	if exactLineH <= 0 {
		exactLineH = lineHeight
	}
	v.lastLineHeight = exactLineH

	totalH := 0
	for i, h := range v.chunkHeights {
		if h > 0 {
			totalH += h
		} else {
			totalH += v.estimateChunkHeight(i, exactLineH, charAdv, innerW, s.Wrap)
		}
	}
	if totalH < innerH {
		totalH = innerH
	}
	v.lastTotalH = totalH

	maxY := totalH - innerH
	if maxY < 0 {
		maxY = 0
	}
	sdist := v.Scroller.Update(
		gtx.Metric, gtx.Source, gtx.Now, gesture.Vertical,
		pointer.ScrollRange{},
		pointer.ScrollRange{Min: -v.scrollY, Max: maxY - v.scrollY},
	)
	v.scrollY += sdist

	if !s.Wrap {
		maxX := v.maxLineWidth - innerW
		if maxX < 0 {
			maxX = 0
		}
		sxdist := v.ScrollerH.Update(
			gtx.Metric, gtx.Source, gtx.Now, gesture.Horizontal,
			pointer.ScrollRange{Min: -v.scrollX, Max: maxX - v.scrollX},
			pointer.ScrollRange{},
		)
		v.scrollX += sxdist
		if v.scrollX > maxX {
			v.scrollX = maxX
		}
	} else {
		v.scrollX = 0
	}
	v.clampScroll()

	firstLine, accumY := v.firstChunkAtFn(v.scrollY, exactLineH, charAdv, innerW, s.Wrap)

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()

	pointer.CursorText.Add(gtx.Ops)
	v.Scroller.Add(gtx.Ops)
	if !s.Wrap {
		v.ScrollerH.Add(gtx.Ops)
	}
	v.Drag.Add(gtx.Ops)
	v.Click.Add(gtx.Ops)
	event.Op(gtx.Ops, v)
	for {
		_, ok := gtx.Event(pointer.Filter{Target: v, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
		if !ok {
			break
		}
	}
	key.InputHintOp{Tag: v, Hint: key.HintText}.Add(gtx.Ops)

	if pad > 0 {
		padTr := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
		defer padTr.Pop()
	}

	lbl := widget.Label{}
	if !s.Wrap {
		lbl.MaxLines = 1
	} else {
		lbl.WrapPolicy = text.WrapGraphemes
	}

	hasSel := v.selStart != v.selEnd
	hasHL := v.highlightEnd > v.highlightStart

	for {
		ev, ok := v.Click.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind != gesture.KindPress || ev.Source != pointer.Mouse {
			continue
		}
		off := v.coordToByteOffset(gtx, ev.Position.X-pad, ev.Position.Y-pad, charAdv, exactLineH, innerW, s.Wrap)
		gtx.Execute(key.FocusCmd{Tag: v})
		switch {
		case ev.NumClicks >= 3:
			v.selStart, v.selEnd = v.sourceLineBoundsAt(off)
			v.dragActive = false
		case ev.NumClicks == 2:
			v.selStart, v.selEnd = v.wordBoundsAt(off)
			v.dragActive = false
		case ev.Modifiers&key.ModShift != 0:
			v.selEnd = off
			v.dragActive = true
		default:
			v.selStart = off
			v.selEnd = off
			v.dragActive = true
		}
		hasSel = v.selStart != v.selEnd
		v.blinkStart = gtx.Now
		v.pushIMEState(gtx)
	}

	for {
		ev, ok := v.Drag.Update(gtx.Metric, gtx.Source, gesture.Both)
		if !ok {
			break
		}
		switch ev.Kind {
		case pointer.Drag:
			if v.dragActive {
				off := v.coordToByteOffset(gtx, int(ev.Position.X)-pad, int(ev.Position.Y)-pad, charAdv, exactLineH, innerW, s.Wrap)
				v.selEnd = off
				hasSel = v.selStart != v.selEnd
			}
		case pointer.Release, pointer.Cancel:
			v.dragActive = false
			hasSel = v.selStart != v.selEnd
			v.pushIMEState(gtx)
		}
	}

	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: v},
			transfer.TargetFilter{Target: v, Type: "application/text"},
			key.Filter{Focus: v, Name: key.NameDeleteBackward, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameDeleteForward, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameReturn, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameEnter, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: v, Name: "V", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "X", Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		v.blinkStart = gtx.Now
		switch ke := ev.(type) {
		case key.FocusEvent:
			if ke.Focus {
				gtx.Execute(key.SoftKeyboardCmd{Show: true})
				v.pushIMEState(gtx)
			} else {
				v.imeStart, v.imeEnd = 0, 0
				v.imeSentSnippet = key.Snippet{}
			}
		case key.EditEvent:
			start, end := v.normSel()
			if v.Replace(start, end, ke.Text) {
				caret := start + len(ke.Text)
				v.selStart = caret
				v.selEnd = caret
				v.imeStart, v.imeEnd = 0, 0
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			}
		case key.SnippetEvent:
			v.imeStart = runeIdxToByte(v.text, ke.Start)
			v.imeEnd = runeIdxToByte(v.text, ke.End)
		case key.SelectionEvent:
			startB := runeIdxToByte(v.text, ke.Start)
			endB := runeIdxToByte(v.text, ke.End)
			v.selStart = startB
			v.selEnd = endB
			v.ensureCaretVisible()
		case transfer.DataEvent:
			rd := ke.Open()
			data, err := io.ReadAll(rd)
			_ = rd.Close()
			if err == nil && len(data) > 0 {
				start, end := v.normSel()
				if v.Replace(start, end, string(data)) {
					caret := start + len(data)
					v.selStart = caret
					v.selEnd = caret
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			}
		case key.Event:
			if ke.State != key.Press {
				continue
			}
			switch ke.Name {
			case key.NameDeleteBackward:
				if v.selStart != v.selEnd {
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
				} else if v.selEnd > 0 {
					prev := v.charLeft(v.selEnd)
					v.DeleteRange(prev, v.selEnd)
					v.selStart = prev
					v.selEnd = prev
				}
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case key.NameDeleteForward:
				if v.selStart != v.selEnd {
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
				} else if v.selEnd < len(v.text) {
					next := v.charRight(v.selEnd)
					v.DeleteRange(v.selEnd, next)
				}
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			case key.NameReturn, key.NameEnter:
				start, end := v.normSel()
				if v.Replace(start, end, "\n") {
					caret := start + 1
					v.selStart = caret
					v.selEnd = caret
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			case key.NameTab:
				start, end := v.normSel()
				if v.Replace(start, end, "\t") {
					caret := start + 1
					v.selStart = caret
					v.selEnd = caret
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			case "V":
				gtx.Execute(clipboard.ReadCmd{Tag: v})
			case "X":
				if sel := v.SelectedText(); sel != "" {
					gtx.Execute(clipboard.WriteCmd{
						Type: "application/text",
						Data: io.NopCloser(strings.NewReader(sel)),
					})
					start, end := v.normSel()
					v.DeleteRange(start, end)
					v.selStart = start
					v.selEnd = start
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			}
		}
		hasSel = v.selStart != v.selEnd
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: v, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: "Z", Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: v, Name: "Y", Required: key.ModShortcut},
			key.Filter{Focus: v, Name: key.NameLeftArrow, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameRightArrow, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NameHome, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NameEnd, Optional: key.ModShortcut | key.ModShift},
			key.Filter{Focus: v, Name: key.NamePageUp, Optional: key.ModShift},
			key.Filter{Focus: v, Name: key.NamePageDown, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		v.blinkStart = gtx.Now
		extend := ke.Modifiers.Contain(key.ModShift)
		wordwise := ke.Modifiers.Contain(key.ModShortcut)
		switch ke.Name {
		case "A":
			v.SelectAll()
		case "C":
			if sel := v.SelectedText(); sel != "" {
				gtx.Execute(clipboard.WriteCmd{
					Type: "application/text",
					Data: io.NopCloser(strings.NewReader(sel)),
				})
			}
		case "Z":
			if ke.Modifiers.Contain(key.ModShift) {
				if v.Redo() {
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			} else {
				if v.Undo() {
					v.ensureCaretVisible()
					v.pushIMEState(gtx)
				}
			}
		case "Y":
			if v.Redo() {
				v.ensureCaretVisible()
				v.pushIMEState(gtx)
			}
		case key.NameLeftArrow:
			pos := v.selEnd
			if wordwise {
				pos = v.wordLeft(pos)
			} else {
				pos = v.charLeft(pos)
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NameRightArrow:
			pos := v.selEnd
			if wordwise {
				pos = v.wordRight(pos)
			} else {
				pos = v.charRight(pos)
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NameUpArrow:
			if s.Wrap {
				prefX := v.visualXAt(v.selEnd, gtx, innerW)
				v.moveCaret(v.wrapLineMoveX(v.selEnd, prefX, -1, gtx, innerW), extend)
			} else {
				col := v.columnAt(v.selEnd)
				v.moveCaret(v.lineUp(v.selEnd, col), extend)
			}
			v.ensureCaretVisible()
		case key.NameDownArrow:
			if s.Wrap {
				prefX := v.visualXAt(v.selEnd, gtx, innerW)
				v.moveCaret(v.wrapLineMoveX(v.selEnd, prefX, +1, gtx, innerW), extend)
			} else {
				col := v.columnAt(v.selEnd)
				v.moveCaret(v.lineDown(v.selEnd, col), extend)
			}
			v.ensureCaretVisible()
		case key.NameHome:
			if wordwise {
				v.moveCaret(0, extend)
			} else {
				lineStart, _ := v.sourceLineBoundsAt(v.selEnd)
				v.moveCaret(lineStart, extend)
			}
			v.ensureCaretVisible()
		case key.NameEnd:
			if wordwise {
				v.moveCaret(len(v.text), extend)
			} else {
				_, lineEnd := v.sourceLineBoundsAt(v.selEnd)
				v.moveCaret(lineEnd, extend)
			}
			v.ensureCaretVisible()
		case key.NamePageUp:
			lines := 1
			if v.lastLineHeight > 0 && v.lastViewportH > 0 {
				lines = v.lastViewportH / v.lastLineHeight
				if lines < 1 {
					lines = 1
				}
			}
			pos := v.selEnd
			if s.Wrap {
				prefX := v.visualXAt(pos, gtx, innerW)
				for i := 0; i < lines; i++ {
					newPos := v.wrapLineMoveX(pos, prefX, -1, gtx, innerW)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			} else {
				col := v.columnAt(pos)
				for i := 0; i < lines; i++ {
					newPos := v.lineUp(pos, col)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		case key.NamePageDown:
			lines := 1
			if v.lastLineHeight > 0 && v.lastViewportH > 0 {
				lines = v.lastViewportH / v.lastLineHeight
				if lines < 1 {
					lines = 1
				}
			}
			pos := v.selEnd
			if s.Wrap {
				prefX := v.visualXAt(pos, gtx, innerW)
				for i := 0; i < lines; i++ {
					newPos := v.wrapLineMoveX(pos, prefX, +1, gtx, innerW)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			} else {
				col := v.columnAt(pos)
				for i := 0; i < lines; i++ {
					newPos := v.lineDown(pos, col)
					if newPos == pos {
						break
					}
					pos = newPos
				}
			}
			v.moveCaret(pos, extend)
			v.ensureCaretVisible()
		}
		hasSel = v.selStart != v.selEnd
		v.pushIMEState(gtx)
	}

	const blinkPeriod = 500 * time.Millisecond
	const blinkSolid = blinkPeriod
	caretFocused := gtx.Focused(v) && v.selStart == v.selEnd && !s.ReadOnly
	caretShow := caretFocused
	if caretFocused {
		elapsed := gtx.Now.Sub(v.blinkStart)
		if elapsed < blinkSolid {
			gtx.Execute(op.InvalidateCmd{At: v.blinkStart.Add(blinkSolid)})
		} else {
			rem := elapsed - blinkSolid
			phase := rem / blinkPeriod
			caretShow = phase%2 == 0
			next := v.blinkStart.Add(blinkSolid + (phase+1)*blinkPeriod)
			gtx.Execute(op.InvalidateCmd{At: next})
		}
	}

	yOff := accumY - v.scrollY
	for line := firstLine; line < len(v.lineStarts); line++ {
		if yOff >= innerH {
			break
		}
		start, end := v.lineBounds(line)

		chunkH := v.chunkHeights[line]
		if chunkH == 0 {
			chunkH = v.estimateChunkHeight(line, exactLineH, charAdv, innerW, s.Wrap)
		}

		var glyphs []widgets.WrapGlyph
		if s.Wrap && end > start {
			glyphs = widgets.ShapeChunkForWrap(s.Shaper, s.Font, s.TextSize, gtx, v.text[start:end], innerW)
		}

		if hasHL && v.highlightEnd > start && v.highlightStart < end {
			v.paintHighlight(gtx, start, end, chunkH, yOff, charAdv, s.Wrap, innerW, s.HighlightColor, v.highlightStart, v.highlightEnd, glyphs)
		}

		if caretShow && v.selEnd >= start && v.selEnd <= end {
			v.paintCaret(gtx, start, end, yOff, charAdv, exactLineH, s.Wrap, innerW, s.Color, glyphs)
		}
		if hasSel {
			selS, selE := v.selStart, v.selEnd
			if selS > selE {
				selS, selE = selE, selS
			}
			if selE > start && selS < end {
				v.paintHighlight(gtx, start, end, chunkH, yOff, charAdv, s.Wrap, innerW, s.SelectionColor, selS, selE, glyphs)
			}
		}

		if len(v.text) <= requestEditorVarScanCutoff {
			v.paintVarHighlights(gtx, start, end, yOff, charAdv, exactLineH, s.Wrap, innerW, s.Env, glyphs)
		}

		tr := op.Offset(image.Pt(-v.scrollX, yOff)).Push(gtx.Ops)
		labelGtx := gtx
		labelGtx.Constraints.Min = image.Point{}
		if s.Wrap {
			labelGtx.Constraints.Max.X = innerW
		} else {
			labelGtx.Constraints.Max.X = 1 << 24
		}
		labelGtx.Constraints.Max.Y = 1 << 24
		lineText := string(v.text[start:end])
		var dims layout.Dimensions
		if tokenizing && len(v.tokens) > 0 {
			spans := v.spansForChunk(start, end, s.Syntax, s.BracketCycle)
			dims = widgets.PaintColoredText(labelGtx, s.Shaper, s.Font, s.TextSize, lineText, spans, s.Color, s.Wrap, innerW)
		} else {
			dims = lbl.Layout(labelGtx, s.Shaper, s.Font, s.TextSize, lineText, textColor)
		}
		tr.Pop()

		if !s.Wrap && dims.Size.X > v.maxLineWidth {
			v.maxLineWidth = dims.Size.X
		}

		actualH := dims.Size.Y
		if actualH <= 0 {
			actualH = lineHeight
		}
		v.chunkHeights[line] = actualH
		yOff += actualH
	}

	return layout.Dimensions{Size: size}
}

func (v *RequestEditor) coordToByteOffset(
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

func (v *RequestEditor) estimateChunkHeight(line, lineHeight int, advance fixed.Int26_6, viewportW int, wrap bool) int {
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

func (v *RequestEditor) firstChunkAtFn(y, lineH int, advance fixed.Int26_6, viewportW int, wrap bool) (int, int) {
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

func (v *RequestEditor) lineBounds(n int) (int, int) {
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

func (v *RequestEditor) wordBoundsAt(byteOff int) (int, int) {
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

func (v *RequestEditor) sourceLineBoundsAt(byteOff int) (int, int) {
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

func (v *RequestEditor) SelectAll() {
	v.selStart = 0
	v.selEnd = len(v.text)
	v.dragActive = false
}

func (v *RequestEditor) moveCaret(newPos int, extend bool) {
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

func (v *RequestEditor) charLeft(off int) int {
	if off <= 0 {
		return 0
	}
	_, sz := utf8.DecodeLastRune(v.text[:off])
	return off - sz
}

func (v *RequestEditor) charRight(off int) int {
	if off >= len(v.text) {
		return len(v.text)
	}
	_, sz := utf8.DecodeRune(v.text[off:])
	return off + sz
}

func (v *RequestEditor) wordLeft(off int) int {
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

func (v *RequestEditor) wordRight(off int) int {
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

func (v *RequestEditor) columnAt(off int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if off <= lineStart {
		return 0
	}
	return utf8.RuneCount(v.text[lineStart:off])
}

func (v *RequestEditor) offsetAtColumn(lineStart, col int) int {
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

func (v *RequestEditor) lineUp(off, col int) int {
	lineStart, _ := v.sourceLineBoundsAt(off)
	if lineStart == 0 {
		return 0
	}
	prevLineStart, _ := v.sourceLineBoundsAt(lineStart - 1)
	return v.offsetAtColumn(prevLineStart, col)
}

func (v *RequestEditor) lineDown(off, col int) int {
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

func (v *RequestEditor) visualXAt(off int, gtx layout.Context, viewportW int) int {
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

func (v *RequestEditor) wrapLineMoveX(off, prefX, dir int, gtx layout.Context, viewportW int) int {
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

func (v *RequestEditor) ensureCaretVisible() {
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

type requestVarClickTag struct {
	ed    *RequestEditor
	start int
}

func (v *RequestEditor) paintVarHighlights(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	yOff int,
	advance fixed.Int26_6,
	exactLineH int,
	wrap bool,
	viewportW int,
	env map[string]string,
	glyphs []widgets.WrapGlyph,
) {
	if advance <= 0 || chunkEnd <= chunkStart {
		return
	}
	chunkText := v.text[chunkStart:chunkEnd]
	if !bytesContainsTwoBraces(chunkText) {
		return
	}
	cornerR := gtx.Dp(unit.Dp(3))
	padY := gtx.Dp(unit.Dp(2))

	idx := 0
	for idx < len(chunkText) {
		s := bytesIndex(chunkText[idx:], "{{")
		if s == -1 {
			break
		}
		s += idx
		e := bytesIndex(chunkText[s+2:], "}}")
		if e == -1 {
			break
		}
		e = s + 2 + e + 2
		name := strings.TrimSpace(string(chunkText[s+2 : e-2]))
		bgColor := theme.VarMissing
		if _, ok := env[name]; ok && len(env) > 0 {
			bgColor = theme.VarFound
		}

		var hitRect image.Rectangle
		if !wrap {
			startRune := byteToRuneIdx(chunkText, s)
			endRune := byteToRuneIdx(chunkText, e)
			colToPx := func(c int) int {
				return (advance * fixed.Int26_6(c)).Round()
			}
			x1 := colToPx(startRune) - v.scrollX
			x2 := colToPx(endRune) - v.scrollX
			hitRect = image.Rect(x1, yOff-padY, x2, yOff+exactLineH+padY)
			paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(hitRect, cornerR).Op(gtx.Ops))
		} else {
			startX, startLine := widgets.CaretXYInWrap(glyphs, s)
			endX, endLine := widgets.CaretXYInWrap(glyphs, e)
			fullWidth := viewportW
			for ln := startLine; ln <= endLine; ln++ {
				x1 := 0
				x2 := fullWidth
				if ln == startLine {
					x1 = startX
				}
				if ln == endLine {
					x2 = endX
				}
				y := yOff + ln*exactLineH
				rect := image.Rect(x1, y-padY, x2, y+exactLineH+padY)
				paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(rect, cornerR).Op(gtx.Ops))
				if ln == startLine {
					hitRect = rect
				}
			}
		}

		chipStart := chunkStart + s
		chipEnd := chunkStart + e
		tag := requestVarClickTag{ed: v, start: chipStart}
		stack := clip.Rect(hitRect).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		event.Op(gtx.Ops, tag)
		v.processVarChipEvents(gtx, tag, hitRect, name, chipStart, chipEnd)
		stack.Pop()

		idx = e
	}
}

func (v *RequestEditor) processVarChipEvents(
	gtx layout.Context,
	tag requestVarClickTag,
	rect image.Rectangle,
	name string,
	chipStart, chipEnd int,
) {
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: tag,
			Kinds:  pointer.Press | pointer.Enter | pointer.Leave,
		})
		if !ok {
			return
		}
		pe, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch pe.Kind {
		case pointer.Press:
			if !pe.Buttons.Contain(pointer.ButtonPrimary) {
				continue
			}
			originX := widgets.GlobalPointerPos.X - pe.Position.X
			originY := widgets.GlobalPointerPos.Y - pe.Position.Y
			widgets.GlobalVarClick = &widgets.VarHoverState{
				Name:   name,
				Pos:    f32.Pt(originX+float32(rect.Min.X), originY+float32(rect.Max.Y)),
				Editor: v,
				Range:  struct{ Start, End int }{chipStart, chipEnd},
			}
		case pointer.Enter:
			originX := widgets.GlobalPointerPos.X - pe.Position.X
			originY := widgets.GlobalPointerPos.Y - pe.Position.Y
			widgets.GlobalVarHover = &widgets.VarHoverState{
				Name:   name,
				Pos:    f32.Pt(originX+float32(rect.Min.X), originY+float32(rect.Max.Y)),
				Editor: v,
				Range:  struct{ Start, End int }{chipStart, chipEnd},
			}
		case pointer.Leave:
			if widgets.GlobalVarHover != nil &&
				widgets.GlobalVarHover.Editor == v &&
				widgets.GlobalVarHover.Range.Start == chipStart {
				widgets.GlobalVarHover = nil
			}
		}
	}
}

func bytesIndex(b []byte, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	return strings.Index(string(b), sub)
}

func bytesContainsTwoBraces(b []byte) bool {
	for i := 0; i+1 < len(b); i++ {
		if b[i] == '{' && b[i+1] == '{' {
			return true
		}
	}
	return false
}

func (v *RequestEditor) paintCaret(
	gtx layout.Context,
	chunkStart, chunkEnd int,
	yOff int,
	advance fixed.Int26_6,
	exactLineH int,
	wrap bool,
	viewportW int,
	col color.NRGBA,
	glyphs []widgets.WrapGlyph,
) {
	if advance <= 0 {
		return
	}
	caretByte := v.selEnd - chunkStart
	if caretByte < 0 {
		caretByte = 0
	}

	var x, y int
	if !wrap {
		chunkText := v.text[chunkStart:chunkEnd]
		caretRune := byteToRuneIdx(chunkText, caretByte)
		colToPx := func(c int) int {
			return (advance * fixed.Int26_6(c)).Round()
		}
		x = colToPx(caretRune) - v.scrollX
		y = yOff
	} else {
		xPx, subLine := widgets.CaretXYInWrap(glyphs, caretByte)
		x = xPx
		y = yOff + subLine*exactLineH
	}
	caretW := gtx.Dp(unit.Dp(1))
	if caretW < 1 {
		caretW = 1
	}
	rect := image.Rect(x, y, x+caretW, y+exactLineH)
	paint.FillShape(gtx.Ops, col, clip.Rect(rect).Op())
}

func (v *RequestEditor) paintHighlight(
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
		r := image.Rect(x1, yOff, x2, yOff+chunkH)
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
		r := image.Rect(x1, y1, x2, y2)
		paint.FillShape(gtx.Ops, col, clip.Rect(r).Op())
	}
}
