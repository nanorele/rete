package apptest

import (
	. "tracto/internal/ui"

	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/f32"
)

const harRunDoc = `{
  "log": {
    "version": "1.2",
    "entries": [
      {"request": {"method": "POST", "url": "https://api.example.com/v1/users?q=1",
        "headers": [{"name":"Content-Type","value":"application/json"},{"name":":authority","value":"api.example.com"},{"name":"Content-Length","value":"9"}],
        "postData": {"mimeType":"application/json","text":"{\"a\":1}"}},
        "response": {"status": 200, "content": {"mimeType":"application/json","text":"{}"}}},
      {"request": {"method": "GET", "url": "https://example.com/socket",
        "headers": [{"name":"Upgrade","value":"websocket"}]},
        "response": {"status": 101},
        "_webSocketMessages": [
          {"type":"send","time":1,"opcode":1,"data":"{\"hi\":1}"},
          {"type":"receive","time":2,"opcode":1,"data":"pong"}
        ]}
    ]
  }
}`

func TestHarRunEntry_HTTP(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	before := len(ui.Tabs)
	ui.HARRunEntry(&ui.HARView.Doc.Entries[0])
	if len(ui.Tabs) != before+1 {
		t.Fatalf("expected a new tab; tabs %d→%d", before, len(ui.Tabs))
	}
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.Method != "POST" {
		t.Errorf("method = %q", rt.Method)
	}
	if got := rt.URLInput.Text(); got != "https://api.example.com/v1/users?q=1" {
		t.Errorf("url = %q", got)
	}
	if got := rt.ReqEditor.Text(); got != `{"a":1}` {
		t.Errorf("body = %q", got)
	}
	if ui.SidebarSection != "requests" {
		t.Errorf("section = %q, want requests", ui.SidebarSection)
	}
	if !rt.URLSubmitted {
		t.Error("URLSubmitted must be set so the request auto-runs")
	}
	hdrs := harTabHeaderNames(rt)
	if hdrs[":authority"] || hdrs["content-length"] {
		t.Errorf("must skip pseudo/recomputed headers, got %v", hdrs)
	}
	if !hdrs["content-type"] {
		t.Errorf("Content-Type header should carry over, got %v", hdrs)
	}
}

func TestHarRunEntry_WebSocket(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	ui.HARRunEntry(&ui.HARView.Doc.Entries[1])
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.Method != workspace.MethodWS {
		t.Errorf("ws method = %q, want %q", rt.Method, workspace.MethodWS)
	}
	if got := rt.URLInput.Text(); got != "wss://example.com/socket" {
		t.Errorf("ws url = %q, want wss://example.com/socket", got)
	}
}

func harTabHeaderNames(rt *workspace.RequestTab) map[string]bool {
	out := map[string]bool{}
	for _, h := range rt.Headers {
		out[strings.ToLower(h.Key.Text())] = true
	}
	return out
}

func TestHarWSURL(t *testing.T) {
	cases := map[string]string{
		"https://x/s": "wss://x/s",
		"http://x/s":  "ws://x/s",
		"wss://x/s":   "wss://x/s",
	}
	for in, want := range cases {
		if got := HarWSURL(in); got != want {
			t.Errorf("HarWSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHarSkipHeader(t *testing.T) {
	for _, n := range []string{":authority", ":method", "Content-Length", "host"} {
		if !HarSkipHeader(n) {
			t.Errorf("%q should be skipped", n)
		}
	}
	for _, n := range []string{"Content-Type", "Accept", "Authorization"} {
		if HarSkipHeader(n) {
			t.Errorf("%q should NOT be skipped", n)
		}
	}
}

func TestRouteDroppedFiles_HAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()

	dir := t.TempDir()
	p := filepath.Join(dir, "drop.har")
	if err := os.WriteFile(p, []byte(harRunDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	ui.OnOSFilesDropped([]string{p}, f32.Point{})
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ui.HARView.Doc == nil {
		t.Fatal("dropping a .har in the HAR section must load it")
	}
	if len(ui.HARView.Doc.Entries) != 2 {
		t.Errorf("loaded entries = %d, want 2", len(ui.HARView.Doc.Entries))
	}
}
