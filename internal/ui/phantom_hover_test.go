//go:build screenshots

package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

// Phantom-hover reproduction.
//
// Root cause: a sidebar row's hover highlight is painted from
// `node.Hover.Update(...)` (collections), `env.Hover.Update(...)` (environments)
// and `row.Hover.Update(...)` (scripts), all called DURING layout. The
// widgets.Hover.entered flag those return is only refreshed by the gio router's
// Enter/Leave pass, which for a frame with no pointer event runs in
// router.Frame() AFTER layout. So when the list content shifts WITHOUT a pointer
// event (a resize-induced re-flow/scroll-clamp, an async tree change, a
// programmatic scroll), the current frame is laid out with the highlight still
// on the OLD row at its NEW screen position — i.e. on the wrong row, away from
// the cursor. The correction only lands on the next frame. During a resize this
// 1-frame lag repeats every frame, so the highlight visibly trails / "floats",
// and it stays on screen until the next render if nothing invalidates.
//
// These tests drive the REAL input.Router + ui.layoutApp and rasterize through
// the real GPU pipeline (gpu/headless), so the screenshots are produced by the
// actual UI code, not a mock.

const phantomShotDir = "testdata/phantom"

type phantomDriver struct {
	t  *testing.T
	ui *AppUI
	r  input.Router
	sz image.Point
}

func newPhantomDriver(t *testing.T, envHeavy bool) *phantomDriver {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "requests"

	root := &collections.CollectionNode{Name: "Sample API", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "col1", Name: "Sample API", Root: root}
	root.Collection = col
	sub := &collections.CollectionNode{Name: "Endpoints", IsFolder: true, Expanded: true, Parent: root, Depth: 1, Collection: col}
	root.Children = append(root.Children, sub)
	for i := 0; i < 40; i++ {
		sub.Children = append(sub.Children, &collections.CollectionNode{
			Name:       fmt.Sprintf("Request %02d", i),
			Request:    &model.ParsedRequest{Name: fmt.Sprintf("Request %02d", i), Method: "GET"},
			Parent:     sub, Depth: 2, Collection: col,
		})
	}
	ui.Collections = []*collections.CollectionUI{{Data: col}}
	ui.updateVisibleCols()

	nEnv := 3
	if envHeavy {
		nEnv = 30
		// give the environments list the whole sidebar so it overflows.
		ui.ColsExpanded = false
		ui.ScriptsExpanded = false
	}
	for i := 0; i < nEnv; i++ {
		env := &environments.EnvironmentUI{Data: &model.ParsedEnvironment{
			ID: fmt.Sprintf("env%02d", i), Name: fmt.Sprintf("Environment %02d", i), HighlightColor: "#3b82f6",
		}}
		env.InitEditor()
		ui.Environments = append(ui.Environments, env)
	}

	return &phantomDriver{t: t, ui: ui, sz: image.Pt(1100, 520)}
}

func (d *phantomDriver) gtx(ops *op.Ops) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(d.sz),
		Now:         fixedTime,
		Source:      d.r.Source(),
	}
}

// tick runs one real frame: queue events -> layout -> router.Frame.
func (d *phantomDriver) tick(events ...pointer.Event) {
	ops := new(op.Ops)
	for _, e := range events {
		d.r.Queue(e)
	}
	d.ui.layoutApp(d.gtx(ops))
	d.r.Frame(ops)
}

func (d *phantomDriver) settle(events ...pointer.Event) { d.tick(events...); d.tick() }

func mv(x, y float32) pointer.Event {
	return pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(x, y)}
}
func scroll(x, y, dy float32) pointer.Event {
	return pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(x, y), Scroll: f32.Pt(0, dy)}
}

func (d *phantomDriver) hoveredCols() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.RowHovered {
			out = append(out, n.Name)
		}
	}
	return out
}
func (d *phantomDriver) hoveredEnvs() []string {
	var out []string
	for _, e := range d.ui.Environments {
		if e.RowHovered {
			out = append(out, e.Data.Name)
		}
	}
	return out
}
func (d *phantomDriver) stickyHovered() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.StickyHovered {
			out = append(out, n.Name)
		}
	}
	return out
}
func (d *phantomDriver) menuHoveredCols() []string {
	var out []string
	for _, n := range d.ui.VisibleCols {
		if n.MenuHovered {
			out = append(out, n.Name)
		}
	}
	return out
}

// shootOps rasterizes a specific ops list (the exact frame that was laid out)
// WITHOUT running another layout that could heal the mis-render.
func (d *phantomDriver) shootOps(name string, ops *op.Ops) {
	win, err := headless.NewWindow(d.sz.X, d.sz.Y)
	if err != nil {
		d.t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()
	if err := win.Frame(ops); err != nil {
		d.t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		d.t.Fatalf("screenshot: %v", err)
	}
	if err := os.MkdirAll(phantomShotDir, 0o755); err != nil {
		d.t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(phantomShotDir, name+".png"))
	if err != nil {
		d.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		d.t.Fatal(err)
	}
	d.t.Logf("wrote %s", filepath.Join(phantomShotDir, name+".png"))
}

// TestPhantomHoverControl: the baseline. Hovering then moving the pointer away
// with a real pointer event leaves no stuck hover (the Leave is delivered).
func TestPhantomHoverControl(t *testing.T) {
	d := newPhantomDriver(t, false)
	var hy float32
	for y := float32(40); y < 300; y += 4 {
		d.settle(mv(120, y))
		if len(d.hoveredCols()) == 1 {
			hy = y
			break
		}
		d.settle(mv(900, 300))
	}
	d.settle(mv(120, hy))
	if got := d.hoveredCols(); len(got) != 1 {
		t.Fatalf("expected one hovered row, got %v", got)
	}
	d.settle(mv(900, 300))
	if got := d.hoveredCols(); len(got) != 0 {
		t.Fatalf("control failed: rows still hovered after moving away: %v", got)
	}
}

// TestPhantomHoverResizeLag is the regression test for the phantom in
// Collections: after a content shift with NO pointer event, the SAME rendered
// frame must already paint the highlight on the row actually under the cursor
// (no one-frame lag onto the stale row).
func TestPhantomHoverResizeLag(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 4; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	const cy = 180
	d.settle(mv(120, cy))
	before := d.hoveredCols()
	if len(before) != 1 {
		t.Skipf("need exactly one hovered row, got %v", before)
	}
	target := before[0]

	// content shift with NO pointer event (resize re-flow / async change / scroll clamp).
	d.ui.ColList.Position.First += 2

	ops := new(op.Ops)
	d.ui.layoutApp(d.gtx(ops))
	during := d.hoveredCols()
	d.shootOps("fixed_resize_shift", ops)
	d.r.Frame(ops)

	settledOps := new(op.Ops)
	d.ui.layoutApp(d.gtx(settledOps))
	settled := d.hoveredCols()
	d.r.Frame(settledOps)

	t.Logf("cursor fixed at y=%d (was over %q before shift)  |  shift frame paints %v  ->  next frame paints %v", cy, target, during, settled)
	if len(during) != 1 || len(settled) != 1 || during[0] != settled[0] {
		t.Fatalf("hover lagged the content shift: shift frame painted %v, next frame painted %v (want one identical row, no lag)", during, settled)
	}
	if during[0] == target {
		t.Fatalf("after shifting the list under a fixed cursor the highlight is still on the pre-shift row %q (phantom not fixed)", target)
	}
	t.Logf("FIXED: shift frame already highlights %v (the row now under the cursor), no phantom", during)
}

// TestPhantomHoverEnvResizeLag confirms the same defect in the Environments
// section (identical Hover-painted-during-layout pattern), i.e. it is not
// Collections-specific.
func TestPhantomHoverEnvResizeLag(t *testing.T) {
	d := newPhantomDriver(t, true)
	for i := 0; i < 4; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	const cy = 180
	d.settle(mv(120, cy))
	before := d.hoveredEnvs()
	if len(before) != 1 {
		t.Skipf("need exactly one hovered env row, got %v", before)
	}

	target := before[0]
	d.ui.EnvList.Position.First += 2
	ops := new(op.Ops)
	d.ui.layoutApp(d.gtx(ops))
	during := d.hoveredEnvs()
	d.shootOps("fixed_env_resize_shift", ops)
	d.r.Frame(ops)

	settledOps := new(op.Ops)
	d.ui.layoutApp(d.gtx(settledOps))
	settled := d.hoveredEnvs()
	d.r.Frame(settledOps)

	t.Logf("cursor fixed at y=%d (was over %q)  |  env shift frame paints %v  ->  next frame paints %v", cy, target, during, settled)
	if len(during) != 1 || len(settled) != 1 || during[0] != settled[0] {
		t.Fatalf("env hover lagged the content shift: shift frame painted %v, next frame painted %v (want identical, no lag)", during, settled)
	}
	t.Logf("FIXED in Environments: shift frame already highlights %v, no phantom", during)
}

// TestStickyBandHoverNoLag verifies the sticky-band row highlight (now geometric)
// does not lag a content shift onto the wrong pinned row.
func TestStickyBandHoverNoLag(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 6; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()
	if d.ui.ColList.Position.First == 0 {
		t.Skip("list did not scroll; no band")
	}

	// hover a pinned band row near the top of the collections area.
	var bandY float32
	for y := float32(48); y < 90; y += 2 {
		d.settle(mv(120, y))
		if len(d.stickyHovered()) == 1 {
			bandY = y
			break
		}
	}
	if bandY == 0 {
		t.Skip("could not hover a band row")
	}
	t.Logf("band hovered at y=%v: %v", bandY, d.stickyHovered())

	// shift the list under the fixed cursor with NO pointer event.
	d.ui.ColList.Position.First += 3

	ops := new(op.Ops)
	d.ui.layoutApp(d.gtx(ops))
	during := d.stickyHovered()
	d.r.Frame(ops)

	d.tick()
	settled := d.stickyHovered()

	t.Logf("after shift: band shift-frame=%v  next-frame=%v", during, settled)
	if len(during) > 1 {
		t.Fatalf("more than one band row highlighted at once: %v", during)
	}
	if strings.Join(during, ",") != strings.Join(settled, ",") {
		t.Fatalf("sticky band hover lagged the shift: shift frame=%v, next frame=%v", during, settled)
	}
}

// TestStickyBandHoverControl: moving the cursor out of the sidebar clears the
// band highlight (no stuck hover).
func TestStickyBandHoverControl(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 6; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()
	for y := float32(48); y < 90; y += 2 {
		d.settle(mv(120, y))
		if len(d.stickyHovered()) == 1 {
			break
		}
	}
	d.settle(mv(900, 300))
	if got := d.stickyHovered(); len(got) != 0 {
		t.Fatalf("band row stayed highlighted after cursor left: %v", got)
	}
}

// TestMenuIconHoverZone checks the geometric "⋮" zone: hovering the right edge of
// a row lights its menu icon, hovering the left does not, and it never lags onto
// the wrong row after a content shift.
func TestMenuIconHoverZone(t *testing.T) {
	d := newPhantomDriver(t, false)
	for i := 0; i < 3; i++ {
		d.tick(scroll(120, 200, 40))
	}
	d.settle()

	// find a row Y that hovers exactly one row at left x.
	var rowY float32
	for y := float32(60); y < 300; y += 4 {
		d.settle(mv(40, y))
		if len(d.hoveredCols()) == 1 {
			rowY = y
			break
		}
	}
	if rowY == 0 {
		t.Skip("no row found")
	}

	// left side: row hovered, menu icon NOT hovered.
	d.settle(mv(40, rowY))
	if got := d.menuHoveredCols(); len(got) != 0 {
		t.Fatalf("menu icon hovered while cursor is on the left of the row: %v", got)
	}
	rowName := d.hoveredCols()

	// right edge (the ⋮ sits ~18dp wide, 10dp from the right; sidebar body is
	// narrower than the window, so probe near the body's right edge).
	var menuOn []string
	for x := float32(d.ui.SidebarWidth); x > float32(d.ui.SidebarWidth)-60; x -= 2 {
		d.settle(mv(x, rowY))
		if len(d.menuHoveredCols()) == 1 {
			menuOn = d.menuHoveredCols()
			break
		}
	}
	if len(menuOn) == 0 {
		t.Fatalf("could not hover the ⋮ zone on the right of row %v", rowName)
	}
	t.Logf("⋮ zone hovered for %v", menuOn)
	if strings.Join(menuOn, ",") != strings.Join(rowName, ",") {
		t.Fatalf("⋮ hover is on a different row (%v) than the row hover (%v)", menuOn, rowName)
	}
}
