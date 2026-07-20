package dropzones

import (
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/flow"
	"tracto/internal/ui/sidebar"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func dropGtx(sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
	}
}

func TestRebuildZones_HARSection(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "har"})

	if len(s.zones) != 1 || s.zones[0].id != "har" {
		t.Fatalf("HAR section must expose a single 'har' zone, got %+v", s.zones)
	}
	z := s.zones[0].rect
	if z.Min.Y != 30 || z.Max != image.Pt(1000, 700) {
		t.Errorf("har zone rect = %v, want top=30 covering the window", z)
	}
}

func TestRebuildZones_LibraryUsesSidebarBands(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{
		SidebarSection: "requests",
		SidebarZones: []sidebar.DropZoneRect{
			{ID: "collections", Rect: image.Rect(36, 0, 260, 320)},
			{ID: "scripts", Rect: image.Rect(36, 320, 260, 430)},
			{ID: "variables", Rect: image.Rect(36, 430, 260, 570)},
		},
	})

	if len(s.zones) != 3 {
		t.Fatalf("library sidebar must expose 3 zones, got %d", len(s.zones))
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
		if s.zones[i].id != w.id || s.zones[i].rect != w.rect {
			t.Errorf("zone %d = %q %v, want %q %v", i, s.zones[i].id, s.zones[i].rect, w.id, w.rect)
		}
	}
}

func TestRebuildZones_NoneForMITM(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "mitm"})
	if len(s.zones) != 0 {
		t.Errorf("MITM sidebar must not host library zones, got %+v", s.zones)
	}
}

func TestRebuildZones_NoneWhenSidebarHidden(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "requests", SidebarHidden: true})
	if len(s.zones) != 0 {
		t.Errorf("a hidden sidebar must expose no library zones, got %+v", s.zones)
	}
}

func TestRebuildZones_NoneWhenBlocked(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "har", Blocked: true})
	if len(s.zones) != 0 {
		t.Errorf("a blocked overlay must expose no zones, got %+v", s.zones)
	}
}

func TestZoneAt(t *testing.T) {
	s := new(State)
	s.zones = []dropZone{
		{id: "collections", rect: image.Rect(0, 0, 100, 100)},
		{id: "variables", rect: image.Rect(0, 100, 100, 200)},
	}
	if got := s.zoneAt(f32.Pt(50, 50)); got != "collections" {
		t.Errorf("zoneAt(50,50) = %q, want collections", got)
	}
	if got := s.zoneAt(f32.Pt(50, 150)); got != "variables" {
		t.Errorf("zoneAt(50,150) = %q, want variables", got)
	}
	if got := s.zoneAt(f32.Pt(500, 500)); got != "" {
		t.Errorf("zoneAt outside zones = %q, want empty", got)
	}
}

func TestDragged(t *testing.T) {
	s := new(State)
	s.Dragged(&Host{}, f32.Pt(12, 34), true)
	s.mu.Lock()
	active, pos := s.active, s.pos
	s.mu.Unlock()
	if !active || pos != (f32.Point{X: 12, Y: 34}) {
		t.Errorf("drag state = active %v pos %v, want true (12,34)", active, pos)
	}
	s.Dragged(&Host{}, f32.Point{}, false)
	s.mu.Lock()
	active = s.active
	s.mu.Unlock()
	if active {
		t.Error("drag-leave must clear the active flag")
	}
}

func TestLayoutOverlay_NoPanicWhenActive(t *testing.T) {
	s := new(State)
	s.zones = []dropZone{
		{id: "collections", label: "Collections", rect: image.Rect(36, 30, 260, 250)},
		{id: "variables", label: "Variables", rect: image.Rect(36, 250, 260, 470)},
	}
	s.active = true
	s.pos = f32.Pt(100, 100)
	s.LayoutOverlay(dropGtx(image.Pt(1000, 700)), &Host{Theme: material.NewTheme()})
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
	setupTestConfigDir(t)
	s := new(State)
	s.Init()
	got := make(chan *collections.CollectionUI, 1)
	host := &Host{
		SidebarSection: "requests",
		PushColLoaded:  func(c *collections.CollectionUI) { got <- c },
	}
	s.zones = []dropZone{{id: "collections", rect: image.Rect(0, 0, 100, 100)}}

	p := dropFile(t, "c.json", `{"info":{"name":"Dropped Coll"},"item":[{"name":"R"}]}`)
	s.Dropped(host, []string{p}, f32.Pt(50, 50))
	s.Drain(host)

	select {
	case col := <-got:
		if col == nil || col.Data == nil || col.Data.Name != "Dropped Coll" {
			t.Fatalf("collection zone imported wrong data: %+v", col)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the collections zone must import a collection")
	}
}

func TestRouteDroppedFiles_VariablesZone(t *testing.T) {
	setupTestConfigDir(t)
	s := new(State)
	s.Init()
	got := make(chan *environments.EnvironmentUI, 1)
	host := &Host{
		SidebarSection: "requests",
		PushEnvLoaded:  func(e *environments.EnvironmentUI) { got <- e },
	}
	s.zones = []dropZone{{id: "variables", rect: image.Rect(0, 0, 100, 100)}}

	p := dropFile(t, "e.json", `{"name":"Dropped Env","values":[{"key":"k","value":"v"}]}`)
	s.Dropped(host, []string{p}, f32.Pt(50, 50))
	s.Drain(host)

	select {
	case env := <-got:
		if env == nil || env.Data == nil || env.Data.Name != "Dropped Env" {
			t.Fatalf("variables zone imported wrong data: %+v", env)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("dropping on the variables zone must import an environment")
	}
}

func TestRouteDroppedFiles_ScriptsZone(t *testing.T) {
	setupTestConfigDir(t)
	s := new(State)
	s.Init()
	host := &Host{SidebarSection: "requests"}
	s.zones = []dropZone{{id: "scripts", rect: image.Rect(0, 0, 100, 100)}}

	before := len(flow.ListScenarios())
	p := dropFile(t, "s.json", `{"name":"Dropped Script","nodes":[{"id":"n1","kind":1,"x":80,"y":200}]}`)
	s.Dropped(host, []string{p}, f32.Pt(50, 50))
	s.Drain(host)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(flow.ListScenarios()) > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("dropping on the scripts zone must import a scenario")
}

func TestRouteDroppedFiles_HARZone(t *testing.T) {
	s := new(State)
	s.Init()
	loaded := make(chan string, 1)
	host := &Host{
		SidebarSection: "har",
		LoadHAR:        func(p string) { loaded <- p },
	}
	s.zones = []dropZone{{id: "har", rect: image.Rect(0, 0, 1000, 700)}}

	p := dropFile(t, "drop.har", `{"log":{"entries":[]}}`)
	s.Dropped(host, []string{p}, f32.Pt(500, 400))
	s.Drain(host)

	select {
	case got := <-loaded:
		if got != p {
			t.Errorf("HAR zone loaded %q, want %q", got, p)
		}
	default:
		t.Fatal("dropping on the HAR zone must load the archive")
	}
}

func TestRouteDroppedFiles_OutsideZonesFallsBackToHAR(t *testing.T) {
	s := new(State)
	s.Init()
	loaded := make(chan string, 1)
	host := &Host{
		SidebarSection: "har",
		LoadHAR:        func(p string) { loaded <- p },
	}

	p := dropFile(t, "drop.har", `{"log":{"entries":[]}}`)
	s.Dropped(host, []string{p}, f32.Pt(9999, 9999))
	s.Drain(host)

	select {
	case got := <-loaded:
		if got != p {
			t.Errorf("fallback loaded %q, want %q", got, p)
		}
	default:
		t.Fatal("HAR section drop must still load even with no zone hit")
	}
}

func TestFirstHARPathAndExt(t *testing.T) {
	if got := firstHARPath([]string{`C:\a\b.txt`, `C:\a\c.har`}); got != `C:\a\c.har` {
		t.Errorf("firstHARPath preferred = %q", got)
	}
	if got := firstHARPath([]string{`C:\a\b.txt`}); got != `C:\a\b.txt` {
		t.Errorf("firstHARPath fallback = %q", got)
	}
	if got := firstHARPath(nil); got != "" {
		t.Errorf("firstHARPath empty = %q", got)
	}
	if got := filepathExt(`C:\dir.x\file.har`); got != ".har" {
		t.Errorf("filepathExt = %q", got)
	}
	if got := filepathExt(`C:\dir.x\noext`); got != "" {
		t.Errorf("filepathExt no-ext = %q", got)
	}
}
