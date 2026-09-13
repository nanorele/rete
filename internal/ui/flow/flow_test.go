package flow

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"github.com/nanorele/gio/app"
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
	"image"
	"image/color"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/settings"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"
	"rete/internal/ws"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

func TestCanvasMarqueeSelectsWhileDragging(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindDelay, 300, 300)
	rig.frame()
	rig.frame()
	start := f32.Pt(100, 100)
	rig.r.Queue(rig.timedEvent(pointer.Move, start, 0), rig.timedEvent(pointer.Press, start, pointer.ButtonPrimary))
	rig.frame()
	if !ed.marquee {
		t.Fatal("pressing empty canvas must start a marquee")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, f32.Pt(200, 200), pointer.ButtonPrimary))
	rig.frame()
	if ed.selected[n.ID] {
		t.Error("a marquee that does not reach the node must not select it")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, f32.Pt(340, 330), pointer.ButtonPrimary))
	rig.frame()
	if !ed.selected[n.ID] {
		t.Error("the node must be selected as soon as the marquee reaches it, before the button is released")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, f32.Pt(150, 150), pointer.ButtonPrimary))
	rig.frame()
	if ed.selected[n.ID] {
		t.Error("shrinking the marquee away from the node must deselect it live")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, f32.Pt(340, 330), pointer.ButtonPrimary), rig.timedEvent(pointer.Release, f32.Pt(340, 330), 0))
	rig.frame()
	if !ed.selected[n.ID] || ed.marquee {
		t.Error("release must keep the live selection and end the marquee")
	}
}

func TestCanvasWarningTooltipOnHover(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	n.URLEd.SetText("http://{{nope}}/x")
	rig.frame()
	rig.frame()
	if n.warnRect.Empty() {
		t.Fatal("a node with missing variables must show the warning icon")
	}
	c := f32.Pt(float32(n.warnRect.Min.X+n.warnRect.Dx()/2), float32(n.warnRect.Min.Y+n.warnRect.Dy()/2))
	rig.r.Queue(rig.timedEvent(pointer.Move, c, 0))
	rig.frame()
	rig.frame()
	if ed.warnTipNode != n.ID {
		t.Errorf("hovering the warning icon must show its tooltip, got %q", ed.warnTipNode)
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, f32.Pt(50, 50), 0))
	rig.frame()
	rig.frame()
	if ed.warnTipNode != "" {
		t.Error("moving away must hide the tooltip")
	}
}

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
	if w != ed.nodeW*2 || h != (ed.nodeH+bodyBoxH(ed.nodeH))*2 {
		t.Errorf("size = (%v,%v), want the zoomed node size with its body box", w, h)
	}

	c0, c1 := ed.envChipRect(n)
	if c1.Y >= sp.Y {
		t.Error("the env chip must sit above the node")
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
		if ed.mode != modeProps || ed.note != "" {
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

func TestScenarioViewPersists(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.zoom = 0.75
	ed.pan = f32.Pt(-120, 40)
	ed.SaveScenario()
	id := ed.Scenario.ID

	s, err := LoadScenario(id)
	if err != nil {
		t.Fatal(err)
	}
	if s.View == nil || s.View.Zoom != 0.75 || s.View.PanX != -120 || s.View.PanY != 40 {
		t.Fatalf("saved view = %+v, want zoom 0.75 pan (-120,40)", s.View)
	}
	if ed.encode() == "" || ed.encode() != ed.lastSaved {
		t.Error("the view must not make the scenario dirty")
	}

	t.Run("opening restores the view instead of fitting", func(t *testing.T) {
		other := newTestEditor()
		other.pendingFit = true
		if !other.OpenScenario(id) {
			t.Fatal("open must succeed")
		}
		if other.zoom != 0.75 || other.pan != f32.Pt(-120, 40) {
			t.Errorf("view = zoom %v pan %v, want the saved one", other.zoom, other.pan)
		}
		if other.pendingFit {
			t.Error("a saved view must suppress the fit")
		}
	})

	t.Run("a pending fit never overwrites the saved view", func(t *testing.T) {
		other := newTestEditor()
		other.Scenario, _ = LoadScenario(id)
		other.pendingFit = true
		other.zoom, other.pan = 1, f32.Point{}
		other.FlushView()
		s, _ := LoadScenario(id)
		if s.View == nil || s.View.Zoom != 0.75 {
			t.Errorf("view = %+v, want the saved 0.75 zoom", s.View)
		}
	})

	t.Run("flush writes only a changed view", func(t *testing.T) {
		ed.zoom = 1.5
		if !ed.viewDirty() {
			t.Fatal("a zoom change must mark the view dirty")
		}
		ed.FlushView()
		s, _ := LoadScenario(id)
		if s.View == nil || s.View.Zoom != 1.5 {
			t.Errorf("view = %+v, want zoom 1.5", s.View)
		}
		if ed.viewDirty() {
			t.Error("flush must clear the dirty view")
		}
	})

	t.Run("switching scenarios flushes the old view", func(t *testing.T) {
		second := NewScenario()
		if err := second.Save(); err != nil {
			t.Fatal(err)
		}
		ed.zoom = 0.5
		if !ed.OpenScenario(second.ID) {
			t.Fatal("open must succeed")
		}
		s, _ := LoadScenario(id)
		if s.View == nil || s.View.Zoom != 0.5 {
			t.Errorf("old scenario view = %+v, want zoom 0.5", s.View)
		}
		if !ed.pendingFit {
			t.Error("a scenario without a view must fit")
		}
	})
}

type parityHit struct {
	method  string
	header  http.Header
	body    string
	close   bool
	length  int64
	chunked bool
}

func parityServer(t *testing.T, respond func(w http.ResponseWriter)) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		last.Store(parityHit{method: r.Method, header: r.Header.Clone(), body: string(data), close: r.Close, length: r.ContentLength, chunked: len(r.TransferEncoding) > 0})
		if respond != nil {
			respond(w)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func withSettings(t *testing.T, apply func()) {
	t.Helper()
	ua, dh, ae, cc := settings.UserAgent, settings.DefaultHeaders, settings.AcceptEncoding, settings.SendConnClose
	af, sc, tw, ji := settings.AutoFormatJSONRequest, settings.StripJSONComments, settings.TrimTrailingWS, settings.JSONIndent
	t.Cleanup(func() {
		settings.UserAgent, settings.DefaultHeaders, settings.AcceptEncoding, settings.SendConnClose = ua, dh, ae, cc
		settings.AutoFormatJSONRequest, settings.StripJSONComments, settings.TrimTrailingWS, settings.JSONIndent = af, sc, tw, ji
	})
	apply()
}

func TestRunHTTPSystemHeadersMatchTab(t *testing.T) {
	srv, last := parityServer(t, nil)

	t.Run("user agent, accept-encoding and default headers", func(t *testing.T) {
		withSettings(t, func() {
			settings.UserAgent = "rete-test/1.0"
			settings.AcceptEncoding = "gzip, br"
			settings.DefaultHeaders = []model.DefaultHeader{{Key: "X-Team", Value: "{{team}}"}, {Key: "X-Set", Value: "default"}}
			settings.SendConnClose = true
		})
		n := &execNode{
			method:  "GET",
			url:     srv.URL,
			env:     map[string]string{"team": "core"},
			headers: [][2]string{{"X-Set", "explicit"}},
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if got := h.header.Get("User-Agent"); got != "rete-test/1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := h.header.Get("Accept-Encoding"); got != "gzip, br" {
			t.Errorf("Accept-Encoding = %q", got)
		}
		if got := h.header.Get("X-Team"); got != "core" {
			t.Errorf("default header must expand variables, got %q", got)
		}
		if got := h.header.Get("X-Set"); got != "explicit" {
			t.Errorf("explicit header must beat the default header, got %q", got)
		}
		if got := h.header.Get("Connection"); got != "close" || !h.close {
			t.Errorf("Connection = %q close=%v, want close", got, h.close)
		}
		if got := h.header.Get("Content-Type"); got != "text/plain" {
			t.Errorf("raw body without content sends text/plain like the tab, got %q", got)
		}
	})

	t.Run("explicit user agent wins", func(t *testing.T) {
		withSettings(t, func() { settings.UserAgent = "rete-test/1.0" })
		n := &execNode{method: "GET", url: srv.URL, headers: [][2]string{{"User-Agent", "mine"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("User-Agent"); got != "mine" {
			t.Errorf("User-Agent = %q, want the explicit one", got)
		}
	})

	t.Run("empty user agent falls back to the app default", func(t *testing.T) {
		withSettings(t, func() { settings.UserAgent = "" })
		n := &execNode{method: "GET", url: srv.URL}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("User-Agent"); got != model.DefaultSettings().UserAgent {
			t.Errorf("User-Agent = %q, want the default", got)
		}
	})
}

func TestRunHTTPRawBodyMatchesTab(t *testing.T) {
	srv, last := parityServer(t, nil)

	t.Run("json body gets application/json", func(t *testing.T) {
		withSettings(t, func() {
			settings.StripJSONComments = true
			settings.TrimTrailingWS = true
			settings.AutoFormatJSONRequest = false
		})
		n := &execNode{method: "POST", url: srv.URL, body: "  {\"a\": 1, // note\n \"b\": \"{{v}}\"}  \t\n", env: map[string]string{"v": "x"}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if got := h.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if strings.Contains(h.body, "//") || strings.HasSuffix(h.body, " ") || strings.HasSuffix(h.body, "\t") {
			t.Errorf("comments and trailing whitespace must be stripped, got %q", h.body)
		}
		if !strings.Contains(h.body, `"b": "x"`) {
			t.Errorf("variables must expand, got %q", h.body)
		}
	})

	t.Run("auto format json request", func(t *testing.T) {
		withSettings(t, func() {
			settings.AutoFormatJSONRequest = true
			settings.JSONIndent = 4
		})
		n := &execNode{method: "PUT", url: srv.URL, body: `{"a":{"b":1}}`}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if h.method != "PUT" || h.body != "{\n    \"a\": {\n        \"b\": 1\n    }\n}" {
			t.Errorf("method=%q body=%q", h.method, h.body)
		}
	})

	t.Run("explicit content type is kept for raw bodies", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, body: "<a/>", headers: [][2]string{{"Content-Type", "application/xml"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("Content-Type"); got != "application/xml" {
			t.Errorf("Content-Type = %q", got)
		}
	})

	t.Run("form body content type overrides a manual header", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, bodyType: "urlencoded", body: "a=1", headers: [][2]string{{"Content-Type", "text/plain"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
	})

	t.Run("every method is sent as typed", func(t *testing.T) {
		for _, m := range methods {
			n := &execNode{method: m, url: srv.URL, body: "{}"}
			if res := runHTTP(context.Background(), n, nil); res.failed {
				t.Fatalf("%s failed: %+v", m, res)
			}
			if got := last.Load().(parityHit).method; got != m {
				t.Errorf("method %s arrived as %q", m, got)
			}
		}
		n := &execNode{url: srv.URL}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).method; got != "GET" {
			t.Errorf("empty method must default to GET, got %q", got)
		}
	})

	t.Run("url is sanitized like the tab", func(t *testing.T) {
		n := &execNode{method: "GET", url: "\t" + srv.URL + "/a b\n"}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
	})
}

func TestRunHTTPDecompressesResponses(t *testing.T) {
	srv, _ := parityServer(t, func(w http.ResponseWriter) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte(`{"items":[1,2,3]}`))
		_ = gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf.Bytes())
	})
	withSettings(t, func() { settings.AcceptEncoding = "gzip" })
	n := &execNode{method: "GET", url: srv.URL}
	res := runHTTP(context.Background(), n, nil)
	if res.failed {
		t.Fatalf("failed: %+v", res)
	}
	if string(res.body) != `{"items":[1,2,3]}` {
		t.Fatalf("body must be decompressed, got %q", res.body)
	}
	if !evalCond(execEdge{cond: CondArrayCount, value: "items", op: "==", count: 3}, &res, nil, nil) {
		t.Error("conditions must see the decoded body")
	}
}

func TestRunGQLSystemHeaders(t *testing.T) {
	srv, last := parityServer(t, nil)
	withSettings(t, func() { settings.UserAgent = "rete-test/1.0" })
	n := &execNode{url: srv.URL, body: "query { me { id } }"}
	if res := runGQL(context.Background(), n, nil); res.failed {
		t.Fatalf("failed: %+v", res)
	}
	h := last.Load().(parityHit)
	if h.method != "POST" || h.header.Get("Content-Type") != "application/json" || h.header.Get("User-Agent") != "rete-test/1.0" {
		t.Errorf("method=%q ct=%q ua=%q", h.method, h.header.Get("Content-Type"), h.header.Get("User-Agent"))
	}
}

func TestRunHTTPContentTypeAndLengthWithoutHeaders(t *testing.T) {
	srv, last := parityServer(t, nil)
	withSettings(t, func() {
		settings.AutoFormatJSONRequest = false
		settings.TrimTrailingWS = false
		settings.StripJSONComments = false
	})
	cases := []struct {
		name     string
		node     *execNode
		wantCT   string
		wantBody string
	}{
		{"raw json", &execNode{method: "POST", url: srv.URL, body: `{"a":1}`}, "application/json", `{"a":1}`},
		{"raw text", &execNode{method: "POST", url: srv.URL, body: "hello world"}, "text/plain", "hello world"},
		{"urlencoded", &execNode{method: "POST", url: srv.URL, bodyType: "urlencoded", body: "a=1\nb=two"}, "application/x-www-form-urlencoded", "a=1&b=two"},
		{"ws-style put", &execNode{method: "PUT", url: srv.URL, body: "[1,2,3]"}, "application/json", "[1,2,3]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runHTTP(context.Background(), tc.node, nil)
			if res.failed {
				t.Fatalf("failed: %+v", res)
			}
			h := last.Load().(parityHit)
			if got := h.header.Get("Content-Type"); got != tc.wantCT {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantCT)
			}
			if h.body != tc.wantBody {
				t.Fatalf("body = %q, want %q", h.body, tc.wantBody)
			}
			if h.chunked {
				t.Error("the body must be sent with a Content-Length, not chunked")
			}
			if h.length != int64(len(tc.wantBody)) {
				t.Errorf("Content-Length = %d, want %d", h.length, len(tc.wantBody))
			}
			if got := h.header.Get("Content-Length"); got != strconv.Itoa(len(tc.wantBody)) {
				t.Errorf("Content-Length header = %q, want %d", got, len(tc.wantBody))
			}
		})
	}

	t.Run("form data", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, bodyType: "form", body: "field=value"}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if !strings.HasPrefix(h.header.Get("Content-Type"), "multipart/form-data; boundary=") {
			t.Errorf("Content-Type = %q", h.header.Get("Content-Type"))
		}
		if h.chunked || h.length != int64(len(h.body)) || h.length == 0 {
			t.Errorf("multipart body must carry its Content-Length, got %d (chunked=%v) for %d bytes", h.length, h.chunked, len(h.body))
		}
	})

	t.Run("graphql", func(t *testing.T) {
		n := &execNode{kind: KindGQLRequest, url: srv.URL, body: "query { me { id } }"}
		res := runGQL(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if h.header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q", h.header.Get("Content-Type"))
		}
		if h.chunked || h.length != int64(len(h.body)) || h.length == 0 {
			t.Errorf("GraphQL body must carry its Content-Length, got %d (chunked=%v) for %d bytes", h.length, h.chunked, len(h.body))
		}
	})
}

func TestRunGQL(t *testing.T) {
	var gotBody, gotCT, gotMethod atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		gotBody.Store(string(data))
		gotCT.Store(r.Header.Get("Content-Type"))
		gotMethod.Store(r.Method)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer srv.Close()

	t.Run("query with variables", func(t *testing.T) {
		n := &execNode{
			kind:    KindGQLRequest,
			url:     srv.URL,
			body:    "query($id: Int) { user(id: $id) { name } }",
			gqlVars: `{"id": {{uid}}}`,
			env:     map[string]string{"uid": "7"},
		}
		res := runGQL(context.Background(), n, nil)
		if !res.hasResp || res.status != 200 || res.failed {
			t.Fatalf("unexpected result: %+v", res)
		}
		if gotMethod.Load() != "POST" {
			t.Errorf("method = %v, want POST", gotMethod.Load())
		}
		if gotCT.Load() != "application/json" {
			t.Errorf("content-type = %v", gotCT.Load())
		}
		var payload struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.Unmarshal([]byte(gotBody.Load().(string)), &payload); err != nil {
			t.Fatalf("payload not JSON: %v", err)
		}
		if !strings.Contains(payload.Query, "user(id: $id)") {
			t.Errorf("query = %q", payload.Query)
		}
		if string(payload.Variables) != `{"id":7}` {
			t.Errorf("variables = %s", payload.Variables)
		}
	})

	t.Run("invalid variables fail", func(t *testing.T) {
		n := &execNode{kind: KindGQLRequest, url: srv.URL, body: "query { x }", gqlVars: "{oops"}
		res := runGQL(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "invalid JSON") {
			t.Fatalf("expected invalid-JSON failure, got %+v", res)
		}
	})
}

func startFlowEchoWS(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					_ = c.Close()
					return
				}
				res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{Subprotocols: []string{"echo-proto"}})
				if err != nil {
					_ = c.Close()
					return
				}
				conn := res.Conn
				for {
					op, payload, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if op == ws.OpText || op == ws.OpBinary {
						_ = conn.WriteMessage(op, payload)
					}
				}
			}(c)
		}
	}()
	return "ws://" + l.Addr().String()
}

func TestRunWS(t *testing.T) {
	url := startFlowEchoWS(t)

	t.Run("send and collect echo", func(t *testing.T) {
		n := &execNode{
			kind:      KindWSRequest,
			url:       url,
			body:      `{"msg":"{{word}}"}`,
			wsOpcode:  "TEXT",
			waitMs:    400,
			subprotos: []string{"echo-proto"},
			env:       map[string]string{"word": "hi"},
		}
		res := runWS(context.Background(), n, nil)
		if !res.hasResp || res.failed {
			t.Fatalf("unexpected result: %+v (%s)", res, res.errMsg)
		}
		if res.status != 101 {
			t.Errorf("status = %d, want 101", res.status)
		}
		if string(res.body) != `{"msg":"hi"}` {
			t.Errorf("collected body = %q", res.body)
		}
	})

	t.Run("binary opcode expects hex", func(t *testing.T) {
		n := &execNode{kind: KindWSRequest, url: url, body: "zz-not-hex", wsOpcode: "BIN", waitMs: 100}
		res := runWS(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "hex payload") {
			t.Fatalf("expected hex failure, got %+v", res)
		}
	})

	t.Run("refused connection fails", func(t *testing.T) {
		n := &execNode{kind: KindWSRequest, url: "ws://127.0.0.1:1", waitMs: 50}
		res := runWS(context.Background(), n, nil)
		if !res.failed {
			t.Fatalf("expected failure, got %+v", res)
		}
	})
}

func TestRunHTTPBodyTypesAuthCookies(t *testing.T) {
	type seen struct {
		ct     string
		body   string
		auth   string
		cookie string
	}
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		last.Store(seen{
			ct:     r.Header.Get("Content-Type"),
			body:   string(data),
			auth:   r.Header.Get("Authorization"),
			cookie: r.Header.Get("Cookie"),
		})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	t.Run("urlencoded", func(t *testing.T) {
		n := &execNode{
			method:   "POST",
			url:      srv.URL,
			bodyType: "urlencoded",
			body:     "a=1\nb=two words\n\n=skipped",
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", s.ct)
		}
		if !strings.Contains(s.body, "a=1") || !strings.Contains(s.body, "b=two+words") {
			t.Errorf("body = %q", s.body)
		}
	})

	t.Run("form with file", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "part.txt")
		if err := os.WriteFile(fp, []byte("FILEDATA"), 0o644); err != nil {
			t.Fatal(err)
		}
		n := &execNode{
			method:   "POST",
			url:      srv.URL,
			bodyType: "form",
			body:     "field=val\nupload=@" + fp,
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		mt, params, err := mime.ParseMediaType(s.ct)
		if err != nil || mt != "multipart/form-data" {
			t.Fatalf("content-type = %q (%v)", s.ct, err)
		}
		mr := multipart.NewReader(strings.NewReader(s.body), params["boundary"])
		got := map[string]string{}
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(p)
			got[p.FormName()] = string(data)
		}
		if got["field"] != "val" || got["upload"] != "FILEDATA" {
			t.Errorf("parts = %v", got)
		}
	})

	t.Run("binary", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "raw.bin")
		if err := os.WriteFile(fp, []byte{1, 2, 3}, 0o644); err != nil {
			t.Fatal(err)
		}
		n := &execNode{method: "POST", url: srv.URL, bodyType: "binary", binPath: fp}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.ct != "application/octet-stream" || s.body != "\x01\x02\x03" {
			t.Errorf("ct=%q body=%q", s.ct, s.body)
		}
	})

	t.Run("missing binary file fails", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, bodyType: "binary", binPath: "no/such/file.bin"}
		res := runHTTP(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "binary body") {
			t.Fatalf("expected binary failure, got %+v", res)
		}
	})

	t.Run("bearer auth and cookies", func(t *testing.T) {
		n := &execNode{
			method:    "GET",
			url:       srv.URL,
			authType:  "bearer",
			authToken: "{{tok}}",
			cookies:   [][2]string{{"sid", "abc"}, {"theme", "dark"}},
			env:       map[string]string{"tok": "T123"},
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.auth != "Bearer T123" {
			t.Errorf("auth = %q", s.auth)
		}
		if s.cookie != "sid=abc; theme=dark" {
			t.Errorf("cookie = %q", s.cookie)
		}
	})

	t.Run("basic auth overrides an explicit header like the HTTP tab", func(t *testing.T) {
		n := &execNode{
			method:   "GET",
			url:      srv.URL,
			headers:  [][2]string{{"Authorization", "custom"}},
			authType: "basic",
			authUser: "u",
			authPass: "p",
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if s := last.Load().(seen); s.auth != "Basic dTpw" {
			t.Errorf("auth = %q, configured auth must win", s.auth)
		}
	})
}

func TestNodeDTONewFieldsRoundTrip(t *testing.T) {
	n := NewNode(KindWSRequest, 10, 20)
	n.URLEd.SetText("wss://x/y")
	n.HeadersEd.SetText("Origin: http://x")
	n.BodyEd.SetText("ping")
	n.SubprotosEd.SetText("a, b")
	n.WSOpcode = "BIN"
	n.WaitMsEd.SetText("250")
	n.InsecureTLS = true
	n.AuthType = "bearer"
	n.AuthTokenEd.SetText("tok")
	n.CookiesEd.SetText("k=v")

	back := nodeFromDTO(nodeToDTO(n))
	if back.Kind != KindWSRequest {
		t.Fatalf("kind = %v", back.Kind)
	}
	if back.SubprotosEd.Text() != "a, b" || back.WSOpcode != "BIN" || back.WaitMsEd.Text() != "250" {
		t.Errorf("ws fields lost: %q %q %q", back.SubprotosEd.Text(), back.WSOpcode, back.WaitMsEd.Text())
	}
	if !back.InsecureTLS || back.AuthType != "bearer" || back.AuthTokenEd.Text() != "tok" || back.CookiesEd.Text() != "k=v" {
		t.Errorf("auth/cookie fields lost")
	}

	g := NewNode(KindGQLRequest, 0, 0)
	g.BodyEd.SetText("query { x }")
	g.VarsEd.SetText(`{"a":1}`)
	gb := nodeFromDTO(nodeToDTO(g))
	if gb.Kind != KindGQLRequest || gb.VarsEd.Text() != `{"a":1}` {
		t.Errorf("gql fields lost: kind=%v vars=%q", gb.Kind, gb.VarsEd.Text())
	}

	h := NewNode(KindRequest, 0, 0)
	h.BodyType = "form"
	h.BinPathEd.SetText("c:/f.bin")
	hb := nodeFromDTO(nodeToDTO(h))
	if hb.BodyType != "form" || hb.BinPathEd.Text() != "c:/f.bin" {
		t.Errorf("http fields lost: %q %q", hb.BodyType, hb.BinPathEd.Text())
	}
}

func TestAddRequestNodeFromTab(t *testing.T) {
	setupFlowConfig(t)
	ed := NewEditor()

	n := ed.AddRequestNode(TabRequest{
		Name:      "My WS",
		Kind:      KindWSRequest,
		URL:       "wss://srv/sock",
		Headers:   [][2]string{{"Origin", "http://x"}},
		Cookies:   [][2]string{{"sid", "1"}},
		AuthType:  "bearer",
		AuthToken: "tok",
		Subprotos: []string{"p1", "p2"},
		WSMessage: "hello",
		WSOpcode:  "TEXT",
	})
	if n.Kind != KindWSRequest || n.URLEd.Text() != "wss://srv/sock" {
		t.Fatalf("node = %v %q", n.Kind, n.URLEd.Text())
	}
	if n.SubprotosEd.Text() != "p1, p2" || n.BodyEd.Text() != "hello" {
		t.Errorf("ws data lost: %q %q", n.SubprotosEd.Text(), n.BodyEd.Text())
	}
	if n.HeadersEd.Text() != "Origin: http://x" || n.CookiesEd.Text() != "sid=1" || n.AuthType != "bearer" {
		t.Errorf("headers/cookies/auth lost")
	}
	if ed.Scenario.NodeByID(n.ID) == nil {
		t.Error("node not attached to the scenario")
	}

	h := ed.AddRequestNode(TabRequest{
		Name:      "Form req",
		Kind:      KindRequest,
		Method:    "POST",
		URL:       "http://x/upload",
		BodyType:  "form",
		FormParts: []TabFormPart{{Key: "f", Value: "v"}, {Key: "file", IsFile: true, FilePath: "c:/a.png"}},
	})
	if h.BodyType != "form" || h.BodyEd.Text() != "f=v\nfile=@c:/a.png" {
		t.Errorf("form body = %q (%q)", h.BodyEd.Text(), h.BodyType)
	}

	g := ed.AddRequestNode(TabRequest{
		Kind:     KindGQLRequest,
		URL:      "http://x/graphql",
		GQLQuery: "query { me }",
		GQLVars:  `{"a":2}`,
	})
	if g.BodyEd.Text() != "query { me }" || g.VarsEd.Text() != `{"a":2}` {
		t.Errorf("gql node = %q %q", g.BodyEd.Text(), g.VarsEd.Text())
	}
	if len(ed.Scenario.Nodes) < 4 {
		t.Errorf("scenario has %d nodes", len(ed.Scenario.Nodes))
	}
}

func testWindow() *app.Window { return &app.Window{} }

func waitRunner(t *testing.T, r *Runner) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !r.Running() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("runner did not finish within 10s")
}

func respResult(status int, body string) *stepResult {
	return &stepResult{hasResp: true, status: status, body: []byte(body)}
}

func TestParseEditorInt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{"plain", "42", 7, 42},
		{"padded", "  8  ", 7, 8},
		{"negative", "-3", 7, -3},
		{"empty uses default", "", 7, 7},
		{"garbage uses default", "abc", 7, 7},
		{"float uses default", "1.5", 7, 7},
		{"zero", "0", 7, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseEditorInt(tt.in, tt.def); got != tt.want {
				t.Errorf("parseEditorInt(%q, %d) = %d, want %d", tt.in, tt.def, got, tt.want)
			}
		})
	}
}

func TestExpandVars(t *testing.T) {
	env := map[string]string{"host": "example.test", "shared": "from-env"}
	vars := map[string]string{"tok": "abc", "shared": "from-vars"}
	tests := []struct {
		name  string
		input string
		env   map[string]string
		vars  map[string]string
		want  string
	}{
		{"no placeholders", "http://plain", env, vars, "http://plain"},
		{"env lookup", "http://{{host}}/x", env, vars, "http://example.test/x"},
		{"vars lookup", "Bearer {{tok}}", env, vars, "Bearer abc"},
		{"vars win over env", "{{shared}}", env, vars, "from-vars"},
		{"unknown stays literal", "{{nope}}", env, vars, "{{nope}}"},
		{"multiple", "{{host}}/{{tok}}", env, vars, "example.test/abc"},
		{"spaces inside braces", "{{ tok }}", env, vars, "abc"},
		{"unterminated stays literal", "a{{tok", env, vars, "a{{tok"},
		{"empty name stays literal", "{{}}", env, vars, "{{}}"},
		{"nil maps pass through", "{{tok}}", nil, nil, "{{tok}}"},
		{"env only", "{{host}}", env, nil, "example.test"},
		{"adjacent placeholders", "{{tok}}{{tok}}", nil, vars, "abcabc"},
		{"text around", "x{{tok}}y", nil, vars, "xabcy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandVars(tt.input, tt.env, tt.vars); got != tt.want {
				t.Errorf("expandVars(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMatchStatus(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		status  int
		want    bool
	}{
		{"empty defaults to 2xx match", "", 204, true},
		{"empty defaults to 2xx miss", "", 404, false},
		{"exact", "404", 404, true},
		{"exact miss", "404", 400, false},
		{"wildcard x", "2xx", 201, true},
		{"wildcard star", "4**", 418, true},
		{"uppercase pattern", "2XX", 200, true},
		{"trimmed", "  200  ", 200, true},
		{"length mismatch", "2xx", 99, false},
		{"partial wildcard miss", "20x", 301, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchStatus(tt.pattern, tt.status); got != tt.want {
				t.Errorf("matchStatus(%q, %d) = %v, want %v", tt.pattern, tt.status, got, tt.want)
			}
		})
	}
}

func TestCompareInt(t *testing.T) {
	tests := []struct {
		name string
		a    int
		op   string
		b    int
		want bool
	}{
		{"gt true", 3, ">", 2, true},
		{"gt false", 2, ">", 3, false},
		{"gte", 2, ">=", 2, true},
		{"lt", 1, "<", 2, true},
		{"lte", 2, "<=", 2, true},
		{"ne", 1, "!=", 2, true},
		{"eq via default", 2, "==", 2, true},
		{"unknown op falls back to eq", 2, "???", 2, true},
		{"unknown op falls back to eq false", 2, "???", 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareInt(tt.a, tt.op, tt.b); got != tt.want {
				t.Errorf("compareInt(%d,%q,%d) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseFloats(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		ok   bool
	}{
		{"both numeric", "1.5", " 2 ", true},
		{"left not numeric", "x", "2", false},
		{"right not numeric", "1", "y", false},
		{"neither numeric", "x", "y", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, ok := parseFloats(tt.a, tt.b); ok != tt.ok {
				t.Errorf("parseFloats(%q,%q) ok = %v, want %v", tt.a, tt.b, ok, tt.ok)
			}
		})
	}
}

func TestCompareValues(t *testing.T) {
	tests := []struct {
		name string
		a    string
		op   string
		b    string
		want bool
	}{
		{"contains", "hello world", "contains", "lo w", true},
		{"contains miss", "hello", "contains", "zz", false},
		{"eq strings", "ok", "==", "ok", true},
		{"eq empty op", "ok", "", "ok", true},
		{"eq numeric equivalence", "1.0", "==", "1", true},
		{"eq strings miss", "ok", "==", "no", false},
		{"ne strings", "ok", "!=", "no", true},
		{"ne numeric", "2", "!=", "2.0", false},
		{"gt numeric", "3", ">", "2", true},
		{"gt non numeric is false", "b", ">", "a", false},
		{"gte", "2", ">=", "2", true},
		{"lt", "1", "<", "2", true},
		{"lte", "2", "<=", "2", true},
		{"unknown op numeric is false", "2", "~=", "2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := compareValues(tt.a, tt.op, tt.b); got != tt.want {
				t.Errorf("compareValues(%q,%q,%q) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestStringifyJSON(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want string
	}{
		{"string is raw", "abc", "abc"},
		{"nil is empty", nil, ""},
		{"number", float64(3), "3"},
		{"bool", true, "true"},
		{"array", []interface{}{1.0, 2.0}, "[1,2]"},
		{"object", map[string]interface{}{"a": "b"}, `{"a":"b"}`},
		{"unmarshalable", make(chan int), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stringifyJSON(tt.in); got != tt.want {
				t.Errorf("stringifyJSON(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJSONPath(t *testing.T) {
	body := `{"data":{"id":7,"tags":["a","b"]},"list":[{"n":1}],"nil":null}`
	tests := []struct {
		name string
		res  *stepResult
		path string
		want string
		ok   bool
	}{
		{"nested object", respResult(200, body), "data.id", "7", true},
		{"array index", respResult(200, body), "data.tags.1", "b", true},
		{"array of objects", respResult(200, body), "list.0.n", "1", true},
		{"empty path returns root", respResult(200, `{"a":1}`), "", `{"a":1}`, true},
		{"padded path", respResult(200, body), "  data.id  ", "7", true},
		{"missing key", respResult(200, body), "data.nope", "", false},
		{"index out of range", respResult(200, body), "data.tags.9", "", false},
		{"negative index", respResult(200, body), "data.tags.-1", "", false},
		{"index on object", respResult(200, body), "data.tags.x", "", false},
		{"descend into scalar", respResult(200, body), "data.id.more", "", false},
		{"no response", &stepResult{}, "a", "", false},
		{"empty body", &stepResult{hasResp: true}, "a", "", false},
		{"invalid json", respResult(200, "not json"), "a", "", false},
		{"json null root", respResult(200, "null"), "a", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := jsonPath(tt.res, tt.path)
			if ok != tt.ok {
				t.Fatalf("jsonPath ok = %v, want %v", ok, tt.ok)
			}
			if ok && stringifyJSON(v) != tt.want {
				t.Errorf("jsonPath value = %q, want %q", stringifyJSON(v), tt.want)
			}
		})
	}
}

func TestJSONPathParsesBodyOnce(t *testing.T) {
	res := respResult(200, `{"a":1}`)
	if _, ok := jsonPath(res, "a"); !ok {
		t.Fatal("first lookup must succeed")
	}
	if !res.jsonParsed {
		t.Error("jsonParsed must be latched after the first lookup")
	}
	res.body = []byte(`{"b":2}`)
	if _, ok := jsonPath(res, "b"); ok {
		t.Error("body is only parsed once; a later body swap must not be re-read")
	}
}

func TestEvalCond(t *testing.T) {
	body := `{"ok":true,"name":"bob","items":[1,2,3],"count":5}`
	env := map[string]string{"field": "name"}
	vars := map[string]string{"want": "bob"}
	tests := []struct {
		name string
		edge execEdge
		res  *stepResult
		want bool
	}{
		{"always", execEdge{cond: CondAlways}, nil, true},
		{"always with nil result", execEdge{cond: CondAlways}, nil, true},
		{"status match", execEdge{cond: CondStatus, value: "2xx"}, respResult(201, body), true},
		{"status miss", execEdge{cond: CondStatus, value: "2xx"}, respResult(500, body), false},
		{"status without response", execEdge{cond: CondStatus, value: "2xx"}, &stepResult{}, false},
		{"has response", execEdge{cond: CondHasResponse}, respResult(200, body), true},
		{"has response false", execEdge{cond: CondHasResponse}, &stepResult{}, false},
		{"no response", execEdge{cond: CondNoResponse}, &stepResult{}, true},
		{"no response false", execEdge{cond: CondNoResponse}, respResult(200, body), false},
		{"body field present", execEdge{cond: CondBodyField, value: "name"}, respResult(200, body), true},
		{"body field absent", execEdge{cond: CondBodyField, value: "missing"}, respResult(200, body), false},
		{"body field via env var", execEdge{cond: CondBodyField, value: "{{field}}"}, respResult(200, body), true},
		{"array count gt", execEdge{cond: CondArrayCount, value: "items", op: ">", count: 2}, respResult(200, body), true},
		{"array count lt", execEdge{cond: CondArrayCount, value: "items", op: "<", count: 2}, respResult(200, body), false},
		{"array count on non array", execEdge{cond: CondArrayCount, value: "count", op: ">", count: 0}, respResult(200, body), false},
		{"array count missing path", execEdge{cond: CondArrayCount, value: "gone", op: ">", count: 0}, respResult(200, body), false},
		{"body value eq", execEdge{cond: CondBodyValue, value: "name", op: "==", value2: "bob"}, respResult(200, body), true},
		{"body value expanded", execEdge{cond: CondBodyValue, value: "name", op: "==", value2: "{{want}}"}, respResult(200, body), true},
		{"body value numeric", execEdge{cond: CondBodyValue, value: "count", op: ">", value2: "3"}, respResult(200, body), true},
		{"body value missing path", execEdge{cond: CondBodyValue, value: "gone", op: "==", value2: "x"}, respResult(200, body), false},
		{"unknown cond", execEdge{cond: CondKind(99)}, respResult(200, body), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evalCond(tt.edge, tt.res, env, vars); got != tt.want {
				t.Errorf("evalCond = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDescribeNetErr(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{"cancelled context", cancelled, errors.New("whatever"), "cancelled"},
		{"plain error", context.Background(), errors.New("boom"), "boom"},
		{
			"url error unwrapped",
			context.Background(),
			&url.Error{Op: "Get", URL: "http://x", Err: errors.New("dial fail")},
			"dial fail",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeNetErr(tt.ctx, tt.err); got != tt.want {
				t.Errorf("describeNetErr = %q, want %q", got, tt.want)
			}
		})
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string { return "deadline" }
func (timeoutErr) Timeout() bool { return true }

func TestDescribeNetErrTimeout(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "http://x", Err: timeoutErr{}}
	if got := describeNetErr(context.Background(), err); got != "timeout: deadline" {
		t.Errorf("describeNetErr = %q, want %q", got, "timeout: deadline")
	}
}

func TestBuildPlan(t *testing.T) {
	t.Run("maps node fields and finds the start", func(t *testing.T) {
		s := &Scenario{}
		start := NewNode(KindStart, 0, 0)
		req := NewNode(KindRequest, 100, 0)
		req.URLEd.SetText("  http://x  ")
		req.HeadersEd.SetText("A: 1\nbad line\n : novalue\nB:2")
		req.BodyEd.SetText("payload")
		s.Nodes = append(s.Nodes, start, req)
		s.Edges = append(s.Edges, NewEdge(start.ID, req.ID))

		plan, startID := buildPlan(s, nil, nil, 100, 50)
		if startID != start.ID {
			t.Fatalf("startID = %q, want %q", startID, start.ID)
		}
		en := plan[req.ID]
		if en.url != "http://x" {
			t.Errorf("url must be trimmed, got %q", en.url)
		}
		if en.body != "payload" {
			t.Errorf("body = %q", en.body)
		}
		if len(en.headers) != 2 || en.headers[0] != [2]string{"A", "1"} || en.headers[1] != [2]string{"B", "2"} {
			t.Errorf("headers = %v, want A:1 and B:2 only", en.headers)
		}
		if len(plan[start.ID].outs) != 1 || plan[start.ID].outs[0].to != req.ID {
			t.Errorf("start must have one outgoing edge, got %+v", plan[start.ID].outs)
		}
	})

	t.Run("count and delay parsing", func(t *testing.T) {
		s := &Scenario{}
		loop := NewNode(KindLoop, 0, 0)
		loop.CountEd.SetText("4")
		loop.DelayEd.SetText("250")
		zero := NewNode(KindLoop, 0, 0)
		zero.CountEd.SetText("0")
		zero.DelayEd.SetText("-5")
		s.Nodes = append(s.Nodes, NewNode(KindStart, 0, 0), loop, zero)

		plan, _ := buildPlan(s, nil, nil, 100, 50)
		if plan[loop.ID].count != 4 || plan[loop.ID].delay != 250*time.Millisecond {
			t.Errorf("count/delay = %d/%v, want 4/250ms", plan[loop.ID].count, plan[loop.ID].delay)
		}
		if plan[zero.ID].count != 1 {
			t.Errorf("non-positive count must clamp to 1, got %d", plan[zero.ID].count)
		}
		if plan[zero.ID].delay != 0 {
			t.Errorf("negative delay must stay 0, got %v", plan[zero.ID].delay)
		}
	})

	t.Run("per node env overrides the active env", func(t *testing.T) {
		s := &Scenario{}
		start := NewNode(KindStart, 0, 0)
		a := NewNode(KindRequest, 0, 0)
		b := NewNode(KindRequest, 0, 0)
		b.EnvID = "custom"
		c := NewNode(KindRequest, 0, 0)
		c.EnvID = "unknown"
		s.Nodes = append(s.Nodes, start, a, b, c)

		active := map[string]string{"k": "active"}
		lookup := func(id string) map[string]string {
			if id == "custom" {
				return map[string]string{"k": "custom"}
			}
			return nil
		}
		plan, _ := buildPlan(s, active, lookup, 100, 50)
		if plan[a.ID].env["k"] != "active" {
			t.Errorf("node without EnvID must use the active env, got %v", plan[a.ID].env)
		}
		if plan[b.ID].env["k"] != "custom" {
			t.Errorf("node with EnvID must use its own env, got %v", plan[b.ID].env)
		}
		if plan[c.ID].env["k"] != "active" {
			t.Errorf("unresolvable EnvID must fall back to the active env, got %v", plan[c.ID].env)
		}
	})

	t.Run("drops edges with unknown endpoints", func(t *testing.T) {
		s := &Scenario{}
		start := NewNode(KindStart, 0, 0)
		s.Nodes = append(s.Nodes, start)
		s.Edges = append(s.Edges, NewEdge(start.ID, "ghost"), NewEdge("ghost", start.ID))
		plan, _ := buildPlan(s, nil, nil, 100, 50)
		if len(plan[start.ID].outs) != 0 {
			t.Errorf("edges to unknown nodes must be dropped, got %+v", plan[start.ID].outs)
		}
	})

	t.Run("no start node", func(t *testing.T) {
		s := &Scenario{Nodes: []*Node{NewNode(KindRequest, 0, 0)}}
		if _, startID := buildPlan(s, nil, nil, 100, 50); startID != "" {
			t.Errorf("startID = %q, want empty", startID)
		}
	})

	t.Run("loop entries sorted top-down and exclude internally linked nodes", func(t *testing.T) {
		s := &Scenario{}
		start := NewNode(KindStart, -500, 0)
		loop := NewNode(KindLoop, 0, 0)
		loop.W, loop.H = 400, 400
		lower := NewNode(KindRequest, 10, 300)
		upper := NewNode(KindRequest, 10, 100)
		chained := NewNode(KindRequest, 200, 200)
		outside := NewNode(KindRequest, 5000, 5000)
		s.Nodes = append(s.Nodes, start, loop, lower, upper, chained, outside)
		s.Edges = append(s.Edges, NewEdge(upper.ID, chained.ID))

		plan, _ := buildPlan(s, nil, nil, 100, 50)
		got := plan[loop.ID].entries
		if len(got) != 2 {
			t.Fatalf("expected 2 loop entries, got %d (%v)", len(got), got)
		}
		if got[0] != upper.ID || got[1] != lower.ID {
			t.Errorf("entries must be ordered by Y then X, got %v", got)
		}
	})
}

func TestRunHTTP(t *testing.T) {
	var gotMethod, gotHeader, gotBody, gotPath atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		if r.ContentLength > 0 {
			_, _ = r.Body.Read(body)
		}
		gotMethod.Store(r.Method)
		gotHeader.Store(r.Header.Get("X-Token"))
		gotBody.Store(string(body))
		gotPath.Store(r.URL.Path)
		switch r.URL.Path {
		case "/fail":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		default:
			w.Header().Set("X-Reply", "yes")
			_, _ = w.Write([]byte(`{"ok":true}`))
		}
	}))
	defer srv.Close()

	t.Run("successful request", func(t *testing.T) {
		n := &execNode{
			method:  "POST",
			url:     srv.URL + "/ok",
			body:    "hello {{who}}",
			headers: [][2]string{{"X-Token", "{{tok}}"}, {"", "skipped"}},
			env:     map[string]string{"who": "world"},
		}
		res := runHTTP(context.Background(), n, map[string]string{"tok": "secret"})
		if !res.hasResp || res.status != 200 || res.failed {
			t.Fatalf("unexpected result: %+v", res)
		}
		if string(res.body) != `{"ok":true}` {
			t.Errorf("body = %q", res.body)
		}
		if res.headers.Get("X-Reply") != "yes" {
			t.Errorf("response headers not captured: %v", res.headers)
		}
		if gotMethod.Load() != "POST" {
			t.Errorf("server saw method %v", gotMethod.Load())
		}
		if gotHeader.Load() != "secret" {
			t.Errorf("header must be expanded from vars, server saw %v", gotHeader.Load())
		}
		if gotBody.Load() != "hello world" {
			t.Errorf("body must be expanded from env, server saw %v", gotBody.Load())
		}
	})

	t.Run("http error status is a failure", func(t *testing.T) {
		n := &execNode{method: "GET", url: srv.URL + "/fail"}
		res := runHTTP(context.Background(), n, nil)
		if !res.hasResp || res.status != 500 || !res.failed {
			t.Fatalf("5xx must be marked failed: %+v", res)
		}
	})

	t.Run("scheme is added when missing", func(t *testing.T) {
		n := &execNode{method: "GET", url: strings.TrimPrefix(srv.URL, "http://") + "/plain"}
		res := runHTTP(context.Background(), n, nil)
		if !res.hasResp {
			t.Fatalf("expected a response, got %+v", res)
		}
		if gotPath.Load() != "/plain" {
			t.Errorf("server saw path %v", gotPath.Load())
		}
	})

	t.Run("spaces are percent encoded", func(t *testing.T) {
		n := &execNode{method: "GET", url: srv.URL + "/a b"}
		res := runHTTP(context.Background(), n, nil)
		if !res.hasResp {
			t.Fatalf("expected a response, got %+v", res)
		}
		if gotPath.Load() != "/a b" {
			t.Errorf("server saw path %v, want %q", gotPath.Load(), "/a b")
		}
	})

	errTests := []struct {
		name string
		node *execNode
		want string
	}{
		{"empty url", &execNode{method: "GET"}, "empty URL"},
		{"whitespace url", &execNode{method: "GET", url: "   "}, "empty URL"},
		{
			"unresolved variable",
			&execNode{method: "GET", url: "http://{{host}}/x"},
			"unresolved variable in URL: http://{{host}}/x",
		},
		{"invalid method", &execNode{method: "BAD METHOD", url: "http://x"}, "invalid request: "},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			res := runHTTP(context.Background(), tt.node, nil)
			if !res.failed || res.hasResp {
				t.Fatalf("expected a failed result, got %+v", res)
			}
			if !strings.HasPrefix(res.errMsg, tt.want) {
				t.Errorf("errMsg = %q, want prefix %q", res.errMsg, tt.want)
			}
		})
	}

	t.Run("transport error", func(t *testing.T) {
		closed := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		addr := closed.URL
		closed.Close()
		res := runHTTP(context.Background(), &execNode{method: "GET", url: addr}, nil)
		if !res.failed || res.errMsg == "" {
			t.Errorf("expected a transport failure, got %+v", res)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := runHTTP(ctx, &execNode{method: "GET", url: srv.URL}, nil)
		if !res.failed || res.errMsg != "cancelled" {
			t.Errorf("expected cancelled, got %+v", res)
		}
	})
}

func TestRunnerStateAccessors(t *testing.T) {
	r := NewRunner()
	if r.Running() || r.Paused() || r.StepMode() || r.Status() != "" {
		t.Error("a fresh runner must be idle")
	}
	if r.NodeState("x") != StIdle || r.EdgeState("x") != StIdle || r.NodeInfo("x") != "" {
		t.Error("unknown ids must report idle/empty")
	}
	if r.LatestRun() != nil {
		t.Error("no runs yet")
	}

	r.setNode("n", StOK)
	r.setNodeInfo("n", "200")
	r.setEdge("e", StFail)
	if r.NodeState("n") != StOK || r.NodeInfo("n") != "200" || r.EdgeState("e") != StFail {
		t.Error("setters must be observable through the accessors")
	}

	r.Reset()
	if r.NodeState("n") != StIdle || r.NodeInfo("n") != "" || r.EdgeState("e") != StIdle {
		t.Error("Reset must clear the node and edge state")
	}
}

func TestRunnerRunsSnapshotIsACopy(t *testing.T) {
	r := NewRunner()
	rec := &RunRecord{Label: "Run 1"}
	r.runs = append(r.runs, rec)
	ent := &RunEntry{Node: "a"}
	r.addEntry(rec, ent)

	runs := r.Runs()
	if len(runs) != 1 || runs[0] != rec {
		t.Fatalf("Runs must return the records, got %+v", runs)
	}
	runs[0] = nil
	if r.Runs()[0] != rec {
		t.Error("Runs must return a copy of the slice")
	}
	if r.LatestRun() != rec {
		t.Error("LatestRun must return the last record")
	}
	entries := r.Entries(rec)
	if len(entries) != 1 || entries[0] != ent {
		t.Fatalf("Entries mismatch: %+v", entries)
	}
	entries[0] = nil
	if r.Entries(rec)[0] != ent {
		t.Error("Entries must return a copy of the slice")
	}
}

func TestRunnerStopWithoutRun(t *testing.T) {
	NewRunner().Stop()
	NewRunner().Step()
	NewRunner().SetStepMode(false)
}

func TestRunnerSetStepMode(t *testing.T) {
	r := NewRunner()
	r.SetStepMode(true)
	if !r.StepMode() {
		t.Error("step mode must be on")
	}
	r.stepCh = make(chan struct{}, 1)
	r.SetStepMode(false)
	if r.StepMode() {
		t.Error("step mode must be off")
	}
	select {
	case <-r.stepCh:
	default:
		t.Error("turning step mode off must release a waiting step")
	}

	r.Step()
	select {
	case <-r.stepCh:
	default:
		t.Error("Step must signal the channel")
	}
}

func TestRunnerStartRejectsBadScenarios(t *testing.T) {
	tests := []struct {
		name       string
		scenario   func() *Scenario
		wantStatus string
	}{
		{
			"no start node",
			func() *Scenario { return &Scenario{Nodes: []*Node{NewNode(KindRequest, 0, 0)}} },
			"No start node",
		},
		{
			"start without outgoing arrows",
			func() *Scenario { return &Scenario{Nodes: []*Node{NewNode(KindStart, 0, 0)}} },
			"Start node has no outgoing arrows",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRunner()
			r.Start(context.Background(), testWindow(), tt.scenario(), nil, nil, 100, 50)
			if r.Running() {
				t.Error("runner must not start")
			}
			if r.Status() != tt.wantStatus {
				t.Errorf("status = %q, want %q", r.Status(), tt.wantStatus)
			}
		})
	}
}

func TestRunnerStartIgnoredWhileRunning(t *testing.T) {
	r := NewRunner()
	r.mu.Lock()
	r.running = true
	r.status = "Running..."
	r.mu.Unlock()

	s := &Scenario{Nodes: []*Node{NewNode(KindStart, 0, 0)}}
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	if r.Status() != "Running..." {
		t.Errorf("a second Start must be ignored, status = %q", r.Status())
	}
}

func TestRunnerRunsRequestScenario(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"t-42","items":[1,2]}`))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	req := NewNode(KindRequest, 200, 0)
	req.NameEd.SetText("Fetch")
	req.URLEd.SetText(srv.URL)
	s.Nodes = append(s.Nodes, start, req)
	s.Edges = append(s.Edges, NewEdge(start.ID, req.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.NodeState(req.ID) != StOK {
		t.Errorf("request node state = %d, want StOK", r.NodeState(req.ID))
	}
	if r.NodeInfo(req.ID) != "200" {
		t.Errorf("node info = %q, want %q", r.NodeInfo(req.ID), "200")
	}
	if r.EdgeState(s.Edges[0].ID) != StOK {
		t.Error("edge must be marked taken")
	}
	if got := r.Status(); got != "Finished · 1 ok · 0 failed" {
		t.Errorf("status = %q", got)
	}

	rec := r.LatestRun()
	if rec == nil || !rec.Done || rec.Failed || rec.Stopped {
		t.Fatalf("run record wrong: %+v", rec)
	}
	if rec.Seq != 1 || !strings.HasPrefix(rec.Label, "Run 1 · ") {
		t.Errorf("record label = %q seq = %d", rec.Label, rec.Seq)
	}
	entries := r.Entries(rec)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	ent := entries[0]
	if ent.Node != "Fetch" || ent.Code != 200 || !ent.OK {
		t.Errorf("entry = %+v", ent)
	}
	if ent.Status != "200 OK" {
		t.Errorf("entry status = %q, want %q", ent.Status, "200 OK")
	}
	if ent.Body != `{"token":"t-42","items":[1,2]}` {
		t.Errorf("entry body = %q", ent.Body)
	}
	if ent.BodyLen != len(ent.Body) {
		t.Errorf("BodyLen = %d, want %d", ent.BodyLen, len(ent.Body))
	}
	if !strings.HasPrefix(ent.Detail, "GET "+srv.URL) {
		t.Errorf("entry detail = %q", ent.Detail)
	}
}

func TestRunnerRecordsFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	req := NewNode(KindRequest, 200, 0)
	req.URLEd.SetText(srv.URL)
	s.Nodes = append(s.Nodes, start, req)
	s.Edges = append(s.Edges, NewEdge(start.ID, req.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.NodeState(req.ID) != StFail {
		t.Errorf("failed request node state = %d, want StFail", r.NodeState(req.ID))
	}
	if got := r.Status(); got != "Finished with errors · 0 ok · 1 failed" {
		t.Errorf("status = %q", got)
	}
	if rec := r.LatestRun(); rec == nil || !rec.Failed {
		t.Error("run record must be marked failed")
	}
}

func TestRunnerRequestWithoutURLFails(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	req := NewNode(KindRequest, 200, 0)
	s.Nodes = append(s.Nodes, start, req)
	s.Edges = append(s.Edges, NewEdge(start.ID, req.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.NodeInfo(req.ID) != "ERR: empty URL" {
		t.Errorf("node info = %q, want %q", r.NodeInfo(req.ID), "ERR: empty URL")
	}
	entries := r.Entries(r.LatestRun())
	if len(entries) != 1 || entries[0].Status != "empty URL" {
		t.Fatalf("entry status wrong: %+v", entries)
	}
}

func TestRunnerSetVarNodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Trace", "trace-1")
		_, _ = w.Write([]byte(`{"token":"t-42"}`))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	req := NewNode(KindRequest, 200, 0)
	req.URLEd.SetText(srv.URL)

	fromPath := NewNode(KindSetVar, 400, 0)
	fromPath.VarNameEd.SetText("tok")
	fromPath.VarValueEd.SetText("$.token")

	fromHeader := NewNode(KindSetVar, 600, 0)
	fromHeader.VarNameEd.SetText("trace")
	fromHeader.VarValueEd.SetText("$header.X-Trace")

	fromStatus := NewNode(KindSetVar, 800, 0)
	fromStatus.VarNameEd.SetText("code")
	fromStatus.VarValueEd.SetText("$status")

	literal := NewNode(KindSetVar, 1000, 0)
	literal.VarNameEd.SetText("greet")
	literal.VarValueEd.SetText("hi {{tok}}")

	missing := NewNode(KindSetVar, 1200, 0)
	missing.VarValueEd.SetText("x")

	badPath := NewNode(KindSetVar, 1400, 0)
	badPath.VarNameEd.SetText("nope")
	badPath.VarValueEd.SetText("$.absent")

	chain := []*Node{start, req, fromPath, fromHeader, fromStatus, literal, missing, badPath}
	s.Nodes = append(s.Nodes, chain...)
	for i := 0; i < len(chain)-1; i++ {
		s.Edges = append(s.Edges, NewEdge(chain[i].ID, chain[i+1].ID))
	}

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	tests := []struct {
		node *Node
		want string
	}{
		{fromPath, "tok set"},
		{fromHeader, "trace set"},
		{fromStatus, "code set"},
		{literal, "greet set"},
		{missing, "no variable name"},
		{badPath, "path not found: $.absent"},
	}
	for _, tt := range tests {
		if got := r.NodeInfo(tt.node.ID); got != tt.want {
			t.Errorf("node %s info = %q, want %q", tt.node.DisplayName(), got, tt.want)
		}
	}
}

func TestRunnerSetVarWithoutResponse(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	header := NewNode(KindSetVar, 200, 0)
	header.VarNameEd.SetText("h")
	header.VarValueEd.SetText("$header.X-Any")
	status := NewNode(KindSetVar, 400, 0)
	status.VarNameEd.SetText("c")
	status.VarValueEd.SetText("$status")

	chain := []*Node{start, header, status}
	s.Nodes = append(s.Nodes, chain...)
	for i := 0; i < len(chain)-1; i++ {
		s.Edges = append(s.Edges, NewEdge(chain[i].ID, chain[i+1].ID))
	}

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if got := r.NodeInfo(header.ID); got != "no response for $header.X-Any" {
		t.Errorf("header info = %q", got)
	}
	if got := r.NodeInfo(status.ID); got != "no response for $status" {
		t.Errorf("status info = %q", got)
	}
}

func TestRunnerConditionRouting(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	req := NewNode(KindRequest, 200, 0)
	req.URLEd.SetText(srv.URL)
	taken := NewNode(KindDelay, 400, 0)
	taken.DelayEd.SetText("0")
	skipped := NewNode(KindDelay, 400, 200)
	skipped.DelayEd.SetText("0")
	s.Nodes = append(s.Nodes, start, req, taken, skipped)

	e0 := NewEdge(start.ID, req.ID)
	pass := NewEdge(req.ID, taken.ID)
	pass.Cond = CondStatus
	pass.ValueEd.SetText("2xx")
	fail := NewEdge(req.ID, skipped.ID)
	fail.Cond = CondStatus
	fail.ValueEd.SetText("5xx")
	s.Edges = append(s.Edges, e0, pass, fail)

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.EdgeState(pass.ID) != StOK {
		t.Error("matching condition edge must be marked OK")
	}
	if r.EdgeState(fail.ID) != StFail {
		t.Error("non-matching condition edge must be marked failed")
	}
	if r.NodeState(taken.ID) != StOK {
		t.Error("node behind the passing edge must run")
	}
	if r.NodeState(skipped.ID) != StIdle {
		t.Errorf("node behind the failing edge must stay idle, got %d", r.NodeState(skipped.ID))
	}
}

func TestRunnerSkipsNoteNodes(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	note := NewNode(KindNote, 200, 0)
	s.Nodes = append(s.Nodes, start, note)
	s.Edges = append(s.Edges, NewEdge(start.ID, note.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.NodeState(note.ID) != StIdle {
		t.Errorf("note nodes must never execute, state = %d", r.NodeState(note.ID))
	}
}

func TestRunnerLoopByCount(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, -600, 0)
	loop := NewNode(KindLoop, 0, 0)
	loop.W, loop.H = 400, 400
	loop.CountEd.SetText("3")
	loop.DelayEd.SetText("0")
	inner := NewNode(KindRequest, 100, 200)
	inner.URLEd.SetText(srv.URL)
	s.Nodes = append(s.Nodes, start, loop, inner)
	s.Edges = append(s.Edges, NewEdge(start.ID, loop.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if got := hits.Load(); got != 3 {
		t.Errorf("inner request ran %d times, want 3", got)
	}
	if got := r.NodeInfo(loop.ID); got != "done ×3" {
		t.Errorf("loop info = %q, want %q", got, "done ×3")
	}
	if got := r.Status(); got != "Finished · 3 ok · 0 failed" {
		t.Errorf("status = %q", got)
	}
}

func TestRunnerLoopOverArray(t *testing.T) {
	var seen []string
	var mu atomicStrings
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/src" {
			_, _ = w.Write([]byte(`{"items":[{"id":"a"},{"id":"b"}]}`))
			return
		}
		mu.add(r.URL.Path)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, -900, 0)
	src := NewNode(KindRequest, -600, 0)
	src.URLEd.SetText(srv.URL + "/src")
	loop := NewNode(KindLoop, 0, 0)
	loop.W, loop.H = 400, 400
	loop.LoopSrcEd.SetText("$.items")
	inner := NewNode(KindRequest, 100, 200)
	inner.URLEd.SetText(srv.URL + "/item/{{loop.item.id}}/{{loop.index}}")
	s.Nodes = append(s.Nodes, start, src, loop, inner)
	s.Edges = append(s.Edges, NewEdge(start.ID, src.ID), NewEdge(src.ID, loop.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	seen = mu.all()
	if len(seen) != 2 {
		t.Fatalf("expected 2 loop iterations, got %d (%v)", len(seen), seen)
	}
	if seen[0] != "/item/a/0" || seen[1] != "/item/b/1" {
		t.Errorf("loop.item / loop.index not expanded per iteration: %v", seen)
	}
	if got := r.NodeInfo(loop.ID); got != "done ×2" {
		t.Errorf("loop info = %q, want %q", got, "done ×2")
	}
}

func TestRunnerLoopMissingArray(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"other":1}`))
	}))
	defer srv.Close()

	s := &Scenario{}
	start := NewNode(KindStart, -900, 0)
	src := NewNode(KindRequest, -600, 0)
	src.URLEd.SetText(srv.URL)
	loop := NewNode(KindLoop, 0, 0)
	loop.W, loop.H = 400, 400
	loop.LoopSrcEd.SetText("$.items")
	inner := NewNode(KindRequest, 100, 200)
	inner.URLEd.SetText(srv.URL)
	s.Nodes = append(s.Nodes, start, src, loop, inner)
	s.Edges = append(s.Edges, NewEdge(start.ID, src.ID), NewEdge(src.ID, loop.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if r.NodeState(loop.ID) != StFail {
		t.Errorf("loop over a missing array must fail, state = %d", r.NodeState(loop.ID))
	}
	if got := r.NodeInfo(loop.ID); got != "no array at $.items" {
		t.Errorf("loop info = %q", got)
	}
	if !strings.HasPrefix(r.Status(), "Finished with errors") {
		t.Errorf("status = %q", r.Status())
	}
}

func TestRunnerDelayNode(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	delay := NewNode(KindDelay, 200, 0)
	delay.DelayEd.SetText("10")
	s.Nodes = append(s.Nodes, start, delay)
	s.Edges = append(s.Edges, NewEdge(start.ID, delay.ID))

	r := NewRunner()
	began := time.Now()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if time.Since(began) < 10*time.Millisecond {
		t.Error("delay node must actually wait")
	}
	if r.NodeState(delay.ID) != StOK {
		t.Errorf("delay node state = %d, want StOK", r.NodeState(delay.ID))
	}
}

func TestRunnerStopCancelsRun(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	delay := NewNode(KindDelay, 200, 0)
	delay.DelayEd.SetText("30000")
	s.Nodes = append(s.Nodes, start, delay)
	s.Edges = append(s.Edges, NewEdge(start.ID, delay.ID))

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	deadline := time.Now().Add(5 * time.Second)
	for r.NodeState(delay.ID) != StRunning && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	r.Stop()
	waitRunner(t, r)

	if !strings.HasPrefix(r.Status(), "Stopped") {
		t.Errorf("status = %q, want a Stopped prefix", r.Status())
	}
	rec := r.LatestRun()
	if rec == nil || !rec.Stopped || !rec.Done {
		t.Errorf("run record must be marked stopped: %+v", rec)
	}
}

func TestRunnerStepModePausesAndResumes(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	delay := NewNode(KindDelay, 200, 0)
	delay.DelayEd.SetText("0")
	s.Nodes = append(s.Nodes, start, delay)
	s.Edges = append(s.Edges, NewEdge(start.ID, delay.ID))

	r := NewRunner()
	r.SetStepMode(true)
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)

	deadline := time.Now().Add(5 * time.Second)
	for !r.Paused() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !r.Paused() {
		t.Fatal("step mode must pause before the first non-start node")
	}
	if !strings.HasPrefix(r.Status(), "Paused · ") {
		t.Errorf("status = %q, want a Paused prefix", r.Status())
	}

	r.Step()
	waitRunner(t, r)
	if r.Paused() {
		t.Error("runner must not stay paused after finishing")
	}
	if r.NodeState(delay.ID) != StOK {
		t.Errorf("stepped node state = %d, want StOK", r.NodeState(delay.ID))
	}
}

func TestRunnerStepLimit(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	a := NewNode(KindDelay, 200, 0)
	a.DelayEd.SetText("0")
	b := NewNode(KindDelay, 400, 0)
	b.DelayEd.SetText("0")
	s.Nodes = append(s.Nodes, start, a, b)
	s.Edges = append(s.Edges,
		NewEdge(start.ID, a.ID),
		NewEdge(a.ID, b.ID),
		NewEdge(b.ID, a.ID),
	)

	r := NewRunner()
	r.Start(context.Background(), testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)

	if !strings.HasPrefix(r.Status(), "Stopped: step limit reached") {
		t.Errorf("a cyclic scenario must hit the step limit, status = %q", r.Status())
	}
	if rec := r.LatestRun(); rec == nil || !rec.Failed {
		t.Error("run record must be marked failed after hitting the limit")
	}
}

func TestRunnerHistoryIsCapped(t *testing.T) {
	r := NewRunner()
	for i := 0; i < maxHistoryRuns+3; i++ {
		r.runs = append(r.runs, &RunRecord{Seq: i})
		if len(r.runs) > maxHistoryRuns {
			r.runs = r.runs[1:]
		}
	}
	if len(r.runs) != maxHistoryRuns {
		t.Errorf("history length = %d, want %d", len(r.runs), maxHistoryRuns)
	}
}

func TestRunnerNilParentContext(t *testing.T) {
	s := &Scenario{}
	start := NewNode(KindStart, 0, 0)
	delay := NewNode(KindDelay, 200, 0)
	delay.DelayEd.SetText("0")
	s.Nodes = append(s.Nodes, start, delay)
	s.Edges = append(s.Edges, NewEdge(start.ID, delay.ID))

	r := NewRunner()
	r.Start(nil, testWindow(), s, nil, nil, 100, 50)
	waitRunner(t, r)
	if r.NodeState(delay.ID) != StOK {
		t.Error("a nil parent context must fall back to Background")
	}
}

type atomicStrings struct {
	v atomic.Value
}

func (a *atomicStrings) add(s string) {
	cur, _ := a.v.Load().([]string)
	next := make([]string, len(cur), len(cur)+1)
	copy(next, cur)
	a.v.Store(append(next, s))
}

func (a *atomicStrings) all() []string {
	cur, _ := a.v.Load().([]string)
	return cur
}

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
	n.EnvID = "e1"
	c0, c1 := ed.envChipRect(n)
	mid := f32.Pt((c0.X+c1.X)/2, (c0.Y+c1.Y)/2)

	ed.onPress(press(mid))
	if ed.envMenuNodeID != n.ID {
		t.Errorf("envMenuNodeID = %q, want %q", ed.envMenuNodeID, n.ID)
	}
	if ed.selNodeID != "" {
		t.Error("opening the env chip must not select the node")
	}

	t.Run("no chip while the node follows the active env", func(t *testing.T) {
		ed := newBlankEditor()
		addNodeTo(ed, KindRequest, 100, 100)
		ed.onPress(press(f32.Pt(c0.X+8, mid.Y)))
		if ed.envMenuNodeID != "" {
			t.Errorf("envMenuNodeID = %q, want none", ed.envMenuNodeID)
		}
		if !ed.marquee {
			t.Error("the empty chip area must behave like empty canvas")
		}
	})
}

func TestOnPressResizesAnyNode(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindRequest, 100, 100)
	_, nw, nh := ed.nodeScreenRect(n)
	ed.onPress(press(f32.Pt(100+nw, 100+nh)))
	if ed.resizeNodeID != n.ID {
		t.Fatalf("resizeNodeID = %q, want %q", ed.resizeNodeID, n.ID)
	}
	ed.onDrag(f32.Pt(100+nw+120, 100+nh+80))
	if n.W != nw+120 || n.H != nh+80 {
		t.Errorf("node size = (%v,%v), want (%v,%v)", n.W, n.H, nw+120, nh+80)
	}
	if _, w2, h2 := ed.nodeScreenRect(n); w2 != n.W || h2 != n.H {
		t.Errorf("screen size = (%v,%v), want the stored size", w2, h2)
	}
	ed.onDrag(f32.Pt(100, 100))
	minW, minH := nodeMinSize(n, ed.nodeW, ed.nodeH)
	if n.W != minW || n.H != minH {
		t.Errorf("clamped to (%v,%v), want (%v,%v)", n.W, n.H, minW, minH)
	}
	if minH <= ed.nodeH {
		t.Error("a request node must keep room for its body box")
	}

	plain := addNodeTo(ed, KindDelay, 600, 100)
	ed.onRelease(f32.Pt(100, 100))
	ed.onPress(press(f32.Pt(600+ed.nodeW, 100+ed.nodeH)))
	if ed.resizeNodeID != plain.ID {
		t.Fatalf("resizeNodeID = %q, want %q", ed.resizeNodeID, plain.ID)
	}
	ed.onDrag(f32.Pt(0, 0))
	if plain.H != ed.nodeH {
		t.Errorf("delay min height = %v, want the header height %v", plain.H, ed.nodeH)
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

func TestWheelZoomWhilePanningKeepsContinuity(t *testing.T) {
	ed := newBlankEditor()
	ed.onPress(pointer.Event{Kind: pointer.Press, Position: f32.Pt(300, 300), Buttons: pointer.ButtonSecondary})
	if !ed.panning {
		t.Fatal("RMB press must start panning")
	}
	ed.onDrag(f32.Pt(340, 320))
	ed.zoomByNotches(f32.Pt(340, 320), 2)
	zoomed := ed.pan
	ed.onDrag(f32.Pt(340, 320))
	if ed.pan != zoomed {
		t.Errorf("pan after a still drag = %v, want the zoomed pan %v", ed.pan, zoomed)
	}
	ed.onDrag(f32.Pt(350, 320))
	if want := zoomed.Add(f32.Pt(10, 0)); ed.pan != want {
		t.Errorf("pan = %v, want %v", ed.pan, want)
	}
}

func TestResizeEdgeAt(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindDelay, 100, 100)
	cases := []struct {
		name string
		pt   f32.Point
		node bool
		edge resizeEdge
	}{
		{"bottom-right corner", f32.Pt(100+ed.nodeW-3, 100+ed.nodeH-3), true, resizeEdge{r: true, b: true}},
		{"left edge", f32.Pt(101, 110), true, resizeEdge{l: true}},
		{"right edge", f32.Pt(100+ed.nodeW-1, 110), true, resizeEdge{r: true}},
		{"top edge", f32.Pt(140, 101), true, resizeEdge{t: true}},
		{"top-left corner", f32.Pt(100, 100), true, resizeEdge{l: true, t: true}},
		{"bottom port wins", f32.Pt(100+ed.nodeW/2, 100+ed.nodeH), false, resizeEdge{}},
		{"inside the node", f32.Pt(140, 120), false, resizeEdge{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, e := ed.resizeEdgeAt(tc.pt)
			if (got == n) != tc.node || e != tc.edge {
				t.Errorf("resizeEdgeAt(%v) = node %v edge %+v, want node %v edge %+v", tc.pt, got == n, e, tc.node, tc.edge)
			}
		})
	}
}

func TestOnDragResizesFromLeftAndTop(t *testing.T) {
	ed := newBlankEditor()
	n := addNodeTo(ed, KindDelay, 300, 300)
	w0, h0 := ed.nodeWH(n)

	ed.onPress(press(f32.Pt(301, 305)))
	if ed.resizeNodeID != n.ID || !ed.resizeEdge.l {
		t.Fatalf("pressing the left edge must start a left resize, got %q %+v", ed.resizeNodeID, ed.resizeEdge)
	}
	ed.onDrag(f32.Pt(261, 305))
	if n.X != 260 || n.W != w0+40 || n.Y != 300 {
		t.Errorf("left resize: pos (%v,%v) size %v, want x 260 w %v", n.X, n.Y, n.W, w0+40)
	}
	ed.onDrag(f32.Pt(900, 305))
	minW, _ := nodeMinSize(n, ed.nodeW, ed.nodeH)
	if n.W != minW || n.X != 300+w0-minW {
		t.Errorf("left resize clamp: x %v w %v, want the right edge pinned at %v", n.X, n.W, 300+w0)
	}
	ed.onRelease(f32.Pt(900, 305))

	ed.onPress(press(f32.Pt(n.X+12, 301)))
	if ed.resizeNodeID != n.ID || !ed.resizeEdge.t {
		t.Fatalf("pressing the top edge must start a top resize, got %q %+v", ed.resizeNodeID, ed.resizeEdge)
	}
	ed.onDrag(f32.Pt(n.X+12, 271))
	if n.Y != 270 || n.H != h0+30 {
		t.Errorf("top resize: y %v h %v, want y 270 h %v", n.Y, n.H, h0+30)
	}
}

func makeFlowGtx(size image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(size),
		Now:         time.Now(),
	}
}

func layoutOnce(t *testing.T, ed *Editor, host *Host, size image.Point) {
	t.Helper()
	if host.Win == nil {
		host.Win = testWindow()
	}
	if host.WinSize == (image.Point{}) {
		host.WinSize = size
	}
	gtx := makeFlowGtx(size)
	ed.Layout(gtx, material.NewTheme(), host)
}

func TestLayoutCollectsFrameEnvironments(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	sv := addNodeTo(ed, KindSetVar, 300, 0)
	sv.VarNameEd.SetText("token")
	req := addNodeTo(ed, KindRequest, 600, 0)
	req.EnvID = "env2"

	envCalls := 0
	host := &Host{
		WinSize:    image.Pt(1200, 800),
		ActiveEnv:  func() map[string]string { return map[string]string{"host": "x"} },
		EnvOptions: func() []EnvOption { return []EnvOption{{ID: "env2", Name: "Prod"}} },
		EnvVars: func(id string) map[string]string {
			envCalls++
			return map[string]string{"secret": id}
		},
	}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.frameEnvs[""]["host"] != "x" {
		t.Errorf("the active env must be cached under the empty key, got %v", ed.frameEnvs)
	}
	if ed.frameEnvs["env2"]["secret"] != "env2" {
		t.Errorf("per-node envs must be cached, got %v", ed.frameEnvs)
	}
	if envCalls != 1 {
		t.Errorf("each env must be resolved once per frame, got %d calls", envCalls)
	}
	if !ed.setVarNames["token"] {
		t.Errorf("set-variable names must be collected, got %v", ed.setVarNames)
	}
	if len(ed.envOpts) != 1 || ed.envOpts[0].Name != "Prod" {
		t.Errorf("env options = %v", ed.envOpts)
	}
}

func TestLayoutWithoutHostCallbacks(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	addNodeTo(ed, KindRequest, 300, 0)
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1000, 700)}, image.Pt(1000, 700))

	if ed.envOpts != nil {
		t.Errorf("env options must be nil without a provider, got %v", ed.envOpts)
	}
	if len(ed.frameEnvs) != 0 {
		t.Errorf("frameEnvs must stay empty without an active env, got %v", ed.frameEnvs)
	}
}

func TestLayoutClampsPanelWidth(t *testing.T) {
	setupFlowConfig(t)
	tests := []struct {
		name  string
		start int
		size  image.Point
		check func(t *testing.T, ed *Editor, size image.Point)
	}{
		{
			"unset width gets a default",
			0,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, _ image.Point) {
				if ed.panelW != 300 {
					t.Errorf("panelW = %d, want the 300dp default", ed.panelW)
				}
			},
		},
		{
			"too wide is clamped to half the window",
			10000,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, size image.Point) {
				if ed.panelW != size.X/2 {
					t.Errorf("panelW = %d, want %d", ed.panelW, size.X/2)
				}
			},
		},
		{
			"too narrow is clamped to the minimum",
			1,
			image.Pt(1200, 800),
			func(t *testing.T, ed *Editor, _ image.Point) {
				if ed.panelW != 56 {
					t.Errorf("panelW = %d, want the 56dp minimum", ed.panelW)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ed := newTestEditor()
			ed.panelW = tt.start
			layoutOnce(t, ed, &Host{WinSize: tt.size}, tt.size)
			tt.check(t, ed, tt.size)
		})
	}
}

func TestLayoutRecordsCanvasOrigin(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	host := &Host{WinSize: image.Pt(1400, 900)}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.canvasOrig != image.Pt(200, 100) {
		t.Errorf("canvasOrig = %v, want (200,100)", ed.canvasOrig)
	}
	if ed.winH != 900 {
		t.Errorf("winH = %d, want 900", ed.winH)
	}
}

func TestLayoutMeasuresNodeSizeAndFits(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.nodeW, ed.nodeH = 0, 0
	ed.pendingFit = true
	ed.Scenario.Nodes[0].X, ed.Scenario.Nodes[0].Y = 4000, 4000
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))

	if ed.nodeW != 176 || ed.nodeH != 56 {
		t.Errorf("node metrics = (%v,%v), want (176,56)", ed.nodeW, ed.nodeH)
	}
	if ed.portHit != 12 {
		t.Errorf("portHit = %v, want 12", ed.portHit)
	}
	if ed.pendingFit {
		t.Error("the deferred fit must have run once the canvas has a size")
	}
	if ed.canvasSize.X <= 0 || ed.canvasSize.Y <= 0 {
		t.Errorf("canvasSize = %v", ed.canvasSize)
	}
	s := ed.toScreen(f32.Pt(4000, 4000))
	if s.X < 0 || s.Y < 0 || s.X > float32(ed.canvasSize.X) || s.Y > float32(ed.canvasSize.Y) {
		t.Errorf("the fitted node projects off canvas at %v", s)
	}
}

func TestLayoutAllPanelModes(t *testing.T) {
	setupFlowConfig(t)
	modes := []struct {
		name string
		mode panelMode
	}{
		{"properties", modeProps},
		{"history", modeHistory},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			ed := newTestEditor()
			req := addNodeTo(ed, KindRequest, 300, 0)
			req.URLEd.SetText("http://{{unknown}}")
			cond := addNodeTo(ed, KindCondition, 600, 0)
			loop := addNodeTo(ed, KindLoop, 900, 0)
			note := addNodeTo(ed, KindNote, 1200, 0)
			sv := addNodeTo(ed, KindSetVar, 1500, 0)
			delay := addNodeTo(ed, KindDelay, 1800, 0)
			for _, n := range []*Node{cond, loop, note, sv, delay} {
				connect(ed, req, n)
			}
			ed.mode = m.mode
			ed.selectOnly(req.ID)

			host := &Host{
				WinSize:    image.Pt(1200, 800),
				EnvOptions: func() []EnvOption { return []EnvOption{{ID: "", Name: "Active"}, {ID: "e", Name: "Prod"}} },
				ActiveEnv:  func() map[string]string { return map[string]string{} },
				EnvVars:    func(string) map[string]string { return map[string]string{} },
			}
			layoutOnce(t, ed, host, image.Pt(1200, 800))
		})
	}
}

func TestLayoutPropsForEveryNodeKind(t *testing.T) {
	setupFlowConfig(t)
	kinds := []NodeKind{KindStart, KindRequest, KindCondition, KindLoop, KindDelay, KindSetVar, KindNote}
	for _, k := range kinds {
		t.Run(k.Title(), func(t *testing.T) {
			ed := newTestEditor()
			n := addNodeTo(ed, k, 300, 0)
			ed.mode = modeProps
			ed.selectOnly(n.ID)
			layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
		})
	}
}

func TestLayoutEdgePropsForEveryCondition(t *testing.T) {
	setupFlowConfig(t)
	for _, cond := range CondKinds {
		t.Run(cond.Title(), func(t *testing.T) {
			ed := newTestEditor()
			a := addNodeTo(ed, KindRequest, 300, 0)
			b := addNodeTo(ed, KindDelay, 600, 0)
			e := connect(ed, a, b)
			e.Cond = cond
			ed.mode = modeProps
			ed.selEdgeID = e.ID
			layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
		})
	}
}

func TestLayoutHistoryWithRuns(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.mode = modeHistory

	rec := &RunRecord{Label: "Run 1 · 12:00:00", Seq: 1, Clock: "12:00:00", Dur: 1500 * time.Millisecond, Done: true}
	ed.Runner.runs = append(ed.Runner.runs, rec)
	ed.Runner.addEntry(rec, &RunEntry{
		Node: "Fetch", Detail: "GET http://x", Code: 200, Status: "200 OK",
		OK: true, Body: `{"a":1}`, BodyLen: 7, Dur: 42 * time.Millisecond, Expanded: true,
	})
	ed.Runner.addEntry(rec, &RunEntry{
		Node: "Broken", Detail: "GET http://y", Code: 0, Status: "no response", OK: false,
	})

	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))

	if len(ed.Runner.Runs()) != 1 {
		t.Errorf("run history must survive a layout pass, got %d", len(ed.Runner.Runs()))
	}
}

func TestLayoutCompactPanel(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.panelW = 1
	ed.mode = modeProps
	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
	if !ed.panelCompact {
		t.Error("a narrow panel must switch to compact mode")
	}

	ed2 := newTestEditor()
	ed2.panelW = 300
	layoutOnce(t, ed2, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
	if ed2.panelCompact {
		t.Error("a wide panel must not be compact")
	}
}

func TestLayoutWhileRunning(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	ed.Runner.mu.Lock()
	ed.Runner.running = true
	ed.Runner.paused = true
	ed.Runner.stepMode = true
	ed.Runner.status = "Paused · Fetch"
	ed.Runner.mu.Unlock()

	layoutOnce(t, ed, &Host{WinSize: image.Pt(1200, 800)}, image.Pt(1200, 800))
}

func TestLayoutWithOpenOverlays(t *testing.T) {
	setupFlowConfig(t)
	ed := newTestEditor()
	n := addNodeTo(ed, KindRequest, 300, 0)
	ed.selectOnly(n.ID)
	ed.mode = modeProps
	ed.envMenuNodeID = n.ID
	ed.envDropOpen = true
	ed.envDropAtY = 200
	ed.connectFromID = n.ID
	ed.connectPos = f32.Pt(500, 300)
	ed.marquee = true
	ed.marqueeStart = f32.Pt(10, 10)
	ed.marqueeCur = f32.Pt(200, 200)

	host := &Host{
		WinSize:           image.Pt(1200, 800),
		EnvOptions:        func() []EnvOption { return []EnvOption{{ID: "", Name: "Active"}, {ID: "e", Name: "Prod"}} },
		ExternalDrag:      true,
		ExternalDragPos:   f32.Pt(600, 400),
		ExternalDragLabel: "dragged request",
	}
	layoutOnce(t, ed, host, image.Pt(1200, 800))

	if ed.extDragLabel != "dragged request" {
		t.Errorf("external drag state must be mirrored, got %q", ed.extDragLabel)
	}
}

func TestNewEditorLoadsLatest(t *testing.T) {
	dir := setupFlowConfig(t)
	writeFlow(t, dir, scenarioDTO{
		ID:    "saved",
		Name:  "Saved flow",
		Nodes: []nodeDTO{{ID: "s", Kind: int(KindStart)}},
	}, time.Now())

	ed := NewEditor()
	if ed.Scenario == nil || ed.Scenario.ID != "saved" {
		t.Fatalf("NewEditor must load the latest scenario, got %+v", ed.Scenario)
	}
	if ed.Runner == nil {
		t.Fatal("NewEditor must create a runner")
	}
	if ed.zoom != 1 || !ed.pendingFit || ed.mode != modeProps {
		t.Errorf("unexpected initial state: zoom=%v pendingFit=%v mode=%v", ed.zoom, ed.pendingFit, ed.mode)
	}
	if ed.selected == nil {
		t.Error("the selection set must be initialised")
	}
	if ed.panelList.Axis != layout.Vertical {
		t.Error("the panel list must be vertical")
	}
}

func setupFlowConfig(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "rete-test")
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
		{"plain node default size", &Node{Kind: KindDelay}, defW, defH},
		{"plain node explicit size", &Node{Kind: KindDelay, W: 500, H: 500}, 500, 500},
		{"request node grows a body box", &Node{Kind: KindRequest}, defW, defH + bodyBoxH(defH)},
		{"request node explicit size", &Node{Kind: KindRequest, W: 500, H: 500}, 500, 500},
		{"ws message node grows a body box", &Node{Kind: KindWSSend}, defW, defH + bodyBoxH(defH)},
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
		{"in header band", loop, &Node{ID: "b", Kind: KindDelay, X: 100, Y: 0}, false},
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

func TestListScenariosHonorsSavedOrder(t *testing.T) {
	dir := setupFlowConfig(t)
	base := time.Now().Add(-time.Hour)
	writeFlow(t, dir, scenarioDTO{ID: "a", Name: "A"}, base)
	writeFlow(t, dir, scenarioDTO{ID: "b", Name: "B"}, base.Add(10*time.Minute))
	writeFlow(t, dir, scenarioDTO{ID: "c", Name: "C"}, base.Add(20*time.Minute))

	if err := SetScenarioOrder([]string{"b", "c", "a", "c", ""}); err != nil {
		t.Fatalf("set order: %v", err)
	}
	ids := func() []string {
		var out []string
		for _, inf := range ListScenarios() {
			out = append(out, inf.ID)
		}
		return out
	}
	if got, want := ids(), []string{"b", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	if err := RenameScenario("a", "A renamed"); err != nil {
		t.Fatal(err)
	}
	if got, want := ids(), []string{"b", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("rename must not move the scenario: %v, want %v", got, want)
	}

	writeFlow(t, dir, scenarioDTO{ID: "fresh", Name: "Fresh"}, time.Now())
	writeFlow(t, dir, scenarioDTO{ID: "older", Name: "Older"}, time.Now().Add(-time.Minute))
	if got, want := ids(), []string{"fresh", "older", "b", "c", "a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("unordered scenarios go first, newest first: %v, want %v", got, want)
	}

	if err := DeleteScenario("c"); err != nil {
		t.Fatal(err)
	}
	if got := readScenarioOrder(); !reflect.DeepEqual(got, []string{"b", "a"}) {
		t.Errorf("delete must drop the id from the saved order, got %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, scenarioOrderFile)); err != nil {
		t.Errorf("order file should exist: %v", err)
	}
	for _, inf := range ListScenarios() {
		if inf.ID == "" || inf.Name == "" && inf.ID != "" {
			t.Errorf("order file must never be listed as a scenario: %+v", inf)
		}
	}
}

func TestSetScenarioOrderBumpsChangeSeq(t *testing.T) {
	setupFlowConfig(t)
	before := ChangeSeq()
	if err := SetScenarioOrder([]string{"x"}); err != nil {
		t.Fatal(err)
	}
	if ChangeSeq() == before {
		t.Error("saving the order must bump ChangeSeq so the sidebar rows refresh")
	}
}

func TestCanvasOverlappingNodesKeepInputSeparate(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	under := addNodeTo(ed, KindRequest, 200, 200)
	over := addNodeTo(ed, KindRequest, 240, 230)
	body := "{"
	for i := 0; i < 12; i++ {
		body += "\n  \"k" + itoa(i) + "\": 1,"
	}
	body += "\n}"
	under.BodyEd.SetText(body)
	over.BodyEd.SetText(body)
	rig.frame()
	rig.frame()
	ub := under.CanvasBodyEditor()
	ob := over.CanvasBodyEditor()
	obox, _ := ed.bodyBoxRect(over)
	pt := f32.Pt(float32(obox.Min.X+10), float32(obox.Min.Y+8))
	rig.r.Queue(rig.timedEvent(pointer.Move, pt, 0), pointer.Event{Kind: pointer.Scroll, Position: pt, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
	rig.frame()
	rig.frame()
	t.Logf("scroll over top body: top scrollY=%d under scrollY=%d", ob.GetScrollY(), ub.GetScrollY())
	if ub.GetScrollY() != 0 {
		t.Error("wheel over the top node's body must not scroll the node underneath")
	}
	if ob.GetScrollY() == 0 {
		t.Error("wheel over the top node's body must scroll its own text")
	}
	for i := 0; i < 25; i++ {
		rig.r.Queue(rig.timedEvent(pointer.Move, pt, 0), pointer.Event{Kind: pointer.Scroll, Position: pt, Source: pointer.Mouse, Scroll: f32.Pt(0, 200), Time: rig.clock})
		rig.frame()
	}
	rig.frame()
	if ub.GetScrollY() != 0 {
		t.Errorf("wheel past the end of the top body must not leak into the node underneath, its scrollY = %d", ub.GetScrollY())
	}
	if ob.GetScrollY() != ob.GetScrollBounds().Max.Y {
		t.Errorf("top body must be scrolled to its end, got %d of %d", ob.GetScrollY(), ob.GetScrollBounds().Max.Y)
	}

	ubox, _ := ed.bodyBoxRect(under)
	hdr := f32.Pt(float32(over.X)+60, float32(ubox.Min.Y)+8)
	if !image.Pt(int(hdr.X), int(hdr.Y)).In(ubox) || hdr.Y < over.Y || hdr.Y > over.Y+ed.nodeH {
		t.Fatalf("setup: the point %v must lie in the top header and over the body box %v of the node underneath", hdr, ubox)
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr, 0), pointer.Event{Kind: pointer.Scroll, Position: hdr, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
	rig.frame()
	rig.frame()
	if ub.GetScrollY() != 0 {
		t.Errorf("wheel over the top node's header must not scroll the body of the node underneath, scrollY = %d", ub.GetScrollY())
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr, 0), rig.timedEvent(pointer.Press, hdr, pointer.ButtonPrimary))
	rig.frame()
	if rig.r.Source().Focused(ub) {
		t.Error("pressing the top node's header must not focus the body editor of the node underneath")
	}
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr.Add(f32.Pt(50, 40)), pointer.ButtonPrimary))
	rig.frame()
	rig.r.Queue(rig.timedEvent(pointer.Move, hdr.Add(f32.Pt(100, 80)), pointer.ButtonPrimary))
	rig.frame()
	rig.r.Queue(rig.timedEvent(pointer.Release, hdr.Add(f32.Pt(100, 80)), 0))
	rig.frame()
	t.Logf("after header drag: over=(%v,%v) under=(%v,%v) dragNode=%q", over.X, over.Y, under.X, under.Y, ed.dragNodeID)
	if over.X != 340 || over.Y != 310 {
		t.Errorf("dragging the top node's header must move it by (100,80), got (%v,%v)", over.X, over.Y)
	}
	if under.X != 200 || under.Y != 200 {
		t.Errorf("the node underneath must stay, got (%v,%v)", under.X, under.Y)
	}
}

func TestCanvasPaletteDragNoMarquee(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	rig.frame()
	rig.frame()
	r1 := ed.paletteRects[1]
	start := f32.Pt(float32(r1.Min.X+r1.Dx()/2), float32(r1.Min.Y+r1.Dy()/2))
	target := f32.Pt(500, 250)
	widgets.GlobalPointerPos = start
	rig.r.Queue(rig.timedEvent(pointer.Move, start, 0), rig.timedEvent(pointer.Press, start, pointer.ButtonPrimary))
	rig.frame()
	if ed.marquee || ed.panning || ed.dragNodeID != "" {
		t.Errorf("pressing a palette button must not start a canvas gesture: marquee=%v panning=%v drag=%q", ed.marquee, ed.panning, ed.dragNodeID)
	}
	if !ed.palDragOn {
		t.Fatal("pressing a palette button must arm the palette drag")
	}
	widgets.GlobalPointerPos = target
	rig.r.Queue(rig.timedEvent(pointer.Move, target, pointer.ButtonPrimary))
	rig.frame()
	if !ed.palDragActive {
		t.Error("dragging over the canvas must arm the drop")
	}
	if ed.marquee {
		t.Error("dragging a palette button must not start a marquee selection on the canvas")
	}
	if len(ed.ghostNodes) != 1 || ed.ghostNodes[0].Kind != KindWSRequest {
		t.Fatalf("the drag ghost must be a real node of the dragged kind, got %d nodes", len(ed.ghostNodes))
	}
	want := ed.toWorld(target)
	g := ed.ghostNodes[0]
	if g.X != want.X-ed.nodeW/2 || g.Y != want.Y-ed.nodeH/2 {
		t.Errorf("ghost at (%v,%v), want centred on the pointer (%v,%v)", g.X, g.Y, want.X-ed.nodeW/2, want.Y-ed.nodeH/2)
	}
	gx, gy := g.X, g.Y
	rig.r.Queue(rig.timedEvent(pointer.Release, target, 0))
	rig.frame()
	if len(ed.ghostNodes) != 0 {
		t.Error("the ghost must disappear once the node is dropped")
	}
	last := ed.Scenario.Nodes[len(ed.Scenario.Nodes)-1]
	if last.Kind != KindWSRequest || last.X != gx || last.Y != gy {
		t.Errorf("dropped node must land exactly where the ghost was: got %v (%v,%v), ghost (%v,%v)", last.Kind, last.X, last.Y, gx, gy)
	}
}

func TestCanvasCustomWidgetsMenu(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	if err := SaveBlock(scenarioDTO{Name: "Auth pair", Nodes: []nodeDTO{
		{Kind: int(KindRequest), X: 0, Y: 0, Name: "Login", Method: "POST"},
		{Kind: int(KindSetVar), X: 260, Y: 0, Name: "Store token"},
	}}); err != nil {
		t.Fatal(err)
	}
	rig.frame()
	rig.frame()
	if ed.customRect.Empty() {
		t.Fatal("the custom widgets button must be part of the toolbar")
	}
	center := func(r image.Rectangle) f32.Point {
		return f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2))
	}
	rig.timedClick(center(ed.customRect))
	if !ed.customMenuOpen {
		t.Fatal("clicking the custom button must open the menu")
	}
	rig.frame()
	if len(ed.customRows) != 1 || ed.customPopup.Empty() {
		t.Fatalf("the menu must list the saved block, rows=%d popup=%v", len(ed.customRows), ed.customPopup)
	}
	if ed.customRows[0].Max.Y > ed.customRect.Min.Y {
		t.Errorf("the menu must open above the button, row %v button %v", ed.customRows[0], ed.customRect)
	}
	before := len(ed.Scenario.Nodes)
	rig.timedClick(center(ed.customRows[0]))
	if len(ed.Scenario.Nodes) != before+2 {
		t.Fatalf("clicking a block must insert its nodes, nodes = %d", len(ed.Scenario.Nodes))
	}
	if ed.customMenuOpen {
		t.Error("inserting a block must close the menu")
	}

	rig.timedClick(center(ed.customRect))
	rig.frame()
	if !ed.customMenuOpen || len(ed.customRows) != 1 {
		t.Fatal("the menu must reopen")
	}
	row := center(ed.customRows[0])
	target := f32.Pt(450, 200)
	widgets.GlobalPointerPos = f32.Pt(0, 0)
	rig.r.Queue(rig.timedEvent(pointer.Move, row, 0), rig.timedEvent(pointer.Press, row, pointer.ButtonPrimary))
	rig.frame()
	rig.r.Queue(rig.timedEvent(pointer.Move, target, pointer.ButtonPrimary))
	rig.frame()
	if !ed.blockDragActive {
		t.Fatal("dragging a block row over the canvas must arm the drop")
	}
	rig.frame()
	if len(ed.ghostNodes) != 2 {
		t.Errorf("the block ghost must show its 2 nodes, got %d", len(ed.ghostNodes))
	}
	if ed.blockDragWin != target {
		t.Errorf("block drag must track the pointer from its own events, got %v want %v", ed.blockDragWin, target)
	}
	before = len(ed.Scenario.Nodes)
	rig.r.Queue(rig.timedEvent(pointer.Release, target, 0))
	rig.frame()
	rig.frame()
	if len(ed.Scenario.Nodes) != before+2 {
		t.Fatalf("releasing over the canvas must drop the block, nodes = %d", len(ed.Scenario.Nodes))
	}
	if ed.customMenuOpen {
		t.Error("dragging a block out must close the menu")
	}
	w := ed.toWorld(target)
	minX, maxX := ed.Scenario.Nodes[before].X, ed.Scenario.Nodes[before].X
	for _, n := range ed.Scenario.Nodes[before:] {
		if n.X < minX {
			minX = n.X
		}
		if nw, _ := ed.nodeWH(n); n.X+nw > maxX {
			maxX = n.X + nw
		}
	}
	if minX > w.X || maxX < w.X {
		t.Errorf("dropped block %v..%v must span the pointer x %v", minX, maxX, w.X)
	}
}

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

func TestCanvasZoomKeepsBodyTopLine(t *testing.T) {
	rig := newCanvasRig(t)
	ed := rig.ed
	n := addNodeTo(ed, KindRequest, 200, 200)
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 40; i++ {
		b.WriteString("\n  \"key")
		b.WriteString(itoa(i))
		b.WriteString("\": \"value\",")
	}
	b.WriteString("\n}")
	n.BodyEd.SetText(b.String())
	lines := 42
	rig.frame()
	rig.frame()
	box, _ := ed.bodyBoxRect(n)
	inBody := f32.Pt(float32(box.Min.X+20), float32(box.Min.Y+20))
	ce := n.CanvasBodyEditor()
	for i := 0; i < 7; i++ {
		rig.r.Queue(rig.timedEvent(pointer.Move, inBody, 0), pointer.Event{Kind: pointer.Scroll, Position: inBody, Source: pointer.Mouse, Scroll: f32.Pt(0, 40), Time: rig.clock})
		rig.frame()
		rig.frame()
	}
	topLine := func() float32 {
		bx, _ := ed.bodyBoxRect(n)
		pad := int(4 * ed.zoom)
		if pad < 1 {
			pad = 1
		}
		viewH := bx.Dy() - 1 - 2*pad
		contentH := ce.GetScrollBounds().Max.Y + viewH
		return float32(ce.GetScrollY()) * float32(lines) / float32(contentH)
	}
	tl0 := topLine()
	worst := float32(0)
	for step := 0; step < 24; step++ {
		ed.zoomByNotches(f32.Pt(50, 50), -1)
		rig.frame()
		d := topLine() - tl0
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	for step := 0; step < 24; step++ {
		ed.zoomByNotches(f32.Pt(50, 50), 1)
		rig.frame()
		d := topLine() - tl0
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
	}
	t.Logf("back at zoom %v scrollY %d topLine %.2f, worst drift %.2f lines", ed.zoom, ce.GetScrollY(), topLine(), worst)
	if worst > 0.15 {
		t.Errorf("top line drifts by up to %.2f lines across zoom levels", worst)
	}
}
