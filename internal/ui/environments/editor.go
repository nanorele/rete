package environments

import (
	"image"
	"strings"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
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

func (env *EnvironmentUI) editedColor() string {
	hex := strings.TrimSpace(env.ColorEditor.Text())
	if _, ok := theme.ParseHex(hex); ok {
		return hex
	}
	if hex == "" {
		return ""
	}
	return env.Data.HighlightColor
}

func (env *EnvironmentUI) editedVars() []model.EnvVar {
	var vars []model.EnvVar
	for _, r := range env.Rows {
		k := strings.TrimSpace(r.KeyEditor.Text())
		if k == "" {
			continue
		}
		vars = append(vars, model.EnvVar{
			Key:   k,
			Value: r.ValEditor.Text(),
		})
	}
	return vars
}

// editorRevs appends the revision of every editor backing this environment.
// EditorDirty runs on every frame and otherwise reads each row back with
// Editor.Text, which allocates a copy per field — thousands of allocations per
// frame on a large environment.
func (env *EnvironmentUI) editorRevs(dst []uint64) []uint64 {
	dst = append(dst, env.NameEditor.Revision(), env.ColorEditor.Revision())
	for _, r := range env.Rows {
		dst = append(dst, r.KeyEditor.Revision(), r.ValEditor.Revision())
	}
	return dst
}

func (env *EnvironmentUI) editorsUnchanged() bool {
	if len(env.dirtyRevs) != 2+2*len(env.Rows) {
		return false
	}
	if env.dirtyRevs[0] != env.NameEditor.Revision() || env.dirtyRevs[1] != env.ColorEditor.Revision() {
		return false
	}
	for i, r := range env.Rows {
		if env.dirtyRevs[2+2*i] != r.KeyEditor.Revision() ||
			env.dirtyRevs[3+2*i] != r.ValEditor.Revision() {
			return false
		}
	}
	return true
}

// InvalidateDirty forces the next EditorDirty to recompare, for when the
// underlying environment changes without the editors changing.
func (env *EnvironmentUI) InvalidateDirty() {
	if env != nil {
		env.dirtyValid = false
	}
}

func (env *EnvironmentUI) EditorDirty() bool {
	if env == nil || env.Data == nil {
		return false
	}
	if env.dirtyValid && env.editorsUnchanged() {
		return env.dirtyCached
	}
	dirty := env.computeDirty()
	env.dirtyRevs = env.editorRevs(env.dirtyRevs[:0])
	env.dirtyCached = dirty
	env.dirtyValid = true
	return dirty
}

func (env *EnvironmentUI) computeDirty() bool {
	if env.NameEditor.Text() != env.Data.Name {
		return true
	}
	if env.editedColor() != env.Data.HighlightColor {
		return true
	}
	vars := env.editedVars()
	if len(vars) != len(env.Data.Vars) {
		return true
	}
	for i, v := range vars {
		if v != env.Data.Vars[i] {
			return true
		}
	}
	return false
}

func (env *EnvironmentUI) Commit(onDirty func()) {
	if env == nil || env.Data == nil {
		return
	}
	env.Data.Name = env.NameEditor.Text()
	env.Data.HighlightColor = env.editedColor()
	env.Data.Vars = env.editedVars()
	env.dirtyValid = false
	_ = persist.SaveEnvironment(env.Data)
	if onDirty != nil {
		onDirty()
	}
}

func (env *EnvironmentUI) LayoutEditor(gtx layout.Context, host *EditorHost) layout.Dimensions {
	if env == nil {
		return layout.Dimensions{}
	}

	// Drain the editors before any click handler runs: Commit and EditorDirty
	// read Text(), which only reflects this frame's keystrokes once Update has
	// processed them. Handling Save first would compare stale text and drop the
	// edit when a keystroke and the click land in the same frame.
	for {
		if _, ok := env.NameEditor.Update(gtx); !ok {
			break
		}
	}
	for {
		if _, ok := env.ColorEditor.Update(gtx); !ok {
			break
		}
	}
	for _, r := range env.Rows {
		for {
			if _, ok := r.KeyEditor.Update(gtx); !ok {
				break
			}
		}
		for {
			if _, ok := r.ValEditor.Update(gtx); !ok {
				break
			}
		}
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
		if env.EditorDirty() {
			env.Commit(host.OnDirty)
		}
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
	dirty := env.EditorDirty()

	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()

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
							paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
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
							paint.FillShape(gtx.Ops, swatch, clip.Rect{Max: gtx.Constraints.Min}.Op())
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
							paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
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
							bg := theme.Border
							fg := theme.FgMuted
							if dirty {
								bg = theme.BtnPrimary
								fg = theme.BtnPrimaryFg
								if env.SaveBtn.Hovered() {
									bg = theme.Shade(theme.BtnPrimary, 0.12)
								}
							} else if env.SaveBtn.Hovered() {
								bg = theme.BorderLight
							}
							paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min = image.Pt(gtx.Dp(18), gtx.Dp(18))
								return widgets.IconSave.Layout(gtx, fg)
							})
						})
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				minKey := widgets.KVKeysMinWidth(gtx, th, &env.keyWidths, len(env.Rows), func(i int) *widget.Editor { return &env.Rows[i].KeyEditor })
				anyDrag := false
				for _, r := range env.Rows {
					widgets.KVRowDragPrepass(gtx, env.rowW, minKey, &env.KeyColW, &r.SplitDrag, &r.splitLX, &env.KeyColBelowMin)
					if r.SplitDrag.Dragging() {
						anyDrag = true
					}
				}
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
						env.rowW = gtx.Constraints.Max.X
						lineExtend := gtx.Dp(unit.Dp(4))
						if i == len(env.Rows)-1 {
							lineExtend = 0
						}
						return widgets.KVRow(gtx, th, &r.KeyEditor, &r.ValEditor, &r.DelBtn, &env.KeyColW, &r.SplitDrag, &r.splitLX, &env.KeyColBelowMin, minKey, nil, nil, nil,
							widgets.KVRowLine{Extend: lineExtend, Active: anyDrag})
					})
				})
			}),
		)
	})
}
