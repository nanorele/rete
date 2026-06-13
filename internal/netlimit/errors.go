package netlimit

import "errors"

var (
	errUnsupported = errors.New("netlimit: not supported on this platform")
	errNoDriver    = errors.New("netlimit: WinDivert driver not available")
	errPerAppSpeed = errors.New("netlimit: per-app speed not available")
)
