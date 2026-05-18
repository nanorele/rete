//go:build windows

package mitm

import (
	"os/exec"
	"syscall"
)

// RevealInExplorer opens File Explorer with the given path selected.
// Equivalent to "explorer.exe /select,<path>". No-op error on failure.
func RevealInExplorer(path string) error {
	cmd := exec.Command("explorer.exe", "/select,", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	// explorer.exe returns 1 even on success; ignore exit status, treat
	// only spawn failures as errors.
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
