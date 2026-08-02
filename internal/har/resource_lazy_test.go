package har

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestResourcesDoNotRetainBodies(t *testing.T) {
	big := strings.Repeat("x", 64<<10)
	bin := base64.StdEncoding.EncodeToString([]byte("\x00\x01\x02binary"))
	doc := `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","text":"` + big + `"}}},
      {"request":{"method":"GET","url":"https://x/b.png"},
       "response":{"status":200,"content":{"mimeType":"image/png","encoding":"base64","text":"` + bin + `"}}}
    ]}}`
	h := mustParse(t, doc)
	res := h.Resources(false)
	if len(res) != 2 {
		t.Fatalf("Resources = %d, want 2", len(res))
	}
	for i, r := range res {
		if r.Body != nil {
			t.Errorf("resource %d retains a decoded copy of its body (%d bytes)", i, len(r.Body))
		}
	}

	if got := string(res[0].Bytes()); got != big {
		t.Errorf("text body round-trip lost content (%d of %d bytes)", len(got), len(big))
	}
	if res[0].Size != len(big) {
		t.Errorf("text Size = %d, want %d", res[0].Size, len(big))
	}
	if got := string(res[1].Bytes()); got != "\x00\x01\x02binary" {
		t.Errorf("base64 body = %q", got)
	}
	if res[1].Size != len("\x00\x01\x02binary") {
		t.Errorf("base64 Size = %d, want %d", res[1].Size, len("\x00\x01\x02binary"))
	}
}

func TestResourceSizeFallsBackForBadBase64(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","encoding":"base64","text":"!!!not base64!!!"}}}
    ]}}`
	res := mustParse(t, doc).Resources(false)
	if len(res) != 1 {
		t.Fatalf("Resources = %d, want 1", len(res))
	}
	if res[0].Size != len("!!!not base64!!!") {
		t.Errorf("Size = %d, want the raw text length %d", res[0].Size, len("!!!not base64!!!"))
	}
	if string(res[0].Bytes()) != "!!!not base64!!!" {
		t.Errorf("Bytes = %q", res[0].Bytes())
	}
}

func TestWebSocketResourceSizeMatchesTranscript(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/sock"},
       "response":{"status":101},
       "_webSocketMessages":[
         {"type":"send","time":1,"opcode":1,"data":"hello"},
         {"type":"receive","time":2,"opcode":1,"data":"world"}
       ]}
    ]}}`
	res := mustParse(t, doc).Resources(false)
	if len(res) != 1 {
		t.Fatalf("Resources = %d, want 1", len(res))
	}
	if res[0].Body != nil {
		t.Error("websocket resource must not retain its transcript")
	}
	body := res[0].Bytes()
	if res[0].Size != len(body) {
		t.Errorf("Size = %d, transcript is %d bytes", res[0].Size, len(body))
	}
	if !strings.Contains(string(body), "hello") || !strings.Contains(string(body), "world") {
		t.Errorf("transcript = %q", body)
	}
}
