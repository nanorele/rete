//go:build membench

package workspace

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"image"

	"tracto/internal/ui/settings"
	"tracto/pkg/syntax"
)

const benchTarget = 10 << 20

type memSample struct {
	heapAlloc  uint64
	heapSys    uint64
	sys        uint64
	totalAlloc uint64
	mallocs    uint64
}

func sampleMem() memSample {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return memSample{
		heapAlloc:  ms.HeapAlloc,
		heapSys:    ms.HeapSys,
		sys:        ms.Sys,
		totalAlloc: ms.TotalAlloc,
		mallocs:    ms.Mallocs,
	}
}

func mb(v uint64) float64 { return float64(v) / (1 << 20) }

type benchRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	resp   *ResponseViewer
	req    *RequestEditor
	size   image.Point
	wrap   bool
	lang   syntax.Lang
	now    time.Time
}

func newBenchRig(wrap bool, lang syntax.Lang) *benchRig {
	return &benchRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		resp:   NewResponseViewer(),
		req:    NewRequestEditor(),
		size:   image.Pt(900, 700),
		wrap:   wrap,
		lang:   lang,
		now:    time.Unix(1700000000, 0),
	}
}

func (rig *benchRig) gtx() layout.Context {
	rig.ops.Reset()
	return layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         rig.now,
		Source:      rig.r.Source(),
	}
}

func (rig *benchRig) frameResp() {
	gtx := rig.gtx()
	ResponseViewerStyle{
		Viewer:   rig.resp,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     rig.wrap,
		Padding:  unit.Dp(4),
		Lang:     rig.lang,
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
	rig.now = rig.now.Add(16 * time.Millisecond)
}

func (rig *benchRig) frameReq() {
	gtx := rig.gtx()
	RequestEditorStyle{
		Viewer:   rig.req,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     rig.wrap,
		Padding:  unit.Dp(4),
		Lang:     rig.lang,
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
	rig.now = rig.now.Add(16 * time.Millisecond)
}

func settleTokens(t *testing.T, frame func()) {
	t.Helper()
	for i := 0; i < 3; i++ {
		frame()
	}
	time.Sleep(tokenizeDebounce + 20*time.Millisecond)
	for i := 0; i < 3; i++ {
		frame()
	}
}

func report(t *testing.T, name string, base, after memSample, frames int) {
	t.Helper()
	fmt.Printf("%-46s heap=%7.1fMB heapSys=%7.1fMB sys=%7.1fMB  churn/frame=%7.2fMB  allocs/frame=%d\n",
		name,
		mb(after.heapAlloc), mb(after.heapSys), mb(after.sys),
		mb(after.totalAlloc-base.totalAlloc)/float64(frames),
		(after.mallocs-base.mallocs)/uint64(frames),
	)
}

func scrollSweep(rig *benchRig, v *textCore, frame func(), steps int) {
	total := v.lastTotalH
	if total <= 0 {
		total = 1
	}
	for i := 0; i < steps; i++ {
		v.SetScrollY(total * i / steps)
		frame()
	}
}

func TestMemBenchResponse(t *testing.T) {
	settings.SyntaxHighlightMaxMB = 100
	cases := []struct {
		id   string
		name string
		wrap bool
		lang syntax.Lang
		text func(int) string
	}{
		{"resp_pretty_wrapON_json", "resp pretty  wrap=on   lang=json ", true, syntax.LangJSON, genPrettyJSON},
		{"resp_pretty_wrapOFF_json", "resp pretty  wrap=off  lang=json ", false, syntax.LangJSON, genPrettyJSON},
		{"resp_pretty_wrapON_plain", "resp pretty  wrap=on   lang=plain", true, syntax.LangPlain, genPrettyJSON},
		{"resp_min_wrapON_json", "resp minified wrap=on  lang=json ", true, syntax.LangJSON, genMinifiedJSON},
		{"resp_min_wrapOFF_json", "resp minified wrap=off lang=json ", false, syntax.LangJSON, genMinifiedJSON},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			body := c.text(benchTarget)
			rig := newBenchRig(c.wrap, c.lang)
			empty := sampleMem()
			rig.resp.SetText(body)
			afterSet := sampleMem()
			settleTokens(t, rig.frameResp)
			afterTok := sampleMem()

			const steps = 40
			before := sampleMem()
			scrollSweep(rig, &rig.resp.textCore, rig.frameResp, steps)
			after := sampleMem()

			fmt.Printf("%s bytes=%dMB lines=%d tokens=%d\n", c.name, len(body)>>20, len(rig.resp.lineStarts), len(rig.resp.tokens))
			fmt.Printf("   empty heap=%.1fMB  afterSetText=%.1fMB  afterTokenize=%.1fMB\n",
				mb(empty.heapAlloc), mb(afterSet.heapAlloc), mb(afterTok.heapAlloc))
			report(t, "   scrolling: ", before, after, steps)
			runtime.KeepAlive(rig)
		})
	}
}

func TestMemBenchRequest(t *testing.T) {
	settings.SyntaxHighlightMaxMB = 100
	cases := []struct {
		id   string
		name string
		wrap bool
		lang syntax.Lang
		text func(int) string
	}{
		{"req_pretty_wrapON_json", "req pretty  wrap=on   lang=json ", true, syntax.LangJSON, genPrettyJSON},
		{"req_pretty_wrapOFF_json", "req pretty  wrap=off  lang=json ", false, syntax.LangJSON, genPrettyJSON},
		{"req_min_wrapON_json", "req minified wrap=on  lang=json ", true, syntax.LangJSON, genMinifiedJSON},
		{"req_min_wrapOFF_json", "req minified wrap=off lang=json ", false, syntax.LangJSON, genMinifiedJSON},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			body := c.text(benchTarget)
			rig := newBenchRig(c.wrap, c.lang)
			empty := sampleMem()
			if !rig.req.SetText(body) {
				t.Fatalf("SetText rejected %d bytes", len(body))
			}
			afterSet := sampleMem()
			settleTokens(t, rig.frameReq)
			afterTok := sampleMem()

			const steps = 40
			before := sampleMem()
			scrollSweep(rig, &rig.req.textCore, rig.frameReq, steps)
			after := sampleMem()

			fmt.Printf("%s bytes=%dMB lines=%d tokens=%d\n", c.name, len(body)>>20, len(rig.req.lineStarts), len(rig.req.tokens))
			fmt.Printf("   empty heap=%.1fMB  afterSetText=%.1fMB  afterTokenize=%.1fMB\n",
				mb(empty.heapAlloc), mb(afterSet.heapAlloc), mb(afterTok.heapAlloc))
			report(t, "   scrolling: ", before, after, steps)
			runtime.KeepAlive(rig)
		})
	}
}

func TestMemBenchProfile(t *testing.T) {
	settings.SyntaxHighlightMaxMB = 100
	out := os.Getenv("MEMBENCH_OUT")
	if out == "" {
		t.Skip("set MEMBENCH_OUT to write heap profile")
	}
	wrap := os.Getenv("MEMBENCH_WRAP") != "0"
	minified := os.Getenv("MEMBENCH_MIN") == "1"
	gen := genPrettyJSON
	if minified {
		gen = genMinifiedJSON
	}
	body := gen(benchTarget)
	rig := newBenchRig(wrap, syntax.LangJSON)
	rig.resp.SetText(body)
	rig.req.SetText(body)
	settleTokens(t, rig.frameResp)
	settleTokens(t, rig.frameReq)

	const steps = 40
	scrollSweep(rig, &rig.resp.textCore, rig.frameResp, steps)
	scrollSweep(rig, &rig.req.textCore, rig.frameReq, steps)

	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	runtime.GC()
	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatal(err)
	}
	m := sampleMem()
	fmt.Printf("profile written: heap=%.1fMB heapSys=%.1fMB sys=%.1fMB\n", mb(m.heapAlloc), mb(m.heapSys), mb(m.sys))
	runtime.KeepAlive(rig)
}
