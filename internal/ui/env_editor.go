package ui

import (
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/widgets"

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
		OnColorSwatchClick: func(env *environments.EnvironmentUI) {
			if env == nil || env.Data == nil {
				return
			}
			if ui.EnvColorPicker.IsOpen() && ui.EnvColorEnvID == env.Data.ID {
				ui.EnvColorPicker.Close()
				return
			}
			ui.EnvColorEnvID = env.Data.ID
			ui.EnvColorPicker.Open(
				colorpicker.KindEnv,
				0,
				environments.HighlightColor(env.Data),
				colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y},
			)
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
				Key:   ui.VarPopup.Name,
				Value: ui.VarPopup.Editor.Text(),
			})
		}
		_ = persist.SaveEnvironment(env.Data)
		ui.activeEnvDirty = true
		return
	}
}
