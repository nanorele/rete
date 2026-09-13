//go:build screenshots

package apptest

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"image"
	"image/color"
	"image/png"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"rete/internal/model"
	. "rete/internal/ui"
	"rete/internal/ui/collections"
	"rete/internal/ui/colorpicker"
	"rete/internal/ui/environments"
	"rete/internal/ui/flow"
	harui "rete/internal/ui/har"
	"rete/internal/ui/mitm"
	"rete/internal/ui/settings"
	"rete/internal/ui/sidebar"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"
	"rete/internal/ui/workspace"
	"rete/internal/ws"
	"strings"
	"testing"
	"time"
)

func respTab(ui *AppUI) *workspace.RequestTab {
	withTab(ui)
	ui.SidebarSection = "requests"
	tab := ui.Tabs[0]
	tab.Method = "POST"
	tab.URLInput.SetText("https://api.example.com/v2/users/search?page=2&limit=50")
	tab.ReqEditor.SetText("{\n  \"query\": \"alice\",\n  \"filters\": {\"active\": true}\n}")
	tab.RespEditor.SetText("{\n  \"users\": [\n    {\"id\": 1, \"name\": \"alice\"}\n  ],\n  \"count\": 1\n}")
	tab.Status = "200 OK · 1.2 KB · 231 ms"
	tab.AddHeader("Authorization", "Bearer abcdef")
	tab.HeadersExpanded = true
	return tab
}

func adaptScenes() []scene {
	return []scene{
		{"adapt-sidebar-max", func(ui *AppUI) {
			respTab(ui)
			ui.SidebarWidth = 640
		}},
		{"adapt-hsplit-resp-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.SplitRatio = 0.95
		}},
		{"adapt-hsplit-req-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.SplitRatio = 0.05
		}},
		{"adapt-vstack-req-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.VStackRatio = 0.01
		}},
		{"adapt-vstack-resp-min", func(ui *AppUI) {
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.VStackRatio = 0.99
		}},
		{"adapt-ws-narrow", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("WS")}
			ui.ActiveIdx = 0
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/socket")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			s.UseMsgpackProto = true
			s.AddSubprotocol("graphql-transport-ws")
			s.SplitRatio = 0.2
			ui.SidebarWidth = 500
		}},
		{"adapt-ws-msgs-min", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("WS")}
			ui.ActiveIdx = 0
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/socket")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			s.SplitRatio = 0.95
		}},
		{"adapt-runner", func(ui *AppUI) {
			tab := respTab(ui)
			tab.RunOpen = true
		}},
		{"adapt-netlimit-narrow", func(ui *AppUI) {
			ui.SidebarSection = "netlimit"
			ui.SidebarWidth = 160
		}},
		{"adapt-mitm-inspector-min", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			ui.MITM.SplitRatio = 0.95
		}},
		{"adapt-mitm-table-min", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			ui.MITM.SplitRatio = 0.05
		}},
		{"adapt-har", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
		}},
		{"adapt-har-inspector-min", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.SplitRatio = 0.95
		}},
		{"adapt-har-table-min", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.SplitRatio = 0.05
		}},
		{"adapt-har-files", func(ui *AppUI) {
			ui.SidebarSection = "har"
			ui.HARView.ApplyLoad([]byte(sampleHAR), "sample.har", nil)
			ui.HARView.TopTab = harui.TabFiles
			ui.HARView.SplitRatio = 0.95
		}},
		{"adapt-settings-tiny", settingsScene(0)},
	}
}

const sampleHAR = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "test", "version": "1.0"},
    "pages": [{"id": "page_1", "title": "Example page with a fairly long title", "startedDateTime": "2026-01-01T00:00:00Z", "pageTimings": {}}],
    "entries": [
      {
        "pageref": "page_1",
        "startedDateTime": "2026-01-01T00:00:01Z",
        "time": 231,
        "request": {
          "method": "GET",
          "url": "https://api.example.com/v2/users/search?page=2&limit=50&sort=name-descending",
          "headers": [{"name": "Accept", "value": "application/json"}, {"name": "Authorization", "value": "Bearer abcdef0123456789"}],
          "queryString": [],
          "bodySize": 0
        },
        "response": {
          "status": 200,
          "statusText": "OK",
          "headers": [{"name": "Content-Type", "value": "application/json; charset=utf-8"}],
          "content": {"size": 64, "mimeType": "application/json", "text": "{\"users\":[{\"id\":1,\"name\":\"alice\"}],\"count\":1}"},
          "bodySize": 64
        }
      },
      {
        "pageref": "page_1",
        "startedDateTime": "2026-01-01T00:00:02Z",
        "time": 87,
        "request": {
          "method": "POST",
          "url": "https://cdn.example.com/static/assets/js/very-long-bundle-name.materialized.min.js",
          "headers": [],
          "queryString": [],
          "bodySize": 0
        },
        "response": {
          "status": 404,
          "statusText": "Not Found",
          "headers": [{"name": "Content-Type", "value": "text/html"}],
          "content": {"size": 22, "mimeType": "text/html", "text": "<html>not found</html>"},
          "bodySize": 22
        }
      }
    ]
  }
}`

func TestAdaptScreenshots(t *testing.T) {
	sizes := []image.Point{{X: 1280, Y: 800}, {X: 700, Y: 500}, {X: 560, Y: 400}}
	for _, sz := range sizes {
		for _, sc := range adaptScenes() {
			sc := sc
			sz := sz
			t.Run(sc.name, func(t *testing.T) {
				renderScene(t, sc, sz)
			})
		}
	}
}

func bodyTypeScenes() []scene {
	mk := func(mut func(*workspace.RequestTab)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			mut(tab)
		}
	}
	return []scene{
		{"bd-form-mixed", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyFormData
			t.FormParts = []*workspace.FormDataPart{
				workspace.NewFormPart("name", "alice", model.FormPartText, "", 0),
				workspace.NewFormPart("avatar", "", model.FormPartFile, "C:\\pics\\avatar.png", 34567),
				workspace.NewFormPart("empty", "", model.FormPartFile, "", 0),
			}
		})},
		{"bd-ue-fields", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyURLEncoded
			t.URLEncoded = []*workspace.URLEncodedPart{
				workspace.NewURLEncodedPart("user", "alice"),
				workspace.NewURLEncodedPart("limit", "50"),
			}
		})},
		{"bd-ue-empty", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyURLEncoded
		})},
		{"bd-binary-empty", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyBinary
		})},
		{"bd-binary-chosen", mk(func(t *workspace.RequestTab) {
			t.BodyType = model.BodyBinary
			t.BinaryFilePath = "C:\\data\\payload.bin"
			t.BinaryFileSize = 123456
		})},
	}
}

func TestBodyTypeScreenshots(t *testing.T) {
	for _, sc := range bodyTypeScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}

func renderGapScene(t *testing.T, setup func(*AppUI), sz image.Point) *image.RGBA {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	seedTestData(ui)
	setup(ui)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	for i := 0; i < 2; i++ {
		ui.LayoutApp(newShotGtx(new(op.Ops), sz))
	}
	ops := new(op.Ops)
	ui.LayoutApp(newShotGtx(ops, sz))
	if err := win.Frame(ops); err != nil {
		t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	return img
}

func pxAt(img *image.RGBA, x, y int) [3]uint8 {
	i := img.PixOffset(x, y)
	return [3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}
}

func gapRightOf(img *image.RGBA, x0, y int) int {
	bg := pxAt(img, x0+1, y)
	n := 0
	for x := x0 + 1; x < img.Rect.Max.X && pxAt(img, x, y) == bg; x++ {
		n++
	}
	return n
}

func gapLeftOfEdge(img *image.RGBA, y int) int {
	w := img.Rect.Max.X
	bg := pxAt(img, w-1, y)
	n := 0
	for x := w - 1; x >= 0 && pxAt(img, x, y) == bg; x-- {
		n++
	}
	return n
}

func TestBoundaryGapsUniform(t *testing.T) {
	sz := image.Point{X: 1280, Y: 800}
	http := func(hide bool, extraTabs int) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			ui.Settings.HideSidebar = hide
			for i := 0; i < extraTabs; i++ {
				ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("Request number long "+string(rune('A'+i))))
			}
		}
	}

	const urlRowY = 100
	const bodyY = 400
	const tabRowY = 60
	const gutterW = 36

	open := renderGapScene(t, http(false, 0), sz)
	hidden := renderGapScene(t, http(true, 0), sz)
	tabs := renderGapScene(t, http(false, 14), sz)

	hiddenGap := gapRightOf(hidden, gutterW, bodyY)
	if hiddenGap < 2 || hiddenGap > 8 {
		t.Fatalf("hidden-sidebar gap out of expected range: %d", hiddenGap)
	}
	sidebarW := 250
	if got := gapRightOf(open, sidebarW, bodyY); got != hiddenGap {
		t.Errorf("open-sidebar gap %d, want %d (same as hidden)", got, hiddenGap)
	}
	if got := gapRightOf(open, sidebarW, urlRowY); got != hiddenGap {
		t.Errorf("open-sidebar gap at URL row %d, want %d", got, hiddenGap)
	}

	wantRight := hiddenGap + 1
	if got := gapLeftOfEdge(open, bodyY); got != wantRight {
		t.Errorf("right edge to response pane %d, want %d", got, wantRight)
	}
	if got := gapLeftOfEdge(open, urlRowY); got != wantRight {
		t.Errorf("right edge to Send button %d, want %d", got, wantRight)
	}
	if got := gapLeftOfEdge(tabs, tabRowY); got != wantRight {
		t.Errorf("right edge to justified tab row %d, want %d", got, wantRight)
	}
}

func collapseHdrScenes() []scene {
	http := func(mode int, mut func(*workspace.RequestTab)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = mode
			if mut != nil {
				mut(tab)
			}
		}
	}
	ws := func(mut func(*workspace.WSSession)) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.example.com/ws")
			tab.AddHeader("Origin", "https://example.com")
			s := tab.EnsureWS()
			if mut != nil {
				mut(s)
			}
		}
	}
	return []scene{
		{"ch-h-hdr-open", http(workspace.LayoutModeHoriz, nil)},
		{"ch-h-hdr-closed", http(workspace.LayoutModeHoriz, func(t *workspace.RequestTab) {
			t.HeadersExpanded = false
		})},
		{"ch-v-hdr-open", http(workspace.LayoutModeVert, nil)},
		{"ch-v-hdr-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.HeadersExpanded = false
		})},
		{"ch-v-req-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.ReqBodyCollapsed = true
			t.VStackRatio = 0.01
		})},
		{"ch-h-req-closed", http(workspace.LayoutModeHoriz, func(t *workspace.RequestTab) {
			t.ReqBodyCollapsed = true
		})},
		{"ch-v-resp-closed", http(workspace.LayoutModeVert, func(t *workspace.RequestTab) {
			t.RespBodyCollapsed = true
			t.VStackRatio = 0.99
		})},
		{"ch-ws-open", ws(nil)},
		{"ch-ws-hdr-closed", ws(func(s *workspace.WSSession) {
			s.HeadersCollapsed = true
		})},
		{"ch-ws-compose-closed", ws(func(s *workspace.WSSession) {
			s.ComposeCollapsed = true
		})},
		{"ch-ws-msgs-closed", ws(func(s *workspace.WSSession) {
			s.MessagesCollapsed = true
		})},
	}
}

func TestCollapseHdrScreenshots(t *testing.T) {
	for _, sc := range collapseHdrScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}

type holderRig struct {
	t  *testing.T
	ui *AppUI
	r  input.Router
	sz image.Point
}

func newHolderRig(t *testing.T) *holderRig {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ScriptsExpanded = false

	root := &collections.CollectionNode{Name: "Sample API", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "col1", Name: "Sample API", Root: root}
	root.Collection = col
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("Request %d", i)
		root.Children = append(root.Children, &collections.CollectionNode{
			Name: name, Request: &model.ParsedRequest{Name: name, Method: "GET"}, Parent: root, Depth: 1, Collection: col,
		})
	}
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()
	for i := 0; i < 3; i++ {
		env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
			ID: fmt.Sprintf("env%d", i), Name: fmt.Sprintf("Env %d", i), HighlightColor: "#3b82f6",
		}}
		env.InitEditor()
		ui.Environments = append(ui.Environments, env)
	}
	rig := &holderRig{t: t, ui: ui, sz: image.Pt(900, 600)}
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	return rig
}

func (rig *holderRig) frame(events ...pointer.Event) *op.Ops {
	ops := new(op.Ops)
	for _, e := range events {
		rig.r.Queue(e)
	}
	rig.ui.LayoutApp(layout.Context{
		Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz), Now: fixedTime, Source: rig.r.Source(),
	})
	rig.r.Frame(ops)
	return ops
}

func (rig *holderRig) shoot(name string, ops *op.Ops) *image.RGBA {
	win, err := headless.NewWindow(rig.sz.X, rig.sz.Y)
	if err != nil {
		rig.t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()
	if err := win.Frame(ops); err != nil {
		rig.t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		rig.t.Fatalf("screenshot: %v", err)
	}
	dir := filepath.Join("testdata", "screenshots")
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		rig.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		rig.t.Fatal(err)
	}
	return img
}

func expectHolderColor(t *testing.T, img *image.RGBA, x, y int, label string) {
	t.Helper()
	got := img.RGBAAt(x, y)
	want := theme.BgDark
	if got.R != want.R || got.G != want.G || got.B != want.B {
		t.Errorf("%s placeholder pixel at (%d,%d) = %v, want theme.BgDark %v", label, x, y, got, color.NRGBA(want))
	}
}

func TestDragHolderPaintedWithThemeBg(t *testing.T) {
	const titleBar = 30
	rig := newHolderRig(t)
	x := float32(120)

	h := rig.ui.ColRowH()
	if h <= 0 {
		t.Fatal("col row height not measured")
	}
	y := float32(titleBar + 27 + h + h/2)
	rig.frame(press(x, y))
	rig.frame(dragTo(x, y+10))
	if rig.ui.DraggedNode == nil || !rig.ui.DragNodeActive {
		t.Fatalf("node drag not active: dragged=%v active=%v", rig.ui.DraggedNode, rig.ui.DragNodeActive)
	}
	ops := rig.frame(dragTo(x, y+10+float32(2*h)))
	img := rig.shoot("dragholder-node_900x600", ops)
	expectHolderColor(t, img, 200, int(y), "node")
	rig.frame(release(x, y+10+float32(2*h)))
	rig.frame()

	eh := rig.ui.EnvRowH()
	if eh <= 0 {
		t.Fatal("env row height not measured")
	}
	ey := float32(titleBar + rig.ui.EnvDivY() + 27 + eh/2)
	rig.frame(press(x, ey))
	rig.frame(dragTo(x, ey+10))
	if rig.ui.DraggedEnv == nil || !rig.ui.DragEnvActive {
		t.Fatalf("env drag not active: dragged=%v active=%v", rig.ui.DraggedEnv, rig.ui.DragEnvActive)
	}
	ops = rig.frame(dragTo(x, ey+10+float32(2*eh)))
	img = rig.shoot("dragholder-env_900x600", ops)
	expectHolderColor(t, img, 200, int(ey), "env")
	rig.frame(release(x, ey+10+float32(2*eh)))
}

func TestEnvRowPopupShots(t *testing.T) {
	sz := image.Pt(900, 500)
	twoEnvs := func(ui *AppUI) {
		ui.SidebarSection = "requests"
		e2 := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "env2", Name: "Staging", HighlightColor: "#22c55e"}}
		e2.InitEditor()
		ui.Environments = append(ui.Environments, e2)
	}
	renderScene(t, scene{"envrow-editing", func(ui *AppUI) {
		twoEnvs(ui)
		ui.EditingEnv = ui.Environments[1]
	}}, sz)
	renderScene(t, scene{"envrow-colorpicker", func(ui *AppUI) {
		twoEnvs(ui)
		ui.EnvColorEnvID = "env2"
		ui.EnvColorPicker.Open(colorpicker.KindEnv, 0, environments.HighlightColor(ui.Environments[1].Data), colorpicker.Anchor{X: 200, Y: 120})
	}}, sz)
	renderScene(t, scene{"envrow-nodemenu-ref", func(ui *AppUI) {
		twoEnvs(ui)
		ui.VisibleCols[1].MenuOpen = true
	}}, sz)
}

func TestFlowBodySelectionSeamsRealFonts(t *testing.T) {
	setupTestConfigDir(t)
	sz := image.Pt(1200, 800)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skip(err)
	}
	defer win.Release()
	var r input.Router
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "flows"
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Source: r.Source(), Now: time.Now()}
		ui.LayoutApp(gtx)
		r.Frame(ops)
		if err := win.Frame(ops); err != nil {
			t.Fatal(err)
		}
	}
	shot := func() *image.RGBA {
		img := image.NewRGBA(image.Rectangle{Max: sz})
		if err := win.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		return img
	}
	frame()
	frame()
	ed := ui.Flow
	n := flow.NewNode(flow.KindRequest, 120, 120)
	n.BodyEd.SetText("aaaa    aaaa\nbbbb    bbbb\ncccc    cccc\ndddd    dddd")
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	frame()
	frame()
	plain := shot()
	ce := n.CanvasBodyEditor()
	r.Source().Execute(key.FocusCmd{Tag: ce})
	frame()
	ce.SetCaret(0, ce.Len())
	frame()
	frame()
	sel := shot()

	var diffRect image.Rectangle
	first := true
	for y := 0; y < sz.Y; y++ {
		for x := 0; x < sz.X; x++ {
			if plain.RGBAAt(x, y) != sel.RGBAAt(x, y) {
				p := image.Rect(x, y, x+1, y+1)
				if first {
					diffRect, first = p, false
				} else {
					diffRect = diffRect.Union(p)
				}
			}
		}
	}
	if first {
		t.Fatal("no selection highlight found")
	}
	x := diffRect.Min.X + 3
	firstHi, lastHi := -1, -1
	var rows []bool
	for y := diffRect.Min.Y; y < diffRect.Max.Y; y++ {
		hi := plain.RGBAAt(x, y) != sel.RGBAAt(x, y)
		rows = append(rows, hi)
		if hi {
			if firstHi < 0 {
				firstHi = y
			}
			lastHi = y
		}
	}
	gaps := 0
	for i, hi := range rows {
		y := diffRect.Min.Y + i
		if y > firstHi && y < lastHi && !hi {
			gaps++
		}
	}
	t.Logf("diff=%v column x=%d highlight rows %d..%d gap rows=%d", diffRect, x, firstHi, lastHi, gaps)
	if gaps > 0 {
		t.Errorf("selection has %d unhighlighted rows between lines", gaps)
	}
	mid := (diffRect.Min.X + diffRect.Max.X) / 2
	base := sel.RGBAAt(mid, firstHi+2)
	odd := 0
	for y := firstHi; y <= lastHi; y++ {
		if c := sel.RGBAAt(mid, y); c != base {
			odd++
			t.Logf("row %d at the blank column x=%d = %v, want %v", y, mid, c, base)
		}
	}
	if odd > 0 {
		t.Errorf("%d rows of the multi-line selection are tinted differently (double-painted seams)", odd)
	}
}

func httpSplitScenes() []scene {
	mk := func(themeID string, mode int) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = themeID
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = mode
		}
	}
	return []scene{
		{"httpsplit-h-dark", mk("dark", workspace.LayoutModeHoriz)},
		{"httpsplit-v-dark", mk("dark", workspace.LayoutModeVert)},
		{"httpsplit-h-dracula", mk("dracula", workspace.LayoutModeHoriz)},
		{"httpsplit-v-dracula", mk("dracula", workspace.LayoutModeVert)},
		{"httpsplit-h-reqcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			tab.ReqBodyCollapsed = true
		}},
		{"httpsplit-v-reqcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.ReqBodyCollapsed = true
			tab.VStackRatio = 0.01
		}},
		{"httpsplit-v-respcollapsed", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeVert
			tab.RespBodyCollapsed = true
			tab.VStackRatio = 0.99
		}},
	}
}

func TestHTTPSplitScreenshots(t *testing.T) {
	for _, sc := range httpSplitScenes() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}

func menuShotItems() []widgets.MenuItem {
	return []widgets.MenuItem{
		{Label: "Open", Click: new(widget.Clickable)},
		{Label: "Copy", Icon: widgets.IconDup, Shortcut: "Ctrl+C", Click: new(widget.Clickable)},
		{Label: "Pinned", Checked: true, Click: new(widget.Clickable)},
		{Label: "Rename", Icon: widgets.IconRename, Bold: true, Click: new(widget.Clickable)},
		{Label: "/mono/path", Mono: true, Click: new(widget.Clickable)},
		{Label: "Colored", LabelCol: color.NRGBA{R: 90, G: 200, B: 250, A: 255}, Click: new(widget.Clickable)},
		{Separator: true},
		{Label: "Disabled", Disabled: true, Click: new(widget.Clickable)},
		{Label: "Delete", Icon: widgets.IconDel, Shortcut: "Del", Danger: true, Click: new(widget.Clickable)},
	}
}

func encodeMenuShot(t *testing.T, win *headless.Window, ops *op.Ops, name string) {
	t.Helper()
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
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", filepath.Join(dir, name+".png"))
}

func TestMenuStatesShot(t *testing.T) {
	sz := image.Pt(480, 520)
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	items := menuShotItems()
	ops := new(op.Ops)
	gtx := newShotGtx(ops, sz)
	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
	st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
	widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
	st.Pop()

	encodeMenuShot(t, win, ops, fmt.Sprintf("menu-states_%dx%d", sz.X, sz.Y))
}

func TestMenuStatesHoverShot(t *testing.T) {
	sz := image.Pt(480, 520)
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	items := menuShotItems()
	target := items[2].Click
	var r input.Router

	frame := func(evs ...pointer.Event) {
		ops := new(op.Ops)
		gtx := newShotGtx(ops, sz)
		gtx.Source = r.Source()
		for _, e := range evs {
			r.Queue(e)
		}
		paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
		st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
		widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
		st.Pop()
		r.Frame(ops)
	}

	var hy float32
	for y := float32(44); y < 260; y += 2 {
		frame(mv(60, y))
		frame()
		if target.Hovered() {
			hy = y
			break
		}
	}
	if hy == 0 {
		t.Skip("could not locate the 3rd item row to hover")
	}

	ops := new(op.Ops)
	gtx := newShotGtx(ops, sz)
	gtx.Source = r.Source()
	r.Queue(mv(60, hy))
	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
	st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
	widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
	st.Pop()

	encodeMenuShot(t, win, ops, fmt.Sprintf("menu-states-hover_%dx%d", sz.X, sz.Y))
	r.Frame(ops)
}

const phantomShotDir = "testdata/phantom"

type phantomDriver struct {
	t  *testing.T
	ui *AppUI
	r  input.Router
	sz image.Point
}

func newPhantomDriver(t *testing.T, envHeavy bool) *phantomDriver {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"

	root := &collections.CollectionNode{Name: "Sample API", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "col1", Name: "Sample API", Root: root}
	root.Collection = col
	sub := &collections.CollectionNode{Name: "Endpoints", IsFolder: true, Expanded: true, Parent: root, Depth: 1, Collection: col}
	root.Children = append(root.Children, sub)
	for i := 0; i < 40; i++ {
		sub.Children = append(sub.Children, &collections.CollectionNode{
			Name:    fmt.Sprintf("Request %02d", i),
			Request: &model.ParsedRequest{Name: fmt.Sprintf("Request %02d", i), Method: "GET"},
			Parent:  sub, Depth: 2, Collection: col,
		})
	}
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	nEnv := 3
	if envHeavy {
		nEnv = 30
		ui.ColsExpanded = false
		ui.ScriptsExpanded = false
	}
	for i := 0; i < nEnv; i++ {
		env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
			ID: fmt.Sprintf("env%02d", i), Name: fmt.Sprintf("Environment %02d", i), HighlightColor: "#3b82f6",
		}}
		env.InitEditor()
		ui.Environments = append(ui.Environments, env)
	}

	return &phantomDriver{t: t, ui: ui, sz: image.Pt(1100, 520)}
}

func (d *phantomDriver) gtx(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(d.sz),
		Now:         fixedTime,
		Source:      d.r.Source(),
	}
}

func (d *phantomDriver) tick(events ...pointer.Event) {
	ops := new(op.Ops)
	for _, e := range events {
		d.r.Queue(e)
	}
	d.ui.LayoutApp(d.gtx(ops))
	d.r.Frame(ops)
}

func (d *phantomDriver) settle(events ...pointer.Event) { d.tick(events...); d.tick() }

func mv(x, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)}
}
func scroll(x, y, dy float32) pointer.Event {
	return pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(x, y), Scroll: f32.Pt(0, dy)}
}

func (d *phantomDriver) hoveredCols() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.RowHovered {
			out = append(out, n.Name)
		}
	}
	return out
}
func (d *phantomDriver) hoveredEnvs() []string {
	var out []string
	for _, e := range d.ui.Environments {
		if e.RowHovered {
			out = append(out, e.Data.Name)
		}
	}
	return out
}
func (d *phantomDriver) stickyHovered() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.StickyHovered {
			out = append(out, n.Name)
		}
	}
	return out
}
func (d *phantomDriver) menuHoveredCols() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.MenuHovered {
			out = append(out, n.Name)
		}
	}
	return out
}

func (d *phantomDriver) shootOps(name string, ops *op.Ops) {
	win, err := headless.NewWindow(d.sz.X, d.sz.Y)
	if err != nil {
		d.t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()
	if err := win.Frame(ops); err != nil {
		d.t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		d.t.Fatalf("screenshot: %v", err)
	}
	if err := os.MkdirAll(phantomShotDir, 0o755); err != nil {
		d.t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(phantomShotDir, name+".png"))
	if err != nil {
		d.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		d.t.Fatal(err)
	}
	d.t.Logf("wrote %s", filepath.Join(phantomShotDir, name+".png"))
}

func TestPhantomHoverControl(t *testing.T) {
	d := newPhantomDriver(t, false)
	var hy float32
	for y := float32(40); y < 300; y += 4 {
		d.settle(mv(120, y))
		if len(d.hoveredCols()) == 1 {
			hy = y
			break
		}
		d.settle(mv(900, 300))
	}
	d.settle(mv(120, hy))
	if got := d.hoveredCols(); len(got) != 1 {
		t.Fatalf("expected one hovered row, got %v", got)
	}
	d.settle(mv(900, 300))
	if got := d.hoveredCols(); len(got) != 0 {
		t.Fatalf("control failed: rows still hovered after moving away: %v", got)
	}
}

func TestPhantomHoverResizeLag(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 4; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	const cy = 180
	d.settle(mv(120, cy))
	before := d.hoveredCols()
	if len(before) != 1 {
		t.Skipf("need exactly one hovered row, got %v", before)
	}
	target := before[0]

	d.ui.ColList.Position.First += 2

	ops := new(op.Ops)
	d.ui.LayoutApp(d.gtx(ops))
	during := d.hoveredCols()
	d.shootOps("fixed_resize_shift", ops)
	d.r.Frame(ops)

	settledOps := new(op.Ops)
	d.ui.LayoutApp(d.gtx(settledOps))
	settled := d.hoveredCols()
	d.r.Frame(settledOps)

	t.Logf("cursor fixed at y=%d (was over %q before shift)  |  shift frame paints %v  ->  next frame paints %v", cy, target, during, settled)
	if len(during) != 1 || len(settled) != 1 || during[0] != settled[0] {
		t.Fatalf("hover lagged the content shift: shift frame painted %v, next frame painted %v (want one identical row, no lag)", during, settled)
	}
	if during[0] == target {
		t.Fatalf("after shifting the list under a fixed cursor the highlight is still on the pre-shift row %q (phantom not fixed)", target)
	}
	t.Logf("FIXED: shift frame already highlights %v (the row now under the cursor), no phantom", during)
}

func TestPhantomHoverEnvResizeLag(t *testing.T) {
	d := newPhantomDriver(t, true)
	for i := 0; i < 4; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	const cy = 180
	d.settle(mv(120, cy))
	before := d.hoveredEnvs()
	if len(before) != 1 {
		t.Skipf("need exactly one hovered env row, got %v", before)
	}

	target := before[0]
	d.ui.EnvList.Position.First += 2
	ops := new(op.Ops)
	d.ui.LayoutApp(d.gtx(ops))
	during := d.hoveredEnvs()
	d.shootOps("fixed_env_resize_shift", ops)
	d.r.Frame(ops)

	settledOps := new(op.Ops)
	d.ui.LayoutApp(d.gtx(settledOps))
	settled := d.hoveredEnvs()
	d.r.Frame(settledOps)

	t.Logf("cursor fixed at y=%d (was over %q)  |  env shift frame paints %v  ->  next frame paints %v", cy, target, during, settled)
	if len(during) != 1 || len(settled) != 1 || during[0] != settled[0] {
		t.Fatalf("env hover lagged the content shift: shift frame painted %v, next frame painted %v (want identical, no lag)", during, settled)
	}
	t.Logf("FIXED in Environments: shift frame already highlights %v, no phantom", during)
}

func TestStickyBandHoverNoLag(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 6; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()
	if d.ui.ColList.Position.First == 0 {
		t.Skip("list did not scroll; no band")
	}

	var bandY float32
	for y := float32(48); y < 90; y += 2 {
		d.settle(mv(120, y))
		if len(d.stickyHovered()) == 1 {
			bandY = y
			break
		}
	}
	if bandY == 0 {
		t.Skip("could not hover a band row")
	}
	t.Logf("band hovered at y=%v: %v", bandY, d.stickyHovered())

	d.ui.ColList.Position.First += 3

	ops := new(op.Ops)
	d.ui.LayoutApp(d.gtx(ops))
	during := d.stickyHovered()
	d.r.Frame(ops)

	d.tick()
	settled := d.stickyHovered()

	t.Logf("after shift: band shift-frame=%v  next-frame=%v", during, settled)
	if len(during) > 1 {
		t.Fatalf("more than one band row highlighted at once: %v", during)
	}
	if strings.Join(during, ",") != strings.Join(settled, ",") {
		t.Fatalf("sticky band hover lagged the shift: shift frame=%v, next frame=%v", during, settled)
	}
}

func TestStickyBandHoverControl(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 6; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()
	for y := float32(48); y < 90; y += 2 {
		d.settle(mv(120, y))
		if len(d.stickyHovered()) == 1 {
			break
		}
	}
	d.settle(mv(900, 300))
	if got := d.stickyHovered(); len(got) != 0 {
		t.Fatalf("band row stayed highlighted after cursor left: %v", got)
	}
}

func TestMenuIconHoverZone(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 3; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	var rowY float32
	for y := float32(60); y < 300; y += 4 {
		d.settle(mv(40, y))
		if len(d.hoveredCols()) == 1 {
			rowY = y
			break
		}
	}
	if rowY == 0 {
		t.Skip("no row found")
	}

	d.settle(mv(40, rowY))
	if got := d.menuHoveredCols(); len(got) != 0 {
		t.Fatalf("menu icon hovered while cursor is on the left of the row: %v", got)
	}
	rowName := d.hoveredCols()

	var menuOn []string
	for x := float32(d.ui.SidebarWidth); x > float32(d.ui.SidebarWidth)-60; x -= 2 {
		d.settle(mv(x, rowY))
		if len(d.menuHoveredCols()) == 1 {
			menuOn = d.menuHoveredCols()
			break
		}
	}
	if len(menuOn) == 0 {
		t.Fatalf("could not hover the ⋮ zone on the right of row %v", rowName)
	}
	t.Logf("⋮ zone hovered for %v", menuOn)
	if strings.Join(menuOn, ",") != strings.Join(rowName, ",") {
		t.Fatalf("⋮ hover is on a different row (%v) than the row hover (%v)", menuOn, rowName)
	}
}

func TestWheelOverScrollbar(t *testing.T) {
	d := newPhantomDriver(t, false)
	d.settle()

	var rowY float32
	for y := float32(60); y < 300; y += 4 {
		d.settle(mv(40, y))
		if len(d.hoveredCols()) == 1 {
			rowY = y
			break
		}
	}
	if rowY == 0 {
		t.Skip("no row found")
	}

	maxRowX := float32(0)
	for x := float32(d.ui.SidebarWidth); x > 0; x -= 1 {
		d.settle(mv(x, rowY))
		if len(d.hoveredCols()) == 1 {
			maxRowX = x
			break
		}
	}
	if maxRowX == 0 {
		t.Skip("could not locate the body's right edge")
	}
	scrollbarX := maxRowX + 5

	d.settle(mv(scrollbarX, rowY))
	if got := d.hoveredCols(); len(got) != 0 {
		t.Fatalf("expected to be over the scrollbar (no row hover) at x=%v, got %v", scrollbarX, got)
	}

	beforeFirst, beforeOff := d.ui.ColList.Position.First, d.ui.ColList.Position.Offset
	d.tick(scroll(scrollbarX, rowY, 60))
	d.tick()
	afterFirst, afterOff := d.ui.ColList.Position.First, d.ui.ColList.Position.Offset

	t.Logf("scroll over scrollbar at x=%v: First %d->%d Offset %d->%d", scrollbarX, beforeFirst, afterFirst, beforeOff, afterOff)
	if afterFirst == beforeFirst && afterOff == beforeOff {
		t.Fatalf("wheel over the scrollbar did not scroll the list (First %d, Offset %d unchanged)", beforeFirst, beforeOff)
	}
}

func startEchoWSServer(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					_ = c.Close()
					return
				}
				res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{})
				if err != nil {
					_ = c.Close()
					return
				}
				conn := res.Conn
				for {
					op, payload, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if op == ws.OpText || op == ws.OpBinary {
						_ = conn.WriteMessage(op, payload)
					}
				}
			}(c)
		}
	}()
	return "ws://" + l.Addr().String()
}

func protoParityScenes(t *testing.T) []scene {
	wsURL := startEchoWSServer(t)
	return []scene{
		{"pp-http", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			respTab(ui)
		}},
		{"pp-ws-open", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText(wsURL)
			tab.AddHeader("Origin", "https://example.com")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			tab.WSConnect(context.Background(), nil, nil, nil)
			deadline := time.Now().Add(3 * time.Second)
			for s.State() != workspace.WSStateOpen && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
			tab.WSSendText(`{"hello":"world"}`)
			time.Sleep(150 * time.Millisecond)
			s.Selected = 1
		}},
		{"pp-gql", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodGraphQL
			tab.URLInput.SetText("https://api.example.com/graphql")
			tab.AddHeader("Authorization", "Bearer abcdef")
			tab.HeadersExpanded = true
			g := tab.EnsureGQL()
			g.Query.SetText("query Users($limit: Int) {\n  users(limit: $limit) {\n    id\n    name\n  }\n}")
			g.Variables.SetText("{\n  \"limit\": 50\n}")
			tab.RespEditor.SetText("{\n  \"data\": {\n    \"users\": [\n      {\"id\": 1, \"name\": \"alice\"}\n    ]\n  }\n}")
			tab.Status = "Ready"
		}},
		{"pp-flow-ws", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			ui.SidebarSection = "flows"
			ui.Flow = flow.NewEditor()
			ui.Flow.AddRequestNode(flow.TabRequest{
				Name:      "Ping socket",
				Kind:      flow.KindWSRequest,
				URL:       "wss://api.example.com/ws",
				Headers:   [][2]string{{"Origin", "https://example.com"}},
				Subprotos: []string{"graphql-ws"},
				WSMessage: `{"type":"ping"}`,
				WSOpcode:  "TEXT",
			})
			ui.Flow.AddRequestNode(flow.TabRequest{
				Name:     "Fetch users",
				Kind:     flow.KindGQLRequest,
				URL:      "https://api.example.com/graphql",
				GQLQuery: "query { users { id } }",
				GQLVars:  `{"limit": 10}`,
			})
		}},
		{"pp-tabctx", func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			respTab(ui)
			ui.TabBar.TabCtxMenuOpen = true
			ui.TabBar.TabCtxMenuIdx = 0
			ui.TabBar.TabCtxMenuPos = f32.Pt(40, 30)
		}},
	}
}

func TestProtoParityScreenshots(t *testing.T) {
	for _, sc := range protoParityScenes(t) {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc, image.Point{X: 1280, Y: 800})
		})
	}
}

func diffBox(a, b *image.RGBA, r image.Rectangle) image.Rectangle {
	var out image.Rectangle
	first := true
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				p := image.Rect(x, y, x+1, y+1)
				if first {
					out, first = p, false
				} else {
					out = out.Union(p)
				}
			}
		}
	}
	return out
}

func TestRowButtonHoverShots(t *testing.T) {
	for _, th := range []string{"dark", "light"} {
		t.Run(th, func(t *testing.T) { runRowButtonHoverShots(t, th) })
	}
}

func runRowButtonHoverShots(t *testing.T, themeID string) {
	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "COLLECTION", IsFolder: true, Expanded: true}
	f1 := &collections.CollectionNode{Name: "Folder-1", IsFolder: true, Expanded: true}
	for i := 0; i < 30; i++ {
		f1.Children = append(f1.Children, mkReq(fmt.Sprintf("f1-req-%d", i)))
	}
	root.Children = []*collections.CollectionNode{f1}
	for i := 0; i < 5; i++ {
		root.Children = append(root.Children, mkReq(fmt.Sprintf("root-req-%d", i)))
	}
	col := &collections.ParsedCollection{ID: "hov", Name: "COLLECTION", Root: root}
	collections.AssignParents(root, nil, col)

	setupTestConfigDir(t)
	for _, name := range []string{"script-one", "script-two"} {
		ed := flow.NewEditor()
		ed.CreateNew()
		if err := flow.RenameScenario(ed.Scenario.ID, name); err != nil {
			t.Fatal(err)
		}
	}
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Settings.Theme = themeID
	settings.Apply(ui.Theme, ui.Settings)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.EnvsExpanded = true
	ui.ScriptsExpanded = true
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.Environments = []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Development"}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Production"}},
	}
	ui.UpdateVisibleCols()

	sz := image.Pt(900, 700)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	var geomNames []string
	sidebar.DebugBandGeom = func(names []string, _ []int, _ int) { geomNames = names }
	defer func() { sidebar.DebugBandGeom = nil }()

	r := new(input.Router)
	now := fixedTime
	gtxFor := func(ops *op.Ops) layout.Context {
		return layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()}
	}
	dir := filepath.Join("testdata", "rowhover", themeID)
	os.MkdirAll(dir, 0o755)
	crop := image.Rect(0, 0, 0, 0)
	save := func(name string, src *image.RGBA) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, src.SubImage(crop))
	}
	frame := func(events ...pointer.Event) *image.RGBA {
		for _, e := range events {
			r.Queue(e)
		}
		now = now.Add(16 * time.Millisecond)
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
		win.Frame(ops)
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		win.Screenshot(img)
		return img
	}
	move := func(x, y float32) *image.RGBA {
		frame(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		return frame()
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	w := ui.SidebarWidth
	crop = image.Rect(0, 0, w+8, sz.Y)
	menuX := float32(w - 19)
	gearX := float32(w - 63)
	swatchX := float32(w - 41)
	labelX := float32(60)
	t.Logf("sidebar width %d", w)

	report := func(label string, x, y float32, hovered func() bool) {
		base := move(labelX, y)
		on := move(x, y)
		if hovered != nil && !hovered() {
			t.Errorf("%s: pointer at (%v,%v) does not hover the button", label, x, y)
		}
		box := diffBox(base, on, image.Rect(0, int(y)-40, w, int(y)+40))
		t.Logf("%-14s at y=%v: hover diff box %v (size %dx%d)", label, y, box, box.Dx(), box.Dy())
		save(label, on)
	}

	findY := func(cond func() bool, x float32) float32 {
		for y := float32(0); y < float32(sz.Y); y += 2 {
			move(x, y)
			if cond() {
				return y
			}
		}
		return -1
	}

	req := f1.Children[0]
	if y := findY(func() bool { return req.MenuHovered }, menuX); y < 0 {
		t.Fatal("node menu button never hovered")
	} else {
		report("node_menu", menuX, y+4, func() bool { return req.MenuBtn.Hovered() })
	}

	e2 := ui.Environments[1]
	if y := findY(func() bool { return e2.MenuHovered }, menuX); y < 0 {
		t.Fatal("env menu button never hovered")
	} else {
		report("env_menu", menuX, y+4, func() bool { return e2.MenuBtn.Hovered() })
		report("env_gear", gearX, y+4, func() bool { return e2.QuickEditBtn.Hovered() })
		report("env_swatch", swatchX, y+4, func() bool { return e2.SelectBtn.Hovered() })
	}

	if len(ui.ScriptRows) < 2 {
		t.Fatalf("script rows = %d, want 2", len(ui.ScriptRows))
	}
	s2 := ui.ScriptRows[1]
	if y := findY(func() bool { return s2.MenuHovered }, menuX); y < 0 {
		t.Fatal("script menu button never hovered")
	} else {
		report("script_menu", menuX, y+4, func() bool { return s2.MenuBtn.Hovered() })
	}

	for step := 0; step < 40; step++ {
		frame(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 200), Scroll: f32.Pt(0, 3)})
		for _, nm := range geomNames {
			if nm == f1.Name && ui.ColList.Position.First > 3 {
				y := findY(func() bool { return f1.StickyHovered && f1.StickyMenuBtn.Hovered() }, menuX)
				if y < 0 {
					t.Fatal("band menu button never hovered")
				}
				report("band_menu", menuX, y+4, func() bool { return f1.StickyMenuBtn.Hovered() })
				return
			}
		}
	}
	t.Fatal("Folder-1 never entered the sticky band")
}

var shotSizes = []image.Point{
	{X: 1280, Y: 800},
	{X: 480, Y: 360},
}

var fixedTime = time.Unix(1700000000, 0)

type scene struct {
	name  string
	setup func(*AppUI)
}

func seedTestData(ui *AppUI) {
	req := &model.ParsedRequest{Name: "Get users", Method: "GET", URL: "{{base_url}}/users"}
	root := &collections.CollectionNode{Name: "Sample API", IsFolder: true, Expanded: true}
	child := &collections.CollectionNode{Name: "Get users", Request: req, Parent: root, Depth: 1}
	root.Children = []*collections.CollectionNode{child}
	col := &collections.ParsedCollection{ID: "col1", Name: "Sample API", Root: root}
	root.Collection = col
	child.Collection = col
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
		ID:             "env1",
		Name:           "Production",
		Vars:           []model.EnvVar{{Key: "base_url", Value: "https://api.example.com"}},
		HighlightColor: "#3b82f6",
	}}
	env.InitEditor()
	ui.Environments = []*environments.EnvironmentUI{env}
	ui.ActiveEnvID = "env1"
	ui.RefreshActiveEnv()
}

func withTab(ui *AppUI) {
	ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("Get users")}
	ui.ActiveIdx = 0
}

func settingsScene(cat int) func(*AppUI) {
	return func(ui *AppUI) {
		ui.SettingsOpen = true
		if ui.SettingsState == nil {
			ui.SettingsState = settings.NewEditor(ui.Settings)
		}
		ui.SettingsState.Category = cat
	}
}

func sceneList() []scene {
	return []scene{
		{"requests-empty", func(ui *AppUI) { ui.SidebarSection = "requests" }},
		{"requests-tab", func(ui *AppUI) { ui.SidebarSection = "requests"; withTab(ui) }},
		{"search-response", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.RespEditor.SetText("{\n  \"users\": [\n    {\"id\": 1, \"name\": \"alice\"},\n    {\"id\": 2, \"name\": \"bob\"},\n    {\"id\": 3, \"name\": \"carol\"}\n  ],\n  \"count\": 3\n}")
			tab.RespSearch.Open = true
			tab.RespSearch.Editor.SetText("name")
		}},
		{"var-body", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.ReqEditor.SetText("{\n  \"url\": \"{{base_url}}/users\",\n  \"token\": \"{{missing_var}}\",\n  \"raw\": {{base_url}}\n}")
		}},
		{"search-request", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.ReqEditor.SetText("{\n  \"name\": \"alice\",\n  \"role\": \"name-holder\",\n  \"nickname\": \"ally\"\n}")
			tab.ReqSearch.Open = true
			tab.ReqSearch.Editor.SetText("name")
		}},
		{"ws-tab", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("WS")}
			ui.ActiveIdx = 0
			tab := ui.Tabs[0]
			tab.Method = workspace.MethodWS
			tab.URLInput.SetText("wss://api.oneme.ru/websocket")
			tab.AddHeader("Origin", "https://web.max.ru")
			s := tab.EnsureWS()
			s.OptionsExpanded = true
			s.UseMsgpackProto = true
			s.AddSubprotocol("graphql-transport-ws")
			s.ProtoCmdEditor.SetText("6")
			s.ComposerEditor.SetText("{\n  \"hello\": \"world\"\n}")
		}},
		{"req-params", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.URLInput.SetText("https://api.example.com/users?page=2&limit=50&sort=name")
			tab.ReqSubTab = 1
			tab.HeadersExpanded = true
		}},
		{"req-auth", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.ReqSubTab = 2
			tab.AuthType = 2
			tab.AuthUser.SetText("admin")
			tab.AuthPass.SetText("s3cr3t")
			tab.HeadersExpanded = true
			tab.FitHeaders = true
		}},
		{"req-cookies", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			withTab(ui)
			tab := ui.Tabs[0]
			tab.ReqSubTab = 3
			for _, kv := range [][2]string{{"session_id", "abc123"}, {"theme", "dark"}} {
				c := &workspace.HeaderItem{}
				c.Key.SetText(kv[0])
				c.Value.SetText(kv[1])
				tab.Cookies = append(tab.Cookies, c)
			}
			tab.HeadersExpanded = true
		}},
		{"flows", func(ui *AppUI) { ui.SidebarSection = "flows" }},
		{"mitm", func(ui *AppUI) { ui.SidebarSection = "mitm" }},
		{"mitm-populated", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			st := &ui.MITM
			st.Ensure()
			f := st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcForward, Method: "GET",
				Host: "api.example.com", Port: "443", Path: "/users?id=7", URL: "https://api.example.com/users?id=7",
				StatusCode: 200, Status: "200 OK", RespSize: 1820, Started: fixedTime.Add(-40 * time.Millisecond), Ended: fixedTime,
				ReqHeaders:  [][2]string{{"Host", "api.example.com"}, {"Accept", "application/json"}},
				RespHeaders: [][2]string{{"Content-Type", "application/json"}, {"Server", "nginx"}},
				ReqBody:     []byte(`{"q":"x"}`), RespBody: []byte(`{"users":[{"id":7,"name":"Ada"}]}`)})
			st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcReverse, TargetDomain: "shop.example.com",
				Method: "POST", Host: "shop.example.com", Path: "/cart", StatusCode: 302, Status: "302 Found",
				RespSize: 64, Started: fixedTime.Add(-12 * time.Millisecond), Ended: fixedTime})
			st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcForward, Method: "GET",
				Host: "cdn.example.com", Path: "/app.js", StatusCode: 404, Status: "404", RespSize: 12,
				Started: fixedTime.Add(-5 * time.Millisecond), Ended: fixedTime})
			st.Store.SetAnnotation(f.ID, "green", "login call")
			st.Selected = f.ID
			st.Proxy.Targets.Add(&mitm.Target{Domain: "shop.example.com", Upstream: mitm.UpstreamAuto, TLS: mitm.TLSDecrypt})
			st.Proxy.MR.Add(mitm.MatchReplaceRule{Enabled: true, Type: mitm.MRResponse, Area: mitm.MRHeader, Pattern: "Content-Security-Policy", Comment: "strip CSP"})
			st.Proxy.ScopeR.Add(mitm.ScopeRule{Enabled: true, Kind: mitm.ScopeInclude, Field: "host", Pattern: "example.com"})
			st.SecTargetsOpen, st.SecMROpen, st.SecScopeOpen = true, true, true
			st.SecTLSOpen = false
		}},
		{"mitm-binary", func(ui *AppUI) {
			ui.SidebarSection = "mitm"
			st := &ui.MITM
			st.Ensure()
			png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x10\x00\x00\x00\x10\x08\x06\x00\x00\x00\x1f\xf3\xffa\x00\x00\x00\x01sRGB\x00\xae\xce\x1c\xe9")
			f := st.Store.Add(&mitm.Flow{Kind: mitm.FlowHTTP, Src: mitm.SrcForward, Method: "GET",
				Host: "cdn.example.com", Port: "443", Path: "/logo.png", URL: "https://cdn.example.com/logo.png",
				StatusCode: 200, Status: "200 OK", RespSize: int64(len(png)), Started: fixedTime.Add(-20 * time.Millisecond), Ended: fixedTime,
				ReqHeaders:  [][2]string{{"Host", "cdn.example.com"}, {"Accept", "image/*"}},
				RespHeaders: [][2]string{{"Content-Type", "image/png"}},
				RespBody:    png})
			st.Selected = f.ID
			st.ActTab = 1
			st.RenderMode = 1
			st.SecTab = 1
			st.BodySearch.Open = true
			st.BodySearch.WholeWord = true
			st.BodySearch.Editor.SetText("49")
		}},
		{"netlimit", func(ui *AppUI) { ui.SidebarSection = "netlimit" }},
		{"env-editor", func(ui *AppUI) { ui.EditingEnv = ui.Environments[0] }},
		{"settings-appearance", settingsScene(0)},
		{"settings-sizes", settingsScene(1)},
		{"settings-http", settingsScene(2)},
		{"settings-advanced", settingsScene(3)},
		{"sidebar-collapsed", func(ui *AppUI) { ui.Settings.HideSidebar = true; withTab(ui) }},
		{"tabbar-hidden", func(ui *AppUI) { ui.Settings.HideTabBar = true; withTab(ui) }},
		{"overlay-tab-ctx", func(ui *AppUI) {
			withTab(ui)
			ui.TabBar.TabCtxMenuOpen = true
			ui.TabBar.TabCtxMenuIdx = 0
			ui.TabBar.TabCtxMenuPos = f32.Pt(40, 20)
		}},
		{"overlay-cols-menu", func(ui *AppUI) { withTab(ui); ui.ColsMenuOpen = true }},
		{"overlay-envs-menu", func(ui *AppUI) { withTab(ui); ui.EnvsMenuOpen = true }},
		{"overlay-env-colorpicker", func(ui *AppUI) {
			withTab(ui)
			ui.EnvColorEnvID = "env1"
			ui.EnvColorPicker.Open(colorpicker.KindEnv, 0, color.NRGBA{R: 59, G: 130, B: 246, A: 255}, colorpicker.Anchor{X: 120, Y: 420})
		}},
		{"overlay-send-menu", func(ui *AppUI) { withTab(ui); ui.Tabs[0].SendMenuOpen = true }},
		{"overlay-method-list", func(ui *AppUI) { withTab(ui); ui.Tabs[0].MethodListOpen = true }},
		{"overlay-protocol-list", func(ui *AppUI) { withTab(ui); ui.Tabs[0].ProtocolListOpen = true }},
		{"requests-multitab", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Tabs = []*workspace.RequestTab{
				workspace.NewRequestTab("Get users"),
				workspace.NewRequestTab("Create user"),
				workspace.NewRequestTab("Delete user"),
				workspace.NewRequestTab("List orders"),
			}
			ui.ActiveIdx = 1
		}},
		{"tabbar-limited-rows", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Settings.LimitTabRows = true
			ui.Settings.MaxTabRows = 3
			ui.Tabs = nil
			for i := 0; i < 40; i++ {
				ui.Tabs = append(ui.Tabs, workspace.NewRequestTab(fmt.Sprintf("Request number %d", i+1)))
			}
			ui.ActiveIdx = 38
		}},
		{"tabbar-expanded-rows", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Settings.LimitTabRows = true
			ui.Settings.MaxTabRows = 3
			ui.Tabs = nil
			for i := 0; i < 40; i++ {
				ui.Tabs = append(ui.Tabs, workspace.NewRequestTab(fmt.Sprintf("Request number %d", i+1)))
			}
			ui.ActiveIdx = 2
			ui.TabBar.ExpandRows = true
		}},
		{"tabbar-maxrows1", func(ui *AppUI) {
			ui.SidebarSection = "requests"
			ui.Settings.LimitTabRows = true
			ui.Settings.MaxTabRows = 1
			ui.Tabs = nil
			for i := 0; i < 40; i++ {
				ui.Tabs = append(ui.Tabs, workspace.NewRequestTab(fmt.Sprintf("Request number %d", i+1)))
			}
			ui.ActiveIdx = 39
		}},
	}
}

type regionJSON struct {
	Name string `json:"name"`
	Rect [4]int `json:"rect"`
}

type manifestJSON struct {
	Scene   string       `json:"scene"`
	Size    [2]int       `json:"size"`
	Regions []regionJSON `json:"regions"`
}

func buildRegions(ui *AppUI, probes map[string]layout.Dimensions, sz image.Point) []regionJSON {
	var out []regionJSON
	titleH := 0
	if d, ok := probes["titlebar"]; ok {
		w := d.Size.X
		if w == 0 {
			w = sz.X
		}
		titleH = d.Size.Y
		out = append(out, regionJSON{"titlebar", [4]int{0, 0, w, titleH}})
	}
	if d, ok := probes["content"]; ok {
		w := d.Size.X
		if w == 0 {
			w = sz.X
		}
		out = append(out, regionJSON{"content", [4]int{0, titleH, w, titleH + d.Size.Y}})
	}
	if d, ok := probes["sidebar"]; ok {
		sw := d.Size.X
		sh := d.Size.Y
		out = append(out, regionJSON{"sidebar", [4]int{0, titleH, sw, titleH + sh}})
		divider := 4
		if ui.HideSidebar() {
			divider = 0
		}
		out = append(out, regionJSON{"main", [4]int{sw + divider, titleH, sz.X, titleH + sh}})
	}
	return out
}

func newShotGtx(ops *op.Ops, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Now:         fixedTime,
	}
}

func renderScene(t *testing.T, sc scene, sz image.Point) {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	seedTestData(ui)
	if sc.setup != nil {
		sc.setup(ui)
	}

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	for i := 0; i < 2; i++ {
		ui.LayoutApp(newShotGtx(new(op.Ops), sz))
	}

	probes := map[string]layout.Dimensions{}
	SetProbeRegion(func(name string, d layout.Dimensions) { probes[name] = d })
	ops := new(op.Ops)
	ui.LayoutApp(newShotGtx(ops, sz))
	SetProbeRegion(nil)

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

	name := fmt.Sprintf("%s_%dx%d", sc.name, sz.X, sz.Y)
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	man := manifestJSON{Scene: name, Size: [2]int{sz.X, sz.Y}, Regions: buildRegions(ui, probes, sz)}
	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".layout.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScreenshots(t *testing.T) {
	for _, sz := range shotSizes {
		sz := sz
		for _, sc := range sceneList() {
			sc := sc
			t.Run(fmt.Sprintf("%s_%dx%d", sc.name, sz.X, sz.Y), func(t *testing.T) {
				renderScene(t, sc, sz)
			})
		}
	}
}

func TestScriptDragShot(t *testing.T) {
	rig := newScriptDragRig(t, "One", "Two", "Three")
	h := float32(rig.ui.ScriptRowH())
	x := float32(100)
	y := rig.rowY(0)
	rig.frame(press(x, y))
	rig.frame(dragTo(x, y+10))
	ops := rig.frame(dragTo(x, y+10+1.5*h))

	win, err := headless.NewWindow(rig.sz.X, rig.sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()
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
	f, err := os.Create(filepath.Join(dir, "scriptdrag-mid_900x600.png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestScrollGutterShots(t *testing.T) {
	for _, th := range []string{"dark", "light"} {
		t.Run(th, func(t *testing.T) { runScrollGutterShots(t, th) })
	}
}

func runScrollGutterShots(t *testing.T, themeID string) {
	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "COLLECTION", IsFolder: true, Expanded: true}
	for i := 0; i < 40; i++ {
		root.Children = append(root.Children, mkReq(fmt.Sprintf("req-%d", i)))
	}
	col := &collections.ParsedCollection{ID: "sg", Name: "COLLECTION", Root: root}
	collections.AssignParents(root, nil, col)

	setupTestConfigDir(t)
	for i := 0; i < 30; i++ {
		ed := flow.NewEditor()
		ed.CreateNew()
		if err := flow.RenameScenario(ed.Scenario.ID, fmt.Sprintf("script-%02d", i)); err != nil {
			t.Fatal(err)
		}
	}
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Settings.Theme = themeID
	settings.Apply(ui.Theme, ui.Settings)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.EnvsExpanded = true
	ui.ScriptsExpanded = true
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	for i := 0; i < 30; i++ {
		ui.Environments = append(ui.Environments, &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: fmt.Sprintf("e%d", i), Name: fmt.Sprintf("Environment %02d", i)}})
	}
	ui.SidebarEnvHeight = 150
	ui.SidebarScriptsHeight = 150
	ui.UpdateVisibleCols()
	ui.OpenRequestInTab(root.Children[2])

	sz := image.Pt(900, 700)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	r := new(input.Router)
	now := fixedTime
	gtxFor := func(ops *op.Ops) layout.Context {
		return layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()}
	}
	dir := filepath.Join("testdata", "scrollgutter", themeID)
	os.MkdirAll(dir, 0o755)
	frame := func(events ...pointer.Event) *image.RGBA {
		for _, e := range events {
			r.Queue(e)
		}
		now = now.Add(16 * time.Millisecond)
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
		win.Frame(ops)
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		win.Screenshot(img)
		return img
	}
	move := func(x, y float32) *image.RGBA {
		frame(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
		return frame()
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	w := ui.SidebarWidth
	{
		full := frame()
		f, _ := os.Create(filepath.Join(dir, "full.png"))
		png.Encode(f, full.SubImage(image.Rect(0, 0, w+8, sz.Y)))
		f.Close()
	}
	t.Logf("sidebar width %d; cols first=%d offlast=%d; envs first=%d offlast=%d; scripts first=%d offlast=%d",
		w, ui.ColList.Position.First, ui.ColList.Position.OffsetLast, ui.EnvList.Position.First, ui.EnvList.Position.OffsetLast,
		ui.ScriptList.Position.First, ui.ScriptList.Position.OffsetLast)
	save := func(name string, img *image.RGBA, y int) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, img.SubImage(image.Rect(w-80, y-30, w+8, y+40)))
	}
	rowOf := func(img *image.RGBA, y int) string {
		s := ""
		for x := w - 24; x <= w+1; x++ {
			c := img.RGBAAt(x, y)
			s += fmt.Sprintf(" %02x%02x%02x", c.R, c.G, c.B)
		}
		return s
	}
	findY := func(cond func() bool, x float32) float32 {
		for y := float32(0); y < float32(sz.Y); y += 2 {
			move(x, y)
			if cond() {
				return y
			}
		}
		return -1
	}
	node := root.Children[5]
	if y := findY(func() bool { return node.RowHovered }, 60); y < 0 {
		t.Fatal("node never hovered")
	} else {
		img := move(60, y+4)
		t.Logf("node hover row y=%v:%s", y+4, rowOf(img, int(y+4)))
		save("cols_hover", img, int(y+4))
	}
	active := root.Children[2]
	if y := findY(func() bool { return active.RowHovered }, 60); y >= 0 {
		img := move(60, 5)
		t.Logf("node active row y=%v:%s", y+4, rowOf(img, int(y+4)))
		save("cols_active", img, int(y+4))
	}
	env := ui.Environments[1]
	if y := findY(func() bool { return env.RowHovered }, 60); y < 0 {
		t.Fatal("env never hovered")
	} else {
		img := move(60, y+4)
		t.Logf("env hover row y=%v:%s", y+4, rowOf(img, int(y+4)))
		save("envs_hover", img, int(y+4))
	}
	for i := 0; i < 6; i++ {
		frame(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 200), Scroll: f32.Pt(0, 3)})
	}
	if y := findY(func() bool { return root.StickyHovered }, 60); y < 0 {
		t.Fatal("sticky band row never hovered")
	} else {
		img := move(60, y+4)
		t.Logf("band hover row y=%v:%s", y+4, rowOf(img, int(y+4)))
		save("band_hover", img, int(y+4))
	}
	scr := ui.ScriptRows[1]
	if y := findY(func() bool { return scr.RowHovered }, 60); y < 0 {
		t.Fatal("script never hovered")
	} else {
		img := move(60, y+4)
		t.Logf("script hover row y=%v:%s", y+4, rowOf(img, int(y+4)))
		save("scripts_hover", img, int(y+4))
	}
}

func TestStickyDupMenuShots(t *testing.T) {
	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "COLLECTION", IsFolder: true, Expanded: true}
	f1 := &collections.CollectionNode{Name: "Folder-1", IsFolder: true, Expanded: true}
	for i := 0; i < 6; i++ {
		f1.Children = append(f1.Children, mkReq(fmt.Sprintf("f1-req-%d", i)))
	}
	long := &collections.CollectionNode{Name: "Very long folder name that wraps onto a second line", IsFolder: true, Expanded: true}
	for i := 0; i < 8; i++ {
		long.Children = append(long.Children, mkReq(fmt.Sprintf("long-req-%d", i)))
	}
	root.Children = []*collections.CollectionNode{f1, long}
	for i := 0; i < 25; i++ {
		root.Children = append(root.Children, mkReq(fmt.Sprintf("root-req-%d", i)))
	}
	col := &collections.ParsedCollection{ID: "dup", Name: "COLLECTION", Root: root}
	collections.AssignParents(root, nil, col)

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	sz := image.Pt(900, 600)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	var geomNames []string
	var geomYs []int
	sidebar.DebugBandGeom = func(names []string, ys []int, _ int) { geomNames, geomYs = names, ys }
	defer func() { sidebar.DebugBandGeom = nil }()
	type menuDraw struct {
		name        string
		anchorY, rh int
	}
	var menus []menuDraw
	sidebar.DebugNodeMenu = func(name string, y, rh int) { menus = append(menus, menuDraw{name, y, rh}) }
	defer func() { sidebar.DebugNodeMenu = nil }()

	r := new(input.Router)
	now := fixedTime
	gtxFor := func(ops *op.Ops) layout.Context {
		return layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()}
	}
	dir := filepath.Join("testdata", "sticky")
	os.MkdirAll(dir, 0o755)
	save := func(name string, src *image.RGBA) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, src.SubImage(image.Rect(0, 0, ui.SidebarWidth+8, 260)))
	}
	frame := func(shot string, events ...pointer.Event) {
		for _, e := range events {
			r.Queue(e)
		}
		now = now.Add(16 * time.Millisecond)
		menus = menus[:0]
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
		if shot != "" {
			win.Frame(ops)
			img := image.NewRGBA(image.Rectangle{Max: win.Size()})
			win.Screenshot(img)
			save(shot, img)
		}
	}
	for i := 0; i < 3; i++ {
		frame("")
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	t.Logf("sidebar width %d, long row h=%d, req row h=%d", ui.SidebarWidth, long.RowHeightPx, f1.Children[0].RowHeightPx)

	idxLong := -1
	for i, n := range ui.VisibleCols {
		if n == long {
			idxLong = i
		}
	}
	inBand := func() (int, bool) {
		for i, nm := range geomNames {
			if nm == long.Name {
				return geomYs[i], true
			}
		}
		return 0, false
	}
	shotN := 0
	for step := 0; step < 80; step++ {
		frame("", pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 300), Scroll: f32.Pt(0, 3)})
		y, ok := inBand()
		if !ok {
			continue
		}
		first := ui.ColList.Position.First
		t.Logf("step %02d First=%d Off=%d idxLong=%d band=%v ys=%v longY=%d", step, first, ui.ColList.Position.Offset, idxLong, geomNames, geomYs, y)
		if shotN < 4 {
			frame(fmt.Sprintf("dup_%02d_plain", shotN))
			long.MenuOpen = true
			frame(fmt.Sprintf("dup_%02d_menu", shotN))
			t.Logf("  menus drawn: %v", menus)
			if len(menus) != 1 {
				t.Errorf("menu for %q drawn %d times: %v", long.Name, len(menus), menus)
			}
			if bandBottom := y + 24; menus[0].anchorY != y {
				t.Errorf("menu anchored at %d, want band row y %d (band bottom %d)", menus[0].anchorY, y, bandBottom)
			}
			long.MenuOpen = false
			frame("")
			shotN++
		}
		if first > idxLong+3 {
			break
		}
	}
}

func TestStickyScrollShots(t *testing.T) {
	path := os.Getenv("STICKY_COLLECTION")
	if path == "" {
		t.Skip("set STICKY_COLLECTION to a collection .json to capture sticky screenshots")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no collection at %s: %v", path, err)
	}
	col, err := collections.ParseCollection(bytes.NewReader(data), "shot")
	if err != nil || col == nil {
		t.Fatalf("parse collection: %v", err)
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = []*collections.CollectionUI{{Data: col}}

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	expand(col.Root)
	ui.UpdateVisibleCols()
	t.Logf("visible nodes: %d", len(ui.VisibleCols))

	sz := image.Pt(900, 800)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	var lastRendered []string
	sidebar.DebugSticky = func(first int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	r := new(input.Router)
	now := fixedTime
	gtxFor := func(ops *op.Ops) layout.Context {
		return layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         now,
			Source:      r.Source(),
		}
	}

	for i := 0; i < 3; i++ {
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
	}

	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0

	dir := filepath.Join("testdata", "sticky")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cropW := ui.SidebarWidth + 8
	if cropW > sz.X {
		cropW = sz.X
	}

	save := func(name string, src *image.RGBA) {
		sub := src.SubImage(image.Rect(0, 0, cropW, sz.Y))
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, sub); err != nil {
			t.Fatal(err)
		}
	}

	const step = 4.0
	for frame := 0; frame < 44; frame++ {
		r.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(120, 420),
			Scroll:   f32.Pt(0, step),
		})
		now = now.Add(16 * time.Millisecond)
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
		if err := win.Frame(ops); err != nil {
			t.Fatalf("frame %d: %v", frame, err)
		}
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		if err := win.Screenshot(img); err != nil {
			t.Fatalf("screenshot %d: %v", frame, err)
		}
		name, entering := "?", false
		if f := ui.ColList.Position.First; f >= 0 && f < len(ui.VisibleCols) {
			n := ui.VisibleCols[f]
			name = n.Name
			entering = (n.IsFolder || n.Depth == 0) && n.Expanded &&
				f+1 < len(ui.VisibleCols) && ui.VisibleCols[f+1].Parent == n
		}
		t.Logf("frame %02d reserve=%d bandH=%d: First=%d Offset=%d top=%q entering=%v rendered=%v",
			frame, reserve, bandH,
			ui.ColList.Position.First, ui.ColList.Position.Offset, name, entering, lastRendered)
		save(fmt.Sprintf("scroll_%02d", frame), img)
	}
}

func TestStickyScrollSynthetic(t *testing.T) {
	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "COLLECTION", IsFolder: true, Expanded: true}
	f1 := &collections.CollectionNode{Name: "Folder-1", IsFolder: true, Expanded: true}
	f1.Children = []*collections.CollectionNode{mkReq("f1-req-0"), mkReq("f1-req-1"), mkReq("f1-req-2")}
	f2 := &collections.CollectionNode{Name: "Folder-2-sub", IsFolder: true, Expanded: true}
	for i := 0; i < 4; i++ {
		f2.Children = append(f2.Children, mkReq(fmt.Sprintf("f2-req-%d", i)))
	}
	f1.Children = append(f1.Children, f2)
	for i := 0; i < 4; i++ {
		f1.Children = append(f1.Children, mkReq(fmt.Sprintf("f1-after-%d", i)))
	}
	tail := []*collections.CollectionNode{f1}
	for i := 0; i < 25; i++ {
		tail = append(tail, mkReq(fmt.Sprintf("root-req-%d", i)))
	}
	root.Children = tail
	col := &collections.ParsedCollection{ID: "syn", Name: "COLLECTION", Root: root}
	collections.AssignParents(root, nil, col)

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.UpdateVisibleCols()

	sz := image.Pt(900, 800)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	r := new(input.Router)
	now := fixedTime
	gtxFor := func(ops *op.Ops) layout.Context {
		return layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()}
	}
	for i := 0; i < 3; i++ {
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0

	dir := filepath.Join("testdata", "sticky")
	os.MkdirAll(dir, 0o755)
	cropW := ui.SidebarWidth + 8
	save := func(name string, src *image.RGBA) {
		f, _ := os.Create(filepath.Join(dir, name+".png"))
		defer f.Close()
		png.Encode(f, src.SubImage(image.Rect(0, 0, cropW, sz.Y)))
	}

	abs := func() int {
		p := ui.ColList.Position.Offset
		for i := 0; i < ui.ColList.Position.First && i < len(ui.VisibleCols); i++ {
			h := ui.VisibleCols[i].RowHeightPx
			if h <= 0 {
				h = ui.ColRowH()
			}
			p += h
		}
		return p
	}
	prevEff := abs() - reserve
	for frame := 0; frame < 60; frame++ {
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 4)})
		now = now.Add(16 * time.Millisecond)
		ops := new(op.Ops)
		ui.LayoutApp(gtxFor(ops))
		r.Frame(ops)
		win.Frame(ops)
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		win.Screenshot(img)
		eff := abs() - reserve
		name := "?"
		if f := ui.ColList.Position.First; f >= 0 && f < len(ui.VisibleCols) {
			name = ui.VisibleCols[f].Name
		}
		t.Logf("syn %02d reserve=%d band=%d First=%d Off=%d eff=%d (d%+d) top=%q rendered=%v",
			frame, reserve, bandH, ui.ColList.Position.First, ui.ColList.Position.Offset, eff, eff-prevEff, name, lastRendered)
		prevEff = eff
		save(fmt.Sprintf("syn_%02d", frame), img)
	}
}
