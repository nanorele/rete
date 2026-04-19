package ui

import (
	"image"
	"testing"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestTitleBarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 30)),
	}

	ui.layoutTitleBar(gtx)

	// Test Button Clicks
	ui.BtnMinimize.Click()
	ui.layoutTitleBar(gtx)

	ui.BtnMaximize.Click()
	ui.layoutTitleBar(gtx)
	if !ui.IsMaximized {
		t.Errorf("expected maximized")
	}

	ui.BtnMaximize.Click() // Toggle off
	ui.layoutTitleBar(gtx)
	if ui.IsMaximized {
		t.Errorf("expected unmaximized")
	}

	ui.BtnClose.Click()
	ui.layoutTitleBar(gtx)

	// Test Title Button variants
	ui.layoutTitleBtn(gtx, &ui.BtnMinimize, 0)
	ui.layoutTitleBtn(gtx, &ui.BtnMaximize, 1)
	ui.IsMaximized = true
	ui.layoutTitleBtn(gtx, &ui.BtnMaximize, 2)
	ui.layoutTitleBtn(gtx, &ui.BtnClose, 3)
}
