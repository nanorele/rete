package sidebar

import (
	"fmt"
	"image"
	"reflect"
	"testing"
	"unsafe"

	"tracto/internal/model"
	"tracto/internal/ui/environments"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func peekHover(h *gesture.Hover) (entered bool, count int) {
	v := reflect.ValueOf(h).Elem()
	e := v.FieldByName("entered")
	c := v.FieldByName("count")
	entered = *(*bool)(unsafe.Pointer(e.UnsafeAddr()))
	count = *(*int)(unsafe.Pointer(c.UnsafeAddr()))
	return
}

func TestRealLayoutEnvHoverScroll(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	colsMenuBtn := &widget.Clickable{}
	colsMenuOpen := false
	envsMenuBtn := &widget.Clickable{}
	envsMenuOpen := false
	colsMenuOpen2 := false
	host.ColsMenuBtn = colsMenuBtn
	host.ColsMenuOpen = &colsMenuOpen
	host.EnvsMenuBtn = envsMenuBtn
	host.EnvsMenuOpen = &envsMenuOpen
	_ = colsMenuOpen2

	host.LayoutToggleBtn = func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}

	colsExp := false
	envsExp := true
	host.ColsExpanded = &colsExp
	host.EnvsExpanded = &envsExp

	const N = 30
	envs := make([]*environments.EnvironmentUI, N)
	for i := range envs {
		envs[i] = &environments.EnvironmentUI{
			Data: &model.ParsedEnvironment{ID: fmt.Sprintf("e%d", i), Name: fmt.Sprintf("env-%d", i)},
		}
		envs[i].InlineNameEd.SingleLine = true
	}
	*host.Environments = envs

	r := new(input.Router)
	frame := func() {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(220, 240)),
			Source:      r.Source(),
		}
		Layout(gtx, host)
		r.Frame(gtx.Ops)
	}

	dump := func(label string) int {
		var idxs []int
		n := 0
		for i, e := range envs {
			ent, cnt := peekHover(&e.Hover)
			if ent || cnt != 0 {
				idxs = append(idxs, i)
				if ent {
					n++
				}
			}
		}
		t.Logf("%-20s hovered=%v", label, idxs)
		return n
	}

	frame()
	// calibrate: sweep Y to find a coordinate that hovers an env row
	hitY := -1
	for y := 0; y < 240; y += 4 {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, float32(y)), Source: pointer.Mouse})
		frame()
		n := 0
		for _, e := range envs {
			if ent, _ := peekHover(&e.Hover); ent {
				n++
			}
		}
		if n > 0 {
			hitY = y
			t.Logf("hover hit at y=%d (n=%d)", y, n)
			break
		}
	}
	if hitY < 0 {
		t.Fatal("could not find a Y that hovers an env row")
	}
	dump("after hover")

	// Drive virtualization directly: advance the first visible index while the
	// cursor stays put. The router's frame-time re-hit-test sees a different row
	// under p.last each frame — the real wheel-scroll condition.
	for s := 1; s <= 12; s++ {
		host.EnvList.Position.First = s
		host.EnvList.Position.Offset = 0
		frame()
	}
	t.Logf("EnvList.Position.First=%d Offset=%d", host.EnvList.Position.First, host.EnvList.Position.Offset)
	nScroll := dump("after scroll")

	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(110, 999), Source: pointer.Mouse})
	frame()
	nOutside := dump("after move outside")

	t.Logf("hovered after scroll=%d, after-outside=%d", nScroll, nOutside)
	if nOutside > 0 {
		t.Errorf("STUCK HOVER: %d env(s) still hovered after cursor left the list", nOutside)
	}
}
