package sidebar

import (
	"image"
	"testing"

	"rete/internal/ui/theme"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestMenuOutlineHandlesTinyRows(t *testing.T) {
	for _, sz := range []image.Point{{}, {X: 1, Y: 1}, {X: 3, Y: 3}, {X: 40, Y: 2}, {X: 240, Y: 23}} {
		gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
		paintMenuOutline(gtx, sz)
	}
}

func TestMenuRowBgSitsBetweenBaseAndHover(t *testing.T) {
	bg := menuRowBg(theme.BgDark)
	if bg == theme.BgDark || bg == theme.BgHover {
		t.Fatalf("menu row bg %v must differ from both base %v and hover %v", bg, theme.BgDark, theme.BgHover)
	}
	lum := func(c interface{ RGBA() (r, g, b, a uint32) }) uint32 {
		r, g, b, _ := c.RGBA()
		return r + g + b
	}
	if lo, mid, hi := lum(theme.BgDark), lum(bg), lum(theme.BgHover); !(lo < mid && mid < hi) {
		t.Fatalf("menu row bg luminance %d not between base %d and hover %d", mid, lo, hi)
	}
}
