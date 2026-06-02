package ui

import (
	"testing"
	"tracto/internal/ui/fontsubset"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/text"
	"golang.org/x/image/math/fixed"
)

func buildFullShaper(t *testing.T) (*text.Shaper, int) {
	var fonts []font.FontFace

	parse := func(b []byte) opentype.Face {
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return face
	}
	subset := func(name string) []byte {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		s, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		return s
	}
	addUI := func(name string) {
		face := parse(subset(name))
		fonts = append(fonts, font.FontFace{Font: face.Font(), Face: face})
	}
	addJBM := func(name string, style font.Style, weight font.Weight) {
		face := parse(subset(name))
		fn := font.Font{Typeface: widgets.MonoFamilyName, Style: style, Weight: weight}
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
	}

	addUI("Inter-Regular.ttf")
	addUI("Inter-Bold.ttf")
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)

	emoji := parse(mustEmbed(t, "NotoColorEmoji.ttf"))
	fonts = append(fonts, font.FontFace{Font: emoji.Font(), Face: emoji})

	firstFallback := len(fonts)
	for _, name := range fallbackFontFiles {
		face := parse(mustEmbed(t, name))
		fonts = append(fonts, font.FontFace{Font: face.Font(), Face: face})
	}

	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(fonts)), firstFallback
}

func mustEmbed(t *testing.T, name string) []byte {
	b, err := loadEmbeddedTTF(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return b
}

func TestScriptFallbackCoverage(t *testing.T) {
	shaper, firstFallback := buildFullShaper(t)

	cases := []struct {
		name string
		s    string
		dir  system.TextDirection
	}{
		{"Hebrew", "שלום", system.RTL},
		{"Arabic", "مرحبا", system.RTL},
		{"Thai", "สวัสดี", system.LTR},
		{"Devanagari", "नमस्ते", system.LTR},
		{"Bengali", "ওহে", system.LTR},
		{"Tamil", "வணக்கம்", system.LTR},
		{"Telugu", "హలో", system.LTR},
		{"Kannada", "ಹಲೋ", system.LTR},
		{"Malayalam", "ഹലോ", system.LTR},
		{"Gujarati", "નમસ્તે", system.LTR},
		{"Gurmukhi", "ਸਤਿਸ੍ਰੀ", system.LTR},
		{"Sinhala", "ආයුබෝවන්", system.LTR},
		{"Georgian", "გამარჯობა", system.LTR},
		{"Armenian", "Բարեւ", system.LTR},
		{"Khmer", "សួស្តី", system.LTR},
		{"Lao", "ສະບາຍດີ", system.LTR},
		{"Myanmar", "မင်္ဂလာပါ", system.LTR},
		{"Ethiopic", "ሰላም", system.LTR},
		{"Han", "你好世界", system.LTR},
		{"Japanese", "こんにちは", system.LTR},
		{"Korean", "안녕하세요", system.LTR},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shaper.LayoutString(text.Parameters{
				PxPerEm:  fixed.I(20),
				MaxWidth: 1 << 20,
				Locale:   system.Locale{Language: "en", Direction: tc.dir},
				Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
			}, tc.s)

			var adv fixed.Int26_6
			seen := map[int]int{}
			primary := 0
			for {
				g, ok := shaper.NextGlyph()
				if !ok {
					break
				}
				if g.Advance == 0 && g.Runes == 0 {
					continue
				}
				adv += g.Advance
				idx := faceIdxFromGlyph(uint64(g.ID))
				seen[idx]++
				if idx < firstFallback {
					primary++
				}
			}
			t.Logf("%-12s faces=%v advance=%v", tc.name, seen, adv)
			if adv == 0 {
				t.Fatalf("%s: zero advance", tc.name)
			}
			if primary > 0 {
				t.Errorf("%s: %d glyph(s) resolved to primary faces (idx<%d) = tofu/.notdef; script not covered: %v",
					tc.name, primary, firstFallback, seen)
			}
		})
	}
}
