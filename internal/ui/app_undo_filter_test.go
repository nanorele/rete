package ui

import (
	"testing"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/key"
)

func filtersContainKey(ui *AppUI, name key.Name) bool {
	for _, f := range ui.contentKeyFilters() {
		if kf, ok := f.(key.Filter); ok && kf.Name == name && kf.Required == key.ModShortcut {
			return true
		}
	}
	return false
}

func TestContentKeyFiltersUndoOnlyInFlows(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	ui.SidebarSection = "collections"
	if filtersContainKey(ui, "Z") {
		t.Errorf("Ctrl+Z must not be a window-level filter outside flows (it would steal undo from the request body)")
	}
	if filtersContainKey(ui, "Y") {
		t.Errorf("Ctrl+Y must not be a window-level filter outside flows")
	}

	for _, n := range []key.Name{"S", "W", "F", key.NameReturn} {
		if !filtersContainKey(ui, n) {
			t.Errorf("Ctrl+%s must remain a window-level filter", string(n))
		}
	}

	ui.SidebarSection = "flows"
	if !filtersContainKey(ui, "Z") {
		t.Errorf("Ctrl+Z must be a window-level filter in flows section")
	}
	if !filtersContainKey(ui, "Y") {
		t.Errorf("Ctrl+Y must be a window-level filter in flows section")
	}
}
