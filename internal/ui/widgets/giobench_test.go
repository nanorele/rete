//go:build membench

package widgets

import (
	"fmt"
	"image"
	"image/color"
	"runtime"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

// perInstance lays out n copies of w and reports bytes allocated per instance
// per frame, plus the op-buffer growth the frame leaves behind.
func perInstance(t *testing.T, n int, build func(th *material.Theme) layout.Widget) (bytesPerInst float64) {
	t.Helper()
	th := newTestTheme()
	var r input.Router
	ops := new(op.Ops)
	now := time.Unix(1700000000, 0)
	w := build(th)
	sz := image.Pt(1200, 800)

	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Source:      r.Source(),
			Now:         now,
		}
		for i := 0; i < n; i++ {
			macro := op.Record(gtx.Ops)
			w(gtx)
			macro.Stop()
		}
		r.Frame(ops)
		now = now.Add(16 * time.Millisecond)
	}

	for i := 0; i < 8; i++ {
		frame()
	}
	const frames = 40
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < frames; i++ {
		frame()
	}
	runtime.ReadMemStats(&b)
	return float64(b.TotalAlloc-a.TotalAlloc) / float64(frames) / float64(n)
}

// perInstanceFresh is perInstance for content that differs every frame, so the
// shaper and path caches miss the way they do for live status text.
func perInstanceFresh(t *testing.T, n int, build func(th *material.Theme, frame int) layout.Widget) float64 {
	t.Helper()
	th := newTestTheme()
	var r input.Router
	ops := new(op.Ops)
	now := time.Unix(1700000000, 0)
	sz := image.Pt(1200, 800)
	f := 0

	frame := func() {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(sz),
			Source:      r.Source(),
			Now:         now,
		}
		w := build(th, f)
		for i := 0; i < n; i++ {
			macro := op.Record(gtx.Ops)
			w(gtx)
			macro.Stop()
		}
		r.Frame(ops)
		now = now.Add(16 * time.Millisecond)
		f++
	}
	for i := 0; i < 8; i++ {
		frame()
	}
	const frames = 40
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < frames; i++ {
		frame()
	}
	runtime.ReadMemStats(&b)
	return float64(b.TotalAlloc-a.TotalAlloc) / float64(frames) / float64(n)
}

func TestGioElementCost(t *testing.T) {
	const n = 200
	col := color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff}

	cases := []struct {
		name  string
		build func(th *material.Theme) layout.Widget
	}{
		{"paint.FillShape+clip.Rect", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, col, clip.Rect{Max: image.Pt(80, 20)}.Op())
				return layout.Dimensions{Size: image.Pt(80, 20)}
			}
		}},
		{"clip.UniformRRect", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				rr := clip.UniformRRect(image.Rectangle{Max: image.Pt(80, 20)}, 4)
				paint.FillShape(gtx.Ops, col, rr.Op(gtx.Ops))
				return layout.Dimensions{Size: image.Pt(80, 20)}
			}
		}},
		{"clip.Stroke", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				var p clip.Path
				p.Begin(gtx.Ops)
				p.MoveTo(f32.Pt(0, 0))
				p.LineTo(f32.Pt(80, 0))
				paint.FillShape(gtx.Ops, col, clip.Stroke{Path: p.End(), Width: 1}.Op())
				return layout.Dimensions{Size: image.Pt(80, 1)}
			}
		}},
		{"layout.Inset", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(40, 12)}
				})
			}
		}},
		{"layout.Flex(3 rigid)", func(th *material.Theme) layout.Widget {
			cell := func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(20, 12)}
			}
			return func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(cell), layout.Rigid(cell), layout.Rigid(cell))
			}
		}},
		{"layout.Stack(2)", func(th *material.Theme) layout.Widget {
			cell := func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(20, 12)}
			}
			return func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(cell), layout.Stacked(cell))
			}
		}},
		{"material.Label(short, repeated)", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				return material.Label(th, unit.Sp(12), "GET /v1/users").Layout(gtx)
			}
		}},
		{"material.Label(short, unique)", func(th *material.Theme) layout.Widget {
			txt := uniqueStrings(n, "GET /v1/users/%d")
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				s := txt[i%n]
				i++
				return material.Label(th, unit.Sp(12), s).Layout(gtx)
			}
		}},
		{"material.Label(80ch, unique)", func(th *material.Theme) layout.Widget {
			txt := uniqueStrings(n, "https://api.example.com/v2/resources/%d?expand=items&limit=50&page=2")
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), txt[i%n])
				i++
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}
		}},
		{"material.Label(80ch, truncated)", func(th *material.Theme) layout.Widget {
			txt := uniqueStrings(n, "https://api.example.com/v2/resources/%d?expand=items&limit=50&page=2")
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = 120
				lbl := material.Label(th, unit.Sp(12), txt[i%n])
				i++
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}
		}},
		{"widget.Clickable(empty)", func(th *material.Theme) layout.Widget {
			clks := make([]widget.Clickable, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				c := &clks[i%n]
				i++
				return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(40, 20)}
				})
			}
		}},
		{"material.Clickable(empty)", func(th *material.Theme) layout.Widget {
			clks := make([]widget.Clickable, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				c := &clks[i%n]
				i++
				return material.Clickable(gtx, c, func(gtx layout.Context) layout.Dimensions {
					return layout.Dimensions{Size: image.Pt(40, 20)}
				})
			}
		}},
		{"widget.Editor(single line)", func(th *material.Theme) layout.Widget {
			eds := make([]widget.Editor, n)
			txt := uniqueStrings(n, "value-example-%d")
			for i := range eds {
				eds[i].SingleLine = true
				eds[i].SetText(txt[i])
			}
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				e := &eds[i%n]
				i++
				return material.Editor(th, e, "").Layout(gtx)
			}
		}},
		{"gesture.Drag", func(th *material.Theme) layout.Widget {
			ds := make([]gesture.Drag, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				d := &ds[i%n]
				i++
				area := clip.Rect{Max: image.Pt(8, 20)}.Push(gtx.Ops)
				d.Add(gtx.Ops)
				for {
					if _, ok := d.Update(gtx.Metric, gtx.Source, gesture.Horizontal); !ok {
						break
					}
				}
				area.Pop()
				return layout.Dimensions{Size: image.Pt(8, 20)}
			}
		}},
		{"gesture.Scroll", func(th *material.Theme) layout.Widget {
			ss := make([]gesture.Scroll, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				s := &ss[i%n]
				i++
				area := clip.Rect{Max: image.Pt(80, 20)}.Push(gtx.Ops)
				s.Add(gtx.Ops)
				s.Update(gtx.Metric, gtx.Source, gtx.Now, gesture.Vertical,
					pointer.ScrollRange{}, pointer.ScrollRange{Min: -100, Max: 100})
				area.Pop()
				return layout.Dimensions{Size: image.Pt(80, 20)}
			}
		}},
		{"widget.Icon", func(th *material.Theme) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(18, 18)
				gtx.Constraints.Max = image.Pt(18, 18)
				return IconHAR.Layout(gtx, col)
			}
		}},
	}

	fmt.Printf("\n=== steady state: same content every frame (%d instances, 1200x800) ===\n", n)
	fmt.Printf("%-34s %14s %12s\n", "element", "B/inst/frame", "KB/frame")
	for _, c := range cases {
		per := perInstance(t, n, c.build)
		fmt.Printf("%-34s %14.1f %12.1f\n", c.name, per, per*float64(n)/1024)
	}

	fmt.Printf("\n=== content differs every frame (%d instances) ===\n", n)
	fresh := []struct {
		name  string
		build func(th *material.Theme, f int) layout.Widget
	}{
		{"material.Label(short)", func(th *material.Theme, f int) layout.Widget {
			txt := uniqueStrings(n, fmt.Sprintf("f%d GET /v1/users/%%d", f))
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				s := txt[i%n]
				i++
				return material.Label(th, unit.Sp(12), s).Layout(gtx)
			}
		}},
		{"material.Label(80ch, MaxLines 1)", func(th *material.Theme, f int) layout.Widget {
			txt := uniqueStrings(n, fmt.Sprintf("f%d https://api.example.com/v2/res/%%d?expand=items&limit=50", f))
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), txt[i%n])
				i++
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				return lbl.Layout(gtx)
			}
		}},
		{"MeasureTextWidth (uncached)", func(th *material.Theme, f int) layout.Widget {
			txt := uniqueStrings(n, fmt.Sprintf("f%d key_%%d", f))
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				MeasureTextWidth(gtx, th, unit.Sp(11), MonoFont, txt[i%n])
				i++
				return layout.Dimensions{}
			}
		}},
	}
	fmt.Printf("%-34s %14s %12s\n", "element", "B/inst/frame", "KB/frame")
	for _, c := range fresh {
		per := perInstanceFresh(t, n, c.build)
		fmt.Printf("%-34s %14.1f %12.1f\n", c.name, per, per*float64(n)/1024)
	}
}

func TestGioListCost(t *testing.T) {
	th := newTestTheme()
	for _, rows := range []int{100, 1000, 10000} {
		txt := uniqueStrings(rows, "GET /v1/resource/%d")
		var lst widget.List
		lst.Axis = layout.Vertical
		var r input.Router
		ops := new(op.Ops)
		now := time.Unix(1700000000, 0)
		sz := image.Pt(600, 700)

		frame := func() {
			ops.Reset()
			gtx := layout.Context{
				Ops:         ops,
				Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
				Constraints: layout.Exact(sz),
				Source:      r.Source(),
				Now:         now,
			}
			material.List(th, &lst).Layout(gtx, rows, func(gtx layout.Context, i int) layout.Dimensions {
				return material.Label(th, unit.Sp(12), txt[i]).Layout(gtx)
			})
			r.Frame(ops)
			now = now.Add(16 * time.Millisecond)
		}
		for i := 0; i < 8; i++ {
			frame()
		}
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		const frames = 40
		for i := 0; i < frames; i++ {
			frame()
		}
		runtime.ReadMemStats(&b)
		fmt.Printf("material.List rows=%6d  %8.1f KB/frame\n",
			rows, float64(b.TotalAlloc-a.TotalAlloc)/float64(frames)/1024)
	}
}

func uniqueStrings(n int, format string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(format, i)
	}
	return out
}
