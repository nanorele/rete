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

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

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
