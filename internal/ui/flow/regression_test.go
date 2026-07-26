package flow

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestItoaMatchesStrconv(t *testing.T) {
	cases := []int{
		0, 1, -1, 9, -9, 10, -10, 999, -999,
		1000000, -1000000,
		999999999999, -999999999999,
		1000000000000, -1000000000000,
		math.MaxInt32, math.MinInt32,
		math.MaxInt64, math.MinInt64,
	}
	for _, v := range cases {
		want := strconv.Itoa(v)
		got := itoa(v)
		if got != want {
			t.Errorf("itoa(%d) = %q, want %q", v, got, want)
		}
	}
}

func loopNodeAt(ed *Editor, x, y, w, h float32) *Node {
	n := addNodeTo(ed, KindLoop, x, y)
	n.W = w
	n.H = h
	return n
}

func scenarioStart(t *testing.T, ed *Editor) *Node {
	t.Helper()
	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindStart {
			return n
		}
	}
	t.Fatal("scenario has no start node")
	return nil
}

func TestValidateScenarioWarnsOnNestedLoop(t *testing.T) {
	ed := newTestEditor()
	start := scenarioStart(t, ed)
	outer := loopNodeAt(ed, 100, 100, 600, 400)
	connect(ed, start, outer)

	inner := loopNodeAt(ed, 200, 250, 200, 120)

	dw, dh := ed.defSizes()
	if !loopContains(outer, inner, dw, dh) {
		t.Fatal("test setup: inner loop is not geometrically inside the outer loop")
	}

	warns := ed.validateScenario()
	found := false
	for _, w := range warns {
		if strings.Contains(w, "nested loop") {
			found = true
		}
	}
	if !found {
		t.Errorf("validateScenario() = %v, want a nested-loop warning since buildPlan never runs it", warns)
	}
}

func TestValidateScenarioNestedLoopIsUnreachable(t *testing.T) {
	ed := newTestEditor()
	start := scenarioStart(t, ed)
	outer := loopNodeAt(ed, 100, 100, 600, 400)
	connect(ed, start, outer)
	loopNodeAt(ed, 200, 250, 200, 120)

	warns := ed.validateScenario()
	found := false
	for _, w := range warns {
		if strings.Contains(w, "unreachable") {
			found = true
		}
	}
	if !found {
		t.Errorf("validateScenario() = %v, want the nested loop counted as unreachable", warns)
	}
}

func TestValidateScenarioNoNestedWarningForPlainMembers(t *testing.T) {
	ed := newTestEditor()
	start := scenarioStart(t, ed)
	outer := loopNodeAt(ed, 100, 100, 600, 400)
	connect(ed, start, outer)

	req := addNodeTo(ed, KindRequest, 200, 250)
	req.URLEd.SetText("https://example.com")

	dw, dh := ed.defSizes()
	if !loopContains(outer, req, dw, dh) {
		t.Fatal("test setup: request is not inside the loop")
	}

	for _, w := range ed.validateScenario() {
		if strings.Contains(w, "nested loop") {
			t.Errorf("unexpected nested-loop warning for a plain member: %v", w)
		}
		if strings.Contains(w, "unreachable") {
			t.Errorf("loop member should be reachable: %v", w)
		}
	}
}

func TestBuildPlanSkipsNestedLoopMembers(t *testing.T) {
	ed := newTestEditor()
	outer := loopNodeAt(ed, 100, 100, 600, 400)
	inner := loopNodeAt(ed, 200, 250, 200, 120)

	dw, dh := ed.defSizes()
	if !loopContains(outer, inner, dw, dh) {
		t.Fatal("test setup: inner loop is not inside the outer loop")
	}

	plan, _ := buildPlan(ed.Scenario, nil, nil, dw, dh)
	ex := plan[outer.ID]
	if ex == nil {
		t.Fatal("outer loop missing from the plan")
	}
	for _, id := range ex.entries {
		if id == inner.ID {
			t.Error("buildPlan must not treat a nested loop as a loop entry; validation warns about it instead")
		}
	}
}
