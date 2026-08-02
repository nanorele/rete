package apptest

import (
	"image"
	"testing"

	. "tracto/internal/ui"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func findGtx(r *input.Router) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(1200, 800)),
		Source:      r.Source(),
	}
}

func findRig(t *testing.T) (*AppUI, *workspace.RequestTab) {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	if len(ui.Tabs) == 0 {
		t.Fatal("expected a request tab to exist")
	}
	tab := ui.Tabs[ui.ActiveIdx]
	tab.RespEditor.SetText(`{"name":"value"}`)
	return ui, tab
}

// Ctrl+F used to fall through to the active request tab from every section that
// is not the HAR viewer, so pressing it over MITM opened a search box on a pane
// that was not on screen — invisible, unclosable, and waiting to pop up the next
// time the workspace came back.
func TestFindShortcut_InertInSectionsWithoutText(t *testing.T) {
	for _, section := range []string{"mitm", "netlimit", "flows"} {
		// mitm has its own search now; what must never happen from any of
		// these sections is the shortcut reaching the hidden request tab
		t.Run(section, func(t *testing.T) {
			ui, tab := findRig(t)
			ui.SidebarSection = section

			var r input.Router
			ui.FindShortcut(findGtx(&r))

			if tab.RespSearch.Open || tab.ReqSearch.Open {
				t.Errorf("Ctrl+F in the %s section opened a search on the hidden request tab", section)
			}
		})
	}
}

func TestFindShortcut_OpensInTheWorkspace(t *testing.T) {
	ui, tab := findRig(t)
	ui.SidebarSection = "requests"

	var r input.Router
	ui.FindShortcut(findGtx(&r))

	if !tab.RespSearch.Open {
		t.Error("Ctrl+F in the workspace must open the response search")
	}
}

func TestFindShortcut_GoesToTheHARViewer(t *testing.T) {
	ui, tab := findRig(t)
	ui.SidebarSection = "har"

	var r input.Router
	ui.FindShortcut(findGtx(&r))

	if tab.RespSearch.Open || tab.ReqSearch.Open {
		t.Error("Ctrl+F in the HAR section must not reach the request tab")
	}
}
