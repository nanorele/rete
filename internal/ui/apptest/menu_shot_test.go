//go:build screenshots

package apptest

import (
	. "tracto/internal/ui"

	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/widget"
)

func menuShotItems() []widgets.MenuItem {
	return []widgets.MenuItem{
		{Label: "Open", Click: new(widget.Clickable)},
		{Label: "Copy", Icon: widgets.IconDup, Shortcut: "Ctrl+C", Click: new(widget.Clickable)},
		{Label: "Pinned", Checked: true, Click: new(widget.Clickable)},
		{Label: "Rename", Icon: widgets.IconRename, Bold: true, Click: new(widget.Clickable)},
		{Label: "/mono/path", Mono: true, Click: new(widget.Clickable)},
		{Label: "Colored", LabelCol: color.NRGBA{R: 90, G: 200, B: 250, A: 255}, Click: new(widget.Clickable)},
		{Separator: true},
		{Label: "Disabled", Disabled: true, Click: new(widget.Clickable)},
		{Label: "Delete", Icon: widgets.IconDel, Shortcut: "Del", Danger: true, Click: new(widget.Clickable)},
	}
}

func encodeMenuShot(t *testing.T, win *headless.Window, ops *op.Ops, name string) {
	t.Helper()
	if err := win.Frame(ops); err != nil {
		t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	dir := filepath.Join("testdata", "screenshots")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s", filepath.Join(dir, name+".png"))
}

func TestMenuStatesShot(t *testing.T) {
	sz := image.Pt(480, 520)
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	items := menuShotItems()
	ops := new(op.Ops)
	gtx := newShotGtx(ops, sz)
	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
	st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
	widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
	st.Pop()

	encodeMenuShot(t, win, ops, fmt.Sprintf("menu-states_%dx%d", sz.X, sz.Y))
}

func TestMenuStatesHoverShot(t *testing.T) {
	sz := image.Pt(480, 520)
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	items := menuShotItems()
	target := items[2].Click
	var r input.Router

	frame := func(evs ...pointer.Event) {
		ops := new(op.Ops)
		gtx := newShotGtx(ops, sz)
		gtx.Source = r.Source()
		for _, e := range evs {
			r.Queue(e)
		}
		paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
		st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
		widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
		st.Pop()
		r.Frame(ops)
	}

	var hy float32
	for y := float32(44); y < 260; y += 2 {
		frame(mv(60, y))
		frame()
		if target.Hovered() {
			hy = y
			break
		}
	}
	if hy == 0 {
		t.Skip("could not locate the 3rd item row to hover")
	}

	ops := new(op.Ops)
	gtx := newShotGtx(ops, sz)
	gtx.Source = r.Source()
	r.Queue(mv(60, hy))
	paint.FillShape(gtx.Ops, ui.Theme.Bg, clip.Rect{Max: sz}.Op())
	st := op.Offset(image.Pt(40, 40)).Push(gtx.Ops)
	widgets.MenuList(gtx, ui.Theme, nil, widgets.MenuMinWidthDp, items)
	st.Pop()

	encodeMenuShot(t, win, ops, fmt.Sprintf("menu-states-hover_%dx%d", sz.X, sz.Y))
	r.Frame(ops)
}
