//go:build windows

package mitm

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const trustInstalledTTL = 30 * time.Second

var (
	trustMu      sync.Mutex
	trustChecked time.Time
	trustCached  bool
)

func InvalidateTrustCache() {
	trustMu.Lock()
	trustChecked = time.Time{}
	trustMu.Unlock()
}

func InstallTrust(certPath string) error {
	if !IsAdmin() {
		return errors.New("trust store install requires Administrator")
	}
	cmd := exec.Command("certutil", "-addstore", "-f", "Root", certPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	InvalidateTrustCache()
	return nil
}

func UninstallTrust() error {
	if !IsAdmin() {
		return errors.New("trust store removal requires Administrator")
	}
	cmd := exec.Command("certutil", "-delstore", "Root", caCommonName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	InvalidateTrustCache()
	return nil
}

func TrustInstalled() bool {
	trustMu.Lock()
	if !trustChecked.IsZero() && time.Since(trustChecked) < trustInstalledTTL {
		v := trustCached
		trustMu.Unlock()
		return v
	}
	trustMu.Unlock()

	v := trustInstalledLive()

	trustMu.Lock()
	trustCached = v
	trustChecked = time.Now()
	trustMu.Unlock()
	return v
}

func trustInstalledLive() bool {
	cmd := exec.Command("certutil", "-store", "Root", caCommonName)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), caCommonName)
}
