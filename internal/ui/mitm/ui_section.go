package mitm

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"
	"time"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type Host struct {
	Theme  *material.Theme
	Window *app.Window

	Elevate func(banner *string, extraArg string)
}

// Layout renders zones C (central) and D (inspector) plus the module
// sub-header. Zone B (accordion) is rendered by LayoutSidebar in the shared
// sidebar; the top-bar proxy status lives in the title bar.
func (s *UIState) Layout(gtx layout.Context, host *Host) layout.Dimensions {
	s.host = host
	s.Ensure()
	s.wireNotify()
	s.consumeStartupFlags()
	s.handleEvents(gtx)
	s.flushConfig()

	paint.FillShape(gtx.Ops, s.host.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())

	body := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.subHeader(gtx) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return s.body(gtx) }),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.statusBar(gtx) }),
	)

	s.trackPointer(gtx)

	// Overlays.
	if s.ClearConfirmOpen {
		s.clearConfirm(gtx)
	}
	if s.CtxOpen {
		s.rowContextMenu(gtx)
	}
	if s.AnnotateOpen {
		s.annotatePopup(gtx)
	}
	return body
}

// trackPointer records the pointer position in section-local coordinates so
// overlays (context menu, popups) can anchor at the cursor.
func (s *UIState) trackPointer(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &s.PtrTag, Kinds: pointer.Move | pointer.Press | pointer.Drag})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok {
			s.LocalPtr.X = int(pe.Position.X)
			s.LocalPtr.Y = int(pe.Position.Y)
		}
	}
	pass := pointer.PassOp{}.Push(gtx.Ops)
	defer pass.Pop()
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &s.PtrTag)
}

func (s *UIState) wireNotify() {
	if !s.NotifySet {
		s.NotifySet = true
		inv := func() {
			if s.host.Window != nil {
				s.host.Window.Invalidate()
			}
		}
		if s.Store != nil {
			s.Store.SetNotify(inv)
		}
		if s.Proxy != nil {
			if s.Proxy.WS != nil {
				s.Proxy.WS.SetNotify(inv)
			}
			if s.Proxy.Targets != nil {
				s.Proxy.Targets.SetNotify(inv)
			}
			if s.Proxy.Manual != nil {
				s.Proxy.Manual.SetNotify(inv)
			}
		}
	}
	if !s.TrustNotifySet {
		s.TrustNotifySet = true
		SetTrustRefreshNotify(func() {
			if s.host.Window != nil {
				s.host.Window.Invalidate()
			}
		})
	}
}

// body lays out C | splitter | D, honouring the inspector-collapsed state.
func (s *UIState) body(gtx layout.Context) layout.Dimensions {
	center := func(gtx layout.Context) layout.Dimensions {
		switch s.View {
		case ViewIntercept:
			return s.interceptView(gtx)
		case ViewWebSockets:
			return s.wsView(gtx)
		default:
			return s.historyView(gtx)
		}
	}

	if s.InspectorCollapsed {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Flexed(1, center),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.collapsedInspectorBar(gtx) }),
		)
	}

	totalW := gtx.Constraints.Max.X
	handleW := gtx.Dp(unit.Dp(6))
	minD := gtx.Dp(unit.Dp(280))
	maxD := gtx.Dp(unit.Dp(560))
	minC := gtx.Dp(unit.Dp(260))

	clampLeft := func(w int) int {
		if w > totalW-handleW-minD {
			w = totalW - handleW - minD
		}
		if w < totalW-handleW-maxD {
			w = totalW - handleW - maxD
		}
		if w < minC {
			w = minC
		}
		return w
	}
	leftFromRatio := func() int { return clampLeft(int(float32(totalW)*s.SplitRatio) - handleW/2) }

	var moved bool
	var finalX float32
	for {
		e, ok := s.SplitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		pos := e.Position.X + float32(s.LeftDrawn)
		switch e.Kind {
		case pointer.Press:
			s.SplitDragX = pos
			s.SplitPx = float32(leftFromRatio())
		case pointer.Drag:
			finalX = pos
			moved = true
		}
	}
	if moved && totalW > 0 {
		s.SplitPx += finalX - s.SplitDragX
		s.SplitDragX = finalX
		left := clampLeft(int(s.SplitPx + 0.5))
		// Drag past the far right collapses the inspector.
		if int(s.SplitPx+0.5) > totalW-handleW-minD+gtx.Dp(unit.Dp(40)) {
			s.InspectorCollapsed = true
			s.MarkDirty()
		}
		s.SplitRatio = (float32(left) + float32(handleW)/2) / float32(totalW)
		s.MarkDirty()
		s.host.Window.Invalidate()
	}

	leftW := leftFromRatio()
	rightW := totalW - leftW - handleW

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			d := center(gtx)
			d.Size.X = leftW
			s.LeftDrawn = leftW
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Constraints.Max.Y
			size := image.Pt(handleW, h)
			line := gtx.Dp(unit.Dp(1))
			lineCol := theme.Border
			if s.SplitDrag.Dragging() {
				lineCol = theme.Accent
			}
			paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt((handleW-line)/2, 0), Max: image.Pt((handleW-line)/2+line, h)}.Op())
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			pointer.CursorColResize.Add(gtx.Ops)
			s.SplitDrag.Add(gtx.Ops)
			event.Op(gtx.Ops, &s.SplitDrag)
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return s.inspector(gtx)
		}),
	)
}

// collapsedInspectorBar is a thin strip with an expand chevron.
func (s *UIState) collapsedInspectorBar(gtx layout.Context) layout.Dimensions {
	w := gtx.Dp(unit.Dp(24))
	h := gtx.Constraints.Max.Y
	gtx.Constraints.Min = image.Pt(w, h)
	gtx.Constraints.Max = image.Pt(w, h)
	for s.InspectorToggle.Clicked(gtx) {
		s.InspectorCollapsed = false
		s.MarkDirty()
	}
	return s.InspectorToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(w, h)}.Op())
		line := gtx.Dp(unit.Dp(1))
		paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: image.Pt(line, h)}.Op())
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(16))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			return widgets.IconChevronL.Layout(gtx, theme.FgMuted)
		})
	})
}

func (s *UIState) statusBar(gtx layout.Context) layout.Dimensions {
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			var msg string
			col := theme.FgMuted
			switch {
			case s.StatusBanner != "":
				msg = s.StatusBanner
				if strings.Contains(strings.ToLower(msg), "administrator") || strings.HasPrefix(msg, "Start failed") {
					col = theme.Danger
				} else if strings.HasPrefix(msg, "Proxy listening") {
					col = theme.MethodGet
				}
			case s.Proxy.Running():
				mode := "tunnel"
				if s.Proxy.Intercepting() {
					mode = "decrypting HTTPS"
				}
				msg = "Proxy: " + s.Proxy.Addr() + "  •  " + mode + "  •  flows=" + fmt.Sprintf("%d", s.Store.Len())
				if held := s.Proxy.Manual.Len(); held > 0 {
					msg += "  •  intercept queue=" + fmt.Sprintf("%d", held)
				}
				col = theme.MethodGet
			case !IsAdmin():
				msg = "Not elevated — restart as administrator to enable a system-wide proxy"
			default:
				msg = "Proxy idle"
			}
			lbl := material.Label(s.host.Theme, unit.Sp(11), msg)
			lbl.Color = col
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
	})
}

// ---------------------------------------------------------------------------
// Start / CA buttons and helpers (kept from the original section)
// ---------------------------------------------------------------------------

func (s *UIState) startBtn(gtx layout.Context) layout.Dimensions {
	admin := IsAdmin()
	running := s.Proxy.Running()

	bg := theme.BtnPrimary
	fg := theme.BtnPrimaryFg
	label := "Start"
	useUAC := false
	ic := widgets.IconPlay
	switch {
	case running:
		bg = theme.Cancel
		fg = theme.DangerFg
		label = "Stop"
		ic = widgets.IconStop
	case !admin:
		useUAC = true
	}

	return s.StartBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := record(gtx)
			dims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := gtx.Dp(unit.Dp(14))
						if useUAC {
							return paintUACShield(gtx, s)
						}
						gtx.Constraints.Min = image.Pt(s, s)
						gtx.Constraints.Max = gtx.Constraints.Min
						return ic.Layout(gtx, fg)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(s.host.Theme, unit.Sp(12), label)
						lbl.Color = fg
						lbl.Font.Weight = font.Bold
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			call.Add(gtx.Ops)
			pointer.CursorPointer.Add(gtx.Ops)
			return dims
		})
	})
}

func (s *UIState) consumeStartupFlags() {
	if s.AutoStart {
		s.AutoStart = false
		if IsAdmin() && !s.Proxy.Running() {
			addr := strings.TrimSpace(s.BindAddr.Text())
			if err := s.Proxy.Start(addr); err != nil {
				s.StatusBanner = "Auto-start failed: " + err.Error()
			} else {
				s.StatusBanner = "Proxy listening on " + s.Proxy.Addr() + " (auto-started after elevation)"
			}
		}
	}
	if s.AutoInstallCA {
		s.AutoInstallCA = false
		ca := s.Proxy.CA()
		if ca == nil {
			if gen, err := GenerateCA(); err == nil {
				if err := gen.Save(MITMDir()); err == nil {
					s.Proxy.SetCA(gen)
					ca = gen
					s.CABanner = "CA generated • " + gen.Fingerprint()
				}
			}
		}
		if ca != nil && IsAdmin() {
			if err := InstallTrust(CACertPath(MITMDir())); err != nil {
				s.CABanner = "Install failed: " + err.Error()
			} else {
				s.CABanner = "CA installed into Windows trust (after elevation) • Firefox needs manual import — see \"Import guide\""
				s.HelpOpen = true
			}
		}
	}
	if s.AutoRemoveCA {
		s.AutoRemoveCA = false
		if IsAdmin() {
			if err := UninstallTrust(); err != nil {
				s.CABanner = "Remove failed: " + err.Error()
			} else {
				s.CABanner = "CA removed from trust store (after elevation)"
			}
		}
	}
}

func shortFingerprint(fp string) string {
	if len(fp) <= 17 {
		return fp
	}
	return fp[:8] + "…" + fp[len(fp)-8:]
}

func genLabel(ca *CA) string {
	if ca == nil {
		return "Generate CA"
	}
	return "Regenerate"
}

func statusLine(f *Flow) string {
	var parts []string
	if f.Status != "" {
		parts = append(parts, f.Status)
	}
	if f.ReqSize > 0 || f.RespSize > 0 {
		parts = append(parts, fmt.Sprintf("req %s  resp %s", humanSize(f.ReqSize), humanSize(f.RespSize)))
	}
	parts = append(parts, humanDuration(f))
	return strings.Join(parts, "  •  ")
}

func humanSize(n int64) string {
	switch {
	case n < 0:
		return "-"
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fK", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/(1024*1024))
	}
}

func humanDuration(f *Flow) string {
	if f.Started.IsZero() {
		return "-"
	}
	end := f.Ended
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(f.Started)
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

// btn is the shared labelled/icon button used across the module.
func btn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, ic *widget.Icon, bg, fg color.NRGBA, enabled bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := record(gtx)
			if !enabled {
				bg = theme.Mix(bg, theme.Bg, 0.6)
			}
			dims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{}
				col := fg
				if !enabled {
					col = theme.FgDim
				}
				if ic != nil {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(s, s)
						gtx.Constraints.Max = gtx.Constraints.Min
						return ic.Layout(gtx, col)
					}))
					children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), label)
					lbl.Color = col
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			call.Add(gtx.Ops)
			if enabled {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return dims
		})
	})
}

func adminBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, enabled bool, useUAC bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := record(gtx)
			fg := th.Fg
			if !enabled {
				fg = theme.FgDim
			}
			dims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{}
				if useUAC && enabled {
					children = append(children,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return paintUACShield(gtx, gtx.Dp(unit.Dp(14)))
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					)
				}
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), label)
					lbl.Color = fg
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, theme.Border, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			call.Add(gtx.Ops)
			if enabled {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			return dims
		})
	})
}

func tab(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), label)
			if active {
				lbl.Color = theme.Accent
				lbl.Font.Weight = font.Bold
			} else {
				lbl.Color = theme.FgMuted
			}
			dims := lbl.Layout(gtx)
			if active {
				h := gtx.Dp(unit.Dp(2))
				paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Min: image.Pt(0, dims.Size.Y+gtx.Dp(unit.Dp(4))), Max: image.Pt(dims.Size.X, dims.Size.Y+gtx.Dp(unit.Dp(4))+h)}.Op())
			}
			return dims
		})
	})
}

// copyText writes text to the clipboard.
func copyText(gtx layout.Context, text string) {
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
}
