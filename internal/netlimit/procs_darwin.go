//go:build darwin

package netlimit

import (
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

func listProcs() ([]ProcInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,comm=").CombinedOutput()
	if err != nil {
		return nil, err
	}
	res := make([]ProcInfo, 0, 256)
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:sp])
		if err != nil {
			continue
		}
		exe := strings.TrimSpace(line[sp+1:])
		name := exe
		if idx := strings.LastIndexByte(name, '/'); idx >= 0 {
			name = name[idx+1:]
		}
		res = append(res, ProcInfo{PID: int32(pid), Name: name, Exe: exe})
	}
	sort.Slice(res, func(i, j int) bool {
		return strings.ToLower(res[i].Name) < strings.ToLower(res[j].Name)
	})
	return res, nil
}
