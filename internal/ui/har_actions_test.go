package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/ui/workspace"
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
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "x.har", nil)

	before := len(ui.Tabs)
	ui.harRunEntry(&ui.HARView.Doc.Entries[0])
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
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "x.har", nil)

	ui.harRunEntry(&ui.HARView.Doc.Entries[1])
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

func TestHarWSText(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "x.har", nil)
	e := &ui.HARView.Doc.Entries[1]

	out := string(harWSText(e, false))
	if !strings.Contains(out, "→ send") || !strings.Contains(out, "← receive") {
		t.Errorf("ws transcript missing direction markers:\n%s", out)
	}
	if !strings.Contains(out, `{"hi":1}`) || !strings.Contains(out, "pong") {
		t.Errorf("ws transcript missing payloads:\n%s", out)
	}

	pretty := string(harWSText(e, true))
	if !strings.Contains(pretty, "\"hi\": 1") {
		t.Errorf("pretty ws transcript should indent JSON:\n%s", pretty)
	}

	empty := harWSText(&ui.HARView.Doc.Entries[0], false)
	if !strings.Contains(string(empty), "No WebSocket frames") {
		t.Errorf("expected placeholder, got %q", empty)
	}
}

func TestHarWSURL(t *testing.T) {
	cases := map[string]string{
		"https://x/s": "wss://x/s",
		"http://x/s":  "ws://x/s",
		"wss://x/s":   "wss://x/s",
	}
	for in, want := range cases {
		if got := harWSURL(in); got != want {
			t.Errorf("harWSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHarSkipHeader(t *testing.T) {
	for _, n := range []string{":authority", ":method", "Content-Length", "host"} {
		if !harSkipHeader(n) {
			t.Errorf("%q should be skipped", n)
		}
	}
	for _, n := range []string{"Content-Type", "Accept", "Authorization"} {
		if harSkipHeader(n) {
			t.Errorf("%q should NOT be skipped", n)
		}
	}
}

func TestHarDisplayMethod(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "x.har", nil)
	if got := harDisplayMethod(&ui.HARView.Doc.Entries[0]); got != "POST" {
		t.Errorf("http method display = %q", got)
	}
	if got := harDisplayMethod(&ui.HARView.Doc.Entries[1]); got != "WS" {
		t.Errorf("ws method display = %q, want WS", got)
	}
}

func TestIsProbablyText(t *testing.T) {
	if !isProbablyText([]byte("hello world\nplain")) {
		t.Error("plain text misclassified as binary")
	}
	if isProbablyText([]byte{0x00, 0x01, 0x02, 0xff, 0xfe}) {
		t.Error("binary misclassified as text")
	}
	if !isProbablyText(nil) {
		t.Error("empty should be treated as text")
	}
}

func TestFirstHARPathAndExt(t *testing.T) {
	if got := firstHARPath([]string{`C:\a\b.txt`, `C:\a\c.har`}); got != `C:\a\c.har` {
		t.Errorf("firstHARPath preferred = %q", got)
	}
	if got := firstHARPath([]string{`C:\a\b.txt`}); got != `C:\a\b.txt` {
		t.Errorf("firstHARPath fallback = %q", got)
	}
	if got := firstHARPath(nil); got != "" {
		t.Errorf("firstHARPath empty = %q", got)
	}
	if got := filepathExt(`C:\dir.x\file.har`); got != ".har" {
		t.Errorf("filepathExt = %q", got)
	}
	if got := filepathExt(`C:\dir.x\noext`); got != "" {
		t.Errorf("filepathExt no-ext = %q", got)
	}
}

func TestRouteDroppedFiles_HAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()

	dir := t.TempDir()
	p := filepath.Join(dir, "drop.har")
	if err := os.WriteFile(p, []byte(harRunDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	ui.routeDroppedFiles(droppedPayload{paths: []string{p}})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.drainLoads() && ui.HARView.Doc != nil {
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
