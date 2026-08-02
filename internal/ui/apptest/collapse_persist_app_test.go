package apptest

import (
	. "tracto/internal/ui"

	"testing"

	"tracto/internal/persist"
	"tracto/internal/ui/workspace"
)

// savedTab writes the app state the way a real shutdown would and reads back the
// single tab it persisted.
func savedTab(t *testing.T, ui *AppUI) persist.TabState {
	t.Helper()
	if !ui.SaveNeeded() {
		t.Fatal("toggling a pane did not schedule a state save")
	}
	ui.SaveStateSync()
	ui.WaitBackgroundSaves()
	state := persist.Load()
	if len(state.Tabs) != 1 {
		t.Fatalf("persisted tabs = %d, want 1", len(state.Tabs))
	}
	return state.Tabs[0]
}

func TestHTTPPaneCollapsePersistsInBothLayouts(t *testing.T) {
	for _, lm := range []struct {
		name string
		mode int
	}{
		{"vertical", workspace.LayoutModeVert},
		{"horizontal", workspace.LayoutModeHoriz},
	} {
		t.Run(lm.name, func(t *testing.T) {
			rig := newAppSliderRig(t)
			tab := rig.ui.Tabs[0]
			tab.LayoutMode = lm.mode
			for i := 0; i < 4; i++ {
				rig.frame()
			}
			rig.ui.SetSaveNeeded(false)

			tab.ReqCollapseBtn.Click()
			tab.RespCollapseBtn.Click()
			tab.ViewGeneratedBtn.Click()
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if !tab.ReqBodyCollapsed || !tab.RespBodyCollapsed || tab.HeadersExpanded {
				t.Fatalf("panes did not collapse: req=%v resp=%v headersExpanded=%v",
					tab.ReqBodyCollapsed, tab.RespBodyCollapsed, tab.HeadersExpanded)
			}

			ts := savedTab(t, rig.ui)
			if !ts.ReqCollapsed || !ts.RespCollapsed || ts.HeadersExpanded {
				t.Fatalf("saved state lost the collapse flags: req=%v resp=%v headers=%v",
					ts.ReqCollapsed, ts.RespCollapsed, ts.HeadersExpanded)
			}

			restored := workspace.TabFromState(ts)
			if !restored.ReqBodyCollapsed || !restored.RespBodyCollapsed || restored.HeadersExpanded {
				t.Fatalf("restart reopened the panes: req=%v resp=%v headers=%v",
					restored.ReqBodyCollapsed, restored.RespBodyCollapsed, restored.HeadersExpanded)
			}

			rig.ui.SetSaveNeeded(false)
			tab.ReqCollapseBtn.Click()
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			if tab.ReqBodyCollapsed {
				t.Fatal("second click must expand the request pane again")
			}
			if ts := savedTab(t, rig.ui); ts.ReqCollapsed {
				t.Error("expanding the request pane was not persisted")
			}
		})
	}
}

func TestWSPaneCollapsePersists(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	tab.Method = workspace.MethodWS
	tab.URLInput.SetText("wss://echo.test/ws")
	tab.EnsureWS()
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	rig.ui.SetSaveNeeded(false)

	tab.WS.ComposeCollapseBtn.Click()
	tab.WS.MessagesCollapseBtn.Click()
	tab.WS.HeadersCollapseBtn.Click()
	tab.WS.OptionsBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	s := tab.WS
	if !s.ComposeCollapsed || !s.MessagesCollapsed || !s.HeadersCollapsed || !s.OptionsExpanded {
		t.Fatalf("ws panes did not toggle: compose=%v messages=%v headers=%v options=%v",
			s.ComposeCollapsed, s.MessagesCollapsed, s.HeadersCollapsed, s.OptionsExpanded)
	}

	ts := savedTab(t, rig.ui)
	if ts.WS == nil {
		t.Fatal("ws state not persisted")
	}
	if !ts.WS.ComposeCollapsed || !ts.WS.MessagesCollapsed || !ts.WS.HeadersCollapsed || !ts.WS.OptionsExpanded {
		t.Fatalf("saved ws state lost the collapse flags: %+v", ts.WS)
	}

	restored := workspace.TabFromState(ts).WS
	if restored == nil {
		t.Fatal("ws session not restored")
	}
	if !restored.ComposeCollapsed || !restored.MessagesCollapsed || !restored.HeadersCollapsed || !restored.OptionsExpanded {
		t.Errorf("restart reopened the ws panes: %+v", restored)
	}
}
