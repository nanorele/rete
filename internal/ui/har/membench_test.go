//go:build membench

package har

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"testing"
)

func benchMB(v uint64) float64 { return float64(v) / (1 << 20) }

func benchChurn(rig *harRig, frames int) float64 {
	rig.frames(5)
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	rig.frames(frames)
	runtime.ReadMemStats(&b)
	return float64(b.TotalAlloc-a.TotalAlloc) / float64(frames) / (1 << 10)
}

func genHAR(entries, bodySize int) string {
	var b strings.Builder
	b.Grow(entries * (bodySize + 512))
	b.WriteString(`{"log":{"version":"1.2","creator":{"name":"bench","version":"1"},"entries":[`)
	payload := strings.Repeat("x", bodySize)
	for i := 0; i < entries; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"startedDateTime":"2024-01-01T10:00:00Z","time":12.5,`+
			`"request":{"method":"GET","url":"https://api%d.example.com/v1/resource/%d?q=1",`+
			`"headers":[{"name":"Accept","value":"*/*"},{"name":"User-Agent","value":"bench"}]},`+
			`"response":{"status":200,"statusText":"OK",`+
			`"headers":[{"name":"Content-Type","value":"application/json"}],`+
			`"content":{"mimeType":"application/json","size":%d,"text":"%s"}}}`,
			i%13, i, bodySize, payload)
	}
	b.WriteString(`]}}`)
	return b.String()
}

func TestHARChurn(t *testing.T) {
	fmt.Printf("\n=== HAR section churn ===\n")
	for _, c := range []struct {
		entries  int
		body     int
		selected bool
	}{
		{0, 0, false},
		{200, 512, false},
		{200, 512, true},
		{2000, 512, true},
		{200, 512 << 10, true},
	} {
		doc := ""
		if c.entries > 0 {
			doc = genHAR(c.entries, c.body)
		}
		rig := newRig(t, doc, image.Pt(1280, 720))
		if c.selected && rig.s.Doc != nil {
			rig.s.SelReq = c.entries / 2
		}
		rig.frames(3)
		runtime.GC()
		var live runtime.MemStats
		runtime.ReadMemStats(&live)
		churn := benchChurn(rig, 40)
		fmt.Printf("entries=%4d body=%7d selected=%-5v  churn=%9.1f KB/frame  heap=%7.1fMB  doc=%6.1fMB\n",
			c.entries, c.body, c.selected, churn, benchMB(live.HeapAlloc), float64(len(doc))/(1<<20))
		runtime.KeepAlive(rig)
	}
}
