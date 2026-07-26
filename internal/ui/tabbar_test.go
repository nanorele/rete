package ui

import (
	"testing"

	"tracto/internal/ui/tabbar"
	"tracto/internal/ui/workspace"
)

func newTabsUI(titles ...string) *AppUI {
	ui := &AppUI{TabBar: tabbar.NewStrip()}
	for _, t := range titles {
		ui.Tabs = append(ui.Tabs, workspace.NewRequestTab(t))
	}
	return ui
}

func tabTitles(ui *AppUI) []string {
	out := make([]string, 0, len(ui.Tabs))
	for _, t := range ui.Tabs {
		out = append(out, t.Title)
	}
	return out
}

func TestCloseTab_OutOfRange(t *testing.T) {
	for _, idx := range []int{-1, 3, 99} {
		ui := newTabsUI("a", "b", "c")
		ui.ActiveIdx = 1
		ui.closeTab(idx)
		if len(ui.Tabs) != 3 {
			t.Errorf("closeTab(%d) removed a tab, want no-op", idx)
		}
		if ui.ActiveIdx != 1 {
			t.Errorf("closeTab(%d) ActiveIdx = %d, want 1", idx, ui.ActiveIdx)
		}
	}
}

func TestCloseTab_ActiveIdx(t *testing.T) {
	cases := []struct {
		name     string
		titles   []string
		active   int
		close    int
		wantTabs []string
		wantIdx  int
	}{
		{"last tab remaining", []string{"a"}, 0, 0, []string{}, -1},
		{"close before active", []string{"a", "b", "c"}, 2, 0, []string{"b", "c"}, 1},
		{"close active middle", []string{"a", "b", "c"}, 1, 1, []string{"a", "c"}, 0},
		{"close active first", []string{"a", "b", "c"}, 0, 0, []string{"b", "c"}, 0},
		{"close active last", []string{"a", "b", "c"}, 2, 2, []string{"a", "b"}, 1},
		{"close after active", []string{"a", "b", "c"}, 0, 2, []string{"a", "b"}, 0},
		{"close after active mid", []string{"a", "b", "c"}, 1, 2, []string{"a", "b"}, 1},
		{"two tabs close first active second", []string{"a", "b"}, 1, 0, []string{"b"}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ui := newTabsUI(c.titles...)
			ui.ActiveIdx = c.active
			ui.closeTab(c.close)

			got := tabTitles(ui)
			if len(got) != len(c.wantTabs) {
				t.Fatalf("tabs = %v, want %v", got, c.wantTabs)
			}
			for i := range got {
				if got[i] != c.wantTabs[i] {
					t.Fatalf("tabs = %v, want %v", got, c.wantTabs)
				}
			}
			if ui.ActiveIdx != c.wantIdx {
				t.Errorf("ActiveIdx = %d, want %d", ui.ActiveIdx, c.wantIdx)
			}
		})
	}
}

func TestCloseTab_ActiveIdxAlwaysValid(t *testing.T) {
	for active := 0; active < 4; active++ {
		for closeIdx := 0; closeIdx < 4; closeIdx++ {
			ui := newTabsUI("a", "b", "c", "d")
			ui.ActiveIdx = active
			ui.closeTab(closeIdx)
			if len(ui.Tabs) == 0 {
				continue
			}
			if ui.ActiveIdx < 0 || ui.ActiveIdx >= len(ui.Tabs) {
				t.Errorf("active=%d close=%d: ActiveIdx = %d out of range (len=%d)",
					active, closeIdx, ui.ActiveIdx, len(ui.Tabs))
			}
		}
	}
}

func TestCloseTab_MarksSaveNeeded(t *testing.T) {
	ui := newTabsUI("a", "b")
	ui.ActiveIdx = 0
	ui.closeTab(1)
	if !ui.saveNeeded {
		t.Error("closeTab did not mark state save needed")
	}
}
