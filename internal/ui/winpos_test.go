package ui

import (
	"testing"

	"tracto/internal/persist"
)

func TestWindowPosSnapshotRoundTrip(t *testing.T) {
	ui := &AppUI{}
	if got := ui.buildStateSnapshot(); got.WindowXPx != nil || got.WindowYPx != nil {
		t.Fatalf("no position seen yet: must not persist one (%v,%v)", got.WindowXPx, got.WindowYPx)
	}

	ui.winXPx, ui.winYPx, ui.winPosSet = -1720, 0, true
	state := ui.buildStateSnapshot()
	if state.WindowXPx == nil || state.WindowYPx == nil {
		t.Fatalf("position not persisted: %+v", state)
	}
	if *state.WindowXPx != -1720 || *state.WindowYPx != 0 {
		t.Errorf("got %d,%d want -1720,0", *state.WindowXPx, *state.WindowYPx)
	}

	loaded := &AppUI{}
	loaded.applyWindowState(state)
	if !loaded.winPosSet || loaded.winXPx != -1720 || loaded.winYPx != 0 {
		t.Errorf("position not restored: set=%v %d,%d", loaded.winPosSet, loaded.winXPx, loaded.winYPx)
	}

	missing := &AppUI{}
	missing.applyWindowState(persist.AppState{WindowXPx: intPtrTest(10)})
	if missing.winPosSet {
		t.Errorf("a half-written position must be ignored")
	}
}

func intPtrTest(i int) *int { return &i }

func TestRestoreWindowPosWithoutHandle(t *testing.T) {
	ui := &AppUI{winPosSet: true, winXPx: 100, winYPx: 100}
	if ui.restoreWindowPos() {
		t.Errorf("restore must fail without a native window handle so the caller falls back to centering")
	}
}
