package mitm

import (
	"fmt"
	"image"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func (s *UIState) interceptView(gtx layout.Context) layout.Dimensions {
	queue := s.Proxy.Manual.Queue()

	// keep the switch in sync with backend state
	s.InterceptSwitch.Value = s.Proxy.Manual.On()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.interceptBar(gtx, len(queue)) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !s.Proxy.Manual.On() {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(12), "Interception is off — traffic passes through and is logged to History")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				})
			}
			if len(queue) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(12), "Waiting for a matching message…")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				})
			}
			return s.heldEditorView(gtx, queue[0])
		}),
	)
}

func (s *UIState) interceptBar(gtx layout.Context, queueLen int) layout.Dimensions {
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Switch(s.host.Theme, &s.InterceptSwitch, "intercept").Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(12), "Intercept")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(20)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Switch(s.host.Theme, &s.InterceptRespSw, "responses").Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), "intercept responses")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), fmt.Sprintf("queue: %d", queueLen))
					col := theme.FgMuted
					if queueLen > 0 {
						col = theme.MethodPost
					}
					lbl.Color = col
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func (s *UIState) heldEditorView(gtx layout.Context, head *Held) layout.Dimensions {
	// Load the raw bytes into the editor once per held item.
	if s.HeldEditorFor != head.ID {
		s.HeldEditorFor = head.ID
		s.HeldEditor.SetText(string(head.Raw))
		s.HeldEditor.SingleLine = false
	}
	kind := "REQUEST"
	if head.Kind == HeldResponse {
		kind = "RESPONSE"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(11), fmt.Sprintf("%s  ·  %s  %s", kind, head.Method, head.URL))
				lbl.Color = theme.Accent
				lbl.Font.Typeface = widgets.MonoTypeface
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			})
		}),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(s.host.Theme, &s.HeldEditor, "")
						ed.TextSize = unit.Sp(11)
						ed.Font.Typeface = widgets.MonoTypeface
						return ed.Layout(gtx)
					})
				})
			})
		}),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.ForwardBtn, "Forward", widgets.IconNext, theme.BtnPrimary, theme.BtnPrimaryFg, true)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.DropBtn, "Drop", widgets.IconClose, theme.Cancel, theme.DangerFg, true)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.ActionBtn, "Action ▾", nil, theme.Border, s.host.Theme.Fg, true)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(s.host.Theme, unit.Sp(10), "edit the message above, then Forward")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
					)
				})
			})
		}),
	)
}
