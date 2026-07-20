package mitm

import (
	"image"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (s *UIState) subHeader(gtx layout.Context) layout.Dimensions {
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			totalW := gtx.Constraints.Max.X
			// Adaptive: drop the Bind field first, then the Clear label, as the
			// bar narrows, so the essential controls + filter never fall off.
			showBind := totalW > gtx.Dp(unit.Dp(720))
			compact := totalW < gtx.Dp(unit.Dp(560))
			clearEnabled := s.Store.Len() > 0 || s.Proxy.WS.Len() > 0

			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.startBtn(gtx) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if compact {
						return iconOnlyBtn(gtx, s.host.Theme, &s.ClearBtn, widgets.IconClear, clearEnabled)
					}
					return btn(gtx, s.host.Theme, &s.ClearBtn, "Clear", widgets.IconClear, theme.Border, s.host.Theme.Fg, clearEnabled)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.viewSwitcher(gtx) }),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			}
			if showBind {
				children = append(children,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(s.host.Theme, unit.Sp(11), "Bind:")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(130))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(150))
						s.BindAddr.ReadOnly = s.Proxy.Running()
						return widgets.TextField(gtx, s.host.Theme, &s.BindAddr, "host:port", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				)
			}
			// Filter absorbs the remaining width and shrinks instead of pushing
			// controls off-screen.
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.filterBox(gtx)
				})
			}))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

// iconOnlyBtn is a compact square icon button used when the bar is narrow.
func iconOnlyBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, ic *widget.Icon, enabled bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := record(gtx)
		dims := layout.UniformInset(unit.Dp(7)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(16))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			col := th.Fg
			if !enabled {
				col = theme.FgDim
			}
			return ic.Layout(gtx, col)
		})
		call := macro.Stop()
		paint.FillShape(gtx.Ops, theme.Border, clip.UniformRRect(image.Rectangle{Max: dims.Size}, 3).Op(gtx.Ops))
		call.Add(gtx.Ops)
		if enabled {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return dims
	})
}

func (s *UIState) viewSwitcher(gtx layout.Context) layout.Dimensions {
	segs := []struct {
		clk   *widget.Clickable
		label string
		id    string
	}{
		{&s.SegInterc, "Intercept", ViewIntercept},
		{&s.SegHistory, "History", ViewHistory},
		{&s.SegWS, "WebSockets", ViewWebSockets},
	}
	children := make([]layout.FlexChild, 0, len(segs))
	for i := range segs {
		sg := segs[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return segBtn(gtx, s.host.Theme, sg.clk, sg.label, s.View == sg.id)
		}))
	}
	macro := record(gtx)
	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	call := macro.Stop()
	// border around the whole segmented control
	widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
	call.Add(gtx.Ops)
	return dims
}

func segBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := record(gtx)
		dims := layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), label)
			if active {
				lbl.Color = theme.BtnPrimaryFg
				lbl.Font.Weight = font.Bold
			} else {
				lbl.Color = theme.FgMuted
			}
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		if active {
			paint.FillShape(gtx.Ops, theme.BtnPrimary, clip.Rect{Max: dims.Size}.Op())
		} else if clk.Hovered() {
			paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: dims.Size}.Op())
		}
		call.Add(gtx.Ops)
		return dims
	})
}

func (s *UIState) filterBox(gtx layout.Context) layout.Dimensions {
	w := gtx.Constraints.Max.X
	if w > gtx.Dp(unit.Dp(300)) {
		w = gtx.Dp(unit.Dp(300))
	}
	if w < gtx.Dp(unit.Dp(90)) {
		w = gtx.Dp(unit.Dp(90))
	}
	gtx.Constraints.Min.X = w
	gtx.Constraints.Max.X = w
	macro := record(gtx)
	dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := gtx.Dp(unit.Dp(14))
				gtx.Constraints.Min = image.Pt(s, s)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widgets.IconSearch.Layout(gtx, theme.FgMuted)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(s.host.Theme, &s.Filter, "filter flows…")
				ed.TextSize = unit.Sp(12)
				return ed.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.Filter.Text() == "" {
					return layout.Dimensions{}
				}
				return s.FilterClr.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(14))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					return widgets.IconClose.Layout(gtx, theme.FgMuted)
				})
			}),
		)
	})
	call := macro.Stop()
	paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(image.Rectangle{Max: dims.Size}, 3).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
	return dims
}
