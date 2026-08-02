//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/widget/material"

	. "tracto/internal/ui"
)

func TestStartupBreakdown(t *testing.T) {
	setupTestConfigDir(t)

	a := snap()
	coll := AppFontCollection()
	b := snap()

	sh := text.NewShaper(text.WithCollection(coll))
	c := snap()

	shNoSys := text.NewShaper(text.NoSystemFonts(), text.WithCollection(coll))
	d := snap()

	th := material.NewTheme()
	th.Shaper = sh
	e := snap()

	ui := NewAppUI()
	ui.Window = new(app.Window)
	f := snap()

	fmt.Printf("\n=== startup breakdown ===\n")
	fmt.Printf("font collection (%2d faces)  +%6.2fMB\n", len(coll), mb(b.HeapAlloc-a.HeapAlloc))
	fmt.Printf("shaper (system fonts on)    +%6.2fMB\n", mb(c.HeapAlloc-b.HeapAlloc))
	fmt.Printf("shaper (system fonts off)   +%6.2fMB\n", mb(d.HeapAlloc-c.HeapAlloc))
	fmt.Printf("material theme              +%6.2fMB\n", mb(e.HeapAlloc-d.HeapAlloc))
	fmt.Printf("NewAppUI (2nd shaper etc)   +%6.2fMB\n", mb(f.HeapAlloc-e.HeapAlloc))
	fmt.Printf("total                        %6.2fMB\n", mb(f.HeapAlloc))
	runtime.KeepAlive(sh)
	runtime.KeepAlive(shNoSys)
	runtime.KeepAlive(th)
	runtime.KeepAlive(ui)
}
