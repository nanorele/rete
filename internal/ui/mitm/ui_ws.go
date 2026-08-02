package mitm

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"unicode/utf8"

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

func (s *UIState) wsView(gtx layout.Context) layout.Dimensions {
	s.WSList.Axis = layout.Vertical
	msgs := s.filteredWS()

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.wsHeader(gtx) }),
		layout.Rigid(hLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(msgs) == 0 {
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(12), "No WebSocket messages captured (requires HTTPS decryption)")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				})
			}
			for len(s.WSRowClk) < len(msgs) {
				s.WSRowClk = append(s.WSRowClk, &widget.Clickable{})
			}
			// Row events are polled in viewRowEvents (outside the List).
			return material.List(s.host.Theme, &s.WSList).Layout(gtx, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
				return s.wsRow(gtx, msgs[i], s.WSRowClk[i], s.WSSelected == msgs[i].ID)
			})
		}),
		layout.Rigid(hLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.wsDetail(gtx) }),
	)
}

func (s *UIState) wsHeader(gtx layout.Context) layout.Dimensions {
	hH := gtx.Dp(unit.Dp(24))
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, hH)}.Op())
	gtx.Constraints.Min.Y = hH
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		hdr := func(txt string, w unit.Dp, al text.Alignment) layout.FlexChild {
			cell := func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(10), txt)
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				lbl.Alignment = al
				return lbl.Layout(gtx)
			}
			if w == 0 {
				return layout.Flexed(1, cell)
			}
			return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				px := gtx.Dp(w)
				gtx.Constraints.Min.X = px
				gtx.Constraints.Max.X = px
				return cell(gtx)
			})
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			hdr("Dir", unit.Dp(60), text.Start),
			hdr("Type", unit.Dp(60), text.Start),
			hdr("URL / message", 0, text.Start),
			hdr("Len", unit.Dp(60), text.End),
			hdr("Time", unit.Dp(80), text.End),
		)
	})
}

func (s *UIState) wsRow(gtx layout.Context, m *WSMessage, clk *widget.Clickable, selected bool) layout.Dimensions {
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
		pointer.CursorPointer.Add(gtx.Ops)
		dir := "◀ s→c"
		dcol := theme.MethodGet
		if m.ToServer {
			dir = "▶ c→s"
			dcol = theme.Accent
		}
		return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(widgets.TableHInset), Right: unit.Dp(widgets.TableHInset)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			cell := func(txt string, w unit.Dp, al text.Alignment, col interface{}) layout.FlexChild {
				c := theme.FgMuted
				if cc, ok := col.(bool); ok && cc {
					c = dcol
				}
				w2 := func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), txt)
					lbl.Color = c
					lbl.Alignment = al
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					lbl.Font.Typeface = widgets.MonoTypeface
					return lbl.Layout(gtx)
				}
				if w == 0 {
					return layout.Flexed(1, w2)
				}
				return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					px := gtx.Dp(w)
					gtx.Constraints.Min.X = px
					gtx.Constraints.Max.X = px
					return w2(gtx)
				})
			}
			preview := wsPreview(m.Payload)
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				cell(dir, unit.Dp(60), text.Start, true),
				cell(WSOpcodeName(m.Opcode), unit.Dp(60), text.Start, false),
				cell(preview, 0, text.Start, false),
				cell(strconv.Itoa(len(m.Payload)), unit.Dp(60), text.End, false),
				cell(m.Time.Format("15:04:05.000"), unit.Dp(80), text.End, false),
			)
		})
	})
}

func (s *UIState) wsDetail(gtx layout.Context) layout.Dimensions {
	m := s.Proxy.WS.FindByID(s.WSSelected)
	gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(140))
	gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(140))
	return bgBar(gtx, theme.BgDark, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if m == nil {
				return emptyLabel(s.host.Theme, gtx, "select a message")
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(10), fmt.Sprintf("%s · %s · %d bytes · %s", WSOpcodeName(m.Opcode), dirName(m.ToServer), len(m.Payload), m.URL))
					lbl.Color = theme.FgMuted
					lbl.MaxLines = 1
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lines := s.paneLines(
						paneTextKey{id: m.ID, rev: s.Proxy.WS.Rev(), kind: paneWS},
						func() string { return string(m.Payload) },
					)
					return s.scrollLines(gtx, &s.BodyList, lines)
				}),
			)
		})
	})
}

func dirName(toServer bool) string {
	if toServer {
		return "client → server"
	}
	return "server → client"
}

func (s *UIState) filteredWS() []*WSMessage {
	q := strings.TrimSpace(strings.ToLower(s.Filter.Text()))
	rev := s.Proxy.WS.Rev()
	if s.wsFilterValid && s.wsFilterRev == rev && s.wsFilterQuery == q {
		return s.wsFilterBuf
	}
	all := s.Proxy.WS.Snapshot()
	if q == "" {
		s.wsFilterBuf = all
	} else {
		out := s.wsFilterBuf[:0]
		for _, m := range all {
			if wsMatches(m, q) {
				out = append(out, m)
			}
		}
		s.wsFilterBuf = out
	}
	s.wsFilterRev = rev
	s.wsFilterQuery = q
	s.wsFilterValid = true
	return s.wsFilterBuf
}

const wsPreviewRunes = 200

// wsPreview renders the row preview from the head of the payload only. The row
// truncates at 200 characters, so converting a multi-megabyte frame to a
// string first cost a full copy per visible row per frame.
func wsPreview(payload []byte) string {
	b := payload
	if len(b) > wsPreviewRunes*utf8.UTFMax {
		b = b[:wsPreviewRunes*utf8.UTFMax]
	}
	out := make([]rune, 0, wsPreviewRunes)
	for _, r := range string(b) {
		if len(out) == wsPreviewRunes {
			break
		}
		if r == '\n' {
			r = ' '
		}
		out = append(out, r)
	}
	return string(out)
}

// wsMatches searches the same "url payload opcode" haystack the filter has
// always used, but walks it in place. Materialising it cost one copy of every
// captured payload per frame.
func wsMatches(m *WSMessage, q string) bool {
	if !isASCII(q) {
		hay := strings.ToLower(m.URL + " " + string(m.Payload) + " " + WSOpcodeName(m.Opcode))
		return strings.Contains(hay, q)
	}
	h := wsHay{url: m.URL, payload: m.Payload, opcode: WSOpcodeName(m.Opcode)}
	n := h.len()
	for i := 0; i+len(q) <= n; i++ {
		j := 0
		for ; j < len(q); j++ {
			if lowerASCII(h.at(i+j)) != q[j] {
				break
			}
		}
		if j == len(q) {
			return true
		}
	}
	return false
}

type wsHay struct {
	url     string
	payload []byte
	opcode  string
}

func (h wsHay) len() int { return len(h.url) + 1 + len(h.payload) + 1 + len(h.opcode) }

func (h wsHay) at(i int) byte {
	if i < len(h.url) {
		return h.url[i]
	}
	i -= len(h.url)
	if i == 0 {
		return ' '
	}
	i--
	if i < len(h.payload) {
		return h.payload[i]
	}
	i -= len(h.payload)
	if i == 0 {
		return ' '
	}
	return h.opcode[i-1]
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}
