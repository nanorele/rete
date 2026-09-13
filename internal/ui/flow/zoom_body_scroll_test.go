package flow

import (
	"strings"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

func TestCanvasZoomKeepsBodyTopLine(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 40; i++ {
		b.WriteString("\n  \"key")
		b.WriteString(itoa(i))
		b.WriteString("\": \"value\",")
	}
	b.WriteString("\n}")
	n.BodyEd.SetText(b.String())
	lines := 42
	rig.frame()
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	inBody := f32.Pt(float32(box.Min.X+20), float32(box.Min.Y+20))
	ce := n.CanvasBodyEditor()
	for i := 0; i < 7; i++ {
		rig.r.Queue(rig.timedEvent(pointer.Move, inBody, 0), pointer.Event{Kind: pointer.Scroll, Position: inBody, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
		rig.frame()
		rig.frame()
	}
	topLine := func() float32 {
		bx, _ := ed.bodyBoxRect(n)
		pad := int(4 * ed.zoom)
		if pad < 1 {
			pad = 1
		}
		viewH := bx.Dy() - 1 - 2*pad
		contentH := ce.GetScrollBounds().Max.Y + viewH
		return float32(ce.GetScrollY()) * float32(lines) / float32(contentH)
	}
	tl0 := topLine()
	worst := float32(0)
	for step := 0; step < 24; step++ {
		ed.zoomByNotches(f32.Pt(50, 50), -1)
		rig.frame()
		d := topLine() - tl0
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	for step := 0; step < 24; step++ {
		ed.zoomByNotches(f32.Pt(50, 50), 1)
		rig.frame()
		d := topLine() - tl0
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	t.Logf("back at zoom %v scrollY %d topLine %.2f, worst drift %.2f lines", ed.zoom, ce.GetScrollY(), topLine(), worst)
	if worst > 0.15 {
		t.Errorf("top line drifts by up to %.2f lines across zoom levels", worst)
	}
}
