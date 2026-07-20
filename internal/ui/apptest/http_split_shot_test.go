//go:build screenshots

package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"

	"tracto/internal/ui/settings"
	"tracto/internal/ui/workspace"
)

func httpSplitScenes() []scene {
	mk := func(themeID string, mode int) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = themeID
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = mode
		}
	}
	return []scene{
		{"httpsplit-h-dark", mk("dark", workspace.LayoutModeHoriz)},
		{"httpsplit-v-dark", mk("dark", workspace.LayoutModeVert)},
		{"httpsplit-h-dracula", mk("dracula", workspace.LayoutModeHoriz)},
		{"httpsplit-v-dracula", mk("dracula", workspace.LayoutModeVert)},
		{"httpsplit-h-reqcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.ReqBodyCollapsed = true
		}},
		{"httpsplit-v-reqcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.ReqBodyCollapsed = true
			tab.VStackRatio = 0.01
		}},
		{"httpsplit-v-respcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.RespBodyCollapsed = true
			tab.VStackRatio = 0.99
		}},
	}
}

func TestHTTPSplitScreenshots(t *testing.T) {
	for _, sc := range httpSplitScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}
