package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"
	"time"

	harui "tracto/internal/ui/har"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
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

func harTestUI(t *testing.T) *AppUI {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	ui.SidebarSection = "har"
	return ui
}

func harTestGtx(r *input.Router, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Source:      r.Source(),
	}
}

func layoutHARTwice(r *input.Router, sz image.Point, w func(layout.Context) layout.Dimensions) layout.Dimensions {
	var dims layout.Dimensions
	for i := 0; i < 2; i++ {
		gtx := harTestGtx(r, sz)
		dims = w(gtx)
		r.Frame(gtx.Ops)
	}
	return dims
}

func TestHARSection_EmptyStateRenders(t *testing.T) {
	ui := harTestUI(t)
	var r input.Router
	sz := image.Pt(1000, 600)

	dims := layoutHARTwice(&r, sz, ui.LayoutHARSection)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Fatalf("empty HAR section produced no dimensions: %+v", dims.Size)
	}
}

func TestHARSection_AllTabsRenderWhenLoaded(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	if ui.HARView.Doc == nil {
		t.Fatal("precondition: doc must load")
	}

	var r input.Router
	sz := image.Pt(1100, 620)
	for _, tab := range []int{harui.TabRequests, harui.TabFiles, harui.TabPages, harui.TabInfo} {
		ui.HARView.TopTab = tab
		dims := layoutHARTwice(&r, sz, ui.LayoutHARSection)
		if dims.Size.X <= 0 || dims.Size.Y <= 0 {
			t.Errorf("tab %d produced no dimensions", tab)
		}
	}
}

func TestHARSection_RequestsTabSelectionRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	ui.HARView.TopTab = harui.TabRequests

	var r input.Router
	sz := image.Pt(1100, 620)

	ui.HARView.InspTab = 1
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("response inspector failed to render")
	}

	ui.HARView.SelReq = 999
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("out-of-range selection broke rendering")
	}
}

func TestHARSection_FilesTabPreviewRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)
	ui.HARView.TopTab = harui.TabFiles

	var r input.Router
	sz := image.Pt(1100, 620)
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("files tab failed to render")
	}

	ui.HARView.SelFile = -1
	if dims := layoutHARTwice(&r, sz, ui.LayoutHARSection); dims.Size.Y <= 0 {
		t.Fatal("files tab with no selection failed to render")
	}
}

func TestHARSection_WebSocketAndPrettyRender(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "ws.har", nil)

	var r input.Router
	sz := image.Pt(1100, 620)

	ui.HARView.SelReq = 1
	ui.HARView.InspTab = 1
	ui.HARView.Pretty = true
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("websocket inspector failed to render")
	}

	ui.HARView.TopTab = harui.TabFiles
	ui.HARView.Pretty = true
	if d := layoutHARTwice(&r, sz, ui.LayoutHARSection); d.Size.Y <= 0 {
		t.Fatal("files pretty view failed to render")
	}
}

func TestHARSection_RoutingThroughLayoutApp(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harTestDoc), "capture.har", nil)

	var r input.Router
	sz := image.Pt(1200, 700)
	for i := 0; i < 2; i++ {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Source:      r.Source(),
			Now:         time.Unix(1700000000, 0),
		}
		ui.LayoutApp(gtx)
		r.Frame(gtx.Ops)
	}
}
