package workspace

import (
	"testing"

	"tracto/internal/model"
)

func TestRespCollapseNoJitterOnDrag(t *testing.T) {
	rig := newVStackRig()
	rig.tab.BodyType = model.BodyNone
	rig.tab.HeadersAbsHeight = 32
	rig.tab.VStackRatio = 0.5
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	if rig.tab.RespBodyCollapsed || rig.tab.ReqBodyCollapsed {
		t.Fatalf("setup: both panes must start expanded")
	}

	gtx := rig.gtx()
	ext := int(rig.tab.stackedSplitExtent(gtx))
	hug := rig.tab.respCollapsedMinPx(gtx)

	y0 := rig.splitDividerY()
	rig.r.Queue(pointerPress(400, y0))
	rig.frame()

	flips := 0
	prev := rig.tab.RespBodyCollapsed
	prevResp := ext - rig.paneH()
	for i := 1; i <= 40; i++ {
		rig.r.Queue(pointerMove(400, y0+i*4))
		rig.frame()
		if rig.tab.RespBodyCollapsed != prev {
			flips++
			prev = rig.tab.RespBodyCollapsed
		}
		resp := ext - rig.paneH()
		if resp > prevResp {
			t.Errorf("step %d: response grew while shrinking the pane: %d -> %d", i, prevResp, resp)
		}
		if rig.tab.RespBodyCollapsed && !near(resp, hug, 4) {
			t.Errorf("step %d: collapsed response must hug its header: got %d, want ~%d", i, resp, hug)
		}
		prevResp = resp
	}
	if flips != 1 {
		t.Errorf("shrinking drag must toggle the response collapse once, got %d toggles", flips)
	}
	if !rig.tab.RespBodyCollapsed {
		t.Fatalf("dragging past the response minimum must collapse it")
	}

	y1 := y0 + 160
	flips = 0
	prev = rig.tab.RespBodyCollapsed
	for i := 1; i <= 40; i++ {
		rig.r.Queue(pointerMove(400, y1-i*4))
		rig.frame()
		if rig.tab.RespBodyCollapsed != prev {
			flips++
			prev = rig.tab.RespBodyCollapsed
		}
		resp := ext - rig.paneH()
		if resp < prevResp {
			t.Errorf("back %d: response shrank while growing the pane: %d -> %d", i, prevResp, resp)
		}
		prevResp = resp
	}
	rig.r.Queue(pointerRelease(400, y1-160))
	rig.frame()
	if flips != 1 {
		t.Errorf("growing drag must toggle the response collapse once, got %d toggles", flips)
	}
	if rig.tab.RespBodyCollapsed {
		t.Fatalf("dragging back must reopen the response body")
	}
}
