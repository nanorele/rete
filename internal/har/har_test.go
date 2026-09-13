package har

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func TestResources_WebSocketExtracted(t *testing.T) {
	h := mustParse(t, wsHAR)
	res := h.Resources(false)
	if len(res) != 1 {
		t.Fatalf("Resources = %d, want 1 (the WS transcript)", len(res))
	}
	r := res[0]
	if r.Method != "WS" {
		t.Errorf("ws resource method = %q, want WS", r.Method)
	}
	if !strings.HasSuffix(r.ZipPath, ".ws.txt") {
		t.Errorf("ws resource path = %q, want .ws.txt suffix", r.ZipPath)
	}
	body := string(r.Bytes())
	for _, want := range []string{">> send", "<< receive", "hello", "world", "[binary]"} {
		if !strings.Contains(body, want) {
			t.Errorf("transcript missing %q:\n%s", want, body)
		}
	}
	// Binary frame "AAEC" is base64 for bytes 0x00 0x01 0x02 — must be decoded.
	if !strings.Contains(body, "\x00\x01\x02") {
		t.Errorf("binary frame not decoded from base64:\n%q", body)
	}
}

func TestResources_KeepsUndecodableBody(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x.com/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","encoding":"base64","text":"!!!not base64!!!"}}}
    ]}}`
	h := mustParse(t, doc)
	res := h.Resources(false)
	if len(res) != 1 {
		t.Fatalf("Resources = %d, want 1 (bad base64 must not drop the file)", len(res))
	}
	if string(res[0].Bytes()) != "!!!not base64!!!" {
		t.Errorf("body = %q, want raw undecoded text", res[0].Bytes())
	}
}

func TestWriteZip_SkipsBadPathsAndContinues(t *testing.T) {
	res := []Resource{
		{ZipPath: "", Body: []byte("dropped")},
		{ZipPath: "host/a.js", Body: []byte("one")},
		{ZipPath: "host/b.js", Body: []byte("two")},
	}
	var buf bytes.Buffer
	n, err := WriteZip(&buf, res)
	if err != nil {
		t.Fatalf("WriteZip: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d, want 2 (empty path skipped, rest written)", n)
	}
}

func TestWriteDir_ContinuesPastFailures(t *testing.T) {
	res := []Resource{
		{ZipPath: "host/good1.js", Body: []byte("one")},
		{ZipPath: "host/bad.js", Body: []byte("two")},
		{ZipPath: "host/good2.js", Body: []byte("three")},
	}
	files := map[string]string{}
	n, err := WriteDir("/out", res,
		func(string) error { return nil },
		func(p string, b []byte) error {
			if strings.Contains(p, "bad.js") {
				return os.ErrPermission
			}
			files[filepath.ToSlash(p)] = string(b)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("WriteDir must not abort on a single failure: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d, want 2 (bad file skipped, others written)", n)
	}
	if files["/out/host/good1.js"] != "one" || files["/out/host/good2.js"] != "three" {
		t.Errorf("surviving files not written: %v", keys(files))
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

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

type failWriter struct {
	allowCalls int
	calls      int
}

func (w *failWriter) Write(p []byte) (int, error) {
	if w.calls >= w.allowCalls {
		return 0, errors.New("writer failed")
	}
	w.calls++
	return len(p), nil
}

func TestZipPath_UnparsableURL(t *testing.T) {
	got := ZipPath("ht\x7ftp://example.com/a/b", "")
	if !strings.HasPrefix(got, "_invalid/") {
		t.Fatalf("ZipPath = %q, want _invalid/ prefix", got)
	}
	if strings.Contains(got[len("_invalid/"):], "/") {
		t.Errorf("_invalid payload must be flattened, got %q", got)
	}
}

func TestZipPath_NoHostAndTrailingSlash(t *testing.T) {
	cases := []struct{ url, mime, want string }{
		{"/just/a/path/", "", "_nohost/just/a/path/index"},
		{"https://example.com", "", "example.com/index"},
		{"https://example.com/lib/mod", "application/javascript", "example.com/lib/mod.js"},
		{"https://example.com/lib/mod.mjs", "application/javascript", "example.com/lib/mod.mjs"},
	}
	for _, c := range cases {
		if got := ZipPath(c.url, c.mime); got != c.want {
			t.Errorf("ZipPath(%q,%q) = %q, want %q", c.url, c.mime, got, c.want)
		}
	}
}

func TestZipPath_DotSegmentsNeutralized(t *testing.T) {
	got := ZipPath("https://example.com/../../etc/passwd", "")
	for _, seg := range strings.Split(got, "/") {
		if seg == ".." || seg == "." || seg == "" {
			t.Fatalf("ZipPath leaked a traversal segment: %q", got)
		}
	}
}

func TestHostOf_Unparsable(t *testing.T) {
	if got := hostOf("ht\x7ftp://example.com/a"); got != "" {
		t.Errorf("hostOf on unparsable URL = %q, want empty", got)
	}
	if got := hostOf("https://example.com:8443/a"); got != "example.com:8443" {
		t.Errorf("hostOf = %q", got)
	}
}

func TestWSTranscript_BinaryFrameNotBase64(t *testing.T) {
	e := &Entry{WebSocketMessages: []WSMessage{
		{Type: "send", Time: 0.5, Opcode: 2, Data: "!!! not base64 !!!"},
	}}
	got := string(WSTranscript(e))
	if !strings.Contains(got, "!!! not base64 !!!") {
		t.Errorf("undecodable binary frame must fall back to raw data, got %q", got)
	}
	if !strings.Contains(got, "[binary]") {
		t.Errorf("missing binary marker: %q", got)
	}
}

func TestWSTranscript_Empty(t *testing.T) {
	if got := WSTranscript(&Entry{}); len(got) != 0 {
		t.Errorf("empty transcript = %q", got)
	}
}

func TestWriteZip_CreateFailsOnOversizedName(t *testing.T) {
	res := []Resource{
		{ZipPath: strings.Repeat("a", 70000), Body: []byte("x")},
		{ZipPath: "host/ok.js", Body: []byte("y")},
	}
	var buf bytes.Buffer
	n, err := WriteZip(&buf, res)
	if err != nil {
		t.Fatalf("WriteZip must survive a bad entry name: %v", err)
	}
	if n != 1 {
		t.Fatalf("wrote %d, want 1", n)
	}
}

func TestWriteZip_UnderlyingWriterFails(t *testing.T) {
	res := []Resource{{ZipPath: "host/a.js", Body: []byte("hello")}}
	n, err := WriteZip(&failWriter{allowCalls: 0}, res)
	if err == nil {
		t.Fatalf("WriteZip must report the underlying writer failure (n=%d)", n)
	}
}

func TestWriteZip_WriterFailsMidStream(t *testing.T) {
	body := make([]byte, 8<<20)
	rnd := rand.New(rand.NewSource(1))
	rnd.Read(body)
	res := []Resource{
		{ZipPath: "host/a.js", Body: body},
		{ZipPath: "host/b.js", Body: body},
	}
	n, err := WriteZip(&failWriter{allowCalls: 2}, res)
	if err == nil {
		t.Fatalf("WriteZip must report failure when the sink dies mid-stream (n=%d)", n)
	}
	if n != 0 {
		t.Errorf("written = %d, want 0 (no entry made it to the sink)", n)
	}
}

func TestWriteDir_SkipsRootOnlyPath(t *testing.T) {
	res := []Resource{
		{ZipPath: "/", Body: []byte("dropped")},
		{ZipPath: "host/keep.js", Body: []byte("kept")},
	}
	files := map[string]string{}
	n, err := WriteDir("/out", res,
		func(string) error { return nil },
		func(p string, b []byte) error { files[p] = string(b); return nil },
	)
	if err != nil {
		t.Fatalf("WriteDir: %v", err)
	}
	if n != 1 {
		t.Fatalf("wrote %d, want 1", n)
	}
}

func TestWriteDir_MkdirFailureSkipsEntry(t *testing.T) {
	res := []Resource{
		{ZipPath: "bad/a.js", Body: []byte("one")},
		{ZipPath: "good/b.js", Body: []byte("two")},
	}
	var written []string
	n, err := WriteDir("/out", res,
		func(p string) error {
			if strings.Contains(p, "bad") {
				return os.ErrPermission
			}
			return nil
		},
		func(p string, b []byte) error { written = append(written, p); return nil },
	)
	if err != nil {
		t.Fatalf("WriteDir must not abort on mkdir failure: %v", err)
	}
	if n != 1 {
		t.Fatalf("wrote %d, want 1 (mkdir failure skipped), files=%v", n, written)
	}
}

func TestSanitize_TruncatesAndReplaces(t *testing.T) {
	got := sanitize(strings.Repeat("a/b", 200))
	if len(got) != 100 {
		t.Errorf("len = %d, want 100", len(got))
	}
	if strings.Contains(got, "/") {
		t.Errorf("separator survived: %q", got)
	}
	if got := sanitize(`a:b?c*d"e<f>g|h\i/j`); strings.ContainsAny(got, `:?*"<>|\/`) {
		t.Errorf("unsanitized: %q", got)
	}
}

func TestSanitizeSegment_ReservedAndTruncated(t *testing.T) {
	for _, in := range []string{"", ".", ".."} {
		if got := sanitizeSegment(in); got != "_" {
			t.Errorf("sanitizeSegment(%q) = %q, want _", in, got)
		}
	}
	if got := sanitizeSegment(strings.Repeat("z", 400)); len(got) != 150 {
		t.Errorf("len = %d, want 150", len(got))
	}
	if got := sanitizeSegment("a/b"); got != "a/b" {
		t.Errorf("segment sanitizer must keep slashes for the caller to split: %q", got)
	}
}

func TestItoa(t *testing.T) {
	cases := map[int]string{0: "0", 1: "1", 9: "9", 10: "10", 1234567890: "1234567890"}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Errorf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseReader_ReadError(t *testing.T) {
	h, err := ParseReader(errReader{})
	if err == nil {
		t.Fatal("ParseReader must surface reader errors")
	}
	if h != nil {
		t.Errorf("HAR = %+v, want nil", h)
	}
}

func TestResponseHeader_Missing(t *testing.T) {
	r := Response{Headers: []Header{{Name: "X-A", Value: "1"}}}
	if got := r.Header("X-Missing"); got != "" {
		t.Errorf("Header(missing) = %q", got)
	}
	if got := r.Header("x-a"); got != "1" {
		t.Errorf("Header must be case-insensitive, got %q", got)
	}
}

func TestContentType_Fallbacks(t *testing.T) {
	var withParams Entry
	withParams.Response.Headers = []Header{{Name: "Content-Type", Value: " text/html; charset=utf-8 "}}
	if got := withParams.ContentType(); got != "text/html" {
		t.Errorf("ContentType = %q, want text/html", got)
	}

	var noParams Entry
	noParams.Response.Headers = []Header{{Name: "content-type", Value: "  application/wasm  "}}
	if got := noParams.ContentType(); got != "application/wasm" {
		t.Errorf("ContentType = %q, want application/wasm", got)
	}

	var none Entry
	if got := none.ContentType(); got != "" {
		t.Errorf("ContentType = %q, want empty", got)
	}

	var blankMime Entry
	blankMime.Response.Content.MimeType = "   "
	if got := blankMime.ContentType(); got != "" {
		t.Errorf("whitespace mimeType must not win, got %q", got)
	}
}

func TestSummary_PendingStatusAndTimestamps(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"startedDateTime":"2024-01-02T00:00:00Z","request":{"method":"GET","url":"https://x/a"},
       "response":{"status":0,"content":{"mimeType":"text/plain","text":"ab"}}},
      {"startedDateTime":"2024-01-01T00:00:00Z","request":{"method":"GET","url":"https://x/b"},
       "response":{"status":200,"content":{"mimeType":"text/plain","text":"cde"}}},
      {"request":{"url":"https://x/c"},"response":{"status":0}}
    ]}}`
	s := mustParse(t, doc).Summary()
	var pending int
	for _, c := range s.Statuses {
		if c.Label == "(pending)" {
			pending = c.Count
		}
	}
	if pending != 2 {
		t.Errorf("(pending) count = %d, want 2 (statuses=%+v)", pending, s.Statuses)
	}
	if s.FirstStarted != "2024-01-01T00:00:00Z" {
		t.Errorf("FirstStarted = %q", s.FirstStarted)
	}
	if s.LastStarted != "2024-01-02T00:00:00Z" {
		t.Errorf("LastStarted = %q", s.LastStarted)
	}
	if s.ResourceCount != 2 || s.TotalBodyBytes != 5 {
		t.Errorf("ResourceCount=%d TotalBodyBytes=%d, want 2/5", s.ResourceCount, s.TotalBodyBytes)
	}
	if len(s.Methods) != 1 || s.Methods[0].Label != "GET" || s.Methods[0].Count != 2 {
		t.Errorf("Methods = %+v, want a single GET x2 (blank method must be dropped)", s.Methods)
	}
}

func TestStatusLabel(t *testing.T) {
	cases := map[int]string{-1: "(pending)", 0: "(pending)", 200: "200", 599: "599"}
	for in, want := range cases {
		if got := statusLabel(in); got != want {
			t.Errorf("statusLabel(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParse_AdversarialDocuments(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		wantErr bool
	}{
		{"truncated", `{"log":{"version":"1.2","entries":[{"request":`, true},
		{"log is a string", `{"log":"nope"}`, true},
		{"log is null", `{"log":null}`, true},
		{"entries is an object", `{"log":{"version":"1.2","entries":{"a":1}}}`, true},
		{"time is a string", `{"log":{"version":"1.2","entries":[{"time":"slow"}]}}`, true},
		{"status is a string", `{"log":{"version":"1.2","entries":[{"response":{"status":"200"}}]}}`, true},
		{"trailing garbage", `{"log":{"version":"1.2"}}xyz`, true},
		{"bare array", `[1,2,3]`, true},
		{"empty object", `{}`, true},
		{"empty log", `{"log":{}}`, true},
		{"only whitespace", "   \n\t ", true},
		{"entries null", `{"log":{"version":"1.2","entries":null}}`, false},
		{"pages only", `{"log":{"pages":[{"id":"p1","title":"t"}]}}`, false},
		{"creator only", `{"log":{"creator":{"name":"x","version":"1"}}}`, false},
		{"unknown fields ignored", `{"log":{"version":"1.2","_extra":{"deep":[1,2]},"entries":[]},"other":1}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h, err := Parse([]byte(c.doc))
			if c.wantErr {
				if err == nil {
					t.Fatalf("Parse(%s) = %+v, want error", c.doc, h)
				}
				if h != nil {
					t.Errorf("HAR must be nil on error, got %+v", h)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%s): %v", c.doc, err)
			}
			if h == nil {
				t.Fatal("HAR is nil without an error")
			}
		})
	}
}

func TestParse_EmptyInputIsNotHAR(t *testing.T) {
	if _, err := Parse(nil); !errors.Is(err, ErrNotHAR) {
		t.Errorf("Parse(nil) err = %v, want ErrNotHAR", err)
	}
	if _, err := Parse([]byte{}); !errors.Is(err, ErrNotHAR) {
		t.Errorf("Parse([]) err = %v, want ErrNotHAR", err)
	}
}

func TestParse_DeeplyNestedDoesNotCrash(t *testing.T) {
	doc := `{"log":{"version":"1.2","entries":[],"_x":` +
		strings.Repeat("[", 20000) + strings.Repeat("]", 20000) + `}}`
	if _, err := Parse([]byte(doc)); err == nil {
		t.Log("deeply nested payload accepted")
	}
}

func TestParse_UnicodeAndEscapes(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GÉT","url":"https://пример.рф/путь/f.js?q=é"},
       "response":{"status":200,"statusText":"ОК","content":{"mimeType":"text/plain","text":"日本語 😀"}}}
    ]}}`
	h := mustParse(t, doc)
	e := h.Entries[0]
	if !strings.Contains(e.Response.Content.Text, "日本語") {
		t.Errorf("unicode body lost: %q", e.Response.Content.Text)
	}
	if !strings.Contains(e.Response.Content.Text, "\U0001F600") {
		t.Errorf("surrogate pair not decoded: %q", e.Response.Content.Text)
	}
	if !strings.Contains(e.Request.URL, "пример") {
		t.Errorf("unicode URL lost: %q", e.Request.URL)
	}
	if e.Request.Method != "GÉT" {
		t.Errorf("unicode method lost: %q", e.Request.Method)
	}
	res := h.Resources(false)
	if len(res) != 1 {
		t.Fatalf("Resources = %d", len(res))
	}
	if strings.ContainsAny(res[0].ZipPath, "\\:?*\"<>|") {
		t.Errorf("unsanitized unicode zip path: %q", res[0].ZipPath)
	}
}

func TestParse_LoneSurrogateBecomesReplacementChar(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"url":"https://x/a"},"response":{"content":{"text":"\ud800"}}}]}}`
	h, err := Parse([]byte(doc))
	if err != nil {
		return
	}
	if got := h.Entries[0].Response.Content.Text; got != "�" {
		t.Errorf("lone surrogate = %q, want U+FFFD replacement", got)
	}
}

func TestParse_InvalidUTF8BytesInBody(t *testing.T) {
	doc := "{\"log\":{\"version\":\"1.2\",\"entries\":[{\"request\":{\"url\":\"https://x/a\"}," +
		"\"response\":{\"content\":{\"text\":\"a\xffb\"}}}]}}"
	if _, err := Parse([]byte(doc)); err != nil {
		return
	}
}

func TestParse_HugeBodyEntry(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	doc := `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/big.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","text":"` + big + `"}}}]}}`
	h := mustParse(t, doc)
	res := h.Resources(true)
	if len(res) != 1 || len(res[0].Bytes()) != 1<<20 {
		t.Fatalf("huge body mishandled: n=%d", len(res))
	}
	var buf bytes.Buffer
	if _, err := WriteZip(&buf, res); err != nil {
		t.Fatalf("WriteZip huge: %v", err)
	}
}

func TestDecodeBody_Base64Variants(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		encoding string
		want     string
		wantErr  bool
	}{
		{"plain", "hello", "", "hello", false},
		{"mixed case encoding label", "aGk=", "BaSe64", "hi", false},
		{"unpadded", "aGk", "base64", "", true},
		{"embedded newline", "aG\nk=", "base64", "hi", false},
		{"url alphabet", "-_8=", "base64", "", true},
		{"empty text", "", "base64", "", false},
		{"whitespace only", " ", "base64", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var e Entry
			e.Response.Content.Text = c.text
			e.Response.Content.Encoding = c.encoding
			got, present, err := e.DecodeBody()
			if c.text == "" {
				if present {
					t.Fatal("empty text must report absent")
				}
				return
			}
			if !present {
				t.Fatal("non-empty text must report present")
			}
			if c.wantErr {
				if err == nil {
					t.Fatalf("want error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeBody: %v", err)
			}
			if string(got) != c.want {
				t.Errorf("body = %q, want %q", got, c.want)
			}
		})
	}
}

func TestIsJavaScript_URLAndMime(t *testing.T) {
	cases := []struct {
		url, mime string
		want      bool
	}{
		{"https://x/a.JS", "", true},
		{"https://x/a.js?v=2", "", true},
		{"https://x/a.json", "", false},
		{"https://x/a", "application/ecmascript", true},
		{"https://x/a", "Text/JavaScript", true},
		{"ht\x7ftp://x/a.js", "", false},
		{"https://x/a", "text/css", false},
	}
	for _, c := range cases {
		var e Entry
		e.Request.URL = c.url
		e.Response.Content.MimeType = c.mime
		if got := e.IsJavaScript(); got != c.want {
			t.Errorf("IsJavaScript(%q,%q) = %v, want %v", c.url, c.mime, got, c.want)
		}
	}
}

func TestLooksLikeJSON_Direct(t *testing.T) {
	cases := []struct {
		body, mime string
		want       bool
	}{
		{"", "text/plain", false},
		{"", "application/json", true},
		{"x", "APPLICATION/JSON", true},
		{"{", "text/plain", true},
		{"[", "text/plain", true},
		{"nope", "text/plain", false},
	}
	for _, c := range cases {
		if got := looksLikeJSON([]byte(c.body), c.mime); got != c.want {
			t.Errorf("looksLikeJSON(%q,%q) = %v, want %v", c.body, c.mime, got, c.want)
		}
	}
}

func TestLooksLikeMarkupContent_ScanLimited(t *testing.T) {
	if looksLikeMarkupContent([]byte("<")) {
		t.Error("a single < is not markup")
	}
	if looksLikeMarkupContent([]byte("<a>")) {
		t.Error("one tag is not enough")
	}
	if !looksLikeMarkupContent([]byte("<a></a>")) {
		t.Error("two tags is markup")
	}
	early := []byte("<a><b>" + strings.Repeat("x", 8000) + "</b></a>")
	if !looksLikeMarkupContent(early) {
		t.Error("tags inside the 4096 scan window must be detected")
	}
	late := []byte("<a>" + strings.Repeat("x", 8000) + "</a>")
	if looksLikeMarkupContent(late) {
		t.Error("tags beyond the 4096 scan window must not count")
	}
}

func TestLooksLikeBraceCode_ScanLimited(t *testing.T) {
	if !looksLikeBraceCode([]byte("x"), "application/typescript") {
		t.Error("typescript mime is brace code")
	}
	if !looksLikeBraceCode([]byte("x"), "text/css") {
		t.Error("/css mime is brace code")
	}
	if looksLikeBraceCode([]byte("a;b;c;d;e;f;"), "text/plain") == false {
		t.Error("dense semicolons on one line are brace code")
	}
	dense := []byte(strings.Repeat("a;", 4000))
	if !looksLikeBraceCode(dense, "") {
		t.Error("long dense payload must be detected within the scan window")
	}
	newlines := []byte(strings.Repeat("a;\n", 100))
	if looksLikeBraceCode(newlines, "") {
		t.Error("well-broken source is not minified brace code")
	}
}

func TestRegexCanStart(t *testing.T) {
	for _, b := range []byte{0, '(', ',', '=', ':', '[', '!', '{', '}', ';', '<'} {
		if !regexCanStart(b) {
			t.Errorf("regexCanStart(%q) = false", b)
		}
	}
	for _, b := range []byte{'a', 'Z', '0', ')', ']', '_', '$', '.'} {
		if regexCanStart(b) {
			t.Errorf("regexCanStart(%q) = true", b)
		}
	}
}

func TestPrettyCode_BraceCodeSniffedWithoutMime(t *testing.T) {
	src := []byte("a{b:1;c:2;}d{e:3;f:4;}")
	out, ok := PrettyCode(src, "text/plain")
	if !ok {
		t.Fatalf("minified brace code without a mime must be beautified, got %q", out)
	}
	if !bytes.Contains(out, []byte("\n")) {
		t.Errorf("no line breaks introduced: %q", out)
	}
}

func TestPrettyCode_EmptyAndUnknown(t *testing.T) {
	if out, ok := PrettyCode(nil, "application/json"); ok || len(out) != 0 {
		t.Errorf("nil = %q ok=%v", out, ok)
	}
	if out, ok := PrettyCode([]byte("   \n  "), ""); ok {
		t.Errorf("whitespace = %q ok=%v", out, ok)
	}
	if out, ok := PrettyCode([]byte("\x00\x01\x02binary"), "application/octet-stream"); ok {
		t.Errorf("binary = %q ok=%v", out, ok)
	}
}

func TestBeautifyBraces_LeadingCloseBrace(t *testing.T) {
	out, ok := beautifyBraces([]byte("}"), false)
	if ok {
		t.Errorf("a bare } is already minimal, got changed to %q", out)
	}
	if string(out) != "}" {
		t.Errorf("out = %q, want }", out)
	}
}

func TestBeautifyBraces_UnbalancedExtraCloses(t *testing.T) {
	out, _ := beautifyBraces([]byte("a{b;}}}"), false)
	if strings.Count(string(out), "}") != 3 {
		t.Errorf("closing braces lost: %q", out)
	}
}

func TestBeautifyBraces_EscapesInsideStrings(t *testing.T) {
	src := []byte(`var s = "a\"b{c}d;e";var t = 'x\'y{z}';`)
	out, _ := beautifyBraces(src, false)
	if !bytes.Contains(out, []byte(`"a\"b{c}d;e"`)) {
		t.Errorf("escaped double-quoted string mangled: %q", out)
	}
	if !bytes.Contains(out, []byte(`'x\'y{z}'`)) {
		t.Errorf("escaped single-quoted string mangled: %q", out)
	}
}

func TestBeautifyBraces_UnterminatedString(t *testing.T) {
	out, _ := beautifyBraces([]byte(`var s = "never closed`), false)
	if !bytes.Contains(out, []byte("never closed")) {
		t.Errorf("unterminated string dropped: %q", out)
	}
}

func TestBeautifyBraces_UnterminatedBlockComment(t *testing.T) {
	out, _ := beautifyBraces([]byte("a=1;/* never closed {;"), false)
	if !bytes.Contains(out, []byte("never closed")) {
		t.Errorf("unterminated comment dropped: %q", out)
	}
}

func TestBeautifyBraces_RegexCharClassAndEscapes(t *testing.T) {
	src := []byte(`var r = /[/{};]\/x/g;var q = /a\\/;`)
	out, _ := beautifyBraces(src, false)
	if !bytes.Contains(out, []byte(`/[/{};]\/x/g`)) {
		t.Errorf("regex with a char class was split: %q", out)
	}
	if !bytes.Contains(out, []byte(`/a\\/`)) {
		t.Errorf("regex with a trailing escape was mangled: %q", out)
	}
}

func TestBeautifyBraces_UnterminatedRegex(t *testing.T) {
	out, _ := beautifyBraces([]byte(`var r = /abc[def`), false)
	if !bytes.Contains(out, []byte("abc[def")) {
		t.Errorf("unterminated regex dropped: %q", out)
	}
}

func TestBeautifyBraces_DivisionIsNotRegex(t *testing.T) {
	out, _ := beautifyBraces([]byte("var x = a / b;var y = c/d;"), false)
	if !bytes.Contains(out, []byte("a / b")) {
		t.Errorf("division mangled: %q", out)
	}
	if strings.Count(string(out), ";") != 2 {
		t.Errorf("statement separators lost, division parsed as regex: %q", out)
	}
}

func TestBeautifyBraces_KeywordAfterCloseWithSpace(t *testing.T) {
	out, _ := beautifyBraces([]byte("if(a){b;} else {c;}"), false)
	got := string(out)
	i := strings.Index(got, "else")
	if i < 0 {
		t.Fatalf("else lost: %q", got)
	}
	j := strings.LastIndex(got[:i], "}")
	if j < 0 {
		t.Fatalf("no closing brace before else: %q", got)
	}
	if strings.Contains(got[j:i], "\n") {
		t.Errorf("else must stay attached to its closing brace across whitespace: %q", got)
	}
}

func TestBeautifyBraces_TrailingCloseAtEOF(t *testing.T) {
	out, ok := beautifyBraces([]byte("function f(){a;}"), false)
	if !ok {
		t.Fatalf("minified function must be beautified: %q", out)
	}
	if strings.HasSuffix(string(out), "\n") {
		t.Errorf("trailing newline not trimmed: %q", out)
	}
	if !strings.HasSuffix(string(out), "}") {
		t.Errorf("out = %q, want to end with }", out)
	}
	if got := strings.Count(string(out), "\n"); got != 2 {
		t.Errorf("line count = %d, want 2 (open, body, close): %q", got, out)
	}
}

func TestBeautifyBraces_SemicolonsInsideParens(t *testing.T) {
	out, _ := beautifyBraces([]byte("for(var i=0;i<3;i++){f(i);}"), false)
	if !bytes.Contains(out, []byte("for(var i=0; i<3; i++)")) {
		t.Errorf("for header split across lines: %q", out)
	}
	if bytes.Contains(out, []byte("i++;\n)")) {
		t.Errorf("semicolon inside parens forced a newline: %q", out)
	}
}

func TestCollapseBlankLines(t *testing.T) {
	cases := map[string]string{
		"a\n\n\n\n\nb": "a\n\nb",
		"a\n\nb":       "a\n\nb",
		"a\nb":         "a\nb",
		"\n\n\n\n":     "\n\n",
		"":             "",
	}
	for in, want := range cases {
		if got := string(collapseBlankLines([]byte(in))); got != want {
			t.Errorf("collapseBlankLines(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBeautifyMarkup_NoTokens(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		out, ok := beautifyMarkup([]byte(in))
		if ok {
			t.Errorf("beautifyMarkup(%q) reported a change: %q", in, out)
		}
		if string(out) != in {
			t.Errorf("beautifyMarkup(%q) = %q, want passthrough", in, out)
		}
	}
}

func TestBeautifyMarkup_EmptyElementStaysOnOneLine(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<ul><li></li><li>x</li></ul>"))
	got := string(out)
	if !strings.Contains(got, "<li></li>") {
		t.Errorf("empty element split across lines: %q", got)
	}
	if !strings.Contains(got, "<li>x</li>") {
		t.Errorf("short text element split across lines: %q", got)
	}
}

func TestScanMarkup_UnterminatedComment(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<div><!-- never closed"))
	if !bytes.Contains(out, []byte("<!-- never closed")) {
		t.Errorf("unterminated comment dropped: %q", out)
	}
}

func TestScanMarkup_CommentAtVeryEnd(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<div><!--"))
	if !bytes.Contains(out, []byte("<!--")) {
		t.Errorf("bare comment opener dropped: %q", out)
	}
}

func TestScanMarkup_CDATA(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<r><![CDATA[ raw <not-a-tag> & stuff ]]><c>x</c></r>"))
	got := string(out)
	if !strings.Contains(got, "<![CDATA[ raw <not-a-tag> & stuff ]]>") {
		t.Errorf("CDATA section mangled: %q", got)
	}
	if !strings.Contains(got, "<c>x</c>") {
		t.Errorf("element after CDATA lost: %q", got)
	}
}

func TestScanMarkup_UnterminatedCDATA(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<r><![CDATA[ never closed"))
	if !bytes.Contains(out, []byte("never closed")) {
		t.Errorf("unterminated CDATA dropped: %q", out)
	}
}

func TestScanMarkup_UnterminatedTag(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<div><span class=\"x\""))
	if !bytes.Contains(out, []byte("<span")) {
		t.Errorf("unterminated tag dropped: %q", out)
	}
}

func TestScanMarkup_UnterminatedQuotedAttribute(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<div><a href=\"never closed"))
	if !bytes.Contains(out, []byte("never closed")) {
		t.Errorf("unterminated attribute dropped: %q", out)
	}
}

func TestScanMarkup_RawElementWithoutCloseTag(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<html><script>var a=1;var b=2;"))
	got := string(out)
	if !strings.Contains(got, "var a=1;") {
		t.Errorf("unterminated script body dropped: %q", got)
	}
	if !strings.Contains(got, "var b=2;") {
		t.Errorf("unterminated script body truncated: %q", got)
	}
}

func TestScanMarkup_EmptyRawElement(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<html><script></script><style>   </style></html>"))
	got := string(out)
	if !strings.Contains(got, "<script>") || !strings.Contains(got, "</script>") {
		t.Errorf("empty script lost: %q", got)
	}
	if !strings.Contains(got, "<style>") || !strings.Contains(got, "</style>") {
		t.Errorf("whitespace-only style lost: %q", got)
	}
}

func TestScanMarkup_UnformattableRawBodyKeptVerbatim(t *testing.T) {
	out, _ := beautifyMarkup([]byte("<html><script>\nx\n</script></html>"))
	if !bytes.Contains(out, []byte("x")) {
		t.Errorf("raw body dropped: %q", out)
	}
}

func TestTagName(t *testing.T) {
	cases := map[string]string{
		"<div>":         "div",
		"</DIV>":        "div",
		"</ div >":      "div",
		"<br/>":         "br",
		"<a href='x'>":  "a",
		"<":             "",
		"</":            "",
		"<ns:elem attr": "ns:elem",
	}
	for in, want := range cases {
		if got := tagName(in); got != want {
			t.Errorf("tagName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanTagEnd_NoCloseAngle(t *testing.T) {
	src := []byte("<div class=x")
	if got := scanTagEnd(src, 0); got != len(src) {
		t.Errorf("scanTagEnd = %d, want %d", got, len(src))
	}
}

func TestIndexFrom_Bounds(t *testing.T) {
	src := []byte("abcXYZdef")
	cases := []struct {
		from int
		sep  string
		want int
	}{
		{-5, "XYZ", 3},
		{0, "XYZ", 3},
		{3, "XYZ", 3},
		{4, "XYZ", -1},
		{len(src), "abc", -1},
		{len(src) + 10, "abc", -1},
		{0, "nope", -1},
	}
	for _, c := range cases {
		if got := indexFrom(src, c.from, []byte(c.sep)); got != c.want {
			t.Errorf("indexFrom(%d,%q) = %d, want %d", c.from, c.sep, got, c.want)
		}
	}
}

func TestIndexFromFold_ASCIIBounds(t *testing.T) {
	src := []byte("abc</SCRIPT>def")
	cases := []struct {
		from int
		sep  string
		want int
	}{
		{-5, "</script", 3},
		{0, "</script", 3},
		{3, "</SCRIPT", 3},
		{4, "</script", -1},
		{0, "nope", -1},
	}
	for _, c := range cases {
		if got := indexFromFold(src, c.from, []byte(c.sep)); got != c.want {
			t.Errorf("indexFromFold(%d,%q) = %d, want %d", c.from, c.sep, got, c.want)
		}
	}
}

func TestRawElementBody(t *testing.T) {
	if got := rawElementBody("script", []byte("   \n  ")); got != "" {
		t.Errorf("blank script body = %q, want empty", got)
	}
	if got := rawElementBody("pre", []byte("  keep\n  me  ")); got != "keep\n  me" {
		t.Errorf("pre body = %q", got)
	}
	if got := rawElementBody("script", []byte("a{b;}")); !strings.Contains(got, "\n") {
		t.Errorf("script body not beautified: %q", got)
	}
}

func TestExportAll_RoundTrip(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x.com/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","text":"var a=1"}}},
      {"request":{"method":"GET","url":"https://x.com/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","text":"var b=2"}}}]}}`
	h := mustParse(t, doc)
	var buf bytes.Buffer
	n, err := h.ExportAll(&buf)
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if n != 2 {
		t.Fatalf("ExportAll wrote %d, want 2", n)
	}
}

func TestResources_SkipsEntriesWithoutBody(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/empty"},"response":{"status":204,"content":{}}},
      {"request":{"method":"GET","url":"https://x/a.js"},
       "response":{"status":200,"content":{"mimeType":"application/javascript","text":"a"}}}]}}`
	h := mustParse(t, doc)
	if got := len(h.Resources(false)); got != 1 {
		t.Errorf("Resources = %d, want 1", got)
	}
	if got := len(h.Resources(true)); got != 1 {
		t.Errorf("Resources(jsOnly) = %d, want 1", got)
	}
}

const sampleHAR = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "Firefox", "version": "126.0"},
    "browser": {"name": "Firefox", "version": "126.0"},
    "pages": [{"id": "page_1", "title": "Example", "startedDateTime": "2024-01-01T10:00:00.000Z"}],
    "entries": [
      {
        "startedDateTime": "2024-01-01T10:00:00.100Z",
        "time": 12.5,
        "request": {"method": "GET", "url": "https://example.com/app/main.js", "httpVersion": "HTTP/2",
          "headers": [{"name": "Accept", "value": "*/*"}], "queryString": []},
        "response": {"status": 200, "statusText": "OK", "httpVersion": "HTTP/2",
          "headers": [{"name": "Content-Type", "value": "application/javascript; charset=utf-8"}],
          "content": {"size": 11, "mimeType": "application/javascript", "text": "console.log"}}
      },
      {
        "startedDateTime": "2024-01-01T10:00:00.200Z",
        "time": 4.0,
        "request": {"method": "POST", "url": "https://api.example.com/v1/data?x=1", "httpVersion": "HTTP/1.1",
          "headers": [], "queryString": [{"name": "x", "value": "1"}]},
        "response": {"status": 201, "statusText": "Created", "httpVersion": "HTTP/1.1",
          "headers": [{"name": "Content-Type", "value": "application/json"}],
          "content": {"size": 4, "mimeType": "application/json", "encoding": "base64", "text": "eyJhIjoxfQ=="}}
      },
      {
        "startedDateTime": "2024-01-01T10:00:00.300Z",
        "time": 0,
        "request": {"method": "GET", "url": "https://example.com/img/logo.png", "httpVersion": "HTTP/2"},
        "response": {"status": 304, "statusText": "Not Modified", "content": {"size": 0, "mimeType": "image/png"}}
      }
    ]
  }
}`

func mustParse(t *testing.T, s string) *HAR {
	t.Helper()
	h, err := Parse([]byte(s))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return h
}

func TestParse_Basic(t *testing.T) {
	h := mustParse(t, sampleHAR)
	if h.Version != "1.2" {
		t.Errorf("version = %q", h.Version)
	}
	if h.Creator.Name != "Firefox" || h.Creator.Version != "126.0" {
		t.Errorf("creator = %+v", h.Creator)
	}
	if len(h.Pages) != 1 || h.Pages[0].ID != "page_1" {
		t.Errorf("pages = %+v", h.Pages)
	}
	if len(h.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(h.Entries))
	}
	e0 := h.Entries[0]
	if e0.Request.Method != "GET" || e0.Request.URL != "https://example.com/app/main.js" {
		t.Errorf("entry0 request = %+v", e0.Request)
	}
	if e0.Response.Status != 200 || len(e0.Response.Headers) != 1 {
		t.Errorf("entry0 response = %+v", e0.Response)
	}
}

func TestParse_Errors(t *testing.T) {
	if _, err := Parse(nil); err != ErrNotHAR {
		t.Errorf("empty: err = %v, want ErrNotHAR", err)
	}
	if _, err := Parse([]byte("not json")); err == nil {
		t.Error("invalid json must error")
	}
	if _, err := Parse([]byte(`{"foo": "bar"}`)); err != ErrNotHAR {
		t.Errorf("non-HAR json: err = %v, want ErrNotHAR", err)
	}
	if _, err := Parse([]byte(`{"log":{"version":"1.2","entries":[]}}`)); err != nil {
		t.Errorf("empty-entries HAR must parse: %v", err)
	}
}

func TestParseReader(t *testing.T) {
	h, err := ParseReader(strings.NewReader(sampleHAR))
	if err != nil {
		t.Fatalf("ParseReader: %v", err)
	}
	if len(h.Entries) != 3 {
		t.Errorf("entries = %d", len(h.Entries))
	}
}

func TestDecodeBody(t *testing.T) {
	h := mustParse(t, sampleHAR)

	body, present, err := h.Entries[0].DecodeBody()
	if err != nil || !present || string(body) != "console.log" {
		t.Errorf("text body: %q present=%v err=%v", body, present, err)
	}

	body, present, err = h.Entries[1].DecodeBody()
	if err != nil || !present || string(body) != `{"a":1}` {
		t.Errorf("base64 body: %q present=%v err=%v", body, present, err)
	}

	_, present, err = h.Entries[2].DecodeBody()
	if present || err != nil {
		t.Errorf("empty body: present=%v err=%v, want present=false", present, err)
	}
}

func TestDecodeBody_BadBase64(t *testing.T) {
	e := Entry{}
	e.Response.Content.Encoding = "base64"
	e.Response.Content.Text = "!!!not base64!!!"
	_, present, err := e.DecodeBody()
	if !present || err == nil {
		t.Errorf("bad base64 must report present + error, got present=%v err=%v", present, err)
	}
}

func TestContentType(t *testing.T) {
	e := Entry{}
	e.Response.Content.MimeType = "text/css"
	e.Response.Headers = []Header{{Name: "Content-Type", Value: "text/html; charset=utf-8"}}
	if got := e.ContentType(); got != "text/css" {
		t.Errorf("ContentType = %q, want text/css", got)
	}
	e2 := Entry{}
	e2.Response.Headers = []Header{{Name: "content-type", Value: "text/html; charset=utf-8"}}
	if got := e2.ContentType(); got != "text/html" {
		t.Errorf("ContentType header fallback = %q, want text/html", got)
	}
}

func TestIsJavaScript(t *testing.T) {
	cases := []struct {
		mime, url string
		want      bool
	}{
		{"application/javascript", "https://x/a", true},
		{"text/ecmascript", "https://x/a", true},
		{"text/plain", "https://x/lib/app.js", true},
		{"text/plain", "https://x/lib/app.txt", false},
		{"image/png", "https://x/logo.png", false},
	}
	for _, c := range cases {
		e := Entry{}
		e.Response.Content.MimeType = c.mime
		e.Request.URL = c.url
		if got := e.IsJavaScript(); got != c.want {
			t.Errorf("IsJavaScript(%q,%q) = %v, want %v", c.mime, c.url, got, c.want)
		}
	}
}

func TestZipPath(t *testing.T) {
	cases := []struct {
		url, mime, want string
	}{
		{"https://example.com/app/main.js", "application/javascript", "example.com/app/main.js"},
		{"https://example.com/", "text/html", "example.com/index"},
		{"https://example.com/api", "application/javascript", "example.com/api.js"},
		{"https://example.com/data?x=1&y=2", "application/json", "example.com/data__x=1&y=2"},
		{"https://host:8443/p/f.css", "text/css", "host_8443/p/f.css"},
	}
	for _, c := range cases {
		if got := ZipPath(c.url, c.mime); got != c.want {
			t.Errorf("ZipPath(%q,%q) = %q, want %q", c.url, c.mime, got, c.want)
		}
	}
	if got := ZipPath("/relative/only", "text/plain"); !strings.HasPrefix(got, "_nohost/") {
		t.Errorf("hostless ZipPath = %q, want _nohost/ prefix", got)
	}
}

func TestResources(t *testing.T) {
	h := mustParse(t, sampleHAR)

	all := h.Resources(false)
	if len(all) != 2 {
		t.Fatalf("Resources(all) = %d, want 2", len(all))
	}
	if all[0].ZipPath != "example.com/app/main.js" || string(all[0].Bytes()) != "console.log" {
		t.Errorf("resource0 = %+v", all[0])
	}
	if all[1].ZipPath != "api.example.com/v1/data__x=1" || string(all[1].Bytes()) != `{"a":1}` {
		t.Errorf("resource1 = %+v", all[1])
	}

	js := h.Resources(true)
	if len(js) != 1 || js[0].ZipPath != "example.com/app/main.js" {
		t.Errorf("Resources(jsOnly) = %+v", js)
	}
}

func TestWriteZip_RoundTrip(t *testing.T) {
	h := mustParse(t, sampleHAR)
	var buf bytes.Buffer
	n, err := h.ExportAll(&buf)
	if err != nil {
		t.Fatalf("ExportAll: %v", err)
	}
	if n != 2 {
		t.Fatalf("wrote %d files, want 2", n)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	got := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		got[f.Name] = string(b)
	}
	if got["example.com/app/main.js"] != "console.log" {
		t.Errorf("main.js content = %q", got["example.com/app/main.js"])
	}
	if got["api.example.com/v1/data__x=1"] != `{"a":1}` {
		t.Errorf("data content = %q", got["api.example.com/v1/data__x=1"])
	}
}

func TestWriteZip_DedupesCollidingPaths(t *testing.T) {
	body := base64.StdEncoding.EncodeToString([]byte("x"))
	_ = body
	res := []Resource{
		{ZipPath: "h/a.js", Body: []byte("one")},
		{ZipPath: "h/a.js", Body: []byte("two")},
		{ZipPath: "h/a.js", Body: []byte("three")},
	}
	var buf bytes.Buffer
	n, err := WriteZip(&buf, res)
	if err != nil {
		t.Fatalf("WriteZip: %v", err)
	}
	if n != 3 {
		t.Fatalf("wrote %d, want 3", n)
	}
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, want := range []string{"h/a.js", "h/a-1.js", "h/a-2.js"} {
		if !names[want] {
			t.Errorf("missing deduped entry %q (got %v)", want, names)
		}
	}
}

func TestSummary(t *testing.T) {
	h := mustParse(t, sampleHAR)
	s := h.Summary()
	if s.EntryCount != 3 {
		t.Errorf("EntryCount = %d, want 3", s.EntryCount)
	}
	if s.ResourceCount != 2 {
		t.Errorf("ResourceCount = %d, want 2", s.ResourceCount)
	}
	if s.TotalBodyBytes != int64(len("console.log")+len(`{"a":1}`)) {
		t.Errorf("TotalBodyBytes = %d", s.TotalBodyBytes)
	}
	if s.PageCount != 1 {
		t.Errorf("PageCount = %d", s.PageCount)
	}
	if s.CreatorName != "Firefox" || s.BrowserVersion != "126.0" {
		t.Errorf("creator/browser = %+v", s)
	}
	if s.FirstStarted != "2024-01-01T10:00:00.100Z" || s.LastStarted != "2024-01-01T10:00:00.300Z" {
		t.Errorf("time range = %q..%q", s.FirstStarted, s.LastStarted)
	}
	if len(s.Methods) != 2 || s.Methods[0].Label != "GET" || s.Methods[0].Count != 2 {
		t.Errorf("Methods = %+v, want GET=2 first", s.Methods)
	}
	statusByLabel := map[string]int{}
	for _, c := range s.Statuses {
		statusByLabel[c.Label] = c.Count
	}
	if statusByLabel["200"] != 1 || statusByLabel["201"] != 1 || statusByLabel["304"] != 1 {
		t.Errorf("Statuses = %+v", s.Statuses)
	}
}

func TestMethods(t *testing.T) {
	h := mustParse(t, sampleHAR)
	got := h.Methods()
	if len(got) != 2 || got[0] != "GET" || got[1] != "POST" {
		t.Errorf("Methods = %v, want [GET POST]", got)
	}
}

func TestSortResourcesByPath(t *testing.T) {
	res := []Resource{{ZipPath: "z/b"}, {ZipPath: "a/a"}, {ZipPath: "m/c"}}
	got := sortResourcesByPath(res)
	if got[0].ZipPath != "a/a" || got[1].ZipPath != "m/c" || got[2].ZipPath != "z/b" {
		t.Errorf("sorted = %+v", got)
	}
	if res[0].ZipPath != "z/b" {
		t.Error("sortResourcesByPath mutated input")
	}
}

func TestPrettyCode_HTML(t *testing.T) {
	src := `<!DOCTYPE html><html><head><title>x</title></head><body><div class="a"><p>hi</p></div></body></html>`
	out, ok := PrettyCode([]byte(src), "text/html")
	if !ok {
		t.Fatal("minified HTML should beautify")
	}
	s := string(out)
	if !strings.Contains(s, "\n") {
		t.Fatalf("HTML must gain line breaks:\n%s", s)
	}
	if !strings.Contains(s, "<title>x</title>") {
		t.Errorf("short leaf element should be inline:\n%s", s)
	}
	if !strings.Contains(s, "<p>hi</p>") {
		t.Errorf("short leaf element should be inline:\n%s", s)
	}
	if !strings.Contains(s, "\n      <p>hi</p>") {
		t.Errorf("expected indentation for nested <p>:\n%s", s)
	}
}

func TestPrettyMarkup_VoidElements(t *testing.T) {
	src := `<body><img src="a.png"><br><input type="text"/><p>x</p></body>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	for _, want := range []string{"\n  <img src=\"a.png\">", "\n  <br>", "\n  <input type=\"text\"/>", "\n  <p>x</p>"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q at body depth (void elements must not nest):\n%s", want, s)
		}
	}
}

func TestPrettyMarkup_AttributesWithBrackets(t *testing.T) {
	src := `<div data-x="a>b" title='c<d'><p>hi</p></div>`
	out, _ := beautifyMarkup([]byte(src))
	if !strings.Contains(string(out), `<div data-x="a>b" title='c<d'>`) {
		t.Errorf("attribute with brackets was mis-parsed:\n%s", out)
	}
}

func TestPrettyMarkup_CommentsAndDoctype(t *testing.T) {
	src := `<!DOCTYPE html><div><!-- keep me --><p>x</p></div>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	if !strings.Contains(s, "<!DOCTYPE html>") {
		t.Errorf("doctype lost:\n%s", s)
	}
	if !strings.Contains(s, "<!-- keep me -->") {
		t.Errorf("comment lost:\n%s", s)
	}
}

func TestPrettyMarkup_EmbeddedScriptAndStyle(t *testing.T) {
	src := `<head><style>body{margin:0}</style><script>function f(){return 1;}</script></head>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	if !strings.Contains(s, "body{\n") || !strings.Contains(s, "margin:0") {
		t.Errorf("embedded CSS not beautified:\n%s", s)
	}
	if !strings.Contains(s, "function f(){\n") || !strings.Contains(s, "return 1;") {
		t.Errorf("embedded JS not beautified:\n%s", s)
	}
	if !strings.Contains(s, "</style>") || !strings.Contains(s, "</script>") {
		t.Errorf("raw-text element close tags lost:\n%s", s)
	}
}

func TestPrettyMarkup_CaseInsensitiveRawClose(t *testing.T) {
	src := `<SCRIPT>var a=1;</SCRIPT>`
	out, _ := beautifyMarkup([]byte(src))
	if !strings.Contains(string(out), "</SCRIPT>") {
		t.Errorf("upper-case </SCRIPT> not matched:\n%s", out)
	}
}

func TestPrettyCode_XML(t *testing.T) {
	src := `<?xml version="1.0"?><root><item id="1"><name>a</name></item></root>`
	out, ok := PrettyCode([]byte(src), "application/xml")
	if !ok {
		t.Fatal("minified XML should beautify")
	}
	s := string(out)
	if !strings.Contains(s, `<?xml version="1.0"?>`) {
		t.Errorf("XML declaration lost:\n%s", s)
	}
	if !strings.Contains(s, "<name>a</name>") {
		t.Errorf("leaf element should be inline:\n%s", s)
	}
}

func TestPrettyCode_SVGByMime(t *testing.T) {
	src := `<svg viewBox="0 0 1 1"><rect x="0" y="0"/></svg>`
	if _, ok := PrettyCode([]byte(src), "image/svg+xml"); !ok {
		t.Error("svg mime should route to markup beautifier")
	}
}

func TestPrettyCode_MarkupSniffWithoutMime(t *testing.T) {
	src := `<ul><li>a</li><li>b</li></ul>`
	if _, ok := PrettyCode([]byte(src), ""); !ok {
		t.Error("markup should be detected from content when mime is absent")
	}
	if out, ok := PrettyCode([]byte("a < b and c < d"), "text/plain"); ok {
		t.Errorf("prose with '<' must pass through, got:\n%s", out)
	}
}

func TestPrettyCode_AlreadyFormattedMarkupIsNoop(t *testing.T) {
	src := "<div>\n  <p>x</p>\n</div>"
	if out, ok := PrettyCode([]byte(src), "text/html"); ok {
		t.Errorf("already-formatted markup should report no change, got:\n%s", out)
	}
}

func TestPrettyCode_CSSRulesBreakOnNewSelector(t *testing.T) {
	src := `body{margin:0}.a,.b{color:red}@media(max-width:600px){.a{display:none}}`
	out, ok := PrettyCode([]byte(src), "text/css")
	if !ok {
		t.Fatal("minified CSS should beautify")
	}
	s := string(out)
	if strings.Contains(s, "}.a") {
		t.Errorf("new CSS selector glued to previous rule's '}':\n%s", s)
	}
	if !strings.Contains(s, "\n.a,.b{") {
		t.Errorf("expected '.a,.b' selector on its own line:\n%s", s)
	}
}

func TestPrettyCode_JSON(t *testing.T) {
	out, ok := PrettyCode([]byte(`{"a":1,"b":[2,3]}`), "application/json")
	if !ok || !strings.Contains(string(out), "\n") {
		t.Fatalf("JSON should prettify: ok=%v out=%q", ok, out)
	}
}

func TestPrettyCode_MinifiedJS(t *testing.T) {
	src := `function f(a){if(a){return a+1;}else{return 0;}}var x={k:1,j:2};`
	out, ok := PrettyCode([]byte(src), "application/javascript")
	if !ok {
		t.Fatalf("minified JS should beautify")
	}
	s := string(out)
	if !strings.Contains(s, "\n") {
		t.Fatalf("beautified JS must contain newlines:\n%s", s)
	}
	if !strings.Contains(s, "\n  ") {
		t.Errorf("expected indentation in:\n%s", s)
	}
	t.Logf("beautified:\n%s", s)
}

func TestBeautify_DoesNotBreakStrings(t *testing.T) {
	src := `var s="a;b{c}d";f();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `"a;b{c}d"`) {
		t.Errorf("string literal was mangled:\n%s", out)
	}
}

func TestBeautify_DoesNotBreakRegex(t *testing.T) {
	src := `var re=/a;b{2}/g;f();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `/a;b{2}/g`) {
		t.Errorf("regex literal was mangled:\n%s", out)
	}
}

func TestBeautify_DoesNotBreakComments(t *testing.T) {
	src := `a();/* x;y{z} */b();// trailing;{}
c();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `/* x;y{z} */`) {
		t.Errorf("block comment mangled:\n%s", out)
	}
	if !strings.Contains(string(out), `// trailing;{}`) {
		t.Errorf("line comment mangled:\n%s", out)
	}
}

func TestBeautify_TemplateLiteral(t *testing.T) {
	src := "var t=`a;b{c}`;f();"
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "`a;b{c}`") {
		t.Errorf("template literal mangled:\n%s", out)
	}
}

func TestBeautify_BalancedBraces(t *testing.T) {
	src := `if(x){a();if(y){b();}}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	if strings.Count(s, "{") != strings.Count(src, "{") || strings.Count(s, "}") != strings.Count(src, "}") {
		t.Errorf("brace count changed:\n%s", s)
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	last := lines[len(lines)-1]
	if last != "}" {
		t.Errorf("last line should be a dedented '}', got %q\nfull:\n%s", last, s)
	}
}

func TestBeautify_ForHeaderStaysOnOneLine(t *testing.T) {
	src := `function f(){for(let e=0;e<n;e++)g(e);for(;x;)h();}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	if !strings.Contains(s, "for(let e=0; e<n; e++)") {
		t.Errorf("for header was split:\n%s", s)
	}
	if !strings.Contains(s, "for(; x;)") {
		t.Errorf("empty-init for header was split:\n%s", s)
	}
	if !strings.Contains(s, "g(e);\n") {
		t.Errorf("statement after for did not break:\n%s", s)
	}
}

func TestBeautify_ElseCatchFinallyStayAttached(t *testing.T) {
	src := `function f(){try{a();}catch{b();}finally{c();}if(x){d();}else{e();}}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	for _, want := range []string{"} catch{", "} finally{", "} else{"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q to stay on the closing-brace line:\n%s", want, s)
		}
	}
	if strings.Contains(s, "}\nelse") || strings.Contains(s, "}\ncatch") || strings.Contains(s, "}\nfinally") {
		t.Errorf("else/catch/finally dangled onto its own line:\n%s", s)
	}
}

func TestBeautify_DoWhileStaysAttached(t *testing.T) {
	src := `function f(){do{a();}while(x);}`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "} while(x)") {
		t.Errorf("do/while tail dangled:\n%s", out)
	}
}

func TestBeautify_DestructuringAssignmentNotSplit(t *testing.T) {
	src := `function f(r){let{body:e,...t}=JSON.parse(r);return e;}`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "}=JSON.parse(r)") {
		t.Errorf("destructuring `}=` was split:\n%s", out)
	}
}

func TestPrettyCode_SvelteKitBundle(t *testing.T) {
	src := "import{a as e}from\"./x.js\";var c=class{constructor(e){this.s=e}" +
		"toString(){return JSON.stringify(this.s)}};function g(...e){let t=5381;" +
		"for(let n of e)if(typeof n==`string`){let e=n.length;for(;e;)t=t*33^n.charCodeAt(--e)}" +
		"else throw TypeError(`bad`);return(t>>>0).toString(36)}"
	out, ok := PrettyCode([]byte(src), "application/javascript")
	if !ok {
		t.Fatal("minified bundle should beautify")
	}
	s := string(out)
	if strings.Count(s, "{") != strings.Count(src, "{") || strings.Count(s, "}") != strings.Count(src, "}") {
		t.Errorf("brace count changed:\n%s", s)
	}
	if !strings.Contains(s, "for(; e;)") {
		t.Errorf("for header inside bundle was split:\n%s", s)
	}
	if !strings.Contains(s, "} else throw") {
		t.Errorf("else clause dangled in bundle:\n%s", s)
	}
}

func TestPrettyCode_PlainTextUnchanged(t *testing.T) {
	if out, ok := PrettyCode([]byte("just some words here"), "text/plain"); ok || string(out) != "just some words here" {
		t.Errorf("plain text must pass through, got ok=%v out=%q", ok, out)
	}
}

func TestLooksLikeBraceCode(t *testing.T) {
	if !looksLikeBraceCode([]byte("x"), "application/javascript") {
		t.Error("javascript mime should be code")
	}
	if !looksLikeBraceCode([]byte("a{b:1}"), "text/css") {
		t.Error("css mime should be code")
	}
	if !looksLikeBraceCode([]byte(`a();b();c();d={x:1};e();f();`), "text/plain") {
		t.Error("minified single line should be detected")
	}
	if looksLikeBraceCode([]byte("the quick brown fox\njumps over\nthe lazy dog"), "text/plain") {
		t.Error("prose should not be code")
	}
}

func TestIndexFromFoldNonASCIIOffsets(t *testing.T) {
	cases := []struct {
		src  string
		from int
		sep  string
		want int
	}{
		{"AİB</SCRIPT>", 0, "</script", 4},
		{"</SCRIPT>", 0, "</script", 0},
		{"abc</Script>", 0, "</script", 3},
		{"İİİ</script>", 0, "</script", 6},
		{"no match here", 0, "</script", -1},
		{"", 0, "</script", -1},
		{"prefix</SCRIPT>", 3, "</script", 6},
	}
	for _, c := range cases {
		got := indexFromFold([]byte(c.src), c.from, []byte(c.sep))
		if got != c.want {
			t.Errorf("indexFromFold(%q, %d, %q) = %d, want %d", c.src, c.from, c.sep, got, c.want)
		}
		if got >= 0 {
			slice := string([]byte(c.src)[got:])
			if !bytes.HasPrefix(bytes.ToLower([]byte(slice)), bytes.ToLower([]byte(c.sep))) {
				t.Errorf("returned offset %d does not align with %q in %q (got %q)", got, c.sep, c.src, slice)
			}
		}
	}
}

func TestIndexFromFoldFromPastEnd(t *testing.T) {
	src := []byte("abc")
	for _, from := range []int{3, 4, 99} {
		if got := indexFromFold(src, from, []byte("a")); got != -1 {
			t.Errorf("indexFromFold(%q, %d, \"a\") = %d, want -1", src, from, got)
		}
	}
}

func TestContentTypeStripsParametersFromMimeType(t *testing.T) {
	cases := []struct {
		name string
		mime string
		want string
	}{
		{"chrome style", "text/html; charset=UTF-8", "text/html"},
		{"no space", "application/json;charset=utf-8", "application/json"},
		{"padded", "  image/png  ", "image/png"},
		{"bare semicolon", "text/plain;", "text/plain"},
		{"params only", "; charset=utf-8", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var e Entry
			e.Response.Content.MimeType = c.mime
			if got := e.ContentType(); got != c.want {
				t.Errorf("ContentType() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSummaryMergesMimeTypeAndHeaderSpellings(t *testing.T) {
	const doc = `{"log":{"version":"1.2","entries":[
	  {"request":{"method":"GET","url":"https://x/a"},
	   "response":{"status":200,"content":{"mimeType":"text/html; charset=utf-8","text":"a"}}},
	  {"request":{"method":"GET","url":"https://x/b"},
	   "response":{"status":200,"headers":[{"name":"Content-Type","value":"text/html; charset=utf-8"}],
	               "content":{"text":"b"}}}
	]}}`
	h, err := Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := h.Summary().MimeTypes
	if len(got) != 1 || got[0].Label != "text/html" || got[0].Count != 2 {
		t.Errorf("MimeTypes = %+v, want one text/html bucket of 2: charset parameters must not split buckets", got)
	}
}

func TestPrettyCodeScriptWithUnicodeNotTruncated(t *testing.T) {
	body := []byte("<div>\n<script>var s = \"İ\";</script>\n</div>")
	out, ok := PrettyCode(body, "text/html")
	if !ok {
		t.Skip("PrettyCode declined to format this input")
	}
	if !bytes.Contains(out, []byte("\";</script>")) && !bytes.Contains(out, []byte("\";")) {
		t.Errorf("script body appears truncated around the unicode char:\n%s", out)
	}
}

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
