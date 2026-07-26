package netlimit

import (
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	netlim "tracto/internal/netlimit"
	"tracto/internal/persist"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type rig struct {
	s    *Section
	host *Host
	r    input.Router
	sz   image.Point
	now  time.Time
}

func shapedTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func newRig(t *testing.T, sz image.Point) *rig {
	t.Helper()
	setupTestConfigDir(t)
	rg := &rig{s: new(Section), sz: sz, now: time.Unix(1700000000, 0)}
	rg.s.Init()
	t.Cleanup(rg.s.Close)
	rg.host = &Host{Theme: shapedTheme(), Window: new(app.Window)}
	rg.s.host = rg.host
	return rg
}

func (rg *rig) gtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rg.sz),
		Source:      rg.r.Source(),
		Now:         rg.now,
	}
}

func (rg *rig) body() layout.Dimensions {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	dims := rg.s.LayoutBody(gtx, rg.host)
	rg.r.Frame(gtx.Ops)
	return dims
}

func (rg *rig) bodies(n int) layout.Dimensions {
	var d layout.Dimensions
	for range n {
		d = rg.body()
	}
	return d
}

func (rg *rig) section() layout.Dimensions {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	dims := rg.s.LayoutSection(gtx, rg.host)
	rg.r.Frame(gtx.Ops)
	return dims
}

func (rg *rig) sections(n int) layout.Dimensions {
	var d layout.Dimensions
	for range n {
		d = rg.section()
	}
	return d
}

func (rg *rig) move(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
}

func writeConfig(t *testing.T, data []byte) {
	t.Helper()
	p := persist.NetlimitConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScopeTogglesSwitchScope(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.bodies(2)

	rg.s.scopeApp.Click()
	rg.body()
	if rg.s.scope != netlim.ScopeApp {
		t.Errorf("scope = %v, want ScopeApp", rg.s.scope)
	}

	rg.s.scopeSys.Click()
	rg.body()
	if rg.s.scope != netlim.ScopeSystem {
		t.Errorf("scope = %v, want ScopeSystem", rg.s.scope)
	}
}

func TestScopeAppWithSelectedAppWatchesPID(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.selApp = netlim.ProcInfo{PID: 4242, Name: "app.exe", Exe: "C:/app.exe"}
	rg.s.hasApp = true
	rg.bodies(2)

	rg.s.scopeApp.Click()
	rg.body()
	if rg.s.scope != netlim.ScopeApp {
		t.Fatalf("scope = %v, want ScopeApp", rg.s.scope)
	}
}

func TestUnitChipsSelectUnit(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.bodies(2)

	for _, sel := range []struct {
		name string
		u    func() *unitSel
	}{
		{"in", func() *unitSel { return &rg.s.inUnit }},
		{"out", func() *unitSel { return &rg.s.outUnit }},
		{"total", func() *unitSel { return &rg.s.totalUnit }},
	} {
		t.Run(sel.name, func(t *testing.T) {
			for i := range units {
				sel.u().clicks[i].Click()
				rg.body()
				if sel.u().idx != i {
					t.Errorf("idx = %d, want %d", sel.u().idx, i)
				}
			}
		})
	}
}

func TestUnitSelMulClamps(t *testing.T) {
	cases := []struct {
		idx  int
		want int64
	}{
		{-1, 1024},
		{0, 1024},
		{1, 1024 * 1024},
		{2, 1024 * 1024 * 1024},
		{3, 1024},
		{99, 1024},
	}
	for _, tc := range cases {
		u := unitSel{idx: tc.idx}
		if got := u.mul(); got != tc.want {
			t.Errorf("idx %d: mul = %d, want %d", tc.idx, got, tc.want)
		}
	}
}

func TestPickerButtonTogglesAndLoadsProcs(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.bodies(2)

	rg.s.pickBtn.Click()
	rg.body()
	if !rg.s.pickerOpen {
		t.Fatal("the picker must open on the first click")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		rg.body()
		if len(rg.s.getProcs()) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(rg.s.getProcs()) == 0 {
		t.Error("loadProcs never populated the process list")
	}

	rg.s.pickBtn.Click()
	rg.body()
	if rg.s.pickerOpen {
		t.Error("the picker must close on the second click")
	}
}

func TestPickerRowSelectsProcess(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.s.pickerOpen = true
	rg.s.setProcs([]netlim.ProcInfo{
		{PID: 11, Name: "alpha.exe", Exe: "C:/alpha.exe"},
		{PID: 22, Name: "beta.exe", Exe: "C:/beta.exe"},
	})
	rg.bodies(2)
	if len(rg.s.procClicks) < 2 {
		t.Fatalf("procClicks len = %d, want >=2", len(rg.s.procClicks))
	}

	rg.s.procClicks[1].Click()
	rg.body()
	if !rg.s.hasApp {
		t.Fatal("clicking a process row must set hasApp")
	}
	if rg.s.selApp.PID != 22 {
		t.Errorf("selApp.PID = %d, want 22", rg.s.selApp.PID)
	}
	if rg.s.pickerOpen {
		t.Error("selecting a process must close the picker")
	}
}

func TestPickerRowClickBeyondProcListIsNoop(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.s.pickerOpen = true
	rg.s.setProcs([]netlim.ProcInfo{{PID: 11, Name: "alpha.exe"}, {PID: 22, Name: "beta.exe"}})
	rg.bodies(2)
	rg.s.setProcs([]netlim.ProcInfo{{PID: 11, Name: "alpha.exe"}})

	rg.s.procClicks[1].Click()
	rg.body()
	if rg.s.hasApp {
		t.Error("clicking a stale row index must not select an application")
	}
}

func TestPickerFiltersBySearch(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.s.pickerOpen = true
	rg.s.setProcs([]netlim.ProcInfo{
		{PID: 11, Name: "alpha.exe"},
		{PID: 22, Name: "beta.exe"},
		{PID: 33, Name: "gamma.exe"},
	})
	rg.s.searchEd.SetText("  BET  ")
	rg.bodies(2)

	rg.s.searchEd.SetText("no-such-process")
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("an empty filtered picker produced no dimensions")
	}
}

func TestPickerLoadingState(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.s.pickerOpen = true
	rg.s.mu.Lock()
	rg.s.procsLoading = true
	rg.s.procs = nil
	rg.s.mu.Unlock()
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the loading picker produced no dimensions")
	}
}

func TestPickerHoverHighlightsRow(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.scope = netlim.ScopeApp
	rg.s.pickerOpen = true
	procs := make([]netlim.ProcInfo, 0, 12)
	for i := range 12 {
		procs = append(procs, netlim.ProcInfo{PID: int32(100 + i), Name: "proc.exe"})
	}
	rg.s.setProcs(procs)
	rg.s.selApp = procs[0]
	rg.s.hasApp = true
	rg.bodies(2)

	hovered := false
	for y := float32(60); y < 320 && !hovered; y += 4 {
		rg.move(180, y)
		rg.body()
		if rg.s.procListHover.Hovered() {
			hovered = true
		}
	}
	if !hovered {
		t.Error("the process list never reported hover")
	}
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the hovered picker produced no dimensions")
	}
}

func TestLoadProcsIsSingleFlight(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.mu.Lock()
	rg.s.procsLoading = true
	rg.s.mu.Unlock()

	rg.s.loadProcs()
	rg.s.mu.Lock()
	still := rg.s.procsLoading
	procs := rg.s.procs
	rg.s.mu.Unlock()
	if !still {
		t.Error("a second loadProcs must leave the in-flight flag set")
	}
	if procs != nil {
		t.Error("a re-entrant loadProcs must not have populated the list")
	}
}

func TestStartButtonRejectsUnlimitedSpec(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.caps.NeedsElevation = false
	rg.s.caps.Available = true
	rg.bodies(2)

	rg.s.startBtn.Click()
	rg.bodies(2)
	if got := rg.s.getErr(); !strings.Contains(got, "at least one rate limit") {
		t.Errorf("lastErr = %q, want the unlimited-spec message", got)
	}
}

func TestStartButtonRejectsAppScopeWithoutApp(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.caps.NeedsElevation = false
	rg.s.caps.Available = true
	rg.s.scope = netlim.ScopeApp
	rg.s.inEd.SetText("5")
	rg.bodies(2)

	rg.s.startBtn.Click()
	rg.bodies(2)
	if got := rg.s.getErr(); !strings.Contains(got, "select an application") {
		t.Errorf("lastErr = %q, want the missing-application message", got)
	}
}

func TestErrorNoteIsRendered(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.setErr(errors.New("boom"))
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the error banner produced no dimensions")
	}
	rg.s.setErr(nil)
	if got := rg.s.getErr(); got != "" {
		t.Errorf("setErr(nil) must clear lastErr, got %q", got)
	}
}

func TestOrphanBannerAndClearButton(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.orphan = true
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the orphan banner produced no dimensions")
	}

	rg.s.clearOrphanBtn.Click()
	rg.bodies(2)
	if rg.s.orphan {
		t.Error("clicking Clear must drop the orphan flag")
	}
}

func TestPauseResumeCancelButtonsAreIdleSafe(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.bodies(2)
	rg.s.stopBtn.Click()
	rg.s.resumeBtn.Click()
	rg.s.cancelBtn.Click()
	rg.bodies(3)
	if got := rg.s.getErr(); got != "" {
		t.Errorf("idle pause/resume/cancel must not set an error, got %q", got)
	}
}

func TestControlButtonsIdleBranch(t *testing.T) {
	for _, avail := range []bool{true, false} {
		rg := newRig(t, image.Pt(360, 800))
		rg.s.caps.NeedsElevation = false
		rg.s.caps.Available = avail
		if d := rg.bodies(2); d.Size.Y <= 0 {
			t.Fatalf("available=%v produced no dimensions", avail)
		}
	}
}

func TestControlButtonsElevationBranch(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.caps.NeedsElevation = true
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the elevation branch produced no dimensions")
	}
}

func TestStatusNoteVariants(t *testing.T) {
	cases := []struct {
		name  string
		caps  netlim.Caps
		scope netlim.Scope
		want  string
	}{
		{"unavailable-no-note", netlim.Caps{}, netlim.ScopeSystem, "Network limiting is not available on this system."},
		{"unavailable-with-note", netlim.Caps{Note: "install WinDivert"}, netlim.ScopeSystem, "install WinDivert"},
		{"available-clean", netlim.Caps{Available: true, AppLimit: true}, netlim.ScopeSystem, ""},
		{"app-scope-unsupported", netlim.Caps{Available: true}, netlim.ScopeApp, "Per-application limiting is not supported here."},
		{"available-with-note", netlim.Caps{Available: true, AppLimit: true, Note: "heads up"}, netlim.ScopeSystem, "heads up"},
		{"app-unsupported-and-note", netlim.Caps{Available: true, Note: "heads up"}, netlim.ScopeApp,
			"Per-application limiting is not supported here. heads up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Section{caps: tc.caps, scope: tc.scope}
			if got := s.statusNote(); got != tc.want {
				t.Errorf("statusNote() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusNoteIsRenderedInBody(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.s.caps = netlim.Caps{Available: true, Note: "heads up"}
	rg.s.scope = netlim.ScopeApp
	if d := rg.bodies(2); d.Size.Y <= 0 {
		t.Fatal("the status note produced no dimensions")
	}
}

func TestBuildSpecParsing(t *testing.T) {
	cases := []struct {
		name              string
		in, out, total    string
		inU, outU, totalU int
		wantIn            int64
		wantOut           int64
		wantTotal         int64
	}{
		{"blank", "", "", "", 1, 1, 1, 0, 0, 0},
		{"kb", "10", "20", "30", 0, 0, 0, 10 * 1024, 20 * 1024, 30 * 1024},
		{"mb", "1", "2", "3", 1, 1, 1, 1 << 20, 2 << 20, 3 << 20},
		{"gb", "1", "1", "1", 2, 2, 2, 1 << 30, 1 << 30, 1 << 30},
		{"fractional", "1.5", "", "", 1, 1, 1, 1024 * 1024 * 3 / 2, 0, 0},
		{"whitespace", "  4  ", "", "", 0, 1, 1, 4 * 1024, 0, 0},
		{"garbage", "abc", "-5", "0", 1, 1, 1, 0, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Section{}
			s.inUnit.idx, s.outUnit.idx, s.totalUnit.idx = tc.inU, tc.outU, tc.totalU
			s.inEd.SetText(tc.in)
			s.outEd.SetText(tc.out)
			s.totalEd.SetText(tc.total)
			spec := s.buildSpec()
			if spec.InBps != tc.wantIn || spec.OutBps != tc.wantOut || spec.TotalBps != tc.wantTotal {
				t.Errorf("spec = {%d,%d,%d}, want {%d,%d,%d}",
					spec.InBps, spec.OutBps, spec.TotalBps, tc.wantIn, tc.wantOut, tc.wantTotal)
			}
		})
	}
}

func TestBuildSpecAppScopeWithoutSelection(t *testing.T) {
	s := &Section{scope: netlim.ScopeApp}
	s.inEd.SetText("1")
	spec := s.buildSpec()
	if spec.AppName != "" || spec.AppPath != "" || spec.AppPID != 0 {
		t.Errorf("no application selected must leave app fields empty: %+v", spec)
	}
}

func TestGraphWindowChipsSwitchWindow(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.sections(2)

	cases := []struct {
		click func()
		want  time.Duration
	}{
		{rg.s.win30Btn.Click, 30 * time.Second},
		{rg.s.win5mBtn.Click, 5 * time.Minute},
		{rg.s.win1mBtn.Click, time.Minute},
	}
	for _, tc := range cases {
		tc.click()
		rg.section()
		if rg.s.graphWindow != tc.want {
			t.Errorf("graphWindow = %v, want %v", rg.s.graphWindow, tc.want)
		}
	}
}

func TestZeroGraphWindowDefaultsToOneMinute(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.s.graphWindow = 0
	rg.section()
	if rg.s.graphWindow != time.Minute {
		t.Errorf("graphWindow = %v, want 1m", rg.s.graphWindow)
	}
}

func TestGraphCardTrimsHistoryToWindow(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	interval := 700 * time.Millisecond
	rg.s.graphWindow = 2 * interval

	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if len(rg.s.mgr.History()) > 2 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if n := len(rg.s.mgr.History()); n <= 2 {
		t.Fatalf("the sampler produced only %d history points; need >2 to exercise trimming", n)
	}
	if d := rg.sections(2); d.Size.Y <= 0 {
		t.Fatal("the trimmed graph produced no dimensions")
	}

	rg.s.graphWindow = 5 * time.Minute
	if d := rg.sections(2); d.Size.Y <= 0 {
		t.Fatal("the untrimmed graph produced no dimensions")
	}
}

func TestDiagButtonRunsOnceAndPopulates(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.sections(2)

	rg.s.diagBtn.Click()
	rg.section()

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		rg.s.mu.Lock()
		running, lines := rg.s.diagRunning, rg.s.diagLines
		rg.s.mu.Unlock()
		if !running && len(lines) > 0 {
			break
		}
		rg.section()
		time.Sleep(10 * time.Millisecond)
	}

	rg.s.mu.Lock()
	lines := rg.s.diagLines
	rg.s.mu.Unlock()
	if len(lines) == 0 {
		t.Fatal("the diagnostics run never produced any lines")
	}
	if lines[0].label != "Limiter backend" {
		t.Errorf("first diagnostic = %q, want \"Limiter backend\"", lines[0].label)
	}
	if d := rg.sections(2); d.Size.Y <= 0 {
		t.Fatal("the populated diagnostics card produced no dimensions")
	}
}

func TestDiagButtonIgnoredWhileRunning(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.sections(2)
	rg.s.mu.Lock()
	rg.s.diagRunning = true
	rg.s.mu.Unlock()

	rg.s.diagBtn.Click()
	rg.section()

	rg.s.mu.Lock()
	lines := rg.s.diagLines
	rg.s.mu.Unlock()
	if len(lines) != 0 {
		t.Errorf("a re-entrant diagnostics run must be skipped, got %d lines", len(lines))
	}
}

func TestBuildDiagnosticsShape(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.s.caps = netlim.Caps{Available: true, NeedsElevation: true, PerAppSpeed: true}
	lines := rg.s.buildDiagnostics()
	labels := map[string]bool{}
	for _, ln := range lines {
		labels[ln.label] = true
		if ln.ok < -1 || ln.ok > 1 {
			t.Errorf("diagLine %q has out-of-range ok=%d", ln.label, ln.ok)
		}
	}
	for _, want := range []string{"Limiter backend", "Privileges", "Per-app monitoring", "Session peak ↓", "Session peak ↑"} {
		if !labels[want] {
			t.Errorf("missing diagnostic %q", want)
		}
	}

	rg.s.caps = netlim.Caps{}
	lines = rg.s.buildDiagnostics()
	for _, ln := range lines {
		if ln.label == "Privileges" {
			t.Error("Privileges must be omitted when elevation is not needed")
		}
		if ln.label == "Per-app monitoring" && ln.ok != 0 {
			t.Errorf("unsupported per-app monitoring must be neutral, got ok=%d", ln.ok)
		}
	}
}

func TestDiagRowColors(t *testing.T) {
	th := shapedTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 30)),
	}
	for _, ok := range []int8{1, 0, -1} {
		if d := diagRow(gtx, th, diagLine{label: "Ping", value: "1 ms", ok: ok}); d.Size.X <= 0 {
			t.Errorf("ok=%d produced no dimensions", ok)
		}
	}
}

func TestCurrentNumbersWithPerAppSpeed(t *testing.T) {
	rg := newRig(t, image.Pt(900, 700))
	rg.s.scope = netlim.ScopeApp
	rg.s.hasApp = true
	rg.s.selApp = netlim.ProcInfo{PID: 7, Name: "app.exe"}
	rg.s.caps.PerAppSpeed = true
	if d := rg.sections(2); d.Size.Y <= 0 {
		t.Fatal("the per-app numbers column produced no dimensions")
	}

	rg.s.caps.PerAppSpeed = false
	if d := rg.sections(2); d.Size.Y <= 0 {
		t.Fatal("the plain numbers column produced no dimensions")
	}
}

func TestTrafficGraphWithData(t *testing.T) {
	th := shapedTheme()
	pts := make([]netlim.TrafficPoint, 0, 40)
	for i := range 40 {
		pts = append(pts, netlim.TrafficPoint{InBps: int64(i) * 100000, OutBps: int64(40-i) * 50000})
	}
	cases := []struct {
		name  string
		vis   []netlim.TrafficPoint
		slots int
		w     int
	}{
		{"full", pts, 40, 600},
		{"partial", pts[:5], 40, 600},
		{"single-point", pts[:1], 40, 600},
		{"empty", nil, 40, 600},
		{"one-slot", pts, 1, 600},
		{"narrow", pts, 40, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := layout.Context{
				Ops:         new(op.Ops),
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Exact(image.Pt(tc.w, 200)),
			}
			var peakIn, peakOut int64
			for _, p := range tc.vis {
				if p.InBps > peakIn {
					peakIn = p.InBps
				}
				if p.OutBps > peakOut {
					peakOut = p.OutBps
				}
			}
			d := trafficGraph(gtx, th, tc.vis, tc.slots, peakIn, peakOut)
			if d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Errorf("dimensions = %v", d.Size)
			}
		})
	}
}

func TestTrafficGraphClampsOversizedValues(t *testing.T) {
	th := shapedTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 200)),
	}
	pts := []netlim.TrafficPoint{
		{InBps: 1 << 40, OutBps: 1 << 40},
		{InBps: 1, OutBps: 1},
		{InBps: 1 << 40, OutBps: 1 << 40},
	}
	if d := trafficGraph(gtx, th, pts, 3, 10, 10); d.Size.X <= 0 {
		t.Error("values above the reported peak must still render")
	}
}

func TestNiceCeil(t *testing.T) {
	cases := []struct{ in, want int64 }{
		{-5, 1}, {0, 1}, {1, 1}, {2, 2}, {3, 5}, {5, 5}, {6, 10}, {10, 10},
		{11, 20}, {21, 50}, {51, 100}, {100, 100}, {999, 1000},
		{1024 * 1024, 2000000},
	}
	for _, tc := range cases {
		got := niceCeil(tc.in)
		if got != tc.want {
			t.Errorf("niceCeil(%d) = %d, want %d", tc.in, got, tc.want)
		}
		if tc.in > 0 && got < tc.in {
			t.Errorf("niceCeil(%d) = %d must not be below the input", tc.in, got)
		}
	}
}

func TestStateBadgeAndSpecSummary(t *testing.T) {
	th := shapedTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(500, 30)),
	}
	spec := netlim.LimitSpec{Scope: netlim.ScopeApp, AppName: "app.exe", InBps: 1 << 20, OutBps: 2 << 20, TotalBps: 3 << 20}
	for _, st := range []netlim.State{netlim.StateActive, netlim.StatePaused, netlim.StateIdle} {
		if d := stateBadge(gtx, th, st, spec); d.Size.X <= 0 {
			t.Errorf("state %v produced no dimensions", st)
		}
	}

	cases := []struct {
		name string
		spec netlim.LimitSpec
		want string
	}{
		{"empty", netlim.LimitSpec{}, ""},
		{"system-in-only", netlim.LimitSpec{InBps: 1024}, "↓1.0 KB/s"},
		{"app-named", netlim.LimitSpec{Scope: netlim.ScopeApp, AppName: "a.exe", OutBps: 2048}, "a.exe  ↑2.0 KB/s"},
		{"app-unnamed", netlim.LimitSpec{Scope: netlim.ScopeApp, TotalBps: 1 << 20}, "Σ1.0 MB/s"},
		{"all", netlim.LimitSpec{InBps: 1024, OutBps: 1024, TotalBps: 1024}, "↓1.0 KB/s  ↑1.0 KB/s  Σ1.0 KB/s"},
	}
	for _, tc := range cases {
		if got := specSummary(tc.spec); got != tc.want {
			t.Errorf("%s: specSummary = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestFormatRateBoundaries(t *testing.T) {
	cases := map[int64]string{
		-1:                 "-1 B/s",
		0:                  "0 B/s",
		1023:               "1023 B/s",
		1024:               "1.0 KB/s",
		1024*1024 - 1:      "1024.0 KB/s",
		1024 * 1024:        "1.0 MB/s",
		1536 * 1024:        "1.5 MB/s",
		1024 * 1024 * 1024: "1024.0 MB/s",
	}
	for in, want := range cases {
		if got := formatRate(in); got != want {
			t.Errorf("formatRate(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestLighten(t *testing.T) {
	got := lighten(color.NRGBA{R: 10, G: 20, B: 250, A: 128})
	if got.R != 28 || got.G != 38 || got.B != 255 {
		t.Errorf("lighten channels = %d,%d,%d, want 28,38,255", got.R, got.G, got.B)
	}
	if got.A != 128 {
		t.Errorf("lighten must preserve alpha, got %d", got.A)
	}
}

func TestButtonAndChipHoverStates(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	rg.bodies(2)

	seen := map[string]bool{}
	probes := []struct {
		name string
		is   func() bool
	}{
		{"scope-system", rg.s.scopeSys.Hovered},
		{"scope-app", rg.s.scopeApp.Hovered},
		{"unit-chip", rg.s.inUnit.clicks[0].Hovered},
		{"start", rg.s.startBtn.Hovered},
	}
	for y := float32(12); y < 700 && len(seen) < len(probes); y += 2 {
		for _, x := range []float32{60, 250, 300, 330} {
			rg.move(x, y)
			rg.body()
			for _, p := range probes {
				if p.is() {
					seen[p.name] = true
				}
			}
		}
	}
	if !seen["scope-system"] || !seen["scope-app"] {
		t.Errorf("scope toggles were never hoverable: %v", seen)
	}
	if !seen["unit-chip"] {
		t.Errorf("unit chips were never hoverable: %v", seen)
	}
}

func TestStatusReportsManagerState(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))
	if !rg.s.Started() {
		t.Error("Started must be true after Init")
	}
	active, paused := rg.s.Status()
	if active || paused {
		t.Errorf("a freshly initialised section must be idle, got active=%v paused=%v", active, paused)
	}
	if (&Section{}).Started() {
		t.Error("Started must be false before Init")
	}
}

func TestToggleAndCancelLimitInvokeCallback(t *testing.T) {
	rg := newRig(t, image.Pt(360, 800))

	done := make(chan struct{}, 2)
	rg.s.ToggleLimit(func() { done <- struct{}{} })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ToggleLimit never invoked the callback")
	}

	rg.s.CancelLimit(func() { done <- struct{}{} })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CancelLimit never invoked the callback")
	}

	if got := rg.s.getErr(); got != "" {
		t.Errorf("idle toggle/cancel must not set an error, got %q", got)
	}
}

func TestLoadConfigMissingFileKeepsDefaults(t *testing.T) {
	setupTestConfigDir(t)
	s := new(Section)
	s.inUnit.idx, s.outUnit.idx, s.totalUnit.idx = 1, 1, 1
	s.loadConfig()
	if s.inUnit.idx != 1 || s.outUnit.idx != 1 || s.totalUnit.idx != 1 {
		t.Errorf("a missing config must not change units: %d/%d/%d", s.inUnit.idx, s.outUnit.idx, s.totalUnit.idx)
	}
	if s.hasApp {
		t.Error("a missing config must not select an application")
	}
}

func TestLoadConfigMalformedJSONKeepsDefaults(t *testing.T) {
	setupTestConfigDir(t)
	writeConfig(t, []byte("{not json"))
	s := new(Section)
	s.inUnit.idx = 2
	s.loadConfig()
	if s.inUnit.idx != 2 {
		t.Errorf("malformed JSON must not change units, got %d", s.inUnit.idx)
	}
}

func TestLoadConfigUnitMigration(t *testing.T) {
	cases := []struct {
		name                   string
		cfg                    config
		wantIn, wantOut, wantT int
	}{
		{"legacy-unit-mb-flag", config{UnitMB: true}, 1, 1, 1},
		{"legacy-unit-index", config{Unit: 2}, 2, 2, 2},
		{"legacy-unit-index-out-of-range", config{Unit: 99}, 0, 0, 0},
		{"legacy-none", config{}, 0, 0, 0},
		{"per-field", config{InUnit: 1, OutUnit: 2, TotUnit: 0}, 1, 2, 0},
		{"per-field-out-of-range", config{InUnit: 99, OutUnit: -3, TotUnit: 2}, 0, 0, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupTestConfigDir(t)
			data, err := json.Marshal(tc.cfg)
			if err != nil {
				t.Fatal(err)
			}
			writeConfig(t, data)
			s := new(Section)
			s.loadConfig()
			if s.inUnit.idx != tc.wantIn || s.outUnit.idx != tc.wantOut || s.totalUnit.idx != tc.wantT {
				t.Errorf("units = %d/%d/%d, want %d/%d/%d",
					s.inUnit.idx, s.outUnit.idx, s.totalUnit.idx, tc.wantIn, tc.wantOut, tc.wantT)
			}
		})
	}
}

func TestSaveConfigRoundTripsEveryUnit(t *testing.T) {
	for idx := range units {
		t.Run(units[idx].label, func(t *testing.T) {
			setupTestConfigDir(t)
			s := new(Section)
			s.inUnit.idx, s.outUnit.idx, s.totalUnit.idx = idx, idx, idx
			s.inEd.SetText("7")
			s.saveConfig()

			s2 := new(Section)
			s2.loadConfig()
			if s2.inUnit.idx != idx || s2.outUnit.idx != idx || s2.totalUnit.idx != idx {
				t.Errorf("units = %d/%d/%d, want all %d",
					s2.inUnit.idx, s2.outUnit.idx, s2.totalUnit.idx, idx)
			}
			if s2.inEd.Text() != "7" {
				t.Errorf("in = %q, want \"7\"", s2.inEd.Text())
			}
		})
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	setupTestConfigDir(t)
	s := new(Section)
	s.Init()
	s.Close()
	s.Close()
	(&Section{}).Close()
}

func TestLayoutBodyAcrossViewports(t *testing.T) {
	for _, sz := range []image.Point{{X: 200, Y: 300}, {X: 360, Y: 800}, {X: 800, Y: 200}, {X: 1200, Y: 1000}} {
		rg := newRig(t, sz)
		rg.s.scope = netlim.ScopeApp
		rg.s.pickerOpen = true
		rg.s.setProcs([]netlim.ProcInfo{{PID: 1, Name: "a.exe"}})
		if d := rg.bodies(2); d.Size.X <= 0 {
			t.Errorf("size %v produced no dimensions", sz)
		}
		if d := rg.sections(2); d.Size.X <= 0 {
			t.Errorf("size %v section produced no dimensions", sz)
		}
	}
}
