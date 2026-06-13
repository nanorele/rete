package ui

import (
	"tracto/internal/ui/settings"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	ui.wireNetTitlebar()
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
