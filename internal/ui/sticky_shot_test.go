//go:build screenshots

package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/sidebar"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestStickyScrollShots(t *testing.T) {
	path := os.Getenv("STICKY_COLLECTION")
	if path == "" {
		path = filepath.Join(os.Getenv("APPDATA"), "tracto", "collections", "4d15febd260a92f83773e39f740a1e2a.json")
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
	ui.updateVisibleCols()
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
		ui.layoutApp(gtxFor(ops))
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
		ui.layoutApp(gtxFor(ops))
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

// TestStickyScrollSynthetic renders a tree with a subfolder that is NOT the
// first child of its parent (sibling requests precede it) and a deeper level,
// to reproduce the root-node and 3rd-level-folder problems.
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
	ui.updateVisibleCols()

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
		ui.layoutApp(gtxFor(ops))
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
				h = ui.colRowH
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
		ui.layoutApp(gtxFor(ops))
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
