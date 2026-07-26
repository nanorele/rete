package mitm

import (
	"strings"
	"testing"
)

func TestIsNoiseMatchesExtensionSuffixOnly(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/users.json", false},
		{"/v1/data.json", false},
		{"/api/list.jsonp", false},
		{"/graphql.json?q=1", false},
		{"/assets/app.js", true},
		{"/assets/app.js?v=3", true},
		{"/assets/app.js#frag", true},
		{"/style.css", true},
		{"/logo.png", true},
		{"/fonts/x.woff2", true},
		{"/api/jsonify", false},
		{"/csset", false},
		{"/a/.js/b", false},
		{"/", false},
		{"", false},
	}
	for _, c := range cases {
		f := &Flow{Path: c.path}
		if got := isNoise(f); got != c.want {
			t.Errorf("isNoise(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestAsCurlQuotesSingleQuotes(t *testing.T) {
	f := &Flow{
		Method:     "POST",
		URL:        "https://example.com/a?q=it's",
		ReqHeaders: [][2]string{{"X-Note", "it's here"}},
		ReqBody:    []byte(`{"msg":"it's"}`),
	}
	out := asCurl(f)
	if strings.Contains(out, "'it's") {
		t.Errorf("unescaped single quote leaked into curl output:\n%s", out)
	}
	for _, want := range []string{`'\''`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected escaped quote %q in output:\n%s", want, out)
		}
	}
}

func TestAsCurlPlainValues(t *testing.T) {
	f := &Flow{
		Method:     "GET",
		URL:        "https://example.com/a",
		ReqHeaders: [][2]string{{"Accept", "application/json"}, {"Host", "example.com"}},
	}
	out := asCurl(f)
	if !strings.Contains(out, "curl -X GET 'https://example.com/a'") {
		t.Errorf("unexpected curl line:\n%s", out)
	}
	if strings.Contains(out, "Host:") {
		t.Errorf("host header must be omitted:\n%s", out)
	}
	if !strings.Contains(out, "-H 'Accept: application/json'") {
		t.Errorf("header missing:\n%s", out)
	}
}

func TestMatchReplaceInvalidRegexDoesNotFallBackToLiteral(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{
		Enabled:     true,
		IsRegex:     true,
		Pattern:     "([unclosed",
		Replacement: "x",
	})
	rules := mr.rules
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	in := "([unclosed here"
	if got := rules[0].applyString(in); got != in {
		t.Errorf("applyString with an invalid regex = %q, want the input unchanged", got)
	}
}

func TestMatchReplaceValidRegexApplies(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	if got := mr.rules[0].applyString("a1b22c"); got != "aNbNc" {
		t.Errorf("applyString = %q, want aNbNc", got)
	}
}

func TestMatchReplaceLiteralStillWorks(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: false, Pattern: "foo", Replacement: "bar"})
	if got := mr.rules[0].applyString("a foo b"); got != "a bar b" {
		t.Errorf("applyString = %q, want 'a bar b'", got)
	}
}

func TestMatchReplaceUpdateRecompiles(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	mr.Update(0, func(r *MatchReplaceRule) { r.Pattern = `[a-z]+` })
	if got := mr.rules[0].applyString("abc123"); got != "N123" {
		t.Errorf("applyString after Update = %q, want N123", got)
	}
}

func TestMatchReplaceRuleCompiledEagerly(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	if mr.rules[0].re == nil {
		t.Error("Add must compile the regex eagerly so read paths never write under RLock")
	}
}

func TestScopeRuleCompiledEagerly(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^ex.*\.com$`, IsRegex: true})
	if sc.rules[0].re == nil {
		t.Error("Add must compile the scope regex eagerly")
	}
	if !sc.InScope(&Flow{Host: "example.com"}) {
		t.Error("host should be in scope")
	}
	if sc.InScope(&Flow{Host: "other.org"}) {
		t.Error("host should not be in scope")
	}
}

func TestScopeInvalidRegexNeverMatches(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "([bad", IsRegex: true})
	if sc.InScope(&Flow{Host: "([bad"}) {
		t.Error("an invalid scope regex must not fall back to a literal match")
	}
}

func TestSortClicksCoversEveryHistColumn(t *testing.T) {
	var s UIState
	if len(s.SortClicks) != len(histCols) {
		t.Errorf("len(SortClicks) = %d, len(histCols) = %d; every column needs a clickable",
			len(s.SortClicks), len(histCols))
	}
}

func TestApplyHeadersLeavesCallerSliceIntact(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "Cookie", Replacement: ""})

	in := [][2]string{{"Host", "example.com"}, {"Cookie", "secret=1"}, {"Accept", "*/*"}}
	before := append([][2]string(nil), in...)

	out := mr.ApplyHeaders(MRRequest, in)
	for i := range before {
		if in[i] != before[i] {
			t.Fatalf("caller's slice was rewritten in place: %v, want %v", in, before)
		}
	}
	if len(out) != 2 || out[0][0] != "Host" || out[1][0] != "Accept" {
		t.Errorf("out = %v, want Cookie removed", out)
	}
}

func TestApplyHeadersEmptyPatternIsSkipped(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "", Replacement: "x"})

	out := mr.ApplyHeaders(MRRequest, [][2]string{{"Host", "h"}})
	if len(out) != 1 || out[0][0] != "Host" {
		t.Fatalf("out = %v, want the input unchanged", out)
	}
	for _, h := range out {
		if h[0] == "" {
			t.Errorf("a rule naming no header injected a nameless header: %v", out)
		}
	}
}

func TestApplyHeadersCollapsesDuplicatesOnReplace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Set-Cookie", Replacement: "a=1"})

	out := mr.ApplyHeaders(MRResponse, [][2]string{
		{"Set-Cookie", "a=old"},
		{"Content-Type", "text/html"},
		{"set-cookie", "b=old"},
	})
	seen := 0
	for _, h := range out {
		if strings.EqualFold(h[0], "Set-Cookie") {
			seen++
			if h[1] != "a=1" {
				t.Errorf("Set-Cookie = %q, want a=1", h[1])
			}
		}
	}
	if seen != 1 {
		t.Errorf("out = %v, want exactly one Set-Cookie header, got %d", out, seen)
	}
	if len(out) != 2 {
		t.Errorf("out = %v, want the untouched Content-Type kept alongside", out)
	}
}

func TestApplyHeadersDeletesEveryDuplicate(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Set-Cookie", Replacement: ""})

	out := mr.ApplyHeaders(MRResponse, [][2]string{
		{"Set-Cookie", "a=1"},
		{"set-cookie", "b=2"},
		{"Content-Type", "text/html"},
	})
	if len(out) != 1 || out[0][0] != "Content-Type" {
		t.Errorf("out = %v, want every Set-Cookie removed", out)
	}
}

func TestApplyHeadersAddsWhenAbsent(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "X-Trace", Replacement: "1"})

	out := mr.ApplyHeaders(MRRequest, [][2]string{{"Host", "h"}})
	if len(out) != 2 || out[1][0] != "X-Trace" || out[1][1] != "1" {
		t.Errorf("out = %v, want X-Trace appended", out)
	}
}
