//go:build darwin

package netlimit

import (
	"os/exec"
	"strconv"
	"strings"
)

type darwinMonitor struct{}

func newMonitor() Monitor {
	return &darwinMonitor{}
}

func (m *darwinMonitor) SystemCounters() (rx, tx uint64, err error) {
	out, err := exec.Command("netstat", "-ibn").CombinedOutput()
	if err != nil {
		return 0, 0, err
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "<Link#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if strings.HasPrefix(fields[0], "lo") {
			continue
		}
		ibytes, e1 := strconv.ParseUint(fields[len(fields)-5], 10, 64)
		obytes, e2 := strconv.ParseUint(fields[len(fields)-2], 10, 64)
		if e1 == nil {
			rx += ibytes
		}
		if e2 == nil {
			tx += obytes
		}
	}
	return rx, tx, nil
}

func (m *darwinMonitor) AppCounters(pid int32) (rx, tx uint64, err error) {
	return 0, 0, errPerAppSpeed
}

func (m *darwinMonitor) Close() error {
	return nil
}
