package workspace

import (
	"fmt"
	"image"
	"runtime"
	"strings"
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

func genPrettyJSON(n int) string {
	var b strings.Builder
	b.Grow(n + 1024)
	b.WriteString("{\n  \"items\": [\n")
	i := 0
	for b.Len() < n {
		fmt.Fprintf(&b,
			"    {\"id\": %d, \"name\": \"item-%d\", \"email\": \"user%d@example.com\", \"active\": true, \"score\": %d.%02d, \"tags\": [\"alpha\", \"beta\", \"gamma\"], \"note\": null},\n",
			i, i, i, i%1000, i%100)
		i++
	}
	b.WriteString("    {\"id\": -1}\n  ]\n}\n")
	return b.String()
}

func genMinifiedJSON(n int) string {
	var b strings.Builder
	b.Grow(n + 1024)
	b.WriteString(`{"items":[`)
	i := 0
	for b.Len() < n {
		fmt.Fprintf(&b,
			`{"id":%d,"name":"item-%d","email":"user%d@example.com","active":true,"score":%d.%02d,"tags":["alpha","beta","gamma"],"note":null},`,
			i, i, i, i%1000, i%100)
		i++
	}
	b.WriteString(`{"id":-1}]}`)
	return b.String()
}

// TestSelectionChurn pins the per-frame allocation of the response viewer on
// a 10 MB body, with and without an active selection. The thresholds are 2x
// the values measured after the G1-G3 fixes (see task_giox.md): a regression
// to the pre-fix numbers (417-1545 KB/frame) fails loudly, while normal
// run-to-run noise does not. TotalAlloc per frame is deterministic, unlike
// frame time, so this is safe for CI.
func TestSelectionChurn(t *testing.T) {
	prevMax := settings.SyntaxHighlightMaxMB
	settings.SyntaxHighlightMaxMB = 200
	defer func() { settings.SyntaxHighlightMaxMB = prevMax }()

	// KB/frame ceilings per (generator, wrap), worst selection case.
	limits := map[string]float64{
		"pretty/wrap=true":  125,
		"pretty/wrap=false": 235,
		"min/wrap=true":     285,
		"min/wrap=false":    10,
	}

	for _, gen := range []struct {
		name string
		fn   func(int) string
	}{{"pretty", genPrettyJSON}, {"min", genMinifiedJSON}} {
		body := gen.fn(10 << 20)
		for _, wrap := range []bool{true, false} {
			for _, sel := range []int{0, 20000} {
				v := NewResponseViewer()
				v.SetText(body)
				shaper := text.NewShaper(text.WithCollection(gofont.Collection()))
				var r input.Router
				ops := new(op.Ops)
				now := time.Unix(1700000000, 0)
				sz := image.Pt(900, 700)

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
						Viewer: v, Shaper: shaper, Font: font.Font{Typeface: "Go Mono"},
						TextSize: unit.Sp(13), Wrap: wrap, Padding: unit.Dp(4),
						Lang: syntax.LangJSON,
					}.Layout(gtx)
					r.Frame(ops)
					now = now.Add(16 * time.Millisecond)
				}

				for i := 0; i < 3; i++ {
					frame()
				}
				time.Sleep(tokenizeDebounce + 20*time.Millisecond)
				if sel > 0 {
					v.selStart, v.selEnd = 0, sel
				}
				for i := 0; i < 5; i++ {
					frame()
				}
				var a, b runtime.MemStats
				runtime.ReadMemStats(&a)
				const frames = 30
				for i := 0; i < frames; i++ {
					frame()
				}
				runtime.ReadMemStats(&b)
				perFrameKB := float64(b.TotalAlloc-a.TotalAlloc) / frames / 1024

				key := fmt.Sprintf("%s/wrap=%v", gen.name, wrap)
				t.Logf("%-7s wrap=%-5v selection=%6d bytes  %10.1f KB/frame (limit %.0f)",
					gen.name, wrap, sel, perFrameKB, limits[key])
				if perFrameKB > limits[key] {
					t.Errorf("%s selection=%d: %.1f KB/frame exceeds %.0f KB/frame",
						key, sel, perFrameKB, limits[key])
				}
				runtime.KeepAlive(v)
			}
		}
	}
}
