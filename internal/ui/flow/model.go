package flow

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"tracto/internal/persist"

	"github.com/nanorele/gio/widget"
)

var errEmptyScenario = errors.New("flow: scenario has no nodes")

type NodeKind int

const (
	KindStart NodeKind = iota
	KindRequest
	KindCondition
	KindLoop
	KindDelay
	KindSetVar
	KindNote
)

func (k NodeKind) Title() string {
	switch k {
	case KindStart:
		return "Start"
	case KindRequest:
		return "HTTP Request"
	case KindCondition:
		return "Condition"
	case KindLoop:
		return "Loop"
	case KindDelay:
		return "Delay"
	case KindSetVar:
		return "Set Variable"
	case KindNote:
		return "Note"
	}
	return "Node"
}

type CondKind int

const (
	CondAlways CondKind = iota
	CondStatus
	CondHasResponse
	CondNoResponse
	CondBodyField
	CondArrayCount
	CondBodyValue
)

var CondKinds = []CondKind{CondAlways, CondStatus, CondHasResponse, CondNoResponse, CondBodyField, CondArrayCount, CondBodyValue}

func (c CondKind) Title() string {
	switch c {
	case CondAlways:
		return "Always"
	case CondStatus:
		return "HTTP status"
	case CondHasResponse:
		return "Has response"
	case CondNoResponse:
		return "No response"
	case CondBodyField:
		return "Body has field"
	case CondArrayCount:
		return "Array count"
	case CondBodyValue:
		return "Field value"
	}
	return "Always"
}

type Node struct {
	ID    string
	Kind  NodeKind
	X, Y  float32
	W, H  float32
	EnvID string

	NameEd     widget.Editor
	Method     string
	URLEd      widget.Editor
	HeadersEd  widget.Editor
	BodyEd     widget.Editor
	CountEd    widget.Editor
	DelayEd    widget.Editor
	VarNameEd  widget.Editor
	VarValueEd widget.Editor
	LoopSrcEd  widget.Editor
}

func NewNode(kind NodeKind, x, y float32) *Node {
	n := &Node{
		ID:   persist.NewRandomID(),
		Kind: kind,
		X:    x,
		Y:    y,
	}
	n.NameEd.SingleLine = true
	n.URLEd.SingleLine = true
	n.CountEd.SingleLine = true
	n.DelayEd.SingleLine = true
	n.VarNameEd.SingleLine = true
	n.VarValueEd.SingleLine = true
	n.LoopSrcEd.SingleLine = true
	n.NameEd.SetText(kind.Title())
	switch kind {
	case KindRequest:
		n.Method = "GET"
	case KindLoop:
		n.CountEd.SetText("3")
		n.DelayEd.SetText("0")
	case KindDelay:
		n.DelayEd.SetText("1000")
	case KindNote:
		n.NameEd.SetText("Note")
	}
	return n
}

func (n *Node) DisplayName() string {
	name := strings.TrimSpace(n.NameEd.Text())
	if name == "" {
		return n.Kind.Title()
	}
	return name
}

func (n *Node) Summary() string {
	switch n.Kind {
	case KindStart:
		return "entry point"
	case KindRequest:
		u := strings.TrimSpace(n.URLEd.Text())
		if u == "" {
			u = "no url"
		}
		return n.Method + " " + u
	case KindCondition:
		return "routes by arrow rules"
	case KindLoop:
		var s string
		if src := strings.TrimSpace(n.LoopSrcEd.Text()); src != "" {
			s = "for each " + src
		} else {
			c := strings.TrimSpace(n.CountEd.Text())
			if c == "" {
				c = "1"
			}
			s = "repeat " + c + "×"
		}
		if d := strings.TrimSpace(n.DelayEd.Text()); d != "" && d != "0" {
			s += " · " + d + " ms"
		}
		return s
	case KindDelay:
		d := strings.TrimSpace(n.DelayEd.Text())
		if d == "" {
			d = "0"
		}
		return d + " ms"
	case KindSetVar:
		name := strings.TrimSpace(n.VarNameEd.Text())
		if name == "" {
			name = "var"
		}
		val := strings.TrimSpace(n.VarValueEd.Text())
		return name + " = " + val
	case KindNote:
		line := strings.TrimSpace(n.BodyEd.Text())
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		return line
	}
	return ""
}

func (n *Node) HasPorts() bool {
	return n.Kind != KindNote
}

func nodeSizeWorld(n *Node, defW, defH float32) (float32, float32) {
	if n.Kind == KindLoop {
		w, h := n.W, n.H
		if w <= 0 {
			w = defW * 2.4
		}
		if h <= 0 {
			h = defH * 4
		}
		return w, h
	}
	return defW, defH
}

func loopContains(loop, n *Node, defW, defH float32) bool {
	if loop.Kind != KindLoop || n == loop {
		return false
	}
	lw, lh := nodeSizeWorld(loop, defW, defH)
	nw, nh := nodeSizeWorld(n, defW, defH)
	cx := n.X + nw/2
	cy := n.Y + nh/2
	return cx >= loop.X && cx <= loop.X+lw && cy >= loop.Y+defH && cy <= loop.Y+lh
}

type Edge struct {
	ID   string
	From string
	To   string

	Cond    CondKind
	ValueEd widget.Editor
	Op      string
	CountEd widget.Editor
	Val2Ed  widget.Editor
}

func NewEdge(from, to string) *Edge {
	e := &Edge{
		ID:   persist.NewRandomID(),
		From: from,
		To:   to,
		Cond: CondAlways,
		Op:   ">",
	}
	e.ValueEd.SingleLine = true
	e.CountEd.SingleLine = true
	e.Val2Ed.SingleLine = true
	e.CountEd.SetText("0")
	return e
}

func (e *Edge) Summary() string {
	val := strings.TrimSpace(e.ValueEd.Text())
	switch e.Cond {
	case CondAlways:
		return "Always"
	case CondStatus:
		if val == "" {
			val = "2xx"
		}
		return "HTTP " + val
	case CondHasResponse:
		return "Has response"
	case CondNoResponse:
		return "No response"
	case CondBodyField:
		if val == "" {
			val = "field"
		}
		return "Has " + val
	case CondArrayCount:
		if val == "" {
			val = "field"
		}
		cnt := strings.TrimSpace(e.CountEd.Text())
		if cnt == "" {
			cnt = "0"
		}
		return "len(" + val + ") " + e.Op + " " + cnt
	case CondBodyValue:
		if val == "" {
			val = "field"
		}
		op := e.Op
		if op == "" {
			op = "=="
		}
		return val + " " + op + " " + strings.TrimSpace(e.Val2Ed.Text())
	}
	return "Always"
}

type Scenario struct {
	ID     string
	NameEd widget.Editor
	Nodes  []*Node
	Edges  []*Edge
}

func NewScenario() *Scenario {
	s := &Scenario{ID: persist.NewRandomID()}
	s.NameEd.SingleLine = true
	s.Nodes = append(s.Nodes, NewNode(KindStart, 80, 200))
	return s
}

func (s *Scenario) NodeByID(id string) *Node {
	for _, n := range s.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (s *Scenario) EdgeByID(id string) *Edge {
	for _, e := range s.Edges {
		if e.ID == id {
			return e
		}
	}
	return nil
}

func (s *Scenario) HasEdge(from, to string) bool {
	for _, e := range s.Edges {
		if e.From == from && e.To == to {
			return true
		}
	}
	return false
}

func (s *Scenario) RemoveNode(id string) {
	for i, n := range s.Nodes {
		if n.ID == id {
			if n.Kind == KindStart {
				return
			}
			s.Nodes = append(s.Nodes[:i], s.Nodes[i+1:]...)
			break
		}
	}
	kept := s.Edges[:0]
	for _, e := range s.Edges {
		if e.From != id && e.To != id {
			kept = append(kept, e)
		}
	}
	s.Edges = kept
}

func (s *Scenario) RemoveEdge(id string) {
	for i, e := range s.Edges {
		if e.ID == id {
			s.Edges = append(s.Edges[:i], s.Edges[i+1:]...)
			return
		}
	}
}

type nodeDTO struct {
	ID       string  `json:"id"`
	Kind     int     `json:"kind"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	W        float32 `json:"w,omitempty"`
	H        float32 `json:"h,omitempty"`
	EnvID    string  `json:"env_id,omitempty"`
	Name     string  `json:"name,omitempty"`
	Method   string  `json:"method,omitempty"`
	URL      string  `json:"url,omitempty"`
	Headers  string  `json:"headers,omitempty"`
	Body     string  `json:"body,omitempty"`
	Count    string  `json:"count,omitempty"`
	DelayMs  string  `json:"delay_ms,omitempty"`
	VarName  string  `json:"var_name,omitempty"`
	VarValue string  `json:"var_value,omitempty"`
	LoopSrc  string  `json:"loop_src,omitempty"`
}

type edgeDTO struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Cond   int    `json:"cond"`
	Value  string `json:"value,omitempty"`
	Op     string `json:"op,omitempty"`
	Count  string `json:"count,omitempty"`
	Value2 string `json:"value2,omitempty"`
}

type scenarioDTO struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Nodes []nodeDTO `json:"nodes"`
	Edges []edgeDTO `json:"edges"`
}

func nodeToDTO(n *Node) nodeDTO {
	return nodeDTO{
		ID:       n.ID,
		Kind:     int(n.Kind),
		X:        n.X,
		Y:        n.Y,
		W:        n.W,
		H:        n.H,
		EnvID:    n.EnvID,
		Name:     n.NameEd.Text(),
		Method:   n.Method,
		URL:      n.URLEd.Text(),
		Headers:  n.HeadersEd.Text(),
		Body:     n.BodyEd.Text(),
		Count:    n.CountEd.Text(),
		DelayMs:  n.DelayEd.Text(),
		VarName:  n.VarNameEd.Text(),
		VarValue: n.VarValueEd.Text(),
		LoopSrc:  n.LoopSrcEd.Text(),
	}
}

func nodeFromDTO(nd nodeDTO) *Node {
	n := NewNode(NodeKind(nd.Kind), nd.X, nd.Y)
	if nd.ID != "" {
		n.ID = nd.ID
	}
	n.W = nd.W
	n.H = nd.H
	n.EnvID = nd.EnvID
	n.NameEd.SetText(nd.Name)
	if nd.Method != "" {
		n.Method = nd.Method
	}
	n.URLEd.SetText(nd.URL)
	n.HeadersEd.SetText(nd.Headers)
	n.BodyEd.SetText(nd.Body)
	n.CountEd.SetText(nd.Count)
	n.DelayEd.SetText(nd.DelayMs)
	n.VarNameEd.SetText(nd.VarName)
	n.VarValueEd.SetText(nd.VarValue)
	n.LoopSrcEd.SetText(nd.LoopSrc)
	return n
}

func edgeToDTO(e *Edge) edgeDTO {
	return edgeDTO{
		ID:     e.ID,
		From:   e.From,
		To:     e.To,
		Cond:   int(e.Cond),
		Value:  e.ValueEd.Text(),
		Op:     e.Op,
		Count:  e.CountEd.Text(),
		Value2: e.Val2Ed.Text(),
	}
}

func edgeFromDTO(ed edgeDTO) *Edge {
	e := NewEdge(ed.From, ed.To)
	if ed.ID != "" {
		e.ID = ed.ID
	}
	e.Cond = CondKind(ed.Cond)
	e.ValueEd.SetText(ed.Value)
	if ed.Op != "" {
		e.Op = ed.Op
	}
	if ed.Count != "" {
		e.CountEd.SetText(ed.Count)
	}
	e.Val2Ed.SetText(ed.Value2)
	return e
}

func (s *Scenario) toDTO() scenarioDTO {
	dto := scenarioDTO{ID: s.ID, Name: strings.TrimSpace(s.NameEd.Text())}
	for _, n := range s.Nodes {
		dto.Nodes = append(dto.Nodes, nodeToDTO(n))
	}
	for _, e := range s.Edges {
		dto.Edges = append(dto.Edges, edgeToDTO(e))
	}
	return dto
}

var changeSeq atomic.Int64

func init() {
	changeSeq.Store(1)
}

func ChangeSeq() int64 {
	return changeSeq.Load()
}

func (s *Scenario) Save() error {
	data, err := json.MarshalIndent(s.toDTO(), "", "  ")
	if err != nil {
		return err
	}
	if err := persist.AtomicWriteFile(filepath.Join(persist.FlowsDir(), s.ID+".json"), data); err != nil {
		return err
	}
	changeSeq.Add(1)
	return nil
}

func readScenarioDTO(id string) (scenarioDTO, error) {
	var dto scenarioDTO
	data, err := os.ReadFile(filepath.Join(persist.FlowsDir(), id+".json"))
	if err != nil {
		return dto, err
	}
	err = json.Unmarshal(data, &dto)
	return dto, err
}

func writeScenarioDTO(dto scenarioDTO) error {
	data, err := json.MarshalIndent(dto, "", "  ")
	if err != nil {
		return err
	}
	if err := persist.AtomicWriteFile(filepath.Join(persist.FlowsDir(), dto.ID+".json"), data); err != nil {
		return err
	}
	changeSeq.Add(1)
	return nil
}

func RenameScenario(id, name string) error {
	dto, err := readScenarioDTO(id)
	if err != nil {
		return err
	}
	dto.Name = strings.TrimSpace(name)
	return writeScenarioDTO(dto)
}

func DuplicateScenario(id string) (string, error) {
	dto, err := readScenarioDTO(id)
	if err != nil {
		return "", err
	}
	dto.ID = persist.NewRandomID()
	if dto.Name == "" {
		dto.Name = "Untitled"
	}
	dto.Name += " Copy"
	if err := writeScenarioDTO(dto); err != nil {
		return "", err
	}
	return dto.ID, nil
}

func ImportScenario(data []byte) (string, error) {
	var dto scenarioDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return "", err
	}
	if len(dto.Nodes) == 0 {
		return "", errEmptyScenario
	}
	dto.ID = persist.NewRandomID()
	if err := writeScenarioDTO(dto); err != nil {
		return "", err
	}
	return dto.ID, nil
}

func encodeScenario(s *Scenario) (string, error) {
	data, err := json.Marshal(s.toDTO())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeScenario(data string) (*Scenario, error) {
	var dto scenarioDTO
	if err := json.Unmarshal([]byte(data), &dto); err != nil {
		return nil, err
	}
	return scenarioFromDTO(dto), nil
}

func scenarioFromDTO(dto scenarioDTO) *Scenario {
	s := &Scenario{ID: dto.ID}
	if s.ID == "" {
		s.ID = persist.NewRandomID()
	}
	s.NameEd.SingleLine = true
	s.NameEd.SetText(dto.Name)
	for _, nd := range dto.Nodes {
		s.Nodes = append(s.Nodes, nodeFromDTO(nd))
	}
	for _, ed := range dto.Edges {
		if s.NodeByID(ed.From) == nil || s.NodeByID(ed.To) == nil {
			continue
		}
		s.Edges = append(s.Edges, edgeFromDTO(ed))
	}
	hasStart := false
	for _, n := range s.Nodes {
		if n.Kind == KindStart {
			hasStart = true
			break
		}
	}
	if !hasStart {
		s.Nodes = append([]*Node{NewNode(KindStart, 80, 200)}, s.Nodes...)
	}
	return s
}

type ScenarioInfo struct {
	ID   string
	Name string
	mod  int64
}

func ListScenarios() []ScenarioInfo {
	dir := persist.FlowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []ScenarioInfo
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			continue
		}
		var dto scenarioDTO
		if err := json.Unmarshal(data, &dto); err != nil || dto.ID == "" {
			continue
		}
		var mod int64
		if info, err := ent.Info(); err == nil {
			mod = info.ModTime().UnixNano()
		}
		out = append(out, ScenarioInfo{ID: dto.ID, Name: dto.Name, mod: mod})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].mod > out[j].mod })
	return out
}

func LoadScenario(id string) (*Scenario, error) {
	data, err := os.ReadFile(filepath.Join(persist.FlowsDir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var dto scenarioDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return nil, err
	}
	return scenarioFromDTO(dto), nil
}

func DeleteScenario(id string) error {
	if err := os.Remove(filepath.Join(persist.FlowsDir(), id+".json")); err != nil {
		return err
	}
	changeSeq.Add(1)
	return nil
}

func LoadLatest() *Scenario {
	dir := persist.FlowsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return NewScenario()
	}
	var bestPath string
	var bestMod int64
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			continue
		}
		info, err := ent.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); bestPath == "" || mod > bestMod {
			bestPath = filepath.Join(dir, ent.Name())
			bestMod = mod
		}
	}
	if bestPath == "" {
		return NewScenario()
	}
	data, err := os.ReadFile(bestPath)
	if err != nil {
		return NewScenario()
	}
	var dto scenarioDTO
	if err := json.Unmarshal(data, &dto); err != nil {
		return NewScenario()
	}
	return scenarioFromDTO(dto)
}
