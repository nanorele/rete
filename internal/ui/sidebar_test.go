package ui

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestSidebarLayout(t *testing.T) {
	setupTestConfigDir(t)
	win := new(app.Window)
	ui := NewAppUI()
	ui.Window = win

	col := &ParsedCollection{
		ID:   "c1",
		Name: "C1",
		Root: &CollectionNode{
			Name:     "R1",
			IsFolder: true,
			Expanded: true,
			Children: []*CollectionNode{
				{
					Name: "Child",
					Request: &ParsedRequest{
						Method: "GET",
					},
				},
			},
		},
	}
	col.Root.Collection = col
	col.Root.Children[0].Parent = col.Root
	col.Root.Children[0].Collection = col
	
	ui.Collections = append(ui.Collections, &CollectionUI{Data: col})
	ui.updateVisibleCols()

	env := &ParsedEnvironment{
		ID:   "e1",
		Name: "E1",
	}
	ui.Environments = append(ui.Environments, &EnvironmentUI{Data: env})

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(300, 768)),
		Now:         time.Now(),
	}

	ui.layoutSidebar(gtx)

	// Test Expand/Collapse Collections
	ui.ColsHeaderClick.Click()
	ui.layoutSidebar(gtx)
	if ui.ColsExpanded {
		t.Errorf("expected collapsed")
	}
	ui.ColsHeaderClick.Click()
	ui.layoutSidebar(gtx)

	// Test Node Menu Open
	node := ui.VisibleCols[1] // The child request
	node.MenuBtn.Click()
	ui.layoutSidebar(gtx)
	if !node.MenuOpen {
		t.Errorf("expected menu open")
	}

	// Test Add Request
	node.AddReqBtn.Click()
	ui.layoutSidebar(gtx)
	if len(node.Children) != 1 {
		t.Errorf("expected child added to node")
	}

	// Test Duplicate
	node.MenuBtn.Click() // Re-open menu
	ui.layoutSidebar(gtx)
	node.DupBtn.Click()
	ui.layoutSidebar(gtx)
	// Duplicate adds to parent
	if len(col.Root.Children) != 2 {
		t.Errorf("expected node duplicated, got %d children", len(col.Root.Children))
	}

	// Test Delete
	node.MenuBtn.Click() // Re-open menu
	ui.layoutSidebar(gtx)
	node.DelBtn.Click()
	ui.layoutSidebar(gtx)
	if len(col.Root.Children) != 1 {
		t.Errorf("expected node deleted")
	}

	// Test Rename
	node = col.Root.Children[0]
	node.MenuBtn.Click() // Re-open menu
	ui.layoutSidebar(gtx)
	node.EditBtn.Click()
	ui.layoutSidebar(gtx)
	if !node.IsRenaming {
		t.Errorf("expected renaming mode")
	}
	node.NameEditor.SetText("New Name")

	// Test Select Env
	envUI := ui.Environments[0]
	envUI.Click.Click()
	ui.layoutSidebar(gtx)
	if ui.ActiveEnvID != "e1" {
		t.Errorf("expected env selected")
	}
	envUI.SelectBtn.Click() // Toggle off
	ui.layoutSidebar(gtx)
	if ui.ActiveEnvID != "" {
		t.Errorf("expected env deselected")
	}

	// Test Env Edit Mode
	envUI.EditBtn.Click()
	ui.layoutSidebar(gtx)
	if ui.EditingEnv != envUI {
		t.Errorf("expected editing env")
	}
}
