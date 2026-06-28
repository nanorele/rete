package har

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

const wsHAR = `{
  "log": {
    "version": "1.2",
    "entries": [
      {
        "request": {"method": "GET", "url": "wss://example.com/socket", "headers": [{"name":"Upgrade","value":"websocket"}]},
        "response": {"status": 101, "statusText": "Switching Protocols"},
        "_webSocketMessages": [
          {"type": "send", "time": 1.0, "opcode": 1, "data": "hello"},
          {"type": "receive", "time": 1.1, "opcode": 1, "data": "world"},
          {"type": "receive", "time": 1.2, "opcode": 2, "data": "AAEC"}
        ]
      }
    ]
  }
}`

func TestParse_WebSocketMessages(t *testing.T) {
	h := mustParse(t, wsHAR)
	if len(h.Entries) != 1 {
		t.Fatalf("entries = %d", len(h.Entries))
	}
	e := h.Entries[0]
	if !e.IsWebSocket() {
		t.Error("entry must be detected as WebSocket")
	}
	if len(e.WebSocketMessages) != 3 {
		t.Fatalf("ws messages = %d, want 3", len(e.WebSocketMessages))
	}
	m0 := e.WebSocketMessages[0]
	if !m0.Sent() || m0.Data != "hello" || m0.Binary() {
		t.Errorf("msg0 = %+v", m0)
	}
	m1 := e.WebSocketMessages[1]
	if m1.Sent() || m1.Data != "world" {
		t.Errorf("msg1 = %+v", m1)
	}
	if !e.WebSocketMessages[2].Binary() {
		t.Error("msg2 must be binary (opcode 2)")
	}
}

func TestIsWebSocket_Detection(t *testing.T) {
	cases := []struct {
		url    string
		status int
		want   bool
	}{
		{"wss://x/s", 0, true},
		{"ws://x/s", 0, true},
		{"https://x/s", 101, true},
		{"https://x/s", 200, false},
	}
	for _, c := range cases {
		e := Entry{}
		e.Request.URL = c.url
		e.Response.Status = c.status
		if got := e.IsWebSocket(); got != c.want {
			t.Errorf("IsWebSocket(%q,%d) = %v, want %v", c.url, c.status, got, c.want)
		}
	}
}

func TestWriteDir(t *testing.T) {
	res := []Resource{
		{ZipPath: "example.com/app/main.js", Body: []byte("one")},
		{ZipPath: "example.com/app/main.js", Body: []byte("two")},
		{ZipPath: "api.example.com/data", Body: []byte("three")},
	}
	dirs := map[string]bool{}
	files := map[string]string{}
	n, err := WriteDir("/out", res,
		func(p string) error { dirs[filepath.ToSlash(p)] = true; return nil },
		func(p string, b []byte) error { files[filepath.ToSlash(p)] = string(b); return nil },
	)
	if err != nil {
		t.Fatalf("WriteDir: %v", err)
	}
	if n != 3 {
		t.Fatalf("wrote %d, want 3", n)
	}
	if files["/out/example.com/app/main.js"] != "one" {
		t.Errorf("main.js = %q", files["/out/example.com/app/main.js"])
	}
	if files["/out/example.com/app/main-1.js"] != "two" {
		t.Errorf("deduped main-1.js = %q (files: %v)", files["/out/example.com/app/main-1.js"], keys(files))
	}
	if files["/out/api.example.com/data"] != "three" {
		t.Errorf("data = %q", files["/out/api.example.com/data"])
	}
	if !dirs["/out/example.com/app"] {
		t.Error("expected mkdir of /out/example.com/app")
	}
}

func TestWriteDirOS_RealFilesystem(t *testing.T) {
	dir := t.TempDir()
	res := []Resource{
		{ZipPath: "host/a/b.js", Body: []byte("xyz")},
	}
	n, err := WriteDirOS(dir, res)
	if err != nil || n != 1 {
		t.Fatalf("WriteDirOS: n=%d err=%v", n, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "host", "a", "b.js"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "xyz" {
		t.Errorf("content = %q", got)
	}
}

func TestPretty(t *testing.T) {
	out, ok := Pretty([]byte(`{"a":1,"b":[2,3]}`), "application/json")
	if !ok {
		t.Fatal("valid JSON must prettify")
	}
	if got := string(out); got == `{"a":1,"b":[2,3]}` || len(got) <= len(`{"a":1,"b":[2,3]}`) {
		t.Errorf("not prettified: %q", got)
	}

	if _, ok := Pretty([]byte(`[1,2,3]`), ""); !ok {
		t.Error("array must be detected as JSON by content")
	}
	if out, ok := Pretty([]byte("plain text"), "text/plain"); ok || string(out) != "plain text" {
		t.Errorf("non-JSON should pass through, got %q ok=%v", out, ok)
	}
	if _, ok := Pretty([]byte(`{bad`), "application/json"); ok {
		t.Error("invalid JSON must not report success")
	}
	if _, ok := Pretty(nil, "application/json"); ok {
		t.Error("empty must not prettify")
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
