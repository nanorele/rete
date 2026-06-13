package widgets

import (
	"time"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
)

type Fade struct {
	val  float32
	last time.Time
}

// Value returns the current animation value (0..1) without advancing it, for
// readers other than the single caller that drives Update each frame.
func (f *Fade) Value() float32 { return f.val }

func (f *Fade) Update(gtx layout.Context, on bool, dur time.Duration) float32 {
	target := float32(0)
	if on {
		target = 1
	}
	if f.last.IsZero() || dur <= 0 {
		f.last = gtx.Now
		f.val = target
		return f.val
	}
	dt := float32(gtx.Now.Sub(f.last).Seconds())
	f.last = gtx.Now
	step := dt / float32(dur.Seconds())
	switch {
	case f.val < target:
		f.val += step
		if f.val >= target {
			f.val = target
		} else {
			gtx.Execute(op.InvalidateCmd{})
		}
	case f.val > target:
		f.val -= step
		if f.val <= target {
			f.val = target
		} else {
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	return f.val
}
