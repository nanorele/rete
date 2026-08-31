//go:build screenshots

package flow

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func shotTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func renderFlowShot(t *testing.T, name string, sz image.Point, ed *Editor, host *Host, tweak ...func()) {
	t.Helper()
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	th := shotTheme()
	gtx := func(ops *op.Ops) layout.Context {
		return layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         time.Now(),
		}
	}
	for i := 0; i < 2; i++ {
		ed.Layout(gtx(new(op.Ops)), th, host)
	}
	for _, fn := range tweak {
		fn()
	}
	ops := new(op.Ops)
	ed.Layout(gtx(ops), th, host)

	if err := win.Frame(ops); err != nil {
		t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	dir := filepath.Join("testdata", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", path)
}

func buildShotScenario(ed *Editor) (sel *Node, wsSend *Node) {
	s := ed.Scenario
	start := s.Nodes[0]
	start.X, start.Y = 40, 120

	login := NewNode(KindRequest, 280, 40)
	login.NameEd.SetText("Login")
	login.URLEd.SetText("https://api.example.com/login")
	login.Method = "POST"

	fetch := NewNode(KindRequest, 540, 40)
	fetch.NameEd.SetText("Fetch users")
	fetch.URLEd.SetText("https://api.example.com/users")

	failNode := NewNode(KindRequest, 540, 220)
	failNode.NameEd.SetText("Broken call")
	failNode.URLEd.SetText("https://api.example.com/broken")

	idle := NewNode(KindDelay, 280, 220)

	wsOpen := NewNode(KindWSRequest, 280, 400)
	wsOpen.NameEd.SetText("Open socket")
	wsOpen.URLEd.SetText("wss://api.example.com/ws")
	wsOpen.KeepOpen = true

	send := NewNode(KindWSSend, 280, 560)
	send.NameEd.SetText("Subscribe")
	send.BodyEd.SetText(`{"type":"subscribe"}`)

	s.Nodes = append(s.Nodes, login, fetch, failNode, idle, wsOpen, send)

	e1 := NewEdge(start.ID, login.ID)
	e2 := NewEdge(login.ID, fetch.ID)
	e3 := NewEdge(login.ID, failNode.ID)
	vEdge := NewEdge(wsOpen.ID, send.ID)
	vEdge.FromSide = SideBottom
	vEdge.ToSide = SideTop
	s.Edges = append(s.Edges, e1, e2, e3, vEdge)

	r := ed.Runner
	r.nodeSt[login.ID] = StOK
	r.nodeSt[fetch.ID] = StOK
	r.nodeSt[failNode.ID] = StFail
	r.nodeSt[wsOpen.ID] = StOK
	r.nodeSt[send.ID] = StRunning
	r.nodeInfo[login.ID] = "200"
	r.nodeInfo[fetch.ID] = "200"
	r.nodeInfo[failNode.ID] = "ERR: timeout"
	r.edgeSt[e1.ID] = StOK
	r.edgeSt[e2.ID] = StOK
	r.edgeSt[e3.ID] = StFail
	r.edgeSt[vEdge.ID] = StOK

	ed.selected = map[string]bool{fetch.ID: true, idle.ID: true}
	ed.selNodeID = fetch.ID
	return fetch, send
}

func TestFlowShotStates(t *testing.T) {
	setupFlowConfig(t)
	sz := image.Pt(1280, 800)
	ed := NewEditor()
	_ = SaveBlock(scenarioDTO{Name: "Auth combo", Nodes: []nodeDTO{
		{Kind: int(KindRequest), X: 0, Y: 0, Name: "Login", Method: "POST"},
		{Kind: int(KindSetVar), X: 260, Y: 0, Name: "Store token"},
	}})
	buildShotScenario(ed)
	ed.mode = modeWidgets
	host := &Host{Win: new(app.Window), RootCtx: context.Background(), WinSize: sz}
	renderFlowShot(t, "flow_states", sz, ed, host)
}

func TestFlowShotPortsHover(t *testing.T) {
	setupFlowConfig(t)
	sz := image.Pt(1280, 800)
	host := &Host{Win: new(app.Window), RootCtx: context.Background(), WinSize: sz}

	ed := NewEditor()
	hovered, _ := buildShotScenario(ed)
	ed.clearSelection()
	ed.mode = modeWidgets
	renderFlowShot(t, "flow_ports_hover", sz, ed, host, func() {
		sp, w, h := ed.nodeScreenRect(hovered)
		ed.setHover(f32.Pt(sp.X+w/2, sp.Y+h/2))
	})

	idle := NewEditor()
	buildShotScenario(idle)
	idle.clearSelection()
	idle.mode = modeWidgets
	renderFlowShot(t, "flow_ports_idle", sz, idle, host)

	used := NewEditor()
	buildShotScenario(used)
	used.clearSelection()
	used.mode = modeWidgets
	var connected *Node
	for _, n := range used.Scenario.Nodes {
		if n.DisplayName() == "Open socket" {
			connected = n
		}
	}
	renderFlowShot(t, "flow_ports_occupied", sz, used, host, func() {
		sp, w, h := used.nodeScreenRect(connected)
		used.setHover(f32.Pt(sp.X+w/2, sp.Y+h/2))
	})
}

func TestFlowShotWSSendProps(t *testing.T) {
	setupFlowConfig(t)
	sz := image.Pt(1280, 800)
	ed := NewEditor()
	_, send := buildShotScenario(ed)
	ed.selectOnly(send.ID)
	ed.mode = modeProps
	host := &Host{Win: new(app.Window), RootCtx: context.Background(), WinSize: sz}
	renderFlowShot(t, "flow_wssend_props", sz, ed, host)
}
