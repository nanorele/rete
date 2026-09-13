package widgets

import (
	"time"

	"github.com/nanorele/gio-x/component"
	"github.com/nanorele/gio/app"
)

func ArmInvalidateTimer(timer **time.Timer, win *app.Window, delay time.Duration) {
	component.ArmInvalidateTimer(timer, win, delay)
}
