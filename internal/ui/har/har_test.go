package har

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"rete/internal/har"
	"rete/internal/ui/theme"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	if !strings.Contains(out, "[binary]") || !strings.Contains(out, "00000000  00 01 02 03 04 05") || !strings.Contains(out, "|......|") {
		t.Errorf("binary frame not hex-dumped:\n%s", out)
	}
	if strings.Contains(out, "\n\n\n") {
		t.Errorf("hex-dumped frame must not open a double blank line:\n%q", out)
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

const harPagesDoc = `{
  "log": {
    "version": "1.2",
    "pages": [
      {"id":"page_1","title":"Home","startedDateTime":"2024-01-01T10:00:00Z"},
      {"id":"page_2","title":"About","startedDateTime":"2024-01-01T10:01:00Z"}
    ],
    "entries": [
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/a"},"response":{"status":200,"content":{"mimeType":"text/html","text":"a"}}},
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/b"},"response":{"status":200,"content":{"mimeType":"text/css","text":"b"}}},
      {"pageref":"page_2","request":{"method":"GET","url":"https://x/c"},"response":{"status":200,"content":{"mimeType":"application/json","text":"c"}}}
    ]
  }
}`

func harPagesState(t *testing.T) *Section {
	t.Helper()
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harPagesDoc), "p.har", nil)
	if st.Doc == nil || len(st.Doc.Pages) != 2 || len(st.Doc.Entries) != 3 {
		t.Fatalf("precondition: doc must load 2 pages / 3 entries, got %+v", st.Doc)
	}
	return st
}

func TestHarPages_FilterByPage(t *testing.T) {
	st := harPagesState(t)

	if got := st.visibleIndices(); len(got) != 3 {
		t.Fatalf("no filter must show all 3 entries, got %v", got)
	}
	st.selectPage("page_1")
	if got := st.visibleIndices(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("page_1 filter = %v, want [0 1]", got)
	}
	st.selectPage("page_2")
	if got := st.visibleIndices(); len(got) != 1 || got[0] != 2 {
		t.Errorf("page_2 filter = %v, want [2]", got)
	}
	st.selectPage("")
	if got := st.visibleIndices(); len(got) != 3 {
		t.Errorf("cleared filter = %v, want all 3", got)
	}
}

func TestHarPages_SelectMovesSelection(t *testing.T) {
	st := harPagesState(t)
	st.selectPage("page_2")
	if st.SelReq != 2 {
		t.Errorf("SelReq = %d, want 2 (first entry of page_2)", st.SelReq)
	}
	st.selectPage("")
	if st.SelReq != 0 {
		t.Errorf("SelReq after reset = %d, want 0", st.SelReq)
	}
}

func TestHarReqLabel_ReflectsPageFilter(t *testing.T) {
	st := harPagesState(t)
	if got := reqLabel(st); got != "3" {
		t.Errorf("unfiltered req label = %q, want 3", got)
	}
	st.selectPage("page_1")
	if got := reqLabel(st); got != "2" {
		t.Errorf("page_1 req label = %q, want 2", got)
	}
	if got := pagesLabel(st); got != "2" {
		t.Errorf("pages label = %q, want 2", got)
	}
}

func TestHarPageRequestCount(t *testing.T) {
	st := harPagesState(t)
	if got := st.pageRequestCount("page_1"); got != 2 {
		t.Errorf("page_1 count = %d, want 2", got)
	}
	if got := st.pageRequestCount("page_2"); got != 1 {
		t.Errorf("page_2 count = %d, want 1", got)
	}
}

func TestHarPages_ResetOnReload(t *testing.T) {
	st := harPagesState(t)
	st.selectPage("page_2")
	st.ApplyLoad([]byte(harPagesDoc), "p2.har", nil)
	if st.SelPageID != "" {
		t.Errorf("reload must clear the page filter, got %q", st.SelPageID)
	}
	if got := st.visibleIndices(); len(got) != 3 {
		t.Errorf("reload must show all entries, got %v", got)
	}
}

func TestSplitNeverProducesNegativeWidths(t *testing.T) {
	widths := []int{0, 1, 50, 100, 200, 300, 400, 526, 527, 600, 1000, 1920}
	for _, w := range widths {
		var s Section
		var r input.Router
		gtx := testGtx(&r, image.Pt(w, 600), time.Unix(1700000000, 0))
		leftW, handleW, rightW := s.split(gtx)

		if leftW < 0 {
			t.Errorf("totalW=%d: leftW = %d, want >= 0", w, leftW)
		}
		if rightW < 0 {
			t.Errorf("totalW=%d: rightW = %d, want >= 0", w, rightW)
		}
		if handleW < 0 {
			t.Errorf("totalW=%d: handleW = %d, want >= 0", w, handleW)
		}
		if leftW+handleW+rightW > w && w > 0 {
			t.Errorf("totalW=%d: panes sum to %d, wider than the window", w, leftW+handleW+rightW)
		}
	}
}

func TestSplitKeepsMinimumWidthWhenRoomAllows(t *testing.T) {
	var s Section
	var r input.Router
	gtx := testGtx(&r, image.Pt(1920, 600), time.Unix(1700000000, 0))
	leftW, _, rightW := s.split(gtx)
	if leftW < 240 {
		t.Errorf("leftW = %d, want at least the 240 minimum on a wide window", leftW)
	}
	if rightW < 280 {
		t.Errorf("rightW = %d, want at least 280 reserved on a wide window", rightW)
	}
}

func TestCopySelectedReqBodyNilDoc(t *testing.T) {
	var s Section
	var r input.Router
	gtx := testGtx(&r, image.Pt(800, 600), time.Unix(1700000000, 0))
	for _, sel := range []int{-1, 0, 5} {
		s.Doc = nil
		s.SelReq = sel
		s.copySelectedReqBody(gtx)
	}
}

func TestRunSelectedNilDoc(t *testing.T) {
	var s Section
	for _, sel := range []int{-1, 0, 5} {
		s.Doc = nil
		s.SelReq = sel
		s.runSelected()
	}
}

func fileIndexByMime(t *testing.T, rig *harRig, mime string) int {
	t.Helper()
	for i, r := range rig.s.Resources {
		if r.MimeType == mime {
			return i
		}
	}
	t.Fatalf("no %s resource among %d files", mime, len(rig.s.Resources))
	return -1
}

func TestHARFileSearch_StateIsPerFile(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1100, 700))
	rig.s.TopTab = TabFiles
	jsonFile := fileIndexByMime(t, rig, "application/json")
	cssFile := fileIndexByMime(t, rig, "text/css")

	rig.s.SelFile = jsonFile
	rig.frames(3)
	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.FileSearch.Open {
		t.Fatal("Ctrl+F must open the file search")
	}
	rig.s.FileSearch.Editor.SetText(`"`)
	rig.frames(3)
	_, total := rig.s.FileSearch.Position()
	if total < 2 {
		t.Fatalf("precondition: the JSON body should hold several quotes, got %d", total)
	}
	rig.s.FileSearch.NextBtn.Click()
	rig.frames(3)
	if cur, _ := rig.s.FileSearch.Position(); cur != 2 {
		t.Fatalf("precondition: expected to be on match 2, got %d", cur)
	}

	rig.s.SelFile = cssFile
	rig.frames(3)
	if rig.s.FileSearch.Open {
		t.Errorf("a file that was never searched must not inherit the open panel")
	}
	if got := rig.s.FileViewer.SelectedText(); got != "" {
		t.Errorf("the other file's match must not stay selected on this file, got %q", got)
	}

	rig.s.SelFile = jsonFile
	rig.frames(3)
	if !rig.s.FileSearch.Open || rig.s.FileSearch.Editor.Text() != `"` {
		t.Fatalf("returning to the file must restore its search, got open=%v query=%q", rig.s.FileSearch.Open, rig.s.FileSearch.Editor.Text())
	}
	if cur, n := rig.s.FileSearch.Position(); cur != 2 || n != total {
		t.Errorf("returning to the file must land on its match 2/%d, got %d/%d", total, cur, n)
	}
	if got := rig.s.FileViewer.SelectedText(); got != `"` {
		t.Errorf("the restored match must be selected again, got %q", got)
	}
}

func TestHARInspector_BinaryBodyShowsHexWithModes(t *testing.T) {
	rig := newRig(t, harBigDoc, image.Pt(1100, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 2
	rig.s.InspTab = 1
	rig.frames(3)

	text := rig.s.ReqViewer.Text()
	if !strings.HasPrefix(text, "00000000  00 01 02 03 04 05 06 07  08 09 00 00 00 00") {
		t.Fatalf("binary response must render as a hex dump, got %q", text)
	}

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.BodySearch.Open {
		t.Error("Ctrl+F must work over the hex dump")
	}
	rig.s.BodySearch.Editor.SetText("08 09")
	rig.frames(3)
	if got := rig.s.ReqViewer.SelectedText(); got != "08 09" {
		t.Errorf("search over the hex dump landed on %q", got)
	}

	rig.s.BodyBin.Btn(2).Click()
	rig.frames(3)
	if got := rig.s.ReqViewer.Text(); got != "AAECAwQFBgcICQAAAAA=" {
		t.Errorf("Base64 chip must re-render the body, got %q", got)
	}
	rig.s.BodyBin.Btn(1).Click()
	rig.frames(3)
	if got := rig.s.ReqViewer.Text(); got != "0001020304050607080900000000" {
		t.Errorf("Hex chip must re-render the body, got %q", got)
	}
}

// bigBodyHAR builds a capture whose single entry carries a response body long
// enough that a match near its end is far outside the first screenful.
func bigBodyHAR(lines int) string {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"marker\": \"MARKER-%04d\", \"pad\": \"%s\" },\n",
			i, i, strings.Repeat("long-", 20))
	}
	sb.WriteString("  ]\n}\n")
	body, _ := json.Marshal(sb.String())

	return `{"log":{"version":"1.2","entries":[
	  {"startedDateTime":"2024-01-01T10:00:00Z",
	   "request":{"method":"GET","url":"https://example.com/big.json","headers":[]},
	   "response":{"status":200,"headers":[{"name":"Content-Type","value":"application/json"}],
	     "content":{"mimeType":"application/json","text":` + string(body) + `}}}
	]}}`
}

func openBodySearch(t *testing.T, rig *harRig, query string) {
	t.Helper()
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 1 // response body; the request body of a GET is empty
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.BodySearch.Open {
		t.Fatal("Ctrl+F must open the body search on the requests tab")
	}
	rig.s.BodySearch.Editor.SetText(query)
	rig.frames(3)
}

func TestHARSearch_RevealsDeepMatch(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0350")

	v := rig.s.ReqViewer
	if got := v.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("search did not land on the match, selection = %q", got)
	}
	if v.GetScrollY() <= 0 {
		t.Errorf("a match 350 entries down must scroll the viewer, scrollY = %d", v.GetScrollY())
	}
	y, ok := v.RevealScreenY()
	if !ok {
		t.Fatal("the viewer never resolved the pending reveal")
	}
	if y < 0 || y >= rig.sz.Y {
		t.Errorf("match revealed at row %d, outside the %d-tall section", y, rig.sz.Y)
	}
}

func TestHARSearch_ClosingClearsTheMatchSelection(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0350")
	if got := rig.s.ReqViewer.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("precondition: match should be selected, got %q", got)
	}

	rig.s.BodySearch.Close(rig.s.ReqViewer)
	rig.frames(2)

	if got := rig.s.ReqViewer.SelectedText(); got != "" {
		t.Errorf("closing the search left %q selected in the inspector body", got)
	}
}

func TestHARSearch_FollowsEntryBodySwap(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0200")
	if got := rig.s.ReqViewer.SelectedText(); got != "MARKER-0200" {
		t.Fatalf("selection = %q before the swap", got)
	}

	// Pretty-printing swaps the viewer text under the open search box.
	rig.s.Pretty = !rig.s.Pretty
	rig.frames(3)

	v := rig.s.ReqViewer
	if got := v.SelectedText(); got != "MARKER-0200" {
		t.Errorf("after the body was reformatted the search still points at %q", got)
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("match not revealed after the swap: y=%d ok=%v", y, ok)
	}
}

// bodyViewer bails out before it processes or draws the search panel when the
// pane has nothing to show, so a box opened there is invisible, unclosable, and
// pops open again on the next pane that does have a body.
func TestHARSearch_NotLeftOpenOnBodylessPane(t *testing.T) {
	rig := newRig(t, bigBodyHAR(20), image.Pt(1100, 700))
	rig.s.TopTab = TabRequests
	rig.s.SelReq = 0
	rig.s.InspTab = 0 // request body of a GET: empty
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("search must not stay open over a pane that shows no body")
	}

	rig.s.InspTab = 1
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("the response pane inherited a search box the user never sees opening")
	}
}

func TestHARSearch_ClosesWhenBodyGoesAway(t *testing.T) {
	rig := newRig(t, bigBodyHAR(20), image.Pt(1100, 700))
	openBodySearch(t, rig, "MARKER-0010")
	if !rig.s.BodySearch.Open {
		t.Fatal("precondition: search open on the response body")
	}

	rig.s.InspTab = 0 // switch to the empty request body
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Error("search stayed open on a pane where it cannot be drawn or dismissed")
	}
}

func TestHARSearch_FileViewerSharesTheFix(t *testing.T) {
	rig := newRig(t, bigBodyHAR(400), image.Pt(1100, 700))
	rig.s.TopTab = TabFiles
	rig.frames(3)
	if len(rig.s.Resources) == 0 {
		t.Skip("capture produced no extractable resources")
	}
	rig.s.SelFile = 0
	rig.frames(3)

	gtx := testGtx(&rig.r, rig.sz, rig.now)
	rig.s.HandleSearchShortcut(gtx)
	if !rig.s.FileSearch.Open {
		t.Fatal("Ctrl+F must open the file search on the files tab")
	}
	rig.s.FileSearch.Editor.SetText("MARKER-0300")
	rig.frames(3)

	v := rig.s.FileViewer
	if got := v.SelectedText(); got != "MARKER-0300" {
		t.Fatalf("file viewer search landed on %q", got)
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("file match not revealed: y=%d ok=%v", y, ok)
	}
}

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
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "capture.har", nil)

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
	st := &Section{}
	st.Ensure()
	st.ApplyLoad(nil, "x", errEmptyPath)
	if st.Doc != nil {
		t.Error("Doc must stay nil on read error")
	}
	if !st.BannerErr || !strings.Contains(st.Banner, "Import failed") {
		t.Errorf("banner = %q err=%v", st.Banner, st.BannerErr)
	}
}

func TestHarApplyLoad_ParseError(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte("not a har"), "x", nil)
	if st.Doc != nil {
		t.Error("Doc must stay nil on parse error")
	}
	if !st.BannerErr || !strings.Contains(st.Banner, "valid HAR") {
		t.Errorf("banner = %q", st.Banner)
	}
}

func TestHarApplyLoad_ReplacesPreviousDoc(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "first.har", nil)
	st.SelReq = 1
	st.ApplyLoad([]byte("garbage"), "second", nil)
	if st.Doc == nil || st.Source != "first.har" {
		t.Errorf("failed load must keep previous doc; Source=%q", st.Source)
	}
	st.ApplyLoad([]byte(harTestDoc), "third.har", nil)
	if st.SelReq != 0 {
		t.Errorf("SelReq after reload = %d, want 0", st.SelReq)
	}
}

func TestHarClear(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
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
	st := &Section{}
	st.Ensure()
	st.queueLoad([]byte(harTestDoc), "queued.har", nil)
	if st.Doc != nil {
		t.Error("queueLoad must not apply until drained")
	}
	changed := st.DrainLoads()
	if !changed {
		t.Error("drainLoads must report a change")
	}
	if st.Doc == nil || st.Source != "queued.har" {
		t.Errorf("drain did not apply queued load; Source=%q", st.Source)
	}
	if st.DrainLoads() {
		t.Error("second drain must report no change")
	}
}

func TestHarQueueLoad_DoesNotBlockWhenFull(t *testing.T) {
	st := &Section{}
	st.Ensure()
	for i := 0; i < 10; i++ {
		st.queueLoad([]byte(harTestDoc), "x", nil)
	}
}

func TestHarLoadPathAsync_EmptyPath(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.LoadPathAsync("   ", nil)
	if !st.DrainLoads() {
		t.Fatal("empty path must queue an error result")
	}
	if !st.BannerErr {
		t.Errorf("empty path must set an error banner, got %q", st.Banner)
	}
}

func TestHarSortedResources_OrderedByPath(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
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
		if got := exportName(in); got != want {
			t.Errorf("exportName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHarSplitURL(t *testing.T) {
	d, f := SplitURL("https://example.com/app/main.js?x=1")
	if d != "example.com" || f != "/app/main.js?x=1" {
		t.Errorf("split = %q,%q", d, f)
	}
	d, f = SplitURL("https://host/")
	if d != "host" || f != "/" {
		t.Errorf("root split = %q,%q", d, f)
	}
}

func TestHarShortType(t *testing.T) {
	if got := shortType("application/javascript"); got != "javascript" {
		t.Errorf("shortType = %q", got)
	}
	if got := shortType("image/png"); got != "png" {
		t.Errorf("shortType = %q", got)
	}
	if got := shortType(""); got != "" {
		t.Errorf("shortType empty = %q", got)
	}
}

func TestHarInfoRows_HasHeadersAndStats(t *testing.T) {
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
	rows := infoRows(st.Doc.Summary())

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
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)

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
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
	st.clear()
	if st.rowCache != nil {
		t.Error("clear must drop rowCache")
	}
	st.ApplyLoad([]byte(harTestDoc), "y.har", nil)
	if len(st.rowCache) != 2 {
		t.Errorf("rowCache after reload = %d, want 2", len(st.rowCache))
	}
}

func TestInspectorBody_CachesUntilKeyChanges(t *testing.T) {
	st := &Section{}
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
	st := &Section{}
	st.Ensure()
	st.inspectorBody("resp/0", func() []byte { return []byte("stale") })
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
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
	st := &Section{}
	st.Ensure()
	st.ApplyLoad([]byte(harTestDoc), "x.har", nil)
	if st.infoCached {
		t.Error("infoCached must be false until the Info view is first rendered")
	}
	if !st.infoCached {
		st.infoRows = infoRows(st.Doc.Summary())
		st.infoCached = true
	}
	first := st.infoRows
	if len(first) == 0 {
		t.Fatal("infoRows empty after build")
	}
	st.ApplyLoad([]byte(harTestDoc), "y.har", nil)
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
