//go:build windows

package netlimit

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wdLayerNetwork = 0
	wdLayerSocket  = 2

	wdFlagSniff    = 0x0001
	wdFlagRecvOnly = 0x0008

	wdEventSocketConnect = 1
	wdEventSocketAccept  = 3
	wdEventSocketClose   = 5
)

type wdAddress struct {
	Timestamp int64
	Flags     uint32
	Reserved2 uint32
	Union     [64]byte
}

func (a *wdAddress) layer() uint8   { return uint8(a.Flags & 0xFF) }
func (a *wdAddress) event() uint8   { return uint8((a.Flags >> 8) & 0xFF) }
func (a *wdAddress) outbound() bool { return (a.Flags>>17)&1 == 1 }
func (a *wdAddress) ipv6() bool     { return (a.Flags>>20)&1 == 1 }

type wdSocketData struct {
	EndpointID       uint64
	ParentEndpointID uint64
	ProcessID        uint32
	LocalAddr        [4]uint32
	RemoteAddr       [4]uint32
	LocalPort        uint16
	RemotePort       uint16
	Protocol         uint8
}

func (a *wdAddress) socket() *wdSocketData {
	return (*wdSocketData)(unsafe.Pointer(&a.Union[0]))
}

var (
	wdOnce   sync.Once
	wdDLL    *windows.LazyDLL
	wdErr    error
	wdOpen   *windows.LazyProc
	wdRecv   *windows.LazyProc
	wdSend   *windows.LazyProc
	wdClose  *windows.LazyProc
	wdSetMin *windows.LazyProc
)

func winDivertLoad() error {
	wdOnce.Do(func() {
		path := findWinDivert()
		if path == "" {
			wdErr = errNoDriver
			return
		}
		wdDLL = windows.NewLazyDLL(path)
		if err := wdDLL.Load(); err != nil {
			wdErr = errNoDriver
			return
		}
		wdOpen = wdDLL.NewProc("WinDivertOpen")
		wdRecv = wdDLL.NewProc("WinDivertRecv")
		wdSend = wdDLL.NewProc("WinDivertSend")
		wdClose = wdDLL.NewProc("WinDivertClose")
		wdSetMin = wdDLL.NewProc("WinDivertSetParam")
	})
	return wdErr
}

func findWinDivert() string {
	candidates := []string{}
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), "WinDivert.dll"))
	}
	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "WinDivert.dll"))
	}
	candidates = append(candidates, "WinDivert.dll")
	for _, c := range candidates {
		if c == "WinDivert.dll" {
			if _, err := windows.LoadLibrary(c); err == nil {
				return c
			}
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func winDivertAvailable() bool {
	return winDivertLoad() == nil
}

type wdHandle struct {
	h windows.Handle
}

func winDivertOpen(filter string, layer int, priority int16, flags uint64) (*wdHandle, error) {
	if err := winDivertLoad(); err != nil {
		return nil, err
	}
	fptr, err := windows.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	r, _, e := wdOpen.Call(
		uintptr(unsafe.Pointer(fptr)),
		uintptr(layer),
		uintptr(priority),
		uintptr(flags),
	)
	h := windows.Handle(r)
	if h == windows.InvalidHandle {
		if e != nil && e != syscall.Errno(0) {
			return nil, e
		}
		return nil, errNoDriver
	}
	return &wdHandle{h: h}, nil
}

func (w *wdHandle) recv(buf []byte, addr *wdAddress) (int, error) {
	var recvLen uint32
	var pbuf unsafe.Pointer
	var blen uintptr
	if len(buf) > 0 {
		pbuf = unsafe.Pointer(&buf[0])
		blen = uintptr(len(buf))
	}
	r, _, e := wdRecv.Call(
		uintptr(w.h),
		uintptr(pbuf),
		blen,
		uintptr(unsafe.Pointer(&recvLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		if e != nil && e != syscall.Errno(0) {
			return 0, e
		}
		return 0, errNoDriver
	}
	return int(recvLen), nil
}

func (w *wdHandle) send(buf []byte, addr *wdAddress) (int, error) {
	var sendLen uint32
	var pbuf unsafe.Pointer
	if len(buf) > 0 {
		pbuf = unsafe.Pointer(&buf[0])
	}
	r, _, e := wdSend.Call(
		uintptr(w.h),
		uintptr(pbuf),
		uintptr(len(buf)),
		uintptr(unsafe.Pointer(&sendLen)),
		uintptr(unsafe.Pointer(addr)),
	)
	if r == 0 {
		if e != nil && e != syscall.Errno(0) {
			return 0, e
		}
		return 0, errNoDriver
	}
	return int(sendLen), nil
}

func (w *wdHandle) close() error {
	if w == nil || w.h == 0 {
		return nil
	}
	wdClose.Call(uintptr(w.h))
	w.h = 0
	return nil
}

type winDivertSniffer struct {
	sockH *wdHandle
	netH  *wdHandle

	mu    sync.RWMutex
	flows map[flowKey]uint32
	pids  map[uint32]*pidCounter

	stop chan struct{}
	wg   sync.WaitGroup
}

type flowKey struct {
	proto           uint8
	localPort       uint16
	remotePort      uint16
	localA, remoteA uint32
}

func newWinDivertSniffer() (*winDivertSniffer, error) {
	if err := winDivertLoad(); err != nil {
		return nil, err
	}
	sockH, err := winDivertOpen("true", wdLayerSocket, 0, wdFlagSniff|wdFlagRecvOnly)
	if err != nil {
		return nil, err
	}
	netH, err := winDivertOpen("ip or ipv6", wdLayerNetwork, -1000, wdFlagSniff)
	if err != nil {
		sockH.close()
		return nil, err
	}
	s := &winDivertSniffer{
		sockH: sockH,
		netH:  netH,
		flows: make(map[flowKey]uint32),
		pids:  make(map[uint32]*pidCounter),
		stop:  make(chan struct{}),
	}
	s.wg.Add(2)
	go s.runSockets()
	go s.runPackets()
	return s, nil
}

func (s *winDivertSniffer) runSockets() {
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
		key := flowKey{
			proto:      sd.Protocol,
			localPort:  sd.LocalPort,
			remotePort: sd.RemotePort,
			localA:     sd.LocalAddr[0],
			remoteA:    sd.RemoteAddr[0],
		}
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

func (s *winDivertSniffer) runPackets() {
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
		pid, length, ok := s.classify(buf[:n], addr.outbound(), addr.ipv6())
		if !ok {
			continue
		}
		c := s.counterFor(pid)
		if addr.outbound() {
			c.tx.Add(uint64(length))
		} else {
			c.rx.Add(uint64(length))
		}
	}
}

func (s *winDivertSniffer) classify(pkt []byte, outbound, isv6 bool) (pid uint32, length int, ok bool) {
	la, ra, lp, rp, proto, l, parsed := parseFlow(pkt, outbound, isv6)
	if !parsed {
		return 0, 0, false
	}
	key := flowKey{proto: proto, localPort: lp, remotePort: rp, localA: la, remoteA: ra}
	s.mu.RLock()
	p, found := s.flows[key]
	s.mu.RUnlock()
	if !found {
		return 0, l, false
	}
	return p, l, true
}

func (s *winDivertSniffer) counterFor(pid uint32) *pidCounter {
	s.mu.RLock()
	c := s.pids[pid]
	s.mu.RUnlock()
	if c != nil {
		return c
	}
	s.mu.Lock()
	c = s.pids[pid]
	if c == nil {
		c = &pidCounter{}
		s.pids[pid] = c
	}
	s.mu.Unlock()
	return c
}

func (s *winDivertSniffer) counters(pid int32) (rx, tx uint64, err error) {
	c := s.counterFor(uint32(pid))
	return c.rx.Load(), c.tx.Load(), nil
}

func (s *winDivertSniffer) close() error {
	close(s.stop)
	s.netH.close()
	s.sockH.close()
	s.wg.Wait()
	return nil
}

func parseFlow(pkt []byte, outbound, isv6 bool) (localA, remoteA uint32, localPort, remotePort uint16, proto uint8, length int, ok bool) {
	if len(pkt) < 20 {
		return
	}
	length = len(pkt)
	var srcA, dstA uint32
	var hdrLen int
	if isv6 {
		if len(pkt) < 40 {
			return
		}
		proto = pkt[6]
		srcA = binary.BigEndian.Uint32(pkt[8:12])
		dstA = binary.BigEndian.Uint32(pkt[36:40])
		hdrLen = 40
	} else {
		proto = pkt[9]
		srcA = binary.BigEndian.Uint32(pkt[12:16])
		dstA = binary.BigEndian.Uint32(pkt[16:20])
		hdrLen = int(pkt[0]&0x0F) * 4
		if hdrLen < 20 {
			hdrLen = 20
		}
	}
	var srcPort, dstPort uint16
	if proto == 6 || proto == 17 {
		if len(pkt) >= hdrLen+4 {
			srcPort = binary.BigEndian.Uint16(pkt[hdrLen : hdrLen+2])
			dstPort = binary.BigEndian.Uint16(pkt[hdrLen+2 : hdrLen+4])
		}
	}
	if outbound {
		localA, remoteA = srcA, dstA
		localPort, remotePort = srcPort, dstPort
	} else {
		localA, remoteA = dstA, srcA
		localPort, remotePort = dstPort, srcPort
	}
	ok = true
	return
}
