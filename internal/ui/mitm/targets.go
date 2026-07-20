package mitm

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Upstream resolution mode for a reverse target.
const (
	UpstreamAuto   = "auto"   // resolve the real address via DNS/DoH
	UpstreamManual = "manual" // dial a user-supplied ip:port
)

// Per-target TLS handling.
const (
	TLSDecrypt = "decrypt" // terminate TLS with our CA and inspect
	TLSTunnel  = "tunnel"  // pass encrypted bytes straight through
)

// Live status of a reverse target.
const (
	StatusWaiting  = "waiting"  // routing not set up yet / no requests
	StatusProxying = "proxying" // at least one request has flowed
	StatusError    = "error"    // last upstream connection failed
)

// Target is one reverse-proxied domain.
type Target struct {
	Domain       string
	Upstream     string // UpstreamAuto | UpstreamManual
	UpstreamAddr string // ip:port when Upstream==manual
	TLS          string // TLSDecrypt | TLSTunnel
	Delay        time.Duration
	DoH          bool

	// runtime, guarded by Targets.mu
	status  string
	lastErr string
	reqs    int64
}

// TargetView is an immutable snapshot of a Target for the UI.
type TargetView struct {
	Target
	Status   string
	LastErr  string
	Requests int64
}

// Targets is the reverse-proxy domain registry.
type Targets struct {
	mu     sync.RWMutex
	list   []*Target
	notify atomic.Pointer[func()]
}

func NewTargets() *Targets { return &Targets{} }

func (t *Targets) SetNotify(fn func()) {
	if fn == nil {
		t.notify.Store(nil)
		return
	}
	t.notify.Store(&fn)
}

func (t *Targets) emit() {
	if p := t.notify.Load(); p != nil {
		(*p)()
	}
}

func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimSuffix(d, ".")
	if h, _, err := net.SplitHostPort(d); err == nil {
		d = h
	}
	return d
}

// Add registers a new reverse target. Returns false if the domain is invalid
// or already present.
func (t *Targets) Add(tg *Target) bool {
	d := normalizeDomain(tg.Domain)
	if !ValidDomain(d) {
		return false
	}
	t.mu.Lock()
	for _, e := range t.list {
		if e.Domain == d {
			t.mu.Unlock()
			return false
		}
	}
	tg.Domain = d
	if tg.Upstream == "" {
		tg.Upstream = UpstreamAuto
	}
	if tg.TLS == "" {
		tg.TLS = TLSDecrypt
	}
	tg.status = StatusWaiting
	t.list = append(t.list, tg)
	t.mu.Unlock()
	t.emit()
	return true
}

// Update replaces the editable fields of an existing target.
func (t *Targets) Update(domain string, edit func(*Target)) {
	d := normalizeDomain(domain)
	t.mu.Lock()
	for _, e := range t.list {
		if e.Domain == d {
			edit(e)
			break
		}
	}
	t.mu.Unlock()
	t.emit()
}

func (t *Targets) Remove(domain string) {
	d := normalizeDomain(domain)
	t.mu.Lock()
	for i, e := range t.list {
		if e.Domain == d {
			t.list = append(t.list[:i], t.list[i+1:]...)
			break
		}
	}
	t.mu.Unlock()
	t.emit()
}

func (t *Targets) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.list)
}

func (t *Targets) Snapshot() []TargetView {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]TargetView, len(t.list))
	for i, e := range t.list {
		out[i] = TargetView{
			Target:   *e,
			Status:   e.status,
			LastErr:  e.lastErr,
			Requests: e.reqs,
		}
	}
	return out
}

// Match finds the target whose domain matches host, honouring wildcard
// entries of the form "*.example.com".
func (t *Targets) Match(host string) (*Target, bool) {
	h := normalizeDomain(host)
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, e := range t.list {
		if domainMatches(e.Domain, h) {
			return e, true
		}
	}
	return nil, false
}

func (t *Targets) markRequest(tg *Target) {
	t.mu.Lock()
	tg.reqs++
	tg.status = StatusProxying
	tg.lastErr = ""
	t.mu.Unlock()
	t.emit()
}

func (t *Targets) markError(tg *Target, err string) {
	t.mu.Lock()
	tg.status = StatusError
	tg.lastErr = err
	t.mu.Unlock()
	t.emit()
}

func domainMatches(pattern, host string) bool {
	if pattern == host {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[1:] // ".example.com"
		return strings.HasSuffix(host, suffix) && host != suffix[1:]
	}
	return false
}

// ValidDomain reports whether s is a syntactically valid domain or wildcard
// domain (e.g. example.com, sub.example.com, *.example.com).
func ValidDomain(s string) bool {
	s = normalizeDomain(s)
	if s == "" || len(s) > 253 {
		return false
	}
	if strings.HasPrefix(s, "*.") {
		s = s[2:]
	}
	if s == "" {
		return false
	}
	labels := strings.Split(s, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 {
			return false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			ok := (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-'
			if !ok {
				return false
			}
		}
		if l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
	}
	return true
}

// HostsLine returns the line a user must add to their system hosts file to
// route a target domain to the local proxy.
func HostsLine(domain string) string {
	d := normalizeDomain(domain)
	d = strings.TrimPrefix(d, "*.")
	return "127.0.0.1    " + d
}
