//go:build windows

package netlimit

import (
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func listProcs() ([]ProcInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap) //nolint:errcheck

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Process32First(snap, &entry); err != nil {
		return nil, err
	}

	out := make([]ProcInfo, 0, 256)
	for {
		name := windows.UTF16ToString(entry.ExeFile[:])
		if name != "" && entry.ProcessID != 0 {
			out = append(out, ProcInfo{
				PID:  int32(entry.ProcessID),
				Name: name,
				Exe:  procExePath(entry.ProcessID),
			})
		}
		if err := windows.Process32Next(snap, &entry); err != nil {
			break
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func procExePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	buf := make([]uint16, windows.MAX_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}
