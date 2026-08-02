//go:build membench

package apptest

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/text"
	"golang.org/x/image/math/fixed"

	. "tracto/internal/ui"
	"tracto/internal/ui/widgets"
)

func TestLazyResolveCost(t *testing.T) {
	bodies := make([]string, 20)
	for j := range bodies {
		var b strings.Builder
		for i := 0; i < 400; i++ {
			fmt.Fprintf(&b, `{"id":%d,"name":"item-%d-%d","email":"user%d@example.com"},`, i, i, j, i)
		}
		bodies[j] = b.String()
	}
	body := bodies[0]

	run := func(sh *text.Shaper) time.Duration {
		best := time.Hour
		for i := 0; i < 20; i++ {
			start := time.Now()
			sh.LayoutString(text.Parameters{
				PxPerEm:  fixed.I(20),
				MaxWidth: 1 << 20,
				Locale:   system.Locale{Language: "en", Direction: system.LTR},
				Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
			}, bodies[i])
			for {
				if _, ok := sh.NextGlyph(); !ok {
					break
				}
			}
			if d := time.Since(start); d < best {
				best = d
			}
		}
		return best
	}

	eager := AppFontCollection()
	full := append([]font.FontFace{}, eager...)
	for _, lf := range AppLazyFontFaces() {
		ff, err := lf.Load()
		if err != nil {
			t.Fatal(err)
		}
		full = append(full, ff)
	}

	withLazy := run(text.NewShaper(text.NoSystemFonts(),
		text.WithCollection(eager), text.WithLazyCollection(AppLazyFontFaces())))
	withEager := run(text.NewShaper(text.NoSystemFonts(), text.WithCollection(full)))

	fmt.Printf("\n=== per-rune resolve cost (%d runes) ===\n", len(body))
	fmt.Printf("eager collection : %8.3fms\n", float64(withEager.Microseconds())/1000)
	fmt.Printf("lazy  collection : %8.3fms  (%+.1f%%)\n",
		float64(withLazy.Microseconds())/1000,
		100*(float64(withLazy)/float64(withEager)-1))
}
