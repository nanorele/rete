//go:build membench

package widgets

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/nanorele/gio-x/component"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func benchGtx(r *input.Router, ops *op.Ops, sz image.Point, now time.Time) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Source:      r.Source(),
		Now:         now,
	}
}

func TestGioXWrapCost(t *testing.T) {
	th := newTestTheme()
	var r input.Router
	ops := new(op.Ops)
	gtx := benchGtx(&r, ops, image.Pt(900, 700), time.Unix(1700000000, 0))
	fnt := font.Font{Typeface: MonoFamilyName}

	fmt.Printf("\n=== component.ShapeChunkForWrap ===\n")
	fmt.Printf("sizeof(component.WrapGlyph) = %d B\n", unsafe.Sizeof(component.WrapGlyph{}))
	fmt.Printf("%10s %12s %14s %14s %12s\n", "chunk", "glyphs", "alloc/call", "B per byte", "retained")

	for _, n := range []int{1 << 10, 8 << 10, 32 << 10, 128 << 10} {
		chunk := []byte(strings.Repeat("abcdefghij0123456789", n/20))
		// warm the shaper caches so the number reflects the wrap output only
		component.ShapeChunkForWrap(th.Shaper, fnt, unit.Sp(13), gtx, chunk, 600)

		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		const reps = 5
		var gl []component.WrapGlyph
		for i := 0; i < reps; i++ {
			gl = component.ShapeChunkForWrap(th.Shaper, fnt, unit.Sp(13), gtx, chunk, 600)
		}
		runtime.ReadMemStats(&b)
		perCall := float64(b.TotalAlloc-a.TotalAlloc) / reps
		fmt.Printf("%9dK %12d %13.0fK %14.1f %11.0fK\n",
			len(chunk)>>10, len(gl), perCall/1024, perCall/float64(len(chunk)),
			float64(uintptr(cap(gl))*unsafe.Sizeof(component.WrapGlyph{}))/1024)
		runtime.KeepAlive(gl)
	}

	fmt.Printf("\n=== hit-test helpers over a materialised chunk ===\n")
	chunk := []byte(strings.Repeat("abcdefghij0123456789", (32<<10)/20))
	gl := component.ShapeChunkForWrap(th.Shaper, fnt, unit.Sp(13), gtx, chunk, 600)
	for _, c := range []struct {
		name string
		fn   func()
	}{
		{"CaretXYInWrap(end)", func() { component.CaretXYInWrap(gl, len(chunk)-1) }},
		{"ByteOffInWrap(last line)", func() { component.ByteOffInWrap(gl, 590, component.WrapMaxLine(gl)) }},
		{"WrapLineStarts", func() { component.WrapLineStarts(gl) }},
	} {
		start := time.Now()
		const reps = 20
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		for i := 0; i < reps; i++ {
			c.fn()
		}
		runtime.ReadMemStats(&b)
		fmt.Printf("%-26s %8.3f ms/call  %8.1f KB/call\n", c.name,
			float64(time.Since(start).Microseconds())/1000/reps,
			float64(b.TotalAlloc-a.TotalAlloc)/reps/1024)
	}
	runtime.KeepAlive(gl)
}

func TestGioXWidgetCost(t *testing.T) {
	const n = 200
	fmt.Printf("\n=== gio-x widgets, %d instances/frame ===\n", n)
	fmt.Printf("sizeof(component.Hover) = %d B, sizeof(component.Fade) = %d B\n",
		unsafe.Sizeof(component.Hover{}), unsafe.Sizeof(component.Fade{}))

	cases := []struct {
		name  string
		build func(th *material.Theme) layout.Widget
	}{
		{"component.Hover", func(th *material.Theme) layout.Widget {
			hs := make([]component.Hover, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				h := &hs[i%n]
				i++
				h.Update(gtx.Source)
				area := clip.Rect{Max: image.Pt(80, 20)}.Push(gtx.Ops)
				h.Add(gtx.Ops)
				area.Pop()
				return layout.Dimensions{Size: image.Pt(80, 20)}
			}
		}},
		{"component.Fade", func(th *material.Theme) layout.Widget {
			fs := make([]component.Fade, n)
			i := 0
			return func(gtx layout.Context) layout.Dimensions {
				f := &fs[i%n]
				i++
				f.Update(gtx, true, 100*time.Millisecond)
				return layout.Dimensions{Size: image.Pt(80, 20)}
			}
		}},
	}
	fmt.Printf("%-26s %14s\n", "element", "B/inst/frame")
	for _, c := range cases {
		fmt.Printf("%-26s %14.1f\n", c.name, perInstance(t, n, c.build))
	}
}

// TestGioXTableCost checks that a ColumnTable inside a VScrollList costs
// per visible row, not per data row (G7 in task_giox.md).
func TestGioXTableCost(t *testing.T) {
	th := newTestTheme()
	cols := []component.TableColumn{
		{Title: "Method", Width: 60},
		{Title: "URL"},
		{Title: "Status", Width: 60},
		{Title: "Size", Width: 80},
	}
	fmt.Printf("\n=== component.ColumnTable in VScrollList ===\n")
	perFrame := map[int]float64{}
	for _, rows := range []int{100, 1000, 10000} {
		txt := uniqueStrings(rows, "/v1/resource/%d")
		tbl := component.NewColumnTable(cols)
		var lst widget.List
		lst.Axis = layout.Vertical
		var r input.Router
		ops := new(op.Ops)
		now := time.Unix(1700000000, 0)
		sz := image.Pt(700, 700)

		frame := func() {
			ops.Reset()
			gtx := benchGtx(&r, ops, sz, now)
			layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return tbl.Header(gtx, th, component.TableColors{})
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return component.VScrollList(gtx, th, &lst, rows, func(gtx layout.Context, i int) layout.Dimensions {
						return tbl.Row(gtx, func(c int) layout.Widget {
							return material.Label(th, unit.Sp(12), txt[i]).Layout
						})
					})
				}),
			)
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
		perFrame[rows] = float64(b.TotalAlloc-a.TotalAlloc) / frames / 1024
		fmt.Printf("rows=%6d ColumnTable %8.1f KB/frame\n", rows, perFrame[rows])
	}
	// The frame cost must not scale with the row count: 100x the rows may
	// cost at most 2x per frame (allowing noise), otherwise the table is
	// paying per data row.
	if base := perFrame[100]; perFrame[10000] > 2*base+8 {
		t.Errorf("frame cost scales with rows: %.1f KB/frame at 100 rows vs %.1f at 10000",
			base, perFrame[10000])
	}
}

func TestGioXListCost(t *testing.T) {
	th := newTestTheme()
	fmt.Printf("\n=== component.VScrollList vs material.List ===\n")
	for _, rows := range []int{100, 10000} {
		txt := uniqueStrings(rows, "GET /v1/resource/%d")
		for _, mode := range []string{"material.List", "VScrollList"} {
			var lst widget.List
			lst.Axis = layout.Vertical
			var r input.Router
			ops := new(op.Ops)
			now := time.Unix(1700000000, 0)
			sz := image.Pt(600, 700)

			frame := func() {
				ops.Reset()
				gtx := benchGtx(&r, ops, sz, now)
				el := func(gtx layout.Context, i int) layout.Dimensions {
					return material.Label(th, unit.Sp(12), txt[i]).Layout(gtx)
				}
				if mode == "VScrollList" {
					component.VScrollList(gtx, th, &lst, rows, el)
				} else {
					material.List(th, &lst).Layout(gtx, rows, el)
				}
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
			fmt.Printf("rows=%6d %-14s %8.1f KB/frame\n", rows, mode,
				float64(b.TotalAlloc-a.TotalAlloc)/frames/1024)
		}
	}
}
