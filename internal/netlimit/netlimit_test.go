package netlimit

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

type fakeShaper struct {
	mu        sync.Mutex
	caps      Caps
	applyErr  error
	removeErr error
	applied   []LimitSpec
	removes   int
}

func (f *fakeShaper) Caps() Caps { return f.caps }

func (f *fakeShaper) Apply(spec LimitSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.applyErr != nil {
		return f.applyErr
	}
	f.applied = append(f.applied, spec)
	return nil
}

func (f *fakeShaper) Remove() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removes++
	return nil
}

func (f *fakeShaper) stats() (applied []LimitSpec, removes int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]LimitSpec(nil), f.applied...), f.removes
}

type fakeMonitor struct {
	mu       sync.Mutex
	sysRx    uint64
	sysTx    uint64
	appRx    uint64
	appTx    uint64
	step     uint64
	sysErr   error
	appErr   error
	closeErr error
	pids     []int32
	closed   bool
}

func (f *fakeMonitor) SystemCounters() (uint64, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sysErr != nil {
		return 0, 0, f.sysErr
	}
	f.sysRx += f.step
	f.sysTx += f.step * 2
	return f.sysRx, f.sysTx, nil
}

func (f *fakeMonitor) AppCounters(pid int32) (uint64, uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pids = append(f.pids, pid)
	if f.appErr != nil {
		return 0, 0, f.appErr
	}
	f.appRx += f.step
	f.appTx += f.step * 3
	return f.appRx, f.appTx, nil
}

func (f *fakeMonitor) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return f.closeErr
}

func (f *fakeMonitor) seenPIDs() []int32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int32(nil), f.pids...)
}

func (f *fakeMonitor) wasClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func newTestManager(sh Shaper, mon Monitor, interval time.Duration, histSize int) *Manager {
	return &Manager{
		shaper:   sh,
		monitor:  mon,
		interval: interval,
		hist:     make([]TrafficPoint, histSize),
	}
}

func TestLimitSpecUnlimited(t *testing.T) {
	tests := []struct {
		name string
		spec LimitSpec
		want bool
	}{
		{"zero value", LimitSpec{}, true},
		{"all negative", LimitSpec{InBps: -1, OutBps: -5, TotalBps: -100}, true},
		{"in only", LimitSpec{InBps: 1}, false},
		{"out only", LimitSpec{OutBps: 1}, false},
		{"total only", LimitSpec{TotalBps: 1}, false},
		{"all set", LimitSpec{InBps: 10, OutBps: 20, TotalBps: 30}, false},
		{"negative in, positive out", LimitSpec{InBps: -10, OutBps: 20}, false},
		{"metadata only", LimitSpec{Scope: ScopeApp, AppPath: "x", AppName: "y", AppPID: 7}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.spec.Unlimited(); got != tt.want {
				t.Fatalf("Unlimited() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRateOf(t *testing.T) {
	tests := []struct {
		name string
		cur  uint64
		prev uint64
		dt   float64
		want int64
	}{
		{"one second", 1000, 0, 1, 1000},
		{"half second doubles rate", 1000, 0, 0.5, 2000},
		{"two seconds halves rate", 1000, 0, 2, 500},
		{"no change", 500, 500, 1, 0},
		{"counter reset clamps to zero", 10, 5000, 1, 0},
		{"delta over interval", 5_500, 500, 2, 2500},
		{"fractional truncates", 10, 0, 3, 3},
		{"tiny dt scales up", 1024, 0, 0.001, 1_024_000},
		{"large counters", 1<<40 + 4096, 1 << 40, 1, 4096},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rateOf(tt.cur, tt.prev, tt.dt); got != tt.want {
				t.Fatalf("rateOf(%d, %d, %v) = %d, want %d", tt.cur, tt.prev, tt.dt, got, tt.want)
			}
		})
	}
}

func TestNewManagerDefaults(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if got := m.Interval(); got != 700*time.Millisecond {
		t.Fatalf("Interval() = %v, want 700ms", got)
	}
	if len(m.hist) != 600 {
		t.Fatalf("history ring = %d, want 600", len(m.hist))
	}
	if m.State() != StateIdle {
		t.Fatalf("initial state = %v, want StateIdle", m.State())
	}
	if m.Active() {
		t.Fatal("fresh manager reports Active")
	}
	if (m.Spec() != LimitSpec{}) {
		t.Fatalf("fresh manager spec = %+v, want zero", m.Spec())
	}
}

func TestManagerCapsNilShaper(t *testing.T) {
	m := &Manager{}
	if got := m.Caps(); (got != Caps{}) {
		t.Fatalf("Caps() with nil shaper = %+v, want zero", got)
	}
	want := Caps{Available: true, SystemLimit: true, Note: "hi"}
	m2 := newTestManager(&fakeShaper{caps: want}, nil, time.Second, 4)
	if got := m2.Caps(); got != want {
		t.Fatalf("Caps() = %+v, want %+v", got, want)
	}
}

func TestManagerApplyNilShaper(t *testing.T) {
	m := &Manager{}
	if err := m.Apply(LimitSpec{InBps: 100}); !errors.Is(err, errUnsupported) {
		t.Fatalf("Apply with nil shaper = %v, want errUnsupported", err)
	}
	if m.State() != StateIdle {
		t.Fatalf("state = %v, want StateIdle", m.State())
	}
}

func TestManagerLifecycle(t *testing.T) {
	sh := &fakeShaper{}
	m := newTestManager(sh, nil, time.Second, 4)

	var changes int
	m.SetOnChange(func() { changes++ })

	if err := m.Pause(); err != nil {
		t.Fatalf("Pause on idle: %v", err)
	}
	if err := m.Resume(); err != nil {
		t.Fatalf("Resume on idle: %v", err)
	}
	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel on idle: %v", err)
	}
	if _, removes := sh.stats(); removes != 0 {
		t.Fatalf("no-op transitions called Remove %d times", removes)
	}
	if changes != 0 {
		t.Fatalf("no-op transitions fired onChange %d times", changes)
	}

	spec := LimitSpec{Scope: ScopeApp, AppPID: 4242, InBps: 1000, OutBps: 2000}
	if err := m.Apply(spec); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if m.State() != StateActive || !m.Active() {
		t.Fatalf("state after Apply = %v, want StateActive", m.State())
	}
	if m.Spec() != spec {
		t.Fatalf("Spec() = %+v, want %+v", m.Spec(), spec)
	}
	if got := m.watchPID.Load(); got != 4242 {
		t.Fatalf("watchPID after ScopeApp apply = %d, want 4242", got)
	}

	if err := m.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if m.State() != StatePaused {
		t.Fatalf("state after Pause = %v, want StatePaused", m.State())
	}
	if m.Spec() != spec {
		t.Fatalf("Pause must retain spec, got %+v", m.Spec())
	}

	if err := m.Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if m.State() != StateActive {
		t.Fatalf("state after Resume = %v, want StateActive", m.State())
	}

	applied, removes := sh.stats()
	if len(applied) != 2 {
		t.Fatalf("shaper Apply calls = %d, want 2", len(applied))
	}
	if applied[1] != spec {
		t.Fatalf("Resume re-applied %+v, want %+v", applied[1], spec)
	}
	if removes != 1 {
		t.Fatalf("shaper Remove calls = %d, want 1", removes)
	}

	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if m.State() != StateIdle {
		t.Fatalf("state after Cancel = %v, want StateIdle", m.State())
	}
	if (m.Spec() != LimitSpec{}) {
		t.Fatalf("Cancel must clear spec, got %+v", m.Spec())
	}
	if got := m.watchPID.Load(); got != 0 {
		t.Fatalf("watchPID after Cancel = %d, want 0", got)
	}
	if changes != 4 {
		t.Fatalf("onChange fired %d times, want 4 (apply/pause/resume/cancel)", changes)
	}
}

func TestManagerTransitionErrors(t *testing.T) {
	applyErr := errors.New("apply boom")
	removeErr := errors.New("remove boom")

	t.Run("apply failure keeps idle", func(t *testing.T) {
		sh := &fakeShaper{applyErr: applyErr}
		m := newTestManager(sh, nil, time.Second, 4)
		if err := m.Apply(LimitSpec{InBps: 1}); !errors.Is(err, applyErr) {
			t.Fatalf("Apply = %v, want %v", err, applyErr)
		}
		if m.State() != StateIdle {
			t.Fatalf("state = %v, want StateIdle", m.State())
		}
		if (m.Spec() != LimitSpec{}) {
			t.Fatalf("failed Apply stored spec %+v", m.Spec())
		}
	})

	t.Run("pause failure keeps active", func(t *testing.T) {
		sh := &fakeShaper{}
		m := newTestManager(sh, nil, time.Second, 4)
		if err := m.Apply(LimitSpec{InBps: 1}); err != nil {
			t.Fatal(err)
		}
		sh.mu.Lock()
		sh.removeErr = removeErr
		sh.mu.Unlock()
		if err := m.Pause(); !errors.Is(err, removeErr) {
			t.Fatalf("Pause = %v, want %v", err, removeErr)
		}
		if m.State() != StateActive {
			t.Fatalf("state = %v, want StateActive", m.State())
		}
	})

	t.Run("resume failure keeps paused", func(t *testing.T) {
		sh := &fakeShaper{}
		m := newTestManager(sh, nil, time.Second, 4)
		if err := m.Apply(LimitSpec{InBps: 1}); err != nil {
			t.Fatal(err)
		}
		if err := m.Pause(); err != nil {
			t.Fatal(err)
		}
		sh.mu.Lock()
		sh.applyErr = applyErr
		sh.mu.Unlock()
		if err := m.Resume(); !errors.Is(err, applyErr) {
			t.Fatalf("Resume = %v, want %v", err, applyErr)
		}
		if m.State() != StatePaused {
			t.Fatalf("state = %v, want StatePaused", m.State())
		}
	})

	t.Run("cancel failure keeps state", func(t *testing.T) {
		sh := &fakeShaper{}
		m := newTestManager(sh, nil, time.Second, 4)
		if err := m.Apply(LimitSpec{InBps: 1}); err != nil {
			t.Fatal(err)
		}
		sh.mu.Lock()
		sh.removeErr = removeErr
		sh.mu.Unlock()
		if err := m.Cancel(); !errors.Is(err, removeErr) {
			t.Fatalf("Cancel = %v, want %v", err, removeErr)
		}
		if m.State() != StateActive {
			t.Fatalf("state = %v, want StateActive", m.State())
		}
	})

	t.Run("nil shaper transitions still change state", func(t *testing.T) {
		m := newTestManager(nil, nil, time.Second, 4)
		m.mu.Lock()
		m.state = StateActive
		m.spec = LimitSpec{OutBps: 9}
		m.mu.Unlock()
		if err := m.Pause(); err != nil {
			t.Fatalf("Pause: %v", err)
		}
		if m.State() != StatePaused {
			t.Fatalf("state = %v, want StatePaused", m.State())
		}
		if err := m.Resume(); err != nil {
			t.Fatalf("Resume: %v", err)
		}
		if m.State() != StateActive {
			t.Fatalf("state = %v, want StateActive", m.State())
		}
		if err := m.Cancel(); err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		if m.State() != StateIdle {
			t.Fatalf("state = %v, want StateIdle", m.State())
		}
	})
}

func TestManagerHistoryRing(t *testing.T) {
	t.Run("partial fill preserves order", func(t *testing.T) {
		m := newTestManager(nil, nil, time.Second, 4)
		if got := m.History(); len(got) != 0 {
			t.Fatalf("empty history = %v", got)
		}
		m.recordHistory(1, 10)
		m.recordHistory(2, 20)
		got := m.History()
		want := []TrafficPoint{{InBps: 1, OutBps: 10}, {InBps: 2, OutBps: 20}}
		if len(got) != len(want) {
			t.Fatalf("len = %d, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("point %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("exact fill", func(t *testing.T) {
		m := newTestManager(nil, nil, time.Second, 3)
		for i := int64(1); i <= 3; i++ {
			m.recordHistory(i, i*10)
		}
		got := m.History()
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		for i := range got {
			want := TrafficPoint{InBps: int64(i + 1), OutBps: int64(i+1) * 10}
			if got[i] != want {
				t.Fatalf("point %d = %+v, want %+v", i, got[i], want)
			}
		}
	})

	t.Run("wrap drops oldest and keeps chronological order", func(t *testing.T) {
		m := newTestManager(nil, nil, time.Second, 3)
		for i := int64(1); i <= 5; i++ {
			m.recordHistory(i, i*10)
		}
		got := m.History()
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		want := []TrafficPoint{{InBps: 3, OutBps: 30}, {InBps: 4, OutBps: 40}, {InBps: 5, OutBps: 50}}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("point %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})

	t.Run("returned slice is a copy", func(t *testing.T) {
		m := newTestManager(nil, nil, time.Second, 3)
		m.recordHistory(1, 1)
		got := m.History()
		got[0] = TrafficPoint{InBps: 999}
		if again := m.History(); again[0].InBps != 1 {
			t.Fatalf("History() aliases internal ring: %+v", again[0])
		}
	})
}

func TestManagerSpeedAccessors(t *testing.T) {
	m := newTestManager(nil, nil, time.Second, 4)
	if got := m.SystemSpeed(); (got != Sample{}) {
		t.Fatalf("SystemSpeed = %+v, want zero", got)
	}
	if got := m.AppSpeed(); (got != Sample{}) {
		t.Fatalf("AppSpeed = %+v, want zero", got)
	}
	m.sysIn.Store(11)
	m.sysOut.Store(22)
	m.appIn.Store(33)
	m.appOut.Store(44)
	if got := (m.SystemSpeed()); got != (Sample{InBps: 11, OutBps: 22}) {
		t.Fatalf("SystemSpeed = %+v", got)
	}
	if got := (m.AppSpeed()); got != (Sample{InBps: 33, OutBps: 44}) {
		t.Fatalf("AppSpeed = %+v", got)
	}
	m.SetWatchPID(77)
	if got := m.watchPID.Load(); got != 77 {
		t.Fatalf("watchPID = %d, want 77", got)
	}
}

func TestManagerSampleLoop(t *testing.T) {
	mon := &fakeMonitor{step: 1_000_000}
	sh := &fakeShaper{}
	m := newTestManager(sh, mon, 2*time.Millisecond, 16)

	if err := m.Apply(LimitSpec{Scope: ScopeSystem, TotalBps: 1 << 20}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	ticks := make(chan struct{}, 256)
	m.SetOnChange(func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})

	m.SetWatchPID(1234)
	m.Start()
	m.Start()

	waitTicks(t, ticks, 4)

	if got := m.SystemSpeed(); got.InBps <= 0 || got.OutBps <= 0 {
		t.Fatalf("SystemSpeed = %+v, want positive rates", got)
	}
	if got := m.AppSpeed(); got.InBps <= 0 || got.OutBps <= 0 {
		t.Fatalf("AppSpeed = %+v, want positive rates", got)
	}
	for _, pid := range mon.seenPIDs() {
		if pid != 1234 {
			t.Fatalf("AppCounters polled pid %d, want 1234", pid)
		}
	}
	if len(m.History()) == 0 {
		t.Fatal("sample loop recorded no history")
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !mon.wasClosed() {
		t.Fatal("Close did not close the monitor")
	}
	if _, removes := sh.stats(); removes != 1 {
		t.Fatalf("Close on active manager Remove calls = %d, want 1", removes)
	}
}

func TestManagerSampleLoopMonitorErrors(t *testing.T) {
	mon := &fakeMonitor{step: 1_000_000, sysErr: errors.New("no sys"), appErr: errors.New("no app")}
	m := newTestManager(nil, mon, 2*time.Millisecond, 16)

	ticks := make(chan struct{}, 64)
	m.SetOnChange(func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})
	m.SetWatchPID(5)
	m.Start()
	waitTicks(t, ticks, 3)

	if got := m.SystemSpeed(); (got != Sample{}) {
		t.Fatalf("SystemSpeed with failing monitor = %+v, want zero", got)
	}
	if got := m.AppSpeed(); (got != Sample{}) {
		t.Fatalf("AppSpeed with failing monitor = %+v, want zero", got)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestManagerSampleLoopNilMonitor(t *testing.T) {
	m := newTestManager(nil, nil, 2*time.Millisecond, 8)
	m.Start()
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestManagerCloseIdleDoesNotRemove(t *testing.T) {
	sh := &fakeShaper{}
	mon := &fakeMonitor{closeErr: errors.New("close boom")}
	m := newTestManager(sh, mon, time.Second, 4)
	if err := m.Close(); err == nil {
		t.Fatal("Close should surface monitor close error")
	}
	if _, removes := sh.stats(); removes != 0 {
		t.Fatalf("idle Close called Remove %d times", removes)
	}
}

func waitTicks(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	deadline := time.After(15 * time.Second)
	for i := 0; i < n; i++ {
		select {
		case <-ch:
		case <-deadline:
			t.Fatalf("timed out waiting for tick %d/%d", i+1, n)
		}
	}
}

func TestManagerMarkerFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marker.json")
	sh := &fakeShaper{}
	m := newTestManager(sh, nil, time.Second, 4)
	m.SetMarkerPath(path)

	spec := LimitSpec{Scope: ScopeSystem, InBps: 1000}
	if err := m.Apply(spec); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	_, statErr := os.Stat(path)
	if runtime.GOOS == "windows" {
		if statErr == nil {
			t.Fatal("marker must not be written on windows")
		}
	} else {
		if statErr != nil {
			t.Fatalf("marker not written: %v", statErr)
		}
		if !m.HasOrphan() {
			t.Log("HasOrphan is false while active, as documented")
		}
	}

	if err := m.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("marker survived Cancel")
	}
}

func TestManagerOrphanHandling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orphan.json")
	if err := os.WriteFile(path, []byte(`{"InBps":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sh := &fakeShaper{}
	m := newTestManager(sh, nil, time.Second, 4)

	if m.HasOrphan() {
		t.Fatal("HasOrphan with empty marker path must be false")
	}
	m.SetMarkerPath(path)

	got := m.HasOrphan()
	want := runtime.GOOS != "windows"
	if got != want {
		t.Fatalf("HasOrphan() = %v, want %v on %s", got, want, runtime.GOOS)
	}

	if err := m.ClearOrphan(); err != nil {
		t.Fatalf("ClearOrphan: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("ClearOrphan left the marker in place")
	}
	if _, removes := sh.stats(); removes != 1 {
		t.Fatalf("ClearOrphan Remove calls = %d, want 1", removes)
	}
	if m.HasOrphan() {
		t.Fatal("HasOrphan true after ClearOrphan")
	}
}

func TestManagerClearOrphanPropagatesError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "orphan.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("remove boom")
	m := newTestManager(&fakeShaper{removeErr: boom}, nil, time.Second, 4)
	m.SetMarkerPath(path)
	if err := m.ClearOrphan(); !errors.Is(err, boom) {
		t.Fatalf("ClearOrphan = %v, want %v", err, boom)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("ClearOrphan must remove the marker even when the shaper fails")
	}

	m2 := newTestManager(nil, nil, time.Second, 4)
	if err := m2.ClearOrphan(); err != nil {
		t.Fatalf("ClearOrphan with nil shaper = %v", err)
	}
}

func TestManagerHasOrphanRequiresIdle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Log("marker files are disabled on windows; state gate exercised via non-windows path")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "m.json")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := newTestManager(&fakeShaper{}, nil, time.Second, 4)
	m.SetMarkerPath(path)
	m.mu.Lock()
	m.state = StateActive
	m.mu.Unlock()
	if m.HasOrphan() {
		t.Fatal("HasOrphan must be false when not idle")
	}
}

func TestManagerListProcs(t *testing.T) {
	m := New()
	procs, err := m.ListProcs()
	if err != nil {
		t.Fatalf("ListProcs: %v", err)
	}
	if len(procs) == 0 {
		t.Fatal("listProcs returned no processes")
	}
	for i, p := range procs {
		if p.PID == 0 {
			t.Fatalf("proc %d has zero PID: %+v", i, p)
		}
		if p.Name == "" {
			t.Fatalf("proc %d has empty name: %+v", i, p)
		}
	}
	for i := 1; i < len(procs); i++ {
		if lower(procs[i-1].Name) > lower(procs[i].Name) {
			t.Fatalf("procs not sorted: %q before %q", procs[i-1].Name, procs[i].Name)
		}
	}
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func TestTCPPing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	res := TCPPing(addr, 3*time.Second)
	if !res.OK {
		t.Fatalf("TCPPing(%s) failed: %+v", addr, res)
	}
	if res.Target != addr {
		t.Fatalf("Target = %q, want %q", res.Target, addr)
	}
	if res.Latency < 0 {
		t.Fatalf("Latency = %v, want >= 0", res.Latency)
	}

	_ = ln.Close()
	<-done

	dead := TCPPing(addr, 2*time.Second)
	if dead.OK {
		t.Fatalf("TCPPing to closed port reported OK: %+v", dead)
	}
	if dead.Latency != 0 {
		t.Fatalf("failed ping Latency = %v, want 0", dead.Latency)
	}
	if dead.Target != addr {
		t.Fatalf("failed ping Target = %q, want %q", dead.Target, addr)
	}

	bad := TCPPing("not-a-valid-host:0", time.Second)
	if bad.OK {
		t.Fatalf("TCPPing to invalid host reported OK: %+v", bad)
	}
}

func TestStateAndScopeConstants(t *testing.T) {
	tests := []struct {
		name string
		got  uint8
		want uint8
	}{
		{"ScopeSystem", uint8(ScopeSystem), 0},
		{"ScopeApp", uint8(ScopeApp), 1},
		{"StateIdle", uint8(StateIdle), 0},
		{"StateActive", uint8(StateActive), 1},
		{"StatePaused", uint8(StatePaused), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("%s = %d, want %d", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestPidCounter(t *testing.T) {
	var c pidCounter
	c.rx.Add(10)
	c.rx.Add(5)
	c.tx.Add(7)
	if got := c.rx.Load(); got != 15 {
		t.Fatalf("rx = %d, want 15", got)
	}
	if got := c.tx.Load(); got != 7 {
		t.Fatalf("tx = %d, want 7", got)
	}
}

func TestSentinelErrors(t *testing.T) {
	for _, err := range []error{errUnsupported, errNoDriver} {
		if err == nil || err.Error() == "" {
			t.Fatalf("sentinel error is empty: %v", err)
		}
	}
	if errors.Is(errUnsupported, errNoDriver) {
		t.Fatal("sentinel errors must be distinct")
	}
}
