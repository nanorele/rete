package workspace

import (
	"strings"
	"testing"

	"tracto/internal/persist"

	"github.com/uorg-saver/easyjson"
)

func marshalTab(t *testing.T, rt *RequestTab) persist.TabState {
	t.Helper()
	state := persist.AppState{Tabs: []persist.TabState{StateFromTab(rt)}}
	data, err := persist.MarshalIndentEasy(&state, "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back persist.AppState
	if err := easyjson.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, data)
	}
	if len(back.Tabs) != 1 {
		t.Fatalf("tabs = %d, want 1", len(back.Tabs))
	}
	return back.Tabs[0]
}

func TestHTTPCollapseStateSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Orders")
	src.Method = "POST"
	src.HeadersExpanded = true
	src.ReqBodyCollapsed = true
	src.RespBodyCollapsed = true
	src.reqRatioSaved = 0.44
	src.respRatioSaved = 0.71

	ts := marshalTab(t, src)
	if !ts.ReqCollapsed || !ts.RespCollapsed {
		t.Fatalf("collapse flags missing from the saved state: %+v", ts)
	}

	dst := TabFromState(ts)
	if !dst.HeadersExpanded {
		t.Error("headers section expansion lost")
	}
	if !dst.ReqBodyCollapsed {
		t.Error("collapsed request pane reopened after restart")
	}
	if !dst.RespBodyCollapsed {
		t.Error("collapsed response pane reopened after restart")
	}
	if dst.reqRatioSaved != 0.44 || dst.respRatioSaved != 0.71 {
		t.Errorf("saved reopen ratios = %v/%v, want 0.44/0.71", dst.reqRatioSaved, dst.respRatioSaved)
	}
}

func TestHTTPCollapsedHeadersSectionSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Orders")
	src.HeadersExpanded = false
	ts := marshalTab(t, src)
	if strings.Contains(string(mustJSON(t, &ts)), `"headers_expanded"`) {
		t.Error("a collapsed headers section must serialize as the omitted default")
	}
	if TabFromState(ts).HeadersExpanded {
		t.Error("collapsed headers section reopened after restart")
	}
}

func mustJSON(t *testing.T, ts *persist.TabState) []byte {
	t.Helper()
	data, err := easyjson.Marshal(ts)
	if err != nil {
		t.Fatalf("marshal tab state: %v", err)
	}
	return data
}

func TestWSCollapseStateSurvivesRestart(t *testing.T) {
	src := NewRequestTab("Socket")
	src.Method = MethodWS
	s := src.EnsureWS()
	s.OptionsExpanded = true
	s.HeadersCollapsed = true
	s.ComposeCollapsed = true
	s.MessagesCollapsed = true
	s.composeSavedRatio = 0.38
	s.msgsSavedRatio = 0.62

	ts := marshalTab(t, src)
	if ts.WS == nil {
		t.Fatal("WS state missing")
	}
	if !ts.WS.HeadersCollapsed || !ts.WS.ComposeCollapsed || !ts.WS.MessagesCollapsed {
		t.Fatalf("WS collapse flags missing from the saved state: %+v", ts.WS)
	}

	d := TabFromState(ts).WS
	if d == nil {
		t.Fatal("WS session not restored")
	}
	if !d.OptionsExpanded || !d.HeadersCollapsed || !d.ComposeCollapsed || !d.MessagesCollapsed {
		t.Errorf("WS collapse state lost: options=%v headers=%v compose=%v messages=%v",
			d.OptionsExpanded, d.HeadersCollapsed, d.ComposeCollapsed, d.MessagesCollapsed)
	}
	if d.composeSavedRatio != 0.38 || d.msgsSavedRatio != 0.62 {
		t.Errorf("saved WS reopen ratios = %v/%v, want 0.38/0.62", d.composeSavedRatio, d.msgsSavedRatio)
	}
}

func TestLayoutPrefsCarryCollapseState(t *testing.T) {
	src := NewRequestTab("src")
	src.Method = MethodWS
	src.ReqBodyCollapsed = true
	src.RespBodyCollapsed = true
	ws := src.EnsureWS()
	ws.HeadersCollapsed = true
	ws.ComposeCollapsed = true
	ws.MessagesCollapsed = true

	var p LayoutPrefs
	src.MergeLayoutPrefs(&p)

	dst := NewRequestTab("dst")
	dst.Method = MethodWS
	dst.EnsureWS()
	dst.ApplyLayoutPrefs(p)

	if !dst.ReqBodyCollapsed || !dst.RespBodyCollapsed {
		t.Errorf("http collapse state not shared: req=%v resp=%v", dst.ReqBodyCollapsed, dst.RespBodyCollapsed)
	}
	if !dst.WS.HeadersCollapsed || !dst.WS.ComposeCollapsed || !dst.WS.MessagesCollapsed {
		t.Errorf("ws collapse state not shared: %+v", dst.WS)
	}
}

func TestCollapseTogglesRequestASave(t *testing.T) {
	rt := NewRequestTab("t")
	if rt.TakeLayoutSaveRequest() {
		t.Fatal("a fresh tab must not request a save")
	}
	rt.layoutSaveNeeded = true
	if !rt.TakeLayoutSaveRequest() {
		t.Fatal("a pending layout change must request a save")
	}
	if rt.TakeLayoutSaveRequest() {
		t.Error("the request must be cleared once taken")
	}
}
