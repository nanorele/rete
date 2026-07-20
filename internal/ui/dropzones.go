package ui

import (
	dropui "tracto/internal/ui/dropzones"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) dropHost() *dropui.Host {
	return &dropui.Host{
		Theme:          ui.Theme,
		Window:         ui.Window,
		Blocked:        ui.SettingsOpen || ui.EditingEnv != nil,
		SidebarSection: ui.SidebarSection,
		SidebarHidden:  ui.hideSidebar(),
		SidebarZones:   ui.sidebarZones,
		LoadHAR: func(path string) {
			ui.HARView.LoadPathAsync(path, ui.Window.Invalidate)
		},
		ImportData:    ui.importDroppedData,
		PushColLoaded: ui.pushColLoaded,
		PushEnvLoaded: ui.pushEnvLoaded,
	}
}

func (ui *AppUI) initDropzones() {
	ui.Drop.Init()
}

func (ui *AppUI) onOSFilesDragged(pos f32.Point, active bool) {
	ui.Drop.Dragged(ui.dropHost(), pos, active)
}

func (ui *AppUI) onOSFilesDropped(paths []string, pos f32.Point) {
	ui.Drop.Dropped(ui.dropHost(), paths, pos)
}

func (ui *AppUI) drainDroppedFiles() {
	ui.Drop.Drain(ui.dropHost())
}

func (ui *AppUI) rebuildDropZones(gtx layout.Context) {
	ui.Drop.RebuildZones(gtx, ui.dropHost())
}

func (ui *AppUI) layoutDropOverlay(gtx layout.Context) {
	ui.Drop.LayoutOverlay(gtx, ui.dropHost())
}
