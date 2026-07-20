package apptest

import (
	. "tracto/internal/ui"

	"image"
	"testing"
	"time"

	"tracto/internal/ui/workspace"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

type appSliderRig struct {
	ui    *AppUI
	r     input.Router
	sz    image.Point
	now   time.Time
	scale float32
}

func newAppSliderRig(t *testing.T) *appSliderRig {
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	ui.SidebarSection = "requests"
	ui.Tabs = []*workspace.RequestTab{workspace.NewRequestTab("T")}
	ui.ActiveIdx = 0
	tab := ui.Tabs[0]
	tab.Method = "POST"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
	tab.AddHeader("Authorization", "secret")
	tab.LayoutMode = workspace.LayoutModeVert
	tab.HeadersExpanded = true
	tab.HeadersAbsHeight = 100
	return &appSliderRig{ui: ui, sz: image.Pt(1100, 800), now: time.Unix(1700000000, 0)}
}

func (rig *appSliderRig) frame() {
	rig.now = rig.now.Add(16 * time.Millisecond)
	scale := rig.scale
	if scale <= 0 {
		scale = 1
	}
	ops := new(op.Ops)
	gtx := layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
		Constraints: layout.Exact(rig.sz),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(ops)
}

func (rig *appSliderRig) press(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) move(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(x), y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) movePt(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *appSliderRig) release(x int, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(float32(x), y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *appSliderRig) findHeadersSlider(t *testing.T, x int) int {
	tab := rig.ui.Tabs[0]
	for y := 80; y < 700; y++ {
		before := tab.HeadersAbsHeight
		rig.press(x, float32(y))
		rig.move(x, float32(y+8))
		changed := tab.HeadersAbsHeight != before
		rig.release(x, float32(y+8))
		if changed {
			tab.HeadersAbsHeight = 100
			for i := 0; i < 3; i++ {
				rig.frame()
			}
			return y
		}
		if tab.HeadersAbsHeight != before {
			tab.HeadersAbsHeight = 100
			for i := 0; i < 3; i++ {
				rig.frame()
			}
		}
	}
	t.Fatalf("headers slider not found")
	return 0
}

func TestHeadersSliderFullAppNoJitter(t *testing.T) {
	rig := newAppSliderRig(t)
	tab := rig.ui.Tabs[0]
	for i := 0; i < 4; i++ {
		rig.frame()
	}

	tab.VStackRatio = 0.01
	for i := 0; i < 3; i++ {
		rig.frame()
	}
	minRatio := tab.VStackRatio
	tab.VStackRatio = minRatio + 0.008
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	x := 700
	sliderY := rig.findHeadersSlider(t, x)
	t.Logf("slider found at y=%d, ratio=%.4f stored=%d", sliderY, tab.VStackRatio, tab.HeadersAbsHeight)

	rig.press(x, float32(sliderY))
	var ratios []float32
	var stores []int
	pos := float32(sliderY)
	for i := 1; i <= 25; i++ {
		pos += 1
		rig.move(x, pos)
		ratios = append(ratios, tab.VStackRatio)
		stores = append(stores, tab.HeadersAbsHeight)
	}
	for i := 1; i <= 25; i++ {
		pos -= 1
		rig.move(x, pos)
		ratios = append(ratios, tab.VStackRatio)
		stores = append(stores, tab.HeadersAbsHeight)
	}
	rig.release(x, pos)
	t.Logf("stores=%v", stores)
	t.Logf("ratios(x1e4)=%v", func() []int {
		out := make([]int, len(ratios))
		for i, r := range ratios {
			out[i] = int(r * 10000)
		}
		return out
	}())

	for i := 1; i < 25; i++ {
		if stores[i] < stores[i-1] {
			t.Fatalf("stored jitter on down step %d: %d -> %d", i, stores[i-1], stores[i])
		}
		if ratios[i] < ratios[i-1]-0.0001 {
			t.Fatalf("ratio jitter on down step %d: %.5f -> %.5f", i, ratios[i-1], ratios[i])
		}
	}
	for i := 26; i < 50; i++ {
		if stores[i] > stores[i-1] {
			t.Fatalf("stored jitter on up step %d: %d -> %d", i, stores[i-1], stores[i])
		}
		if ratios[i] > ratios[i-1]+0.0001 {
			t.Fatalf("ratio jitter on up step %d: %.5f -> %.5f", i, ratios[i-1], ratios[i])
		}
	}
}
