//go:build windows

package folderpick

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

type fakeCOM struct {
	vtbl *[24]uintptr
}

type callRecord struct {
	this uintptr
	args []uintptr
}

func newFakeCOM(t *testing.T, slots map[int]uintptr) (unsafe.Pointer, func()) {
	t.Helper()
	vtbl := new([24]uintptr)
	for idx, fn := range slots {
		vtbl[idx] = fn
	}
	obj := &fakeCOM{vtbl: vtbl}
	return unsafe.Pointer(obj), func() { runtime.KeepAlive(obj); runtime.KeepAlive(vtbl) }
}

func TestComCallN(t *testing.T) {
	var rec callRecord

	noArgs := syscall.NewCallback(func(this uintptr) uintptr {
		rec = callRecord{this: this}
		return 0x1234
	})
	oneArg := syscall.NewCallback(func(this uintptr, a uintptr) uintptr {
		rec = callRecord{this: this, args: []uintptr{a}}
		return a + 1
	})
	threeArgs := syscall.NewCallback(func(this, a, b, c uintptr) uintptr {
		rec = callRecord{this: this, args: []uintptr{a, b, c}}
		return a + b + c
	})

	cases := []struct {
		name string
		slot int
		fn   uintptr
		args []uintptr
		want uintptr
	}{
		{"no arguments", 3, noArgs, nil, 0x1234},
		{"one argument", 9, oneArg, []uintptr{41}, 42},
		{"three arguments", 20, threeArgs, []uintptr{1, 2, 3}, 6},
		{"high slot index", 23, oneArg, []uintptr{7}, 8},
		{"slot zero", 0, noArgs, nil, 0x1234},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec = callRecord{}
			this, keepAlive := newFakeCOM(t, map[int]uintptr{c.slot: c.fn})
			defer keepAlive()

			got := comCallN(this, c.slot, c.args...)
			if got != c.want {
				t.Errorf("comCallN() = %d, want %d", got, c.want)
			}
			if rec.this != uintptr(this) {
				t.Errorf("callback this = %#x, want %#x", rec.this, uintptr(this))
			}
			if len(rec.args) != len(c.args) {
				t.Fatalf("callback got %d args, want %d", len(rec.args), len(c.args))
			}
			for i, a := range c.args {
				if rec.args[i] != a {
					t.Errorf("arg %d = %d, want %d", i, rec.args[i], a)
				}
			}
		})
	}
}

func TestComRelease(t *testing.T) {
	calls := 0
	release := syscall.NewCallback(func(this uintptr) uintptr {
		calls++
		return 0
	})

	t.Run("nil is a no-op", func(t *testing.T) {
		comRelease(nil)
		if calls != 0 {
			t.Errorf("comRelease(nil) invoked the vtable %d times, want 0", calls)
		}
	})

	t.Run("calls IUnknown slot 2", func(t *testing.T) {
		this, keepAlive := newFakeCOM(t, map[int]uintptr{_slotRelease: release})
		defer keepAlive()
		comRelease(this)
		if calls != 1 {
			t.Errorf("comRelease() invoked Release %d times, want 1", calls)
		}
	})
}

func TestVtableSlotConstants(t *testing.T) {
	cases := []struct {
		name string
		got  int
		want int
	}{
		{"IUnknown::Release", _slotRelease, 2},
		{"IModalWindow::Show", _slotShow, 3},
		{"IFileDialog::SetOptions", _slotSetOptions, 9},
		{"IFileDialog::SetTitle", _slotSetTitle, 17},
		{"IFileDialog::GetResult", _slotGetResult, 20},
		{"IShellItem::GetDisplayName", _slotGetDisplayName, 5},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s slot = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestWin32Constants(t *testing.T) {
	cases := []struct {
		name string
		got  uint64
		want uint64
	}{
		{"CLSCTX_INPROC_SERVER", _CLSCTX_INPROC_SERVER, 0x1},
		{"COINIT_APARTMENTTHREADED", _COINIT_APARTMENTTHREADED, 0x2},
		{"FOS_PICKFOLDERS", _FOS_PICKFOLDERS, 0x20},
		{"FOS_FORCEFILESYSTEM", _FOS_FORCEFILESYSTEM, 0x40},
		{"SIGDN_FILESYSPATH", _SIGDN_FILESYSPATH, 0x80058000},
		{"dialog options", _FOS_PICKFOLDERS | _FOS_FORCEFILESYSTEM, 0x60},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %#x, want %#x", c.name, c.got, c.want)
		}
	}
}

func TestDialogGUIDs(t *testing.T) {
	cases := []struct {
		name string
		got  windows.GUID
		want string
	}{
		{"CLSID_FileOpenDialog", clsidFileOpenDialog, "{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}"},
		{"IID_IFileOpenDialog", iidIFileOpenDialog, "{D57C7288-D4AD-4768-BE02-9D969532D960}"},
	}
	for _, c := range cases {
		want, err := windows.GUIDFromString(c.want)
		if err != nil {
			t.Fatalf("GUIDFromString(%q) error = %v", c.want, err)
		}
		if c.got != want {
			t.Errorf("%s = %v, want %v", c.name, c.got, want)
		}
	}
}

func TestPickFolderDialogReturnsFalseWhenCoCreateFails(t *testing.T) {
	saved := procCoCreateInstance
	procCoCreateInstance = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentProcessId")
	t.Cleanup(func() { procCoCreateInstance = saved })

	path, ok := PickFolderDialog("Choose a folder")
	if ok {
		t.Errorf("PickFolderDialog ok = true, want false when CoCreateInstance fails")
	}
	if path != "" {
		t.Errorf("PickFolderDialog path = %q, want empty", path)
	}
}

func TestLazyProcsResolve(t *testing.T) {
	procs := []struct {
		name string
		proc *windows.LazyProc
	}{
		{"ole32.CoInitializeEx", procCoInitializeEx},
		{"ole32.CoUninitialize", procCoUninitialize},
		{"ole32.CoCreateInstance", procCoCreateInstance},
		{"ole32.CoTaskMemFree", procCoTaskMemFree},
		{"user32.GetForegroundWindow", procGetForegroundWindow},
	}
	for _, p := range procs {
		if err := p.proc.Find(); err != nil {
			t.Errorf("%s not found: %v", p.name, err)
		}
	}
}
