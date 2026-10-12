package widgets

import (
	"image"
	"image/color"

	"rete/internal/ui/theme"

	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func OutlineButton(gtx layout.Context, th *material.Theme, btn *widget.Clickable, txt string, outline, surface color.NRGBA, inset layout.Inset) layout.Dimensions {
	return btn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				bg := surface
				if btn.Hovered() {
					bg = theme.Mix(surface, outline, 0.18)
				}
				PaintPopupSurface(gtx, gtx.Constraints.Min, 4, bg, outline)
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Point{}
					lbl := material.Label(th, unit.Sp(12), txt)
					lbl.Color = outline
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}
