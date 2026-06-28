package flow

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
)

func newFitEditor() *Editor {
	return &Editor{
		Scenario: &Scenario{Nodes: []*Node{
			{ID: "a", X: 100, Y: 100},
			{ID: "b", X: 900, Y: 700},
		}},
		zoom:       1,
		nodeW:      176,
		nodeH:      56,
		selected:   map[string]bool{},
		pendingFit: true,
	}
}

func TestMaybeFitOnShow_WaitsForRealCanvasSize(t *testing.T) {
	ed := newFitEditor()

	ed.maybeFitOnShow()
	if !ed.pendingFit {
		t.Fatal("pendingFit must survive while the canvas size is zero")
	}

	ed.canvasSize = image.Pt(800, 600)
	ed.maybeFitOnShow()
	if ed.pendingFit {
		t.Fatal("pendingFit must be cleared after the deferred fit runs")
	}

	for _, n := range ed.Scenario.Nodes {
		s := ed.toScreen(f32.Pt(n.X, n.Y))
		if s.X < 0 || s.Y < 0 || s.X > 800 || s.Y > 600 {
			t.Errorf("node %s projects to %v, outside the fitted 800x600 canvas", n.ID, s)
		}
	}
}

func TestMaybeFitOnShow_UnchangedScenarioKeepsView(t *testing.T) {
	ed := newFitEditor()
	ed.canvasSize = image.Pt(800, 600)
	ed.maybeFitOnShow()

	ed.pan = f32.Pt(12, 34)
	ed.zoom = 2
	ed.maybeFitOnShow()
	if ed.pan != (f32.Pt(12, 34)) || ed.zoom != 2 {
		t.Errorf("unchanged scenario must keep pan=%v zoom=%v, got pan=%v zoom=%v",
			f32.Pt(12, 34), float32(2), ed.pan, ed.zoom)
	}
}

func TestMaybeFitOnShow_RefitsAfterScenarioChange(t *testing.T) {
	ed := newFitEditor()
	ed.canvasSize = image.Pt(800, 600)
	ed.maybeFitOnShow()

	ed.pan = f32.Pt(999, 999)
	ed.zoom = 5
	ed.pendingFit = true
	ed.maybeFitOnShow()
	if ed.pendingFit {
		t.Fatal("pendingFit must be cleared by the refit")
	}
	if ed.zoom == 5 {
		t.Error("a scenario change must refit (zoom should no longer be the stale 5)")
	}
}
