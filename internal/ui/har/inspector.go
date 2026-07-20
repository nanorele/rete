package har

import (
	"image"
	"io"
	"strconv"
	"strings"

	"tracto/internal/har"
	"tracto/internal/ui/settings"
	"tracto/pkg/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (s *Section) inspector(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	if s.SelReq < 0 || s.SelReq >= len(s.Doc.Entries) {
		return centered(th, gtx, "Select a request to inspect")
	}
	e := &s.Doc.Entries[s.SelReq]
	isWS := e.IsWebSocket()
	respLabel := "Response"
	respCount := ""
	if isWS {
		respLabel = "Messages"
		respCount = strconv.Itoa(len(e.WebSocketMessages))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.inspectorHeader(gtx, e) }),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.tab(gtx, &s.InspTabReq, "Request", "", s.InspTab == 0)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.tab(gtx, &s.InspTabResp, respLabel, respCount, s.InspTab == 1)
					}),
				)
			})
		}),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if s.InspTab == 1 {
				if isWS {
					identity := "ws/" + strconv.Itoa(s.SelReq)
					body := s.inspectorBody(identity+"|p="+boolStr(s.Pretty), func() []byte {
						return wsText(e, s.Pretty)
					})
					return s.bodyPane(gtx, e.Response.Headers, &s.RespHdrList, body, "websocket/frames", identity)
				}
				identity := "resp/" + strconv.Itoa(s.SelReq)
				body := s.inspectorBody(identity, func() []byte { return respBody(e) })
				return s.bodyPane(gtx, e.Response.Headers, &s.RespHdrList, body, e.ContentType(), identity)
			}
			identity := "req/" + strconv.Itoa(s.SelReq)
			body := s.inspectorBody(identity, func() []byte { return []byte(e.Request.PostData.Text) })
			return s.bodyPane(gtx, e.Request.Headers, &s.ReqHdrList, body, e.Request.PostData.MimeType, identity)
		}),
	)
}

func (s *Section) inspectorHeader(gtx layout.Context, e *har.Entry) layout.Dimensions {
	th := s.host.Theme
	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), displayMethod(e))
				lbl.Color = theme.MethodColor(e.Request.Method)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), e.Request.URL)
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), statusText(e))
				lbl.Color = statusColor(e.Response.Status)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn(gtx, th, &s.RunBtn, "Run", widgets.IconPlay, theme.BtnPrimary, theme.BtnPrimaryFg, true)
			}),
		)
	})
}

func paneRowPx(gtx layout.Context) int {
	return gtx.Dp(unit.Dp(32))
}

func paneRow(gtx layout.Context, rowH int, content layout.Widget) layout.Dimensions {
	inner := gtx
	inner.Constraints.Min.Y = 0
	inner.Constraints.Max.Y = rowH
	macro := op.Record(gtx.Ops)
	dims := content(inner)
	call := macro.Stop()
	off := (rowH - dims.Size.Y) / 2
	if off < 0 {
		off = 0
	}
	stk := op.Offset(image.Pt(0, off)).Push(gtx.Ops)
	call.Add(gtx.Ops)
	stk.Pop()
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, rowH)}
}

func paneSurface(gtx layout.Context, w layout.Widget) layout.Dimensions {
	bdr := gtx.Dp(unit.Dp(1))
	sz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
	inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
	paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.Rect(inner).Op())
	gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min
	defer op.Offset(image.Pt(bdr, 0)).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	w(gtx)
	return layout.Dimensions{Size: sz}
}

func (s *Section) bodyPane(gtx layout.Context, headers []har.Header, hdrList *widget.List, body []byte, mime, identity string) layout.Dimensions {
	th := s.host.Theme

	var hdrMoved bool
	var hdrFinalY float32
	for {
		e, ok := s.HdrSplitDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		pos := e.Position.Y + float32(s.hdrSliderY)
		switch e.Kind {
		case pointer.Press:
			s.HdrDragY = pos
			s.hdrPx = float32(s.hdrDrawnH)
		case pointer.Drag:
			hdrFinalY = pos
			hdrMoved = true
		}
	}
	if hdrMoved {
		s.hdrPx += hdrFinalY - s.HdrDragY
		s.HdrDragY = hdrFinalY
		s.HdrH = s.hdrPx / gtx.Metric.PxPerDp
		s.host.Window.Invalidate()
	}

	line := gtx.Dp(unit.Dp(1))
	rowH := paneRowPx(gtx)
	sliderH := gtx.Dp(unit.Dp(4))
	h := gtx.Dp(unit.Dp(s.HdrH))
	avail := gtx.Constraints.Max.Y - 2*rowH - 3*line - sliderH - gtx.Dp(unit.Dp(60))
	if h > avail {
		h = avail
	}
	if minH := gtx.Dp(unit.Dp(40)); h < minH {
		h = minH
	}
	s.hdrDrawnH = h
	s.hdrSliderY = rowH + line + h

	headersRow := func(gtx layout.Context) layout.Dimensions {
		return paneRow(gtx, rowH, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := widgets.MonoLabel(th, unit.Sp(12), "Headers")
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			})
		})
	}

	headersContent := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = h
		gtx.Constraints.Max.Y = h
		return paneSurface(gtx, func(gtx layout.Context) layout.Dimensions {
			if len(headers) == 0 {
				return centered(th, gtx, "no headers")
			}
			return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				hdrList.Axis = layout.Vertical
				return material.List(th, hdrList).Layout(gtx, len(headers), func(gtx layout.Context, i int) layout.Dimensions {
					return headerRow(th, gtx, headers[i])
				})
			})
		})
	}

	sliderHandle := func(gtx layout.Context) layout.Dimensions {
		size := image.Point{X: gtx.Constraints.Max.X, Y: sliderH}
		defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
		pointer.CursorRowResize.Add(gtx.Ops)
		s.HdrSplitDrag.Add(gtx.Ops)
		for {
			_, ok := gtx.Event(pointer.Filter{Target: &s.HdrSplitDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
		}
		return layout.Dimensions{Size: size}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(headersRow),
		layout.Rigid(hLine),
		layout.Rigid(headersContent),
		layout.Rigid(sliderHandle),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.bodyHeader(gtx, rowH, "Body", mime, len(body), &s.PrettyBtn, s.Pretty, &s.ReqCopyBtn, len(body) > 0)
		}),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return paneSurface(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.bodyViewer(gtx, s.ReqViewer, &s.ReqViewerKey, &s.BodySearch, &s.ReqScrollDrag, &s.ReqScrollDragY, identity, body, mime, s.Pretty)
			})
		}),
	)
}

func (s *Section) bodyHeader(gtx layout.Context, rowH int, label, mime string, size int, prettyBtn *widget.Clickable, pretty bool, copyBtn *widget.Clickable, enabled bool) layout.Dimensions {
	th := s.host.Theme
	return paneRow(gtx, rowH, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					txt := label
					if mime != "" {
						txt = label + " — " + mime
					}
					if size > 0 {
						txt += "  (" + humanSize(int64(size)) + ")"
					}
					lbl := widgets.MonoLabel(th, unit.Sp(12), txt)
					lbl.Font.Weight = font.Bold
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.toggleBtn(gtx, prettyBtn, "Pretty", pretty, enabled)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return btn(gtx, th, copyBtn, "Copy", widgets.IconDup, theme.Border, th.Fg, enabled)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		)
	})
}

func (s *Section) toggleBtn(gtx layout.Context, clk *widget.Clickable, label string, active, enabled bool) layout.Dimensions {
	th := s.host.Theme
	bg := theme.Border
	fg := th.Fg
	if active {
		bg = theme.BtnPrimary
		fg = theme.BtnPrimaryFg
	}
	return btn(gtx, th, clk, label, nil, bg, fg, enabled)
}

func (s *Section) bodyViewer(gtx layout.Context, viewer *workspace.ResponseViewer, key *string, search *workspace.SearchBox, scrollDrag *gesture.Drag, scrollDragY *float32, identity string, body []byte, mime string, pretty bool) layout.Dimensions {
	th := s.host.Theme
	if len(body) == 0 {
		return centered(th, gtx, "no body")
	}
	if !isProbablyText(body) {
		return centered(th, gtx, "[binary data — "+humanSize(int64(len(body)))+"]")
	}
	k := identity + "|pretty=" + boolStr(pretty)
	if *key != k {
		*key = k
		text := body
		if pretty {
			if p, ok := har.PrettyCode(body, mime); ok {
				text = p
			}
		}
		viewer.SetText(string(text))
		search.Invalidate()
	}
	search.Process(gtx, viewer)
	vs := workspace.ResponseViewerStyle{
		Viewer:           viewer,
		Shaper:           th.Shaper,
		Font:             widgets.MonoFont,
		TextSize:         settings.BodyTextSize,
		Color:            theme.Fg,
		HighlightColor:   theme.WithAlpha(theme.Accent, 150),
		SearchMatchColor: theme.WithAlpha(theme.Accent, 60),
		SelectionColor:   theme.Selection,
		Wrap:             true,
		Padding:          unit.Dp(8),
		Lang:             syntax.Detect(mime, body),
		Syntax:           theme.Syntax,
		BracketCycle:     settings.BracketColorization,
	}
	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions { return vs.Layout(gtx) }),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return s.bodyScrollbar(gtx, viewer, scrollDrag, scrollDragY)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return workspace.SearchOverlay(gtx, th, search)
		}),
	)
}

func (s *Section) bodyScrollbar(gtx layout.Context, viewer *workspace.ResponseViewer, scrollDrag *gesture.Drag, scrollDragY *float32) layout.Dimensions {
	bounds := viewer.GetScrollBounds()
	totalH := float32(bounds.Max.Y)
	viewH := float32(gtx.Constraints.Max.Y)
	if totalH <= viewH || totalH == 0 {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	scrollY := float32(viewer.GetScrollY())
	maxScroll := totalH - viewH
	if maxScroll <= 0 {
		maxScroll = 1
	}
	frac := scrollY / maxScroll
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	thumbH := viewH * (viewH / totalH)
	if thumbH < 20 {
		thumbH = 20
	}
	thumbY := frac * (viewH - thumbH)
	trackW := gtx.Dp(unit.Dp(10))
	thumbW := gtx.Dp(unit.Dp(6))

	trackRect := image.Rect(gtx.Constraints.Max.X-trackW, 0, gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	stack := clip.Rect(trackRect).Push(gtx.Ops)
	for {
		e, ok := scrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			*scrollDragY = e.Position.Y
		case pointer.Drag:
			delta := e.Position.Y - *scrollDragY
			*scrollDragY = e.Position.Y
			if viewH > thumbH {
				scrollY += delta / (viewH - thumbH) * maxScroll
			}
			ny := int(scrollY)
			if ny < 0 {
				ny = 0
			}
			viewer.SetScrollY(ny)
			if s.host.Window != nil {
				s.host.Window.Invalidate()
			}
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	scrollDrag.Add(gtx.Ops)
	stack.Pop()

	rect := image.Rect(
		gtx.Constraints.Max.X-thumbW-gtx.Dp(unit.Dp(2)),
		int(thumbY),
		gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))
	return layout.Dimensions{Size: gtx.Constraints.Max}
}

func headerRow(th *material.Theme, gtx layout.Context, h har.Header) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(170))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			lbl := material.Label(th, unit.Sp(11), h.Name)
			lbl.Color = theme.FgMuted
			lbl.Font.Typeface = widgets.MonoTypeface
			lbl.MaxLines = 1
			lbl.Truncator = "…"
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(11), h.Value)
			lbl.Color = theme.Fg
			lbl.Font.Typeface = widgets.MonoTypeface
			return lbl.Layout(gtx)
		}),
	)
}

func (s *Section) copySelectedFile(gtx layout.Context) {
	if s.SelFile < 0 || s.SelFile >= len(s.Resources) {
		return
	}
	if sel := s.FileViewer.SelectedText(); sel != "" {
		clipboardWrite(gtx, []byte(sel))
		return
	}
	clipboardWrite(gtx, s.Resources[s.SelFile].Body)
}

func (s *Section) copySelectedReqBody(gtx layout.Context) {
	if s.SelReq < 0 || s.SelReq >= len(s.Doc.Entries) {
		return
	}
	if sel := s.ReqViewer.SelectedText(); sel != "" {
		clipboardWrite(gtx, []byte(sel))
		return
	}
	e := &s.Doc.Entries[s.SelReq]
	var body []byte
	if s.InspTab == 1 {
		if e.IsWebSocket() {
			body = wsText(e, s.Pretty)
		} else {
			body = respBody(e)
		}
	} else {
		body = []byte(e.Request.PostData.Text)
	}
	clipboardWrite(gtx, body)
}

func clipboardWrite(gtx layout.Context, body []byte) {
	gtx.Execute(clipboard.WriteCmd{
		Type: "application/text",
		Data: io.NopCloser(strings.NewReader(string(body))),
	})
}

func (s *Section) runSelected() {
	if s.SelReq < 0 || s.SelReq >= len(s.Doc.Entries) {
		return
	}
	if s.host.RunEntry != nil {
		s.host.RunEntry(&s.Doc.Entries[s.SelReq])
	}
}

func wsText(e *har.Entry, pretty bool) []byte {
	if len(e.WebSocketMessages) == 0 {
		return []byte("No WebSocket frames captured in this archive.")
	}
	var b strings.Builder
	for i, m := range e.WebSocketMessages {
		dir := "← receive"
		if m.Sent() {
			dir = "→ send"
		}
		kind := "text"
		if m.Binary() {
			kind = "binary"
		}
		b.WriteString(dir)
		b.WriteString("  [")
		b.WriteString(kind)
		b.WriteString("]\n")
		if m.Binary() {
			b.WriteString("[binary frame, " + strconv.Itoa(len(m.Data)) + " base64 chars]\n")
		} else {
			data := m.Data
			if pretty {
				if p, ok := har.Pretty([]byte(data), ""); ok {
					data = string(p)
				}
			}
			b.WriteString(data)
			b.WriteString("\n")
		}
		if i != len(e.WebSocketMessages)-1 {
			b.WriteString("\n")
		}
	}
	return []byte(b.String())
}

func displayMethod(e *har.Entry) string {
	if e.IsWebSocket() {
		return "WS"
	}
	return e.Request.Method
}

func respBody(e *har.Entry) []byte {
	body, _, err := e.DecodeBody()
	if err != nil {
		return []byte(e.Response.Content.Text)
	}
	return body
}

func statusText(e *har.Entry) string {
	if e.Response.Status <= 0 {
		return "(no response)"
	}
	s := strconv.Itoa(e.Response.Status)
	if e.Response.StatusText != "" {
		s += " " + e.Response.StatusText
	}
	return s
}
