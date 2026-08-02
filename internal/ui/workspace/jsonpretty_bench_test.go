//go:build jsonbench

package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func genNestedArrayJSON(n int) []byte {
	var b strings.Builder
	b.Grow(n + 4096)
	b.WriteByte('[')
	group := 0
	for b.Len() < n {
		if group > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"group":%d,"label":"group-%d","meta":{"created":"2024-03-1%dT08:2%d:11Z","tags":["alpha","beta","gamma-%d"],"flags":{"active":true,"beta":false,"score":%d.%02d}},"rows":[`,
			group, group, group%10, group%6, group%97, group%1000, group%100)
		for r := 0; r < 8; r++ {
			if r > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `{"id":%d,"name":"item-%d-%d","email":"user%d@example.com","path":["a","b\"c","d\\e"],"values":[%d,%d.%d,null,true],"nested":{"deep":{"deeper":{"leaf":"value-%d"}}}}`,
				group*8+r, group, r, group*8+r, r, r, group%10, group*8+r)
		}
		b.WriteString(`]}`)
		group++
	}
	b.WriteByte(']')
	return []byte(b.String())
}

func benchSizes() []int { return []int{10 << 20, 50 << 20, 100 << 20} }

func timeBest(n int, fn func() int) (time.Duration, int) {
	best := time.Hour
	out := 0
	for i := 0; i < n; i++ {
		runtime.GC()
		t0 := time.Now()
		out = fn()
		if d := time.Since(t0); d < best {
			best = d
		}
	}
	return best, out
}

func report(label string, in, out int, d time.Duration) {
	fmt.Printf("%-10s in=%4dMB out=%4dMB  %10.3fms  %7.0f MB/s\n",
		label, in>>20, out>>20, float64(d.Nanoseconds())/1e6,
		float64(in)/(1<<20)/d.Seconds())
}

func TestJSONPrettyTiming(t *testing.T) {
	for _, size := range benchSizes() {
		src := genNestedArrayJSON(size)

		dPar, outLen := timeBest(3, func() int {
			return len(formatJSON(src, &JSONFormatterState{}))
		})
		report("parallel", len(src), outLen, dPar)

		dSer, _ := timeBest(3, func() int {
			return len(appendFormatJSON(make([]byte, 0, len(src)*3), src, &JSONFormatterState{}))
		})
		report("serial", len(src), outLen, dSer)

		dAlloc, _ := timeBest(3, func() int {
			b := make([]byte, outLen)
			runtime.KeepAlive(b)
			return len(b)
		})
		report("alloc only", len(src), outLen, dAlloc)
		fmt.Println()
	}
}

func TestJSONPreviewPipelineTiming(t *testing.T) {
	for _, size := range benchSizes() {
		src := genNestedArrayJSON(size)
		path := filepath.Join(t.TempDir(), "resp.json")
		if err := os.WriteFile(path, src, 0o644); err != nil {
			t.Fatal(err)
		}

		st := &JSONFormatterState{}
		t0 := time.Now()
		first, loaded, isJSON := loadPreviewFromFile(path, int64(len(src)), st, "application/json")
		dFirst := time.Since(t0)
		if !isJSON {
			t.Fatalf("expected JSON detection")
		}

		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t1 := time.Now()
		out := int64(len(first))
		buf := make([]byte, previewBatchSize)
		for off := loaded; off < int64(len(src)); {
			_, _ = f.Seek(off, io.SeekStart)
			n, _ := io.ReadFull(f, buf)
			if n <= 0 {
				break
			}
			out += int64(len(formatJSON(buf[:n], st)))
			off += int64(n)
		}
		dRest := time.Since(t1)
		_ = f.Close()

		fmt.Printf("preview  %4dMB  first %dMB batch %7.1fms   remaining %7.1fms   total pretty %4dMB\n",
			len(src)>>20, previewBatchSize>>20,
			float64(dFirst.Nanoseconds())/1e6, float64(dRest.Nanoseconds())/1e6, out>>20)
	}
}

func BenchmarkFormatJSON(b *testing.B) {
	for _, size := range benchSizes() {
		src := genNestedArrayJSON(size)
		b.Run(fmt.Sprintf("%dMB", size>>20), func(b *testing.B) {
			b.SetBytes(int64(len(src)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				out := formatJSON(src, &JSONFormatterState{})
				runtime.KeepAlive(out)
			}
		})
	}
}
