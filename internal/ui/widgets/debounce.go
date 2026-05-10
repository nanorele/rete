package widgets

import (
	"time"

	"github.com/nanorele/gio/app"
)

func ArmInvalidateTimer(timer **time.Timer, win *app.Window, delay time.Duration) {
	if win == nil {
		return
	}
	if *timer != nil {
		(*timer).Stop()
	}
	*timer = time.AfterFunc(delay, win.Invalidate)
}
