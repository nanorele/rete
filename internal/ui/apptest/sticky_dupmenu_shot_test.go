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
