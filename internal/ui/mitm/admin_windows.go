//go:build windows

package mitm

import "golang.org/x/sys/windows"

func IsAdmin() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}
