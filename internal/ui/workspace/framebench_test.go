//go:build membench

package workspace

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/gpu/headless"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"tracto/internal/ui/settings"
	"tracto/pkg/syntax"
)

func TestFrameTime(t *testing.T) {
	settings.SyntaxHighlightMaxMB = 100
	sz := image.Pt(900, 700)
	win, err := headless.NewWindow(sz.X, sz.Y)
	if err != nil {
		t.Skipf("headless GPU backend unavailable: %v", err)
	}
	defer win.Release()

	for _, c := range []struct {
		name string
		wrap bool
		gen  func(int) string
	}{
		{"pretty wrap=on ", true, genPrettyJSON},
		{"pretty wrap=off", false, genPrettyJSON},
		{"min    wrap=on ", true, genMinifiedJSON},
	} {
		body := c.gen(benchTarget)
		v := NewResponseViewer()
		v.SetText(body)
		shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
		var r input.Router
		ops := new(op.Ops)
		now := time.Unix(1700000000, 0)

		frame := func() {
			ops.Reset()
			gtx := layout.Context{
				Ops:         ops,
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Exact(sz),
				Now:         now,
				Source:      r.Source(),
			}
			ResponseViewerStyle{
				Viewer: v, Shaper: shaper, TextSize: unit.Sp(13),
				Wrap: c.wrap, Padding: unit.Dp(4), Lang: syntax.LangJSON,
			}.Layout(gtx)
			r.Frame(ops)
			now = now.Add(16 * time.Millisecond)
		}

		for i := 0; i < 3; i++ {
			frame()
		}
		time.Sleep(tokenizeDebounce + 20*time.Millisecond)
		for i := 0; i < 3; i++ {
			frame()
		}

		const steps = 60
		total := v.lastTotalH
		if total <= 0 {
			total = 1
		}
		var layoutNs, gpuNs int64
		for i := 0; i < steps; i++ {
			v.SetScrollY(total * i / steps)
			t0 := time.Now()
			frame()
			t1 := time.Now()
			if err := win.Frame(ops); err != nil {
				t.Fatalf("frame: %v", err)
			}
			layoutNs += t1.Sub(t0).Nanoseconds()
			gpuNs += time.Since(t1).Nanoseconds()
		}
		fmt.Printf("%s  layout=%6.2fms/frame  gpu=%6.2fms/frame\n",
			c.name,
			float64(layoutNs)/float64(steps)/1e6,
			float64(gpuNs)/float64(steps)/1e6)
	}
}
