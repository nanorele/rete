package widgets

import (
	"image"
	"image/color"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

const (
	MenuRadiusDp    = 5
	MenuRowRadiusDp = 4
	MenuBorderDp    = 1
	MenuListPadDp   = 4
	MenuRowPadVDp   = 5
	MenuRowPadHDp   = 10
	MenuGutterDp    = 18
	MenuMinWidthDp  = 168
)

type MenuItem struct {
	Label     string
	Shortcut  string
	Icon      *widget.Icon
	Click     *widget.Clickable
	Danger    bool
	Disabled  bool
	Checked   bool
	Bold      bool
	Mono      bool
	Separator bool
	LabelCol  color.NRGBA
}

func MenuShadow(gtx layout.Context, sz image.Point) {
	if sz.X <= 0 || sz.Y <= 0 {
		return
	}
	radius := gtx.Dp(unit.Dp(MenuRadiusDp))
	layers := []struct {
		spread, dy int
		a          uint8
	}{
		{gtx.Dp(2), gtx.Dp(1), 34},
		{gtx.Dp(5), gtx.Dp(2), 22},
		{gtx.Dp(9), gtx.Dp(4), 12},
		{gtx.Dp(14), gtx.Dp(6), 6},
	}
	for _, l := range layers {
		r := image.Rect(-l.spread, -l.spread+l.dy, sz.X+l.spread, sz.Y+l.spread+l.dy)
		paint.FillShape(gtx.Ops, color.NRGBA{A: l.a},
			clip.UniformRRect(r, radius+l.spread).Op(gtx.Ops))
	}
}

func MenuSurface(gtx layout.Context, tag event.Tag, minWidthDp int, content layout.Widget) layout.Dimensions {
	minW := gtx.Dp(unit.Dp(float32(minWidthDp)))

	measGtx := gtx
	measGtx.Constraints.Min = image.Point{}
	measGtx.Constraints.Max.Y = 1 << 24
	m := op.Record(measGtx.Ops)
	nat := content(measGtx)
	m.Stop()

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
	paint.FillShape(gtx.Ops, theme.BgMenu,
		clip.UniformRRect(image.Rectangle{Max: sz}, radius).Op(gtx.Ops))

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

	st := clip.UniformRRect(image.Rectangle{Max: sz}, radius).Push(gtx.Ops)
	call.Add(gtx.Ops)
	st.Pop()

	bw := float32(gtx.Dp(unit.Dp(MenuBorderDp)))
	paint.FillShape(gtx.Ops, theme.BorderLight, clip.Stroke{
		Path:  clip.UniformRRect(image.Rectangle{Max: sz}, radius).Path(gtx.Ops),
		Width: bw,
	}.Op())

	return layout.Dimensions{Size: sz}
}

func menuItemsContent(th *material.Theme, items []MenuItem) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		rowMinX := gtx.Constraints.Min.X
		pad := unit.Dp(MenuListPadDp)
		return layout.Inset{Top: pad, Bottom: pad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, len(items))
			for i := range items {
				it := items[i]
				children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = rowMinX
					return MenuRow(gtx, th, it)
				})
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	}
}

func MenuList(gtx layout.Context, th *material.Theme, tag event.Tag, minWidthDp int, items []MenuItem) layout.Dimensions {
	return MenuSurface(gtx, tag, minWidthDp, menuItemsContent(th, items))
}

type MenuAnchor struct {
	Pt          image.Point
	AlignRight  bool
	AlignBottom bool
	Clamp       image.Point
}

func (a MenuAnchor) resolve(size image.Point) image.Point {
	x, y := a.Pt.X, a.Pt.Y
	if a.AlignRight {
		x -= size.X
	}
	if a.AlignBottom {
		y -= size.Y
	}
	if a.Clamp.X > 0 {
		if x+size.X > a.Clamp.X {
			x = a.Clamp.X - size.X
		}
		if x < 0 {
			x = 0
		}
	}
	if a.Clamp.Y > 0 {
		if y+size.Y > a.Clamp.Y {
			y = a.Clamp.Y - size.Y
		}
		if y < 0 {
			y = 0
		}
	}
	return image.Pt(x, y)
}

func DeferMenuSurfaceAt(gtx layout.Context, tag event.Tag, anchor MenuAnchor, minWidthDp int, content layout.Widget) layout.Dimensions {
	rec := op.Record(gtx.Ops)
	mGtx := gtx
	mGtx.Constraints.Min = image.Point{}
	dims := MenuSurface(mGtx, tag, minWidthDp, content)
	call := rec.Stop()

	pos := anchor.resolve(dims.Size)
	macro := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	call.Add(gtx.Ops)
	op.Defer(gtx.Ops, macro.Stop())
	return dims
}

func DeferMenuAt(gtx layout.Context, th *material.Theme, tag event.Tag, anchor MenuAnchor, minWidthDp int, items []MenuItem) layout.Dimensions {
	return DeferMenuSurfaceAt(gtx, tag, anchor, minWidthDp, menuItemsContent(th, items))
}

func DeferMenu(gtx layout.Context, th *material.Theme, tag event.Tag, anchor image.Point, minWidthDp int, items []MenuItem) layout.Dimensions {
	return DeferMenuAt(gtx, th, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, items)
}

func DeferMenuSurface(gtx layout.Context, tag event.Tag, anchor image.Point, minWidthDp int, content layout.Widget) layout.Dimensions {
	return DeferMenuSurfaceAt(gtx, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, content)
}

func MenuRow(gtx layout.Context, th *material.Theme, it MenuItem) layout.Dimensions {
	if it.Separator {
		return menuDivider(gtx)
	}

	txtCol := th.Fg
	icoCol := th.Fg
	switch {
	case it.Disabled:
		txtCol, icoCol = theme.FgDisabled, theme.FgDisabled
	case it.Danger:
		txtCol, icoCol = theme.Danger, theme.Danger
	case it.LabelCol != (color.NRGBA{}):
		txtCol, icoCol = it.LabelCol, it.LabelCol
	}

	body := func(gtx layout.Context) layout.Dimensions {
		fill := gtx.Constraints.Min.X > 0
		w := gtx.Constraints.Min.X

		label := func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			var lbl material.LabelStyle
			if it.Mono {
				lbl = MonoLabel(th, unit.Sp(12), it.Label)
			} else {
				lbl = material.Label(th, unit.Sp(12), it.Label)
			}
			lbl.Color = txtCol
			lbl.MaxLines = 1
			lbl.Truncator = "…"
			lbl.LineHeightScale = 1.0
			if it.Bold {
				lbl.Font.Weight = font.Bold
			}
			return lbl.Layout(gtx)
		}
		shortcut := func(gtx layout.Context) layout.Dimensions {
			if it.Shortcut == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), it.Shortcut)
				lbl.Color = theme.FgMuted
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		}

		hovered := it.Click != nil && it.Click.Hovered() && !it.Disabled
		rowContent := func(gtx layout.Context) layout.Dimensions {
			if fill {
				gtx.Constraints.Min.X = w
			} else {
				gtx.Constraints.Min.X = 0
			}
			return layout.Inset{
				Top:    unit.Dp(MenuRowPadVDp),
				Bottom: unit.Dp(MenuRowPadVDp),
				Left:   unit.Dp(MenuRowPadHDp),
				Right:  unit.Dp(MenuRowPadHDp),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gutter := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					g := gtx.Dp(unit.Dp(MenuGutterDp))
					gtx.Constraints.Min = image.Pt(g, g)
					gtx.Constraints.Max = image.Pt(g, g)
					switch {
					case it.Checked:
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							sz := gtx.Dp(unit.Dp(14))
							gtx.Constraints.Min = image.Pt(sz, sz)
							return IconCheck.Layout(gtx, icoCol)
						})
					case it.Icon != nil:
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							sz := gtx.Dp(unit.Dp(15))
							gtx.Constraints.Min = image.Pt(sz, sz)
							return it.Icon.Layout(gtx, icoCol)
						})
					default:
						return layout.Dimensions{Size: image.Pt(g, g)}
					}
				})
				spacer := layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout)
				flex := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}
				if fill {
					return flex.Layout(gtx,
						gutter, spacer,
						layout.Flexed(1, label),
						layout.Rigid(shortcut),
					)
				}
				return flex.Layout(gtx,
					gutter, spacer,
					layout.Rigid(label),
					layout.Rigid(shortcut),
				)
			})
		}

		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				if hovered {
					paint.FillShape(gtx.Ops, theme.BgHover,
						clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(MenuRowRadiusDp))).Op(gtx.Ops))
				}
				if it.Disabled {
					pointer.CursorDefault.Add(gtx.Ops)
				} else {
					pointer.CursorPointer.Add(gtx.Ops)
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(rowContent),
		)
	}

	if it.Disabled || it.Click == nil {
		return body(gtx)
	}
	return material.Clickable(gtx, it.Click, body)
}

func menuDivider(gtx layout.Context) layout.Dimensions {
	w := gtx.Constraints.Min.X
	vpad := gtx.Dp(unit.Dp(4))
	hpad := gtx.Dp(unit.Dp(8))
	line := gtx.Dp(unit.Dp(1))
	if line < 1 {
		line = 1
	}
	defer op.Offset(image.Pt(hpad, vpad)).Push(gtx.Ops).Pop()
	lw := w - 2*hpad
	if lw < 0 {
		lw = 0
	}
	paint.FillShape(gtx.Ops, theme.DividerLight, clip.Rect{Max: image.Pt(lw, line)}.Op())
	return layout.Dimensions{Size: image.Pt(w, line+2*vpad)}
}
