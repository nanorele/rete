package ui

import (
	"tracto/internal/ui/settings"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) layoutTitleBar(gtx layout.Context) layout.Dimensions {
	return ui.TitleBar.Layout(gtx, ui.Theme, ui.Window, ui.Title, ui.BugReportURL, ui.SettingsOpen, func() {
		ui.SettingsOpen = !ui.SettingsOpen
		if ui.SettingsOpen && ui.SettingsState == nil {
			ui.SettingsState = settings.NewEditor(ui.Settings)
		}
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
