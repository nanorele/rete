//go:build !windows

package ui

import "github.com/nanorele/gio/io/event"

func windowHandleFromEvent(event.Event) (uintptr, bool) { return 0, false }

func windowVisible(uintptr) bool { return true }

func windowPosition(uintptr) (int, int, bool) { return 0, 0, false }

func moveWindowTo(uintptr, int, int) bool { return false }
