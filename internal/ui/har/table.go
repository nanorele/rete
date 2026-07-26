package har

import (
	"image"
	"image/color"
	"net/url"
	"strconv"
	"strings"

	"tracto/internal/har"
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

func (s *Section) requestsView(gtx layout.Context) layout.Dimensions {
	leftW, handleW, rightW := s.split(gtx)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = leftW
			gtx.Constraints.Max.X = leftW
			d := s.requestTable(gtx)
			d.Size.X = leftW
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.splitHandle(gtx, handleW) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = rightW
			gtx.Constraints.Max.X = rightW
			return s.inspector(gtx)
		}),
	)
}

func (s *Section) split(gtx layout.Context) (leftW, handleW, rightW int) {
	totalW := gtx.Constraints.Max.X
	handleW = gtx.Dp(unit.Dp(6))

	clampLeft := func(w int) int {
		if w < 240 {
			w = 240
		}
		if w > totalW-handleW-280 {
			w = totalW - handleW - 280
		}
		if w < 0 {
			w = 0
		}
		return w
	}
	leftFromRatio := func() int {
		return clampLeft(int(float32(totalW)*s.SplitRatio) - handleW/2)
	}

	var moved bool
	var finalX float32
	for {
		e, ok := s.SplitDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		pos := e.Position.X + float32(s.leftDrawn)
		switch e.Kind {
		case pointer.Press:
			s.SplitDragX = pos
			s.splitPx = float32(leftFromRatio())
		case pointer.Drag:
			finalX = pos
			moved = true
		}
	}
	if moved && totalW > 0 {
		s.splitPx += finalX - s.SplitDragX
		s.SplitDragX = finalX
		left := clampLeft(int(s.splitPx + 0.5))
		s.SplitRatio = (float32(left) + float32(handleW)/2) / float32(totalW)
		s.host.Window.Invalidate()
	}

	if handleW > totalW {
		handleW = totalW
	}
	leftW = leftFromRatio()
	s.leftDrawn = leftW
	rightW = totalW - leftW - handleW
	if rightW < 0 {
		rightW = 0
	}
	return leftW, handleW, rightW
}

func (s *Section) splitHandle(gtx layout.Context, handleW int) layout.Dimensions {
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
}

func tableColumns() []widgets.TableColumn {
	return []widgets.TableColumn{
		{Title: "#", Width: unit.Dp(32), Align: text.Start},
		{Title: "Method", Width: unit.Dp(56), Align: text.Start},
		{Title: "Status", Width: unit.Dp(48), Align: text.Start},
		{Title: "Domain", Width: unit.Dp(150), Min: unit.Dp(60), Align: text.Start},
		{Title: "File", Width: 0, Align: text.Start},
		{Title: "Type", Width: unit.Dp(90), Min: unit.Dp(48), Align: text.Start},
		{Title: "Size", Width: unit.Dp(64), Align: text.End},
	}
}

func (s *Section) requestTable(gtx layout.Context) layout.Dimensions {
	th := s.host.Theme
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	entries := s.Doc.Entries
	vis := s.visibleIndices()
	if len(vis) == 0 {
		msg := "No requests in this archive"
		if s.SelPageID != "" {
			msg = "No requests for this page"
		}
		return centered(th, gtx, msg)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.Table.Header(gtx, th) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			for len(s.ReqRows) < len(entries) {
				s.ReqRows = append(s.ReqRows, &widget.Clickable{})
			}
			if len(s.rowCache) != len(entries) {
				s.rowCache = buildRowCache(entries)
			}
			return material.List(th, &s.ReqList).Layout(gtx, len(vis), func(gtx layout.Context, row int) layout.Dimensions {
				i := vis[row]
				clk := s.ReqRows[i]
				for clk.Clicked(gtx) {
					s.SelReq = i
				}
				return requestRow(gtx, th, s.Table, &s.rowCache[i], &entries[i], clk, s.SelReq == i)
			})
		}),
	)
}

type rowDisplay struct {
	index, domain, file, typ, size string
}

func buildRowCache(entries []har.Entry) []rowDisplay {
	out := make([]rowDisplay, len(entries))
	for i := range entries {
		e := &entries[i]
		domain, file := SplitURL(e.Request.URL)
		out[i] = rowDisplay{
			index:  strconv.Itoa(i + 1),
			domain: domain,
			file:   file,
			typ:    shortType(e.ContentType()),
			size:   humanSize(entrySize(e)),
		}
	}
	return out
}

func requestRow(gtx layout.Context, th *material.Theme, t *widgets.Table, d *rowDisplay, e *har.Entry, clk *widget.Clickable, selected bool) layout.Dimensions {
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
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return t.Row(gtx, func(i int) layout.Widget {
				switch i {
				case 0:
					return textCell(th, d.index, text.Start, theme.FgMuted, true)
				case 1:
					return methodCellW(th, e.Request.Method)
				case 2:
					return statusCellW(th, e.Response.Status)
				case 3:
					return textCell(th, d.domain, text.Start, theme.FgMuted, true)
				case 4:
					return textCell(th, d.file, text.Start, th.Fg, true)
				case 5:
					return textCell(th, d.typ, text.Start, theme.FgMuted, false)
				default:
					return textCell(th, d.size, text.End, theme.FgMuted, true)
				}
			})
		})
	})
}

func textCell(th *material.Theme, s string, al text.Alignment, col color.NRGBA, mono bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Truncator = "…"
		lbl.Color = col
		if mono {
			lbl.Font.Typeface = widgets.MonoTypeface
		}
		return lbl.Layout(gtx)
	}
}

func methodCellW(th *material.Theme, method string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), method)
		lbl.Color = theme.MethodColor(method)
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	}
}

func statusCellW(th *material.Theme, code int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		s := "—"
		if code > 0 {
			s = strconv.Itoa(code)
		}
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = statusColor(code)
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	}
}

func statusColor(code int) color.NRGBA {
	switch {
	case code >= 500:
		return theme.Danger
	case code >= 400:
		return theme.VarMissing
	case code >= 300:
		return theme.Accent
	case code >= 200:
		return theme.VarFound
	default:
		return theme.FgMuted
	}
}

func SplitURL(rawURL string) (domain, file string) {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return "", rawURL
	}
	domain = u.Host
	file = u.Path
	if u.RawQuery != "" {
		file += "?" + u.RawQuery
	}
	if file == "" || file == "/" {
		file = "/"
	}
	return domain, file
}

func shortType(mime string) string {
	if mime == "" {
		return ""
	}
	if i := strings.IndexByte(mime, '/'); i >= 0 {
		return mime[i+1:]
	}
	return mime
}

func entrySize(e *har.Entry) int64 {
	if e.Response.Content.Size > 0 {
		return e.Response.Content.Size
	}
	if e.Response.BodySize > 0 {
		return e.Response.BodySize
	}
	return int64(len(e.Response.Content.Text))
}
