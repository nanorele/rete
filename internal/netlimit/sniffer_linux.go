//go:build linux

package netlimit

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const ethPAll = 0x0003

type portKey struct {
	v6   bool
	port uint16
}

type linuxSniffer struct {
	fd int

	mu        sync.RWMutex
	watch     map[int32]bool
	ports     map[portKey]int32
	pidByPort map[int32]*pidCounter

	stop chan struct{}
	wg   sync.WaitGroup
}

func htons(x uint16) uint16 { return (x << 8) | (x >> 8) }

func newLinuxSniffer() (*linuxSniffer, error) {
	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_DGRAM, int(htons(ethPAll)))
	if err != nil {
		return nil, err
	}
	s := &linuxSniffer{
		fd:        fd,
		watch:     make(map[int32]bool),
		ports:     make(map[portKey]int32),
		pidByPort: make(map[int32]*pidCounter),
		stop:      make(chan struct{}),
	}
	s.wg.Add(2)
	go s.refreshLoop()
	go s.packetLoop()
	return s, nil
}

func (s *linuxSniffer) counterFor(pid int32) *pidCounter {
	s.mu.RLock()
	c := s.pidByPort[pid]
	s.mu.RUnlock()
	if c != nil {
		return c
	}
	s.mu.Lock()
	c = s.pidByPort[pid]
	if c == nil {
		c = &pidCounter{}
		s.pidByPort[pid] = c
		s.watch[pid] = true
	}
	s.mu.Unlock()
	return c
}

func (s *linuxSniffer) counters(pid int32) (rx, tx uint64, err error) {
	c := s.counterFor(pid)
	return c.rx.Load(), c.tx.Load(), nil
}

func (s *linuxSniffer) refreshLoop() {
	defer s.wg.Done()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.refresh()
		}
	}
}

func (s *linuxSniffer) refresh() {
	s.mu.RLock()
	watched := make([]int32, 0, len(s.watch))
	for pid := range s.watch {
		watched = append(watched, pid)
	}
	s.mu.RUnlock()
	if len(watched) == 0 {
		return
	}

	inodeToPid := make(map[string]int32)
	for _, pid := range watched {
		dir := filepath.Join("/proc", strconv.Itoa(int(pid)), "fd")
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			link, err := os.Readlink(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inode := link[len("socket:[") : len(link)-1]
				inodeToPid[inode] = pid
			}
		}
	}

	ports := make(map[portKey]int32)
	parsePortsFile("/proc/net/tcp", false, inodeToPid, ports)
	parsePortsFile("/proc/net/udp", false, inodeToPid, ports)
	parsePortsFile("/proc/net/tcp6", true, inodeToPid, ports)
	parsePortsFile("/proc/net/udp6", true, inodeToPid, ports)

	s.mu.Lock()
	s.ports = ports
	s.mu.Unlock()
}

func parsePortsFile(path string, v6 bool, inodeToPid map[string]int32, out map[portKey]int32) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 10 {
			continue
		}
		inode := fields[9]
		pid, ok := inodeToPid[inode]
		if !ok {
			continue
		}
		local := fields[1]
		colon := strings.IndexByte(local, ':')
		if colon < 0 {
			continue
		}
		port, err := strconv.ParseUint(local[colon+1:], 16, 16)
		if err != nil {
			continue
		}
		out[portKey{v6: v6, port: uint16(port)}] = pid
	}
}

func (s *linuxSniffer) packetLoop() {
	defer s.wg.Done()
	buf := make([]byte, 65536)
	for {
		select {
		case <-s.stop:
			return
		default:
		}
		n, from, err := unix.Recvfrom(s.fd, buf, 0)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return
		}
		ll, ok := from.(*unix.SockaddrLinklayer)
		if !ok {
			continue
		}
		outbound := ll.Pkttype == unix.PACKET_OUTGOING
		s.attribute(buf[:n], outbound)
	}
}

func (s *linuxSniffer) attribute(pkt []byte, outbound bool) {
	if len(pkt) < 20 {
		return
	}
	var v6 bool
	var proto uint8
	var hdrLen int
	switch pkt[0] >> 4 {
	case 4:
		proto = pkt[9]
		hdrLen = int(pkt[0]&0x0F) * 4
		if hdrLen < 20 {
			hdrLen = 20
		}
	case 6:
		if len(pkt) < 40 {
			return
		}
		v6 = true
		proto = pkt[6]
		hdrLen = 40
	default:
		return
	}
	if proto != 6 && proto != 17 {
		return
	}
	if len(pkt) < hdrLen+4 {
		return
	}
	var localPort uint16
	if outbound {
		localPort = binary.BigEndian.Uint16(pkt[hdrLen : hdrLen+2])
	} else {
		localPort = binary.BigEndian.Uint16(pkt[hdrLen+2 : hdrLen+4])
	}

	s.mu.RLock()
	pid, ok := s.ports[portKey{v6: v6, port: localPort}]
	s.mu.RUnlock()
	if !ok {
		return
	}
	c := s.counterFor(pid)
	if outbound {
		c.tx.Add(uint64(len(pkt)))
	} else {
		c.rx.Add(uint64(len(pkt)))
	}
}

func (s *linuxSniffer) close() error {
	close(s.stop)
	unix.Close(s.fd)
	s.wg.Wait()
	return nil
}
