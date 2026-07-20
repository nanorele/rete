package mitm

import (
	"fmt"
	"image"
	"net"
	"net/url"
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

func (s *UIState) inspector(gtx layout.Context) layout.Dimensions {
	paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	f := s.Store.FindByID(s.Selected)
	if f == nil {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(s.host.Theme, unit.Sp(12), "Select a flow to inspect")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.inspectorHeader(gtx, f) }),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.inspectorTabs(gtx, f) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if f.Kind == FlowTunnel {
				return s.tunnelPane(gtx, f)
			}
			resp := s.ActTab == 1
			switch s.RenderMode {
			case 2:
				return s.hexPane(gtx, f, resp)
			case 3:
				return s.renderPane(gtx, f)
			case 0:
				return s.rawPane(gtx, f, resp)
			default:
				return s.prettyPane(gtx, f, resp)
			}
		}),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.inspectorActions(gtx) }),
	)
}

func (s *UIState) inspectorHeader(gtx layout.Context, f *Flow) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(11), f.Method)
				lbl.Color = theme.MethodColor(f.Method)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				u := f.URL
				if u == "" {
					u = f.Host + f.Path
				}
				lbl := material.Label(s.host.Theme, unit.Sp(12), u)
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.InspectorToggle.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(16))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					pointer.CursorPointer.Add(gtx.Ops)
					return widgets.IconChevronR.Layout(gtx, theme.FgMuted)
				})
			}),
		)
	})
}

func (s *UIState) inspectorTabs(gtx layout.Context, f *Flow) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tab(gtx, s.host.Theme, &s.TabReq, "Request", s.ActTab == 0)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tab(gtx, s.host.Theme, &s.TabResp, "Response", s.ActTab == 1)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(10), statusLine(f))
					lbl.Color = theme.FgMuted
					lbl.Alignment = 2
					lbl.MaxLines = 1
					return layout.Inset{Top: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, lbl.Layout)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if f.Kind == FlowTunnel {
				return layout.Dimensions{}
			}
			return layout.Inset{Left: unit.Dp(8), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				modes := []struct {
					clk *widget.Clickable
					lbl string
					id  int
				}{
					{&s.ViewRaw, "Raw", 0},
					{&s.ViewPretty, "Pretty", 1},
					{&s.ViewHex, "Hex", 2},
				}
				children := []layout.FlexChild{}
				for i := range modes {
					m := modes[i]
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return modeChip(gtx, s.host.Theme, m.clk, m.lbl, s.RenderMode == m.id)
					}))
				}
				if s.ActTab == 1 {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return modeChip(gtx, s.host.Theme, &s.ViewRender, "Render", s.RenderMode == 3)
					}))
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
		}),
	)
}

func modeChip(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := record(gtx)
		dims := layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), label)
			if active {
				lbl.Color = theme.Accent
				lbl.Font.Weight = font.Bold
			} else {
				lbl.Color = theme.FgMuted
			}
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		if active {
			paint.FillShape(gtx.Ops, theme.AccentDim, clip.UniformRRect(image.Rectangle{Max: dims.Size}, 3).Op(gtx.Ops))
		}
		call.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(dims.Size.X+gtx.Dp(unit.Dp(4)), dims.Size.Y)}
	})
}

// ---- panes ----

func (s *UIState) rawPane(gtx layout.Context, f *Flow, resp bool) layout.Dimensions {
	txt := flowAsText(f, resp)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.scrollText(gtx, &s.BodyList, txt)
			})
		})
	})
}

func (s *UIState) prettyPane(gtx layout.Context, f *Flow, resp bool) layout.Dimensions {
	headers := f.ReqHeaders
	body := f.ReqBody
	if resp {
		headers = f.RespHeaders
		body = f.RespBody
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sectionTabs(gtx) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch s.SecTab {
			case 1:
				return s.bodyPane(gtx, body, f.Error)
			case 2:
				return s.paramsPane(gtx, f, resp)
			case 3:
				return s.cookiesPane(gtx, headers, resp)
			default:
				return s.headersPane(gtx, headers, resp)
			}
		}),
	)
}

func (s *UIState) sectionTabs(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tab(gtx, s.host.Theme, &s.SecHeaders, "Headers", s.SecTab == 0)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tab(gtx, s.host.Theme, &s.SecBody, "Body", s.SecTab == 1)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tab(gtx, s.host.Theme, &s.SecParams, "Params", s.SecTab == 2)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return tab(gtx, s.host.Theme, &s.SecCookies, "Cookies", s.SecTab == 3)
		}),
	)
}

func (s *UIState) headersPane(gtx layout.Context, headers [][2]string, resp bool) layout.Dimensions {
	list := &s.ReqHeadersList
	if resp {
		list = &s.RespHeadersList
	}
	list.Axis = layout.Vertical
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(headers) == 0 {
			return emptyLabel(s.host.Theme, gtx, "no headers")
		}
		return material.List(s.host.Theme, list).Layout(gtx, len(headers), func(gtx layout.Context, i int) layout.Dimensions {
			return headerRow(gtx, s.host.Theme, headers[i])
		})
	})
}

func headerRow(gtx layout.Context, th *material.Theme, h [2]string) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(150))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				lbl := material.Label(th, unit.Sp(11), h[0])
				lbl.Color = theme.Accent
				lbl.Font.Typeface = widgets.MonoTypeface
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), h[1])
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
		)
	})
}

func (s *UIState) bodyPane(gtx layout.Context, body []byte, errMsg string) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			if errMsg != "" {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), "Error: "+errMsg)
					lbl.Color = theme.Danger
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				})
			}
			if len(body) == 0 {
				return emptyLabel(s.host.Theme, gtx, "no body")
			}
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				preview := body
				if len(preview) > 64*1024 {
					preview = preview[:64*1024]
				}
				return s.scrollText(gtx, &s.BodyList, string(preview))
			})
		})
	})
}

func (s *UIState) paramsPane(gtx layout.Context, f *Flow, resp bool) layout.Dimensions {
	pairs := parseParams(f)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(pairs) == 0 {
			return emptyLabel(s.host.Theme, gtx, "no query/body params")
		}
		return material.List(s.host.Theme, &s.ReqHeadersList).Layout(gtx, len(pairs), func(gtx layout.Context, i int) layout.Dimensions {
			return headerRow(gtx, s.host.Theme, pairs[i])
		})
	})
}

func (s *UIState) cookiesPane(gtx layout.Context, headers [][2]string, resp bool) layout.Dimensions {
	pairs := parseCookies(headers, resp)
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(pairs) == 0 {
			return emptyLabel(s.host.Theme, gtx, "no cookies")
		}
		return material.List(s.host.Theme, &s.RespHeadersList).Layout(gtx, len(pairs), func(gtx layout.Context, i int) layout.Dimensions {
			return headerRow(gtx, s.host.Theme, pairs[i])
		})
	})
}

func (s *UIState) hexPane(gtx layout.Context, f *Flow, resp bool) layout.Dimensions {
	body := f.ReqBody
	if resp {
		body = f.RespBody
	}
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			if len(body) == 0 {
				return emptyLabel(s.host.Theme, gtx, "no body")
			}
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.scrollText(gtx, &s.BodyList, hexDump(body))
			})
		})
	})
}

func (s *UIState) renderPane(gtx layout.Context, f *Flow) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(10), "Rendered HTML (text extraction — external resources not fetched)")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return s.scrollText(gtx, &s.BodyList, stripHTML(string(f.RespBody)))
					})
				})
			}),
		)
	})
}

func (s *UIState) scrollText(gtx layout.Context, list *widget.List, txt string) layout.Dimensions {
	list.Axis = layout.Vertical
	lines := strings.Split(txt, "\n")
	return material.List(s.host.Theme, list).Layout(gtx, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
		lbl := material.Label(s.host.Theme, unit.Sp(11), lines[i])
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	})
}

func emptyLabel(th *material.Theme, gtx layout.Context, msg string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), msg)
		lbl.Color = theme.FgMuted
		return lbl.Layout(gtx)
	})
}

func (s *UIState) inspectorActions(gtx layout.Context) layout.Dimensions {
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, s.host.Theme, &s.InspSendRepeater, "Repeater", nil, theme.Border, s.host.Theme.Fg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, s.host.Theme, &s.InspSendIntruder, "Intruder", nil, theme.Border, s.host.Theme.Fg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, s.host.Theme, &s.InspSendComparer, "Comparer", nil, theme.Border, s.host.Theme.Fg, true)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, s.host.Theme, &s.InspSendDecoder, "Decoder", nil, theme.Border, s.host.Theme.Fg, true)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btn(gtx, s.host.Theme, &s.InspCopy, "Copy", widgets.IconDup, theme.Border, s.host.Theme.Fg, true)
				}),
			)
		})
	})
}

// ---- tunnel pane ----

func (s *UIState) tunnelPane(gtx layout.Context, f *Flow) layout.Dimensions {
	tunnelState := "Active (browser keep-alive)"
	if f.TunnelClosed {
		tunnelState = "Closed"
	}
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					kvRow(s.host.Theme, "Status", tunnelStatusText(f)),
					kvRow(s.host.Theme, "Tunnel", tunnelState),
					kvRow(s.host.Theme, "Target", net.JoinHostPort(f.Host, f.Port)),
					kvRow(s.host.Theme, "Scheme", "https (TLS)"),
					kvRow(s.host.Theme, "Client", f.ClientAddr),
					kvRow(s.host.Theme, "Bytes ↑", humanSize(f.BytesOut)+"  (client → server)"),
					kvRow(s.host.Theme, "Bytes ↓", humanSize(f.BytesIn)+"  (server → client)"),
				)
			})
		})
	})
}

func kvRow(th *material.Theme, key, val string) layout.FlexChild {
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

func tunnelStatusText(f *Flow) string {
	switch {
	case f.Error != "" && f.Status != "":
		return f.Status + "  (" + f.Error + ")"
	case f.Error != "":
		return f.Error
	case f.Status != "":
		return f.Status
	default:
		return "…"
	}
}

// ---- parsing helpers ----

func parseParams(f *Flow) [][2]string {
	var out [][2]string
	if i := strings.IndexByte(f.Path, '?'); i >= 0 {
		if vals, err := url.ParseQuery(f.Path[i+1:]); err == nil {
			for k, vs := range vals {
				for _, v := range vs {
					out = append(out, [2]string{k, v})
				}
			}
		}
	}
	ct := ""
	for _, h := range f.ReqHeaders {
		if strings.EqualFold(h[0], "content-type") {
			ct = h[1]
		}
	}
	if strings.Contains(ct, "application/x-www-form-urlencoded") && len(f.ReqBody) > 0 {
		if vals, err := url.ParseQuery(string(f.ReqBody)); err == nil {
			for k, vs := range vals {
				for _, v := range vs {
					out = append(out, [2]string{"(body) " + k, v})
				}
			}
		}
	}
	return out
}

func parseCookies(headers [][2]string, resp bool) [][2]string {
	var out [][2]string
	for _, h := range headers {
		if !resp && strings.EqualFold(h[0], "cookie") {
			for _, c := range strings.Split(h[1], ";") {
				c = strings.TrimSpace(c)
				if k, v, ok := strings.Cut(c, "="); ok {
					out = append(out, [2]string{k, v})
				}
			}
		}
		if resp && strings.EqualFold(h[0], "set-cookie") {
			parts := strings.SplitN(h[1], ";", 2)
			if k, v, ok := strings.Cut(parts[0], "="); ok {
				out = append(out, [2]string{k, v})
			}
		}
	}
	return out
}

func hexDump(b []byte) string {
	const perLine = 16
	if len(b) > 32*1024 {
		b = b[:32*1024]
	}
	var sb strings.Builder
	for off := 0; off < len(b); off += perLine {
		fmt.Fprintf(&sb, "%08x  ", off)
		end := off + perLine
		if end > len(b) {
			end = len(b)
		}
		for i := off; i < off+perLine; i++ {
			if i < end {
				fmt.Fprintf(&sb, "%02x ", b[i])
			} else {
				sb.WriteString("   ")
			}
		}
		sb.WriteString(" |")
		for i := off; i < end; i++ {
			c := b[i]
			if c >= 32 && c < 127 {
				sb.WriteByte(c)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}

func stripHTML(s string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			sb.WriteByte(' ')
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return strings.TrimSpace(sb.String())
}
