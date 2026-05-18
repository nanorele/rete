//go:build !windows

package mitm

import "errors"

func RevealInExplorer(path string) error {
	return errors.New("reveal-in-explorer not implemented on this platform; path: " + path)
}
