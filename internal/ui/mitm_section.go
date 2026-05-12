package ui

import (
	"fmt"
	"image"
	"image/color"
	"net"
	"strconv"
	"strings"
	"time"

	"tracto/internal/ui/mitm"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) layoutMITMSection(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	st.Ensure()
	if st.Store != nil {
		st.Store.SetNotify(func() {
			if ui.Window != nil {
				ui.Window.Invalidate()
			}
		})
	}

	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	st.List.Axis = layout.Vertical
	st.ReqHeadersList.Axis = layout.Vertical
	st.RespHeadersList.Axis = layout.Vertical

	for st.StartBtn.Clicked(gtx) {
		if !mitm.IsAdmin() {
			st.StatusBanner = "Administrator privileges required to start the proxy"
		} else if st.Proxy.Running() {
			st.StatusBanner = "Proxy already running on " + st.Proxy.Addr()
		} else {
			addr := strings.TrimSpace(st.BindAddr.Text())
			if err := st.Proxy.Start(addr); err != nil {
				st.StatusBanner = "Start failed: " + err.Error()
			} else {
				st.StatusBanner = "Proxy listening on " + st.Proxy.Addr()
			}
		}
	}
	for st.StopBtn.Clicked(gtx) {
		if st.Proxy.Running() {
			st.Proxy.Stop()
			st.StatusBanner = "Proxy stopped"
		}
	}
	for st.ClearBtn.Clicked(gtx) {
		st.Store.Clear()
		st.Selected = 0
	}
	for st.TabReq.Clicked(gtx) {
		st.ActTab = 0
	}
	for st.TabResp.Clicked(gtx) {
		st.ActTab = 1
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmToolbar(gtx) }),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return ui.mitmBody(gtx) }),
		layout.Rigid(mitmHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmStatusBar(gtx) }),
	)
}

func (ui *AppUI) mitmToolbar(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(40)))}.Op())
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s := gtx.Dp(unit.Dp(18))
				gtx.Constraints.Min = image.Pt(s, s)
				gtx.Constraints.Max = gtx.Constraints.Min
				return widgets.IconMITM.Layout(gtx, theme.Accent)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(13), "MITM Proxy")
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmStartBtn(gtx) }),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mitmBtn(gtx, ui.Theme, &st.StopBtn, "Stop", widgets.IconStop, theme.Border, ui.Theme.Fg, st.Proxy.Running())
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mitmBtn(gtx, ui.Theme, &st.ClearBtn, "Clear", widgets.IconBack, theme.Border, ui.Theme.Fg, st.Store.Len() > 0)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), "Bind:")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(140))
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(200))
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ed := material.Editor(ui.Theme, &st.BindAddr, "host:port")
						ed.TextSize = unit.Sp(12)
						ed.HintColor = theme.FgMuted
						if st.Proxy.Running() {
							st.BindAddr.ReadOnly = true
						} else {
							st.BindAddr.ReadOnly = false
						}
						return ed.Layout(gtx)
					})
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				count := st.Store.Len()
				lbl := material.Label(ui.Theme, unit.Sp(11), fmt.Sprintf("%d flows", count))
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (ui *AppUI) mitmStartBtn(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	admin := mitm.IsAdmin()
	running := st.Proxy.Running()
	enabled := admin && !running

	bg := theme.Accent
	fg := ui.Theme.ContrastFg
	label := "Start Proxy"
	if running {
		bg = theme.Cancel
		fg = theme.Fg
		label = "Running…"
	} else if !admin {
		bg = theme.BgField
		fg = theme.FgDim
	}

	return st.StartBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := mitmRecord(gtx)
			dims := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						s := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(s, s)
						gtx.Constraints.Max = gtx.Constraints.Min
						if !admin {
							return widgets.IconShield.Layout(gtx, fg)
						}
						return widgets.IconPlay.Layout(gtx, fg)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(12), label)
						lbl.Color = fg
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
				)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			if !admin && !running {
				widgets.PaintBorder1px(gtx, sz, theme.Border)
			}
			call.Add(gtx.Ops)
			if !enabled {
				st.StartBtn.Hovered()
			}
			return dims
		})
	})
}

func (ui *AppUI) mitmBody(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM

	for {
		e, ok := st.SplitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			st.SplitDragX = e.Position.X
		case pointer.Drag:
			delta := e.Position.X - st.SplitDragX
			st.SplitRatio += delta / float32(gtx.Constraints.Max.X)
			if st.SplitRatio < 0.15 {
				st.SplitRatio = 0.15
			}
			if st.SplitRatio > 0.85 {
				st.SplitRatio = 0.85
			}
			st.SplitDragX = e.Position.X
		}
	}

	totalW := gtx.Constraints.Max.X
	handleW := gtx.Dp(unit.Dp(6))
	leftW := int(float32(totalW)*st.SplitRatio) - handleW/2
	if leftW < 200 {
		leftW = 200
	}
	if leftW > totalW-handleW-260 {
		leftW = totalW - handleW - 260
	}
	rightW := totalW - leftW - handleW

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			return ui.mitmFlowTable(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			h := gtx.Constraints.Max.Y
			size := image.Pt(handleW, h)
			line := gtx.Dp(unit.Dp(1))
			lineCol := theme.Border
			if st.SplitDrag.Dragging() {
				lineCol = theme.Accent
			}
			paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt((handleW-line)/2, 0), Max: image.Pt((handleW-line)/2+line, h)}.Op())
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			pointer.CursorColResize.Add(gtx.Ops)
			st.SplitDrag.Add(gtx.Ops)
			event.Op(gtx.Ops, &st.SplitDrag)
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return ui.mitmInspector(gtx)
		}),
	)
}

func (ui *AppUI) mitmFlowTable(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(mitmTableHeader(ui.Theme)),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			flows := st.Store.Snapshot()
			if len(flows) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(12), "No traffic captured yet")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				})
			}
			for len(st.RowClicks) < len(flows) {
				st.RowClicks = append(st.RowClicks, &widget.Clickable{})
			}
			return material.List(ui.Theme, &st.List).Layout(gtx, len(flows), func(gtx layout.Context, i int) layout.Dimensions {
				f := flows[i]
				clk := st.RowClicks[i]
				for clk.Clicked(gtx) {
					st.Selected = f.ID
				}
				return mitmFlowRow(gtx, ui.Theme, f, clk, st.Selected == f.ID)
			})
		}),
	)
}

func mitmTableHeader(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(24)))}.Op())
		return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				colHeader(th, "#", 32),
				colHeader(th, "Method", 64),
				colHeader(th, "Host", 0),
				colHeaderRight(th, "Status", 56),
				colHeaderRight(th, "Size", 64),
				colHeaderRight(th, "Time", 56),
			)
		})
	}
}

func colHeader(th *material.Theme, s string, w int) layout.FlexChild {
	return colHeaderAligned(th, s, w, text.Start)
}

func colHeaderRight(th *material.Theme, s string, w int) layout.FlexChild {
	return colHeaderAligned(th, s, w, text.End)
}

func colHeaderAligned(th *material.Theme, s string, w int, al text.Alignment) layout.FlexChild {
	if w == 0 {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(10), s)
			lbl.Color = theme.FgMuted
			lbl.Font.Weight = font.Bold
			lbl.Alignment = al
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		lbl := material.Label(th, unit.Sp(10), s)
		lbl.Color = theme.FgMuted
		lbl.Font.Weight = font.Bold
		lbl.Alignment = al
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func mitmFlowRow(gtx layout.Context, th *material.Theme, f *mitm.Flow, clk *widget.Clickable, selected bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowH := gtx.Dp(unit.Dp(22))
		gtx.Constraints.Min.Y = rowH
		bg := theme.Bg
		if selected {
			bg = theme.AccentDim
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, rowH)}.Op())
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				cellText(th, fmt.Sprintf("%d", f.ID), 32, text.Start, theme.FgMuted, false),
				cellMethod(th, f.Method, 64),
				cellHost(th, f),
				cellStatus(th, f, 56),
				cellText(th, humanSize(f.RespSize), 64, text.End, theme.FgMuted, true),
				cellText(th, humanDuration(f), 56, text.End, theme.FgMuted, true),
			)
		})
	})
}

func cellText(th *material.Theme, s string, w int, al text.Alignment, col color.NRGBA, mono bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if w > 0 {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
		}
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Color = col
		if mono {
			lbl.Font.Typeface = widgets.MonoTypeface
		}
		return lbl.Layout(gtx)
	})
}

func cellMethod(th *material.Theme, method string, w int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		lbl := material.Label(th, unit.Sp(11), method)
		lbl.Color = theme.MethodColor(method)
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func cellHost(th *material.Theme, f *mitm.Flow) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		text := f.Host
		if f.Path != "" {
			text = f.Host + f.Path
		} else if f.Port != "" && f.Port != "80" && f.Port != "443" {
			text = f.Host + ":" + f.Port
		}
		lbl := material.Label(th, unit.Sp(11), text)
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	})
}

func cellStatus(th *material.Theme, f *mitm.Flow, w int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		var s string
		col := theme.FgMuted
		switch {
		case f.Error != "":
			s = "ERR"
			col = theme.Danger
		case f.StatusCode == 0:
			s = "…"
		default:
			s = strconv.Itoa(f.StatusCode)
			switch {
			case f.StatusCode >= 500:
				col = theme.Danger
			case f.StatusCode >= 400:
				col = theme.VarMissing
			case f.StatusCode >= 300:
				col = theme.Accent
			case f.StatusCode >= 200:
				col = theme.VarFound
			}
		}
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = col
		lbl.Alignment = text.End
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	})
}

func (ui *AppUI) mitmInspector(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	f := mitmFindByID(st.Store, st.Selected)
	if f == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(12), "Select a flow to inspect")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmInspectorHeader(gtx, f) }),
		layout.Rigid(mitmHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmInspectorTabs(gtx) }),
		layout.Rigid(mitmHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if st.ActTab == 1 {
				if f.Kind == mitm.FlowTunnel {
					return ui.mitmTunnelPane(gtx, f)
				}
				return ui.mitmInspectorPane(gtx, "Response", f.Status, f.RespHeaders, f.RespBody, &st.RespHeadersList, f.Error)
			}
			return ui.mitmInspectorPane(gtx, "Request", fmt.Sprintf("%s %s", f.Method, f.URL), f.ReqHeaders, f.ReqBody, &st.ReqHeadersList, "")
		}),
	)
}

func (ui *AppUI) mitmTunnelPane(gtx layout.Context, f *mitm.Flow) layout.Dimensions {
	tunnelState := "Active (browser keep-alive)"
	if f.TunnelClosed {
		tunnelState = "Closed"
	}
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							mitmKV(ui.Theme, "Status", tunnelStatusText(f)),
							mitmKV(ui.Theme, "Tunnel", tunnelState),
							mitmKV(ui.Theme, "Target", net.JoinHostPort(f.Host, f.Port)),
							mitmKV(ui.Theme, "Scheme", "https (TLS)"),
							mitmKV(ui.Theme, "Client", f.ClientAddr),
							mitmKV(ui.Theme, "Started", f.Started.Format("15:04:05.000")),
							mitmKV(ui.Theme, "Establish", humanDuration(f)),
							mitmKV(ui.Theme, "Bytes ↑", humanSize(f.BytesOut)+"  (client → server)"),
							mitmKV(ui.Theme, "Bytes ↓", humanSize(f.BytesIn)+"  (server → client)"),
						)
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11),
					"TLS tunnel — request and response payload are encrypted end-to-end.\n"+
						"\"Establish\" is the time to dial upstream and write the 200 response;\n"+
						"the TCP connection then stays open under browser keep-alive until either side closes.\n"+
						"Decryption requires generating a Tracto root CA and installing it into the system trust store (not yet implemented).")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if f.Error == "" {
					return layout.Dimensions{}
				}
				lbl := material.Label(ui.Theme, unit.Sp(11), "Error: "+f.Error)
				lbl.Color = theme.Danger
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
		)
	})
}

func mitmKV(th *material.Theme, key, val string) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(unit.Dp(100))
					gtx.Constraints.Max.X = gtx.Constraints.Min.X
					lbl := material.Label(th, unit.Sp(11), key)
					lbl.Color = theme.FgMuted
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), val)
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				}),
			)
		})
	})
}

func tunnelStatusText(f *mitm.Flow) string {
	switch {
	case f.Error != "":
		return f.Status + "  (" + f.Error + ")"
	case f.Status != "":
		return f.Status
	default:
		return "…"
	}
}

func (ui *AppUI) mitmInspectorHeader(gtx layout.Context, f *mitm.Flow) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), f.Method)
				lbl.Color = theme.MethodColor(f.Method)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				url := f.URL
				if url == "" {
					url = f.Host + f.Path
				}
				lbl := material.Label(ui.Theme, unit.Sp(12), url)
				lbl.MaxLines = 1
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), mitmStatusLine(f))
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (ui *AppUI) mitmInspectorTabs(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return mitmTab(gtx, ui.Theme, &st.TabReq, "Request", st.ActTab == 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return mitmTab(gtx, ui.Theme, &st.TabResp, "Response", st.ActTab == 1)
		}),
	)
}

func mitmTab(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
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

func (ui *AppUI) mitmInspectorPane(gtx layout.Context, _ string, _ string, headers [][2]string, body []byte, list *widget.List, errMsg string) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), "Headers")
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(180))
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					if len(headers) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(11), "no headers")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						})
					}
					return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return material.List(ui.Theme, list).Layout(gtx, len(headers), func(gtx layout.Context, i int) layout.Dimensions {
							h := headers[i]
							return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = gtx.Dp(unit.Dp(160))
									gtx.Constraints.Max.X = gtx.Constraints.Min.X
									lbl := material.Label(ui.Theme, unit.Sp(11), h[0])
									lbl.Color = theme.Accent
									lbl.Font.Typeface = widgets.MonoTypeface
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(ui.Theme, unit.Sp(11), h[1])
									lbl.Font.Typeface = widgets.MonoTypeface
									return lbl.Layout(gtx)
								}),
							)
						})
					})
				})
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), "Body")
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
					if errMsg != "" {
						return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(11), "Error: "+errMsg)
							lbl.Color = theme.Danger
							lbl.Font.Typeface = widgets.MonoTypeface
							return lbl.Layout(gtx)
						})
					}
					if len(body) == 0 {
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(11), "no body")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						})
					}
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						preview := body
						if len(preview) > 32*1024 {
							preview = preview[:32*1024]
						}
						lbl := material.Label(ui.Theme, unit.Sp(11), string(preview))
						lbl.Font.Typeface = widgets.MonoTypeface
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	})
}

func (ui *AppUI) mitmStatusBar(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(22)))}.Op())
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var msg string
		col := theme.FgMuted
		switch {
		case st.StatusBanner != "":
			msg = st.StatusBanner
			if strings.HasPrefix(strings.ToLower(msg), "administrator") || strings.HasPrefix(msg, "Start failed") {
				col = theme.Danger
			} else if strings.HasPrefix(msg, "Proxy listening") {
				col = theme.VarFound
			}
		case st.Proxy.Running():
			msg = "Proxy: " + st.Proxy.Addr() + "  •  flows=" + strconv.Itoa(st.Store.Len())
			col = theme.VarFound
		case !mitm.IsAdmin():
			msg = "Not elevated — restart as administrator to enable system-wide proxy"
		default:
			msg = "Proxy idle"
		}
		lbl := material.Label(ui.Theme, unit.Sp(11), msg)
		lbl.Color = col
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func mitmFindByID(s *mitm.Store, id uint64) *mitm.Flow {
	for _, f := range s.Snapshot() {
		if f.ID == id {
			return f
		}
	}
	return nil
}

func mitmStatusLine(f *mitm.Flow) string {
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

func humanDuration(f *mitm.Flow) string {
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

func mitmBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, ic *widget.Icon, bg, fg color.NRGBA, enabled bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			macro := mitmRecord(gtx)
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
					return lbl.Layout(gtx)
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
			})
			call := macro.Stop()
			sz := dims.Size
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: sz}, 3).Op(gtx.Ops))
			call.Add(gtx.Ops)
			return dims
		})
	})
}
