package mitm

import (
	"sync"
	"sync/atomic"
)

// Held message kinds.
const (
	HeldRequest  = "request"
	HeldResponse = "response"
)

// Interceptor implements the manual-interception queue used by the Intercept
// view. When On, matching messages are paused until the user forwards or drops
// them. It is independent of the HTTPS-decrypt toggle (Proxy.SetIntercept).
type Interceptor struct {
	on       atomic.Bool
	catchAll atomic.Bool // ignore intercept-rules, hold everything
	doResp   atomic.Bool

	mu     sync.Mutex
	queue  []*Held
	nextID uint64
	notify func()
}

// Held is one paused message awaiting a decision.
type Held struct {
	ID     uint64
	FlowID uint64
	Kind   string // HeldRequest | HeldResponse
	Method string
	URL    string
	Host   string
	Src    string
	Raw    []byte // editable serialized message

	resume chan heldResult
}

type heldResult struct {
	edited []byte
	drop   bool
}

func NewInterceptor() *Interceptor { return &Interceptor{} }

func (in *Interceptor) SetNotify(fn func()) { in.notify = fn }

func (in *Interceptor) emit() {
	if in.notify != nil {
		in.notify()
	}
}

func (in *Interceptor) SetOn(on bool) {
	in.on.Store(on)
	if !on {
		in.drainAll()
	}
	in.emit()
}
func (in *Interceptor) On() bool { return in.on.Load() }

func (in *Interceptor) SetInterceptResponses(on bool) { in.doResp.Store(on) }
func (in *Interceptor) InterceptResponses() bool      { return in.doResp.Load() }

func (in *Interceptor) Len() int {
	in.mu.Lock()
	defer in.mu.Unlock()
	return len(in.queue)
}

// Queue returns a shallow snapshot of pending held messages (Raw shared).
func (in *Interceptor) Queue() []*Held {
	in.mu.Lock()
	defer in.mu.Unlock()
	out := make([]*Held, len(in.queue))
	copy(out, in.queue)
	return out
}

// Hold pauses a message, blocking the calling proxy goroutine until the user
// resolves it. It returns the (possibly edited) bytes to forward and whether
// the message should be dropped. If interception is off it returns immediately
// with the original raw bytes.
func (in *Interceptor) Hold(h *Held) ([]byte, bool) {
	if !in.on.Load() {
		return h.Raw, false
	}
	h.resume = make(chan heldResult, 1)
	in.mu.Lock()
	in.nextID++
	h.ID = in.nextID
	in.queue = append(in.queue, h)
	in.mu.Unlock()
	in.emit()

	res := <-h.resume
	return res.edited, res.drop
}

func (in *Interceptor) resolve(id uint64, edited []byte, drop bool) {
	in.mu.Lock()
	var found *Held
	for i, h := range in.queue {
		if h.ID == id {
			found = h
			in.queue = append(in.queue[:i], in.queue[i+1:]...)
			break
		}
	}
	in.mu.Unlock()
	if found != nil {
		if edited == nil {
			edited = found.Raw
		}
		found.resume <- heldResult{edited: edited, drop: drop}
		in.emit()
	}
}

// Forward releases a held message, optionally with edited bytes (nil = as-is).
func (in *Interceptor) Forward(id uint64, edited []byte) { in.resolve(id, edited, false) }

// Drop discards a held message and closes its connection.
func (in *Interceptor) Drop(id uint64) { in.resolve(id, nil, true) }

// drainAll forwards every queued message unchanged (used when turning off).
func (in *Interceptor) drainAll() {
	in.mu.Lock()
	q := in.queue
	in.queue = nil
	in.mu.Unlock()
	for _, h := range q {
		h.resume <- heldResult{edited: h.Raw, drop: false}
	}
	in.emit()
}
