package mitm

import (
	"sync"
	"sync/atomic"
	"time"
)

type FlowKind int

const (
	FlowHTTP FlowKind = iota
	FlowTunnel
)

// Source identifies how the flow was captured.
const (
	SrcForward = "fwd"
	SrcReverse = "rev"
)

type Flow struct {
	ID         uint64
	Kind       FlowKind
	ClientAddr string
	Started    time.Time
	Ended      time.Time

	Scheme  string
	Method  string
	Host    string
	Port    string
	Path    string
	URL     string
	Version string

	// Src is SrcForward or SrcReverse. TargetDomain names the reverse
	// Targets entry that matched (reverse flows only).
	Src          string
	TargetDomain string

	ReqHeaders [][2]string
	ReqBody    []byte
	ReqSize    int64

	Status      string
	StatusCode  int
	RespHeaders [][2]string
	RespBody    []byte
	RespSize    int64

	Error string

	BytesIn  int64
	BytesOut int64

	TunnelClosed bool

	// WebSocket marks a flow that was upgraded to a WebSocket; its frames
	// live in the proxy WSStore keyed by this flow ID.
	WebSocket bool

	// User annotations.
	Highlight string // named colour key ("", "red", "yellow", ...)
	Comment   string

	// stored marks a flow currently held by its Store, so Update knows
	// whether the flow's bodies count towards the store's byte total.
	stored bool
}

const (
	MaxFlows = 2000
	// MaxFlowBytes bounds the retained request and response bodies. A flow
	// count alone is not a memory bound: 2000 large downloads would retain
	// gigabytes.
	MaxFlowBytes = 256 << 20
)

type Store struct {
	mu     sync.RWMutex
	flows  []*Flow
	bytes  int64
	nextID uint64
	rev    uint64
	notify atomic.Pointer[func()]

	metaRev   uint64
	metaCache []*Flow
}

func flowBytes(f *Flow) int64 { return int64(len(f.ReqBody) + len(f.RespBody)) }

func (f *Flow) Live() bool { return f.Ended.IsZero() }

func NewStore() *Store {
	return &Store{}
}

func (s *Store) SetNotify(fn func()) {
	if fn == nil {
		s.notify.Store(nil)
		return
	}
	s.notify.Store(&fn)
}

func (s *Store) emit() {
	if p := s.notify.Load(); p != nil {
		(*p)()
	}
}

func (s *Store) Add(f *Flow) *Flow {
	s.mu.Lock()
	s.nextID++
	f.ID = s.nextID
	if f.Started.IsZero() {
		f.Started = time.Now()
	}
	s.flows = append(s.flows, f)
	f.stored = true
	s.bytes += flowBytes(f)
	drop := 0
	for drop < len(s.flows)-1 && (len(s.flows)-drop > MaxFlows || s.bytes > MaxFlowBytes) {
		s.bytes -= flowBytes(s.flows[drop])
		s.flows[drop].stored = false
		drop++
	}
	if drop > 0 {
		s.flows = append(s.flows[:0], s.flows[drop:]...)
		clear(s.flows[len(s.flows):cap(s.flows)])
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
	return f
}

// Rev changes whenever any flow does, so callers can cache derived views
// instead of rebuilding them every frame.
func (s *Store) Rev() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

// Update runs fn under the store lock. It takes the flow fn mutates so the
// running body-byte total can absorb the change without rescanning the store.
func (s *Store) Update(f *Flow, fn func()) {
	s.mu.Lock()
	var before int64
	if f != nil && f.stored {
		before = flowBytes(f)
	}
	fn()
	if f != nil && f.stored {
		s.bytes += flowBytes(f) - before
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
}

func (s *Store) MarkAllEnded() {
	now := time.Now()
	s.mu.Lock()
	for _, f := range s.flows {
		if f.Ended.IsZero() {
			f.Ended = now
		}
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
}

func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.flows)
}

func (s *Store) At(i int) *Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if i < 0 || i >= len(s.flows) {
		return nil
	}
	c := cloneFlow(s.flows[i])
	return c
}

func (s *Store) Snapshot() []*Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Flow, len(s.flows))
	for i, f := range s.flows {
		out[i] = cloneFlow(f)
	}
	return out
}

// SnapshotMeta returns body-free copies of every flow. The result is cached
// until a flow changes, so a redraw that follows no capture activity costs
// nothing. Each rebuild allocates fresh backing, so an already handed-out
// snapshot stays consistent; callers must still treat the slice and its
// elements as read-only.
func (s *Store) SnapshotMeta() []*Flow {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.metaCache != nil && s.metaRev == s.rev {
		return s.metaCache
	}
	back := make([]Flow, len(s.flows))
	cache := make([]*Flow, len(s.flows))
	for i, f := range s.flows {
		back[i] = *f
		back[i].ReqBody = nil
		back[i].RespBody = nil
		back[i].ReqHeaders = nil
		back[i].RespHeaders = nil
		cache[i] = &back[i]
	}
	s.metaCache = cache
	s.metaRev = s.rev
	return s.metaCache
}

// FindByID returns the flow with the given ID, or nil. A finished flow is no
// longer touched by the proxy, so its bodies and headers are shared rather
// than copied; callers must treat the result as read-only.
func (s *Store) FindByID(id uint64) *Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.flows {
		if f.ID == id {
			if f.Live() {
				return cloneFlow(f)
			}
			c := *f
			c.stored = false
			return &c
		}
	}
	return nil
}

func cloneFlow(f *Flow) *Flow {
	if f == nil {
		return nil
	}
	c := *f
	c.stored = false
	if f.ReqBody != nil {
		c.ReqBody = append([]byte(nil), f.ReqBody...)
	}
	if f.RespBody != nil {
		c.RespBody = append([]byte(nil), f.RespBody...)
	}
	if f.ReqHeaders != nil {
		c.ReqHeaders = append([][2]string(nil), f.ReqHeaders...)
	}
	if f.RespHeaders != nil {
		c.RespHeaders = append([][2]string(nil), f.RespHeaders...)
	}
	return &c
}

func (s *Store) Clear() {
	s.mu.Lock()
	for _, f := range s.flows {
		f.stored = false
	}
	s.flows = nil
	s.bytes = 0
	s.rev++
	s.mu.Unlock()
	s.emit()
}

// Delete removes a single flow by ID.
func (s *Store) Delete(id uint64) {
	s.mu.Lock()
	for i, f := range s.flows {
		if f.ID == id {
			f.stored = false
			s.bytes -= flowBytes(f)
			s.flows = append(s.flows[:i], s.flows[i+1:]...)
			clear(s.flows[len(s.flows):cap(s.flows)])
			break
		}
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
}

// SetAnnotation records a highlight colour key and/or comment on a flow.
func (s *Store) SetAnnotation(id uint64, highlight, comment string) {
	s.mu.Lock()
	for _, f := range s.flows {
		if f.ID == id {
			f.Highlight = highlight
			f.Comment = comment
			break
		}
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
}
