//go:build !windows

package mitm

import "errors"

var ErrUACDenied = errors.New("UAC elevation denied by user")

func RelaunchAsAdmin(_ ...string) error {
	return errors.New("relaunch as admin not implemented on this platform; please re-run with sudo or pkexec")
}

func CanRequestElevation() bool { return false }
