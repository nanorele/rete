package environments

import (
	"image"
	"strings"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type EditorHost struct {
	Theme              *material.Theme
	Window             *app.Window
	OnClose            func()
	OnDirty            func()
	OnColorSwatchClick func(env *EnvironmentUI)
}

func (env *EnvironmentUI) Commit(onDirty func()) {
	if env == nil || env.Data == nil {
		return
	}
	env.Data.Name = env.NameEditor.Text()
	hex := strings.TrimSpace(env.ColorEditor.Text())
	if _, ok := theme.ParseHex(hex); ok {
		env.Data.HighlightColor = hex
	} else if hex == "" {
		env.Data.HighlightColor = ""
	}
	env.Data.Vars = nil
	for _, r := range env.Rows {
		k := strings.TrimSpace(r.KeyEditor.Text())
		v := r.ValEditor.Text()
		if k == "" {
			continue
		}
		env.Data.Vars = append(env.Data.Vars, model.EnvVar{
			Key:   k,
			Value: v,
		})
	}
	_ = persist.SaveEnvironment(env.Data)
	if onDirty != nil {
		onDirty()
	}
}

func (env *EnvironmentUI) LayoutEditor(gtx layout.Context, host *EditorHost) layout.Dimensions {
	if env == nil {
		return layout.Dimensions{}
	}

	if env.BackBtn.Clicked(gtx) {
		env.Commit(host.OnDirty)
		if host.OnClose != nil {
			host.OnClose()
		}
		if host.Window != nil {
			host.Window.Invalidate()
		}
		return layout.Dimensions{}
	}
	for env.AddBtn.Clicked(gtx) {
		r := &EnvVarRow{}
		env.Rows = append(env.Rows, r)
		if host.Window != nil {
			host.Window.Invalidate()
		}
	}
	for env.ColorReset.Clicked(gtx) {
		env.ColorEditor.SetText("")
		env.Data.HighlightColor = ""
		if host.Window != nil {
			host.Window.Invalidate()
		}
	}
	for env.ColorSwatchBtn.Clicked(gtx) {
		if host.OnColorSwatchClick != nil {
			host.OnColorSwatchClick(env)
		}
		if host.Window != nil {
			host.Window.Invalidate()
		}
	}
	for env.SaveBtn.Clicked(gtx) {
		env.Commit(host.OnDirty)
		if host.Window != nil {
			host.Window.Invalidate()
		}
	}
	for i := 0; i < len(env.Rows); i++ {
		if env.Rows[i].DelBtn.Clicked(gtx) {
			widgets.ResetEditorHScroll(&env.Rows[i].KeyEditor)
			widgets.ResetEditorHScroll(&env.Rows[i].ValEditor)
			env.Rows = append(env.Rows[:i], env.Rows[i+1:]...)
			i--
			if host.Window != nil {
				host.Window.Invalidate()
			}
		}
	}

	th := host.Theme

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.BackBtn, func(gtx layout.Context) layout.Dimensions {
							bg := theme.Border
							if env.BackBtn.Hovered() {
								bg = theme.BorderLight
							}
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
										return widgets.IconBack.Layout(gtx, th.Fg)
									}),
								)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return widgets.TextField(gtx, th, &env.NameEditor, "Environment Name", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						sw := gtx.Dp(unit.Dp(28))
						gtx.Constraints.Min = image.Pt(sw, sw)
						gtx.Constraints.Max = gtx.Constraints.Min
						return material.Clickable(gtx, &env.ColorSwatchBtn, func(gtx layout.Context) layout.Dimensions {
							swatch := HighlightColor(env.Data)
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							paint.FillShape(gtx.Ops, swatch, rect.Op(gtx.Ops))
							borderCol := theme.Border
							if env.ColorSwatchBtn.Hovered() {
								borderCol = theme.BorderLight
							}
							widgets.PaintBorder1px(gtx, gtx.Constraints.Min, borderCol)
							return layout.Dimensions{Size: gtx.Constraints.Min}
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(90))
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return widgets.TextField(gtx, th, &env.ColorEditor, "#hex", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						sz := gtx.Dp(unit.Dp(22))
						gtx.Constraints.Min = image.Pt(sz, sz)
						gtx.Constraints.Max = gtx.Constraints.Min
						return material.Clickable(gtx, &env.ColorReset, func(gtx layout.Context) layout.Dimensions {
							bg := theme.BgField
							if env.ColorReset.Hovered() {
								bg = theme.BgHover
							}
							paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 3).Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								isz := gtx.Dp(unit.Dp(14))
								gtx.Constraints.Min = image.Pt(isz, isz)
								gtx.Constraints.Max = gtx.Constraints.Min
								return widgets.IconRefresh.Layout(gtx, theme.FgMuted)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return material.Clickable(gtx, &env.SaveBtn, func(gtx layout.Context) layout.Dimensions {
							size := gtx.Dp(28)
							gtx.Constraints.Min = image.Pt(size, size)
							gtx.Constraints.Max = gtx.Constraints.Min
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4)
							bg := theme.BtnPrimary
							if env.SaveBtn.Hovered() {
								bg = theme.Shade(theme.BtnPrimary, 0.12)
							}
							paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min = image.Pt(gtx.Dp(18), gtx.Dp(18))
								return widgets.IconSave.Layout(gtx, theme.BtnPrimaryFg)
							})
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				minKey := widgets.KVKeysMinWidth(gtx, th, len(env.Rows), func(i int) *widget.Editor { return &env.Rows[i].KeyEditor })
				return material.List(th, &env.List).Layout(gtx, len(env.Rows)+1, func(gtx layout.Context, i int) layout.Dimensions {
					if i == len(env.Rows) {
						return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							btn := widgets.FilledButton(th, &env.AddBtn, "+ Add Variable", theme.Border, th.Fg)
							btn.Inset = layout.UniformInset(unit.Dp(8))
							return btn.Layout(gtx)
						})
					}

					r := env.Rows[i]
					return layout.Inset{Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return widgets.KVRow(gtx, th, &r.KeyEditor, &r.ValEditor, &r.DelBtn, &env.KeyColW, &r.SplitDrag, &r.splitLX, &env.KeyColBelowMin, minKey, nil)
					})
				})
			}),
		)
	})
}
