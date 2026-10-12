package sidebar

import (
	"image"
	"image/color"

	"rete/internal/ui/collections"
	"rete/internal/ui/environments"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func separatorContent(gtx layout.Context, lbl material.LabelStyle) layout.Dimensions {
	lbl.Text = " "
	lbl.Color = color.NRGBA{}
	return layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			return lbl.Layout(gtx)
		}),
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			w := gtx.Constraints.Max.X
			h := gtx.Constraints.Min.Y
			line := max(1, gtx.Dp(unit.Dp(1)))
			y := (h - line) / 2
			paint.FillShape(gtx.Ops, theme.DividerLight, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(w, y+line)}.Op())
			return layout.Dimensions{Size: image.Pt(w, h)}
		}),
	)
}

func separatorMenuItems(del *widget.Clickable) []widgets.MenuItem {
	return []widgets.MenuItem{{Label: "Delete", Click: del, Icon: widgets.IconDel, Danger: true}}
}

func countCollections(cols []*collections.CollectionUI) int {
	n := 0
	for _, c := range cols {
		if c != nil && c.Data != nil && !c.Data.IsSeparator() {
			n++
		}
	}
	return n
}

func countEnvironments(envs []*environments.EnvironmentUI) int {
	n := 0
	for _, e := range envs {
		if e != nil && !e.IsSeparator {
			n++
		}
	}
	return n
}

func countScripts(rows []*ScriptRow) int {
	n := 0
	for _, r := range rows {
		if r != nil && !r.IsSeparator {
			n++
		}
	}
	return n
}
