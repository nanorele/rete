package netlimit

import (
	"encoding/json"
	"net"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type Scope uint8

const (
	ScopeSystem Scope = iota
	ScopeApp
)

type LimitSpec struct {
	Scope    Scope
	AppPath  string
	AppName  string
	AppPID   int32
	InBps    int64
	OutBps   int64
	TotalBps int64
}

func (s LimitSpec) Unlimited() bool {
	return s.InBps <= 0 && s.OutBps <= 0 && s.TotalBps <= 0
}

type Sample struct {
	InBps  int64
	OutBps int64
}

type TrafficPoint struct {
	InBps  int64
	OutBps int64
}

type PingResult struct {
	Target  string
	Latency time.Duration
	OK      bool
}

func TCPPing(target string, timeout time.Duration) PingResult {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return PingResult{Target: target}
	}
	lat := time.Since(start)
	_ = conn.Close()
	return PingResult{Target: target, Latency: lat, OK: true}
}

type ProcInfo struct {
	PID  int32
	Name string
	Exe  string
}

type Caps struct {
	Available      bool
	SystemLimit    bool
	AppLimit       bool
	InboundLimit   bool
	PerAppSpeed    bool
	NeedsElevation bool
	Note           string
}

type Monitor interface {
	SystemCounters() (rx, tx uint64, err error)
	AppCounters(pid int32) (rx, tx uint64, err error)
	Close() error
}

type Shaper interface {
	Caps() Caps
	Apply(LimitSpec) error
	Remove() error
}

type State uint8

const (
	StateIdle State = iota
	StateActive
	StatePaused
)

type Manager struct {
	mu      sync.Mutex
	state   State
	spec    LimitSpec
	shaper  Shaper
	monitor Monitor

	sysIn, sysOut atomic.Int64
	appIn, appOut atomic.Int64
	watchPID      atomic.Int32

	interval   time.Duration
	stopCh     chan struct{}
	doneCh     chan struct{}
	onChange   func()
	markerPath string

	histMu   sync.Mutex
	hist     []TrafficPoint
	histPos  int
	histFull bool
}

func (m *Manager) SetMarkerPath(p string) {
	m.mu.Lock()
	m.markerPath = p
	m.mu.Unlock()
}

func (m *Manager) writeMarker(spec LimitSpec) {
	m.mu.Lock()
	p := m.markerPath
	m.mu.Unlock()
	if p == "" || runtime.GOOS == "windows" {
		return
	}
	data, err := json.Marshal(spec)
	if err != nil {
		return
	}
	_ = os.WriteFile(p, data, 0644)
}

func (m *Manager) removeMarker() {
	m.mu.Lock()
	p := m.markerPath
	m.mu.Unlock()
	if p == "" {
		return
	}
	_ = os.Remove(p)
}

func (m *Manager) HasOrphan() bool {
	m.mu.Lock()
	p := m.markerPath
	st := m.state
	m.mu.Unlock()
	if p == "" || runtime.GOOS == "windows" || st != StateIdle {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func (m *Manager) ClearOrphan() error {
	m.mu.Lock()
	sh := m.shaper
	m.mu.Unlock()
	var err error
	if sh != nil {
		err = sh.Remove()
	}
	m.removeMarker()
	return err
}

func New() *Manager {
	return &Manager{
		monitor:  newMonitor(),
		shaper:   newShaper(),
		interval: 700 * time.Millisecond,
		hist:     make([]TrafficPoint, 600),
	}
}

func (m *Manager) SetOnChange(fn func()) {
	m.mu.Lock()
	m.onChange = fn
	m.mu.Unlock()
}

func (m *Manager) Caps() Caps {
	if m.shaper == nil {
		return Caps{}
	}
	return m.shaper.Caps()
}

func (m *Manager) ListProcs() ([]ProcInfo, error) {
	return listProcs()
}

func (m *Manager) Start() {
	m.mu.Lock()
	if m.stopCh != nil {
		m.mu.Unlock()
		return
	}
	m.stopCh = make(chan struct{})
	m.doneCh = make(chan struct{})
	stop := m.stopCh
	done := m.doneCh
	m.mu.Unlock()
	go m.sampleLoop(stop, done)
}

func (m *Manager) sampleLoop(stop, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(m.interval)
	defer t.Stop()

	var (
		haveSys              bool
		prevSysRx, prevSysTx uint64
		prevSysAt            time.Time
		watched              int32
		haveApp              bool
		prevAppRx, prevAppTx uint64
		prevAppAt            time.Time
	)

	for {
		select {
		case <-stop:
			return
		case now := <-t.C:
			if m.monitor == nil {
				continue
			}
			if rx, tx, err := m.monitor.SystemCounters(); err == nil {
				if haveSys {
					dt := now.Sub(prevSysAt).Seconds()
					if dt > 0 {
						m.sysIn.Store(rateOf(rx, prevSysRx, dt))
						m.sysOut.Store(rateOf(tx, prevSysTx, dt))
					}
				}
				prevSysRx, prevSysTx, prevSysAt, haveSys = rx, tx, now, true
			}

			pid := m.watchPID.Load()
			if pid != watched {
				watched = pid
				haveApp = false
				m.appIn.Store(0)
				m.appOut.Store(0)
			}
			if pid > 0 {
				if rx, tx, err := m.monitor.AppCounters(pid); err == nil {
					if haveApp {
						dt := now.Sub(prevAppAt).Seconds()
						if dt > 0 {
							m.appIn.Store(rateOf(rx, prevAppRx, dt))
							m.appOut.Store(rateOf(tx, prevAppTx, dt))
						}
					}
					prevAppRx, prevAppTx, prevAppAt, haveApp = rx, tx, now, true
				}
			}

			m.recordHistory(m.sysIn.Load(), m.sysOut.Load())
			m.notify()
		}
	}
}

func (m *Manager) recordHistory(in, out int64) {
	m.histMu.Lock()
	m.hist[m.histPos] = TrafficPoint{InBps: in, OutBps: out}
	m.histPos++
	if m.histPos >= len(m.hist) {
		m.histPos = 0
		m.histFull = true
	}
	m.histMu.Unlock()
}

func (m *Manager) History() []TrafficPoint {
	m.histMu.Lock()
	defer m.histMu.Unlock()
	if m.histFull {
		out := make([]TrafficPoint, 0, len(m.hist))
		out = append(out, m.hist[m.histPos:]...)
		out = append(out, m.hist[:m.histPos]...)
		return out
	}
	out := make([]TrafficPoint, m.histPos)
	copy(out, m.hist[:m.histPos])
	return out
}

func (m *Manager) Interval() time.Duration {
	return m.interval
}

func rateOf(cur, prev uint64, dt float64) int64 {
	if cur < prev {
		return 0
	}
	return int64(float64(cur-prev) / dt)
}

func (m *Manager) notify() {
	m.mu.Lock()
	fn := m.onChange
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

func (m *Manager) SetWatchPID(pid int32) {
	m.watchPID.Store(pid)
}

func (m *Manager) SystemSpeed() Sample {
	return Sample{InBps: m.sysIn.Load(), OutBps: m.sysOut.Load()}
}

func (m *Manager) AppSpeed() Sample {
	return Sample{InBps: m.appIn.Load(), OutBps: m.appOut.Load()}
}

func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *Manager) Spec() LimitSpec {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.spec
}

func (m *Manager) Active() bool {
	return m.State() == StateActive
}

func (m *Manager) Apply(spec LimitSpec) error {
	m.mu.Lock()
	sh := m.shaper
	m.mu.Unlock()
	if sh == nil {
		return errUnsupported
	}
	if err := sh.Apply(spec); err != nil {
		return err
	}
	m.mu.Lock()
	m.spec = spec
	m.state = StateActive
	m.mu.Unlock()
	m.writeMarker(spec)
	if spec.Scope == ScopeApp {
		m.SetWatchPID(spec.AppPID)
	}
	m.notify()
	return nil
}

func (m *Manager) Pause() error {
	m.mu.Lock()
	sh := m.shaper
	active := m.state == StateActive
	m.mu.Unlock()
	if !active {
		return nil
	}
	if sh != nil {
		if err := sh.Remove(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.state = StatePaused
	m.mu.Unlock()
	m.removeMarker()
	m.notify()
	return nil
}

func (m *Manager) Resume() error {
	m.mu.Lock()
	sh := m.shaper
	spec := m.spec
	paused := m.state == StatePaused
	m.mu.Unlock()
	if !paused {
		return nil
	}
	if sh != nil {
		if err := sh.Apply(spec); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.state = StateActive
	m.mu.Unlock()
	m.writeMarker(spec)
	m.notify()
	return nil
}

func (m *Manager) Cancel() error {
	m.mu.Lock()
	sh := m.shaper
	idle := m.state == StateIdle
	m.mu.Unlock()
	if idle {
		return nil
	}
	if sh != nil {
		if err := sh.Remove(); err != nil {
			return err
		}
	}
	m.mu.Lock()
	m.state = StateIdle
	m.spec = LimitSpec{}
	m.mu.Unlock()
	m.removeMarker()
	m.SetWatchPID(0)
	m.notify()
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	stop := m.stopCh
	done := m.doneCh
	m.stopCh = nil
	m.doneCh = nil
	sh := m.shaper
	active := m.state != StateIdle
	mon := m.monitor
	m.mu.Unlock()

	if stop != nil {
		close(stop)
		<-done
	}
	if active && sh != nil {
		_ = sh.Remove()
		m.removeMarker()
	}
	if mon != nil {
		return mon.Close()
	}
	return nil
}
