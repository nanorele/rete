package ui

import (
	"tracto/internal/ui/workspace"
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

	ui.Tabs = nil
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"))
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T2 long title for multi words test"))
	ui.ActiveIdx = 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1024, 768)),
		Now:         time.Now(),
	}

	ui.layoutTabBar(gtx)

	ui.Tabs[1].TabBtn.Click()
	ui.layoutTabBar(gtx)
	if ui.ActiveIdx != 1 {
		t.Errorf("expected tab 1 active, got %d", ui.ActiveIdx)
	}

	ui.TabBar.AddTabBtn.Click()
	ui.layoutContent(gtx)
	if len(ui.Tabs) != 3 {
		t.Errorf("expected 3 tabs after add, got %d", len(ui.Tabs))
	}

	ui.closeTab(0)
	if len(ui.Tabs) != 2 {
		t.Errorf("expected 2 tabs after close")
	}

	_ = material.NewTheme()
}

func TestTabBar_Dragging(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("T1"), workspace.NewRequestTab("T2"))

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(800, 100)),
	}

	ui.TabBar.TabDragging = true
	ui.TabBar.TabDragIdx = 0
	ui.TabBar.TabDragCurrentX = 100
	ui.TabBar.TabDragCurrentY = 50
	ui.layoutTabBar(gtx)
}

func TestTabBarWrapping(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.Tabs = nil
	for i := 0; i < 20; i++ {
		ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("Tab"))
	}

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 800)),
		Now:         time.Now(),
	}

	ui.layoutTabBar(gtx)
}
