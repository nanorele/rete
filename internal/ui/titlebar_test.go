package ui

import (
	"image"
	"testing"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
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

	// Test Settings button toggles state
	ui.IsMaximized = false
	ui.SettingsOpen = false
	ui.SettingsBtn.Click()
	ui.layoutTitleBar(gtx)
	if !ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=true after click")
	}
	if ui.SettingsState == nil {
		t.Errorf("expected SettingsState to be initialized after first open")
	}
	ui.SettingsBtn.Click()
	ui.layoutTitleBar(gtx)
	if ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=false after second click")
	}
}

func TestTitleBarSettingsButtonHitArea(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win
	ui.SettingsOpen = false

	var router input.Router
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 30)),
		Source:      router.Source(),
	}

	// Render once so click area is registered.
	ui.layoutTitleBar(gtx)
	router.Frame(ops)

	// "Tracto" 14sp ≈ 50px wide; 12dp left pad + label + 8dp gap → button starts ~70px in.
	// Settings button content (icon + spacer + text + 20px inset) ≈ 90px wide.
	// Click in the middle of the expected button area.
	router.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(110, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(110, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)

	// Re-render so Clickable.update consumes the queued events.
	ops.Reset()
	gtx.Ops = ops
	ui.layoutTitleBar(gtx)

	if !ui.SettingsOpen {
		t.Errorf("expected SettingsOpen=true after pointer click in button area")
	}

	// Click outside the button (in the flexed drag area, e.g. x=600) should NOT toggle.
	router.Frame(ops)
	router.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(600, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(600, 15), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	ops.Reset()
	gtx.Ops = ops
	prev := ui.SettingsOpen
	ui.layoutTitleBar(gtx)
	if ui.SettingsOpen != prev {
		t.Errorf("click in drag area should not toggle SettingsOpen")
	}
}
