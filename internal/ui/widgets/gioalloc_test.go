//go:build membench

package widgets

import (
	"fmt"
	"image"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func heap() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestGioRetainedCost(t *testing.T) {
	const n = 2000
	fmt.Printf("\n=== retained cost of idle widget state ===\n")
	fmt.Printf("sizeof(widget.Editor)    = %d B\n", unsafe.Sizeof(widget.Editor{}))
	fmt.Printf("sizeof(widget.Clickable) = %d B\n", unsafe.Sizeof(widget.Clickable{}))
	fmt.Printf("sizeof(widget.List)      = %d B\n", unsafe.Sizeof(widget.List{}))
	fmt.Printf("sizeof(widget.Bool)      = %d B\n", unsafe.Sizeof(widget.Bool{}))

	a := heap()
	eds := make([]widget.Editor, n)
	b := heap()
	for i := range eds {
		eds[i].SingleLine = true
		eds[i].SetText(fmt.Sprintf("value-%d", i))
	}
	c := heap()

	th := newTestTheme()
	var r input.Router
	ops := new(op.Ops)
	now := time.Unix(1700000000, 0)
	for f := 0; f < 3; f++ {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 800)),
			Source:      r.Source(),
			Now:         now,
		}
		for i := range eds {
			macro := op.Record(gtx.Ops)
			material.Editor(th, &eds[i], "").Layout(gtx)
			macro.Stop()
		}
		r.Frame(ops)
		now = now.Add(16 * time.Millisecond)
	}
	d := heap()

	fmt.Printf("%d editors, zero value   +%7.2fMB (%6.0f B each)\n", n, float64(b-a)/(1<<20), float64(b-a)/n)
	fmt.Printf("  after SetText          +%7.2fMB (%6.0f B each)\n", float64(c-b)/(1<<20), float64(c-b)/n)
	fmt.Printf("  after 3 layouts        +%7.2fMB (%6.0f B each)\n", float64(d-c)/(1<<20), float64(d-c)/n)
	fmt.Printf("  total                   %7.2fMB (%6.0f B each)\n", float64(d-a)/(1<<20), float64(d-a)/n)
	runtime.KeepAlive(eds)
}

func TestGioOpsHighWater(t *testing.T) {
	th := newTestTheme()
	var r input.Router
	ops := new(op.Ops)
	now := time.Unix(1700000000, 0)
	txt := uniqueStrings(4000, "GET /v1/resource/%d")

	frame := func(rows int) {
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(1200, 800)),
			Source:      r.Source(),
			Now:         now,
		}
		for i := 0; i < rows; i++ {
			macro := op.Record(gtx.Ops)
			material.Label(th, unit.Sp(12), txt[i]).Layout(gtx)
			macro.Stop()
		}
		r.Frame(ops)
		now = now.Add(16 * time.Millisecond)
	}

	base := heap()
	for i := 0; i < 3; i++ {
		frame(20)
	}
	small := heap()
	for i := 0; i < 3; i++ {
		frame(4000)
	}
	big := heap()
	for i := 0; i < 10; i++ {
		frame(20)
	}
	after := heap()

	fmt.Printf("\n=== op.Ops / router high-water mark ===\n")
	fmt.Printf("20-row frames      %7.2fMB\n", float64(small-base)/(1<<20))
	fmt.Printf("after 4000-row     %7.2fMB\n", float64(big-base)/(1<<20))
	fmt.Printf("back to 20-row     %7.2fMB  (not reclaimed: %.2fMB)\n",
		float64(after-base)/(1<<20), float64(after-small)/(1<<20))
	runtime.KeepAlive(ops)
}
