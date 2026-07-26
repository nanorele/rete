package flow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/persist"
)

func setupFlowConfig(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "tracto-test")
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	return persist.FlowsDir()
}

func mkNode(kind NodeKind, x, y float32, set func(*Node)) *Node {
	n := NewNode(kind, x, y)
	if set != nil {
		set(n)
	}
	return n
}

func TestNodeKindTitle(t *testing.T) {
	tests := []struct {
		name string
		kind NodeKind
		want string
	}{
		{"start", KindStart, "Start"},
		{"request", KindRequest, "HTTP Request"},
		{"condition", KindCondition, "Condition"},
		{"loop", KindLoop, "Loop"},
		{"delay", KindDelay, "Delay"},
		{"setvar", KindSetVar, "Set Variable"},
		{"note", KindNote, "Note"},
		{"unknown", NodeKind(99), "Node"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCondKindTitle(t *testing.T) {
	tests := []struct {
		name string
		cond CondKind
		want string
	}{
		{"always", CondAlways, "Always"},
		{"status", CondStatus, "HTTP status"},
		{"has response", CondHasResponse, "Has response"},
		{"no response", CondNoResponse, "No response"},
		{"body field", CondBodyField, "Body has field"},
		{"array count", CondArrayCount, "Array count"},
		{"body value", CondBodyValue, "Field value"},
		{"unknown", CondKind(42), "Always"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.Title(); got != tt.want {
				t.Errorf("Title() = %q, want %q", got, tt.want)
			}
		})
	}
	if len(CondKinds) != 7 {
		t.Errorf("CondKinds should list all 7 kinds, got %d", len(CondKinds))
	}
}

func TestNewNodeDefaults(t *testing.T) {
	tests := []struct {
		name      string
		kind      NodeKind
		wantName  string
		wantMeth  string
		wantCount string
		wantDelay string
	}{
		{"start", KindStart, "Start", "", "", ""},
		{"request", KindRequest, "HTTP Request", "GET", "", ""},
		{"condition", KindCondition, "Condition", "", "", ""},
		{"loop", KindLoop, "Loop", "", "3", "0"},
		{"delay", KindDelay, "Delay", "", "", "1000"},
		{"setvar", KindSetVar, "Set Variable", "", "", ""},
		{"note", KindNote, "Note", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNode(tt.kind, 10, 20)
			if n.ID == "" {
				t.Error("ID must be generated")
			}
			if n.X != 10 || n.Y != 20 {
				t.Errorf("position = (%v,%v), want (10,20)", n.X, n.Y)
			}
			if got := n.NameEd.Text(); got != tt.wantName {
				t.Errorf("name = %q, want %q", got, tt.wantName)
			}
			if n.Method != tt.wantMeth {
				t.Errorf("method = %q, want %q", n.Method, tt.wantMeth)
			}
			if got := n.CountEd.Text(); got != tt.wantCount {
				t.Errorf("count = %q, want %q", got, tt.wantCount)
			}
			if got := n.DelayEd.Text(); got != tt.wantDelay {
				t.Errorf("delay = %q, want %q", got, tt.wantDelay)
			}
			if !n.NameEd.SingleLine || !n.URLEd.SingleLine {
				t.Error("name and url editors must be single line")
			}
		})
	}
}

func TestNodeDisplayName(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"custom", "My node", "My node"},
		{"trimmed", "  spaced  ", "spaced"},
		{"empty falls back to kind", "", "HTTP Request"},
		{"blank falls back to kind", "   ", "HTTP Request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := NewNode(KindRequest, 0, 0)
			n.NameEd.SetText(tt.text)
			if got := n.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeSummary(t *testing.T) {
	tests := []struct {
		name string
		node *Node
		want string
	}{
		{"start", NewNode(KindStart, 0, 0), "entry point"},
		{
			"request with url",
			mkNode(KindRequest, 0, 0, func(n *Node) { n.URLEd.SetText(" http://x ") }),
			"GET http://x",
		},
		{"request without url", NewNode(KindRequest, 0, 0), "GET no url"},
		{"condition", NewNode(KindCondition, 0, 0), "routes by arrow rules"},
		{"loop by count", NewNode(KindLoop, 0, 0), "repeat 3×"},
		{
			"loop with empty count",
			mkNode(KindLoop, 0, 0, func(n *Node) { n.CountEd.SetText("") }),
			"repeat 1×",
		},
		{
			"loop with delay",
			mkNode(KindLoop, 0, 0, func(n *Node) { n.DelayEd.SetText("250") }),
			"repeat 3× · 250 ms",
		},
		{
			"loop over source",
			mkNode(KindLoop, 0, 0, func(n *Node) { n.LoopSrcEd.SetText("$.items") }),
			"for each $.items",
		},
		{"delay", NewNode(KindDelay, 0, 0), "1000 ms"},
		{
			"delay empty",
			mkNode(KindDelay, 0, 0, func(n *Node) { n.DelayEd.SetText("") }),
			"0 ms",
		},
		{
			"setvar",
			mkNode(KindSetVar, 0, 0, func(n *Node) {
				n.VarNameEd.SetText("tok")
				n.VarValueEd.SetText("abc")
			}),
			"tok = abc",
		},
		{"setvar unnamed", NewNode(KindSetVar, 0, 0), "var = "},
		{
			"note first line only",
			mkNode(KindNote, 0, 0, func(n *Node) { n.BodyEd.SetText("first\nsecond") }),
			"first",
		},
		{"unknown kind", &Node{Kind: NodeKind(77)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNodeHasPorts(t *testing.T) {
	for _, k := range []NodeKind{KindStart, KindRequest, KindCondition, KindLoop, KindDelay, KindSetVar} {
		if !NewNode(k, 0, 0).HasPorts() {
			t.Errorf("kind %v must have ports", k)
		}
	}
	if NewNode(KindNote, 0, 0).HasPorts() {
		t.Error("note must not have ports")
	}
}

func TestNodeSizeWorld(t *testing.T) {
	var defW, defH float32 = 100, 50
	tests := []struct {
		name  string
		node  *Node
		wantW float32
		wantH float32
	}{
		{"plain node ignores W/H", &Node{Kind: KindRequest, W: 500, H: 500}, defW, defH},
		{"loop default size", &Node{Kind: KindLoop}, defW * 2.4, defH * 4},
		{"loop explicit size", &Node{Kind: KindLoop, W: 300, H: 400}, 300, 400},
		{"loop negative size falls back", &Node{Kind: KindLoop, W: -1, H: -1}, defW * 2.4, defH * 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, h := nodeSizeWorld(tt.node, defW, defH)
			if w != tt.wantW || h != tt.wantH {
				t.Errorf("nodeSizeWorld = (%v,%v), want (%v,%v)", w, h, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestLoopContains(t *testing.T) {
	loop := &Node{ID: "loop", Kind: KindLoop, X: 0, Y: 0, W: 400, H: 300}
	tests := []struct {
		name string
		loop *Node
		node *Node
		want bool
	}{
		{"inside body", loop, &Node{ID: "a", Kind: KindRequest, X: 100, Y: 100}, true},
		{"in header band", loop, &Node{ID: "b", Kind: KindRequest, X: 100, Y: 0}, false},
		{"below body", loop, &Node{ID: "c", Kind: KindRequest, X: 100, Y: 400}, false},
		{"left of loop", loop, &Node{ID: "d", Kind: KindRequest, X: -200, Y: 100}, false},
		{"right of loop", loop, &Node{ID: "e", Kind: KindRequest, X: 400, Y: 100}, false},
		{"self", loop, loop, false},
		{"non loop container", &Node{ID: "n", Kind: KindRequest}, &Node{ID: "a", X: 10, Y: 10}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := loopContains(tt.loop, tt.node, 100, 50); got != tt.want {
				t.Errorf("loopContains = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEdgeSummary(t *testing.T) {
	build := func(cond CondKind, val, op, count, val2 string) *Edge {
		e := NewEdge("a", "b")
		e.Cond = cond
		e.ValueEd.SetText(val)
		if op != "" {
			e.Op = op
		}
		e.CountEd.SetText(count)
		e.Val2Ed.SetText(val2)
		return e
	}
	tests := []struct {
		name string
		edge *Edge
		want string
	}{
		{"always", build(CondAlways, "", "", "", ""), "Always"},
		{"status explicit", build(CondStatus, "404", "", "", ""), "HTTP 404"},
		{"status default", build(CondStatus, "", "", "", ""), "HTTP 2xx"},
		{"has response", build(CondHasResponse, "", "", "", ""), "Has response"},
		{"no response", build(CondNoResponse, "", "", "", ""), "No response"},
		{"body field", build(CondBodyField, "data.id", "", "", ""), "Has data.id"},
		{"body field default", build(CondBodyField, "", "", "", ""), "Has field"},
		{"array count", build(CondArrayCount, "items", ">", "3", ""), "len(items) > 3"},
		{"array count defaults", build(CondArrayCount, "", "", "", ""), "len(field) > 0"},
		{"body value", build(CondBodyValue, "st", "==", "", "ok"), "st == ok"},
		{"body value default op", func() *Edge {
			e := build(CondBodyValue, "st", "", "", "ok")
			e.Op = ""
			return e
		}(), "st == ok"},
		{"unknown", build(CondKind(55), "", "", "", ""), "Always"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.edge.Summary(); got != tt.want {
				t.Errorf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewEdgeDefaults(t *testing.T) {
	e := NewEdge("from", "to")
	if e.ID == "" || e.From != "from" || e.To != "to" {
		t.Errorf("unexpected edge: %+v", e)
	}
	if e.Cond != CondAlways || e.Op != ">" || e.CountEd.Text() != "0" {
		t.Errorf("unexpected defaults: cond=%v op=%q count=%q", e.Cond, e.Op, e.CountEd.Text())
	}
}

func TestNewScenarioHasStart(t *testing.T) {
	s := NewScenario()
	if s.ID == "" {
		t.Error("scenario needs an ID")
	}
	if len(s.Nodes) != 1 || s.Nodes[0].Kind != KindStart {
		t.Fatalf("new scenario must contain exactly one start node, got %d", len(s.Nodes))
	}
}

func TestScenarioLookups(t *testing.T) {
	s := &Scenario{}
	a := NewNode(KindRequest, 0, 0)
	a.ID = "a"
	b := NewNode(KindRequest, 0, 0)
	b.ID = "b"
	s.Nodes = append(s.Nodes, a, b)
	e := NewEdge("a", "b")
	e.ID = "e1"
	s.Edges = append(s.Edges, e)

	if s.NodeByID("a") != a {
		t.Error("NodeByID(a) must return node a")
	}
	if s.NodeByID("missing") != nil {
		t.Error("NodeByID for unknown id must return nil")
	}
	if s.EdgeByID("e1") != e {
		t.Error("EdgeByID(e1) must return edge e1")
	}
	if s.EdgeByID("nope") != nil {
		t.Error("EdgeByID for unknown id must return nil")
	}
	if !s.HasEdge("a", "b") {
		t.Error("HasEdge(a,b) must be true")
	}
	if s.HasEdge("b", "a") {
		t.Error("HasEdge is directional")
	}
}

func TestScenarioRemoveNode(t *testing.T) {
	newScenario := func() *Scenario {
		s := &Scenario{}
		start := NewNode(KindStart, 0, 0)
		start.ID = "start"
		a := NewNode(KindRequest, 0, 0)
		a.ID = "a"
		b := NewNode(KindRequest, 0, 0)
		b.ID = "b"
		s.Nodes = append(s.Nodes, start, a, b)
		e1 := NewEdge("start", "a")
		e1.ID = "e1"
		e2 := NewEdge("a", "b")
		e2.ID = "e2"
		s.Edges = append(s.Edges, e1, e2)
		return s
	}

	t.Run("removes node and its edges", func(t *testing.T) {
		s := newScenario()
		s.RemoveNode("a")
		if s.NodeByID("a") != nil {
			t.Error("node a must be gone")
		}
		if len(s.Edges) != 0 {
			t.Errorf("edges touching a must be dropped, got %d", len(s.Edges))
		}
	})

	t.Run("start node is protected with its edges", func(t *testing.T) {
		s := newScenario()
		s.RemoveNode("start")
		if s.NodeByID("start") == nil {
			t.Error("start node must not be removable")
		}
		if len(s.Edges) != 2 {
			t.Errorf("protecting start must keep all edges, got %d", len(s.Edges))
		}
	})

	t.Run("unknown id still prunes nothing", func(t *testing.T) {
		s := newScenario()
		s.RemoveNode("zzz")
		if len(s.Nodes) != 3 || len(s.Edges) != 2 {
			t.Errorf("unknown id must be a no-op, got %d nodes %d edges", len(s.Nodes), len(s.Edges))
		}
	})
}

func TestScenarioRemoveEdge(t *testing.T) {
	s := &Scenario{}
	e1 := NewEdge("a", "b")
	e1.ID = "e1"
	e2 := NewEdge("b", "c")
	e2.ID = "e2"
	s.Edges = append(s.Edges, e1, e2)

	s.RemoveEdge("e1")
	if len(s.Edges) != 1 || s.Edges[0].ID != "e2" {
		t.Fatalf("expected only e2 left, got %+v", s.Edges)
	}
	s.RemoveEdge("nope")
	if len(s.Edges) != 1 {
		t.Errorf("removing unknown edge must be a no-op, got %d", len(s.Edges))
	}
}

func TestNodeDTORoundTrip(t *testing.T) {
	n := NewNode(KindRequest, 12, 34)
	n.W, n.H = 7, 8
	n.EnvID = "env1"
	n.Method = "POST"
	n.NameEd.SetText("call")
	n.URLEd.SetText("http://x")
	n.HeadersEd.SetText("A: 1")
	n.BodyEd.SetText("{}")
	n.CountEd.SetText("5")
	n.DelayEd.SetText("9")
	n.VarNameEd.SetText("v")
	n.VarValueEd.SetText("w")
	n.LoopSrcEd.SetText("$.a")

	got := nodeFromDTO(nodeToDTO(n))
	if got.ID != n.ID || got.Kind != n.Kind || got.X != 12 || got.Y != 34 {
		t.Errorf("identity/position lost: %+v", got)
	}
	if got.W != 7 || got.H != 8 || got.EnvID != "env1" || got.Method != "POST" {
		t.Errorf("size/env/method lost: w=%v h=%v env=%q m=%q", got.W, got.H, got.EnvID, got.Method)
	}
	fields := map[string][2]string{
		"name":     {got.NameEd.Text(), "call"},
		"url":      {got.URLEd.Text(), "http://x"},
		"headers":  {got.HeadersEd.Text(), "A: 1"},
		"body":     {got.BodyEd.Text(), "{}"},
		"count":    {got.CountEd.Text(), "5"},
		"delay":    {got.DelayEd.Text(), "9"},
		"varname":  {got.VarNameEd.Text(), "v"},
		"varvalue": {got.VarValueEd.Text(), "w"},
		"loopsrc":  {got.LoopSrcEd.Text(), "$.a"},
	}
	for name, pair := range fields {
		if pair[0] != pair[1] {
			t.Errorf("%s = %q, want %q", name, pair[0], pair[1])
		}
	}
}

func TestNodeFromDTOFallbacks(t *testing.T) {
	n := nodeFromDTO(nodeDTO{Kind: int(KindRequest)})
	if n.ID == "" {
		t.Error("empty dto id must get a fresh generated id")
	}
	if n.Method != "GET" {
		t.Errorf("empty dto method must keep the kind default GET, got %q", n.Method)
	}
}

func TestEdgeDTORoundTrip(t *testing.T) {
	e := NewEdge("a", "b")
	e.Cond = CondArrayCount
	e.Op = "<="
	e.ValueEd.SetText("items")
	e.CountEd.SetText("4")
	e.Val2Ed.SetText("x")

	got := edgeFromDTO(edgeToDTO(e))
	if got.ID != e.ID || got.From != "a" || got.To != "b" || got.Cond != CondArrayCount {
		t.Errorf("core fields lost: %+v", got)
	}
	if got.Op != "<=" || got.ValueEd.Text() != "items" || got.CountEd.Text() != "4" || got.Val2Ed.Text() != "x" {
		t.Errorf("value fields lost: op=%q val=%q cnt=%q val2=%q",
			got.Op, got.ValueEd.Text(), got.CountEd.Text(), got.Val2Ed.Text())
	}
}

func TestEdgeFromDTOFallbacks(t *testing.T) {
	e := edgeFromDTO(edgeDTO{From: "a", To: "b"})
	if e.ID == "" {
		t.Error("empty dto id must get a fresh generated id")
	}
	if e.Op != ">" {
		t.Errorf("empty dto op must keep the default, got %q", e.Op)
	}
	if e.CountEd.Text() != "0" {
		t.Errorf("empty dto count must keep the default 0, got %q", e.CountEd.Text())
	}
}

func TestScenarioFromDTO(t *testing.T) {
	t.Run("drops edges with unknown endpoints", func(t *testing.T) {
		dto := scenarioDTO{
			ID:    "s1",
			Nodes: []nodeDTO{{ID: "a", Kind: int(KindStart)}, {ID: "b", Kind: int(KindRequest)}},
			Edges: []edgeDTO{{ID: "ok", From: "a", To: "b"}, {ID: "bad", From: "a", To: "ghost"}},
		}
		s := scenarioFromDTO(dto)
		if len(s.Edges) != 1 || s.Edges[0].ID != "ok" {
			t.Errorf("dangling edge must be dropped, got %+v", s.Edges)
		}
	})

	t.Run("injects a start node when missing", func(t *testing.T) {
		s := scenarioFromDTO(scenarioDTO{ID: "s2", Nodes: []nodeDTO{{ID: "a", Kind: int(KindRequest)}}})
		if len(s.Nodes) != 2 || s.Nodes[0].Kind != KindStart {
			t.Fatalf("start node must be prepended, got %d nodes", len(s.Nodes))
		}
	})

	t.Run("keeps an existing start node", func(t *testing.T) {
		s := scenarioFromDTO(scenarioDTO{ID: "s3", Nodes: []nodeDTO{{ID: "st", Kind: int(KindStart)}}})
		if len(s.Nodes) != 1 {
			t.Errorf("must not add a second start node, got %d", len(s.Nodes))
		}
	})

	t.Run("generates an ID when empty", func(t *testing.T) {
		s := scenarioFromDTO(scenarioDTO{})
		if s.ID == "" {
			t.Error("empty dto ID must be replaced by a generated one")
		}
	})

	t.Run("carries the name", func(t *testing.T) {
		s := scenarioFromDTO(scenarioDTO{ID: "s4", Name: "My flow"})
		if s.NameEd.Text() != "My flow" {
			t.Errorf("name = %q, want %q", s.NameEd.Text(), "My flow")
		}
	})
}

func TestEncodeDecodeScenario(t *testing.T) {
	s := NewScenario()
	s.NameEd.SetText("  Round trip  ")
	n := NewNode(KindRequest, 50, 60)
	n.URLEd.SetText("http://example.test")
	s.Nodes = append(s.Nodes, n)
	s.Edges = append(s.Edges, NewEdge(s.Nodes[0].ID, n.ID))

	data, err := encodeScenario(s)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := decodeScenario(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("ID = %q, want %q", got.ID, s.ID)
	}
	if got.NameEd.Text() != "Round trip" {
		t.Errorf("name must be trimmed on encode, got %q", got.NameEd.Text())
	}
	if len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Errorf("expected 2 nodes / 1 edge, got %d / %d", len(got.Nodes), len(got.Edges))
	}
}

func TestDecodeScenarioInvalidJSON(t *testing.T) {
	if _, err := decodeScenario("{not json"); err == nil {
		t.Error("expected an error for malformed JSON")
	}
}

func TestSaveLoadDeleteScenario(t *testing.T) {
	dir := setupFlowConfig(t)
	s := NewScenario()
	s.NameEd.SetText("Persisted")

	before := ChangeSeq()
	if err := s.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	if ChangeSeq() <= before {
		t.Error("Save must bump the change sequence")
	}
	if _, err := os.Stat(filepath.Join(dir, s.ID+".json")); err != nil {
		t.Fatalf("scenario file missing: %v", err)
	}

	loaded, err := LoadScenario(s.ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.NameEd.Text() != "Persisted" || loaded.ID != s.ID {
		t.Errorf("loaded scenario mismatch: id=%q name=%q", loaded.ID, loaded.NameEd.Text())
	}

	if err := DeleteScenario(s.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := LoadScenario(s.ID); err == nil {
		t.Error("loading a deleted scenario must fail")
	}
	if err := DeleteScenario(s.ID); err == nil {
		t.Error("deleting a missing scenario must fail")
	}
}

func TestLoadScenarioCorrupt(t *testing.T) {
	dir := setupFlowConfig(t)
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadScenario("bad"); err == nil {
		t.Error("corrupt file must produce an error")
	}
}

func writeFlow(t *testing.T, dir string, dto scenarioDTO, mod time.Time) {
	t.Helper()
	data, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, dto.ID+".json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestListScenarios(t *testing.T) {
	dir := setupFlowConfig(t)
	base := time.Now().Add(-time.Hour)
	writeFlow(t, dir, scenarioDTO{ID: "old", Name: "Old"}, base)
	writeFlow(t, dir, scenarioDTO{ID: "new", Name: "New"}, base.Add(30*time.Minute))
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "noid.json"), []byte(`{"name":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := ListScenarios()
	if len(got) != 2 {
		t.Fatalf("expected 2 valid scenarios, got %d (%+v)", len(got), got)
	}
	if got[0].ID != "new" || got[1].ID != "old" {
		t.Errorf("expected newest first, got %q then %q", got[0].ID, got[1].ID)
	}
	if got[0].Name != "New" {
		t.Errorf("name = %q, want %q", got[0].Name, "New")
	}
}

func TestListScenariosMissingDir(t *testing.T) {
	persist.SetConfigOverride(filepath.Join(t.TempDir(), "no-such"))
	t.Cleanup(func() { persist.SetConfigOverride("") })
	dir := persist.FlowsDir()
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if got := ListScenarios(); got != nil {
		t.Errorf("missing directory must yield nil, got %+v", got)
	}
}

func TestRenameScenario(t *testing.T) {
	dir := setupFlowConfig(t)
	writeFlow(t, dir, scenarioDTO{ID: "s1", Name: "Before"}, time.Now())

	if err := RenameScenario("s1", "  After  "); err != nil {
		t.Fatalf("rename: %v", err)
	}
	dto, err := readScenarioDTO("s1")
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if dto.Name != "After" {
		t.Errorf("name = %q, want trimmed %q", dto.Name, "After")
	}
	if err := RenameScenario("ghost", "X"); err == nil {
		t.Error("renaming a missing scenario must fail")
	}
}

func TestDuplicateScenario(t *testing.T) {
	dir := setupFlowConfig(t)
	writeFlow(t, dir, scenarioDTO{
		ID:    "src",
		Name:  "Original",
		Nodes: []nodeDTO{{ID: "a", Kind: int(KindStart)}},
	}, time.Now())

	newID, err := DuplicateScenario("src")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if newID == "src" || newID == "" {
		t.Fatalf("duplicate must get a fresh id, got %q", newID)
	}
	dto, err := readScenarioDTO(newID)
	if err != nil {
		t.Fatalf("read copy: %v", err)
	}
	if dto.Name != "Original Copy" {
		t.Errorf("copy name = %q, want %q", dto.Name, "Original Copy")
	}
	if len(dto.Nodes) != 1 {
		t.Errorf("copy must keep nodes, got %d", len(dto.Nodes))
	}
	if _, err := readScenarioDTO("src"); err != nil {
		t.Errorf("original must survive: %v", err)
	}
}

func TestDuplicateScenarioUnnamed(t *testing.T) {
	dir := setupFlowConfig(t)
	writeFlow(t, dir, scenarioDTO{ID: "src"}, time.Now())
	newID, err := DuplicateScenario("src")
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	dto, _ := readScenarioDTO(newID)
	if dto.Name != "Untitled Copy" {
		t.Errorf("unnamed copy = %q, want %q", dto.Name, "Untitled Copy")
	}
}

func TestDuplicateScenarioMissing(t *testing.T) {
	setupFlowConfig(t)
	if _, err := DuplicateScenario("ghost"); err == nil {
		t.Error("duplicating a missing scenario must fail")
	}
}

func TestImportScenario(t *testing.T) {
	setupFlowConfig(t)
	tests := []struct {
		name    string
		data    string
		wantErr bool
	}{
		{"valid", `{"id":"orig","name":"Imported","nodes":[{"id":"a","kind":0}]}`, false},
		{"malformed", `{`, true},
		{"no nodes", `{"id":"x","name":"Empty"}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ImportScenario([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("import: %v", err)
			}
			if id == "orig" {
				t.Error("import must assign a fresh id")
			}
			dto, err := readScenarioDTO(id)
			if err != nil {
				t.Fatalf("read imported: %v", err)
			}
			if dto.Name != "Imported" || len(dto.Nodes) != 1 {
				t.Errorf("imported content wrong: %+v", dto)
			}
		})
	}
}

func TestImportScenarioEmptyError(t *testing.T) {
	setupFlowConfig(t)
	_, err := ImportScenario([]byte(`{"id":"x"}`))
	if err != errEmptyScenario {
		t.Errorf("err = %v, want errEmptyScenario", err)
	}
}

func TestLoadLatest(t *testing.T) {
	t.Run("empty directory yields a fresh scenario", func(t *testing.T) {
		setupFlowConfig(t)
		s := LoadLatest()
		if s == nil || len(s.Nodes) != 1 || s.Nodes[0].Kind != KindStart {
			t.Fatalf("expected a fresh scenario with a start node, got %+v", s)
		}
	})

	t.Run("picks the newest file", func(t *testing.T) {
		dir := setupFlowConfig(t)
		base := time.Now().Add(-2 * time.Hour)
		writeFlow(t, dir, scenarioDTO{ID: "a", Name: "A", Nodes: []nodeDTO{{ID: "n", Kind: int(KindStart)}}}, base)
		writeFlow(t, dir, scenarioDTO{ID: "b", Name: "B", Nodes: []nodeDTO{{ID: "n", Kind: int(KindStart)}}}, base.Add(time.Hour))
		s := LoadLatest()
		if s.ID != "b" {
			t.Errorf("expected newest scenario b, got %q", s.ID)
		}
	})

	t.Run("corrupt newest file falls back to a fresh scenario", func(t *testing.T) {
		dir := setupFlowConfig(t)
		if err := os.WriteFile(filepath.Join(dir, "x.json"), []byte("{{{"), 0o644); err != nil {
			t.Fatal(err)
		}
		s := LoadLatest()
		if len(s.Nodes) != 1 || s.Nodes[0].Kind != KindStart {
			t.Errorf("expected fresh fallback scenario, got %d nodes", len(s.Nodes))
		}
	})

	t.Run("missing directory yields a fresh scenario", func(t *testing.T) {
		persist.SetConfigOverride(filepath.Join(t.TempDir(), "gone"))
		t.Cleanup(func() { persist.SetConfigOverride("") })
		if err := os.RemoveAll(persist.FlowsDir()); err != nil {
			t.Fatal(err)
		}
		if s := LoadLatest(); s == nil || len(s.Nodes) != 1 {
			t.Error("expected a fresh scenario when the flows directory is absent")
		}
	})
}

func TestWriteScenarioDTOBumpsChangeSeq(t *testing.T) {
	setupFlowConfig(t)
	before := ChangeSeq()
	if err := writeScenarioDTO(scenarioDTO{ID: "seq", Name: "n"}); err != nil {
		t.Fatalf("write: %v", err)
	}
	if ChangeSeq() != before+1 {
		t.Errorf("ChangeSeq = %d, want %d", ChangeSeq(), before+1)
	}
}

func TestScenarioToDTO(t *testing.T) {
	s := NewScenario()
	s.NameEd.SetText(" flow ")
	n := NewNode(KindDelay, 1, 2)
	s.Nodes = append(s.Nodes, n)
	s.Edges = append(s.Edges, NewEdge(s.Nodes[0].ID, n.ID))

	dto := s.toDTO()
	if dto.Name != "flow" {
		t.Errorf("name must be trimmed, got %q", dto.Name)
	}
	if len(dto.Nodes) != 2 || len(dto.Edges) != 1 {
		t.Errorf("expected 2 nodes / 1 edge, got %d / %d", len(dto.Nodes), len(dto.Edges))
	}
	if !strings.EqualFold(dto.Nodes[1].DelayMs, "1000") {
		t.Errorf("delay node default must be carried, got %q", dto.Nodes[1].DelayMs)
	}
}
