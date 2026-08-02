//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"

	. "tracto/internal/ui"
)

func seedCollection(ui *AppUI, folders, perFolder int) {
	root := &collections.CollectionNode{Name: "Big API", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "big", Name: "Big API", Root: root}
	root.Collection = col
	for f := 0; f < folders; f++ {
		folder := &collections.CollectionNode{
			Name: fmt.Sprintf("group-%02d", f), IsFolder: true, Expanded: true,
			Parent: root, Depth: 1, Collection: col,
		}
		for i := 0; i < perFolder; i++ {
			req := &model.ParsedRequest{
				Name:   fmt.Sprintf("request-%02d-%03d", f, i),
				Method: "GET",
				URL:    fmt.Sprintf("{{base_url}}/group/%d/item/%d", f, i),
			}
			folder.Children = append(folder.Children, &collections.CollectionNode{
				Name: req.Name, Request: req, Parent: folder, Depth: 2, Collection: col,
			})
		}
		root.Children = append(root.Children, folder)
	}
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()
}

func seedEnvironments(ui *AppUI, envs, vars int) {
	ui.Environments = nil
	for e := 0; e < envs; e++ {
		vs := make([]model.EnvVar, vars)
		for i := range vs {
			vs[i] = model.EnvVar{
				Key:   fmt.Sprintf("var_%03d", i),
				Value: fmt.Sprintf("value-%d-%d-with-some-length", e, i),
			}
		}
		env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
			ID:   fmt.Sprintf("env%d", e),
			Name: fmt.Sprintf("Environment %d", e),
			Vars: vs,
		}}
		ui.Environments = append(ui.Environments, env)
	}
	if envs > 0 {
		ui.ActiveEnvID = "env0"
		ui.RefreshActiveEnv()
	}
}

func TestSectionsWithData(t *testing.T) {
	fmt.Printf("\n=== sections with data ===\n")

	cases := []struct {
		name  string
		setup func(*idleRig)
	}{
		{"baseline (empty)", func(rig *idleRig) {}},
		{"collections 20x50", func(rig *idleRig) { seedCollection(rig.ui, 20, 50) }},
		{"collections 100x100", func(rig *idleRig) { seedCollection(rig.ui, 100, 100) }},
		{"environments 10x200", func(rig *idleRig) { seedEnvironments(rig.ui, 10, 200) }},
		{"env editor open 200", func(rig *idleRig) {
			seedEnvironments(rig.ui, 10, 200)
			rig.ui.EditingEnv = rig.ui.Environments[0]
			rig.ui.Environments[0].InitEditor()
		}},
		{"env editor open 2000", func(rig *idleRig) {
			seedEnvironments(rig.ui, 1, 2000)
			rig.ui.EditingEnv = rig.ui.Environments[0]
			rig.ui.Environments[0].InitEditor()
		}},
		{"flows section", func(rig *idleRig) { rig.ui.SetSidebarSection("flows") }},
		{"netlimit section", func(rig *idleRig) { rig.ui.SetSidebarSection("netlimit") }},
		{"mitm section", func(rig *idleRig) { rig.ui.SetSidebarSection("mitm") }},
		{"har section", func(rig *idleRig) { rig.ui.SetSidebarSection("har") }},
	}

	for _, c := range cases {
		rig := newIdleRig(t)
		before := snap()
		c.setup(rig)
		for i := 0; i < 20; i++ {
			rig.frame()
		}
		after := snap()
		churn, _ := rig.churn(60)
		fmt.Printf("%-22s retained=%7.2fMB (+%6.2fMB)  churn=%8.1f KB/frame\n",
			c.name, mb(after.HeapAlloc), mb(after.HeapAlloc-before.HeapAlloc), churn)
		runtime.KeepAlive(rig)
	}
}
