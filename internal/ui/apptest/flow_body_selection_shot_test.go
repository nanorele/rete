//go:build screenshots

package apptest

import (
	"image"
	"testing"
	"time"

	. "rete/internal/ui"
	"rete/internal/ui/flow"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestFlowBodySelectionSeamsRealFonts(t *testing.T) {
	setupTestConfigDir(t)
	sz := image.Pt(1200, 800)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skip(err)
	}
	defer win.Release()
	var r input.Router
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "flows"
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{Ops: ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(sz), Source: r.Source(), Now: time.Now()}
		ui.LayoutApp(gtx)
		r.Frame(ops)
		if err := win.Frame(ops); err != nil {
			t.Fatal(err)
		}
	}
	shot := func() *image.RGBA {
		img := image.NewRGBA(image.Rectangle{Max: sz})
		if err := win.Screenshot(img); err != nil {
			t.Fatal(err)
		}
		return img
	}
	frame()
	frame()
	ed := ui.Flow
	n := flow.NewNode(flow.KindRequest, 120, 120)
	n.BodyEd.SetText("aaaa    aaaa\nbbbb    bbbb\ncccc    cccc\ndddd    dddd")
	ed.Scenario.Nodes = append(ed.Scenario.Nodes, n)
	frame()
	frame()
	plain := shot()
	ce := n.CanvasBodyEditor()
	r.Source().Execute(key.FocusCmd{Tag: ce})
	frame()
	ce.SetCaret(0, ce.Len())
	frame()
	frame()
	sel := shot()

	var diffRect image.Rectangle
	first := true
	for y := 0; y < sz.Y; y++ {
		for x := 0; x < sz.X; x++ {
			if plain.RGBAAt(x, y) != sel.RGBAAt(x, y) {
				p := image.Rect(x, y, x+1, y+1)
				if first {
					diffRect, first = p, false
				} else {
					diffRect = diffRect.Union(p)
				}
			}
		}
	}
	if first {
		t.Fatal("no selection highlight found")
	}
	x := diffRect.Min.X + 3
	firstHi, lastHi := -1, -1
	var rows []bool
	for y := diffRect.Min.Y; y < diffRect.Max.Y; y++ {
		hi := plain.RGBAAt(x, y) != sel.RGBAAt(x, y)
		rows = append(rows, hi)
		if hi {
			if firstHi < 0 {
				firstHi = y
			}
			lastHi = y
		}
	}
	gaps := 0
	for i, hi := range rows {
		y := diffRect.Min.Y + i
		if y > firstHi && y < lastHi && !hi {
			gaps++
		}
	}
	t.Logf("diff=%v column x=%d highlight rows %d..%d gap rows=%d", diffRect, x, firstHi, lastHi, gaps)
	if gaps > 0 {
		t.Errorf("selection has %d unhighlighted rows between lines", gaps)
	}
	mid := (diffRect.Min.X + diffRect.Max.X) / 2
	base := sel.RGBAAt(mid, firstHi+2)
	odd := 0
	for y := firstHi; y <= lastHi; y++ {
		if c := sel.RGBAAt(mid, y); c != base {
			odd++
			t.Logf("row %d at the blank column x=%d = %v, want %v", y, mid, c, base)
		}
	}
	if odd > 0 {
		t.Errorf("%d rows of the multi-line selection are tinted differently (double-painted seams)", odd)
	}
}
