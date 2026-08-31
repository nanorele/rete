package workspace

import (
	"image"
	"testing"
)

func TestWSHeaderRowsMatchHTTPReference(t *testing.T) {
	rig := newVStackRig()
	rig.size = image.Pt(1400, 700)
	rig.tab.LayoutMode = LayoutModeHoriz
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	ref := rig.tab.headersRowPx(rig.gtx())

	rig.tab.Method = MethodWS
	rig.tab.URLInput.SetText("ws://example.com/socket")
	s := rig.tab.EnsureWS()
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if s.wsRowH != ref {
		t.Errorf("WS section header %dpx != HTTP reference %dpx", s.wsRowH, ref)
	}
	if s.statusRowH != ref {
		t.Errorf("WS status row %dpx != HTTP reference %dpx", s.statusRowH, ref)
	}
}

func TestGQLHeaderRowsMatchHTTPReference(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 700))
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.tab.Status = "Ready"
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	ref := rig.tab.headersRowPx(rig.gtx())
	if rig.tab.respHeaderH != ref {
		t.Errorf("GraphQL response header %dpx != reference %dpx", rig.tab.respHeaderH, ref)
	}
}

func TestGQLSplitRecordsMeasuredAnchors(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 700))
	rig.tab.LayoutMode = LayoutModeHoriz
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.splitPaneRec <= 0 || rig.tab.splitRespRec <= 0 {
		t.Fatalf("GraphQL panes not recorded: pane=%d resp=%d", rig.tab.splitPaneRec, rig.tab.splitRespRec)
	}
	if rig.tab.PaneDrawnH <= 0 {
		t.Fatalf("PaneDrawnH not recorded for GraphQL: %d", rig.tab.PaneDrawnH)
	}
}

func TestGQLHeadersResizableViaDrag(t *testing.T) {
	rig := newGQLRig(image.Pt(1400, 800))
	rig.tab.LayoutMode = LayoutModeHoriz
	rig.tab.HeadersExpanded = true
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	before := rig.tab.headersRenderH
	if before <= 0 {
		t.Fatal("headers area not rendered")
	}
	moved := false
	for y := 100; y < 700 && !moved; y++ {
		rig.drag(400, y, y+40)
		if rig.tab.headersRenderH != before {
			moved = true
		}
	}
	if !moved {
		t.Fatal("dragging never resized the GraphQL headers area")
	}
	if rig.tab.headersRenderH <= before {
		t.Errorf("headers area %dpx after dragging down, want more than %dpx", rig.tab.headersRenderH, before)
	}
}
