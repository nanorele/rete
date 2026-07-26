package har

import (
	"bytes"
	"errors"
	"math/rand"
	"os"
	"strings"
	"testing"
)

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
	if len(res) != 1 || len(res[0].Body) != 1<<20 {
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
