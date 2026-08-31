package flow

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func nodeCenter(ed *Editor, n *Node) f32.Point {
	sp, w, h := ed.nodeScreenRect(n)
	return f32.Pt(sp.X+w/2, sp.Y+h/2)
}

func TestHoverNodeAt(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 400)

	if got := ed.hoverNodeAt(nodeCenter(ed, a)); got != a.ID {
		t.Errorf("hover over a = %q, want %q", got, a.ID)
	}
	if got := ed.hoverNodeAt(ed.toScreen(ed.outPort(b))); got != b.ID {
		t.Errorf("hover on b's out port must count as hovering b, got %q", got)
	}
	if got := ed.hoverNodeAt(f32.Pt(2000, 2000)); got != "" {
		t.Errorf("hover over empty canvas = %q, want none", got)
	}
}

func TestHoverLoopBodyDoesNotCount(t *testing.T) {
	ed := newBlankEditor()
	loop := addNodeTo(ed, KindLoop, 100, 100)
	loop.W, loop.H = 500, 400

	if got := ed.hoverNodeAt(f32.Pt(350, 400)); got != "" {
		t.Errorf("hovering the empty loop body = %q, want none", got)
	}
	header := f32.Pt(300, 120)
	if got := ed.hoverNodeAt(header); got != loop.ID {
		t.Errorf("hovering the loop header = %q, want the loop", got)
	}
	if got := ed.hoverNodeAt(ed.toScreen(ed.outPortBottom(loop))); got != loop.ID {
		t.Errorf("hovering the loop bottom port = %q, want the loop", got)
	}
}

func TestPortVisibility(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 100)

	t.Run("hidden without hover", func(t *testing.T) {
		ed.hoverNodeID = ""
		ed.connectFromID = ""
		in, out := ed.portVisibility(a)
		if in || out {
			t.Errorf("ports must stay hidden when nothing is hovered, got in=%v out=%v", in, out)
		}
	})

	t.Run("shown on the hovered node", func(t *testing.T) {
		ed.hoverNodeID = a.ID
		ed.connectFromID = ""
		in, out := ed.portVisibility(a)
		if !in || !out {
			t.Errorf("hovered node must show both port sides, got in=%v out=%v", in, out)
		}
		if in, out := ed.portVisibility(b); in || out {
			t.Errorf("other nodes stay hidden, got in=%v out=%v", in, out)
		}
	})

	t.Run("while connecting targets show in ports", func(t *testing.T) {
		ed.hoverNodeID = ""
		ed.connectFromID = a.ID
		if in, out := ed.portVisibility(b); !in || out {
			t.Errorf("drop target shows only in ports, got in=%v out=%v", in, out)
		}
		if in, out := ed.portVisibility(a); in || !out {
			t.Errorf("connection source shows only its out port, got in=%v out=%v", in, out)
		}
	})

	t.Run("hover updates from pointer moves", func(t *testing.T) {
		ed.connectFromID = ""
		ed.setHover(nodeCenter(ed, b))
		ed.refreshHoverNode()
		if ed.hoverNodeID != b.ID {
			t.Fatalf("hoverNodeID = %q, want %q", ed.hoverNodeID, b.ID)
		}
		ed.hoverOn = false
		ed.refreshHoverNode()
		if ed.hoverNodeID != "" {
			t.Errorf("leaving the canvas must clear the hover, got %q", ed.hoverNodeID)
		}
	})
}

func TestCanvasReceivesHoverMoves(t *testing.T) {
	setupFlowConfig(t)
	sz := image.Pt(900, 600)
	var r input.Router
	ed := NewEditor()
	ed.Scenario = NewScenario()
	ed.pendingFit = false
	ed.zoom = 1
	ed.pan = f32.Point{}
	n := addNodeTo(ed, KindRequest, 200, 200)
	host := &Host{Win: testWindow(), WinSize: sz}

	frame := func() {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Source:      r.Source(),
			Now:         time.Now(),
		}
		ed.Layout(gtx, material.NewTheme(), host)
		r.Frame(gtx.Ops)
	}

	frame()
	center := nodeCenter(ed, n)
	r.Queue(pointer.Event{Kind: pointer.Move, Position: center, Source: pointer.Mouse})
	frame()

	if !ed.hoverOn {
		t.Fatal("a pointer move over the canvas must reach the canvas handler")
	}
	if ed.hoverNodeID != n.ID {
		t.Errorf("hoverNodeID = %q, want %q", ed.hoverNodeID, n.ID)
	}

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(sz.X)-20, 40), Source: pointer.Mouse})
	frame()
	if ed.hoverOn {
		t.Error("moving onto the side panel must leave the canvas")
	}
	if ed.hoverNodeID != "" {
		t.Errorf("leaving the canvas must hide the ports, hoverNodeID = %q", ed.hoverNodeID)
	}
}

func TestSingleOutgoingEdge(t *testing.T) {
	t.Run("press on an occupied out port does not start a connection", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		e := connect(ed, a, b)

		ed.onPress(press(ed.toScreen(ed.outPort(a))))
		if ed.connectFromID != "" {
			t.Fatalf("connectFromID = %q, want none", ed.connectFromID)
		}
		if ed.reconnectEdge != nil {
			t.Error("pressing an occupied out port must not detach the edge")
		}
		ed.onRelease(ed.toScreen(ed.outPort(a)))
		if ed.Scenario.EdgeByID(e.ID) == nil {
			t.Error("the existing edge must stay untouched")
		}
		if len(ed.undoStack) != 0 {
			t.Errorf("a no-op click must not push history, got %d snapshots", len(ed.undoStack))
		}
	})

	t.Run("dragging from an occupied out port never moves the edge", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		c := addNodeTo(ed, KindDelay, 500, 400)
		e := connect(ed, a, b)

		ed.onPress(press(ed.toScreen(ed.outPort(a))))
		ed.onDrag(f32.Pt(520, 420))
		if ed.reconnectEdge != nil {
			t.Fatalf("dragging must not detach the existing edge, got %v", ed.reconnectEdge)
		}
		ed.onRelease(nodeCenter(ed, c))

		if len(ed.Scenario.Edges) != 1 {
			t.Fatalf("the node must keep exactly one outgoing edge, got %d", len(ed.Scenario.Edges))
		}
		got := ed.Scenario.Edges[0]
		if got != e || got.To != b.ID {
			t.Errorf("edge = %v → %q, want the original edge to %q", got == e, got.To, b.ID)
		}
	})

	t.Run("occupied bottom port cannot start a connection", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		c := addNodeTo(ed, KindDelay, 100, 500)
		e := connect(ed, a, b)

		ed.onPress(press(ed.toScreen(ed.outPortBottom(a))))
		if ed.connectFromID != "" {
			t.Fatalf("connectFromID = %q, want none", ed.connectFromID)
		}
		ed.onDrag(f32.Pt(140, 520))
		ed.onRelease(ed.toScreen(ed.inPortSide(c, SideTop)))

		if len(ed.Scenario.Edges) != 1 {
			t.Fatalf("the second out port must not add an edge, got %d", len(ed.Scenario.Edges))
		}
		got := ed.Scenario.Edges[0]
		if got != e || got.To != b.ID {
			t.Errorf("edge = %v → %q, want the original edge to %q", got == e, got.To, b.ID)
		}
	})

	t.Run("only the connected out port stays visible", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)

		if r, bo := ed.outPortSidesShown(a); !r || !bo {
			t.Errorf("free node must offer both out sides, got right=%v bottom=%v", r, bo)
		}

		e := connect(ed, a, b)
		if r, bo := ed.outPortSidesShown(a); !r || bo {
			t.Errorf("right-connected node must show only the right port, got right=%v bottom=%v", r, bo)
		}

		e.FromSide = SideBottom
		if r, bo := ed.outPortSidesShown(a); r || !bo {
			t.Errorf("bottom-connected node must show only the bottom port, got right=%v bottom=%v", r, bo)
		}

		cond := addNodeTo(ed, KindCondition, 100, 400)
		if r, bo := ed.outPortSidesShown(cond); r || bo {
			t.Errorf("condition slots are drawn separately, got right=%v bottom=%v", r, bo)
		}
	})

	t.Run("release cannot add a second outgoing edge", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		c := addNodeTo(ed, KindDelay, 500, 400)
		connect(ed, a, b)

		ed.connectFromID = a.ID
		ed.onRelease(nodeCenter(ed, c))
		if len(ed.Scenario.Edges) != 1 {
			t.Fatalf("a second outgoing edge must be rejected, got %d", len(ed.Scenario.Edges))
		}
		if ed.Scenario.Edges[0].To != b.ID {
			t.Error("the original edge must stay untouched")
		}
	})

	t.Run("condition keeps branching", func(t *testing.T) {
		ed := newBlankEditor()
		cond := addNodeTo(ed, KindCondition, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		c := addNodeTo(ed, KindDelay, 500, 400)
		connect(ed, cond, b)

		ed.onPress(press(ed.toScreen(ed.outPortAt(cond, 1))))
		ed.onRelease(nodeCenter(ed, c))
		if len(ed.Scenario.Edges) != 2 {
			t.Fatalf("condition must keep multiple outgoing edges, got %d", len(ed.Scenario.Edges))
		}
	})

	t.Run("incoming edges are not capped", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindRequest, 100, 400)
		target := addNodeTo(ed, KindDelay, 600, 250)
		connect(ed, a, target)

		ed.onPress(press(ed.toScreen(ed.outPort(b))))
		ed.onRelease(nodeCenter(ed, target))
		if len(ed.Scenario.Edges) != 2 {
			t.Fatalf("a node may receive several edges, got %d", len(ed.Scenario.Edges))
		}
	})
}
