//go:build windows

package netlimit

import (
	"testing"
	"time"
)

func fullBucket(burst float64) *tokenBucket {
	return &tokenBucket{rate: 1e9, burst: burst, tokens: burst, last: time.Now()}
}

func TestNewTokenBucketBurst(t *testing.T) {
	const floor = 64 * 1024
	tests := []struct {
		name      string
		rate      float64
		wantBurst float64
	}{
		{"zero rate clamps to floor", 0, floor},
		{"negative rate clamps to floor", -1000, floor},
		{"small rate clamps to floor", 1000, floor},
		{"quarter below floor", 262143, floor},
		{"quarter exactly at floor", 262144, floor},
		{"quarter above floor", 1_000_000, 250_000},
		{"large rate", 1 << 30, float64(1<<30) * 0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTokenBucket(tt.rate)
			if b.rate != tt.rate {
				t.Fatalf("rate = %v, want %v", b.rate, tt.rate)
			}
			if b.burst != tt.wantBurst {
				t.Fatalf("burst = %v, want %v", b.burst, tt.wantBurst)
			}
			if b.tokens != tt.wantBurst {
				t.Fatalf("initial tokens = %v, want burst %v", b.tokens, tt.wantBurst)
			}
			if b.last.IsZero() {
				t.Fatal("last not initialised")
			}
		})
	}
}

func TestTokenBucketWaitNoRateIsNoop(t *testing.T) {
	tests := []struct {
		name string
		rate float64
	}{
		{"zero rate", 0},
		{"negative rate", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &tokenBucket{rate: tt.rate, burst: 100, tokens: 7, last: time.Time{}}
			b.wait(1 << 20)
			if b.tokens != 7 {
				t.Fatalf("tokens = %v, want untouched 7", b.tokens)
			}
			if !b.last.IsZero() {
				t.Fatal("last must not be touched when rate <= 0")
			}
		})
	}
}

func TestTokenBucketWaitDeductsFromSaturatedBucket(t *testing.T) {
	tests := []struct {
		name  string
		burst float64
		n     int
		want  float64
	}{
		{"single byte", 65536, 1, 65535},
		{"typical mtu", 65536, 1500, 64036},
		{"exactly burst", 65536, 65536, 0},
		{"max sniffed packet", 65536, 65535, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &tokenBucket{
				rate:   1000,
				burst:  tt.burst,
				tokens: 0,
				last:   time.Now().Add(-time.Hour),
			}
			b.wait(tt.n)
			if b.tokens != tt.want {
				t.Fatalf("tokens = %v, want %v", b.tokens, tt.want)
			}
			if time.Since(b.last) > time.Minute {
				t.Fatalf("last not advanced: %v", b.last)
			}
		})
	}
}

func TestTokenBucketWaitClampsAccrualToBurst(t *testing.T) {
	b := &tokenBucket{rate: 1, burst: 100, tokens: 1e9, last: time.Now()}
	b.wait(10)
	if b.tokens != 90 {
		t.Fatalf("tokens = %v, want 90 (clamped to burst then debited)", b.tokens)
	}
}

func TestTokenBucketWaitSpendsWithoutRefill(t *testing.T) {
	b := &tokenBucket{rate: 1, burst: 65536, tokens: 1000, last: time.Now()}
	b.wait(400)
	if b.tokens < 600 || b.tokens >= 601 {
		t.Fatalf("tokens = %v, want just above 600 (rate 1 B/s accrues ~0)", b.tokens)
	}
	b.wait(600)
	if b.tokens < 0 || b.tokens >= 1 {
		t.Fatalf("tokens = %v, want just above 0", b.tokens)
	}
}

func TestTokenBucketWaitRefillsWhenShort(t *testing.T) {
	start := time.Now()
	b := &tokenBucket{rate: 1e9, burst: 250e6, tokens: 0, last: start}
	b.wait(1_000_000)
	if b.tokens < 0 {
		t.Fatalf("tokens went negative: %v", b.tokens)
	}
	if b.tokens > b.burst {
		t.Fatalf("tokens %v exceed burst %v", b.tokens, b.burst)
	}
	if !b.last.After(start) {
		t.Fatalf("last = %v, want advanced past %v", b.last, start)
	}
}

func TestWinShaperCapsInvariants(t *testing.T) {
	s := newShaper()
	caps := s.Caps()
	if !caps.NeedsElevation {
		t.Fatal("windows shaper must report NeedsElevation")
	}
	if caps.Available != caps.SystemLimit ||
		caps.Available != caps.AppLimit ||
		caps.Available != caps.InboundLimit ||
		caps.Available != caps.PerAppSpeed {
		t.Fatalf("capability flags must track Available: %+v", caps)
	}
	if caps.Available && caps.Note != "" {
		t.Fatalf("Note must be empty when available, got %q", caps.Note)
	}
	if !caps.Available && caps.Note == "" {
		t.Fatal("Note must explain why the shaper is unavailable")
	}
}

func TestWinShaperRemoveInactive(t *testing.T) {
	s := &winShaper{}
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove on inactive shaper = %v", err)
	}
	if err := s.Remove(); err != nil {
		t.Fatalf("second Remove = %v", err)
	}
	if s.active {
		t.Fatal("inactive shaper became active")
	}
}

func TestWinShaperApplyWithoutDriver(t *testing.T) {
	if winDivertAvailable() {
		t.Log("WinDivert present; not exercising Apply against the live driver")
		return
	}
	s := &winShaper{}
	err := s.Apply(LimitSpec{InBps: 1000})
	if err == nil {
		t.Fatal("Apply without WinDivert must fail")
	}
	if s.active {
		t.Fatal("failed Apply left the shaper active")
	}
}

func TestWinShaperRemoveActiveTearsDownState(t *testing.T) {
	s := &winShaper{
		active:      true,
		stop:        make(chan struct{}),
		flows:       map[flowKey]uint32{{proto: 6}: 1},
		inBucket:    newTokenBucket(1000),
		outBucket:   newTokenBucket(2000),
		totalBucket: newTokenBucket(3000),
		scope:       ScopeApp,
		targetPID:   99,
	}
	stop := s.stop
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.active {
		t.Fatal("shaper still active after Remove")
	}
	if s.inBucket != nil || s.outBucket != nil || s.totalBucket != nil {
		t.Fatal("Remove must drop the token buckets")
	}
	if s.flows != nil {
		t.Fatal("Remove must drop the flow table")
	}
	if s.netH != nil || s.sockH != nil {
		t.Fatal("Remove must drop the handles")
	}
	select {
	case <-stop:
	default:
		t.Fatal("Remove must close the stop channel")
	}
	if s.active {
		t.Fatal("Remove must clear active before releasing the lock to prevent a double close")
	}
	if err := s.Remove(); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
}

func TestWinShaperThrottleSystemScope(t *testing.T) {
	pkt := ipv4TCP(t, 0x0A000001, 0x5DB8D822, 4000, 443, 60)

	tests := []struct {
		name      string
		outbound  bool
		wantTotal float64
		wantIn    float64
		wantOut   float64
	}{
		{"outbound debits total and out", true, 65536 - 60, 65536, 65536 - 60},
		{"inbound debits total and in", false, 65536 - 60, 65536 - 60, 65536},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winShaper{
				scope:       ScopeSystem,
				totalBucket: fullBucket(65536),
				inBucket:    fullBucket(65536),
				outBucket:   fullBucket(65536),
			}
			s.throttle(pkt, tt.outbound, false)
			if s.totalBucket.tokens != tt.wantTotal {
				t.Fatalf("total tokens = %v, want %v", s.totalBucket.tokens, tt.wantTotal)
			}
			if s.inBucket.tokens != tt.wantIn {
				t.Fatalf("in tokens = %v, want %v", s.inBucket.tokens, tt.wantIn)
			}
			if s.outBucket.tokens != tt.wantOut {
				t.Fatalf("out tokens = %v, want %v", s.outBucket.tokens, tt.wantOut)
			}
		})
	}
}

func TestWinShaperThrottleNilBuckets(t *testing.T) {
	pkt := ipv4TCP(t, 1, 2, 10, 20, 40)
	s := &winShaper{scope: ScopeSystem}
	s.throttle(pkt, true, false)
	s.throttle(pkt, false, false)
}

func TestWinShaperThrottleAppScope(t *testing.T) {
	const localA, remoteA uint32 = 0x0A000001, 0x5DB8D822
	const localPort, remotePort uint16 = 4000, 443
	pkt := ipv4TCP(t, localA, remoteA, localPort, remotePort, 100)
	knownKey := flowKey{proto: 6, localPort: localPort, remotePort: remotePort, localA: localA, remoteA: remoteA}

	tests := []struct {
		name      string
		flows     map[flowKey]uint32
		targetPID uint32
		pkt       []byte
		wantSpend float64
	}{
		{
			name:      "matching pid is throttled",
			flows:     map[flowKey]uint32{knownKey: 900},
			targetPID: 900,
			pkt:       pkt,
			wantSpend: 100,
		},
		{
			name:      "other pid is untouched",
			flows:     map[flowKey]uint32{knownKey: 901},
			targetPID: 900,
			pkt:       pkt,
			wantSpend: 0,
		},
		{
			name:      "unknown flow is untouched",
			flows:     map[flowKey]uint32{},
			targetPID: 900,
			pkt:       pkt,
			wantSpend: 0,
		},
		{
			name:      "unparseable packet is untouched",
			flows:     map[flowKey]uint32{knownKey: 900},
			targetPID: 900,
			pkt:       make([]byte, 8),
			wantSpend: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winShaper{
				scope:       ScopeApp,
				targetPID:   tt.targetPID,
				flows:       tt.flows,
				totalBucket: fullBucket(65536),
			}
			s.throttle(tt.pkt, true, false)
			if got := 65536 - s.totalBucket.tokens; got != tt.wantSpend {
				t.Fatalf("tokens spent = %v, want %v", got, tt.wantSpend)
			}
		})
	}
}

func TestWinShaperThrottleAppScopeInbound(t *testing.T) {
	const localA, remoteA uint32 = 0x0A000001, 0x5DB8D822
	const localPort, remotePort uint16 = 4000, 443
	pkt := ipv4TCP(t, remoteA, localA, remotePort, localPort, 120)
	key := flowKey{proto: 6, localPort: localPort, remotePort: remotePort, localA: localA, remoteA: remoteA}

	s := &winShaper{
		scope:     ScopeApp,
		targetPID: 55,
		flows:     map[flowKey]uint32{key: 55},
		inBucket:  fullBucket(65536),
		outBucket: fullBucket(65536),
	}
	s.throttle(pkt, false, false)
	if s.inBucket.tokens != 65536-120 {
		t.Fatalf("inbound packet did not debit the in bucket: %v", s.inBucket.tokens)
	}
	if s.outBucket.tokens != 65536 {
		t.Fatalf("inbound packet debited the out bucket: %v", s.outBucket.tokens)
	}
}
