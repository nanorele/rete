package main

import (
	"os"
	"tracto/internal/ui"

	"github.com/nanorele/gio/app"
)

const appTitle = "Rete 0.5.8"
const bugReportURL = "https://github.com/nanorele/rete/issues/new"

func main() {
	go func() {
		uiApp := ui.NewAppUI()
		uiApp.Title = appTitle
		uiApp.BugReportURL = bugReportURL
		applyStartupArgs(uiApp, os.Args[1:])
		if err := uiApp.Run(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func applyStartupArgs(u *ui.AppUI, args []string) {
	for _, a := range args {
		switch a {
		case "--mitm-start":
			u.SetSidebarSection("mitm")
			u.MITMAutoStart = true
		case "--mitm-install-ca":
			u.SetSidebarSection("mitm")
			u.MITMAutoInstallCA = true
		case "--mitm-remove-ca":
			u.SetSidebarSection("mitm")
			u.MITMAutoRemoveCA = true
		}
	}
}
