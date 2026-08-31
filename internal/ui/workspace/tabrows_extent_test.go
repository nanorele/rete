package workspace

import "testing"

func TestTabRowsExpandCollapseKeepsCollapsedRequestHug(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatal("setup: request must collapse")
	}
	min := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.tab.splitPaneRec; !near(got, min, 4) {
		t.Fatalf("setup: collapsed pane %d, want ~%d", got, min)
	}

	rig.size.Y -= 7 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	if got := rig.tab.splitPaneRec; got > min+4 {
		t.Errorf("collapsed pane must not grow while tab rows are expanded: got %d, want <= ~%d", got, min)
	}

	rig.size.Y += 7 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	if got := rig.tab.splitPaneRec; !near(got, min, 4) {
		t.Errorf("collapsed pane must return to its hug height after tab rows collapse: got %d, want ~%d", got, min)
	}
}

func TestTabRowsExpandCollapseKeepsOpenRequestSize(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	min := rig.tab.stackedReqPaneMinPx(rig.gtx())
	ext := rig.tab.stackedSplitExtent(rig.gtx())
	rig.tab.VStackRatio = (float32(min) + 20) / ext
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	before := rig.tab.splitPaneRec
	if before <= 0 {
		t.Fatalf("setup: no rendered pane height")
	}

	rig.size.Y -= 4 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}
	rig.size.Y += 4 * 36
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	if got := rig.tab.splitPaneRec; !near(got, before, 4) {
		t.Errorf("request pane must return to its size after tab rows collapse: got %d, want ~%d", got, before)
	}
}
