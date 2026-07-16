package workspace

import "testing"

func newWSVRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("wss://example.com/socket")
	s := rig.tab.EnsureWS()
	s.OptionsExpanded = false
	return rig
}

func (rig *vstackRig) wsExtent() float32 {
	return float32(rig.size.Y - 37 - 2 - 30 - 4)
}

func (rig *vstackRig) wsPaneH() int {
	s := rig.tab.EnsureWS()
	return int(s.ComposerRatio*rig.wsExtent() + 0.5)
}

func (rig *vstackRig) wsDividerY() int {
	return rig.paneTop() + rig.wsPaneH() + 2
}

func (rig *vstackRig) wsHeadersSliderY() int {
	s := rig.tab.EnsureWS()
	return rig.paneTop() + 66 + s.headersRenderH + 2
}

func TestWSBodyCannotCoverCompose(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.wsDividerY(), rig.paneTop()+5)
	if !s.ComposeCollapsed {
		t.Errorf("dragging the WS split to the top must collapse Compose, not bury it")
	}
	if got, want := rig.wsPaneH(), s.composerMinPx(rig.gtx()); !near(got, want, 8) {
		t.Errorf("composer pane must stop at its section headers: pane %d, want ~%d", got, want)
	}
	if got := s.headersRenderH; !near(got, 120, 4) {
		t.Errorf("WS headers must keep their height when the split hits the minimum, got %d", got)
	}

	rig.drag(400, rig.wsDividerY(), rig.wsDividerY()+100)
	if s.ComposeCollapsed {
		t.Errorf("dragging the WS split back down must expand Compose")
	}
}

func TestWSHeadersComposeSlider(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := s.headersRenderH; !near(got, 120, 4) {
		t.Fatalf("setup: WS headers should render at ~120, got %d", got)
	}

	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()+50)
	if got := s.HeadersAbsHeight; !near(got, 170, 4) {
		t.Errorf("slider down should grow WS headers to ~170, got %d", got)
	}

	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()+500)
	extent := int(rig.wsExtent())
	if got := extent - rig.wsPaneH(); !near(got, 120, 6) {
		t.Errorf("over-dragging the slider must stop when WS Body hits its minimum, got body %dpx", got)
	}

	before := s.HeadersAbsHeight
	rig.drag(400, rig.wsHeadersSliderY(), rig.wsHeadersSliderY()-40)
	if got := s.HeadersAbsHeight; !near(got, before-40, 4) {
		t.Errorf("slider up must respond immediately after an over-drag: got %d, want ~%d", got, before-40)
	}
}

func TestWSMessagesCollapse(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	s.MessagesCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !s.MessagesCollapsed {
		t.Fatalf("collapse button must collapse the WS Body pane")
	}
	extent := int(rig.wsExtent())
	if got, want := extent-rig.wsPaneH(), s.msgsCollapsedMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed WS Body should hug its status row: got %d, want ~%d", got, want)
	}

	rig.drag(400, rig.wsDividerY(), rig.wsDividerY()-100)
	if s.MessagesCollapsed {
		t.Errorf("dragging the WS split up must expand the Body pane")
	}
}

func TestWSComposeCollapseButton(t *testing.T) {
	rig := newWSVRig()
	s := rig.tab.EnsureWS()
	s.ComposerRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	s.ComposeCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !s.ComposeCollapsed {
		t.Fatalf("collapse button must collapse Compose")
	}
	if got, want := rig.wsPaneH(), s.composerMinPx(rig.gtx()); !near(got, want, 6) {
		t.Errorf("collapsed Compose should shrink the composer pane to its headers: pane %d, want ~%d", got, want)
	}

	s.ComposeCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if s.ComposeCollapsed {
		t.Fatalf("second click must expand Compose")
	}
	if got := rig.wsPaneH(); got < s.composerMinPx(rig.gtx())+100 {
		t.Errorf("expanding must reopen the compose editor: pane %d", got)
	}
}
