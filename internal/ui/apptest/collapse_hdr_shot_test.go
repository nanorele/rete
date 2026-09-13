//go:build screenshots

package apptest

import (
	. "rete/internal/ui"

	"image"
	"testing"

	"rete/internal/ui/settings"
	"rete/internal/ui/workspace"
)

func collapseHdrScenes() []scene {
	http := func(mode int, mut func(*workspace.RequestTab)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = mode
			if mut != nil {
				mut(tab)
			}
		}
	}
	ws := func(mut func(*workspace.WSSession)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/ws")
			tab.AddHeader("Origin", "https://example.com")
			s := tab.EnsureWS()
			if mut != nil {
				mut(s)
			}
		}
	}
	return []scene{
		{"ch-h-hdr-open", http(workspace.LayoutModeHoriz, nil)},
		{"ch-h-hdr-closed", http(workspace.LayoutModeHoriz, func(t *workspace.RequestTab) {
			t.HeadersExpanded = false
		})},
		{"ch-v-hdr-open", http(workspace.LayoutModeVert, nil)},
		{"ch-v-hdr-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.HeadersExpanded = false
		})},
		{"ch-v-req-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.ReqBodyCollapsed = true
			t.VStackRatio = 0.01
		})},
		{"ch-h-req-closed", http(workspace.LayoutModeHoriz, func(t *workspace.RequestTab) {
			t.ReqBodyCollapsed = true
		})},
		{"ch-v-resp-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.RespBodyCollapsed = true
			t.VStackRatio = 0.99
		})},
		{"ch-ws-open", ws(nil)},
		{"ch-ws-hdr-closed", ws(func(s *workspace.WSSession) {
			s.HeadersCollapsed = true
		})},
		{"ch-ws-compose-closed", ws(func(s *workspace.WSSession) {
			s.ComposeCollapsed = true
		})},
		{"ch-ws-msgs-closed", ws(func(s *workspace.WSSession) {
			s.MessagesCollapsed = true
		})},
	}
}

func TestCollapseHdrScreenshots(t *testing.T) {
	for _, sc := range collapseHdrScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}
