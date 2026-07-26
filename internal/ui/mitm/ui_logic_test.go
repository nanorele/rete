package mitm

import (
	"image"
	"strings"
	"testing"
	"time"
)

func TestPipeline_SerializeRoundTrip(t *testing.T) {
	headers := [][2]string{{"Zeta", "1"}, {"Alpha", "2"}, {"Content-Length", "5"}}
	raw := serializeRequest("POST", "/submit", "HTTP/1.1", headers, []byte("hello"))
	s := string(raw)
	if !strings.HasPrefix(s, "POST /submit HTTP/1.1\r\n") {
		t.Fatalf("bad request line: %q", s)
	}
	if strings.Index(s, "Alpha:") > strings.Index(s, "Zeta:") {
		t.Error("writeHeaderPairs must emit headers in sorted order")
	}

	er, ok := parseRequestRaw(raw)
	if !ok {
		t.Fatal("parseRequestRaw failed on our own output")
	}
	if er.Method != "POST" || er.RequestURI != "/submit" {
		t.Errorf("method/uri = %q %q", er.Method, er.RequestURI)
	}
	if string(er.Body) != "hello" {
		t.Errorf("body = %q", er.Body)
	}
	if headerVal(er.Headers, "alpha") != "2" {
		t.Errorf("headers lost: %+v", er.Headers)
	}
}

func TestPipeline_SerializeResponseRoundTrip(t *testing.T) {
	headers := [][2]string{{"Content-Type", "text/plain"}, {"Content-Length", "2"}}
	raw := serializeResponse("200 OK", "HTTP/1.1", headers, []byte("ok"))
	if !strings.HasPrefix(string(raw), "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("bad status line: %q", raw)
	}
	er, ok := parseResponseRaw(raw)
	if !ok {
		t.Fatal("parseResponseRaw failed on our own output")
	}
	if er.Status != "200 OK" || string(er.Body) != "ok" {
		t.Errorf("status=%q body=%q", er.Status, er.Body)
	}
}

func TestPipeline_SerializeDefaultsProto(t *testing.T) {
	if got := string(serializeRequest("GET", "/", "", nil, nil)); !strings.HasPrefix(got, "GET / HTTP/1.1\r\n") {
		t.Errorf("empty proto must default to HTTP/1.1, got %q", got)
	}
	if got := string(serializeResponse("204 No Content", "", nil, nil)); !strings.HasPrefix(got, "HTTP/1.1 204") {
		t.Errorf("empty proto must default to HTTP/1.1, got %q", got)
	}
}

func TestPipeline_ParseRejectsGarbage(t *testing.T) {
	if _, ok := parseRequestRaw([]byte("this is not http")); ok {
		t.Error("garbage must not parse as a request")
	}
	if _, ok := parseResponseRaw([]byte("this is not http")); ok {
		t.Error("garbage must not parse as a response")
	}
}

func TestPipeline_NormalizeCRLF(t *testing.T) {
	cases := map[string]string{
		"a\nb":       "a\r\nb",
		"a\r\nb":     "a\r\nb",
		"a\r\n\nb":   "a\r\n\r\nb",
		"no newline": "no newline",
		"":           "",
	}
	for in, want := range cases {
		if got := string(normalizeCRLF([]byte(in))); got != want {
			t.Errorf("normalizeCRLF(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPipeline_ParseToleratesBareLF(t *testing.T) {
	raw := []byte("GET /x HTTP/1.1\nHost: example.com\nX-Test: 1\n\nbody")
	er, ok := parseRequestRaw(raw)
	if !ok {
		t.Fatal("bare-LF request must parse after normalisation")
	}
	if er.RequestURI != "/x" || headerVal(er.Headers, "X-Test") != "1" {
		t.Errorf("parsed wrong: %+v", er)
	}
}

func TestPipeline_PairsToHeader(t *testing.T) {
	h := pairsToHeader([][2]string{{"A", "1"}, {"A", "2"}, {"B", "3"}})
	if got := h.Values("A"); len(got) != 2 {
		t.Errorf("duplicate header keys must be preserved, got %v", got)
	}
	if h.Get("B") != "3" {
		t.Errorf("B = %q", h.Get("B"))
	}
}

func TestPipeline_IsWebSocketUpgrade(t *testing.T) {
	cases := []struct {
		upgrade, connection string
		want                bool
	}{
		{"websocket", "Upgrade", true},
		{"WebSocket", "keep-alive, Upgrade", true},
		{"websocket", "keep-alive", false},
		{"h2c", "Upgrade", false},
		{"", "", false},
	}
	for _, c := range cases {
		h := pairsToHeader([][2]string{{"Upgrade", c.upgrade}, {"Connection", c.connection}})
		if got := isWebSocketUpgrade(h); got != c.want {
			t.Errorf("isWebSocketUpgrade(%q,%q) = %v, want %v", c.upgrade, c.connection, got, c.want)
		}
	}
}

func TestMatchReplace_HeadersAddReplaceDelete(t *testing.T) {
	cases := []struct {
		name     string
		rule     MatchReplaceRule
		in, want [][2]string
	}{
		{
			"replace-existing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Server", Replacement: "tracto"},
			[][2]string{{"Server", "nginx"}, {"X", "1"}},
			[][2]string{{"Server", "tracto"}, {"X", "1"}},
		},
		{
			"delete-existing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"},
			[][2]string{{"X-Frame-Options", "DENY"}, {"X", "1"}},
			[][2]string{{"X", "1"}},
		},
		{
			"add-missing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-New", Replacement: "v"},
			[][2]string{{"X", "1"}},
			[][2]string{{"X", "1"}, {"X-New", "v"}},
		},
		{
			"case-insensitive-name",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "server", Replacement: "t"},
			[][2]string{{"SERVER", "nginx"}},
			[][2]string{{"SERVER", "t"}},
		},
		{
			"disabled-noop",
			MatchReplaceRule{Enabled: false, Type: MRResponse, Area: MRHeader, Pattern: "Server", Replacement: "t"},
			[][2]string{{"Server", "nginx"}},
			[][2]string{{"Server", "nginx"}},
		},
		{
			"wrong-type-noop",
			MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "Server", Replacement: "t"},
			[][2]string{{"Server", "nginx"}},
			[][2]string{{"Server", "nginx"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMatchReplace()
			m.Add(c.rule)
			got := m.ApplyHeaders(MRResponse, append([][2]string(nil), c.in...))
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("header %d = %v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestMatchReplace_BodyAndFirstLine(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "secret", Replacement: "REDACTED"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRFirstLine, Pattern: "/old", Replacement: "/new"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: `v\d+`, IsRegex: true, Replacement: "vX"})

	if got := string(m.ApplyBody(MRResponse, []byte("a secret v12 b"))); got != "a REDACTED vX b" {
		t.Errorf("body = %q", got)
	}
	if got := m.ApplyFirstLine(MRRequest, "/old/path"); got != "/new/path" {
		t.Errorf("first line = %q", got)
	}

	orig := []byte("nothing to change")
	if got := m.ApplyBody(MRResponse, orig); &got[0] != &orig[0] {
		t.Error("an unchanged body must be returned without copying")
	}
	if got := m.ApplyFirstLine(MRResponse, "200 OK"); got != "200 OK" {
		t.Errorf("wrong-type first line must be untouched, got %q", got)
	}
}

func TestMatchReplace_InvalidRegexDoesNotPanic(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "([unclosed", IsRegex: true, Replacement: "x"})
	if got := string(m.ApplyBody(MRResponse, []byte("unrelated body"))); got != "unrelated body" {
		t.Errorf("an uncompilable regex must leave non-matching text alone, got %q", got)
	}
}

func TestMatchReplace_EmptyPatternIsNoop(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "", Replacement: "X"})
	if got := string(m.ApplyBody(MRResponse, []byte("abc"))); got != "abc" {
		t.Errorf("an empty pattern must be a no-op, got %q", got)
	}
}

func TestMatchReplace_RemoveUpdateMove(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a", Replacement: "1"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "b", Replacement: "2"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "c", Replacement: "3"})

	m.Move(0, 1)
	if got := m.Snapshot(); got[0].Pattern != "b" || got[1].Pattern != "a" {
		t.Errorf("Move(0,1) = %q,%q", got[0].Pattern, got[1].Pattern)
	}
	m.Move(0, -1)
	m.Move(2, 5)
	m.Move(-1, 1)
	if len(m.Snapshot()) != 3 {
		t.Error("out-of-range moves must not change the list")
	}

	m.Update(1, func(r *MatchReplaceRule) { r.Pattern = "z"; r.Enabled = false })
	if got := m.Snapshot(); got[1].Pattern != "z" || got[1].Enabled {
		t.Errorf("Update failed: %+v", got[1])
	}
	m.Update(99, func(r *MatchReplaceRule) { r.Pattern = "boom" })

	m.Remove(0)
	if len(m.Snapshot()) != 2 {
		t.Errorf("Remove failed, len=%d", len(m.Snapshot()))
	}
	m.Remove(99)
	m.Remove(-1)
	if len(m.Snapshot()) != 2 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestMatchReplace_UpdateRecompilesRegex(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a+", IsRegex: true, Replacement: "X"})
	if got := string(m.ApplyBody(MRResponse, []byte("aaa b"))); got != "X b" {
		t.Fatalf("initial = %q", got)
	}
	m.Update(0, func(r *MatchReplaceRule) { r.Pattern = "b+" })
	if got := string(m.ApplyBody(MRResponse, []byte("aaa bbb"))); got != "aaa X" {
		t.Errorf("Update must invalidate the cached regex, got %q", got)
	}
}

func TestMatchReplace_EnabledFor(t *testing.T) {
	m := NewMatchReplace()
	if m.enabledFor(MRResponse, MRBody) {
		t.Error("an empty ruleset must report nothing enabled")
	}
	m.Add(MatchReplaceRule{Enabled: false, Type: MRResponse, Area: MRBody, Pattern: "a"})
	if m.enabledFor(MRResponse, MRBody) {
		t.Error("a disabled rule must not count")
	}
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a"})
	if !m.enabledFor(MRResponse, MRBody) {
		t.Error("an enabled rule must be reported")
	}
	if m.enabledFor(MRRequest, MRBody) || m.enabledFor(MRResponse, MRHeader) {
		t.Error("type/area must be matched exactly")
	}
}

func TestScope_Rules(t *testing.T) {
	f := &Flow{Host: "api.example.com", Scheme: "https", Port: "443", Path: "/v1/users"}
	cases := []struct {
		name  string
		rules []ScopeRule
		want  bool
	}{
		{"empty", nil, true},
		{"include-match", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"}}, true},
		{"include-miss", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "other"}}, false},
		{"exclude-match", []ScopeRule{{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/v1"}}, false},
		{"exclude-miss", []ScopeRule{{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health"}}, true},
		{"exclude-wins", []ScopeRule{
			{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"},
			{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/v1"},
		}, false},
		{"disabled-ignored", []ScopeRule{{Enabled: false, Kind: ScopeInclude, Field: "host", Pattern: "other"}}, true},
		{"protocol", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "protocol", Pattern: "https"}}, true},
		{"port", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "port", Pattern: "443"}}, true},
		{"unknown-field-uses-host", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "weird", Pattern: "api"}}, true},
		{"regex", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^api\.`, IsRegex: true}}, true},
		{"regex-miss", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^cdn\.`, IsRegex: true}}, false},
		{"bad-regex", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "([", IsRegex: true}}, false},
		{"case-insensitive", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "EXAMPLE"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewScope()
			for _, r := range c.rules {
				s.Add(r)
			}
			if got := s.InScope(f); got != c.want {
				t.Errorf("InScope = %v, want %v", got, c.want)
			}
		})
	}
}

func TestScope_RemoveUpdateSnapshot(t *testing.T) {
	s := NewScope()
	s.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "a"})
	s.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "b"})
	if s.Len() != 2 {
		t.Fatalf("Len = %d", s.Len())
	}
	s.Update(0, func(r *ScopeRule) { r.Pattern = "z" })
	if got := s.Snapshot(); got[0].Pattern != "z" {
		t.Errorf("Update failed: %+v", got[0])
	}
	s.Update(99, func(r *ScopeRule) { r.Pattern = "boom" })
	s.Remove(0)
	if s.Len() != 1 {
		t.Errorf("Remove failed, len=%d", s.Len())
	}
	s.Remove(99)
	s.Remove(-1)
	if s.Len() != 1 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestScope_UpdateRecompilesRegex(t *testing.T) {
	s := NewScope()
	s.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^api\.`, IsRegex: true})
	if !s.InScope(&Flow{Host: "api.example.com"}) {
		t.Fatal("initial regex should match")
	}
	s.Update(0, func(r *ScopeRule) { r.Pattern = `^cdn\.` })
	if s.InScope(&Flow{Host: "api.example.com"}) {
		t.Error("Update must invalidate the cached regex")
	}
	if !s.InScope(&Flow{Host: "cdn.example.com"}) {
		t.Error("the updated regex must apply")
	}
}

func TestInterceptRules_ShouldIntercept(t *testing.T) {
	f := &Flow{
		Host: "api.example.com", ClientAddr: "10.0.0.5:33", Method: "POST",
		URL: "https://api.example.com/v1", Path: "/v1/users.json?token=abc",
		StatusCode: 404,
		ReqHeaders: [][2]string{{"X-Auth", "1"}},
		RespHeaders: [][2]string{{"Content-Type", "application/json; charset=utf-8"}},
	}
	cases := []struct {
		name    string
		conds   []InterceptCond
		enabled bool
		inScope bool
		want    bool
	}{
		{"no-rules", nil, true, true, true},
		{"disabled-set", []InterceptCond{{Enabled: true, Field: CondHost, Value: "nope"}}, false, true, true},
		{"all-conds-disabled", []InterceptCond{{Enabled: false, Field: CondHost, Value: "nope"}}, true, true, true},
		{"host-hit", []InterceptCond{{Enabled: true, Field: CondHost, Value: "example.com"}}, true, true, true},
		{"host-miss", []InterceptCond{{Enabled: true, Field: CondHost, Value: "other.org"}}, true, true, false},
		{"ip", []InterceptCond{{Enabled: true, Field: CondIP, Value: "10.0.0.5"}}, true, true, true},
		{"method", []InterceptCond{{Enabled: true, Field: CondMethod, Value: "post"}}, true, true, true},
		{"method-miss", []InterceptCond{{Enabled: true, Field: CondMethod, Value: "GET"}}, true, true, false},
		{"url", []InterceptCond{{Enabled: true, Field: CondURL, Value: "/v1"}}, true, true, true},
		{"filetype", []InterceptCond{{Enabled: true, Field: CondFileType, Value: "json"}}, true, true, true},
		{"filetype-dotted", []InterceptCond{{Enabled: true, Field: CondFileType, Value: ".json"}}, true, true, true},
		{"filetype-miss", []InterceptCond{{Enabled: true, Field: CondFileType, Value: "css"}}, true, true, false},
		{"mime", []InterceptCond{{Enabled: true, Field: CondMIME, Value: "application/json"}}, true, true, true},
		{"status", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "404"}}, true, true, true},
		{"status-miss", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "200"}}, true, true, false},
		{"status-nan", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "abc"}}, true, true, false},
		{"param", []InterceptCond{{Enabled: true, Field: CondParam, Value: "token"}}, true, true, true},
		{"header", []InterceptCond{{Enabled: true, Field: CondHeader, Value: "X-Auth"}}, true, true, true},
		{"header-miss", []InterceptCond{{Enabled: true, Field: CondHeader, Value: "X-None"}}, true, true, false},
		{"scope-true", []InterceptCond{{Enabled: true, Field: CondScope}}, true, true, true},
		{"scope-false", []InterceptCond{{Enabled: true, Field: CondScope}}, true, false, false},
		{"unknown-field", []InterceptCond{{Enabled: true, Field: "bogus", Value: "x"}}, true, true, false},
		{"and-both-hit", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "example"},
			{Enabled: true, Field: CondMethod, Value: "POST"},
		}, true, true, true},
		{"and-one-miss", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "example"},
			{Enabled: true, Field: CondMethod, Value: "GET"},
		}, true, true, false},
		{"or-one-hit", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "nope"},
			{Enabled: true, Or: true, Field: CondMethod, Value: "POST"},
		}, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ir := NewInterceptRules()
			ir.SetEnabled(HeldRequest, c.enabled)
			for _, cc := range c.conds {
				ir.Add(HeldRequest, cc)
			}
			if got := ir.ShouldIntercept(HeldRequest, f, c.inScope); got != c.want {
				t.Errorf("ShouldIntercept = %v, want %v", got, c.want)
			}
		})
	}
}

func TestInterceptRules_ReqAndRespAreSeparate(t *testing.T) {
	ir := NewInterceptRules()
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "req-only"})
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 {
		t.Errorf("request set = %d conds", len(conds))
	}
	if _, conds := ir.Snapshot(HeldResponse); len(conds) != 0 {
		t.Errorf("response set must stay empty, got %d", len(conds))
	}
	if _, conds := ir.Snapshot("anything-else"); len(conds) != 1 {
		t.Error("an unknown kind must fall back to the request set")
	}

	ir.SetEnabled(HeldResponse, false)
	if enabled, _ := ir.Snapshot(HeldResponse); enabled {
		t.Error("SetEnabled(false) not reflected")
	}
	if enabled, _ := ir.Snapshot(HeldRequest); !enabled {
		t.Error("SetEnabled must not leak across kinds")
	}
}

func TestInterceptRules_RemoveUpdate(t *testing.T) {
	ir := NewInterceptRules()
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "a"})
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "b"})

	ir.Update(HeldRequest, 0, func(c *InterceptCond) { c.Value = "z" })
	if _, conds := ir.Snapshot(HeldRequest); conds[0].Value != "z" {
		t.Errorf("Update failed: %+v", conds[0])
	}
	ir.Update(HeldRequest, 99, func(c *InterceptCond) { c.Value = "boom" })

	ir.Remove(HeldRequest, 0)
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 || conds[0].Value != "b" {
		t.Errorf("Remove failed: %+v", conds)
	}
	ir.Remove(HeldRequest, 99)
	ir.Remove(HeldRequest, -1)
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestPathOnlyAndHeaderVal(t *testing.T) {
	cases := map[string]string{
		"/a/b.json?x=1":  "/a/b.json",
		"/a/b.json#frag": "/a/b.json",
		"/a/b.json":      "/a/b.json",
		"":               "",
		"?only":          "",
	}
	for in, want := range cases {
		if got := pathOnly(in); got != want {
			t.Errorf("pathOnly(%q) = %q, want %q", in, got, want)
		}
	}

	h := [][2]string{{"Content-Type", "text/html"}, {"X-A", "1"}}
	if got := headerVal(h, "content-type"); got != "text/html" {
		t.Errorf("headerVal case-insensitive = %q", got)
	}
	if got := headerVal(h, "missing"); got != "" {
		t.Errorf("headerVal missing = %q", got)
	}
	if got := headerVal(nil, "x"); got != "" {
		t.Errorf("headerVal nil = %q", got)
	}
}

func TestTargets_AddValidationAndDuplicates(t *testing.T) {
	tg := NewTargets()
	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"EXAMPLE.com", false},
		{"sub.example.com", true},
		{"*.wild.example.com", true},
		{"example.com:443", false},
		{"nodot", false},
		{"", false},
		{"-bad.example.com", false},
		{"bad-.example.com", false},
		{"under_score.com", false},
		{strings.Repeat("a", 64) + ".com", false},
	}
	for _, c := range cases {
		got := tg.Add(&Target{Domain: c.domain})
		if got != c.want {
			t.Errorf("Add(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}

func TestTargets_AddAppliesDefaults(t *testing.T) {
	tg := NewTargets()
	if !tg.Add(&Target{Domain: "a.example.com"}) {
		t.Fatal("add failed")
	}
	v := tg.Snapshot()[0]
	if v.Upstream != UpstreamAuto || v.TLS != TLSDecrypt || v.Status != StatusWaiting {
		t.Errorf("defaults wrong: %+v", v)
	}
}

func TestTargets_MatchWildcard(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "exact.example.com"})
	tg.Add(&Target{Domain: "*.wild.example.com"})

	cases := []struct {
		host string
		want bool
	}{
		{"exact.example.com", true},
		{"EXACT.example.com", true},
		{"exact.example.com:443", true},
		{"a.wild.example.com", true},
		{"a.b.wild.example.com", true},
		{"wild.example.com", false},
		{"other.example.com", false},
		{"", false},
	}
	for _, c := range cases {
		_, ok := tg.Match(c.host)
		if ok != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.host, ok, c.want)
		}
	}
}

func TestTargets_UpdateRemoveAndStatus(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "a.example.com"})
	tg.Update("A.EXAMPLE.COM", func(x *Target) { x.UpstreamAddr = "1.2.3.4:80" })
	if got := tg.Snapshot()[0].UpstreamAddr; got != "1.2.3.4:80" {
		t.Errorf("Update must normalise the domain, got %q", got)
	}
	tg.Update("missing.example.com", func(x *Target) { x.UpstreamAddr = "boom" })

	m, ok := tg.Match("a.example.com")
	if !ok {
		t.Fatal("match failed")
	}
	if same, _ := tg.Match("a.example.com"); same == m {
		t.Error("Match must return a copy, not the shared internal pointer")
	}
	tg.markRequest(m.Domain)
	if v := tg.Snapshot()[0]; v.Status != StatusProxying || v.Requests != 1 {
		t.Errorf("markRequest: %+v", v)
	}
	tg.markError(m.Domain, "dial failed")
	if v := tg.Snapshot()[0]; v.Status != StatusError || v.LastErr != "dial failed" {
		t.Errorf("markError: %+v", v)
	}

	tg.Remove("missing.example.com")
	if tg.Len() != 1 {
		t.Errorf("removing a missing domain changed the list, len=%d", tg.Len())
	}
	tg.Remove("A.example.com")
	if tg.Len() != 0 {
		t.Errorf("Remove failed, len=%d", tg.Len())
	}
}

func TestTargets_Notify(t *testing.T) {
	tg := NewTargets()
	var n int
	tg.SetNotify(func() { n++ })
	tg.Add(&Target{Domain: "a.example.com"})
	if n == 0 {
		t.Error("Add must notify")
	}
	tg.SetNotify(nil)
	before := n
	tg.Remove("a.example.com")
	if n != before {
		t.Error("SetNotify(nil) must stop notifications")
	}
}

func TestHostsLine(t *testing.T) {
	cases := map[string]string{
		"example.com":      "127.0.0.1    example.com",
		"*.example.com":    "127.0.0.1    example.com",
		"EXAMPLE.com:443 ": "127.0.0.1    example.com",
	}
	for in, want := range cases {
		if got := HostsLine(in); got != want {
			t.Errorf("HostsLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRules_SetGetRemove(t *testing.T) {
	r := NewRules()
	r.Set("  Example.COM:8080 ", HostRule{Delay: 50 * time.Millisecond, UseDoH: true})
	if got, ok := r.Get("example.com"); !ok || got.Delay != 50*time.Millisecond || !got.UseDoH {
		t.Errorf("Get after normalised Set = %+v ok=%v", got, ok)
	}
	if _, ok := r.Get("other.com"); ok {
		t.Error("missing host must report not found")
	}
	r.Set("", HostRule{Delay: time.Second})
	if r.Len() != 1 {
		t.Errorf("an empty host must be rejected, len=%d", r.Len())
	}

	r.Set("b.com", HostRule{})
	snap := r.Snapshot()
	if len(snap) != 2 || snap[0].Host != "b.com" || snap[1].Host != "example.com" {
		t.Errorf("Snapshot must be sorted by host: %+v", snap)
	}

	r.Remove("EXAMPLE.com")
	if r.Len() != 1 {
		t.Errorf("Remove failed, len=%d", r.Len())
	}
	r.Remove("nonexistent.com")
	if r.Len() != 1 {
		t.Error("removing a missing host must not change the map")
	}
}

func TestNormalizeRuleHostAndDomain(t *testing.T) {
	cases := map[string]string{
		"  Example.COM  ": "example.com",
		"example.com:443": "example.com",
		"":                "",
		"[::1]:80":        "::1",
	}
	for in, want := range cases {
		if got := normalizeRuleHost(in); got != want {
			t.Errorf("normalizeRuleHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeDomain("Example.COM."); got != "example.com" {
		t.Errorf("normalizeDomain trailing dot = %q", got)
	}
}

func TestStore_DeleteAndAnnotate(t *testing.T) {
	s := NewStore()
	a := s.Add(&Flow{Host: "a"})
	b := s.Add(&Flow{Host: "b"})
	c := s.Add(&Flow{Host: "c"})

	s.SetAnnotation(b.ID, "red", "look here")
	got := s.FindByID(b.ID)
	if got == nil || got.Highlight != "red" || got.Comment != "look here" {
		t.Errorf("SetAnnotation failed: %+v", got)
	}
	s.SetAnnotation(99999, "blue", "nope")

	s.Delete(b.ID)
	if s.Len() != 2 {
		t.Fatalf("Delete failed, len=%d", s.Len())
	}
	if s.FindByID(b.ID) != nil {
		t.Error("deleted flow is still findable")
	}
	if s.FindByID(a.ID) == nil || s.FindByID(c.ID) == nil {
		t.Error("Delete removed the wrong flows")
	}
	s.Delete(99999)
	if s.Len() != 2 {
		t.Error("deleting a missing ID must not change the store")
	}
}

func TestStore_SnapshotIsolation(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a", ReqBody: []byte("body"), ReqHeaders: [][2]string{{"X", "1"}}})

	snap := s.Snapshot()[0]
	snap.ReqBody[0] = 'B'
	snap.ReqHeaders[0][1] = "mutated"
	snap.Host = "changed"

	again := s.Snapshot()[0]
	if string(again.ReqBody) != "body" || again.ReqHeaders[0][1] != "1" || again.Host != "a" {
		t.Errorf("Snapshot must deep-copy: %+v", again)
	}

	meta := s.SnapshotMeta()[0]
	if meta.ReqBody != nil || meta.RespBody != nil || meta.ReqHeaders != nil {
		t.Error("SnapshotMeta must drop bodies and headers")
	}
	if meta.Host != "a" {
		t.Errorf("SnapshotMeta must keep metadata, got %q", meta.Host)
	}
}

func TestStore_AtAndMarkAllEnded(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a"})
	s.Add(&Flow{Host: "b", Ended: time.Unix(1, 0)})

	if got := s.At(0); got == nil || got.Host != "a" {
		t.Errorf("At(0) = %+v", got)
	}
	if s.At(-1) != nil || s.At(99) != nil {
		t.Error("out-of-range At must return nil")
	}
	if !s.At(0).Live() {
		t.Error("a flow without Ended must be Live")
	}

	s.MarkAllEnded()
	for i, f := range s.Snapshot() {
		if f.Ended.IsZero() {
			t.Errorf("flow %d still has a zero Ended", i)
		}
	}
	if got := s.Snapshot()[1].Ended; !got.Equal(time.Unix(1, 0)) {
		t.Errorf("MarkAllEnded must not overwrite an existing Ended, got %v", got)
	}
}

func TestStore_ClearAndNotify(t *testing.T) {
	s := NewStore()
	var n int
	s.SetNotify(func() { n++ })
	s.Add(&Flow{Host: "a"})
	if n == 0 {
		t.Error("Add must notify")
	}
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear failed")
	}
	s.SetNotify(nil)
	before := n
	s.Add(&Flow{Host: "b"})
	if n != before {
		t.Error("SetNotify(nil) must stop notifications")
	}
}

func TestWSStore_Basics(t *testing.T) {
	s := NewWSStore()
	var n int
	s.SetNotify(func() { n++ })
	s.Add(&WSMessage{FlowID: 1, Payload: []byte("hi"), Opcode: 1})
	s.Add(&WSMessage{FlowID: 1, Payload: []byte("there"), Opcode: 2})
	if s.Len() != 2 || n != 2 {
		t.Fatalf("len=%d notifies=%d", s.Len(), n)
	}

	snap := s.Snapshot()
	if snap[0].ID == 0 || snap[1].ID <= snap[0].ID {
		t.Errorf("IDs must be assigned and increasing: %d %d", snap[0].ID, snap[1].ID)
	}
	if snap[0].Time.IsZero() {
		t.Error("Add must stamp a time")
	}
	snap[0].Payload[0] = 'X'
	if string(s.Snapshot()[0].Payload) != "hi" {
		t.Error("Snapshot must deep-copy payloads")
	}

	found := s.FindByID(snap[1].ID)
	if found == nil || string(found.Payload) != "there" {
		t.Errorf("FindByID = %+v", found)
	}
	if s.FindByID(99999) != nil {
		t.Error("FindByID must return nil for a missing ID")
	}

	s.SetNotify(nil)
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear failed")
	}
}

func TestWSOpcodeName(t *testing.T) {
	cases := map[byte]string{0x1: "text", 0x2: "binary", 0x8: "close", 0x9: "ping", 0xA: "pong", 0x0: "cont", 0x7: "cont"}
	for op, want := range cases {
		if got := WSOpcodeName(op); got != want {
			t.Errorf("WSOpcodeName(%#x) = %q, want %q", op, got, want)
		}
	}
}

func TestDirNameAndHighlightColor(t *testing.T) {
	if dirName(true) != "client → server" || dirName(false) != "server → client" {
		t.Error("dirName wrong")
	}
	keys := annotateColorKeys()
	if len(keys) != 6 || keys[0] != "" {
		t.Errorf("annotateColorKeys = %v", keys)
	}
	if highlightColor("").A != 0 {
		t.Error("the empty key must yield a transparent colour")
	}
	if highlightColor("nonsense").A != 0 {
		t.Error("an unknown key must yield a transparent colour")
	}
	for _, k := range keys[1:] {
		if highlightColor(k).A == 0 {
			t.Errorf("colour %q must be opaque", k)
		}
	}
}

func TestFlowAsTextAndCurl(t *testing.T) {
	f := &Flow{
		Method: "POST", Path: "/submit", Version: "HTTP/1.1",
		URL:        "https://example.com/submit",
		ReqHeaders: [][2]string{{"Host", "example.com"}, {"X-A", "1"}},
		ReqBody:    []byte("payload"),
		Status:     "200 OK",
		RespHeaders: [][2]string{{"Content-Type", "text/plain"}},
		RespBody:    []byte("done"),
	}
	req := flowAsText(f, false)
	if !strings.HasPrefix(req, "POST /submit HTTP/1.1\n") || !strings.Contains(req, "X-A: 1") || !strings.HasSuffix(req, "payload") {
		t.Errorf("request text = %q", req)
	}
	resp := flowAsText(f, true)
	if !strings.HasPrefix(resp, "200 OK\n") || !strings.HasSuffix(resp, "done") {
		t.Errorf("response text = %q", resp)
	}

	curl := asCurl(f)
	if !strings.Contains(curl, "curl -X POST 'https://example.com/submit'") {
		t.Errorf("curl = %q", curl)
	}
	if strings.Contains(curl, "-H 'Host:") {
		t.Error("asCurl must skip the Host header")
	}
	if !strings.Contains(curl, "-H 'X-A: 1'") || !strings.Contains(curl, "--data-binary 'payload'") {
		t.Errorf("curl = %q", curl)
	}
	if got := asCurl(&Flow{Method: "GET", URL: "https://x/"}); strings.Contains(got, "--data-binary") {
		t.Errorf("a bodyless flow must not emit --data-binary: %q", got)
	}
}

func TestParseParams(t *testing.T) {
	f := &Flow{
		Path:       "/search?q=go&page=2",
		ReqHeaders: [][2]string{{"Content-Type", "application/x-www-form-urlencoded"}},
		ReqBody:    []byte("extra=1"),
	}
	got := parseParams(f)
	keys := map[string]string{}
	for _, kv := range got {
		keys[kv[0]] = kv[1]
	}
	if keys["q"] != "go" || keys["page"] != "2" {
		t.Errorf("query params missing: %+v", got)
	}
	if keys["(body) extra"] != "1" {
		t.Errorf("form body params missing: %+v", got)
	}

	if got := parseParams(&Flow{Path: "/no-query"}); len(got) != 0 {
		t.Errorf("a path with no query must yield nothing, got %+v", got)
	}
	if got := parseParams(&Flow{Path: "/x", ReqBody: []byte("a=1")}); len(got) != 0 {
		t.Errorf("a body without a form content-type must be ignored, got %+v", got)
	}
}

func TestParseCookies(t *testing.T) {
	req := parseCookies([][2]string{{"Cookie", "sid=abc; theme=dark; broken"}}, false)
	if len(req) != 2 || req[0][0] != "sid" || req[1][1] != "dark" {
		t.Errorf("request cookies = %+v", req)
	}
	resp := parseCookies([][2]string{
		{"Set-Cookie", "sid=xyz; Path=/; HttpOnly"},
		{"Set-Cookie", "other=2"},
	}, true)
	if len(resp) != 2 || resp[0][0] != "sid" || resp[0][1] != "xyz" || resp[1][1] != "2" {
		t.Errorf("response cookies = %+v", resp)
	}
	if got := parseCookies([][2]string{{"Cookie", "a=1"}}, true); len(got) != 0 {
		t.Error("Cookie headers must be ignored in response mode")
	}
	if got := parseCookies(nil, false); len(got) != 0 {
		t.Errorf("nil headers = %+v", got)
	}
}

func TestHexDump(t *testing.T) {
	got := hexDump([]byte("AB\x00"))
	if !strings.HasPrefix(got, "00000000  41 42 00 ") {
		t.Errorf("hex bytes wrong: %q", got)
	}
	if !strings.Contains(got, "|AB.|") {
		t.Errorf("ascii gutter wrong: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("must end with a newline: %q", got)
	}
	if hexDump(nil) != "" {
		t.Error("empty input must produce empty output")
	}

	long := hexDump(make([]byte, 40*1024))
	lines := strings.Count(long, "\n")
	if lines != 32*1024/16 {
		t.Errorf("hexDump must truncate at 32K: %d lines", lines)
	}
}

func TestStripHTML(t *testing.T) {
	cases := map[string]string{
		"<html><body>Hello <b>world</b></body></html>": "Hello  world",
		"plain":       "plain",
		"":            "",
		"<p>a</p><p>b</p>": "a  b",
	}
	for in, want := range cases {
		if got := stripHTML(in); got != want {
			t.Errorf("stripHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsNoise(t *testing.T) {
	noisy := []string{"/a/app.css", "/x.js", "/img.png", "/f.woff2", "/i.ico"}
	for _, p := range noisy {
		if !isNoise(&Flow{Path: p}) {
			t.Errorf("%q should be noise", p)
		}
	}
	quiet := []string{"/api/users", "/", "", "/graphql"}
	for _, p := range quiet {
		if isNoise(&Flow{Path: p}) {
			t.Errorf("%q should not be noise", p)
		}
	}
}

func TestSortFlows(t *testing.T) {
	base := time.Unix(1700000000, 0)
	mk := func(id uint64, method, host, path, src string, code int, size int64, dur time.Duration) *Flow {
		return &Flow{ID: id, Method: method, Host: host, Path: path, Src: src,
			StatusCode: code, RespSize: size, Started: base, Ended: base.Add(dur)}
	}
	flows := []*Flow{
		mk(3, "POST", "c.com", "/z", SrcReverse, 500, 30, 3*time.Second),
		mk(1, "GET", "a.com", "/x", SrcForward, 200, 10, time.Second),
		mk(2, "DELETE", "b.com", "/y", SrcForward, 404, 20, 2*time.Second),
	}
	cases := []struct {
		col   string
		asc   bool
		first uint64
	}{
		{"#", true, 1},
		{"#", false, 3},
		{"Method", true, 2},
		{"Host / Path", true, 1},
		{"Src", true, 1},
		{"Status", true, 1},
		{"Status", false, 3},
		{"Size", true, 1},
		{"Time", true, 1},
		{"Time", false, 3},
		{"unknown", true, 1},
	}
	for _, c := range cases {
		cp := append([]*Flow(nil), flows...)
		sortFlows(cp, c.col, c.asc)
		if cp[0].ID != c.first {
			t.Errorf("sort by %q asc=%v put flow %d first, want %d", c.col, c.asc, cp[0].ID, c.first)
		}
	}
}

func TestFilteredFlows_TokenFilters(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)

	cases := []struct {
		filter string
		want   int
	}{
		{"", 6},
		{"src:rev", 1},
		{"src:fwd", 5},
		{"status:500", 1},
		{"status:304", 1},
		{"status:101", 1},
		{"mime:.css", 1},
		{"checkout", 1},
		{"example.com", 6},
		{"shop checkout", 1},
		{"shop nomatch", 0},
		{"interesting", 1},
		{"src:rev checkout", 1},
		{"src:rev users", 0},
	}
	for _, c := range cases {
		rig.s.Filter.SetText(c.filter)
		if got := len(rig.s.filteredFlows()); got != c.want {
			t.Errorf("filter %q matched %d flows, want %d", c.filter, got, c.want)
		}
	}
}

func TestFilteredFlows_HideNoise(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)
	rig.s.HideNoiseSw.Value = true
	for _, f := range rig.s.filteredFlows() {
		if isNoise(f) {
			t.Errorf("noisy flow %q survived the filter", f.Path)
		}
	}
}

func TestUIEvents_ContextMenuActions(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	first := rig.s.Store.Snapshot()[0]

	steps := []struct {
		name  string
		click func()
		check func(*testing.T)
	}{
		{"copy-url", func() { rig.s.CtxCopyURL.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "URL copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"copy-curl", func() { rig.s.CtxCopyCurl.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "curl copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"copy-req", func() { rig.s.CtxCopyReq.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "Request copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"repeat", func() { rig.s.CtxRepeat.Click() }, func(t *testing.T) {
			if !strings.Contains(rig.s.StatusBanner, "Repeat") {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"to-repeater", func() { rig.s.CtxToRepeater.Click() }, func(t *testing.T) {
			if !strings.Contains(rig.s.StatusBanner, "Repeater") {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"add-scope", func() { rig.s.CtxAddScope.Click() }, func(t *testing.T) {
			if rig.s.Proxy.ScopeR.Len() != 1 {
				t.Errorf("scope len = %d, want 1", rig.s.Proxy.ScopeR.Len())
			}
		}},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			rig.s.CtxOpen = true
			rig.s.CtxFlowID = first.ID
			st.click()
			rig.frames(2)
			if rig.s.CtxOpen {
				t.Error("the menu must close after an action")
			}
			st.check(t)
		})
	}
}

func TestUIEvents_ContextDeleteAndAnnotate(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	first := rig.s.Store.Snapshot()[0]

	rig.s.CtxOpen = true
	rig.s.CtxFlowID = first.ID
	rig.s.CtxAnnotate.Click()
	rig.frames(2)
	if !rig.s.AnnotateOpen || rig.s.AnnotateFlowID != first.ID {
		t.Fatalf("annotate popup did not open for flow %d", first.ID)
	}
	if rig.s.AnnotateComment.Text() != "interesting" {
		t.Errorf("the existing comment must be loaded, got %q", rig.s.AnnotateComment.Text())
	}

	rig.s.AnnotateComment.SetText("edited note")
	rig.s.AnnotateColors[3].Click()
	rig.frames(2)
	if got := rig.s.Store.FindByID(first.ID); got == nil || got.Highlight != annotateColorKeys()[3] {
		t.Errorf("colour swatch did not apply: %+v", got)
	}

	rig.s.AnnotateSave.Click()
	rig.frames(2)
	if rig.s.AnnotateOpen {
		t.Error("Save must close the popup")
	}
	if got := rig.s.Store.FindByID(first.ID); got == nil || got.Comment != "edited note" {
		t.Errorf("comment not saved: %+v", got)
	}

	rig.s.Selected = first.ID
	rig.s.CtxOpen = true
	rig.s.CtxFlowID = first.ID
	rig.s.CtxDelete.Click()
	rig.frames(2)
	if rig.s.Store.FindByID(first.ID) != nil {
		t.Error("Delete did not remove the flow")
	}
	if rig.s.Selected != 0 {
		t.Errorf("deleting the selected flow must clear the selection, got %d", rig.s.Selected)
	}
}

func TestUIEvents_ContextActionsWithMissingFlow(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.frames(2)
	rig.s.CtxOpen = true
	rig.s.CtxFlowID = 424242
	rig.s.StatusBanner = ""
	rig.s.CtxCopyURL.Click()
	rig.frames(2)
	if rig.s.StatusBanner != "" {
		t.Errorf("a missing flow must not set a banner, got %q", rig.s.StatusBanner)
	}
	if rig.s.CtxOpen {
		t.Error("the menu must still close")
	}
}

func TestUIEvents_InspectorTabsAndModeButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.frames(2)

	rig.s.TabResp.Click()
	rig.frames(2)
	if rig.s.ActTab != 1 {
		t.Errorf("ActTab = %d, want 1", rig.s.ActTab)
	}
	rig.s.TabReq.Click()
	rig.frames(2)
	if rig.s.ActTab != 0 {
		t.Errorf("ActTab = %d, want 0", rig.s.ActTab)
	}

	modes := []struct {
		click func()
		want  int
	}{
		{func() { rig.s.ViewPretty.Click() }, 1},
		{func() { rig.s.ViewHex.Click() }, 2},
		{func() { rig.s.ViewRender.Click() }, 3},
		{func() { rig.s.ViewRaw.Click() }, 0},
	}
	for _, m := range modes {
		m.click()
		rig.frames(2)
		if rig.s.RenderMode != m.want {
			t.Errorf("RenderMode = %d, want %d", rig.s.RenderMode, m.want)
		}
	}

	secs := []struct {
		click func()
		want  int
	}{
		{func() { rig.s.SecBody.Click() }, 1},
		{func() { rig.s.SecParams.Click() }, 2},
		{func() { rig.s.SecCookies.Click() }, 3},
		{func() { rig.s.SecHeaders.Click() }, 0},
	}
	for _, m := range secs {
		m.click()
		rig.frames(2)
		if rig.s.SecTab != m.want {
			t.Errorf("SecTab = %d, want %d", rig.s.SecTab, m.want)
		}
	}
}

func TestUIEvents_SendBusAndCopy(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.frames(2)

	cases := []struct {
		click func()
		want  string
	}{
		{func() { rig.s.InspSendRepeater.Click() }, "Repeater"},
		{func() { rig.s.InspSendIntruder.Click() }, "Intruder"},
		{func() { rig.s.InspSendComparer.Click() }, "Comparer"},
		{func() { rig.s.InspSendDecoder.Click() }, "Decoder"},
		{func() { rig.s.InspCopy.Click() }, "Copied"},
	}
	for _, c := range cases {
		rig.s.StatusBanner = ""
		c.click()
		rig.frames(2)
		if !strings.Contains(rig.s.StatusBanner, c.want) {
			t.Errorf("banner = %q, want a mention of %q", rig.s.StatusBanner, c.want)
		}
	}
}

func TestUIEvents_TargetRowControls(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamAuto, TLS: TLSDecrypt})
	rig.sidebarFrames(2)

	row := rig.s.TargetRows["shop.example.com"]
	if row == nil {
		t.Fatal("target row was not created by the sidebar layout")
	}

	row.UpstreamManual.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Upstream; got != UpstreamManual {
		t.Errorf("Upstream = %q, want %q", got, UpstreamManual)
	}
	row.UpstreamAuto.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Upstream; got != UpstreamAuto {
		t.Errorf("Upstream = %q, want %q", got, UpstreamAuto)
	}

	row.TLSTunnel.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].TLS; got != TLSTunnel {
		t.Errorf("TLS = %q, want %q", got, TLSTunnel)
	}
	row.TLSDecrypt.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].TLS; got != TLSDecrypt {
		t.Errorf("TLS = %q, want %q", got, TLSDecrypt)
	}

	row.Copy.Click()
	rig.sidebarFrames(2)
	if rig.s.StatusBanner != "Copied hosts line" {
		t.Errorf("banner = %q", rig.s.StatusBanner)
	}

	row.Expand.Click()
	rig.sidebarFrames(2)
	if !row.Expanded {
		t.Error("Expand must open the row")
	}

	row.Remove.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 0 {
		t.Errorf("Remove failed, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetRows["shop.example.com"] != nil {
		t.Error("Remove must drop the row state")
	}
}

func TestUIEvents_TargetDelayInput(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamAuto, TLS: TLSDecrypt})
	rig.sidebarFrames(2)

	row := rig.s.TargetRows["shop.example.com"]
	row.Expanded = true
	row.DelayInput.SetText("250")
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Delay; got != 250*time.Millisecond {
		t.Errorf("Delay = %v, want 250ms", got)
	}

	row.DelayInput.SetText("not-a-number")
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Delay; got != 250*time.Millisecond {
		t.Errorf("an unparseable delay must be ignored, got %v", got)
	}
}

func TestUIEvents_SidebarRuleRowControls(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	rig.s.SecIRulesOpen = true
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a"})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "b"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "x"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "y"})
	rig.sidebarFrames(2)

	if len(rig.s.MRRows) < 2 {
		t.Fatalf("MR rows not built: %d", len(rig.s.MRRows))
	}
	rig.s.MRRows[0].Down.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.MR.Snapshot(); got[0].Pattern != "b" {
		t.Errorf("Down did not reorder: %q first", got[0].Pattern)
	}
	rig.s.MRRows[0].Up.Click()
	rig.sidebarFrames(2)

	rig.s.MRRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) != 1 {
		t.Errorf("MR Remove failed, len=%d", len(rig.s.Proxy.MR.Snapshot()))
	}

	if len(rig.s.ScopeRows) < 1 {
		t.Fatalf("scope rows not built: %d", len(rig.s.ScopeRows))
	}
	rig.s.ScopeRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.ScopeR.Len() != 0 {
		t.Errorf("scope Remove failed, len=%d", rig.s.Proxy.ScopeR.Len())
	}

	if len(rig.s.IRuleRows) < 1 {
		t.Fatalf("intercept rows not built: %d", len(rig.s.IRuleRows))
	}
	rig.s.IRuleRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(HeldRequest); len(conds) != 0 {
		t.Errorf("intercept Remove failed, len=%d", len(conds))
	}
}

func TestUIEvents_IRuleOrToggle(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	before := rig.s.IRuleOr
	rig.s.IRuleOrBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.IRuleOr == before {
		t.Error("the AND/OR button must toggle")
	}
	rig.s.IRuleValInput.SetText("x")
	rig.s.IRuleAddBtn.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(HeldRequest); len(conds) != 1 || conds[0].Or != rig.s.IRuleOr {
		t.Errorf("the added condition must carry the OR flag: %+v", conds)
	}
}

func TestConfig_LoadMissingIsZero(t *testing.T) {
	setupTestConfigDir(t)
	c := LoadConfig()
	if c.BindAddr != "" || len(c.Targets) != 0 {
		t.Errorf("a missing config file must load as zero, got %+v", c)
	}
}

func TestConfig_ApplyToIgnoresInvalidTargets(t *testing.T) {
	p := NewProxy(NewStore())
	p.Rules = NewRules()
	c := Config{Targets: []TargetConfig{{Domain: "good.example.com"}, {Domain: "nodot"}}}
	c.ApplyTo(p)
	if p.Targets.Len() != 1 {
		t.Errorf("invalid targets must be skipped, len=%d", p.Targets.Len())
	}
}

func TestUIState_EnsureDefaults(t *testing.T) {
	setupTestConfigDir(t)
	var s UIState
	s.Ensure()
	if s.Store == nil || s.Proxy == nil {
		t.Fatal("Ensure must allocate the store and proxy")
	}
	if s.SplitRatio != 0.62 {
		t.Errorf("SplitRatio = %v, want 0.62", s.SplitRatio)
	}
	if s.View != ViewHistory || s.IRulesActive != HeldRequest {
		t.Errorf("view/irules = %q/%q", s.View, s.IRulesActive)
	}
	if s.SortColumn != "#" || !s.SortAsc {
		t.Errorf("sort = %q asc=%v", s.SortColumn, s.SortAsc)
	}
	if s.BindAddr.Text() != DefaultAddr {
		t.Errorf("BindAddr = %q, want %q", s.BindAddr.Text(), DefaultAddr)
	}
	if s.TargetRows == nil {
		t.Error("TargetRows must be allocated")
	}
	if !s.SecTargetsOpen || !s.SecTLSOpen {
		t.Error("the primary sections must default to open")
	}

	store, proxy := s.Store, s.Proxy
	s.SplitRatio = 0.9
	s.Ensure()
	if s.Store != store || s.Proxy != proxy || s.SplitRatio != 0.9 {
		t.Error("a second Ensure must not reset existing state")
	}
}

func TestUIState_ApplyLoadedConfig(t *testing.T) {
	setupTestConfigDir(t)
	var s UIState
	s.Ensure()
	s.Config = Config{
		BindAddr: "127.0.0.1:7777", View: ViewWebSockets,
		SortColumn: "Size", SortAsc: false,
		InspectorCollapsed: true, Decrypt: true, InterceptResponses: true,
	}
	s.applyLoadedConfig()

	if s.BindAddr.Text() != "127.0.0.1:7777" || s.View != ViewWebSockets {
		t.Errorf("bind/view = %q/%q", s.BindAddr.Text(), s.View)
	}
	if s.SortColumn != "Size" || s.SortAsc {
		t.Errorf("sort = %q asc=%v", s.SortColumn, s.SortAsc)
	}
	if !s.InspectorCollapsed || !s.DecryptSwitch.Value || !s.InterceptRespSw.Value {
		t.Error("toggles not applied")
	}
	if !s.Proxy.Manual.InterceptResponses() {
		t.Error("the response-intercept flag must reach the interceptor")
	}
}
