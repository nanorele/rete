package mitm

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

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

// MITM sidebar palette, matched to the shared sidebar: headers and section
// bodies sit on the sidebar background, separated only by hairlines, and form
// cards are raised above it.
func secHeaderBg() color.NRGBA { return theme.BgDark }
func bodyBg() color.NRGBA      { return theme.BgDark }

// groupBg raises form cards a touch above the section body, staying below the
// border tone so the card outline still reads.
func groupBg() color.NRGBA { return theme.Mix(bodyBg(), theme.BgField, 0.28) }

// LayoutSidebar renders zone B: the accordion of MITM tool sections.
func (s *UIState) LayoutSidebar(gtx layout.Context, host *Host) layout.Dimensions {
	s.host = host
	s.Ensure()
	s.wireNotify()
	s.sidebarEvents(gtx)
	s.flushConfig()
	s.SidebarList.Axis = layout.Vertical

	paint.FillShape(gtx.Ops, bodyBg(), clip.Rect{Max: gtx.Constraints.Max}.Op())

	var rows []layout.Widget
	rows = append(rows, s.secTargets()...)
	rows = append(rows, s.secTLS()...)
	rows = append(rows, s.secIRules()...)
	rows = append(rows, s.secMR()...)
	rows = append(rows, s.secScope()...)

	// Use the embedded layout.List (not material.List) so rows span the full
	// width — material.List reserves a scrollbar gutter that leaves a visible
	// gap on the right of the header bars. Wheel/drag scrolling still works.
	return s.SidebarList.List.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i](gtx)
	})
}

// ---------------------------------------------------------------------------
// shared building blocks
// ---------------------------------------------------------------------------

// secHeader mirrors the HTTP request pane section headers (Headers / Params /
// …): a bold mono title on the left, the collapse chevron as a square button on
// the right, and the same paddings and hairline.
func (s *UIState) secHeader(clk *widget.Clickable, title, count string, open *bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X
		macro := record(gtx)
		dims := layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(s.host.Theme, unit.Sp(12), title)
						lbl.Font.Weight = font.Bold
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					})
				}),
				layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if count == "" || count == "0" {
						return layout.Dimensions{}
					}
					if isNumeric(count) {
						return countBadge(gtx, s.host.Theme, count)
					}
					lbl := material.Label(s.host.Theme, unit.Sp(10), count)
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					ic := widgets.IconExpandMore
					if *open {
						ic = widgets.IconExpandLess
					}
					return widgets.SquareBtn(gtx, clk, ic, s.host.Theme)
				}),
			)
		})
		call := macro.Stop()

		h := dims.Size.Y
		paint.FillShape(gtx.Ops, secHeaderBg(), clip.Rect{Max: image.Pt(w, h)}.Op())
		call.Add(gtx.Ops)
		line := gtx.Dp(unit.Dp(1))
		paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, h-line), Max: image.Pt(w, h)}.Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// countBadge draws a small pill with the active-item count.
func countBadge(gtx layout.Context, th *material.Theme, count string) layout.Dimensions {
	macro := record(gtx)
	dims := layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(10), count)
		lbl.Color = theme.BtnPrimaryFg
		return lbl.Layout(gtx)
	})
	call := macro.Stop()
	paint.FillShape(gtx.Ops, theme.Accent, clip.UniformRRect(image.Rectangle{Max: dims.Size}, dims.Size.Y/2).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

func pad(w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, w)
	}
}

// listRowDivider is the thin separator drawn between consecutive item rows in
// the accordion lists, matching the HTTP KV-row dividers (BorderLight), inset
// to align with the row content.
func listRowDivider(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		h := gtx.Dp(unit.Dp(1))
		w := gtx.Constraints.Max.X
		paint.FillShape(gtx.Ops, theme.BorderLight, clip.Rect{Max: image.Pt(w, h)}.Op())
		return layout.Dimensions{Size: image.Pt(w, h)}
	})
}

// fieldLabel is a small uppercase caption placed above an input/control.
func (s *UIState) fieldLabel(txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(s.host.Theme, unit.Sp(9), strings.ToUpper(txt))
		lbl.Color = theme.FgMuted
		lbl.Font.Weight = font.Bold
		return layout.Inset{Bottom: unit.Dp(3)}.Layout(gtx, lbl.Layout)
	}
}

// group wraps content in a subtly recessed card so a form reads as one unit.
func group(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := record(gtx)
	dims := layout.UniformInset(unit.Dp(8)).Layout(gtx, w)
	call := macro.Stop()
	sz := image.Pt(gtx.Constraints.Max.X, dims.Size.Y)
	paint.FillShape(gtx.Ops, groupBg(), clip.UniformRRect(image.Rectangle{Max: sz}, 4).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widgets.PaintBorder1px(gtx, sz, theme.Border)
	dims.Size = sz
	return dims
}

// inlineLabel is a fixed-width caption for inline "label: control" rows.
func (s *UIState) inlineLabel(txt string, wDp unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(wDp)
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		lbl := material.Label(s.host.Theme, unit.Sp(11), txt)
		lbl.Color = theme.FgMuted
		return lbl.Layout(gtx)
	}
}

func vSpace(dp float32) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.Spacer{Height: unit.Dp(dp)}.Layout(gtx) }
}

func (s *UIState) smallLabel(txt string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(s.host.Theme, unit.Sp(10), txt)
		lbl.Color = theme.FgMuted
		return lbl.Layout(gtx)
	}
}

func cycleBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string) layout.Dimensions {
	return btn(gtx, th, clk, label, nil, theme.Border, th.Fg, true)
}

// btnWide is a full-width primary/action button with centred content.
func btnWide(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, ic *widget.Icon, bg, fg color.NRGBA) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := record(gtx)
		w := gtx.Constraints.Max.X
		dims := layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = w
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ic == nil {
							return layout.Dimensions{}
						}
						s := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(s, s)
						gtx.Constraints.Max = gtx.Constraints.Min
						return ic.Layout(gtx, fg)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if ic == nil {
							return layout.Dimensions{}
						}
						return layout.Spacer{Width: unit.Dp(6)}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), label)
						lbl.Color = fg
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
				)
			})
		})
		call := macro.Stop()
		sz := image.Pt(w, dims.Size.Y)
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
		call.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: sz}
	})
}

func switchRow(gtx layout.Context, th *material.Theme, sw *widget.Bool, label string) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return material.Switch(th, sw, label).Layout(gtx) }),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), label)
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		}),
	)
}

// ---------------------------------------------------------------------------
// 4.1 Targets (reverse proxy)
// ---------------------------------------------------------------------------

func (s *UIState) secTargets() []layout.Widget {
	targets := s.Proxy.Targets.Snapshot()
	rows := []layout.Widget{
		s.secHeader(&s.SecTargetsHdr, "Targets · reverse proxy", fmt.Sprintf("%d", len(targets)), &s.SecTargetsOpen),
	}
	if !s.SecTargetsOpen {
		return rows
	}

	// add form
	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.fieldLabel("Add reverse-proxy domain")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.TextField(gtx, s.host.Theme, &s.TargetInput, "example.com  or  *.example.com", true, nil, 0, unit.Sp(12))
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.TargetAddBtn, "Add domain", widgets.IconAdd, theme.BtnPrimary, theme.BtnPrimaryFg)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.TargetBanner == "" {
						return layout.Dimensions{}
					}
					col := theme.FgMuted
					if strings.Contains(s.TargetBanner, "invalid") || strings.Contains(s.TargetBanner, "exists") {
						col = theme.Danger
					} else if strings.HasPrefix(s.TargetBanner, "Added") {
						col = theme.MethodGet
					}
					lbl := material.Label(s.host.Theme, unit.Sp(10), s.TargetBanner)
					lbl.Color = col
					lbl.MaxLines = 2
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, lbl.Layout)
				}),
			)
		})
	}))

	if len(targets) == 0 {
		rows = append(rows, pad(s.smallLabel("No domains yet — reverse mode is inactive. Add a domain above, then route it to 127.0.0.1 (Copy hosts line).")))
		return rows
	}

	for i := range targets {
		t := targets[i]
		if i > 0 {
			rows = append(rows, listRowDivider)
		}
		rows = append(rows, s.targetRow(t))
	}
	return rows
}

func (s *UIState) targetRow(t TargetView) layout.Widget {
	row := s.TargetRows[t.Domain]
	if row == nil {
		row = &TargetRow{}
		s.TargetRows[t.Domain] = row
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return statusDot(gtx, t.Status) }),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return row.Expand.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								pointer.CursorPointer.Add(gtx.Ops)
								lbl := material.Label(s.host.Theme, unit.Sp(11), t.Domain)
								lbl.Font.Typeface = widgets.MonoTypeface
								lbl.MaxLines = 1
								return lbl.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return iconBtn(gtx, s.host.Theme, &row.Copy, widgets.IconDup)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return iconBtn(gtx, s.host.Theme, &row.Remove, widgets.IconDel)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					status := t.Status
					if t.Status == StatusError && t.LastErr != "" {
						status = "error: " + t.LastErr
					}
					lbl := material.Label(s.host.Theme, unit.Sp(10), fmt.Sprintf("%s · %d req · %s", status, t.Requests, HostsLine(t.Domain)))
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !row.Expanded {
						return layout.Dimensions{}
					}
					return s.targetOptions(gtx, t, row)
				}),
			)
		})
	}
}

func (s *UIState) targetOptions(gtx layout.Context, t TargetView, row *TargetRow) layout.Dimensions {
	// Events (upstream mode, TLS, DoH, addr/delay) are polled in
	// targetsEvents; this builder is display-only.
	const lblW = unit.Dp(66)
	return layout.Inset{Top: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Upstream", lblW)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toggleChip(gtx, s.host.Theme, &row.UpstreamAuto, "auto", t.Upstream == UpstreamAuto)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toggleChip(gtx, s.host.Theme, &row.UpstreamManual, "manual", t.Upstream == UpstreamManual)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if t.Upstream != UpstreamManual {
						return layout.Dimensions{}
					}
					return layout.Inset{Top: unit.Dp(4), Left: lblW}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						if row.AddrInput.Text() == "" && t.UpstreamAddr != "" {
							row.AddrInput.SetText(t.UpstreamAddr)
						}
						row.AddrInput.SingleLine = true
						return widgets.TextField(gtx, s.host.Theme, &row.AddrInput, "real ip:port", true, nil, 0, unit.Sp(11))
					})
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("TLS", lblW)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toggleChip(gtx, s.host.Theme, &row.TLSDecrypt, "decrypt", t.TLS == TLSDecrypt)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return toggleChip(gtx, s.host.Theme, &row.TLSTunnel, "tunnel", t.TLS == TLSTunnel)
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Delay", lblW)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Max.X = gtx.Dp(unit.Dp(56))
							if row.DelayInput.Text() == "" && t.Delay > 0 {
								row.DelayInput.SetText(strconv.FormatInt(t.Delay.Milliseconds(), 10))
							}
							row.DelayInput.SingleLine = true
							return widgets.TextField(gtx, s.host.Theme, &row.DelayInput, "0", true, nil, 0, unit.Sp(11))
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(s.host.Theme, unit.Sp(10), "ms")
							lbl.Color = theme.FgMuted
							return layout.Inset{Left: unit.Dp(4)}.Layout(gtx, lbl.Layout)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							row.DoH.Value = t.DoH
							return switchRow(gtx, s.host.Theme, &row.DoH, "DoH resolve")
						}),
					)
				}),
			)
		})
	})
}

func (s *UIState) addTarget() {
	d := strings.TrimSpace(s.TargetInput.Text())
	if !ValidDomain(d) {
		s.TargetBanner = "invalid domain"
		return
	}
	if s.Proxy.Targets.Add(&Target{Domain: d, Upstream: UpstreamAuto, TLS: TLSDecrypt}) {
		s.TargetBanner = "Added " + d + " — add hosts line to route traffic here"
		s.TargetInput.SetText("")
		s.MarkDirty()
	} else {
		s.TargetBanner = "domain already exists or invalid"
	}
}

// ---------------------------------------------------------------------------
// status dot + chip helpers
// ---------------------------------------------------------------------------

func statusDot(gtx layout.Context, status string) layout.Dimensions {
	col := theme.MethodPost // waiting = amber
	switch status {
	case StatusProxying:
		col = theme.MethodGet
	case StatusError:
		col = theme.Danger
	}
	d := gtx.Dp(unit.Dp(9))
	sz := image.Pt(d, d)
	paint.FillShape(gtx.Ops, col, clip.Ellipse{Max: sz}.Op(gtx.Ops))
	return layout.Dimensions{Size: sz}
}

func toggleChip(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	bg := theme.Border
	fg := th.Fg
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return btn(gtx, th, clk, label, nil, bg, fg, true)
}

func iconBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, ic *widget.Icon) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			s := gtx.Dp(unit.Dp(16))
			gtx.Constraints.Min = image.Pt(s, s)
			gtx.Constraints.Max = gtx.Constraints.Min
			col := theme.FgMuted
			if clk.Hovered() {
				col = th.Fg
			}
			pointer.CursorPointer.Add(gtx.Ops)
			return ic.Layout(gtx, col)
		})
	})
}
