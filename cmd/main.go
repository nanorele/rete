package main

import (
	"os"
	"tracto/internal/ui"

	"github.com/nanorele/gio/app"
)

const appTitle = "Rete 0.5.0"
const bugReportURL = "https://github.com/nanorele/rete/issues/new"

func main() {
	go func() {
		uiApp := ui.NewAppUI()
		uiApp.Title = appTitle
		uiApp.BugReportURL = bugReportURL
		if err := uiApp.Run(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}
