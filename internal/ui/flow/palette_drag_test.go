package flow

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"

	"rete/internal/ui/widgets"
)

func TestCanvasPaletteDragNoMarquee(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	rig.frame()
	rig.frame()
	r1 := ed.paletteRects[1]
	start := f32.Pt(float32(r1.Min.X+r1.Dx()/2), float32(r1.Min.Y+r1.Dy()/2))
	target := f32.Pt(500, 250)
	widgets.GlobalPointerPos = start
	rig.r.Queue(rig.timedEvent(pointer.Move, start, 0), rig.timedEvent(pointer.Press, start, pointer.ButtonPrimary))
	rig.frame()
	if ed.marquee || ed.panning || ed.dragNodeID != "" {
		t.Errorf("pressing a palette button must not start a canvas gesture: marquee=%v panning=%v drag=%q", ed.marquee, ed.panning, ed.dragNodeID)
	}
	if !ed.palDragOn {
		t.Fatal("pressing a palette button must arm the palette drag")
	}
	widgets.GlobalPointerPos = target
	rig.r.Queue(rig.timedEvent(pointer.Move, target, pointer.ButtonPrimary))
	rig.frame()
	if !ed.palDragActive {
		t.Error("dragging over the canvas must arm the drop")
	}
	if ed.marquee {
		t.Error("dragging a palette button must not start a marquee selection on the canvas")
	}
	if len(ed.ghostNodes) != 1 || ed.ghostNodes[0].Kind != KindWSRequest {
		t.Fatalf("the drag ghost must be a real node of the dragged kind, got %d nodes", len(ed.ghostNodes))
	}
	want := ed.toWorld(target)
	g := ed.ghostNodes[0]
	if g.X != want.X-ed.nodeW/2 || g.Y != want.Y-ed.nodeH/2 {
		t.Errorf("ghost at (%v,%v), want centred on the pointer (%v,%v)", g.X, g.Y, want.X-ed.nodeW/2, want.Y-ed.nodeH/2)
	}
	gx, gy := g.X, g.Y
	rig.r.Queue(rig.timedEvent(pointer.Release, target, 0))
	rig.frame()
	if len(ed.ghostNodes) != 0 {
		t.Error("the ghost must disappear once the node is dropped")
	}
	last := ed.Scenario.Nodes[len(ed.Scenario.Nodes)-1]
	if last.Kind != KindWSRequest || last.X != gx || last.Y != gy {
		t.Errorf("dropped node must land exactly where the ghost was: got %v (%v,%v), ghost (%v,%v)", last.Kind, last.X, last.Y, gx, gy)
	}
}

func TestCanvasCustomWidgetsMenu(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	if err := SaveBlock(scenarioDTO{Name: "Auth pair", Nodes: []nodeDTO{
		{Kind: int(KindRequest), X: 0, Y: 0, Name: "Login", Method: "POST"},
		{Kind: int(KindSetVar), X: 260, Y: 0, Name: "Store token"},
	}}); err != nil {
		t.Fatal(err)
	}
	rig.frame()
	rig.frame()
	if ed.customRect.Empty() {
		t.Fatal("the custom widgets button must be part of the toolbar")
	}
	center := func(r image.Rectangle) f32.Point {
		return f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2))
	}
	rig.timedClick(center(ed.customRect))
	if !ed.customMenuOpen {
		t.Fatal("clicking the custom button must open the menu")
	}
	rig.frame()
	if len(ed.customRows) != 1 || ed.customPopup.Empty() {
		t.Fatalf("the menu must list the saved block, rows=%d popup=%v", len(ed.customRows), ed.customPopup)
	}
	if ed.customRows[0].Max.Y > ed.customRect.Min.Y {
		t.Errorf("the menu must open above the button, row %v button %v", ed.customRows[0], ed.customRect)
	}
	before := len(ed.Scenario.Nodes)
	rig.timedClick(center(ed.customRows[0]))
	if len(ed.Scenario.Nodes) != before+2 {
		t.Fatalf("clicking a block must insert its nodes, nodes = %d", len(ed.Scenario.Nodes))
	}
	if ed.customMenuOpen {
		t.Error("inserting a block must close the menu")
	}

	rig.timedClick(center(ed.customRect))
	rig.frame()
	if !ed.customMenuOpen || len(ed.customRows) != 1 {
		t.Fatal("the menu must reopen")
	}
	row := center(ed.customRows[0])
	target := f32.Pt(450, 200)
	widgets.GlobalPointerPos = row
	rig.r.Queue(rig.timedEvent(pointer.Move, row, 0), rig.timedEvent(pointer.Press, row, pointer.ButtonPrimary))
	rig.frame()
	widgets.GlobalPointerPos = target
	rig.r.Queue(rig.timedEvent(pointer.Move, target, pointer.ButtonPrimary))
	rig.frame()
	if !ed.blockDragActive {
		t.Fatal("dragging a block row over the canvas must arm the drop")
	}
	rig.frame()
	if len(ed.ghostNodes) != 2 {
		t.Errorf("the block ghost must show its 2 nodes, got %d", len(ed.ghostNodes))
	}
	before = len(ed.Scenario.Nodes)
	rig.r.Queue(rig.timedEvent(pointer.Release, target, 0))
	rig.frame()
	rig.frame()
	if len(ed.Scenario.Nodes) != before+2 {
		t.Fatalf("releasing over the canvas must drop the block, nodes = %d", len(ed.Scenario.Nodes))
	}
	if ed.customMenuOpen {
		t.Error("dragging a block out must close the menu")
	}
	w := ed.toWorld(target)
	minX, maxX := ed.Scenario.Nodes[before].X, ed.Scenario.Nodes[before].X
	for _, n := range ed.Scenario.Nodes[before:] {
		if n.X < minX {
			minX = n.X
		}
		if nw, _ := ed.nodeWH(n); n.X+nw > maxX {
			maxX = n.X + nw
		}
	}
	if minX > w.X || maxX < w.X {
		t.Errorf("dropped block %v..%v must span the pointer x %v", minX, maxX, w.X)
	}
}
