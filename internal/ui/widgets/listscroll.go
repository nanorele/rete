package widgets

import (
	"image"

	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func clampVS(v float32) float32 {
	if v >= 1 {
		return 1
	} else if v <= 0 {
		return 0
	}
	return v
}

func listViewport(lp layout.Position, elements, majorAxisSize int) (float32, float32) {
	if elements <= 0 || lp.Length <= 0 {
		return 0, 1
	}
	lengthEstPx := float32(lp.Length)
	elementLenEstPx := lengthEstPx / float32(elements)
	listOffsetF := float32(lp.Offset)
	listOffsetL := float32(lp.OffsetLast)
	viewportStart := clampVS((float32(lp.First)*elementLenEstPx + listOffsetF) / lengthEstPx)
	viewportEnd := clampVS((float32(lp.First+lp.Count)*elementLenEstPx + listOffsetL) / lengthEstPx)
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

// VScrollList lays out a vertical widget.List and draws a scrollbar that appears
// only when the content overflows the viewport. When overflowing it reserves a
// gutter on the right so the scrollbar never overlaps the row content.
func VScrollList(gtx layout.Context, th *material.Theme, list *widget.List, length int, element layout.ListElement) layout.Dimensions {
	sb := material.Scrollbar(th, &list.Scrollbar)
	sbW := gtx.Dp(sb.Width())
	fullMaxX := gtx.Constraints.Max.X

	prevStart, prevEnd := listViewport(list.Position, length, gtx.Constraints.Max.Y)
	overflow := prevEnd-prevStart < 1

	listGtx := gtx
	if overflow {
		if listGtx.Constraints.Max.X -= sbW; listGtx.Constraints.Max.X < 0 {
			listGtx.Constraints.Max.X = 0
		}
		if listGtx.Constraints.Min.X > listGtx.Constraints.Max.X {
			listGtx.Constraints.Min.X = listGtx.Constraints.Max.X
		}
	}
	dims := list.Layout(listGtx, length, element)

	start, end := listViewport(list.Position, length, dims.Size.Y)
	sgtx := gtx
	sgtx.Constraints.Min = image.Pt(fullMaxX, dims.Size.Y)
	sgtx.Constraints.Max = image.Pt(fullMaxX, dims.Size.Y)
	bounds := clip.Rect{Max: image.Pt(fullMaxX, dims.Size.Y)}.Push(gtx.Ops)
	layout.E.Layout(sgtx, func(gtx layout.Context) layout.Dimensions {
		bar := sb.Layout(gtx, layout.Vertical, start, end)
		if bar.Size.X > 0 {
			cl := clip.Rect{Max: bar.Size}.Push(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			cl.Pop()
		}
		return bar
	})
	bounds.Pop()

	if delta := list.Scrollbar.ScrollDistance(); delta != 0 {
		list.List.ScrollBy(delta * float32(length))
	}

	dims.Size.X = fullMaxX
	return dims
}
