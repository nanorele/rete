//go:build screenshots

package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/workspace"
)

func bodyTypeScenes() []scene {
	mk := func(mut func(*workspace.RequestTab)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			mut(tab)
		}
	}
	return []scene{
		{"bd-form-mixed", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyFormData
			t.FormParts = []*workspace.FormDataPart{
				workspace.NewFormPart("name", "alice", model.FormPartText, "", 0),
				workspace.NewFormPart("avatar", "", model.FormPartFile, "C:\\pics\\avatar.png", 34567),
				workspace.NewFormPart("empty", "", model.FormPartFile, "", 0),
			}
		})},
		{"bd-ue-fields", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyURLEncoded
			t.URLEncoded = []*workspace.URLEncodedPart{
				workspace.NewURLEncodedPart("user", "alice"),
				workspace.NewURLEncodedPart("limit", "50"),
			}
		})},
		{"bd-ue-empty", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyURLEncoded
		})},
		{"bd-binary-empty", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyBinary
		})},
		{"bd-binary-chosen", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyBinary
			t.BinaryFilePath = "C:\\data\\payload.bin"
			t.BinaryFileSize = 123456
		})},
	}
}

func TestBodyTypeScreenshots(t *testing.T) {
	for _, sc := range bodyTypeScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}
