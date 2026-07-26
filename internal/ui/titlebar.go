package ui

import (
	"strings"

	"tracto/internal/ui/mitm"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	ui.wireNetTitlebar()
	ui.wireMITMTitlebar()
	return ui.TitleBar.Layout(gtx, ui.Theme, ui.Window, ui.Title, ui.BugReportURL, ui.SettingsOpen, func() {
		ui.SettingsOpen = !ui.SettingsOpen
		if ui.SettingsOpen && ui.SettingsState == nil {
			ui.SettingsState = settings.NewEditor(ui.Settings)
		}

		ui.VarPopup.Close()
		ui.EnvColorPicker.Close()
		widgets.GlobalVarHover = nil
		widgets.GlobalVarClick = nil
	})
}

// wireMITMTitlebar feeds the centered proxy-status block, shown only when the
// Proxy module is active. Clicking it toggles Start/Stop.
func (ui *AppUI) wireMITMTitlebar() {
	if ui.SidebarSection != "mitm" || ui.SettingsOpen {
		ui.TitleBar.MITMShow = false
		return
	}
	st := &ui.MITM
	st.Ensure()
	if !st.Proxy.Running() {
		ui.TitleBar.MITMShow = false
		return
	}
	ui.TitleBar.MITMShow = true
	ui.TitleBar.MITMActive = true
	ui.TitleBar.MITMAddr = st.Proxy.Addr()
	if ui.TitleBar.MITMAddr == "" {
		ui.TitleBar.MITMAddr = st.BindAddr.Text()
	}
	ui.TitleBar.MITMFlows = st.Store.Len()
	ui.TitleBar.OnMITMToggle = func() {
		switch {
		case st.Proxy.Running():
			st.Proxy.Stop()
			st.StatusBanner = "Proxy stopped"
		case !mitm.IsAdmin():
			ui.elevateAndRelaunch(&st.StatusBanner, "--mitm-start")
		default:
			addr := strings.TrimSpace(st.BindAddr.Text())
			if err := st.Proxy.Start(addr); err != nil {
				st.StatusBanner = "Start failed: " + err.Error()
			} else {
				st.StatusBanner = "Proxy listening on " + st.Proxy.Addr()
			}
		}
		ui.Window.Invalidate()
	}
}

func (ui *AppUI) settingsHost() *settings.Host {
	return &settings.Host{
		Theme:   ui.Theme,
		Window:  ui.Window,
		Current: &ui.Settings,
		Open:    &ui.SettingsOpen,
		OnClose: func() {
			ui.SettingsOpen = false
			ui.SettingsState = nil
		},
		OnSave: ui.saveState,
	}
}
