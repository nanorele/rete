package flow

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func makeFlowGtx(size image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
		Now:         time.Now(),
	}
}

func layoutOnce(t *testing.T, ed *Editor, host *Host, size image.Point) {
	t.Helper()
	if host.Win == nil {
		host.Win = testWindow()
	}
	if host.WinSize == (image.Point{}) {
		host.WinSize = size
	}
	gtx := makeFlowGtx(size)
	ed.Layout(gtx, material.NewTheme(), host)
}

func TestLayoutCollectsFrameEnvironments(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	sv := addNodeTo(ed, KindSetVar, 300, 0)
	sv.VarNameEd.SetText("token")
	req := addNodeTo(ed, KindRequest, 600, 0)
	req.EnvID = "env2"

	envCalls := 0
	host := &Host{
		WinSize:    image.Pt(1200, 800),
		ActiveEnv:  func() map[string]string { return map[string]string{"host": "x"} },
		EnvOptions: func() []EnvOption { return []EnvOption{{ID: "env2", Name: "Prod"}} },
		EnvVars: func(id string) map[string]string {
			envCalls++
			return map[string]string{"secret": id}
		},
	}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.frameEnvs[""]["host"] != "x" {
		t.Errorf("the active env must be cached under the empty key, got %v", ed.frameEnvs)
	}
	if ed.frameEnvs["env2"]["secret"] != "env2" {
		t.Errorf("per-node envs must be cached, got %v", ed.frameEnvs)
	}
	if envCalls != 1 {
		t.Errorf("each env must be resolved once per frame, got %d calls", envCalls)
	}
	if !ed.setVarNames["token"] {
		t.Errorf("set-variable names must be collected, got %v", ed.setVarNames)
	}
	if len(ed.envOpts) != 1 || ed.envOpts[0].Name != "Prod" {
		t.Errorf("env options = %v", ed.envOpts)
	}
}

func TestLayoutWithoutHostCallbacks(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	addNodeTo(ed, KindRequest, 300, 0)
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1000, 700)}, image.Pt(1000, 700))

	if ed.envOpts != nil {
		t.Errorf("env options must be nil without a provider, got %v", ed.envOpts)
	}
	if len(ed.frameEnvs) != 0 {
		t.Errorf("frameEnvs must stay empty without an active env, got %v", ed.frameEnvs)
	}
}

func TestLayoutClampsPanelWidth(t *testing.T) {
	setupFlowConfig(t)
	tests := []struct {
		name  string
		start int
		size  image.Point
		check func(t *testing.T, ed *Editor, size image.Point)
	}{
		{
			"unset width gets a default",
			0,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, _ image.Point) {
				if ed.panelW != 300 {
					t.Errorf("panelW = %d, want the 300dp default", ed.panelW)
				}
			},
		},
		{
			"too wide is clamped to half the window",
			10000,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, size image.Point) {
				if ed.panelW != size.X/2 {
					t.Errorf("panelW = %d, want %d", ed.panelW, size.X/2)
				}
			},
		},
		{
			"too narrow is clamped to the minimum",
			1,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, _ image.Point) {
				if ed.panelW != 56 {
					t.Errorf("panelW = %d, want the 56dp minimum", ed.panelW)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newTestEditor()
			ed.panelW = tt.start
			layoutOnce(t, ed, &Host{WinSize: tt.size}, tt.size)
			tt.check(t, ed, tt.size)
		})
	}
}

func TestLayoutRecordsCanvasOrigin(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	host := &Host{WinSize: image.Pt(1400, 900)}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.canvasOrig != image.Pt(200, 100) {
		t.Errorf("canvasOrig = %v, want (200,100)", ed.canvasOrig)
	}
	if ed.winH != 900 {
		t.Errorf("winH = %d, want 900", ed.winH)
	}
}

func TestLayoutMeasuresNodeSizeAndFits(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.nodeW, ed.nodeH = 0, 0
	ed.pendingFit = true
	ed.Scenario.Nodes[0].X, ed.Scenario.Nodes[0].Y = 4000, 4000
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))

	if ed.nodeW != 176 || ed.nodeH != 56 {
		t.Errorf("node metrics = (%v,%v), want (176,56)", ed.nodeW, ed.nodeH)
	}
	if ed.portHit != 12 {
		t.Errorf("portHit = %v, want 12", ed.portHit)
	}
	if ed.pendingFit {
		t.Error("the deferred fit must have run once the canvas has a size")
	}
	if ed.canvasSize.X <= 0 || ed.canvasSize.Y <= 0 {
		t.Errorf("canvasSize = %v", ed.canvasSize)
	}
	s := ed.toScreen(f32.Pt(4000, 4000))
	if s.X < 0 || s.Y < 0 || s.X > float32(ed.canvasSize.X) || s.Y > float32(ed.canvasSize.Y) {
		t.Errorf("the fitted node projects off canvas at %v", s)
	}
}

func TestLayoutAllPanelModes(t *testing.T) {
	setupFlowConfig(t)
	modes := []struct {
		name string
		mode panelMode
	}{
		{"widgets", modeWidgets},
		{"properties", modeProps},
		{"history", modeHistory},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			ed := newTestEditor()
			req := addNodeTo(ed, KindRequest, 300, 0)
			req.URLEd.SetText("http://{{unknown}}")
			cond := addNodeTo(ed, KindCondition, 600, 0)
			loop := addNodeTo(ed, KindLoop, 900, 0)
			note := addNodeTo(ed, KindNote, 1200, 0)
			sv := addNodeTo(ed, KindSetVar, 1500, 0)
			delay := addNodeTo(ed, KindDelay, 1800, 0)
			for _, n := range []*Node{cond, loop, note, sv, delay} {
				connect(ed, req, n)
			}
			ed.mode = m.mode
			ed.selectOnly(req.ID)

			host := &Host{
				WinSize:    image.Pt(1200, 800),
				EnvOptions: func() []EnvOption { return []EnvOption{{ID: "", Name: "Active"}, {ID: "e", Name: "Prod"}} },
				ActiveEnv:  func() map[string]string { return map[string]string{} },
				EnvVars:    func(string) map[string]string { return map[string]string{} },
			}
			layoutOnce(t, ed, host, image.Pt(1200, 800))
		})
	}
}

func TestLayoutPropsForEveryNodeKind(t *testing.T) {
	setupFlowConfig(t)
	kinds := []NodeKind{KindStart, KindRequest, KindCondition, KindLoop, KindDelay, KindSetVar, KindNote}
	for _, k := range kinds {
		t.Run(k.Title(), func(t *testing.T) {
			ed := newTestEditor()
			n := addNodeTo(ed, k, 300, 0)
			ed.mode = modeProps
			ed.selectOnly(n.ID)
			layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
		})
	}
}

func TestLayoutEdgePropsForEveryCondition(t *testing.T) {
	setupFlowConfig(t)
	for _, cond := range CondKinds {
		t.Run(cond.Title(), func(t *testing.T) {
			ed := newTestEditor()
			a := addNodeTo(ed, KindRequest, 300, 0)
			b := addNodeTo(ed, KindDelay, 600, 0)
			e := connect(ed, a, b)
			e.Cond = cond
			ed.mode = modeProps
			ed.selEdgeID = e.ID
			layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
		})
	}
}

func TestLayoutHistoryWithRuns(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.mode = modeHistory

	rec := &RunRecord{Label: "Run 1 · 12:00:00", Seq: 1, Clock: "12:00:00", Dur: 1500 * time.Millisecond, Done: true}
	ed.Runner.runs = append(ed.Runner.runs, rec)
	ed.Runner.addEntry(rec, &RunEntry{
		Node: "Fetch", Detail: "GET http://x", Code: 200, Status: "200 OK",
		OK: true, Body: `{"a":1}`, BodyLen: 7, Dur: 42 * time.Millisecond, Expanded: true,
	})
	ed.Runner.addEntry(rec, &RunEntry{
		Node: "Broken", Detail: "GET http://y", Code: 0, Status: "no response", OK: false,
	})

	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))

	if len(ed.Runner.Runs()) != 1 {
		t.Errorf("run history must survive a layout pass, got %d", len(ed.Runner.Runs()))
	}
}

func TestLayoutCompactPanel(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.panelW = 1
	ed.mode = modeWidgets
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
	if !ed.panelCompact {
		t.Error("a narrow panel must switch to compact mode")
	}

	ed2 := newTestEditor()
	ed2.panelW = 300
	layoutOnce(t, ed2, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
	if ed2.panelCompact {
		t.Error("a wide panel must not be compact")
	}
}

func TestLayoutWhileRunning(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.Runner.mu.Lock()
	ed.Runner.running = true
	ed.Runner.paused = true
	ed.Runner.stepMode = true
	ed.Runner.status = "Paused · Fetch"
	ed.Runner.mu.Unlock()

	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
}

func TestLayoutWithOpenOverlays(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	n := addNodeTo(ed, KindRequest, 300, 0)
	ed.selectOnly(n.ID)
	ed.mode = modeProps
	ed.envMenuNodeID = n.ID
	ed.envDropOpen = true
	ed.envDropAtY = 200
	ed.connectFromID = n.ID
	ed.connectPos = f32.Pt(500, 300)
	ed.marquee = true
	ed.marqueeStart = f32.Pt(10, 10)
	ed.marqueeCur = f32.Pt(200, 200)

	host := &Host{
		WinSize:           image.Pt(1200, 800),
		EnvOptions:        func() []EnvOption { return []EnvOption{{ID: "", Name: "Active"}, {ID: "e", Name: "Prod"}} },
		ExternalDrag:      true,
		ExternalDragPos:   f32.Pt(600, 400),
		ExternalDragLabel: "dragged request",
	}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.extDragLabel != "dragged request" {
		t.Errorf("external drag state must be mirrored, got %q", ed.extDragLabel)
	}
}

func TestNewEditorLoadsLatest(t *testing.T) {
	dir := setupFlowConfig(t)
	writeFlow(t, dir, scenarioDTO{
		ID:    "saved",
		Name:  "Saved flow",
		Nodes: []nodeDTO{{ID: "s", Kind: int(KindStart)}},
	}, time.Now())

	ed := NewEditor()
	if ed.Scenario == nil || ed.Scenario.ID != "saved" {
		t.Fatalf("NewEditor must load the latest scenario, got %+v", ed.Scenario)
	}
	if ed.Runner == nil {
		t.Fatal("NewEditor must create a runner")
	}
	if ed.zoom != 1 || !ed.pendingFit || ed.mode != modeWidgets {
		t.Errorf("unexpected initial state: zoom=%v pendingFit=%v mode=%v", ed.zoom, ed.pendingFit, ed.mode)
	}
	if ed.selected == nil {
		t.Error("the selection set must be initialised")
	}
	if ed.panelList.Axis != layout.Vertical {
		t.Error("the panel list must be vertical")
	}
}
