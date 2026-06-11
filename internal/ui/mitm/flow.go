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

	TunnelClosed bool
}

const MaxFlows = 2000

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
	if len(s.flows) > MaxFlows {
		drop := len(s.flows) - MaxFlows
		s.flows = append(s.flows[:0], s.flows[drop:]...)
	}
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

func (s *Store) SnapshotMeta() []*Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Flow, len(s.flows))
	for i, f := range s.flows {
		c := *f
		c.ReqBody = nil
		c.RespBody = nil
		c.ReqHeaders = nil
		c.RespHeaders = nil
		out[i] = &c
	}
	return out
}

func (s *Store) FindByID(id uint64) *Flow {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.flows {
		if f.ID == id {
			return cloneFlow(f)
		}
	}
	return nil
}

func cloneFlow(f *Flow) *Flow {
	if f == nil {
		return nil
	}
	c := *f
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
	s.flows = nil
	s.mu.Unlock()
	s.emit()
}
