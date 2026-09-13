package flow

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"rete/internal/model"
	"rete/internal/ui/settings"
	"rete/internal/ui/widgets"
	"rete/internal/utils"
	"rete/internal/ws"

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

	bodyType  string
	authType  string
	authToken string
	authUser  string
	authPass  string
	cookies   [][2]string
	gqlVars   string
	subprotos []string
	wsOpcode  string
	waitMs    int
	binPath   string
	insecure  bool
	keepOpen  bool
	wsClose   bool
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
			id:        n.ID,
			kind:      n.Kind,
			name:      n.DisplayName(),
			method:    n.Method,
			url:       strings.TrimSpace(n.URLEd.Text()),
			body:      n.BodyEd.Text(),
			env:       env,
			varName:   strings.TrimSpace(n.VarNameEd.Text()),
			varValue:  strings.TrimSpace(n.VarValueEd.Text()),
			loopSrc:   strings.TrimSpace(n.LoopSrcEd.Text()),
			bodyType:  n.BodyType,
			authType:  n.AuthType,
			authToken: strings.TrimSpace(n.AuthTokenEd.Text()),
			authUser:  n.AuthUserEd.Text(),
			authPass:  n.AuthPassEd.Text(),
			gqlVars:   strings.TrimSpace(n.VarsEd.Text()),
			wsOpcode:  n.WSOpcode,
			waitMs:    parseEditorInt(n.WaitMsEd.Text(), 1000),
			binPath:   strings.TrimSpace(n.BinPathEd.Text()),
			insecure:  n.InsecureTLS,
			keepOpen:  n.KeepOpen,
			wsClose:   n.WSClose,
		}
		for _, line := range strings.Split(n.HeadersEd.Text(), "\n") {
			k, v, ok := strings.Cut(line, ":")
			k = strings.TrimSpace(k)
			if !ok || k == "" {
				continue
			}
			en.headers = append(en.headers, [2]string{k, strings.TrimSpace(v)})
		}
		for _, line := range strings.Split(n.CookiesEd.Text(), "\n") {
			k, v, _ := strings.Cut(line, "=")
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			en.cookies = append(en.cookies, [2]string{k, strings.TrimSpace(v)})
		}
		for _, sp := range strings.Split(n.SubprotosEd.Text(), ",") {
			if sp = strings.TrimSpace(sp); sp != "" {
				en.subprotos = append(en.subprotos, sp)
			}
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
		start, end, ok := widgets.FindVar(input, i)
		if !ok {
			b.WriteString(input[i:])
			break
		}
		b.WriteString(input[i:start])
		k := strings.TrimSpace(input[start+2 : end-2])
		if v, ok := vars[k]; ok {
			b.WriteString(v)
		} else if v, ok := env[k]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(input[start:end])
		}
		i = end
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
		var wsSocks []*liveWS
		defer func() {
			for _, s := range wsSocks {
				s.close()
			}
		}()
		curSock := func() *liveWS {
			for i := len(wsSocks) - 1; i >= 0; i-- {
				if !wsSocks[i].closed {
					return wsSocks[i]
				}
			}
			return nil
		}
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
			case KindRequest, KindWSRequest, KindGQLRequest, KindWSSend:
				reqStart := time.Now()
				var rr stepResult
				var detail string
				switch n.kind {
				case KindWSRequest:
					var sock *liveWS
					rr, sock = runWSOpen(ctx, n, vars)
					if sock != nil {
						wsSocks = append(wsSocks, sock)
					}
					detail = "WS " + expandVars(n.url, n.env, vars)
				case KindWSSend:
					rr = runWSSend(ctx, n, vars, curSock())
					detail = "WS send"
				case KindGQLRequest:
					rr = runGQL(ctx, n, vars)
					detail = "GraphQL " + expandVars(n.url, n.env, vars)
				default:
					rr = runHTTP(ctx, n, vars)
					detail = n.method + " " + expandVars(n.url, n.env, vars)
				}
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
					Detail:  detail,
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

			ok := ctx.Err() == nil && (res == nil || !res.failed || !n.kind.IsRequest())
			if !ok {
				anyFail = true
			}
			if n.kind.IsRequest() {
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

func resolveURL(n *execNode, vars map[string]string) (string, string) {
	raw := strings.ReplaceAll(n.url, "\n", "")
	raw = strings.ReplaceAll(raw, "\t", "")
	raw = strings.TrimSpace(utils.SanitizeText(raw))
	rawURL := strings.TrimSpace(expandVars(raw, n.env, vars))
	if rawURL == "" {
		return "", "empty URL"
	}
	if strings.Contains(rawURL, "{{") {
		return "", "unresolved variable in URL: " + rawURL
	}
	return strings.ReplaceAll(rawURL, " ", "%20"), ""
}

func (n *execNode) authHeader(vars map[string]string) string {
	switch n.authType {
	case "bearer":
		tok := strings.TrimSpace(expandVars(n.authToken, n.env, vars))
		if tok == "" {
			return ""
		}
		return "Bearer " + tok
	case "basic":
		u := expandVars(n.authUser, n.env, vars)
		p := expandVars(n.authPass, n.env, vars)
		if u == "" && p == "" {
			return ""
		}
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
	}
	return ""
}

func (n *execNode) cookieHeader(vars map[string]string) string {
	var parts []string
	for _, c := range n.cookies {
		k := strings.TrimSpace(expandVars(c[0], n.env, vars))
		if k == "" {
			continue
		}
		parts = append(parts, k+"="+strings.TrimSpace(expandVars(c[1], n.env, vars)))
	}
	return strings.Join(parts, "; ")
}

func (n *execNode) applyHeaders(h http.Header, vars map[string]string) {
	for _, hd := range n.headers {
		k := strings.TrimSpace(expandVars(hd[0], n.env, vars))
		if k == "" {
			continue
		}
		h.Add(k, strings.TrimSpace(expandVars(hd[1], n.env, vars)))
	}
	if a := n.authHeader(vars); a != "" {
		h.Set("Authorization", a)
	}
	if c := n.cookieHeader(vars); c != "" {
		h.Set("Cookie", c)
	}
}

func flowUserAgent() string {
	if ua := strings.TrimSpace(settings.UserAgent); ua != "" {
		return ua
	}
	return model.DefaultSettings().UserAgent
}

func (n *execNode) applySystemHeaders(req *http.Request, vars map[string]string, contentType string, ctExplicit bool) {
	h := req.Header
	if h.Get("User-Agent") == "" {
		h.Set("User-Agent", flowUserAgent())
	}
	if contentType != "" && !ctExplicit && h.Get("Content-Type") == "" {
		h.Set("Content-Type", contentType)
	}
	for _, dh := range settings.DefaultHeaders {
		k := strings.TrimSpace(dh.Key)
		if k == "" || h.Get(k) != "" {
			continue
		}
		h.Set(k, expandVars(dh.Value, n.env, vars))
	}
	if contentType != "" && ctExplicit {
		h.Set("Content-Type", contentType)
	}
	if ae := strings.TrimSpace(settings.AcceptEncoding); ae != "" && h.Get("Accept-Encoding") == "" {
		h.Set("Accept-Encoding", ae)
	}
	if settings.SendConnClose {
		req.Close = true
		if h.Get("Connection") == "" {
			h.Set("Connection", "close")
		}
	}
}

var bodyReplacer = strings.NewReplacer("\u2003", "\t", "\uFEFF", "")

func prepareRawBody(body string) string {
	body = bodyReplacer.Replace(body)
	if settings.TrimTrailingWS {
		body = utils.TrimTrailingWhitespace(body)
	}
	if settings.StripJSONComments {
		if stripped := utils.StripJSONComments(body); json.Valid([]byte(stripped)) {
			body = stripped
		}
	}
	if settings.AutoFormatJSONRequest && json.Valid([]byte(body)) {
		var v interface{}
		if err := json.Unmarshal([]byte(body), &v); err == nil {
			indent := settings.JSONIndent
			if indent < 0 {
				indent = 2
			}
			if formatted, err := json.MarshalIndent(v, "", strings.Repeat(" ", indent)); err == nil {
				body = string(formatted)
			}
		}
	}
	return body
}

func sniffContentType(body string) string {
	t := strings.TrimLeft(strings.TrimPrefix(body, "\uFEFF"), " \t\r\n")
	if strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[") {
		return "application/json"
	}
	return "text/plain"
}

func (n *execNode) buildBody(vars map[string]string) (io.Reader, string, bool, string) {
	switch n.bodyType {
	case "urlencoded":
		form := url.Values{}
		for _, line := range strings.Split(expandVars(n.body, n.env, vars), "\n") {
			k, v, _ := strings.Cut(line, "=")
			if k = strings.TrimSpace(k); k == "" {
				continue
			}
			form.Add(k, strings.TrimSpace(v))
		}
		return strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", true, ""
	case "form":
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		for _, line := range strings.Split(expandVars(n.body, n.env, vars), "\n") {
			k, v, _ := strings.Cut(line, "=")
			if k = strings.TrimSpace(k); k == "" {
				continue
			}
			v = strings.TrimSpace(v)
			if path, isFile := strings.CutPrefix(v, "@"); isFile {
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, "", false, "form file " + path + ": " + err.Error()
				}
				fw, err := mw.CreateFormFile(k, filepath.Base(path))
				if err != nil {
					return nil, "", false, err.Error()
				}
				_, _ = fw.Write(data)
			} else {
				_ = mw.WriteField(k, v)
			}
		}
		_ = mw.Close()
		return &buf, mw.FormDataContentType(), true, ""
	case "binary":
		if n.binPath == "" {
			return nil, "", false, "binary body: no file selected"
		}
		path := expandVars(n.binPath, n.env, vars)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", false, "binary body: " + err.Error()
		}
		ct := mime.TypeByExtension(filepath.Ext(path))
		if ct == "" {
			ct = "application/octet-stream"
		}
		return bytes.NewReader(data), ct, true, ""
	default:
		body := prepareRawBody(expandVars(n.body, n.env, vars))
		return strings.NewReader(body), sniffContentType(body), false, ""
	}
}

func doHTTP(ctx context.Context, n *execNode, vars map[string]string, method, rawURL string, bodyReader io.Reader, contentType string, ctExplicit bool) stepResult {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return stepResult{failed: true, errMsg: "invalid request: " + err.Error()}
	}
	n.applyHeaders(req.Header, vars)
	n.applySystemHeaders(req, vars, contentType, ctExplicit)
	resp, err := settings.HTTPClient.Do(req)
	if err != nil {
		return stepResult{failed: true, errMsg: describeNetErr(ctx, err)}
	}
	defer func() { _ = resp.Body.Close() }()
	stream := utils.DecompressBody(resp)
	defer func() { _ = stream.Close() }()
	body, rerr := io.ReadAll(io.LimitReader(stream, maxBodyBytes))
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

func runHTTP(ctx context.Context, n *execNode, vars map[string]string) stepResult {
	rawURL, urlErr := resolveURL(n, vars)
	if urlErr != "" {
		return stepResult{failed: true, errMsg: urlErr}
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	bodyReader, contentType, ctExplicit, bodyErr := n.buildBody(vars)
	if bodyErr != "" {
		return stepResult{failed: true, errMsg: bodyErr}
	}
	method := strings.ToUpper(strings.TrimSpace(n.method))
	if method == "" {
		method = http.MethodGet
	}
	return doHTTP(ctx, n, vars, method, rawURL, bodyReader, contentType, ctExplicit)
}

func runGQL(ctx context.Context, n *execNode, vars map[string]string) stepResult {
	rawURL, urlErr := resolveURL(n, vars)
	if urlErr != "" {
		return stepResult{failed: true, errMsg: urlErr}
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "http://" + rawURL
	}
	payload := struct {
		Query     string          `json:"query"`
		Variables json.RawMessage `json:"variables,omitempty"`
	}{Query: expandVars(n.body, n.env, vars)}
	if varsText := strings.TrimSpace(expandVars(n.gqlVars, n.env, vars)); varsText != "" {
		if !json.Valid([]byte(varsText)) {
			return stepResult{failed: true, errMsg: "GraphQL variables: invalid JSON"}
		}
		payload.Variables = json.RawMessage(varsText)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return stepResult{failed: true, errMsg: "GraphQL payload: " + err.Error()}
	}
	return doHTTP(ctx, n, vars, http.MethodPost, rawURL, bytes.NewReader(data), "application/json", true)
}

func parseWSHexBody(s string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '\t', '\n', '\r', ',', ':', '-':
			return -1
		}
		return r
	}, s)
	clean = strings.TrimPrefix(clean, "0x")
	return hex.DecodeString(clean)
}

type liveWS struct {
	conn    *ws.Conn
	status  int
	headers http.Header
	stop    func() bool
	closed  bool
}

func (l *liveWS) close() {
	if l == nil || l.closed {
		return
	}
	l.closed = true
	_ = l.conn.WriteClose(ws.CloseNormal, "")
	_ = l.conn.Close()
	if l.stop != nil {
		l.stop()
	}
}

func wsSendPayload(opcode, msg string) (ws.Opcode, []byte, string) {
	if strings.EqualFold(opcode, "BIN") {
		decoded, derr := parseWSHexBody(msg)
		if derr != nil {
			return ws.OpBinary, nil, "hex payload: " + derr.Error()
		}
		return ws.OpBinary, decoded, ""
	}
	return ws.OpText, []byte(msg), ""
}

func collectWSReplies(ctx context.Context, conn *ws.Conn, waitMs int) []byte {
	wait := time.Duration(waitMs) * time.Millisecond
	if wait <= 0 {
		return nil
	}
	deadline := time.Now().Add(wait)
	_ = conn.Underlying().SetReadDeadline(deadline)
	var collected [][]byte
	total := 0
	for time.Now().Before(deadline) && ctx.Err() == nil && total < maxBodyBytes {
		op, payload, rerr := conn.ReadMessage()
		if rerr != nil {
			break
		}
		if op != ws.OpText && op != ws.OpBinary {
			continue
		}
		collected = append(collected, payload)
		total += len(payload) + 1
	}
	_ = conn.Underlying().SetReadDeadline(time.Time{})
	return bytes.Join(collected, []byte("\n"))
}

func runWS(ctx context.Context, n *execNode, vars map[string]string) stepResult {
	res, sock := runWSOpen(ctx, n, vars)
	sock.close()
	return res
}

func runWSOpen(ctx context.Context, n *execNode, vars map[string]string) (stepResult, *liveWS) {
	rawURL, urlErr := resolveURL(n, vars)
	if urlErr != "" {
		return stepResult{failed: true, errMsg: urlErr}, nil
	}
	switch {
	case strings.HasPrefix(rawURL, "ws://"), strings.HasPrefix(rawURL, "wss://"):
	case strings.HasPrefix(rawURL, "http://"):
		rawURL = "ws://" + strings.TrimPrefix(rawURL, "http://")
	case strings.HasPrefix(rawURL, "https://"):
		rawURL = "wss://" + strings.TrimPrefix(rawURL, "https://")
	default:
		rawURL = "ws://" + rawURL
	}

	headers := http.Header{}
	n.applyHeaders(headers, vars)
	if headers.Get("Origin") == "" {
		if origin := ws.DefaultOrigin(rawURL); origin != "" {
			headers.Set("Origin", origin)
		}
	}
	opts := ws.DialOptions{
		Subprotocols: n.subprotos,
		Headers:      headers,
		DialTimeout:  15 * time.Second,
	}
	if n.insecure {
		opts.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	}
	res, err := ws.Dial(ctx, rawURL, opts)
	if err != nil {
		out := stepResult{failed: true, errMsg: describeNetErr(ctx, err)}
		if res != nil && res.Response != nil {
			out.hasResp = true
			out.status = res.Response.StatusCode
			out.headers = res.Response.Header
			out.body = res.ResponseBody
		}
		return out, nil
	}
	conn := res.Conn
	stopWatch := context.AfterFunc(ctx, func() { _ = conn.Close() })
	sock := &liveWS{
		conn:    conn,
		status:  res.Response.StatusCode,
		headers: res.Response.Header,
		stop:    stopWatch,
	}

	out := stepResult{
		hasResp: true,
		status:  res.Response.StatusCode,
		headers: res.Response.Header,
	}

	if msg := expandVars(n.body, n.env, vars); strings.TrimSpace(msg) != "" {
		op, payload, perr := wsSendPayload(n.wsOpcode, msg)
		if perr != "" {
			sock.close()
			return stepResult{failed: true, errMsg: perr}, nil
		}
		if werr := conn.WriteMessage(op, payload); werr != nil {
			sock.close()
			out.failed = true
			out.errMsg = "send: " + werr.Error()
			return out, nil
		}
	}

	out.body = collectWSReplies(ctx, conn, n.waitMs)
	if !n.keepOpen {
		sock.close()
		return out, nil
	}
	return out, sock
}

func runWSSend(ctx context.Context, n *execNode, vars map[string]string, sock *liveWS) stepResult {
	if sock == nil || sock.closed {
		return stepResult{failed: true, errMsg: "no open WebSocket — put a WebSocket node with 'keep socket open' before this step"}
	}
	out := stepResult{hasResp: true, status: sock.status, headers: sock.headers}
	msg := expandVars(n.body, n.env, vars)
	if strings.TrimSpace(msg) != "" {
		op, payload, perr := wsSendPayload(n.wsOpcode, msg)
		if perr != "" {
			return stepResult{failed: true, errMsg: perr}
		}
		if werr := sock.conn.WriteMessage(op, payload); werr != nil {
			sock.close()
			out.failed = true
			out.errMsg = "send: " + werr.Error()
			return out
		}
	}
	out.body = collectWSReplies(ctx, sock.conn, n.waitMs)
	if n.wsClose {
		sock.close()
	}
	return out
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
