//go:build windows

package netlimit

import (
	"encoding/binary"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"
)

func TestQuoteArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "server", "server"},
		{"empty string", "", `""`},
		{"flag", "--elevated", "--elevated"},
		{"path without spaces", `C:\tools\app.exe`, `C:\tools\app.exe`},
		{"trailing backslash without spaces", `C:\tools\`, `C:\tools\`},
		{"space", "hello world", `"hello world"`},
		{"tab", "a\tb", "\"a\tb\""},
		{"newline", "a\nb", "\"a\nb\""},
		{"vertical tab", "a\vb", "\"a\vb\""},
		{"embedded quote", `a"b`, `"a\"b"`},
		{"backslash before quote is doubled", `a\"b`, `"a\\\"b"`},
		{"two backslashes before quote", `a\\"b`, `"a\\\\\"b"`},
		{"path with spaces and trailing backslash", `C:\my dir\`, `"C:\my dir\\"`},
		{"path with spaces", `C:\Program Files\app.exe`, `"C:\Program Files\app.exe"`},
		{"interior backslash with space", `a\ b`, `"a\ b"`},
		{"only a quote", `"`, `"\""`},
		{"only backslashes with space", `\\ `, `"\\ "`},
		{"unicode passes through", "прокси", "прокси"},
		{"unicode with space", "про кси", `"про кси"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteArg(tt.in); got != tt.want {
				t.Fatalf("quoteArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinCmdLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"nil", nil, ""},
		{"empty slice", []string{}, ""},
		{"single plain", []string{"a"}, "a"},
		{"single empty arg", []string{""}, `""`},
		{"two plain", []string{"a", "b"}, "a b"},
		{"quoted middle", []string{"a", "b c", "d"}, `a "b c" d`},
		{"path argument", []string{"--file", `C:\Program Files\x.txt`}, `--file "C:\Program Files\x.txt"`},
		{"empty among plain", []string{"a", "", "b"}, `a "" b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinCmdLine(tt.args); got != tt.want {
				t.Fatalf("joinCmdLine(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{"nil slice", nil, "-x", false},
		{"present first", []string{"-x", "-y"}, "-x", true},
		{"present last", []string{"-y", "-x"}, "-x", true},
		{"absent", []string{"-y", "-z"}, "-x", false},
		{"case sensitive", []string{"-X"}, "-x", false},
		{"prefix is not a match", []string{"-xx"}, "-x", false},
		{"empty needle absent", []string{"-x"}, "", false},
		{"empty needle present", []string{"-x", ""}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasArg(tt.args, tt.want); got != tt.ok {
				t.Fatalf("hasArg(%q, %q) = %v, want %v", tt.args, tt.want, got, tt.ok)
			}
		})
	}
}

func TestIsElevatedIsCached(t *testing.T) {
	first := IsElevated()
	if second := IsElevated(); second != first {
		t.Fatalf("IsElevated returned %v then %v", first, second)
	}
}

func TestErrUACDenied(t *testing.T) {
	if ErrUACDenied == nil || ErrUACDenied.Error() == "" {
		t.Fatal("ErrUACDenied must carry a message")
	}
}

func TestMibIfRow2Layout(t *testing.T) {
	var r mibIfRow2
	if got := unsafe.Offsetof(r.InOctets); got != 1208 {
		t.Fatalf("InOctets offset = %d, want 1208", got)
	}
	if got := unsafe.Offsetof(r.OutOctets); got != 1280 {
		t.Fatalf("OutOctets offset = %d, want 1280", got)
	}
	if got := unsafe.Sizeof(r); got != 1352 {
		t.Fatalf("sizeof(mibIfRow2) = %d, want 1352", got)
	}
}

func TestSystemCountersWindows(t *testing.T) {
	m := newMonitor()
	defer m.Close()
	rx, tx, err := m.SystemCounters()
	if err != nil {
		t.Fatalf("SystemCounters: %v", err)
	}
	if rx == 0 && tx == 0 {
		t.Logf("counters both zero (rx=%d tx=%d) — unusual but not fatal", rx, tx)
	}
}

func ipv6TCP(t *testing.T, src, dst [16]byte, sport, dport uint16) []byte {
	t.Helper()
	pkt := make([]byte, 60)
	pkt[0] = 0x60
	binary.BigEndian.PutUint16(pkt[4:6], 20)
	pkt[6] = 6
	pkt[7] = 64
	copy(pkt[8:24], src[:])
	copy(pkt[24:40], dst[:])
	binary.BigEndian.PutUint16(pkt[40:42], sport)
	binary.BigEndian.PutUint16(pkt[42:44], dport)
	return pkt
}

func addr6(t *testing.T, words ...uint32) [16]byte {
	t.Helper()
	var a [16]byte
	for i, w := range words {
		binary.BigEndian.PutUint32(a[i*4:i*4+4], w)
	}
	return a
}

func TestParseFlowIPv6UsesLowWordOfBothAddresses(t *testing.T) {
	src := addr6(t, 0x20010db8, 0x0, 0x0, 0x1)
	dst := addr6(t, 0x26064700, 0x0, 0x0, 0x6810_85e5)

	localA, remoteA, localPort, remotePort, proto, length, ok := parseFlow(ipv6TCP(t, src, dst, 4000, 443), true, true)
	if !ok {
		t.Fatal("parseFlow failed on a valid IPv6 packet")
	}
	if localA != 0x1 {
		t.Errorf("localA = %#x, want the LOW word of the source address (%#x): "+
			"WinDivert's socket-layer Addr[0] is the least significant 32 bits", localA, 0x1)
	}
	if remoteA != 0x6810_85e5 {
		t.Errorf("remoteA = %#x, want the LOW word of the destination address (%#x), not the first",
			remoteA, 0x6810_85e5)
	}
	if localPort != 4000 || remotePort != 443 {
		t.Errorf("ports = %d/%d, want 4000/443", localPort, remotePort)
	}
	if proto != 6 {
		t.Errorf("proto = %d, want 6", proto)
	}
	if length != 60 {
		t.Errorf("length = %d, want 60", length)
	}
}

func TestParseFlowIPv6DirectionIsSymmetric(t *testing.T) {
	src := addr6(t, 0x20010db8, 0, 0, 1)
	dst := addr6(t, 0x26064700, 0, 0, 2)
	pkt := ipv6TCP(t, src, dst, 4000, 443)

	outL, outR, outLP, outRP, _, _, ok1 := parseFlow(pkt, true, true)
	inL, inR, inLP, inRP, _, _, ok2 := parseFlow(pkt, false, true)
	if !ok1 || !ok2 {
		t.Fatal("parseFlow failed")
	}
	if outL != inR || outR != inL {
		t.Errorf("addresses not symmetric: out(%#x,%#x) in(%#x,%#x)", outL, outR, inL, inR)
	}
	if outLP != inRP || outRP != inLP {
		t.Errorf("ports not symmetric: out(%d,%d) in(%d,%d)", outLP, outRP, inLP, inRP)
	}
}

// windivertAddr6 mirrors how WinDivert fills WINDIVERT_DATA_SOCKET.LocalAddr /
// RemoteAddr: IPv6 addresses are stored in reversed word order, so Addr[0] is
// the least significant 32 bits. This is also why Addr[0] carries the plain
// IPv4 address for v4 flows, which arrive as IPv4-mapped IPv6 addresses.
func windivertAddr6(a [16]byte) [4]uint32 {
	var out [4]uint32
	for i := range out {
		out[i] = binary.BigEndian.Uint32(a[12-i*4 : 16-i*4])
	}
	return out
}

func TestWinDivertAddr6IsReversedWordOrder(t *testing.T) {
	v4mapped := addr6(t, 0, 0, 0x0000FFFF, 0x0A000001)
	got := windivertAddr6(v4mapped)
	if got[0] != 0x0A000001 {
		t.Fatalf("Addr[0] = %#x, want the IPv4 address %#x for a v4-mapped address", got[0], 0x0A000001)
	}
	if got[1] != 0x0000FFFF {
		t.Fatalf("Addr[1] = %#x, want %#x", got[1], 0x0000FFFF)
	}
}

func TestParseFlowIPv6MatchesSocketLayerKeyWordIndex(t *testing.T) {
	src := addr6(t, 0xAABBCCDD, 0x11111111, 0x22222222, 0x33333333)
	dst := addr6(t, 0x99887766, 0x44444444, 0x55555555, 0x66666666)

	localA, remoteA, _, _, _, _, ok := parseFlow(ipv6TCP(t, src, dst, 1, 2), true, true)
	if !ok {
		t.Fatal("parseFlow failed")
	}
	var sd wdSocketData
	sd.LocalAddr = windivertAddr6(src)
	sd.RemoteAddr = windivertAddr6(dst)

	if localA != sd.LocalAddr[0] || remoteA != sd.RemoteAddr[0] {
		t.Errorf("network key (%#x,%#x) must match socket key (%#x,%#x) or per-app IPv6 shaping never fires",
			localA, remoteA, sd.LocalAddr[0], sd.RemoteAddr[0])
	}
}

func TestThrottleAppScopeIgnoresUnknownFlowWithZeroTarget(t *testing.T) {
	s := &winShaper{
		active:      true,
		scope:       ScopeApp,
		targetPID:   0,
		flows:       map[flowKey]uint32{},
		totalBucket: newTokenBucket(1),
	}
	before := s.totalBucket.tokens
	pkt := ipv4TCP(t, 0x0A000001, 0x5DB8D822, 4000, 443, 60)
	s.throttle(pkt, true, false)
	if s.totalBucket.tokens != before {
		t.Error("an unknown flow must not be throttled even when targetPID is 0")
	}
}

func TestThrottleAppScopeStillMatchesKnownFlow(t *testing.T) {
	pkt := ipv4TCP(t, 0x0A000001, 0x5DB8D822, 4000, 443, 60)
	localA, remoteA, lp, rp, proto, _, ok := parseFlow(pkt, true, false)
	if !ok {
		t.Fatal("parseFlow failed")
	}
	key := flowKey{proto: proto, localPort: lp, remotePort: rp, localA: localA, remoteA: remoteA}
	s := &winShaper{
		active:      true,
		scope:       ScopeApp,
		targetPID:   0,
		flows:       map[flowKey]uint32{key: 0},
		totalBucket: newTokenBucket(1e9),
	}
	before := s.totalBucket.tokens
	s.throttle(pkt, true, false)
	if s.totalBucket.tokens >= before {
		t.Error("a known flow belonging to the target PID must still be throttled")
	}
}

func TestTokenBucketWaitLargerThanBurstTerminates(t *testing.T) {
	b := newTokenBucket(1000)
	done := make(chan struct{})
	go func() {
		defer close(done)
		b.wait(int(b.burst) + 1)
	}()
	select {
	case <-done:
	case <-timeoutCh():
		t.Fatal("tokenBucket.wait hung on a request larger than the burst")
	}
}

func TestTokenBucketWaitNormalRequest(t *testing.T) {
	b := newTokenBucket(1e9)
	before := b.tokens
	b.wait(1500)
	if b.tokens >= before {
		t.Error("wait did not debit the bucket")
	}
}

func timeoutCh() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Second)
		close(ch)
	}()
	return ch
}

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

func waitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(15 * time.Second):
		t.Fatalf("%s did not return after its stop channel was closed", what)
	}
}

func closedStop() chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func TestShaperLoopsExitOnAlreadyClosedStop(t *testing.T) {
	tests := []struct {
		name string
		run  func(s *winShaper)
	}{
		{"trackSockets", (*winShaper).trackSockets},
		{"shapeLoop", (*winShaper).shapeLoop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winShaper{stop: closedStop()}
			done := make(chan struct{})
			s.wg.Add(1)
			go func() {
				defer close(done)
				tt.run(s)
			}()
			waitClosed(t, done, tt.name)
			s.wg.Wait()
		})
	}
}

func TestSnifferLoopsExitOnAlreadyClosedStop(t *testing.T) {
	tests := []struct {
		name string
		run  func(s *winDivertSniffer)
	}{
		{"runSockets", (*winDivertSniffer).runSockets},
		{"runPackets", (*winDivertSniffer).runPackets},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &winDivertSniffer{
				flows: make(map[flowKey]uint32),
				pids:  make(map[uint32]*pidCounter),
				stop:  closedStop(),
			}
			done := make(chan struct{})
			s.wg.Add(1)
			go func() {
				defer close(done)
				tt.run(s)
			}()
			waitClosed(t, done, tt.name)
			s.wg.Wait()
		})
	}
}

func TestWinShaperRemoveClosesOpenHandles(t *testing.T) {
	s := &winShaper{
		active: true,
		stop:   make(chan struct{}),
		netH:   &wdHandle{},
		sockH:  &wdHandle{},
		flows:  map[flowKey]uint32{},
	}
	if err := s.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if s.netH != nil || s.sockH != nil {
		t.Fatalf("Remove left handles behind: netH=%v sockH=%v", s.netH, s.sockH)
	}
	if s.active {
		t.Fatal("shaper still active after Remove")
	}
}

func TestWinShaperApplyRejectsWithoutDriverAndStaysClean(t *testing.T) {
	if winDivertAvailable() {
		t.Log("WinDivert present; not exercising Apply against the live driver")
		return
	}
	specs := []LimitSpec{
		{Scope: ScopeSystem, TotalBps: 1 << 20},
		{Scope: ScopeApp, AppPID: 1234, InBps: 1000, OutBps: 2000},
		{},
	}
	for _, spec := range specs {
		s := &winShaper{}
		if err := s.Apply(spec); err != errNoDriver {
			t.Fatalf("Apply(%+v) = %v, want errNoDriver", spec, err)
		}
		if s.active || s.netH != nil || s.sockH != nil || s.stop != nil || s.flows != nil {
			t.Fatalf("failed Apply mutated the shaper: %+v", s)
		}
		if err := s.Remove(); err != nil {
			t.Fatalf("Remove after failed Apply = %v", err)
		}
	}
}

func TestWDHandleCloseForwardsHandleOnce(t *testing.T) {
	saved := wdClose
	wdClose = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetEvent")
	t.Cleanup(func() { wdClose = saved })

	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	defer windows.CloseHandle(ev) //nolint:errcheck

	signaled := func() bool {
		st, err := windows.WaitForSingleObject(ev, 0)
		if err != nil {
			t.Fatalf("WaitForSingleObject: %v", err)
		}
		return st == windows.WAIT_OBJECT_0
	}

	h := &wdHandle{h: ev}
	if err := h.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !signaled() {
		t.Fatal("close did not pass the handle to WinDivertClose")
	}
	if h.h != 0 {
		t.Fatalf("close left h = %v, want 0", h.h)
	}

	if err := windows.ResetEvent(ev); err != nil {
		t.Fatalf("ResetEvent: %v", err)
	}
	if err := h.close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
	if signaled() {
		t.Fatal("close is not idempotent: the handle was released twice")
	}
}

func TestFindWinDivertUsesWorkingDirectory(t *testing.T) {
	_ = winDivertLoad()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "WinDivert.dll"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(wd, "WinDivert.dll")
	if got := findWinDivert(); got != want {
		t.Fatalf("findWinDivert() = %q, want %q", got, want)
	}
}

func TestFindWinDivertEmptyWhenAbsent(t *testing.T) {
	_ = winDivertLoad()
	t.Chdir(t.TempDir())
	t.Setenv("PATH", t.TempDir())
	if got := findWinDivert(); got != "" {
		t.Fatalf("findWinDivert() = %q, want %q", got, "")
	}
}

func TestFindWinDivertFallsBackToLoaderSearchPath(t *testing.T) {
	_ = winDivertLoad()

	src := filepath.Join(os.Getenv("SystemRoot"), "System32", "version.dll")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	dllDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dllDir, "WinDivert.dll"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Chdir(t.TempDir())
	t.Setenv("PATH", dllDir)

	t.Cleanup(func() {
		const unchangedRefcount = 0x2
		name, err := windows.UTF16PtrFromString("WinDivert.dll")
		if err != nil {
			return
		}
		var mod windows.Handle
		if err := windows.GetModuleHandleEx(unchangedRefcount, name, &mod); err == nil && mod != 0 {
			_ = windows.FreeLibrary(mod)
		}
	})

	if got := findWinDivert(); got != "WinDivert.dll" {
		t.Fatalf("findWinDivert() = %q, want the bare loader-resolved name", got)
	}
}

func TestTokenBucketWaitClampsSleepToOneMillisecond(t *testing.T) {
	b := &tokenBucket{rate: 1e12, burst: 1e6, tokens: 0, last: time.Now()}
	b.wait(1)
	if b.tokens != b.burst-1 {
		t.Fatalf("tokens = %v, want %v (refill clamped to burst, then debited)", b.tokens, b.burst-1)
	}
}

func ipv4Packet(t *testing.T, ihlWords int, proto uint8, src, dst uint32, sport, dport uint16, total int) []byte {
	t.Helper()
	hdr := ihlWords * 4
	if total < hdr {
		total = hdr
	}
	pkt := make([]byte, total)
	pkt[0] = 0x40 | byte(ihlWords&0x0F)
	pkt[9] = proto
	if len(pkt) >= 20 {
		binary.BigEndian.PutUint32(pkt[12:16], src)
		binary.BigEndian.PutUint32(pkt[16:20], dst)
	}
	if hdr >= 20 && len(pkt) >= hdr+4 {
		binary.BigEndian.PutUint16(pkt[hdr:hdr+2], sport)
		binary.BigEndian.PutUint16(pkt[hdr+2:hdr+4], dport)
	}
	return pkt
}

func ipv4TCP(t *testing.T, src, dst uint32, sport, dport uint16, total int) []byte {
	t.Helper()
	return ipv4Packet(t, 5, 6, src, dst, sport, dport, total)
}

func ipv6Packet(t *testing.T, proto uint8, srcHi, dstHi uint32, sport, dport uint16, total int) []byte {
	t.Helper()
	if total < 40 {
		total = 40
	}
	pkt := make([]byte, total)
	pkt[0] = 0x60
	pkt[6] = proto
	binary.BigEndian.PutUint32(pkt[8:12], srcHi)
	binary.BigEndian.PutUint32(pkt[24:28], dstHi)
	if len(pkt) >= 44 {
		binary.BigEndian.PutUint16(pkt[40:42], sport)
		binary.BigEndian.PutUint16(pkt[42:44], dport)
	}
	return pkt
}

func TestParseFlowIPv4(t *testing.T) {
	const src, dst uint32 = 0xC0A8010A, 0x5DB8D822
	const sport, dport uint16 = 5000, 443

	tests := []struct {
		name       string
		pkt        []byte
		outbound   bool
		wantOK     bool
		wantLocalA uint32
		wantRemA   uint32
		wantLPort  uint16
		wantRPort  uint16
		wantProto  uint8
		wantLen    int
	}{
		{
			name:       "outbound tcp",
			pkt:        ipv4Packet(t, 5, 6, src, dst, sport, dport, 64),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantLPort: sport, wantRPort: dport,
			wantProto: 6, wantLen: 64,
		},
		{
			name:       "inbound tcp swaps local and remote",
			pkt:        ipv4Packet(t, 5, 6, src, dst, sport, dport, 64),
			outbound:   false,
			wantOK:     true,
			wantLocalA: dst, wantRemA: src,
			wantLPort: dport, wantRPort: sport,
			wantProto: 6, wantLen: 64,
		},
		{
			name:       "outbound udp",
			pkt:        ipv4Packet(t, 5, 17, src, dst, 53, 5353, 48),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantLPort: 53, wantRPort: 5353,
			wantProto: 17, wantLen: 48,
		},
		{
			name:       "options in header shift the ports",
			pkt:        ipv4Packet(t, 8, 6, src, dst, sport, dport, 80),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantLPort: sport, wantRPort: dport,
			wantProto: 6, wantLen: 80,
		},
		{
			name:       "ihl below minimum is clamped to 20",
			pkt:        ipv4Packet(t, 0, 6, src, dst, sport, dport, 64),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantLPort: 0, wantRPort: 0,
			wantProto: 6, wantLen: 64,
		},
		{
			name:       "icmp has no ports",
			pkt:        ipv4Packet(t, 5, 1, src, dst, 0, 0, 40),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantProto: 1, wantLen: 40,
		},
		{
			name:       "tcp truncated before ports",
			pkt:        ipv4Packet(t, 5, 6, src, dst, sport, dport, 22),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantLPort: 0, wantRPort: 0,
			wantProto: 6, wantLen: 22,
		},
		{
			name:     "too short to be an ip header",
			pkt:      make([]byte, 19),
			outbound: true,
			wantOK:   false,
		},
		{
			name:     "empty packet",
			pkt:      nil,
			outbound: true,
			wantOK:   false,
		},
		{
			name:       "exactly the minimum header",
			pkt:        ipv4Packet(t, 5, 6, src, dst, sport, dport, 20),
			outbound:   true,
			wantOK:     true,
			wantLocalA: src, wantRemA: dst,
			wantProto: 6, wantLen: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			la, ra, lp, rp, proto, l, ok := parseFlow(tt.pkt, tt.outbound, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if la != tt.wantLocalA || ra != tt.wantRemA {
				t.Fatalf("addrs = (%#x, %#x), want (%#x, %#x)", la, ra, tt.wantLocalA, tt.wantRemA)
			}
			if lp != tt.wantLPort || rp != tt.wantRPort {
				t.Fatalf("ports = (%d, %d), want (%d, %d)", lp, rp, tt.wantLPort, tt.wantRPort)
			}
			if proto != tt.wantProto {
				t.Fatalf("proto = %d, want %d", proto, tt.wantProto)
			}
			if l != tt.wantLen {
				t.Fatalf("length = %d, want %d", l, tt.wantLen)
			}
		})
	}
}

func TestParseFlowIPv4DirectionIsSymmetric(t *testing.T) {
	const a, b uint32 = 0x0A000001, 0x08080808
	out := ipv4Packet(t, 5, 6, a, b, 1111, 2222, 64)
	in := ipv4Packet(t, 5, 6, b, a, 2222, 1111, 64)

	la1, ra1, lp1, rp1, pr1, _, ok1 := parseFlow(out, true, false)
	la2, ra2, lp2, rp2, pr2, _, ok2 := parseFlow(in, false, false)
	if !ok1 || !ok2 {
		t.Fatalf("parse failed: ok1=%v ok2=%v", ok1, ok2)
	}
	k1 := flowKey{proto: pr1, localPort: lp1, remotePort: rp1, localA: la1, remoteA: ra1}
	k2 := flowKey{proto: pr2, localPort: lp2, remotePort: rp2, localA: la2, remoteA: ra2}
	if k1 != k2 {
		t.Fatalf("both directions of one flow must map to the same key: %+v vs %+v", k1, k2)
	}
}

func TestParseFlowIPv6(t *testing.T) {
	const srcHi, dstHi uint32 = 0x20010DB8, 0xFE800000
	const sport, dport uint16 = 6000, 8443

	tests := []struct {
		name      string
		pkt       []byte
		outbound  bool
		wantOK    bool
		wantProto uint8
		wantLPort uint16
		wantRPort uint16
		wantLen   int
	}{
		{
			name:      "outbound tcp",
			pkt:       ipv6Packet(t, 6, srcHi, dstHi, sport, dport, 60),
			outbound:  true,
			wantOK:    true,
			wantProto: 6,
			wantLPort: sport, wantRPort: dport,
			wantLen: 60,
		},
		{
			name:      "inbound udp swaps ports",
			pkt:       ipv6Packet(t, 17, srcHi, dstHi, sport, dport, 60),
			outbound:  false,
			wantOK:    true,
			wantProto: 17,
			wantLPort: dport, wantRPort: sport,
			wantLen: 60,
		},
		{
			name:      "header only, no ports",
			pkt:       ipv6Packet(t, 6, srcHi, dstHi, sport, dport, 40),
			outbound:  true,
			wantOK:    true,
			wantProto: 6,
			wantLen:   40,
		},
		{
			name:     "truncated ipv6 header",
			pkt:      make([]byte, 39),
			outbound: true,
			wantOK:   false,
		},
		{
			name:     "shorter than any ip header",
			pkt:      make([]byte, 12),
			outbound: true,
			wantOK:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, lp, rp, proto, l, ok := parseFlow(tt.pkt, tt.outbound, true)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if proto != tt.wantProto {
				t.Fatalf("proto = %d, want %d", proto, tt.wantProto)
			}
			if lp != tt.wantLPort || rp != tt.wantRPort {
				t.Fatalf("ports = (%d, %d), want (%d, %d)", lp, rp, tt.wantLPort, tt.wantRPort)
			}
			if l != tt.wantLen {
				t.Fatalf("length = %d, want %d", l, tt.wantLen)
			}
		})
	}
}

func TestParseFlowIPv6UsesSourceLowWord(t *testing.T) {
	const srcLo uint32 = 0xDEADBEEF
	src := addr6(t, 0x20010DB8, 0, 0, srcLo)
	dst := addr6(t, 0xFE800000, 0, 0, 0x2)
	la, _, _, _, _, _, ok := parseFlow(ipv6TCP(t, src, dst, 1, 2), true, true)
	if !ok {
		t.Fatal("parse failed")
	}
	if la != srcLo {
		t.Fatalf("outbound local addr = %#x, want source low word %#x", la, srcLo)
	}
}

func TestWDAddressFlagAccessors(t *testing.T) {
	tests := []struct {
		name         string
		flags        uint32
		wantLayer    uint8
		wantEvent    uint8
		wantOutbound bool
		wantIPv6     bool
	}{
		{"zero", 0, 0, 0, false, false},
		{"network layer inbound v4", 0x0000, wdLayerNetwork, 0, false, false},
		{"socket layer connect", uint32(wdLayerSocket) | uint32(wdEventSocketConnect)<<8, wdLayerSocket, wdEventSocketConnect, false, false},
		{"socket layer close", uint32(wdLayerSocket) | uint32(wdEventSocketClose)<<8, wdLayerSocket, wdEventSocketClose, false, false},
		{"socket layer accept", uint32(wdLayerSocket) | uint32(wdEventSocketAccept)<<8, wdLayerSocket, wdEventSocketAccept, false, false},
		{"outbound bit", 1 << 17, 0, 0, true, false},
		{"ipv6 bit", 1 << 20, 0, 0, false, true},
		{"outbound ipv6", 1<<17 | 1<<20, 0, 0, true, true},
		{"neighbouring bits ignored", 1<<16 | 1<<18 | 1<<19 | 1<<21, 0, 0, false, false},
		{"all fields", 0xFF | 0xEE<<8 | 1<<17 | 1<<20, 0xFF, 0xEE, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := wdAddress{Flags: tt.flags}
			if got := a.layer(); got != tt.wantLayer {
				t.Fatalf("layer = %d, want %d", got, tt.wantLayer)
			}
			if got := a.event(); got != tt.wantEvent {
				t.Fatalf("event = %d, want %d", got, tt.wantEvent)
			}
			if got := a.outbound(); got != tt.wantOutbound {
				t.Fatalf("outbound = %v, want %v", got, tt.wantOutbound)
			}
			if got := a.ipv6(); got != tt.wantIPv6 {
				t.Fatalf("ipv6 = %v, want %v", got, tt.wantIPv6)
			}
		})
	}
}

func TestWDSocketDataFitsUnion(t *testing.T) {
	var a wdAddress
	if got := unsafe.Sizeof(wdSocketData{}); got > uintptr(len(a.Union)) {
		t.Fatalf("sizeof(wdSocketData) = %d, exceeds the %d-byte union", got, len(a.Union))
	}
	if got := unsafe.Offsetof(a.Union); got != 16 {
		t.Fatalf("Union offset = %d, want 16", got)
	}
}

func TestWDAddressSocketOverlay(t *testing.T) {
	var a wdAddress
	sd := a.socket()
	sd.ProcessID = 4321
	sd.Protocol = 6
	sd.LocalPort = 1234
	sd.RemotePort = 443
	sd.LocalAddr[0] = 0x0A000001
	sd.RemoteAddr[0] = 0x5DB8D822

	again := a.socket()
	if again.ProcessID != 4321 || again.Protocol != 6 {
		t.Fatalf("socket overlay lost data: %+v", *again)
	}
	if again.LocalPort != 1234 || again.RemotePort != 443 {
		t.Fatalf("ports lost: %+v", *again)
	}
	if again.LocalAddr[0] != 0x0A000001 || again.RemoteAddr[0] != 0x5DB8D822 {
		t.Fatalf("addrs lost: %+v", *again)
	}
	if a.Union == [64]byte{} {
		t.Fatal("socket() did not write into the union storage")
	}
}

func newTestSniffer() *winDivertSniffer {
	return &winDivertSniffer{
		flows: make(map[flowKey]uint32),
		pids:  make(map[uint32]*pidCounter),
		stop:  make(chan struct{}),
	}
}

func TestSnifferCounterFor(t *testing.T) {
	s := newTestSniffer()
	c1 := s.counterFor(10)
	if c1 == nil {
		t.Fatal("counterFor returned nil")
	}
	if c2 := s.counterFor(10); c1 != c2 {
		t.Fatal("counterFor must return the same counter for the same pid")
	}
	if c3 := s.counterFor(11); c1 == c3 {
		t.Fatal("counterFor must not share counters across pids")
	}

	c1.rx.Add(100)
	c1.tx.Add(200)
	rx, tx, err := s.counters(10)
	if err != nil {
		t.Fatalf("counters: %v", err)
	}
	if rx != 100 || tx != 200 {
		t.Fatalf("counters(10) = (%d, %d), want (100, 200)", rx, tx)
	}
	rx, tx, err = s.counters(999)
	if err != nil || rx != 0 || tx != 0 {
		t.Fatalf("counters for unknown pid = (%d, %d, %v), want zeros", rx, tx, err)
	}
}

func TestSnifferCounterForConcurrent(t *testing.T) {
	s := newTestSniffer()
	const workers = 16
	done := make(chan *pidCounter, workers)
	for i := 0; i < workers; i++ {
		go func() { done <- s.counterFor(7) }()
	}
	first := <-done
	for i := 1; i < workers; i++ {
		if got := <-done; got != first {
			t.Fatal("concurrent counterFor created duplicate counters")
		}
	}
}

func TestSnifferClassify(t *testing.T) {
	const localA, remoteA uint32 = 0x0A000001, 0x5DB8D822
	const localPort, remotePort uint16 = 4000, 443
	key := flowKey{proto: 6, localPort: localPort, remotePort: remotePort, localA: localA, remoteA: remoteA}

	tests := []struct {
		name       string
		flows      map[flowKey]uint32
		pkt        []byte
		outbound   bool
		wantPID    uint32
		wantLength int
		wantOK     bool
	}{
		{
			name:       "known outbound flow",
			flows:      map[flowKey]uint32{key: 321},
			pkt:        ipv4TCP(t, localA, remoteA, localPort, remotePort, 100),
			outbound:   true,
			wantPID:    321,
			wantLength: 100,
			wantOK:     true,
		},
		{
			name:       "known inbound flow",
			flows:      map[flowKey]uint32{key: 321},
			pkt:        ipv4TCP(t, remoteA, localA, remotePort, localPort, 80),
			outbound:   false,
			wantPID:    321,
			wantLength: 80,
			wantOK:     true,
		},
		{
			name:       "unknown flow reports length but not ok",
			flows:      map[flowKey]uint32{},
			pkt:        ipv4TCP(t, localA, remoteA, localPort, remotePort, 100),
			outbound:   true,
			wantPID:    0,
			wantLength: 100,
			wantOK:     false,
		},
		{
			name:       "unparseable packet",
			flows:      map[flowKey]uint32{key: 321},
			pkt:        make([]byte, 4),
			outbound:   true,
			wantPID:    0,
			wantLength: 0,
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestSniffer()
			s.flows = tt.flows
			pid, length, ok := s.classify(tt.pkt, tt.outbound, false)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if pid != tt.wantPID {
				t.Fatalf("pid = %d, want %d", pid, tt.wantPID)
			}
			if length != tt.wantLength {
				t.Fatalf("length = %d, want %d", length, tt.wantLength)
			}
		})
	}
}

func TestWDHandleCloseNil(t *testing.T) {
	var h *wdHandle
	if err := h.close(); err != nil {
		t.Fatalf("close on nil handle = %v", err)
	}
	zero := &wdHandle{}
	if err := zero.close(); err != nil {
		t.Fatalf("close on zero handle = %v", err)
	}
}

func TestSnifferCloseWithoutHandles(t *testing.T) {
	s := newTestSniffer()
	if err := s.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	select {
	case <-s.stop:
	default:
		t.Fatal("close must close the stop channel")
	}
}

func TestWinMonitorAppCounters(t *testing.T) {
	m := &winMonitor{sniff: newTestSniffer()}
	m.sniff.counterFor(31).rx.Add(64)
	m.sniff.counterFor(31).tx.Add(128)

	rx, tx, err := m.AppCounters(31)
	if err != nil {
		t.Fatalf("AppCounters: %v", err)
	}
	if rx != 64 || tx != 128 {
		t.Fatalf("AppCounters(31) = (%d, %d), want (64, 128)", rx, tx)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWinMonitorCloseWithoutSniffer(t *testing.T) {
	m := newMonitor()
	if err := m.Close(); err != nil {
		t.Fatalf("Close on unused monitor = %v", err)
	}
}

func TestDriverAbsentErrorPaths(t *testing.T) {
	if winDivertAvailable() {
		t.Log("WinDivert present; driver-absent error paths are not reachable here")
		return
	}
	if _, err := winDivertOpen("true", wdLayerNetwork, 0, 0); err == nil {
		t.Fatal("winDivertOpen without the driver must fail")
	}
	if _, err := newWinDivertSniffer(); err == nil {
		t.Fatal("newWinDivertSniffer without the driver must fail")
	}
	m := &winMonitor{}
	if _, _, err := m.AppCounters(1); err == nil {
		t.Fatal("AppCounters without the driver must fail")
	}
	if m.sniff != nil {
		t.Fatal("failed sniffer construction must not be cached")
	}
}

func TestWinDivertLoadIsStable(t *testing.T) {
	first := winDivertLoad()
	second := winDivertLoad()
	if (first == nil) != (second == nil) {
		t.Fatalf("winDivertLoad is not idempotent: %v then %v", first, second)
	}
	if first != nil && first != errNoDriver {
		t.Logf("winDivertLoad failed with %v", first)
	}
	if got := winDivertAvailable(); got != (first == nil) {
		t.Fatalf("winDivertAvailable = %v, want %v", got, first == nil)
	}
}
