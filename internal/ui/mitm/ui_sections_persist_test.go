package mitm

import (
	"image"
	"testing"
)

func TestSidebarSectionDefaultsOnFirstRun(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	if !rig.s.SecTargetsOpen || !rig.s.SecTLSOpen {
		t.Errorf("Targets and TLS must start expanded: targets=%v tls=%v", rig.s.SecTargetsOpen, rig.s.SecTLSOpen)
	}
	if rig.s.SecIRulesOpen || rig.s.SecMROpen || rig.s.SecScopeOpen {
		t.Errorf("the remaining sections must start collapsed: irules=%v mr=%v scope=%v",
			rig.s.SecIRulesOpen, rig.s.SecMROpen, rig.s.SecScopeOpen)
	}
}

func TestSidebarSectionStatePersistsAcrossRestart(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.sidebarFrames(2)
	rig.s.Dirty()

	rig.s.SecTargetsHdr.Click()
	rig.s.SecScopeHdr.Click()
	rig.s.InspectorToggle.Click()
	rig.sidebarFrames(2)
	rig.frames(2)

	if rig.s.SecTargetsOpen || !rig.s.SecScopeOpen {
		t.Fatalf("toggles did not land: targets=%v scope=%v", rig.s.SecTargetsOpen, rig.s.SecScopeOpen)
	}
	if !rig.s.InspectorCollapsed {
		t.Fatal("the inspector toggle did not collapse the inspector")
	}
	// The frames above already ran flushConfig, which consumes the dirty flag
	// and leaves a debounced save pending.
	if !rig.s.savePending {
		t.Fatal("toggling sections must schedule a config save")
	}
	if err := SaveConfig(rig.s.SnapshotConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	restarted := &UIState{}
	restarted.Ensure()
	t.Cleanup(func() {
		if restarted.Proxy != nil && restarted.Proxy.Running() {
			restarted.Proxy.Stop()
		}
	})
	if restarted.SecTargetsOpen {
		t.Error("a collapsed Targets section reopened after restart")
	}
	if !restarted.SecScopeOpen {
		t.Error("an expanded Scope section collapsed after restart")
	}
	if !restarted.SecTLSOpen {
		t.Error("the untouched TLS section lost its expanded state")
	}
	if !restarted.InspectorCollapsed {
		t.Error("the collapsed inspector reopened after restart")
	}
}

func TestSidebarSectionStoredFalseBeatsDefault(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	rig.s.SecTargetsOpen = false
	rig.s.SecTLSOpen = false
	if err := SaveConfig(rig.s.SnapshotConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	restarted := &UIState{}
	restarted.Ensure()
	t.Cleanup(func() {
		if restarted.Proxy != nil && restarted.Proxy.Running() {
			restarted.Proxy.Stop()
		}
	})
	if restarted.SecTargetsOpen || restarted.SecTLSOpen {
		t.Errorf("stored collapsed sections must beat the first-run defaults: targets=%v tls=%v",
			restarted.SecTargetsOpen, restarted.SecTLSOpen)
	}
}
