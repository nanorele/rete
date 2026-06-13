//go:build !windows

package netlimit

import (
	"fmt"
	"os"
)

func IsElevated() bool {
	return os.Geteuid() == 0
}

func RelaunchElevated(extraArgs ...string) error {
	return fmt.Errorf("relaunch as administrator is only supported on Windows")
}
