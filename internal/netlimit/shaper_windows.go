//go:build windows

package netlimit

import (
	"sync"
	"time"
)

type tokenBucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newTokenBucket(rate float64) *tokenBucket {
	burst := rate * 0.25
	if burst < 64*1024 {
		burst = 64 * 1024
	}
	return &tokenBucket{rate: rate, burst: burst, tokens: burst, last: time.Now()}
}

func (b *tokenBucket) wait(n int) {
	if b.rate <= 0 {
		return
	}
	for {
		now := time.Now()
		b.tokens += now.Sub(b.last).Seconds() * b.rate
		b.last = now
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
		if b.tokens >= float64(n) {
			b.tokens -= float64(n)
			return
		}
		deficit := float64(n) - b.tokens
		sleep := time.Duration(deficit / b.rate * float64(time.Second))
		if sleep < time.Millisecond {
			sleep = time.Millisecond
		}
		time.Sleep(sleep)
	}
}

type winShaper struct {
	mu     sync.Mutex
	active bool

	netH  *wdHandle
	sockH *wdHandle
	flows map[flowKey]uint32

	inBucket    *tokenBucket
	outBucket   *tokenBucket
	totalBucket *tokenBucket

	scope     Scope
	targetPID uint32

	stop chan struct{}
	wg   sync.WaitGroup
}

func newShaper() Shaper {
	return &winShaper{}
}

func (s *winShaper) Caps() Caps {
	avail := winDivertAvailable()
	note := ""
	if !avail {
		note = "WinDivert.dll not found — place WinDivert.dll and WinDivert64.sys next to the executable to enable limiting and per-app speed"
	}
	return Caps{
		Available:      avail,
		SystemLimit:    avail,
		AppLimit:       avail,
		InboundLimit:   avail,
		PerAppSpeed:    avail,
		NeedsElevation: true,
		Note:           note,
	}
}

func (s *winShaper) Apply(spec LimitSpec) error {
	if err := winDivertLoad(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		s.removeLocked()
	}

	filter := "ip or ipv6"
	netH, err := winDivertOpen(filter, wdLayerNetwork, 0, 0)
	if err != nil {
		return err
	}
	s.netH = netH
	s.flows = make(map[flowKey]uint32)
	s.scope = spec.Scope
	s.targetPID = uint32(spec.AppPID)

	if spec.TotalBps > 0 {
		s.totalBucket = newTokenBucket(float64(spec.TotalBps))
	}
	if spec.InBps > 0 {
		s.inBucket = newTokenBucket(float64(spec.InBps))
	}
	if spec.OutBps > 0 {
		s.outBucket = newTokenBucket(float64(spec.OutBps))
	}

	s.stop = make(chan struct{})

	if spec.Scope == ScopeApp {
		sockH, err := winDivertOpen("true", wdLayerSocket, 0, wdFlagSniff|wdFlagRecvOnly)
		if err != nil {
			netH.close() //nolint:errcheck
			s.netH = nil
			return err
		}
		s.sockH = sockH
		s.wg.Add(1)
		go s.trackSockets()
	}

	s.wg.Add(1)
	go s.shapeLoop()
	s.active = true
	return nil
}

func (s *winShaper) trackSockets() {
	defer s.wg.Done()
	var addr wdAddress
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		if _, err := s.sockH.recv(nil, &addr); err != nil {
			return
		}
		if addr.layer() != wdLayerSocket {
			continue
		}
		sd := addr.socket()
		key := flowKey{proto: sd.Protocol, localPort: sd.LocalPort, remotePort: sd.RemotePort, localA: sd.LocalAddr[0], remoteA: sd.RemoteAddr[0]}
		switch addr.event() {
		case wdEventSocketConnect, wdEventSocketAccept:
			s.mu.Lock()
			s.flows[key] = sd.ProcessID
			s.mu.Unlock()
		case wdEventSocketClose:
			s.mu.Lock()
			delete(s.flows, key)
			s.mu.Unlock()
		}
	}
}

func (s *winShaper) shapeLoop() {
	defer s.wg.Done()
	buf := make([]byte, 65535)
	var addr wdAddress
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		n, err := s.netH.recv(buf, &addr)
		if err != nil {
			return
		}
		s.throttle(buf[:n], addr.outbound(), addr.ipv6())
		if _, err := s.netH.send(buf[:n], &addr); err != nil {
			return
		}
	}
}

func (s *winShaper) throttle(pkt []byte, outbound, isv6 bool) {
	if s.scope == ScopeApp {
		la, ra, lp, rp, proto, _, ok := parseFlow(pkt, outbound, isv6)
		if !ok {
			return
		}
		key := flowKey{proto: proto, localPort: lp, remotePort: rp, localA: la, remoteA: ra}
		s.mu.Lock()
		pid := s.flows[key]
		s.mu.Unlock()
		if pid != s.targetPID {
			return
		}
	}
	n := len(pkt)
	if s.totalBucket != nil {
		s.totalBucket.wait(n)
	}
	if outbound {
		if s.outBucket != nil {
			s.outBucket.wait(n)
		}
	} else {
		if s.inBucket != nil {
			s.inBucket.wait(n)
		}
	}
}

func (s *winShaper) Remove() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeLocked()
	return nil
}

func (s *winShaper) removeLocked() {
	if !s.active {
		return
	}
	if s.stop != nil {
		close(s.stop)
	}
	if s.netH != nil {
		s.netH.close() //nolint:errcheck
	}
	if s.sockH != nil {
		s.sockH.close() //nolint:errcheck
	}
	s.mu.Unlock()
	s.wg.Wait()
	s.mu.Lock()
	s.netH = nil
	s.sockH = nil
	s.flows = nil
	s.inBucket = nil
	s.outBucket = nil
	s.totalBucket = nil
	s.active = false
}
