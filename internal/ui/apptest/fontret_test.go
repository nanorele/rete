//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/nanorele/gio/font/opentype"

	. "tracto/internal/ui"
)

func TestFontRawRetention(t *testing.T) {
	names := []string{
		"Inter-Regular.ttf",
		"JetBrainsMono-Regular.ttf",
		"NotoColorEmoji.ttf",
		"NotoSansCJK-Regular.otf",
	}

	fmt.Printf("\n=== raw byte retention after Parse ===\n")
	fmt.Printf("%-30s %8s %10s %10s\n", "font", "raw MB", "total MB", "leaked raw")
	for _, n := range names {
		before := snap()
		b, err := LoadEmbeddedTTF(n)
		if err != nil {
			t.Fatal(err)
		}
		raw := float64(len(b)) / (1 << 20)
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatal(err)
		}
		f := face.Face()
		b = nil
		after := snap()
		total := mb(after.HeapAlloc - before.HeapAlloc)
		fmt.Printf("%-30s %8.2f %10.2f %10s\n", n, raw, total,
			map[bool]string{true: "YES", false: "no"}[total > raw*0.8])
		runtime.KeepAlive(f)
		runtime.KeepAlive(face)
	}
}
