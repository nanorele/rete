package widgets

import (
	"image"
	"image/color"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio-x/component"

	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
)

func MenuBorderColor() color.NRGBA {
	return theme.Mix(theme.BgMenu, theme.Fg, 0.42)
}

func PaintPopupSurface(gtx layout.Context, sz image.Point, radiusDp int, surface, border color.NRGBA) {
	if sz.X <= 0 || sz.Y <= 0 {
		return
	}
	r := gtx.Dp(unit.Dp(float32(radiusDp)))
	paint.FillShape(gtx.Ops, border, clip.UniformRRect(image.Rectangle{Max: sz}, r).Op(gtx.Ops))
	inner := image.Rect(1, 1, sz.X-1, sz.Y-1)
	if inner.Dx() <= 0 || inner.Dy() <= 0 {
		return
	}
	ir := r - 1
	if ir < 0 {
		ir = 0
	}
	paint.FillShape(gtx.Ops, surface, clip.UniformRRect(inner, ir).Op(gtx.Ops))
}

func (m menuStyler) surface(gtx layout.Context, tag event.Tag, content layout.Widget) layout.Dimensions {
	minW := gtx.Dp(unit.Dp(float32(m.MinWidthDp)))

	measGtx := gtx
	measGtx.Constraints.Min = image.Point{}
	measGtx.Constraints.Max.Y = 1 << 24
	rm := op.Record(measGtx.Ops)
	nat := content(measGtx)
	rm.Stop()

	w := nat.Size.X
	if w < minW {
		w = minW
	}
	if max := gtx.Constraints.Max.X; max > 0 && w > max {
		w = max
	}

	cGtx := gtx
	cGtx.Constraints.Min = image.Pt(w, 0)
	cGtx.Constraints.Max.X = w
	cGtx.Constraints.Max.Y = 1 << 24
	rec := op.Record(cGtx.Ops)
	dims := content(cGtx)
	call := rec.Stop()

	sz := image.Pt(w, dims.Size.Y)
	radius := gtx.Dp(unit.Dp(MenuRadiusDp))

	MenuShadow(gtx, sz)
	PaintPopupSurface(gtx, sz, MenuRadiusDp, m.Colors.Surface, m.Colors.Border)

	if tag != nil {
		st := clip.Rect{Max: sz}.Push(gtx.Ops)
		event.Op(gtx.Ops, tag)
		st.Pop()
		for {
			if _, ok := gtx.Event(pointer.Filter{Target: tag, Kinds: pointer.Press}); !ok {
				break
			}
		}
	}

	inner := image.Rect(1, 1, sz.X-1, sz.Y-1)
	ir := radius - 1
	if ir < 0 {
		ir = 0
	}
	st := clip.UniformRRect(inner, ir).Push(gtx.Ops)
	call.Add(gtx.Ops)
	st.Pop()

	return layout.Dimensions{Size: sz}
}

func (m menuStyler) itemsContent(entries []MenuItem) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		rowMinX := gtx.Constraints.Min.X
		pad := unit.Dp(MenuListPadDp)
		if theme.CompactMenus {
			pad = 0
		}
		return layout.Inset{Top: pad, Bottom: pad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, len(entries))
			for i := range entries {
				it := entries[i]
				children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = rowMinX
					return m.AnchoredMenu.Row(gtx, it)
				})
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	}
}

func (m menuStyler) list(gtx layout.Context, tag event.Tag, entries []MenuItem) layout.Dimensions {
	return m.surface(gtx, tag, m.itemsContent(entries))
}

func (m menuStyler) deferSurfaceAt(gtx layout.Context, tag event.Tag, anchor MenuAnchor, content layout.Widget) layout.Dimensions {
	rec := op.Record(gtx.Ops)
	mGtx := gtx
	mGtx.Constraints.Min = image.Point{}
	dims := m.surface(mGtx, tag, content)
	call := rec.Stop()

	pos := anchor.Resolve(dims.Size)
	macro := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	call.Add(gtx.Ops)
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

func (m menuStyler) deferAt(gtx layout.Context, tag event.Tag, anchor MenuAnchor, entries []MenuItem) layout.Dimensions {
	return m.deferSurfaceAt(gtx, tag, anchor, m.itemsContent(entries))
}

type menuStyler struct {
	component.AnchoredMenu
}
