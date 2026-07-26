//go:build windows

package netlimit

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

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
