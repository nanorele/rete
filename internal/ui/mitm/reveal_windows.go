//go:build windows

package mitm

import (
	"os/exec"
	"syscall"
)

func RevealInExplorer(path string) error {
	cmd := exec.Command("explorer.exe", "/select,", path)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}

	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
