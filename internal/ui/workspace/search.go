package workspace

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type matchSpan struct {
	start int
	end   int
}

type searchableEditor interface {
	Text() string
	SelectedText() string
	SetCaret(start, end int)
	SetSearchSpans(spans []matchSpan)
	SetRevealInset(px int)
	RevealScreenY() (int, bool)
	ClearSearchCaret()
}

type SearchableEditor = searchableEditor

type SearchBox struct {
	Open          bool
	CaseSensitive bool

	Editor   widget.Editor
	PrevBtn  widget.Clickable
	NextBtn  widget.Clickable
	CloseBtn widget.Clickable
	CaseBtn  widget.Clickable

	query      string
	spans      []matchSpan
	current    int
	cache      string
	cacheDirty bool
	wantFocus  bool
	panelH     int
}

func (s *SearchBox) invalidate() { s.cacheDirty = true }

func (s *SearchBox) Invalidate() { s.invalidate() }

// Close dismisses the panel and clears ed's match highlights. Hosts that stop
// drawing a viewer must call it, otherwise the box stays open on a pane that
// can neither show it nor take the Escape that would dismiss it.
func (s *SearchBox) Close(ed SearchableEditor) {
	if !s.Open {
		return
	}
	s.closeOn(ed)
}

// foldForSearch lowercases s without ever changing its byte length, so match
// offsets found in the folded copy address the original text unchanged. Runes
// whose lowercase form has a different UTF-8 size (İ, K, …) are left as-is.
func foldForSearch(s string) string {
	asciiOnly := true
	hasUpper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			asciiOnly = false
			break
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}
	if asciiOnly {
		if !hasUpper {
			return s
		}
		b := make([]byte, len(s))
		for i := 0; i < len(s); i++ {
			c := s[i]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			b[i] = c
		}
		return string(b)
	}
	b := make([]byte, 0, len(s))
	var enc [utf8.UTFMax]byte
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if lr := unicode.ToLower(r); lr != r && utf8.RuneLen(lr) == sz {
			n := utf8.EncodeRune(enc[:], lr)
			b = append(b, enc[:n]...)
		} else {
			b = append(b, s[i:i+sz]...)
		}
		i += sz
	}
	return string(b)
}

func (s *SearchBox) recompute(text string) {
	if s.cacheDirty || len(s.cache) != len(text) {
		s.cache = foldForSearch(text)
		s.cacheDirty = false
	}
	s.spans = s.spans[:0]
	q := s.Editor.Text()
	s.query = q
	if q == "" {
		s.current = -1
		return
	}
	var hay, needle string
	if s.CaseSensitive {
		hay = text
		needle = q
	} else {
		hay = s.cache
		needle = foldForSearch(q)
	}
	nLen := len(needle)
	if nLen == 0 {
		s.current = -1
		return
	}
	off := 0
	for off <= len(hay)-nLen {
		idx := strings.Index(hay[off:], needle)
		if idx < 0 {
			break
		}
		pos := off + idx
		s.spans = append(s.spans, matchSpan{start: pos, end: pos + nLen})
		off = pos + nLen
	}
	s.clampCurrent()
}

func (s *SearchBox) clampCurrent() {
	if len(s.spans) == 0 {
		s.current = -1
		return
	}
	if s.current < 0 {
		s.current = 0
	}
	if s.current >= len(s.spans) {
		s.current = len(s.spans) - 1
	}
}

func (s *SearchBox) apply(ed searchableEditor) {
	ed.SetSearchSpans(s.spans)
	if s.current < 0 || s.current >= len(s.spans) {
		// typing past the last match that existed must not leave that match
		// highlighted next to a 0/0 counter
		ed.ClearSearchCaret()
		return
	}
	m := s.spans[s.current]
	ed.SetRevealInset(s.panelH)
	ed.SetCaret(m.start, m.end)
}

func (s *SearchBox) refresh(ed searchableEditor, text string, resetToFirst bool) {
	s.recompute(text)
	if resetToFirst {
		if len(s.spans) > 0 {
			s.current = 0
		} else {
			s.current = -1
		}
	}
	s.apply(ed)
}

func (s *SearchBox) navigate(dir int, ed searchableEditor) {
	if len(s.spans) == 0 {
		return
	}
	s.current += dir
	if s.current >= len(s.spans) {
		s.current = 0
	}
	if s.current < 0 {
		s.current = len(s.spans) - 1
	}
	s.apply(ed)
}

func (s *SearchBox) closeOn(ed searchableEditor) {
	s.Open = false
	s.spans = s.spans[:0]
	s.current = -1
	s.wantFocus = false
	ed.SetSearchSpans(nil)
	ed.SetRevealInset(0)
	ed.ClearSearchCaret()
}

func (t *RequestTab) HandleSearchShortcut(gtx layout.Context) {
	if gtx.Focused(&t.ReqEditor) || gtx.Focused(&t.ReqSearch.Editor) {
		t.toggleSearch(gtx, &t.ReqSearch, &t.ReqEditor)
		return
	}
	t.toggleSearch(gtx, &t.RespSearch, t.RespEditor)
}

func (t *RequestTab) toggleSearch(gtx layout.Context, box *SearchBox, ed searchableEditor) {
	box.Toggle(gtx, ed)
}

func (s *SearchBox) Toggle(gtx layout.Context, ed SearchableEditor) {
	box := s
	if box.Open && gtx.Focused(&box.Editor) {
		box.closeOn(ed)
		return
	}
	seeded := false
	if sel := ed.SelectedText(); sel != "" && !strings.ContainsRune(sel, '\n') {
		box.Editor.SetText(sel)
		seeded = true
	}
	box.Open = true
	box.wantFocus = true
	box.refresh(ed, ed.Text(), seeded)
}

func (t *RequestTab) updateSearch(gtx layout.Context, box *SearchBox, ed searchableEditor) {
	box.Process(gtx, ed)
}

func (s *SearchBox) Process(gtx layout.Context, ed SearchableEditor) {
	box := s
	if !box.Open {
		return
	}

	for box.CaseBtn.Clicked(gtx) {
		box.CaseSensitive = !box.CaseSensitive
		box.refresh(ed, ed.Text(), false)
	}
	for box.NextBtn.Clicked(gtx) {
		box.navigate(1, ed)
	}
	for box.PrevBtn.Clicked(gtx) {
		box.navigate(-1, ed)
	}
	for box.CloseBtn.Clicked(gtx) {
		box.closeOn(ed)
		return
	}

	queryChanged := false
	for {
		ev, ok := box.Editor.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			box.navigate(1, ed)
		case widget.ChangeEvent:
			queryChanged = true
		}
	}
	if box.Editor.Text() != box.query {
		queryChanged = true
	}

	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &box.Editor, Name: key.NameEscape},
			key.Filter{Focus: &box.Editor, Name: key.NameReturn, Required: key.ModShift},
			key.Filter{Focus: &box.Editor, Name: key.NameEnter, Required: key.ModShift},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		switch ke.Name {
		case key.NameEscape:
			box.closeOn(ed)
			return
		case key.NameReturn, key.NameEnter:
			box.navigate(-1, ed)
		}
	}

	if queryChanged {
		box.refresh(ed, ed.Text(), true)
	} else if box.cacheDirty {
		box.refresh(ed, ed.Text(), false)
	}
}

func (t *RequestTab) layoutSearchOverlay(gtx layout.Context, th *material.Theme, box *SearchBox) layout.Dimensions {
	return SearchOverlay(gtx, th, box)
}

// SearchOverlay draws the panel pinned to the top-right of the pane. The corner
// is fixed on purpose: a panel that relocates itself is more disorienting than
// the match it occasionally covers, which the reveal inset already avoids
// wherever the document has room to scroll.
func SearchOverlay(gtx layout.Context, th *material.Theme, box *SearchBox) layout.Dimensions {
	if !box.Open {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min = gtx.Constraints.Max
	return layout.NE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: unit.Dp(6), Right: unit.Dp(7)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			dims := searchPanelBackground(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(5), Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							w := gtx.Dp(unit.Dp(190))
							h := gtx.Dp(unit.Dp(24))
							gtx.Constraints.Min = image.Pt(w, h)
							gtx.Constraints.Max.X = w
							gtx.Constraints.Max.Y = h
							dims := widgets.TextField(gtx, th, &box.Editor, "Find", true, nil, 0, unit.Sp(12))
							if box.wantFocus {
								box.wantFocus = false
								gtx.Execute(key.FocusCmd{Tag: &box.Editor})
								if n := box.Editor.Len(); n > 0 {
									box.Editor.SetCaret(0, n)
								}
							}
							return dims
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							cur := 0
							if len(box.spans) > 0 && box.current >= 0 {
								cur = box.current + 1
							}
							txt := strconv.Itoa(cur) + "/" + strconv.Itoa(len(box.spans))
							gtx.Constraints.Min.X = gtx.Dp(unit.Dp(46))
							lbl := widgets.MonoLabel(th, unit.Sp(11), txt)
							lbl.Color = theme.FgDim
							lbl.Alignment = text.End
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return searchCaseButton(gtx, th, &box.CaseBtn, box.CaseSensitive)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, &box.PrevBtn, widgets.IconExpandLess, th, 26, 18)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, &box.NextBtn, widgets.IconExpandMore, th, 26, 18)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, &box.CloseBtn, widgets.IconClose, th, 26, 16)
						}),
					)
				})
			})
			box.panelH = dims.Size.Y + gtx.Dp(unit.Dp(14))
			return dims
		})
	})
}

func searchPanelBackground(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()

	radius := gtx.Dp(unit.Dp(4))
	const layers = 5
	for i := layers; i >= 1; i-- {
		spread := gtx.Dp(unit.Dp(float32(i)))
		drop := gtx.Dp(unit.Dp(float32(i) * 0.6))
		rect := image.Rectangle{
			Min: image.Pt(-spread, -spread+drop),
			Max: image.Pt(dims.Size.X+spread, dims.Size.Y+spread+drop),
		}
		alpha := uint8(14 + (layers-i)*8)
		rr := clip.UniformRRect(rect, radius+spread)
		paint.FillShape(gtx.Ops, color.NRGBA{A: alpha}, rr.Op(gtx.Ops))
	}

	rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, radius)
	paint.FillShape(gtx.Ops, theme.Bg, rr.Op(gtx.Ops))
	widgets.PaintBorder1px(gtx, dims.Size, theme.Border)

	call.Add(gtx.Ops)
	return dims
}

func searchCaseButton(gtx layout.Context, th *material.Theme, clk *widget.Clickable, active bool) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(unit.Dp(26))
		gtx.Constraints.Min = image.Pt(size, size)
		gtx.Constraints.Max = gtx.Constraints.Min
		rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(3)))
		if active {
			paint.FillShape(gtx.Ops, theme.BtnPrimary, rr.Op(gtx.Ops))
		} else if clk.Hovered() {
			paint.FillShape(gtx.Ops, theme.BgHover, rr.Op(gtx.Ops))
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, unit.Sp(11), "Aa")
			lbl.Color = theme.FgDim
			if active {
				lbl.Color = theme.BtnPrimaryFg
			}
			return lbl.Layout(gtx)
		})
	})
}
