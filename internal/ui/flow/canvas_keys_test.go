package flow

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type canvasRig struct {
	r    input.Router
	ed   *Editor
	host *Host
	sz   image.Point
}

func newCanvasRig(t *testing.T) *canvasRig {
	t.Helper()
	setupFlowConfig(t)
	rig := &canvasRig{sz: image.Pt(900, 600)}
	rig.ed = NewEditor()
	rig.ed.Scenario = NewScenario()
	rig.ed.pendingFit = false
	rig.ed.zoom = 1
	rig.ed.pan = f32.Point{}
	rig.host = &Host{Win: testWindow(), WinSize: rig.sz}
	return rig
}

func (rig *canvasRig) frame() {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         time.Now(),
	}
	rig.ed.Layout(gtx, material.NewTheme(), rig.host)
	rig.r.Frame(gtx.Ops)
}

func (rig *canvasRig) click(pt f32.Point) {
	rig.r.Queue(
		pointer.Event{Kind: pointer.Move, Position: pt, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Press, Position: pt, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: pt, Source: pointer.Mouse},
	)
	rig.frame()
	rig.frame()
}

func (rig *canvasRig) key(name key.Name) {
	rig.r.Queue(key.Event{Name: name, State: key.Press}, key.Event{Name: name, State: key.Release})
	rig.frame()
	rig.frame()
}

func TestCanvasBodyEditorTypingAndKeys(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	rig.frame()

	box, ok := ed.bodyBoxRect(n)
	if !ok {
		t.Fatal("request node must expose a body box")
	}
	inBody := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y+box.Dy()/2))
	rig.click(inBody)
	if !rig.r.Source().Focused(&n.BodyEd) {
		t.Fatal("clicking the body box must focus the body editor")
	}
	if !ed.selected[n.ID] {
		t.Error("clicking the body box selects the node")
	}
	if ed.hoverNodeID != n.ID {
		t.Errorf("the body box is part of the node for hovering, hoverNodeID = %q", ed.hoverNodeID)
	}

	rig.r.Queue(key.EditEvent{Text: "abc"})
	rig.frame()
	rig.frame()
	if got := n.BodyEd.Text(); got != "abc" {
		t.Fatalf("typing must reach the body editor, got %q", got)
	}
	n.BodyEd.SetCaret(n.BodyEd.Len(), n.BodyEd.Len())

	rig.key(key.NameDeleteBackward)
	if ed.Scenario.NodeByID(n.ID) == nil {
		t.Fatal("Backspace inside the body editor must edit text, not delete the selected node")
	}
	if got := n.BodyEd.Text(); got != "ab" {
		t.Errorf("Backspace must edit the body, got %q", got)
	}
	rig.key(key.NameDeleteForward)
	if ed.Scenario.NodeByID(n.ID) == nil {
		t.Fatal("Delete inside the body editor must not delete the node")
	}

	header := f32.Pt(float32(box.Min.X+box.Dx()/2), float32(box.Min.Y)-20)
	rig.click(header)
	if !rig.r.Source().Focused(ed) {
		t.Fatal("clicking the node header must move focus to the canvas")
	}
	rig.key(key.NameDeleteForward)
	if ed.Scenario.NodeByID(n.ID) != nil {
		t.Fatal("Delete on the focused canvas must delete the selected node")
	}
}

func TestCanvasBodyPressDoesNotDragNode(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	start := f32.Pt(float32(box.Min.X+10), float32(box.Min.Y+10))
	rig.r.Queue(
		pointer.Event{Kind: pointer.Move, Position: start, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Press, Position: start, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Move, Position: start.Add(f32.Pt(80, 40)), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary},
		pointer.Event{Kind: pointer.Release, Position: start.Add(f32.Pt(80, 40)), Source: pointer.Mouse},
	)
	rig.frame()
	rig.frame()
	if n.X != 200 || n.Y != 200 {
		t.Errorf("dragging inside the body box selects text, it must not move the node, got (%v,%v)", n.X, n.Y)
	}
	if ed.marquee {
		t.Error("no marquee may start from the body box")
	}
}
