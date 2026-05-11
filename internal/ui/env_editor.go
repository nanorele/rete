package ui

import (
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/environments"

	"github.com/nanorele/gio/layout"
)

func (ui *AppUI) commitEditingEnv() {
	ui.EditingEnv.Commit(func() { ui.activeEnvDirty = true })
}

func (ui *AppUI) envEditorHost() *environments.EditorHost {
	return &environments.EditorHost{
		Theme:  ui.Theme,
		Window: ui.Window,
		OnClose: func() {
			ui.EditingEnv = nil
		},
		OnDirty: func() {
			ui.activeEnvDirty = true
		},
	}
}

func (ui *AppUI) layoutEnvEditor(gtx layout.Context) layout.Dimensions {
	return ui.EditingEnv.LayoutEditor(gtx, ui.envEditorHost())
}

func (ui *AppUI) saveVarPopup() {
	if ui.VarPopup.EnvID == "" {
		return
	}
	for _, env := range ui.Environments {
		if env.Data.ID != ui.VarPopup.EnvID {
			continue
		}
		updated := false
		for i, v := range env.Data.Vars {
			if v.Key == ui.VarPopup.Name {
				env.Data.Vars[i].Value = ui.VarPopup.Editor.Text()
				updated = true
				break
			}
		}
		if !updated {
			env.Data.Vars = append(env.Data.Vars, model.EnvVar{
				Key:     ui.VarPopup.Name,
				Value:   ui.VarPopup.Editor.Text(),
				Enabled: true,
			})
		}
		_ = persist.SaveEnvironment(env.Data)
		ui.activeEnvDirty = true
		return
	}
}
