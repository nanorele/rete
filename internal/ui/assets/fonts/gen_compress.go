//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andybalholm/brotli"
)

func main() {
	dir := filepath.Join("internal", "ui", "assets", "fonts", "ttf")
	entries, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	var rawTotal, brTotal int
	for _, e := range entries {
		name := e.Name()
		switch filepath.Ext(name) {
		case ".ttf", ".otf":
		default:
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			panic(err)
		}
		var buf bytes.Buffer
		w := brotli.NewWriterOptions(&buf, brotli.WriterOptions{Quality: 11, LGWin: 24})
		if _, err := w.Write(raw); err != nil {
			panic(err)
		}
		if err := w.Close(); err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".br"), buf.Bytes(), 0o644); err != nil {
			panic(err)
		}
		rawTotal += len(raw)
		brTotal += buf.Len()
		fmt.Printf("%-32s %8d -> %8d  (%.1f%%)\n", name, len(raw), buf.Len(), 100*float64(buf.Len())/float64(len(raw)))
	}
	fmt.Printf("%-32s %8d -> %8d  (%.1f%%)\n", "TOTAL", rawTotal, brTotal, 100*float64(brTotal)/float64(rawTotal))
}
