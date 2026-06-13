package workspace

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/settings"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget"
)

func TestProcessTemplate_EdgeCases(t *testing.T) {
	env := map[string]string{
		"a":     "X",
		"":      "EMPTY",
		"long":  strings.Repeat("v", 256),
		"multi": "L1\nL2",
	}
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty input empty env", "", "Y"[:0]},
		{"two templates", "{{a}}-{{a}}", "X-X"},
		{"empty key with empty env entry", "{{}}", "EMPTY"},
		{"spaces-only key", "{{   }}", "EMPTY"},
		{"value with newline", "X={{multi}}", "X=L1\nL2"},
		{"long value", "[{{long}}]", "[" + strings.Repeat("v", 256) + "]"},
		{"leading unmatched brace", "}}{{a}}", "}}X"},
		{"empty env map non-nil", "{{a}}", "{{a}}"},
		{"only braces no end", "{{", "{{"},
		{"close before open", "}}{{", "}}{{"},
		{"template with tab inside", "{{\ta\t}}", "X"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.name == "empty env map non-nil" {
				got = processTemplate(tc.in, map[string]string{})
			} else {
				got = processTemplate(tc.in, env)
			}
			if got != tc.want {
				t.Errorf("processTemplate(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	if got := processTemplate("{{a}}", nil); got != "{{a}}" {
		t.Errorf("nil env should pass through, got %q", got)
	}
	if got := processTemplate("no templates here", env); got != "no templates here" {
		t.Errorf("no '{{' should pass through, got %q", got)
	}
}

func TestFormatSize_Boundaries(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1025, "1.0 KB"},
		{1<<20 - 1, "1024.0 KB"},
		{1 << 20, "1.0 MB"},
		{1<<30 - 1, "1024.0 MB"},
		{1 << 30, "1.00 GB"},
		{2 << 30, "2.00 GB"},
	}
	for _, tc := range cases {
		if got := formatSize(tc.in); got != tc.want {
			t.Errorf("formatSize(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := formatSize(-1); got != "0 B" {
		t.Errorf("formatSize(-1) must clamp to 0 B (no negative size), got %q", got)
	}
	if got := formatSize(-9999); got != "0 B" {
		t.Errorf("formatSize(-9999) must clamp to 0 B, got %q", got)
	}
}

func TestTrimTrailingWhitespace(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"abc", "abc"},
		{"abc   ", "abc"},
		{"a\nb  \nc\t\t", "a\nb\nc"},
		{"  ", ""},
		{"line1\r\nline2  \r", "line1\nline2"},
		{"   \n   \n", "\n\n"},
	}
	for _, tc := range cases {
		if got := trimTrailingWhitespace(tc.in); got != tc.want {
			t.Errorf("trimTrailingWhitespace(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLooksLikeJSON_BOM(t *testing.T) {
	if looksLikeJSON([]byte("\xEF\xBB\xBF{\"a\":1}")) {
		t.Logf("looksLikeJSON treats BOM as non-JSON (expected)")
	}

	if !looksLikeJSON([]byte("\n\n\n[")) {
		t.Errorf("expected leading newlines to be skipped")
	}
	if looksLikeJSON(nil) {
		t.Errorf("nil should be false")
	}
}

func TestLoadPreviewFromFile_Missing(t *testing.T) {
	result, n, isJSON := loadPreviewFromFile(filepath.Join(t.TempDir(), "nope"), 100, &JSONFormatterState{}, "")
	if result != "" || n != 0 || isJSON {
		t.Errorf("expected zero values on missing file, got (%q,%d,%v)", result, n, isJSON)
	}
}

func TestLoadPreviewFromFile_EmptyFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview-empty")
	_ = tmp.Close()
	defer os.Remove(tmp.Name())

	result, n, isJSON := loadPreviewFromFile(tmp.Name(), 0, &JSONFormatterState{}, "")
	if result != "" || n != 0 || isJSON {
		t.Errorf("empty file should return zero values, got (%q,%d,%v)", result, n, isJSON)
	}
}

func TestLoadPreviewFromFile_AutoFormatDisabled(t *testing.T) {
	prev := settings.AutoFormatJSON
	settings.AutoFormatJSON = false
	defer func() { settings.AutoFormatJSON = prev }()

	tmp, _ := os.CreateTemp("", "preview-noauto")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	body := `{"a":1}`
	_ = os.WriteFile(tmpPath, []byte(body), 0644)

	result, _, isJSON := loadPreviewFromFile(tmpPath, int64(len(body)), &JSONFormatterState{}, "")
	if isJSON {
		t.Errorf("with AutoFormatJSON=false, isJSON should be false")
	}
	if result != body {
		t.Errorf("expected unformatted body, got %q", result)
	}
}

func TestGetPreviewBuf(t *testing.T) {
	buf, release := getPreviewBuf(0)
	if len(buf) != 0 {
		t.Errorf("expected len 0, got %d", len(buf))
	}
	release()

	buf, release = getPreviewBuf(100)
	if len(buf) != 100 {
		t.Errorf("expected len 100, got %d", len(buf))
	}
	release()

	huge := int64(previewBatchSize + 10)
	buf, release = getPreviewBuf(huge)
	if int64(len(buf)) != huge {
		t.Errorf("expected len %d for over-batch, got %d", huge, len(buf))
	}
	release()
}

func TestBuildBody_None(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyNone
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil reader")
	}
	if ct != "" {
		t.Errorf("expected empty content-type, got %q", ct)
	}
}

func TestBuildBody_URLEncoded(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.URLEncoded = []*URLEncodedPart{
		NewURLEncodedPart("k1", "v1"),
		NewURLEncodedPart("k2", "{{var}}"),
		NewURLEncodedPart(" ", "blank-key-skip"),
		NewURLEncodedPart("", "also-skip"),
	}
	env := map[string]string{"var": "VAL"}
	r, ct, err := tab.buildBody(context.Background(), env)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ct != "application/x-www-form-urlencoded" {
		t.Errorf("ct = %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "k1=v1") || !strings.Contains(got, "k2=VAL") {
		t.Errorf("body = %q", got)
	}
	if strings.Contains(got, "blank") || strings.Contains(got, "also-skip") {
		t.Errorf("body should not include blank-key parts: %q", got)
	}
}

func TestBuildBody_Binary_NoPath(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = ""
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected err for no file selected")
	}
	if r != nil || ct != "" {
		t.Errorf("expected zero values on err")
	}
}

func TestBuildBody_Binary_MissingFile(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = filepath.Join(t.TempDir(), "does-not-exist")
	_, _, err := tab.buildBody(context.Background(), nil)
	if err == nil {
		t.Errorf("expected err for missing file")
	}
}

func TestBuildBody_Binary_OK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	_ = os.WriteFile(path, []byte("hello"), 0644)
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.BinaryFilePath = path
	r, ct, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer r.(io.Closer).Close()
	if ct != "application/octet-stream" {
		t.Errorf("unexpected ct: %q", ct)
	}
	data, _ := io.ReadAll(r)
	if string(data) != "hello" {
		t.Errorf("body = %q", string(data))
	}
}

func TestBuildBody_Raw_TemplatedAndStripped(t *testing.T) {
	prevStrip := settings.StripJSONComments
	settings.StripJSONComments = true
	defer func() { settings.StripJSONComments = prevStrip }()

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.ReqEditor.SetText(`{"k":"{{v}}"} // trailing comment`)
	r, ct, err := tab.buildBody(context.Background(), map[string]string{"v": "VAL"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ct != "" {
		t.Errorf("raw should not set explicit content-type, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if strings.Contains(got, "//") {
		t.Errorf("expected comment stripped, got %q", got)
	}
	if !strings.Contains(got, `"VAL"`) {
		t.Errorf("expected template substitution, got %q", got)
	}
}

func TestBuildBody_FormData_Text(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("name", "{{val}}", model.FormPartText, "", 0),
		NewFormPart("", "skipped", model.FormPartText, "", 0),
	}
	r, ct, err := tab.buildBody(context.Background(), map[string]string{"val": "ALICE"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.HasPrefix(ct, "multipart/form-data") {
		t.Errorf("expected multipart, got %q", ct)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "ALICE") || !strings.Contains(got, `name="name"`) {
		t.Errorf("multipart body missing field: %q", got)
	}
	if strings.Contains(got, "skipped") {
		t.Errorf("blank-key form part should be skipped")
	}
}

func TestBuildBody_FormData_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "up.txt")
	_ = os.WriteFile(path, []byte("FILE-DATA"), 0644)

	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("upload", "", model.FormPartFile, path, 9),
		NewFormPart("empty-file", "", model.FormPartFile, "", 0),
	}
	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	data, _ := io.ReadAll(r)
	got := string(data)
	if !strings.Contains(got, "FILE-DATA") || !strings.Contains(got, "up.txt") {
		t.Errorf("file part missing: %q", got)
	}
}

func TestBuildBody_FormData_FileMissing(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("upload", "", model.FormPartFile, filepath.Join(t.TempDir(), "nope"), 0),
	}
	r, _, err := tab.buildBody(context.Background(), nil)
	if err != nil {
		t.Fatalf("buildBody itself shouldn't fail synchronously: %v", err)
	}

	_, readErr := io.ReadAll(r)
	if readErr == nil {
		t.Errorf("expected pipe error from missing file")
	}
}

func TestCleanupOrphanRespTmp_NoPanic(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("TMPDIR", dir)
	t.Setenv("TMP", dir)
	t.Setenv("TEMP", dir)

	old := filepath.Join(dir, "tracto-resp-old.tmp")
	fresh := filepath.Join(dir, "tracto-resp-fresh.tmp")
	other := filepath.Join(dir, "not-tracto.tmp")
	_ = os.WriteFile(old, []byte("x"), 0644)
	_ = os.WriteFile(fresh, []byte("x"), 0644)
	_ = os.WriteFile(other, []byte("x"), 0644)
	past := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(old, past, past)

	CleanupOrphanRespTmp()

	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("expected old file removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("expected fresh file kept: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("expected unrelated file kept: %v", err)
	}
}

func TestUpdateSystemHeaders_UserOverrideCaseInsensitive(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.AddHeader("content-type", "x/custom")
	tab.UpdateSystemHeaders()

	count := 0
	for _, h := range tab.Headers {
		if strings.EqualFold(h.Key.Text(), "Content-Type") {
			count++
			if h.IsGenerated {
				t.Errorf("user header should remain non-generated")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Content-Type header, got %d", count)
	}
}

func TestUpdateSystemHeaders_GeneratedToManualOnEdit(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.UpdateSystemHeaders()

	var ct *HeaderItem
	for _, h := range tab.Headers {
		if h.IsGenerated && strings.EqualFold(h.Key.Text(), "Content-Type") {
			ct = h
			break
		}
	}
	if ct == nil {
		t.Fatalf("no generated Content-Type")
	}
	ct.Value.SetText("application/edited")
	tab.UpdateSystemHeaders()

	if ct.IsGenerated {
		t.Errorf("edited generated header should switch to manual")
	}
}

func TestUpdateSystemHeaders_BodyTypeSwitch(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyURLEncoded
	tab.UpdateSystemHeaders()

	found := false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/x-www-form-urlencoded" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected generated urlencoded Content-Type")
	}

	tab.BodyType = model.BodyBinary
	tab.UpdateSystemHeaders()
	found = false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/octet-stream" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected generated octet-stream after switch")
	}

	tab.BodyType = model.BodyNone
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" {
			t.Errorf("BodyNone should have no generated Content-Type, got %q", h.Value.Text())
		}
	}
}

func TestUpdateSystemHeaders_FormDataNoStaleContentType(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyBinary
	tab.UpdateSystemHeaders()

	tab.BodyType = model.BodyFormData
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" {
			t.Errorf("form-data should not auto-set Content-Type (boundary added at send time), got %q", h.Value.Text())
		}
	}
}

func TestUpdateSystemHeaders_RawJSONDetection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.BodyType = model.BodyRaw
	tab.ReqEditor.SetText(`   {"x":1}`)
	tab.UpdateSystemHeaders()

	found := false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			found = true
		}
	}
	if !found {
		t.Errorf("JSON body should be detected as application/json")
	}

	tab.ReqEditor.SetText("\xEF\xBB\xBF[1,2,3]")
	tab.UpdateSystemHeaders()
	found = false
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			found = true
		}
	}
	if !found {
		t.Errorf("BOM-prefixed JSON should still detect as application/json")
	}

	tab.ReqEditor.SetText("hello world")
	tab.UpdateSystemHeaders()
	for _, h := range tab.Headers {
		if h.IsGenerated && h.Key.Text() == "Content-Type" && h.Value.Text() == "application/json" {
			t.Errorf("plain text should not be detected as JSON")
		}
	}
}

func TestPerformSearch_RegexSpecialChars(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("price: $5.00 + $3.00 = $8.00")
	tab.invalidateSearchCache()
	tab.SearchEditor.SetText("$5.00")
	tab.performSearch()
	if len(tab.searchResults) != 1 {
		t.Errorf("expected literal dollar match, got %d", len(tab.searchResults))
	}

	tab.SearchEditor.SetText(".00")
	tab.performSearch()
	if len(tab.searchResults) != 3 {
		t.Errorf("expected literal '.00' to find 3, got %d", len(tab.searchResults))
	}
}

func TestPerformSearch_QueryLongerThanText(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("hi")
	tab.invalidateSearchCache()
	tab.SearchEditor.SetText("longer than text")
	tab.performSearch()
	if len(tab.searchResults) != 0 {
		t.Errorf("expected no matches")
	}
}

func TestPerformSearch_OverlappingNoDuplicate(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("aaaa")
	tab.invalidateSearchCache()
	tab.SearchEditor.SetText("aa")
	tab.performSearch()

	if len(tab.searchResults) != 2 {
		t.Errorf("expected 2 non-overlapping matches, got %d: %v", len(tab.searchResults), tab.searchResults)
	}
}

func TestSearchNavigate_DirZero(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("hello hello")
	tab.SearchEditor.SetText("hello")
	tab.invalidateSearchCache()
	tab.performSearch()
	tab.searchCurrent = 0
	tab.searchNavigate(0)
	if tab.searchCurrent != 0 {
		t.Errorf("dir=0 should stay at 0, got %d", tab.searchCurrent)
	}
}

func TestAsciiToLower_Empty(t *testing.T) {
	if asciiToLower("") != "" {
		t.Errorf("empty should stay empty")
	}
	if asciiToLower("ASCII") != "ascii" {
		t.Errorf("ASCII conversion failed")
	}
	if asciiToLower("\x00\x01\x7F") != "\x00\x01\x7F" {
		t.Errorf("control chars should be unchanged")
	}
}

func TestCheckDirty_UnicodeURL(t *testing.T) {
	req := &model.ParsedRequest{
		Method:  "GET",
		URL:     "http://пример.рф",
		Body:    "",
		Name:    "T",
		Headers: map[string]string{},
	}
	node := &collections.CollectionNode{
		Request:    req,
		Collection: &collections.ParsedCollection{},
	}
	tab := &RequestTab{LinkedNode: node, Method: "GET", Title: "T"}
	tab.URLInput.SetText("http://пример.рф")
	tab.checkDirty()

	if tab.IsDirty {
		t.Error("unicode URL matching the linked request must not be marked dirty (rune-vs-byte regression)")
	}
}

func TestSaveToCollection_FormAndURLEncoded(t *testing.T) {
	col := &collections.ParsedCollection{}
	req := &model.ParsedRequest{
		Method:     "POST",
		URL:        "http://x",
		Name:       "T",
		Headers:    map[string]string{},
		FormParts:  []model.ParsedFormPart{{Key: "old"}},
		URLEncoded: []model.ParsedKV{{Key: "oldue"}},
	}
	node := &collections.CollectionNode{Request: req, Collection: col}
	tab := &RequestTab{LinkedNode: node, Method: "POST", Title: "T"}
	tab.URLInput.SetText("http://x")
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{
		NewFormPart("k1", "v1", model.FormPartText, "", 0),
		NewFormPart("", "skipped", model.FormPartText, "", 0),
		NewFormPart("file1", "", model.FormPartFile, "/tmp/x", 100),
	}
	tab.URLEncoded = []*URLEncodedPart{
		NewURLEncodedPart("a", "1"),
		NewURLEncodedPart("", "skip"),
	}
	tab.BinaryFilePath = "/path/binary"

	_ = tab.SaveToCollection()

	if len(req.FormParts) != 2 {
		t.Errorf("expected 2 saved form parts (blank-key skipped), got %d", len(req.FormParts))
	}
	if req.FormParts[0].Key != "k1" || req.FormParts[0].Value != "v1" {
		t.Errorf("form part 0 wrong: %+v", req.FormParts[0])
	}
	if req.FormParts[1].FilePath != "/tmp/x" || req.FormParts[1].Kind != model.FormPartFile {
		t.Errorf("file part wrong: %+v", req.FormParts[1])
	}
	if len(req.URLEncoded) != 1 || req.URLEncoded[0].Key != "a" {
		t.Errorf("urlencoded mismatch: %+v", req.URLEncoded)
	}
	if req.BinaryPath != "/path/binary" {
		t.Errorf("binary path not saved: %q", req.BinaryPath)
	}
	if req.BodyType != model.BodyFormData {
		t.Errorf("body type not saved")
	}
}

func TestSaveToCollection_GeneratedHeadersExcluded(t *testing.T) {
	req := &model.ParsedRequest{Method: "GET", URL: "http://x", Name: "T"}
	node := &collections.CollectionNode{Request: req, Collection: &collections.ParsedCollection{}}
	tab := &RequestTab{LinkedNode: node, Method: "GET", Title: "T"}
	tab.URLInput.SetText("http://x")

	user := &HeaderItem{IsGenerated: false}
	user.Key.SetText("X-Custom")
	user.Value.SetText("yes")

	gen := &HeaderItem{IsGenerated: true}
	gen.Key.SetText("User-Agent")
	gen.Value.SetText("auto")

	blank := &HeaderItem{IsGenerated: false}

	tab.Headers = []*HeaderItem{user, gen, blank}
	tab.SaveToCollection()

	if len(req.Headers) != 1 {
		t.Errorf("expected only 1 header (no generated, no blank), got %v", req.Headers)
	}
	if req.Headers["X-Custom"] != "yes" {
		t.Errorf("missing custom header: %v", req.Headers)
	}
	if _, ok := req.Headers["User-Agent"]; ok {
		t.Errorf("generated header should not be saved")
	}
}

func TestPrepareRequest_URLSpaceEncoding(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("example.com/path with spaces")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if !strings.Contains(req.URL.String(), "%20") {
		t.Errorf("expected spaces encoded, got %s", req.URL.String())
	}
	if !strings.HasPrefix(req.URL.String(), "http://") {
		t.Errorf("expected http:// prefix auto-added")
	}
}

func TestPrepareRequest_TabNewlineSanitize(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://ex\nample.\tcom")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if strings.ContainsAny(req.URL.String(), "\n\t") {
		t.Errorf("URL still contains tab/newline: %q", req.URL.String())
	}
}

func TestPrepareRequest_DefaultHeadersApplied(t *testing.T) {
	prev := settings.DefaultHeaders
	settings.DefaultHeaders = []model.DefaultHeader{
		{Key: "X-Default", Value: "{{token}}"},
		{Key: "", Value: "skipped"},
	}
	defer func() { settings.DefaultHeaders = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), map[string]string{"token": "TKN"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if req.Header.Get("X-Default") != "TKN" {
		t.Errorf("default header not applied: %q", req.Header.Get("X-Default"))
	}
}

func TestPrepareRequest_DefaultHeaderRespectsUserOverride(t *testing.T) {
	prev := settings.DefaultHeaders
	settings.DefaultHeaders = []model.DefaultHeader{
		{Key: "X-Default", Value: "default-val"},
	}
	defer func() { settings.DefaultHeaders = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	tab.AddHeader("X-Default", "user-val")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if got := req.Header.Get("X-Default"); got != "user-val" {
		t.Errorf("user-defined header should win, got %q", got)
	}
}

func TestPrepareRequest_AcceptEncodingDefault(t *testing.T) {
	prev := settings.AcceptEncoding
	settings.AcceptEncoding = "gzip, deflate"
	defer func() { settings.AcceptEncoding = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if req.Header.Get("Accept-Encoding") != "gzip, deflate" {
		t.Errorf("expected Accept-Encoding set, got %q", req.Header.Get("Accept-Encoding"))
	}
}

func TestPrepareRequest_SendConnClose(t *testing.T) {
	prev := settings.SendConnClose
	settings.SendConnClose = true
	defer func() { settings.SendConnClose = prev }()

	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, _, cancel, err := tab.prepareRequest(context.Background(), nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if !req.Close {
		t.Errorf("expected Close=true")
	}
	if req.Header.Get("Connection") != "close" {
		t.Errorf("expected Connection: close header, got %q", req.Header.Get("Connection"))
	}
}

func TestPrepareRequest_NilParentContext(t *testing.T) {
	tab := NewRequestTab("t")
	tab.URLInput.SetText("http://x")
	req, ctx, cancel, err := tab.prepareRequest(nil, nil) //nolint:staticcheck // nil parent context is the case under test
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	defer cancel()
	if ctx == nil || req == nil {
		t.Errorf("nil parent should be replaced with background")
	}
}

func TestExecuteRequest_FormDataPostsMultipart(t *testing.T) {
	var gotCT string
	var bodyContains bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		bodyContains = strings.Contains(string(data), "ALICE")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tab := NewRequestTab("t")
	tab.URLInput.SetText(srv.URL)
	tab.Method = "POST"
	tab.BodyType = model.BodyFormData
	tab.FormParts = []*FormDataPart{NewFormPart("name", "ALICE", model.FormPartText, "", 0)}

	tab.ExecuteRequest(context.Background(), new(app.Window), nil)
	select {
	case <-tab.responseChan:
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout")
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Errorf("expected multipart/form-data CT, got %q", gotCT)
	}
	if !bodyContains {
		t.Errorf("expected ALICE in body")
	}
}

func TestAddHeader_NotGeneratedByDefault(t *testing.T) {
	tab := NewRequestTab("t")
	tab.AddHeader("X", "Y")
	if tab.Headers[0].IsGenerated {
		t.Errorf("AddHeader should not mark as generated")
	}
	if tab.Headers[0].LastAutoKey != "" || tab.Headers[0].LastAutoVal != "" {
		t.Errorf("AddHeader should not set LastAuto* fields")
	}

	tab.addSystemHeader("A", "B")
	if !tab.Headers[1].IsGenerated {
		t.Errorf("addSystemHeader must mark generated")
	}
	if tab.Headers[1].LastAutoKey != "A" || tab.Headers[1].LastAutoVal != "B" {
		t.Errorf("addSystemHeader should record LastAuto*")
	}
}

func TestGetCleanTitle_Cache(t *testing.T) {
	tab := &RequestTab{Title: "alpha"}
	if tab.GetCleanTitle() != "alpha" {
		t.Errorf("first call wrong")
	}
	if tab.cleanTitleSrc != "alpha" {
		t.Errorf("cache key not set")
	}

	tab.cleanTitle = "OVERRIDE"
	if tab.GetCleanTitle() != "OVERRIDE" {
		t.Errorf("cache not used")
	}

	tab.Title = "beta"
	if tab.GetCleanTitle() != "beta" {
		t.Errorf("cache should invalidate on Title change")
	}
}

func TestJSONFormatterState_StringWithEscape(t *testing.T) {
	state := &JSONFormatterState{}
	got := formatJSON([]byte(`{"k":"a\"b","x":1}`), state)
	if !strings.Contains(got, `"a\"b"`) {
		t.Errorf("escaped quote in string broken: %q", got)
	}
}

func TestJSONFormatterState_StringSplitMidEscape(t *testing.T) {
	doc1 := []byte(`{"k":"a\`)
	doc2 := []byte(`"b","x":1}`)
	state := &JSONFormatterState{}
	a := formatJSON(doc1, state)
	b := formatJSON(doc2, state)
	full := a + b
	if !strings.Contains(full, `"a\"b"`) {
		t.Errorf("escape spanning batches broken: %q", full)
	}
}

func TestBytesAndTextAlignment(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText("hello")
	if string(tab.ReqEditor.Bytes()) != tab.ReqEditor.Text() {
		t.Errorf("Bytes/Text mismatch")
	}
	if tab.ReqEditor.Len() != len("hello") {
		t.Errorf("Len mismatch: %d vs %d", tab.ReqEditor.Len(), len("hello"))
	}
}

func TestMoveURLWord(t *testing.T) {
	s := "https://example.com/api/v1?key=val&x=y"
	n := len([]rune(s))

	cases := []struct {
		name string
		pos  int
		dir  int
		want int
	}{
		{"from-end-back-to-start-of-trailing-y", n, -1, 37},
		{"from-y-back-to-start-of-x", 37, -1, 35},
		{"from-x-back-to-start-of-val", 35, -1, 31},
		{"from-val-back-to-start-of-key", 31, -1, 27},
		{"from-0-forward-to-end-of-https", 0, 1, 5},
		{"forward-past-protocol-sep", 5, 1, len("https://") + len("example")},
		{"forward-stops-at-slash-boundary", len("https://example.com"), 1, len("https://example.com/") + len("api")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := moveURLWord(s, c.pos, c.dir)
			if got != c.want {
				t.Errorf("moveURLWord(%q, %d, %d) = %d, want %d", s, c.pos, c.dir, got, c.want)
			}
		})
	}

	if got := moveURLWord("", 0, 1); got != 0 {
		t.Errorf("empty forward: got %d, want 0", got)
	}
	if got := moveURLWord("", 0, -1); got != 0 {
		t.Errorf("empty backward: got %d, want 0", got)
	}
	if got := moveURLWord("abc", 100, 1); got != 3 {
		t.Errorf("oob forward: got %d, want 3", got)
	}
	if got := moveURLWord("abc", -5, 1); got != 3 {
		t.Errorf("negative pos forward: got %d, want 3", got)
	}
}

func TestMoveURLWord_Variables(t *testing.T) {
	s := "http://{{host}}/api/{{path}}?k={{val}}"

	cases := []struct {
		name string
		pos  int
		dir  int
		want int
	}{
		{"forward-from-protocol-jumps-over-whole-var", 4, 1, 15},
		{"forward-from-inside-var-jumps-to-end-of-var", 10, 1, 15},
		{"forward-from-var-start-jumps-to-var-end", 7, 1, 15},
		{"forward-after-var-into-next-word", 15, 1, 19},
		{"backward-from-after-var-jumps-to-var-start", 15, -1, 7},
		{"backward-from-inside-var-jumps-to-var-start", 11, -1, 7},
		{"backward-from-var-end-jumps-to-var-start", 14, -1, 7},
		{"forward-skips-into-second-var", 19, 1, 28},
		{"backward-from-third-var-end", 38, -1, 31},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := moveURLWord(s, c.pos, c.dir)
			if got != c.want {
				t.Errorf("moveURLWord(%q, %d, %d) = %d, want %d", s, c.pos, c.dir, got, c.want)
			}
		})
	}

	if got := moveURLWord("{{a}}{{b}}", 0, 1); got != 5 {
		t.Errorf("adjacent vars forward from 0: got %d, want 5", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 5, 1); got != 10 {
		t.Errorf("adjacent vars forward from 5: got %d, want 10", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 10, -1); got != 5 {
		t.Errorf("adjacent vars backward from 10: got %d, want 5", got)
	}
	if got := moveURLWord("{{a}}{{b}}", 5, -1); got != 0 {
		t.Errorf("adjacent vars backward from 5: got %d, want 0", got)
	}

	if got := moveURLWord("{{abc", 0, 1); got != 5 {
		t.Errorf("unmatched {{ forward: got %d, want 5", got)
	}
}

func TestIsURLWordSep(t *testing.T) {
	seps := []rune{'/', ':', '?', '#', '&', '=', '.', ' ', '\t', '@', ','}
	for _, r := range seps {
		if !isURLWordSep(r) {
			t.Errorf("expected %q to be separator", r)
		}
	}
	nonSeps := []rune{'a', 'Z', '0', '9', '-', '_', 'ё'}
	for _, r := range nonSeps {
		if isURLWordSep(r) {
			t.Errorf("did not expect %q to be separator", r)
		}
	}
}

func TestWidgetEditorLenIsRunes(t *testing.T) {
	var e widget.Editor
	e.SetText("привет")
	if e.Len() != 6 {
		t.Errorf("gio widget.Editor.Len returned %d, expected rune count 6", e.Len())
	}
	if len(e.Text()) != 12 {
		t.Errorf("Text byte length wrong: %d", len(e.Text()))
	}
}
