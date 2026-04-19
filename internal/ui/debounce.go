package ui

import (
	"time"

	"github.com/nanorele/gio/app"
)

func armInvalidateTimer(timer **time.Timer, win *app.Window, delay time.Duration) {
	if win == nil {
		return
	}
	if *timer != nil {
		(*timer).Stop()
	}
	*timer = time.AfterFunc(delay, win.Invalidate)
}

// debounceDim returns a stabilized constraint value. While target changes
// frame-to-frame (e.g. during a window resize) it keeps returning *last and
// re-arms an invalidate timer. Once target has held steady for settleDelay,
// it promotes *pending into *last.
//
// Why: the forked widget.Editor invalidates and reshapes the full text when
// its Max.X or Max.Y changes (see widget/text.go calculateViewSize). For a
// 10 MB response that's a multi-second stall per frame during resize.
//
// settleDelay is tuned so rapid resize frames (~16–50 ms apart) always keep
// resetting the timer, while a held-steady size settles quickly after the
// user releases the drag.
const (
	settleDelay     = 80 * time.Millisecond
	settleTimerFire = 100 * time.Millisecond
)

func debounceDim(target int, last, pending *int, changeTime *time.Time, timer **time.Timer, win *app.Window, now time.Time, isDragging bool) int {
	if *last <= 0 {
		*last = target
	}
	if target == *last || isDragging {
		return *last
	}
	if *pending != target {
		*pending = target
		*changeTime = now
		armInvalidateTimer(timer, win, settleTimerFire)
	}
	if now.Sub(*changeTime) > settleDelay {
		*last = *pending
		*pending = 0
	}
	return *last
}
