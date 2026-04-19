package ui

import (
	"testing"
	"time"
	"github.com/nanorele/gio/app"
)

func TestDebounce(t *testing.T) {
	var timer *time.Timer
	win := new(app.Window)
	armInvalidateTimer(&timer, win, 1*time.Millisecond)
	if timer == nil {
		t.Errorf("expected timer to be armed")
	}
	
	// re-arm
	armInvalidateTimer(&timer, win, 1*time.Millisecond)
	
	// test nil win
	var timer2 *time.Timer
	armInvalidateTimer(&timer2, nil, 1*time.Millisecond)
	if timer2 != nil {
		t.Errorf("expected timer not to be armed")
	}
}
