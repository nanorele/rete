package workspace

import (
	"encoding/json"
	"io"
	"testing"

	"tracto/internal/model"
)

func TestAtoiDefault(t *testing.T) {
	cases := []struct {
		in       string
		def, min int
		want     int
	}{
		{"5", 1, 0, 5},
		{"  7  ", 1, 0, 7},
		{"", 3, 0, 3},
		{"abc", 3, 0, 3},
		{"0", 3, 1, 3},
		{"1", 3, 1, 1},
		{"-4", 9, 0, 9},
		{"-4", 9, -10, -4},
		{"2.5", 8, 0, 8},
		{"999999999999999999999", 8, 0, 8},
	}
	for _, c := range cases {
		if got := atoiDefault(c.in, c.def, c.min); got != c.want {
			t.Errorf("atoiDefault(%q, %d, %d) = %d, want %d", c.in, c.def, c.min, got, c.want)
		}
	}
}

func TestHTTPMethod(t *testing.T) {
	cases := []struct{ method, want string }{
		{MethodGraphQL, "POST"},
		{"GET", "GET"},
		{"DELETE", "DELETE"},
		{"", ""},
	}
	for _, c := range cases {
		tab := NewRequestTab("t")
		tab.Method = c.method
		if got := tab.httpMethod(); got != c.want {
			t.Errorf("httpMethod() with Method=%q = %q, want %q", c.method, got, c.want)
		}
	}
}

func TestAuthTypeModelRoundTrip(t *testing.T) {
	for _, at := range []int{authNone, authBearer, authBasic} {
		s := authTypeToModel(at)
		if got := authTypeFromModel(s); got != at {
			t.Errorf("round trip auth %d -> %q -> %d", at, s, got)
		}
	}
	if got := authTypeFromModel("nonsense"); got != authNone {
		t.Errorf("authTypeFromModel(nonsense) = %d, want authNone", got)
	}
	if got := authTypeFromModel(""); got != authNone {
		t.Errorf("authTypeFromModel(empty) = %d, want authNone", got)
	}
	if got := authTypeToModel(99); got != "" {
		t.Errorf("authTypeToModel(99) = %q, want empty", got)
	}
}

func TestApplyAuthRoundTrip(t *testing.T) {
	cases := []model.ParsedAuth{
		{Type: "bearer", Token: "tok"},
		{Type: "basic", Username: "u", Password: "p"},
		{Type: "", Token: "", Username: "", Password: ""},
		{Type: "bearer", Token: "токен ünïcode"},
	}
	for _, want := range cases {
		tab := NewRequestTab("t")
		tab.ApplyAuth(want)
		got := tab.AuthModel()
		if got.Type != want.Type || got.Token != want.Token ||
			got.Username != want.Username || got.Password != want.Password {
			t.Errorf("ApplyAuth/AuthModel round trip: got %+v, want %+v", got, want)
		}
	}
}

func TestApplyAuthUnknownTypeNormalizes(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyAuth(model.ParsedAuth{Type: "oauth2", Token: "tok"})
	if tab.AuthType != authNone {
		t.Errorf("AuthType = %d, want authNone for unknown type", tab.AuthType)
	}
	if got := tab.AuthModel().Type; got != "" {
		t.Errorf("AuthModel().Type = %q, want empty", got)
	}
}

func TestApplyCookiesRoundTrip(t *testing.T) {
	in := []model.ParsedKV{
		{Key: "session", Value: "abc"},
		{Key: "theme", Value: "dark"},
	}
	tab := NewRequestTab("t")
	tab.ApplyCookies(in)
	got := tab.CookieModels()
	if len(got) != len(in) {
		t.Fatalf("CookieModels() len = %d, want %d", len(got), len(in))
	}
	for i := range in {
		if got[i] != in[i] {
			t.Errorf("cookie[%d] = %+v, want %+v", i, got[i], in[i])
		}
	}
}

func TestApplyCookiesReplacesPrevious(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyCookies([]model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}})
	tab.ApplyCookies([]model.ParsedKV{{Key: "c", Value: "3"}})
	got := tab.CookieModels()
	if len(got) != 1 || got[0].Key != "c" {
		t.Errorf("CookieModels() = %+v, want only cookie c", got)
	}
}

func TestCookieModelsSkipsEmptyKeys(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ApplyCookies([]model.ParsedKV{{Key: "", Value: "orphan"}, {Key: "k", Value: "v"}})
	got := tab.CookieModels()
	if len(got) != 1 || got[0].Key != "k" {
		t.Errorf("CookieModels() = %+v, want only cookie k", got)
	}
}

func TestEnsureGQLIsIdempotent(t *testing.T) {
	tab := NewRequestTab("t")
	if tab.GQL != nil {
		t.Fatal("new tab already has a GQL session")
	}
	g := tab.EnsureGQL()
	if g == nil {
		t.Fatal("EnsureGQL returned nil")
	}
	if g.VarsSplitRatio != 0.6 {
		t.Errorf("VarsSplitRatio = %v, want 0.6", g.VarsSplitRatio)
	}
	if again := tab.EnsureGQL(); again != g {
		t.Error("EnsureGQL created a second session")
	}
}

func TestGraphQLPayload(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		vars      string
		env       map[string]string
		wantQuery string
		wantVars  string
		wantErr   bool
	}{
		{
			name:      "query only",
			query:     "{ me { id } }",
			wantQuery: "{ me { id } }",
		},
		{
			name:      "query with variables",
			query:     "query($id:ID!){ user(id:$id){ name } }",
			vars:      `{"id":"7"}`,
			wantQuery: "query($id:ID!){ user(id:$id){ name } }",
			wantVars:  `{"id":"7"}`,
		},
		{
			name:      "whitespace-only variables omitted",
			query:     "{ me }",
			vars:      "   \n\t ",
			wantQuery: "{ me }",
		},
		{
			name:    "invalid json variables",
			query:   "{ me }",
			vars:    `{"id":}`,
			wantErr: true,
		},
		{
			name:      "env substitution in query and vars",
			query:     "{ user(id:{{uid}}) }",
			vars:      `{"tenant":"{{tenant}}"}`,
			env:       map[string]string{"uid": "42", "tenant": "acme"},
			wantQuery: "{ user(id:42) }",
			wantVars:  `{"tenant":"acme"}`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab := NewRequestTab("t")
			g := tab.EnsureGQL()
			g.Query.SetText(c.query)
			g.Variables.SetText(c.vars)

			data, err := tab.graphQLPayload(c.env)
			if c.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("graphQLPayload error: %v", err)
			}
			var p struct {
				Query     string          `json:"query"`
				Variables json.RawMessage `json:"variables"`
			}
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatalf("payload is not valid JSON: %v (%s)", err, data)
			}
			if p.Query != c.wantQuery {
				t.Errorf("query = %q, want %q", p.Query, c.wantQuery)
			}
			if c.wantVars == "" {
				if len(p.Variables) != 0 {
					t.Errorf("variables = %s, want omitted", p.Variables)
				}
			} else if string(p.Variables) != c.wantVars {
				t.Errorf("variables = %s, want %s", p.Variables, c.wantVars)
			}
		})
	}
}

func TestBuildGraphQLBody(t *testing.T) {
	tab := NewRequestTab("t")
	g := tab.EnsureGQL()
	g.Query.SetText("{ me }")

	r, ct, err := tab.buildGraphQLBody(nil)
	if err != nil {
		t.Fatalf("buildGraphQLBody error: %v", err)
	}
	if ct != "application/json" {
		t.Errorf("content type = %q, want application/json", ct)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !json.Valid(b) {
		t.Errorf("body is not valid JSON: %s", b)
	}

	g.Variables.SetText(`{"bad":}`)
	if _, _, err := tab.buildGraphQLBody(nil); err == nil {
		t.Error("buildGraphQLBody accepted invalid variables JSON")
	}
}
