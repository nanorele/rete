package workspace

import (
	"image"
	"strconv"
	"testing"
)

func newHSplitRig() *vstackRig {
	rig := newVStackRig()
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.size = image.Pt(1200, 600)
	return rig
}

func TestSubTabSwitchFitsHeadersAreaExactly(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneBefore := rig.paneH()

	rig.tab.AuthTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != 100 {
		t.Errorf("switching to Auth must fit the area to the auth panel: got %d, want 100", got)
	}
	if got := rig.paneH(); !near(got, paneBefore-20, 3) {
		t.Errorf("request pane must follow the fitted headers area: pane %d, want ~%d", got, paneBefore-20)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != 32 {
		t.Errorf("switching to empty Cookies must shrink the area to one row: got %d, want 32", got)
	}
	if got := rig.paneH(); !near(got, paneBefore-88, 3) {
		t.Errorf("request pane must shrink back after Auth: pane %d, want ~%d", got, paneBefore-88)
	}

	rig.tab.HeadersTabBtn.Click()
	rig.frame()
	rig.frame()
	wantHeaders := len(rig.tab.Headers)*28 + 4
	if got := rig.tab.HeadersAbsHeight; got != wantHeaders {
		t.Errorf("switching to Headers must fit its rows: got %d, want %d", got, wantHeaders)
	}
	if got := rig.paneH(); !near(got, paneBefore+wantHeaders-120, 3) {
		t.Errorf("request pane must track the headers fit: pane %d, want ~%d", got, paneBefore+wantHeaders-120)
	}
}

func TestSubTabSwitchKeepsManualHeadersHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()+40)
	manual := rig.tab.HeadersAbsHeight
	if !near(manual, 160, 4) {
		t.Fatalf("setup: manual resize should land at ~160, got %d", manual)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual {
		t.Errorf("after a manual resize, tab switching must not shrink the area: got %d, want %d", got, manual)
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-80)
	manual2 := rig.tab.HeadersAbsHeight
	rig.tab.AuthTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got < 100 {
		t.Errorf("area must still grow to fit the Auth panel: got %d, want >= 100", got)
	}

	rig.tab.CookiesTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual2 {
		t.Errorf("leaving Auth must return to the manual height: got %d, want %d", got, manual2)
	}
}

func TestReqCollapseKeepsHeadersRenderHeight(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	before := rig.tab.headersRenderH
	if before != 120 {
		t.Fatalf("setup: headers should render at 120, got %d", before)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; got != before {
		t.Errorf("collapsing request must not squeeze headers: %d -> %d", before, got)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; got != before {
		t.Errorf("expanding request must not change headers: %d -> %d", before, got)
	}
}

func TestManualResizeSurvivesTabRoundTrip(t *testing.T) {
	rig := newVStackRig()
	for i := 0; i < 15; i++ {
		rig.tab.AddHeader("K"+strconv.Itoa(i), "v")
	}
	rig.tab.HeadersAbsHeight = 424
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.drag(400, rig.headersSliderY(), rig.headersSliderY()-100)
	manual := rig.tab.HeadersAbsHeight
	if manual >= rig.tab.headersFitDp(rig.tab.Headers) {
		t.Fatalf("setup: manual resize must land below the content fit, got %d", manual)
	}

	rig.tab.ParamsTabBtn.Click()
	rig.frame()
	rig.frame()
	rig.tab.HeadersTabBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.HeadersAbsHeight; got != manual {
		t.Errorf("tab round trip must keep the manual height: got %d, want %d", got, manual)
	}
	if got := rig.tab.headersRenderH; !near(got, manual, 2) {
		t.Errorf("rendered headers must stay at the manual height: got %d, want ~%d", got, manual)
	}
}

func TestCollapsedRequestAnchorsToBottomHorizontal(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.prevStacked {
		t.Fatalf("setup: rig must lay out horizontally")
	}
	if got := rig.tab.headersRenderH; !near(got, 120, 3) {
		t.Fatalf("setup: headers should render at stored height, got %d", got)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if !rig.tab.ReqBodyCollapsed {
		t.Fatalf("collapse button must collapse the request body")
	}
	gtx := rig.gtx()
	want := rig.tab.reqPaneH - rig.tab.reqPaneAboveHeadersPx(gtx) - rig.tab.reqPaneBelowHeadersPx(gtx)
	if got := rig.tab.headersRenderH; !near(got, want, 5) {
		t.Errorf("collapsed request must give its space to the headers area: got %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.headersRenderH; !near(got, 120, 3) {
		t.Errorf("expanding must restore the stored headers height: got %d", got)
	}
}

func TestBothCollapsedCompactBoxHorizontal(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	full := rig.tab.reqPaneBoxH

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.tab.reqPaneBoxH; got != full {
		t.Errorf("with headers expanded the collapsed request pane must keep full height: got %d, want %d", got, full)
	}

	rig.tab.HeadersExpanded = false
	rig.frame()
	rig.frame()
	gtx := rig.gtx()
	compact := rig.tab.headersRowPx(gtx) + rig.tab.reqPaneBelowHeadersContentPx(gtx)
	if got := rig.tab.reqPaneBoxH; !near(got, compact, 3) {
		t.Errorf("hiding headers must shrink the request pane box to its rows: got %d, want ~%d (full %d)", got, compact, full)
	}

	rig.tab.HeadersExpanded = true
	rig.frame()
	rig.frame()
	if got := rig.tab.reqPaneBoxH; got != full {
		t.Errorf("re-expanding headers must restore the full pane box: got %d, want %d", got, full)
	}
}

func TestReqCollapseCarriesIntoVerticalLayout(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	want := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, want, 4) {
		t.Errorf("collapsed request must stay visually collapsed after switching layouts: pane %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); got < want+100 {
		t.Errorf("expanding after the layout switch must reopen editor space: pane %d, want >= %d", got, want+100)
	}
}

func TestCollapseSequencesKeepPanesHugged(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneStart := rig.paneH()
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	reqMin := rig.tab.stackedReqPaneMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, reqMin, 4) {
		t.Fatalf("setup: collapsed request should hug: %d, want ~%d", got, reqMin)
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, reqMin, 4) {
		t.Errorf("collapsing response must not inflate the collapsed request pane: %d, want ~%d", got, reqMin)
	}
	if got, want := rig.tab.respPaneBoxH, rig.tab.respCollapsedMinPx(rig.gtx()); !near(got, want, 4) {
		t.Errorf("collapsed response must hug its header: %d, want ~%d", got, want)
	}

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	wantFill := extent - rig.tab.respCollapsedMinPx(rig.gtx())
	if got := rig.paneH(); !near(got, wantFill, 5) {
		t.Errorf("expanding request while response is collapsed must fill the space: %d, want ~%d", got, wantFill)
	}

	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, paneStart, 4) {
		t.Errorf("expanding response must restore the original split: %d, want ~%d", got, paneStart)
	}
}

func TestHeadersToggleWhileAllCollapsedRehugs(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()
	hugged := rig.paneH()
	if want := rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(hugged, want, 4) {
		t.Fatalf("setup: collapsed request should hug: %d, want ~%d", hugged, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	opened := rig.paneH()
	if want := rig.tab.stackedReqPaneMinPx(rig.gtx()); !near(opened, want, 4) {
		t.Errorf("opening headers must grow the pane exactly to the headers min: %d, want ~%d", opened, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	rig.frame()
	rig.frame()
	if got := rig.paneH(); !near(got, hugged, 4) {
		t.Errorf("hiding headers must re-hug the collapsed request pane: %d, want ~%d", got, hugged)
	}
}

func TestExpandInHorizontalThenBackToVertical(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersExpanded = false
	rig.tab.HeadersAbsHeight = 120
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	paneStart := rig.paneH()

	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeHoriz
	rig.size = image.Pt(1200, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.ReqCollapseBtn.Click()
	rig.frame()
	rig.frame()
	if rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: request must be expanded in horizontal")
	}

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if got := rig.paneH(); !near(got, paneStart, 4) {
		t.Errorf("expanded request must get its split back after returning to vertical: %d, want ~%d", got, paneStart)
	}
}

func TestRespCollapseCarriesIntoVerticalLayout(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.HeadersAbsHeight = 120
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	rig.tab.RespCollapseBtn.Click()
	rig.frame()
	rig.frame()

	rig.tab.LayoutMode = LayoutModeVert
	rig.size = image.Pt(800, 600)
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	extent := int(rig.tab.stackedSplitExtent(rig.gtx()))
	respPx := extent - rig.paneH()
	if want := rig.tab.respCollapsedMinPx(rig.gtx()); !near(respPx, want, 4) {
		t.Errorf("collapsed response must stay collapsed after switching layouts: got %d, want ~%d", respPx, want)
	}
}
