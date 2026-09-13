package flow

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

func TestCanvasOverlappingNodesKeepInputSeparate(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	under := addNodeTo(ed, KindRequest, 200, 200)
	over := addNodeTo(ed, KindRequest, 240, 230)
	body := "{"
	for i := 0; i < 12; i++ {
		body += "\n  \"k" + itoa(i) + "\": 1,"
	}
	body += "\n}"
	under.BodyEd.SetText(body)
	over.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	ub := under.CanvasBodyEditor()
	ob := over.CanvasBodyEditor()
	obox, _ := ed.bodyBoxRect(over)
	pt := f32.Pt(float32(obox.Min.X+10), float32(obox.Min.Y+8))
	rig.r.Queue(rig.timedEvent(pointer.Move, pt, 0), pointer.Event{Kind: pointer.Scroll, Position: pt, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
	rig.frame()
	rig.frame()
	t.Logf("scroll over top body: top scrollY=%d under scrollY=%d", ob.GetScrollY(), ub.GetScrollY())
	if ub.GetScrollY() != 0 {
		t.Error("wheel over the top node's body must not scroll the node underneath")
	}
	if ob.GetScrollY() == 0 {
		t.Error("wheel over the top node's body must scroll its own text")
	}
	for i := 0; i < 25; i++ {
		rig.r.Queue(rig.timedEvent(pointer.Move, pt, 0), pointer.Event{Kind: pointer.Scroll, Position: pt, Source: pointer.Mouse, Scroll: f32.Pt(0, 200), Time: rig.clock})
		rig.frame()
	}
	rig.frame()
	if ub.GetScrollY() != 0 {
		t.Errorf("wheel past the end of the top body must not leak into the node underneath, its scrollY = %d", ub.GetScrollY())
	}
	if ob.GetScrollY() != ob.GetScrollBounds().Max.Y {
		t.Errorf("top body must be scrolled to its end, got %d of %d", ob.GetScrollY(), ob.GetScrollBounds().Max.Y)
	}

	ubox, _ := ed.bodyBoxRect(under)
	hdr := f32.Pt(float32(over.X)+60, float32(ubox.Min.Y)+8)
	if !image.Pt(int(hdr.X), int(hdr.Y)).In(ubox) || hdr.Y < over.Y || hdr.Y > over.Y+ed.nodeH {
		t.Fatalf("setup: the point %v must lie in the top header and over the body box %v of the node underneath", hdr, ubox)
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr, 0), pointer.Event{Kind: pointer.Scroll, Position: hdr, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
	rig.frame()
	rig.frame()
	if ub.GetScrollY() != 0 {
		t.Errorf("wheel over the top node's header must not scroll the body of the node underneath, scrollY = %d", ub.GetScrollY())
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr, 0), rig.timedEvent(pointer.Press, hdr, pointer.ButtonPrimary))
	rig.frame()
	if rig.r.Source().Focused(ub) {
		t.Error("pressing the top node's header must not focus the body editor of the node underneath")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr.Add(f32.Pt(50, 40)), pointer.ButtonPrimary))
	rig.frame()
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr.Add(f32.Pt(100, 80)), pointer.ButtonPrimary))
	rig.frame()
	rig.r.Queue(rig.timedEvent(pointer.Release, hdr.Add(f32.Pt(100, 80)), 0))
	rig.frame()
	t.Logf("after header drag: over=(%v,%v) under=(%v,%v) dragNode=%q", over.X, over.Y, under.X, under.Y, ed.dragNodeID)
	if over.X != 340 || over.Y != 310 {
		t.Errorf("dragging the top node's header must move it by (100,80), got (%v,%v)", over.X, over.Y)
	}
	if under.X != 200 || under.Y != 200 {
		t.Errorf("the node underneath must stay, got (%v,%v)", under.X, under.Y)
	}
}
