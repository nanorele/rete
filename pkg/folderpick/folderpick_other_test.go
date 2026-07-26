//go:build !windows

package folderpick

import "testing"

func TestPickFolderDialogStub(t *testing.T) {
	cases := []struct {
		name  string
		title string
	}{
		{"empty title", ""},
		{"plain title", "Choose a folder"},
		{"unicode title", "Выберите папку"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, ok := PickFolderDialog(c.title)
			if ok {
				t.Errorf("PickFolderDialog(%q) ok = true, want false on non-Windows", c.title)
			}
			if path != "" {
				t.Errorf("PickFolderDialog(%q) path = %q, want %q", c.title, path, "")
			}
		})
	}
}
