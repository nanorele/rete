package ui

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func TestTabBarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	ui.Tabs = nil // Clear default tab
	ui.Tabs = append(ui.Tabs, NewRequestTab("T1"))
	ui.Tabs = append(ui.Tabs, NewRequestTab("T2 long title for multi words test"))
	ui.ActiveIdx = 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.layoutTabBar(gtx)
	
	// Test Tab Selection
	ui.Tabs[1].TabBtn.Click()
	ui.layoutTabBar(gtx)
	if ui.ActiveIdx != 1 {
		t.Errorf("expected tab 1 active, got %d", ui.ActiveIdx)
	}

	// Test Add Tab - must call layoutContent to trigger handler
	ui.AddTabBtn.Click()
	ui.layoutContent(gtx)
	if len(ui.Tabs) != 3 {
		t.Errorf("expected 3 tabs after add, got %d", len(ui.Tabs))
	}

	// Test Close Tab
	ui.closeTab(0)
	if len(ui.Tabs) != 2 {
		t.Errorf("expected 2 tabs after close")
	}

	// Test measureTabWidth
	th := material.NewTheme()
	measureTabWidth(gtx, th, "Short")
	measureTabWidth(gtx, th, "A very long title that should be truncated or wrapped")
	measureTabWidth(gtx, th, "")
}

func TestTabBarWrapping(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	
	// Add many tabs to force wrapping/multiple rows
	ui.Tabs = nil
	for i := 0; i < 20; i++ {
		ui.Tabs = append(ui.Tabs, NewRequestTab("Tab"))
	}
	
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 800)),
		Now:         time.Now(),
	}
	
	ui.layoutTabBar(gtx)
	// rows should be > 1
	if len(ui.tabRowsBuf) <= 1 {
		t.Errorf("expected multiple rows for many tabs in narrow width, got %d", len(ui.tabRowsBuf))
	}
}
