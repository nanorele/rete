package workspace

import (
	"net/http"
	"testing"
)

func TestWSHandshakeHeaders(t *testing.T) {
	rt := &RequestTab{}

	origin := &HeaderItem{}
	origin.Key.SetText("Origin")
	origin.Value.SetText("https://web.max.ru")

	templated := &HeaderItem{}
	templated.Key.SetText("X-Token")
	templated.Value.SetText("{{tok}}")

	gen := &HeaderItem{IsGenerated: true}
	gen.Key.SetText("Content-Length")
	gen.Value.SetText("0")

	empty := &HeaderItem{}
	empty.Key.SetText("   ")
	empty.Value.SetText("ignored")

	rt.Headers = []*HeaderItem{origin, templated, gen, empty}

	h := rt.wsHandshakeHeaders(map[string]string{"tok": "abc"}, http.Header{"User-Agent": {"tracto/1"}})

	if got := h.Get("Origin"); got != "https://web.max.ru" {
		t.Fatalf("Origin = %q, want https://web.max.ru", got)
	}
	if got := h.Get("X-Token"); got != "abc" {
		t.Fatalf("X-Token = %q, want abc (templated)", got)
	}
	if got := h.Get("User-Agent"); got != "tracto/1" {
		t.Fatalf("User-Agent = %q, want tracto/1 (merged extra)", got)
	}
	if _, ok := h["Content-Length"]; ok {
		t.Fatal("generated header should be skipped")
	}
	if len(h) != 3 {
		t.Fatalf("header count = %d, want 3 (empty-key row dropped)", len(h))
	}
}

func TestWSHandshakeHeadersEmpty(t *testing.T) {
	rt := &RequestTab{}
	if h := rt.wsHandshakeHeaders(nil, nil); h != nil {
		t.Fatalf("expected nil for no headers, got %v", h)
	}
}

func TestDefaultOrigin(t *testing.T) {
	cases := map[string]string{
		"wss://api.oneme.ru/websocket":   "https://api.oneme.ru",
		"ws://localhost:8080/ws":         "http://localhost:8080",
		"wss://api.oneme.ru:443/ws":      "https://api.oneme.ru",
		"ws://example.com:80/ws":         "http://example.com",
		"wss://example.com:8443/ws":      "https://example.com:8443",
		"https://example.com/x":          "https://example.com",
		"not a url with spaces":          "",
		"":                               "",
		"/relative/path":                 "",
	}
	for in, want := range cases {
		if got := defaultOrigin(in); got != want {
			t.Errorf("defaultOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}
