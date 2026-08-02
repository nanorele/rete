//go:build membench

package har

import (
	"fmt"
	"runtime"
	"testing"

	"tracto/internal/har"
)

func harSnap() uint64 {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

func TestHARLoadRetention(t *testing.T) {
	const entries, bodySize = 400, 256 << 10
	doc := genHAR(entries, bodySize)
	raw := []byte(doc)
	srcMB := float64(len(raw)) / (1 << 20)

	base := harSnap()
	parsed, err := har.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	afterParse := harSnap()

	st := &Section{}
	st.Ensure()
	st.ApplyLoad(raw, "bench.har", nil)
	afterApply := harSnap()

	res := sortedResources(parsed)
	afterResources := harSnap()

	rows := buildRowCache(parsed.Entries)
	afterRows := harSnap()

	fmt.Printf("\n=== HAR retention (source %.1fMB, %d entries) ===\n", srcMB, entries)
	fmt.Printf("har.Parse            +%7.1fMB  (%.2fx source)\n",
		float64(afterParse-base)/(1<<20), float64(afterParse-base)/float64(len(raw)))
	fmt.Printf("ApplyLoad (2nd copy) +%7.1fMB\n", float64(afterApply-afterParse)/(1<<20))
	fmt.Printf("sortedResources      +%7.1fMB  (%d files)\n", float64(afterResources-afterApply)/(1<<20), len(res))
	fmt.Printf("buildRowCache        +%7.1fMB  (%d rows)\n", float64(afterRows-afterResources)/(1<<20), len(rows))
	fmt.Printf("total resident        %7.1fMB\n", float64(afterRows)/(1<<20))

	runtime.KeepAlive(parsed)
	runtime.KeepAlive(st)
	runtime.KeepAlive(res)
	runtime.KeepAlive(rows)
	runtime.KeepAlive(raw)
}
