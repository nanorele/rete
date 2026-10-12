//go:build screenshots

package apptest

import (
	"image"
	"testing"

	"rete/internal/model"
	. "rete/internal/ui"
	"rete/internal/ui/collections"
	"rete/internal/ui/environments"
	"rete/internal/ui/flow"
)

func tasksScenes() []scene {
	return []scene{
		{"tasks-node-menu-export", func(ui *AppUI) {
			withTab(ui)
			ui.ColsExpanded = true
			root := ui.VisibleCols[0]
			root.MenuOpen = true
			root.SubMenu.SetPath(4, 0)
		}},
		{"tasks-separators", func(ui *AppUI) {
			withTab(ui)
			ui.ColsExpanded, ui.EnvsExpanded, ui.ScriptsExpanded = true, true, true
			second := &collections.CollectionNode{Name: "Billing", IsFolder: true, Expanded: false}
			col2 := &collections.ParsedCollection{ID: "col2", Name: "Billing", Root: second}
			collections.AssignParents(second, nil, col2)
			ui.Collections = append(ui.Collections, collections.NewSeparator(""), &collections.CollectionUI{Data: col2})
			ui.UpdateVisibleCols()
			ui.Environments = append(ui.Environments,
				&environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "sep_e"}, IsSeparator: true},
				&environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "env2", Name: "Prod"}},
			)
			ed := flow.NewEditor()
			ed.CreateNew()
			_, _ = flow.AddScenarioSeparator("")
			ed2 := flow.NewEditor()
			ed2.CreateNew()
		}},
		{"tasks-close-confirm", func(ui *AppUI) {
			withTab(ui)
			tab := ui.Tabs[0]
			tab.LinkedNode = ui.VisibleCols[0].Children[0]
			tab.URLInput.SetText("{{base_url}}/users?page=2")
			tab.IsDirty = true
			ui.RequestCloseTab(0)
		}},
		{"tasks-method-get", func(ui *AppUI) {
			withTab(ui)
			ui.Tabs[0].Method = "GET"
			ui.Tabs[0].URLInput.SetText("https://api.example.com/{{$randomUUID}}")
		}},
		{"tasks-method-post", func(ui *AppUI) {
			withTab(ui)
			ui.Tabs[0].Method = "POST"
			ui.Tabs[0].URLInput.SetText("https://api.example.com/{{$randomUUID}}")
		}},
	}
}

func TestTasksScreenshots(t *testing.T) {
	sz := image.Point{X: 1100, Y: 700}
	for _, sc := range tasksScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, sz)
		})
	}
}
