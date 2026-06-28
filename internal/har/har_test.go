package har

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

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
	if all[0].ZipPath != "example.com/app/main.js" || string(all[0].Body) != "console.log" {
		t.Errorf("resource0 = %+v", all[0])
	}
	if all[1].ZipPath != "api.example.com/v1/data__x=1" || string(all[1].Body) != `{"a":1}` {
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
