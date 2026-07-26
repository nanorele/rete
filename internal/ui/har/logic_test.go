package har

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tracto/internal/ui/theme"
)

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{-4096, "-"},
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024*1024 - 1, "1024.0K"},
		{1024 * 1024, "1.0M"},
		{3 * 1024 * 1024, "3.0M"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestBoolStr(t *testing.T) {
	if got := boolStr(true); got != "1" {
		t.Errorf("boolStr(true) = %q", got)
	}
	if got := boolStr(false); got != "0" {
		t.Errorf("boolStr(false) = %q", got)
	}
}

func TestStatusColor_Buckets(t *testing.T) {
	cases := []struct {
		code int
		want color.NRGBA
	}{
		{-1, theme.FgMuted},
		{0, theme.FgMuted},
		{100, theme.FgMuted},
		{199, theme.FgMuted},
		{200, theme.VarFound},
		{204, theme.VarFound},
		{299, theme.VarFound},
		{301, theme.Accent},
		{399, theme.Accent},
		{404, theme.VarMissing},
		{499, theme.VarMissing},
		{500, theme.Danger},
		{599, theme.Danger},
	}
	for _, c := range cases {
		if got := statusColor(c.code); got != c.want {
			t.Errorf("statusColor(%d) = %+v, want %+v", c.code, got, c.want)
		}
	}
}

func TestSplitURL_Table(t *testing.T) {
	cases := []struct {
		in           string
		domain, file string
	}{
		{"https://example.com/app/main.js?x=1", "example.com", "/app/main.js?x=1"},
		{"https://host/", "host", "/"},
		{"https://host", "host", "/"},
		{"http://host:8080/a/b", "host:8080", "/a/b"},
		{"https://host/?q=1", "host", "/?q=1"},
		{"", "", "/"},
		{"not a url", "", "not a url"},
		{"http://a\nb/", "", "http://a\nb/"},
		{"://missing-scheme", "", "://missing-scheme"},
		{"wss://ws.example.com/socket", "ws.example.com", "/socket"},
	}
	for _, c := range cases {
		d, f := SplitURL(c.in)
		if d != c.domain || f != c.file {
			t.Errorf("SplitURL(%q) = %q,%q, want %q,%q", c.in, d, f, c.domain, c.file)
		}
	}
}

func TestShortType_Table(t *testing.T) {
	cases := map[string]string{
		"application/javascript": "javascript",
		"image/png":              "png",
		"text/html":              "html",
		"":                       "",
		"json":                   "json",
		"a/b/c":                  "b/c",
	}
	for in, want := range cases {
		if got := shortType(in); got != want {
			t.Errorf("shortType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsProbablyText_Table(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want bool
	}{
		{"nil", nil, true},
		{"empty", []byte{}, true},
		{"ascii", []byte("hello world\r\n\tplain"), true},
		{"json", []byte(`{"a":1}`), true},
		{"nul", []byte("abc\x00def"), false},
		{"mostly-control", bytes.Repeat([]byte{0x01}, 100), false},
		{"few-control", append(bytes.Repeat([]byte("a"), 100), 0x01), true},
		{"large-text", bytes.Repeat([]byte("x"), 20000), true},
		{"large-binary", append(bytes.Repeat([]byte{0x02}, 9000), 'a'), false},
	}
	for _, c := range cases {
		if got := isProbablyText(c.in); got != c.want {
			t.Errorf("isProbablyText(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExportMsg_Table(t *testing.T) {
	cases := []struct {
		written, total int
		dest, want     string
	}{
		{3, 3, "/out", "Exported 3 files to /out"},
		{0, 0, "/out", "Exported 0 files to /out"},
		{1, 3, "/out", "Exported 1 of 3 files to /out (2 skipped)"},
	}
	for _, c := range cases {
		if got := exportMsg(c.written, c.total, c.dest); got != c.want {
			t.Errorf("exportMsg(%d,%d,%q) = %q, want %q", c.written, c.total, c.dest, got, c.want)
		}
	}
}

func TestEntrySize_Precedence(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	e := &rig.s.Doc.Entries[0]
	if got := entrySize(e); got != 42 {
		t.Errorf("Content.Size must win: got %d, want 42", got)
	}
	e.Response.Content.Size = 0
	e.Response.BodySize = 17
	if got := entrySize(e); got != 17 {
		t.Errorf("BodySize fallback: got %d, want 17", got)
	}
	e.Response.BodySize = 0
	if got := entrySize(e); got != int64(len(e.Response.Content.Text)) {
		t.Errorf("text-length fallback: got %d, want %d", got, len(e.Response.Content.Text))
	}
}

func TestStatusText_Table(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	cases := []struct {
		idx  int
		want string
	}{
		{0, "201 Created"},
		{1, "404 Not Found"},
		{2, "500"},
		{4, "(no response)"},
	}
	for _, c := range cases {
		if got := statusText(&rig.s.Doc.Entries[c.idx]); got != c.want {
			t.Errorf("statusText(entry %d) = %q, want %q", c.idx, got, c.want)
		}
	}
}

func TestDisplayMethod_Table(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	cases := map[int]string{0: "POST", 1: "GET", 3: "WS", 4: "DELETE"}
	for idx, want := range cases {
		if got := displayMethod(&rig.s.Doc.Entries[idx]); got != want {
			t.Errorf("displayMethod(entry %d) = %q, want %q", idx, got, want)
		}
	}
}

func TestWSText_BinaryAndSeparators(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	out := string(wsText(&rig.s.Doc.Entries[3], false))

	if !strings.Contains(out, "→ send") || !strings.Contains(out, "← receive") {
		t.Errorf("missing direction markers:\n%s", out)
	}
	if !strings.Contains(out, "[binary]") || !strings.Contains(out, "base64 chars]") {
		t.Errorf("binary frame not summarised:\n%s", out)
	}
	if !strings.Contains(out, "[text]") || !strings.Contains(out, "plain") {
		t.Errorf("text frame missing:\n%s", out)
	}
	if strings.HasSuffix(out, "\n\n") {
		t.Errorf("no blank separator expected after the last frame:\n%q", out)
	}

	pretty := string(wsText(&rig.s.Doc.Entries[3], true))
	if !strings.Contains(pretty, "\"op\": 1") {
		t.Errorf("pretty mode must indent JSON frames:\n%s", pretty)
	}
}

func TestRespBody_DecodeFallback(t *testing.T) {
	const badBase64 = `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/bad"},
       "response":{"status":200,"content":{"mimeType":"image/png","encoding":"base64","text":"!!!not-base64!!!"}}}
    ]}}`
	rig := newRig(t, badBase64, image.Pt(800, 600))
	got := string(respBody(&rig.s.Doc.Entries[0]))
	if got != "!!!not-base64!!!" {
		t.Errorf("undecodable body must fall back to the raw text, got %q", got)
	}

	rig2 := newRig(t, harBigDoc, image.Pt(800, 600))
	if got := string(respBody(&rig2.s.Doc.Entries[0])); got != `{"id":7,"ok":true}` {
		t.Errorf("plain body = %q", got)
	}
}

func TestVisibleIndices_NilDoc(t *testing.T) {
	st := &Section{}
	st.Ensure()
	if got := st.visibleIndices(); got != nil {
		t.Errorf("nil doc must yield no indices, got %v", got)
	}
	if got := st.pageRequestCount("p1"); got != 0 {
		t.Errorf("nil doc page count = %d, want 0", got)
	}
}

func TestVisibleIndices_CachedUntilPageChanges(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	st := rig.s
	first := st.visibleIndices()
	second := st.visibleIndices()
	if len(first) != len(second) {
		t.Fatalf("cached result changed: %v vs %v", first, second)
	}
	st.selectPage("p1")
	if got := st.visibleIndices(); len(got) != 2 {
		t.Errorf("p1 = %v, want 2 entries", got)
	}
	st.selectPage("")
	if got := st.visibleIndices(); len(got) != 5 {
		t.Errorf("all = %v, want 5 entries", got)
	}
}

func TestSortedResources_Ordered(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	if len(rig.s.Resources) < 2 {
		t.Fatalf("precondition: need >= 2 resources, got %d", len(rig.s.Resources))
	}
	for i := 1; i < len(rig.s.Resources); i++ {
		if rig.s.Resources[i-1].ZipPath > rig.s.Resources[i].ZipPath {
			t.Errorf("not sorted: %q before %q", rig.s.Resources[i-1].ZipPath, rig.s.Resources[i].ZipPath)
		}
	}
}

func TestSetBanner(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.setBanner("hello", false)
	st.DrainLoads()
	if st.Banner != "hello" || st.BannerErr {
		t.Errorf("banner = %q err=%v", st.Banner, st.BannerErr)
	}
	st.setBanner("boom", true)
	st.DrainLoads()
	if st.Banner != "boom" || !st.BannerErr {
		t.Errorf("error banner = %q err=%v", st.Banner, st.BannerErr)
	}
}

func TestQueueLoad_RequiresEnsuredChannel(t *testing.T) {
	st := &Section{}
	st.queueLoad([]byte(harNoPagesDoc), "dropped.har", nil)
	if st.loaded != nil {
		t.Fatal("queueLoad must not create the channel from a background goroutine (data race); ensureLoaded owns creation on the UI thread")
	}

	st.ensureLoaded()
	st.queueLoad([]byte(harNoPagesDoc), "lazy.har", nil)
	if st.loaded == nil {
		t.Fatal("ensureLoaded must create the channel")
	}
	if !st.DrainLoads() {
		t.Fatal("drain must report the queued load")
	}
	if st.Source != "lazy.har" {
		t.Errorf("Source = %q", st.Source)
	}
}

func TestLoadPathAsync_RealFileAndMissingFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.har")
	if err := os.WriteFile(good, []byte(harNoPagesDoc), 0o600); err != nil {
		t.Fatal(err)
	}

	st := &Section{}
	st.Ensure()
	var invalidated atomic.Int32
	st.LoadPathAsync(`"`+good+`"`, func() { invalidated.Add(1) })
	waitDrain(t, st)
	if st.Doc == nil {
		t.Fatalf("file load failed: %q", st.Banner)
	}
	if st.Source != "good.har" {
		t.Errorf("Source = %q, want good.har", st.Source)
	}
	if invalidated.Load() == 0 {
		t.Error("invalidate callback was never called")
	}

	st2 := &Section{}
	st2.Ensure()
	st2.LoadPathAsync(filepath.Join(dir, "missing.har"), nil)
	waitDrain(t, st2)
	if !st2.BannerErr {
		t.Errorf("missing file must set an error banner, got %q", st2.Banner)
	}
}

func waitDrain(t *testing.T, st *Section) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st.DrainLoads() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("no load result arrived within the deadline")
}

func TestBrowse_Variants(t *testing.T) {
	t.Run("nil-chooser", func(t *testing.T) {
		rig := newRig(t, "", image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.ChooseHAR = nil
		rig.s.browse()
		if rig.s.DrainLoads() {
			t.Error("a nil chooser must not queue anything")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		rig := newRig(t, "", image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.ChooseHAR = func() (io.ReadCloser, error) { return nil, nil }
		rig.s.browse()
		time.Sleep(50 * time.Millisecond)
		if rig.s.DrainLoads() {
			t.Error("a cancelled chooser must not queue anything")
		}
	})

	t.Run("error", func(t *testing.T) {
		rig := newRig(t, "", image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.ChooseHAR = func() (io.ReadCloser, error) { return nil, errors.New("nope") }
		rig.s.browse()
		waitDrain(t, rig.s)
		if !rig.s.BannerErr || !strings.Contains(rig.s.Banner, "nope") {
			t.Errorf("banner = %q err=%v", rig.s.Banner, rig.s.BannerErr)
		}
	})

	t.Run("success", func(t *testing.T) {
		rig := newRig(t, "", image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.ChooseHAR = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(harBigDoc)), nil
		}
		rig.s.browse()
		waitDrain(t, rig.s)
		if rig.s.Doc == nil || len(rig.s.Doc.Entries) != 5 {
			t.Fatalf("browse did not load the document: %q", rig.s.Banner)
		}
	})
}

type memWriteCloser struct {
	bytes.Buffer
	closeErr error
	closed   bool
}

func (m *memWriteCloser) Close() error { m.closed = true; return m.closeErr }

func TestExportZip_Variants(t *testing.T) {
	t.Run("no-resources", func(t *testing.T) {
		rig := newRig(t, harNoPagesDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		called := false
		rig.host.CreateFile = func(string) (io.WriteCloser, error) { called = true; return nil, nil }
		rig.s.exportZip()
		time.Sleep(30 * time.Millisecond)
		if called {
			t.Error("export must not run without resources")
		}
	})

	t.Run("nil-creator", func(t *testing.T) {
		rig := newRig(t, harBigDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.CreateFile = nil
		rig.s.exportZip()
		time.Sleep(30 * time.Millisecond)
		if rig.s.Banner != "" && strings.Contains(rig.s.Banner, "Export") {
			t.Errorf("nil creator must be a no-op, banner=%q", rig.s.Banner)
		}
	})

	t.Run("create-error", func(t *testing.T) {
		rig := newRig(t, harBigDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.CreateFile = func(string) (io.WriteCloser, error) { return nil, errors.New("disk full") }
		rig.s.exportZip()
		waitBanner(t, rig.s, "disk full")
		if !rig.s.BannerErr {
			t.Error("create error must set BannerErr")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		rig := newRig(t, harBigDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.CreateFile = func(string) (io.WriteCloser, error) { return nil, nil }
		rig.s.exportZip()
		time.Sleep(30 * time.Millisecond)
		if strings.Contains(rig.s.Banner, "Export failed") {
			t.Errorf("cancel must not report a failure, banner=%q", rig.s.Banner)
		}
	})

	t.Run("close-error", func(t *testing.T) {
		rig := newRig(t, harBigDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		rig.host.CreateFile = func(string) (io.WriteCloser, error) {
			return &memWriteCloser{closeErr: errors.New("fsync failed")}, nil
		}
		rig.s.exportZip()
		waitBanner(t, rig.s, "fsync failed")
	})

	t.Run("success", func(t *testing.T) {
		rig := newRig(t, harBigDoc, image.Pt(800, 600))
		rig.s.host = rig.host
		var sink *memWriteCloser
		var name string
		rig.host.CreateFile = func(n string) (io.WriteCloser, error) {
			name = n
			sink = &memWriteCloser{}
			return sink, nil
		}
		rig.s.exportZip()
		waitBanner(t, rig.s, "Exported")
		if rig.s.BannerErr {
			t.Errorf("success must not set BannerErr: %q", rig.s.Banner)
		}
		if name != "capture.zip" {
			t.Errorf("suggested name = %q, want capture.zip", name)
		}
		if sink == nil || !sink.closed || sink.Len() == 0 {
			t.Error("zip writer must be written to and closed")
		}
		if !bytes.HasPrefix(sink.Bytes(), []byte("PK")) {
			t.Errorf("output is not a zip archive: %x", sink.Bytes()[:4])
		}
	})
}

func waitBanner(t *testing.T, st *Section, substr string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		st.DrainLoads()
		if strings.Contains(st.Banner, substr) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("banner never contained %q (got %q)", substr, st.Banner)
}

func TestExportDir_NoResourcesIsNoop(t *testing.T) {
	rig := newRig(t, harNoPagesDoc, image.Pt(800, 600))
	rig.s.host = rig.host
	if len(rig.s.Resources) != 0 {
		t.Fatalf("precondition: doc must have no resources, got %d", len(rig.s.Resources))
	}
	rig.s.Banner = ""
	rig.s.exportDir()
	time.Sleep(30 * time.Millisecond)
	if rig.s.Banner != "" {
		t.Errorf("no-resource export must not set a banner, got %q", rig.s.Banner)
	}
}

func TestEnsure_Defaults(t *testing.T) {
	st := &Section{}
	st.Ensure()
	if st.SplitRatio != 0.42 {
		t.Errorf("SplitRatio = %v, want 0.42", st.SplitRatio)
	}
	if st.HdrH != 150 {
		t.Errorf("HdrH = %v, want 150", st.HdrH)
	}
	if st.SelReq != -1 || st.SelFile != -1 {
		t.Errorf("selections = %d/%d, want -1/-1", st.SelReq, st.SelFile)
	}
	if st.ReqViewer == nil || st.FileViewer == nil || st.Table == nil || st.loaded == nil {
		t.Error("Ensure must allocate viewers, table and load channel")
	}

	prevViewer, prevTable, prevRatio := st.ReqViewer, st.Table, float32(0.9)
	st.SplitRatio = prevRatio
	st.Ensure()
	if st.ReqViewer != prevViewer || st.Table != prevTable {
		t.Error("Ensure must not reallocate existing state")
	}
	if st.SplitRatio != prevRatio {
		t.Errorf("Ensure must not overwrite a set SplitRatio, got %v", st.SplitRatio)
	}
}

func TestTableColumns_Shape(t *testing.T) {
	cols := tableColumns()
	want := []string{"#", "Method", "Status", "Domain", "File", "Type", "Size"}
	if len(cols) != len(want) {
		t.Fatalf("column count = %d, want %d", len(cols), len(want))
	}
	for i, w := range want {
		if cols[i].Title != w {
			t.Errorf("column %d = %q, want %q", i, cols[i].Title, w)
		}
	}
	if cols[4].Width != 0 {
		t.Errorf("the File column must flex (width 0), got %v", cols[4].Width)
	}
}

func TestInfoRows_AllSections(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(800, 600))
	rows := infoRows(rig.s.Doc.Summary())

	headers := map[string]bool{}
	kv := map[string]string{}
	for _, r := range rows {
		if r.header {
			headers[r.key] = true
		} else {
			kv[r.key] = r.val
		}
	}
	for _, h := range []string{"Archive", "Methods", "Status codes", "Content types"} {
		if !headers[h] {
			t.Errorf("missing header section %q", h)
		}
	}
	if kv["HAR version"] != "1.2" {
		t.Errorf("HAR version = %q", kv["HAR version"])
	}
	if kv["Creator"] != "Chrome 125" {
		t.Errorf("Creator = %q, want \"Chrome 125\"", kv["Creator"])
	}
	if kv["Pages"] != "2" {
		t.Errorf("Pages = %q, want 2", kv["Pages"])
	}
	if kv["Requests"] != "5" {
		t.Errorf("Requests = %q, want 5", kv["Requests"])
	}
}

func TestInfoRows_MinimalArchive(t *testing.T) {
	rig := newRig(t, `{"log":{"version":"1.2","entries":[],"pages":[]}}`, image.Pt(800, 600))
	rows := infoRows(rig.s.Doc.Summary())
	kv := map[string]string{}
	for _, r := range rows {
		if r.header {
			if r.key != "Archive" {
				t.Errorf("an archive with no entries must not emit the %q section", r.key)
			}
			continue
		}
		kv[r.key] = r.val
	}
	if kv["Creator"] != "—" || kv["Browser"] != "—" {
		t.Errorf("missing creator/browser must render as a dash: %q / %q", kv["Creator"], kv["Browser"])
	}
	if kv["First request"] != "—" || kv["Last request"] != "—" {
		t.Errorf("missing timestamps must render as a dash: %q / %q", kv["First request"], kv["Last request"])
	}
	if kv["Requests"] != "0" || kv["Files with body"] != "0" {
		t.Errorf("counts = %q / %q, want 0 / 0", kv["Requests"], kv["Files with body"])
	}
}
