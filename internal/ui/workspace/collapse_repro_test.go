package workspace

import "testing"

func TestReqBodyCollapseByDragAndButton(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request body must start expanded")
	}

	rig.drag(400, rig.splitDividerY(), rig.paneTop()+10)
	if !rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the split to the request minimum must flip the collapse state")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed request pane should hug headers+request header: pane %d, want ~%d", got, want)
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()+120)
	if rig.tab.ReqBodyCollapsed {
		t.Errorf("dragging the split back down must expand the request body")
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("collapse button must collapse the request body")
	}
	if got, want := rig.paneH(), rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapse button should shrink the pane to its header: pane %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("second click must expand the request body")
	}
	minOpen := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); got < minOpen+100 {
		t.Errorf("expanding must reopen editor space: pane %d, want >= %d", got, minOpen+100)
	}
}

func TestRespBodyCollapseButtonAndDragExpand(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("collapse button must collapse the response body")
	}
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))
	respPx := extent - rig.paneH()
	if want := rig.tab.respCollapsedMinPx(rig.gtx()); !near(respPx, want, 4) {
		t.Errorf("collapsed response pane should hug its header: got %d, want ~%d", respPx, want)
	}

	rig.drag(400, rig.splitDividerY(), rig.splitDividerY()-100)
	if rig.tab.RespBodyCollapsed {
		t.Errorf("dragging the split up must expand the response body")
	}
	respPx = extent - rig.paneH()
	if respPx < 100 {
		t.Errorf("response should reopen to at least its drag minimum, got %dpx", respPx)
	}
}
