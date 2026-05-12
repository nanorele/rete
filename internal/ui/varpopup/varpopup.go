package varpopup

import (
	"image"
	"image/color"

	"tracto/internal/ui/environments"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
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

type State struct {
	Open        bool
	Name        string
	EnvID       string
	Editor      widget.Editor
	Range       struct{ Start, End int }
	SrcEditor   any
	Pos         f32.Point
	EnvBtn      widget.Clickable
	EnvMenuOpen bool
	EnvList     widget.List
	EnvClicks   []widget.Clickable

	tag struct{}
}

type Host struct {
	Theme        *material.Theme
	Window       *app.Window
	Environments *[]*environments.EnvironmentUI
	ActiveEnvID  *string

	ActiveEnvVar      func(name string) (string, bool)
	OnDismiss         func()
	OnSelectEnv       func(envID string)
	RefreshActiveEnv  func()
	SaveState         func()
}

func (s *State) OpenAt(name string, value string, srcEditor any, rng struct{ Start, End int }, pos f32.Point, envID string) {
	s.Open = true
	s.Name = name
	s.EnvID = envID
	s.Editor.SetText(value)
	s.Range = rng
	s.SrcEditor = srcEditor
	s.Pos = pos
	s.EnvMenuOpen = false
}

func (s *State) Close() {
	s.Open = false
	s.EnvMenuOpen = false
}

func (s *State) Layout(gtx layout.Context, host *Host) {
	if s == nil || !s.Open {
		return
	}
	popupW := gtx.Dp(unit.Dp(360))
	popupH := gtx.Dp(unit.Dp(180))
	if s.EnvMenuOpen {
		popupH = gtx.Dp(unit.Dp(340))
	}

	gap := gtx.Dp(unit.Dp(4))

	px := int(s.Pos.X)
	py := int(s.Pos.Y) + gap
	if px+popupW > gtx.Constraints.Max.X {
		px = gtx.Constraints.Max.X - popupW
	}
	if px < 0 {
		px = 0
	}
	if py+popupH > gtx.Constraints.Max.Y {
		py = int(s.Pos.Y) - popupH - gap
	}
	if py < 0 {
		py = 0
	}

	popupRect := image.Rect(px, py, px+popupW, py+popupH)

	layout.Stack{}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, color.NRGBA{A: 80}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			// Pass-through: see app.go popup-close backdrop. Without PassOp,
			// this full-screen press-catcher's non-pass hit-node short-
			// circuits Gio's cursor hit-test walk, leaving every widget
			// below at the unset → CursorDefault fallback.
			passStack := pointer.PassOp{}.Push(gtx.Ops)
			for {
				ev, ok := gtx.Event(pointer.Filter{
					Target: &s.tag,
					Kinds:  pointer.Press,
				})
				if !ok {
					break
				}
				pe, ok := ev.(pointer.Event)
				if !ok {
					continue
				}
				p := image.Pt(int(pe.Position.X), int(pe.Position.Y))
				if p.In(popupRect) {
					continue
				}
				if host.OnDismiss != nil {
					host.OnDismiss()
				}
				s.Open = false
				s.EnvMenuOpen = false
			}
			event.Op(gtx.Ops, &s.tag)
			passStack.Pop()
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			defer op.Offset(image.Pt(px, py)).Push(gtx.Ops).Pop()
			gtx.Constraints.Min = image.Pt(popupW, popupH)
			gtx.Constraints.Max = image.Pt(popupW, popupH)
			paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: image.Pt(popupW, popupH)}, 8).Op(gtx.Ops))
			widget.Border{Color: theme.BorderLight, CornerRadius: unit.Dp(8), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(popupW, popupH)}
			})
			return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(host.Theme, unit.Sp(13), "Variable: "+s.Name)
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						hint := "Value in active environment:"
						if *host.ActiveEnvID == "" {
							hint = "No environment selected — pick one below."
						}
						lbl := material.Label(host.Theme, unit.Sp(11), hint)
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.TextField(gtx, host.Theme, &s.Editor, "Value", true, nil, 0, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.layoutEnvSelect(gtx, host)
					}),
				)
			})
		}),
	)
}

func (s *State) layoutEnvSelect(gtx layout.Context, host *Host) layout.Dimensions {
	s.EnvList.Axis = layout.Vertical
	if s.EnvBtn.Clicked(gtx) {
		s.EnvMenuOpen = !s.EnvMenuOpen
	}

	currentName := "(no environment)"
	for _, e := range *host.Environments {
		if e.Data.ID == *host.ActiveEnvID {
			currentName = e.Data.Name
			break
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(host.Theme, unit.Sp(11), "Environment:")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &s.EnvBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(26)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				pointer.CursorPointer.Add(gtx.Ops)
				borderC := theme.BorderLight
				if s.EnvMenuOpen {
					borderC = theme.Accent
				}
				widget.Border{Color: borderC, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: size}
				})
				return layout.Inset{Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							lbl := material.Label(host.Theme, unit.Sp(12), currentName)
							lbl.MaxLines = 1
							lbl.Truncator = "…"
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min = image.Pt(gtx.Dp(14), gtx.Dp(14))
							return widgets.IconDropDown.Layout(gtx, theme.FgMuted)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !s.EnvMenuOpen {
				return layout.Dimensions{}
			}
			entries := len(*host.Environments) + 1
			if len(s.EnvClicks) < entries {
				s.EnvClicks = make([]widget.Clickable, entries)
			}
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				listH := gtx.Dp(unit.Dp(140))
				if gtx.Constraints.Max.Y < listH {
					listH = gtx.Constraints.Max.Y
				}
				gtx.Constraints.Max.Y = listH
				gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, listH)
				paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
				widget.Border{Color: theme.BorderLight, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: gtx.Constraints.Min}
				})
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return material.List(host.Theme, &s.EnvList).Layout(gtx, entries, func(gtx layout.Context, i int) layout.Dimensions {
						var envID, envName, preview string
						if i == 0 {
							envID = ""
							envName = "(no environment)"
						} else {
							e := (*host.Environments)[i-1]
							envID = e.Data.ID
							envName = e.Data.Name
							for _, v := range e.Data.Vars {
								if v.Key == s.Name && v.Value != "" {
									preview = v.Value
									break
								}
							}
						}
						for s.EnvClicks[i].Clicked(gtx) {
							if host.OnSelectEnv != nil {
								host.OnSelectEnv(envID)
							}
							if host.RefreshActiveEnv != nil {
								host.RefreshActiveEnv()
							}
							var val string
							if host.ActiveEnvVar != nil {
								val, _ = host.ActiveEnvVar(s.Name)
							}
							s.Editor.SetText(val)
							s.EnvID = envID
							s.EnvMenuOpen = false
							if host.SaveState != nil {
								host.SaveState()
							}
							if host.Window != nil {
								host.Window.Invalidate()
							}
						}
						isActive := *host.ActiveEnvID == envID
						return material.Clickable(gtx, &s.EnvClicks[i], func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Constraints.Max.X
							bg := theme.Transparent
							if isActive {
								bg = theme.AccentDim
							} else if s.EnvClicks[i].Hovered() {
								bg = theme.BgHover
							}
							rowH := gtx.Dp(unit.Dp(28))
							paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, rowH)}, 4).Op(gtx.Ops))
							pointer.CursorPointer.Add(gtx.Ops)
							return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(host.Theme, unit.Sp(12), envName)
										if isActive {
											lbl.Font.Weight = font.Bold
										}
										lbl.MaxLines = 1
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
									layout.Flexed(0.5, func(gtx layout.Context) layout.Dimensions {
										txt := preview
										if i == 0 {
											txt = ""
										} else if preview == "" {
											txt = "(undefined)"
										}
										lbl := material.Label(host.Theme, unit.Sp(11), txt)
										lbl.Color = theme.FgMuted
										lbl.MaxLines = 1
										lbl.Truncator = "…"
										return lbl.Layout(gtx)
									}),
								)
							})
						})
					})
				})
			})
		}),
	)
}
