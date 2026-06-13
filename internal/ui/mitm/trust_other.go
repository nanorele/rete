//go:build !windows

package mitm

import "errors"

// SetTrustRefreshNotify is a no-op off Windows: trust-store checks there are
// synchronous (TrustInstalled always reports false), so there is no async
// result to notify the UI about.
func SetTrustRefreshNotify(func()) {}

func InstallTrust(certPath string) error {
	return errors.New("trust store install not implemented on this platform — install " + certPath + " manually")
}

func UninstallTrust() error {
	return errors.New("trust store removal not implemented on this platform")
}

func TrustInstalled() bool { return false }
