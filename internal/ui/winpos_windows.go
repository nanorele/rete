//go:build windows

package ui

import (
	"unsafe"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/event"
	"golang.org/x/sys/windows"
)

var (
	modUser32           = windows.NewLazySystemDLL("user32.dll")
	procGetWindowRect   = modUser32.NewProc("GetWindowRect")
	procSetWindowPos    = modUser32.NewProc("SetWindowPos")
	procMonitorFromRect = modUser32.NewProc("MonitorFromRect")
	procIsWindowVisible = modUser32.NewProc("IsWindowVisible")
)

const (
	swpNoSize            = 0x0001
	swpNoZOrder          = 0x0004
	swpNoActivate        = 0x0010
	monitorDefaultToNull = 0x00000000
)

type win32Rect struct {
	Left, Top, Right, Bottom int32
}

func windowHandleFromEvent(e event.Event) (uintptr, bool) {
	ve, ok := e.(app.Win32ViewEvent)
	if !ok || ve.HWND == 0 {
		return 0, false
	}
	return ve.HWND, true
}

func windowVisible(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsWindowVisible.Call(hwnd)
	return ret != 0
}

func windowRect(hwnd uintptr) (win32Rect, bool) {
	var r win32Rect
	if hwnd == 0 {
		return r, false
	}
	ret, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	if ret == 0 || r.Right <= r.Left || r.Bottom <= r.Top {
		return r, false
	}
	return r, true
}

func rectOnScreen(r win32Rect) bool {
	mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), monitorDefaultToNull)
	return mon != 0
}

// windowPosition reports the window's top-left corner in physical pixels. It
// fails for windows that no monitor shows: a minimized window parks at
// (-32000,-32000), and saving that would relaunch the app off-screen.
func windowPosition(hwnd uintptr) (int, int, bool) {
	r, ok := windowRect(hwnd)
	if !ok || !rectOnScreen(r) {
		return 0, 0, false
	}
	return int(r.Left), int(r.Top), true
}

// moveWindowTo places the window's top-left at x,y without touching its size.
// It refuses positions whose resulting rect lies outside every monitor (a
// disconnected display, or a resolution change since the position was saved).
func moveWindowTo(hwnd uintptr, x, y int) bool {
	r, ok := windowRect(hwnd)
	if !ok {
		return false
	}
	target := win32Rect{
		Left:   int32(x),
		Top:    int32(y),
		Right:  int32(x) + (r.Right - r.Left),
		Bottom: int32(y) + (r.Bottom - r.Top),
	}
	if !rectOnScreen(target) {
		return false
	}
	ret, _, _ := procSetWindowPos.Call(hwnd, 0, uintptr(int32(x)), uintptr(int32(y)), 0, 0,
		swpNoSize|swpNoZOrder|swpNoActivate)
	return ret != 0
}
