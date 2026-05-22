//go:build windows

package mitm

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	adminOnce sync.Once
	adminVal  bool
)

func IsAdmin() bool {
	adminOnce.Do(func() {
		token := windows.GetCurrentProcessToken()
		adminVal = token.IsElevated()
	})
	return adminVal
}
