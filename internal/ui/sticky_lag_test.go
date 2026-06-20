package ui

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"path/filepath"
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

func TestStickyBandDoesNotLagList(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	cur := root
	for _, nm := range []string{"A", "B", "C", "D"} {
		f := &collections.CollectionNode{Name: nm, IsFolder: true, Expanded: true}
		cur.Children = append(cur.Children, f)
		cur = f
	}
	for i := 0; i < 40; i++ {
		cur.Children = append(cur.Children, &collections.CollectionNode{
			Name: fmt.Sprintf("req-%d", i), Request: &model.ParsedRequest{Method: "GET"},
		})
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	ancestorsOf := func(n *collections.CollectionNode) []string {
		var out []string
		for p := n.Parent; p != nil; p = p.Parent {
			out = append([]string{p.Name}, out...)
		}
		return out
	}

	// Overlay model: the band pins the ancestors of the first row the overlay does
	// NOT hide (the row resting at the band's bottom edge), plus that row itself
	// when it is a folder being scrolled into. This mirrors the production band and
	// is what makes the pinned context match the content shown beneath it.
	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.colRowH > 0 {
			return ui.colRowH
		}
		return 24
	}
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}
	expectedBand := func(i int) []string {
		n := ui.VisibleCols[i]
		out := ancestorsOf(n)
		if (n.IsFolder || n.Depth == 0) && n.Expanded &&
			i+1 < len(ui.VisibleCols) && ui.VisibleCols[i+1].Parent == n {
			out = append(out, n.Name)
		}
		return out
	}

	absScroll := func(ui *AppUI) int {
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

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         now,
			Source:      r.Source(),
		}
		ui.layoutApp(gtx)
		r.Frame(ops)
	}

	frame()

	maxAnc := 0
	prevEffective := absScroll(ui) - reserve
	streak, worst := 0, 0
	for step := 0; step < 80; step++ {
		r.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(120, 400),
			Scroll:   f32.Pt(0, 6),
		})
		now = now.Add(16 * time.Millisecond)
		frame()

		first := ui.ColList.Position.First
		if first < 0 || first >= len(ui.VisibleCols) {
			continue
		}
		// VS Code reach-up model: the band pins the ancestor chain of the TOP row
		// (`first`) — the scope the user is inside — plus that row itself when it is a
		// folder being entered. The band must equal this every frame (it does not look
		// DOWN past itself, which is what used to make it jump ahead into the next
		// subfolder and cover it). _ = rowUnderBand keeps the helper referenced.
		_ = rowUnderBand
		if bandH > 0 {
			want := expectedBand(first)
			if len(want) > maxAnc {
				maxAnc = len(want)
			}
			if fmt.Sprint(lastRendered) == fmt.Sprint(want) {
				streak = 0
			} else {
				streak++
				if streak > worst {
					worst = streak
				}
				if streak > 6 {
					t.Fatalf("step %d: sticky band did not match the top row's scope for %d frames — First=%d\n  rendered = %v\n  reach-up expects = %v",
						step, streak, first, lastRendered, want)
				}
			}
		}
		// Overlay: the list is never offset (reserve is reported as 0), so the
		// content position == absScroll and must move strictly with the scroll.
		effective := absScroll(ui) - reserve
		if effective < prevEffective-2 {
			t.Fatalf("step %d: content scrolled BACKWARDS — First=%d Offset=%d reserve=%d: effective %d -> %d",
				step, first, ui.ColList.Position.Offset, reserve, prevEffective, effective)
		}
		prevEffective = effective
	}

	if maxAnc < 4 {
		t.Fatalf("scroll only ever reached %d pinned ancestors; the test never exercised the deep boundaries", maxAnc)
	}
}

// TestStickyBandDoesNotLurchAtSiblingFolder guards the jerk that the deep-chain
// lag test above structurally cannot see. With sibling folders, scrolling a
// folder's last child into the *next* folder makes the band's inner row swap
// (fldA->fldB); the old code restarted that folder's slide-in, dropping a whole
// band row of reserve in one frame -> the content lurched forward. The three
// invariants the lag test checks (band names, occlusion, backward scroll) all
// stayed green through that bug; only a forward-lurch check catches it.
func TestStickyBandDoesNotLurchAtSiblingFolder(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	for _, fn := range []string{"fldA", "fldB", "fldC", "fldD"} {
		f := &collections.CollectionNode{Name: fn, IsFolder: true, Expanded: true}
		for i := 0; i < 25; i++ {
			f.Children = append(f.Children, &collections.CollectionNode{
				Name: fmt.Sprintf("%s-req-%d", fn, i), Request: &model.ParsedRequest{Method: "GET"},
			})
		}
		root.Children = append(root.Children, f)
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	absScroll := func(ui *AppUI) int {
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

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Now:         now,
			Source:      r.Source(),
		}
		ui.layoutApp(gtx)
		r.Frame(ops)
	}

	frame()

	const delta = 6
	// A lurch is the content position jumping by more than the scroll the user
	// applied. The bug produced ~one whole band row (>>delta); allow delta plus
	// a few px of band-row rounding.
	forwardMax := delta + 4
	prevEffective := absScroll(ui) - reserve
	seen := map[string]bool{}
	for step := 0; step < 320; step++ {
		r.Queue(pointer.Event{
			Kind:     pointer.Scroll,
			Source:   pointer.Mouse,
			Position: f32.Pt(120, 400),
			Scroll:   f32.Pt(0, delta),
		})
		now = now.Add(16 * time.Millisecond)
		frame()

		if len(lastRendered) > 0 {
			seen[lastRendered[len(lastRendered)-1]] = true
		}
		// Overlay: content == absScroll, never jumps in either direction.
		_ = bandH
		effective := absScroll(ui) - reserve
		if effective < prevEffective-4 {
			t.Fatalf("step %d: content scrolled BACKWARDS: effective %d -> %d (First=%d Offset=%d band=%v)",
				step, prevEffective, effective, ui.ColList.Position.First, ui.ColList.Position.Offset, lastRendered)
		}
		if effective > prevEffective+forwardMax {
			t.Fatalf("step %d: content LURCHED FORWARD: effective %d -> %d (+%d > scroll %d) (First=%d Offset=%d band=%v)",
				step, prevEffective, effective, effective-prevEffective, delta,
				ui.ColList.Position.First, ui.ColList.Position.Offset, lastRendered)
		}
		prevEffective = effective
	}

	// The band pins a sibling folder (as innermost ancestor) only while First is
	// inside that folder's subtree. Confirm we scrolled through several of them.
	for _, fn := range []string{"fldB", "fldC"} {
		if !seen[fn] {
			t.Fatalf("scroll never pinned %q as innermost; sibling boundaries not exercised (seen=%v)", fn, seen)
		}
	}
}

// TestStickyNoJumpAtNestedSubfolder guards the 3rd-level bug: a subfolder that
// is NOT the first child of its parent (sibling requests precede it) was
// misclassified as a sibling "swap" instead of a "descent", so the band gained a
// row with a discrete reserve step -> the content jumped backwards when First
// reached the subfolder. The deep-chain lag test cannot see this because there
// every folder is the FIRST child of its parent.
// TestStickySiblingPushOutSlidesNotVanish guards the VS Code–style transition
// between two SIBLING folders (ordinal, not nesting). The old folder must slide
// up and out (shown shrinking over several frames) while the band does NOT grow,
// rather than the band extending and the old folder vanishing in one frame.
func TestStickySiblingPushOutSlidesNotVanish(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "c1", Name: "root", Root: root}
	for _, fn := range []string{"A", "B", "C", "D", "E"} {
		f := &collections.CollectionNode{Name: fn, IsFolder: true, Expanded: true}
		for i := 0; i < 10; i++ {
			f.Children = append(f.Children, &collections.CollectionNode{
				Name: fmt.Sprintf("%s-r%d", fn, i), Request: &model.ParsedRequest{Method: "GET"}})
		}
		root.Children = append(root.Children, f)
	}
	collections.AssignParents(root, nil, col)
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	var band []string
	sidebar.DebugSticky = func(_ int, n []string) { band = append(band[:0], n...) }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
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

	rowH := ui.colRowH
	if rowH <= 0 {
		rowH = 24
	}
	twoRow := 2*rowH + 4 // root + one folder + border, with tolerance

	// Record (deepest pinned folder name, bandH) per frame.
	type fr struct {
		deepest string
		h       int
	}
	var frames []fr
	for step := 0; step < 900; step++ {
		before := ui.ColList.Position.First*10000 + ui.ColList.Position.Offset
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		d := ""
		if len(band) > 0 {
			d = band[len(band)-1]
		}
		frames = append(frames, fr{d, bandH})
		if ui.ColList.Position.First*10000+ui.ColList.Position.Offset == before {
			break
		}
	}

	// Count sibling transitions where the leaving (top-level) folder slid out:
	// it stayed the deepest pinned row across >=2 frames whose band height was
	// shrinking and dipped well under the two-row height (a real slide, not an
	// instant vanish). Also assert the band never extended past two rows for a
	// top-level folder (no "becomes one node bigger").
	siblings := map[string]bool{"A": true, "B": true, "C": true, "D": true, "E": true}
	slidOut := 0
	run := 0 // consecutive frames with the same top-level deepest folder
	minHInRun := 1 << 30
	prevDeepest := ""
	flush := func(next string) {
		if siblings[prevDeepest] && run >= 2 && minHInRun < twoRow-rowH/2 && siblings[next] && next != prevDeepest {
			slidOut++ // leaving folder shrank to a sliver before its sibling took over
		}
	}
	for _, f := range frames {
		if siblings[f.deepest] && f.h > twoRow {
			t.Fatalf("band extended past two rows (%d > %d) for top-level folder %q — it grew instead of pushing out", f.h, twoRow, f.deepest)
		}
		if f.deepest == prevDeepest {
			run++
			if f.h < minHInRun {
				minHInRun = f.h
			}
		} else {
			flush(f.deepest)
			prevDeepest = f.deepest
			run = 1
			minHInRun = f.h
		}
	}
	if slidOut < 2 {
		t.Fatalf("only %d sibling-folder transitions showed the leaving folder sliding out; expected the push-out at several boundaries", slidOut)
	}
}

func TestStickyNoJumpAtNestedSubfolder(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true

	mkReq := func(name string) *collections.CollectionNode {
		return &collections.CollectionNode{Name: name, Request: &model.ParsedRequest{Method: "GET"}}
	}
	root := &collections.CollectionNode{Name: "root", IsFolder: true, Expanded: true}
	f1 := &collections.CollectionNode{Name: "F1", IsFolder: true, Expanded: true}
	f1.Children = []*collections.CollectionNode{mkReq("f1-r0"), mkReq("f1-r1"), mkReq("f1-r2")}
	f2 := &collections.CollectionNode{Name: "F2sub", IsFolder: true, Expanded: true} // subfolder after siblings
	for i := 0; i < 40; i++ {
		f2.Children = append(f2.Children, mkReq(fmt.Sprintf("f2-r%d", i)))
	}
	f1.Children = append(f1.Children, f2, mkReq("f1-after"))
	root.Children = []*collections.CollectionNode{f1}
	for i := 0; i < 10; i++ {
		root.Children = append(root.Children, mkReq(fmt.Sprintf("root-r%d", i)))
	}
	c := &collections.ParsedCollection{ID: "nest", Name: "root", Root: root}
	collections.AssignParents(root, nil, c)
	ui.Collections = []*collections.CollectionUI{{Data: c}}
	ui.updateVisibleCols()

	absScroll := func(ui *AppUI) int {
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

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 760)
	now := time.Unix(1700000000, 0)
	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		ui.layoutApp(layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Now: now, Source: r.Source()})
		r.Frame(ops)
	}
	frame()

	const delta = 6
	forwardMax := delta + 4
	prevEffective := absScroll(ui) - reserve
	sawSub := false
	for step := 0; step < 90; step++ {
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 400), Scroll: f32.Pt(0, delta)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if len(lastRendered) > 0 && lastRendered[len(lastRendered)-1] == "F2sub" {
			sawSub = true
		}
		if reserve > bandH {
			t.Fatalf("step %d: reserve=%d exceeds bandH=%d", step, reserve, bandH)
		}
		effective := absScroll(ui) - reserve
		if effective < prevEffective-4 {
			t.Fatalf("step %d: content scrolled BACKWARDS at subfolder: effective %d -> %d (First=%d Off=%d reserve=%d band=%v)",
				step, prevEffective, effective, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, lastRendered)
		}
		if effective > prevEffective+forwardMax {
			t.Fatalf("step %d: content LURCHED FORWARD at subfolder: effective %d -> %d (+%d > scroll %d) (First=%d Off=%d reserve=%d band=%v)",
				step, prevEffective, effective, effective-prevEffective, delta, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, lastRendered)
		}
		prevEffective = effective
	}
	if !sawSub {
		t.Fatal("scroll never pinned F2sub as the innermost header; the nested-subfolder boundary was not exercised")
	}
}

// TestStickyRealCollectionScrollTopToBottom is the reproduction the user asked
// for: load the real collection from the app's data dir, expand every node, and
// scroll from the top all the way to the bottom and back. Every frame it asserts
// the content never jumps — neither backwards nor forwards by more than the
// scroll the user applied — which is what "дёргается" looks like in numbers.
//
// Set STICKY_COLLECTION to a specific .json, else it loads every collection in
// %APPDATA%/tracto/collections. Skips if none are present (e.g. CI).
func TestStickyRealCollectionScrollTopToBottom(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/tracto/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.updateVisibleCols()
	t.Logf("collections=%d visible nodes=%d", len(cols), len(ui.VisibleCols))
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small to exercise scrolling (%d nodes)", len(ui.VisibleCols))
	}

	absScroll := func() int {
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

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var reserve, bandH int
	sidebar.DebugBand = func(r, b int) { reserve, bandH = r, b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
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

	const delta = 6
	// A jerk is the content moving against, or far past, the scroll the user
	// applied. Real rows have varied (sometimes 2-line) heights, so allow a few
	// px of band-row rounding; a whole-row jump (~24px) is well outside this.
	const tol = 6
	topName := func() (string, int, bool) {
		f := ui.ColList.Position.First
		if f < 0 || f >= len(ui.VisibleCols) {
			return "?", -1, false
		}
		n := ui.VisibleCols[f]
		entering := (n.IsFolder || n.Depth == 0) && n.Expanded &&
			f+1 < len(ui.VisibleCols) && ui.VisibleCols[f+1].Parent == n
		return n.Name, n.Depth, entering
	}

	// scrollDir runs frames in one direction until it stops making progress
	// (clamped at an end), recording every frame whose content jump exceeds the
	// scroll the user applied. Returns (frames, jerks) instead of failing fast so
	// the whole collection is exercised and every jerk site is reported.
	type jerk struct{ msg string }
	scrollDir := func(dir string, sign int) (int, []jerk) {
		prevEff := absScroll() - reserve
		stall, frames := 0, 0
		var jerks []jerk
		for step := 0; step < 6000; step++ {
			r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, float32(sign*delta))})
			now = now.Add(16 * time.Millisecond)
			before := absScroll()
			frame()
			frames++
			eff := absScroll() - reserve
			d := eff - prevEff
			name, depth, entering := topName()
			var bad string
			if sign > 0 { // down: eff rises ~delta; backward (d<-tol) or forward lurch (d>delta+tol) is a jerk
				if d < -tol {
					bad = fmt.Sprintf("BACKWARDS d=%d", d)
				} else if d > delta+tol {
					bad = fmt.Sprintf("FORWARD-LURCH d=%d (>scroll %d)", d, delta)
				}
			} else { // up: eff falls ~delta; forward (d>tol) or backward lurch (d<-(delta+tol)) is a jerk
				if d > tol {
					bad = fmt.Sprintf("FORWARD d=%d", d)
				} else if d < -(delta + tol) {
					bad = fmt.Sprintf("BACKWARD-LURCH d=%d (>scroll %d)", d, delta)
				}
			}
			if bad != "" {
				jerks = append(jerks, jerk{fmt.Sprintf("%s step %d: %s top=%q depth=%d entering=%v First=%d Off=%d reserve=%d bandH=%d band=%v",
					dir, step, bad, name, depth, entering, ui.ColList.Position.First, ui.ColList.Position.Offset, reserve, bandH, lastRendered)})
			}
			prevEff = eff
			if absScroll() == before {
				if stall++; stall >= 3 {
					break // clamped at an end
				}
			} else {
				stall = 0
			}
		}
		return frames, jerks
	}

	downFrames, downJerks := scrollDir("down", +1)
	if ui.ColList.Position.First == 0 {
		t.Fatal("scrolling down never advanced past the first node")
	}
	t.Logf("scrolled to bottom in %d frames (First=%d); %d jerks", downFrames, ui.ColList.Position.First, len(downJerks))
	upFrames, upJerks := scrollDir("up", -1)
	t.Logf("scrolled back toward top in %d frames (First=%d Off=%d); %d jerks", upFrames, ui.ColList.Position.First, ui.ColList.Position.Offset, len(upJerks))

	all := append(downJerks, upJerks...)
	if len(all) > 0 {
		const show = 25
		for i, j := range all {
			if i >= show {
				t.Logf("... and %d more", len(all)-show)
				break
			}
			t.Log(j.msg)
		}
		t.Fatalf("real collection scroll produced %d content jerks (see above)", len(all))
	}
}

// TestStickyRealCollectionBandMatchesContent checks the VS Code reach-up invariant
// on the real collection: the pinned band always shows the ancestor chain of the
// TOP row (the scope the user is inside), plus that row itself when it is a folder
// being entered — never a folder reached by looking DOWN past the band, which is
// what used to make it jump ahead into the next subfolder and cover the next
// sibling header.
func TestStickyRealCollectionBandMatchesContent(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/tracto/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.updateVisibleCols()
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small (%d nodes)", len(ui.VisibleCols))
	}

	ancestorsOf := func(n *collections.CollectionNode) []string {
		var out []string
		for p := n.Parent; p != nil; p = p.Parent {
			out = append([]string{p.Name}, out...)
		}
		return out
	}
	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.colRowH > 0 {
			return ui.colRowH
		}
		return 24
	}
	// The first row whose bottom edge is below the band's bottom — the row the band
	// rests on (the first the overlay does not fully hide). Returns its index.
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}
	// Expected pinned chain for the row the band rests on: its ancestors, plus the
	// row itself when it is a folder being scrolled into (its header is at the band
	// edge but its children show beneath, so it is legitimately pinned too).
	expectedBand := func(i int) []string {
		n := ui.VisibleCols[i]
		out := ancestorsOf(n)
		if (n.IsFolder || n.Depth == 0) && n.Expanded &&
			i+1 < len(ui.VisibleCols) && ui.VisibleCols[i+1].Parent == n {
			out = append(out, n.Name)
		}
		return out
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
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

	// The band can briefly differ from the content beneath it only while a scope
	// boundary is crossing (the bottom row is sliding out / in over a few frames).
	// A *persistent* mismatch is the lag bug. Allow short transition streaks but
	// fail if the band is wrong for more than `maxStreak` consecutive frames.
	const maxStreak = 10
	var mism []string
	checks, streak, worst, mismTotal := 0, 0, 0, 0
	for step := 0; step < 1400; step++ {
		before := ui.ColList.Position.Offset + ui.ColList.Position.First
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH <= 0 {
			streak = 0
			continue // nothing pinned (top of a collection)
		}
		_ = rowUnderBand
		first := ui.ColList.Position.First
		want := expectedBand(first)
		checks++
		if fmt.Sprint(lastRendered) == fmt.Sprint(want) {
			streak = 0
		} else {
			streak++
			mismTotal++
			if streak > worst {
				worst = streak
			}
			if len(mism) < 20 {
				mism = append(mism, fmt.Sprintf("step %d (streak %d): band=%v but top row %q reach-up chain is %v (First=%d Off=%d bandH=%d)",
					step, streak, lastRendered, ui.VisibleCols[first].Name, want, ui.ColList.Position.First, ui.ColList.Position.Offset, bandH))
			}
		}
		if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
			break // clamped at the bottom
		}
	}
	t.Logf("compared %d frames; %d mismatched (all transitions); worst consecutive streak = %d", checks, mismTotal, worst)
	if worst > maxStreak {
		for _, m := range mism {
			t.Log(m)
		}
		t.Fatalf("sticky band lagged the content for %d consecutive frames (> %d) — a persistent lag, not a transition", worst, maxStreak)
	}
}

// TestStickyRealCollectionNoDuplicateUnderBand checks the VS Code–style smooth
// transition: when a folder header docks into the band, it must not appear BOTH
// pinned in the band AND as a real list row peeking out just beneath it. A
// duplicate frame is one where the band's innermost pinned node is the very same
// node whose real row straddles the band's bottom edge (so the user sees it
// twice). The slide makes the pinned copy cover its own real row, so there are
// none — except possibly a 1px rounding sliver, which the tolerance absorbs.
func TestStickyRealCollectionNoDuplicateUnderBand(t *testing.T) {
	cols := loadRealCollections(t)
	if len(cols) == 0 {
		t.Skip("no real collections found (set STICKY_COLLECTION or populate %APPDATA%/tracto/collections)")
	}

	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"
	ui.ColsExpanded = true
	ui.Collections = cols

	var expand func(n *collections.CollectionNode)
	expand = func(n *collections.CollectionNode) {
		n.Expanded = true
		for _, c := range n.Children {
			expand(c)
		}
	}
	for _, cu := range cols {
		if cu.Data != nil && cu.Data.Root != nil {
			expand(cu.Data.Root)
		}
	}
	ui.updateVisibleCols()
	if len(ui.VisibleCols) < 10 {
		t.Skipf("collection too small (%d nodes)", len(ui.VisibleCols))
	}

	rowH := func(i int) int {
		if i >= 0 && i < len(ui.VisibleCols) {
			if h := ui.VisibleCols[i].RowHeightPx; h > 0 {
				return h
			}
		}
		if ui.colRowH > 0 {
			return ui.colRowH
		}
		return 24
	}
	// screen-top Y of row i (overlay: list is not offset).
	rowTop := func(i int) int {
		y := -ui.ColList.Position.Offset
		for j := ui.ColList.Position.First; j < i && j < len(ui.VisibleCols); j++ {
			y += rowH(j)
		}
		return y
	}
	// The node whose row straddles the band's bottom edge.
	rowUnderBand := func(bandH int) int {
		y := -ui.ColList.Position.Offset
		for i := ui.ColList.Position.First; i < len(ui.VisibleCols); i++ {
			h := rowH(i)
			if y+h > bandH {
				return i
			}
			y += h
		}
		return len(ui.VisibleCols) - 1
	}

	var lastRendered []string
	sidebar.DebugSticky = func(_ int, names []string) { lastRendered = names }
	defer func() { sidebar.DebugSticky = nil }()
	var bandH int
	sidebar.DebugBand = func(_, b int) { bandH = b }
	defer func() { sidebar.DebugBand = nil }()

	sz := image.Pt(900, 800)
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

	var dups []string
	for step := 0; step < 1400; step++ {
		before := ui.ColList.Position.Offset + ui.ColList.Position.First
		r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(120, 420), Scroll: f32.Pt(0, 6)})
		now = now.Add(16 * time.Millisecond)
		frame()
		if bandH <= 0 || len(lastRendered) == 0 {
			if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
				break
			}
			continue
		}
		ui_ := rowUnderBand(bandH)
		under := ui.VisibleCols[ui_]
		// Is the band's innermost pinned node the same node straddling the band
		// bottom? If so, its real row peeks out below its own pinned copy.
		isFolderUnder := (under.IsFolder || under.Depth == 0) && under.Expanded &&
			ui_+1 < len(ui.VisibleCols) && ui.VisibleCols[ui_+1].Parent == under
		peek := rowTop(ui_) + rowH(ui_) - bandH // px of `under`'s real row visible below the band
		if isFolderUnder && lastRendered[len(lastRendered)-1] == under.Name && peek > 2 {
			if len(dups) < 20 {
				dups = append(dups, fmt.Sprintf("step %d: %q pinned AND its real row peeks %dpx below the band (First=%d Off=%d bandH=%d band=%v)",
					step, under.Name, peek, ui.ColList.Position.First, ui.ColList.Position.Offset, bandH, lastRendered))
			}
		}
		if ui.ColList.Position.Offset+ui.ColList.Position.First == before {
			break
		}
	}
	if len(dups) > 0 {
		for _, d := range dups {
			t.Log(d)
		}
		t.Fatalf("sticky header duplicated with its list row in %d+ frames (no smooth transition)", len(dups))
	}
}

// loadRealCollections reads the user's real collections from STICKY_COLLECTION
// (a single file) or every *.json in %APPDATA%/tracto/collections.
func loadRealCollections(t *testing.T) []*collections.CollectionUI {
	t.Helper()
	var paths []string
	if p := os.Getenv("STICKY_COLLECTION"); p != "" {
		paths = []string{p}
	} else {
		dir := filepath.Join(os.Getenv("APPDATA"), "tracto", "collections")
		matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
		paths = matches
	}
	var out []*collections.CollectionUI
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		col, err := collections.ParseCollection(bytes.NewReader(data), filepath.Base(p))
		if err != nil || col == nil || col.Root == nil {
			continue
		}
		out = append(out, &collections.CollectionUI{Data: col})
	}
	return out
}
