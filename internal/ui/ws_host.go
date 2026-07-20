package ui

import (
	"crypto/tls"

	"tracto/internal/ui/mitm"
	"tracto/internal/ui/workspace"
)

func (ui *AppUI) triggerWSAction(rt *workspace.RequestTab) {
	s := rt.EnsureWS()
	if s.State() == workspace.WSStateOpen {
		rt.SendFromComposer()
		return
	}
	rt.WSConnect(ui.rootCtx, ui.buildWSTLSConfig(rt), ui.activeEnvSnapshot(), nil)
}

func (ui *AppUI) wireWSHost(rt *workspace.RequestTab) {
	rt.WSHost = workspace.WSHostFuncs{
		OnConnect: func(t *workspace.RequestTab) {
			t.WSConnect(ui.rootCtx, ui.buildWSTLSConfig(t), ui.activeEnvSnapshot(), nil)
			ui.saveState()
		},
		OnDisconnect: func(t *workspace.RequestTab) {
			t.WSDisconnect()
		},
	}
}

func (ui *AppUI) buildWSTLSConfig(rt *workspace.RequestTab) *tls.Config {
	s := rt.EnsureWS()
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if s.InsecureSkipVerify {
		cfg.InsecureSkipVerify = true
		return cfg
	}
	if s.UseTractoCA {
		if pool := mitm.TractoTrustPool(); pool != nil {
			cfg.RootCAs = pool
		}
	}
	return cfg
}
