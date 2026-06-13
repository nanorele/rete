package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tracto/internal/ui/settings"
	"tracto/internal/utils"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget"
)

const (
	StIdle = iota
	StRunning
	StOK
	StFail
)

const (
	maxRunSteps    = 10000
	maxBodyBytes   = 8 << 20
	maxHistoryBody = 64 << 10
	maxHistoryRuns = 20
)

type execEdge struct {
	id     string
	to     string
	cond   CondKind
	value  string
	op     string
	count  int
	value2 string
}

type execNode struct {
	id       string
	kind     NodeKind
	name     string
	method   string
	url      string
	headers  [][2]string
	body     string
	env      map[string]string
	count    int
	delay    time.Duration
	varName  string
	varValue string
	loopSrc  string
	entries  []string
	outs     []execEdge
}

type stepResult struct {
	hasResp    bool
	status     int
	body       []byte
	headers    http.Header
	failed     bool
	errMsg     string
	jsonVal    interface{}
	jsonParsed bool
}

type RunEntry struct {
	Node     string
	Detail   string
	Code     int
	Status   string
	OK       bool
	Body     string
	BodyLen  int
	Dur      time.Duration
	Expanded bool
	Click    widget.Clickable
}

type RunRecord struct {
	Label   string
	Seq     int
	Clock   string
	Dur     time.Duration
	Done    bool
	Failed  bool
	Stopped bool
	SelBtn  widget.Clickable
	entries []*RunEntry
}

type Runner struct {
	mu       sync.Mutex
	running  bool
	paused   bool
	stepMode bool
	stepCh   chan struct{}
	status   string
	nodeSt   map[string]int
	nodeInfo map[string]string
	edgeSt   map[string]int
	cancel   context.CancelFunc
	runs     []*RunRecord
	runSeq   int
}

func NewRunner() *Runner {
	return &Runner{
		nodeSt:   make(map[string]int),
		nodeInfo: make(map[string]string),
		edgeSt:   make(map[string]int),
	}
}

func (r *Runner) Running() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

func (r *Runner) Status() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status
}

func (r *Runner) NodeState(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodeSt[id]
}

func (r *Runner) NodeInfo(id string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nodeInfo[id]
}

func (r *Runner) EdgeState(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.edgeSt[id]
}

func (r *Runner) Runs() []*RunRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*RunRecord, len(r.runs))
	copy(out, r.runs)
	return out
}

func (r *Runner) Entries(rec *RunRecord) []*RunEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*RunEntry, len(rec.entries))
	copy(out, rec.entries)
	return out
}

func (r *Runner) LatestRun() *RunRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.runs) == 0 {
		return nil
	}
	return r.runs[len(r.runs)-1]
}

func (r *Runner) Stop() {
	r.mu.Lock()
	cancel := r.cancel
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (r *Runner) StepMode() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stepMode
}

func (r *Runner) Paused() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.paused
}

func (r *Runner) SetStepMode(on bool) {
	r.mu.Lock()
	r.stepMode = on
	ch := r.stepCh
	r.mu.Unlock()
	if !on && ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (r *Runner) Step() {
	r.mu.Lock()
	ch := r.stepCh
	r.mu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (r *Runner) Reset() {
	r.mu.Lock()
	r.nodeSt = make(map[string]int)
	r.nodeInfo = make(map[string]string)
	r.edgeSt = make(map[string]int)
	r.status = ""
	r.mu.Unlock()
}

func (r *Runner) setNode(id string, st int) {
	r.mu.Lock()
	r.nodeSt[id] = st
	r.mu.Unlock()
}

func (r *Runner) setNodeInfo(id, info string) {
	r.mu.Lock()
	r.nodeInfo[id] = info
	r.mu.Unlock()
}

func (r *Runner) setEdge(id string, st int) {
	r.mu.Lock()
	r.edgeSt[id] = st
	r.mu.Unlock()
}

func (r *Runner) addEntry(rec *RunRecord, ent *RunEntry) {
	r.mu.Lock()
	rec.entries = append(rec.entries, ent)
	r.mu.Unlock()
}

func parseEditorInt(s string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

func buildPlan(s *Scenario, activeEnv map[string]string, envVars func(id string) map[string]string, defW, defH float32) (map[string]*execNode, string) {
	plan := make(map[string]*execNode, len(s.Nodes))
	startID := ""
	for _, n := range s.Nodes {
		env := activeEnv
		if n.EnvID != "" && envVars != nil {
			if m := envVars(n.EnvID); m != nil {
				env = m
			}
		}
		en := &execNode{
			id:       n.ID,
			kind:     n.Kind,
			name:     n.DisplayName(),
			method:   n.Method,
			url:      strings.TrimSpace(n.URLEd.Text()),
			body:     n.BodyEd.Text(),
			env:      env,
			varName:  strings.TrimSpace(n.VarNameEd.Text()),
			varValue: strings.TrimSpace(n.VarValueEd.Text()),
			loopSrc:  strings.TrimSpace(n.LoopSrcEd.Text()),
		}
		for _, line := range strings.Split(n.HeadersEd.Text(), "\n") {
			k, v, ok := strings.Cut(line, ":")
			k = strings.TrimSpace(k)
			if !ok || k == "" {
				continue
			}
			en.headers = append(en.headers, [2]string{k, strings.TrimSpace(v)})
		}
		if c := parseEditorInt(n.CountEd.Text(), 1); c > 0 {
			en.count = c
		} else {
			en.count = 1
		}
		if ms := parseEditorInt(n.DelayEd.Text(), 0); ms > 0 {
			en.delay = time.Duration(ms) * time.Millisecond
		}
		if n.Kind == KindStart && startID == "" {
			startID = n.ID
		}
		plan[n.ID] = en
	}

	loopMembers := make(map[string]map[string]bool)
	for _, loop := range s.Nodes {
		if loop.Kind != KindLoop {
			continue
		}
		members := make(map[string]bool)
		for _, n := range s.Nodes {
			if n == loop || n.Kind == KindLoop || n.Kind == KindStart || n.Kind == KindNote {
				continue
			}
			if loopContains(loop, n, defW, defH) {
				members[n.ID] = true
			}
		}
		loopMembers[loop.ID] = members
	}

	for _, e := range s.Edges {
		from := plan[e.From]
		if from == nil || plan[e.To] == nil {
			continue
		}
		from.outs = append(from.outs, execEdge{
			id:     e.ID,
			to:     e.To,
			cond:   e.Cond,
			value:  strings.TrimSpace(e.ValueEd.Text()),
			op:     e.Op,
			count:  parseEditorInt(e.CountEd.Text(), 0),
			value2: strings.TrimSpace(e.Val2Ed.Text()),
		})
	}

	for loopID, members := range loopMembers {
		loop := plan[loopID]
		for id := range members {
			hasInternalIn := false
			for _, e := range s.Edges {
				if e.To == id && members[e.From] {
					hasInternalIn = true
					break
				}
			}
			if !hasInternalIn {
				loop.entries = append(loop.entries, id)
			}
		}
		entries := loop.entries
		for i := 0; i < len(entries); i++ {
			for j := i + 1; j < len(entries); j++ {
				a := s.NodeByID(entries[i])
				b := s.NodeByID(entries[j])
				if a != nil && b != nil && (b.Y < a.Y || (b.Y == a.Y && b.X < a.X)) {
					entries[i], entries[j] = entries[j], entries[i]
				}
			}
		}
	}

	return plan, startID
}

func expandVars(input string, env, vars map[string]string) string {
	if (env == nil && vars == nil) || !strings.Contains(input, "{{") {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); {
		start := strings.Index(input[i:], "{{")
		if start == -1 {
			b.WriteString(input[i:])
			break
		}
		b.WriteString(input[i : i+start])
		rest := input[i+start:]
		end := strings.Index(rest[2:], "}}")
		if end == -1 {
			b.WriteString(rest)
			break
		}
		end += 4
		k := strings.TrimSpace(rest[2 : end-2])
		if v, ok := vars[k]; ok {
			b.WriteString(v)
		} else if v, ok := env[k]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(rest[:end])
		}
		i += start + end
	}
	return b.String()
}

func (r *Runner) Start(parent context.Context, win *app.Window, s *Scenario, activeEnv map[string]string, envVars func(id string) map[string]string, defW, defH float32) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	plan, startID := buildPlan(s, activeEnv, envVars, defW, defH)
	if startID == "" {
		r.status = "No start node"
		r.mu.Unlock()
		return
	}
	if len(plan[startID].outs) == 0 {
		r.status = "Start node has no outgoing arrows"
		r.mu.Unlock()
		return
	}
	r.running = true
	r.paused = false
	r.status = "Running..."
	r.nodeSt = make(map[string]int)
	r.nodeInfo = make(map[string]string)
	r.edgeSt = make(map[string]int)
	r.stepCh = make(chan struct{}, 1)
	r.runSeq++
	clock := time.Now().Format("15:04:05")
	rec := &RunRecord{
		Label: fmt.Sprintf("Run %d · %s", r.runSeq, clock),
		Seq:   r.runSeq,
		Clock: clock,
	}
	r.runs = append(r.runs, rec)
	if len(r.runs) > maxHistoryRuns {
		r.runs = r.runs[1:]
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	r.cancel = cancel
	r.mu.Unlock()
	win.Invalidate()

	started := time.Now()

	go func() {
		defer cancel()
		steps := 0
		anyFail := false
		limitHit := false
		okReq := 0
		failReq := 0
		vars := make(map[string]string)

		var visit func(id string, in *stepResult)

		waitStep := func(n *execNode) {
			r.mu.Lock()
			step := r.stepMode
			ch := r.stepCh
			r.mu.Unlock()
			if !step || n.kind == KindStart {
				return
			}
			r.mu.Lock()
			r.paused = true
			r.status = "Paused · " + n.name
			r.mu.Unlock()
			win.Invalidate()
			select {
			case <-ctx.Done():
			case <-ch:
			}
			r.mu.Lock()
			r.paused = false
			if ctx.Err() == nil {
				r.status = "Running..."
			}
			r.mu.Unlock()
			win.Invalidate()
		}

		followOuts := func(n *execNode, res *stepResult) {
			for _, oe := range n.outs {
				if ctx.Err() != nil {
					return
				}
				pass := evalCond(oe, res, n.env, vars)
				if pass {
					r.setEdge(oe.id, StOK)
				} else {
					r.setEdge(oe.id, StFail)
				}
				win.Invalidate()
				if pass {
					visit(oe.to, res)
				}
			}
		}

		visit = func(id string, in *stepResult) {
			if ctx.Err() != nil {
				return
			}
			if steps >= maxRunSteps {
				limitHit = true
				return
			}
			steps++
			n := plan[id]
			if n == nil || n.kind == KindNote {
				return
			}
			r.setNode(id, StRunning)
			win.Invalidate()
			waitStep(n)
			if ctx.Err() != nil {
				return
			}

			res := in
			switch n.kind {
			case KindRequest:
				reqStart := time.Now()
				rr := runHTTP(ctx, n, vars)
				res = &rr
				info := "ERR"
				if rr.hasResp {
					info = strconv.Itoa(rr.status)
				} else if rr.errMsg != "" {
					info = "ERR: " + rr.errMsg
				}
				r.setNodeInfo(id, info)
				ent := &RunEntry{
					Node:    n.name,
					Detail:  n.method + " " + expandVars(n.url, n.env, vars),
					Code:    rr.status,
					OK:      !rr.failed,
					Dur:     time.Since(reqStart),
					BodyLen: len(rr.body),
				}
				switch {
				case rr.hasResp:
					ent.Status = strconv.Itoa(rr.status) + " " + http.StatusText(rr.status)
					if rr.errMsg != "" {
						ent.Status += " · " + rr.errMsg
					}
					body := rr.body
					if len(body) > maxHistoryBody {
						body = body[:maxHistoryBody]
					}
					ent.Body = utils.SanitizeText(string(body))
				case rr.errMsg != "":
					ent.Status = rr.errMsg
				default:
					ent.Status = "no response"
				}
				r.addEntry(rec, ent)
			case KindDelay:
				select {
				case <-ctx.Done():
				case <-time.After(n.delay):
				}
			case KindSetVar:
				if n.varName == "" {
					r.setNodeInfo(id, "no variable name")
				} else {
					name := expandVars(n.varName, n.env, vars)
					raw := n.varValue
					switch {
					case strings.HasPrefix(raw, "$header."):
						hname := strings.TrimPrefix(raw, "$header.")
						if in != nil && in.hasResp && in.headers != nil {
							vars[name] = in.headers.Get(hname)
							r.setNodeInfo(id, name+" set")
						} else {
							vars[name] = ""
							r.setNodeInfo(id, "no response for "+raw)
						}
					case raw == "$status":
						if in != nil && in.hasResp {
							vars[name] = strconv.Itoa(in.status)
							r.setNodeInfo(id, name+" set")
						} else {
							vars[name] = ""
							r.setNodeInfo(id, "no response for $status")
						}
					case strings.HasPrefix(raw, "$."):
						if v, ok := jsonPath(in, strings.TrimPrefix(raw, "$.")); ok {
							vars[name] = stringifyJSON(v)
							r.setNodeInfo(id, name+" set")
						} else {
							vars[name] = ""
							r.setNodeInfo(id, "path not found: "+raw)
						}
					default:
						vars[name] = expandVars(raw, n.env, vars)
						r.setNodeInfo(id, name+" set")
					}
				}
			}

			ok := ctx.Err() == nil && !(res != nil && res.failed && n.kind == KindRequest)
			if !ok {
				anyFail = true
			}
			if n.kind == KindRequest {
				if ok {
					okReq++
				} else {
					failReq++
				}
			}
			if ok {
				r.setNode(id, StOK)
			} else {
				r.setNode(id, StFail)
			}
			win.Invalidate()
			if ctx.Err() != nil {
				return
			}

			if n.kind == KindLoop {
				iters := n.count
				var items []interface{}
				useSrc := false
				if n.loopSrc != "" {
					if v, found := jsonPath(in, strings.TrimPrefix(n.loopSrc, "$.")); found {
						if arr, isArr := v.([]interface{}); isArr {
							items = arr
							iters = len(arr)
							useSrc = true
						}
					}
					if !useSrc {
						iters = 0
						anyFail = true
						r.setNode(id, StFail)
						r.setNodeInfo(id, "no array at "+n.loopSrc)
						win.Invalidate()
					}
				}
				for it := 0; it < iters; it++ {
					if ctx.Err() != nil {
						return
					}
					if iters > 1 || useSrc {
						r.setNodeInfo(id, fmt.Sprintf("iteration %d / %d", it+1, iters))
						win.Invalidate()
					}
					vars["loop.index"] = strconv.Itoa(it)
					if useSrc {
						for k := range vars {
							if strings.HasPrefix(k, "loop.item") {
								delete(vars, k)
							}
						}
						item := items[it]
						vars["loop.item"] = stringifyJSON(item)
						if m, isMap := item.(map[string]interface{}); isMap {
							for k, v := range m {
								vars["loop.item."+k] = stringifyJSON(v)
							}
						}
					}
					if it > 0 && n.delay > 0 {
						select {
						case <-ctx.Done():
							return
						case <-time.After(n.delay):
						}
					}
					for _, en := range n.entries {
						if ctx.Err() != nil {
							return
						}
						visit(en, in)
					}
				}
				if (iters > 1 || useSrc) && ctx.Err() == nil {
					r.setNodeInfo(id, fmt.Sprintf("done ×%d", iters))
				}
			}
			followOuts(n, res)
		}

		visit(startID, &stepResult{})

		r.mu.Lock()
		r.running = false
		r.paused = false
		r.cancel = nil
		r.stepCh = nil
		rec.Done = true
		rec.Dur = time.Since(started)
		counts := fmt.Sprintf(" · %d ok · %d failed", okReq, failReq)
		switch {
		case ctx.Err() != nil:
			r.status = "Stopped" + counts
			rec.Stopped = true
		case limitHit:
			r.status = "Stopped: step limit reached" + counts
			rec.Failed = true
		case anyFail:
			r.status = "Finished with errors" + counts
			rec.Failed = true
		default:
			r.status = "Finished" + counts
		}
		r.mu.Unlock()
		win.Invalidate()
	}()
}

func stringifyJSON(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(data)
}

func describeNetErr(ctx context.Context, err error) string {
	if ctx.Err() == context.Canceled {
		return "cancelled"
	}
	var uerr *url.Error
	if errors.As(err, &uerr) {
		msg := uerr.Err.Error()
		if uerr.Timeout() {
			msg = "timeout: " + msg
		}
		return msg
	}
	return err.Error()
}

func runHTTP(ctx context.Context, n *execNode, vars map[string]string) stepResult {
	rawURL := strings.TrimSpace(expandVars(n.url, n.env, vars))
	if rawURL == "" {
		return stepResult{failed: true, errMsg: "empty URL"}
	}
	if strings.Contains(rawURL, "{{") {
		return stepResult{failed: true, errMsg: "unresolved variable in URL: " + rawURL}
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	rawURL = strings.ReplaceAll(rawURL, " ", "%20")

	var bodyReader io.Reader
	if body := expandVars(n.body, n.env, vars); body != "" {
		bodyReader = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, n.method, rawURL, bodyReader)
	if err != nil {
		return stepResult{failed: true, errMsg: "invalid request: " + err.Error()}
	}
	for _, h := range n.headers {
		k := strings.TrimSpace(expandVars(h[0], n.env, vars))
		if k == "" {
			continue
		}
		req.Header.Add(k, strings.TrimSpace(expandVars(h[1], n.env, vars)))
	}
	resp, err := settings.HTTPClient.Do(req)
	if err != nil {
		return stepResult{failed: true, errMsg: describeNetErr(ctx, err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	res := stepResult{
		hasResp: true,
		status:  resp.StatusCode,
		body:    body,
		headers: resp.Header,
		failed:  resp.StatusCode >= 400,
	}
	if rerr != nil && ctx.Err() == nil {
		res.errMsg = "body read error: " + rerr.Error()
	}
	return res
}

func evalCond(e execEdge, res *stepResult, env, vars map[string]string) bool {
	if res == nil {
		res = &stepResult{}
	}
	value := expandVars(e.value, env, vars)
	switch e.cond {
	case CondAlways:
		return true
	case CondStatus:
		return res.hasResp && matchStatus(value, res.status)
	case CondHasResponse:
		return res.hasResp
	case CondNoResponse:
		return !res.hasResp
	case CondBodyField:
		_, ok := jsonPath(res, value)
		return ok
	case CondArrayCount:
		v, ok := jsonPath(res, value)
		if !ok {
			return false
		}
		arr, isArr := v.([]interface{})
		if !isArr {
			return false
		}
		return compareInt(len(arr), e.op, e.count)
	case CondBodyValue:
		v, ok := jsonPath(res, value)
		if !ok {
			return false
		}
		return compareValues(stringifyJSON(v), e.op, expandVars(e.value2, env, vars))
	}
	return false
}

func compareValues(a, op, b string) bool {
	switch op {
	case "contains":
		return strings.Contains(a, b)
	case "==", "":
		if fa, fb, ok := parseFloats(a, b); ok {
			return fa == fb
		}
		return a == b
	case "!=":
		if fa, fb, ok := parseFloats(a, b); ok {
			return fa != fb
		}
		return a != b
	}
	fa, fb, ok := parseFloats(a, b)
	if !ok {
		return false
	}
	switch op {
	case ">":
		return fa > fb
	case ">=":
		return fa >= fb
	case "<":
		return fa < fb
	case "<=":
		return fa <= fb
	}
	return false
}

func parseFloats(a, b string) (float64, float64, bool) {
	fa, errA := strconv.ParseFloat(strings.TrimSpace(a), 64)
	fb, errB := strconv.ParseFloat(strings.TrimSpace(b), 64)
	return fa, fb, errA == nil && errB == nil
}

func matchStatus(pattern string, status int) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		pattern = "2xx"
	}
	code := strconv.Itoa(status)
	if len(pattern) != len(code) {
		return false
	}
	for i := 0; i < len(pattern); i++ {
		if pattern[i] == 'x' || pattern[i] == '*' {
			continue
		}
		if pattern[i] != code[i] {
			return false
		}
	}
	return true
}

func jsonPath(res *stepResult, path string) (interface{}, bool) {
	if !res.hasResp || len(res.body) == 0 {
		return nil, false
	}
	if !res.jsonParsed {
		res.jsonParsed = true
		_ = json.Unmarshal(res.body, &res.jsonVal)
	}
	if res.jsonVal == nil {
		return nil, false
	}
	cur := res.jsonVal
	path = strings.TrimSpace(path)
	if path == "" {
		return cur, true
	}
	for _, seg := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]interface{}:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

func compareInt(a int, op string, b int) bool {
	switch op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	case "!=":
		return a != b
	default:
		return a == b
	}
}
