package flow

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
)

func newBlankEditor() *Editor {
	return &Editor{
		Scenario:   &Scenario{},
		Runner:     NewRunner(),
		zoom:       1,
		nodeW:      176,
		nodeH:      56,
		portHit:    12,
		selected:   make(map[string]bool),
		canvasSize: image.Pt(800, 600),
	}
}

func press(pt f32.Point) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Position: pt, Buttons: pointer.ButtonPrimary}
}

func shiftPress(pt f32.Point) pointer.Event {
	e := press(pt)
	e.Modifiers = key.ModShift
	return e
}

func TestOnPressPansWithSecondaryButton(t *testing.T) {
	for _, btn := range []pointer.Buttons{pointer.ButtonSecondary, pointer.ButtonTertiary} {
		ed := newBlankEditor()
		ed.pan = f32.Pt(5, 6)
		ed.onPress(pointer.Event{Kind: pointer.Press, Position: f32.Pt(300, 200), Buttons: btn})
		if !ed.panning {
			t.Fatalf("button %v must start panning", btn)
		}
		if ed.panStart != f32.Pt(300, 200) || ed.panOrigin != f32.Pt(5, 6) {
			t.Errorf("pan anchors = %v / %v", ed.panStart, ed.panOrigin)
		}

		ed.onDrag(f32.Pt(360, 240))
		if ed.pan != f32.Pt(65, 46) {
			t.Errorf("pan = %v, want (65,46)", ed.pan)
		}
	}
}

func TestOnPressViewBadges(t *testing.T) {
	t.Run("fit badge refits the view", func(t *testing.T) {
		ed := newBlankEditor()
		ed.Scenario.Nodes = append(ed.Scenario.Nodes, &Node{ID: "a", X: 5000, Y: 5000})
		ed.fitBadge = image.Rect(6, 560, 80, 590)
		ed.onPress(press(f32.Pt(10, 570)))
		if ed.pan == (f32.Point{}) {
			t.Error("pressing the fit badge must move the view")
		}
		if ed.marquee || ed.dragNodeID != "" {
			t.Error("badge presses must not start any other interaction")
		}
	})

	t.Run("zoom badge resets the zoom", func(t *testing.T) {
		ed := newBlankEditor()
		ed.zoom = 2.5
		ed.zoomBadge = image.Rect(90, 560, 200, 590)
		ed.onPress(press(f32.Pt(100, 570)))
		if ed.zoom != 1 {
			t.Errorf("zoom = %v, want 1", ed.zoom)
		}
	})
}

func TestOnPressSelectsNodeAndStartsDrag(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)

	ed.onPress(press(f32.Pt(150, 120)))
	if ed.selNodeID != n.ID || !ed.selected[n.ID] {
		t.Fatal("pressing a node body must select it")
	}
	if ed.dragNodeID != n.ID {
		t.Fatal("pressing a node body must arm a drag")
	}
	if ed.dragOff != f32.Pt(50, 20) {
		t.Errorf("drag offset = %v, want (50,20)", ed.dragOff)
	}
	if ed.mode != modeProps {
		t.Error("selecting must switch to the properties panel")
	}
	if ed.pendingSnap == "" {
		t.Error("a pending undo snapshot must be recorded")
	}
}

func TestOnPressEmptySpaceStartsMarquee(t *testing.T) {
	ed := newBlankEditor()
	addNodeTo(ed, KindRequest, 100, 100)
	ed.onPress(press(f32.Pt(600, 500)))
	if !ed.marquee {
		t.Fatal("pressing empty canvas must start a marquee")
	}
	if ed.marqueeStart != f32.Pt(600, 500) || ed.marqueeCur != f32.Pt(600, 500) {
		t.Errorf("marquee anchors = %v / %v", ed.marqueeStart, ed.marqueeCur)
	}
}

func TestOnPressOutPortStartsConnection(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)
	port := ed.toScreen(ed.outPort(n))

	ed.onPress(press(port))
	if ed.connectFromID != n.ID {
		t.Fatalf("connectFromID = %q, want %q", ed.connectFromID, n.ID)
	}
	if ed.reconnectEdge != nil {
		t.Error("a fresh connection must not carry a reconnect edge")
	}
	if ed.dragNodeID != "" {
		t.Error("a port press must not also drag the node")
	}
}

func TestOnPressInPortDetachesLastEdge(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 100)
	e := connect(ed, a, b)
	ed.selEdgeID = e.ID

	ed.onPress(press(ed.toScreen(ed.inPort(b))))
	if ed.reconnectEdge != e {
		t.Fatal("pressing an in port must detach the incoming edge")
	}
	if ed.Scenario.EdgeByID(e.ID) != nil {
		t.Error("the detached edge must leave the scenario while it is dragged")
	}
	if ed.connectFromID != a.ID {
		t.Errorf("connectFromID = %q, want the edge source %q", ed.connectFromID, a.ID)
	}
	if ed.selEdgeID != "" {
		t.Error("the selection must drop the detached edge")
	}
	if len(ed.undoStack) != 1 {
		t.Errorf("detaching must push one undo snapshot, got %d", len(ed.undoStack))
	}
}

func TestOnPressConditionSlots(t *testing.T) {
	t.Run("the free slot starts a new connection", func(t *testing.T) {
		ed := newBlankEditor()
		cond := addNodeTo(ed, KindCondition, 100, 100)
		ed.onPress(press(ed.toScreen(ed.outPortAt(cond, 0))))
		if ed.connectFromID != cond.ID || ed.reconnectEdge != nil {
			t.Errorf("expected a fresh connection, got from=%q reconnect=%v", ed.connectFromID, ed.reconnectEdge)
		}
	})

	t.Run("an occupied slot detaches its edge", func(t *testing.T) {
		ed := newBlankEditor()
		cond := addNodeTo(ed, KindCondition, 100, 100)
		target := addNodeTo(ed, KindDelay, 600, 100)
		e := connect(ed, cond, target)

		ed.onPress(press(ed.toScreen(ed.outPortAt(cond, 0))))
		if ed.reconnectEdge != e {
			t.Fatalf("occupied slot must detach its edge, got %v", ed.reconnectEdge)
		}
		if ed.Scenario.EdgeByID(e.ID) != nil {
			t.Error("the detached edge must leave the scenario")
		}
		if ed.connectFromID != cond.ID {
			t.Errorf("connectFromID = %q", ed.connectFromID)
		}
	})
}

func TestOnPressEnvChipOpensMenu(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)
	c0, c1 := ed.envChipRect(n)
	mid := f32.Pt((c0.X+c1.X)/2, (c0.Y+c1.Y)/2)

	ed.onPress(press(mid))
	if ed.envMenuNodeID != n.ID {
		t.Errorf("envMenuNodeID = %q, want %q", ed.envMenuNodeID, n.ID)
	}
	if ed.selNodeID != "" {
		t.Error("opening the env chip must not select the node")
	}
}

func TestOnPressSelectsEdge(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 600, 400)
	e := connect(ed, a, b)
	p0 := ed.toScreen(ed.edgeOutPos(e, a))
	p1 := ed.toScreen(ed.inPort(b))
	c0, c1 := ed.edgeControls(p0, p1)
	onCurve := bezierAt(p0, c0, c1, p1, 0.5)

	ed.selected = map[string]bool{a.ID: true}
	ed.selNodeID = a.ID
	ed.onPress(press(onCurve))

	if ed.selEdgeID != e.ID {
		t.Fatalf("selEdgeID = %q, want %q", ed.selEdgeID, e.ID)
	}
	if ed.selNodeID != "" || len(ed.selected) != 0 {
		t.Error("selecting an edge must clear the node selection")
	}
	if ed.mode != modeProps {
		t.Error("selecting an edge must switch to the properties panel")
	}
}

func TestOnPressLoopInteractions(t *testing.T) {
	newLoopEditor := func() (*Editor, *Node) {
		ed := newBlankEditor()
		loop := addNodeTo(ed, KindLoop, 100, 100)
		loop.W, loop.H = 400, 300
		return ed, loop
	}

	t.Run("the bottom-right corner starts a resize", func(t *testing.T) {
		ed, loop := newLoopEditor()
		ed.onPress(press(f32.Pt(500, 400)))
		if ed.resizeNodeID != loop.ID {
			t.Fatalf("resizeNodeID = %q, want %q", ed.resizeNodeID, loop.ID)
		}
		if !ed.selected[loop.ID] {
			t.Error("resizing must also select the loop")
		}
		if ed.pendingSnap == "" {
			t.Error("a pending undo snapshot must be recorded")
		}
	})

	t.Run("the header selects and drags the loop", func(t *testing.T) {
		ed, loop := newLoopEditor()
		ed.onPress(press(f32.Pt(200, 120)))
		if ed.dragNodeID != loop.ID {
			t.Fatalf("dragNodeID = %q, want %q", ed.dragNodeID, loop.ID)
		}
	})

	t.Run("the body is transparent to presses", func(t *testing.T) {
		ed, loop := newLoopEditor()
		ed.onPress(press(f32.Pt(300, 300)))
		if ed.selected[loop.ID] {
			t.Error("pressing the loop body must not select the loop")
		}
		if !ed.marquee {
			t.Error("pressing the loop body must fall through to a marquee")
		}
	})

	t.Run("the out port starts a connection", func(t *testing.T) {
		ed, loop := newLoopEditor()
		ed.onPress(press(ed.toScreen(ed.outPort(loop))))
		if ed.connectFromID != loop.ID {
			t.Errorf("connectFromID = %q, want %q", ed.connectFromID, loop.ID)
		}
	})

}

func TestTrySelectNodeShiftToggles(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindRequest, 400, 100)

	ed.trySelectNode(a, shiftPress(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	ed.trySelectNode(b, shiftPress(f32.Pt(450, 120)), f32.Pt(450, 120), 1)
	if !ed.selected[a.ID] || !ed.selected[b.ID] {
		t.Fatalf("shift must extend the selection, got %v", ed.selected)
	}
	if ed.dragNodeID != "" {
		t.Error("a shift press must not arm a drag")
	}

	ed.trySelectNode(b, shiftPress(f32.Pt(450, 120)), f32.Pt(450, 120), 1)
	if ed.selected[b.ID] {
		t.Error("shift on a selected node must deselect it")
	}
	if ed.selNodeID != "" {
		t.Error("deselecting the focused node must clear selNodeID")
	}
}

func TestTrySelectNodeKeepsMultiSelection(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindRequest, 400, 100)
	ed.selected = map[string]bool{a.ID: true, b.ID: true}

	ed.trySelectNode(b, press(f32.Pt(450, 120)), f32.Pt(450, 120), 1)
	if len(ed.selected) != 2 {
		t.Errorf("pressing an already selected node must keep the group, got %v", ed.selected)
	}
	if ed.selNodeID != b.ID {
		t.Errorf("selNodeID = %q, want %q", ed.selNodeID, b.ID)
	}
}

func TestTrySelectNodeDoubleClickFocusesName(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)

	ed.trySelectNode(n, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	if ed.focusNameID != "" {
		t.Error("a single click must not focus the name editor")
	}
	ed.trySelectNode(n, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	if ed.focusNameID != n.ID {
		t.Error("a quick second click must focus the name editor")
	}

	ed.focusNameID = ""
	ed.lastClickAt = time.Now().Add(-time.Second)
	ed.trySelectNode(n, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	if ed.focusNameID != "" {
		t.Error("a slow second click must not count as a double click")
	}
}

func TestTrySelectNodeStartNeverRenames(t *testing.T) {
	ed := newBlankEditor()
	start := addNodeTo(ed, KindStart, 100, 100)
	ed.trySelectNode(start, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	ed.trySelectNode(start, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	if ed.focusNameID != "" {
		t.Error("the start node must not be renameable by double click")
	}
}

func TestTrySelectNodeRaisesZOrder(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindRequest, 400, 100)
	c := addNodeTo(ed, KindRequest, 700, 100)

	ed.trySelectNode(a, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	order := ed.Scenario.Nodes
	if len(order) != 3 {
		t.Fatalf("node count changed: %d", len(order))
	}
	if order[0] != b || order[1] != c || order[2] != a {
		t.Errorf("selected node must move to the end of the draw order, got %v", []string{
			order[0].ID, order[1].ID, order[2].ID,
		})
	}
}

func TestTrySelectNodeLoopCollectsMembers(t *testing.T) {
	ed := newBlankEditor()
	loop := addNodeTo(ed, KindLoop, 100, 100)
	loop.W, loop.H = 400, 300
	inside := addNodeTo(ed, KindDelay, 150, 200)
	addNodeTo(ed, KindDelay, 900, 900)
	alsoSelected := addNodeTo(ed, KindDelay, 200, 250)
	ed.selected[loop.ID] = true
	ed.selected[alsoSelected.ID] = true

	ed.trySelectNode(loop, press(f32.Pt(200, 120)), f32.Pt(200, 120), 0)
	if len(ed.dragMembers) != 1 || ed.dragMembers[0] != inside.ID {
		t.Errorf("dragMembers = %v, want just the contained unselected node %q", ed.dragMembers, inside.ID)
	}
	if ed.Scenario.Nodes[len(ed.Scenario.Nodes)-1] == loop {
		t.Error("loops must not be raised in the draw order")
	}
}

func TestOnDragMovesNodeAndFollowers(t *testing.T) {
	ed := newBlankEditor()
	loop := addNodeTo(ed, KindLoop, 100, 100)
	loop.W, loop.H = 400, 300
	member := addNodeTo(ed, KindDelay, 150, 200)
	friend := addNodeTo(ed, KindDelay, 900, 900)

	ed.selected[loop.ID] = true
	ed.selected[friend.ID] = true
	ed.trySelectNode(loop, press(f32.Pt(200, 120)), f32.Pt(200, 120), 0)

	ed.onDrag(f32.Pt(230, 160))
	if loop.X != 130 || loop.Y != 140 {
		t.Fatalf("loop moved to (%v,%v), want (130,140)", loop.X, loop.Y)
	}
	if member.X != 180 || member.Y != 240 {
		t.Errorf("contained node moved to (%v,%v), want (180,240)", member.X, member.Y)
	}
	if friend.X != 930 || friend.Y != 940 {
		t.Errorf("co-selected node moved to (%v,%v), want (930,940)", friend.X, friend.Y)
	}
	if !ed.dragMoved {
		t.Error("dragMoved must latch")
	}
	if len(ed.undoStack) != 1 {
		t.Errorf("the first movement must commit the pending snapshot, got %d entries", len(ed.undoStack))
	}

	ed.onDrag(f32.Pt(260, 200))
	if len(ed.undoStack) != 1 {
		t.Errorf("later movements must not push more snapshots, got %d", len(ed.undoStack))
	}
}

func TestOnDragWithoutMovementKeepsHistoryClean(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)
	ed.trySelectNode(n, press(f32.Pt(150, 120)), f32.Pt(150, 120), 0)
	ed.onDrag(f32.Pt(150, 120))
	if ed.dragMoved {
		t.Error("a drag that does not move must not latch")
	}
	if len(ed.undoStack) != 0 {
		t.Errorf("no snapshot must be committed, got %d", len(ed.undoStack))
	}
}

func TestOnDragResizesLoopWithMinimums(t *testing.T) {
	ed := newBlankEditor()
	loop := addNodeTo(ed, KindLoop, 100, 100)
	loop.W, loop.H = 400, 300
	ed.onPress(press(f32.Pt(500, 400)))

	ed.onDrag(f32.Pt(700, 600))
	if loop.W != 600 || loop.H != 500 {
		t.Errorf("loop size = (%v,%v), want (600,500)", loop.W, loop.H)
	}
	if !ed.resizeMoved {
		t.Error("resizeMoved must latch")
	}
	if len(ed.undoStack) != 1 {
		t.Errorf("the first resize must commit the pending snapshot, got %d", len(ed.undoStack))
	}

	ed.onDrag(f32.Pt(110, 110))
	if loop.W != ed.nodeW*1.2 || loop.H != ed.nodeH*2 {
		t.Errorf("loop clamped to (%v,%v), want (%v,%v)", loop.W, loop.H, ed.nodeW*1.2, ed.nodeH*2)
	}
}

func TestOnDragResizeOfMissingNodeIsSafe(t *testing.T) {
	ed := newBlankEditor()
	ed.resizeNodeID = "ghost"
	ed.onDrag(f32.Pt(100, 100))

	ed.resizeNodeID = ""
	ed.dragNodeID = "ghost"
	ed.onDrag(f32.Pt(100, 100))
}

func TestOnDragUpdatesConnectionAndMarquee(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)

	ed.connectFromID = n.ID
	ed.onDrag(f32.Pt(400, 300))
	if ed.connectPos != ed.toWorld(f32.Pt(400, 300)) {
		t.Errorf("connectPos = %v", ed.connectPos)
	}

	ed.connectFromID = ""
	ed.marquee = true
	ed.onDrag(f32.Pt(500, 400))
	if ed.marqueeCur != f32.Pt(500, 400) {
		t.Errorf("marqueeCur = %v", ed.marqueeCur)
	}
}

func TestOnReleaseCreatesEdge(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 100)

	ed.onPress(press(ed.toScreen(ed.outPort(a))))
	ed.onRelease(f32.Pt(550, 120))

	if len(ed.Scenario.Edges) != 1 {
		t.Fatalf("expected 1 new edge, got %d", len(ed.Scenario.Edges))
	}
	e := ed.Scenario.Edges[0]
	if e.From != a.ID || e.To != b.ID {
		t.Errorf("edge = %s -> %s, want %s -> %s", e.From, e.To, a.ID, b.ID)
	}
	if ed.selEdgeID != e.ID || ed.mode != modeProps {
		t.Error("the new edge must be selected and shown in the properties panel")
	}
	if len(ed.undoStack) != 1 {
		t.Errorf("creating an edge must push one undo snapshot, got %d", len(ed.undoStack))
	}
	if ed.connectFromID != "" || ed.reconnectEdge != nil {
		t.Error("connect state must be cleared on release")
	}
}

func TestOnReleaseRejectsInvalidTargets(t *testing.T) {
	tests := []struct {
		name   string
		target func(ed *Editor) *Node
	}{
		{"start node", func(ed *Editor) *Node { return addNodeTo(ed, KindStart, 500, 100) }},
		{"note node", func(ed *Editor) *Node { return addNodeTo(ed, KindNote, 500, 100) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newBlankEditor()
			a := addNodeTo(ed, KindRequest, 100, 100)
			tt.target(ed)
			ed.connectFromID = a.ID
			ed.onRelease(f32.Pt(550, 120))
			if len(ed.Scenario.Edges) != 0 {
				t.Errorf("no edge must be created, got %d", len(ed.Scenario.Edges))
			}
		})
	}

	t.Run("self", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		ed.connectFromID = a.ID
		ed.onRelease(f32.Pt(150, 120))
		if len(ed.Scenario.Edges) != 0 {
			t.Errorf("self edges must be rejected, got %d", len(ed.Scenario.Edges))
		}
	})

	t.Run("duplicate", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 500, 100)
		connect(ed, a, b)
		ed.connectFromID = a.ID
		ed.onRelease(f32.Pt(550, 120))
		if len(ed.Scenario.Edges) != 1 {
			t.Errorf("duplicate edges must be rejected, got %d", len(ed.Scenario.Edges))
		}
	})

	t.Run("empty canvas", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		ed.connectFromID = a.ID
		ed.onRelease(f32.Pt(700, 500))
		if len(ed.Scenario.Edges) != 0 {
			t.Errorf("no edge must be created, got %d", len(ed.Scenario.Edges))
		}
	})
}

func TestOnReleaseRetargetsReconnectedEdge(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 100)
	c := addNodeTo(ed, KindDelay, 500, 400)
	e := connect(ed, a, b)

	ed.onPress(press(ed.toScreen(ed.inPort(b))))
	if ed.reconnectEdge != e {
		t.Fatal("setup: the edge must be detached")
	}
	ed.onRelease(f32.Pt(550, 420))

	if len(ed.Scenario.Edges) != 1 {
		t.Fatalf("expected the edge back exactly once, got %d", len(ed.Scenario.Edges))
	}
	got := ed.Scenario.Edges[0]
	if got != e {
		t.Error("the same edge object must be reused")
	}
	if got.To != c.ID {
		t.Errorf("edge target = %q, want %q", got.To, c.ID)
	}
}

func TestOnReleaseDropsDetachedEdgeOnEmptyCanvas(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 500, 100)
	connect(ed, a, b)

	ed.onPress(press(ed.toScreen(ed.inPort(b))))
	ed.onRelease(f32.Pt(700, 550))

	if len(ed.Scenario.Edges) != 0 {
		t.Errorf("dropping a detached edge on empty canvas must delete it, got %d", len(ed.Scenario.Edges))
	}
	if ed.reconnectEdge != nil {
		t.Error("reconnect state must be cleared")
	}
}

func TestOnReleaseIntoLoopHeaderOnly(t *testing.T) {
	ed := newBlankEditor()
	a := addNodeTo(ed, KindRequest, 100, 100)
	loop := addNodeTo(ed, KindLoop, 500, 100)
	loop.W, loop.H = 400, 300

	ed.connectFromID = a.ID
	ed.onRelease(f32.Pt(600, 300))
	if len(ed.Scenario.Edges) != 0 {
		t.Fatalf("dropping into the loop body must not connect, got %d edges", len(ed.Scenario.Edges))
	}

	ed.connectFromID = a.ID
	ed.onRelease(f32.Pt(600, 120))
	if len(ed.Scenario.Edges) != 1 {
		t.Fatalf("dropping on the loop header must connect, got %d edges", len(ed.Scenario.Edges))
	}
	if ed.Scenario.Edges[0].To != loop.ID {
		t.Errorf("edge target = %q, want the loop", ed.Scenario.Edges[0].To)
	}
}

func TestOnReleaseAppliesMarqueeAndClearsState(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindDelay, 100, 100)

	ed.onPress(press(f32.Pt(50, 50)))
	if !ed.marquee {
		t.Fatal("setup: a marquee must have started")
	}
	ed.onDrag(f32.Pt(400, 400))
	ed.onRelease(f32.Pt(400, 400))

	if !ed.selected[n.ID] {
		t.Error("the marquee must have selected the node")
	}
	if ed.marquee || ed.panning || ed.dragNodeID != "" || ed.resizeNodeID != "" {
		t.Error("all interaction state must be cleared on release")
	}
	if ed.pendingSnap != "" {
		t.Error("the pending snapshot must be discarded on release")
	}
	if len(ed.dragMembers) != 0 {
		t.Error("drag members must be cleared")
	}
}
