//go:build screenshots

package apptest

import (
	. "tracto/internal/ui"

	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/mitm"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

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
