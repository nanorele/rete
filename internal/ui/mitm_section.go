package ui

import (
	"errors"
	"os"

	"tracto/internal/ui/mitm"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) mitmHost() *mitm.Host {
	return &mitm.Host{
		Theme:   ui.Theme,
		Window:  ui.Window,
		Elevate: ui.elevateAndRelaunch,
	}
}

func (ui *AppUI) layoutMITMSection(gtx layout.Context) layout.Dimensions {
	return ui.MITM.Layout(gtx, ui.mitmHost())
}

func (ui *AppUI) layoutMITMSidebar(gtx layout.Context) layout.Dimensions {
	return ui.MITM.LayoutSidebar(gtx, ui.mitmHost())
}

func (ui *AppUI) elevateAndRelaunch(banner *string, extraArg string) {
	if !mitm.CanRequestElevation() {
		*banner = "Administrator privileges required (no UAC available on this platform)"
		return
	}
	err := mitm.RelaunchAsAdmin(extraArg)
	switch {
	case err == nil:
		ui.shutdownAndExit()
	case errors.Is(err, mitm.ErrUACDenied):
		*banner = "Elevation denied"
	default:
		*banner = "Restart as admin failed: " + err.Error()
	}
}

func (ui *AppUI) shutdownAndExit() {
	if ui.MITM.Proxy != nil && ui.MITM.Proxy.Running() {
		ui.MITM.Proxy.Stop()
	}
	if ui.rootCancel != nil {
		ui.rootCancel()
	}
	ui.stateSaveWG.Wait()
	ui.flushCollectionSavesSync()
	ui.saveStateSync()
	os.Exit(0)
}
