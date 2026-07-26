//go:build windows

package netlimit

import (
	"encoding/binary"
	"testing"
	"time"
)

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
