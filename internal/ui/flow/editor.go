package flow

import (
	"context"
	"encoding/json"
	"image"
	"image/color"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type panelMode int

const (
	modeWidgets panelMode = iota
	modeProps
	modeHistory
)

const (
	minZoom      = 0.3
	maxZoom      = 3.0
	zoomStep     = 1.1
	historyLimit = 100
)

type EnvOption struct {
	ID   string
	Name string
}

type Host struct {
	Win        *app.Window
	RootCtx    context.Context
	ActiveEnv  func() map[string]string
	EnvOptions func() []EnvOption
	EnvVars    func(id string) map[string]string
	WinSize    image.Point

	ExternalDrag      bool
	ExternalDragPos   f32.Point
	ExternalDragLabel string
}

type Editor struct {
	Scenario *Scenario
	Runner   *Runner

	pan        f32.Point
	zoom       float32
	canvasSize image.Point
	canvasOrig image.Point
	pendingFit bool
	nodeW      float32
	nodeH      float32
	portHit    float32

	selNodeID string
	selEdgeID string
	selected  map[string]bool
	mode      panelMode
	histRun   *RunRecord

	dragNodeID      string
	dragMembers     []string
	dragOff         f32.Point
	dragMoved       bool
	resizeNodeID    string
	resizeMoved     bool
	panning         bool
	panStart        f32.Point
	panOrigin       f32.Point
	marquee         bool
	marqueeStart    f32.Point
	marqueeCur      f32.Point
	connectFromID   string
	connectFromSide string
	connectToID     string
	connectToSide   string
	connectPos      f32.Point
	reconnectEdge   *Edge
	wantFocus       bool

	hoverPos    f32.Point
	hoverOn     bool
	hoverNodeID string

	envOpts []EnvOption

	undoStack   []string
	redoStack   []string
	pendingSnap string
	clipboard   string

	panelW      int
	panelDrag   gesture.Drag
	panelDragX  float32
	panelPx     float32
	canvasDrawn int

	palDragKind   NodeKind
	palDragOn     bool
	palDragActive bool
	extDrag       bool
	extDragPos    f32.Point
	extDragLabel  string

	blockNameEd     widget.Editor
	blockSaveBtn    widget.Clickable
	blockBtns       []widget.Clickable
	blockDelBtns    []widget.Clickable
	blockDragTags   []bool
	blockDragIdx    int
	blockDragName   string
	blockDragOn     bool
	blockDragActive bool
	blocksCache     []BlockInfo
	blocksSeq       int64
	blocksLoaded    bool

	wsKeepBtn  widget.Clickable
	wsCloseBtn widget.Clickable

	note string

	BtnWidgets widget.Clickable
	BtnProps   widget.Clickable
	BtnHistory widget.Clickable
	BtnRun     widget.Clickable
	BtnSave    widget.Clickable
	BtnNew     widget.Clickable
	BtnDelete  widget.Clickable

	BtnStep     widget.Clickable
	BtnStepMode widget.Clickable

	addBtns     [9]widget.Clickable
	palDragTags [9]bool
	methodBtn   [8]widget.Clickable
	bodyTypeBtn [4]widget.Clickable
	authKindBtn [3]widget.Clickable
	wsOpBtn     [2]widget.Clickable
	wsInsecBtn  [1]widget.Clickable
	condBtn     [7]widget.Clickable
	opBtn       [6]widget.Clickable
	valOpBtn    [7]widget.Clickable
	envBtns     []widget.Clickable
	envDropBtn  widget.Clickable
	envDropOpen bool
	envDropAtY  float32
	winH        int

	envMenuNodeID string

	panelCompact bool
	fitBadge     image.Rectangle
	zoomBadge    image.Rectangle

	lastSaved    string
	nextAutosave time.Time

	lastClickAt time.Time
	lastClickID string
	focusNameID string

	frameEnvs   map[string]map[string]string
	setVarNames map[string]bool

	panelList widget.List
}

func NewEditor() *Editor {
	ed := &Editor{
		Scenario:   LoadLatest(),
		Runner:     NewRunner(),
		mode:       modeWidgets,
		zoom:       1,
		selected:   make(map[string]bool),
		pendingFit: true,
	}
	ed.panelList.Axis = layout.Vertical
	ed.blockNameEd.SingleLine = true
	return ed
}

func (ed *Editor) blocks() []BlockInfo {
	if seq := BlockSeq(); !ed.blocksLoaded || seq != ed.blocksSeq {
		ed.blocksCache = ListBlocks()
		ed.blocksSeq = seq
		ed.blocksLoaded = true
	}
	return ed.blocksCache
}

func (ed *Editor) saveSelectionAsBlock(name string) bool {
	var dto scenarioDTO
	inBlock := make(map[string]bool)
	for _, n := range ed.Scenario.Nodes {
		if ed.selected[n.ID] && n.Kind != KindStart {
			dto.Nodes = append(dto.Nodes, nodeToDTO(n))
			inBlock[n.ID] = true
		}
	}
	if len(dto.Nodes) == 0 {
		ed.note = "Nothing to save as a block"
		return false
	}
	for _, e := range ed.Scenario.Edges {
		if inBlock[e.From] && inBlock[e.To] {
			dto.Edges = append(dto.Edges, edgeToDTO(e))
		}
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Block"
	}
	dto.Name = name
	if err := SaveBlock(dto); err != nil {
		ed.note = "Block save failed: " + err.Error()
		return false
	}
	ed.note = "Block saved: " + name
	return true
}

func (ed *Editor) insertBlockAt(dto scenarioDTO, at f32.Point) {
	if len(dto.Nodes) == 0 {
		return
	}
	ed.pushHistory()
	minX, minY := dto.Nodes[0].X, dto.Nodes[0].Y
	maxX, maxY := dto.Nodes[0].X, dto.Nodes[0].Y
	for _, nd := range dto.Nodes {
		if nd.X < minX {
			minX = nd.X
		}
		if nd.Y < minY {
			minY = nd.Y
		}
		if nd.X > maxX {
			maxX = nd.X
		}
		if nd.Y > maxY {
			maxY = nd.Y
		}
	}
	offX := at.X - (minX+maxX+ed.nodeW)/2
	offY := at.Y - (minY+maxY+ed.nodeH)/2
	idMap := make(map[string]string, len(dto.Nodes))
	ed.selected = make(map[string]bool)
	ed.selEdgeID = ""
	for _, nd := range dto.Nodes {
		if NodeKind(nd.Kind) == KindStart {
			continue
		}
		n := nodeFromDTO(nd)
		old := n.ID
		n.ID = persist.NewRandomID()
		n.X += offX
		n.Y += offY
		idMap[old] = n.ID
		ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
		ed.selected[n.ID] = true
		ed.selNodeID = n.ID
	}
	for _, edto := range dto.Edges {
		from, okF := idMap[edto.From]
		to, okT := idMap[edto.To]
		if !okF || !okT {
			continue
		}
		e := edgeFromDTO(edto)
		e.ID = persist.NewRandomID()
		e.From = from
		e.To = to
		ed.Scenario.Edges = append(ed.Scenario.Edges, e)
	}
	ed.mode = modeProps
}

func (ed *Editor) SaveScenario() {
	if err := ed.Scenario.Save(); err != nil {
		ed.note = "Save failed: " + err.Error()
	} else {
		ed.note = "Saved"
		ed.lastSaved = ed.encode()
	}
}

func (ed *Editor) OpenScenario(id string) bool {
	if ed.Runner.Running() {
		return false
	}
	if ed.Scenario != nil && ed.Scenario.ID == id {
		return true
	}
	s, err := LoadScenario(id)
	if err != nil {
		return false
	}
	ed.pushHistory()
	ed.Scenario = s
	ed.Runner.Reset()
	ed.clearSelection()
	name := strings.TrimSpace(s.NameEd.Text())
	if name == "" {
		name = "Untitled"
	}
	ed.note = "Opened: " + name
	ed.lastSaved = ed.encode()
	ed.pendingFit = true
	return true
}

func (ed *Editor) CreateNew() {
	if ed.Runner.Running() {
		return
	}
	ed.pushHistory()
	ed.Scenario = NewScenario()
	ed.Runner.Reset()
	ed.pan = f32.Point{}
	ed.zoom = 1
	ed.pendingFit = true
	ed.clearSelection()
	ed.mode = modeWidgets
	ed.note = ""
	_ = ed.Scenario.Save()
	ed.lastSaved = ed.encode()
}

const autosaveEvery = 5 * time.Second

func (ed *Editor) autosave() {
	now := time.Now()
	if ed.nextAutosave.IsZero() {
		ed.nextAutosave = now.Add(autosaveEvery)
		ed.lastSaved = ed.encode()
		return
	}
	if now.Before(ed.nextAutosave) {
		return
	}
	ed.nextAutosave = now.Add(autosaveEvery)
	enc := ed.encode()
	if enc == "" || enc == ed.lastSaved {
		return
	}
	if err := ed.Scenario.Save(); err == nil {
		ed.lastSaved = enc
		if ed.note == "" || ed.note == "Saved" || ed.note == "Auto-saved" {
			ed.note = "Auto-saved"
		}
	}
}

func (ed *Editor) defSizes() (float32, float32) {
	w, h := ed.nodeW, ed.nodeH
	if w <= 0 {
		w = 176
	}
	if h <= 0 {
		h = 56
	}
	return w, h
}

func (ed *Editor) validateScenario() []string {
	var warns []string
	reach := map[string]bool{}
	var startID string
	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindStart {
			startID = n.ID
			break
		}
	}
	if startID != "" {
		dw, dh := ed.defSizes()
		queue := []string{startID}
		reach[startID] = true
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, e := range ed.Scenario.Edges {
				if e.From == id && !reach[e.To] {
					reach[e.To] = true
					queue = append(queue, e.To)
				}
			}
			n := ed.Scenario.NodeByID(id)
			if n != nil && n.Kind == KindLoop {
				for _, o := range ed.Scenario.Nodes {
					if o.Kind == KindLoop {
						continue
					}
					if !reach[o.ID] && loopContains(n, o, dw, dh) {
						reach[o.ID] = true
						queue = append(queue, o.ID)
					}
				}
			}
		}
	}
	unreach := 0
	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindStart || n.Kind == KindNote {
			continue
		}
		if !reach[n.ID] {
			unreach++
		}
		if n.Kind.IsRequest() && n.Kind != KindWSSend && strings.TrimSpace(n.URLEd.Text()) == "" {
			warns = append(warns, "empty URL: "+n.DisplayName())
		}
	}
	hasWSSend, hasKeepOpen := false, false
	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindWSSend {
			hasWSSend = true
		}
		if n.Kind == KindWSRequest && n.KeepOpen {
			hasKeepOpen = true
		}
	}
	if hasWSSend && !hasKeepOpen {
		warns = append(warns, "WS Message needs a WebSocket node with 'keep socket open'")
	}
	dw, dh := ed.defSizes()
	for _, n := range ed.Scenario.Nodes {
		if n.Kind != KindLoop {
			continue
		}
		for _, o := range ed.Scenario.Nodes {
			if o == n || o.Kind != KindLoop {
				continue
			}
			if loopContains(o, n, dw, dh) {
				warns = append(warns, "nested loop never runs: "+n.DisplayName())
				break
			}
		}
	}
	if unreach > 0 {
		warns = append(warns, itoa(unreach)+" unreachable")
	}
	return warns
}

func (ed *Editor) ToggleRun(host *Host) {
	if ed.Runner.Running() {
		ed.Runner.Stop()
		return
	}
	ed.note = ""
	if warns := ed.validateScenario(); len(warns) > 0 {
		ed.note = "⚠ " + strings.Join(warns, " · ")
	}
	w, h := ed.defSizes()
	var env map[string]string
	if host.ActiveEnv != nil {
		env = host.ActiveEnv()
	}
	ed.Runner.Start(host.RootCtx, host.Win, ed.Scenario, env, host.EnvVars, w, h)
	ed.mode = modeHistory
	ed.histRun = nil
}

func (ed *Editor) encode() string {
	data, err := encodeScenario(ed.Scenario)
	if err != nil {
		return ""
	}
	return data
}

func (ed *Editor) pushSnapshot(snap string) {
	if snap == "" {
		return
	}
	if len(ed.undoStack) > 0 && ed.undoStack[len(ed.undoStack)-1] == snap {
		return
	}
	ed.undoStack = append(ed.undoStack, snap)
	if len(ed.undoStack) > historyLimit {
		ed.undoStack = ed.undoStack[1:]
	}
	ed.redoStack = ed.redoStack[:0]
}

func (ed *Editor) pushHistory() {
	ed.pushSnapshot(ed.encode())
}

func (ed *Editor) commitPending() {
	if ed.pendingSnap != "" {
		ed.pushSnapshot(ed.pendingSnap)
		ed.pendingSnap = ""
	}
}

func (ed *Editor) restore(data string) {
	s, err := decodeScenario(data)
	if err != nil {
		return
	}
	ed.Scenario = s
	ed.pruneSelection()
}

func (ed *Editor) pruneSelection() {
	if ed.selNodeID != "" && ed.Scenario.NodeByID(ed.selNodeID) == nil {
		ed.selNodeID = ""
	}
	if ed.selEdgeID != "" && ed.Scenario.EdgeByID(ed.selEdgeID) == nil {
		ed.selEdgeID = ""
	}
	for id := range ed.selected {
		if ed.Scenario.NodeByID(id) == nil {
			delete(ed.selected, id)
		}
	}
}

func (ed *Editor) Undo() {
	cur := ed.encode()
	for len(ed.undoStack) > 0 {
		top := ed.undoStack[len(ed.undoStack)-1]
		ed.undoStack = ed.undoStack[:len(ed.undoStack)-1]
		if top == cur {
			continue
		}
		if cur != "" {
			ed.redoStack = append(ed.redoStack, cur)
		}
		ed.restore(top)
		return
	}
}

func (ed *Editor) Redo() {
	if len(ed.redoStack) == 0 {
		return
	}
	top := ed.redoStack[len(ed.redoStack)-1]
	ed.redoStack = ed.redoStack[:len(ed.redoStack)-1]
	if cur := ed.encode(); cur != "" {
		ed.undoStack = append(ed.undoStack, cur)
	}
	ed.restore(top)
}

func (ed *Editor) copySelection() {
	if len(ed.selected) == 0 {
		return
	}
	var dto scenarioDTO
	copied := make(map[string]bool)
	for _, n := range ed.Scenario.Nodes {
		if ed.selected[n.ID] && n.Kind != KindStart {
			dto.Nodes = append(dto.Nodes, nodeToDTO(n))
			copied[n.ID] = true
		}
	}
	if len(dto.Nodes) == 0 {
		return
	}
	for _, e := range ed.Scenario.Edges {
		if copied[e.From] && copied[e.To] {
			dto.Edges = append(dto.Edges, edgeToDTO(e))
		}
	}
	data, err := json.Marshal(dto)
	if err != nil {
		return
	}
	ed.clipboard = string(data)
}

func (ed *Editor) paste() {
	if ed.clipboard == "" {
		return
	}
	var dto scenarioDTO
	if err := json.Unmarshal([]byte(ed.clipboard), &dto); err != nil {
		return
	}
	if len(dto.Nodes) == 0 {
		return
	}
	ed.pushHistory()
	idMap := make(map[string]string, len(dto.Nodes))
	ed.selected = make(map[string]bool)
	ed.selEdgeID = ""
	for _, nd := range dto.Nodes {
		n := nodeFromDTO(nd)
		old := n.ID
		n.ID = persist.NewRandomID()
		n.X += 28
		n.Y += 28
		idMap[old] = n.ID
		ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
		ed.selected[n.ID] = true
		ed.selNodeID = n.ID
	}
	for _, edto := range dto.Edges {
		from, okF := idMap[edto.From]
		to, okT := idMap[edto.To]
		if !okF || !okT {
			continue
		}
		e := edgeFromDTO(edto)
		e.ID = persist.NewRandomID()
		e.From = from
		e.To = to
		ed.Scenario.Edges = append(ed.Scenario.Edges, e)
	}
	ed.mode = modeProps
}

func (ed *Editor) deleteSelection() {
	if len(ed.selected) == 0 && ed.selEdgeID == "" {
		return
	}
	ed.pushHistory()
	for id := range ed.selected {
		ed.Scenario.RemoveNode(id)
	}
	if ed.selEdgeID != "" {
		ed.Scenario.RemoveEdge(ed.selEdgeID)
	}
	ed.clearSelection()
}

func (ed *Editor) cancelInteraction() {
	if ed.reconnectEdge != nil {
		ed.Scenario.Edges = append(ed.Scenario.Edges, ed.reconnectEdge)
		ed.reconnectEdge = nil
	}
	ed.connectFromID = ""
	ed.connectFromSide = ""
	ed.connectToID = ""
	ed.connectToSide = ""
	if ed.envMenuNodeID != "" || ed.envDropOpen {
		ed.envMenuNodeID = ""
		ed.envDropOpen = false
		return
	}
	ed.clearSelection()
}

func (ed *Editor) EnvDropOpen() bool {
	return ed.envDropOpen || ed.envMenuNodeID != ""
}

func (ed *Editor) CloseEnvDrop() {
	ed.envDropOpen = false
	ed.envMenuNodeID = ""
}

func (ed *Editor) selectedNode() *Node {
	if ed.selNodeID == "" {
		return nil
	}
	return ed.Scenario.NodeByID(ed.selNodeID)
}

func (ed *Editor) selectedEdge() *Edge {
	if ed.selEdgeID == "" {
		return nil
	}
	return ed.Scenario.EdgeByID(ed.selEdgeID)
}

func (ed *Editor) selectOnly(id string) {
	ed.selected = map[string]bool{id: true}
	ed.selNodeID = id
	ed.selEdgeID = ""
	ed.envDropOpen = false
}

func (ed *Editor) clearSelection() {
	ed.selected = make(map[string]bool)
	ed.selNodeID = ""
	ed.selEdgeID = ""
	ed.envDropOpen = false
}

func (ed *Editor) toScreen(p f32.Point) f32.Point {
	return f32.Pt(p.X*ed.zoom+ed.pan.X, p.Y*ed.zoom+ed.pan.Y)
}

func (ed *Editor) toWorld(p f32.Point) f32.Point {
	return f32.Pt((p.X-ed.pan.X)/ed.zoom, (p.Y-ed.pan.Y)/ed.zoom)
}

func (ed *Editor) nodeWH(n *Node) (float32, float32) {
	return nodeSizeWorld(n, ed.nodeW, ed.nodeH)
}

func (ed *Editor) Layout(gtx layout.Context, th *material.Theme, host *Host) layout.Dimensions {
	ed.autosave()
	ed.winH = host.WinSize.Y
	ed.canvasOrig = image.Pt(host.WinSize.X-gtx.Constraints.Max.X, host.WinSize.Y-gtx.Constraints.Max.Y)
	ed.extDrag = host.ExternalDrag
	ed.extDragPos = host.ExternalDragPos
	ed.extDragLabel = host.ExternalDragLabel
	if host.EnvOptions != nil {
		ed.envOpts = host.EnvOptions()
	} else {
		ed.envOpts = nil
	}

	ed.frameEnvs = map[string]map[string]string{}
	if host.ActiveEnv != nil {
		ed.frameEnvs[""] = host.ActiveEnv()
	}
	ed.setVarNames = map[string]bool{}
	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindSetVar {
			if name := n.VarNameEd.Text(); name != "" {
				ed.setVarNames[name] = true
			}
		}
		if n.EnvID != "" && host.EnvVars != nil {
			if _, ok := ed.frameEnvs[n.EnvID]; !ok {
				ed.frameEnvs[n.EnvID] = host.EnvVars(n.EnvID)
			}
		}
	}

	minPanel := gtx.Dp(unit.Dp(56))
	maxPanel := gtx.Constraints.Max.X / 2
	if ed.panelW <= 0 {
		ed.panelW = gtx.Dp(unit.Dp(300))
	}
	if ed.panelW < minPanel {
		ed.panelW = minPanel
	}
	if ed.panelW > maxPanel && maxPanel > minPanel {
		ed.panelW = maxPanel
	}

	var moved bool
	var finalX float32
	for {
		e, ok := ed.panelDrag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		pos := e.Position.X + float32(ed.canvasDrawn)
		switch e.Kind {
		case pointer.Press:
			ed.panelDragX = pos
			ed.panelPx = float32(ed.panelW)
		case pointer.Drag:
			finalX = pos
			moved = true
		}
	}
	if moved {
		ed.panelPx -= finalX - ed.panelDragX
		ed.panelDragX = finalX
		ed.panelW = int(ed.panelPx + 0.5)
		if ed.panelW < minPanel {
			ed.panelW = minPanel
		}
		if ed.panelW > maxPanel && maxPanel > minPanel {
			ed.panelW = maxPanel
		}
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			d := ed.layoutCanvas(gtx, th, host)
			ed.canvasDrawn = d.Size.X
			return d
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			hit := gtx.Dp(unit.Dp(4))
			h := gtx.Constraints.Max.Y
			size := image.Point{X: hit, Y: h}
			lineCol := theme.BorderSubtle
			if ed.panelDrag.Dragging() {
				lineCol = theme.Accent
			}
			paint.FillShape(gtx.Ops, lineCol, clip.Rect{Max: image.Pt(1, h)}.Op())
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			pointer.CursorColResize.Add(gtx.Ops)
			ed.panelDrag.Add(gtx.Ops)
			event.Op(gtx.Ops, &ed.panelDrag)
			for {
				_, ok := gtx.Event(pointer.Filter{Target: &ed.panelDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
				if !ok {
					break
				}
			}
			return layout.Dimensions{Size: size}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = ed.panelW
			gtx.Constraints.Max.X = ed.panelW
			return ed.layoutPanel(gtx, th, host)
		}),
	)
}

func stateColor(st int, idle color.NRGBA) color.NRGBA {
	switch st {
	case StRunning:
		return color.NRGBA{R: 235, G: 180, B: 60, A: 255}
	case StOK:
		return color.NRGBA{R: 70, G: 190, B: 100, A: 255}
	case StFail:
		return theme.Danger
	}
	return idle
}

func kindColor(k NodeKind) color.NRGBA {
	switch k {
	case KindStart:
		return color.NRGBA{R: 70, G: 190, B: 100, A: 255}
	case KindRequest:
		return theme.Accent
	case KindWSRequest:
		return color.NRGBA{R: 255, G: 145, B: 80, A: 255}
	case KindWSSend:
		return color.NRGBA{R: 255, G: 190, B: 130, A: 255}
	case KindGQLRequest:
		return color.NRGBA{R: 225, G: 70, B: 155, A: 255}
	case KindCondition:
		return color.NRGBA{R: 186, G: 85, B: 211, A: 255}
	case KindLoop:
		return color.NRGBA{R: 255, G: 180, B: 0, A: 255}
	case KindDelay:
		return color.NRGBA{R: 13, G: 184, B: 214, A: 255}
	case KindSetVar:
		return color.NRGBA{R: 90, G: 200, B: 170, A: 255}
	case KindNote:
		return color.NRGBA{R: 200, G: 190, B: 120, A: 255}
	}
	return theme.FgMuted
}

func (ed *Editor) portPos(n *Node, side string) f32.Point {
	w, h := ed.nodeWH(n)
	switch side {
	case SideLeft:
		return f32.Pt(n.X, n.Y+ed.nodeH/2)
	case SideTop:
		return f32.Pt(n.X+w/2, n.Y)
	case SideBottom:
		return f32.Pt(n.X+w/2, n.Y+h)
	}
	return f32.Pt(n.X+w, n.Y+ed.nodeH/2)
}

func (ed *Editor) inPort(n *Node) f32.Point {
	return ed.portPos(n, SideLeft)
}

func (ed *Editor) inPortSide(n *Node, side string) f32.Point {
	return ed.portPos(n, normToSide(side))
}

func (ed *Editor) outPortBottom(n *Node) f32.Point {
	return ed.portPos(n, SideBottom)
}

func (ed *Editor) outPortSide(n *Node, side string) f32.Point {
	if n.Kind == KindCondition {
		return ed.outPort(n)
	}
	return ed.portPos(n, normFromSide(side))
}

func (ed *Editor) edgeGeom(e *Edge, from, to *Node) (p0, p1, o0, o1 f32.Point) {
	if from.Kind == KindCondition {
		p0 = ed.edgeOutPos(e, from)
		o0 = f32.Pt(1, 0)
	} else {
		side := normFromSide(e.FromSide)
		p0 = ed.portPos(from, side)
		o0 = sideNormal(side)
	}
	toSide := normToSide(e.ToSide)
	p1 = ed.portPos(to, toSide)
	o1 = sideNormal(toSide)
	return
}

func (ed *Editor) edgesAtPort(n *Node, side string) []*Edge {
	var out []*Edge
	for _, e := range ed.Scenario.Edges {
		switch {
		case e.From == n.ID && n.Kind != KindCondition && normFromSide(e.FromSide) == side:
			out = append(out, e)
		case e.To == n.ID && normToSide(e.ToSide) == side:
			out = append(out, e)
		}
	}
	return out
}

func (ed *Editor) nearestSide(n *Node, pt f32.Point) (string, float32) {
	best := ""
	bestD := float32(math.MaxFloat32)
	for _, side := range allSides {
		if n.Kind == KindCondition && side == SideRight {
			continue
		}
		if d := dist(pt, ed.toScreen(ed.portPos(n, side))); d < bestD {
			best, bestD = side, d
		}
	}
	return best, bestD
}

func (ed *Editor) portAt(n *Node, pt f32.Point) (string, bool) {
	side, d := ed.nearestSide(n, pt)
	return side, side != "" && d <= ed.portHit
}

func (ed *Editor) outEdges(n *Node) []*Edge {
	var out []*Edge
	for _, e := range ed.Scenario.Edges {
		if e.From == n.ID {
			out = append(out, e)
		}
	}
	return out
}

func (ed *Editor) outSlots(n *Node) int {
	if n.Kind != KindCondition {
		return 1
	}
	return len(ed.outEdges(n)) + 1
}

func (ed *Editor) outPortAt(n *Node, slot int) f32.Point {
	w, _ := ed.nodeWH(n)
	total := ed.outSlots(n)
	if total <= 1 {
		return f32.Pt(n.X+w, n.Y+ed.nodeH/2)
	}
	frac := float32(slot+1) / float32(total+1)
	return f32.Pt(n.X+w, n.Y+ed.nodeH*frac)
}

func (ed *Editor) outPort(n *Node) f32.Point {
	return ed.outPortAt(n, ed.outSlots(n)-1)
}

func (ed *Editor) edgeOutPos(e *Edge, from *Node) f32.Point {
	if from.Kind != KindCondition {
		return ed.outPortAt(from, 0)
	}
	for i, oe := range ed.outEdges(from) {
		if oe.ID == e.ID {
			return ed.outPortAt(from, i)
		}
	}
	return ed.outPort(from)
}

func collectPlaceholders(s string, out map[string]bool) {
	for i := 0; ; {
		start, end, ok := widgets.FindVar(s, i)
		if !ok {
			return
		}
		name := strings.TrimSpace(s[start+2 : end-2])
		if name != "" {
			out[name] = true
		}
		i = end
	}
}

func (ed *Editor) missingVars(n *Node) []string {
	used := map[string]bool{}
	switch n.Kind {
	case KindRequest, KindWSRequest, KindGQLRequest, KindWSSend:
		collectPlaceholders(n.URLEd.Text(), used)
		collectPlaceholders(n.HeadersEd.Text(), used)
		collectPlaceholders(n.BodyEd.Text(), used)
		collectPlaceholders(n.CookiesEd.Text(), used)
		collectPlaceholders(n.AuthTokenEd.Text(), used)
		collectPlaceholders(n.AuthUserEd.Text(), used)
		collectPlaceholders(n.AuthPassEd.Text(), used)
		collectPlaceholders(n.VarsEd.Text(), used)
	case KindSetVar:
		collectPlaceholders(n.VarValueEd.Text(), used)
	default:
		return nil
	}
	if len(used) == 0 {
		return nil
	}
	env := ed.frameEnvs[n.EnvID]
	if env == nil {
		env = ed.frameEnvs[""]
	}
	var missing []string
	for name := range used {
		if ed.setVarNames[name] || strings.HasPrefix(name, "loop.") {
			continue
		}
		if _, ok := env[name]; ok {
			continue
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

func dist(a, b f32.Point) float32 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return float32(math.Hypot(float64(dx), float64(dy)))
}

func bezierAt(p0, c0, c1, p1 f32.Point, t float32) f32.Point {
	u := 1 - t
	a := u * u * u
	b := 3 * u * u * t
	c := 3 * u * t * t
	d := t * t * t
	return f32.Pt(
		a*p0.X+b*c0.X+c*c1.X+d*p1.X,
		a*p0.Y+b*c0.Y+c*c1.Y+d*p1.Y,
	)
}

func (ed *Editor) edgeControls(p0, p1 f32.Point) (f32.Point, f32.Point) {
	dx := float32(math.Abs(float64(p1.X-p0.X))) / 2
	if min := 48 * ed.zoom; dx < min {
		dx = min
	}
	return f32.Pt(p0.X+dx, p0.Y), f32.Pt(p1.X-dx, p1.Y)
}

func (ed *Editor) edgeControlsDir(p0, p1, o0, o1 f32.Point) (f32.Point, f32.Point) {
	min := 48 * ed.zoom
	k := func(o f32.Point) float32 {
		var d float32
		if o.X != 0 {
			d = float32(math.Abs(float64(p1.X-p0.X))) / 2
		} else {
			d = float32(math.Abs(float64(p1.Y-p0.Y))) / 2
		}
		if d < min {
			d = min
		}
		return d
	}
	k0, k1 := k(o0), k(o1)
	return f32.Pt(p0.X+o0.X*k0, p0.Y+o0.Y*k0), f32.Pt(p1.X+o1.X*k1, p1.Y+o1.Y*k1)
}

func (ed *Editor) layoutCanvas(gtx layout.Context, th *material.Theme, host *Host) layout.Dimensions {
	size := gtx.Constraints.Max
	ed.canvasSize = size
	ed.nodeW = float32(gtx.Dp(unit.Dp(176)))
	ed.nodeH = float32(gtx.Dp(unit.Dp(56)))
	ed.portHit = float32(gtx.Dp(unit.Dp(12)))

	ed.maybeFitOnShow()

	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: ed},
			key.Filter{Focus: ed, Name: key.NameDeleteForward},
			key.Filter{Focus: ed, Name: key.NameDeleteBackward},
			key.Filter{Focus: ed, Name: key.NameEscape},
			key.Filter{Focus: ed, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: ed, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: ed, Name: "V", Required: key.ModShortcut},
			key.Filter{Focus: ed, Name: "D", Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		e, ok := ev.(key.Event)
		if !ok || e.State != key.Press {
			continue
		}
		switch e.Name {
		case key.NameDeleteForward, key.NameDeleteBackward:
			ed.deleteSelection()
		case key.NameEscape:
			ed.cancelInteraction()
		case "A":
			ed.selected = make(map[string]bool)
			for _, n := range ed.Scenario.Nodes {
				ed.selected[n.ID] = true
				ed.selNodeID = n.ID
			}
			ed.selEdgeID = ""
		case "C":
			ed.copySelection()
		case "V":
			ed.paste()
		case "D":
			ed.copySelection()
			ed.paste()
		}
	}

	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  ed,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll | pointer.Move | pointer.Enter | pointer.Leave,
			ScrollX: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Move, pointer.Enter:
			ed.setHover(e.Position)
		case pointer.Leave:
			ed.hoverOn = false
		case pointer.Press:
			ed.onPress(e)
		case pointer.Drag:
			ed.onDrag(e.Position)
		case pointer.Release, pointer.Cancel:
			ed.onRelease(e.Position)
		case pointer.Scroll:
			notches := 0
			if e.Scroll.Y < 0 {
				notches = 1
			} else if e.Scroll.Y > 0 {
				notches = -1
			}
			if notches != 0 {
				if e.Modifiers.Contain(key.ModCtrl) || e.Modifiers.Contain(key.ModShortcut) {
					notches *= 3
				}
				ed.zoomByNotches(e.Position, notches)
			}
		}
	}

	if ed.wantFocus {
		ed.wantFocus = false
		gtx.Execute(key.FocusCmd{Tag: ed})
	}

	ed.refreshHoverNode()

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: size}.Op())
	ed.drawGrid(gtx, size)
	event.Op(gtx.Ops, ed)

	for _, n := range ed.Scenario.Nodes {
		if n.Kind == KindLoop {
			ed.drawNode(gtx, th, n)
		}
	}
	for _, e := range ed.Scenario.Edges {
		ed.drawEdge(gtx, th, e)
	}
	ed.drawConnectPreview(gtx)
	for _, n := range ed.Scenario.Nodes {
		if n.Kind != KindLoop {
			ed.drawNode(gtx, th, n)
		}
	}
	ed.drawMarquee(gtx)
	ed.drawDropGhost(gtx, th)
	ed.drawEnvMenu(gtx, th)
	ed.drawViewBadges(gtx, th, size)

	return layout.Dimensions{Size: size}
}

func (ed *Editor) drawConnectPreview(gtx layout.Context) {
	var p0, p1, o0, o1 f32.Point
	if from := ed.connectingNode(); from != nil {
		if from.Kind == KindCondition {
			p0 = ed.toScreen(ed.outPort(from))
			o0 = f32.Pt(1, 0)
		} else {
			p0 = ed.toScreen(ed.portPos(from, ed.connectFromSide))
			o0 = sideNormal(ed.connectFromSide)
		}
		p1 = ed.toScreen(ed.connectPos)
		o1 = f32.Pt(-o0.X, -o0.Y)
	} else if to := ed.Scenario.NodeByID(ed.connectToID); to != nil {
		p1 = ed.toScreen(ed.portPos(to, ed.connectToSide))
		o1 = sideNormal(ed.connectToSide)
		p0 = ed.toScreen(ed.connectPos)
		o0 = f32.Pt(-o1.X, -o1.Y)
	} else {
		return
	}
	c0, c1 := ed.edgeControlsDir(p0, p1, o0, o1)
	ed.strokeBezier(gtx, p0, c0, c1, p1, theme.Accent, float32(gtx.Dp(unit.Dp(2)))*ed.zoom)
	if ed.connectToID != "" {
		ed.drawArrowHead(gtx, p1, o1, theme.Accent)
	}
}

func (ed *Editor) drawEnvMenu(gtx layout.Context, th *material.Theme) {
	if ed.envMenuNodeID == "" || len(ed.envOpts) == 0 {
		return
	}
	n := ed.Scenario.NodeByID(ed.envMenuNodeID)
	if n == nil {
		ed.envMenuNodeID = ""
		return
	}

	if len(ed.envBtns) < len(ed.envOpts) {
		ed.envBtns = make([]widget.Clickable, len(ed.envOpts))
	}
	for i, o := range ed.envOpts {
		i, o := i, o
		for ed.envBtns[i].Clicked(gtx) {
			if n.EnvID != o.ID {
				ed.pushHistory()
				n.EnvID = o.ID
			}
			ed.envMenuNodeID = ""
		}
	}

	items := make([]widgets.MenuItem, len(ed.envOpts))
	for i, o := range ed.envOpts {
		name := o.Name
		if o.ID == "" {
			name = "Active environment"
		}
		items[i] = widgets.MenuItem{
			Label:   name,
			Click:   &ed.envBtns[i],
			Checked: n.EnvID == o.ID,
		}
	}

	c0, c1 := ed.envChipRect(n)
	anchor := widgets.MenuAnchor{
		Pt:    image.Pt(int(c0.X), int(c1.Y)+gtx.Dp(unit.Dp(2))),
		Clamp: ed.canvasSize,
	}
	widgets.DeferMenuAt(gtx, th, &ed.envMenuNodeID, anchor, widgets.MenuMinWidthDp, items)
}

func (ed *Editor) drawDropGhost(gtx layout.Context, th *material.Theme) {
	var pos f32.Point
	var label string
	switch {
	case ed.palDragActive:
		gp := widgets.GlobalPointerPos
		pos = f32.Pt(gp.X-float32(ed.canvasOrig.X), gp.Y-float32(ed.canvasOrig.Y))
		label = ed.palDragKind.Title()
	case ed.blockDragActive:
		gp := widgets.GlobalPointerPos
		pos = f32.Pt(gp.X-float32(ed.canvasOrig.X), gp.Y-float32(ed.canvasOrig.Y))
		label = ed.blockDragName
	case ed.extDrag:
		pos = f32.Pt(ed.extDragPos.X-float32(ed.canvasOrig.X), ed.extDragPos.Y-float32(ed.canvasOrig.Y))
		label = ed.extDragLabel
	default:
		return
	}
	if pos.X < 0 || pos.Y < 0 || pos.X > float32(ed.canvasSize.X) || pos.Y > float32(ed.canvasSize.Y) {
		return
	}
	w := int(ed.nodeW * ed.zoom)
	h := int(ed.nodeH * ed.zoom)
	rect := image.Rect(int(pos.X)-w/2, int(pos.Y)-h/2, int(pos.X)+w/2, int(pos.Y)+h/2)
	r := int(float32(gtx.Dp(unit.Dp(6))) * ed.zoom)
	fill := theme.Accent
	fill.A = 26
	paint.FillShape(gtx.Ops, fill, clip.UniformRRect(rect, r).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, theme.Accent, clip.Stroke{Path: clip.UniformRRect(rect, r).Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	if label != "" {
		pad := int(float32(gtx.Dp(unit.Dp(10))) * ed.zoom)
		ed.drawText(gtx, th, image.Pt(rect.Min.X+pad, rect.Min.Y+(h-gtx.Sp(unit.Sp(12*ed.zoom)))/2), w-pad*2, unit.Sp(12*ed.zoom), label, theme.Fg)
	}
}

func zoomLevelBounds() (int, int) {
	lo := int(math.Ceil(math.Log(minZoom) / math.Log(zoomStep)))
	hi := int(math.Floor(math.Log(maxZoom) / math.Log(zoomStep)))
	return lo, hi
}

func (ed *Editor) zoomByNotches(pt f32.Point, notches int) {
	if notches == 0 {
		return
	}
	lo, hi := zoomLevelBounds()
	level := int(math.Round(math.Log(float64(ed.zoom)) / math.Log(zoomStep)))
	level += notches
	if level < lo {
		level = lo
	}
	if level > hi {
		level = hi
	}
	nz := float32(math.Pow(zoomStep, float64(level)))
	if nz == ed.zoom {
		return
	}
	ratio := nz / ed.zoom
	ed.pan = f32.Pt(pt.X-(pt.X-ed.pan.X)*ratio, pt.Y-(pt.Y-ed.pan.Y)*ratio)
	ed.zoom = nz
}

func (ed *Editor) setHover(pt f32.Point) {
	ed.hoverPos = pt
	ed.hoverOn = true
}

func (ed *Editor) nodeHoverHit(n *Node, pt f32.Point) bool {
	sp, nw, nh := ed.nodeScreenRect(n)
	if n.Kind == KindLoop {
		nh = ed.nodeH * ed.zoom
	}
	m := ed.portHit
	if pt.X >= sp.X-m && pt.X <= sp.X+nw+m && pt.Y >= sp.Y-m && pt.Y <= sp.Y+nh+m {
		return true
	}
	if n.Kind == KindLoop {
		return dist(pt, ed.toScreen(ed.outPortBottom(n))) <= m
	}
	return false
}

func (ed *Editor) hoverNodeAt(pt f32.Point) string {
	nodes := ed.Scenario.Nodes
	for i := len(nodes) - 1; i >= 0; i-- {
		n := nodes[i]
		if n.Kind == KindLoop || !n.HasPorts() {
			continue
		}
		if ed.nodeHoverHit(n, pt) {
			return n.ID
		}
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		n := nodes[i]
		if n.Kind != KindLoop {
			continue
		}
		if ed.nodeHoverHit(n, pt) {
			return n.ID
		}
	}
	return ""
}

func (ed *Editor) refreshHoverNode() {
	if !ed.hoverOn {
		ed.hoverNodeID = ""
		return
	}
	ed.hoverNodeID = ed.hoverNodeAt(ed.hoverPos)
}

func (ed *Editor) connecting() bool {
	return ed.connectFromID != "" || ed.connectToID != ""
}

func (ed *Editor) anchoredNodeID() string {
	if ed.connectFromID != "" {
		return ed.connectFromID
	}
	return ed.connectToID
}

func (ed *Editor) showPorts(n *Node) bool {
	if !n.HasPorts() {
		return false
	}
	return ed.hoverNodeID == n.ID || ed.anchoredNodeID() == n.ID
}

func (ed *Editor) canStartOut(n *Node) bool {
	return n.HasPorts() && n.Kind != KindCondition && ed.firstOutEdge(n) == nil
}

func (ed *Editor) firstOutEdge(n *Node) *Edge {
	for _, e := range ed.Scenario.Edges {
		if e.From == n.ID {
			return e
		}
	}
	return nil
}

func (ed *Editor) connectingNode() *Node {
	if ed.connectFromID == "" {
		return nil
	}
	return ed.Scenario.NodeByID(ed.connectFromID)
}

func (ed *Editor) nodeScreenRect(n *Node) (f32.Point, float32, float32) {
	sp := ed.toScreen(f32.Pt(n.X, n.Y))
	w, h := ed.nodeWH(n)
	return sp, w * ed.zoom, h * ed.zoom
}

func (ed *Editor) envChipRect(n *Node) (f32.Point, f32.Point) {
	sp, w, _ := ed.nodeScreenRect(n)
	gap := 4 * ed.zoom
	chipH := 16 * ed.zoom
	return f32.Pt(sp.X, sp.Y-gap-chipH), f32.Pt(sp.X+w, sp.Y-gap)
}

func (ed *Editor) bodyBoxRect(n *Node) (image.Rectangle, bool) {
	if !n.HasBodyBox() {
		return image.Rectangle{}, false
	}
	sp, w, h := ed.nodeScreenRect(n)
	top := sp.Y + ed.nodeH*ed.zoom
	return image.Rect(int(sp.X), int(top), int(sp.X+w), int(sp.Y+h)), true
}

func (ed *Editor) envName(id string) string {
	if id == "" {
		return "active env"
	}
	for _, o := range ed.envOpts {
		if o.ID == id {
			return o.Name
		}
	}
	return "missing env"
}

func (ed *Editor) trySelectNode(n *Node, e pointer.Event, w f32.Point, zIdx int) bool {
	if e.Modifiers.Contain(key.ModShift) {
		if ed.selected[n.ID] {
			delete(ed.selected, n.ID)
			if ed.selNodeID == n.ID {
				ed.selNodeID = ""
			}
		} else {
			ed.selected[n.ID] = true
			ed.selNodeID = n.ID
			ed.selEdgeID = ""
		}
		ed.mode = modeProps
		return true
	}
	if !ed.selected[n.ID] {
		ed.selectOnly(n.ID)
	} else {
		ed.selNodeID = n.ID
		ed.selEdgeID = ""
	}
	ed.mode = modeProps
	now := time.Now()
	if n.ID == ed.lastClickID && now.Sub(ed.lastClickAt) < 400*time.Millisecond && n.Kind != KindStart {
		ed.focusNameID = n.ID
	}
	ed.lastClickAt = now
	ed.lastClickID = n.ID
	ed.pendingSnap = ed.encode()
	ed.dragNodeID = n.ID
	ed.dragMoved = false
	ed.dragOff = w.Sub(f32.Pt(n.X, n.Y))
	ed.dragMembers = ed.dragMembers[:0]
	if n.Kind == KindLoop {
		for _, o := range ed.Scenario.Nodes {
			if o.Kind == KindLoop || ed.selected[o.ID] {
				continue
			}
			if loopContains(n, o, ed.nodeW, ed.nodeH) {
				ed.dragMembers = append(ed.dragMembers, o.ID)
			}
		}
	}
	if zIdx >= 0 && n.Kind != KindLoop {
		nodes := ed.Scenario.Nodes
		ed.Scenario.Nodes = append(append(nodes[:zIdx:zIdx], nodes[zIdx+1:]...), n)
	}
	return true
}

func (ed *Editor) condSlotHit(n *Node) float32 {
	total := ed.outSlots(n)
	spacing := ed.nodeH * ed.zoom / float32(total+1)
	hit := ed.portHit
	if max := spacing * 0.45; hit > max {
		hit = max
	}
	return hit
}

func (ed *Editor) grabEdge(e *Edge, freeTarget bool, w f32.Point) {
	ed.pushHistory()
	ed.Scenario.RemoveEdge(e.ID)
	ed.reconnectEdge = e
	ed.connectPos = w
	ed.connectFromID, ed.connectFromSide = "", ""
	ed.connectToID, ed.connectToSide = "", ""
	if freeTarget {
		ed.connectFromID = e.From
		ed.connectFromSide = normFromSide(e.FromSide)
	} else {
		ed.connectToID = e.To
		ed.connectToSide = normToSide(e.ToSide)
	}
	if ed.selEdgeID == e.ID {
		ed.selEdgeID = ""
	}
}

func (ed *Editor) pressCondSlots(n *Node, pt f32.Point, w f32.Point) bool {
	outs := ed.outEdges(n)
	hit := ed.condSlotHit(n)
	if dist(pt, ed.toScreen(ed.outPortAt(n, len(outs)))) <= hit {
		ed.connectFromID = n.ID
		ed.connectFromSide = SideRight
		ed.connectPos = w
		ed.reconnectEdge = nil
		return true
	}
	for i, oe := range outs {
		if dist(pt, ed.toScreen(ed.outPortAt(n, i))) <= hit {
			ed.grabEdge(oe, true, w)
			return true
		}
	}
	return false
}

func (ed *Editor) pressPorts(n *Node, pt f32.Point, w f32.Point) bool {
	if !n.HasPorts() {
		return false
	}
	if n.Kind == KindCondition && ed.pressCondSlots(n, pt, w) {
		return true
	}
	side, ok := ed.portAt(n, pt)
	if !ok {
		return false
	}
	cands := ed.edgesAtPort(n, side)
	if len(cands) == 0 {
		if !ed.canStartOut(n) {
			return false
		}
		ed.connectFromID = n.ID
		ed.connectFromSide = side
		ed.connectPos = w
		ed.reconnectEdge = nil
		return true
	}
	pick := cands[len(cands)-1]
	for _, e := range cands {
		if e.ID == ed.selEdgeID {
			pick = e
			break
		}
	}
	ed.grabEdge(pick, pick.To == n.ID, w)
	return true
}

func (ed *Editor) onPress(e pointer.Event) {
	pt := e.Position
	w := ed.toWorld(pt)
	ed.setHover(pt)
	ed.wantFocus = true

	if e.Buttons.Contain(pointer.ButtonSecondary) || e.Buttons.Contain(pointer.ButtonTertiary) {
		ed.panning = true
		ed.panStart = pt
		ed.panOrigin = ed.pan
		return
	}

	pp := image.Pt(int(pt.X), int(pt.Y))
	if pp.In(ed.fitBadge) {
		ed.fitView()
		return
	}
	if pp.In(ed.zoomBadge) {
		ed.resetZoom()
		return
	}

	nodes := ed.Scenario.Nodes
	for i := len(nodes) - 1; i >= 0; i-- {
		n := nodes[i]
		if n.Kind == KindLoop {
			continue
		}
		if n.Kind.IsRequest() {
			c0, c1 := ed.envChipRect(n)
			if pt.X >= c0.X && pt.X <= c1.X && pt.Y >= c0.Y && pt.Y <= c1.Y {
				ed.envMenuNodeID = n.ID
				return
			}
		}
		if ed.pressPorts(n, pt, w) {
			return
		}
		if box, ok := ed.bodyBoxRect(n); ok && pp.In(box) {
			ed.wantFocus = false
			if !ed.selected[n.ID] {
				ed.selectOnly(n.ID)
			} else {
				ed.selNodeID = n.ID
				ed.selEdgeID = ""
			}
			ed.mode = modeProps
			return
		}
		sp, nw, nh := ed.nodeScreenRect(n)
		if pt.X >= sp.X && pt.X <= sp.X+nw && pt.Y >= sp.Y && pt.Y <= sp.Y+nh {
			ed.trySelectNode(n, e, w, i)
			return
		}
	}

	if e2, t := ed.edgeHit(pt); e2 != nil {
		switch {
		case t >= 0.8:
			ed.grabEdge(e2, true, w)
		case t <= 0.2:
			ed.grabEdge(e2, false, w)
		default:
			ed.selEdgeID = e2.ID
			ed.selNodeID = ""
			ed.selected = make(map[string]bool)
			ed.mode = modeProps
		}
		return
	}

	for i := len(nodes) - 1; i >= 0; i-- {
		n := nodes[i]
		if n.Kind != KindLoop {
			continue
		}
		sp, nw, nh := ed.nodeScreenRect(n)
		corner := f32.Pt(sp.X+nw, sp.Y+nh)
		if dist(pt, corner) <= ed.portHit*1.2 {
			ed.pendingSnap = ed.encode()
			ed.resizeNodeID = n.ID
			ed.resizeMoved = false
			ed.selectOnly(n.ID)
			ed.mode = modeProps
			return
		}
		if ed.pressPorts(n, pt, w) {
			return
		}
		headerH := ed.nodeH * ed.zoom
		if pt.X >= sp.X && pt.X <= sp.X+nw && pt.Y >= sp.Y && pt.Y <= sp.Y+headerH {
			ed.trySelectNode(n, e, w, -1)
			return
		}
	}

	ed.marquee = true
	ed.marqueeStart = pt
	ed.marqueeCur = pt
}

func (ed *Editor) lastEdgeTo(nodeID string) *Edge {
	for i := len(ed.Scenario.Edges) - 1; i >= 0; i-- {
		if ed.Scenario.Edges[i].To == nodeID {
			return ed.Scenario.Edges[i]
		}
	}
	return nil
}

func (ed *Editor) onDrag(pt f32.Point) {
	ed.setHover(pt)
	switch {
	case ed.connecting():
		ed.connectPos = ed.toWorld(pt)
	case ed.resizeNodeID != "":
		if n := ed.Scenario.NodeByID(ed.resizeNodeID); n != nil {
			w := ed.toWorld(pt)
			minW := ed.nodeW * 1.2
			minH := ed.nodeH * 2
			nw := w.X - n.X
			nh := w.Y - n.Y
			if nw < minW {
				nw = minW
			}
			if nh < minH {
				nh = minH
			}
			if nw != n.W || nh != n.H {
				if !ed.resizeMoved {
					ed.commitPending()
				}
				ed.resizeMoved = true
			}
			n.W = nw
			n.H = nh
		}
	case ed.dragNodeID != "":
		if n := ed.Scenario.NodeByID(ed.dragNodeID); n != nil {
			w := ed.toWorld(pt).Sub(ed.dragOff)
			dx := w.X - n.X
			dy := w.Y - n.Y
			if dx != 0 || dy != 0 {
				if !ed.dragMoved {
					ed.commitPending()
				}
				ed.dragMoved = true
			}
			n.X = w.X
			n.Y = w.Y
			if ed.selected[n.ID] {
				for _, o := range ed.Scenario.Nodes {
					if o.ID != n.ID && ed.selected[o.ID] {
						o.X += dx
						o.Y += dy
					}
				}
			}
			for _, id := range ed.dragMembers {
				if o := ed.Scenario.NodeByID(id); o != nil {
					o.X += dx
					o.Y += dy
				}
			}
		}
	case ed.panning:
		ed.pan = ed.panOrigin.Add(pt.Sub(ed.panStart))
	case ed.marquee:
		ed.marqueeCur = pt
	}
}

func (ed *Editor) dropTargetAt(pt f32.Point, exclude string, allowStart bool) (*Node, string) {
	check := func(n *Node) (string, bool) {
		if n.ID == exclude || !n.HasPorts() || (n.Kind == KindStart && !allowStart) {
			return "", false
		}
		sp, nw, nh := ed.nodeScreenRect(n)
		if n.Kind == KindLoop {
			nh = ed.nodeH * ed.zoom
		}
		side, d := ed.nearestSide(n, pt)
		rectHit := pt.X >= sp.X && pt.X <= sp.X+nw && pt.Y >= sp.Y && pt.Y <= sp.Y+nh
		if side != "" && (d <= ed.portHit*1.5 || rectHit) {
			return side, true
		}
		return "", false
	}
	nodes := ed.Scenario.Nodes
	for i := len(nodes) - 1; i >= 0; i-- {
		if n := nodes[i]; n.Kind != KindLoop {
			if side, ok := check(n); ok {
				return n, side
			}
		}
	}
	for i := len(nodes) - 1; i >= 0; i-- {
		if n := nodes[i]; n.Kind == KindLoop {
			if side, ok := check(n); ok {
				return n, side
			}
		}
	}
	return nil, ""
}

func (ed *Editor) finishConnect(pt f32.Point) {
	e := ed.reconnectEdge
	restore := func() {
		if e != nil {
			ed.Scenario.Edges = append(ed.Scenario.Edges, e)
		}
	}
	attach := func(e *Edge) {
		ed.Scenario.Edges = append(ed.Scenario.Edges, e)
		ed.selEdgeID = e.ID
		ed.selNodeID = ""
		ed.selected = make(map[string]bool)
		ed.mode = modeProps
	}
	if from := ed.connectingNode(); from != nil {
		n, side := ed.dropTargetAt(pt, from.ID, false)
		if n == nil {
			return
		}
		capped := e == nil && from.Kind != KindCondition && ed.firstOutEdge(from) != nil
		if capped || ed.Scenario.HasEdge(from.ID, n.ID) {
			restore()
			return
		}
		if e == nil {
			ed.pushHistory()
			e = NewEdge(from.ID, n.ID)
		} else {
			e.To = n.ID
		}
		e.FromSide = ed.connectFromSide
		if from.Kind == KindCondition {
			e.FromSide = SideRight
		}
		e.ToSide = side
		attach(e)
		return
	}
	to := ed.Scenario.NodeByID(ed.connectToID)
	if to == nil || e == nil {
		return
	}
	n, side := ed.dropTargetAt(pt, to.ID, true)
	if n == nil {
		return
	}
	if (n.Kind != KindCondition && ed.firstOutEdge(n) != nil) || ed.Scenario.HasEdge(n.ID, to.ID) {
		restore()
		return
	}
	e.From = n.ID
	e.FromSide = side
	if n.Kind == KindCondition {
		e.FromSide = SideRight
	}
	e.ToSide = ed.connectToSide
	attach(e)
}

func (ed *Editor) onRelease(pt f32.Point) {
	if ed.connecting() {
		ed.finishConnect(pt)
	}
	if ed.marquee {
		ed.applyMarquee(pt)
	}
	ed.connectFromID = ""
	ed.connectFromSide = ""
	ed.connectToID = ""
	ed.connectToSide = ""
	ed.reconnectEdge = nil
	ed.dragNodeID = ""
	ed.dragMembers = ed.dragMembers[:0]
	ed.dragMoved = false
	ed.resizeNodeID = ""
	ed.resizeMoved = false
	ed.panning = false
	ed.marquee = false
	ed.pendingSnap = ""
}

func (ed *Editor) applyMarquee(pt f32.Point) {
	x0, x1 := ed.marqueeStart.X, pt.X
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	y0, y1 := ed.marqueeStart.Y, pt.Y
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	if x1-x0 < 4 && y1-y0 < 4 {
		ed.clearSelection()
		return
	}
	ed.selected = make(map[string]bool)
	ed.selNodeID = ""
	ed.selEdgeID = ""
	for _, n := range ed.Scenario.Nodes {
		sp, nw, nh := ed.nodeScreenRect(n)
		if n.Kind == KindLoop {
			nh = ed.nodeH * ed.zoom
		}
		if sp.X <= x1 && sp.X+nw >= x0 && sp.Y <= y1 && sp.Y+nh >= y0 {
			ed.selected[n.ID] = true
			ed.selNodeID = n.ID
		}
	}
	if len(ed.selected) > 0 {
		ed.mode = modeProps
	}
}

func (ed *Editor) edgeAt(pt f32.Point) *Edge {
	e, _ := ed.edgeHit(pt)
	return e
}

func (ed *Editor) edgeHit(pt f32.Point) (*Edge, float32) {
	const samples = 28
	hit := ed.portHit * 0.7
	for i := len(ed.Scenario.Edges) - 1; i >= 0; i-- {
		e := ed.Scenario.Edges[i]
		from := ed.Scenario.NodeByID(e.From)
		to := ed.Scenario.NodeByID(e.To)
		if from == nil || to == nil {
			continue
		}
		w0, w1, o0, o1 := ed.edgeGeom(e, from, to)
		p0 := ed.toScreen(w0)
		p1 := ed.toScreen(w1)
		c0, c1 := ed.edgeControlsDir(p0, p1, o0, o1)
		bestT := float32(-1)
		bestD := hit
		for k := 0; k <= samples; k++ {
			t := float32(k) / samples
			if d := dist(pt, bezierAt(p0, c0, c1, p1, t)); d <= bestD {
				bestD = d
				bestT = t
			}
		}
		if bestT >= 0 {
			return e, bestT
		}
	}
	return nil, 0
}

func (ed *Editor) drawGrid(gtx layout.Context, size image.Point) {
	step := int(float32(gtx.Dp(unit.Dp(28))) * ed.zoom)
	if step <= 4 {
		return
	}
	col := theme.Mix(theme.BgDark, theme.Fg, 0.06)
	offX := int(ed.pan.X) % step
	if offX < 0 {
		offX += step
	}
	offY := int(ed.pan.Y) % step
	if offY < 0 {
		offY += step
	}
	for x := offX; x < size.X; x += step {
		paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+1, size.Y)}.Op())
	}
	for y := offY; y < size.Y; y += step {
		paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(0, y), Max: image.Pt(size.X, y+1)}.Op())
	}
}

func (ed *Editor) drawMarquee(gtx layout.Context) {
	if !ed.marquee {
		return
	}
	x0, x1 := int(ed.marqueeStart.X), int(ed.marqueeCur.X)
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	y0, y1 := int(ed.marqueeStart.Y), int(ed.marqueeCur.Y)
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	rect := image.Rect(x0, y0, x1, y1)
	fill := theme.Accent
	fill.A = 30
	paint.FillShape(gtx.Ops, fill, clip.Rect(rect).Op())
	paint.FillShape(gtx.Ops, theme.Accent, clip.Stroke{Path: clip.Rect(rect).Path(), Width: 1}.Op())
}

func (ed *Editor) drawViewBadges(gtx layout.Context, th *material.Theme, size image.Point) {
	pad := gtx.Dp(unit.Dp(6))
	bh := gtx.Sp(unit.Sp(10)) + pad
	x := pad
	badge := func(txt string) image.Rectangle {
		tw := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(10), font.Font{}, txt)
		rect := image.Rect(x, size.Y-bh-pad*2, x+tw+pad*2, size.Y-pad)
		rr := gtx.Dp(unit.Dp(4))
		paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(rect, rr).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
		ed.drawText(gtx, th, image.Pt(rect.Min.X+pad, rect.Min.Y+pad/2+1), tw+pad, unit.Sp(10), txt, theme.FgMuted)
		x = rect.Max.X + pad
		return rect
	}
	ed.fitBadge = badge("fit view")
	ed.zoomBadge = image.Rectangle{}
	if ed.zoom < 0.999 || ed.zoom > 1.001 {
		ed.zoomBadge = badge("zoom " + itoa(int(ed.zoom*100+0.5)) + "% · reset")
	}
}

func (ed *Editor) maybeFitOnShow() {
	if ed.pendingFit && ed.canvasSize.X > 0 && ed.canvasSize.Y > 0 {
		ed.pendingFit = false
		ed.fitView()
	}
}

func (ed *Editor) fitView() {
	if len(ed.Scenario.Nodes) == 0 || ed.canvasSize.X <= 0 || ed.canvasSize.Y <= 0 {
		return
	}
	minX, minY := float32(math.MaxFloat32), float32(math.MaxFloat32)
	maxX, maxY := float32(-math.MaxFloat32), float32(-math.MaxFloat32)
	for _, n := range ed.Scenario.Nodes {
		w, h := ed.nodeWH(n)
		if n.X < minX {
			minX = n.X
		}
		if n.Y < minY {
			minY = n.Y
		}
		if n.X+w > maxX {
			maxX = n.X + w
		}
		if n.Y+h > maxY {
			maxY = n.Y + h
		}
	}
	const margin = 48
	minX -= margin
	minY -= margin
	maxX += margin
	maxY += margin
	z := float32(ed.canvasSize.X) / (maxX - minX)
	if zh := float32(ed.canvasSize.Y) / (maxY - minY); zh < z {
		z = zh
	}
	if z > 1 {
		z = 1
	}
	if z < minZoom {
		z = minZoom
	}
	ed.zoom = z
	cx := (minX + maxX) / 2
	cy := (minY + maxY) / 2
	ed.pan = f32.Pt(float32(ed.canvasSize.X)/2-cx*z, float32(ed.canvasSize.Y)/2-cy*z)
}

func (ed *Editor) resetZoom() {
	c := f32.Pt(float32(ed.canvasSize.X)/2, float32(ed.canvasSize.Y)/2)
	ratio := 1 / ed.zoom
	ed.pan = f32.Pt(c.X-(c.X-ed.pan.X)*ratio, c.Y-(c.Y-ed.pan.Y)*ratio)
	ed.zoom = 1
}

func itoa(v int) string {
	return strconv.Itoa(v)
}

func (ed *Editor) strokeBezier(gtx layout.Context, p0, c0, c1, p1 f32.Point, col color.NRGBA, width float32) {
	if width < 1 {
		width = 1
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(p0)
	p.CubeTo(c0, c1, p1)
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
}

func (ed *Editor) drawEdge(gtx layout.Context, th *material.Theme, e *Edge) {
	from := ed.Scenario.NodeByID(e.From)
	to := ed.Scenario.NodeByID(e.To)
	if from == nil || to == nil {
		return
	}
	w0, w1, o0, o1 := ed.edgeGeom(e, from, to)
	p0 := ed.toScreen(w0)
	p1 := ed.toScreen(w1)
	c0, c1 := ed.edgeControlsDir(p0, p1, o0, o1)

	st := ed.Runner.EdgeState(e.ID)
	col := stateColor(st, theme.BorderLight)
	if e.ID == ed.selEdgeID && st == StIdle {
		col = theme.Accent
	}
	width := float32(gtx.Dp(unit.Dp(2))) * ed.zoom
	if e.ID == ed.selEdgeID {
		width = float32(gtx.Dp(unit.Dp(3))) * ed.zoom
		if st != StIdle {
			glow := theme.Accent
			glow.A = 110
			ed.strokeBezier(gtx, p0, c0, c1, p1, glow, width+float32(gtx.Dp(unit.Dp(3)))*ed.zoom)
		}
	}
	ed.strokeBezier(gtx, p0, c0, c1, p1, col, width)
	ed.drawArrowHead(gtx, p1, o1, col)

	mid := bezierAt(p0, c0, c1, p1, 0.5)
	label := e.Summary()
	lblSp := unit.Sp(10 * ed.zoom)
	txtW := widgets.MeasureTextWidthCached(gtx, th, lblSp, font.Font{}, label)
	padX := int(float32(gtx.Dp(unit.Dp(6))) * ed.zoom)
	padY := int(float32(gtx.Dp(unit.Dp(3))) * ed.zoom)
	bw := txtW + padX*2
	bh := gtx.Sp(lblSp) + padY*2 + gtx.Dp(unit.Dp(2))
	rect := image.Rect(int(mid.X)-bw/2, int(mid.Y)-bh/2, int(mid.X)+bw/2, int(mid.Y)+bh/2)
	rr := gtx.Dp(unit.Dp(4))
	paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(rect, rr).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: clip.UniformRRect(rect, rr).Path(gtx.Ops), Width: 1}.Op())
	ed.drawText(gtx, th, image.Pt(rect.Min.X+padX, rect.Min.Y+padY), bw, lblSp, label, theme.Fg)
}

func (ed *Editor) drawArrowHead(gtx layout.Context, p1, o1 f32.Point, col color.NRGBA) {
	ah := float32(gtx.Dp(unit.Dp(7))) * ed.zoom
	base := f32.Pt(p1.X+o1.X*ah, p1.Y+o1.Y*ah)
	perp := f32.Pt(-o1.Y, o1.X)
	var arr clip.Path
	arr.Begin(gtx.Ops)
	arr.MoveTo(p1)
	arr.LineTo(f32.Pt(base.X+perp.X*ah*0.6, base.Y+perp.Y*ah*0.6))
	arr.LineTo(f32.Pt(base.X-perp.X*ah*0.6, base.Y-perp.Y*ah*0.6))
	arr.Close()
	paint.FillShape(gtx.Ops, col, clip.Outline{Path: arr.End()}.Op())
}

func (ed *Editor) drawBodyBox(gtx layout.Context, th *material.Theme, n *Node, box image.Rectangle) {
	paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect{Min: box.Min, Max: image.Pt(box.Max.X, box.Min.Y+1)}.Op())
	inset := int(6 * ed.zoom)
	if inset < 2 {
		inset = 2
	}
	field := box.Inset(inset)
	if field.Dx() <= 0 || field.Dy() <= 0 {
		return
	}
	e, hint := n.canvasBodyEditor()
	rr := int(4 * ed.zoom)
	paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(field, rr).Op(gtx.Ops))
	bcol := theme.Border
	if gtx.Source.Focused(e) {
		bcol = theme.Accent
	}
	paint.FillShape(gtx.Ops, bcol, clip.Stroke{Path: clip.UniformRRect(field, rr).Path(gtx.Ops), Width: 1}.Op())
	pad := int(4 * ed.zoom)
	if pad < 1 {
		pad = 1
	}
	inner := field.Inset(pad)
	if inner.Dx() <= 0 || inner.Dy() <= 0 {
		return
	}
	defer op.Offset(inner.Min).Push(gtx.Ops).Pop()
	defer clip.Rect{Max: inner.Size()}.Push(gtx.Ops).Pop()
	if ed.zoom < 0.45 {
		line := e.Text()
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		if line == "" {
			line = hint
		}
		ed.drawText(gtx, th, image.Point{}, inner.Dx(), unit.Sp(10*ed.zoom), line, theme.FgDim)
		return
	}
	defer pointer.PassOp{}.Push(gtx.Ops).Pop()
	g := gtx
	g.Constraints = layout.Exact(inner.Size())
	me := material.Editor(th, e, hint)
	me.TextSize = unit.Sp(10 * ed.zoom)
	me.Font.Typeface = widgets.MonoTypeface
	me.HintColor = theme.FgDim
	me.Layout(g)
}

func (ed *Editor) drawText(gtx layout.Context, th *material.Theme, off image.Point, maxW int, size unit.Sp, txt string, col color.NRGBA) {
	defer op.Offset(off).Push(gtx.Ops).Pop()
	g := gtx
	g.Constraints.Min = image.Point{}
	g.Constraints.Max = image.Pt(maxW, gtx.Constraints.Max.Y)
	lbl := material.Label(th, size, txt)
	lbl.Color = col
	lbl.MaxLines = 1
	lbl.Layout(g)
}

func (ed *Editor) drawNode(gtx layout.Context, th *material.Theme, n *Node) {
	sp, nwF, nhF := ed.nodeScreenRect(n)
	w := int(nwF)
	h := int(nhF)
	if sp.X > float32(ed.canvasSize.X) || sp.Y > float32(ed.canvasSize.Y) ||
		sp.X+float32(w) < 0 || sp.Y+float32(h) < 0 {
		return
	}
	x := int(sp.X)
	y := int(sp.Y)
	rect := image.Rect(x, y, x+w, y+h)
	r := int(float32(gtx.Dp(unit.Dp(6))) * ed.zoom)
	headerH := int(ed.nodeH * ed.zoom)

	isLoop := n.Kind == KindLoop
	if isLoop {
		body := theme.BgPopup
		body.A = 130
		paint.FillShape(gtx.Ops, body, clip.UniformRRect(rect, r).Op(gtx.Ops))
		hdr := image.Rect(x, y, x+w, y+headerH)
		paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(hdr, r).Op(gtx.Ops))
		paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect{Min: image.Pt(x, y+headerH-1), Max: image.Pt(x+w, y+headerH)}.Op())
	} else {
		paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(rect, r).Op(gtx.Ops))
	}

	stripeW := int(float32(gtx.Dp(unit.Dp(3))) * ed.zoom)
	if stripeW < 1 {
		stripeW = 1
	}
	stripe := image.Rect(x, y+r, x+stripeW, y+headerH-r)
	paint.FillShape(gtx.Ops, kindColor(n.Kind), clip.Rect(stripe).Op())

	st := ed.Runner.NodeState(n.ID)
	border := stateColor(st, theme.BorderLight)
	bw := float32(gtx.Dp(unit.Dp(1))) * ed.zoom
	if st != StIdle {
		bw = float32(gtx.Dp(unit.Dp(2))) * ed.zoom
	}
	sel := ed.selected[n.ID] || n.ID == ed.selNodeID
	if sel {
		if st == StIdle {
			border = theme.Accent
		}
		bw = float32(gtx.Dp(unit.Dp(2))) * ed.zoom
	}
	if bw < 1 {
		bw = 1
	}
	paint.FillShape(gtx.Ops, border, clip.Stroke{Path: clip.UniformRRect(rect, r).Path(gtx.Ops), Width: bw}.Op())
	if sel {
		gap := int(float32(gtx.Dp(unit.Dp(3)))*ed.zoom + 0.5)
		if gap < 2 {
			gap = 2
		}
		outer := rect.Inset(-gap)
		ow := float32(gtx.Dp(unit.Dp(1))) * ed.zoom
		if ow < 1 {
			ow = 1
		}
		ring := theme.Accent
		if st != StIdle {
			ow *= 1.6
		} else {
			ring.A = 170
		}
		paint.FillShape(gtx.Ops, ring, clip.Stroke{Path: clip.UniformRRect(outer, r+gap).Path(gtx.Ops), Width: ow}.Op())
	}

	padX := int(float32(gtx.Dp(unit.Dp(10))) * ed.zoom)
	ed.drawText(gtx, th, image.Pt(x+padX, y+int(float32(gtx.Dp(unit.Dp(8)))*ed.zoom)), w-padX*2, unit.Sp(12*ed.zoom), n.DisplayName(), theme.Fg)
	ed.drawText(gtx, th, image.Pt(x+padX, y+int(float32(gtx.Dp(unit.Dp(28)))*ed.zoom)), w-padX*2, unit.Sp(10*ed.zoom), n.Summary(), theme.FgMuted)

	if isLoop {
		endLbl := "↻ iteration end · ×"
		if src := strings.TrimSpace(n.LoopSrcEd.Text()); src != "" {
			endLbl = "↻ iteration end · each " + src
		} else if cnt := strings.TrimSpace(n.CountEd.Text()); cnt != "" {
			endLbl += cnt
		} else {
			endLbl += "1"
		}
		loopCol := kindColor(KindLoop)
		lblSp := unit.Sp(9 * ed.zoom)
		footH := gtx.Sp(lblSp) + int(6*ed.zoom)
		footTop := y + h - footH
		if footTop > y+headerH+gtx.Sp(lblSp) {
			cl := clip.UniformRRect(rect, r).Push(gtx.Ops)
			footBg := loopCol
			footBg.A = 22
			paint.FillShape(gtx.Ops, footBg, clip.Rect(image.Rect(x, footTop, x+w, y+h)).Op())
			paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect(image.Rect(x, footTop, x+w, footTop+1)).Op())
			cl.Pop()
			ed.drawText(gtx, th, image.Pt(x+padX, y+headerH+int(3*ed.zoom)), w-padX*2, lblSp, "▶ iteration start", loopCol)
			ed.drawText(gtx, th, image.Pt(x+padX, footTop+int(3*ed.zoom)), w-padX*2, lblSp, endLbl, loopCol)

			ax := float32(x) + float32(stripeW)*2.5
			topY := float32(y+headerH) + float32(gtx.Sp(lblSp)) + 8*ed.zoom
			botY := float32(footTop) - 4*ed.zoom
			if botY > topY+12*ed.zoom {
				lcol := loopCol
				lcol.A = 110
				lw := float32(gtx.Dp(unit.Dp(1)))
				if lw < 1 {
					lw = 1
				}
				var lp clip.Path
				lp.Begin(gtx.Ops)
				lp.MoveTo(f32.Pt(ax, botY))
				lp.LineTo(f32.Pt(ax, topY))
				paint.FillShape(gtx.Ops, lcol, clip.Stroke{Path: lp.End(), Width: lw}.Op())
				ah := 4 * ed.zoom
				if ah < 3 {
					ah = 3
				}
				var ap clip.Path
				ap.Begin(gtx.Ops)
				ap.MoveTo(f32.Pt(ax, topY))
				ap.LineTo(f32.Pt(ax-ah*0.6, topY+ah))
				ap.LineTo(f32.Pt(ax+ah*0.6, topY+ah))
				ap.Close()
				paint.FillShape(gtx.Ops, lcol, clip.Outline{Path: ap.End()}.Op())
			}
		}

		hl := int(float32(gtx.Dp(unit.Dp(10))) * ed.zoom)
		cornerCol := theme.FgMuted
		if n.ID == ed.resizeNodeID || n.ID == ed.selNodeID {
			cornerCol = theme.Accent
		}
		for i := 0; i < 2; i++ {
			off := i * hl / 2
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(float32(x+w-hl+off), float32(y+h)))
			p.LineTo(f32.Pt(float32(x+w), float32(y+h-hl+off)))
			paint.FillShape(gtx.Ops, cornerCol, clip.Stroke{Path: p.End(), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
		}
	}

	chipRight := x
	if n.Kind.IsRequest() {
		c0, c1 := ed.envChipRect(n)
		label := "env: " + ed.envName(n.EnvID)
		lblSp := unit.Sp(9 * ed.zoom)
		tw := widgets.MeasureTextWidthCached(gtx, th, lblSp, font.Font{}, label)
		chipPad := int(4 * ed.zoom)
		chip := image.Rect(int(c0.X), int(c0.Y), int(c0.X)+tw+chipPad*2, int(c1.Y))
		bg := theme.BgPopup
		paint.FillShape(gtx.Ops, bg, clip.UniformRRect(chip, int(4*ed.zoom)).Op(gtx.Ops))
		bcol := theme.BorderSubtle
		if n.EnvID != "" {
			bcol = theme.Accent
		}
		paint.FillShape(gtx.Ops, bcol, clip.Stroke{Path: clip.UniformRRect(chip, int(4*ed.zoom)).Path(gtx.Ops), Width: 1}.Op())
		ed.drawText(gtx, th, image.Pt(chip.Min.X+chipPad, chip.Min.Y+int(2*ed.zoom)), tw+chipPad, lblSp, label, theme.FgMuted)
		chipRight = chip.Max.X
	}

	if info := ed.Runner.NodeInfo(n.ID); info != "" {
		infoCol := stateColor(st, theme.FgMuted)
		infoSp := unit.Sp(10 * ed.zoom)
		infoY := y - gtx.Sp(unit.Sp(11*ed.zoom)) - int(4*ed.zoom)
		infoX := x
		maxW := w
		if chipRight > x {
			tw := widgets.MeasureTextWidthCached(gtx, th, infoSp, font.Font{}, info)
			minX := chipRight + int(6*ed.zoom)
			infoX = x + w - tw
			if infoX < minX {
				infoX = minX
			}
			maxW = x + w - infoX
			if maxW < 1 {
				maxW = 1
			}
		}
		ed.drawText(gtx, th, image.Pt(infoX, infoY), maxW, infoSp, info, infoCol)
	}

	if box, ok := ed.bodyBoxRect(n); ok {
		ed.drawBodyBox(gtx, th, n, box)
	}

	if ed.showPorts(n) {
		portR := int(float32(gtx.Dp(unit.Dp(5))) * ed.zoom)
		if portR < 2 {
			portR = 2
		}
		anchorSide := ""
		switch n.ID {
		case ed.connectFromID:
			anchorSide = ed.connectFromSide
		case ed.connectToID:
			anchorSide = ed.connectToSide
		}
		for _, side := range allSides {
			if n.Kind == KindCondition && side == SideRight {
				continue
			}
			occupied := len(ed.edgesAtPort(n, side)) > 0
			if !occupied && n.Kind == KindStart && !ed.canStartOut(n) {
				continue
			}
			col := theme.FgMuted
			if side == anchorSide {
				col = theme.Accent
			}
			drawPort(gtx, ed.toScreen(ed.portPos(n, side)), portR, col, occupied)
		}
		if n.Kind == KindCondition {
			total := ed.outSlots(n)
			for s := 0; s < total; s++ {
				p := ed.toScreen(ed.outPortAt(n, s))
				if s == total-1 {
					col := kindColor(n.Kind)
					if ed.connectFromID == n.ID {
						col = theme.Accent
					}
					drawPort(gtx, p, portR, col, false)
					arm := float32(portR) * 0.55
					lw := float32(gtx.Dp(unit.Dp(1)))
					if lw < 1 {
						lw = 1
					}
					var ph clip.Path
					ph.Begin(gtx.Ops)
					ph.MoveTo(f32.Pt(p.X-arm, p.Y))
					ph.LineTo(f32.Pt(p.X+arm, p.Y))
					paint.FillShape(gtx.Ops, col, clip.Stroke{Path: ph.End(), Width: lw}.Op())
					var pv clip.Path
					pv.Begin(gtx.Ops)
					pv.MoveTo(f32.Pt(p.X, p.Y-arm))
					pv.LineTo(f32.Pt(p.X, p.Y+arm))
					paint.FillShape(gtx.Ops, col, clip.Stroke{Path: pv.End(), Width: lw}.Op())
				} else {
					drawPort(gtx, p, portR, theme.FgMuted, true)
				}
			}
		}
	}

	if missing := ed.missingVars(n); len(missing) > 0 {
		warn := "⚠ missing: " + strings.Join(missing, ", ")
		warnCol := color.NRGBA{R: 235, G: 180, B: 60, A: 255}
		ed.drawText(gtx, th, image.Pt(x, y+h+int(4*ed.zoom)), w*2, unit.Sp(9*ed.zoom), warn, warnCol)
	}
}

func drawPort(gtx layout.Context, c f32.Point, r int, col color.NRGBA, filled bool) {
	rect := image.Rect(int(c.X)-r, int(c.Y)-r, int(c.X)+r, int(c.Y)+r)
	fill := theme.BgDark
	if filled {
		fill = col
	}
	paint.FillShape(gtx.Ops, fill, clip.Ellipse(rect).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, clip.Stroke{Path: clip.Ellipse(rect).Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(2)))}.Op())
}

func (ed *Editor) viewCenterWorld() f32.Point {
	c := f32.Pt(float32(ed.canvasSize.X)/2, float32(ed.canvasSize.Y)/2)
	return ed.toWorld(c)
}

func (ed *Editor) newNodeAt(kind NodeKind, x, y float32) *Node {
	n := NewNode(kind, x, y)
	if kind == KindLoop {
		n.W = ed.nodeW * 2.4
		n.H = ed.nodeH * 4
	}
	return n
}

func (ed *Editor) addNode(kind NodeKind) {
	ed.pushHistory()
	c := ed.viewCenterWorld()
	off := float32(len(ed.Scenario.Nodes)%5) * 24
	n := ed.newNodeAt(kind, c.X-ed.nodeW/2+off, c.Y-ed.nodeH/2+off)
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	ed.selectOnly(n.ID)
	ed.mode = modeProps
}

func (ed *Editor) windowToCanvas(winPos f32.Point) (f32.Point, bool) {
	local := f32.Pt(winPos.X-float32(ed.canvasOrig.X), winPos.Y-float32(ed.canvasOrig.Y))
	if local.X < 0 || local.Y < 0 || local.X > float32(ed.canvasSize.X) || local.Y > float32(ed.canvasSize.Y) {
		return f32.Point{}, false
	}
	return local, true
}

func (ed *Editor) dropKindAtWindow(kind NodeKind, winPos f32.Point) bool {
	local, ok := ed.windowToCanvas(winPos)
	if !ok {
		return false
	}
	w := ed.toWorld(local)
	ed.pushHistory()
	n := ed.newNodeAt(kind, w.X-ed.nodeW/2, w.Y-ed.nodeH/2)
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	ed.selectOnly(n.ID)
	ed.mode = modeProps
	return true
}

func (ed *Editor) addBlock(id string) {
	dto, err := LoadBlock(id)
	if err != nil {
		ed.note = "Block load failed: " + err.Error()
		return
	}
	ed.insertBlockAt(dto, ed.viewCenterWorld())
}

func (ed *Editor) dropBlockAtWindow(id string, winPos f32.Point) bool {
	local, ok := ed.windowToCanvas(winPos)
	if !ok {
		return false
	}
	dto, err := LoadBlock(id)
	if err != nil {
		ed.note = "Block load failed: " + err.Error()
		return false
	}
	ed.insertBlockAt(dto, ed.toWorld(local))
	return true
}

func (ed *Editor) DropCollectionNode(src *collections.CollectionNode, winPos f32.Point) bool {
	if src == nil {
		return false
	}
	local, ok := ed.windowToCanvas(winPos)
	if !ok {
		return false
	}
	w := ed.toWorld(local)

	if !src.IsFolder && src.Request != nil {
		ed.pushHistory()
		n := ed.nodeFromRequest(src.Name, src.Request, w.X-ed.nodeW/2, w.Y-ed.nodeH/2)
		ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
		ed.selectOnly(n.ID)
		ed.mode = modeProps
		return true
	}

	if src.IsFolder || src.Parent == nil {
		type namedReq struct {
			name string
			req  *model.ParsedRequest
		}
		var reqs []namedReq
		var walk func(n *collections.CollectionNode)
		walk = func(n *collections.CollectionNode) {
			for _, c := range n.Children {
				if c.Request != nil {
					reqs = append(reqs, namedReq{c.Name, c.Request})
				}
				if c.IsFolder {
					walk(c)
				}
			}
		}
		walk(src)
		if len(reqs) == 0 {
			return true
		}
		ed.pushHistory()
		groups := make(map[string][]namedReq)
		for _, r := range reqs {
			groups[r.req.Method] = append(groups[r.req.Method], r)
		}
		var order []string
		for _, m := range methods {
			if _, ok := groups[m]; ok {
				order = append(order, m)
			}
		}
		var rest []string
		for m := range groups {
			known := false
			for _, k := range methods {
				if m == k {
					known = true
					break
				}
			}
			if !known {
				rest = append(rest, m)
			}
		}
		sort.Strings(rest)
		order = append(order, rest...)

		gapX := ed.nodeW + 60
		gapY := ed.nodeH + bodyBoxH(ed.nodeH) + 36
		ed.selected = make(map[string]bool)
		ed.selEdgeID = ""
		for gi, m := range order {
			for ri, r := range groups[m] {
				n := ed.nodeFromRequest(r.name, r.req, w.X+float32(gi)*gapX, w.Y+float32(ri)*gapY)
				ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
				ed.selected[n.ID] = true
				ed.selNodeID = n.ID
			}
		}
		ed.mode = modeProps
		return true
	}
	return false
}

func (ed *Editor) nodeFromRequest(name string, req *model.ParsedRequest, x, y float32) *Node {
	kind := KindRequest
	switch req.Method {
	case "WS":
		kind = KindWSRequest
	case "GRAPHQL":
		kind = KindGQLRequest
	}
	n := NewNode(kind, x, y)
	if name == "" {
		name = "Request"
	}
	n.NameEd.SetText(name)
	if kind == KindRequest && req.Method != "" {
		n.Method = req.Method
	}
	n.URLEd.SetText(req.URL)
	n.BodyEd.SetText(req.Body)
	if len(req.Headers) > 0 {
		keys := make([]string, 0, len(req.Headers))
		for k := range req.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var b []byte
		for i, k := range keys {
			if i > 0 {
				b = append(b, '\n')
			}
			b = append(b, k...)
			b = append(b, ':', ' ')
			b = append(b, req.Headers[k]...)
		}
		n.HeadersEd.SetText(string(b))
	}
	if kind == KindRequest {
		n.BodyType = flowBodyType(req.BodyType)
		switch req.BodyType {
		case model.BodyURLEncoded:
			var lines []string
			for _, kv := range req.URLEncoded {
				if kv.Key == "" || kv.Disabled {
					continue
				}
				lines = append(lines, kv.Key+"="+kv.Value)
			}
			n.BodyEd.SetText(strings.Join(lines, "\n"))
		case model.BodyFormData:
			var lines []string
			for _, fp := range req.FormParts {
				if fp.Key == "" || fp.Disabled {
					continue
				}
				if fp.Kind == model.FormPartFile {
					lines = append(lines, fp.Key+"=@"+fp.FilePath)
				} else {
					lines = append(lines, fp.Key+"="+fp.Value)
				}
			}
			n.BodyEd.SetText(strings.Join(lines, "\n"))
		case model.BodyBinary:
			n.BinPathEd.SetText(req.BinaryPath)
		}
	}
	if req.Auth.Type == "bearer" || req.Auth.Type == "basic" {
		n.AuthType = req.Auth.Type
		n.AuthTokenEd.SetText(req.Auth.Token)
		n.AuthUserEd.SetText(req.Auth.Username)
		n.AuthPassEd.SetText(req.Auth.Password)
	}
	if len(req.Cookies) > 0 {
		var lines []string
		for _, c := range req.Cookies {
			if c.Key == "" {
				continue
			}
			lines = append(lines, c.Key+"="+c.Value)
		}
		n.CookiesEd.SetText(strings.Join(lines, "\n"))
	}
	return n
}

func flowBodyType(bt model.BodyType) string {
	switch bt {
	case model.BodyURLEncoded:
		return "urlencoded"
	case model.BodyFormData:
		return "form"
	case model.BodyBinary:
		return "binary"
	default:
		return "raw"
	}
}
