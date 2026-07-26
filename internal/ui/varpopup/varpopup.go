package varpopup

import (
	"image"
	"time"

	"tracto/internal/ui/environments"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

const hoverGrace = 80 * time.Millisecond

const previewMaxRunes = 24

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

	Hover     widgets.Hover
	orig      string
	lastHover time.Time

	tag      struct{}
	valueTag struct{}
}

type Host struct {
	Theme        *material.Theme
	Window       *app.Window
	Environments *[]*environments.EnvironmentUI
	ActiveEnvID  *string

	ActiveEnvVar     func(name string) (string, bool)
	ChipHovered      func() bool
	OnDismiss        func()
	OnSelectEnv      func(envID string)
	RefreshActiveEnv func()
	SaveState        func()
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
	s.orig = value
	s.lastHover = time.Time{}
}

func (s *State) Close() {
	s.Open = false
	s.EnvMenuOpen = false
}

func (s *State) Changed() bool {
	return s.Open && s.Editor.Text() != s.orig
}

func (s *State) dismiss(host *Host, save bool) {
	if save && host.OnDismiss != nil {
		host.OnDismiss()
	}
	s.Open = false
	s.EnvMenuOpen = false
	if host.Window != nil {
		host.Window.Invalidate()
	}
}

func (s *State) Layout(gtx layout.Context, host *Host) {
	if s == nil || !s.Open {
		return
	}
	for {
		ev, ok := s.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			s.dismiss(host, true)
			return
		}
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Name: key.NameEscape},
			key.Filter{Name: key.NameReturn},
			key.Filter{Name: key.NameEnter},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameEscape, key.NameReturn, key.NameEnter:
				s.dismiss(host, true)
				return
			}
		}
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &s.valueTag, Kinds: pointer.Press})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok && s.EnvMenuOpen {
			s.EnvMenuOpen = false
			if host.Window != nil {
				host.Window.Invalidate()
			}
		}
	}

	if s.EnvBtn.Clicked(gtx) {
		s.EnvMenuOpen = !s.EnvMenuOpen
		if host.Window != nil {
			host.Window.Invalidate()
		}
	}

	hovered := s.Hover.Update(gtx.Source)
	if host.ChipHovered != nil && host.ChipHovered() {
		hovered = true
	}
	if gtx.Focused(&s.Editor) {
		hovered = true
	}
	if hovered || s.lastHover.IsZero() {
		s.lastHover = gtx.Now
	}
	if gtx.Now.Sub(s.lastHover) > hoverGrace {
		s.dismiss(host, s.Editor.Text() != s.orig)
		return
	}
	gtx.Execute(op.InvalidateCmd{At: s.lastHover.Add(hoverGrace)})

	popupW := gtx.Dp(unit.Dp(360))
	maxPopupH := gtx.Dp(unit.Dp(420))
	if maxPopupH > gtx.Constraints.Max.Y {
		maxPopupH = gtx.Constraints.Max.Y
	}

	cGtx := gtx
	cGtx.Constraints.Min = image.Pt(popupW, 0)
	cGtx.Constraints.Max = image.Pt(popupW, maxPopupH)
	contentMacro := op.Record(gtx.Ops)
	contentDims := layout.UniformInset(unit.Dp(12)).Layout(cGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				hint := "Value of {{" + s.Name + "}} in active environment:"
				if *host.ActiveEnvID == "" {
					hint = "Value of {{" + s.Name + "}} — no environment selected, pick one below."
				}
				lbl := material.Label(host.Theme, unit.Sp(11), hint)
				lbl.Color = theme.FgMuted
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				dims := widgets.TextField(gtx, host.Theme, &s.Editor, "Value", true, nil, 0, unit.Sp(12))
				if s.EnvMenuOpen {
					pass := pointer.PassOp{}.Push(gtx.Ops)
					cl := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
					event.Op(gtx.Ops, &s.valueTag)
					cl.Pop()
					pass.Pop()
				}
				return dims
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.layoutEnvSelect(gtx, host)
			}),
		)
	})
	contentCall := contentMacro.Stop()

	popupH := contentDims.Size.Y
	popupSize := image.Pt(popupW, popupH)

	px := int(s.Pos.X)
	py := int(s.Pos.Y)
	if px+popupW > gtx.Constraints.Max.X {
		px = gtx.Constraints.Max.X - popupW
	}
	if px < 0 {
		px = 0
	}
	if py+popupH > gtx.Constraints.Max.Y {
		py = int(s.Pos.Y) - popupH
	}
	if py < 0 {
		py = 0
	}

	macro := op.Record(gtx.Ops)
	off := op.Offset(image.Pt(px, py)).Push(gtx.Ops)

	widgets.MenuShadow(gtx, popupSize)
	widgets.PaintPopupSurface(gtx, popupSize, 8, theme.BgMenu, widgets.MenuBorderColor())

	blockClip := clip.Rect{Max: popupSize}.Push(gtx.Ops)
	event.Op(gtx.Ops, &s.tag)
	pointer.CursorDefault.Add(gtx.Ops)
	blockClip.Pop()

	contentCall.Add(gtx.Ops)

	hoverClip := clip.Rect{Max: popupSize}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	s.Hover.Add(gtx.Ops)
	pass.Pop()
	hoverClip.Pop()

	off.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

func (s *State) layoutEnvSelect(gtx layout.Context, host *Host) layout.Dimensions {
	s.EnvList.Axis = layout.Vertical

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
				paint.FillShape(gtx.Ops, theme.BgField, clip.Rect{Max: size}.Op())
				pointer.CursorPointer.Add(gtx.Ops)
				borderC := theme.BorderLight
				if s.EnvMenuOpen {
					borderC = theme.Accent
				}
				widgets.PaintBorder1px(gtx, size, borderC)
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
			entries := len(*host.Environments) + 1
			if len(s.EnvClicks) < entries {
				s.EnvClicks = make([]widget.Clickable, entries)
			}
			menuWasOpen := s.EnvMenuOpen
			for i := 0; i < entries; i++ {
				envID := ""
				if i > 0 {
					envID = (*host.Environments)[i-1].Data.ID
				}
				for s.EnvClicks[i].Clicked(gtx) {
					if !menuWasOpen {
						continue
					}
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
					s.orig = val
					s.EnvID = envID
					s.EnvMenuOpen = false
					if host.SaveState != nil {
						host.SaveState()
					}
					if host.Window != nil {
						host.Window.Invalidate()
					}
				}
			}
			if !s.EnvMenuOpen {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				maxListH := gtx.Dp(unit.Dp(140))
				if gtx.Constraints.Max.Y < maxListH {
					maxListH = gtx.Constraints.Max.Y
				}
				gtx.Constraints.Max.Y = maxListH
				gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
				listMacro := op.Record(gtx.Ops)
				listDims := layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
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
						isActive := *host.ActiveEnvID == envID
						previewTxt := preview
						if i == 0 {
							previewTxt = ""
						} else if preview == "" {
							previewTxt = "(undefined)"
						}
						if r := []rune(previewTxt); len(r) > previewMaxRunes {
							previewTxt = string(r[:previewMaxRunes]) + "…"
						}
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return widgets.MenuRow(gtx, host.Theme, widgets.MenuItem{
							Label:    envName,
							Shortcut: previewTxt,
							Click:    &s.EnvClicks[i],
							Checked:  isActive,
							Bold:     isActive,
						})
					})
				})
				listCall := listMacro.Stop()
				paint.FillShape(gtx.Ops, theme.BgField, clip.Rect{Max: listDims.Size}.Op())
				widgets.PaintBorder1px(gtx, listDims.Size, theme.BorderLight)
				listCall.Add(gtx.Ops)
				return listDims
			})
		}),
	)
}
