package sidebar

import (
	"math"
	"os"
	"path/filepath"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/widgets"
)

func addNewCollection(host *Host) {
	id := persist.NewRandomID()
	root := &collections.CollectionNode{
		Name:     "New Collection",
		IsFolder: true,
		Depth:    0,
		Expanded: true,
	}
	root.NameEditor.SingleLine = true
	root.NameEditor.Submit = true
	col := &collections.ParsedCollection{
		ID:   id,
		Name: "New Collection",
		Root: root,
	}
	collections.AssignParents(root, nil, col)
	*host.Collections = append(*host.Collections, &collections.CollectionUI{Data: col})
	*host.ColsExpanded = true
	host.MarkCollectionDirty(col)
	host.UpdateVisibleCols()
	host.Window.Invalidate()
}

func addNewEnvironment(host *Host) {
	id := persist.NewRandomID()
	env := &model.ParsedEnvironment{
		ID:   id,
		Name: "New Environment",
	}
	envUI := &environments.EnvironmentUI{Data: env}
	_ = persist.SaveEnvironment(env)
	*host.Environments = append(*host.Environments, envUI)
	*host.EnvsExpanded = true
	*host.EditingEnv = envUI
	envUI.InitEditor()
	host.SaveState()
	host.Window.Invalidate()
}

func deleteEnvironment(host *Host, env *environments.EnvironmentUI) {
	if env == nil || env.Data == nil {
		return
	}
	for i, e := range *host.Environments {
		if e == env {
			*host.Environments = append((*host.Environments)[:i], (*host.Environments)[i+1:]...)
			break
		}
	}
	widgets.ResetEditorHScroll(&env.NameEditor)
	widgets.ResetEditorHScroll(&env.ColorEditor)
	widgets.ResetEditorHScroll(&env.InlineNameEd)
	for _, r := range env.Rows {
		widgets.ResetEditorHScroll(&r.KeyEditor)
		widgets.ResetEditorHScroll(&r.ValEditor)
	}
	if *host.ActiveEnvID == env.Data.ID {
		*host.ActiveEnvID = ""
		*host.ActiveEnvDirty = true
	}
	if *host.EditingEnv == env {
		*host.EditingEnv = nil
	}
	if env.Data.ID != "" {
		_ = os.Remove(filepath.Join(persist.EnvironmentsDir(), env.Data.ID+".json"))
	}
	host.SaveState()
	host.Window.Invalidate()
}

func duplicateEnvironment(host *Host, src *environments.EnvironmentUI) {
	if src == nil || src.Data == nil {
		return
	}
	id := persist.NewRandomID()
	dup := &model.ParsedEnvironment{
		ID:             id,
		Name:           src.Data.Name + " (copy)",
		HighlightColor: src.Data.HighlightColor,
	}
	dup.Vars = append(dup.Vars, src.Data.Vars...)
	envUI := &environments.EnvironmentUI{Data: dup}
	_ = persist.SaveEnvironment(dup)
	*host.Environments = append(*host.Environments, envUI)
	*host.EnvsExpanded = true
	host.SaveState()
	host.Window.Invalidate()
}

func dragEnvDropTargetIdx(host *Host) int {
	if *host.DraggedEnv == nil || !*host.DragEnvActive || *host.EnvRowH <= 0 {
		return -1
	}
	srcIdx := -1
	for i, e := range *host.Environments {
		if e == *host.DraggedEnv {
			srcIdx = i
			break
		}
	}
	if srcIdx < 0 {
		return -1
	}
	rowsDelta := int(math.Round(float64(*host.DragEnvCurrentY-*host.DragEnvOriginY) / float64(*host.EnvRowH)))
	target := srcIdx + rowsDelta
	if target < 0 {
		target = 0
	}
	if target >= len(*host.Environments) {
		target = len(*host.Environments) - 1
	}
	return target
}

func commitEnvDrop(host *Host, src *environments.EnvironmentUI) {
	target := dragEnvDropTargetIdx(host)
	if target < 0 {
		return
	}
	srcIdx := -1
	for i, e := range *host.Environments {
		if e == src {
			srcIdx = i
			break
		}
	}
	if srcIdx < 0 || srcIdx == target {
		return
	}
	envs := make([]*environments.EnvironmentUI, 0, len(*host.Environments))
	moved := (*host.Environments)[srcIdx]
	for i, e := range *host.Environments {
		if i == srcIdx {
			continue
		}
		envs = append(envs, e)
	}
	insertIdx := target
	if insertIdx > len(envs) {
		insertIdx = len(envs)
	}
	envs = append(envs[:insertIdx], append([]*environments.EnvironmentUI{moved}, envs[insertIdx:]...)...)
	*host.Environments = envs
	host.SaveState()
	host.Window.Invalidate()
}
