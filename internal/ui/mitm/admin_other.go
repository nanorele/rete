//go:build !windows

package mitm

import "os"

func IsAdmin() bool {
	return os.Geteuid() == 0
}
