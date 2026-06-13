//go:build windows

package netlimit

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

var ErrUACDenied = errors.New("UAC elevation denied by user")

var (
	adminOnce sync.Once
	adminVal  bool
)

func IsElevated() bool {
	adminOnce.Do(func() {
		token := windows.GetCurrentProcessToken()
		adminVal = token.IsElevated()
	})
	return adminVal
}

func RelaunchElevated(extraArgs ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}

	args := append([]string{}, os.Args[1:]...)
	for _, a := range extraArgs {
		if a == "" || hasArg(args, a) {
			continue
		}
		args = append(args, a)
	}
	argsLine := joinCmdLine(args)

	verbPtr, _ := windows.UTF16PtrFromString("runas")
	exePtr, _ := windows.UTF16PtrFromString(exe)
	var argsPtr *uint16
	if argsLine != "" {
		argsPtr, _ = windows.UTF16PtrFromString(argsLine)
	}
	cwd, _ := os.Getwd()
	cwdPtr, _ := windows.UTF16PtrFromString(cwd)

	if err := windows.ShellExecute(0, verbPtr, exePtr, argsPtr, cwdPtr, windows.SW_NORMAL); err != nil {
		var errno syscall.Errno
		if errors.As(err, &errno) && errno == 1223 {
			return ErrUACDenied
		}
		return err
	}
	return nil
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func joinCmdLine(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(quoteArg(a))
	}
	return b.String()
}

func quoteArg(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n\v\"") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	backslashes := 0
	for _, r := range s {
		switch r {
		case '\\':
			backslashes++
		case '"':
			for k := 0; k < backslashes*2; k++ {
				b.WriteByte('\\')
			}
			backslashes = 0
			b.WriteString(`\"`)
		default:
			for k := 0; k < backslashes; k++ {
				b.WriteByte('\\')
			}
			backslashes = 0
			b.WriteRune(r)
		}
	}
	for k := 0; k < backslashes*2; k++ {
		b.WriteByte('\\')
	}
	b.WriteByte('"')
	return b.String()
}
