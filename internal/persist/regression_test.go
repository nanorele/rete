package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tracto/internal/model"
)

func TestAtomicWriteFileSurvivesConcurrentReader(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	if err := os.WriteFile(p, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	stop := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			mu.Lock()
			s := stop
			mu.Unlock()
			if s {
				return
			}
			_, _ = os.ReadFile(p)
		}
	}()

	const n = 200
	fails := 0
	for i := 0; i < n; i++ {
		if err := AtomicWriteFile(p, []byte(`{"a":2}`)); err != nil {
			fails++
		}
	}
	mu.Lock()
	stop = true
	mu.Unlock()
	<-done

	if fails > 0 {
		t.Errorf("AtomicWriteFile failed %d/%d times under a concurrent reader; the rename must be retried", fails, n)
	}
}

func TestRenameWithRetryMissingSource(t *testing.T) {
	dir := t.TempDir()
	err := renameWithRetry(filepath.Join(dir, "nope"), filepath.Join(dir, "dst"))
	if err == nil {
		t.Fatal("renaming a missing source must fail")
	}
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want a not-exist error returned without retrying", err)
	}
}

func TestMarshalRequestNullRawURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"null", "null"},
		{"json null with spaces", "  null  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &model.ParsedRequest{
				Method: "GET",
				URL:    "https://example.com/a",
				RawURL: json.RawMessage(c.raw),
			}
			out := MarshalRequest(req)
			if got := out["url"]; got != "https://example.com/a" {
				t.Errorf("url = %v, want the plain URL string", got)
			}
		})
	}
}

func TestMarshalRequestKeepsUnchangedRawURLObjectVerbatim(t *testing.T) {
	const raw = `{"raw":"https://example.com/v1?a=1","protocol":"https","host":["example","com"],` +
		`"path":["v1"],"query":[{"key":"a","value":"1","description":"hand written"}],"variable":[{"key":"v"}]}`
	req := &model.ParsedRequest{Method: "GET", URL: "https://example.com/v1?a=1", RawURL: json.RawMessage(raw)}

	obj, ok := MarshalRequest(req)["url"].(map[string]any)
	if !ok {
		t.Fatalf("url = %T, want a map", MarshalRequest(req)["url"])
	}
	q, ok := obj["query"].([]any)
	if !ok || len(q) != 1 {
		t.Fatalf("query = %#v, want the original single entry", obj["query"])
	}
	entry, _ := q[0].(map[string]any)
	if entry["description"] != "hand written" {
		t.Errorf("an untouched URL must keep its query metadata verbatim, got %#v", entry)
	}
}

func TestMarshalRequestRebuildsRawURLComponentsWhenURLEdited(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "https://new.example.com:8443/v2/items?page=2&q=x#frag",
		RawURL: json.RawMessage(`{"raw":"https://old.example.com/v1/users?page=1",` +
			`"protocol":"https","host":["old","example","com"],"path":["v1","users"],` +
			`"query":[{"key":"page","value":"1"}],"variable":[{"key":"keepme"}]}`),
	}
	obj, ok := MarshalRequest(req)["url"].(map[string]any)
	if !ok {
		t.Fatalf("url = %T, want a map", MarshalRequest(req)["url"])
	}
	if obj["raw"] != req.URL {
		t.Errorf("raw = %v, want %q", obj["raw"], req.URL)
	}
	host, _ := obj["host"].([]any)
	if len(host) != 3 || host[0] != "new" {
		t.Errorf("host = %#v, want the edited host; importers resolve from components, not raw", obj["host"])
	}
	path, _ := obj["path"].([]any)
	if len(path) != 2 || path[0] != "v2" || path[1] != "items" {
		t.Errorf("path = %#v, want [v2 items]", obj["path"])
	}
	if obj["port"] != "8443" {
		t.Errorf("port = %v, want 8443", obj["port"])
	}
	if obj["hash"] != "frag" {
		t.Errorf("hash = %v, want frag", obj["hash"])
	}
	query, _ := obj["query"].([]any)
	if len(query) != 2 {
		t.Fatalf("query = %#v, want two entries", obj["query"])
	}
	first, _ := query[0].(map[string]any)
	if first["key"] != "page" || first["value"] != "2" {
		t.Errorf("query[0] = %#v, want page=2", query[0])
	}
	if obj["variable"] == nil {
		t.Error("non-URL keys such as variable must survive the rebuild")
	}
}

func TestMarshalRequestUnparseableEditedURLDropsStaleComponents(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "{{baseUrl}}/items",
		RawURL: json.RawMessage(`{"raw":"https://old.example.com/v1","host":["old","example","com"],"protocol":"https"}`),
	}
	obj, ok := MarshalRequest(req)["url"].(map[string]any)
	if !ok {
		t.Fatalf("url = %T, want a map", MarshalRequest(req)["url"])
	}
	if obj["raw"] != "{{baseUrl}}/items" {
		t.Errorf("raw = %v, want the templated URL", obj["raw"])
	}
	for _, k := range []string{"protocol", "host", "port", "path", "query", "hash"} {
		if v, ok := obj[k]; ok {
			t.Errorf("%s = %#v; a URL we cannot decompose must carry raw alone, never stale or escaped components", k, v)
		}
	}
}

func TestMarshalRequestNullRawHeadersFallsBack(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "GET",
		URL:        "https://example.com",
		RawHeaders: json.RawMessage("null"),
		Headers:    map[string]string{"X-Test": "v"},
	}
	out := MarshalRequest(req)
	arr, ok := out["header"].([]any)
	if !ok {
		t.Fatalf("header = %T (%v), want a slice built from req.Headers", out["header"], out["header"])
	}
	if len(arr) != 1 {
		t.Fatalf("header = %v, want the one header from req.Headers", arr)
	}
}

func TestMarshalRequestPreservesUnsupportedAuth(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "https://example.com",
		Extras: map[string]json.RawMessage{
			"auth": json.RawMessage(`{"type":"oauth2"}`),
		},
		Auth: model.ParsedAuth{Type: ""},
	}
	out := MarshalRequest(req)
	raw, ok := out["auth"]
	if !ok {
		t.Fatal("an auth type tracto does not model must survive the round trip, not be destroyed")
	}
	if !strings.Contains(string(raw.(json.RawMessage)), "oauth2") {
		t.Errorf("auth = %s, want the original oauth2 block preserved", raw)
	}
}

func TestMarshalRequestKeepsSetAuth(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "https://example.com",
		Auth:   model.ParsedAuth{Type: "bearer", Token: "tok"},
	}
	out := MarshalRequest(req)
	if out["auth"] == nil {
		t.Error("auth was dropped even though the request has bearer auth")
	}
}

func TestConfigOverrideConcurrentWithReaders(t *testing.T) {
	orig := ConfigDir()
	t.Cleanup(func() { SetConfigOverride("") })

	dir := t.TempDir()
	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = StateFilePath()
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		SetConfigOverride(dir)
		SetConfigOverride("")
	}
	close(stop)
	wg.Wait()

	SetConfigOverride("")
	if got := ConfigDir(); got != orig {
		t.Errorf("ConfigDir after clearing override = %q, want %q", got, orig)
	}
}
