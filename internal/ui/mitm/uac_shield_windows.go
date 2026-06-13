//go:build windows

package mitm

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errShellAPI = errors.New("shell32 call failed")

const (
	siidShield     = 77
	shgsiIcon      = 0x000000100
	shgsiSmallIcon = 0x000000001
	shgsiLargeIcon = 0x000000000
)

type shStockIconInfo struct {
	cbSize        uint32
	hIcon         windows.Handle
	iSysIconIndex int32
	iIcon         int32
	szPath        [windows.MAX_PATH]uint16
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}

type bitmapInfo struct {
	BiSize          uint32
	BiWidth         int32
	BiHeight        int32
	BiPlanes        uint16
	BiBitCount      uint16
	BiCompression   uint32
	BiSizeImage     uint32
	BiXPelsPerMeter int32
	BiYPelsPerMeter int32
	BiClrUsed       uint32
	BiClrImportant  uint32
}

type bitmapInfoHeader struct {
	bmiHeader bitmapInfo
	bmiColors [256]uint32
}

var (
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	user32                 = windows.NewLazySystemDLL("user32.dll")
	gdi32                  = windows.NewLazySystemDLL("gdi32.dll")
	procSHGetStockIconInfo = shell32.NewProc("SHGetStockIconInfo")
	procDestroyIcon        = user32.NewProc("DestroyIcon")
	procGetIconInfo        = user32.NewProc("GetIconInfo")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetDIBits          = gdi32.NewProc("GetDIBits")
	procDeleteObject       = gdi32.NewProc("DeleteObject")

	shieldOnce sync.Once
	shieldPNG  []byte
	shieldErr  error
)

func UACShieldPNG() ([]byte, error) {
	shieldOnce.Do(func() {
		shieldPNG, shieldErr = renderShieldPNG()
	})
	return shieldPNG, shieldErr
}

func renderShieldPNG() ([]byte, error) {
	var info shStockIconInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	r, _, _ := procSHGetStockIconInfo.Call(
		uintptr(siidShield),
		uintptr(shgsiIcon|shgsiSmallIcon),
		uintptr(unsafe.Pointer(&info)),
	)
	if r != 0 {
		return nil, errShellAPI
	}
	defer procDestroyIcon.Call(uintptr(info.hIcon)) //nolint:errcheck

	var ii iconInfo
	r, _, _ = procGetIconInfo.Call(uintptr(info.hIcon), uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return nil, errShellAPI
	}
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(uintptr(ii.hbmMask)) //nolint:errcheck
	}
	if ii.hbmColor != 0 {
		defer procDeleteObject.Call(uintptr(ii.hbmColor)) //nolint:errcheck
	}

	const w, h = 16, 16
	pixels := make([]byte, w*h*4)
	var bi bitmapInfoHeader
	bi.bmiHeader.BiSize = 40
	bi.bmiHeader.BiWidth = w
	bi.bmiHeader.BiHeight = -h
	bi.bmiHeader.BiPlanes = 1
	bi.bmiHeader.BiBitCount = 32
	bi.bmiHeader.BiCompression = 0

	hdc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, hdc) //nolint:errcheck

	r, _, _ = procGetDIBits.Call(
		hdc,
		uintptr(ii.hbmColor),
		0,
		h,
		uintptr(unsafe.Pointer(&pixels[0])),
		uintptr(unsafe.Pointer(&bi)),
		0,
	)
	if r == 0 {
		return nil, errShellAPI
	}

	for i := 0; i < len(pixels); i += 4 {
		pixels[i], pixels[i+2] = pixels[i+2], pixels[i]
	}

	img := &image.RGBA{Pix: pixels, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
