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

// Unified context-menu / popup styling, modelled on VS Code's menus but using
// this app's theme tokens. Every popup in the app is built from MenuSurface
// (the chrome: soft shadow, always-present 1px border, rounded surface) plus
// MenuRow (a hoverable, clickable item with a leading check/icon gutter).
//
// All popups are dismissed centrally: clicking a row performs its action and
// closes the menu, and the app's root backdrop closes every open popup on any
// outside click (see app.closeAllPopups).
const (
	MenuRadiusDp    = 5  // corner radius of the surface and hover rows
	MenuRowRadiusDp = 4  // corner radius of the per-row hover highlight
	MenuBorderDp    = 1  // border is always drawn, kept minimal
	MenuListPadDp   = 4  // padding above/below the row list
	MenuRowPadVDp   = 5  // row vertical padding
	MenuRowPadHDp   = 10 // row horizontal padding
	MenuGutterDp    = 18 // leading check/icon column width
	MenuMinWidthDp  = 168
)

// MenuItem describes a single row in a unified popup menu.
type MenuItem struct {
	Label     string
	Shortcut  string       // optional, right-aligned, muted
	Icon      *widget.Icon // optional leading icon (shown when not Checked)
	Click     *widget.Clickable
	Danger    bool // red label/icon (destructive action)
	Disabled  bool // dimmed, no hover, not clickable
	Checked   bool // shows a check in the leading gutter (selection menus)
	Bold      bool
	Mono      bool        // monospace label
	Separator bool        // renders a divider; all other fields ignored
	LabelCol  color.NRGBA // optional label/icon color override (zero => default)
}

// MenuShadow paints a soft drop shadow for a rounded rect of the given size,
// anchored at the current origin. gio has no blur primitive, so the shadow is
// approximated with a few translucent, progressively larger rounded rects.
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

// MenuSurface draws the unified popup chrome around content and returns its
// dimensions. It:
//   - measures content to a uniform width (>= minWidthDp) so all rows align,
//   - paints a soft shadow, a rounded BgMenu fill and an always-present 1px
//     border,
//   - clips content to the rounded rect,
//   - registers tag as a press-catcher over the whole surface so that clicks
//     inside the menu's own padding do not fall through to the root backdrop
//     (which would otherwise dismiss the menu).
//
// The surface is drawn at the current origin; callers position it with op.Offset
// (typically inside an op.Record / op.Defer that lifts the menu above the rest
// of the frame).
func MenuSurface(gtx layout.Context, tag event.Tag, minWidthDp int, content layout.Widget) layout.Dimensions {
	minW := gtx.Dp(unit.Dp(float32(minWidthDp)))

	// Pass 1: natural width with unbounded height.
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

	// Pass 2: record content at the resolved uniform width.
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

// menuItemsContent lays the items out vertically with the standard list padding.
// It propagates the incoming Min.X to every row so rows fill the menu's resolved
// width in the real pass and shrink to content in the measuring pass (Min.X==0),
// independent of how layout.Flex treats the cross axis.
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

// MenuList is a convenience that wraps a slice of MenuItem in MenuSurface, laid
// out vertically with the standard list padding (no positioning/clamping).
func MenuList(gtx layout.Context, th *material.Theme, tag event.Tag, minWidthDp int, items []MenuItem) layout.Dimensions {
	return MenuSurface(gtx, tag, minWidthDp, menuItemsContent(th, items))
}

// MenuAnchor describes where a deferred menu is placed, in the current
// coordinate space. By default Pt is the menu's top-left corner. AlignRight /
// AlignBottom make Pt the menu's right / bottom edge instead (so the menu grows
// left / up from Pt) — useful for menus opened from a "⋮" button at the right
// edge of a row. Clamp, when an axis is > 0, keeps the menu within [0, Clamp]
// on that axis; a zero axis means "do not clamp" (the menu may overflow, e.g.
// row menus that intentionally extend past their row).
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

// DeferMenuSurfaceAt lays out content inside the unified popup chrome, positions
// it according to anchor, and emits it above the rest of the frame via op.Defer.
// The content is laid out exactly once and then repositioned, so item
// clickables behave correctly. Returns the menu size.
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

// DeferMenuAt is DeferMenuSurfaceAt for a vertical list of MenuItem rows.
func DeferMenuAt(gtx layout.Context, th *material.Theme, tag event.Tag, anchor MenuAnchor, minWidthDp int, items []MenuItem) layout.Dimensions {
	return DeferMenuSurfaceAt(gtx, tag, anchor, minWidthDp, menuItemsContent(th, items))
}

// DeferMenu places a top-left-anchored menu and clamps it within
// gtx.Constraints.Max — the common case for menus opened at the app root (e.g.
// at a cursor position).
func DeferMenu(gtx layout.Context, th *material.Theme, tag event.Tag, anchor image.Point, minWidthDp int, items []MenuItem) layout.Dimensions {
	return DeferMenuAt(gtx, th, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, items)
}

// DeferMenuSurface places top-left-anchored custom content and clamps it within
// gtx.Constraints.Max.
func DeferMenuSurface(gtx layout.Context, tag event.Tag, anchor image.Point, minWidthDp int, content layout.Widget) layout.Dimensions {
	return DeferMenuSurfaceAt(gtx, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, content)
}

// MenuRow renders a single unified menu item: a leading check/icon gutter, a
// label, and an optional muted right-aligned shortcut. It fills the available
// width and highlights on hover. Disabled rows are dimmed and inert.
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
		// When an explicit Min.X is supplied (the menu's resolved uniform width)
		// the row fills it and the shortcut is pushed to the right edge. With
		// Min.X == 0 (the surface's natural-width measuring pass) the row shrinks
		// to its content so the menu sizes to the widest item, not the window.
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
	// Width follows the menu's resolved width (Min.X). During the natural-width
	// measuring pass Min.X is 0, so the divider contributes no width — it must
	// not stretch the menu to the full window width.
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
