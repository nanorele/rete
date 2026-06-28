package ui

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/ui/flow"
	"tracto/internal/ui/sidebar"
	"tracto/internal/ui/syntax"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func dropGtx(sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
	}
}

func TestRebuildDropZones_HARSection(t *testing.T) {
	ui := harTestUI(t)
	ui.dnd.topY = 30
	ui.rebuildDropZones(dropGtx(image.Pt(1000, 700)))

	if len(ui.dnd.zones) != 1 || ui.dnd.zones[0].id != "har" {
		t.Fatalf("HAR section must expose a single 'har' zone, got %+v", ui.dnd.zones)
	}
	z := ui.dnd.zones[0].rect
	if z.Min.Y != 30 || z.Max != image.Pt(1000, 700) {
		t.Errorf("har zone rect = %v, want top=30 covering the window", z)
	}
}

func TestRebuildDropZones_LibraryUsesSidebarBands(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.SidebarWidth = 260
	ui.dnd.topY = 30
	ui.sidebarZones = []sidebar.DropZoneRect{
		{ID: "collections", Rect: image.Rect(36, 0, 260, 320)},
		{ID: "scripts", Rect: image.Rect(36, 320, 260, 430)},
		{ID: "variables", Rect: image.Rect(36, 430, 260, 570)},
	}
	ui.rebuildDropZones(dropGtx(image.Pt(1000, 700)))

	if len(ui.dnd.zones) != 3 {
		t.Fatalf("library sidebar must expose 3 zones, got %d", len(ui.dnd.zones))
	}
	want := []struct {
		id   string
		rect image.Rectangle
	}{
		{"collections", image.Rect(36, 30, 260, 350)},
		{"scripts", image.Rect(36, 350, 260, 460)},
		{"variables", image.Rect(36, 460, 260, 600)},
	}
	for i, w := range want {
		if ui.dnd.zones[i].id != w.id || ui.dnd.zones[i].rect != w.rect {
			t.Errorf("zone %d = %q %v, want %q %v", i, ui.dnd.zones[i].id, ui.dnd.zones[i].rect, w.id, w.rect)
		}
	}
}

func TestRebuildDropZones_NoneForMITM(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "mitm"
	ui.dnd.topY = 30
	ui.rebuildDropZones(dropGtx(image.Pt(1000, 700)))
	if len(ui.dnd.zones) != 0 {
		t.Errorf("MITM sidebar must not host library zones, got %+v", ui.dnd.zones)
	}
}

func TestRebuildDropZones_NoneWhenSidebarHidden(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.Settings.HideSidebar = true
	ui.dnd.topY = 30
	ui.rebuildDropZones(dropGtx(image.Pt(1000, 700)))
	if len(ui.dnd.zones) != 0 {
		t.Errorf("a hidden sidebar must expose no library zones, got %+v", ui.dnd.zones)
	}
}

func TestZoneAt(t *testing.T) {
	ui := harTestUI(t)
	ui.dnd.zones = []dropZone{
		{id: "collections", rect: image.Rect(0, 0, 100, 100)},
		{id: "variables", rect: image.Rect(0, 100, 100, 200)},
	}
	if got := ui.zoneAt(f32.Pt(50, 50)); got != "collections" {
		t.Errorf("zoneAt(50,50) = %q, want collections", got)
	}
	if got := ui.zoneAt(f32.Pt(50, 150)); got != "variables" {
		t.Errorf("zoneAt(50,150) = %q, want variables", got)
	}
	if got := ui.zoneAt(f32.Pt(500, 500)); got != "" {
		t.Errorf("zoneAt outside zones = %q, want empty", got)
	}
}

func TestOnOSFilesDragged(t *testing.T) {
	ui := harTestUI(t)
	ui.onOSFilesDragged(f32.Pt(12, 34), true)
	ui.dnd.mu.Lock()
	active, pos := ui.dnd.active, ui.dnd.pos
	ui.dnd.mu.Unlock()
	if !active || pos != (f32.Point{X: 12, Y: 34}) {
		t.Errorf("drag state = active %v pos %v, want true (12,34)", active, pos)
	}
	ui.onOSFilesDragged(f32.Point{}, false)
	ui.dnd.mu.Lock()
	active = ui.dnd.active
	ui.dnd.mu.Unlock()
	if active {
		t.Error("drag-leave must clear the active flag")
	}
}

func TestLayoutDropOverlay_NoPanicWhenActive(t *testing.T) {
	ui := harTestUI(t)
	ui.dnd.zones = []dropZone{
		{id: "collections", label: "Collections", rect: image.Rect(36, 30, 260, 250)},
		{id: "variables", label: "Variables", rect: image.Rect(36, 250, 260, 470)},
	}
	ui.dnd.active = true
	ui.dnd.pos = f32.Pt(100, 100)
	ui.layoutDropOverlay(dropGtx(image.Pt(1000, 700)))
}

func dropFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRouteDroppedFiles_CollectionsZone(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.dnd.zones = []dropZone{{id: "collections", rect: image.Rect(0, 0, 100, 100)}}

	p := dropFile(t, "c.json", `{"info":{"name":"Dropped Coll"},"item":[{"name":"R"}]}`)
	ui.routeDroppedFiles(droppedPayload{paths: []string{p}, pos: f32.Pt(50, 50)})

	select {
	case col := <-ui.ColLoadedChan:
		if col == nil || col.Data == nil || col.Data.Name != "Dropped Coll" {
			t.Fatalf("collection zone imported wrong data: %+v", col)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the collections zone must import a collection")
	}
}

func TestRouteDroppedFiles_VariablesZone(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.dnd.zones = []dropZone{{id: "variables", rect: image.Rect(0, 0, 100, 100)}}

	p := dropFile(t, "e.json", `{"name":"Dropped Env","values":[{"key":"k","value":"v"}]}`)
	ui.routeDroppedFiles(droppedPayload{paths: []string{p}, pos: f32.Pt(50, 50)})

	select {
	case env := <-ui.EnvLoadedChan:
		if env == nil || env.Data == nil || env.Data.Name != "Dropped Env" {
			t.Fatalf("variables zone imported wrong data: %+v", env)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the variables zone must import an environment")
	}
}

func TestRouteDroppedFiles_ScriptsZone(t *testing.T) {
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.dnd.zones = []dropZone{{id: "scripts", rect: image.Rect(0, 0, 100, 100)}}

	before := len(flow.ListScenarios())
	p := dropFile(t, "s.json", `{"name":"Dropped Script","nodes":[{"id":"n1","kind":1,"x":80,"y":200}]}`)
	ui.routeDroppedFiles(droppedPayload{paths: []string{p}, pos: f32.Pt(50, 50)})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(flow.ListScenarios()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the scripts zone must import a scenario")
}

func TestRouteDroppedFiles_HARZoneLoadsArchive(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.dnd.zones = []dropZone{{id: "har", rect: image.Rect(0, 0, 1000, 700)}}

	p := dropFile(t, "drop.har", harTestDoc)
	ui.routeDroppedFiles(droppedPayload{paths: []string{p}, pos: f32.Pt(500, 400)})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.drainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the HAR zone must load the archive")
}

func TestRouteDroppedFiles_OutsideZonesFallsBackToHAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()

	p := dropFile(t, "drop.har", harTestDoc)
	ui.routeDroppedFiles(droppedPayload{paths: []string{p}, pos: f32.Pt(9999, 9999)})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.drainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("HAR section drop must still load even with no zone hit")
}

func TestHarPrettyShared(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "p.har", nil)
	ui.HARView.Pretty = true

	sz := image.Pt(1100, 620)
	render := func() {
		gtx := dropGtx(sz)
		for i := 0; i < 2; i++ {
			gtx.Ops = new(op.Ops)
			ui.layoutHARSection(gtx)
		}
	}

	ui.HARView.TopTab = harTabRequests
	ui.HARView.SelReq = 0
	ui.HARView.InspTab = 1
	render()
	if !strings.Contains(ui.HARView.ReqViewerKey, "pretty=1") {
		t.Errorf("requests viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.ReqViewerKey)
	}

	ui.HARView.TopTab = harTabFiles
	ui.HARView.SelFile = 0
	render()
	if !strings.Contains(ui.HARView.FileViewerKey, "pretty=1") {
		t.Errorf("files viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.FileViewerKey)
	}
}

func TestHarRunEntry_CarriesLangHint(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harRunDoc), "x.har", nil)

	ui.harRunEntry(&ui.HARView.Doc.Entries[0])
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.ReqLangHint != syntax.LangJSON {
		t.Errorf("ReqLangHint = %v, want LangJSON so the request tab keeps the HAR colouring", rt.ReqLangHint)
	}
}
