package ui

import "tracto/internal/ui/varpopup"

func (ui *AppUI) varPopupHost() *varpopup.Host {
	return &varpopup.Host{
		Theme:        ui.Theme,
		Window:       ui.Window,
		Environments: &ui.Environments,
		ActiveEnvID:  &ui.ActiveEnvID,
		ActiveEnvVar: func(name string) (string, bool) {
			if ui.activeEnvVars == nil {
				return "", false
			}
			v, ok := ui.activeEnvVars[name]
			return v, ok
		},
		OnDismiss: ui.saveVarPopup,
		OnSelectEnv: func(envID string) {
			ui.ActiveEnvID = envID
			ui.activeEnvDirty = true
		},
		RefreshActiveEnv: ui.refreshActiveEnv,
		SaveState:        ui.saveState,
	}
}
