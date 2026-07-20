package mitm

import (
	"fmt"
	"image"
	"image/color"
	"sort"
	"strconv"
	"strings"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

var histCols = []string{"#", "Method", "Host / Path", "Src", "Status", "Size", "Time"}

func histColWidth(i int) unit.Dp {
	switch i {
	case 0:
		return unit.Dp(40)
	case 1:
		return unit.Dp(66)
	case 2:
		return 0 // flex
	case 3:
		return unit.Dp(54)
	case 4:
		return unit.Dp(56)
	case 5:
		return unit.Dp(60)
	default:
		return unit.Dp(60)
	}
}

func (s *UIState) historyView(gtx layout.Context) layout.Dimensions {
	s.List.Axis = layout.Vertical

	flows := s.filteredFlows()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.historyHeader(gtx) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(flows) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					msg := "No traffic captured yet"
					if s.Store.Len() > 0 {
						msg = "No flows match the filter"
					}
					lbl := material.Label(s.host.Theme, unit.Sp(12), msg)
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				})
			}
			for len(s.RowClicks) < len(flows) {
				s.RowClicks = append(s.RowClicks, &widget.Clickable{})
			}
			for len(s.RowMore) < len(flows) {
				s.RowMore = append(s.RowMore, &widget.Clickable{})
			}
			// Row events are polled in viewRowEvents (outside the List).
			return material.List(s.host.Theme, &s.List).Layout(gtx, len(flows), func(gtx layout.Context, i int) layout.Dimensions {
				return s.flowRow(gtx, flows[i], s.RowClicks[i], s.RowMore[i], s.Selected == flows[i].ID)
			})
		}),
	)
}

// historyHeader is a sortable column header row.
func (s *UIState) historyHeader(gtx layout.Context) layout.Dimensions {
	hH := gtx.Dp(unit.Dp(24))
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, hH)}.Op())
	gtx.Constraints.Min.Y = hH
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(histCols)+1)
		for i := range histCols {
			i := i
			title := histCols[i]
			if s.SortColumn == title {
				if s.SortAsc {
					title += " ▲"
				} else {
					title += " ▼"
				}
			}
			cell := func(gtx layout.Context) layout.Dimensions {
				return s.SortClicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					al := text.Start
					if i >= 4 {
						al = text.End
					}
					lbl := material.Label(s.host.Theme, unit.Sp(10), title)
					lbl.Color = theme.FgMuted
					lbl.Font.Weight = font.Bold
					lbl.Alignment = al
					lbl.MaxLines = 1
					pointer.CursorPointer.Add(gtx.Ops)
					return lbl.Layout(gtx)
				})
			}
			w := histColWidth(i)
			if w == 0 {
				children = append(children, layout.Flexed(1, cell))
			} else {
				px := gtx.Dp(w)
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = px
					gtx.Constraints.Max.X = px
					return cell(gtx)
				}))
			}
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
	})
}

func (s *UIState) flowRow(gtx layout.Context, f *Flow, clk, more *widget.Clickable, selected bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		rowH := gtx.Dp(unit.Dp(22))
		gtx.Constraints.Min.Y = rowH
		bg := theme.Bg
		if hl := highlightColor(f.Highlight); hl != (color.NRGBA{}) {
			bg = hl
		}
		if selected {
			bg = theme.AccentDim
		} else if clk.Hovered() {
			bg = theme.BgHover
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, rowH)}.Op())
		pointer.CursorPointer.Add(gtx.Ops)
		hovered := clk.Hovered()
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cells := []func(gtx layout.Context) layout.Dimensions{
				func(gtx layout.Context) layout.Dimensions {
					return textCell(s.host.Theme, fmt.Sprintf("%d", f.ID), text.Start, theme.FgMuted, false)(gtx)
				},
				methodCell(s.host.Theme, f.Method),
				hostCell(s.host.Theme, f),
				srcCell(s.host.Theme, f),
				statusCell(s.host.Theme, f),
				func(gtx layout.Context) layout.Dimensions {
					return textCell(s.host.Theme, humanSize(f.RespSize), text.End, theme.FgMuted, true)(gtx)
				},
				func(gtx layout.Context) layout.Dimensions {
					return textCell(s.host.Theme, humanDuration(f), text.End, theme.FgMuted, true)(gtx)
				},
			}
			children := make([]layout.FlexChild, 0, len(cells)+1)
			for i := range cells {
				c := cells[i]
				w := histColWidth(i)
				if w == 0 {
					children = append(children, layout.Flexed(1, c))
				} else {
					px := gtx.Dp(w)
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = px
						gtx.Constraints.Max.X = px
						return c(gtx)
					}))
				}
				children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return more.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(16))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					col := theme.Transparent
					if hovered || more.Hovered() {
						col = theme.FgMuted
					}
					pointer.CursorPointer.Add(gtx.Ops)
					return widgets.IconMore.Layout(gtx, col)
				})
			}))
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func textCell(th *material.Theme, s string, al text.Alignment, col color.NRGBA, mono bool) func(layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Color = col
		if mono {
			lbl.Font.Typeface = widgets.MonoTypeface
		}
		return lbl.Layout(gtx)
	}
}

func methodCell(th *material.Theme, method string) func(layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), method)
		lbl.Color = theme.MethodColor(method)
		lbl.Font.Weight = font.Bold
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	}
}

func hostCell(th *material.Theme, f *Flow) func(layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
		host := f.Host
		if f.Path != "" {
			host = f.Host + f.Path
		} else if f.Port != "" && f.Port != "80" && f.Port != "443" {
			host = f.Host + ":" + f.Port
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), host)
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				lbl.Font.Typeface = widgets.MonoTypeface
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if f.Comment == "" {
					return layout.Dimensions{}
				}
				lbl := material.Label(th, unit.Sp(10), "  💬 "+f.Comment)
				lbl.Color = theme.FgMuted
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}),
		)
	}
}

func srcCell(th *material.Theme, f *Flow) func(layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
		src := f.Src
		if src == "" {
			src = SrcForward
		}
		col := theme.FgMuted
		if src == SrcReverse {
			col = theme.Accent
		}
		lbl := material.Label(th, unit.Sp(10), src)
		lbl.Color = col
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	}
}

func statusCell(th *material.Theme, f *Flow) func(layout.Context) layout.Dimensions {
	return func(gtx layout.Context) layout.Dimensions {
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
				col = theme.MethodPost
			case f.StatusCode >= 300:
				col = theme.Accent
			case f.StatusCode >= 200:
				col = theme.MethodGet
			}
		}
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = col
		lbl.Alignment = text.End
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	}
}

func (s *UIState) filteredFlows() []*Flow {
	flows := s.Store.SnapshotMeta()
	q := strings.TrimSpace(strings.ToLower(s.Filter.Text()))
	hideNoise := s.HideNoiseSw.Value

	var srcFilter, statusFilter, mimeFilter string
	var textTokens []string
	if q != "" {
		for _, tok := range strings.Fields(q) {
			switch {
			case strings.HasPrefix(tok, "src:"):
				srcFilter = strings.TrimPrefix(tok, "src:")
			case strings.HasPrefix(tok, "status:"):
				statusFilter = strings.TrimPrefix(tok, "status:")
			case strings.HasPrefix(tok, "mime:"):
				mimeFilter = strings.TrimPrefix(tok, "mime:")
			default:
				textTokens = append(textTokens, tok)
			}
		}
	}

	out := flows[:0]
	for _, f := range flows {
		if hideNoise && isNoise(f) {
			continue
		}
		if srcFilter != "" {
			src := f.Src
			if src == "" {
				src = SrcForward
			}
			if !strings.Contains(src, srcFilter) {
				continue
			}
		}
		if statusFilter != "" && !strings.Contains(strconv.Itoa(f.StatusCode), statusFilter) {
			continue
		}
		if mimeFilter != "" {
			// mime not stored in meta snapshot; approximate via path extension
			if !strings.Contains(strings.ToLower(f.Path), mimeFilter) {
				continue
			}
		}
		if len(textTokens) > 0 {
			hay := strings.ToLower(f.Method + " " + f.Host + f.Path + " " + f.URL + " " + f.Comment)
			ok := true
			for _, t := range textTokens {
				if !strings.Contains(hay, t) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		out = append(out, f)
	}
	sortFlows(out, s.SortColumn, s.SortAsc)
	return out
}

func isNoise(f *Flow) bool {
	p := strings.ToLower(f.Path)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico", ".css", ".js", ".woff", ".woff2", ".ttf"} {
		if strings.Contains(p, ext) {
			return true
		}
	}
	return false
}

func sortFlows(flows []*Flow, col string, asc bool) {
	less := func(a, b *Flow) bool {
		switch col {
		case "Method":
			return a.Method < b.Method
		case "Host / Path":
			return a.Host+a.Path < b.Host+b.Path
		case "Src":
			return a.Src < b.Src
		case "Status":
			return a.StatusCode < b.StatusCode
		case "Size":
			return a.RespSize < b.RespSize
		case "Time":
			return a.Ended.Sub(a.Started) < b.Ended.Sub(b.Started)
		default:
			return a.ID < b.ID
		}
	}
	sort.SliceStable(flows, func(i, j int) bool {
		if asc {
			return less(flows[i], flows[j])
		}
		return less(flows[j], flows[i])
	})
}

// ---------------------------------------------------------------------------
// annotations / export helpers
// ---------------------------------------------------------------------------

func annotateColorKeys() []string {
	return []string{"", "red", "orange", "yellow", "green", "blue"}
}

func highlightColor(key string) color.NRGBA {
	switch key {
	case "red":
		return color.NRGBA{R: 120, G: 40, B: 40, A: 255}
	case "orange":
		return color.NRGBA{R: 120, G: 80, B: 30, A: 255}
	case "yellow":
		return color.NRGBA{R: 110, G: 110, B: 30, A: 255}
	case "green":
		return color.NRGBA{R: 40, G: 100, B: 50, A: 255}
	case "blue":
		return color.NRGBA{R: 40, G: 70, B: 120, A: 255}
	}
	return color.NRGBA{}
}

func flowAsText(f *Flow, resp bool) string {
	var b strings.Builder
	if resp {
		fmt.Fprintf(&b, "%s\n", f.Status)
		for _, h := range f.RespHeaders {
			fmt.Fprintf(&b, "%s: %s\n", h[0], h[1])
		}
		b.WriteString("\n")
		b.Write(f.RespBody)
		return b.String()
	}
	fmt.Fprintf(&b, "%s %s %s\n", f.Method, f.Path, f.Version)
	for _, h := range f.ReqHeaders {
		fmt.Fprintf(&b, "%s: %s\n", h[0], h[1])
	}
	b.WriteString("\n")
	b.Write(f.ReqBody)
	return b.String()
}

func asCurl(f *Flow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "curl -X %s '%s'", f.Method, f.URL)
	for _, h := range f.ReqHeaders {
		if strings.EqualFold(h[0], "host") {
			continue
		}
		fmt.Fprintf(&b, " \\\n  -H '%s: %s'", h[0], h[1])
	}
	if len(f.ReqBody) > 0 {
		fmt.Fprintf(&b, " \\\n  --data-binary '%s'", string(f.ReqBody))
	}
	return b.String()
}
