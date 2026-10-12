package ui

import (
	"image"
	"image/color"
	"strconv"

	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"
	"rete/internal/ui/workspace"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type closeConfirm struct {
	Open       bool
	Tabs       []*workspace.RequestTab
	Dirty      []*workspace.RequestTab
	SaveBtn    widget.Clickable
	DiscardBtn widget.Clickable
	CancelBtn  widget.Clickable
	focused    bool
	tag        struct{}
}

func unsavedTabs(tabs []*workspace.RequestTab) []*workspace.RequestTab {
	var dirty []*workspace.RequestTab
	for _, t := range tabs {
		if t != nil && t.IsDirty && t.LinkedNode != nil {
			dirty = append(dirty, t)
		}
	}
	return dirty
}

func (ui *AppUI) requestCloseTabs(tabs []*workspace.RequestTab) {
	if len(tabs) == 0 {
		return
	}
	dirty := unsavedTabs(tabs)
	if len(dirty) == 0 {
		ui.closeTabsNow(tabs)
		return
	}
	ui.closeConfirm = closeConfirm{Open: true, Tabs: tabs, Dirty: dirty}
	ui.TabBar.TabCtxMenuOpen = false
	ui.Window.Invalidate()
}

func (ui *AppUI) requestCloseTab(idx int) {
	if idx < 0 || idx >= len(ui.Tabs) {
		return
	}
	ui.requestCloseTabs([]*workspace.RequestTab{ui.Tabs[idx]})
}

func (ui *AppUI) closeTabsNow(tabs []*workspace.RequestTab) {
	var active *workspace.RequestTab
	if ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
		active = ui.Tabs[ui.ActiveIdx]
	}
	closing := make(map[*workspace.RequestTab]struct{}, len(tabs))
	for _, t := range tabs {
		closing[t] = struct{}{}
	}
	for i := len(ui.Tabs) - 1; i >= 0; i-- {
		if _, ok := closing[ui.Tabs[i]]; ok {
			ui.closeTab(i)
		}
	}
	if active == nil {
		return
	}
	if _, closed := closing[active]; closed {
		return
	}
	for i, t := range ui.Tabs {
		if t == active {
			ui.ActiveIdx = i
			break
		}
	}
}

func (ui *AppUI) resolveCloseConfirm(save bool) {
	cc := &ui.closeConfirm
	if !cc.Open {
		return
	}
	if save {
		for _, t := range cc.Dirty {
			if col := t.SaveToCollection(); col != nil {
				ui.markCollectionDirty(col)
			}
		}
	}
	tabs := cc.Tabs
	*cc = closeConfirm{}
	ui.closeTabsNow(tabs)
	ui.Window.Invalidate()
}

func (ui *AppUI) cancelCloseConfirm() {
	ui.closeConfirm = closeConfirm{}
	ui.Window.Invalidate()
}

func closeConfirmMessage(dirty []*workspace.RequestTab) (string, []string) {
	if len(dirty) == 1 {
		return "Do you want to save the changes you made to " + dirty[0].GetCleanTitle() + "?", nil
	}
	names := make([]string, 0, len(dirty))
	for _, t := range dirty {
		names = append(names, t.GetCleanTitle())
	}
	return "Do you want to save the changes to the following " + strconv.Itoa(len(dirty)) + " requests?", names
}

func (ui *AppUI) layoutCloseConfirm(gtx layout.Context) {
	cc := &ui.closeConfirm
	if !cc.Open {
		return
	}
	for cc.SaveBtn.Clicked(gtx) {
		ui.resolveCloseConfirm(true)
		return
	}
	for cc.DiscardBtn.Clicked(gtx) {
		ui.resolveCloseConfirm(false)
		return
	}
	for cc.CancelBtn.Clicked(gtx) {
		ui.cancelCloseConfirm()
		return
	}
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &cc.tag, Name: key.NameEscape},
			key.Filter{Focus: &cc.tag, Name: key.NameReturn},
			key.Filter{Focus: &cc.tag, Name: key.NameEnter},
		)
		if !ok {
			break
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press {
			switch e.Name {
			case key.NameEscape:
				ui.cancelCloseConfirm()
				return
			case key.NameReturn, key.NameEnter:
				ui.resolveCloseConfirm(true)
				return
			}
		}
	}
	if !cc.focused {
		cc.focused = true
		gtx.Execute(key.FocusCmd{Tag: &cc.tag})
	}

	size := gtx.Constraints.Max
	macro := op.Record(gtx.Ops)
	paint.FillShape(gtx.Ops, color.NRGBA{A: 120}, clip.Rect{Max: size}.Op())
	scrim := clip.Rect{Max: size}.Push(gtx.Ops)
	event.Op(gtx.Ops, &cc.tag)
	pointer.CursorDefault.Add(gtx.Ops)
	for {
		if _, ok := gtx.Event(pointer.Filter{Target: &cc.tag, Kinds: pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Scroll}); !ok {
			break
		}
	}
	scrim.Pop()

	panelW := gtx.Dp(unit.Dp(400))
	if maxW := size.X - gtx.Dp(unit.Dp(32)); panelW > maxW {
		panelW = maxW
	}
	pGtx := gtx
	pGtx.Constraints.Min = image.Pt(panelW, 0)
	pGtx.Constraints.Max = image.Pt(panelW, size.Y-gtx.Dp(unit.Dp(32)))
	rec := op.Record(gtx.Ops)
	dims := ui.layoutCloseConfirmPanel(pGtx)
	call := rec.Stop()

	pos := image.Pt((size.X-dims.Size.X)/2, (size.Y-dims.Size.Y)/2)
	if pos.Y < 0 {
		pos.Y = 0
	}
	off := op.Offset(pos).Push(gtx.Ops)
	widgets.MenuShadow(gtx, dims.Size)
	widgets.PaintPopupSurface(gtx, dims.Size, 6, theme.BgPopup, widgets.MenuBorderColor())
	call.Add(gtx.Ops)
	off.Pop()
	op.Defer(gtx.Ops, macro.Stop())
}

func (ui *AppUI) layoutCloseConfirmPanel(gtx layout.Context) layout.Dimensions {
	cc := &ui.closeConfirm
	th := ui.Theme
	title, names := closeConfirmMessage(cc.Dirty)
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(13), title)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			}),
		}
		for _, n := range names {
			n := n
			children = append(children,
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(12), "• "+n)
						lbl.MaxLines = 1
						lbl.Truncator = "…"
						return lbl.Layout(gtx)
					})
				}),
			)
		}
		children = append(children,
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), "Your changes will be lost if you don't save them.")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				inset := layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
				return layout.Flex{Axis: layout.Horizontal, Spacing: layout.SpaceStart}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := widgets.PrimaryButton(th, &cc.SaveBtn, "Save")
						btn.Inset = inset
						return btn.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.OutlineButton(gtx, th, &cc.DiscardBtn, "Don't Save", theme.Danger, theme.BgPopup, inset)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						btn := widgets.FilledButton(th, &cc.CancelBtn, "Cancel", theme.BgSecondary, th.Fg)
						btn.Inset = inset
						return btn.Layout(gtx)
					}),
				)
			}),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}
