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

	// TunnelClosed is set when a CONNECT bridge actually finishes (TCP
	// closed by either side). For tunnels, Ended is stamped at handshake
	// completion, so Duration reflects the time to establish the tunnel
	// — not the lifetime of the underlying TCP keep-alive.
	TunnelClosed bool
}

type Store struct {
	mu     sync.RWMutex
	flows  []*Flow
	nextID uint64
	notify atomic.Pointer[func()]
}

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
	s.mu.Unlock()
	s.emit()
	return f
}

func (s *Store) Update(fn func()) {
	s.mu.Lock()
	fn()
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
	c := *s.flows[i]
	return &c
}

func (s *Store) Snapshot() []*Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Flow, len(s.flows))
	for i, f := range s.flows {
		c := *f
		out[i] = &c
	}
	return out
}

func (s *Store) Clear() {
	s.mu.Lock()
	s.flows = nil
	s.mu.Unlock()
	s.emit()
}
