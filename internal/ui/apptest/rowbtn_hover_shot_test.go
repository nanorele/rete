//go:build screenshots

package apptest

import (
	. "rete/internal/ui"

	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"rete/internal/model"
	"rete/internal/ui/collections"
	"rete/internal/ui/environments"
	"rete/internal/ui/flow"
	"rete/internal/ui/settings"
	"rete/internal/ui/sidebar"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

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
