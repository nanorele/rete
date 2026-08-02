package workspace

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/image/math/fixed"
)

type searchRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
	ed     *RequestEditor
	box    *SearchBox
	size   image.Point
	pad    unit.Dp
	wrap   bool
}

func newSearchRig(txt string, wrap bool) *searchRig {
	rig := &searchRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
		box:    &SearchBox{},
		size:   image.Pt(400, 300),
		pad:    unit.Dp(4),
		wrap:   wrap,
	}
	rig.v.SetText(txt)
	return rig
}

func newSearchEditorRig(txt string, wrap bool) *searchRig {
	rig := &searchRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		ed:     NewRequestEditor(),
		box:    &SearchBox{},
		size:   image.Pt(400, 300),
		wrap:   wrap,
	}
	rig.ed.SetText(txt)
	return rig
}

func (rig *searchRig) gtx() layout.Context {
	rig.ops.Reset()
	return layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
}

func (rig *searchRig) frame() {
	gtx := rig.gtx()
	if rig.v != nil {
		ResponseViewerStyle{
			Viewer:   rig.v,
			Shaper:   rig.shaper,
			TextSize: unit.Sp(13),
			Wrap:     rig.wrap,
			Padding:  rig.pad,
		}.Layout(gtx)
	} else {
		RequestEditorStyle{
			Viewer:   rig.ed,
			Shaper:   rig.shaper,
			TextSize: unit.Sp(13),
			Wrap:     rig.wrap,
		}.Layout(gtx)
	}
	rig.r.Frame(rig.ops)
}

func (rig *searchRig) theme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = rig.shaper
	return th
}

func (rig *searchRig) core() *textCore {
	if rig.v != nil {
		return &rig.v.textCore
	}
	return &rig.ed.textCore
}

func (rig *searchRig) target() searchableEditor {
	if rig.v != nil {
		return rig.v
	}
	return rig.ed
}

func (rig *searchRig) metrics() (charAdv fixed.Int26_6, lineH, innerW, innerH, pad int) {
	gtx := rig.gtx()
	m := op.Record(gtx.Ops)
	paint.ColorOp{}.Add(gtx.Ops)
	col := m.Stop()
	var fnt font.Font
	charAdv = measureCharAdvance(rig.shaper, fnt, unit.Sp(13), gtx)
	lineH = measureLineHeight(rig.shaper, fnt, unit.Sp(13), col, gtx)
	pad = gtx.Dp(rig.pad)
	if 2*pad >= rig.size.X || 2*pad >= rig.size.Y {
		pad = 0
	}
	innerW = rig.size.X - 2*pad
	innerH = rig.size.Y - 2*pad
	return
}

// matchOnScreen probes the viewport with the same hit-test the mouse uses and
// reports whether any visible pixel maps into [start,end).
func (rig *searchRig) matchOnScreen(start, end int) bool {
	c := rig.core()
	charAdv, lineH, innerW, innerH, _ := rig.metrics()
	if lineH <= 0 {
		return false
	}
	gtx := rig.gtx()
	stepY := lineH / 2
	if stepY < 1 {
		stepY = 1
	}
	for y := 0; y < innerH; y += stepY {
		for x := 0; x <= innerW; x += 3 {
			off := c.coordToByteOffset(gtx, x, y, charAdv, lineH, innerW, rig.wrap)
			if off >= start && off < end {
				return true
			}
		}
	}
	return false
}

func (rig *searchRig) search(q string) {
	rig.box.Editor.SetText(q)
	rig.box.Open = true
	rig.box.invalidate()
	rig.box.refresh(rig.target(), rig.target().Text(), true)
	rig.frame()
}

func (rig *searchRig) next() {
	rig.box.navigate(1, rig.target())
	rig.frame()
}

func (rig *searchRig) report(t *testing.T, label string) bool {
	t.Helper()
	b := rig.box
	if b.current < 0 || b.current >= len(b.spans) {
		t.Errorf("%s: no current match (spans=%d)", label, len(b.spans))
		return false
	}
	m := b.spans[b.current]
	if !rig.matchOnScreen(m.start, m.end) {
		c := rig.core()
		t.Errorf("%s: match %d/%d at bytes [%d,%d) is NOT visible (scrollY=%d scrollX=%d viewportH=%d totalH=%d)",
			label, b.current+1, len(b.spans), m.start, m.end, c.scrollY, c.scrollX, c.lastViewportH, c.lastTotalH)
		return false
	}
	return true
}

func minifiedJSON(n int) string {
	var sb strings.Builder
	sb.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		fmt.Fprintf(&sb, `{"id":%d,"name":"item-%04d","tag":"filler-payload-value"}`, i, i)
	}
	sb.WriteString(`]}`)
	return sb.String()
}

func prettyJSON(n int) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"name\": \"item-%04d\", \"tag\": \"%s\" },\n", i, i, strings.Repeat("long-", 30))
	}
	sb.WriteString("  ]\n}\n")
	return sb.String()
}

func TestSearchScroll_MinifiedWrapOn(t *testing.T) {
	rig := newSearchRig(minifiedJSON(400), true)
	rig.frame()
	rig.frame()
	rig.search("item-0300")
	rig.report(t, "wrap-on minified, first match")
}

func TestSearchScroll_MinifiedWrapOff(t *testing.T) {
	rig := newSearchRig(minifiedJSON(400), false)
	rig.frame()
	rig.frame()
	rig.search("item-0300")
	rig.report(t, "wrap-off minified, first match")
}

func TestSearchScroll_PrettyWrapOn(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()
	rig.search("item-0150")
	rig.report(t, "wrap-on pretty, first match")
}

func TestSearchScroll_NavigateAllPretty(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("name")
	n := len(rig.box.spans)
	if n < 10 {
		t.Fatalf("expected many matches, got %d", n)
	}
	bad := 0
	for i := 0; i < n; i++ {
		if !rig.report(t, fmt.Sprintf("pretty next #%d", i+1)) {
			bad++
			if bad > 3 {
				t.Fatalf("too many invisible matches")
			}
		}
		rig.next()
	}
}

func TestSearchScroll_EditorWrapOn(t *testing.T) {
	rig := newSearchEditorRig(prettyJSON(120), true)
	rig.frame()
	rig.frame()
	rig.search("item-0090")
	rig.report(t, "editor wrap-on, first match")
}

// The panel is pinned top-right and must stay there, so the reveal is what
// keeps matches out from under it: a match anywhere the document can scroll
// has to land clear of the reserved band.
func TestSearchPanel_RevealKeepsMatchOutOfPanelBand(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()
	rig.box.panelH = 40

	rig.search("item-0150")
	y, ok := rig.core().RevealScreenY()
	if !ok {
		t.Fatal("reveal not resolved")
	}
	if y < rig.box.panelH {
		t.Errorf("match revealed at row %d, inside the %d-tall panel band", y, rig.box.panelH)
	}
}

func TestSearch_QueryThatStopsMatchingClearsTheHighlight(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("item-0030")
	if got := rig.v.SelectedText(); got != "item-0030" {
		t.Fatalf("precondition: selection = %q", got)
	}

	rig.search("item-0030-no-such-thing")
	if len(rig.box.spans) != 0 {
		t.Fatalf("precondition: expected no matches, got %d", len(rig.box.spans))
	}
	c := rig.core()
	if c.highlightEnd > c.highlightStart {
		t.Errorf("0/0 still highlights [%d,%d) from the query that used to match",
			c.highlightStart, c.highlightEnd)
	}
	if got := rig.v.SelectedText(); got != "" {
		t.Errorf("0/0 still leaves %q selected", got)
	}
}

func TestSearchClose_ClearsTheLastMatchSelection(t *testing.T) {
	rig := newSearchRig(prettyJSON(60), true)
	rig.frame()
	rig.frame()
	rig.search("item-0030")
	if got := rig.v.SelectedText(); got != "item-0030" {
		t.Fatalf("precondition: match should be selected, got %q", got)
	}

	rig.box.closeOn(rig.target())
	rig.frame()

	if got := rig.v.SelectedText(); got != "" {
		t.Errorf("closing the search left %q selected in the viewer", got)
	}
	c := rig.core()
	if c.highlightEnd > c.highlightStart {
		t.Errorf("closing the search left the match highlight [%d,%d)", c.highlightStart, c.highlightEnd)
	}
}

func TestSearchScroll_BeforeFirstLayout(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.search("item-0150")
	rig.frame()
	rig.frame()
	rig.report(t, "search issued before first layout")
}
