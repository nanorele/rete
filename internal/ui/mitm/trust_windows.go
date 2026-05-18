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

// trustInstalledTTL bounds how often we shell out to certutil. Without
// this cache, TrustInstalled() runs once per frame from the CA bar —
// during a 60Hz drag of the splitter that's 60 forked processes per
// second, which makes the UI feel like it's chewing through molasses.
const trustInstalledTTL = 30 * time.Second

var (
	trustMu      sync.Mutex
	trustChecked time.Time
	trustCached  bool
)

// InvalidateTrustCache forces the next TrustInstalled call to re-check.
// Called after Install/Uninstall so the UI reflects the change instantly.
func InvalidateTrustCache() {
	trustMu.Lock()
	trustChecked = time.Time{}
	trustMu.Unlock()
}

// InstallTrust adds certPath to the LocalMachine\Root store via certutil.
// Requires the calling process to be elevated.
//
// Browsers that use Windows trust (Chrome, Edge, IE, .NET, Java with
// Windows-store integration) pick this up automatically. Firefox uses
// its own NSS database and is NOT touched here — the user must import
// the certificate manually; the UI provides instructions and a reveal-
// in-Explorer button so they don't have to hunt for the file.
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

// UninstallTrust removes any previously installed Tracto root by subject
// CN. Firefox's NSS DB is not modified here for the same reason as
// InstallTrust — we don't touch other apps' state.
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

// TrustInstalled reports whether a cert with the Tracto root CN appears
// in LocalMachine\Root. Cached — see trustInstalledTTL.
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
