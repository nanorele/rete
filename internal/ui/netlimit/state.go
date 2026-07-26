package netlimit

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	netlim "tracto/internal/netlimit"
	"tracto/internal/persist"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
)

type Section struct {
	host *Host
	mgr  *netlim.Manager

	caps netlim.Caps

	scope    netlim.Scope
	scopeSys widget.Clickable
	scopeApp widget.Clickable

	inEd      widget.Editor
	outEd     widget.Editor
	totalEd   widget.Editor
	inUnit    unitSel
	outUnit   unitSel
	totalUnit unitSel

	startBtn  widget.Clickable
	stopBtn   widget.Clickable
	resumeBtn widget.Clickable
	cancelBtn widget.Clickable
	relaunch  widget.Clickable

	pickBtn       widget.Clickable
	pickerOpen    bool
	searchEd      widget.Editor
	procList      widget.List
	procClicks    []widget.Clickable
	procListHover widgets.Hover
	selApp        netlim.ProcInfo
	hasApp        bool

	orphan         bool
	clearOrphanBtn widget.Clickable

	secList     widget.List
	graphWindow time.Duration
	win30Btn    widget.Clickable
	win1mBtn    widget.Clickable
	win5mBtn    widget.Clickable

	diagBtn     widget.Clickable
	diagRunning bool
	diagLines   []diagLine

	mu           sync.Mutex
	procs        []netlim.ProcInfo
	procsLoading bool
	lastErr      string
}

type diagLine struct {
	label string
	value string
	ok    int8
}

var units = []struct {
	label string
	mul   int64
}{
	{"KB/s", 1024},
	{"MB/s", 1024 * 1024},
	{"GB/s", 1024 * 1024 * 1024},
}

type unitSel struct {
	idx    int
	clicks []widget.Clickable
}

func (u *unitSel) ensure() {
	if len(u.clicks) < len(units) {
		u.clicks = make([]widget.Clickable, len(units))
	}
}

func (u *unitSel) mul() int64 {
	if u.idx < 0 || u.idx >= len(units) {
		return units[0].mul
	}
	return units[u.idx].mul
}

type config struct {
	Scope   int    `json:"scope"`
	AppPath string `json:"app_path,omitempty"`
	AppName string `json:"app_name,omitempty"`
	In      string `json:"in,omitempty"`
	Out     string `json:"out,omitempty"`
	Total   string `json:"total,omitempty"`
	UnitMB  bool   `json:"unit_mb"`
	Unit    int    `json:"unit"`
	InUnit  int    `json:"in_unit"`
	OutUnit int    `json:"out_unit"`
	TotUnit int    `json:"total_unit"`
}

func (s *Section) Init() {
	s.mgr = netlim.New()
	s.mgr.SetMarkerPath(persist.NetlimitMarkerPath())
	s.caps = s.mgr.Caps()
	s.inEd.SingleLine = true
	s.outEd.SingleLine = true
	s.totalEd.SingleLine = true
	s.searchEd.SingleLine = true
	s.inUnit.idx = 1
	s.outUnit.idx = 1
	s.totalUnit.idx = 1
	s.inUnit.ensure()
	s.outUnit.ensure()
	s.totalUnit.ensure()
	s.procList.Axis = layout.Vertical
	s.secList.Axis = layout.Vertical
	s.graphWindow = time.Minute
	s.loadConfig()
	s.orphan = s.mgr.HasOrphan()
	s.mgr.Start()
}

func (s *Section) loadConfig() {
	data, err := os.ReadFile(persist.NetlimitConfigPath())
	if err != nil {
		return
	}
	var c config
	if json.Unmarshal(data, &c) != nil {
		return
	}
	s.scope = netlim.Scope(c.Scope)
	clampUnit := func(v int) int {
		if v < 0 || v >= len(units) {
			return 0
		}
		return v
	}
	if c.InUnit == 0 && c.OutUnit == 0 && c.TotUnit == 0 {
		shared := 0
		if c.Unit > 0 && c.Unit < len(units) {
			shared = c.Unit
		} else if c.UnitMB {
			shared = 1
		}
		s.inUnit.idx = shared
		s.outUnit.idx = shared
		s.totalUnit.idx = shared
	} else {
		s.inUnit.idx = clampUnit(c.InUnit)
		s.outUnit.idx = clampUnit(c.OutUnit)
		s.totalUnit.idx = clampUnit(c.TotUnit)
	}
	s.inEd.SetText(c.In)
	s.outEd.SetText(c.Out)
	s.totalEd.SetText(c.Total)
	if c.AppName != "" {
		s.selApp = netlim.ProcInfo{Name: c.AppName, Exe: c.AppPath}
		s.hasApp = true
	}
}

func (s *Section) saveConfig() {
	c := config{
		Scope:   int(s.scope),
		AppPath: s.selApp.Exe,
		AppName: s.selApp.Name,
		In:      s.inEd.Text(),
		Out:     s.outEd.Text(),
		Total:   s.totalEd.Text(),
		InUnit:  s.inUnit.idx,
		OutUnit: s.outUnit.idx,
		TotUnit: s.totalUnit.idx,
		Unit:    s.inUnit.idx,
		UnitMB:  s.inUnit.idx >= 1,
	}
	if data, err := json.Marshal(c); err == nil {
		_ = persist.AtomicWriteFile(persist.NetlimitConfigPath(), data)
	}
}

func (s *Section) Close() {
	if s.mgr != nil {
		_ = s.mgr.Close()
	}
}

func (s *Section) Started() bool {
	return s.mgr != nil
}

func (s *Section) Status() (active, paused bool) {
	state := s.mgr.State()
	return state == netlim.StateActive, state == netlim.StatePaused
}

func (s *Section) ToggleLimit(invalidate func()) {
	go func() {
		if s.mgr.State() == netlim.StatePaused {
			s.setErr(s.mgr.Resume())
		} else {
			s.setErr(s.mgr.Pause())
		}
		invalidate()
	}()
}

func (s *Section) CancelLimit(invalidate func()) {
	go func() {
		s.setErr(s.mgr.Cancel())
		invalidate()
	}()
}

func (s *Section) setErr(err error) {
	s.mu.Lock()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
}

func (s *Section) getErr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Section) setProcs(p []netlim.ProcInfo) {
	s.mu.Lock()
	s.procs = p
	s.procsLoading = false
	s.mu.Unlock()
}

func (s *Section) getProcs() []netlim.ProcInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.procs
}

func (s *Section) isProcsLoading() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.procsLoading
}

func (s *Section) buildSpec() netlim.LimitSpec {
	parse := func(ed *widget.Editor, u *unitSel) int64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(ed.Text()), 64)
		if err != nil || v <= 0 {
			return 0
		}
		return int64(v * float64(u.mul()))
	}
	spec := netlim.LimitSpec{
		Scope:    s.scope,
		InBps:    parse(&s.inEd, &s.inUnit),
		OutBps:   parse(&s.outEd, &s.outUnit),
		TotalBps: parse(&s.totalEd, &s.totalUnit),
	}
	if s.scope == netlim.ScopeApp && s.hasApp {
		spec.AppPID = s.selApp.PID
		spec.AppName = s.selApp.Name
		spec.AppPath = s.selApp.Exe
	}
	return spec
}

func (s *Section) loadProcs() {
	s.mu.Lock()
	if s.procsLoading {
		s.mu.Unlock()
		return
	}
	s.procsLoading = true
	s.mu.Unlock()
	win := s.host.Window
	go func() {
		procs, _ := s.mgr.ListProcs()
		s.setProcs(procs)
		if win != nil {
			win.Invalidate()
		}
	}()
}

func (s *Section) buildDiagnostics() []diagLine {
	caps := s.caps
	var out []diagLine

	if caps.Available {
		out = append(out, diagLine{"Limiter backend", "available", 1})
	} else {
		out = append(out, diagLine{"Limiter backend", "unavailable", -1})
	}

	if caps.NeedsElevation {
		if netlim.IsElevated() {
			out = append(out, diagLine{"Privileges", "elevated", 1})
		} else {
			out = append(out, diagLine{"Privileges", "not elevated", -1})
		}
	}

	if caps.PerAppSpeed {
		out = append(out, diagLine{"Per-app monitoring", "supported", 1})
	} else {
		out = append(out, diagLine{"Per-app monitoring", "unsupported", 0})
	}

	for _, t := range []string{"1.1.1.1:443", "8.8.8.8:53"} {
		r := netlim.TCPPing(t, 3*time.Second)
		if r.OK {
			out = append(out, diagLine{"Ping " + t, fmt.Sprintf("%d ms", r.Latency.Milliseconds()), 1})
		} else {
			out = append(out, diagLine{"Ping " + t, "no response", -1})
		}
	}

	if ifaces, err := net.Interfaces(); err == nil {
		active := 0
		for _, in := range ifaces {
			if in.Flags&net.FlagUp != 0 && in.Flags&net.FlagLoopback == 0 {
				if addrs, _ := in.Addrs(); len(addrs) > 0 {
					active++
				}
			}
		}
		out = append(out, diagLine{"Active interfaces", strconv.Itoa(active), 0})
	}

	hist := s.mgr.History()
	var pIn, pOut int64
	for _, p := range hist {
		if p.InBps > pIn {
			pIn = p.InBps
		}
		if p.OutBps > pOut {
			pOut = p.OutBps
		}
	}
	out = append(out, diagLine{"Session peak ↓", formatRate(pIn), 0})
	out = append(out, diagLine{"Session peak ↑", formatRate(pOut), 0})

	return out
}
