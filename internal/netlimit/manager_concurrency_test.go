package netlimit

import (
	"sync"
	"testing"
	"time"
)

func TestSampleLoopWithNilMonitorRecordsNothing(t *testing.T) {
	pacer := newTestManager(nil, &fakeMonitor{step: 1000}, 2*time.Millisecond, 8)
	ticks := make(chan struct{}, 64)
	pacer.SetOnChange(func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})

	m := newTestManager(nil, nil, 2*time.Millisecond, 8)
	m.SetWatchPID(42)
	m.Start()
	pacer.Start()

	waitTicks(t, ticks, 5)

	if got := m.History(); len(got) != 0 {
		t.Fatalf("nil monitor recorded %d history points, want 0", len(got))
	}
	if got := m.SystemSpeed(); (got != Sample{}) {
		t.Fatalf("SystemSpeed with nil monitor = %+v, want zero", got)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := pacer.Close(); err != nil {
		t.Fatalf("pacer Close: %v", err)
	}
}

func TestManagerConcurrentStateChanges(t *testing.T) {
	m := newTestManager(&fakeShaper{}, &fakeMonitor{step: 4096}, time.Millisecond, 32)
	m.SetOnChange(func() {})
	m.Start()

	const workers = 8
	const rounds = 150
	spec := LimitSpec{Scope: ScopeApp, AppPID: 99, InBps: 1 << 20, OutBps: 1 << 19}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			<-start
			for i := 0; i < rounds; i++ {
				switch (w + i) % 8 {
				case 0:
					_ = m.Apply(spec)
				case 1:
					_ = m.Pause()
				case 2:
					_ = m.Resume()
				case 3:
					_ = m.Cancel()
				case 4:
					_ = m.State()
					_ = m.Active()
				case 5:
					_ = m.Spec()
					_ = m.Caps()
				case 6:
					_ = m.History()
					_ = m.SystemSpeed()
					_ = m.AppSpeed()
				case 7:
					m.SetWatchPID(int32(i))
					m.SetOnChange(func() {})
				}
			}
		}(w)
	}
	close(start)
	wg.Wait()

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := m.State(); got != StateIdle {
		t.Fatalf("state after Close = %v, want StateIdle", got)
	}
	if got := m.Spec(); (got != LimitSpec{}) {
		t.Fatalf("spec after Close = %+v, want zero", got)
	}
}

func TestManagerStartStopCycles(t *testing.T) {
	mon := &fakeMonitor{step: 2048}
	m := newTestManager(&fakeShaper{}, mon, 2*time.Millisecond, 8)
	ticks := make(chan struct{}, 64)
	m.SetOnChange(func() {
		select {
		case ticks <- struct{}{}:
		default:
		}
	})

	for cycle := 0; cycle < 3; cycle++ {
		m.Start()
		waitTicks(t, ticks, 2)
		if err := m.Close(); err != nil {
			t.Fatalf("cycle %d Close: %v", cycle, err)
		}
		m.mu.Lock()
		stopCh, doneCh := m.stopCh, m.doneCh
		m.mu.Unlock()
		if stopCh != nil || doneCh != nil {
			t.Fatalf("cycle %d left channels behind", cycle)
		}
	}
}
