//go:build screenshots

package apptest

import (
	. "rete/internal/ui"

	"image"
	"testing"

	"rete/internal/ui/settings"
	"rete/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/op"
)

func renderGapScene(t *testing.T, setup func(*AppUI), sz image.Point) *image.RGBA {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.Tabs = nil
	seedTestData(ui)
	setup(ui)

	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	for i := 0; i < 2; i++ {
		ui.LayoutApp(newShotGtx(new(op.Ops), sz))
	}
	ops := new(op.Ops)
	ui.LayoutApp(newShotGtx(ops, sz))
	if err := win.Frame(ops); err != nil {
		t.Fatalf("frame: %v", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: win.Size()})
	if err := win.Screenshot(img); err != nil {
		t.Fatalf("screenshot: %v", err)
	}
	return img
}

func pxAt(img *image.RGBA, x, y int) [3]uint8 {
	i := img.PixOffset(x, y)
	return [3]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}
}

func gapRightOf(img *image.RGBA, x0, y int) int {
	bg := pxAt(img, x0+1, y)
	n := 0
	for x := x0 + 1; x < img.Rect.Max.X && pxAt(img, x, y) == bg; x++ {
		n++
	}
	return n
}

func gapLeftOfEdge(img *image.RGBA, y int) int {
	w := img.Rect.Max.X
	bg := pxAt(img, w-1, y)
	n := 0
	for x := w - 1; x >= 0 && pxAt(img, x, y) == bg; x-- {
		n++
	}
	return n
}

func TestBoundaryGapsUniform(t *testing.T) {
	sz := image.Point{X: 1280, Y: 800}
	http := func(hide bool, extraTabs int) func(*AppUI) {
		return func(ui *AppUI) {
			ui.Settings.Theme = "dark"
			settings.Apply(ui.Theme, ui.Settings)
			tab := respTab(ui)
			tab.LayoutMode = workspace.LayoutModeHoriz
			ui.Settings.HideSidebar = hide
			for i := 0; i < extraTabs; i++ {
				ui.Tabs = append(ui.Tabs, workspace.NewRequestTab("Request number long "+string(rune('A'+i))))
			}
		}
	}

	const urlRowY = 100
	const bodyY = 400
	const tabRowY = 60
	const gutterW = 36

	open := renderGapScene(t, http(false, 0), sz)
	hidden := renderGapScene(t, http(true, 0), sz)
	tabs := renderGapScene(t, http(false, 14), sz)

	hiddenGap := gapRightOf(hidden, gutterW, bodyY)
	if hiddenGap < 2 || hiddenGap > 8 {
		t.Fatalf("hidden-sidebar gap out of expected range: %d", hiddenGap)
	}
	sidebarW := 250
	if got := gapRightOf(open, sidebarW, bodyY); got != hiddenGap {
		t.Errorf("open-sidebar gap %d, want %d (same as hidden)", got, hiddenGap)
	}
	if got := gapRightOf(open, sidebarW, urlRowY); got != hiddenGap {
		t.Errorf("open-sidebar gap at URL row %d, want %d", got, hiddenGap)
	}

	wantRight := hiddenGap + 1
	if got := gapLeftOfEdge(open, bodyY); got != wantRight {
		t.Errorf("right edge to response pane %d, want %d", got, wantRight)
	}
	if got := gapLeftOfEdge(open, urlRowY); got != wantRight {
		t.Errorf("right edge to Send button %d, want %d", got, wantRight)
	}
	if got := gapLeftOfEdge(tabs, tabRowY); got != wantRight {
		t.Errorf("right edge to justified tab row %d, want %d", got, wantRight)
	}
}
