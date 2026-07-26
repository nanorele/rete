package varpopup

import (
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/environments"
)

func manyEnvs(n int) []*environments.EnvironmentUI {
	out := make([]*environments.EnvironmentUI, 0, n)
	for i := 0; i < n; i++ {
		id := string(rune('a'+i%26)) + string(rune('0'+i/26))
		out = append(out, &environments.EnvironmentUI{
			Data: &model.ParsedEnvironment{
				ID:   id,
				Name: "env-" + id,
				Vars: []model.EnvVar{{Key: "token", Value: "tok-" + id}},
			},
		})
	}
	return out
}

func TestEnvClickDrainsForRowOutsideViewport(t *testing.T) {
	envs := manyEnvs(60)
	rg := openRig(t, envs, envs[0].Data.ID)
	rg.s.EnvMenuOpen = true
	rg.frame()

	last := len(envs)
	if len(rg.s.EnvClicks) <= last {
		t.Fatalf("len(EnvClicks) = %d, want > %d", len(rg.s.EnvClicks), last)
	}

	rg.s.EnvClicks[last].Click()
	rg.frame()

	wantID := envs[last-1].Data.ID
	if len(rg.selected) == 0 {
		t.Fatal("a click on a row scrolled out of the viewport was dropped; poll clicks before List.Layout")
	}
	if got := rg.selected[len(rg.selected)-1]; got != wantID {
		t.Errorf("selected env = %q, want %q", got, wantID)
	}
	if rg.s.EnvMenuOpen {
		t.Error("selecting an environment must close the menu")
	}
}

func TestEnvClickNoEnvironmentRow(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frame()

	rg.s.EnvClicks[0].Click()
	rg.frame()

	if len(rg.selected) == 0 {
		t.Fatal("the (no environment) row click was dropped")
	}
	if got := rg.selected[len(rg.selected)-1]; got != "" {
		t.Errorf("selected env = %q, want the empty id", got)
	}
	if rg.s.EnvID != "" {
		t.Errorf("EnvID = %q, want empty", rg.s.EnvID)
	}
}

func TestEnvClickWhileMenuClosedIsDiscarded(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = false
	rg.frame()

	rg.s.EnvClicks[1].Click()
	rg.frame()

	if len(rg.selected) != 0 {
		t.Errorf("a click while the menu is closed must not select an environment, got %v", rg.selected)
	}

	rg.s.EnvMenuOpen = true
	rg.frame()
	if len(rg.selected) != 0 {
		t.Errorf("a stranded click fired late when the menu reopened, got %v", rg.selected)
	}
}
