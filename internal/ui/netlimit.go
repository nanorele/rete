package ui

import (
	netui "tracto/internal/ui/netlimit"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) netHost() *netui.Host {
	return &netui.Host{
		Theme:  ui.Theme,
		Window: ui.Window,
	}
}

func (ui *AppUI) initNetlimit() {
	ui.Net.Init()
}

func (ui *AppUI) closeNetlimit() {
	ui.Net.Close()
}

func (ui *AppUI) wireNetTitlebar() {
	if !ui.Net.Started() {
		return
	}
	ui.TitleBar.NetActive, ui.TitleBar.NetPaused = ui.Net.Status()
	ui.TitleBar.OnNetToggle = func() {
		ui.Net.ToggleLimit(ui.Window.Invalidate)
	}
	ui.TitleBar.OnNetCancel = func() {
		ui.Net.CancelLimit(ui.Window.Invalidate)
	}
}

func (ui *AppUI) layoutNetlimitBody(gtx layout.Context) layout.Dimensions {
	return ui.Net.LayoutBody(gtx, ui.netHost())
}

func (ui *AppUI) layoutNetlimitSection(gtx layout.Context) layout.Dimensions {
	return ui.Net.LayoutSection(gtx, ui.netHost())
}

func (ui *AppUI) layoutSidebarSectionNetlimitBtn(gtx layout.Context) layout.Dimensions {
	return ui.layoutSidebarSectionBtn(gtx, &ui.BtnSecNetlimit, widgets.IconNetlimit, "netlimit")
}
