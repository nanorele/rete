package mitm

import (
	"sync"
	"sync/atomic"
	"time"
)

// WSMessage is one captured WebSocket frame.
type WSMessage struct {
	ID       uint64
	FlowID   uint64
	URL      string
	ToServer bool // client -> server when true
	Opcode   byte // 0x1 text, 0x2 binary, 0x8 close, 0x9 ping, 0xA pong
	Payload  []byte
	Time     time.Time
}

const (
	maxWSMessages = 5000
	// maxWSBytes bounds the retained payloads. A message count alone is not a
	// memory bound: 5000 multi-megabyte frames would retain gigabytes.
	maxWSBytes = 64 << 20
)

type WSStore struct {
	mu     sync.RWMutex
	msgs   []*WSMessage
	bytes  int64
	nextID uint64
	rev    uint64
	notify atomic.Pointer[func()]
}

func NewWSStore() *WSStore { return &WSStore{} }

func (s *WSStore) SetNotify(fn func()) {
	if fn == nil {
		s.notify.Store(nil)
		return
	}
	s.notify.Store(&fn)
}

func (s *WSStore) emit() {
	if p := s.notify.Load(); p != nil {
		(*p)()
	}
}

func (s *WSStore) Add(m *WSMessage) {
	s.mu.Lock()
	s.nextID++
	m.ID = s.nextID
	if m.Time.IsZero() {
		m.Time = time.Now()
	}
	s.msgs = append(s.msgs, m)
	s.bytes += int64(len(m.Payload))
	drop := 0
	for drop < len(s.msgs)-1 && (len(s.msgs)-drop > maxWSMessages || s.bytes > maxWSBytes) {
		s.bytes -= int64(len(s.msgs[drop].Payload))
		drop++
	}
	if drop > 0 {
		s.msgs = append(s.msgs[:0], s.msgs[drop:]...)
		clear(s.msgs[len(s.msgs):cap(s.msgs)])
	}
	s.rev++
	s.mu.Unlock()
	s.emit()
}

// Rev changes whenever the message list does, so callers can cache derived
// views instead of rebuilding them every frame.
func (s *WSStore) Rev() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.rev
}

// Snapshot returns the captured messages in order. A message is never mutated
// after [WSStore.Add] takes it, so the returned values are shared rather than
// deep-copied; callers must treat them, and their payloads, as read-only.
func (s *WSStore) Snapshot() []*WSMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WSMessage, len(s.msgs))
	copy(out, s.msgs)
	return out
}

// FindByID returns the message with the given ID, or nil. The result is shared
// and read-only, as with [WSStore.Snapshot].
func (s *WSStore) FindByID(id uint64) *WSMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.msgs {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (s *WSStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.msgs)
}

func (s *WSStore) Clear() {
	s.mu.Lock()
	s.msgs = nil
	s.bytes = 0
	s.rev++
	s.mu.Unlock()
	s.emit()
}

// WSOpcodeName returns a short label for a frame opcode.
func WSOpcodeName(op byte) string {
	switch op {
	case 0x1:
		return "text"
	case 0x2:
		return "binary"
	case 0x8:
		return "close"
	case 0x9:
		return "ping"
	case 0xA:
		return "pong"
	}
	return "cont"
}
