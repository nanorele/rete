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

// randStickyName returns a random alphanumeric name (letters + digits). Tests use
// synthetic random names instead of any real working-directory collection.
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

// buildRandomStickyTree builds a folder tree with mixed nesting depths — 2-level,
// 3-level and deeper — and random alphanumeric names for folders and requests.
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

// buildShortTailStickyTree reproduces the user's exact topology with random names:
// deeply nested folders whose deepest scopes hold only ONE request, immediately
// followed by a shallower sibling/uncle folder. Scrolling onto that single child
// is what made the reach-down band jump ahead into the next subfolder and cover
// the following sibling header ("Получение файлов" covering "Очереди").
func buildShortTailStickyTree(rng *rand.Rand) *collections.CollectionNode {
	root := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
	// A handful of d1 folders; each contains a short-tailed nested chain plus a few
	// leaf requests, so leaving the chain crosses one or two scope levels into the
	// next d1 sibling — exactly the boundary that exhibited the bug.
	for s := 0; s < 5; s++ {
		d1 := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
		root.Children = append(root.Children, d1)
		// nested chain d2 > d3 > ... each with a single request (the "short tail").
		depth := 2 + rng.Intn(3) // 2..4 extra levels
		cur := d1
		for d := 0; d < depth; d++ {
			sub := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
			cur.Children = append(cur.Children, sub)
			// one or two requests in the intermediate folder, then descend.
			for i, n := 0, 1+rng.Intn(2); i < n; i++ {
				sub.Children = append(sub.Children,
					&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
			}
			cur = sub
		}
		// a couple of trailing leaves directly under d1 so the chain is followed by
		// shallower content before the next d1 sibling.
		for i := 0; i < 2+rng.Intn(3); i++ {
			d1.Children = append(d1.Children,
				&collections.CollectionNode{Name: randStickyName(rng), Request: &model.ParsedRequest{Method: "GET"}})
		}
	}
	return root
}

// stickyIsAncestorOrSelf reports whether a is n or an ancestor of n.
func stickyIsAncestorOrSelf(a, n *collections.CollectionNode) bool {
	for p := n; p != nil; p = p.Parent {
		if p == a {
			return true
		}
	}
	return false
}

// TestStickyBandPinsOnlyAncestors is the direct regression test for the user's
// report: while the top row is still inside one subfolder, the sticky band must
// NEVER pin a different folder (and so cover the next sibling). VS Code reach-up:
// every pinned row is an ancestor-or-self of the TOP row. It also checks the band
// never overshoots its pinned-row count (which is what made it tall enough to
// cover the following sibling header). Random alphanumeric names, mixed depths.
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
				// Every LOGICALLY pinned row (reported chain) must be an ancestor-or-self
				// of the top row. A pinned row that is NOT (a different subfolder) is the
				// bug: the band claims a folder the user has not entered and covers the
				// real next sibling. (Incoming folders rendered sliding into a slot during
				// a transition are visual-only and not reported here.)
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
				// The opaque band must not paint past the rows it actually DRAWS (no
				// empty-space fill); drawn includes the incoming folder(s) sliding into a
				// slot during a seamless transition.
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

// TestStickyNoFlickerOnExit scrolls a deeply/variably nested random tree from top
// to bottom and asserts the sticky band never "artifacts" while exiting folders:
//
//   - No flicker: because the scroll only moves forward and folder names are unique,
//     each folder must be pinned over a single CONTIGUOUS run of frames. A folder
//     that drops out of the band and then re-appears is the visual artifact the user
//     reported when exiting two-level folders (the band momentarily lost its deepest
//     row and re-rendered an ancestor as a tall sliding header).
//   - No empty opaque band: the opaque (filled) band height never exceeds the number
//     of pinned rows by more than a row's slack.
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
	// The exact set of frames each folder was pinned in.
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
		// The opaque band must never be taller than the rows it DRAWS (no empty space);
		// drawn includes the incoming folder sliding into a slot during a transition.
		if rows := drawn; rows > 0 {
			if over := solidH - (rows*rowH + gapTol); over > gapWorst {
				gapWorst = over
				gapDesc = fmt.Sprintf("step %d solidH=%d rows=%d band=%v", step, solidH, rows, band)
			}
		}
		if ui.ColList.Position.First+ui.ColList.Position.Offset == before {
			break // clamped at the bottom
		}
	}

	// For each folder, the longest run of CONSECUTIVE absent frames between its first
	// and last pinned frame is its worst flicker gap. A folder dropping out of the
	// band and re-appearing is the artifact. The reach-up band pins the ancestor
	// chain of the top row, which is monotone as you scroll, so a folder is pinned
	// over a single contiguous run — the worst gap is 0. A small bound is kept only
	// as slack; a SUSTAINED drop is the real bug (the band losing a folder it is
	// still inside).
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

// TestStickySeamlessTransition is the regression test for the user's reports about
// nested transitions:
//   - "appears only after scrolling": between two short folders the band briefly
//     collapsed to root (a gap) before the next folder popped in.
//   - "nodes of nesting 2 and deeper lag by one node" ("Входящее сообщение",
//     "Цитирование текстовым"): a nested subfolder chain appeared one level per node
//     (a staircase) instead of together with the folder being entered.
//
// VS Code keeps the slot occupied (the incoming folder slides in as the outgoing
// slides out) AND, when entering a folder, its nested first-child chain slides in
// together (the deep rows ride at their real positions, so they do not opaquely
// cover the content below — verified by the gap/jerk/duplicate tests).
//
// The tree (random alphanumeric names) targets the patterns:
//   - sibling swap:  s1 (1 child) -> s2 (many children)   [d1 -> d1]
//   - descent:       d1 > d2 > d3 nested first-child subfolders
//
// Invariants: never collapse to root-only mid-scope (the gap); and the nested
// first-child chain (d2, d3) is present as soon as the top row enters d1 (no lag).
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
			// Several adjacent SHORT d1 folders (1 child each) — the sibling-swap worst
			// case: leaving one immediately enters the next, with no room to spare.
			for i := 0; i < 4; i++ {
				f := &collections.CollectionNode{Name: randStickyName(rng), IsFolder: true, Expanded: true}
				f.Children = append(f.Children, mkReq())
				root.Children = append(root.Children, f)
			}
			// A d1 folder whose FIRST child is a d2 subfolder (descent), which in turn
			// has a first-child d3 subfolder (deeper descent), each with leaves.
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
			// A final long d1 folder so the descent chain has a sibling to exit into.
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
				// No root-only collapse: while the top row is strictly inside a folder
				// (depth >= 2: root + at least one folder), the band must keep at least
				// two visible lines — the outgoing folder slides out only as the incoming
				// one slides in (this is the sibling-swap gap the user saw between two
				// short folders). A deepest line may briefly slide out on a last child,
				// but the root + a folder line are always present.
				if depthOf(top) >= 2 && visible < 2 {
					gaps++
					if worst == "" {
						worst = fmt.Sprintf("step %d First=%d(%q) depth=%d Off=%d visible=%d ys=%v",
							step, f, top.Name, depthOf(top), ui.ColList.Position.Offset, visible, ys)
					}
				}
				// Deep descent appears together (no per-level "lag by one node"): the
				// instant the top row is the d1 folder, its nested first-child chain
				// d2 > d3 must ALREADY be drawn — sliding in with the parent, not popping
				// in one (and two) nodes later (the user's report for "Входящее сообщение"
				// / "Цитирование текстовым"). The deep rows ride at their real positions
				// (they do not opaquely cover the content below — see the gap/jerk tests).
				if !d1Checked && f == d1Idx && d2Idx >= 0 && d3Idx >= 0 && ui.ColList.Position.Offset > 0 {
					d1Checked = true
					if !inName(d2.Name) || !inName(d3.Name) {
						t.Fatalf("deep descent lags: top is on the parent %q but its nested subfolders %q/%q are not both in the band (names=%v) — they would dock a node (or two) later",
							d1.Name, d2.Name, d3.Name, names)
					}
				}
				// And once the top row is the d2 folder, d3 is present too.
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

// TestStickyNonFirstSubfolderSwap is the regression test for the user's report that
// the smooth move "doesn't always work" — specifically for "Получение файлов", a
// subfolder that is NOT the first child of its parent. Leaving the first subfolder
// into a later sibling subfolder (a depth-2+ swap) used to make the band DIP: the
// leaving subfolder slid out (band shrank to root+parent) and then the successor
// POPPED in a row taller. The successor must instead slide into the slot as the
// predecessor leaves, so while the top row is inside the parent's depth-2 subtree
// the band keeps its depth-2 row (never dips to just root+parent).
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
			// A d1 folder P with TWO subfolders A (first) and B (NOT first) — the
			// "Получение" > ["Получение уведомлений", "Получение файлов"] shape.
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
			// A trailing d1 sibling so P has somewhere to exit into.
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
				// While the top row is strictly inside the parent's depth-2 subtree
				// (depth >= 3: root + P + a depth-2 subfolder), the band must keep its
				// depth-2 row — bottom stays at least 3 rows. A dip to 2 rows is the
				// leaving-subfolder-slid-out-before-successor-arrived bug.
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

// TestStickyDeepChainSlidesInNotPops is the regression test for the user's report
// that the transition into a deeply-nested chain — "Webhooks > Отправленные
// сообщения > Отправленные сообщения с телефона" — was not seamless. Entering a
// successor folder that is itself a deep first-child chain used to make the whole
// chain POP in at once: while the predecessor sibling's last rows were still on
// screen, only the successor's OUTERMOST row pre-loaded, then its inner d3/d4 rows
// all appeared in a single frame (band grew several rows at once). The chain must
// instead STAGGER in — each nested header revealed only after its parent docks — so
// the band never gains the whole chain in one frame.
//
// Note: with uniform row heights (synthetic single-line names), the deepest two
// rows are geometrically forced to settle on the same frame (slot spacing equals
// row spacing), so the achievable floor is a 2-row step, not 1. On real data, whose
// folder names wrap to taller rows, the chain spreads further and lands one row per
// frame. The invariant asserted here — never a 3+ row single-frame jump for a
// 3-level chain — is exactly what separates the staggered slide from the old pop.
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
			// W (d1) holds a sibling subtree SIB (d2, several leaves) followed by a
			// deeply-nested successor chain SUCC (d2) > INNER (d3) > DEEP (d4) > leaves
			// — the "Webhooks > Отправленное сообщение > …с телефона > Медиа" shape.
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
			// A trailing d1 sibling so the chain has somewhere to exit into.
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
				// Track the worst single-frame GROWTH. Shrink (rows docking and the chain
				// leaving) is fine; only sudden growth is the non-seamless pop. A 3-level
				// chain appearing in one frame (growth >= 3) is the bug; the staggered
				// slide grows at most 2 (the uniform-height deepest-pair floor).
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
			// The deep chain must genuinely appear (otherwise the no-pop assertion is
			// vacuously true): every level of the SUCC > INNER > DEEP chain was pinned.
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
