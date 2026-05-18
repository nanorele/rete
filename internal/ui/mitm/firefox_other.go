//go:build !windows

package mitm

func FirefoxEnterpriseRootsEnabled() bool { return false }
