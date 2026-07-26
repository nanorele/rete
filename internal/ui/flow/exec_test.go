package flow

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
)

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
