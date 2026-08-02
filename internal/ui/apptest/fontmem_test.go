//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/nanorele/gio/font/opentype"

	. "tracto/internal/ui"
)

func TestFontParseCost(t *testing.T) {
	names := append([]string{
		"Inter-Regular.ttf",
		"Inter-Bold.ttf",
		"JetBrainsMono-Regular.ttf",
		"JetBrainsMono-Bold.ttf",
		"JetBrainsMono-Italic.ttf",
		"JetBrainsMono-BoldItalic.ttf",
		"NotoColorEmoji.ttf",
	}, FallbackFontFiles()...)

	type row struct {
		name    string
		raw     int
		parsed  float64
		faceMem float64
	}
	var rows []row
	var totalRaw int
	var totalParsed, totalFace float64

	for _, n := range names {
		b, err := LoadEmbeddedTTF(n)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		before := snap()
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatalf("%s: parse: %v", n, err)
		}
		after := snap()
		f := face.Face()
		afterFace := snap()

		rows = append(rows, row{
			name:    n,
			raw:     len(b),
			parsed:  mb(after.HeapAlloc - before.HeapAlloc),
			faceMem: mb(afterFace.HeapAlloc - after.HeapAlloc),
		})
		totalRaw += len(b)
		totalParsed += mb(after.HeapAlloc - before.HeapAlloc)
		totalFace += mb(afterFace.HeapAlloc - after.HeapAlloc)
		runtime.KeepAlive(f)
		runtime.KeepAlive(face)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].parsed > rows[j].parsed })
	fmt.Printf("\n=== per-font retained cost ===\n")
	fmt.Printf("%-34s %9s %9s %9s\n", "font", "raw MB", "parsed MB", "face MB")
	for _, r := range rows {
		fmt.Printf("%-34s %9.2f %9.2f %9.2f\n", r.name, float64(r.raw)/(1<<20), r.parsed, r.faceMem)
	}
	fmt.Printf("%-34s %9.2f %9.2f %9.2f\n", "TOTAL", float64(totalRaw)/(1<<20), totalParsed, totalFace)
}
