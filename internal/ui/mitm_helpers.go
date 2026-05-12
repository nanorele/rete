package ui

import (
	"image"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
)

func mitmHLine(gtx layout.Context) layout.Dimensions {
	rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
	paint.FillShape(gtx.Ops, theme.Border, rect.Op())
	return layout.Dimensions{Size: rect.Max}
}

func mitmBoxed(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	sz := dims.Size
	paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(image.Rectangle{Max: sz}, 4).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widgets.PaintBorder1px(gtx, sz, theme.Border)
	return dims
}

func mitmRecord(gtx layout.Context) op.MacroOp {
	return op.Record(gtx.Ops)
}
