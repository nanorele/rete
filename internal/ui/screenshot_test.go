//go:build screenshots

package ui

import (
	"encoding/json"
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
	"tracto/internal/ui/settings"
	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

const (
	shotW = 1280
	shotH = 800
)

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
	ui.updateVisibleCols()

	env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
		ID:             "env1",
		Name:           "Production",
		Vars:           []model.EnvVar{{Key: "base_url", Value: "https://api.example.com"}},
		HighlightColor: "#3b82f6",
	}}
	env.InitEditor()
	ui.Environments = []*environments.EnvironmentUI{env}
	ui.ActiveEnvID = "env1"
	ui.refreshActiveEnv()
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
		{"flows", func(ui *AppUI) { ui.SidebarSection = "flows" }},
		{"mitm", func(ui *AppUI) { ui.SidebarSection = "mitm" }},
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
			ui.TabBar.TabCtxMenuPos = f32.Pt(220, 64)
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

func buildRegions(ui *AppUI, probes map[string]layout.Dimensions) []regionJSON {
	var out []regionJSON
	titleH := 0
	if d, ok := probes["titlebar"]; ok {
		w := d.Size.X
		if w == 0 {
			w = shotW
		}
		titleH = d.Size.Y
		out = append(out, regionJSON{"titlebar", [4]int{0, 0, w, titleH}})
	}
	if d, ok := probes["content"]; ok {
		w := d.Size.X
		if w == 0 {
			w = shotW
		}
		out = append(out, regionJSON{"content", [4]int{0, titleH, w, titleH + d.Size.Y}})
	}
	if d, ok := probes["sidebar"]; ok {
		sw := d.Size.X
		sh := d.Size.Y
		out = append(out, regionJSON{"sidebar", [4]int{0, titleH, sw, titleH + sh}})
		divider := 4
		if ui.hideSidebar() {
			divider = 0
		}
		out = append(out, regionJSON{"main", [4]int{sw + divider, titleH, shotW, titleH + sh}})
	}
	return out
}

func newShotGtx(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(shotW, shotH)),
		Now:         fixedTime,
	}
}

func renderScene(t *testing.T, sc scene) {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	seedTestData(ui)
	if sc.setup != nil {
		sc.setup(ui)
	}

	win, err := headless.NewWindow(shotW, shotH)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	for i := 0; i < 2; i++ {
		ui.layoutApp(newShotGtx(new(op.Ops)))
	}

	probes := map[string]layout.Dimensions{}
	probeRegion = func(name string, d layout.Dimensions) { probes[name] = d }
	ops := new(op.Ops)
	ui.layoutApp(newShotGtx(ops))
	probeRegion = nil

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

	f, err := os.Create(filepath.Join(dir, sc.name+".png"))
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

	man := manifestJSON{Scene: sc.name, Size: [2]int{shotW, shotH}, Regions: buildRegions(ui, probes)}
	mb, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sc.name+".layout.json"), mb, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScreenshots(t *testing.T) {
	for _, sc := range sceneList() {
		sc := sc
		t.Run(sc.name, func(t *testing.T) {
			renderScene(t, sc)
		})
	}
}
