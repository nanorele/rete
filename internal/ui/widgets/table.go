package widgets

import (
	"image"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

const (
	TableHInset   = 8
	tableMinFlex  = 48
	tableHeaderDp = 24
	tableColMinDp = 24
	tableCellPad  = 6
)

type TableColumn struct {
	Title string
	Width unit.Dp
	Min   unit.Dp
	Align text.Alignment
}

type Table struct {
	cols     []TableColumn
	flexIdx  int
	override []int
	drags    []gesture.Drag
	lastX    []float32
}

func NewTable(cols []TableColumn) *Table {
	t := &Table{cols: cols, flexIdx: -1}
	for i, c := range cols {
		if c.Width == 0 && t.flexIdx < 0 {
			t.flexIdx = i
		}
	}
	t.override = make([]int, len(cols))
	t.drags = make([]gesture.Drag, len(cols))
	t.lastX = make([]float32, len(cols))
	return t
}

func (t *Table) Columns() []TableColumn { return t.cols }

func (t *Table) fixedPx(gtx layout.Context, i int) int {
	if t.override[i] > 0 {
		return t.override[i]
	}
	return gtx.Dp(t.cols[i].Width)
}

func (t *Table) minPx(gtx layout.Context, i int) int {
	m := t.cols[i].Min
	if m <= 0 {
		m = tableColMinDp
	}
	return gtx.Dp(m)
}

func (t *Table) fixedSum(gtx layout.Context) int {
	s := 0
	for i := range t.cols {
		if i != t.flexIdx {
			s += t.fixedPx(gtx, i)
		}
	}
	return s
}

func (t *Table) resizable(i int) bool {
	return t.flexIdx >= 0 && i != t.flexIdx
}

func (t *Table) Cell(gtx layout.Context, i int, w layout.Widget) layout.FlexChild {
	in := layout.Inset{}
	if i > 0 {
		in.Left = unit.Dp(tableCellPad)
	}
	if i < len(t.cols)-1 {
		in.Right = unit.Dp(tableCellPad)
	}
	pad := func(gtx layout.Context) layout.Dimensions { return in.Layout(gtx, w) }
	if i == t.flexIdx {
		return layout.Flexed(1, pad)
	}
	px := t.fixedPx(gtx, i)
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = px
		gtx.Constraints.Max.X = px
		return pad(gtx)
	})
}

func (t *Table) Row(gtx layout.Context, cell func(i int) layout.Widget) layout.Dimensions {
	children := make([]layout.FlexChild, len(t.cols))
	for i := range t.cols {
		w := cell(i)
		if w == nil {
			w = func(layout.Context) layout.Dimensions { return layout.Dimensions{} }
		}
		children[i] = t.Cell(gtx, i, w)
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func (t *Table) Header(gtx layout.Context, th *material.Theme) layout.Dimensions {
	t.updateResize(gtx)

	hH := gtx.Dp(unit.Dp(tableHeaderDp))
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, hH)}.Op())

	layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(TableHInset), Right: unit.Dp(TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return t.Row(gtx, func(i int) layout.Widget {
			c := t.cols[i]
			return func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(10), c.Title)
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				lbl.Alignment = c.Align
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}
		})
	})

	t.drawHeaderSeparators(gtx, hH)
	t.layoutHandles(gtx, hH)
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, hH)}
}

func (t *Table) drawHeaderSeparators(gtx layout.Context, hH int) {
	if t.flexIdx < 0 {
		return
	}
	xs := t.boundaries(gtx)
	line := gtx.Dp(unit.Dp(1))
	for i := 0; i < len(t.cols)-1; i++ {
		cx := xs[i]
		paint.FillShape(gtx.Ops, theme.BorderLight, clip.Rect{Min: image.Pt(cx-line/2, 0), Max: image.Pt(cx-line/2+line, hH)}.Op())
	}
}

func (t *Table) boundaries(gtx layout.Context) []int {
	left := gtx.Dp(unit.Dp(TableHInset))
	contentW := gtx.Constraints.Max.X - 2*left
	flexW := contentW - t.fixedSum(gtx)
	if minF := gtx.Dp(unit.Dp(tableMinFlex)); flexW < minF {
		flexW = minF
	}
	xs := make([]int, len(t.cols))
	x := left
	for i := range t.cols {
		w := t.fixedPx(gtx, i)
		if i == t.flexIdx {
			w = flexW
		}
		x += w
		xs[i] = x
	}
	return xs
}

func (t *Table) handleX(xs []int, i int) int {
	if i > t.flexIdx {
		return xs[i-1]
	}
	return xs[i]
}

func (t *Table) layoutHandles(gtx layout.Context, hH int) {
	if t.flexIdx < 0 {
		return
	}
	xs := t.boundaries(gtx)
	grab := gtx.Dp(unit.Dp(6))
	for i := range t.cols {
		if !t.resizable(i) {
			continue
		}
		cx := t.handleX(xs, i)
		area := image.Rect(cx-grab/2, 0, cx+grab/2, hH)
		st := clip.Rect(area).Push(gtx.Ops)
		pointer.CursorColResize.Add(gtx.Ops)
		t.drags[i].Add(gtx.Ops)
		event.Op(gtx.Ops, &t.drags[i])
		st.Pop()
		if t.drags[i].Dragging() {
			line := gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Min: image.Pt(cx-line/2, 0), Max: image.Pt(cx-line/2+line, hH)}.Op())
		}
	}
}

func (t *Table) updateResize(gtx layout.Context) {
	if t.flexIdx < 0 {
		return
	}
	for i := range t.drags {
		if !t.resizable(i) {
			continue
		}
		for {
			ev, ok := t.drags[i].Update(gtx.Metric, gtx.Source, gesture.Horizontal)
			if !ok {
				break
			}
			switch ev.Kind {
			case pointer.Press:
				t.lastX[i] = ev.Position.X
			case pointer.Drag:
				d := ev.Position.X - t.lastX[i]
				t.lastX[i] = ev.Position.X
				if i > t.flexIdx {
					d = -d
				}
				t.resizeCol(gtx, i, int(d))
			}
		}
	}
}

func (t *Table) resizeCol(gtx layout.Context, i, delta int) {
	cur := t.fixedPx(gtx, i)
	left := gtx.Dp(unit.Dp(TableHInset))
	contentW := gtx.Constraints.Max.X - 2*left
	flexW := contentW - t.fixedSum(gtx)
	minFlex := gtx.Dp(unit.Dp(tableMinFlex))
	minCol := t.minPx(gtx, i)

	newW := cur + delta
	if newW < minCol {
		newW = minCol
	}
	if grew := newW - cur; flexW-grew < minFlex {
		newW = cur + (flexW - minFlex)
	}
	if newW < minCol {
		newW = minCol
	}
	t.override[i] = newW
}
