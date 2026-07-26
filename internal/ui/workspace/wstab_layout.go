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
	var compMinPx, msgsMinPx float32
	if stacked {
		ratio = &s.ComposerRatio
		flexExtent = float32(gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(2)) - t.layoutModeBarHeight(gtx) - gtx.Dp(unit.Dp(4)))
		if pool := s.splitPaneRec + s.msgsPaneRec; pool > 0 {
			flexExtent = float32(pool)
		}
		if flexExtent < 1 {
			flexExtent = 1
		}
		dragAxis = gesture.Vertical
		compMinPx = float32(s.composerMinPx(gtx))
		msgsMinPx = float32(gtx.Dp(unit.Dp(120)))
		if s.MessagesCollapsed {
			msgsMinPx = float32(s.msgsCollapsedMinPx(gtx))
		}
	} else {
		ratio = &s.SplitRatio
		flexExtent = float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
		if pool := s.splitPaneRec + s.msgsPaneRec; pool > 0 {
			flexExtent = float32(pool)
		}
		dragAxis = gesture.Horizontal
		compMinPx = 0.2 * flexExtent
		msgsMinPx = 0.2 * flexExtent
	}

	for s.ComposeCollapseBtn.Clicked(gtx) {
		s.ComposeCollapsed = !s.ComposeCollapsed
		if stacked && flexExtent > 0 {
			if s.ComposeCollapsed {
				s.composeSavedRatio = *ratio
				*ratio = float32(s.composerMinPx(gtx)) / flexExtent
			} else {
				restore := s.composeSavedRatio
				minOpen := (float32(s.composerMinPx(gtx)) + float32(gtx.Dp(unit.Dp(120)))) / flexExtent
				if restore < minOpen {
					restore = minOpen
				}
				*ratio = restore
			}
			compMinPx = float32(s.composerMinPx(gtx))
		}
		win.Invalidate()
	}
	for s.MessagesCollapseBtn.Clicked(gtx) {
		s.MessagesCollapsed = !s.MessagesCollapsed
		if stacked && flexExtent > 0 {
			if s.MessagesCollapsed {
				s.msgsSavedRatio = *ratio
				msgsMinPx = float32(s.msgsCollapsedMinPx(gtx))
				*ratio = 1 - msgsMinPx/flexExtent
			} else {
				msgsMinPx = float32(gtx.Dp(unit.Dp(120)))
				restore := s.msgsSavedRatio
				maxOpen := 1 - msgsMinPx/flexExtent
				if restore <= 0 || restore > maxOpen {
					restore = maxOpen
				}
				*ratio = restore
			}
		}
		win.Invalidate()
	}

	var moved bool
	var finalPos float32
	var released bool
	for {
		e, ok := s.SplitDrag.Update(gtx.Metric, gtx.Source, dragAxis)
		if !ok {
			break
		}
		pos := float32(s.paneDrawn)
		if stacked {
			pos += e.Position.Y
		} else {
			pos += e.Position.X
		}
		switch e.Kind {
		case pointer.Press:
			s.SplitDragX = pos
			s.splitPanePx = *ratio * flexExtent
		case pointer.Drag:
			finalPos = pos
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	minR := float32(0.2)
	maxR := float32(0.8)
	if flexExtent > 0 {
		minR = compMinPx / flexExtent
		maxR = 1 - msgsMinPx/flexExtent
	}
	if minR > maxR {
		minR, maxR = 0.5, 0.5
		if flexExtent > 0 {
			r := compMinPx / flexExtent
			if cap := 1 - float32(s.msgsCollapsedMinPx(gtx))/flexExtent; r > cap {
				r = cap
			}
			if r < 0 {
				r = 0
			}
			minR, maxR = r, r
		}
	}
	if *ratio < minR {
		*ratio = minR
	}
	if *ratio > maxR {
		*ratio = maxR
	}

	if moved && flexExtent > 0 {
		delta := finalPos - s.SplitDragX
		oldSnap := int(*ratio*flexExtent + 0.5)
		newPane := s.splitPanePx + delta
		if stacked {
			if !s.ComposeCollapsed && newPane < float32(s.composerMinPx(gtx))-0.5 {
				s.ComposeCollapsed = true
				minR = float32(s.composerMinPx(gtx)) / flexExtent
			} else if s.ComposeCollapsed && newPane > float32(s.composerMinPx(gtx))+float32(gtx.Dp(unit.Dp(6))) {
				s.ComposeCollapsed = false
				minR = float32(s.composerMinPx(gtx)) / flexExtent
			}
			if s.MessagesCollapsed && flexExtent-newPane > float32(s.msgsCollapsedMinPx(gtx))+float32(gtx.Dp(unit.Dp(6))) {
				s.MessagesCollapsed = false
				maxR = 1 - float32(gtx.Dp(unit.Dp(120)))/flexExtent
			}
			if minR > maxR {
				r := float32(s.composerMinPx(gtx)) / flexExtent
				if cap := 1 - float32(s.msgsCollapsedMinPx(gtx))/flexExtent; r > cap {
					r = cap
				}
				if r < 0 {
					r = 0
				}
				minR, maxR = r, r
			}
		}
		s.splitPanePx = newPane
		if newPane < minR*flexExtent {
			newPane = minR * flexExtent
		} else if newPane > maxR*flexExtent {
			newPane = maxR * flexExtent
		}
		snap := oldSnap
		if d := newPane - float32(oldSnap); d >= 0.75 || d <= -0.75 {
			snap = int(newPane + 0.5)
		}
		*ratio = float32(snap) / flexExtent
		s.SplitDragX = finalPos
		win.Invalidate()
	}
	if released {
		win.Invalidate()
	}

	var hcMoved, hcReleased bool
	var hcPos float32
	for {
		e, ok := s.HeadersComposeDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			s.HeadersComposeDragX = e.Position.Y + float32(s.hcSliderY)
			s.hcHeadersPx = 0
			if !s.HeadersCollapsed {
				s.hcHeadersPx = float32(s.headersRenderH)
			}
			if stacked && flexExtent > 0 {
				headersNow := 0
				if !s.HeadersCollapsed {
					headersNow = s.headersRenderH
				}
				s.hcComposePx = int(*ratio*flexExtent) - s.composerChromeExceptHeadersPx(gtx) - gtx.Dp(unit.Dp(3)) - headersNow
				if s.hcComposePx < 0 {
					s.hcComposePx = 0
				}
			}
		case pointer.Drag:
			hcPos = e.Position.Y + float32(s.hcSliderY)
			hcMoved = true
		case pointer.Cancel, pointer.Release:
			hcReleased = true
		}
	}
	if hcMoved {
		delta := hcPos - s.HeadersComposeDragX
		oldSnap := float32(0)
		if !s.HeadersCollapsed {
			oldSnap = float32(gtx.Dp(unit.Dp(s.HeadersAbsHeight)))
		}
		newH := s.hcHeadersPx + delta
		wasCollapsed := s.HeadersCollapsed
		s.HeadersCollapsed = false
		chromeOpen := float32(s.composerChromeExceptHeadersPx(gtx))
		s.HeadersCollapsed = wasCollapsed
		pad := float32(gtx.Dp(unit.Dp(3)))
		hcMax := newH
		if stacked && flexExtent > 0 {
			hcMax = flexExtent - msgsMinPx - chromeOpen - pad - float32(s.hcComposePx)
		} else if s.composerPaneH > 0 {
			hcMax = float32(s.composerPaneH) - chromeOpen
		}
		if hcMax < 0 {
			hcMax = 0
		}
		s.hcHeadersPx = newH
		if newH > hcMax {
			newH = hcMax
		}
		if newH < 0 {
			newH = 0
		}
		if newH <= 0.5 {
			if !s.HeadersCollapsed {
				s.HeadersCollapsed = true
				s.HeadersAbsHeight = 120
			}
			if stacked && flexExtent > 0 {
				*ratio = (float32(s.composerChromeExceptHeadersPx(gtx)) + pad + float32(s.hcComposePx)) / flexExtent
			}
		} else {
			wasOpen := !s.HeadersCollapsed
			s.HeadersCollapsed = false
			if d := newH - oldSnap; !wasOpen || d >= 0.75 || d <= -0.75 {
				s.HeadersAbsHeight = int(newH/gtx.Metric.PxPerDp + 0.5)
			}
			if stacked && flexExtent > 0 {
				newSnap := float32(gtx.Dp(unit.Dp(s.HeadersAbsHeight)))
				*ratio = (chromeOpen + pad + newSnap + float32(s.hcComposePx)) / flexExtent
			}
		}
		s.HeadersComposeDragX = hcPos
		win.Invalidate()
	}
	if hcReleased {
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
				return t.layoutModeBar(gtx, th, &t.LayoutHorizBtn, &t.LayoutVertBtn, stacked)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: flexAxis}.Layout(gtx,
					layout.Flexed(*ratio, func(gtx layout.Context) layout.Dimensions {
						if stacked {
							s.splitPaneRec = gtx.Constraints.Max.Y
						} else {
							s.splitPaneRec = gtx.Constraints.Max.X
						}
						d := leftInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return t.layoutWSComposerPane(gtx, th, activeEnv)
						})
						if stacked {
							s.paneDrawn = d.Size.Y
						} else {
							s.paneDrawn = d.Size.X
						}
						return d
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
						if stacked {
							s.msgsPaneRec = gtx.Constraints.Max.Y
						} else {
							s.msgsPaneRec = gtx.Constraints.Max.X
						}
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
		s.OptionsExpanded = true
		s.AddSubprotocol("")
		s.FitSubprotos = true
	}
	for s.HeadersAddBtn.Clicked(gtx) {
		s.HeadersCollapsed = false
		t.AddHeader("", "")
	}
	for s.HeadersCollapseBtn.Clicked(gtx) {
		s.HeadersCollapsed = !s.HeadersCollapsed
	}
	for s.OfferDeflateBtn.Clicked(gtx) {
		s.OfferDeflate = !s.OfferDeflate
	}
	for s.MsgpackProtoBtn.Clicked(gtx) {
		s.UseMsgpackProto = !s.UseMsgpackProto
	}
	for i := 0; i < len(t.Headers); i++ {
		if t.Headers[i].DelBtn.Clicked(gtx) {
			t.Headers = append(t.Headers[:i], t.Headers[i+1:]...)
			i--
		}
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
	for s.ComposerCopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(s.ComposerEditor.Text())),
		})
	}
	for s.ComposerSendBtn.Clicked(gtx) {
		if s.State() == WSStateOpen {
			t.SendFromComposer()
		}
	}
	for s.FilterMenuBtn.Clicked(gtx) {
		s.FilterMenuOpen = !s.FilterMenuOpen
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
	if s.UseMsgpackProto {
		t.WSSendProto(txt)
		return
	}
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

	subprotosBody := func(gtx layout.Context) layout.Dimensions {
		bdr := gtx.Dp(unit.Dp(1))
		sz := gtx.Constraints.Max
		paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
		inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
		paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
		gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
		gtx.Constraints.Max = gtx.Constraints.Min
		op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
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
	}

	headersBody := func(gtx layout.Context) layout.Dimensions {
		bdr := gtx.Dp(unit.Dp(1))
		sz := gtx.Constraints.Max
		paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
		inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
		paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.Rect(inner).Op())
		gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
		gtx.Constraints.Max = gtx.Constraints.Min
		op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
		if len(t.Headers) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := widgets.MonoLabel(th, unit.Sp(11), "No headers — add Origin, Cookie, etc.")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			})
		}
		minKey := widgets.KVKeysMinWidth(gtx, th, len(t.Headers), func(i int) *widget.Editor { return &t.Headers[i].Key })
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return widgets.VScrollList(gtx, th, &t.HeadersList, len(t.Headers), func(gtx layout.Context, i int) layout.Dimensions {
				hd := t.Headers[i]
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widgets.KVRow(gtx, th, &hd.Key, &hd.Value, &hd.DelBtn, &t.HeaderKeyW, &hd.SplitDrag, &hd.splitLastX, &t.HeaderKeyBelowMin, minKey, activeEnv, &hd.RowHover, &hd.RowFade)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i >= len(t.Headers)-1 {
							return layout.Dimensions{}
						}
						return rowDivider(gtx)
					}),
				)
			})
		})
	}

	composeBody := func(gtx layout.Context) layout.Dimensions {
		bdr := gtx.Dp(unit.Dp(1))
		sz := gtx.Constraints.Max
		paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
		inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
		paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
		gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
		gtx.Constraints.Max = gtx.Constraints.Min
		op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
		return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			ed := material.Editor(th, &s.ComposerEditor, "type your message")
			ed.TextSize = unit.Sp(12)
			ed.HintColor = theme.FgMuted
			ed.Font.Typeface = widgets.MonoTypeface
			return ed.Layout(gtx)
		})
	}

	s.composerPaneH = gtx.Constraints.Max.Y

	sliderTop := 0
	acc := func(w layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := w(gtx)
			sliderTop += d.Size.Y
			return d
		})
	}
	children := []layout.FlexChild{
		acc(func(gtx layout.Context) layout.Dimensions {
			d := t.layoutWSConnectionHeader(gtx, th)
			s.wsRowH = d.Size.Y
			return d
		}),
	}
	if s.OptionsExpanded {
		children = append(children,
			acc(wsHLine),
			acc(func(gtx layout.Context) layout.Dimensions {
				d := t.layoutWSOptions(gtx, th)
				s.optionsRowH = d.Size.Y
				return d
			}),
		)
		if s.FitSubprotos {
			fit := len(s.Subprotocols)*28 + 11
			if fit > s.SubprotosAbsHeight {
				s.SubprotosAbsHeight = fit
			}
			s.FitSubprotos = false
		}
		if len(s.Subprotocols) > 0 {
			children = append(children,
				acc(wsHLine),
				acc(func(gtx layout.Context) layout.Dimensions {
					h := s.subprotosListPx(gtx)
					if avail := s.composerPaneH - (s.composerChromeExceptHeadersPx(gtx) - h) - gtx.Dp(unit.Dp(3)); h > avail {
						h = avail
					}
					if h < 0 {
						h = 0
					}
					gtx.Constraints.Min.Y = h
					gtx.Constraints.Max.Y = h
					d := subprotosBody(gtx)
					d.Size.Y = h
					return d
				}),
			)
		}
	}
	children = append(children,
		acc(wsHLine),
		acc(func(gtx layout.Context) layout.Dimensions { return t.layoutWSHeadersHeader(gtx, th) }),
	)
	if !s.HeadersCollapsed {
		children = append(children,
			acc(wsHLine),
			acc(func(gtx layout.Context) layout.Dimensions {
				if s.HeadersAbsHeight <= 0 {
					s.HeadersAbsHeight = 120
				}
				h := gtx.Dp(unit.Dp(s.HeadersAbsHeight))
				available := s.composerPaneH - s.composerChromeExceptHeadersPx(gtx)
				if available < 0 {
					available = 0
				}
				if h > available {
					h = available
				}
				if h < 0 {
					h = 0
				}
				s.headersRenderH = h
				gtx.Constraints.Min.Y = h
				gtx.Constraints.Max.Y = h
				d := headersBody(gtx)
				d.Size.Y = h
				return d
			}),
		)
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			s.hcSliderY = sliderTop
			thick := gtx.Dp(unit.Dp(4))
			size := image.Point{X: gtx.Constraints.Max.X, Y: thick}
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			pointer.CursorRowResize.Add(gtx.Ops)
			s.HeadersComposeDrag.Add(gtx.Ops)
			for {
				_, ok := gtx.Event(pointer.Filter{Target: &s.HeadersComposeDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
				if !ok {
					break
				}
			}
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(wsHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return t.layoutWSComposerHeader(gtx, th) }),
	)
	if !s.ComposeCollapsed {
		children = append(children,
			layout.Rigid(wsHLine),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return t.layoutWSProtoFields(gtx, th) }),
			layout.Flexed(1, composeBody),
		)
	}

	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (s *WSSession) sectionRowPx(gtx layout.Context) int {
	if s.wsRowH > 0 {
		return s.wsRowH
	}
	return gtx.Dp(unit.Dp(32))
}

func (s *WSSession) subprotosListPx(gtx layout.Context) int {
	if s.SubprotosAbsHeight <= 0 {
		s.SubprotosAbsHeight = 90
	}
	h := gtx.Dp(unit.Dp(s.SubprotosAbsHeight))
	if maxH := gtx.Dp(unit.Dp(200)); h > maxH {
		h = maxH
	}
	if minH := gtx.Dp(unit.Dp(60)); h < minH {
		h = minH
	}
	return h
}

func (s *WSSession) composerChromeExceptHeadersPx(gtx layout.Context) int {
	row := s.sectionRowPx(gtx)
	line := gtx.Dp(unit.Dp(1))
	h := row
	if s.OptionsExpanded {
		opt := s.optionsRowH
		if opt <= 0 {
			opt = gtx.Dp(unit.Dp(33))
		}
		h += line + opt
		if len(s.Subprotocols) > 0 {
			h += line + s.subprotosListPx(gtx)
		}
	}
	h += line + row
	if !s.HeadersCollapsed {
		h += line
	}
	h += gtx.Dp(unit.Dp(4))
	h += line + row
	if !s.ComposeCollapsed {
		h += line
		if s.UseMsgpackProto {
			h += gtx.Dp(unit.Dp(31))
		}
	}
	return h + 2*line
}

func (s *WSSession) composerMinPx(gtx layout.Context) int {
	h := s.composerChromeExceptHeadersPx(gtx) + gtx.Dp(unit.Dp(3))
	if !s.HeadersCollapsed {
		hd := s.HeadersAbsHeight
		if hd <= 0 {
			hd = 120
		}
		h += gtx.Dp(unit.Dp(hd))
	}
	return h
}

func (s *WSSession) msgsCollapsedMinPx(gtx layout.Context) int {
	h := s.statusRowH
	if h <= 0 {
		h = gtx.Dp(unit.Dp(32))
	}
	return h + 2*gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(2))
}

func (t *RequestTab) layoutWSProtoFields(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	if !s.UseMsgpackProto {
		return layout.Dimensions{}
	}
	field := func(label string, ed *widget.Editor, w int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(11), label)
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					fw := gtx.Dp(unit.Dp(float32(w)))
					fh := gtx.Dp(unit.Dp(22))
					gtx.Constraints.Min = image.Pt(fw, fh)
					gtx.Constraints.Max = image.Pt(fw, fh)
					return widget.Border{Color: theme.Border, CornerRadius: unit.Dp(2), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						paint.FillShape(gtx.Ops, theme.BgField, clip.Rect{Max: gtx.Constraints.Min}.Op())
						return layout.UniformInset(unit.Dp(3)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							e := material.Editor(th, ed, "0")
							e.TextSize = unit.Sp(11)
							e.Font.Typeface = widgets.MonoTypeface
							e.HintColor = theme.FgMuted
							return e.Layout(gtx)
						})
					})
				}),
			)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					field("cmd", &s.ProtoCmdEditor, 48),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					field("seq", &s.ProtoSeqEditor, 64),
					layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
					field("opcode", &s.ProtoOpcodeEditor, 64),
				)
			})
		}),
		layout.Rigid(wsHLine),
	)
}

func (t *RequestTab) layoutWSConnectionHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Connection")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &s.AddSubprotoBtn, widgets.IconAdd, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return collapseChevron(gtx, th, &s.OptionsBtn, !s.OptionsExpanded)
			}),
		)
	})
}

func (t *RequestTab) layoutWSHeadersHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Headers")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &s.HeadersAddBtn, widgets.IconAdd, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return collapseChevron(gtx, th, &s.HeadersCollapseBtn, s.HeadersCollapsed)
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
				return wsOptionToggle(gtx, th, &s.MsgpackProtoBtn, "lz4+msgpack", s.UseMsgpackProto)
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
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Compose")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutWSOpcodeSelector(gtx, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &s.ComposerWrapBtn, iconWrap, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &s.ComposerCopyBtn, iconCopy, th)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				enabled := s.State() == WSStateOpen
				bg := theme.BtnPrimary
				fg := theme.BtnPrimaryFg
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
						lbl.MaxLines = 1
						return lbl.Layout(gtx)
					})
					call := macro.Stop()
					rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(4)))
					paint.FillShape(gtx.Ops, bg, rr.Op(gtx.Ops))
					call.Add(gtx.Ops)
					return dims
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return collapseChevron(gtx, th, &s.ComposeCollapseBtn, s.ComposeCollapsed)
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
			anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(28)))}
			widgets.DeferMenuAt(gtx, th, &s.OpcodeMenuOpen, anchor, 120, []widgets.MenuItem{
				{Label: "TEXT", Click: &s.OpcodeTextChoice, Checked: s.OpcodeText, Mono: true},
				{Label: "BIN", Click: &s.OpcodeBinChoice, Checked: !s.OpcodeText, Mono: true},
			})
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
				paint.FillShape(gtx.Ops, bg, clip.Rect{Max: dim.Size}.Op())
				widgets.PaintBorder1px(gtx, dim.Size, theme.Border)
				call.Add(gtx.Ops)
				return dim
			})
		}),
	)
}

func (t *RequestTab) layoutWSMessagesPane(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	if s.MessagesCollapsed && s.statusRowH > 0 {
		capped := s.statusRowH + 2*gtx.Dp(unit.Dp(1))
		if gtx.Constraints.Max.Y > capped {
			gtx.Constraints.Max.Y = capped
		}
		gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
	}
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		statusRow := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := t.layoutWSStatusRow(gtx, th)
			s.statusRowH = d.Size.Y
			return d
		})
		if s.MessagesCollapsed {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, statusRow)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			statusRow,
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
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					statusText, statusErr := s.statusSnapshot()
					msg := statusText + s.formatNegotiated()
					if msg == "" {
						msg = "Idle"
					}
					col := theme.Fg
					if statusErr {
						col = theme.Danger
					}
					lbl := widgets.MonoLabel(th, unit.Sp(12), msg)
					lbl.Color = col
					lbl.Font.Weight = font.Bold
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					return lbl.Layout(gtx)
				})
			}),
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
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutWSFilterMenu(gtx, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &s.ClearBtn, widgets.IconClear, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return collapseChevron(gtx, th, &s.MessagesCollapseBtn, s.MessagesCollapsed)
			}),
		)
	})
}

func (t *RequestTab) layoutWSFilterMenu(gtx layout.Context, th *material.Theme) layout.Dimensions {
	s := t.EnsureWS()
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !s.FilterMenuOpen {
				return layout.Dimensions{}
			}
			anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(28)))}
			widgets.DeferMenuAt(gtx, th, &s.FilterMenuOpen, anchor, 160, []widgets.MenuItem{
				{Label: "Show PING", Click: &s.FilterPingBtn, Checked: !s.Filter.HidePing, Mono: true},
				{Label: "Show PONG", Click: &s.FilterPongBtn, Checked: !s.Filter.HidePong, Mono: true},
				{Label: "Show CLOSE", Click: &s.FilterCloseBtn, Checked: !s.Filter.HideClose, Mono: true},
			})
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return widgets.SquareBtn(gtx, &s.FilterMenuBtn, widgets.IconMore, th)
		}),
	)
}

func wsTableHeader(th *material.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(22)))}.Op())
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				wsColHeader(th, "Sess", wsColSess),
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
	wsColSess = 32
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
		sessLabel := ""
		if m.Session > 0 {
			sessLabel = "#" + itoa(m.Session)
		}
		if m.Error != "" {
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					wsCellText(th, sessLabel, wsColSess, text.Start, theme.FgMuted, true),
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
					wsCellText(th, sessLabel, wsColSess, text.Start, theme.FgMuted, true),
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
				wsCellText(th, sessLabel, wsColSess, text.Start, theme.FgMuted, true),
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
		sym := "▼"
		col := theme.VarFound
		if d == ws.DirOut {
			sym = "▲"
			col = theme.Accent
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
		if m.Proto != nil {
			preview = previewProto(m.Proto)
		}
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
		cut := 256
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

func previewProto(p *ProtoView) string {
	head := fmt.Sprintf("cmd=%d seq=%d op=%d", p.Cmd, p.Seq, p.Opcode)
	if p.Cof > 0 {
		head += " lz4"
	}
	if p.DecodeErr != "" {
		return head + " ⚠ " + p.DecodeErr
	}
	body := strings.Join(strings.Fields(p.JSON), " ")
	if body == "" {
		return head
	}
	return head + "  " + body
}

func protoDetailText(p *ProtoView) string {
	var b strings.Builder
	fmt.Fprintf(&b, "cmd=%d  seq=%d  opcode=%d\n", p.Cmd, p.Seq, p.Opcode)
	if p.Cof > 0 {
		fmt.Fprintf(&b, "lz4 cof=%d  wire=%dB  msgpack=%dB\n\n", p.Cof, p.BodyLen, p.RawLen)
	} else {
		fmt.Fprintf(&b, "uncompressed  msgpack=%dB\n\n", p.RawLen)
	}
	if p.DecodeErr != "" {
		b.WriteString("decode error: " + p.DecodeErr)
		return b.String()
	}
	b.WriteString(p.JSON)
	return b.String()
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
	if m.Proto != nil && !asHex {
		return protoDetailText(m.Proto)
	}
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
	s.sessionMu.Lock()
	sub := s.subprotocol
	ext := s.negotiatedExt
	s.sessionMu.Unlock()
	var b strings.Builder
	if sub != "" {
		b.WriteString("  •  subprotocol=")
		b.WriteString(sub)
	}
	if ext.Negotiated {
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
			return sp.DelBtn.Layout(gtx, widgets.DeleteButtonInside)
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

func wsToggleSized(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, on bool, sz unit.Sp, inset layout.Inset) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := theme.BgField
		fg := th.Fg
		if on {
			bg = theme.BtnPrimary
			fg = theme.BtnPrimaryFg
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
