package sidebar

import (
	"image"

	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func clamp1sb(v float32) float32 {
	if v >= 1 {
		return 1
	} else if v <= 0 {
		return 0
	}
	return v
}

func viewportFromList(lp layout.Position, elements, majorAxisSize int) (float32, float32) {
	if elements <= 0 || lp.Length <= 0 {
		return 0, 1
	}
	lengthEstPx := float32(lp.Length)
	elementLenEstPx := lengthEstPx / float32(elements)
	listOffsetF := float32(lp.Offset)
	listOffsetL := float32(lp.OffsetLast)
	viewportStart := clamp1sb((float32(lp.First)*elementLenEstPx + listOffsetF) / lengthEstPx)
	viewportEnd := clamp1sb((float32(lp.First+lp.Count)*elementLenEstPx + listOffsetL) / lengthEstPx)
	viewportFraction := viewportEnd - viewportStart
	visibleFraction := float32(majorAxisSize) / lengthEstPx
	err := visibleFraction - viewportFraction
	adjStart := viewportStart
	adjEnd := viewportEnd
	if viewportFraction < 1 {
		startShare := viewportStart / (1 - viewportFraction)
		endShare := (1 - viewportEnd) / (1 - viewportFraction)
		adjStart -= startShare * err
		adjEnd += endShare * err
	}
	return adjStart, adjEnd
}

func sidebarBarWidth(gtx layout.Context, th *material.Theme, list *widget.List) int {
	return gtx.Dp(material.Scrollbar(th, &list.Scrollbar).Width())
}

// layoutSidebarScrollbar draws a widget.List's scrollbar as a self-contained
// top overlay, independent of any sticky-header band rendered beneath it: the
// thumb stays fully visible above sticky rows, its geometry is derived only
// from the list position (never the band), and it owns its own drag and cursor
// so dragging it neither grabs sticky nodes nor shows the text I-beam.
func layoutSidebarScrollbar(gtx layout.Context, th *material.Theme, list *widget.List, length, majorAxisSize int, fade float32) {
	if list == nil || length <= 0 || majorAxisSize <= 0 {
		return
	}
	start, end := viewportFromList(list.Position, length, majorAxisSize)

	sb := material.Scrollbar(th, &list.Scrollbar)
	sb.Indicator.Color.A = uint8(float32(sb.Indicator.Color.A) * fade)
	sb.Indicator.HoverColor.A = uint8(float32(sb.Indicator.HoverColor.A) * fade)

	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, majorAxisSize)
	layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		dims := sb.Layout(gtx, layout.Vertical, start, end)
		cl := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		cl.Pop()
		return dims
	})

	if delta := list.Scrollbar.ScrollDistance(); delta != 0 {
		list.List.ScrollBy(delta * float32(length))
	}
}
