package mitm

import (
	"fmt"
	"image"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"tracto/internal/ui/settings"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"
	"tracto/pkg/syntax"

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
	// widget.Clickable.Layout drains every pending click before it draws, so a
	// Clicked poll that runs after the button was laid out never sees one.
	if s.BodyViewer != nil && s.showsTextPane(f) {
		for s.BodySearchBtn.Clicked(gtx) {
			s.BodySearch.Toggle(gtx, s.BodyViewer)
		}
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
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					if !s.showsTextPane(f) {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}
					return layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, &s.BodySearchBtn, widgets.IconSearch, s.host.Theme, 22, 15)
						})
					})
				}))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
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

// textPane renders one of the inspector's scrollable texts through the shared
// viewer, which is what gives these panes selection, wrapping and Ctrl+F. build
// only runs when key changes, so switching flows or tabs re-renders but a redraw
// does not re-derive the text.
func (s *UIState) textPane(gtx layout.Context, key paneTextKey, lang syntax.Lang, build func() string) layout.Dimensions {
	if s.BodyViewer == nil {
		s.BodyViewer = workspace.NewResponseViewer()
	}
	if s.BodyViewerKey != key {
		s.BodyViewerKey = key
		s.BodyViewer.SetText(build())
		s.BodySearch.Invalidate()
	}
	if s.BodyViewer.Len() == 0 {
		s.BodySearch.Close(s.BodyViewer)
		return emptyLabel(s.host.Theme, gtx, "no body")
	}
	s.BodySearch.Process(gtx, s.BodyViewer)

	vs := workspace.ResponseViewerStyle{
		Viewer:         s.BodyViewer,
		Shaper:         s.host.Theme.Shaper,
		Font:           widgets.MonoFont,
		TextSize:       unit.Sp(11),
		Color:          theme.Fg,
		Background:     theme.BgField,
		SelectionColor: theme.Selection,
		Wrap:           true,
		Padding:        unit.Dp(6),
		Lang:           lang,
		Syntax:         theme.Syntax,
		BracketCycle:   settings.BracketColorization,
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return vs.Layout(gtx) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			dims, moved := widgets.ViewerScrollbar(gtx, s.BodyViewer, &s.BodyDrag, &s.BodyDragY)
			if moved && s.host.Window != nil {
				s.host.Window.Invalidate()
			}
			return dims
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return workspace.SearchOverlay(gtx, s.host.Theme, &s.BodySearch)
		}),
	)
}

// HandleSearchShortcut opens Ctrl+F over whichever inspector text pane is on
// screen. Panes built from key/value rows have nothing for it to search.
func (s *UIState) HandleSearchShortcut(gtx layout.Context) {
	if s.BodyViewer == nil || s.BodyViewer.Len() == 0 || !s.textPaneShowing() {
		return
	}
	s.BodySearch.Toggle(gtx, s.BodyViewer)
	if s.host.Window != nil {
		s.host.Window.Invalidate()
	}
}

func (s *UIState) textPaneShowing() bool {
	if s.Store == nil {
		return false
	}
	return s.showsTextPane(s.Store.FindByID(s.Selected))
}

func (s *UIState) showsTextPane(f *Flow) bool {
	if f == nil || f.Kind == FlowTunnel || s.InspectorCollapsed {
		return false
	}
	// pretty mode splits into Headers / Body / Params / Cookies; only Body is text
	return s.RenderMode != 1 || s.SecTab == 1
}

func (s *UIState) rawPane(gtx layout.Context, f *Flow, resp bool) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			return s.textPane(gtx, s.paneKey(f, paneRaw, resp), syntax.LangPlain, func() string {
				return flowAsText(f, resp)
			})
		})
	})
}

func (s *UIState) paneKey(f *Flow, kind paneKind, resp bool) paneTextKey {
	return paneTextKey{id: f.ID, rev: s.Store.Rev(), kind: kind, resp: resp}
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
				return s.bodyPane(gtx, s.paneKey(f, paneBody, resp), body, contentType(headers), f.Error)
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

func (s *UIState) bodyPane(gtx layout.Context, key paneTextKey, body []byte, mime, errMsg string) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			if errMsg != "" {
				s.BodySearch.Close(s.BodyViewer)
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), "Error: "+errMsg)
					lbl.Color = theme.Danger
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				})
			}
			return s.textPane(gtx, key, syntax.Detect(mime, body), func() string {
				preview := body
				if len(preview) > 64*1024 {
					preview = preview[:64*1024]
				}
				return string(preview)
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
			return s.textPane(gtx, s.paneKey(f, paneHex, resp), syntax.LangPlain, func() string {
				return hexDump(body)
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
					return s.textPane(gtx, s.paneKey(f, paneRender, true), syntax.LangPlain, func() string {
						return stripHTML(string(f.RespBody))
					})
				})
			}),
		)
	})
}

// paneKind distinguishes the derived texts the inspector caches, so switching
// tabs rebuilds instead of reusing the previous pane's lines.
type paneKind uint8

const (
	paneRaw paneKind = iota + 1
	paneBody
	paneHex
	paneRender
	paneWS
)

type paneTextKey struct {
	id   uint64
	rev  uint64
	kind paneKind
	resp bool
}

// paneLines returns the rendered text split into lines, rebuilding only when
// the flow, its revision or the pane changes. Rebuilding per frame meant a
// full copy (and for hex, a 4x expansion) of the selected body on every
// redraw.
func (s *UIState) paneLines(key paneTextKey, build func() string) []string {
	if s.paneCache.lines == nil || s.paneCacheKey != key {
		s.paneCacheKey = key
		s.paneCache.txt = build()
		s.paneCache.lines = splitPaneLines(s.paneCache.txt)
	}
	return s.paneCache.lines
}

// paneLineChunk bounds how much text one list row carries. Rows are laid out
// with a wrapping label, so a minified body arriving as a single line would
// otherwise be shaped in full on every frame.
const paneLineChunk = 2000

func splitPaneLines(txt string) []string {
	raw := strings.Split(txt, "\n")
	long := false
	for _, ln := range raw {
		if len(ln) > paneLineChunk {
			long = true
			break
		}
	}
	if !long {
		return raw
	}
	out := make([]string, 0, len(raw))
	for _, ln := range raw {
		for len(ln) > paneLineChunk {
			cut := paneLineChunk
			for cut > 0 && !utf8.RuneStart(ln[cut]) {
				cut--
			}
			if cut == 0 {
				cut = paneLineChunk
			}
			out = append(out, ln[:cut])
			ln = ln[cut:]
		}
		out = append(out, ln)
	}
	return out
}

func (s *UIState) scrollLines(gtx layout.Context, list *widget.List, lines []string) layout.Dimensions {
	list.Axis = layout.Vertical
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
	ct := contentType(f.ReqHeaders)
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

func contentType(headers [][2]string) string {
	ct := ""
	for _, h := range headers {
		if strings.EqualFold(h[0], "content-type") {
			ct = h[1]
		}
	}
	return ct
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
