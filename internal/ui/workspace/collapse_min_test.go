package workspace

import "testing"

func (rig *vstackRig) hbSliderScreenY() int {
	return rig.paneTop() + rig.tab.reqPaneAboveHeadersPx(rig.gtx()) + rig.tab.headersRenderH + 2
}

func TestHeadersSliderDownReachesRequestCollapsedSize(t *testing.T) {
	btn := newHSplitRig()
	btn.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	btn.tab.ReqCollapseBtn.Click()
	btn.frame()
	btn.frame()
	want := btn.tab.headersRenderH

	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()+900)

	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the headers slider past the body must collapse the request body")
	}
	if got := rig.tab.headersRenderH; got != want {
		t.Errorf("headers area at its manual maximum must match the collapsed-by-button size: got %d, want %d", got, want)
	}
}

func TestHeadersSliderUpReopensRequestBody(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 80
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()+900)
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request body should be collapsed")
	}

	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()-200)
	if rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the headers slider back up must reopen the request body")
	}
	if got, want := rig.tab.headersRenderH, rig.tab.HeadersAbsHeight; got != want {
		t.Errorf("reopened headers should render at their stored height: got %d, want %d", got, want)
	}
}

func TestSplitDragDownReachesResponseCollapsedSize(t *testing.T) {
	btn := newVStackRig()
	btn.tab.HeadersAbsHeight = 80
	btn.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	ext := btn.tab.stackedSplitExtent(btn.gtx())
	btn.tab.RespCollapseBtn.Click()
	btn.frame()
	btn.frame()
	want := int(ext) - btn.paneH()

	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+400)

	if !rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split to the bottom must collapse the response")
	}
	if got := int(ext) - rig.paneH(); !near(got, want, 1) {
		t.Errorf("response at its manual minimum must match the collapsed-by-button size: got %d, want %d", got, want)
	}
}

func TestSplitDragUpReopensResponse(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+400)
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("setup: response should be collapsed")
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()-200)
	if rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split back up must reopen the response")
	}
	gtx := rig.gtx()
	ext := int(rig.tab.stackedSplitExtent(gtx))
	if got := ext - rig.paneH(); got < 120 {
		t.Errorf("reopened response must keep its 120px minimum: got %d", got)
	}
}

func TestHeadersSliderToZeroMatchesHeadersChevron(t *testing.T) {
	btn := newVStackRig()
	btn.tab.HeadersAbsHeight = 80
	btn.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		btn.frame()
	}
	btn.tab.ViewGeneratedBtn.Click()
	btn.frame()
	btn.frame()

	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 80
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.drag(400, rig.hbSliderScreenY(), rig.hbSliderScreenY()-200)

	if rig.tab.HeadersExpanded {
		t.Errorf("dragging the headers slider to zero must collapse the headers area")
	}
	if got, want := rig.tab.stackedReqPaneMinPx(rig.gtx()), btn.tab.stackedReqPaneMinPx(btn.gtx()); got != want {
		t.Errorf("collapsed-by-drag headers must hug like collapsed-by-button: got %d, want %d", got, want)
	}
}
