package har

import (
	"strings"
	"testing"
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

func TestHarWSText(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harRunDoc), "x.har", nil)
	e := &st.Doc.Entries[1]

	out := string(wsText(e, false))
	if !strings.Contains(out, "→ send") || !strings.Contains(out, "← receive") {
		t.Errorf("ws transcript missing direction markers:\n%s", out)
	}
	if !strings.Contains(out, `{"hi":1}`) || !strings.Contains(out, "pong") {
		t.Errorf("ws transcript missing payloads:\n%s", out)
	}

	pretty := string(wsText(e, true))
	if !strings.Contains(pretty, "\"hi\": 1") {
		t.Errorf("pretty ws transcript should indent JSON:\n%s", pretty)
	}

	empty := wsText(&st.Doc.Entries[0], false)
	if !strings.Contains(string(empty), "No WebSocket frames") {
		t.Errorf("expected placeholder, got %q", empty)
	}
}

func TestHarDisplayMethod(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harRunDoc), "x.har", nil)
	if got := displayMethod(&st.Doc.Entries[0]); got != "POST" {
		t.Errorf("http method display = %q", got)
	}
	if got := displayMethod(&st.Doc.Entries[1]); got != "WS" {
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
