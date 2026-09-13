package binview

import (
	"image"

	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type Picker struct {
	Mode Mode
	btns [modeCount]widget.Clickable
}

func (p *Picker) Btn(m Mode) *widget.Clickable {
	return &p.btns[m]
}

func (p *Picker) Update(gtx layout.Context) bool {
	changed := false
	for i := range p.btns {
		for p.btns[i].Clicked(gtx) {
			if p.Mode != Mode(i) {
				p.Mode = Mode(i)
				changed = true
			}
		}
	}
	return changed
}

func (p *Picker) Layout(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return p.LayoutSized(gtx, th, unit.Sp(11), layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)})
}

func (p *Picker) LayoutSized(gtx layout.Context, th *material.Theme, sz unit.Sp, inset layout.Inset) layout.Dimensions {
	children := make([]layout.FlexChild, 0, 2*modeCount)
	for i := range p.btns {
		m := Mode(i)
		clk := &p.btns[i]
		if i > 0 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return Chip(gtx, th, clk, m.Label(), p.Mode == m, sz, inset)
		}))
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
}

func Chip(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool, sz unit.Sp, inset layout.Inset) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := theme.BgField
		fg := th.Fg
		if on {
			bg = theme.BtnPrimary
			fg = theme.BtnPrimaryFg
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		pointer.CursorPointer.Add(gtx.Ops)
		macro := op.Record(gtx.Ops)
		dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, sz, label)
			lbl.Color = fg
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(3)))
		paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
		widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
		call.Add(gtx.Ops)
		return dims
	})
}

func Bar(gtx layout.Context, th *material.Theme, p *Picker, note string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := widgets.MonoLabel(th, unit.Sp(11), "binary")
				lbl.Color = theme.FgMuted
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.Layout(gtx, th)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if note == "" {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
				}
				return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(11), note)
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				})
			}),
		)
	})
}
