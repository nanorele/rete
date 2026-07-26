package settings

import (
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/theme"
)

func TestApplyPropagatesCompactMenus(t *testing.T) {
	old := theme.CompactMenus
	t.Cleanup(func() { theme.CompactMenus = old })

	s := model.DefaultSettings()
	if s.CompactMenus {
		t.Error("menu padding should be kept by default")
	}

	s.CompactMenus = true
	Apply(nil, s)
	if !theme.CompactMenus {
		t.Error("Apply should enable theme.CompactMenus")
	}

	s.CompactMenus = false
	Apply(nil, s)
	if theme.CompactMenus {
		t.Error("Apply should disable theme.CompactMenus")
	}
}

func TestEditorCompactMenusRoundTrip(t *testing.T) {
	cur := model.DefaultSettings()
	cur.CompactMenus = true
	e := NewEditor(cur)
	if !e.CompactMenus.Value {
		t.Error("NewEditor should seed CompactMenus from current settings")
	}

	e.CompactMenus.Value = false
	host := &Host{Current: &cur, OnSave: func() {}}
	e.Apply(host)
	if cur.CompactMenus {
		t.Error("Apply should write the switch value back into settings")
	}

	e.Reset()
	if e.CompactMenus.Value != model.DefaultSettings().CompactMenus {
		t.Error("Reset should restore the default menu padding")
	}
}
