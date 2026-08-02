//go:build membench

package apptest

import (
	"fmt"
	"image"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"

	. "tracto/internal/ui"
	"tracto/internal/ui/workspace"
)

func mb(v uint64) float64 { return float64(v) / (1 << 20) }

func snap() runtime.MemStats {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms
}

type idleRig struct {
	ui  *AppUI
	r   input.Router
	ops *op.Ops
	sz  image.Point
	now time.Time
}

func newIdleRig(t *testing.T) *idleRig {
	t.Helper()
	setupTestConfigDir(t)
	ui := NewAppUI()
	ui.Window = new(app.Window)
	return &idleRig{
		ui:  ui,
		ops: new(op.Ops),
		sz:  image.Pt(1280, 720),
		now: time.Unix(1700000000, 0),
	}
}

func (rig *idleRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
	rig.ui.LayoutApp(gtx)
	rig.r.Frame(rig.ops)
	rig.now = rig.now.Add(16 * time.Millisecond)
}

func (rig *idleRig) churn(frames int) (perFrame float64, opsBytes int) {
	for i := 0; i < 5; i++ {
		rig.frame()
	}
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < frames; i++ {
		rig.frame()
	}
	runtime.ReadMemStats(&b)
	return float64(b.TotalAlloc-a.TotalAlloc) / float64(frames) / (1 << 10), 0
}

func TestIdleAppMemory(t *testing.T) {
	base := snap()

	rig := newIdleRig(t)
	built := snap()

	rig.frame()
	first := snap()

	for i := 0; i < 30; i++ {
		rig.frame()
	}
	warm := snap()

	perFrame, _ := rig.churn(60)

	fmt.Printf("\n=== idle app (empty state, 1280x720) ===\n")
	fmt.Printf("base            heap=%7.2fMB sys=%7.2fMB\n", mb(base.HeapAlloc), mb(base.Sys))
	fmt.Printf("after NewAppUI  heap=%7.2fMB sys=%7.2fMB  (+%.2fMB)\n", mb(built.HeapAlloc), mb(built.Sys), mb(built.HeapAlloc-base.HeapAlloc))
	fmt.Printf("after 1 frame   heap=%7.2fMB sys=%7.2fMB  (+%.2fMB)\n", mb(first.HeapAlloc), mb(first.Sys), mb(first.HeapAlloc-built.HeapAlloc))
	fmt.Printf("after 31 frames heap=%7.2fMB sys=%7.2fMB  (+%.2fMB)\n", mb(warm.HeapAlloc), mb(warm.Sys), mb(warm.HeapAlloc-first.HeapAlloc))
	fmt.Printf("steady churn    %.1f KB/frame\n", perFrame)

	if out := os.Getenv("IDLE_HEAP_PROFILE"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		runtime.GC()
		if err := pprof.WriteHeapProfile(f); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		fmt.Printf("heap profile -> %s\n", out)
	}
	runtime.KeepAlive(rig)
}

func TestIdleSectionMemory(t *testing.T) {
	sections := []string{"requests", "flows", "mitm", "har", "netlimit"}
	for _, sec := range sections {
		func() {
			rig := newIdleRig(t)
			before := snap()
			rig.ui.SetSidebarSection(sec)
			for i := 0; i < 20; i++ {
				rig.frame()
			}
			after := snap()
			perFrame, _ := rig.churn(60)
			fmt.Printf("section %-9s resident=%7.2fMB (+%.2fMB over idle) churn=%6.1f KB/frame\n",
				sec, mb(after.HeapAlloc), mb(after.HeapAlloc-before.HeapAlloc), perFrame)
			runtime.KeepAlive(rig)
		}()
	}
}

func TestTabScaling(t *testing.T) {
	rig := newIdleRig(t)
	for i := 0; i < 10; i++ {
		rig.frame()
	}
	one := snap()
	base := len(rig.ui.Tabs)

	for i := 0; i < 20; i++ {
		rig.ui.Tabs = append(rig.ui.Tabs, workspace.NewRequestTab("New request"))
	}
	for i := 0; i < 10; i++ {
		rig.frame()
	}
	many := snap()

	n := len(rig.ui.Tabs) - base
	fmt.Printf("\n=== tabs ===\n")
	fmt.Printf("%d tab(s)  heap=%7.2fMB\n", base, mb(one.HeapAlloc))
	fmt.Printf("+%d tabs   heap=%7.2fMB  => %.0f KB per empty tab\n",
		n, mb(many.HeapAlloc), float64(many.HeapAlloc-one.HeapAlloc)/float64(n)/1024)
	runtime.KeepAlive(rig)
}
