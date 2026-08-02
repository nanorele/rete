package apptest

import (
	"testing"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/text"
	"golang.org/x/image/math/fixed"

	. "tracto/internal/ui"
	"tracto/internal/ui/widgets"
)

type lazyProbe struct {
	shaper *text.Shaper
	loaded map[string]int
	eager  int
}

func newLazyProbe(t *testing.T) *lazyProbe {
	t.Helper()
	eager := AppFontCollection()
	p := &lazyProbe{loaded: map[string]int{}, eager: len(eager)}
	lazy := AppLazyFontFaces()
	wrapped := make([]text.LazyFace, len(lazy))
	for i, lf := range lazy {
		name := string(lf.Typeface)
		load := lf.Load
		wrapped[i] = text.LazyFace{
			Typeface: lf.Typeface,
			Ranges:   lf.Ranges,
			Load: func() (font.FontFace, error) {
				p.loaded[name]++
				return load()
			},
		}
	}
	p.shaper = text.NewShaper(
		text.NoSystemFonts(),
		text.WithCollection(eager),
		text.WithLazyCollection(wrapped),
	)
	return p
}

func (p *lazyProbe) shape(s string, dir system.TextDirection) (advance fixed.Int26_6, primaryGlyphs int) {
	p.shaper.LayoutString(text.Parameters{
		PxPerEm:  fixed.I(20),
		MaxWidth: 1 << 20,
		Locale:   system.Locale{Language: "en", Direction: dir},
		Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
	}, s)
	for {
		g, ok := p.shaper.NextGlyph()
		if !ok {
			break
		}
		if g.Advance == 0 && g.Runes == 0 {
			continue
		}
		advance += g.Advance
		if faceIdxFromGlyph(uint64(g.ID)) < p.eager {
			primaryGlyphs++
		}
	}
	return advance, primaryGlyphs
}

func TestLazyFontsStayUnloadedForLatinAndCyrillic(t *testing.T) {
	p := newLazyProbe(t)
	for _, s := range []string{
		`{"name":"item-1","ok":true,"score":12.5}`,
		"Привет, мир! Ёжик — тест.",
		"Grüße, naïve café — ½ ± 3°C",
		"ΑΒΓΔ αβγδ",
	} {
		if adv, _ := p.shape(s, system.LTR); adv == 0 {
			t.Fatalf("zero advance for %q", s)
		}
	}
	if len(p.loaded) != 0 {
		t.Fatalf("lazy faces loaded for Latin/Cyrillic/Greek text: %v", p.loaded)
	}
}

func TestLazyFontsLoadOnlyClaimedScript(t *testing.T) {
	cases := []struct {
		name string
		s    string
		dir  system.TextDirection
		want string
	}{
		{"Hebrew", "שלום", system.RTL, "Noto Sans Hebrew"},
		{"Arabic", "مرحبا", system.RTL, "Noto Sans Arabic"},
		{"Thai", "สวัสดี", system.LTR, "Noto Sans Thai"},
		{"Devanagari", "नमस्ते", system.LTR, "Noto Sans Devanagari"},
		{"Bengali", "ওহে", system.LTR, "Noto Sans Bengali"},
		{"Tamil", "வணக்கம்", system.LTR, "Noto Sans Tamil"},
		{"Telugu", "హలో", system.LTR, "Noto Sans Telugu"},
		{"Kannada", "ಹಲೋ", system.LTR, "Noto Sans Kannada"},
		{"Malayalam", "ഹലോ", system.LTR, "Noto Sans Malayalam"},
		{"Gujarati", "નમસ્તે", system.LTR, "Noto Sans Gujarati"},
		{"Gurmukhi", "ਸਤਿਸ੍ਰੀ", system.LTR, "Noto Sans Gurmukhi"},
		{"Sinhala", "ආයුබෝවන්", system.LTR, "Noto Sans Sinhala"},
		{"Georgian", "გამარჯობა", system.LTR, "Noto Sans Georgian"},
		{"Armenian", "Բարեւ", system.LTR, "Noto Sans Armenian"},
		{"Khmer", "សួស្តី", system.LTR, "Noto Sans Khmer"},
		{"Lao", "ສະບາຍດີ", system.LTR, "Noto Sans Lao"},
		{"Myanmar", "မင်္ဂလာပါ", system.LTR, "Noto Sans Myanmar"},
		{"Ethiopic", "ሰላም", system.LTR, "Noto Sans Ethiopic"},
		{"Han", "你好世界", system.LTR, "Noto Sans CJK SC"},
		{"Japanese", "こんにちは", system.LTR, "Noto Sans CJK SC"},
		{"Korean", "안녕하세요", system.LTR, "Noto Sans CJK SC"},
		{"Emoji", "🙂🚀🎉", system.LTR, "Noto Color Emoji"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newLazyProbe(t)
			adv, primary := p.shape(tc.s, tc.dir)
			if adv == 0 {
				t.Fatalf("%s: zero advance", tc.name)
			}
			if primary > 0 {
				t.Errorf("%s: %d glyph(s) fell back to a primary face (tofu)", tc.name, primary)
			}
			if p.loaded[tc.want] != 1 {
				t.Fatalf("%s: want %q loaded once, got %v", tc.name, tc.want, p.loaded)
			}
			if len(p.loaded) != 1 {
				t.Errorf("%s: extra faces loaded: %v", tc.name, p.loaded)
			}
		})
	}
}

func TestLazyFontsLoadAllWhenRuneIsUnclaimed(t *testing.T) {
	p := newLazyProbe(t)
	// Runic is claimed by no range and covered by no embedded face; the
	// safety net must still consult every deferred face before giving up.
	p.shape("ᚠᚢᚦ", system.LTR)
	if got, want := len(p.loaded), len(AppLazyFontFaces()); got != want {
		t.Fatalf("unclaimed rune loaded %d faces, want all %d: %v", got, want, p.loaded)
	}
}

func TestLazyFontsMatchEagerCoverage(t *testing.T) {
	eager := AppFontCollection()
	full := append([]font.FontFace{}, eager...)
	for _, lf := range AppLazyFontFaces() {
		ff, err := lf.Load()
		if err != nil {
			t.Fatalf("load %s: %v", lf.Typeface, err)
		}
		full = append(full, ff)
	}
	eagerShaper := text.NewShaper(text.NoSystemFonts(), text.WithCollection(full))

	samples := []struct {
		s   string
		dir system.TextDirection
	}{
		{"שלום", system.RTL}, {"مرحبا", system.RTL}, {"สวัสดี", system.LTR},
		{"नमस्ते", system.LTR}, {"你好世界", system.LTR}, {"안녕하세요", system.LTR},
		{"🙂🚀", system.LTR}, {"Привет", system.LTR}, {"hello", system.LTR},
	}
	for _, sm := range samples {
		p := newLazyProbe(t)
		lazyAdv, _ := p.shape(sm.s, sm.dir)

		var eagerAdv fixed.Int26_6
		eagerShaper.LayoutString(text.Parameters{
			PxPerEm:  fixed.I(20),
			MaxWidth: 1 << 20,
			Locale:   system.Locale{Language: "en", Direction: sm.dir},
			Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
		}, sm.s)
		for {
			g, ok := eagerShaper.NextGlyph()
			if !ok {
				break
			}
			if g.Advance == 0 && g.Runes == 0 {
				continue
			}
			eagerAdv += g.Advance
		}
		if lazyAdv != eagerAdv {
			t.Errorf("%q: lazy advance %v != eager advance %v", sm.s, lazyAdv, eagerAdv)
		}
	}
}
