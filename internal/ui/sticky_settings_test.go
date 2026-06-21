package ui

import (
	"image"
	"math/rand"
	"testing"
	"time"

	"tracto/internal/ui/collections"
	"tracto/internal/ui/sidebar"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestStickyMaxLinesSetting(t *testing.T) {
	for _, limit := range []int{1, 2, 3, 4} {
		setupTestConfigDir(t)
		ui := NewAppUI()
		ui.Window = new(app.Window)
		ui.Tabs = nil
		ui.SidebarSection = "requests"
		ui.ColsExpanded = true
		ui.Settings.StickyMaxLines = limit

		rng := rand.New(rand.NewSource(int64(limit) * 7))
		root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		cur := root
		for d := 0; d < 7; d++ {
			sub := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			cur.Children = append(cur.Children, sub)
			for i := 0; i < 5; i++ {
				sub.Children = append(sub.Children, &collections.CollectionNode{Name: randStickyName(rng)})
			}
			cur = sub
		}
		col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
		collections.AssignParents(root, nil, col)
		ui.Collections = []*collections.CollectionUI{{Data: col}}
		ui.updateVisibleCols()

		worst := 0
		sidebar.DebugSticky = func(_ int, names []string) {
			if len(names) > worst {
				worst = len(names)
			}
		}
		defer func() { sidebar.DebugSticky = nil }()

		sz := image.Pt(900, 600)
		now := time.Unix(1700000000, 0)
		r := new(input.Router)
		frame := func() {
			ops := new(op.Ops)
			ui.layoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
			r.Frame(ops)
		}
		for i := 0; i < 3; i++ {
			frame()
		}
		ui.ColList.Position.First = 0
		ui.ColList.Position.Offset = 0
		frame()
		for step := 0; step < 400; step++ {
			before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
			r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
			now = now.Add(16 * time.Millisecond)
			frame()
			if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
				break
			}
		}
		if worst > limit {
			t.Fatalf("StickyMaxLines=%d but the band pinned %d rows", limit, worst)
		}
		if worst == 0 {
			t.Fatalf("StickyMaxLines=%d: band never pinned any row (test did not exercise it)", limit)
		}
	}
}

func TestStickyScrollThroughBand(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	rng := rand.New(rand.NewSource(7))
	root := buildRandomStickyTree(rng, []int{2, 3, 4, 2, 3})
	col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()
	total := len(ui.VisibleCols)
	if total < 40 {
		t.Skipf("tree too small (%d nodes)", total)
	}

	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 600)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.layoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	for i := 0; i < 3; i++ {
		frame()
	}
	ui.ColList.Position.First = 0
	ui.ColList.Position.Offset = 0
	frame()

	maxFirst := 0
	sawBand := false
	for step := 0; step < 2000; step++ {
		before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 8)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH > 0 {
			sawBand = true
		}
		if ui.ColList.Position.First > maxFirst {
			maxFirst = ui.ColList.Position.First
		}
		if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
			break
		}
	}
	if !sawBand {
		t.Skip("band never rendered; cannot exercise scroll-through-band")
	}
	if maxFirst < total-25 {
		t.Fatalf("scrolling over the band stalled the list at First=%d of %d nodes (band swallowed scroll?)", maxFirst, total)
	}
}
