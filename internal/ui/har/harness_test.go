package har

import (
	"image"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tracto/internal/har"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

const harBigDoc = `{
  "log": {
    "version": "1.2",
    "creator": {"name": "Chrome", "version": "125"},
    "browser": {"name": "Chrome", "version": "125"},
    "pages": [
      {"id":"p1","title":"Home","startedDateTime":"2024-01-01T10:00:00Z"},
      {"id":"p2","title":"","startedDateTime":"2024-01-01T10:05:00Z"}
    ],
    "entries": [
      {"pageref":"p1","startedDateTime":"2024-01-01T10:00:00.1Z",
       "request":{"method":"POST","url":"https://api.example.com/v1/users?page=2",
         "headers":[{"name":"Content-Type","value":"application/json"},{"name":"Accept","value":"*/*"}],
         "postData":{"mimeType":"application/json","text":"{\"name\":\"a\",\"tags\":[1,2,3]}"}},
       "response":{"status":201,"statusText":"Created",
         "headers":[{"name":"Content-Type","value":"application/json"}],
         "content":{"mimeType":"application/json","size":42,"text":"{\"id\":7,\"ok\":true}"}}},
      {"pageref":"p1","startedDateTime":"2024-01-01T10:00:01Z",
       "request":{"method":"GET","url":"https://cdn.example.com/static/app.css","headers":[]},
       "response":{"status":404,"statusText":"Not Found","headers":[],
         "content":{"mimeType":"text/css","text":"body{}"}}},
      {"pageref":"p2","startedDateTime":"2024-01-01T10:05:00.1Z",
       "request":{"method":"GET","url":"https://example.com/img.png","headers":[]},
       "response":{"status":500,"headers":[],
         "content":{"mimeType":"image/png","encoding":"base64","text":"AAECAwQFBgcICQAAAAA="}}},
      {"pageref":"p2","startedDateTime":"2024-01-01T10:05:01Z",
       "request":{"method":"GET","url":"https://example.com/sock","headers":[{"name":"Upgrade","value":"websocket"}]},
       "response":{"status":101,"headers":[]},
       "_webSocketMessages":[
         {"type":"send","time":1,"opcode":1,"data":"{\"op\":1}"},
         {"type":"receive","time":2,"opcode":2,"data":"AAECAwQF"},
         {"type":"receive","time":3,"opcode":1,"data":"plain"}
       ]},
      {"startedDateTime":"2024-01-01T10:06:00Z",
       "request":{"method":"DELETE","url":"https://example.com/gone","headers":[]},
       "response":{"status":0,"headers":[],"content":{"mimeType":""}}}
    ]
  }
}`

const harNoPagesDoc = `{
  "log": {"version":"1.2","entries":[
    {"request":{"method":"GET","url":"https://x/a"},"response":{"status":200,"content":{"mimeType":"text/plain"}}}
  ]}
}`

// material.NewTheme leaves the shaper without a font collection, so text
// measurement falls back to whatever fonts the OS happens to expose — zero
// width on a CI box with no installed fonts, which silently collapses every
// label-sized hit area. Pin the embedded Go fonts so layout is identical
// everywhere.
func testTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func testHost() *Host {
	return &Host{Theme: testTheme(), Window: new(app.Window)}
}

func testGtx(r *input.Router, sz image.Point, now time.Time) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Source:      r.Source(),
		Now:         now,
	}
}

type harRig struct {
	s    *Section
	host *Host
	r    input.Router
	sz   image.Point
	now  time.Time
}

func newRig(t *testing.T, doc string, sz image.Point) *harRig {
	t.Helper()
	rig := &harRig{s: &Section{}, host: testHost(), sz: sz, now: time.Unix(1700000000, 0)}
	rig.s.Ensure()
	if doc != "" {
		rig.s.ApplyLoad([]byte(doc), "capture.har", nil)
		if rig.s.Doc == nil {
			t.Fatalf("precondition: doc must parse; banner=%q", rig.s.Banner)
		}
	}
	return rig
}

func (rig *harRig) frame() layout.Dimensions {
	rig.now = rig.now.Add(16 * time.Millisecond)
	gtx := testGtx(&rig.r, rig.sz, rig.now)
	dims := rig.s.Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *harRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *harRig) press(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *harRig) move(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *harRig) release(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frames(2)
}

func (rig *harRig) click(x, y float32) {
	rig.press(x, y)
	rig.release(x, y)
}

func TestLayout_EmptyState(t *testing.T) {
	rig := newRig(t, "", image.Pt(1000, 600))
	dims := rig.frames(2)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("empty state produced no dimensions: %+v", dims.Size)
	}
	if rig.s.Doc != nil {
		t.Error("empty rig must have no doc")
	}
}

func TestLayout_AllTabsRender(t *testing.T) {
	tabs := []struct {
		name string
		tab  int
	}{
		{"requests", TabRequests},
		{"files", TabFiles},
		{"pages", TabPages},
		{"info", TabInfo},
		{"unknown-falls-back", 99},
	}
	for _, tc := range tabs {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, harBigDoc, image.Pt(1200, 700))
			rig.s.TopTab = tc.tab
			if d := rig.frames(2); d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Fatalf("tab %d produced no dimensions", tc.tab)
			}
		})
	}
}

func TestLayout_InspectorVariants(t *testing.T) {
	cases := []struct {
		name    string
		selReq  int
		inspTab int
		pretty  bool
	}{
		{"request-tab", 0, 0, false},
		{"response-tab", 0, 1, false},
		{"response-pretty", 0, 1, true},
		{"error-status", 1, 1, false},
		{"binary-body", 2, 1, false},
		{"websocket", 3, 1, false},
		{"websocket-pretty", 3, 1, true},
		{"no-response", 4, 1, false},
		{"no-request-body", 4, 0, false},
		{"unselected", -1, 0, false},
		{"out-of-range", 999, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, harBigDoc, image.Pt(1200, 700))
			rig.s.TopTab = TabRequests
			rig.s.SelReq = tc.selReq
			rig.s.InspTab = tc.inspTab
			rig.s.Pretty = tc.pretty
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatal("inspector produced no dimensions")
			}
		})
	}
}

func TestLayout_FilesTabVariants(t *testing.T) {
	cases := []struct {
		name    string
		selFile int
		pretty  bool
	}{
		{"first", 0, false},
		{"first-pretty", 0, true},
		{"unselected", -1, false},
		{"out-of-range", 999, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, harBigDoc, image.Pt(1200, 700))
			rig.s.TopTab = TabFiles
			rig.s.SelFile = tc.selFile
			rig.s.Pretty = tc.pretty
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatal("files view produced no dimensions")
			}
		})
	}
}

func TestLayout_EmptyCollections(t *testing.T) {
	cases := []struct {
		name string
		tab  int
	}{
		{"no-pages", TabPages},
		{"no-files", TabFiles},
		{"requests", TabRequests},
		{"info", TabInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, harNoPagesDoc, image.Pt(1000, 600))
			rig.s.TopTab = tc.tab
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatal("no dimensions")
			}
		})
	}
}

func TestLayout_PageFilterWithNoMatches(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1000, 600))
	rig.s.TopTab = TabRequests
	rig.s.selectPage("nonexistent-page")
	if got := rig.s.visibleIndices(); len(got) != 0 {
		t.Fatalf("expected no visible entries, got %v", got)
	}
	if rig.s.SelReq != -1 {
		t.Errorf("SelReq = %d, want -1 when the page filter matches nothing", rig.s.SelReq)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("empty-filter view produced no dimensions")
	}
}

func TestLayout_TabClicksSwitchTopTab(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1400, 700))
	rig.frames(2)

	seen := map[int]bool{}
	for x := float32(150); x < 560; x += 4 {
		rig.s.TopTab = -1
		rig.click(x, 16)
		if rig.s.TopTab >= 0 {
			seen[rig.s.TopTab] = true
		}
	}
	for _, want := range []int{TabRequests, TabFiles, TabPages, TabInfo} {
		if !seen[want] {
			t.Errorf("toolbar tab %d was never reachable by clicking", want)
		}
	}
}

func TestLayout_RowClickSelects(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.frames(2)

	sel := map[int]bool{}
	for y := float32(40); y < 260; y += 2 {
		rig.click(200, y)
		sel[rig.s.SelReq] = true
	}
	if len(sel) < 3 {
		t.Errorf("clicking the request table selected only %d distinct rows: %v", len(sel), sel)
	}
}

func TestLayout_FileRowClickSelects(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabFiles
	rig.frames(2)
	if len(rig.s.Resources) < 2 {
		t.Fatalf("precondition: need >= 2 resources, got %d", len(rig.s.Resources))
	}

	sel := map[int]bool{}
	for y := float32(40); y < 200; y += 2 {
		rig.click(120, y)
		sel[rig.s.SelFile] = true
	}
	if len(sel) < 2 {
		t.Errorf("clicking the file list selected only %d distinct rows: %v", len(sel), sel)
	}
}

func TestLayout_PageRowClickSelectsAndSwitchesTab(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabPages
	rig.frames(2)

	seen := map[string]bool{}
	for y := float32(40); y < 220; y += 2 {
		rig.s.TopTab = TabPages
		rig.click(200, y)
		seen[rig.s.SelPageID] = true
	}
	if !seen["p1"] || !seen["p2"] || !seen[""] {
		t.Errorf("page rows did not cover all/p1/p2: %v", seen)
	}
}

func TestLayout_SplitDragMovesRatio(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.frames(2)

	before := rig.s.SplitRatio
	handleX := float32(rig.s.leftDrawn) + 3
	rig.press(handleX, 400)
	rig.move(handleX+120, 400)
	rig.release(handleX+120, 400)

	if rig.s.SplitRatio <= before {
		t.Errorf("dragging the split right must grow SplitRatio: %v -> %v", before, rig.s.SplitRatio)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after split drag")
	}
}

func TestLayout_SplitDragClampsAtEdges(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.frames(2)

	handleX := float32(rig.s.leftDrawn) + 3
	rig.press(handleX, 400)
	rig.move(-5000, 400)
	rig.release(-5000, 400)
	rig.frames(2)
	if rig.s.leftDrawn < 240 {
		t.Errorf("left pane below minimum after dragging far left: %d", rig.s.leftDrawn)
	}

	handleX = float32(rig.s.leftDrawn) + 3
	rig.press(handleX, 400)
	rig.move(5000, 400)
	rig.release(5000, 400)
	rig.frames(2)
	if rig.s.leftDrawn > 1200-6-280 {
		t.Errorf("left pane above maximum after dragging far right: %d", rig.s.leftDrawn)
	}
}

func TestLayout_HeaderSplitDragResizes(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 0
	rig.frames(2)

	x := float32(rig.s.leftDrawn + 200)
	moved := false
	for y := float32(60); y < 620; y++ {
		before := rig.s.HdrH
		rig.press(x, y)
		rig.move(x, y+20)
		rig.release(x, y+20)
		if rig.s.HdrH != before {
			moved = true
			break
		}
	}
	if !moved {
		t.Error("header splitter drag never changed HdrH")
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after header drag")
	}
}

func TestLayout_BodyScrollbarDrag(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1
	rig.frames(3)

	x := float32(rig.sz.X - 4)
	for y := float32(400); y < 660; y += 10 {
		rig.press(x, y)
		rig.move(x, y+40)
		rig.release(x, y+40)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after scrollbar drag")
	}
}

func TestLayout_NarrowAndTinyViewports(t *testing.T) {
	for _, sz := range []image.Point{{X: 320, Y: 240}, {X: 640, Y: 200}, {X: 1920, Y: 1080}} {
		rig := newRig(t, harBigDoc, sz)
		for _, tab := range []int{TabRequests, TabFiles, TabPages, TabInfo} {
			rig.s.TopTab = tab
			if d := rig.frames(2); d.Size.X <= 0 {
				t.Errorf("size %v tab %d produced no dimensions", sz, tab)
			}
		}
	}
}

func TestLayout_HandleSearchShortcut(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		tab     int
		selReq  int
		selFile int
	}{
		{"no-doc", "", TabRequests, -1, -1},
		{"requests", harBigDoc, TabRequests, 0, 0},
		{"requests-unselected", harBigDoc, TabRequests, -1, 0},
		{"files", harBigDoc, TabFiles, 0, 0},
		{"files-unselected", harBigDoc, TabFiles, 0, -1},
		{"pages-noop", harBigDoc, TabPages, 0, 0},
		{"info-noop", harBigDoc, TabInfo, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, tc.doc, image.Pt(1200, 700))
			rig.s.TopTab = tc.tab
			rig.s.SelReq = tc.selReq
			rig.s.SelFile = tc.selFile
			rig.frames(2)
			gtx := testGtx(&rig.r, rig.sz, rig.now)
			rig.s.HandleSearchShortcut(gtx)
			rig.frames(2)
		})
	}
}

func TestLayout_SearchOverlayRenders(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1
	rig.frames(2)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	rig.r.Frame(gtx.Ops)
	if d := rig.frames(3); d.Size.Y <= 0 {
		t.Fatal("layout broke with the search overlay open")
	}
}

func TestLayout_CopyAndRunActions(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.frames(2)
	gtx := testGtx(&rig.r, rig.sz, rig.now)

	var ran int
	rig.host.RunEntry = func(e *har.Entry) { ran++ }
	rig.s.host = rig.host

	rig.s.SelReq = 0
	rig.s.runSelected()
	if ran != 1 {
		t.Errorf("runSelected did not invoke RunEntry (%d)", ran)
	}
	rig.s.SelReq = 999
	rig.s.runSelected()
	if ran != 1 {
		t.Errorf("out-of-range runSelected must be a no-op (%d)", ran)
	}

	rig.s.SelReq = 0
	for _, tab := range []int{0, 1} {
		rig.s.InspTab = tab
		rig.s.copySelectedReqBody(gtx)
	}
	rig.s.SelReq = 3
	rig.s.InspTab = 1
	rig.s.copySelectedReqBody(gtx)
	rig.s.SelReq = -1
	rig.s.copySelectedReqBody(gtx)

	rig.s.SelFile = 0
	rig.s.copySelectedFile(gtx)
	rig.s.SelFile = -1
	rig.s.copySelectedFile(gtx)
}

func TestLayout_TopTabButtonHandlers(t *testing.T) {
	cases := []struct {
		name string
		btn  func(*Section) *widget.Clickable
		want int
	}{
		{"requests", func(s *Section) *widget.Clickable { return &s.TabReq }, TabRequests},
		{"files", func(s *Section) *widget.Clickable { return &s.TabFiles }, TabFiles},
		{"pages", func(s *Section) *widget.Clickable { return &s.TabPages }, TabPages},
		{"info", func(s *Section) *widget.Clickable { return &s.TabInfo }, TabInfo},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newRig(t, harBigDoc, image.Pt(1200, 700))
			rig.frames(2)
			rig.s.TopTab = -1
			tc.btn(rig.s).Click()
			rig.frame()
			if rig.s.TopTab != tc.want {
				t.Errorf("TopTab = %d, want %d", rig.s.TopTab, tc.want)
			}
		})
	}
}

func TestLayout_InspectorTabButtonHandlers(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.frames(2)

	rig.s.InspTabResp.Click()
	rig.frame()
	if rig.s.InspTab != 1 {
		t.Errorf("InspTab = %d, want 1 after clicking Response", rig.s.InspTab)
	}
	rig.s.InspTabReq.Click()
	rig.frame()
	if rig.s.InspTab != 0 {
		t.Errorf("InspTab = %d, want 0 after clicking Request", rig.s.InspTab)
	}
}

func TestLayout_PrettyButtonToggles(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1
	rig.frames(2)

	if rig.s.Pretty {
		t.Fatal("precondition: Pretty must start off")
	}
	rig.s.PrettyBtn.Click()
	rig.frame()
	if !rig.s.Pretty {
		t.Error("Pretty must be on after one click")
	}
	rig.s.PrettyBtn.Click()
	rig.frame()
	if rig.s.Pretty {
		t.Error("Pretty must be off after a second click")
	}
}

func TestLayout_ClearButtonHandler(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.frames(2)
	rig.s.ClearBtn.Click()
	rig.frame()
	if rig.s.Doc != nil {
		t.Error("Clear must drop the document")
	}
	if rig.s.SelReq != -1 || rig.s.SelFile != -1 {
		t.Errorf("Clear must reset selections, got %d/%d", rig.s.SelReq, rig.s.SelFile)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after Clear")
	}
}

func TestLayout_BrowseButtonHandler(t *testing.T) {
	rig := newRig(t, "", image.Pt(1200, 700))
	rig.frames(2)
	var calls int32
	rig.host.ChooseHAR = func() (io.ReadCloser, error) {
		atomic.AddInt32(&calls, 1)
		return io.NopCloser(strings.NewReader(harBigDoc)), nil
	}
	rig.s.BrowseBtn.Click()
	rig.frame()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rig.s.Doc == nil {
		rig.frame()
		time.Sleep(2 * time.Millisecond)
	}
	if atomic.LoadInt32(&calls) == 0 {
		t.Fatal("Import button never invoked the chooser")
	}
	if rig.s.Doc == nil {
		t.Fatalf("Import did not load the document: %q", rig.s.Banner)
	}
}

func TestLayout_ExportButtonHandlers(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.frames(2)
	var created int32
	rig.host.CreateFile = func(string) (io.WriteCloser, error) {
		atomic.AddInt32(&created, 1)
		return &memWriteCloser{}, nil
	}
	rig.s.ExportZipBtn.Click()
	rig.frame()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&created) == 0 {
		rig.frame()
		time.Sleep(2 * time.Millisecond)
	}
	if atomic.LoadInt32(&created) == 0 {
		t.Fatal("ZIP button never invoked CreateFile")
	}
	waitBanner(t, rig.s, "Exported")
}

func TestLayout_ExportDirButtonNoopWithoutResources(t *testing.T) {
	rig := newRig(t, harNoPagesDoc, image.Pt(1200, 700))
	if len(rig.s.Resources) != 0 {
		t.Fatalf("precondition: doc must have no resources, got %d", len(rig.s.Resources))
	}
	rig.frames(2)
	rig.s.Banner = ""
	rig.s.ExportDirBtn.Click()
	rig.frames(3)
	if rig.s.Banner != "" {
		t.Errorf("Export → Folder with no resources must stay a no-op, banner=%q", rig.s.Banner)
	}
}

func TestLayout_CopyButtonHandlers(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.frames(2)
	rig.s.ReqCopyBtn.Click()
	rig.frames(2)

	rig.s.TopTab = TabFiles
	rig.s.SelFile = 0
	rig.frames(2)
	rig.s.CopyBodyBtn.Click()
	rig.frames(2)
}

func TestLayout_RunButtonHandler(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 2
	var got *har.Entry
	rig.host.RunEntry = func(e *har.Entry) { got = e }
	rig.frames(2)

	rig.s.RunBtn.Click()
	rig.frame()
	if got == nil {
		t.Fatal("Run button never invoked RunEntry")
	}
	if got != &rig.s.Doc.Entries[2] {
		t.Error("Run must pass the selected entry")
	}
}

func TestLayout_LargeBodyScrollbarAndDrag(t *testing.T) {
	body := strings.Repeat("line of response text\\n", 400)
	doc := `{"log":{"version":"1.2","entries":[
      {"request":{"method":"GET","url":"https://x/big"},
       "response":{"status":200,"content":{"mimeType":"text/plain","text":"` + body + `"}}}
    ]}}`
	rig := newRig(t, doc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1
	rig.frames(3)

	before := rig.s.ReqViewer.GetScrollY()
	x := float32(rig.sz.X - 5)
	dragged := false
	for y := float32(360); y < 690 && !dragged; y += 6 {
		rig.press(x, y)
		rig.move(x, y+120)
		rig.release(x, y+120)
		if rig.s.ReqViewer.GetScrollY() != before {
			dragged = true
		}
	}
	if !dragged {
		t.Error("dragging the body scrollbar never changed the scroll position")
	}
	if rig.s.ReqViewer.GetScrollY() < 0 {
		t.Errorf("scroll position went negative: %d", rig.s.ReqViewer.GetScrollY())
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after the scrollbar drag")
	}
}

func TestLayout_CopyPrefersSelectedText(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1
	rig.frames(3)
	rig.s.ReqViewer.SetCaret(0, 5)
	if rig.s.ReqViewer.SelectedText() == "" {
		t.Fatal("precondition: viewer must report a selection")
	}
	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.copySelectedReqBody(gtx)

	rig.s.TopTab = TabFiles
	rig.s.SelFile = 0
	rig.frames(3)
	rig.s.FileViewer.SetCaret(0, 3)
	if rig.s.FileViewer.SelectedText() == "" {
		t.Fatal("precondition: file viewer must report a selection")
	}
	gtx = testGtx(&rig.r, rig.sz, rig.now)
	rig.s.copySelectedFile(gtx)
}

func TestLayout_ToolbarShowsErrorBanner(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.frames(2)
	rig.s.ApplyLoad([]byte("not a har"), "bad.har", nil)
	if !rig.s.BannerErr {
		t.Fatal("precondition: a parse failure must set BannerErr")
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke while showing an error banner")
	}

	rig.s.Banner, rig.s.BannerErr, rig.s.Source = "", false, ""
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke with an empty status")
	}
}

func TestLayout_RunSelectedWithoutHostCallback(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1200, 700))
	rig.frames(2)
	rig.host.RunEntry = nil
	rig.s.SelReq = 0
	rig.s.runSelected()
}
