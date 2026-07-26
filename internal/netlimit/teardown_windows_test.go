//go:build windows

package netlimit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s did not return after its stop channel was closed", what)
	}
}

func closedStop() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestShaperLoopsExitOnAlreadyClosedStop(t *testing.T) {
	tests := []struct {
		name string
		run  func(s *winShaper)
	}{
		{"trackSockets", (*winShaper).trackSockets},
		{"shapeLoop", (*winShaper).shapeLoop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winShaper{stop: closedStop()}
			done := make(chan struct{})
			s.wg.Add(1)
			go func() {
				defer close(done)
				tt.run(s)
			}()
			waitClosed(t, done, tt.name)
			s.wg.Wait()
		})
	}
}

func TestSnifferLoopsExitOnAlreadyClosedStop(t *testing.T) {
	tests := []struct {
		name string
		run  func(s *winDivertSniffer)
	}{
		{"runSockets", (*winDivertSniffer).runSockets},
		{"runPackets", (*winDivertSniffer).runPackets},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winDivertSniffer{
				flows: make(map[flowKey]uint32),
				pids:  make(map[uint32]*pidCounter),
				stop:  closedStop(),
			}
			done := make(chan struct{})
			s.wg.Add(1)
			go func() {
				defer close(done)
				tt.run(s)
			}()
			waitClosed(t, done, tt.name)
			s.wg.Wait()
		})
	}
}

func TestWinShaperRemoveClosesOpenHandles(t *testing.T) {
	s := &winShaper{
		active: true,
		stop:   make(chan struct{}),
		netH:   &wdHandle{},
		sockH:  &wdHandle{},
		flows:  map[flowKey]uint32{},
	}
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.netH != nil || s.sockH != nil {
		t.Fatalf("Remove left handles behind: netH=%v sockH=%v", s.netH, s.sockH)
	}
	if s.active {
		t.Fatal("shaper still active after Remove")
	}
}

func TestWinShaperApplyRejectsWithoutDriverAndStaysClean(t *testing.T) {
	if winDivertAvailable() {
		t.Log("WinDivert present; not exercising Apply against the live driver")
		return
	}
	specs := []LimitSpec{
		{Scope: ScopeSystem, TotalBps: 1 << 20},
		{Scope: ScopeApp, AppPID: 1234, InBps: 1000, OutBps: 2000},
		{},
	}
	for _, spec := range specs {
		s := &winShaper{}
		if err := s.Apply(spec); err != errNoDriver {
			t.Fatalf("Apply(%+v) = %v, want errNoDriver", spec, err)
		}
		if s.active || s.netH != nil || s.sockH != nil || s.stop != nil || s.flows != nil {
			t.Fatalf("failed Apply mutated the shaper: %+v", s)
		}
		if err := s.Remove(); err != nil {
			t.Fatalf("Remove after failed Apply = %v", err)
		}
	}
}

func TestWDHandleCloseForwardsHandleOnce(t *testing.T) {
	saved := wdClose
	wdClose = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetEvent")
	t.Cleanup(func() { wdClose = saved })

	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	defer windows.CloseHandle(ev) //nolint:errcheck

	signaled := func() bool {
		st, err := windows.WaitForSingleObject(ev, 0)
		if err != nil {
			t.Fatalf("WaitForSingleObject: %v", err)
		}
		return st == windows.WAIT_OBJECT_0
	}

	h := &wdHandle{h: ev}
	if err := h.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !signaled() {
		t.Fatal("close did not pass the handle to WinDivertClose")
	}
	if h.h != 0 {
		t.Fatalf("close left h = %v, want 0", h.h)
	}

	if err := windows.ResetEvent(ev); err != nil {
		t.Fatalf("ResetEvent: %v", err)
	}
	if err := h.close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if signaled() {
		t.Fatal("close is not idempotent: the handle was released twice")
	}
}

func TestFindWinDivertUsesWorkingDirectory(t *testing.T) {
	_ = winDivertLoad()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WinDivert.dll"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wd, "WinDivert.dll")
	if got := findWinDivert(); got != want {
		t.Fatalf("findWinDivert() = %q, want %q", got, want)
	}
}

func TestFindWinDivertEmptyWhenAbsent(t *testing.T) {
	_ = winDivertLoad()
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if got := findWinDivert(); got != "" {
		t.Fatalf("findWinDivert() = %q, want %q", got, "")
	}
}

func TestFindWinDivertFallsBackToLoaderSearchPath(t *testing.T) {
	_ = winDivertLoad()

	src := filepath.Join(os.Getenv("SystemRoot"), "System32", "version.dll")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	dllDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dllDir, "WinDivert.dll"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())
	t.Setenv("PATH", dllDir)

	t.Cleanup(func() {
		const unchangedRefcount = 0x2
		name, err := windows.UTF16PtrFromString("WinDivert.dll")
		if err != nil {
			return
		}
		var mod windows.Handle
		if err := windows.GetModuleHandleEx(unchangedRefcount, name, &mod); err == nil && mod != 0 {
			_ = windows.FreeLibrary(mod)
		}
	})

	if got := findWinDivert(); got != "WinDivert.dll" {
		t.Fatalf("findWinDivert() = %q, want the bare loader-resolved name", got)
	}
}

func TestTokenBucketWaitClampsSleepToOneMillisecond(t *testing.T) {
	b := &tokenBucket{rate: 1e12, burst: 1e6, tokens: 0, last: time.Now()}
	b.wait(1)
	if b.tokens != b.burst-1 {
		t.Fatalf("tokens = %v, want %v (refill clamped to burst, then debited)", b.tokens, b.burst-1)
	}
}
