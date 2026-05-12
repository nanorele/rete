package ui

import (
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) closeTab(idx int) {
	if idx < 0 || idx >= len(ui.Tabs) {
		return
	}
	// Dismiss any pending var hover/click referencing this tab's editors
	// before they get torn down — pointer.Leave will never arrive for an
	// editor that's no longer mounted, so the global state would leak.
	widgets.GlobalVarHover = nil
	widgets.GlobalVarClick = nil
	ui.VarPopup.Close()
	tab := ui.Tabs[idx]
	tab.CancelRequest()
	tab.MarkClosed()
	widgets.ResetEditorHScroll(&tab.URLInput)
	for _, h := range tab.Headers {
		widgets.ResetEditorHScroll(&h.Key)
		widgets.ResetEditorHScroll(&h.Value)
	}
	for _, p := range tab.FormParts {
		widgets.ResetEditorHScroll(&p.Key)
		widgets.ResetEditorHScroll(&p.Value)
	}
	for _, ue := range tab.URLEncoded {
		widgets.ResetEditorHScroll(&ue.Key)
		widgets.ResetEditorHScroll(&ue.Value)
	}
	ui.TabBar.Forget(tab)
	ui.Tabs = append(ui.Tabs[:idx], ui.Tabs[idx+1:]...)
	if len(ui.Tabs) == 0 {
		ui.ActiveIdx = -1
	} else {
		if ui.ActiveIdx >= idx && ui.ActiveIdx > 0 {
			ui.ActiveIdx--
		} else if ui.ActiveIdx >= len(ui.Tabs) {
			ui.ActiveIdx = len(ui.Tabs) - 1
		}
		if ui.ActiveIdx < 0 {
			ui.ActiveIdx = 0
		}
	}
	ui.saveState()
}

func (ui *AppUI) layoutTabBar(gtx layout.Context) layout.Dimensions {
	return ui.TabBar.Layout(gtx, ui.Theme, &ui.Tabs, &ui.ActiveIdx, ui.revealLinkedNode, ui.saveState)
}
