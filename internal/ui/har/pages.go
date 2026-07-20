package har

import (
	"image"
	"strconv"
	"strings"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (s *Section) pagesView(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	pages := s.Doc.Pages
	if len(pages) == 0 {
		return centered(th, gtx, "No pages in this archive")
	}
	rows := len(pages) + 1
	for len(s.PageRows) < rows {
		s.PageRows = append(s.PageRows, &widget.Clickable{})
	}
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &s.PagesList).Layout(gtx, rows, func(gtx layout.Context, row int) layout.Dimensions {
			clk := s.PageRows[row]
			if row == 0 {
				for clk.Clicked(gtx) {
					s.selectPage("")
					s.TopTab = TabRequests
				}
				return pageRow(gtx, th, clk, "All pages", "", len(s.Doc.Entries), s.SelPageID == "")
			}
			p := pages[row-1]
			for clk.Clicked(gtx) {
				s.selectPage(p.ID)
				s.TopTab = TabRequests
			}
			title := p.Title
			if strings.TrimSpace(title) == "" {
				title = p.ID
			}
			return pageRow(gtx, th, clk, title, p.StartedDateTime, s.pageRequestCount(p.ID), s.SelPageID == p.ID)
		})
	})
}

func pageRow(gtx layout.Context, th *material.Theme, clk *widget.Clickable, title, when string, count int, selected bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowH := gtx.Dp(unit.Dp(30))
		gtx.Constraints.Min.Y = rowH
		bg := theme.Bg
		if selected {
			bg = theme.AccentDim
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, rowH)}.Op())
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), title)
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					if selected {
						lbl.Color = theme.Accent
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if when == "" {
						return layout.Dimensions{}
					}
					lbl := material.Label(th, unit.Sp(10), when)
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(70))
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					lbl := material.Label(th, unit.Sp(11), strconv.Itoa(count)+" req")
					lbl.Color = theme.FgMuted
					lbl.Alignment = text.End
					lbl.Font.Typeface = widgets.MonoTypeface
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}
