//go:build !windows

package mitm

import "errors"

func SetTrustRefreshNotify(func()) {}

func InstallTrust(certPath string) error {
	return errors.New("trust store install not implemented on this platform — install " + certPath + " manually")
}

func UninstallTrust() error {
	return errors.New("trust store removal not implemented on this platform")
}

func TrustInstalled() bool { return false }
