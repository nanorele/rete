package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/nanorele/gio/f32"
)

func TestWSSendModel(t *testing.T) {
	t.Run("dto roundtrip keeps ws fields and edge sides", func(t *testing.T) {
		s := &Scenario{ID: "s1"}
		wsNode := mkNode(KindWSRequest, 10, 20, func(n *Node) {
			n.KeepOpen = true
		})
		send := mkNode(KindWSSend, 300, 20, func(n *Node) {
			n.BodyEd.SetText("ping")
			n.WSClose = true
		})
		s.Nodes = append(s.Nodes, NewNode(KindStart, 0, 0), wsNode, send)
		e := NewEdge(wsNode.ID, send.ID)
		e.FromSide = SideBottom
		e.ToSide = SideTop
		s.Edges = append(s.Edges, e)

		enc, err := encodeScenario(s)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		got, err := decodeScenario(enc)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		gw := got.NodeByID(wsNode.ID)
		if gw == nil || !gw.KeepOpen {
			t.Error("KeepOpen must survive the roundtrip")
		}
		gs := got.NodeByID(send.ID)
		if gs == nil || gs.Kind != KindWSSend || !gs.WSClose {
			t.Errorf("WS send node lost fields: %+v", gs)
		}
		ge := got.EdgeByID(e.ID)
		if ge == nil || ge.FromSide != SideBottom || ge.ToSide != SideTop {
			t.Errorf("edge sides lost: %+v", ge)
		}
	})

	t.Run("buildPlan copies keepOpen and wsClose", func(t *testing.T) {
		s := &Scenario{ID: "s2"}
		wsNode := mkNode(KindWSRequest, 0, 0, func(n *Node) { n.KeepOpen = true })
		send := mkNode(KindWSSend, 0, 0, func(n *Node) { n.WSClose = true })
		s.Nodes = append(s.Nodes, wsNode, send)
		plan, _ := buildPlan(s, nil, nil, 100, 50)
		if !plan[wsNode.ID].keepOpen {
			t.Error("keepOpen not copied into the plan")
		}
		if !plan[send.ID].wsClose {
			t.Error("wsClose not copied into the plan")
		}
	})

	t.Run("validate warns when WS send has no keep-open socket", func(t *testing.T) {
		ed := newBlankEditor()
		addNodeTo(ed, KindStart, 0, 0)
		addNodeTo(ed, KindWSSend, 200, 0)
		warns := ed.validateScenario()
		found := false
		for _, w := range warns {
			if strings.Contains(w, "keep socket open") {
				found = true
			}
			if strings.Contains(w, "empty URL") {
				t.Errorf("WS send must not require a URL, got %q", w)
			}
		}
		if !found {
			t.Errorf("expected keep-open warning, got %v", warns)
		}
	})
}

func TestRunWSSend(t *testing.T) {
	url := startFlowEchoWS(t)

	t.Run("no open socket fails", func(t *testing.T) {
		n := &execNode{kind: KindWSSend, body: "hi", waitMs: 50}
		res := runWSSend(context.Background(), n, nil, nil)
		if !res.failed || !strings.Contains(res.errMsg, "no open WebSocket") {
			t.Fatalf("expected no-socket failure, got %+v", res)
		}
	})

	t.Run("keep-open socket accepts later sends", func(t *testing.T) {
		open := &execNode{kind: KindWSRequest, url: url, waitMs: 50, keepOpen: true}
		res, sock := runWSOpen(context.Background(), open, nil)
		if res.failed || sock == nil {
			t.Fatalf("open failed: %+v sock=%v", res, sock)
		}
		defer sock.close()

		send := &execNode{kind: KindWSSend, body: "hello-{{w}}", waitMs: 400, env: map[string]string{"w": "ws"}}
		out := runWSSend(context.Background(), send, nil, sock)
		if out.failed {
			t.Fatalf("send failed: %+v", out)
		}
		if string(out.body) != "hello-ws" {
			t.Errorf("echo body = %q", out.body)
		}
		if sock.closed {
			t.Error("socket must stay open without wsClose")
		}

		closing := &execNode{kind: KindWSSend, body: "bye", waitMs: 200, wsClose: true}
		out = runWSSend(context.Background(), closing, nil, sock)
		if out.failed || string(out.body) != "bye" {
			t.Fatalf("closing send failed: %+v body=%q", out, out.body)
		}
		if !sock.closed {
			t.Error("wsClose must close the socket")
		}

		res2 := runWSSend(context.Background(), &execNode{kind: KindWSSend, body: "x", waitMs: 50}, nil, sock)
		if !res2.failed {
			t.Error("send into a closed socket must fail")
		}
	})

	t.Run("without keepOpen no socket is returned", func(t *testing.T) {
		open := &execNode{kind: KindWSRequest, url: url, waitMs: 50}
		res, sock := runWSOpen(context.Background(), open, nil)
		if res.failed {
			t.Fatalf("open failed: %+v", res)
		}
		if sock != nil {
			t.Error("keepOpen=false must not return a live socket")
		}
	})
}

func TestRunnerWSSendScenario(t *testing.T) {
	url := startFlowEchoWS(t)
	s := NewScenario()
	start := s.Nodes[0]
	wsNode := mkNode(KindWSRequest, 200, 0, func(n *Node) {
		n.URLEd.SetText(url)
		n.KeepOpen = true
		n.WaitMsEd.SetText("50")
	})
	send := mkNode(KindWSSend, 400, 0, func(n *Node) {
		n.BodyEd.SetText("step-message")
		n.WaitMsEd.SetText("400")
	})
	s.Nodes = append(s.Nodes, wsNode, send)
	s.Edges = append(s.Edges, NewEdge(start.ID, wsNode.ID), NewEdge(wsNode.ID, send.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 176, 56)
	waitRunner(t, r)

	if st := r.NodeState(send.ID); st != StOK {
		t.Fatalf("WS send state = %d, want StOK (%s)", st, r.Status())
	}
	rec := r.LatestRun()
	if rec == nil {
		t.Fatal("no run record")
	}
	entries := r.Entries(rec)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if entries[1].Detail != "WS send" {
		t.Errorf("detail = %q", entries[1].Detail)
	}
	if entries[1].Body != "step-message" {
		t.Errorf("echo body = %q", entries[1].Body)
	}
}

func TestBlocksSaveInsert(t *testing.T) {
	setupFlowConfig(t)

	ed := newBlankEditor()
	start := addNodeTo(ed, KindStart, 0, 0)
	a := addNodeTo(ed, KindRequest, 100, 100)
	b := addNodeTo(ed, KindDelay, 400, 100)
	connect(ed, a, b)
	ed.selected = map[string]bool{start.ID: true, a.ID: true, b.ID: true}

	if !ed.saveSelectionAsBlock("My combo") {
		t.Fatalf("save failed: %s", ed.note)
	}
	blocks := ListBlocks()
	if len(blocks) != 1 || blocks[0].Name != "My combo" {
		t.Fatalf("ListBlocks = %+v", blocks)
	}
	dto, err := LoadBlock(blocks[0].ID)
	if err != nil {
		t.Fatalf("LoadBlock: %v", err)
	}
	if len(dto.Nodes) != 2 {
		t.Fatalf("block must keep 2 nodes (start excluded), got %d", len(dto.Nodes))
	}
	if len(dto.Edges) != 1 {
		t.Fatalf("block must keep the internal edge, got %d", len(dto.Edges))
	}

	before := len(ed.Scenario.Nodes)
	ed.insertBlockAt(dto, f32.Pt(1000, 1000))
	if got := len(ed.Scenario.Nodes) - before; got != 2 {
		t.Fatalf("insert added %d nodes, want 2", got)
	}
	if len(ed.Scenario.Edges) != 2 {
		t.Fatalf("insert must add the edge, got %d edges total", len(ed.Scenario.Edges))
	}
	if len(ed.selected) != 2 {
		t.Errorf("inserted nodes must be selected, got %d", len(ed.selected))
	}
	for _, nd := range dto.Nodes {
		for id := range ed.selected {
			if id == nd.ID {
				t.Error("inserted nodes must get fresh IDs")
			}
		}
	}

	if err := DeleteBlock(blocks[0].ID); err != nil {
		t.Fatalf("DeleteBlock: %v", err)
	}
	if left := ListBlocks(); len(left) != 0 {
		t.Errorf("blocks after delete = %+v", left)
	}
}

func TestFourPorts(t *testing.T) {
	t.Run("bottom port starts a connection with side", func(t *testing.T) {
		ed := newBlankEditor()
		n := addNodeTo(ed, KindRequest, 100, 100)
		ed.onPress(press(ed.toScreen(ed.outPortBottom(n))))
		if ed.connectFromID != n.ID || ed.connectFromSide != SideBottom {
			t.Fatalf("connectFromID=%q side=%q", ed.connectFromID, ed.connectFromSide)
		}
	})

	t.Run("release near the top port lands on it", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 100, 500)
		ed.onPress(press(ed.toScreen(ed.outPortBottom(a))))
		ed.onRelease(ed.toScreen(ed.inPortSide(b, SideTop)))
		if len(ed.Scenario.Edges) != 1 {
			t.Fatalf("edge not created")
		}
		e := ed.Scenario.Edges[0]
		if e.FromSide != SideBottom || e.ToSide != SideTop {
			t.Errorf("edge sides = %q → %q, want bottom → top", e.FromSide, e.ToSide)
		}
	})

	t.Run("vertical edge geometry uses vertical tangents", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 100, 500)
		e := connect(ed, a, b)
		e.FromSide = SideBottom
		e.ToSide = SideTop
		p0, p1, o0, o1 := ed.edgeGeom(e, a, b)
		if o0 != f32.Pt(0, 1) || o1 != f32.Pt(0, -1) {
			t.Errorf("tangents = %v %v", o0, o1)
		}
		if p0 != ed.outPortBottom(a) {
			t.Errorf("p0 = %v, want bottom port %v", p0, ed.outPortBottom(a))
		}
		if p1 != ed.inPortSide(b, SideTop) {
			t.Errorf("p1 = %v, want top port %v", p1, ed.inPortSide(b, SideTop))
		}
	})

	t.Run("legacy edges keep horizontal ports", func(t *testing.T) {
		ed := newBlankEditor()
		a := addNodeTo(ed, KindRequest, 100, 100)
		b := addNodeTo(ed, KindDelay, 600, 100)
		e := connect(ed, a, b)
		p0, p1, o0, o1 := ed.edgeGeom(e, a, b)
		if p0 != ed.outPort(a) || p1 != ed.inPort(b) {
			t.Errorf("legacy geometry moved: %v %v", p0, p1)
		}
		if o0 != f32.Pt(1, 0) || o1 != f32.Pt(-1, 0) {
			t.Errorf("legacy tangents = %v %v", o0, o1)
		}
	})
}
