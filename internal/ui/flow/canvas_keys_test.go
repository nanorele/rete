package flow

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"

	"rete/internal/ui/widgets"
)

type canvasRig struct {
	r     input.Router
	ed    *Editor
	host  *Host
	sz    image.Point
	th    *material.Theme
	clock time.Duration
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
	if rig.th == nil {
		rig.th = material.NewTheme()
		rig.th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	}
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         time.Now(),
	}
	rig.ed.Layout(gtx, rig.th, rig.host)
	rig.r.Frame(gtx.Ops)
}

func (rig *canvasRig) timedEvent(kind pointer.Kind, pt f32.Point, btn pointer.Buttons) pointer.Event {
	rig.clock += 700 * time.Millisecond
	return pointer.Event{Kind: kind, Position: pt, Source: pointer.Mouse, Buttons: btn, Time: rig.clock}
}

func (rig *canvasRig) timedClick(pt f32.Point) {
	rig.r.Queue(
		rig.timedEvent(pointer.Move, pt, 0),
		rig.timedEvent(pointer.Press, pt, pointer.ButtonPrimary),
		rig.timedEvent(pointer.Release, pt, 0),
	)
	rig.frame()
	rig.frame()
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
	canvasEd, _ := n.canvasBodyEditor()
	if !rig.r.Source().Focused(canvasEd) {
		t.Fatal("clicking the body box must focus the canvas body editor")
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
	canvasEd.SetCaret(canvasEd.Len(), canvasEd.Len())

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

func TestCanvasBodyCaretAndSelection(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	const body = "hello world this is body\nsecond line here"
	n.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	canvasEd, _ := n.canvasBodyEditor()
	if canvasEd.Text() != n.BodyEd.Text() {
		t.Fatalf("canvas editor = %q, want the node body", canvasEd.Text())
	}

	first := f32.Pt(float32(box.Min.X+8), float32(box.Min.Y+8))
	far := f32.Pt(float32(box.Min.X+90), float32(box.Min.Y+8))
	rig.timedClick(first)
	s0, _ := canvasEd.Selection()
	rig.timedClick(far)
	s1, e1 := canvasEd.Selection()
	if s1 <= s0 || s1 != e1 {
		t.Errorf("caret must move right on click without selecting: %d -> %d,%d", s0, s1, e1)
	}
	if ed.mode != modeProps {
		t.Fatal("clicking the body opens the properties panel, which lays out the node body too")
	}

	rig.r.Queue(
		rig.timedEvent(pointer.Move, first, 0),
		rig.timedEvent(pointer.Press, first, pointer.ButtonPrimary),
		rig.timedEvent(pointer.Move, far, pointer.ButtonPrimary),
		rig.timedEvent(pointer.Release, far, 0),
	)
	rig.frame()
	rig.frame()
	s2, e2 := canvasEd.Selection()
	if s2 == e2 {
		t.Error("dragging inside the body must select text")
	}

	rig.r.Queue(key.EditEvent{Text: "X"})
	rig.frame()
	rig.frame()
	if got := n.BodyEd.Text(); got != canvasEd.Text() || got == body {
		t.Errorf("typing on the canvas must replace the selection and reach the node body, got %q", got)
	}

	n.BodyEd.SetText("from panel")
	rig.frame()
	if canvasEd.Text() != "from panel" {
		t.Errorf("panel edits must reach the canvas editor, got %q", canvasEd.Text())
	}
}

func TestCanvasBodyClickDoesNotScroll(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	body := "{"
	for i := 0; i < 12; i++ {
		body += "\n  \"key" + itoa(i) + "\": \"value value value\","
	}
	body += "\n}"
	n.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	canvasEd, _ := n.canvasBodyEditor()
	if canvasEd.GetScrollY() != 0 {
		t.Fatalf("scrollY before = %d", canvasEd.GetScrollY())
	}
	box, _ := ed.bodyBoxRect(n)
	rig.timedClick(f32.Pt(float32(box.Min.X+30), float32(box.Max.Y-8)))
	if got := canvasEd.GetScrollY(); got != 0 {
		t.Errorf("clicking a partly visible line must not scroll the body, scrollY = %d", got)
	}
	if !rig.r.Source().Focused(canvasEd) {
		t.Error("the click must still focus the body editor")
	}
	s, e := canvasEd.Selection()
	if s == 0 || s != e {
		t.Errorf("the click must still place the caret, selection = %d,%d", s, e)
	}

	rig.r.Queue(key.Event{Name: key.NameDownArrow, State: key.Press}, key.Event{Name: key.NameDownArrow, State: key.Release})
	rig.frame()
	rig.frame()
	if canvasEd.GetScrollY() == 0 {
		t.Error("keyboard navigation below the visible area must still scroll to the caret")
	}
}

func (rig *canvasRig) chord(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press}, key.Event{Name: name, Modifiers: mods, State: key.Release})
	rig.frame()
	rig.frame()
}

func TestCanvasBodyEditorShortcuts(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	rig.timedClick(f32.Pt(float32(box.Min.X+10), float32(box.Min.Y+10)))
	canvasEd := n.CanvasBodyEditor()
	if !rig.r.Source().Focused(canvasEd) {
		t.Fatal("setup: body editor must be focused")
	}
	nodes := len(ed.Scenario.Nodes)

	rig.r.Queue(key.EditEvent{Text: "alpha beta"})
	rig.frame()
	rig.frame()
	rig.chord("Z", key.ModShortcut)
	if got := canvasEd.Text(); got != "" {
		t.Errorf("Ctrl+Z in the body must undo typing, got %q", got)
	}
	if got := n.BodyEd.Text(); got != "" {
		t.Errorf("the undo must reach the node body, got %q", got)
	}
	if len(ed.Scenario.Nodes) != nodes {
		t.Fatal("Ctrl+Z inside the body must not undo the scenario")
	}
	rig.chord("Y", key.ModShortcut)
	if got := canvasEd.Text(); got != "alpha beta" {
		t.Errorf("Ctrl+Y in the body must redo typing, got %q", got)
	}
	rig.chord("Z", key.ModShortcut)
	rig.chord("Z", key.ModShortcut|key.ModShift)
	if got := canvasEd.Text(); got != "alpha beta" {
		t.Errorf("Ctrl+Shift+Z in the body must redo typing, got %q", got)
	}

	canvasEd.SetCaret(canvasEd.Len(), canvasEd.Len())
	rig.chord(key.NameLeftArrow, key.ModShortcut)
	if s, _ := canvasEd.Selection(); s != len("alpha ") {
		t.Errorf("Ctrl+Left must jump a word, caret = %d", s)
	}
	rig.chord(key.NameLeftArrow, key.ModShortcut|key.ModShift)
	if s, e := canvasEd.Selection(); s != 0 || e != len("alpha ") {
		t.Errorf("Ctrl+Shift+Left must extend by a word, selection = %d,%d", s, e)
	}
	rig.chord("A", key.ModShortcut)
	if s, e := canvasEd.Selection(); s == e {
		t.Error("Ctrl+A in the body must select the text, not the nodes")
	}
	if len(ed.selected) != 1 {
		t.Errorf("Ctrl+A in the body must not select every node, selected = %d", len(ed.selected))
	}
	canvasEd.SetCaret(0, 0)
	rig.chord(key.NameTab, 0)
	if got := canvasEd.Text(); got != "\talpha beta" {
		t.Errorf("Tab must insert a tab, got %q", got)
	}
	rig.chord(key.NameDeleteBackward, key.ModShortcut)
	if got := canvasEd.Text(); got != "alpha beta" {
		t.Errorf("Ctrl+Backspace must delete a word, got %q", got)
	}

	rig.timedClick(f32.Pt(float32(box.Min.X+10), float32(box.Min.Y)-30))
	if !rig.r.Source().Focused(ed) {
		t.Fatal("clicking the header must focus the canvas")
	}
	ed.pushHistory()
	addNodeTo(ed, KindDelay, 600, 200)
	rig.chord("Z", key.ModShortcut)
	if len(ed.Scenario.Nodes) != nodes {
		t.Errorf("Ctrl+Z on the focused canvas must undo the scenario, nodes = %d", len(ed.Scenario.Nodes))
	}
	rig.chord("Y", key.ModShortcut)
	if len(ed.Scenario.Nodes) != nodes+1 {
		t.Errorf("Ctrl+Y on the focused canvas must redo the scenario, nodes = %d", len(ed.Scenario.Nodes))
	}
}

func TestCanvasWheelOverBodyNeverZooms(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	body := "{"
	for i := 0; i < 12; i++ {
		body += "\n  \"key" + itoa(i) + "\": \"value\","
	}
	body += "\n}"
	n.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	inBody := f32.Pt(float32(box.Min.X+20), float32(box.Min.Y+10))
	canvasEd := n.CanvasBodyEditor()
	scroll := func(pt f32.Point, dy float32) {
		rig.r.Queue(
			rig.timedEvent(pointer.Move, pt, 0),
			pointer.Event{Kind: pointer.Scroll, Position: pt, Source: pointer.Mouse, Scroll: f32.Pt(0, dy), Time: rig.clock},
		)
		rig.frame()
		rig.frame()
	}

	zoom0 := ed.zoom
	scroll(inBody, -40)
	if ed.zoom != zoom0 {
		t.Errorf("wheel up at the top of the body must not zoom the canvas, zoom %v -> %v", zoom0, ed.zoom)
	}
	scroll(inBody, 40)
	if ed.zoom != zoom0 {
		t.Errorf("wheel down over the body must not zoom the canvas, zoom %v -> %v", zoom0, ed.zoom)
	}
	if canvasEd.GetScrollY() == 0 {
		t.Error("wheel down over the body must scroll the text")
	}
	for i := 0; i < 20; i++ {
		scroll(inBody, 200)
	}
	if ed.zoom != zoom0 {
		t.Errorf("wheel down at the bottom of the body must not zoom the canvas, zoom %v -> %v", zoom0, ed.zoom)
	}

	scroll(f32.Pt(50, 50), 40)
	if ed.zoom == zoom0 {
		t.Error("wheel over empty canvas must still zoom")
	}
}

func TestCanvasZoomScalesBodyScroll(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	body := "{"
	for i := 0; i < 30; i++ {
		body += "\n  \"key" + itoa(i) + "\": \"value\","
	}
	body += "\n}"
	n.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	inBody := f32.Pt(float32(box.Min.X+20), float32(box.Min.Y+10))
	ce := n.CanvasBodyEditor()
	for i := 0; i < 5; i++ {
		rig.r.Queue(rig.timedEvent(pointer.Move, inBody, 0), pointer.Event{Kind: pointer.Scroll, Position: inBody, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
		rig.frame()
		rig.frame()
	}
	sy0 := ce.GetScrollY()
	if sy0 == 0 {
		t.Fatal("setup: the body must be scrolled")
	}
	frac := func() float32 {
		return float32(ce.GetScrollY()) / float32(ce.GetScrollBounds().Max.Y)
	}
	f0 := frac()
	ed.zoomByNotches(f32.Pt(50, 50), -3)
	rig.frame()
	if got := frac(); got < f0-0.03 || got > f0+0.03 || ce.GetScrollY() >= sy0 {
		t.Errorf("zoom out: body scroll fraction = %v (offset %d), want ~%v with a smaller offset than %d", got, ce.GetScrollY(), f0, sy0)
	}
	ed.zoomByNotches(f32.Pt(50, 50), 6)
	rig.frame()
	if got := frac(); got < f0-0.03 || got > f0+0.03 || ce.GetScrollY() <= sy0 {
		t.Errorf("zoom in: body scroll fraction = %v (offset %d), want ~%v with a larger offset than %d", got, ce.GetScrollY(), f0, sy0)
	}
	ed.resetZoom()
	rig.frame()
	if got := ce.GetScrollY(); got < sy0-2 || got > sy0+2 {
		t.Errorf("reset zoom: body scroll = %d, want ~%d", got, sy0)
	}

	for i := 0; i < 14; i++ {
		ed.zoomByNotches(f32.Pt(50, 50), -1)
		rig.frame()
	}
	if ed.zoom > 0.33 {
		t.Fatalf("setup: zoom = %v, want the minimum", ed.zoom)
	}
	if got := frac(); got < f0-0.05 || got > f0+0.05 {
		t.Errorf("minimum zoom: body scroll fraction = %v, want ~%v (the editor must stay live at 30%%)", got, f0)
	}
}

func TestCanvasPaletteBarAddsAndDropsNodes(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	rig.frame()
	rig.frame()
	if ed.paletteBar.Empty() || ed.paletteBar.Max.Y < rig.sz.Y-40 {
		t.Fatalf("palette bar must sit at the bottom of the canvas, got %v", ed.paletteBar)
	}
	if ed.fitBadge.Min.Y > 20 {
		t.Errorf("fit view badge must sit at the top, got %v", ed.fitBadge)
	}
	before := len(ed.Scenario.Nodes)

	r0 := ed.paletteRects[0]
	rig.timedClick(f32.Pt(float32(r0.Min.X+r0.Dx()/2), float32(r0.Min.Y+r0.Dy()/2)))
	if len(ed.Scenario.Nodes) != before+1 {
		t.Fatalf("clicking a palette button must add a node, nodes = %d", len(ed.Scenario.Nodes))
	}
	if got := ed.Scenario.Nodes[before].Kind; got != KindRequest {
		t.Errorf("first palette button must add an HTTP request, got %v", got)
	}

	r1 := ed.paletteRects[1]
	start := f32.Pt(float32(r1.Min.X+r1.Dx()/2), float32(r1.Min.Y+r1.Dy()/2))
	target := f32.Pt(500, 250)
	widgets.GlobalPointerPos = start
	rig.r.Queue(rig.timedEvent(pointer.Move, start, 0), rig.timedEvent(pointer.Press, start, pointer.ButtonPrimary))
	rig.frame()
	widgets.GlobalPointerPos = target
	rig.r.Queue(rig.timedEvent(pointer.Move, target, pointer.ButtonPrimary))
	rig.frame()
	if !ed.palDragActive {
		t.Fatal("dragging a palette button over the canvas must arm the drop")
	}
	rig.r.Queue(rig.timedEvent(pointer.Release, target, 0))
	rig.frame()
	rig.frame()
	if len(ed.Scenario.Nodes) != before+2 {
		t.Fatalf("releasing over the canvas must drop a node, nodes = %d", len(ed.Scenario.Nodes))
	}
	n := ed.Scenario.Nodes[before+1]
	if n.Kind != KindWSRequest {
		t.Errorf("second palette button must drop a WebSocket node, got %v", n.Kind)
	}
	sp, w, h := ed.nodeScreenRect(n)
	if target.X < sp.X || target.X > sp.X+w || target.Y < sp.Y || target.Y > sp.Y+h {
		t.Errorf("dropped node %v..%v must be under the pointer %v", sp, sp.Add(f32.Pt(w, h)), target)
	}
}
