package workspace

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tracto/internal/model"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget/material"
)

func drainSpecBody(t *testing.T, s *runSpec, env map[string]string) (*http.Request, string) {
	t.Helper()
	req, err := s.newRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if req.Body == nil {
		return req, ""
	}
	b, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return req, string(b)
}

func TestBuildRunSpec_RawBodyStaysTemplated(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = "POST"
	tab.BodyType = model.BodyRaw
	tab.URLInput.SetText("http://example.com/{{path}}")
	tab.ReqEditor.SetText(`{"n":"{{who}}"}`)
	tab.AddHeader("X-Fixed", "1")
	tab.AddHeader("", "ignored")

	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if !spec.useTmplBody {
		t.Error("raw bodies must stay templated so each iteration re-renders them")
	}
	if spec.bodyTmpl != `{"n":"{{who}}"}` {
		t.Errorf("bodyTmpl = %q, want the unrendered template", spec.bodyTmpl)
	}
	if spec.urlTmpl != "http://example.com/{{path}}" {
		t.Errorf("urlTmpl = %q, want the unrendered template", spec.urlTmpl)
	}
	for _, h := range spec.headers {
		if strings.TrimSpace(h[0]) == "" {
			t.Errorf("blank header keys must be dropped: %+v", spec.headers)
		}
	}

	env := map[string]string{"path": "v1", "who": "ann"}
	req, body := drainSpecBody(t, spec, env)
	if req.URL.String() != "http://example.com/v1" {
		t.Errorf("URL = %q, want http://example.com/v1", req.URL.String())
	}
	if body != `{"n":"ann"}` {
		t.Errorf("body = %q, want the rendered template", body)
	}
	if req.Header.Get("X-Fixed") != "1" {
		t.Errorf("X-Fixed header = %q", req.Header.Get("X-Fixed"))
	}
}

func TestBuildRunSpec_StripsNewlinesAndTabsFromURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("  http://exa\nmple.com/a\tb  ")
	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.urlTmpl != "http://example.com/ab" {
		t.Errorf("urlTmpl = %q, want newlines/tabs stripped and trimmed", spec.urlTmpl)
	}
}

func TestBuildRunSpec_NonRawBodyIsMaterialized(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = "POST"
	tab.BodyType = model.BodyURLEncoded
	tab.URLInput.SetText("http://example.com")
	tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("a", "{{v}}"))

	spec, err := tab.buildRunSpec(context.Background(), map[string]string{"v": "1"})
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.useTmplBody {
		t.Error("url-encoded bodies must be materialized once, not templated per iteration")
	}
	if got := string(spec.bodyBytes); got != "a=1" {
		t.Errorf("bodyBytes = %q, want a=1", got)
	}
	if spec.explicitCT != "application/x-www-form-urlencoded" {
		t.Errorf("explicitCT = %q", spec.explicitCT)
	}
	req, body := drainSpecBody(t, spec, nil)
	if body != "a=1" {
		t.Errorf("request body = %q", body)
	}
	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Errorf("Content-Type = %q", req.Header.Get("Content-Type"))
	}
}

func TestBuildRunSpec_GraphQLAlwaysBuildsJSONBody(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.BodyType = model.BodyNone
	tab.URLInput.SetText("http://example.com/graphql")
	tab.EnsureGQL().Query.SetText("{ me }")

	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.method != "POST" {
		t.Errorf("method = %q, want POST for GraphQL", spec.method)
	}
	if !strings.Contains(string(spec.bodyBytes), `"query"`) {
		t.Errorf("bodyBytes = %q, want a GraphQL payload", spec.bodyBytes)
	}
	if spec.explicitCT != "application/json" {
		t.Errorf("explicitCT = %q", spec.explicitCT)
	}
}

func TestBuildRunSpec_GraphQLPropagatesBodyError(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.URLInput.SetText("http://example.com/graphql")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")
	g.Variables.SetText(`{"bad":}`)
	if _, err := tab.buildRunSpec(context.Background(), nil); err == nil {
		t.Error("buildRunSpec must surface an invalid-variables error")
	}
}

func TestBuildRunSpec_BodyNoneHasNoBody(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyNone
	tab.ReqEditor.SetText("ignored")
	tab.URLInput.SetText("http://example.com")
	spec, err := tab.buildRunSpec(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.useTmplBody || spec.bodyBytes != nil {
		t.Errorf("BodyNone must produce no body: tmpl=%v bytes=%q", spec.useTmplBody, spec.bodyBytes)
	}
	req, body := drainSpecBody(t, spec, nil)
	if body != "" {
		t.Errorf("request body = %q, want empty", body)
	}
	if req.Body != nil {
		t.Error("request must carry a nil body when there is nothing to send")
	}
}

func TestBuildRunSpec_CarriesAuthAndCookies(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://example.com")
	tab.AuthType = authBearer
	tab.AuthToken.SetText("{{tok}}")
	tab.ApplyCookies([]model.ParsedKV{{Key: "sid", Value: "{{sid}}"}})

	env := map[string]string{"tok": "abc", "sid": "42"}
	spec, err := tab.buildRunSpec(context.Background(), env)
	if err != nil {
		t.Fatalf("buildRunSpec: %v", err)
	}
	if spec.authHeader != "Bearer abc" {
		t.Errorf("authHeader = %q", spec.authHeader)
	}
	if spec.cookieHeader != "sid=42" {
		t.Errorf("cookieHeader = %q", spec.cookieHeader)
	}
	req, _ := drainSpecBody(t, spec, env)
	if req.Header.Get("Authorization") != "Bearer abc" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
	if req.Header.Get("Cookie") != "sid=42" {
		t.Errorf("Cookie = %q", req.Header.Get("Cookie"))
	}
}

func TestNewRequest_EmptyURLFails(t *testing.T) {
	s := &runSpec{method: "GET", urlTmpl: "{{missing}}"}
	if _, err := s.newRequest(context.Background(), map[string]string{"missing": ""}); err == nil {
		t.Error("a URL that renders empty must be rejected")
	}
	s2 := &runSpec{method: "GET", urlTmpl: ""}
	if _, err := s2.newRequest(context.Background(), nil); err == nil {
		t.Error("an empty URL template must be rejected")
	}
}

func TestNewRequest_AddsSchemeAndEscapesSpaces(t *testing.T) {
	s := &runSpec{method: "GET", urlTmpl: "example.com/a b"}
	req, err := s.newRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if got := req.URL.String(); got != "http://example.com/a%20b" {
		t.Errorf("URL = %q, want scheme added and space escaped", got)
	}

	for _, raw := range []string{"http://x.test/p", "https://x.test/p"} {
		s := &runSpec{method: "GET", urlTmpl: raw}
		req, err := s.newRequest(context.Background(), nil)
		if err != nil {
			t.Fatalf("newRequest(%q): %v", raw, err)
		}
		if req.URL.String() != raw {
			t.Errorf("URL = %q, want %q untouched", req.URL.String(), raw)
		}
	}
}

func TestNewRequest_RejectsBadMethod(t *testing.T) {
	s := &runSpec{method: "BAD METHOD", urlTmpl: "http://x.test"}
	if _, err := s.newRequest(context.Background(), nil); err == nil {
		t.Error("an invalid HTTP method must be rejected")
	}
}

func TestNewRequest_DropsHeadersThatRenderEmpty(t *testing.T) {
	s := &runSpec{
		method:  "GET",
		urlTmpl: "http://x.test",
		headers: [][2]string{{"{{hk}}", "v"}, {"X-Ok", "  {{hv}}  "}},
	}
	req, err := s.newRequest(context.Background(), map[string]string{"hk": "", "hv": "yes"})
	if err != nil {
		t.Fatalf("newRequest: %v", err)
	}
	if len(req.Header) != 1 {
		t.Errorf("header count = %d, want only X-Ok: %+v", len(req.Header), req.Header)
	}
	if req.Header.Get("X-Ok") != "yes" {
		t.Errorf("X-Ok = %q, want trimmed and rendered", req.Header.Get("X-Ok"))
	}
}

func TestRunOnceSpec_AgainstTestServer(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(200)
			_, _ = w.Write([]byte("hello"))
		case "/redirlike":
			w.WriteHeader(304)
		case "/boom":
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	cases := []struct {
		path   string
		code   int
		wantOK bool
	}{
		{"/ok", 200, true},
		{"/redirlike", 304, true},
		{"/boom", 500, false},
		{"/nope", 404, false},
	}
	for _, c := range cases {
		s := &runSpec{method: "GET", urlTmpl: srv.URL + c.path}
		code, lat, ok := runOnceSpec(context.Background(), s, nil)
		if code != c.code || ok != c.wantOK {
			t.Errorf("%s: code=%d ok=%v, want %d/%v", c.path, code, ok, c.code, c.wantOK)
		}
		if lat < 0 {
			t.Errorf("%s: latency must not be negative, got %v", c.path, lat)
		}
	}
	if atomic.LoadInt32(&hits) != int32(len(cases)) {
		t.Errorf("server saw %d hits, want %d", hits, len(cases))
	}
}

func TestRunOnceSpec_FailuresReportZeroCode(t *testing.T) {
	badSpec := &runSpec{method: "GET", urlTmpl: ""}
	if code, _, ok := runOnceSpec(context.Background(), badSpec, nil); code != 0 || ok {
		t.Errorf("build failure: code=%d ok=%v, want 0/false", code, ok)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	dialSpec := &runSpec{method: "GET", urlTmpl: url}
	if code, _, ok := runOnceSpec(context.Background(), dialSpec, nil); code != 0 || ok {
		t.Errorf("transport failure: code=%d ok=%v, want 0/false", code, ok)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer live.Close()
	if code, _, ok := runOnceSpec(ctx, &runSpec{method: "GET", urlTmpl: live.URL}, nil); code != 0 || ok {
		t.Errorf("cancelled context: code=%d ok=%v, want 0/false", code, ok)
	}
}

func TestRunnerRecordAndSnapshot(t *testing.T) {
	r := newRequestRunner()
	r.record(200, 5*time.Millisecond, true)
	r.record(200, 15*time.Millisecond, true)
	r.record(500, 1*time.Millisecond, false)

	snap := r.snapshot()
	if snap.completed != 3 || snap.success != 2 || snap.failed != 1 {
		t.Errorf("completed/success/failed = %d/%d/%d, want 3/2/1", snap.completed, snap.success, snap.failed)
	}
	if snap.minLat != int64(time.Millisecond) {
		t.Errorf("minLat = %d, want %d", snap.minLat, int64(time.Millisecond))
	}
	if snap.maxLat != int64(15*time.Millisecond) {
		t.Errorf("maxLat = %d, want %d", snap.maxLat, int64(15*time.Millisecond))
	}
	if snap.sumLat != int64(21*time.Millisecond) {
		t.Errorf("sumLat = %d, want %d", snap.sumLat, int64(21*time.Millisecond))
	}
	if len(snap.buckets) != 2 {
		t.Fatalf("bucket count = %d, want 2", len(snap.buckets))
	}
	var b200 statusBucket
	for _, b := range snap.buckets {
		if b.code == 200 {
			b200 = b
		}
	}
	if b200.count != 2 {
		t.Errorf("200 bucket count = %d, want 2", b200.count)
	}
	if b200.minLat != int64(5*time.Millisecond) || b200.maxLat != int64(15*time.Millisecond) {
		t.Errorf("200 bucket min/max = %d/%d", b200.minLat, b200.maxLat)
	}
	if snap.p50 <= 0 || snap.p99 <= 0 {
		t.Errorf("percentiles must be computed once samples exist: %+v", snap)
	}
}

func TestRunnerSnapshotSortOrder(t *testing.T) {
	mk := func() *RequestRunner {
		r := newRequestRunner()
		r.record(500, 30*time.Millisecond, false)
		r.record(500, 40*time.Millisecond, false)
		r.record(200, 10*time.Millisecond, true)
		r.record(200, 20*time.Millisecond, true)
		r.record(200, 12*time.Millisecond, true)
		r.record(404, 1*time.Millisecond, false)
		return r
	}
	cases := []struct {
		name string
		col  int
		asc  bool
		want []int
	}{
		{"code-asc", 0, true, []int{200, 404, 500}},
		{"code-desc", 0, false, []int{500, 404, 200}},
		{"count-desc", 1, false, []int{200, 500, 404}},
		{"share-desc", 2, false, []int{200, 500, 404}},
		{"avg-asc", 3, true, []int{404, 200, 500}},
		{"min-desc", 4, false, []int{500, 200, 404}},
		{"max-desc", 5, false, []int{500, 200, 404}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := mk()
			r.SortCol = c.col
			r.SortAsc = c.asc
			snap := r.snapshot()
			got := make([]int, len(snap.buckets))
			for i, b := range snap.buckets {
				got[i] = b.code
			}
			if len(got) != len(c.want) {
				t.Fatalf("bucket count = %d, want %d", len(got), len(c.want))
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Fatalf("order = %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestRunnerSnapshotEmptyIsZeroed(t *testing.T) {
	r := newRequestRunner()
	snap := r.snapshot()
	if snap.completed != 0 || len(snap.buckets) != 0 {
		t.Errorf("fresh runner snapshot = %+v, want zeroed", snap)
	}
	if snap.elapsed != 0 {
		t.Errorf("elapsed = %v, want 0 before the run starts", snap.elapsed)
	}
}

func TestRunnerSnapshotElapsedFreezesAfterEnd(t *testing.T) {
	r := newRequestRunner()
	r.mu.Lock()
	r.startedAt = time.Now().Add(-2 * time.Second)
	r.mu.Unlock()
	live := r.snapshot().elapsed
	if live < 2*time.Second {
		t.Errorf("running elapsed = %v, want >= 2s", live)
	}

	r.mu.Lock()
	r.endedAt = r.startedAt.Add(1500 * time.Millisecond)
	r.mu.Unlock()
	frozen := r.snapshot().elapsed
	if frozen != 1500*time.Millisecond {
		t.Errorf("finished elapsed = %v, want exactly 1.5s", frozen)
	}
	if again := r.snapshot().elapsed; again != frozen {
		t.Errorf("finished elapsed drifted: %v -> %v", frozen, again)
	}
}

func TestRunnerResetCounters(t *testing.T) {
	r := newRequestRunner()
	r.record(200, time.Millisecond, true)
	r.record(500, time.Second, false)
	r.sent.Store(9)
	r.inFlight.Store(3)
	_ = r.snapshot()

	r.resetCounters()
	snap := r.snapshot()
	if snap.completed != 0 || snap.success != 0 || snap.failed != 0 {
		t.Errorf("counters not cleared: %+v", snap)
	}
	if snap.minLat != 0 || snap.maxLat != 0 || snap.sumLat != 0 {
		t.Errorf("latency accumulators not cleared: %+v", snap)
	}
	if len(snap.buckets) != 0 {
		t.Errorf("buckets not cleared: %+v", snap.buckets)
	}
	if snap.p50 != 0 || snap.p90 != 0 || snap.p99 != 0 {
		t.Errorf("percentiles not cleared: %+v", snap)
	}
	if r.sent.Load() != 0 || r.inFlight.Load() != 0 {
		t.Errorf("sent/inFlight = %d/%d, want 0/0", r.sent.Load(), r.inFlight.Load())
	}

	r.record(404, 2*time.Millisecond, false)
	if snap := r.snapshot(); snap.completed != 1 || snap.p50 != int64(2*time.Millisecond) {
		t.Errorf("recording after reset: %+v", snap)
	}
}

func TestRunnerRecordCapsLatencySamples(t *testing.T) {
	r := newRequestRunner()
	for i := 0; i < 50002; i++ {
		r.record(200, time.Millisecond, true)
	}
	r.mu.Lock()
	n := len(r.lat)
	r.mu.Unlock()
	if n != 50000 {
		t.Errorf("latency sample buffer = %d, want capped at 50000", n)
	}
	if snap := r.snapshot(); snap.completed != 50002 {
		t.Errorf("completed = %d, want every request counted even past the sample cap", snap.completed)
	}
}

func TestPercentile(t *testing.T) {
	cases := []struct {
		name   string
		sorted []int64
		p      float64
		want   int64
	}{
		{"empty", nil, 0.5, 0},
		{"single", []int64{7}, 0.99, 7},
		{"median", []int64{1, 2, 3, 4, 5}, 0.5, 3},
		{"p90", []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.9, 9},
		{"p0", []int64{4, 5, 6}, 0, 4},
		{"p1", []int64{4, 5, 6}, 1, 6},
		{"over-one-clamps", []int64{4, 5, 6}, 2, 6},
		{"negative-clamps", []int64{4, 5, 6}, -1, 4},
	}
	for _, c := range cases {
		if got := percentile(c.sorted, c.p); got != c.want {
			t.Errorf("%s: percentile(%v, %v) = %d, want %d", c.name, c.sorted, c.p, got, c.want)
		}
	}
}

func TestFmtMs(t *testing.T) {
	cases := []struct {
		ns   int64
		want string
	}{
		{0, "0"},
		{-5, "0"},
		{1_500_000, "1.5"},
		{9_940_000, "9.9"},
		{10_000_000, "10"},
		{10_600_000, "11"},
		{1_000_000_000, "1000"},
	}
	for _, c := range cases {
		if got := fmtMs(c.ns); got != c.want {
			t.Errorf("fmtMs(%d) = %q, want %q", c.ns, got, c.want)
		}
	}
}

func TestStatusLabelAndFailColor(t *testing.T) {
	if got := statusLabel(0); got != "ERR" {
		t.Errorf("statusLabel(0) = %q, want ERR", got)
	}
	if got := statusLabel(503); got != "503" {
		t.Errorf("statusLabel(503) = %q", got)
	}
	if failColor(0) != nil {
		t.Error("failColor(0) must be nil so the cell keeps the default colour")
	}
	c := failColor(3)
	if c == nil {
		t.Fatal("failColor(3) must return a colour")
	}
	if other := failColor(9); other == c {
		t.Error("failColor must hand out independent copies, not a shared pointer")
	}
}

func TestSplitValuesAndEnvForIteration(t *testing.T) {
	if got := splitValues(" a , b ,, c "); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitValues = %#v", got)
	}
	if got := splitValues("  ,  "); len(got) != 0 {
		t.Errorf("splitValues(blank) = %#v, want empty", got)
	}

	base := map[string]string{"host": "x"}
	if got := envForIteration(base, nil, 3); len(got) != 1 {
		t.Errorf("no variables must reuse the base env, got %#v", got)
	}
	vars := []runVarSnapshot{{name: "id", vals: []string{"1", "2"}}}
	e0 := envForIteration(base, vars, 0)
	e3 := envForIteration(base, vars, 3)
	if e0["id"] != "1" || e3["id"] != "2" {
		t.Errorf("values must cycle by index: %q / %q", e0["id"], e3["id"])
	}
	if e0["host"] != "x" {
		t.Error("base env keys must survive")
	}
	if base["id"] != "" {
		t.Error("envForIteration must not mutate the base env")
	}
}

func TestSnapshotVariablesSkipsIncomplete(t *testing.T) {
	r := newRequestRunner()
	r.addVar()
	r.addVar()
	r.addVar()
	r.Variables[0].Name.SetText("  ")
	r.Variables[0].Values.SetText("1,2")
	r.Variables[1].Name.SetText("empty")
	r.Variables[1].Values.SetText("  ,  ")
	r.Variables[2].Name.SetText("  ok  ")
	r.Variables[2].Values.SetText("a, b")

	got := r.snapshotVariables()
	if len(got) != 1 {
		t.Fatalf("snapshotVariables = %#v, want only the complete row", got)
	}
	if got[0].name != "ok" {
		t.Errorf("name = %q, want trimmed 'ok'", got[0].name)
	}
	if len(got[0].vals) != 2 {
		t.Errorf("vals = %#v, want 2", got[0].vals)
	}
}

func TestEndedAtStopped(t *testing.T) {
	r := newRequestRunner()
	r.plannedN = 0
	if r.endedAtStopped() {
		t.Error("a duration run (plannedN=0) must never report 'stopped'")
	}
	r.plannedN = 5
	r.record(200, time.Millisecond, true)
	if !r.endedAtStopped() {
		t.Error("1 of 5 completed must report 'stopped'")
	}
	for i := 0; i < 4; i++ {
		r.record(200, time.Millisecond, true)
	}
	if r.endedAtStopped() {
		t.Error("5 of 5 completed must not report 'stopped'")
	}
}

func TestRunnerStatusText(t *testing.T) {
	tab := NewRequestTab("t")
	if got := tab.runnerStatusText(); got != "Multiple · ready to start" {
		t.Errorf("fresh status = %q", got)
	}
	r := tab.EnsureRun()
	r.started = true
	r.plannedN = 4
	r.record(200, time.Millisecond, true)

	r.running.Store(true)
	if got := tab.runnerStatusText(); !strings.Contains(got, "running 1/4") {
		t.Errorf("running status = %q, want the planned count", got)
	}
	r.plannedN = 0
	if got := tab.runnerStatusText(); !strings.Contains(got, "1 done") {
		t.Errorf("duration-mode running status = %q", got)
	}

	r.running.Store(false)
	r.plannedN = 4
	if got := tab.runnerStatusText(); !strings.Contains(got, "stopped at 1") {
		t.Errorf("stopped status = %q", got)
	}
	r.plannedN = 1
	if got := tab.runnerStatusText(); !strings.Contains(got, "finished 1") {
		t.Errorf("finished status = %q", got)
	}
}

func TestRunnerSendLabel(t *testing.T) {
	tab := NewRequestTab("t")
	if got, _ := tab.runnerSendLabel(); got != "START" {
		t.Errorf("label = %q, want START", got)
	}
	r := tab.EnsureRun()
	r.started = true
	if got, _ := tab.runnerSendLabel(); got != "RERUN" {
		t.Errorf("label = %q, want RERUN", got)
	}
	r.running.Store(true)
	if got, _ := tab.runnerSendLabel(); got != "STOP" {
		t.Errorf("label = %q, want STOP", got)
	}
}

func TestRunnerBackToConfigRefusesWhileRunning(t *testing.T) {
	r := newRequestRunner()
	r.started = true
	r.running.Store(true)
	r.backToConfig()
	if !r.started {
		t.Error("backToConfig must be a no-op while the run is in flight")
	}
	r.running.Store(false)
	r.backToConfig()
	if r.started {
		t.Error("backToConfig must clear started once the run is over")
	}
}

func TestRunnerStopWithoutCancelIsSafe(t *testing.T) {
	r := newRequestRunner()
	r.stop()
}

func waitRunFinished(t *testing.T, r *RequestRunner, d time.Duration) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if r.started && !r.running.Load() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("run did not finish within %v (completed=%d)", d, r.snapshot().completed)
}

func TestStartRun_IterationsCompleteAndRecord(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("X-Run") != "1" {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.AddHeader("X-Run", "1")
	r := tab.EnsureRun()
	r.IterEditor.SetText("6")
	r.WorkEditor.SetText("2")
	r.DelayEditor.SetText("0")

	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 15*time.Second)

	snap := r.snapshot()
	if snap.completed != 6 {
		t.Errorf("completed = %d, want 6", snap.completed)
	}
	if snap.success != 6 || snap.failed != 0 {
		t.Errorf("success/failed = %d/%d, want 6/0", snap.success, snap.failed)
	}
	if atomic.LoadInt32(&hits) != 6 {
		t.Errorf("server saw %d requests, want 6", hits)
	}
	if r.plannedN != 6 {
		t.Errorf("plannedN = %d, want 6", r.plannedN)
	}
	if r.endedAtStopped() {
		t.Error("a fully completed run must not report 'stopped'")
	}
	if snap.elapsed < 0 {
		t.Errorf("elapsed = %v, want a non-negative duration", snap.elapsed)
	}
	if r.sent.Load() != 6 {
		t.Errorf("sent = %d, want 6", r.sent.Load())
	}
	if r.inFlight.Load() != 0 {
		t.Errorf("inFlight = %d, want 0 after the run drains", r.inFlight.Load())
	}
}

func TestStartRun_VariablesCycleAcrossIterations(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Query().Get("id")]++
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL + "/?id={{id}}")
	r := tab.EnsureRun()
	r.IterEditor.SetText("4")
	r.WorkEditor.SetText("1")
	r.addVar()
	r.Variables[0].Name.SetText("id")
	r.Variables[0].Values.SetText("a,b")

	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 15*time.Second)

	mu.Lock()
	defer mu.Unlock()
	if seen["a"] != 2 || seen["b"] != 2 {
		t.Errorf("variable cycling produced %#v, want a=2 b=2", seen)
	}
}

func TestStartRun_StopCancelsEarly(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	defer close(release)

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.IterEditor.SetText("500")
	r.WorkEditor.SetText("2")

	tab.StartRun(context.Background(), new(app.Window), nil)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && r.inFlight.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	tab.EnsureRun().stop()
	waitRunFinished(t, r, 15*time.Second)

	if snap := r.snapshot(); snap.completed >= 500 {
		t.Errorf("completed = %d, want the run cut short", snap.completed)
	}
	if r.running.Load() {
		t.Error("running must be false after the run unwinds")
	}
}

func TestStartRun_DurationModeStopsOnDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.Mode = runByDuration
	r.DurEditor.SetText("1")
	r.WorkEditor.SetText("2")

	start := time.Now()
	tab.StartRun(context.Background(), new(app.Window), nil)
	waitRunFinished(t, r, 30*time.Second)
	elapsed := time.Since(start)

	if r.plannedN != 0 {
		t.Errorf("plannedN = %d, want 0 in duration mode", r.plannedN)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("duration run ended after %v, want ~1s", elapsed)
	}
	if elapsed > 20*time.Second {
		t.Errorf("duration run overran: %v", elapsed)
	}
	if snap := r.snapshot(); snap.completed == 0 {
		t.Error("duration run recorded nothing")
	}
}

func TestStartRun_RejectsSecondConcurrentRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.running.Store(true)
	tab.StartRun(context.Background(), new(app.Window), nil)
	if r.started {
		t.Error("StartRun must bail out while another run is in flight")
	}
	r.running.Store(false)
}

func TestStartRun_BuildFailureLeavesRunnerIdle(t *testing.T) {
	tab := NewRequestTab("t")
	tab.Method = MethodGraphQL
	tab.URLInput.SetText("http://127.0.0.1:1/graphql")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")
	g.Variables.SetText(`{"bad":}`)

	r := tab.EnsureRun()
	tab.StartRun(context.Background(), new(app.Window), nil)
	if r.started || r.running.Load() {
		t.Errorf("a spec build failure must leave the runner idle: started=%v running=%v", r.started, r.running.Load())
	}
}

func TestRunnerAction_DispatchesByState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	r := tab.EnsureRun()
	r.IterEditor.SetText("2")
	r.WorkEditor.SetText("1")

	win := new(app.Window)
	tab.RunnerAction(context.Background(), win, nil)
	waitRunFinished(t, r, 15*time.Second)
	if !r.started {
		t.Fatal("first action must start the run")
	}

	tab.RunnerAction(context.Background(), win, nil)
	if r.started {
		t.Error("a finished run must go back to config on the next action")
	}
}

func TestApplyFormPartsAndURLEncoded(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(file, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}

	tab := NewRequestTab("t")
	tab.applyFormParts([]model.ParsedFormPart{
		{Key: "text", Value: "v", Kind: model.FormPartText},
		{Key: "file", Kind: model.FormPartFile, FilePath: file},
		{Key: "gone", Kind: model.FormPartFile, FilePath: filepath.Join(dir, "missing")},
		{Key: "off", Value: "x", Kind: model.FormPartText, Disabled: true},
	})
	if len(tab.FormParts) != 4 {
		t.Fatalf("FormParts len = %d, want 4", len(tab.FormParts))
	}
	if tab.FormParts[1].FileSize != 10 {
		t.Errorf("file part size = %d, want 10", tab.FormParts[1].FileSize)
	}
	if tab.FormParts[2].FileSize != 0 {
		t.Errorf("missing file size = %d, want 0", tab.FormParts[2].FileSize)
	}
	if !tab.FormParts[3].Disabled {
		t.Error("Disabled flag must carry over")
	}

	tab.applyFormParts(nil)
	if len(tab.FormParts) != 0 {
		t.Errorf("applyFormParts(nil) must clear, got %d", len(tab.FormParts))
	}

	tab.applyURLEncoded([]model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2", Disabled: true}})
	if len(tab.URLEncoded) != 2 || tab.URLEncoded[0].Key.Text() != "a" || !tab.URLEncoded[1].Disabled {
		t.Errorf("applyURLEncoded produced %#v", tab.URLEncoded)
	}
	tab.applyURLEncoded(nil)
	if len(tab.URLEncoded) != 0 {
		t.Errorf("applyURLEncoded(nil) must clear, got %d", len(tab.URLEncoded))
	}
}

func TestExampleStatusText(t *testing.T) {
	cases := []struct {
		name string
		ex   model.ParsedExample
		want string
	}{
		{"code and status", model.ParsedExample{Code: 200, Status: "OK", RespBody: "abc"}, "200 OK"},
		{"code only", model.ParsedExample{Code: 404, RespBody: ""}, "404"},
		{"status only", model.ParsedExample{Status: "Created"}, "Created"},
		{"neither", model.ParsedExample{}, "Example"},
	}
	for _, c := range cases {
		got := exampleStatusText(c.ex)
		if !strings.HasPrefix(got, c.want+"  ") {
			t.Errorf("%s: exampleStatusText = %q, want prefix %q", c.name, got, c.want)
		}
		if !strings.Contains(got, formatSize(int64(len(c.ex.RespBody)))) {
			t.Errorf("%s: exampleStatusText = %q, want the body size appended", c.name, got)
		}
	}
}

func TestExampleMenuLabel(t *testing.T) {
	if got := exampleMenuLabel(0); got != "Example #1" {
		t.Errorf("exampleMenuLabel(0) = %q", got)
	}
	if got := exampleMenuLabel(11); got != "Example #12" {
		t.Errorf("exampleMenuLabel(11) = %q", got)
	}
}

func exampleTab(t *testing.T) *RequestTab {
	t.Helper()
	tab := NewRequestTab("t")
	tab.Method = "PUT"
	tab.LastHTTPMethod = "PUT"
	tab.URLInput.SetText("http://base.test/orig")
	tab.ReqEditor.SetText("original body")
	tab.BodyType = model.BodyRaw
	tab.AddHeader("X-Base", "1")
	tab.applyURLEncoded([]model.ParsedKV{{Key: "b", Value: "2"}})
	tab.Status = "200 OK"
	tab.RespEditor.SetText("original response")
	tab.Examples = []model.ParsedExample{{
		Name:     "sample",
		Method:   "POST",
		URL:      "http://example.test/e",
		Body:     "example body",
		Headers:  map[string]string{"X-Ex": "9"},
		BodyType: model.BodyRaw,
		Code:     201,
		Status:   "Created",
		RespBody: `{"ok":true}`,
	}}
	return tab
}

func TestApplyExampleCapturesAndRestoresBaseState(t *testing.T) {
	th := material.NewTheme()
	tab := exampleTab(t)

	tab.applyExample(th, 0)
	if tab.ExampleSel != 0 {
		t.Fatalf("ExampleSel = %d, want 0", tab.ExampleSel)
	}
	if !tab.BaseState.valid {
		t.Fatal("applying an example must capture the base state first")
	}
	if tab.Method != "POST" || tab.LastHTTPMethod != "POST" {
		t.Errorf("method = %q / last = %q, want POST", tab.Method, tab.LastHTTPMethod)
	}
	if tab.URLInput.Text() != "http://example.test/e" {
		t.Errorf("URL = %q", tab.URLInput.Text())
	}
	if tab.ReqEditor.Text() != "example body" {
		t.Errorf("body = %q", tab.ReqEditor.Text())
	}
	if tab.RespEditor.Text() != `{"ok":true}` {
		t.Errorf("response = %q", tab.RespEditor.Text())
	}
	if !tab.respIsJSON {
		t.Error("a JSON example response must be flagged as JSON")
	}
	if !strings.HasPrefix(tab.Status, "201 Created") {
		t.Errorf("status = %q", tab.Status)
	}
	found := false
	for _, h := range tab.Headers {
		if h.Key.Text() == "X-Ex" {
			found = true
		}
		if h.Key.Text() == "X-Base" && !h.IsGenerated {
			t.Error("the example must replace the base headers")
		}
	}
	if !found {
		t.Error("example headers were not applied")
	}

	tab.applyExample(th, -1)
	if tab.ExampleSel != -1 {
		t.Errorf("ExampleSel = %d, want -1", tab.ExampleSel)
	}
	if tab.Method != "PUT" || tab.URLInput.Text() != "http://base.test/orig" {
		t.Errorf("base state not restored: method=%q url=%q", tab.Method, tab.URLInput.Text())
	}
	if tab.ReqEditor.Text() != "original body" {
		t.Errorf("request body not restored: %q", tab.ReqEditor.Text())
	}
	if tab.RespEditor.Text() != "original response" {
		t.Errorf("response not restored: %q", tab.RespEditor.Text())
	}
	if tab.Status != "200 OK" {
		t.Errorf("status not restored: %q", tab.Status)
	}
	if len(tab.URLEncoded) != 1 || tab.URLEncoded[0].Key.Text() != "b" {
		t.Errorf("url-encoded parts not restored: %#v", tab.URLEncoded)
	}
	if tab.BaseState.valid {
		t.Error("restoring must consume the captured base state")
	}
	restoredBase := false
	for _, h := range tab.Headers {
		if h.Key.Text() == "X-Base" {
			restoredBase = true
		}
	}
	if !restoredBase {
		t.Error("base headers were not restored")
	}
}

func TestApplyExampleOutOfRangeWithoutBaseStateIsSafe(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://keep.test")
	tab.applyExample(nil, 5)
	if tab.ExampleSel != -1 {
		t.Errorf("ExampleSel = %d, want -1", tab.ExampleSel)
	}
	if tab.URLInput.Text() != "http://keep.test" {
		t.Errorf("URL must be untouched when there is nothing to restore: %q", tab.URLInput.Text())
	}
}

func TestApplyExampleSwitchingExamplesKeepsOriginalBase(t *testing.T) {
	th := material.NewTheme()
	tab := exampleTab(t)
	tab.Examples = append(tab.Examples, model.ParsedExample{
		Method: "DELETE", URL: "http://example.test/second", Code: 204,
	})

	tab.applyExample(th, 0)
	tab.applyExample(th, 1)
	if tab.URLInput.Text() != "http://example.test/second" {
		t.Errorf("URL = %q", tab.URLInput.Text())
	}
	tab.applyExample(th, -1)
	if tab.URLInput.Text() != "http://base.test/orig" {
		t.Errorf("switching examples must not overwrite the original base: %q", tab.URLInput.Text())
	}
}

func TestApplyExampleWSMethodDoesNotBecomeLastHTTPMethod(t *testing.T) {
	tab := NewRequestTab("t")
	tab.LastHTTPMethod = "GET"
	tab.Examples = []model.ParsedExample{{Method: MethodWS, URL: "ws://x.test"}}
	tab.applyExample(nil, 0)
	if tab.Method != MethodWS {
		t.Errorf("Method = %q, want WS", tab.Method)
	}
	if tab.LastHTTPMethod != "GET" {
		t.Errorf("LastHTTPMethod = %q, want the previous HTTP method preserved", tab.LastHTTPMethod)
	}
}

func TestApplyExampleBinarySizeFromDisk(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "up.bin")
	if err := os.WriteFile(file, []byte("abcd"), 0o600); err != nil {
		t.Fatal(err)
	}
	tab := NewRequestTab("t")
	tab.Examples = []model.ParsedExample{
		{URL: "http://x.test", BinaryPath: file},
		{URL: "http://x.test", BinaryPath: filepath.Join(dir, "missing")},
	}
	tab.applyExample(nil, 0)
	if tab.BinaryFileSize != 4 {
		t.Errorf("BinaryFileSize = %d, want 4", tab.BinaryFileSize)
	}
	tab.applyExample(nil, 1)
	if tab.BinaryFileSize != 0 {
		t.Errorf("BinaryFileSize = %d, want 0 for a missing file", tab.BinaryFileSize)
	}
}
