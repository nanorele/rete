//go:build membench

package workspace

import (
	"fmt"
	"image"
	"runtime"
	"testing"
	"time"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"tracto/internal/ui/settings"
	"tracto/pkg/syntax"
)

const streamBatch = 8 << 20

type timingRig struct {
	v      *ResponseViewer
	shaper *text.Shaper
	r      input.Router
	ops    *op.Ops
	size   image.Point
	now    time.Time
	wrap   bool
}

func newTimingRig(wrap bool) *timingRig {
	return &timingRig{
		v:      NewResponseViewer(),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		ops:    new(op.Ops),
		size:   image.Pt(500, 560),
		now:    time.Unix(1700000000, 0),
		wrap:   wrap,
	}
}

func (rig *timingRig) frame() time.Duration {
	start := time.Now()
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	ResponseViewerStyle{
		Viewer: rig.v, Shaper: rig.shaper, Font: font.Font{Typeface: "Go Mono"},
		TextSize: unit.Sp(13),
		Wrap:     rig.wrap, Padding: unit.Dp(4), Lang: syntax.LangJSON,
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
	rig.now = rig.now.Add(16 * time.Millisecond)
	return time.Since(start)
}

func (rig *timingRig) steadyFrame() time.Duration {
	best := time.Hour
	for i := 0; i < 5; i++ {
		if d := rig.frame(); d < best {
			best = d
		}
	}
	return best
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func TestLoadTiming(t *testing.T) {
	settings.SyntaxHighlightMaxMB = 200
	shapes := []struct {
		name string
		gen  func(int) string
	}{
		{"min", genMinifiedJSON},
		{"pretty", genPrettyJSON},
	}
	for _, sh := range shapes {
		for _, total := range []int{10 << 20, 50 << 20, 100 << 20} {
			body := sh.gen(total)
			for _, wrap := range []bool{true, false} {
				rig := newTimingRig(wrap)

				var appendTotal, frameDuringLoad time.Duration
				batches := 0
				for off := 0; off < len(body); off += streamBatch {
					end := off + streamBatch
					if end > len(body) {
						end = len(body)
					}
					t0 := time.Now()
					if off == 0 {
						rig.v.SetText(body[:end])
					} else {
						rig.v.Append(body[off:end])
					}
					appendTotal += time.Since(t0)
					batches++
					frameDuringLoad += rig.frame()
				}
				time.Sleep(tokenizeDebounce + 20*time.Millisecond)
				tokStart := time.Now()
				rig.frame()
				tokenize := time.Since(tokStart)

				top := rig.steadyFrame()

				t0 := time.Now()
				rig.v.moveCaret(len(rig.v.text), false)
				rig.v.ensureCaretVisible()
				jump := rig.frame() + time.Since(t0) - rig.frame()
				bottom := rig.steadyFrame()

				rig.v.SetScrollY(rig.v.lastTotalH / 2)
				mid := rig.steadyFrame()

				var ms2 runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&ms2)

				fmt.Printf("%-6s %3dMB wrap=%-5v lines=%8d  append=%6.0fms/%2db  load=%6.0fms tok=%6.0fms  top=%5.1f mid=%5.1f bot=%5.1f jump=%5.1f ms  heap=%6.1fMB  tokens=%9d cap=%9d (%6.1fMB)\n",
					sh.name, total>>20, wrap, len(rig.v.lineStarts),
					ms(appendTotal), batches, ms(frameDuringLoad), ms(tokenize),
					ms(top), ms(mid), ms(bottom), ms(jump), mb(ms2.HeapAlloc),
					len(rig.v.tokens), cap(rig.v.tokens), float64(cap(rig.v.tokens)*8)/(1<<20))
				runtime.KeepAlive(rig)
			}
		}
	}
}
