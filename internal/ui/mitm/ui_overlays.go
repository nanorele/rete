package mitm

import (
	"fmt"
	"image"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

// overlayCatcher closes overlays on a click outside them.
func (s *UIState) overlayCatcher(gtx layout.Context, onClose func()) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &s.OverlayCatch, Kinds: pointer.Press})
		if !ok {
			break
		}
		if pe, ok := ev.(pointer.Event); ok && pe.Kind == pointer.Press {
			onClose()
			s.host.Window.Invalidate()
		}
	}
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, &s.OverlayCatch)
}

func (s *UIState) rowContextMenu(gtx layout.Context) {
	s.overlayCatcher(gtx, func() { s.CtxOpen = false })
	anchor := image.Pt(s.CtxPos.X, s.CtxPos.Y)
	widgets.DeferMenu(gtx, s.host.Theme, &s.OverlayCatch, anchor, 190, []widgets.MenuItem{
		{Label: "Send to Repeater", Click: &s.CtxToRepeater, Icon: widgets.IconUpload},
		{Label: "Add to scope", Click: &s.CtxAddScope},
		{Separator: true},
		{Label: "Copy URL", Click: &s.CtxCopyURL, Icon: widgets.IconDup},
		{Label: "Copy as curl", Click: &s.CtxCopyCurl},
		{Label: "Copy request", Click: &s.CtxCopyReq},
		{Separator: true},
		{Label: "Repeat request", Click: &s.CtxRepeat, Icon: widgets.IconRefresh},
		{Label: "Annotate…", Click: &s.CtxAnnotate, Icon: widgets.IconRename},
		{Separator: true},
		{Label: "Delete from history", Click: &s.CtxDelete, Icon: widgets.IconDel, Danger: true},
	})
}

func (s *UIState) clearConfirm(gtx layout.Context) {
	// dim + center a modal card
	paint.FillShape(gtx.Ops, theme.WithAlpha(theme.Bg, 180), clip.Rect{Max: gtx.Constraints.Max}.Op())
	s.overlayCatcher(gtx, func() { s.ClearConfirmOpen = false })

	macro := op.Record(gtx.Ops)
	cardGtx := gtx
	cardGtx.Constraints.Min = image.Point{}
	cardGtx.Constraints.Max.X = gtx.Dp(unit.Dp(320))
	dims := s.modalCard(cardGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(13), fmt.Sprintf("Clear %d flows?", s.Store.Len()))
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(11), "This removes all captured flows and WebSocket messages.")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(14)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return btn(gtx, s.host.Theme, &s.ClearNoBtn, "Cancel", nil, theme.Border, s.host.Theme.Fg, true)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return btn(gtx, s.host.Theme, &s.ClearYesBtn, "Clear", nil, theme.Cancel, theme.DangerFg, true)
					}),
				)
			}),
		)
	})
	call := macro.Stop()
	pos := image.Pt((gtx.Constraints.Max.X-dims.Size.X)/2, (gtx.Constraints.Max.Y-dims.Size.Y)/3)
	def := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	call.Add(gtx.Ops)
	op.Defer(gtx.Ops, def.Stop())
}

func (s *UIState) annotatePopup(gtx layout.Context) {
	paint.FillShape(gtx.Ops, theme.WithAlpha(theme.Bg, 180), clip.Rect{Max: gtx.Constraints.Max}.Op())
	s.overlayCatcher(gtx, func() { s.AnnotateOpen = false })

	macro := op.Record(gtx.Ops)
	cardGtx := gtx
	cardGtx.Constraints.Min = image.Point{}
	cardGtx.Constraints.Max.X = gtx.Dp(unit.Dp(340))
	dims := s.modalCard(cardGtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(s.host.Theme, unit.Sp(13), "Annotate flow")
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, s.annotateSwatches(gtx)...)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				s.AnnotateComment.SingleLine = false
				return widgets.TextField(gtx, s.host.Theme, &s.AnnotateComment, "comment…", true, nil, 0, unit.Sp(12))
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return btn(gtx, s.host.Theme, &s.AnnotateSave, "Save", nil, theme.BtnPrimary, theme.BtnPrimaryFg, true)
					}),
				)
			}),
		)
	})
	call := macro.Stop()
	pos := image.Pt((gtx.Constraints.Max.X-dims.Size.X)/2, (gtx.Constraints.Max.Y-dims.Size.Y)/3)
	def := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	call.Add(gtx.Ops)
	op.Defer(gtx.Ops, def.Stop())
}

func (s *UIState) annotateSwatches(gtx layout.Context) []layout.FlexChild {
	keys := annotateColorKeys()
	children := make([]layout.FlexChild, 0, len(keys))
	for i := range keys {
		i := i
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.AnnotateColors[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				sz := gtx.Dp(unit.Dp(22))
				col := highlightColor(keys[i])
				if keys[i] == "" {
					// "none" swatch
					paint.FillShape(gtx.Ops, theme.BgField, clip.Rect{Max: image.Pt(sz, sz)}.Op())
					widgets.PaintBorder1px(gtx, image.Pt(sz, sz), theme.Border)
					lbl := material.Label(s.host.Theme, unit.Sp(11), "∅")
					lbl.Color = theme.FgMuted
					_ = lbl
				} else {
					paint.FillShape(gtx.Ops, col, clip.Rect{Max: image.Pt(sz, sz)}.Op())
				}
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Dimensions{Size: image.Pt(sz+gtx.Dp(unit.Dp(6)), sz)}
			})
		}))
	}
	return children
}

func (s *UIState) modalCard(gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(unit.Dp(14)).Layout(gtx, w)
	call := macro.Stop()
	sz := dims.Size
	paint.FillShape(gtx.Ops, theme.BgMenu, clip.UniformRRect(image.Rectangle{Max: sz}, 6).Op(gtx.Ops))
	call.Add(gtx.Ops)
	widgets.PaintBorder1px(gtx, sz, theme.Border)
	return dims
}
