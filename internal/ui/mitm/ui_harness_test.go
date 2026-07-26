package mitm

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type uiRig struct {
	s    *UIState
	host *Host
	r    input.Router
	sz   image.Point
	now  time.Time
}

func newUIRig(t *testing.T, sz image.Point) *uiRig {
	t.Helper()
	setupTestConfigDir(t)
	rig := &uiRig{
		s:    &UIState{},
		host: &Host{Theme: material.NewTheme(), Window: new(app.Window)},
		sz:   sz,
		now:  time.Unix(1700000000, 0),
	}
	rig.s.Ensure()
	t.Cleanup(func() {
		if rig.s.Proxy != nil && rig.s.Proxy.Running() {
			rig.s.Proxy.Stop()
		}
	})
	return rig
}

func (rig *uiRig) gtx() layout.Context {
	rig.now = rig.now.Add(16 * time.Millisecond)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
}

func (rig *uiRig) frame() layout.Dimensions {
	gtx := rig.gtx()
	dims := rig.s.Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *uiRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *uiRig) sidebarFrame() layout.Dimensions {
	gtx := rig.gtx()
	gtx.Constraints = layout.Exact(image.Pt(300, rig.sz.Y))
	dims := rig.s.LayoutSidebar(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *uiRig) sidebarFrames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.sidebarFrame()
	}
	return d
}

func (rig *uiRig) press(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *uiRig) move(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *uiRig) release(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frames(2)
}

func seedFlows(s *UIState) {
	base := time.Unix(1700000000, 0)
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Src: SrcForward, ClientAddr: "127.0.0.1:5511",
		Scheme: "https", Method: "GET", Host: "api.example.com", Port: "443",
		Path: "/v1/users?page=2&sort=name", URL: "https://api.example.com/v1/users?page=2&sort=name",
		Version: "HTTP/1.1",
		ReqHeaders: [][2]string{
			{"Host", "api.example.com"},
			{"Cookie", "sid=abc; theme=dark"},
			{"Content-Type", "application/x-www-form-urlencoded"},
		},
		ReqBody: []byte("a=1&b=2"), ReqSize: 7,
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "application/json"}, {"Set-Cookie", "sid=xyz; Path=/; HttpOnly"}},
		RespBody:    []byte(`{"users":[{"id":1,"name":"a"}]}`), RespSize: 31,
		Started: base, Ended: base.Add(120 * time.Millisecond),
		Highlight: "red", Comment: "interesting",
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Src: SrcReverse, TargetDomain: "shop.example.com",
		Scheme: "http", Method: "POST", Host: "shop.example.com", Port: "80",
		Path: "/checkout", URL: "http://shop.example.com/checkout",
		ReqHeaders: [][2]string{{"Content-Type", "application/json"}},
		ReqBody:    []byte(`{"cart":1}`),
		Status:     "500 Internal Server Error", StatusCode: 500,
		RespHeaders: [][2]string{{"Content-Type", "text/html"}},
		RespBody:    []byte("<html><body>Server <b>error</b></body></html>"),
		Started:     base.Add(time.Second), Ended: base.Add(time.Second + 3*time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "cdn.example.com",
		Path: "/assets/app.css", URL: "https://cdn.example.com/assets/app.css",
		StatusCode: 304, Status: "304 Not Modified",
		Started:    base.Add(2 * time.Second), Ended: base.Add(2*time.Second + 5*time.Millisecond),
	})
	s.Store.Add(&Flow{
		Kind: FlowTunnel, Method: "CONNECT", Host: "secure.example.com", Port: "443",
		BytesIn: 4096, BytesOut: 2048, Started: base.Add(3 * time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "ws.example.com", Path: "/socket",
		URL: "https://ws.example.com/socket", WebSocket: true,
		StatusCode: 101, Status: "101 Switching Protocols",
		Started:    base.Add(4 * time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "broken.example.com", Path: "/x",
		Error: "dial tcp: connection refused", Started: base.Add(5 * time.Second),
	})

	flows := s.Store.Snapshot()
	wsID := flows[4].ID
	for i := 0; i < 4; i++ {
		s.Proxy.WS.Add(&WSMessage{
			FlowID: wsID, URL: "wss://ws.example.com/socket",
			ToServer: i%2 == 0, Opcode: byte(1 + i%2),
			Payload: []byte(`{"frame":` + string(rune('0'+i)) + `}`),
			Time:    base.Add(time.Duration(i) * time.Second),
		})
	}
}

func TestUILayout_AllViewsRender(t *testing.T) {
	for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets, "unknown"} {
		t.Run(view, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1400, 800))
			seedFlows(rig.s)
			rig.s.View = view
			if d := rig.frames(2); d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Fatalf("view %q produced no dimensions", view)
			}
		})
	}
}

func TestUILayout_EmptyStoreRenders(t *testing.T) {
	for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets} {
		t.Run(view, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 700))
			rig.s.View = view
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatalf("empty view %q produced no dimensions", view)
			}
		})
	}
}

func TestUILayout_InspectorTabsAndModes(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)
	flows := rig.s.Store.Snapshot()

	for _, f := range flows {
		for actTab := 0; actTab < 2; actTab++ {
			for mode := 0; mode < 4; mode++ {
				for sec := 0; sec < 4; sec++ {
					rig.s.Selected = f.ID
					rig.s.ActTab = actTab
					rig.s.RenderMode = mode
					rig.s.SecTab = sec
					if d := rig.frames(1); d.Size.Y <= 0 {
						t.Fatalf("flow %d tab %d mode %d sec %d produced no dimensions", f.ID, actTab, mode, sec)
					}
				}
			}
		}
	}
}

func TestUILayout_InspectorCollapsed(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.InspectorCollapsed = true
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("collapsed inspector produced no dimensions")
	}
	rig.s.InspectorToggle.Click()
	rig.frames(2)
	if rig.s.InspectorCollapsed {
		t.Error("toggle must expand the inspector")
	}
	rig.s.InspectorToggle.Click()
	rig.frames(2)
	if !rig.s.InspectorCollapsed {
		t.Error("toggle must collapse the inspector again")
	}
}

func TestUILayout_UnselectedAndMissingFlow(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)
	rig.s.Selected = 0
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("no selection produced no dimensions")
	}
	rig.s.Selected = 999999
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("stale selection produced no dimensions")
	}
}

func TestUILayout_OverlaysRender(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*UIState)
	}{
		{"clear-confirm", func(s *UIState) { s.ClearConfirmOpen = true }},
		{"context-menu", func(s *UIState) {
			s.CtxOpen = true
			s.CtxFlowID = s.Store.Snapshot()[0].ID
			s.CtxPos.X, s.CtxPos.Y = 300, 200
		}},
		{"annotate", func(s *UIState) {
			s.AnnotateOpen = true
			s.AnnotateFlowID = s.Store.Snapshot()[0].ID
		}},
		{"help", func(s *UIState) { s.HelpOpen = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1300, 800))
			seedFlows(rig.s)
			rig.frames(1)
			tc.setup(rig.s)
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatalf("overlay %q produced no dimensions", tc.name)
			}
		})
	}
}

func TestUILayout_ViewSwitcherButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	rig.s.SegInterc.Click()
	rig.frames(2)
	if rig.s.View != ViewIntercept {
		t.Errorf("View = %q, want %q", rig.s.View, ViewIntercept)
	}
	rig.s.SegWS.Click()
	rig.frames(2)
	if rig.s.View != ViewWebSockets {
		t.Errorf("View = %q, want %q", rig.s.View, ViewWebSockets)
	}
	rig.s.SegHistory.Click()
	rig.frames(2)
	if rig.s.View != ViewHistory {
		t.Errorf("View = %q, want %q", rig.s.View, ViewHistory)
	}
}

func TestUILayout_ClearFlowThroughConfirm(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	if rig.s.Store.Len() == 0 {
		t.Fatal("precondition: store must have flows")
	}

	rig.s.ClearBtn.Click()
	rig.frames(2)
	if !rig.s.ClearConfirmOpen {
		t.Fatal("Clear must open the confirmation modal")
	}
	rig.s.ClearNoBtn.Click()
	rig.frames(2)
	if rig.s.ClearConfirmOpen {
		t.Error("No must close the modal")
	}
	if rig.s.Store.Len() == 0 {
		t.Error("No must not clear the store")
	}

	rig.s.ClearBtn.Click()
	rig.frames(2)
	rig.s.ClearYesBtn.Click()
	rig.frames(2)
	if rig.s.Store.Len() != 0 {
		t.Errorf("Yes must clear the store, %d flows left", rig.s.Store.Len())
	}
	if rig.s.ClearConfirmOpen {
		t.Error("Yes must close the modal")
	}
}

func TestUILayout_FilterEditorNarrowsRows(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	all := len(rig.s.filteredFlows())
	rig.s.Filter.SetText("checkout")
	rig.frames(2)
	got := rig.s.filteredFlows()
	if len(got) != 1 || got[0].Path != "/checkout" {
		t.Fatalf("filter did not narrow to /checkout: %d rows", len(got))
	}

	rig.s.FilterClr.Click()
	rig.frames(2)
	if rig.s.Filter.Text() != "" {
		t.Errorf("clear button must empty the filter, got %q", rig.s.Filter.Text())
	}
	if len(rig.s.filteredFlows()) != all {
		t.Errorf("clearing the filter must restore all %d rows", all)
	}
}

func TestUILayout_HideNoiseSwitch(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	before := len(rig.s.filteredFlows())
	rig.s.HideNoiseSw.Value = true
	after := len(rig.s.filteredFlows())
	if after >= before {
		t.Errorf("hiding noise must drop the .css row: %d -> %d", before, after)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke with noise hidden")
	}
}

func TestUILayout_SortColumnButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	seen := map[string]bool{}
	for i := range rig.s.SortClicks {
		rig.s.SortClicks[i].Click()
		rig.frames(2)
		seen[rig.s.SortColumn] = true
	}
	if len(seen) < 3 {
		t.Errorf("clicking the header columns changed the sort column to only %v", seen)
	}

	rig.s.SortClicks[1].Click()
	rig.frames(2)
	if rig.s.SortColumn != histCols[1] || !rig.s.SortAsc {
		t.Fatalf("first click on a new column must select it ascending: %q asc=%v", rig.s.SortColumn, rig.s.SortAsc)
	}
	rig.s.SortClicks[1].Click()
	rig.frames(2)
	if rig.s.SortColumn != histCols[1] || rig.s.SortAsc {
		t.Errorf("re-clicking the active column must flip direction: %q asc=%v", rig.s.SortColumn, rig.s.SortAsc)
	}
}

func TestUILayout_RowClickSelects(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	sel := map[uint64]bool{}
	for y := float32(80); y < 300; y += 2 {
		rig.press(200, y)
		rig.release(200, y)
		sel[rig.s.Selected] = true
	}
	delete(sel, 0)
	if len(sel) < 3 {
		t.Errorf("clicking history rows selected only %d distinct flows: %v", len(sel), sel)
	}
}

func TestUILayout_SplitDragMovesRatio(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	before := rig.s.SplitRatio
	x := float32(rig.s.LeftDrawn) + 3
	rig.press(x, 400)
	rig.move(x-150, 400)
	rig.release(x-150, 400)
	if rig.s.SplitRatio >= before {
		t.Errorf("dragging the split left must shrink SplitRatio: %v -> %v", before, rig.s.SplitRatio)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after the split drag")
	}
}

func TestUILayout_WebSocketViewAndSelection(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.View = ViewWebSockets
	rig.frames(2)

	msgs := rig.s.Proxy.WS.Snapshot()
	if len(msgs) == 0 {
		t.Fatal("precondition: WS store must have frames")
	}
	for _, m := range msgs {
		rig.s.WSSelected = m.ID
		if d := rig.frames(1); d.Size.Y <= 0 {
			t.Fatalf("WS message %d produced no dimensions", m.ID)
		}
	}
	rig.s.WSSelected = 999999
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("stale WS selection broke the layout")
	}

	rig.s.Filter.SetText("frame")
	if got := rig.s.filteredWS(); len(got) != len(msgs) {
		t.Errorf("filter 'frame' matched %d of %d frames", len(got), len(msgs))
	}
	rig.s.Filter.SetText("no-such-payload")
	if got := rig.s.filteredWS(); len(got) != 0 {
		t.Errorf("non-matching filter returned %d frames", len(got))
	}
	rig.frames(2)
}

func TestUILayout_InterceptView(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.View = ViewIntercept
	rig.frames(2)

	rig.s.Proxy.Manual.SetOn(true)
	rig.frames(2)
	if !rig.s.Proxy.Manual.On() {
		t.Fatal("manual interception must be on")
	}

	done := make(chan struct{})
	go func() {
		rig.s.Proxy.Manual.Hold(&Held{
			Kind: HeldRequest, Method: "GET", URL: "https://x/y", Host: "x",
			Raw: []byte("GET /y HTTP/1.1\r\nHost: x\r\n\r\n"),
		})
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rig.s.Proxy.Manual.Len() == 0 {
		rig.frame()
		time.Sleep(2 * time.Millisecond)
	}
	if rig.s.Proxy.Manual.Len() == 0 {
		t.Fatal("held message never reached the queue")
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("intercept view with a held message produced no dimensions")
	}

	rig.s.ForwardBtn.Click()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rig.frame()
		select {
		case <-done:
			rig.s.Proxy.Manual.SetOn(false)
			rig.frames(2)
			if rig.s.Proxy.Manual.On() {
				t.Error("SetOn(false) must disable manual interception")
			}
			return
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("Forward never released the held message")
}

func TestUILayout_SidebarSectionsRender(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamManual, UpstreamAddr: "127.0.0.1:9", TLS: TLSDecrypt})
	rig.s.Proxy.Targets.Add(&Target{Domain: "*.api.example.com", Upstream: UpstreamAuto, TLS: TLSTunnel, DoH: true})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: false, Type: MRRequest, Area: MRBody, Pattern: "a", Replacement: "b", IsRegex: true, Comment: "swap"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "example.com"})
	rig.s.Proxy.IRules.Add(HeldResponse, InterceptCond{Enabled: true, Or: true, Field: CondStatus, Value: "500"})

	all := []*bool{
		&rig.s.SecTargetsOpen, &rig.s.SecTLSOpen, &rig.s.SecIRulesOpen,
		&rig.s.SecMROpen, &rig.s.SecScopeOpen,
	}
	for _, open := range all {
		*open = false
	}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("collapsed sidebar produced no dimensions")
	}
	for _, open := range all {
		*open = true
	}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("expanded sidebar produced no dimensions")
	}

	rig.s.TargetRows["shop.example.com"] = &TargetRow{Expanded: true}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("expanded target row produced no dimensions")
	}
	for _, kind := range []string{HeldRequest, HeldResponse} {
		rig.s.IRulesActive = kind
		if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
			t.Fatalf("intercept rules (%s) produced no dimensions", kind)
		}
	}
}

func TestUILayout_SidebarAccordionHeaders(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.sidebarFrames(2)

	toggles := []struct {
		name  string
		click func()
		flag  func() bool
	}{
		{"targets", func() { rig.s.SecTargetsHdr.Click() }, func() bool { return rig.s.SecTargetsOpen }},
		{"tls", func() { rig.s.SecTLSHdr.Click() }, func() bool { return rig.s.SecTLSOpen }},
		{"irules", func() { rig.s.SecIRulesHdr.Click() }, func() bool { return rig.s.SecIRulesOpen }},
		{"mr", func() { rig.s.SecMRHdr.Click() }, func() bool { return rig.s.SecMROpen }},
		{"scope", func() { rig.s.SecScopeHdr.Click() }, func() bool { return rig.s.SecScopeOpen }},
	}
	for _, tc := range toggles {
		before := tc.flag()
		tc.click()
		rig.sidebarFrames(2)
		if tc.flag() == before {
			t.Errorf("clicking the %s header did not toggle its section", tc.name)
		}
	}
}

func TestUILayout_SidebarAddTarget(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.sidebarFrames(2)

	rig.s.TargetInput.SetText("added.example.com")
	rig.s.TargetAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 1 {
		t.Fatalf("target was not added, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetInput.Text() != "" {
		t.Errorf("the input must be cleared after adding, got %q", rig.s.TargetInput.Text())
	}

	rig.s.TargetInput.SetText("not-a-domain")
	rig.s.TargetAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 1 {
		t.Errorf("an invalid domain must not be added, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetBanner == "" {
		t.Error("an invalid domain must set a banner")
	}
}

func TestUILayout_SidebarAddScopeAndMRAndIRule(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecScopeOpen = true
	rig.s.SecMROpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.ScopePatInput.SetText("example.com")
	rig.s.ScopeAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.ScopeR.Len() != 1 {
		t.Errorf("scope rule was not added, len=%d", rig.s.Proxy.ScopeR.Len())
	}

	rig.s.MRPatInput.SetText("secret")
	rig.s.MRReplInput.SetText("REDACTED")
	rig.s.MRAddBtn.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) != 1 {
		t.Errorf("match&replace rule was not added, len=%d", len(rig.s.Proxy.MR.Snapshot()))
	}

	rig.s.IRuleValInput.SetText("example.com")
	rig.s.IRuleAddBtn.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(rig.s.IRulesActive); len(conds) != 1 {
		t.Errorf("intercept condition was not added, len=%d", len(conds))
	}
}

func TestUILayout_SidebarPresets(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.MRPresetCSP.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) == 0 {
		t.Error("the CSP preset must add at least one match&replace rule")
	}

	rig.s.IRulePresetImg.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(rig.s.IRulesActive); len(conds) == 0 {
		t.Error("the image preset must add at least one intercept condition")
	}
}

func TestUILayout_SidebarCycleSelectors(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	cases := []struct {
		name  string
		click func()
		get   func() int
	}{
		{"mr-type", func() { rig.s.MRTypeBtn.Click() }, func() int { return rig.s.MRTypeSel }},
		{"mr-area", func() { rig.s.MRAreaBtn.Click() }, func() int { return rig.s.MRAreaSel }},
		{"scope-kind", func() { rig.s.ScopeKindBtn.Click() }, func() int { return rig.s.ScopeKindSel }},
		{"scope-field", func() { rig.s.ScopeFieldBtn.Click() }, func() int { return rig.s.ScopeFieldSel }},
		{"irule-field", func() { rig.s.IRuleFieldBtn.Click() }, func() int { return rig.s.IRuleFieldSel }},
	}
	for _, tc := range cases {
		seen := map[int]bool{tc.get(): true}
		for i := 0; i < 12; i++ {
			tc.click()
			rig.sidebarFrames(1)
			seen[tc.get()] = true
		}
		if len(seen) < 2 {
			t.Errorf("%s selector never changed value", tc.name)
		}
		if tc.get() < 0 {
			t.Errorf("%s selector went negative: %d", tc.name, tc.get())
		}
	}
}

func TestUILayout_SidebarIRuleTabs(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.IRulesRespTab.Click()
	rig.sidebarFrames(2)
	if rig.s.IRulesActive != HeldResponse {
		t.Errorf("IRulesActive = %q, want %q", rig.s.IRulesActive, HeldResponse)
	}
	rig.s.IRulesReqTab.Click()
	rig.sidebarFrames(2)
	if rig.s.IRulesActive != HeldRequest {
		t.Errorf("IRulesActive = %q, want %q", rig.s.IRulesActive, HeldRequest)
	}
}

func TestUILayout_NarrowViewport(t *testing.T) {
	for _, sz := range []image.Point{{X: 500, Y: 300}, {X: 800, Y: 400}, {X: 1920, Y: 1080}} {
		rig := newUIRig(t, sz)
		seedFlows(rig.s)
		rig.s.Selected = rig.s.Store.Snapshot()[0].ID
		for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets} {
			rig.s.View = view
			if d := rig.frames(2); d.Size.X <= 0 {
				t.Errorf("size %v view %q produced no dimensions", sz, view)
			}
		}
	}
}

func TestUILayout_ConfigRoundTrip(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.Proxy.Targets.Add(&Target{Domain: "a.example.com", Upstream: UpstreamManual, UpstreamAddr: "1.2.3.4:80", TLS: TLSTunnel, Delay: 250 * time.Millisecond, DoH: true})
	rig.s.Proxy.Rules.Set("b.example.com", HostRule{Delay: 100 * time.Millisecond, UseDoH: true})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "p", Replacement: "q", Comment: "c"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondMethod, Value: "POST"})
	rig.s.Proxy.IRules.SetEnabled(HeldResponse, false)
	rig.s.BindAddr.SetText("127.0.0.1:9999")
	rig.s.SortColumn = "Status"
	rig.s.SortAsc = false
	rig.s.InspectorCollapsed = true
	rig.s.View = ViewWebSockets

	c := rig.s.SnapshotConfig()
	if err := SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded := LoadConfig()
	if loaded.BindAddr != "127.0.0.1:9999" || loaded.View != ViewWebSockets {
		t.Errorf("bind/view not persisted: %+v", loaded)
	}
	if loaded.SortColumn != "Status" || loaded.SortAsc {
		t.Errorf("sort not persisted: %q asc=%v", loaded.SortColumn, loaded.SortAsc)
	}
	if !loaded.InspectorCollapsed {
		t.Error("inspector collapse not persisted")
	}
	if len(loaded.Targets) != 1 || loaded.Targets[0].DelayMs != 250 || !loaded.Targets[0].DoH {
		t.Errorf("targets not persisted: %+v", loaded.Targets)
	}
	if len(loaded.Rules) != 1 || loaded.Rules[0].DelayMs != 100 {
		t.Errorf("rules not persisted: %+v", loaded.Rules)
	}
	if len(loaded.MatchReplace) != 1 || loaded.MatchReplace[0].Comment != "c" {
		t.Errorf("match&replace not persisted: %+v", loaded.MatchReplace)
	}
	if len(loaded.Scope) != 1 || loaded.Scope[0].Kind != ScopeExclude {
		t.Errorf("scope not persisted: %+v", loaded.Scope)
	}
	if len(loaded.InterceptReq) != 1 || loaded.InterceptReq[0].Value != "POST" {
		t.Errorf("intercept conditions not persisted: %+v", loaded.InterceptReq)
	}
	if loaded.IRespEnabled == nil || *loaded.IRespEnabled {
		t.Error("response ruleset enable flag not persisted")
	}

	p2 := NewProxy(NewStore())
	p2.Rules = NewRules()
	loaded.ApplyTo(p2)
	if p2.Targets.Len() != 1 || p2.Rules.Len() != 1 || len(p2.MR.Snapshot()) != 1 || p2.ScopeR.Len() != 1 {
		t.Errorf("ApplyTo did not restore the proxy state")
	}
	if enabled, conds := p2.IRules.Snapshot(HeldResponse); enabled || len(conds) != 0 {
		t.Errorf("response ruleset restored wrong: enabled=%v conds=%d", enabled, len(conds))
	}
}

func TestUILayout_DirtyFlag(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.frames(1)
	rig.s.MarkDirty()
	if !rig.s.Dirty() {
		t.Fatal("MarkDirty must set the flag")
	}
	if rig.s.Dirty() {
		t.Error("Dirty must clear the flag after reading")
	}
}

func TestUILayout_MRAddRejectsEmptyPattern(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.sidebarFrames(2)

	for _, pat := range []string{"", "   "} {
		rig.s.MRPatInput.SetText(pat)
		rig.s.MRReplInput.SetText("injected")
		rig.s.MRAddBtn.Click()
		rig.sidebarFrames(2)
		if n := len(rig.s.Proxy.MR.Snapshot()); n != 0 {
			t.Fatalf("pattern %q added %d rule(s); a rule naming no header would be applied to every message", pat, n)
		}
	}

	rig.s.MRPatInput.SetText("  X-Trace  ")
	rig.s.MRAddBtn.Click()
	rig.sidebarFrames(2)
	rules := rig.s.Proxy.MR.Snapshot()
	if len(rules) != 1 {
		t.Fatalf("a non-empty pattern must still be accepted, len=%d", len(rules))
	}
	if rules[0].Pattern != "X-Trace" {
		t.Errorf("Pattern = %q, want the trimmed name", rules[0].Pattern)
	}
}
