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

const maxWSMessages = 5000

type WSStore struct {
	mu     sync.RWMutex
	msgs   []*WSMessage
	nextID uint64
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
	if len(s.msgs) > maxWSMessages {
		drop := len(s.msgs) - maxWSMessages
		s.msgs = append(s.msgs[:0], s.msgs[drop:]...)
	}
	s.mu.Unlock()
	s.emit()
}

func (s *WSStore) Snapshot() []*WSMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*WSMessage, len(s.msgs))
	for i, m := range s.msgs {
		c := *m
		c.Payload = append([]byte(nil), m.Payload...)
		out[i] = &c
	}
	return out
}

func (s *WSStore) FindByID(id uint64) *WSMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.msgs {
		if m.ID == id {
			c := *m
			c.Payload = append([]byte(nil), m.Payload...)
			return &c
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
