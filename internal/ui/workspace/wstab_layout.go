package workspace

import (
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"
	"unicode/utf8"

	"tracto/internal/ui/settings"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ws"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type WSHostFuncs struct {
	OnConnect    func(*RequestTab)
	OnDisconnect func(*RequestTab)
}

func (t *RequestTab) layoutWSBody(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string) layout.Dimensions {
	t.AttachWSWindow(win)
	s := t.EnsureWS()
	t.handleWSButtons(gtx)
	s.refreshDetail()

	for t.LayoutHorizBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeHoriz {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeHoriz
		}
		win.Invalidate()
	}
	for t.LayoutVertBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeVert {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeVert
		}
		win.Invalidate()
	}

	minPaneW := gtx.Dp(unit.Dp(280))
	var stacked bool
	switch t.LayoutMode {
	case LayoutModeHoriz:
		stacked = false
	case LayoutModeVert:
		stacked = true
	default:
		breakpoint := settings.StackBreakpointDp
		if breakpoint <= 0 {
			breakpoint = 720
		}
		stacked = gtx.Constraints.Max.X < gtx.Dp(unit.Dp(float32(breakpoint))) || gtx.Constraints.Max.X < 2*minPaneW
	}

	var ratio *float32
	var flexExtent float32
	var dragAxis gesture.Axis
	if stacked {
		ratio = &s.ComposerRatio
		flexExtent = float32(gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(40)))
		dragAxis = gesture.Vertical
	} else {
		ratio = &s.SplitRatio
		flexExtent = float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
		dragAxis = gesture.Horizontal
	}

	var moved bool
	var finalPos float32
	var released bool
	for {
		e, ok := s.SplitDrag.Update(gtx.Metric, gtx.Source, dragAxis)
		if !ok {
			break
		}
		var pos float32
		if stacked {
			pos = e.Position.Y
		} else {
			pos = e.Position.X
		}
		switch e.Kind {
		case pointer.Press:
			s.SplitDragX = pos
		case pointer.Drag:
			finalPos = pos
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	const minR, maxR = 0.2, 0.8
	if *ratio < minR {
		*ratio = minR
	}
	if *ratio > maxR {
		*ratio = maxR
	}

	if moved && flexExtent > 0 {
		delta := finalPos - s.SplitDragX
		oldRatio := *ratio
		*ratio += delta / flexExtent
		if *ratio < minR {
			*ratio = minR
		} else if *ratio > maxR {
			*ratio = maxR
		}
		s.SplitDragX = finalPos - ((*ratio - oldRatio) * flexExtent)
		win.Invalidate()
	}
	if released {
		win.Invalidate()
	}

	flexAxis := layout.Horizontal
	leftInset := layout.Inset{Right: unit.Dp(1)}
	rightInset := layout.Inset{Left: unit.Dp(1)}
	if stacked {
		flexAxis = layout.Vertical
		leftInset = layout.Inset{Bottom: unit.Dp(1)}
		rightInset = layout.Inset{Top: unit.Dp(1)}
	}

	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBar(gtx, &t.LayoutHorizBtn, &t.LayoutVertBtn, stacked)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: flexAxis}.Layout(gtx,
					layout.Flexed(*ratio, func(gtx layout.Context) layout.Dimensions {
						return leftInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return t.layoutWSComposerPane(gtx, th, activeEnv)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						thick := gtx.Dp(unit.Dp(4))
						var size image.Point
						var cursor pointer.Cursor
						if stacked {
							size = image.Point{X: gtx.Constraints.Min.X, Y: thick}
							cursor = pointer.CursorRowResize
						} else {
							size = image.Point{X: thick, Y: gtx.Constraints.Min.Y}
							cursor = pointer.CursorColResize
						}
						defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
						cursor.Add(gtx.Ops)
						s.SplitDrag.Add(gtx.Ops)
						event.Op(gtx.Ops, &s.SplitDrag)
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1-*ratio, func(gtx layout.Context) layout.Dimensions {
						return rightInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return t.layoutWSMessagesPane(gtx, th)
						})
					}),
				)
			}),
		)
	})
}

func (t *RequestTab) handleWSButtons(gtx layout.Context) {
	s := t.EnsureWS()
	for s.DisconnectBtn.Clicked(gtx) {
		if t.WSHost.OnDisconnect != nil {
			t.WSHost.OnDisconnect(t)
		} else {
			t.WSDisconnect()
		}
	}
	for s.PingBtn.Clicked(gtx) {
		t.WSSendPing()
	}
	for s.ClearBtn.Clicked(gtx) {
		s.ClearMessages()
		s.Selected = -1
	}
	for s.OptionsBtn.Clicked(gtx) {
		s.OptionsExpanded = !s.OptionsExpanded
	}
	for s.AddSubprotoBtn.Clicked(gtx) {
		s.AddSubprotocol("")
	}
	for s.OfferDeflateBtn.Clicked(gtx) {
		s.OfferDeflate = !s.OfferDeflate
	}
	for s.InsecureBtn.Clicked(gtx) {
		s.InsecureSkipVerify = !s.InsecureSkipVerify
	}
	for s.UseTractoCABtn.Clicked(gtx) {
		s.UseTractoCA = !s.UseTractoCA
	}
	for i := len(s.Subprotocols) - 1; i >= 0; i-- {
		if s.Subprotocols[i].DelBtn.Clicked(gtx) {
			s.Subprotocols = append(s.Subprotocols[:i], s.Subprotocols[i+1:]...)
		}
	}
	for s.OpcodeMenuBtn.Clicked(gtx) {
		s.OpcodeMenuOpen = !s.OpcodeMenuOpen
	}
	for s.OpcodeTextChoice.Clicked(gtx) {
		s.OpcodeText = true
		s.OpcodeMenuOpen = false
	}
	for s.OpcodeBinChoice.Clicked(gtx) {
		s.OpcodeText = false
		s.OpcodeMenuOpen = false
	}
	for s.ComposerWrapBtn.Clicked(gtx) {
		s.ComposerWrap = !s.ComposerWrap
	}
	for s.ComposerSendBtn.Clicked(gtx) {
		if s.State() == WSStateOpen {
			t.SendFromComposer()
		}
	}
	for s.FilterPingBtn.Clicked(gtx) {
		s.Filter.HidePing = !s.Filter.HidePing
	}
	for s.FilterPongBtn.Clicked(gtx) {
		s.Filter.HidePong = !s.Filter.HidePong
	}
	for s.FilterCloseBtn.Clicked(gtx) {
		s.Filter.HideClose = !s.Filter.HideClose
	}
	for s.DetailTextBtn.Clicked(gtx) {
		s.DetailHex = false
	}
	for s.DetailHexBtn.Clicked(gtx) {
		s.DetailHex = true
	}
	for s.DetailCopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(s.DetailEditor.Text())),
		})
	}
}

func (t *RequestTab) SendFromComposer() {
	s := t.EnsureWS()
	txt := s.ComposerEditor.Text()
	if s.OpcodeText {
		t.WSSendText(txt)
		return
	}
	payload, err := parseHexInput(txt)
	if err != nil {
		s.appendError("Hex parse: " + err.Error())
		return
	}
	t.WSSendBinary(payload)
}

func parseHexInput(s string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ':', '-':
			return -1
		}
		return r
	}, s)
	clean = strings.TrimPrefix(clean, "0x")
	return hex.DecodeString(clean)
}

func (t *RequestTab) layoutWSComposerPane(gtx layout.Context, th *material.Theme, activeEnv map[string]string) layout.Dimensions {
	s := t.EnsureWS()
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return t.layoutWSSubprotocolsHeader(gtx, th) }),
			layout.Rigid(wsHLine),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(s.Subprotocols) == 0 {
					return layout.Dimensions{}
				}
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.SubprotosList.Layout(gtx, len(s.Subprotocols), func(gtx layout.Context, i int) layout.Dimensions {
						sp := s.Subprotocols[i]
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Top: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return wsSubprotoRow(gtx, th, sp, activeEnv)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if i >= len(s.Subprotocols)-1 {
									return layout.Dimensions{}
								}
								return wsHLine(gtx)
							}),
						)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(s.Subprotocols) == 0 {
					return layout.Dimensions{}
				}
				return wsHLine(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.OptionsExpanded {
					return layout.Dimensions{}
				}
				return t.layoutWSOptions(gtx, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !s.OptionsExpanded {
					return layout.Dimensions{}
				}
				return wsHLine(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return t.layoutWSComposerHeader(gtx, th) }),
			layout.Rigid(wsHLine),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				bdr := gtx.Dp(unit.Dp(1))
				sz := gtx.Constraints.Max
				paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
				inner := image.Rect(bdr, bdr, sz.X-bdr, sz.Y-bdr)
				paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
				gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
				gtx.Constraints.Max = gtx.Constraints.Min
				op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
				return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(th, &s.ComposerEditor, "type your message")
					ed.TextSize = unit.Sp(12)
					ed.HintColor = theme.FgMuted
					ed.Font.Typeface = widgets.MonoTypeface
					return ed.Layout(gtx)
				})
			}),
		)
	})
}

func (t *RequestTab) layoutWSSubprotocolsHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Subprotocols")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
				return widgets.Bordered1px(gtx, unit.Dp(4), theme.Border, func(gtx layout.Context) layout.Dimensions {
					btn := widgets.MonoButton(th, &s.AddSubprotoBtn, "Add")
					btn.TextSize = unit.Sp(12)
					btn.Background = theme.BgField
					btn.Color = th.Fg
					btn.Inset = layout.UniformInset(unit.Dp(6))
					return btn.Layout(gtx)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
				label := "Show Options"
				if s.OptionsExpanded {
					label = "Hide Options"
				}
				return widgets.Bordered1px(gtx, unit.Dp(4), theme.Border, func(gtx layout.Context) layout.Dimensions {
					btn := widgets.MonoButton(th, &s.OptionsBtn, label)
					btn.TextSize = unit.Sp(12)
					btn.Background = theme.BgField
					btn.Color = th.Fg
					btn.Inset = layout.UniformInset(unit.Dp(6))
					return btn.Layout(gtx)
				})
			}),
		)
	})
}

func (t *RequestTab) layoutWSOptions(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsOptionToggle(gtx, th, &s.OfferDeflateBtn, "permessage-deflate", s.OfferDeflate)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsOptionToggle(gtx, th, &s.UseTractoCABtn, "use Tracto CA", s.UseTractoCA)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsOptionToggle(gtx, th, &s.InsecureBtn, "skip TLS verify", s.InsecureSkipVerify)
			}),
		)
	})
}

func (t *RequestTab) layoutWSComposerHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Compose")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutWSOpcodeSelector(gtx, th)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtnSlim(gtx, &s.ComposerWrapBtn, iconWrap, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				enabled := s.State() == WSStateOpen
				bg := theme.Accent
				fg := th.ContrastFg
				if !enabled {
					bg = theme.Border
					fg = theme.FgDim
				}
				return s.ComposerSendBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if enabled {
						pointer.CursorPointer.Add(gtx.Ops)
					}
					macro := op.Record(gtx.Ops)
					dims := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(th, unit.Sp(11), "Send")
						lbl.Color = fg
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					})
					call := macro.Stop()
					rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(4)))
					paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
					call.Add(gtx.Ops)
					return dims
				})
			}),
		)
	})
}

func (t *RequestTab) layoutWSOpcodeSelector(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	current := "TEXT"
	if !s.OpcodeText {
		current = "BIN"
	}
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !s.OpcodeMenuOpen {
				return layout.Dimensions{}
			}
			macro := op.Record(gtx.Ops)
			layout.Inset{Top: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{
					Color:        theme.BorderLight,
					CornerRadius: unit.Dp(2),
					Width:        unit.Dp(1),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
							paint.FillShape(gtx.Ops, theme.BgMenu, rect.Op(gtx.Ops))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							rowW := gtx.Dp(unit.Dp(120))
							menuItem := func(clk *widget.Clickable, name string, active bool) layout.FlexChild {
								return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = rowW
									gtx.Constraints.Max.X = rowW
									return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
										if clk.Hovered() {
											paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: image.Pt(rowW, gtx.Dp(unit.Dp(28)))}.Op())
										}
										pointer.CursorPointer.Add(gtx.Ops)
										return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											display := "  " + name
											if active {
												display = "✓ " + name
											}
											return widgets.MonoLabel(th, unit.Sp(11), display).Layout(gtx)
										})
									})
								})
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								menuItem(&s.OpcodeTextChoice, "TEXT", s.OpcodeText),
								menuItem(&s.OpcodeBinChoice, "BIN", !s.OpcodeText),
							)
						}),
					)
				})
			})
			op.Defer(gtx.Ops, macro.Stop())
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &s.OpcodeMenuBtn, func(gtx layout.Context) layout.Dimensions {
				bg := theme.BgField
				if s.OpcodeMenuBtn.Hovered() {
					bg = theme.BgHover
				}
				pointer.CursorPointer.Add(gtx.Ops)
				macro := op.Record(gtx.Ops)
				dim := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), "Opcode:")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), current)
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							is := gtx.Dp(unit.Dp(12))
							gtx.Constraints.Min = image.Pt(is, is)
							gtx.Constraints.Max = gtx.Constraints.Min
							return widgets.IconDropDown.Layout(gtx, theme.FgMuted)
						}),
					)
				})
				call := macro.Stop()
				rrFill := clip.UniformRRect(image.Rectangle{Max: dim.Size}, gtx.Dp(unit.Dp(4)))
				paint.FillShape(gtx.Ops, bg, rrFill.Op(gtx.Ops))
				widgets.PaintBorder1px(gtx, dim.Size, theme.Border)
				call.Add(gtx.Ops)
				return dim
			})
		}),
	)
}

func (t *RequestTab) layoutWSMessagesPane(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return t.layoutWSStatusRow(gtx, th) }),
			layout.Rigid(wsHLine),
			layout.Rigid(wsTableHeader(th)),
			layout.Rigid(wsHLine),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return t.layoutWSMessagesList(gtx, th) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.Selected < 0 {
					return layout.Dimensions{}
				}
				return wsHLine(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.Selected < 0 {
					return layout.Dimensions{}
				}
				return t.layoutWSDetail(gtx, th)
			}),
		)
	})
}

func wsHeaderContentHeight(gtx layout.Context, th *material.Theme) int {
	lineH, _ := widgets.LineMetrics(gtx, th, unit.Sp(12))
	return lineH + 2*gtx.Dp(unit.Dp(6))
}

func (t *RequestTab) layoutWSStatusRow(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = wsHeaderContentHeight(gtx, th)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return wsStatePill(gtx, th, s.State(), s.statusErr)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				msg := s.statusText + s.formatNegotiated()
				if msg == "" {
					msg = "Idle"
				}
				col := theme.Fg
				if s.statusErr {
					col = theme.Danger
				}
				lbl := widgets.MonoLabel(th, unit.Sp(11), msg)
				lbl.Color = col
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsMiniToggle(gtx, th, &s.FilterPingBtn, "PING", !s.Filter.HidePing)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsMiniToggle(gtx, th, &s.FilterPongBtn, "PONG", !s.Filter.HidePong)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return wsMiniToggle(gtx, th, &s.FilterCloseBtn, "CLOSE", !s.Filter.HideClose)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if s.State() != WSStateOpen {
					return layout.Dimensions{}
				}
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return wsMiniBtn(gtx, th, &s.PingBtn, "Ping", theme.BgField, th.Fg)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return wsMiniBtn(gtx, th, &s.DisconnectBtn, "DC", theme.Cancel, th.Fg)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if len(s.Messages) == 0 {
					return layout.Dimensions{}
				}
				return wsMiniBtn(gtx, th, &s.ClearBtn, "Clear", theme.BgField, th.Fg)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		)
	})
}

func wsTableHeader(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(22)))}.Op())
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				wsColHeader(th, "Time", wsColTime),
				wsColHeader(th, "Dir", wsColDir),
				wsColHeader(th, "Op", wsColOp),
				wsColHeader(th, "Data", 0),
				wsColHeaderRight(th, "Size", wsColSize),
			)
		})
	}
}

const (
	wsColTime = 92
	wsColDir  = 28
	wsColOp   = 56
	wsColSize = 60
)

func (t *RequestTab) layoutWSMessagesList(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	s.sessionMu.Lock()
	msgs := s.filteredView()
	s.sessionMu.Unlock()
	if len(msgs) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, unit.Sp(11), "No messages yet")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		})
	}
	for len(s.RowClicks) < len(msgs) {
		s.RowClicks = append(s.RowClicks, &widget.Clickable{})
	}
	for i := range msgs {
		if s.RowClicks[i].Clicked(gtx) {
			s.Selected = msgs[i].id
		}
	}
	return material.List(th, &s.MessagesList).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
		return wsMessageRow(gtx, th, msgs[i].WSDisplayMessage, s.RowClicks[i], s.Selected == msgs[i].id)
	})
}

type indexedMessage struct {
	WSDisplayMessage
	id int
}

func (s *WSSession) filteredView() []indexedMessage {
	out := make([]indexedMessage, 0, len(s.Messages))
	for i, m := range s.Messages {
		if s.Filter.HidePing && m.Opcode == ws.OpPing {
			continue
		}
		if s.Filter.HidePong && m.Opcode == ws.OpPong {
			continue
		}
		if s.Filter.HideClose && m.Opcode == ws.OpClose {
			continue
		}
		out = append(out, indexedMessage{WSDisplayMessage: m, id: i})
	}
	return out
}

func wsMessageRow(gtx layout.Context, th *material.Theme, m WSDisplayMessage, clk *widget.Clickable, selected bool) layout.Dimensions {
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
		if m.IsSep {
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := widgets.MonoLabel(th, unit.Sp(11), m.Note)
				lbl.Color = theme.Accent
				lbl.Alignment = text.Middle
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			})
		}
		if m.Error != "" {
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					wsCellText(th, m.Time.Format("15:04:05.000"), wsColTime, text.Start, theme.FgMuted, true),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(th, unit.Sp(11), "ERR  "+m.Error)
						lbl.Color = theme.Danger
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					}),
				)
			})
		}
		if m.Note != "" && m.Opcode == 0 && m.Dir == 0 && len(m.Payload) == 0 {
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					wsCellText(th, m.Time.Format("15:04:05.000"), wsColTime, text.Start, theme.FgMuted, true),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(th, unit.Sp(11), m.Note)
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
				)
			})
		}
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				wsCellText(th, m.Time.Format("15:04:05.000"), wsColTime, text.Start, theme.FgMuted, true),
				wsCellDir(th, m.Dir, wsColDir),
				wsCellOp(th, m.Opcode, wsColOp),
				wsCellPreview(th, m),
				wsCellText(th, humanBytes(int64(len(m.Payload))), wsColSize, text.End, theme.FgMuted, true),
			)
		})
	})
}

func wsCellDir(th *material.Theme, d ws.Dir, w int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		var sym string
		col := theme.FgMuted
		if d == ws.DirOut {
			sym = "▲"
			col = theme.Accent
		} else {
			sym = "▼"
			col = theme.VarFound
		}
		lbl := widgets.MonoLabel(th, unit.Sp(11), sym)
		lbl.Color = col
		lbl.Font.Weight = font.Bold
		return lbl.Layout(gtx)
	})
}

func wsCellOp(th *material.Theme, op ws.Opcode, w int) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		col := theme.Fg
		switch op {
		case ws.OpText:
			col = theme.Accent
		case ws.OpBinary:
			col = theme.VarFound
		case ws.OpPing, ws.OpPong:
			col = theme.FgMuted
		case ws.OpClose:
			col = theme.Danger
		}
		lbl := widgets.MonoLabel(th, unit.Sp(11), op.String())
		lbl.Color = col
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func wsCellPreview(th *material.Theme, m WSDisplayMessage) layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		preview := previewPayload(m.Payload, m.Opcode)
		lbl := widgets.MonoLabel(th, unit.Sp(11), preview)
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		return lbl.Layout(gtx)
	})
}

func previewPayload(p []byte, op ws.Opcode) string {
	if op == ws.OpClose {
		if len(p) >= 2 {
			code, reason := ws.ParseClosePayload(p)
			if reason == "" {
				return fmt.Sprintf("code=%d", code)
			}
			return fmt.Sprintf("code=%d %q", code, reason)
		}
		return ""
	}
	if op == ws.OpBinary || !utf8.Valid(p) {
		if len(p) > 64 {
			return hex.EncodeToString(p[:64]) + "…"
		}
		return hex.EncodeToString(p)
	}
	s := string(p)
	if len(s) > 256 {
		s = s[:256] + "…"
	}
	return s
}

func humanBytes(n int64) string {
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

func (s *WSSession) refreshDetail() {
	if s.Selected < 0 {
		s.DetailSrcID = -1
		return
	}
	s.sessionMu.Lock()
	if s.Selected >= len(s.Messages) {
		s.sessionMu.Unlock()
		s.Selected = -1
		s.DetailSrcID = -1
		return
	}
	msg := s.Messages[s.Selected]
	s.sessionMu.Unlock()
	if s.DetailSrcID == s.Selected && s.DetailSrcHex == s.DetailHex {
		return
	}
	text := detailText(msg, s.DetailHex)
	s.DetailEditor.SetText(text)
	s.DetailSrcID = s.Selected
	s.DetailSrcHex = s.DetailHex
}

func detailText(m WSDisplayMessage, asHex bool) string {
	if m.Opcode == ws.OpClose && len(m.Payload) >= 2 && !asHex {
		code, reason := ws.ParseClosePayload(m.Payload)
		return fmt.Sprintf("code=%d\nreason=%s", code, reason)
	}
	if asHex {
		return hexDump(m.Payload)
	}
	if utf8.Valid(m.Payload) {
		return string(m.Payload)
	}
	return hexDump(m.Payload)
}

func hexDump(p []byte) string {
	if len(p) == 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(len(p) * 3)
	for i, byteVal := range p {
		if i > 0 {
			if i%16 == 0 {
				b.WriteByte('\n')
			} else if i%8 == 0 {
				b.WriteString("  ")
			} else {
				b.WriteByte(' ')
			}
		}
		const hexChars = "0123456789abcdef"
		b.WriteByte(hexChars[byteVal>>4])
		b.WriteByte(hexChars[byteVal&0x0F])
	}
	return b.String()
}

func (t *RequestTab) layoutWSDetail(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	s.sessionMu.Lock()
	if s.Selected < 0 || s.Selected >= len(s.Messages) {
		s.sessionMu.Unlock()
		return layout.Dimensions{}
	}
	msg := s.Messages[s.Selected]
	s.sessionMu.Unlock()

	gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(220))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(th, unit.Sp(11), fmt.Sprintf("Detail • %s • %s • %s",
							msg.Time.Format("15:04:05.000"),
							dirString(msg.Dir),
							msg.Opcode.String()))
						lbl.Font.Weight = font.Bold
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
					layout.Flexed(1, layout.Spacer{}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return wsOptionToggle(gtx, th, &s.DetailTextBtn, "TEXT", !s.DetailHex)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return wsOptionToggle(gtx, th, &s.DetailHexBtn, "HEX", s.DetailHex)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.SquareBtn(gtx, &s.DetailCopyBtn, iconCopy, th)
					}),
				)
			})
		}),
		layout.Rigid(wsHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			bdr := gtx.Dp(unit.Dp(1))
			sz := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
			inner := image.Rect(bdr, bdr, sz.X-bdr, sz.Y-bdr)
			paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
			gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
			gtx.Constraints.Max = gtx.Constraints.Min
			op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
			return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(th, &s.DetailEditor, "")
				ed.TextSize = unit.Sp(11)
				ed.Font.Typeface = widgets.MonoTypeface
				return ed.Layout(gtx)
			})
		}),
	)
}

func dirString(d ws.Dir) string {
	if d == ws.DirOut {
		return "OUT ▲"
	}
	return "IN ▼"
}

func (s *WSSession) formatNegotiated() string {
	if s.State() != WSStateOpen {
		return ""
	}
	var b strings.Builder
	if s.subprotocol != "" {
		b.WriteString("  •  subprotocol=")
		b.WriteString(s.subprotocol)
	}
	if s.negotiatedExt.Negotiated {
		b.WriteString("  •  deflate")
	}
	return b.String()
}

func wsSubprotoRow(gtx layout.Context, th *material.Theme, sp *WSSubprotoItem, env map[string]string) layout.Dimensions {
	fieldH := gtx.Dp(unit.Dp(26))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = fieldH
			gtx.Constraints.Max.Y = fieldH
			return widgets.TextFieldOverlay(gtx, th, &sp.Editor, "subprotocol", true, env, 0, unit.Sp(11))
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bw := gtx.Dp(unit.Dp(20))
			gtx.Constraints.Min = image.Pt(bw, fieldH)
			gtx.Constraints.Max = gtx.Constraints.Min
			return sp.DelBtn.Layout(gtx, deleteButtonInside)
		}),
	)
}

func wsHLine(gtx layout.Context) layout.Dimensions {
	h := gtx.Dp(unit.Dp(1))
	paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, h)}.Op())
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
}

func wsOptionToggle(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool) layout.Dimensions {
	return wsToggleSized(gtx, th, clk, label, on, unit.Sp(11), layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)})
}

func wsMiniToggle(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool) layout.Dimensions {
	return wsToggleSized(gtx, th, clk, label, on, unit.Sp(9), layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)})
}

func wsToggleSized(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool, sz unit.Sp, inset layout.Inset) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := theme.BgField
		fg := th.Fg
		if on {
			bg = theme.Accent
			fg = th.ContrastFg
		}
		pointer.CursorPointer.Add(gtx.Ops)
		macro := op.Record(gtx.Ops)
		dims := inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, sz, label)
			lbl.Color = fg
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(3)))
		paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
		widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
		call.Add(gtx.Ops)
		return dims
	})
}

func wsMiniBtn(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, bg color.NRGBA, fg color.NRGBA) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		pointer.CursorPointer.Add(gtx.Ops)
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, unit.Sp(9), label)
			lbl.Color = fg
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
		call := macro.Stop()
		rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(3)))
		paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
		widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
		call.Add(gtx.Ops)
		return dims
	})
}

func wsStatePill(gtx layout.Context, th *material.Theme, st WSState, isErr bool) layout.Dimensions {
	var label string
	var col color.NRGBA
	switch st {
	case WSStateOpen:
		label = "OPEN"
		col = theme.VarFound
	case WSStateConnecting:
		label = "CONNECTING"
		col = theme.Accent
	case WSStateClosing:
		label = "CLOSING"
		col = theme.FgMuted
	case WSStateClosed:
		label = "CLOSED"
		col = theme.FgMuted
		if isErr {
			col = theme.Danger
		}
	default:
		label = "IDLE"
		col = theme.FgMuted
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		lbl := widgets.MonoLabel(th, unit.Sp(10), label)
		lbl.Color = th.ContrastFg
		lbl.Font.Weight = font.Bold
		return lbl.Layout(gtx)
	})
	call := macro.Stop()
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rectangle{Max: dims.Size}, 3).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

func wsColHeader(th *material.Theme, s string, w int) layout.FlexChild {
	return wsColHeaderAligned(th, s, w, text.Start)
}

func wsColHeaderRight(th *material.Theme, s string, w int) layout.FlexChild {
	return wsColHeaderAligned(th, s, w, text.End)
}

func wsColHeaderAligned(th *material.Theme, s string, w int, al text.Alignment) layout.FlexChild {
	if w == 0 {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, unit.Sp(10), s)
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
		lbl := widgets.MonoLabel(th, unit.Sp(10), s)
		lbl.Color = theme.FgMuted
		lbl.Font.Weight = font.Bold
		lbl.Alignment = al
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func wsCellText(th *material.Theme, s string, w int, al text.Alignment, col color.NRGBA, mono bool) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if w > 0 {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
		}
		var lbl material.LabelStyle
		if mono {
			lbl = widgets.MonoLabel(th, unit.Sp(11), s)
		} else {
			lbl = material.Label(th, unit.Sp(11), s)
		}
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		lbl.Color = col
		return lbl.Layout(gtx)
	})
}
