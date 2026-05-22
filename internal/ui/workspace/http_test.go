package workspace

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/widgets"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/widget/material"
)

func TestCancelRequest(t *testing.T) {
	tab := &RequestTab{}
	called := false
	tab.cancelFn = func() { called = true }

	tab.CancelRequest()
	if !called {
		t.Errorf("expected cancelFn to be called")
	}
	if tab.cancelFn != nil {
		t.Errorf("expected cancelFn to be nil")
	}
}

func TestCleanupRespFile(t *testing.T) {
	tab := &RequestTab{}
	tmp, _ := os.CreateTemp("", "test")
	_ = tmp.Close()

	tab.respFile = tmp.Name()

	win := new(app.Window)
	widgets.ArmInvalidateTimer(&tab.reqWidthTimer, win, 1*time.Minute)
	widgets.ArmInvalidateTimer(&tab.respWidthTimer, win, 1*time.Minute)

	tab.cleanupRespFile()
	if tab.respFile != "" {
		t.Errorf("expected respFile to be cleared")
	}
	if _, err := os.Stat(tmp.Name()); !os.IsNotExist(err) {
		t.Errorf("expected file to be deleted")
	}
	if tab.reqWidthTimer != nil || tab.respWidthTimer != nil {
		t.Errorf("expected timers to be stopped and cleared")
	}
}

func TestPrepareRequest(t *testing.T) {
	tab := NewRequestTab("test")
	tab.Method = "POST"
	tab.URLInput.SetText("{{host}}/api")
	tab.ReqEditor.SetText("{\"key\": \"{{val}}\"} // comment")

	tab.AddHeader("Auth", "Bearer {{token}}")

	env := map[string]string{
		"host":  "example.com",
		"val":   "123",
		"token": "secret",
	}

	req, ctx, cancel, err := tab.prepareRequest(context.Background(), env)
	if err != nil {
		t.Fatalf("prepareRequest error: %v", err)
	}
	defer cancel()

	if req.Method != "POST" {
		t.Errorf("expected POST, got %s", req.Method)
	}
	if req.URL.String() != "http://example.com/api" {
		t.Errorf("expected http://example.com/api, got %s", req.URL.String())
	}
	if req.Header.Get("Auth") != "Bearer secret" {
		t.Errorf("expected auth header, got %s", req.Header.Get("Auth"))
	}

	buf := make([]byte, 100)
	n, _ := req.Body.Read(buf)
	bodyStr := string(buf[:n])
	if bodyStr != "{\"key\": \"123\"} " {
		t.Errorf("expected body without comment and templated, got %q", bodyStr)
	}

	if ctx == nil {
		t.Errorf("expected context")
	}
}

func TestPrepareRequest_EmptyURL(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	_, _, _, err := tab.prepareRequest(context.Background(), nil)
	if err == nil {
		t.Errorf("expected error for empty URL")
	}
}

func TestExecuteRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = true
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout waiting for response")
	}
}

func TestExecuteRequestToFile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`file content`))
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.PreviewEnabled = false
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"

	tmp, _ := os.CreateTemp("", "save-target")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	tab.SaveToFilePath = tmpPath
	tab.beginRequest()

	win := new(app.Window)
	tab.ExecuteRequestToFile(context.Background(), win, nil, tmp)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "200 OK") {
			t.Errorf("expected 200 OK, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}

	data, _ := os.ReadFile(tmpPath)
	if string(data) != "file content" {
		t.Errorf("file content mismatch")
	}
}

func TestExecuteRequest_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	tab := NewRequestTab("test")
	tab.URLInput.SetText(srv.URL)
	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)

	select {
	case res := <-tab.responseChan:
		if !strings.HasPrefix(res.status, "404 Not Found") {
			t.Errorf("expected 404, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

func TestExecuteRequest_PrepareError(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("   ")
	win := new(app.Window)
	tab.ExecuteRequest(context.Background(), win, nil)
	if !strings.HasPrefix(tab.Status, "Error") {
		t.Errorf("expected Error status, got %s", tab.Status)
	}
}

func TestSendResponse_DeliversOnCanceledContext(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(5)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !tab.sendResponse(ctx, tabResponse{requestID: 5, status: "Cancelled"}) {
		t.Fatalf("sendResponse returned false even though responseChan was empty")
	}

	select {
	case got := <-tab.responseChan:
		if got.status != "Cancelled" {
			t.Fatalf("expected status Cancelled, got %q", got.status)
		}
	default:
		t.Fatalf("responseChan was empty — Cancelled status was dropped")
	}
}

func TestSendResponse_StaleID(t *testing.T) {
	tab := NewRequestTab("test")
	tab.requestID.Store(10)

	tab.sendResponse(context.Background(), tabResponse{requestID: 9, status: "Stale"})

	th := material.NewTheme()
	gtx := layout.Context{Ops: new(op.Ops)}
	tab.Layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	if tab.Status == "Stale" {
		t.Errorf("stale response should be ignored")
	}

	tab.sendResponse(context.Background(), tabResponse{requestID: 10, status: "Fresh"})
	tab.Layout(gtx, th, new(app.Window), nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	if !strings.Contains(tab.Status, "Fresh") {
		t.Errorf("fresh response should be accepted, got %s", tab.Status)
	}
}

func TestExecuteRequestToFile_Error(t *testing.T) {
	tab := NewRequestTab("test")
	tab.URLInput.SetText("http://localhost:1")
	failWriter := &failingWriteCloser{}
	tab.ExecuteRequestToFile(context.Background(), new(app.Window), nil, failWriter)

	select {
	case res := <-tab.responseChan:
		if !strings.Contains(res.status, "Error") {
			t.Errorf("expected Error status, got %s", res.status)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

type failingWriteCloser struct{}

func (f *failingWriteCloser) Write(p []byte) (n int, err error) { return 0, io.ErrClosedPipe }
func (f *failingWriteCloser) Close() error                      { return nil }

func TestStreamResponse_Cancellation(t *testing.T) {
	tab := NewRequestTab("test")
	ctx, cancel := context.WithCancel(context.Background())
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("start"))
		time.Sleep(100 * time.Millisecond)
		cancel()
		_ = pw.Close()
	}()
	var dest bytes.Buffer
	_, err := tab.streamResponse(ctx, pr, &dest, new(app.Window), true, "")
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestLoadPreviewForSavedFile(t *testing.T) {
	setupTestConfigDir(t)
	tmp, _ := os.CreateTemp("", "resp")
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	content := `{"foo": "bar"}`
	_ = os.WriteFile(tmpPath, []byte(content), 0644)

	tab := NewRequestTab("test")
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.window = new(app.Window)
	tab.loadPreviewForSavedFile()

	select {
	case res := <-tab.previewChan:
		if res.body == "" {
			t.Errorf("expected body loaded")
		}
	case <-time.After(1 * time.Second):
		t.Errorf("timeout")
	}
}

func TestDecompressBody_Gzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "text/html")
		gz := gzip.NewWriter(w)
		_, _ = gz.Write([]byte("<html>hello</html>"))
		_ = gz.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.Uncompressed {
		t.Fatalf("expected Uncompressed=false (manual Accept-Encoding)")
	}

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "<html>hello</html>" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Deflate_Zlib(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "deflate")
		zw := zlib.NewWriter(w)
		_, _ = zw.Write([]byte("plain text"))
		_ = zw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "deflate")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, _ := io.ReadAll(body)
	if string(data) != "plain text" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Identity(t *testing.T) {
	resp := &http.Response{
		Body:   io.NopCloser(strings.NewReader("raw")),
		Header: http.Header{},
	}
	resp.Header.Set("Content-Encoding", "identity")
	body := decompressBody(resp)
	data, _ := io.ReadAll(body)
	if string(data) != "raw" {
		t.Errorf("got %q", string(data))
	}
}

func TestStreamResponse_SniffHTMLMeta(t *testing.T) {
	// CP1251 "Привет" inside an HTML doc with <meta charset="windows-1251">.
	// Content-Type carries no charset, so streamResponse must sniff.
	cp1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	body := append([]byte(`<html><head><meta charset="windows-1251"></head><body>`), cp1251...)
	body = append(body, []byte(`</body></html>`)...)

	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), bytes.NewReader(body), &dest, new(app.Window), true, "text/html")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk)
		default:
			if !strings.Contains(got.String(), "Привет") {
				t.Errorf("sniffed preview missing cyrillic; got %q", got.String())
			}
			return
		}
	}
}

func TestStreamResponse_SniffBOM_UTF16LE(t *testing.T) {
	// UTF-16LE BOM + "hello"
	body := []byte{0xFF, 0xFE,
		'h', 0x00, 'e', 0x00, 'l', 0x00, 'l', 0x00, 'o', 0x00,
	}

	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), bytes.NewReader(body), &dest, new(app.Window), true, "text/plain")
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk)
		default:
			if got.String() != "hello" {
				t.Errorf("got %q, want %q", got.String(), "hello")
			}
			return
		}
	}
}

func TestStreamResponse_PlainUTF8NoSniff(t *testing.T) {
	body := []byte("plain utf-8 — ok")
	tab := NewRequestTab("test")
	var dest bytes.Buffer
	_, err := tab.streamResponse(context.Background(), bytes.NewReader(body), &dest, new(app.Window), true, "text/plain")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var got strings.Builder
	for {
		select {
		case chunk := <-tab.appendChan:
			got.WriteString(chunk)
		default:
			if got.String() != string(body) {
				t.Errorf("got %q, want %q", got.String(), string(body))
			}
			return
		}
	}
}

func TestBuildCurlCommand_GET(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users/42")
	tab.Method = "GET"
	tab.Headers = nil
	got := BuildCurlCommand(tab, nil)
	if !strings.HasPrefix(got, "curl 'https://api.example.com/users/42'") {
		t.Errorf("missing URL prefix: %s", got)
	}
	if strings.Contains(got, "-X GET") {
		t.Errorf("should omit -X for GET: %s", got)
	}
}

func TestBuildCurlCommand_POSTJSON(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://api.example.com/users")
	tab.Method = "POST"
	tab.Headers = nil
	tab.AddHeader("Content-Type", "application/json")
	tab.ReqEditor.SetText(`{"name":"a"}`)
	got := BuildCurlCommand(tab, nil)
	if !strings.Contains(got, "-X POST") {
		t.Errorf("missing -X POST: %s", got)
	}
	if !strings.Contains(got, "-H 'Content-Type: application/json'") {
		t.Errorf("missing content-type header: %s", got)
	}
	if !strings.Contains(got, `--data-raw '{"name":"a"}'`) {
		t.Errorf("missing body: %s", got)
	}
}

func TestBuildCurlCommand_QuoteEscape(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("https://example.com/")
	tab.Method = "POST"
	tab.Headers = nil
	tab.ReqEditor.SetText(`it's "quoted"`)
	got := BuildCurlCommand(tab, nil)
	if !strings.Contains(got, `--data-raw 'it'\''s "quoted"'`) {
		t.Errorf("single-quote escape broken: %s", got)
	}
}

func TestBuildCurlCommand_TemplateSubstitution(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("{{base}}/items")
	tab.Method = "GET"
	tab.Headers = nil
	got := BuildCurlCommand(tab, map[string]string{"base": "https://api.example.com"})
	if !strings.Contains(got, "'https://api.example.com/items'") {
		t.Errorf("template not substituted: %s", got)
	}
}

func TestBuildCurlCommand_EmptyURL(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("")
	if got := BuildCurlCommand(tab, nil); got != "" {
		t.Errorf("expected empty cURL for empty URL, got %q", got)
	}
}

func TestFormatTimings(t *testing.T) {
	if got := formatTimings(Timings{}); got != "" {
		t.Errorf("zero timings should produce empty string, got %q", got)
	}
	tm := Timings{DNS: 10 * time.Millisecond, TTFB: 100 * time.Millisecond}
	got := formatTimings(tm)
	if !strings.Contains(got, "DNS 10ms") {
		t.Errorf("missing DNS: %s", got)
	}
	if !strings.Contains(got, "TTFB 100ms") {
		t.Errorf("missing TTFB: %s", got)
	}
	if strings.Contains(got, "Connect") {
		t.Errorf("should skip zero Connect phase: %s", got)
	}
}

func TestExecuteRequest_CapturesTimingsAndFilename(t *testing.T) {
	setupTestConfigDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Small delay so TTFB exceeds Windows time.Now() granularity.
		time.Sleep(5 * time.Millisecond)
		w.Header().Set("Content-Disposition", `attachment; filename="hello.txt"`)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.Method = "GET"
	tab.window = new(app.Window)
	tab.ExecuteRequest(context.Background(), tab.window, nil)

	select {
	case res := <-tab.responseChan:
		if res.filename != "hello.txt" {
			t.Errorf("filename got %q want hello.txt", res.filename)
		}
		if res.timings.TTFB <= 0 {
			t.Errorf("expected positive TTFB, got %v", res.timings.TTFB)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestDecompressBody_Brotli(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		bw := brotli.NewWriter(w)
		_, _ = bw.Write([]byte("brotli payload"))
		_ = bw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "br")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "brotli payload" {
		t.Errorf("got %q", string(data))
	}
}

func TestDecompressBody_Zstd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "zstd")
		zw, _ := zstd.NewWriter(w)
		_, _ = zw.Write([]byte("zstd payload"))
		_ = zw.Close()
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	req.Header.Set("Accept-Encoding", "zstd")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body := decompressBody(resp)
	defer func() { _ = body.Close() }()
	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "zstd payload" {
		t.Errorf("got %q", string(data))
	}
}
