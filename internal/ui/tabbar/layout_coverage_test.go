package tabbar

import (
	"image"
	"testing"
	"time"

	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

const insetLeftPx = 4

type dragRig struct {
	t         *testing.T
	s         *Strip
	th        *material.Theme
	tabs      []*workspace.RequestTab
	active    int
	r         input.Router
	ops       *op.Ops
	now       time.Time
	w, h      int
	exact     bool
	limitRows bool
	maxRows   int
	saves     int
	reveals   []*workspace.RequestTab
}

func newDragRig(t *testing.T, titles []string, w, h int) *dragRig {
	t.Helper()
	g := &dragRig{
		t:     t,
		s:     NewStrip(),
		th:    newTestTheme(),
		ops:   new(op.Ops),
		now:   time.Unix(1700000000, 0),
		w:     w,
		h:     h,
		exact: true,
	}
	for _, ti := range titles {
		g.tabs = append(g.tabs, workspace.NewRequestTab(ti))
	}
	return g
}

func (g *dragRig) frame() layout.Dimensions {
	g.ops.Reset()
	cs := layout.Exact(image.Pt(g.w, g.h))
	if !g.exact {
		cs = layout.Constraints{Max: image.Pt(g.w, g.h)}
	}
	gtx := layout.Context{
		Ops:         g.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: cs,
		Source:      g.r.Source(),
		Now:         g.now,
	}
	dims := g.s.Layout(gtx, g.th, &g.tabs, &g.active, g.limitRows, g.maxRows,
		func(tb *workspace.RequestTab) { g.reveals = append(g.reveals, tb) },
		func() { g.saves++ },
	)
	g.r.Frame(g.ops)
	return dims
}

func (g *dragRig) advance(d time.Duration) { g.now = g.now.Add(d) }

func (g *dragRig) queue(evs ...pointer.Event) {
	for _, e := range evs {
		g.r.Queue(e)
	}
}

func (g *dragRig) press(x, y float32) {
	g.queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	g.frame()
}

func (g *dragRig) drag(x, y float32) {
	g.queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	g.frame()
}

func (g *dragRig) release(x, y float32) {
	g.queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	g.frame()
}

func (g *dragRig) cancel() {
	g.queue(pointer.Event{Kind: pointer.Cancel})
	g.frame()
}

func (g *dragRig) natWidth(i int) int {
	g.t.Helper()
	c, ok := g.s.widthCache[g.tabs[i]]
	if !ok {
		g.t.Fatalf("tab %d has no cached width; call frame() first", i)
	}
	return c.width
}

func (g *dragRig) centerX(i int) float32 {
	g.t.Helper()
	x := 0
	for j := 0; j < i; j++ {
		x += g.natWidth(j)
	}
	return float32(insetLeftPx+x) + float32(g.natWidth(i))/2
}

func (g *dragRig) titles() []string {
	out := make([]string, len(g.tabs))
	for i, tb := range g.tabs {
		out[i] = tb.Title
	}
	return out
}

func sameOrder(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestDrag_ReordersTabToTheRight(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(2)

	g.press(from, 18)
	if g.s.TabDragIdx != 0 {
		t.Fatalf("press on the first tab must arm the drag (TabDragIdx=0), got %d", g.s.TabDragIdx)
	}
	if g.s.TabDragging {
		t.Fatal("a bare press must not start dragging yet")
	}

	g.advance(200 * time.Millisecond)
	g.drag(to, 18)

	if !g.s.TabDragging {
		t.Fatal("moving far enough after the hold delay must start a drag")
	}
	if got, want := g.titles(), []string{"bravo", "charlie", "alpha"}; !sameOrder(got, want) {
		t.Fatalf("drag to the right must move the tab to the end: got %v, want %v", got, want)
	}
	if g.s.TabDragIdx != 2 {
		t.Errorf("TabDragIdx must follow the dragged tab, got %d", g.s.TabDragIdx)
	}
	if g.active != 2 {
		t.Errorf("the active tab index must follow the dragged tab: got %d, want 2", g.active)
	}
}

func TestDrag_ReordersTabToTheLeft(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.active = 2
	g.frame()

	from := g.centerX(2)
	to := g.centerX(0)

	g.press(from, 18)
	if g.s.TabDragIdx != 2 {
		t.Fatalf("press on the third tab must arm the drag (TabDragIdx=2), got %d", g.s.TabDragIdx)
	}
	g.advance(200 * time.Millisecond)
	g.drag(to, 18)

	if got, want := g.titles(), []string{"charlie", "alpha", "bravo"}; !sameOrder(got, want) {
		t.Fatalf("drag to the left must move the tab to the front: got %v, want %v", got, want)
	}
	if g.s.TabDragIdx != 0 {
		t.Errorf("TabDragIdx must follow the dragged tab, got %d", g.s.TabDragIdx)
	}
	if g.active != 0 {
		t.Errorf("the active tab index must follow the dragged tab: got %d, want 0", g.active)
	}
}

func TestDrag_ActiveIndexFollowsDisplacedNeighbour(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.active = 1
	g.frame()

	from := g.centerX(0)
	to := g.centerX(1)

	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(to, 18)

	if got, want := g.titles(), []string{"bravo", "alpha", "charlie"}; !sameOrder(got, want) {
		t.Fatalf("one-step drag must swap the first two tabs: got %v, want %v", got, want)
	}
	if g.active != 0 {
		t.Errorf("the displaced active tab (bravo) moved to slot 0; activeIdx = %d, want 0", g.active)
	}
}

func TestDrag_PastRightEndLandsOnLastTab(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(800, 18)

	if got, want := g.titles(), []string{"bravo", "charlie", "alpha"}; !sameOrder(got, want) {
		t.Fatalf("dragging into the empty space right of the + button must drop the tab last: got %v, want %v", got, want)
	}
}

func TestDrag_BelowTheBarClampsToTheLastRow(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(1)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(to, 400)

	if got, want := g.titles(), []string{"bravo", "alpha", "charlie"}; !sameOrder(got, want) {
		t.Fatalf("a drag far below a single-row bar must still target that row: got %v, want %v", got, want)
	}
}

func TestDrag_AboveTheBarClampsToTheFirstRow(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(1)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(to, -120)

	if got, want := g.titles(), []string{"bravo", "alpha", "charlie"}; !sameOrder(got, want) {
		t.Fatalf("a drag above the bar must still target the first row: got %v, want %v", got, want)
	}
}

func TestDrag_ShortMoveBeforeDelayDoesNotStartDrag(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()
	before := g.titles()

	from := g.centerX(0)
	to := g.centerX(2)

	g.press(from, 18)
	g.advance(20 * time.Millisecond)
	g.drag(to, 18)
	if g.s.TabDragging {
		t.Error("a move within the 150ms hold window must not start a drag")
	}
	if !sameOrder(g.titles(), before) {
		t.Errorf("no reorder may happen before the drag starts: %v", g.titles())
	}

	g.advance(200 * time.Millisecond)
	g.drag(from+2, 18)
	if g.s.TabDragging {
		t.Error("a move under the 10px threshold must not start a drag")
	}
	if !sameOrder(g.titles(), before) {
		t.Errorf("no reorder may happen for a sub-threshold move: %v", g.titles())
	}
}

func TestDrag_ReleaseSavesAndClearsState(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(2)

	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(to, 18)
	if g.saves != 0 {
		t.Fatalf("onSave must not fire mid-drag, got %d calls", g.saves)
	}

	g.release(800, 18)
	if g.saves != 1 {
		t.Errorf("releasing after a reorder must persist once, got %d calls", g.saves)
	}
	if g.s.TabDragging {
		t.Error("release must end the drag")
	}
	if g.s.TabDragIdx != -1 {
		t.Errorf("release must clear TabDragIdx, got %d", g.s.TabDragIdx)
	}
}

func TestDrag_ReleaseWithoutDragDoesNotSave(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.release(from, 18)

	if g.saves != 0 {
		t.Errorf("a plain click must not trigger a save, got %d calls", g.saves)
	}
	if g.s.TabDragIdx != -1 {
		t.Errorf("release must clear TabDragIdx, got %d", g.s.TabDragIdx)
	}
}

func TestDrag_CancelEndsTheDrag(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(1)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(to, 18)
	if !g.s.TabDragging {
		t.Fatal("precondition: the drag must be active")
	}

	g.cancel()
	if g.s.TabDragging {
		t.Error("cancel must end the drag")
	}
	if g.s.TabDragIdx != -1 {
		t.Errorf("cancel must clear TabDragIdx, got %d", g.s.TabDragIdx)
	}
	if g.saves != 1 {
		t.Errorf("a cancelled drag still leaves the tabs reordered, so it must persist once; got %d", g.saves)
	}
}

func TestClick_ActivatesTabAndRevealsLinkedNode(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie"}, 1000, 200)
	g.frame()

	x := g.centerX(1)
	g.press(x, 18)
	g.release(x, 18)
	g.frame()

	if g.active != 1 {
		t.Fatalf("clicking the second tab must activate it, got activeIdx=%d", g.active)
	}
	if len(g.reveals) != 1 || g.reveals[0] != g.tabs[1] {
		t.Fatalf("clicking a tab must reveal its linked node exactly once, got %d call(s)", len(g.reveals))
	}

	g.s.TabCtxMenuOpen = true
	x2 := g.centerX(2)
	g.press(x2, 18)
	g.release(x2, 18)
	g.frame()

	if g.active != 2 {
		t.Errorf("clicking the third tab must activate it, got activeIdx=%d", g.active)
	}
	if g.s.TabCtxMenuOpen {
		t.Error("activating a tab must close an open context menu")
	}
}

func TestClick_OnAlreadyActiveTabKeepsIt(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo"}, 1000, 200)
	g.frame()

	x := g.centerX(0)
	g.press(x, 18)
	g.release(x, 18)
	g.frame()

	if g.active != 0 {
		t.Errorf("clicking the active tab must keep it active, got %d", g.active)
	}
	if len(g.reveals) != 1 {
		t.Errorf("expected one reveal call, got %d", len(g.reveals))
	}
}

func TestClick_NilCallbacksAreSafe(t *testing.T) {
	th := newTestTheme()
	s := NewStrip()
	tabs := []*workspace.RequestTab{
		workspace.NewRequestTab("alpha"),
		workspace.NewRequestTab("bravo"),
	}
	active := 0

	var r input.Router
	ops := new(op.Ops)
	now := time.Unix(1700000000, 0)
	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1000, 200)),
			Source:      r.Source(),
			Now:         now,
		}
		s.Layout(gtx, th, &tabs, &active, false, 0, nil, nil)
		r.Frame(ops)
	}
	frame()

	w0 := s.widthCache[tabs[0]].width
	x := float32(insetLeftPx+w0) + float32(s.widthCache[tabs[1]].width)/2

	r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, 18), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	frame()
	now = now.Add(200 * time.Millisecond)
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x+60, 18), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	frame()
	r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x+60, 18), Source: pointer.Mouse})
	frame()
	frame()

	if active < 0 || active >= len(tabs) {
		t.Errorf("activeIdx went out of range with nil callbacks: %d", active)
	}
}

func TestDrag_AcrossRowsReorders(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}, 200, 400)
	g.frame()

	if len(g.s.rowsBuf) < 2 {
		t.Fatalf("precondition: expected a wrapped multi-row bar, got %d row(s)", len(g.s.rowsBuf))
	}
	before := g.titles()

	g.press(insetLeftPx+float32(g.natWidth(0))/2, 18)
	if g.s.TabDragIdx != 0 {
		t.Fatalf("precondition: press must arm the first tab, got %d", g.s.TabDragIdx)
	}
	g.advance(200 * time.Millisecond)
	g.drag(insetLeftPx+8, 18+36)

	if sameOrder(g.titles(), before) {
		t.Errorf("dragging onto a lower row must reorder the tabs, order unchanged: %v", g.titles())
	}
	if g.s.TabDragIdx < 0 || g.s.TabDragIdx >= len(g.tabs) {
		t.Errorf("TabDragIdx out of range after a cross-row drag: %d", g.s.TabDragIdx)
	}
}

func TestDragGhost_FallsBackToInfoWidthWhenTabRowIsHidden(t *testing.T) {
	th := newTestTheme()
	s := NewStrip()
	var tabs []*workspace.RequestTab
	for i := 0; i < 30; i++ {
		tabs = append(tabs, workspace.NewRequestTab("tab"))
	}
	active := 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(300, 800)},
	}
	s.Layout(gtx, th, &tabs, &active, true, 1, nil, nil)
	if len(s.rowsBuf) <= 1 {
		t.Fatalf("precondition: expected overflowing rows, got %d", len(s.rowsBuf))
	}

	s.TabDragging = true
	s.TabDragIdx = 0
	s.TabDragCurrentX = 40
	s.TabDragCurrentY = 10

	gtx.Ops = new(op.Ops)
	dims := s.Layout(gtx, th, &tabs, &active, true, 1, nil, nil)
	if dims.Size.Y <= 0 {
		t.Fatal("dragging a tab that lives on a hidden row must still lay out")
	}
	if got := s.infoBuf[0].FinalWidth; got <= 0 {
		t.Errorf("the hidden dragged tab must still have a positive final width, got %d", got)
	}
}

func TestOverflowChevronRow_ShrinksTabsToAtLeastOnePixel(t *testing.T) {
	th := newTestTheme()
	s := NewStrip()
	var tabs []*workspace.RequestTab
	for i := 0; i < 30; i++ {
		tabs = append(tabs, workspace.NewRequestTab("tab"))
	}
	active := 0

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(12, 800)},
	}
	dims := s.Layout(gtx, th, &tabs, &active, true, 2, nil, nil)
	if dims.Size.Y <= 0 {
		t.Fatal("a degenerate-width overflowing bar must still lay out")
	}

	for i, in := range s.infoBuf {
		if in.FinalWidth < 0 {
			t.Errorf("tab %d got a negative final width %d", i, in.FinalWidth)
		}
	}
	shrunk := false
	for _, in := range s.infoBuf {
		if in.FinalWidth == 1 {
			shrunk = true
			break
		}
	}
	if !shrunk {
		t.Errorf("the chevron row must clamp its over-shrunk tab to 1px; widths=%v", s.infoBuf)
	}
}

func TestOverflowChevron_PressTogglesExpandAndCollapse(t *testing.T) {
	g := newDragRig(t, nil, 300, 800)
	g.exact = false
	g.limitRows = true
	g.maxRows = 3
	for i := 0; i < 30; i++ {
		g.tabs = append(g.tabs, workspace.NewRequestTab("tab"))
	}
	g.frame()
	if len(g.s.rowsBuf) <= 3 {
		t.Fatalf("precondition: expected overflowing rows, got %d", len(g.s.rowsBuf))
	}

	g.press(insetLeftPx+8, 18)
	g.release(insetLeftPx+8, 18)
	if !g.s.ExpandRows {
		t.Fatal("pressing the chevron must expand the hidden rows")
	}

	g.press(insetLeftPx+8, 18)
	g.release(insetLeftPx+8, 18)
	if g.s.ExpandRows {
		t.Fatal("pressing the chevron again must collapse the rows")
	}
}

func TestPress_OnChevronDoesNotArmADrag(t *testing.T) {
	g := newDragRig(t, nil, 300, 800)
	g.exact = false
	g.limitRows = true
	g.maxRows = 3
	for i := 0; i < 30; i++ {
		g.tabs = append(g.tabs, workspace.NewRequestTab("tab"))
	}
	g.frame()

	g.press(insetLeftPx+8, 18)
	if g.s.TabDragIdx != -1 {
		t.Errorf("pressing the chevron must not arm a tab drag, got TabDragIdx=%d", g.s.TabDragIdx)
	}
	if g.s.TabDragging {
		t.Error("pressing the chevron must not start a drag")
	}
}

func TestPress_OnEmptySpaceDoesNotArmADrag(t *testing.T) {
	g := newDragRig(t, []string{"alpha"}, 1000, 200)
	g.frame()

	g.press(800, 18)
	if g.s.TabDragIdx != -1 {
		t.Errorf("pressing empty bar space must not arm a drag, got TabDragIdx=%d", g.s.TabDragIdx)
	}

	g.advance(200 * time.Millisecond)
	g.drag(g.centerX(0), 18)
	if g.s.TabDragging {
		t.Error("a drag started from empty space must not become a tab drag")
	}
}

func TestExpandRows_ResetsWhenOverflowGoesAway(t *testing.T) {
	g := newDragRig(t, nil, 300, 800)
	g.exact = false
	g.limitRows = true
	g.maxRows = 3
	for i := 0; i < 30; i++ {
		g.tabs = append(g.tabs, workspace.NewRequestTab("tab"))
	}
	g.frame()
	g.s.ExpandRows = true
	g.frame()
	if !g.s.ExpandRows {
		t.Fatal("precondition: ExpandRows must stay set while the bar overflows")
	}

	g.tabs = g.tabs[:2]
	g.frame()
	if g.s.ExpandRows {
		t.Error("ExpandRows must reset once the bar no longer overflows")
	}
}

func TestDrag_SurvivesTabRemovalBetweenFrames(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo", "charlie", "delta"}, 1000, 200)
	g.frame()

	from := g.centerX(0)
	to := g.centerX(3)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)

	g.queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(to, 18), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	g.tabs = g.tabs[:1]
	g.frame()

	if len(g.tabs) != 1 {
		t.Fatalf("tab list changed unexpectedly: %v", g.titles())
	}
	if g.active < 0 || g.active >= len(g.tabs) {
		t.Errorf("activeIdx %d is out of range for %d tab(s)", g.active, len(g.tabs))
	}
	g.release(to, 18)
}

func TestDrag_DeepRowStackClampsAfterCollapse(t *testing.T) {
	g := newDragRig(t, nil, 300, 900)
	g.exact = false
	for i := 0; i < 24; i++ {
		g.tabs = append(g.tabs, workspace.NewRequestTab("tab"))
	}
	g.frame()
	rows := len(g.s.rowsBuf)
	if rows < 4 {
		t.Fatalf("precondition: expected a tall wrapped bar, got %d row(s)", rows)
	}

	g.press(insetLeftPx+8, float32(rows-2)*36+18)
	if g.s.TabDragIdx < 0 {
		t.Fatalf("precondition: press on the last row must arm a drag, got %d", g.s.TabDragIdx)
	}
	g.advance(200 * time.Millisecond)

	g.queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(insetLeftPx+8, float32(rows-2)*36+18), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	g.tabs = g.tabs[:3]
	g.active = 0
	g.frame()

	if g.active < 0 || g.active >= len(g.tabs) {
		t.Errorf("activeIdx %d out of range after the bar collapsed to %d tabs", g.active, len(g.tabs))
	}
}

func TestLayout_UnicodeAndVeryLongTitles(t *testing.T) {
	g := newDragRig(t, []string{
		"日本語のタブ名",
		"очень длинное название вкладки с несколькими словами",
		"emoji 🚀 title",
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}, 900, 400)
	dims := g.frame()
	if dims.Size.Y <= 0 {
		t.Fatal("unicode titles must still lay out")
	}
	limit := 200
	for i := range g.tabs {
		if w := g.natWidth(i); w <= 0 || w > limit {
			t.Errorf("tab %d width %d is outside (0, %d]", i, w, limit)
		}
	}
}

func TestLayout_DirtyDraggedTabGhost(t *testing.T) {
	g := newDragRig(t, []string{"alpha", "bravo"}, 1000, 200)
	g.tabs[0].IsDirty = true
	g.frame()

	from := g.centerX(0)
	g.press(from, 18)
	g.advance(200 * time.Millisecond)
	g.drag(g.centerX(1), 18)
	if !g.s.TabDragging {
		t.Fatal("precondition: the drag must be active for the ghost to render")
	}
	if d := g.frame(); d.Size.Y <= 0 {
		t.Error("the drag ghost of a dirty tab must lay out")
	}
	g.release(800, 18)
}
