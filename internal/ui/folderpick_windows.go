//go:build windows

package ui

import (
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32             = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize   = modOle32.NewProc("CoUninitialize")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procCoTaskMemFree    = modOle32.NewProc("CoTaskMemFree")

	modUser32               = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow = modUser32.NewProc("GetForegroundWindow")
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

const (
	_CLSCTX_INPROC_SERVER     = 0x1
	_COINIT_APARTMENTTHREADED = 0x2

	_FOS_PICKFOLDERS     = 0x20
	_FOS_FORCEFILESYSTEM = 0x40

	_SIGDN_FILESYSPATH = 0x80058000

	_slotShow           = 3
	_slotSetOptions     = 9
	_slotSetTitle       = 17
	_slotGetResult      = 20
	_slotGetDisplayName = 5
	_slotRelease        = 2
)

func comCallN(this unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(this)
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(this)}, args...)...)
	return r
}

func comRelease(this unsafe.Pointer) {
	if this != nil {
		comCallN(this, _slotRelease)
	}
}

func pickFolderDialog(title string) (string, bool) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hr, _, _ := procCoInitializeEx.Call(0, _COINIT_APARTMENTTHREADED)
	if hr == 0 || hr == 1 {
		defer procCoUninitialize.Call()
	}

	var dialog unsafe.Pointer
	ret, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOpenDialog)),
		0,
		_CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)),
		uintptr(unsafe.Pointer(&dialog)),
	)
	if ret != 0 || dialog == nil {
		return "", false
	}
	defer comRelease(dialog)

	comCallN(dialog, _slotSetOptions, _FOS_PICKFOLDERS|_FOS_FORCEFILESYSTEM)
	if title != "" {
		if titlePtr, err := windows.UTF16PtrFromString(title); err == nil {
			comCallN(dialog, _slotSetTitle, uintptr(unsafe.Pointer(titlePtr)))
		}
	}

	owner, _, _ := procGetForegroundWindow.Call()
	if r := comCallN(dialog, _slotShow, owner); r != 0 {
		return "", false
	}

	var item unsafe.Pointer
	if r := comCallN(dialog, _slotGetResult, uintptr(unsafe.Pointer(&item))); r != 0 || item == nil {
		return "", false
	}
	defer comRelease(item)

	var pathPtr *uint16
	if r := comCallN(item, _slotGetDisplayName, _SIGDN_FILESYSPATH, uintptr(unsafe.Pointer(&pathPtr))); r != 0 || pathPtr == nil {
		return "", false
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pathPtr)))

	path := windows.UTF16PtrToString(pathPtr)
	if path == "" {
		return "", false
	}
	return path, true
}
