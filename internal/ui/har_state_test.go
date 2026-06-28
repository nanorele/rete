package ui

import (
	"strings"
	"testing"
)

const harTestDoc = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "Firefox", "version": "126.0"},
    "pages": [{"id": "p1", "title": "t", "startedDateTime": "2024-01-01T10:00:00Z"}],
    "entries": [
      {"startedDateTime": "2024-01-01T10:00:00.1Z", "request": {"method": "GET", "url": "https://example.com/app.js"},
        "response": {"status": 200, "content": {"mimeType": "application/javascript", "text": "code"}}},
      {"startedDateTime": "2024-01-01T10:00:00.2Z", "request": {"method": "GET", "url": "https://example.com/empty"},
        "response": {"status": 204, "content": {"mimeType": "text/plain"}}}
    ]
  }
}`

func TestHarApplyLoad_Success(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "capture.har", nil)

	if st.Doc == nil {
		t.Fatal("Doc must be set after a successful load")
	}
	if len(st.Doc.Entries) != 2 {
		t.Errorf("entries = %d, want 2", len(st.Doc.Entries))
	}
	if len(st.Resources) != 1 {
		t.Errorf("resources = %d, want 1", len(st.Resources))
	}
	if st.SelReq != 0 || st.SelFile != 0 {
		t.Errorf("selection = req %d file %d, want 0/0", st.SelReq, st.SelFile)
	}
	if st.BannerErr {
		t.Error("BannerErr must be false on success")
	}
	if !strings.Contains(st.Banner, "capture.har") || !strings.Contains(st.Banner, "2 requests") {
		t.Errorf("banner = %q", st.Banner)
	}
	if st.Source != "capture.har" {
		t.Errorf("Source = %q", st.Source)
	}
}

func TestHarApplyLoad_ReadError(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad(nil, "x", errEmptyPath)
	if st.Doc != nil {
		t.Error("Doc must stay nil on read error")
	}
	if !st.BannerErr || !strings.Contains(st.Banner, "Import failed") {
		t.Errorf("banner = %q err=%v", st.Banner, st.BannerErr)
	}
}

func TestHarApplyLoad_ParseError(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte("not a har"), "x", nil)
	if st.Doc != nil {
		t.Error("Doc must stay nil on parse error")
	}
	if !st.BannerErr || !strings.Contains(st.Banner, "valid HAR") {
		t.Errorf("banner = %q", st.Banner)
	}
}

func TestHarApplyLoad_ReplacesPreviousDoc(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "first.har", nil)
	st.SelReq = 1
	st.applyLoad([]byte("garbage"), "second", nil)
	if st.Doc == nil || st.Source != "first.har" {
		t.Errorf("failed load must keep previous doc; Source=%q", st.Source)
	}
	st.applyLoad([]byte(harTestDoc), "third.har", nil)
	if st.SelReq != 0 {
		t.Errorf("SelReq after reload = %d, want 0", st.SelReq)
	}
}

func TestHarClear(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	st.clear()
	if st.Doc != nil || st.Resources != nil || st.Source != "" {
		t.Error("clear must reset doc/resources/source")
	}
	if st.SelReq != -1 || st.SelFile != -1 {
		t.Errorf("clear must reset selections to -1, got %d/%d", st.SelReq, st.SelFile)
	}
	if st.Banner != "" {
		t.Errorf("clear must reset banner, got %q", st.Banner)
	}
}

func TestHarQueueAndDrain(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.queueLoad([]byte(harTestDoc), "queued.har", nil)
	if st.Doc != nil {
		t.Error("queueLoad must not apply until drained")
	}
	changed := st.drainLoads()
	if !changed {
		t.Error("drainLoads must report a change")
	}
	if st.Doc == nil || st.Source != "queued.har" {
		t.Errorf("drain did not apply queued load; Source=%q", st.Source)
	}
	if st.drainLoads() {
		t.Error("second drain must report no change")
	}
}

func TestHarQueueLoad_DoesNotBlockWhenFull(t *testing.T) {
	st := &harState{}
	st.ensure()
	for i := 0; i < 10; i++ {
		st.queueLoad([]byte(harTestDoc), "x", nil)
	}
}

func TestHarLoadPathAsync_EmptyPath(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.loadPathAsync("   ", nil)
	if !st.drainLoads() {
		t.Fatal("empty path must queue an error result")
	}
	if !st.BannerErr {
		t.Errorf("empty path must set an error banner, got %q", st.Banner)
	}
}

func TestHarSortedResources_OrderedByPath(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	for i := 1; i < len(st.Resources); i++ {
		if st.Resources[i-1].ZipPath > st.Resources[i].ZipPath {
			t.Errorf("resources not sorted: %q before %q", st.Resources[i-1].ZipPath, st.Resources[i].ZipPath)
		}
	}
}

func TestBaseName(t *testing.T) {
	cases := map[string]string{
		`C:\dir\sub\capture.har`: "capture.har",
		"/home/user/a.har":       "a.har",
		"plain.har":              "plain.har",
		"":                       "",
	}
	for in, want := range cases {
		if got := baseName(in); got != want {
			t.Errorf("baseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestItoaN(t *testing.T) {
	cases := map[int]string{0: "0", 7: "7", 42: "42", 1000: "1000", -5: "-5"}
	for in, want := range cases {
		if got := itoaN(in); got != want {
			t.Errorf("itoaN(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestHarExportName(t *testing.T) {
	cases := map[string]string{
		"capture.har": "capture.zip",
		"a.b.har":     "a.b.zip",
		"noext":       "noext.zip",
		"":            "har-export.zip",
	}
	for in, want := range cases {
		if got := harExportName(in); got != want {
			t.Errorf("harExportName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHarSplitURL(t *testing.T) {
	d, f := harSplitURL("https://example.com/app/main.js?x=1")
	if d != "example.com" || f != "/app/main.js?x=1" {
		t.Errorf("split = %q,%q", d, f)
	}
	d, f = harSplitURL("https://host/")
	if d != "host" || f != "/" {
		t.Errorf("root split = %q,%q", d, f)
	}
}

func TestHarShortType(t *testing.T) {
	if got := harShortType("application/javascript"); got != "javascript" {
		t.Errorf("shortType = %q", got)
	}
	if got := harShortType("image/png"); got != "png" {
		t.Errorf("shortType = %q", got)
	}
	if got := harShortType(""); got != "" {
		t.Errorf("shortType empty = %q", got)
	}
}

func TestHarInfoRows_HasHeadersAndStats(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	rows := harInfoRows(st.Doc.Summary())

	var headers, kvs int
	found := map[string]bool{}
	for _, r := range rows {
		if r.header {
			headers++
			found[r.key] = true
		} else {
			kvs++
		}
	}
	if !found["Archive"] || !found["Methods"] || !found["Status codes"] {
		t.Errorf("missing expected header sections: %+v", found)
	}
	if kvs == 0 {
		t.Error("expected key/value rows")
	}
}

func TestBuildRowCache_PopulatedOnLoad(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)

	if len(st.rowCache) != len(st.Doc.Entries) {
		t.Fatalf("rowCache len = %d, want %d", len(st.rowCache), len(st.Doc.Entries))
	}
	r0 := st.rowCache[0]
	if r0.index != "1" {
		t.Errorf("row 0 index = %q, want \"1\"", r0.index)
	}
	if r0.domain != "example.com" || r0.file != "/app.js" {
		t.Errorf("row 0 domain/file = %q,%q", r0.domain, r0.file)
	}
	if r0.typ != "javascript" {
		t.Errorf("row 0 type = %q, want javascript", r0.typ)
	}
	if st.rowCache[1].index != "2" {
		t.Errorf("row 1 index = %q, want \"2\"", st.rowCache[1].index)
	}
}

func TestBuildRowCache_ClearedAndRebuilt(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	st.clear()
	if st.rowCache != nil {
		t.Error("clear must drop rowCache")
	}
	st.applyLoad([]byte(harTestDoc), "y.har", nil)
	if len(st.rowCache) != 2 {
		t.Errorf("rowCache after reload = %d, want 2", len(st.rowCache))
	}
}

func TestInspectorBody_CachesUntilKeyChanges(t *testing.T) {
	st := &harState{}
	calls := 0
	build := func() []byte { calls++; return []byte("body-A") }

	if got := string(st.inspectorBody("k1", build)); got != "body-A" {
		t.Fatalf("first call = %q", got)
	}
	for i := 0; i < 5; i++ {
		if got := string(st.inspectorBody("k1", build)); got != "body-A" {
			t.Fatalf("cached call = %q", got)
		}
	}
	if calls != 1 {
		t.Fatalf("build ran %d times for one key, want 1", calls)
	}

	build2 := func() []byte { calls++; return []byte("body-B") }
	if got := string(st.inspectorBody("k2", build2)); got != "body-B" {
		t.Errorf("after key change = %q", got)
	}
	if calls != 2 {
		t.Errorf("build ran %d times total, want 2", calls)
	}
}

func TestInspectorBody_ResetOnLoadAndClear(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.inspectorBody("resp/0", func() []byte { return []byte("stale") })
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	if st.bodyCacheKey != "" || st.bodyCache != nil {
		t.Errorf("load must reset body cache, got key=%q len=%d", st.bodyCacheKey, len(st.bodyCache))
	}
	st.inspectorBody("resp/0", func() []byte { return []byte("fresh") })
	st.clear()
	if st.bodyCacheKey != "" || st.bodyCache != nil {
		t.Errorf("clear must reset body cache, got key=%q len=%d", st.bodyCacheKey, len(st.bodyCache))
	}
}

func TestInfoRows_CachedAndInvalidated(t *testing.T) {
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harTestDoc), "x.har", nil)
	if st.infoCached {
		t.Error("infoCached must be false until the Info view is first rendered")
	}
	if !st.infoCached {
		st.infoRows = harInfoRows(st.Doc.Summary())
		st.infoCached = true
	}
	first := st.infoRows
	if len(first) == 0 {
		t.Fatal("infoRows empty after build")
	}
	st.applyLoad([]byte(harTestDoc), "y.har", nil)
	if st.infoCached || st.infoRows != nil {
		t.Error("reload must reset the info cache")
	}
	st.clear()
	if st.infoCached || st.infoRows != nil {
		t.Error("clear must reset the info cache")
	}
}

func TestJoinNameVersionAndOrDash(t *testing.T) {
	if got := joinNameVersion("Firefox", "126"); got != "Firefox 126" {
		t.Errorf("join = %q", got)
	}
	if got := joinNameVersion("Firefox", ""); got != "Firefox" {
		t.Errorf("join no version = %q", got)
	}
	if got := joinNameVersion("", ""); got != "—" {
		t.Errorf("join empty = %q", got)
	}
	if got := orDash("  "); got != "—" {
		t.Errorf("orDash blank = %q", got)
	}
	if got := orDash("x"); got != "x" {
		t.Errorf("orDash = %q", got)
	}
}
