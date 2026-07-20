package apptest

import (
	. "tracto/internal/ui"

	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/ui/flow"
	harui "tracto/internal/ui/har"
	"tracto/internal/ui/sidebar"
	"tracto/pkg/syntax"

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

func dropFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func libraryTestUI(t *testing.T) *AppUI {
	t.Helper()
	ui := harTestUI(t)
	ui.SidebarSection = "requests"
	ui.SidebarWidth = 260
	ui.Drop.TopY = 30
	ui.SetSidebarZones([]sidebar.DropZoneRect{
		{ID: "collections", Rect: image.Rect(36, 0, 260, 320)},
		{ID: "scripts", Rect: image.Rect(36, 320, 260, 430)},
		{ID: "variables", Rect: image.Rect(36, 430, 260, 570)},
	})
	ui.RebuildDropZones(dropGtx(image.Pt(1000, 700)))
	return ui
}

func TestDropPipeline_CollectionsZone(t *testing.T) {
	ui := libraryTestUI(t)

	p := dropFile(t, "c.json", `{"info":{"name":"Dropped Coll"},"item":[{"name":"R"}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 100))
	ui.DrainDroppedFiles()

	select {
	case col := <-ui.ColLoadedChan:
		if col == nil || col.Data == nil || col.Data.Name != "Dropped Coll" {
			t.Fatalf("collection zone imported wrong data: %+v", col)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the collections zone must import a collection")
	}
}

func TestDropPipeline_VariablesZone(t *testing.T) {
	ui := libraryTestUI(t)

	p := dropFile(t, "e.json", `{"name":"Dropped Env","values":[{"key":"k","value":"v"}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 500))
	ui.DrainDroppedFiles()

	select {
	case env := <-ui.EnvLoadedChan:
		if env == nil || env.Data == nil || env.Data.Name != "Dropped Env" {
			t.Fatalf("variables zone imported wrong data: %+v", env)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the variables zone must import an environment")
	}
}

func TestDropPipeline_ScriptsZone(t *testing.T) {
	ui := libraryTestUI(t)

	before := len(flow.ListScenarios())
	p := dropFile(t, "s.json", `{"name":"Dropped Script","nodes":[{"id":"n1","kind":1,"x":80,"y":200}]}`)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(100, 400))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(flow.ListScenarios()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the scripts zone must import a scenario")
}

func TestDropPipeline_HARZoneLoadsArchive(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.Drop.TopY = 30
	ui.RebuildDropZones(dropGtx(image.Pt(1000, 700)))

	p := dropFile(t, "drop.har", harTestDoc)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(500, 400))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the HAR zone must load the archive")
}

func TestDropPipeline_OutsideZonesFallsBackToHAR(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()

	p := dropFile(t, "drop.har", harTestDoc)
	ui.OnOSFilesDropped([]string{p}, f32.Pt(9999, 9999))
	ui.DrainDroppedFiles()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ui.HARView.DrainLoads() && ui.HARView.Doc != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("HAR section drop must still load even with no zone hit")
}

func TestHarPrettyShared(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "p.har", nil)
	ui.HARView.Pretty = true

	sz := image.Pt(1100, 620)
	render := func() {
		gtx := dropGtx(sz)
		for i := 0; i < 2; i++ {
			gtx.Ops = new(op.Ops)
			ui.LayoutHARSection(gtx)
		}
	}

	ui.HARView.TopTab = harui.TabRequests
	ui.HARView.SelReq = 0
	ui.HARView.InspTab = 1
	render()
	if !strings.Contains(ui.HARView.ReqViewerKey, "pretty=1") {
		t.Errorf("requests viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.ReqViewerKey)
	}

	ui.HARView.TopTab = harui.TabFiles
	ui.HARView.SelFile = 0
	render()
	if !strings.Contains(ui.HARView.FileViewerKey, "pretty=1") {
		t.Errorf("files viewer key = %q, want pretty=1 from the shared toggle", ui.HARView.FileViewerKey)
	}
}

func TestHarRunEntry_CarriesLangHint(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.Ensure()
	ui.HARView.ApplyLoad([]byte(harRunDoc), "x.har", nil)

	ui.HARRunEntry(&ui.HARView.Doc.Entries[0])
	rt := ui.Tabs[ui.ActiveIdx]
	if rt.ReqLangHint != syntax.LangJSON {
		t.Errorf("ReqLangHint = %v, want LangJSON so the request tab keeps the HAR colouring", rt.ReqLangHint)
	}
}
