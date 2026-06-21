package ui

import (
	"fmt"
	"image"
	"math/rand"
	"sort"
	"testing"
	"time"

	"tracto/internal/model"
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

func randStickyName(rng *rand.Rand) string {
	const al = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6+rng.Intn(8))
	for i := range b {
		b[i] = al[rng.Intn(len(al))]
	}
	return string(b)
}

func stickyDepthOf(n *collections.CollectionNode) int {
	d := 0
	for p := n.Parent; p != nil; p = p.Parent {
		d++
	}
	return d
}

func buildRandomStickyTree(rng *rand.Rand, maxDepths []int) *collections.CollectionNode {
	root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
	var grow func(parent *collections.CollectionNode, depth, maxDepth int)
	grow = func(parent *collections.CollectionNode, depth, maxDepth int) {
		for i, n := 0, 2+rng.Intn(5); i < n; i++ {
			parent.Children = append(parent.Children,
				&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
		}
		if depth >= maxDepth {
			return
		}
		for i, n := 0, 1+rng.Intn(2); i < n; i++ {
			f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			parent.Children = append(parent.Children, f)
			grow(f, depth+1, maxDepth)
		}
	}
	for _, md := range maxDepths {
		f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		root.Children = append(root.Children, f)
		grow(f, 1, md)
	}
	return root
}

func buildShortTailStickyTree(rng *rand.Rand) *collections.CollectionNode {
	root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
	for s := 0; s < 5; s++ {
		d1 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		root.Children = append(root.Children, d1)
		depth := 2 + rng.Intn(3)
		cur := d1
		for d := 0; d < depth; d++ {
			sub := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			cur.Children = append(cur.Children, sub)
			for i, n := 0, 1+rng.Intn(2); i < n; i++ {
				sub.Children = append(sub.Children,
					&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
			}
			cur = sub
		}
		for i := 0; i < 2+rng.Intn(3); i++ {
			d1.Children = append(d1.Children,
				&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
		}
	}
	return root
}

func stickyIsAncestorOrSelf(a, n *collections.CollectionNode) bool {
	for p := n; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

func TestStickyBandPinsOnlyAncestors(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			root := buildShortTailStickyTree(rng)
			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.updateVisibleCols()

			var band []string
			sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
			defer func() { sidebar.DebugSticky = nil }()
			var solidH int
			sidebar.DebugBandSolid = func(s int) { solidH = s }
			defer func() { sidebar.DebugBandSolid = nil }()
			var drawn int
			sidebar.DebugBandGeom = func(n []string, _ []int, _ int) { drawn = len(n) }
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
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

			const rowH = 24
			nameToNode := map[string]*collections.CollectionNode{}
			for _, n := range ui.VisibleCols {
				nameToNode[n.Name] = n
			}

			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				band = band[:0]
				solidH, drawn = 0, 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()

				first := ui.ColList.Position.First
				if first < 0 || first >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[first]
				for _, nm := range band {
					n := nameToNode[nm]
					if n == nil {
						continue
					}
					if !stickyIsAncestorOrSelf(n, top) {
						t.Fatalf("step %d: band pins %q which is NOT an ancestor of the top row %q (band=%v First=%d Off=%d) — the band jumped ahead and would cover the next sibling",
							step, nm, top.Name, band, first, ui.ColList.Position.Offset)
					}
				}
				if drawn > 0 && solidH > drawn*rowH+rowH {
					t.Fatalf("step %d: opaque band %dpx overshoots its %d drawn rows (band=%v First=%d)", step, solidH, drawn, band, first)
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
		})
	}
}

func TestStickyNoFlickerOnExit(t *testing.T) {
	for _, seed := range []int64{1, 7, 42, 100, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			runStickyFlickerScroll(t, seed)
		})
	}
}

func runStickyFlickerScroll(t *testing.T, seed int64) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	ui.Settings.StickyMaxLines = 64

	rng := rand.New(rand.NewSource(seed))
	root := buildRandomStickyTree(rng, []int{1, 1, 2, 3, 1, 4, 2, 5})
	col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	var solidH int
	sidebar.DebugBandSolid = func(s int) { solidH = s }
	defer func() { sidebar.DebugBandSolid = nil }()
	var drawn int
	sidebar.DebugBandGeom = func(names []string, _ []int, _ int) { drawn = len(names) }
	defer func() { sidebar.DebugBandGeom = nil }()

	sz := image.Pt(900, 520)
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

	const rowH = 24
	const gapTol = 12
	seen := map[string]map[int]bool{}
	gapWorst, gapDesc := 0, ""

	step := 0
	for ; step < 4000; step++ {
		before := ui.ColList.Position.First + ui.ColList.Position.Offset
		band = band[:0]
		solidH, drawn = 0, 0
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()

		for _, nm := range band {
			if seen[nm] == nil {
				seen[nm] = map[int]bool{}
			}
			seen[nm][step] = true
		}
		if rows := drawn; rows > 0 {
			if over := solidH - (rows*rowH + gapTol); over > gapWorst {
				gapWorst = over
				gapDesc = fmt.Sprintf("step %d solidH=%d rows=%d band=%v", step, solidH, rows, band)
			}
		}
		if ui.ColList.Position.First+ui.ColList.Position.Offset == before {
			break
		}
	}

	const maxGap = 10
	var flick []string
	worstGap := 0
	for nm, set := range seen {
		lo, hi := 1<<30, -1
		for s := range set {
			if s < lo {
				lo = s
			}
			if s > hi {
				hi = s
			}
		}
		run := 0
		folderWorst := 0
		for s := lo; s <= hi; s++ {
			if set[s] {
				run = 0
			} else {
				run++
				if run > folderWorst {
					folderWorst = run
				}
			}
		}
		if folderWorst > worstGap {
			worstGap = folderWorst
		}
		if folderWorst > maxGap {
			flick = append(flick, fmt.Sprintf("%q dropped out of the band for %d consecutive frames within [%d..%d]", nm, folderWorst, lo, hi))
		}
	}
	t.Logf("seed %d: %d frames, worst single-folder flicker gap = %d frame(s)", seed, step, worstGap)
	if len(flick) > 0 {
		sort.Strings(flick)
		for _, f := range flick {
			t.Log(f)
		}
		t.Fatalf("sticky band flickered (gap > %d frame) for %d folder(s) — a sustained drop, not a 1-frame boundary blip (seed %d)", maxGap, len(flick), seed)
	}
	if gapWorst > 0 {
		t.Fatalf("sticky band opaque fill exceeded its pinned rows by %dpx (empty space): %s", gapWorst, gapDesc)
	}
}

func TestStickySeamlessTransition(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 4; i++ {
				f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
				f.Children = append(f.Children, mkReq())
				root.Children = append(root.Children, f)
			}
			d1 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			d2 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			d3 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 5; i++ {
				d3.Children = append(d3.Children, mkReq())
			}
			d2.Children = append(d2.Children, d3)
			for i := 0; i < 4; i++ {
				d2.Children = append(d2.Children, mkReq())
			}
			d1.Children = append(d1.Children, d2)
			for i := 0; i < 4; i++ {
				d1.Children = append(d1.Children, mkReq())
			}
			root.Children = append(root.Children, d1)
			last := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 6; i++ {
				last.Children = append(last.Children, mkReq())
			}
			root.Children = append(root.Children, last)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.updateVisibleCols()

			depthOf := func(n *collections.CollectionNode) int { return stickyDepthOf(n) }
			d1Idx, d2Idx, d3Idx := -1, -1, -1
			for i, n := range ui.VisibleCols {
				switch n {
				case d1:
					d1Idx = i
				case d2:
					d2Idx = i
				case d3:
					d3Idx = i
				}
			}

			var band []string
			sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
			defer func() { sidebar.DebugSticky = nil }()
			var names []string
			var ys []int
			var bottom int
			sidebar.DebugBandGeom = func(n []string, y []int, b int) {
				names = append(names[:0], n...)
				ys = append(ys[:0], y...)
				bottom = b
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
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

			const rowH = 24
			inName := func(s string) bool {
				for _, n := range names {
					if n == s {
						return true
					}
				}
				return false
			}
			gaps, worst := 0, ""
			d1Checked, d2Checked := false, false
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				band = band[:0]
				names, ys, bottom = names[:0], ys[:0], 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()

				f := ui.ColList.Position.First
				if f < 0 || f >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[f]
				visible := 0
				for _, y := range ys {
					if y+rowH > 2 && y < bottom {
						visible++
					}
				}
				if depthOf(top) >= 2 && visible < 2 {
					gaps++
					if worst == "" {
						worst = fmt.Sprintf("step %d First=%d(%q) depth=%d Off=%d visible=%d ys=%v",
							step, f, top.Name, depthOf(top), ui.ColList.Position.Offset, visible, ys)
					}
				}
				if !d1Checked && f == d1Idx && d2Idx >= 0 && d3Idx >= 0 && ui.ColList.Position.Offset > 0 {
					d1Checked = true
					if !inName(d2.Name) || !inName(d3.Name) {
						t.Fatalf("deep descent lags: top is on the parent %q but its nested subfolders %q/%q are not both in the band (names=%v) — they would dock a node (or two) later",
							d1.Name, d2.Name, d3.Name, names)
					}
				}
				if !d2Checked && f == d2Idx && d3Idx >= 0 && ui.ColList.Position.Offset > 0 {
					d2Checked = true
					if !inName(d3.Name) {
						t.Fatalf("descent late: top is on %q but its first-child subfolder %q is not in the band (names=%v)",
							d2.Name, d3.Name, names)
					}
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			if gaps > 0 {
				t.Fatalf("band collapsed to root-only in %d frame(s) (the sibling-swap gap): %s", gaps, worst)
			}
			if !d1Checked || !d2Checked {
				t.Fatalf("test never exercised the descent (d1Checked=%v d2Checked=%v; idx d1=%d d2=%d d3=%d)", d1Checked, d2Checked, d1Idx, d2Idx, d3Idx)
			}
		})
	}
}

func TestStickyNonFirstSubfolderSwap(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			p := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			a := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			b := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 2+rng.Intn(3); i++ {
				a.Children = append(a.Children, mkReq())
			}
			for i := 0; i < 2+rng.Intn(3); i++ {
				b.Children = append(b.Children, mkReq())
			}
			p.Children = append(p.Children, a, b)
			for i := 0; i < 2; i++ {
				p.Children = append(p.Children, mkReq())
			}
			root.Children = append(root.Children, p)
			q := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			for i := 0; i < 6; i++ {
				q.Children = append(q.Children, mkReq())
			}
			root.Children = append(root.Children, q)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.updateVisibleCols()

			aIdx, bIdx := -1, -1
			for i, n := range ui.VisibleCols {
				switch n {
				case a:
					aIdx = i
				case b:
					bIdx = i
				}
			}

			var names []string
			var bottom int
			sidebar.DebugBandGeom = func(n []string, _ []int, bt int) {
				names = append(names[:0], n...)
				bottom = bt
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 480)
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

			const rowH = 24
			depthOf := func(n *collections.CollectionNode) int { return stickyDepthOf(n) }
			bVisible := false
			dips, dipDesc := 0, ""
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				names, bottom = names[:0], 0
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()
				f := ui.ColList.Position.First
				if f < 0 || f >= len(ui.VisibleCols) {
					continue
				}
				top := ui.VisibleCols[f]
				for _, nm := range names {
					if nm == b.Name {
						bVisible = true
					}
				}
				if depthOf(top) >= 3 && bottom < 3*rowH-2 {
					dips++
					if dipDesc == "" {
						dipDesc = fmt.Sprintf("step %d First=%d(%q) depth=%d bottom=%d names=%v", step, f, top.Name, depthOf(top), bottom, names)
					}
				}
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			if aIdx < 0 || bIdx < 0 {
				t.Fatalf("could not locate subfolders A=%d B=%d", aIdx, bIdx)
			}
			if !bVisible {
				t.Fatal("the non-first subfolder B never appeared in the band")
			}
			if dips > 0 {
				t.Fatalf("band dipped below depth-2 while inside a depth-2 subtree in %d frame(s) (subfolder swap not smooth): %s", dips, dipDesc)
			}
		})
	}
}

func TestStickyDeepChainSlidesInNotPops(t *testing.T) {
	for _, seed := range []int64{1, 2, 3, 7, 13, 42, 99, 2024} {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			setupTestConfigDir(t)
			ui := NewAppUI()
			ui.Window = new(app.Window)
			ui.Tabs = nil
			ui.SidebarSection = "requests"
			ui.ColsExpanded = true

			rng := rand.New(rand.NewSource(seed))
			mkReq := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}}
			}
			folder := func() *collections.CollectionNode {
				return &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			}
			root := folder()
			w := folder()
			sib := folder()
			for i := 0; i < 4+rng.Intn(3); i++ {
				sib.Children = append(sib.Children, mkReq())
			}
			succ := folder()
			inner := folder()
			deep := folder()
			for i := 0; i < 3+rng.Intn(3); i++ {
				deep.Children = append(deep.Children, mkReq())
			}
			inner.Children = append(inner.Children, deep)
			for i := 0; i < 2+rng.Intn(2); i++ {
				inner.Children = append(inner.Children, mkReq())
			}
			succ.Children = append(succ.Children, inner)
			for i := 0; i < 2+rng.Intn(2); i++ {
				succ.Children = append(succ.Children, mkReq())
			}
			w.Children = append(w.Children, sib, succ)
			root.Children = append(root.Children, w)
			tail := folder()
			for i := 0; i < 6; i++ {
				tail.Children = append(tail.Children, mkReq())
			}
			root.Children = append(root.Children, tail)

			col := &collections.ParsedCollection{ID: "c1", Name: root.Name, Root: root}
			collections.AssignParents(root, nil, col)
			ui.Collections = []*collections.CollectionUI{{Data: col}}
			ui.updateVisibleCols()

			var names []string
			sidebar.DebugBandGeom = func(n []string, _ []int, _ int) {
				names = append(names[:0], n...)
			}
			defer func() { sidebar.DebugBandGeom = nil }()

			sz := image.Pt(900, 520)
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

			seen := map[string]bool{}
			prev := -1
			worstJump, jumpDesc := 0, ""
			for step := 0; step < 4000; step++ {
				before := ui.ColList.Position.First*100000 + ui.ColList.Position.Offset
				names = names[:0]
				r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 130), Scroll: f32.Pt(0, 6)})
				now = now.Add(16 * time.Millisecond)
				frame()
				cur := len(names)
				for _, nm := range names {
					seen[nm] = true
				}
				if prev >= 0 && cur-prev > worstJump {
					worstJump = cur - prev
					f := ui.ColList.Position.First
					nm := "?"
					if f < len(ui.VisibleCols) {
						nm = ui.VisibleCols[f].Name
					}
					jumpDesc = fmt.Sprintf("step %d First=%d(%q) rows %d->%d names=%v", step, f, nm, prev, cur, names)
				}
				prev = cur
				if ui.ColList.Position.First*100000+ui.ColList.Position.Offset == before {
					break
				}
			}
			for _, n := range []*collections.CollectionNode{succ, inner, deep} {
				if !seen[n.Name] {
					t.Fatalf("deep-chain folder %q never appeared in the band", n.Name)
				}
			}
			if worstJump > 2 {
				t.Fatalf("sticky band grew by %d rows in a single frame (deep chain popped instead of staggering in): %s", worstJump, jumpDesc)
			}
		})
	}
}
