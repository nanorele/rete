package workspace

import "testing"

func TestCollapsedHeadersRowKeepsExpandedHeight(t *testing.T) {
	rig := newHSplitRig()
	rig.tab.ReqBodyCollapsed = true
	rig.tab.HeadersExpanded = false
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	line := 1
	want := rig.tab.headersRowH + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed headers pane must hold only its two header rows: box %d, want %d", got, want)
	}
}

func TestCollapsedStackedRequestHugsHeaderRow(t *testing.T) {
	rig := newVStackRig()
	rig.tab.HeadersAbsHeight = 100
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	rig.tab.ReqCollapseBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	line, slider := 1, 4
	want := rig.tab.headersRowH + line + rig.tab.headersRenderH + slider + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed stacked request pane must hug its content: box %d, want %d", got, want)
	}

	rig.tab.ViewGeneratedBtn.Click()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	want = rig.tab.headersRowH + line + rig.tab.reqHeaderH
	if got := rig.tab.reqPaneBoxH; got != want {
		t.Errorf("collapsed request pane with hidden headers must hold only header rows: box %d, want %d", got, want)
	}
}
