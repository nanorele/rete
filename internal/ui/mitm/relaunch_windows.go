//go:build windows

package mitm

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// ErrUACDenied is returned by RelaunchAsAdmin when the user clicked
// "No" on the UAC prompt (Windows error code 1223 / ERROR_CANCELLED).
var ErrUACDenied = errors.New("UAC elevation denied by user")

// RelaunchAsAdmin re-launches the current executable elevated via the
// shell's "runas" verb. extraArgs are appended to the current process'
// command-line so the new instance can pick up where the old one left
// off (e.g. "--mitm-start"). Returns nil on UAC accept (the new process
// is launching — caller should exit), ErrUACDenied if the user declined,
// or another error otherwise.
func RelaunchAsAdmin(extraArgs ...string) error {
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

// CanRequestElevation reports whether elevation can plausibly be
// requested via UAC. On Windows we always allow the prompt — failure
// to authenticate just returns ErrUACDenied later.
func CanRequestElevation() bool { return true }

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

// joinCmdLine quotes args following Windows CommandLineToArgvW rules so
// that os.Args in the elevated process matches the originals.
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
			for k := 0; k < backslashes; k++ {
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
