//go:build !windows

package mitm

import "errors"

func UACShieldPNG() ([]byte, error) {
	return nil, errors.New("UAC shield not available on this platform")
}
