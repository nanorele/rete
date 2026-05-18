//go:build windows

package mitm

import (
	"sync"

	"golang.org/x/sys/windows"
)

// Process elevation never changes during the process lifetime, so the
// answer is computed once. Without this, IsAdmin gets hit several times
// per frame from button-state checks and turns into measurable jitter.
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
