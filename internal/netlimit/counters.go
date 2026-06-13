package netlimit

import "sync/atomic"

type pidCounter struct {
	rx atomic.Uint64
	tx atomic.Uint64
}
