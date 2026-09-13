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
		if ed.showPorts(a) {
			t.Error("ports must stay hidden when nothing is hovered")
		}
	})

	t.Run("shown on the hovered node only", func(t *testing.T) {
		ed.hoverNodeID = a.ID
		ed.connectFromID = ""
		if !ed.showPorts(a) {
			t.Error("hovered node must show its ports")
		}
		if ed.showPorts(b) {
			t.Error("other nodes stay hidden")
		}
	})

	t.Run("hovering a connected node lights nothing else", func(t *testing.T) {
		c := addNodeTo(ed, KindDelay, 500, 400)
		connect(ed, a, b)
		ed.hoverNodeID = a.ID
		ed.connectFromID = ""
		if ed.showPorts(b) || ed.showPorts(c) {
			t.Error("hovering a node with an arrow must not reveal ports on other nodes")
		}
		ed.Scenario.Edges = nil
		ed.Scenario.RemoveNode(c.ID)
	})

	t.Run("while connecting only the anchored node and the hovered target show ports", func(t *testing.T) {
		c := addNodeTo(ed, KindDelay, 500, 400)
		ed.hoverNodeID = b.ID
		ed.connectFromID = a.ID
		if !ed.showPorts(a) {
			t.Error("connection source keeps its ports visible")
		}
		if !ed.showPorts(b) {
			t.Error("hovered drop target shows its ports")
		}
		if ed.showPorts(c) {
			t.Error("non-hovered nodes must not light up while connecting")
		}
		ed.Scenario.RemoveNode(c.ID)
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
	t.Run("press on an occupied out port grabs the arrow tail", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		e := connect(ed, a, b)

		ed.onPress(press(ed.toScreen(ed.outPort(a))))
		if ed.connectFromID != "" {
			t.Fatalf("connectFromID = %q, want none: the tail is the free end", ed.connectFromID)
		}
		if ed.connectToID != b.ID || ed.connectToSide != SideLeft {
			t.Fatalf("anchored target = %q/%q, want %q/%q", ed.connectToID, ed.connectToSide, b.ID, SideLeft)
		}
		if ed.reconnectEdge != e {
			t.Error("the pressed edge must be the one being moved")
		}
		ed.onRelease(ed.toScreen(ed.outPort(a)))
		got := ed.Scenario.EdgeByID(e.ID)
		if got == nil || got.From != a.ID || got.FromSide != SideRight || got.To != b.ID {
			t.Errorf("releasing on the same port must put the arrow back unchanged, got %+v", got)
		}
		if len(ed.undoStack) != 1 {
			t.Errorf("a grab pushes exactly one snapshot, got %d", len(ed.undoStack))
		}
	})

	t.Run("dragging the tail re-sources the arrow without touching others", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		c := addNodeTo(ed, KindDelay, 500, 400)
		d := addNodeTo(ed, KindDelay, 900, 400)
		e := connect(ed, a, b)
		other := connect(ed, c, d)

		ed.onPress(press(ed.toScreen(ed.outPort(a))))
		ed.onDrag(f32.Pt(520, 420))
		if ed.reconnectEdge != e {
			t.Fatalf("dragging must move the grabbed edge, got %v", ed.reconnectEdge)
		}
		ed.onRelease(nodeCenter(ed, c))
		if len(ed.Scenario.Edges) != 2 {
			t.Fatalf("edge count must stay 2, got %d", len(ed.Scenario.Edges))
		}
		if e.From != a.ID || e.To != b.ID {
			t.Errorf("c already has an outgoing arrow, so the move must be refused and %q → %q restored, got %q → %q", a.ID, b.ID, e.From, e.To)
		}
		if other.From != c.ID || other.To != d.ID {
			t.Error("the other arrow must never be affected")
		}

		ed.onPress(press(ed.toScreen(ed.outPort(a))))
		ed.onRelease(nodeCenter(ed, d))
		if e.From != d.ID || e.To != b.ID || e.FromSide == "" {
			t.Errorf("dropping the tail on a free node re-sources the arrow, got %q → %q side %q", e.From, e.To, e.FromSide)
		}
		if other.From != c.ID || other.To != d.ID {
			t.Error("the other arrow must never be affected")
		}
	})

	t.Run("shared in port moves the selected arrow, never the other one", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 50)
		b := addNodeTo(ed, KindRequest, 100, 400)
		target := addNodeTo(ed, KindDelay, 600, 250)
		free := addNodeTo(ed, KindDelay, 600, 600)
		ea := connect(ed, a, target)
		eb := connect(ed, b, target)

		ed.selEdgeID = ea.ID
		ed.onPress(press(ed.toScreen(ed.inPort(target))))
		if ed.reconnectEdge != ea || ed.connectFromID != a.ID {
			t.Fatalf("the selected arrow must be the grabbed one, got %v from %q", ed.reconnectEdge, ed.connectFromID)
		}
		if ed.Scenario.EdgeByID(eb.ID) == nil {
			t.Fatal("the other arrow must stay attached while dragging")
		}
		ed.onRelease(nodeCenter(ed, free))
		if ea.To != free.ID {
			t.Errorf("selected arrow must be re-targeted, got → %q", ea.To)
		}
		if eb.From != b.ID || eb.To != target.ID || ed.Scenario.EdgeByID(eb.ID) == nil {
			t.Error("the other arrow must be untouched")
		}

		ed.selEdgeID = ""
		ed.onPress(press(ed.toScreen(ed.inPort(target))))
		if ed.reconnectEdge != eb {
			t.Fatalf("with nothing selected the topmost arrow is grabbed, got %v", ed.reconnectEdge)
		}
		ed.onRelease(ed.toScreen(ed.inPortSide(target, SideTop)))
		if eb.To != target.ID || eb.ToSide != SideTop {
			t.Errorf("an arrow can be moved between ports of the same node, got → %q side %q", eb.To, eb.ToSide)
		}
		if ea.To != free.ID {
			t.Error("the previously moved arrow must be untouched")
		}
	})

	t.Run("grabbing the arrow head near its end", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 600, 100)
		c := addNodeTo(ed, KindDelay, 600, 400)
		e := connect(ed, a, b)

		from, to := ed.Scenario.NodeByID(e.From), ed.Scenario.NodeByID(e.To)
		w0, w1, o0, o1 := ed.edgeGeom(e, from, to)
		p0, p1 := ed.toScreen(w0), ed.toScreen(w1)
		c0, c1 := ed.edgeControlsDir(p0, p1, o0, o1)
		ed.onPress(press(bezierAt(p0, c0, c1, p1, 0.9)))
		if ed.reconnectEdge != e || ed.connectFromID != a.ID {
			t.Fatalf("pressing near the head must grab the head, got %v from %q", ed.reconnectEdge, ed.connectFromID)
		}
		ed.onRelease(nodeCenter(ed, c))
		if e.To != c.ID {
			t.Errorf("head dropped on c must re-target, got → %q", e.To)
		}

		from, to = ed.Scenario.NodeByID(e.From), ed.Scenario.NodeByID(e.To)
		w0, w1, o0, o1 = ed.edgeGeom(e, from, to)
		p0, p1 = ed.toScreen(w0), ed.toScreen(w1)
		c0, c1 = ed.edgeControlsDir(p0, p1, o0, o1)
		ed.onPress(press(bezierAt(p0, c0, c1, p1, 0.5)))
		if ed.reconnectEdge != nil || ed.selEdgeID != e.ID {
			t.Error("pressing the middle of an arrow selects it")
		}
		ed.onRelease(bezierAt(p0, c0, c1, p1, 0.5))
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

	t.Run("occupied ports follow the edge sides", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)

		for _, side := range allSides {
			if got := ed.edgesAtPort(a, side); len(got) != 0 {
				t.Errorf("free node has no occupied ports, side %q got %d", side, len(got))
			}
		}

		e := connect(ed, a, b)
		if got := ed.edgesAtPort(a, SideRight); len(got) != 1 || got[0] != e {
			t.Errorf("right port of the source must hold the edge, got %v", got)
		}
		if got := ed.edgesAtPort(b, SideLeft); len(got) != 1 || got[0] != e {
			t.Errorf("left port of the target must hold the edge, got %v", got)
		}

		e.FromSide = SideBottom
		e.ToSide = SideRight
		if got := ed.edgesAtPort(a, SideRight); len(got) != 0 {
			t.Error("moving the tail to the bottom frees the right port")
		}
		if got := ed.edgesAtPort(a, SideBottom); len(got) != 1 {
			t.Error("bottom port must now be occupied")
		}
		if got := ed.edgesAtPort(b, SideRight); len(got) != 1 {
			t.Error("an arrow may enter through the right port")
		}
	})

	t.Run("start node offers four ports", func(t *testing.T) {
		ed := newBlankEditor()
		start := addNodeTo(ed, KindStart, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		for _, side := range allSides {
			ed.onPress(press(ed.toScreen(ed.portPos(start, side))))
			if ed.connectFromID != start.ID || ed.connectFromSide != side {
				t.Errorf("side %q: press must start a connection from the start node, got from=%q side=%q", side, ed.connectFromID, ed.connectFromSide)
			}
			ed.onRelease(f32.Pt(2000, 2000))
		}
		ed.onPress(press(ed.toScreen(ed.portPos(start, SideLeft))))
		ed.onRelease(nodeCenter(ed, b))
		if len(ed.Scenario.Edges) != 1 || ed.Scenario.Edges[0].FromSide != SideLeft {
			t.Fatalf("an arrow may leave through the left port, edges=%d", len(ed.Scenario.Edges))
		}
		if !ed.canStartOut(b) {
			t.Error("the target still has no outgoing arrow")
		}
		if ed.canStartOut(start) {
			t.Error("start is capped after its single outgoing arrow")
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
