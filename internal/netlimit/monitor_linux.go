//go:build linux

package netlimit

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type linuxMonitor struct {
	sniff *linuxSniffer
}

func newMonitor() Monitor {
	return &linuxMonitor{}
}

func (m *linuxMonitor) SystemCounters() (rx, tx uint64, err error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		iface := strings.TrimSpace(line[:idx])
		if iface == "lo" || strings.HasPrefix(iface, "ifb") {
			continue
		}
		fields := strings.Fields(line[idx+1:])
		if len(fields) < 9 {
			continue
		}
		if v, e := strconv.ParseUint(fields[0], 10, 64); e == nil {
			rx += v
		}
		if v, e := strconv.ParseUint(fields[8], 10, 64); e == nil {
			tx += v
		}
	}
	return rx, tx, sc.Err()
}

func (m *linuxMonitor) AppCounters(pid int32) (rx, tx uint64, err error) {
	if m.sniff == nil {
		s, err := newLinuxSniffer()
		if err != nil {
			return 0, 0, err
		}
		m.sniff = s
	}
	return m.sniff.counters(pid)
}

func (m *linuxMonitor) Close() error {
	if m.sniff != nil {
		return m.sniff.close()
	}
	return nil
}
