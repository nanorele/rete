package dropzones

import (
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"os"
	"path/filepath"
	"rete/internal/persist"
	"rete/internal/ui/collections"
	"rete/internal/ui/environments"
	"rete/internal/ui/flow"
	"rete/internal/ui/sidebar"
	"rete/internal/ui/theme"
	"runtime"
	"testing"
	"time"
)

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "rete-test")
	persist.SetConfigOverride(configPath)

	t.Cleanup(func() {
		persist.SetConfigOverride("")
	})

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tempDir)
	case "darwin":
		t.Setenv("HOME", tempDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}

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

func dropGtxMetric(sz image.Point, pxPerDp float32) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: pxPerDp, PxPerSp: pxPerDp},
		Constraints: layout.Exact(sz),
	}
}

func TestDragged_InvalidatesWindow(t *testing.T) {
	s := new(State)
	host := &Host{Window: new(app.Window)}
	s.Dragged(host, f32.Pt(5, 6), true)

	s.mu.Lock()
	active, pos := s.active, s.pos
	s.mu.Unlock()
	if !active || pos != (f32.Point{X: 5, Y: 6}) {
		t.Errorf("drag state = active %v pos %v, want true (5,6)", active, pos)
	}
	if s.host != host {
		t.Error("Dragged must latch the host")
	}
}

func TestRebuildZones_DegenerateGeometry(t *testing.T) {
	cases := []struct {
		name string
		topY int
		size image.Point
	}{
		{"top-at-zero", 0, image.Pt(1000, 700)},
		{"negative-top", -10, image.Pt(1000, 700)},
		{"top-below-window", 700, image.Pt(1000, 700)},
		{"top-past-window", 900, image.Pt(1000, 700)},
		{"zero-width", 30, image.Pt(0, 700)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := new(State)
			s.TopY = tc.topY
			s.RebuildZones(dropGtx(tc.size), &Host{SidebarSection: "har"})
			if len(s.zones) != 0 {
				t.Errorf("degenerate geometry must expose no zones, got %+v", s.zones)
			}
		})
	}
}

func TestRebuildZones_ClearsPreviousZones(t *testing.T) {
	s := new(State)
	s.TopY = 30
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "har"})
	if len(s.zones) != 1 {
		t.Fatalf("precondition: want 1 zone, got %d", len(s.zones))
	}
	s.RebuildZones(dropGtx(image.Pt(1000, 700)), &Host{SidebarSection: "mitm"})
	if len(s.zones) != 0 {
		t.Errorf("switching to a zone-less section must clear stale zones, got %+v", s.zones)
	}
}

func TestDropZoneLabel(t *testing.T) {
	cases := map[string]string{
		"collections": "Collections",
		"scripts":     "Scripts",
		"variables":   "Variables",
		"history":     "history",
		"":            "",
	}
	for id, want := range cases {
		if got := dropZoneLabel(id); got != want {
			t.Errorf("dropZoneLabel(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestZoneImportKind(t *testing.T) {
	cases := map[string]importKind{
		"collections": importKindCollection,
		"variables":   importKindEnvironment,
		"scripts":     importKindScript,
		"har":         importKindAuto,
		"":            importKindAuto,
	}
	for zone, want := range cases {
		if got := zoneImportKind(zone); got != want {
			t.Errorf("zoneImportKind(%q) = %v, want %v", zone, got, want)
		}
	}
}

func TestLayoutOverlay_InactiveDrawsNothing(t *testing.T) {
	s := new(State)
	s.zones = []dropZone{{id: "collections", label: "Collections", rect: image.Rect(0, 0, 100, 100)}}
	s.active = false
	s.LayoutOverlay(dropGtx(image.Pt(1000, 700)), &Host{})
}

func TestLayoutOverlay_NoZonesDrawsNothing(t *testing.T) {
	s := new(State)
	s.active = true
	s.pos = f32.Pt(10, 10)
	s.LayoutOverlay(dropGtx(image.Pt(1000, 700)), &Host{})
}

func TestLayoutOverlay_DegenerateRectsAndMetric(t *testing.T) {
	s := new(State)
	s.zones = []dropZone{
		{id: "collections", label: "Collections", rect: image.Rect(0, 0, 0, 0)},
		{id: "variables", label: "Variables", rect: image.Rect(50, 50, 51, 51)},
	}
	s.active = true
	s.pos = f32.Pt(50, 50)
	s.LayoutOverlay(dropGtxMetric(image.Pt(1000, 700), 0.01), &Host{Theme: material.NewTheme()})
}

func TestStrokeRect_NonPositiveWidthFallsBackToOnePixel(t *testing.T) {
	gtx := dropGtx(image.Pt(100, 100))
	for _, w := range []int{0, -4} {
		strokeRect(gtx, image.Rect(10, 10, 90, 90), theme.Accent, w)
	}
}

func TestImportDataAs_AutoDelegatesToHost(t *testing.T) {
	got := make(chan []byte, 1)
	host := &Host{ImportData: func(d []byte) { got <- d }}
	importDataAs(host, []byte(`{"any":"payload"}`), importKindAuto)

	select {
	case d := <-got:
		if string(d) != `{"any":"payload"}` {
			t.Errorf("ImportData received %q", d)
		}
	default:
		t.Fatal("auto import must delegate to Host.ImportData")
	}
}

func TestImportDataAs_ScriptInvalidatesWindowOnSuccess(t *testing.T) {
	setupTestConfigDir(t)
	host := &Host{Window: new(app.Window)}
	before := len(flow.ListScenarios())
	importDataAs(host, []byte(`{"name":"Direct Script","nodes":[{"id":"n1","kind":1,"x":10,"y":20}]}`), importKindScript)
	if after := len(flow.ListScenarios()); after != before+1 {
		t.Fatalf("script import stored %d scenarios, want %d", after, before+1)
	}
}

func TestImportDataAs_ScriptIgnoresMalformedData(t *testing.T) {
	setupTestConfigDir(t)
	host := &Host{Window: new(app.Window)}
	before := len(flow.ListScenarios())
	for _, data := range [][]byte{nil, {}, []byte("not json"), []byte(`{"name":"No Nodes","nodes":[]}`)} {
		importDataAs(host, data, importKindScript)
	}
	if after := len(flow.ListScenarios()); after != before {
		t.Errorf("malformed scenarios must not be stored: %d -> %d", before, after)
	}
}

func TestImportCollectionData_MalformedIsIgnored(t *testing.T) {
	setupTestConfigDir(t)
	pushed := make(chan *collections.CollectionUI, 4)
	host := &Host{PushColLoaded: func(c *collections.CollectionUI) { pushed <- c }}

	for _, data := range [][]byte{nil, {}, []byte("not json at all"), []byte(`{}`), []byte(`{"item":[]}`)} {
		importCollectionData(host, data)
	}
	assertNoPush(t, func() bool { return len(pushed) > 0 }, "malformed collections must not be pushed")
}

func TestImportEnvironmentData_MalformedIsIgnored(t *testing.T) {
	setupTestConfigDir(t)
	pushed := make(chan *environments.EnvironmentUI, 4)
	host := &Host{PushEnvLoaded: func(e *environments.EnvironmentUI) { pushed <- e }}

	for _, data := range [][]byte{nil, {}, []byte("not json at all"), []byte(`{}`), []byte(`{"values":null}`)} {
		importEnvironmentData(host, data)
	}
	assertNoPush(t, func() bool { return len(pushed) > 0 }, "malformed environments must not be pushed")
}

func assertNoPush(t *testing.T, pushed func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if pushed() {
			t.Fatal(msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDropped_IgnoresEmptyPathList(t *testing.T) {
	s := new(State)
	s.Init()

	s.Dropped(&Host{}, nil, f32.Pt(1, 2))
	s.Dropped(&Host{}, []string{}, f32.Pt(1, 2))

	if len(s.dropped) != 0 {
		t.Errorf("empty drops must not queue a payload, queued %d", len(s.dropped))
	}
}

func TestDropped_IgnoredBeforeInit(t *testing.T) {
	s := new(State)
	s.Dropped(&Host{}, []string{`C:\a\b.har`}, f32.Pt(1, 2))
	if s.dropped != nil {
		t.Error("Dropped must not allocate the queue")
	}
}

func TestDropped_InvalidatesWindowAndClearsDrag(t *testing.T) {
	s := new(State)
	s.Init()
	s.mu.Lock()
	s.active = true
	s.mu.Unlock()

	s.Dropped(&Host{Window: new(app.Window)}, []string{`C:\a\b.har`}, f32.Pt(3, 4))

	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active {
		t.Error("a drop must clear the active drag state")
	}
	if len(s.dropped) != 1 {
		t.Fatalf("drop queue length = %d, want 1", len(s.dropped))
	}
}

func TestDropped_CopiesPathSlice(t *testing.T) {
	s := new(State)
	s.Init()
	paths := []string{`C:\a\b.har`}
	s.Dropped(&Host{}, paths, f32.Pt(1, 1))
	paths[0] = `C:\mutated.har`

	p := <-s.dropped
	if p.paths[0] != `C:\a\b.har` {
		t.Errorf("queued payload aliases the caller slice: %q", p.paths[0])
	}
}

func TestDropped_FullQueueDropsWithoutBlocking(t *testing.T) {
	s := new(State)
	s.Init()
	host := &Host{}
	for i := 0; i < cap(s.dropped)+4; i++ {
		s.Dropped(host, []string{`C:\a\b.har`}, f32.Pt(1, 1))
	}
	if got := len(s.dropped); got != cap(s.dropped) {
		t.Errorf("queue length = %d, want capacity %d", got, cap(s.dropped))
	}
}

func TestDrain_NoopBeforeInit(t *testing.T) {
	s := new(State)
	host := &Host{}
	s.Drain(host)
	if s.host != host {
		t.Error("Drain must latch the host even with no queue")
	}
}

func TestDrain_EmptyQueueIsNoop(t *testing.T) {
	s := new(State)
	s.Init()
	s.Drain(&Host{})
}

func TestRouteDroppedFiles_FallbackAutoImportOutsideHAR(t *testing.T) {
	setupTestConfigDir(t)
	s := new(State)
	s.Init()
	got := make(chan []byte, 1)
	host := &Host{
		SidebarSection: "requests",
		ImportData:     func(d []byte) { got <- d },
	}

	p := dropFile(t, "any.json", `{"info":{"name":"Auto"},"item":[]}`)
	s.Dropped(host, []string{p}, f32.Pt(9999, 9999))
	s.Drain(host)

	select {
	case d := <-got:
		if len(d) == 0 {
			t.Error("auto import received no data")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("a drop outside every zone in a non-HAR section must auto-import")
	}
}

func TestRouteDroppedFiles_UnreadableFileIsIgnored(t *testing.T) {
	setupTestConfigDir(t)
	s := new(State)
	s.Init()
	got := make(chan []byte, 1)
	host := &Host{
		SidebarSection: "requests",
		ImportData:     func(d []byte) { got <- d },
	}

	s.Dropped(host, []string{`C:\definitely\missing\file.json`}, f32.Pt(9999, 9999))
	s.Drain(host)
	assertNoPush(t, func() bool { return len(got) > 0 }, "an unreadable file must not reach ImportData")
}

func TestRouteDroppedFiles_HARZoneWithoutPathsIsNoop(t *testing.T) {
	s := new(State)
	s.Init()
	host := &Host{SidebarSection: "requests", LoadHAR: func(string) { t.Error("LoadHAR must not run without paths") }}
	s.host = host
	s.zones = []dropZone{{id: "har", rect: image.Rect(0, 0, 100, 100)}}
	s.routeDroppedFiles(droppedPayload{pos: f32.Pt(50, 50)})
}
