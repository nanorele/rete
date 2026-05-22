package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"tracto/internal/ui/mitm"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
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

func mitmBgBar(gtx layout.Context, bg color.NRGBA, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()
	sz := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: sz}.Op())
	call.Add(gtx.Ops)
	dims.Size = sz
	return dims
}

func mitmRecord(gtx layout.Context) op.MacroOp {
	return op.Record(gtx.Ops)
}

var (
	uacShieldOnce sync.Once
	uacShieldOp   paint.ImageOp
	uacShieldOK   bool
)

func loadUACShield() {
	data, err := mitm.UACShieldPNG()
	if err != nil {
		return
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	uacShieldOp = paint.NewImageOp(img)
	uacShieldOK = true
}

func paintUACShield(gtx layout.Context, sz int) layout.Dimensions {
	uacShieldOnce.Do(loadUACShield)
	gtx.Constraints.Min = image.Pt(sz, sz)
	gtx.Constraints.Max = gtx.Constraints.Min
	if !uacShieldOK {
		return widgets.IconShield.Layout(gtx, theme.Accent)
	}
	im := widget.Image{Src: uacShieldOp, Fit: widget.Contain}
	return im.Layout(gtx)
}
