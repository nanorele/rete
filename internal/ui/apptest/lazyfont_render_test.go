package apptest

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"

	. "tracto/internal/ui"
	"tracto/internal/ui/workspace"
)

// TestLazyFontsRenderMultilingualResponse rasterizes a response body mixing
// Latin, Cyrillic, CJK, RTL and color emoji through the real app shaper, so a
// deferred face that fails to load shows up as missing ink rather than only as
// a shaping-level difference.
func TestLazyFontsRenderMultilingualResponse(t *testing.T) {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	if len(ui.Tabs) == 0 {
		ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("multilang")}
		ui.ActiveIdx = 0
	}

	sz := image.Pt(900, 620)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	shot := func(body string) image.Image {
		ui.Tabs[ui.ActiveIdx].RespEditor.SetText(body)
		for i := 0; i < 2; i++ {
			ui.LayoutApp(renderGtx(new(op.Ops), sz))
		}
		ops := new(op.Ops)
		ui.LayoutApp(renderGtx(ops, sz))
		if err := win.Frame(ops); err != nil {
			t.Fatalf("frame: %v", err)
		}
		img := image.NewRGBA(image.Rectangle{Max: win.Size()})
		if err := win.Screenshot(img); err != nil {
			t.Fatalf("screenshot: %v", err)
		}
		return img
	}

	blank := shot("{\n  \"a\": 1\n}")
	filled := shot("{\n  \"ru\": \"Привет\",\n  \"zh\": \"你好世界\",\n  \"he\": \"שלום\",\n  \"th\": \"สวัสดี\",\n  \"emoji\": \"🙂🚀🎉\"\n}")

	if ink(filled) <= ink(blank) {
		t.Fatalf("multilingual body drew no additional ink (blank=%d filled=%d)", ink(blank), ink(filled))
	}
	baseHues, emojiHues := distinctHues(blank), distinctHues(filled)
	if emojiHues < baseHues+4 {
		t.Errorf("color emoji bitmaps added no hues: plain body %d, multilingual body %d", baseHues, emojiHues)
	}
}

func renderGtx(ops *op.Ops, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Now:         time.Unix(1700000000, 0),
	}
}

func ink(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r>>8 > 0x90 || g>>8 > 0x90 || bl>>8 > 0x90 {
				n++
			}
		}
	}
	return n
}

// distinctHues counts strongly saturated colors, which monochrome text cannot
// produce but color emoji bitmaps do.
func distinctHues(img image.Image) int {
	b := img.Bounds()
	seen := map[uint32]bool{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r32, g32, b32, _ := img.At(x, y).RGBA()
			r, g, bl := r32>>8, g32>>8, b32>>8
			maxc, minc := max3(r, g, bl), min3(r, g, bl)
			if maxc-minc < 0x50 {
				continue
			}
			seen[(r>>5)<<10|(g>>5)<<5|(bl>>5)] = true
		}
	}
	return len(seen)
}

func max3(a, b, c uint32) uint32 {
	if b > a {
		a = b
	}
	if c > a {
		a = c
	}
	return a
}

func min3(a, b, c uint32) uint32 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
