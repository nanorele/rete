package widgets

import (
	"image"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
)

// ScrollableViewer is the part of a text viewer a scrollbar needs.
type ScrollableViewer interface {
	GetScrollBounds() image.Rectangle
	GetScrollY() int
	SetScrollY(int)
}

// ViewerScrollbar draws the draggable track a text viewer is scrolled by, and
// reports whether the drag moved it so the caller can invalidate. It fills the
// constraints, so stack it over the viewer rather than beside it.
func ViewerScrollbar(gtx layout.Context, v ScrollableViewer, drag *gesture.Drag, dragY *float32) (layout.Dimensions, bool) {
	dims := layout.Dimensions{Size: gtx.Constraints.Max}
	totalH := float32(v.GetScrollBounds().Max.Y)
	viewH := float32(gtx.Constraints.Max.Y)
	if totalH <= viewH || totalH == 0 {
		return dims, false
	}

	scrollY := float32(v.GetScrollY())
	maxScroll := totalH - viewH
	if maxScroll <= 0 {
		maxScroll = 1
	}
	frac := scrollY / maxScroll
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	thumbH := viewH * (viewH / totalH)
	if minH := float32(gtx.Dp(unit.Dp(20))); thumbH < minH {
		thumbH = minH
	}
	thumbY := frac * (viewH - thumbH)
	trackW := gtx.Dp(unit.Dp(10))
	thumbW := gtx.Dp(unit.Dp(6))

	moved := false
	stack := clip.Rect(image.Rect(gtx.Constraints.Max.X-trackW, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)).Push(gtx.Ops)
	for {
		e, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			*dragY = e.Position.Y
		case pointer.Drag:
			delta := e.Position.Y - *dragY
			*dragY = e.Position.Y
			if viewH > thumbH {
				scrollY += delta / (viewH - thumbH) * maxScroll
			}
			ny := int(scrollY)
			if ny < 0 {
				ny = 0
			}
			v.SetScrollY(ny)
			moved = true
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	drag.Add(gtx.Ops)
	stack.Pop()

	pad := gtx.Dp(unit.Dp(2))
	rect := image.Rect(
		gtx.Constraints.Max.X-thumbW-pad,
		int(thumbY),
		gtx.Constraints.Max.X-pad,
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	return dims, moved
}
