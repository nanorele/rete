package flow

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"rete/internal/model"
	"rete/internal/ui/settings"
)

type parityHit struct {
	method string
	header http.Header
	body   string
	close  bool
}

func parityServer(t *testing.T, respond func(w http.ResponseWriter)) (*httptest.Server, *atomic.Value) {
	t.Helper()
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		last.Store(parityHit{method: r.Method, header: r.Header.Clone(), body: string(data), close: r.Close})
		if respond != nil {
			respond(w)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &last
}

func withSettings(t *testing.T, apply func()) {
	t.Helper()
	ua, dh, ae, cc := settings.UserAgent, settings.DefaultHeaders, settings.AcceptEncoding, settings.SendConnClose
	af, sc, tw, ji := settings.AutoFormatJSONRequest, settings.StripJSONComments, settings.TrimTrailingWS, settings.JSONIndent
	t.Cleanup(func() {
		settings.UserAgent, settings.DefaultHeaders, settings.AcceptEncoding, settings.SendConnClose = ua, dh, ae, cc
		settings.AutoFormatJSONRequest, settings.StripJSONComments, settings.TrimTrailingWS, settings.JSONIndent = af, sc, tw, ji
	})
	apply()
}

func TestRunHTTPSystemHeadersMatchTab(t *testing.T) {
	srv, last := parityServer(t, nil)

	t.Run("user agent, accept-encoding and default headers", func(t *testing.T) {
		withSettings(t, func() {
			settings.UserAgent = "rete-test/1.0"
			settings.AcceptEncoding = "gzip, br"
			settings.DefaultHeaders = []model.DefaultHeader{{Key: "X-Team", Value: "{{team}}"}, {Key: "X-Set", Value: "default"}}
			settings.SendConnClose = true
		})
		n := &execNode{
			method:  "GET",
			url:     srv.URL,
			env:     map[string]string{"team": "core"},
			headers: [][2]string{{"X-Set", "explicit"}},
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if got := h.header.Get("User-Agent"); got != "rete-test/1.0" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := h.header.Get("Accept-Encoding"); got != "gzip, br" {
			t.Errorf("Accept-Encoding = %q", got)
		}
		if got := h.header.Get("X-Team"); got != "core" {
			t.Errorf("default header must expand variables, got %q", got)
		}
		if got := h.header.Get("X-Set"); got != "explicit" {
			t.Errorf("explicit header must beat the default header, got %q", got)
		}
		if got := h.header.Get("Connection"); got != "close" || !h.close {
			t.Errorf("Connection = %q close=%v, want close", got, h.close)
		}
		if got := h.header.Get("Content-Type"); got != "text/plain" {
			t.Errorf("raw body without content sends text/plain like the tab, got %q", got)
		}
	})

	t.Run("explicit user agent wins", func(t *testing.T) {
		withSettings(t, func() { settings.UserAgent = "rete-test/1.0" })
		n := &execNode{method: "GET", url: srv.URL, headers: [][2]string{{"User-Agent", "mine"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("User-Agent"); got != "mine" {
			t.Errorf("User-Agent = %q, want the explicit one", got)
		}
	})

	t.Run("empty user agent falls back to the app default", func(t *testing.T) {
		withSettings(t, func() { settings.UserAgent = "" })
		n := &execNode{method: "GET", url: srv.URL}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("User-Agent"); got != model.DefaultSettings().UserAgent {
			t.Errorf("User-Agent = %q, want the default", got)
		}
	})
}

func TestRunHTTPRawBodyMatchesTab(t *testing.T) {
	srv, last := parityServer(t, nil)

	t.Run("json body gets application/json", func(t *testing.T) {
		withSettings(t, func() {
			settings.StripJSONComments = true
			settings.TrimTrailingWS = true
			settings.AutoFormatJSONRequest = false
		})
		n := &execNode{method: "POST", url: srv.URL, body: "  {\"a\": 1, // note\n \"b\": \"{{v}}\"}  \t\n", env: map[string]string{"v": "x"}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if got := h.header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if strings.Contains(h.body, "//") || strings.HasSuffix(h.body, " ") || strings.HasSuffix(h.body, "\t") {
			t.Errorf("comments and trailing whitespace must be stripped, got %q", h.body)
		}
		if !strings.Contains(h.body, `"b": "x"`) {
			t.Errorf("variables must expand, got %q", h.body)
		}
	})

	t.Run("auto format json request", func(t *testing.T) {
		withSettings(t, func() {
			settings.AutoFormatJSONRequest = true
			settings.JSONIndent = 4
		})
		n := &execNode{method: "PUT", url: srv.URL, body: `{"a":{"b":1}}`}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		h := last.Load().(parityHit)
		if h.method != "PUT" || h.body != "{\n    \"a\": {\n        \"b\": 1\n    }\n}" {
			t.Errorf("method=%q body=%q", h.method, h.body)
		}
	})

	t.Run("explicit content type is kept for raw bodies", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, body: "<a/>", headers: [][2]string{{"Content-Type", "application/xml"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("Content-Type"); got != "application/xml" {
			t.Errorf("Content-Type = %q", got)
		}
	})

	t.Run("form body content type overrides a manual header", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, bodyType: "urlencoded", body: "a=1", headers: [][2]string{{"Content-Type", "text/plain"}}}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
	})

	t.Run("every method is sent as typed", func(t *testing.T) {
		for _, m := range methods {
			n := &execNode{method: m, url: srv.URL, body: "{}"}
			if res := runHTTP(context.Background(), n, nil); res.failed {
				t.Fatalf("%s failed: %+v", m, res)
			}
			if got := last.Load().(parityHit).method; got != m {
				t.Errorf("method %s arrived as %q", m, got)
			}
		}
		n := &execNode{url: srv.URL}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if got := last.Load().(parityHit).method; got != "GET" {
			t.Errorf("empty method must default to GET, got %q", got)
		}
	})

	t.Run("url is sanitized like the tab", func(t *testing.T) {
		n := &execNode{method: "GET", url: "\t" + srv.URL + "/a b\n"}
		if res := runHTTP(context.Background(), n, nil); res.failed {
			t.Fatalf("failed: %+v", res)
		}
	})
}

func TestRunHTTPDecompressesResponses(t *testing.T) {
	srv, _ := parityServer(t, func(w http.ResponseWriter) {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte(`{"items":[1,2,3]}`))
		_ = gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf.Bytes())
	})
	withSettings(t, func() { settings.AcceptEncoding = "gzip" })
	n := &execNode{method: "GET", url: srv.URL}
	res := runHTTP(context.Background(), n, nil)
	if res.failed {
		t.Fatalf("failed: %+v", res)
	}
	if string(res.body) != `{"items":[1,2,3]}` {
		t.Fatalf("body must be decompressed, got %q", res.body)
	}
	if !evalCond(execEdge{cond: CondArrayCount, value: "items", op: "==", count: 3}, &res, nil, nil) {
		t.Error("conditions must see the decoded body")
	}
}

func TestRunGQLSystemHeaders(t *testing.T) {
	srv, last := parityServer(t, nil)
	withSettings(t, func() { settings.UserAgent = "rete-test/1.0" })
	n := &execNode{url: srv.URL, body: "query { me { id } }"}
	if res := runGQL(context.Background(), n, nil); res.failed {
		t.Fatalf("failed: %+v", res)
	}
	h := last.Load().(parityHit)
	if h.method != "POST" || h.header.Get("Content-Type") != "application/json" || h.header.Get("User-Agent") != "rete-test/1.0" {
		t.Errorf("method=%q ct=%q ua=%q", h.method, h.header.Get("Content-Type"), h.header.Get("User-Agent"))
	}
}
