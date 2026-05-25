package ui

import (
	"crypto/tls"
	"crypto/x509"
	"os"

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
		if pool := ui.tractoTrustPool(); pool != nil {
			cfg.RootCAs = pool
		}
	}
	return cfg
}

func (ui *AppUI) tractoTrustPool() *x509.CertPool {
	caPath := mitm.CACertPath(mitm.MITMDir())
	if _, err := os.Stat(caPath); err != nil {
		return nil
	}
	pemBytes, err := os.ReadFile(caPath)
	if err != nil {
		return nil
	}
	systemPool, _ := x509.SystemCertPool()
	if systemPool == nil {
		systemPool = x509.NewCertPool()
	}
	if !systemPool.AppendCertsFromPEM(pemBytes) {
		return nil
	}
	return systemPool
}
