package flow

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/f32"
)

func newTestEditor() *Editor {
	ed := &Editor{
		Scenario:   NewScenario(),
		Runner:     NewRunner(),
		zoom:       1,
		nodeW:      176,
		nodeH:      56,
		portHit:    12,
		selected:   make(map[string]bool),
		canvasSize: image.Pt(800, 600),
	}
	return ed
}

func addNodeTo(ed *Editor, kind NodeKind, x, y float32) *Node {
	n := NewNode(kind, x, y)
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	return n
}

func connect(ed *Editor, from, to *Node) *Edge {
	e := NewEdge(from.ID, to.ID)
	ed.Scenario.Edges = append(ed.Scenario.Edges, e)
	return e
}

func TestDefSizes(t *testing.T) {
	tests := []struct {
		name  string
		w, h  float32
		wantW float32
		wantH float32
	}{
		{"measured sizes", 200, 80, 200, 80},
		{"zero falls back", 0, 0, 176, 56},
		{"negative falls back", -5, -5, 176, 56},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := &Editor{nodeW: tt.w, nodeH: tt.h}
			w, h := ed.defSizes()
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("defSizes = (%v,%v), want (%v,%v)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestValidateScenario(t *testing.T) {
	t.Run("fully connected scenario has no warnings", func(t *testing.T) {
		ed := newTestEditor()
		req := addNodeTo(ed, KindRequest, 300, 0)
		req.URLEd.SetText("http://x")
		connect(ed, ed.Scenario.Nodes[0], req)
		if warns := ed.validateScenario(); len(warns) != 0 {
			t.Errorf("expected no warnings, got %v", warns)
		}
	})

	t.Run("reports empty URLs", func(t *testing.T) {
		ed := newTestEditor()
		req := addNodeTo(ed, KindRequest, 300, 0)
		req.NameEd.SetText("Nameless")
		req.URLEd.SetText("   ")
		connect(ed, ed.Scenario.Nodes[0], req)
		warns := ed.validateScenario()
		if len(warns) != 1 || warns[0] != "empty URL: Nameless" {
			t.Errorf("warns = %v", warns)
		}
	})

	t.Run("counts unreachable nodes", func(t *testing.T) {
		ed := newTestEditor()
		a := addNodeTo(ed, KindDelay, 300, 0)
		addNodeTo(ed, KindDelay, 600, 0)
		connect(ed, ed.Scenario.Nodes[0], a)
		warns := ed.validateScenario()
		if len(warns) != 1 || warns[0] != "1 unreachable" {
			t.Errorf("warns = %v, want [1 unreachable]", warns)
		}
	})

	t.Run("notes are never unreachable", func(t *testing.T) {
		ed := newTestEditor()
		addNodeTo(ed, KindNote, 900, 900)
		if warns := ed.validateScenario(); len(warns) != 0 {
			t.Errorf("notes must not warn, got %v", warns)
		}
	})

	t.Run("loop members count as reachable", func(t *testing.T) {
		ed := newTestEditor()
		loop := addNodeTo(ed, KindLoop, 300, 0)
		loop.W, loop.H = 400, 400
		inner := addNodeTo(ed, KindDelay, 350, 200)
		_ = inner
		connect(ed, ed.Scenario.Nodes[0], loop)
		if warns := ed.validateScenario(); len(warns) != 0 {
			t.Errorf("nodes inside a reachable loop must be reachable, got %v", warns)
		}
	})

	t.Run("scenario without a start node reports everything unreachable", func(t *testing.T) {
		ed := newTestEditor()
		ed.Scenario = &Scenario{Nodes: []*Node{NewNode(KindDelay, 0, 0), NewNode(KindDelay, 100, 0)}}
		warns := ed.validateScenario()
		if len(warns) != 1 || warns[0] != "2 unreachable" {
			t.Errorf("warns = %v, want [2 unreachable]", warns)
		}
	})
}

func TestPushSnapshotAndUndoRedo(t *testing.T) {
	t.Run("ignores empty and duplicate snapshots", func(t *testing.T) {
		ed := newTestEditor()
		ed.pushSnapshot("")
		if len(ed.undoStack) != 0 {
			t.Error("empty snapshot must be ignored")
		}
		ed.pushSnapshot("a")
		ed.pushSnapshot("a")
		if len(ed.undoStack) != 1 {
			t.Errorf("duplicate snapshot must be ignored, stack = %v", ed.undoStack)
		}
	})

	t.Run("caps the history", func(t *testing.T) {
		ed := newTestEditor()
		for i := 0; i < historyLimit+10; i++ {
			ed.pushSnapshot(itoa(i))
		}
		if len(ed.undoStack) != historyLimit {
			t.Errorf("undo stack = %d entries, want %d", len(ed.undoStack), historyLimit)
		}
		if ed.undoStack[len(ed.undoStack)-1] != itoa(historyLimit+9) {
			t.Error("the newest snapshot must survive the cap")
		}
	})

	t.Run("a new snapshot clears the redo stack", func(t *testing.T) {
		ed := newTestEditor()
		ed.redoStack = []string{"x"}
		ed.pushSnapshot("a")
		if len(ed.redoStack) != 0 {
			t.Error("pushing a snapshot must clear redo")
		}
	})

	t.Run("undo restores the previous scenario and redo reapplies it", func(t *testing.T) {
		ed := newTestEditor()
		ed.pushHistory()
		added := addNodeTo(ed, KindDelay, 300, 0)
		if len(ed.Scenario.Nodes) != 2 {
			t.Fatal("setup failed")
		}

		ed.Undo()
		if len(ed.Scenario.Nodes) != 1 {
			t.Fatalf("undo must drop the added node, got %d nodes", len(ed.Scenario.Nodes))
		}
		if len(ed.redoStack) != 1 {
			t.Fatalf("undo must fill the redo stack, got %d", len(ed.redoStack))
		}

		ed.Redo()
		if len(ed.Scenario.Nodes) != 2 || ed.Scenario.NodeByID(added.ID) == nil {
			t.Errorf("redo must bring the node back, got %d nodes", len(ed.Scenario.Nodes))
		}
	})

	t.Run("undo with an empty stack is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		before := ed.encode()
		ed.Undo()
		if ed.encode() != before {
			t.Error("undo without history must not change the scenario")
		}
	})

	t.Run("redo with an empty stack is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		before := ed.encode()
		ed.Redo()
		if ed.encode() != before {
			t.Error("redo without history must not change the scenario")
		}
	})

	t.Run("undo skips a snapshot identical to the current state", func(t *testing.T) {
		ed := newTestEditor()
		ed.pushHistory()
		ed.Undo()
		if len(ed.redoStack) != 0 {
			t.Errorf("an identical snapshot must be discarded without a redo entry, got %d", len(ed.redoStack))
		}
	})

	t.Run("commitPending pushes once", func(t *testing.T) {
		ed := newTestEditor()
		ed.pendingSnap = "snap"
		ed.commitPending()
		ed.commitPending()
		if len(ed.undoStack) != 1 || ed.undoStack[0] != "snap" {
			t.Errorf("undo stack = %v", ed.undoStack)
		}
		if ed.pendingSnap != "" {
			t.Error("pendingSnap must be consumed")
		}
	})
}

func TestRestoreInvalidDataKeepsScenario(t *testing.T) {
	ed := newTestEditor()
	before := ed.Scenario
	ed.restore("{{{not json")
	if ed.Scenario != before {
		t.Error("a failed decode must leave the scenario untouched")
	}
}

func TestPruneSelection(t *testing.T) {
	ed := newTestEditor()
	n := addNodeTo(ed, KindDelay, 0, 0)
	e := connect(ed, ed.Scenario.Nodes[0], n)
	ed.selected = map[string]bool{n.ID: true, "ghost": true}
	ed.selNodeID = "ghost"
	ed.selEdgeID = e.ID

	ed.pruneSelection()
	if ed.selNodeID != "" {
		t.Error("a selection pointing at a removed node must be cleared")
	}
	if ed.selEdgeID != e.ID {
		t.Error("a live edge selection must survive")
	}
	if ed.selected["ghost"] || !ed.selected[n.ID] {
		t.Errorf("selection set = %v", ed.selected)
	}

	ed.Scenario.RemoveEdge(e.ID)
	ed.pruneSelection()
	if ed.selEdgeID != "" {
		t.Error("a selection pointing at a removed edge must be cleared")
	}
}

func TestCopyPaste(t *testing.T) {
	t.Run("copies selected nodes and their internal edges", func(t *testing.T) {
		ed := newTestEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 300, 100)
		connect(ed, ed.Scenario.Nodes[0], a)
		connect(ed, a, b)
		ed.selected = map[string]bool{a.ID: true, b.ID: true}

		ed.copySelection()
		if ed.clipboard == "" {
			t.Fatal("clipboard must be filled")
		}

		ed.paste()
		if len(ed.Scenario.Nodes) != 5 {
			t.Fatalf("expected 5 nodes after paste, got %d", len(ed.Scenario.Nodes))
		}
		if len(ed.Scenario.Edges) != 3 {
			t.Fatalf("expected 3 edges after paste, got %d", len(ed.Scenario.Edges))
		}
		if len(ed.selected) != 2 {
			t.Errorf("pasted nodes must be selected, got %d", len(ed.selected))
		}
		pasted := ed.Scenario.Nodes[3]
		if pasted.ID == a.ID {
			t.Error("pasted node must get a fresh id")
		}
		if pasted.X != a.X+28 || pasted.Y != a.Y+28 {
			t.Errorf("pasted node offset = (%v,%v), want (%v,%v)", pasted.X, pasted.Y, a.X+28, a.Y+28)
		}
		if ed.mode != modeProps {
			t.Error("paste must switch to the properties panel")
		}
	})

	t.Run("never copies the start node", func(t *testing.T) {
		ed := newTestEditor()
		ed.selected = map[string]bool{ed.Scenario.Nodes[0].ID: true}
		ed.copySelection()
		if ed.clipboard != "" {
			t.Errorf("start-only selection must not fill the clipboard, got %q", ed.clipboard)
		}
	})

	t.Run("copying nothing keeps the clipboard", func(t *testing.T) {
		ed := newTestEditor()
		ed.clipboard = "keep"
		ed.copySelection()
		if ed.clipboard != "keep" {
			t.Error("an empty selection must not touch the clipboard")
		}
	})

	t.Run("paste ignores empty and malformed clipboards", func(t *testing.T) {
		ed := newTestEditor()
		ed.paste()
		ed.clipboard = "not json"
		ed.paste()
		ed.clipboard = `{"id":"x","name":"y"}`
		ed.paste()
		if len(ed.Scenario.Nodes) != 1 {
			t.Errorf("nothing must be pasted, got %d nodes", len(ed.Scenario.Nodes))
		}
	})

	t.Run("paste drops edges whose endpoints were not copied", func(t *testing.T) {
		ed := newTestEditor()
		ed.clipboard = `{"id":"c","nodes":[{"id":"a","kind":4}],"edges":[{"id":"e","from":"a","to":"outside"}]}`
		ed.paste()
		if len(ed.Scenario.Edges) != 0 {
			t.Errorf("dangling edges must not be pasted, got %d", len(ed.Scenario.Edges))
		}
	})
}

func TestDeleteSelection(t *testing.T) {
	t.Run("deletes selected nodes and the selected edge", func(t *testing.T) {
		ed := newTestEditor()
		a := addNodeTo(ed, KindDelay, 100, 0)
		b := addNodeTo(ed, KindDelay, 200, 0)
		e := connect(ed, a, b)
		ed.selected = map[string]bool{a.ID: true}
		ed.selEdgeID = e.ID

		ed.deleteSelection()
		if ed.Scenario.NodeByID(a.ID) != nil {
			t.Error("selected node must be deleted")
		}
		if len(ed.Scenario.Edges) != 0 {
			t.Errorf("edges must be gone, got %d", len(ed.Scenario.Edges))
		}
		if len(ed.selected) != 0 || ed.selEdgeID != "" {
			t.Error("selection must be cleared")
		}
		if len(ed.undoStack) != 1 {
			t.Errorf("delete must push one undo snapshot, got %d", len(ed.undoStack))
		}
	})

	t.Run("empty selection is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		ed.deleteSelection()
		if len(ed.undoStack) != 0 {
			t.Error("deleting nothing must not push history")
		}
	})

	t.Run("the start node survives deletion", func(t *testing.T) {
		ed := newTestEditor()
		start := ed.Scenario.Nodes[0]
		ed.selected = map[string]bool{start.ID: true}
		ed.deleteSelection()
		if ed.Scenario.NodeByID(start.ID) == nil {
			t.Error("the start node must not be deletable")
		}
	})
}

func TestSelectionHelpers(t *testing.T) {
	ed := newTestEditor()
	n := addNodeTo(ed, KindDelay, 0, 0)
	e := connect(ed, ed.Scenario.Nodes[0], n)

	ed.selEdgeID = e.ID
	ed.envDropOpen = true
	ed.selectOnly(n.ID)
	if !ed.selected[n.ID] || ed.selNodeID != n.ID {
		t.Error("selectOnly must select the node")
	}
	if ed.selEdgeID != "" || ed.envDropOpen {
		t.Error("selectOnly must clear the edge selection and close the env dropdown")
	}
	if ed.selectedNode() != n {
		t.Error("selectedNode must return the selected node")
	}
	if ed.selectedEdge() != nil {
		t.Error("selectedEdge must be nil when no edge is selected")
	}

	ed.selEdgeID = e.ID
	if ed.selectedEdge() != e {
		t.Error("selectedEdge must return the selected edge")
	}

	ed.clearSelection()
	if len(ed.selected) != 0 || ed.selNodeID != "" || ed.selEdgeID != "" {
		t.Error("clearSelection must reset everything")
	}
	if ed.selectedNode() != nil {
		t.Error("selectedNode must be nil after clearing")
	}
}

func TestCancelInteraction(t *testing.T) {
	t.Run("restores an edge being reconnected", func(t *testing.T) {
		ed := newTestEditor()
		n := addNodeTo(ed, KindDelay, 0, 0)
		e := NewEdge(ed.Scenario.Nodes[0].ID, n.ID)
		ed.reconnectEdge = e
		ed.connectFromID = ed.Scenario.Nodes[0].ID

		ed.cancelInteraction()
		if len(ed.Scenario.Edges) != 1 || ed.Scenario.Edges[0] != e {
			t.Error("the detached edge must be put back")
		}
		if ed.reconnectEdge != nil || ed.connectFromID != "" {
			t.Error("connect state must be cleared")
		}
	})

	t.Run("an open env menu is closed before the selection", func(t *testing.T) {
		ed := newTestEditor()
		n := addNodeTo(ed, KindRequest, 0, 0)
		ed.selectOnly(n.ID)
		ed.envMenuNodeID = n.ID

		ed.cancelInteraction()
		if ed.envMenuNodeID != "" {
			t.Error("the env menu must be closed")
		}
		if !ed.selected[n.ID] {
			t.Error("the selection must survive the first Escape")
		}

		ed.cancelInteraction()
		if len(ed.selected) != 0 {
			t.Error("the second Escape must clear the selection")
		}
	})
}

func TestEnvDropState(t *testing.T) {
	ed := newTestEditor()
	if ed.EnvDropOpen() {
		t.Error("closed by default")
	}
	ed.envDropOpen = true
	if !ed.EnvDropOpen() {
		t.Error("envDropOpen must report open")
	}
	ed.envDropOpen = false
	ed.envMenuNodeID = "n"
	if !ed.EnvDropOpen() {
		t.Error("an open node env menu must report open")
	}
	ed.CloseEnvDrop()
	if ed.EnvDropOpen() {
		t.Error("CloseEnvDrop must close both")
	}
}

func TestScreenWorldRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		pan  f32.Point
		zoom float32
	}{
		{"identity", f32.Pt(0, 0), 1},
		{"panned", f32.Pt(30, -12), 1},
		{"zoomed", f32.Pt(0, 0), 2},
		{"panned and zoomed", f32.Pt(-40, 90), 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newTestEditor()
			ed.pan, ed.zoom = tt.pan, tt.zoom
			world := f32.Pt(123, 456)
			got := ed.toWorld(ed.toScreen(world))
			if math.Abs(float64(got.X-world.X)) > 0.01 || math.Abs(float64(got.Y-world.Y)) > 0.01 {
				t.Errorf("round trip = %v, want %v", got, world)
			}
		})
	}
}

func TestZoomLevelBounds(t *testing.T) {
	lo, hi := zoomLevelBounds()
	if lo >= 0 || hi <= 0 {
		t.Fatalf("bounds = (%d,%d), want lo<0<hi", lo, hi)
	}
	if z := math.Pow(zoomStep, float64(lo)); z < minZoom {
		t.Errorf("lowest level zoom %v is below minZoom %v", z, minZoom)
	}
	if z := math.Pow(zoomStep, float64(hi)); z > maxZoom {
		t.Errorf("highest level zoom %v is above maxZoom %v", z, maxZoom)
	}
}

func TestZoomByNotches(t *testing.T) {
	t.Run("zero notches is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		ed.zoomByNotches(f32.Pt(400, 300), 0)
		if ed.zoom != 1 {
			t.Errorf("zoom = %v, want 1", ed.zoom)
		}
	})

	t.Run("zooming in and out is symmetric", func(t *testing.T) {
		ed := newTestEditor()
		ed.zoomByNotches(f32.Pt(400, 300), 1)
		if ed.zoom <= 1 {
			t.Fatalf("zoom in must raise the zoom, got %v", ed.zoom)
		}
		ed.zoomByNotches(f32.Pt(400, 300), -1)
		if math.Abs(float64(ed.zoom-1)) > 0.001 {
			t.Errorf("zoom back = %v, want 1", ed.zoom)
		}
		if math.Abs(float64(ed.pan.X)) > 0.01 || math.Abs(float64(ed.pan.Y)) > 0.01 {
			t.Errorf("pan must return to the origin, got %v", ed.pan)
		}
	})

	t.Run("the anchor point stays put", func(t *testing.T) {
		ed := newTestEditor()
		anchor := f32.Pt(300, 200)
		before := ed.toWorld(anchor)
		ed.zoomByNotches(anchor, 3)
		after := ed.toWorld(anchor)
		if math.Abs(float64(before.X-after.X)) > 0.01 || math.Abs(float64(before.Y-after.Y)) > 0.01 {
			t.Errorf("world point under the cursor moved from %v to %v", before, after)
		}
	})

	t.Run("clamped at both ends", func(t *testing.T) {
		ed := newTestEditor()
		ed.zoomByNotches(f32.Pt(0, 0), 500)
		if ed.zoom > maxZoom {
			t.Errorf("zoom = %v exceeds maxZoom %v", ed.zoom, maxZoom)
		}
		high := ed.zoom
		ed.zoomByNotches(f32.Pt(0, 0), 500)
		if ed.zoom != high {
			t.Error("further zoom in at the cap must be a no-op")
		}

		ed.zoomByNotches(f32.Pt(0, 0), -500)
		if ed.zoom < minZoom {
			t.Errorf("zoom = %v is below minZoom %v", ed.zoom, minZoom)
		}
	})
}

func TestFitViewAndResetZoom(t *testing.T) {
	t.Run("fits all nodes on the canvas", func(t *testing.T) {
		ed := newTestEditor()
		ed.Scenario = &Scenario{Nodes: []*Node{
			{ID: "a", X: -500, Y: -500},
			{ID: "b", X: 1500, Y: 1200},
		}}
		ed.fitView()
		for _, n := range ed.Scenario.Nodes {
			s := ed.toScreen(f32.Pt(n.X, n.Y))
			if s.X < 0 || s.Y < 0 || s.X > 800 || s.Y > 600 {
				t.Errorf("node %s projects off canvas at %v", n.ID, s)
			}
		}
	})

	t.Run("never zooms past 1 for a small scenario", func(t *testing.T) {
		ed := newTestEditor()
		ed.Scenario = &Scenario{Nodes: []*Node{{ID: "a", X: 0, Y: 0}}}
		ed.fitView()
		if ed.zoom != 1 {
			t.Errorf("zoom = %v, want 1", ed.zoom)
		}
	})

	t.Run("clamps to minZoom for a huge scenario", func(t *testing.T) {
		ed := newTestEditor()
		ed.Scenario = &Scenario{Nodes: []*Node{{ID: "a", X: 0, Y: 0}, {ID: "b", X: 500000, Y: 500000}}}
		ed.fitView()
		if ed.zoom != minZoom {
			t.Errorf("zoom = %v, want minZoom %v", ed.zoom, minZoom)
		}
	})

	t.Run("no nodes or no canvas is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		ed.Scenario = &Scenario{}
		ed.pan, ed.zoom = f32.Pt(5, 5), 2
		ed.fitView()
		if ed.pan != f32.Pt(5, 5) || ed.zoom != 2 {
			t.Error("fitView with no nodes must not move the view")
		}

		ed2 := newTestEditor()
		ed2.canvasSize = image.Point{}
		ed2.pan, ed2.zoom = f32.Pt(5, 5), 2
		ed2.fitView()
		if ed2.pan != f32.Pt(5, 5) || ed2.zoom != 2 {
			t.Error("fitView with no canvas must not move the view")
		}
	})

	t.Run("resetZoom keeps the canvas centre", func(t *testing.T) {
		ed := newTestEditor()
		ed.zoom = 2.5
		ed.pan = f32.Pt(-100, -80)
		centre := f32.Pt(400, 300)
		before := ed.toWorld(centre)
		ed.resetZoom()
		if ed.zoom != 1 {
			t.Fatalf("zoom = %v, want 1", ed.zoom)
		}
		after := ed.toWorld(centre)
		if math.Abs(float64(before.X-after.X)) > 0.01 || math.Abs(float64(before.Y-after.Y)) > 0.01 {
			t.Errorf("centre moved from %v to %v", before, after)
		}
	})
}

func TestItoa(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want string
	}{
		{"zero", 0, "0"},
		{"single digit", 7, "7"},
		{"multi digit", 12345, "12345"},
		{"negative", -42, "-42"},
		{"largest safe positive", 999999999999, "999999999999"},
		{"largest safe negative", -99999999999, "-99999999999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itoa(tt.in); got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCollectPlaceholders(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"none", "plain text", nil},
		{"single", "{{host}}", []string{"host"}},
		{"multiple", "{{a}}/{{b}}", []string{"a", "b"}},
		{"trimmed", "{{  a  }}", []string{"a"}},
		{"unterminated", "{{a", nil},
		{"empty name skipped", "{{}}", nil},
		{"partial after valid", "{{a}}{{b", []string{"a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := map[string]bool{}
			collectPlaceholders(tt.in, got)
			if len(got) != len(tt.want) {
				t.Fatalf("collected %v, want %v", got, tt.want)
			}
			for _, name := range tt.want {
				if !got[name] {
					t.Errorf("missing placeholder %q in %v", name, got)
				}
			}
		})
	}
}

func TestMissingVars(t *testing.T) {
	newEd := func() *Editor {
		ed := newTestEditor()
		ed.frameEnvs = map[string]map[string]string{
			"":     {"host": "x"},
			"env2": {"other": "y"},
		}
		ed.setVarNames = map[string]bool{"token": true}
		return ed
	}

	t.Run("request node reports only unknown names", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindRequest, 0, 0)
		n.URLEd.SetText("http://{{host}}/{{missingA}}")
		n.HeadersEd.SetText("Auth: {{token}}")
		n.BodyEd.SetText("{{missingB}} {{loop.item}}")
		got := ed.missingVars(n)
		if len(got) != 2 || got[0] != "missingA" || got[1] != "missingB" {
			t.Errorf("missingVars = %v, want sorted [missingA missingB]", got)
		}
	})

	t.Run("uses the node env when set", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindRequest, 0, 0)
		n.EnvID = "env2"
		n.URLEd.SetText("{{other}}/{{host}}")
		got := ed.missingVars(n)
		if len(got) != 1 || got[0] != "host" {
			t.Errorf("missingVars = %v, want [host]", got)
		}
	})

	t.Run("falls back to the active env for an unknown env id", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindRequest, 0, 0)
		n.EnvID = "ghost"
		n.URLEd.SetText("{{host}}")
		if got := ed.missingVars(n); len(got) != 0 {
			t.Errorf("missingVars = %v, want none", got)
		}
	})

	t.Run("setvar node scans only its value", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindSetVar, 0, 0)
		n.VarNameEd.SetText("{{ignored}}")
		n.VarValueEd.SetText("{{nope}}")
		got := ed.missingVars(n)
		if len(got) != 1 || got[0] != "nope" {
			t.Errorf("missingVars = %v, want [nope]", got)
		}
	})

	t.Run("other kinds are never checked", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindDelay, 0, 0)
		n.BodyEd.SetText("{{nope}}")
		if got := ed.missingVars(n); got != nil {
			t.Errorf("missingVars = %v, want nil", got)
		}
	})

	t.Run("no placeholders yields nil", func(t *testing.T) {
		ed := newEd()
		n := NewNode(KindRequest, 0, 0)
		n.URLEd.SetText("http://plain")
		if got := ed.missingVars(n); got != nil {
			t.Errorf("missingVars = %v, want nil", got)
		}
	})
}

func TestOutSlotsAndPorts(t *testing.T) {
	ed := newTestEditor()
	req := addNodeTo(ed, KindRequest, 0, 0)
	cond := addNodeTo(ed, KindCondition, 200, 0)
	a := addNodeTo(ed, KindDelay, 400, 0)
	b := addNodeTo(ed, KindDelay, 400, 200)

	if got := ed.outSlots(req); got != 1 {
		t.Errorf("a non-condition node must have 1 slot, got %d", got)
	}
	if got := ed.outSlots(cond); got != 1 {
		t.Errorf("a condition with no edges must have 1 free slot, got %d", got)
	}

	e1 := connect(ed, cond, a)
	e2 := connect(ed, cond, b)
	if got := ed.outSlots(cond); got != 3 {
		t.Errorf("a condition with 2 edges must have 3 slots, got %d", got)
	}
	if got := len(ed.outEdges(cond)); got != 2 {
		t.Errorf("outEdges = %d, want 2", got)
	}
	if got := len(ed.outEdges(a)); got != 0 {
		t.Errorf("outEdges of a leaf = %d, want 0", got)
	}

	single := ed.outPortAt(req, 0)
	if single.Y != req.Y+ed.nodeH/2 {
		t.Errorf("a single out port must sit at the vertical middle, got %v", single)
	}
	if single.X != req.X+ed.nodeW {
		t.Errorf("a single out port must sit at the right edge, got %v", single)
	}

	p0 := ed.outPortAt(cond, 0)
	p1 := ed.outPortAt(cond, 1)
	if !(p0.Y < p1.Y) {
		t.Errorf("condition slots must be ordered top to bottom, got %v then %v", p0, p1)
	}
	if got := ed.edgeOutPos(e1, cond); got != p0 {
		t.Errorf("edge 1 must leave from slot 0, got %v want %v", got, p0)
	}
	if got := ed.edgeOutPos(e2, cond); got != p1 {
		t.Errorf("edge 2 must leave from slot 1, got %v want %v", got, p1)
	}
	if got := ed.edgeOutPos(e1, req); got != single {
		t.Errorf("a non-condition source always uses slot 0, got %v", got)
	}
	stray := NewEdge(cond.ID, "ghost")
	if got := ed.edgeOutPos(stray, cond); got != ed.outPort(cond) {
		t.Errorf("an unknown edge must fall back to the free port, got %v", got)
	}

	in := ed.inPort(req)
	if in.X != req.X || in.Y != req.Y+ed.nodeH/2 {
		t.Errorf("in port = %v", in)
	}
}

func TestCondSlotHitShrinksWithSlots(t *testing.T) {
	ed := newTestEditor()
	cond := addNodeTo(ed, KindCondition, 0, 0)
	wide := ed.condSlotHit(cond)
	for i := 0; i < 8; i++ {
		n := addNodeTo(ed, KindDelay, float32(200*i), 0)
		connect(ed, cond, n)
	}
	narrow := ed.condSlotHit(cond)
	if narrow >= wide {
		t.Errorf("hit radius must shrink as slots multiply: %v -> %v", wide, narrow)
	}
	if narrow <= 0 {
		t.Errorf("hit radius must stay positive, got %v", narrow)
	}
}

func TestApplyMarquee(t *testing.T) {
	t.Run("selects intersecting nodes", func(t *testing.T) {
		ed := newTestEditor()
		inside := addNodeTo(ed, KindDelay, 100, 100)
		outside := addNodeTo(ed, KindDelay, 700, 500)
		ed.marqueeStart = f32.Pt(50, 50)
		ed.applyMarquee(f32.Pt(400, 400))

		if !ed.selected[inside.ID] {
			t.Error("a node inside the marquee must be selected")
		}
		if ed.selected[outside.ID] {
			t.Error("a node outside the marquee must not be selected")
		}
		if ed.mode != modeProps {
			t.Error("a non-empty marquee must switch to the properties panel")
		}
	})

	t.Run("works when dragged backwards", func(t *testing.T) {
		ed := newTestEditor()
		n := addNodeTo(ed, KindDelay, 100, 100)
		ed.marqueeStart = f32.Pt(400, 400)
		ed.applyMarquee(f32.Pt(50, 50))
		if !ed.selected[n.ID] {
			t.Error("a marquee dragged up-left must still select")
		}
	})

	t.Run("a tiny marquee clears the selection", func(t *testing.T) {
		ed := newTestEditor()
		n := addNodeTo(ed, KindDelay, 0, 0)
		ed.selected = map[string]bool{n.ID: true}
		ed.marqueeStart = f32.Pt(100, 100)
		ed.applyMarquee(f32.Pt(102, 101))
		if len(ed.selected) != 0 {
			t.Errorf("a click-sized marquee must clear the selection, got %v", ed.selected)
		}
	})

	t.Run("a loop is hit by its header only", func(t *testing.T) {
		ed := newTestEditor()
		loop := addNodeTo(ed, KindLoop, 0, 0)
		loop.W, loop.H = 400, 400
		ed.marqueeStart = f32.Pt(10, 300)
		ed.applyMarquee(f32.Pt(200, 390))
		if ed.selected[loop.ID] {
			t.Error("a marquee over the loop body must not select the loop itself")
		}
	})
}

func TestEdgeAtAndLastEdgeTo(t *testing.T) {
	ed := newTestEditor()
	a := addNodeTo(ed, KindDelay, 0, 0)
	b := addNodeTo(ed, KindDelay, 400, 0)
	e := connect(ed, a, b)

	start := ed.toScreen(ed.edgeOutPos(e, a))
	if got := ed.edgeAt(start); got != e {
		t.Errorf("a point on the edge must hit it, got %v", got)
	}
	if got := ed.edgeAt(f32.Pt(0, 5000)); got != nil {
		t.Errorf("a far away point must hit nothing, got %v", got)
	}

	if got := ed.lastEdgeTo(b.ID); got != e {
		t.Error("lastEdgeTo must find the edge")
	}
	e2 := connect(ed, ed.Scenario.Nodes[0], b)
	if got := ed.lastEdgeTo(b.ID); got != e2 {
		t.Error("lastEdgeTo must return the most recently added edge")
	}
	if got := ed.lastEdgeTo("ghost"); got != nil {
		t.Error("lastEdgeTo for an unknown node must be nil")
	}
}

func TestEdgeAtSkipsDanglingEdges(t *testing.T) {
	ed := newTestEditor()
	a := addNodeTo(ed, KindDelay, 0, 0)
	ed.Scenario.Edges = append(ed.Scenario.Edges, NewEdge(a.ID, "ghost"), NewEdge("ghost", a.ID))
	if got := ed.edgeAt(ed.toScreen(ed.outPort(a))); got != nil {
		t.Errorf("edges with missing endpoints must be skipped, got %v", got)
	}
}

func TestConnectingNode(t *testing.T) {
	ed := newTestEditor()
	n := addNodeTo(ed, KindDelay, 0, 0)
	if ed.connectingNode() != nil {
		t.Error("no connection in progress")
	}
	ed.connectFromID = n.ID
	if ed.connectingNode() != n {
		t.Error("connectingNode must resolve the id")
	}
	ed.connectFromID = "ghost"
	if ed.connectingNode() != nil {
		t.Error("an unknown id must resolve to nil")
	}
}

func TestNodeScreenRectAndEnvChipRect(t *testing.T) {
	ed := newTestEditor()
	ed.zoom = 2
	ed.pan = f32.Pt(10, 20)
	n := addNodeTo(ed, KindRequest, 100, 50)

	sp, w, h := ed.nodeScreenRect(n)
	if sp != ed.toScreen(f32.Pt(100, 50)) {
		t.Errorf("origin = %v", sp)
	}
	if w != ed.nodeW*2 || h != ed.nodeH*2 {
		t.Errorf("size = (%v,%v), want the zoomed node size", w, h)
	}

	c0, c1 := ed.envChipRect(n)
	if c0.Y <= sp.Y+h {
		t.Error("the env chip must sit below the node")
	}
	if c1.X-c0.X != w {
		t.Errorf("chip width = %v, want the node width %v", c1.X-c0.X, w)
	}
	if c1.Y <= c0.Y {
		t.Error("the chip must have a positive height")
	}
}

func TestEnvName(t *testing.T) {
	ed := newTestEditor()
	ed.envOpts = []EnvOption{{ID: "e1", Name: "Staging"}}
	tests := []struct {
		name string
		id   string
		want string
	}{
		{"empty id", "", "active env"},
		{"known id", "e1", "Staging"},
		{"unknown id", "gone", "missing env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ed.envName(tt.id); got != tt.want {
				t.Errorf("envName(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestWindowToCanvas(t *testing.T) {
	ed := newTestEditor()
	ed.canvasOrig = image.Pt(100, 50)
	tests := []struct {
		name string
		in   f32.Point
		ok   bool
		want f32.Point
	}{
		{"inside", f32.Pt(300, 250), true, f32.Pt(200, 200)},
		{"top left corner", f32.Pt(100, 50), true, f32.Pt(0, 0)},
		{"left of canvas", f32.Pt(50, 250), false, f32.Point{}},
		{"above canvas", f32.Pt(300, 10), false, f32.Point{}},
		{"right of canvas", f32.Pt(1000, 250), false, f32.Point{}},
		{"below canvas", f32.Pt(300, 1000), false, f32.Point{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ed.windowToCanvas(tt.in)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("local = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewNodeAtAndAddNode(t *testing.T) {
	t.Run("loop nodes get a container size", func(t *testing.T) {
		ed := newTestEditor()
		n := ed.newNodeAt(KindLoop, 0, 0)
		if n.W != ed.nodeW*2.4 || n.H != ed.nodeH*4 {
			t.Errorf("loop size = (%v,%v)", n.W, n.H)
		}
		plain := ed.newNodeAt(KindDelay, 0, 0)
		if plain.W != 0 || plain.H != 0 {
			t.Errorf("non-loop nodes must keep an auto size, got (%v,%v)", plain.W, plain.H)
		}
	})

	t.Run("addNode centres in the view and selects", func(t *testing.T) {
		ed := newTestEditor()
		ed.addNode(KindDelay)
		if len(ed.Scenario.Nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(ed.Scenario.Nodes))
		}
		n := ed.Scenario.Nodes[1]
		if !ed.selected[n.ID] || ed.selNodeID != n.ID {
			t.Error("the new node must be selected")
		}
		if ed.mode != modeProps {
			t.Error("addNode must switch to the properties panel")
		}
		if len(ed.undoStack) != 1 {
			t.Errorf("addNode must push one undo snapshot, got %d", len(ed.undoStack))
		}
		centre := ed.viewCenterWorld()
		if n.X != centre.X-ed.nodeW/2+24 || n.Y != centre.Y-ed.nodeH/2+24 {
			t.Errorf("node placed at (%v,%v), expected the view centre with a cascade offset", n.X, n.Y)
		}
	})
}

func TestDropKindAtWindow(t *testing.T) {
	t.Run("drops inside the canvas", func(t *testing.T) {
		ed := newTestEditor()
		ed.canvasOrig = image.Pt(100, 50)
		if !ed.dropKindAtWindow(KindDelay, f32.Pt(300, 250)) {
			t.Fatal("drop must succeed")
		}
		if len(ed.Scenario.Nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(ed.Scenario.Nodes))
		}
		n := ed.Scenario.Nodes[1]
		if n.X != 200-ed.nodeW/2 || n.Y != 200-ed.nodeH/2 {
			t.Errorf("node placed at (%v,%v)", n.X, n.Y)
		}
		if !ed.selected[n.ID] {
			t.Error("the dropped node must be selected")
		}
	})

	t.Run("rejects drops outside the canvas", func(t *testing.T) {
		ed := newTestEditor()
		ed.canvasOrig = image.Pt(100, 50)
		if ed.dropKindAtWindow(KindDelay, f32.Pt(10, 10)) {
			t.Error("a drop outside the canvas must be rejected")
		}
		if len(ed.Scenario.Nodes) != 1 {
			t.Error("nothing must be added")
		}
	})
}

func TestNodeFromRequest(t *testing.T) {
	tests := []struct {
		name        string
		nodeName    string
		req         *model.ParsedRequest
		wantName    string
		wantMethod  string
		wantHeaders string
	}{
		{
			"full request",
			"Login",
			&model.ParsedRequest{Method: "POST", URL: "http://x", Body: "{}", Headers: map[string]string{"B": "2", "A": "1"}},
			"Login", "POST", "A: 1\nB: 2",
		},
		{
			"unnamed request",
			"",
			&model.ParsedRequest{Method: "PUT", URL: "http://y"},
			"Request", "PUT", "",
		},
		{
			"missing method keeps the default",
			"X",
			&model.ParsedRequest{URL: "http://z"},
			"X", "GET", "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newTestEditor()
			n := ed.nodeFromRequest(tt.nodeName, tt.req, 10, 20)
			if n.Kind != KindRequest {
				t.Error("must produce a request node")
			}
			if n.NameEd.Text() != tt.wantName {
				t.Errorf("name = %q, want %q", n.NameEd.Text(), tt.wantName)
			}
			if n.Method != tt.wantMethod {
				t.Errorf("method = %q, want %q", n.Method, tt.wantMethod)
			}
			if n.HeadersEd.Text() != tt.wantHeaders {
				t.Errorf("headers = %q, want %q (sorted, one per line)", n.HeadersEd.Text(), tt.wantHeaders)
			}
			if n.URLEd.Text() != tt.req.URL {
				t.Errorf("url = %q, want %q", n.URLEd.Text(), tt.req.URL)
			}
		})
	}
}

func TestDropCollectionNode(t *testing.T) {
	t.Run("nil source is rejected", func(t *testing.T) {
		ed := newTestEditor()
		if ed.DropCollectionNode(nil, f32.Pt(10, 10)) {
			t.Error("nil source must be rejected")
		}
	})

	t.Run("a drop outside the canvas is rejected", func(t *testing.T) {
		ed := newTestEditor()
		ed.canvasOrig = image.Pt(100, 100)
		src := &collections.CollectionNode{Name: "r", Request: &model.ParsedRequest{Method: "GET", URL: "http://x"}}
		if ed.DropCollectionNode(src, f32.Pt(0, 0)) {
			t.Error("a drop outside the canvas must be rejected")
		}
	})

	t.Run("single request becomes one node", func(t *testing.T) {
		ed := newTestEditor()
		parent := &collections.CollectionNode{Name: "folder", IsFolder: true}
		src := &collections.CollectionNode{
			Name:    "Get user",
			Parent:  parent,
			Request: &model.ParsedRequest{Method: "GET", URL: "http://x"},
		}
		if !ed.DropCollectionNode(src, f32.Pt(400, 300)) {
			t.Fatal("drop must succeed")
		}
		if len(ed.Scenario.Nodes) != 2 {
			t.Fatalf("expected 2 nodes, got %d", len(ed.Scenario.Nodes))
		}
		if ed.Scenario.Nodes[1].NameEd.Text() != "Get user" {
			t.Errorf("node name = %q", ed.Scenario.Nodes[1].NameEd.Text())
		}
	})

	t.Run("folder becomes a grid grouped by method", func(t *testing.T) {
		ed := newTestEditor()
		root := &collections.CollectionNode{Name: "root", IsFolder: true}
		sub := &collections.CollectionNode{Name: "sub", IsFolder: true, Parent: root}
		root.Children = []*collections.CollectionNode{
			{Name: "p1", Parent: root, Request: &model.ParsedRequest{Method: "POST", URL: "http://p1"}},
			{Name: "g1", Parent: root, Request: &model.ParsedRequest{Method: "GET", URL: "http://g1"}},
			sub,
			{Name: "folder-no-request", Parent: root, IsFolder: true},
		}
		sub.Children = []*collections.CollectionNode{
			{Name: "g2", Parent: sub, Request: &model.ParsedRequest{Method: "GET", URL: "http://g2"}},
			{Name: "w1", Parent: sub, Request: &model.ParsedRequest{Method: "WEIRD", URL: "http://w1"}},
		}

		if !ed.DropCollectionNode(root, f32.Pt(400, 300)) {
			t.Fatal("drop must succeed")
		}
		added := ed.Scenario.Nodes[1:]
		if len(added) != 4 {
			t.Fatalf("expected 4 request nodes, got %d", len(added))
		}
		if len(ed.selected) != 4 {
			t.Errorf("all dropped nodes must be selected, got %d", len(ed.selected))
		}

		byName := map[string]*Node{}
		for _, n := range added {
			byName[n.NameEd.Text()] = n
		}
		if byName["g1"] == nil || byName["g2"] == nil || byName["p1"] == nil || byName["w1"] == nil {
			t.Fatalf("missing nodes: %v", byName)
		}
		if byName["g1"].X != byName["g2"].X {
			t.Error("requests with the same method must share a column")
		}
		if byName["g1"].Y == byName["g2"].Y {
			t.Error("requests in the same column must be stacked")
		}
		if !(byName["g1"].X < byName["p1"].X && byName["p1"].X < byName["w1"].X) {
			t.Errorf("columns must follow the known method order then unknown ones: GET=%v POST=%v WEIRD=%v",
				byName["g1"].X, byName["p1"].X, byName["w1"].X)
		}
	})

	t.Run("an empty folder adds nothing but is accepted", func(t *testing.T) {
		ed := newTestEditor()
		root := &collections.CollectionNode{Name: "root", IsFolder: true}
		if !ed.DropCollectionNode(root, f32.Pt(400, 300)) {
			t.Fatal("an empty folder drop must still be accepted")
		}
		if len(ed.Scenario.Nodes) != 1 {
			t.Errorf("nothing must be added, got %d nodes", len(ed.Scenario.Nodes))
		}
	})

	t.Run("a leaf without a request is rejected", func(t *testing.T) {
		ed := newTestEditor()
		parent := &collections.CollectionNode{Name: "p", IsFolder: true}
		src := &collections.CollectionNode{Name: "leaf", Parent: parent}
		if ed.DropCollectionNode(src, f32.Pt(400, 300)) {
			t.Error("a non-folder leaf without a request must be rejected")
		}
	})
}

func TestViewCenterWorld(t *testing.T) {
	ed := newTestEditor()
	ed.zoom = 2
	ed.pan = f32.Pt(-200, -100)
	got := ed.viewCenterWorld()
	want := ed.toWorld(f32.Pt(400, 300))
	if got != want {
		t.Errorf("viewCenterWorld = %v, want %v", got, want)
	}
}

func TestGeometryHelpers(t *testing.T) {
	t.Run("dist", func(t *testing.T) {
		if got := dist(f32.Pt(0, 0), f32.Pt(3, 4)); got != 5 {
			t.Errorf("dist = %v, want 5", got)
		}
		if got := dist(f32.Pt(1, 1), f32.Pt(1, 1)); got != 0 {
			t.Errorf("dist = %v, want 0", got)
		}
	})

	t.Run("bezier endpoints", func(t *testing.T) {
		p0, c0, c1, p1 := f32.Pt(0, 0), f32.Pt(10, 0), f32.Pt(20, 30), f32.Pt(30, 30)
		if got := bezierAt(p0, c0, c1, p1, 0); got != p0 {
			t.Errorf("t=0 gives %v, want %v", got, p0)
		}
		if got := bezierAt(p0, c0, c1, p1, 1); got != p1 {
			t.Errorf("t=1 gives %v, want %v", got, p1)
		}
		mid := bezierAt(p0, c0, c1, p1, 0.5)
		if mid.X <= p0.X || mid.X >= p1.X {
			t.Errorf("midpoint %v must lie between the endpoints", mid)
		}
	})

	t.Run("edge controls", func(t *testing.T) {
		ed := newTestEditor()
		c0, c1 := ed.edgeControls(f32.Pt(0, 10), f32.Pt(400, 90))
		if c0.X != 200 || c0.Y != 10 {
			t.Errorf("c0 = %v, want horizontal at half the span", c0)
		}
		if c1.X != 200 || c1.Y != 90 {
			t.Errorf("c1 = %v", c1)
		}

		near0, near1 := ed.edgeControls(f32.Pt(0, 0), f32.Pt(10, 0))
		if near0.X != 48 || near1.X != 10-48 {
			t.Errorf("short edges must use the minimum handle length, got %v and %v", near0, near1)
		}

		ed.zoom = 2
		z0, _ := ed.edgeControls(f32.Pt(0, 0), f32.Pt(10, 0))
		if z0.X != 96 {
			t.Errorf("the minimum handle must scale with zoom, got %v", z0.X)
		}
	})
}

func TestStateColor(t *testing.T) {
	idle := color.NRGBA{R: 1, G: 2, B: 3, A: 4}
	tests := []struct {
		name string
		st   int
		want color.NRGBA
	}{
		{"idle passes through", StIdle, idle},
		{"running", StRunning, color.NRGBA{R: 235, G: 180, B: 60, A: 255}},
		{"ok", StOK, color.NRGBA{R: 70, G: 190, B: 100, A: 255}},
		{"fail", StFail, theme.Danger},
		{"unknown passes through", 99, idle},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stateColor(tt.st, idle); got != tt.want {
				t.Errorf("stateColor = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKindColorIsDistinct(t *testing.T) {
	kinds := []NodeKind{KindStart, KindRequest, KindCondition, KindLoop, KindDelay, KindSetVar, KindNote}
	seen := map[color.NRGBA]NodeKind{}
	for _, k := range kinds {
		c := kindColor(k)
		if prev, dup := seen[c]; dup {
			t.Errorf("kinds %v and %v share the colour %v", prev, k, c)
		}
		seen[c] = k
	}
	if got := kindColor(NodeKind(99)); got != theme.FgMuted {
		t.Errorf("unknown kind colour = %v, want FgMuted", got)
	}
}

func TestStatusCodeColor(t *testing.T) {
	tests := []struct {
		name string
		code int
		ok   bool
		want color.NRGBA
	}{
		{"failure wins over the code", 200, false, theme.Danger},
		{"redirect", 302, true, color.NRGBA{R: 235, G: 180, B: 60, A: 255}},
		{"success", 201, true, color.NRGBA{R: 70, G: 190, B: 100, A: 255}},
		{"client error but ok flag", 404, true, color.NRGBA{R: 70, G: 190, B: 100, A: 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusCodeColor(tt.code, tt.ok); got != tt.want {
				t.Errorf("statusCodeColor(%d,%v) = %v, want %v", tt.code, tt.ok, got, tt.want)
			}
		})
	}
}

func TestFmtDur(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero", 0, ""},
		{"negative", -time.Second, ""},
		{"sub second", 250 * time.Millisecond, "250ms"},
		{"just under a second", 999 * time.Millisecond, "999ms"},
		{"exactly a second", time.Second, "1.0s"},
		{"seconds", 2500 * time.Millisecond, "2.5s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmtDur(tt.in); got != tt.want {
				t.Errorf("fmtDur(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinComma(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, ""},
		{"single", []string{"a"}, "{{a}}"},
		{"several", []string{"a", "b", "c"}, "{{a}}, {{b}}, {{c}}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinComma(tt.in); got != tt.want {
				t.Errorf("joinComma(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPaletteItemsCoverAllInsertableKinds(t *testing.T) {
	ed := newTestEditor()
	items := ed.paletteItems()
	if len(items) != len(ed.addBtns) {
		t.Fatalf("palette has %d items but %d buttons", len(items), len(ed.addBtns))
	}
	seen := map[NodeKind]bool{}
	for _, it := range items {
		if seen[it.kind] {
			t.Errorf("duplicate palette kind %v", it.kind)
		}
		seen[it.kind] = true
		if it.title == "" || it.desc == "" {
			t.Errorf("palette item %v needs a title and description", it.kind)
		}
	}
	if seen[KindStart] {
		t.Error("the start node must not be insertable from the palette")
	}
}

func TestEncodeFailureYieldsEmptyString(t *testing.T) {
	ed := newTestEditor()
	if ed.encode() == "" {
		t.Error("a valid scenario must encode to a non-empty string")
	}
}

func TestSaveScenario(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.SaveScenario()
	if ed.note != "Saved" {
		t.Errorf("note = %q, want %q", ed.note, "Saved")
	}
	if ed.lastSaved != ed.encode() {
		t.Error("lastSaved must match the encoded scenario")
	}
	if _, err := LoadScenario(ed.Scenario.ID); err != nil {
		t.Errorf("scenario must be on disk: %v", err)
	}
}

func TestOpenScenario(t *testing.T) {
	setupFlowConfig(t)
	other := NewScenario()
	other.NameEd.SetText("Other flow")
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	t.Run("loads and resets the view", func(t *testing.T) {
		ed := newTestEditor()
		if !ed.OpenScenario(other.ID) {
			t.Fatal("open must succeed")
		}
		if ed.Scenario.ID != other.ID {
			t.Errorf("scenario id = %q, want %q", ed.Scenario.ID, other.ID)
		}
		if ed.note != "Opened: Other flow" {
			t.Errorf("note = %q", ed.note)
		}
		if !ed.pendingFit {
			t.Error("opening must request a fit")
		}
		if len(ed.undoStack) != 1 {
			t.Errorf("opening must push one undo snapshot, got %d", len(ed.undoStack))
		}
	})

	t.Run("unnamed scenarios get a placeholder note", func(t *testing.T) {
		blank := NewScenario()
		if err := blank.Save(); err != nil {
			t.Fatal(err)
		}
		ed := newTestEditor()
		ed.OpenScenario(blank.ID)
		if ed.note != "Opened: Untitled" {
			t.Errorf("note = %q, want %q", ed.note, "Opened: Untitled")
		}
	})

	t.Run("reopening the current scenario is a no-op", func(t *testing.T) {
		ed := newTestEditor()
		id := ed.Scenario.ID
		if !ed.OpenScenario(id) {
			t.Fatal("must report success")
		}
		if len(ed.undoStack) != 0 {
			t.Error("reopening must not push history")
		}
	})

	t.Run("missing scenario fails", func(t *testing.T) {
		ed := newTestEditor()
		if ed.OpenScenario("ghost") {
			t.Error("opening a missing scenario must fail")
		}
	})

	t.Run("refused while a run is in flight", func(t *testing.T) {
		ed := newTestEditor()
		ed.Runner.mu.Lock()
		ed.Runner.running = true
		ed.Runner.mu.Unlock()
		if ed.OpenScenario(other.ID) {
			t.Error("opening must be refused while running")
		}
	})
}

func TestCreateNew(t *testing.T) {
	setupFlowConfig(t)

	t.Run("replaces the scenario and resets the view", func(t *testing.T) {
		ed := newTestEditor()
		old := ed.Scenario.ID
		ed.pan, ed.zoom = f32.Pt(50, 50), 2.5
		ed.mode = modeHistory
		ed.note = "stale"
		addNodeTo(ed, KindDelay, 0, 0)

		ed.CreateNew()
		if ed.Scenario.ID == old {
			t.Error("a new scenario must be created")
		}
		if len(ed.Scenario.Nodes) != 1 {
			t.Errorf("a new scenario must have just the start node, got %d", len(ed.Scenario.Nodes))
		}
		if ed.pan != (f32.Point{}) || ed.zoom != 1 || !ed.pendingFit {
			t.Error("the view must be reset")
		}
		if ed.mode != modeWidgets || ed.note != "" {
			t.Errorf("panel state must be reset, mode=%v note=%q", ed.mode, ed.note)
		}
		if _, err := LoadScenario(ed.Scenario.ID); err != nil {
			t.Errorf("the new scenario must be saved: %v", err)
		}
	})

	t.Run("refused while a run is in flight", func(t *testing.T) {
		ed := newTestEditor()
		id := ed.Scenario.ID
		ed.Runner.mu.Lock()
		ed.Runner.running = true
		ed.Runner.mu.Unlock()
		ed.CreateNew()
		if ed.Scenario.ID != id {
			t.Error("CreateNew must be refused while running")
		}
	})
}

func TestAutosave(t *testing.T) {
	setupFlowConfig(t)

	t.Run("the first call only arms the timer", func(t *testing.T) {
		ed := newTestEditor()
		ed.autosave()
		if ed.nextAutosave.IsZero() {
			t.Fatal("the timer must be armed")
		}
		if ed.lastSaved != ed.encode() {
			t.Error("the baseline must be recorded")
		}
		if _, err := LoadScenario(ed.Scenario.ID); err == nil {
			t.Error("the first call must not write to disk")
		}
	})

	t.Run("waits for the interval", func(t *testing.T) {
		ed := newTestEditor()
		ed.autosave()
		addNodeTo(ed, KindDelay, 0, 0)
		ed.autosave()
		if _, err := LoadScenario(ed.Scenario.ID); err == nil {
			t.Error("autosave must not fire before the interval elapses")
		}
	})

	t.Run("writes a changed scenario once the interval elapses", func(t *testing.T) {
		ed := newTestEditor()
		ed.autosave()
		addNodeTo(ed, KindDelay, 0, 0)
		ed.nextAutosave = time.Now().Add(-time.Second)

		ed.autosave()
		if _, err := LoadScenario(ed.Scenario.ID); err != nil {
			t.Fatalf("autosave must write: %v", err)
		}
		if ed.note != "Auto-saved" {
			t.Errorf("note = %q, want %q", ed.note, "Auto-saved")
		}
		if !ed.nextAutosave.After(time.Now()) {
			t.Error("the timer must be re-armed")
		}
	})

	t.Run("an unchanged scenario is not rewritten", func(t *testing.T) {
		ed := newTestEditor()
		ed.autosave()
		ed.nextAutosave = time.Now().Add(-time.Second)
		ed.autosave()
		if ed.note != "" {
			t.Errorf("note = %q, want an empty note for an unchanged scenario", ed.note)
		}
		if _, err := LoadScenario(ed.Scenario.ID); err == nil {
			t.Error("an unchanged scenario must not be written")
		}
	})

	t.Run("a user note is not overwritten", func(t *testing.T) {
		ed := newTestEditor()
		ed.autosave()
		addNodeTo(ed, KindDelay, 0, 0)
		ed.nextAutosave = time.Now().Add(-time.Second)
		ed.note = "⚠ something"
		ed.autosave()
		if ed.note != "⚠ something" {
			t.Errorf("note = %q, want the user note kept", ed.note)
		}
	})
}

func TestToggleRunStopsARunningScenario(t *testing.T) {
	ed := newTestEditor()
	ed.Runner.mu.Lock()
	ed.Runner.running = true
	ed.Runner.cancel = func() { ed.note = "cancelled" }
	ed.Runner.mu.Unlock()

	ed.ToggleRun(&Host{Win: testWindow()})
	if ed.note != "cancelled" {
		t.Error("ToggleRun on a running scenario must stop it")
	}
}

func TestToggleRunReportsValidationWarnings(t *testing.T) {
	ed := newTestEditor()
	req := addNodeTo(ed, KindRequest, 300, 0)
	connect(ed, ed.Scenario.Nodes[0], req)

	host := &Host{
		Win:       testWindow(),
		RootCtx:   nil,
		ActiveEnv: func() map[string]string { return map[string]string{"k": "v"} },
	}
	ed.ToggleRun(host)
	waitRunner(t, ed.Runner)

	if !strings.HasPrefix(ed.note, "⚠ ") || !strings.Contains(ed.note, "empty URL") {
		t.Errorf("note = %q, want an empty URL warning", ed.note)
	}
	if ed.mode != modeHistory {
		t.Error("running must switch to the history panel")
	}
	if ed.histRun != nil {
		t.Error("the pinned history run must be cleared")
	}
}
