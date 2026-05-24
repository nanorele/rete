package ui

import (
	"fmt"
	"testing"
	"tracto/internal/ui/fontsubset"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/io/system"
	"github.com/nanorele/gio/text"
	"golang.org/x/image/math/fixed"
)

func TestEmojiFontMetadata(t *testing.T) {
	face, err := opentype.Parse(fontNotoColorEmoji)
	if err != nil {
		t.Fatalf("parse NotoColorEmoji: %v", err)
	}
	fnt := face.Font()
	t.Logf("Family: %q  Style: %v  Weight: %v", fnt.Typeface, fnt.Style, fnt.Weight)
	if fnt.Typeface != widgets.EmojiTypeface {
		t.Errorf("emoji font typeface = %q, want %q", fnt.Typeface, widgets.EmojiTypeface)
	}
}

// buildShaper mirrors the registration order in NewAppUI:
//
//	[Inter Regular, Inter Bold, JBM-Regular..BoldItalic, NotoColorEmoji].
//
// System fonts are disabled — only embedded faces participate, matching
// the production shaper configuration.
func buildShaper(t *testing.T) (*text.Shaper, []string) {
	var fonts []font.FontFace
	var faceNames []string

	addTextFont := func(name string) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		stripped, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		face, err := opentype.Parse(stripped)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := face.Font()
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}
	addJBM := func(name string, style font.Style, weight font.Weight) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		stripped, err := fontsubset.SubsetEmoji(b)
		if err != nil {
			t.Fatalf("subset %s: %v", name, err)
		}
		face, err := opentype.Parse(stripped)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := font.Font{Typeface: widgets.MonoFamilyName, Style: style, Weight: weight}
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}
	addEmojiFont := func(name string) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
		face, err := opentype.Parse(b)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		fn := face.Font()
		fonts = append(fonts, font.FontFace{Font: fn, Face: face})
		faceNames = append(faceNames, fmt.Sprintf("%s(%s)", name, fn.Typeface))
	}

	addTextFont("Inter-Regular.ttf")
	addTextFont("Inter-Bold.ttf")
	addJBM("JetBrainsMono-Regular.ttf", font.Regular, font.Normal)
	addJBM("JetBrainsMono-Bold.ttf", font.Regular, font.Bold)
	addJBM("JetBrainsMono-Italic.ttf", font.Italic, font.Normal)
	addJBM("JetBrainsMono-BoldItalic.ttf", font.Italic, font.Bold)
	addEmojiFont("NotoColorEmoji.ttf")

	return text.NewShaper(text.NoSystemFonts(), text.WithCollection(fonts)), faceNames
}

const facebits = 16
const sizebits = 16
const gidbits = 64 - facebits - sizebits

func faceIdxFromGlyph(id uint64) int {
	return int(id >> (gidbits + sizebits))
}

// Face indices in the production order from NewAppUI / buildShaper.
const (
	interRegularIdx = 0
	interBoldIdx    = 1
	jbmRegularIdx   = 2
	jbmBoldIdx      = 3
	jbmItalicIdx    = 4
	jbmBoldItIdx    = 5
	emojiFaceIdx    = 6
)

func TestEmojiShapingPureEmojisUseEmojiFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	t.Logf("Font collection:")
	for i, n := range faceNames {
		t.Logf("  face[%d] = %s", i, n)
	}

	// "Pure" emojis — supplementary plane characters Inter does NOT cover,
	// so they MUST resolve through NotoColorEmoji.
	cases := []struct {
		name string
		s    string
	}{
		{"😀", "\U0001F600"},
		{"🚀", "\U0001F680"},
		{"🎉", "\U0001F389"},
		{"👍", "\U0001F44D"},
		{"🇺🇸 flag", "\U0001F1FA\U0001F1F8"},
		{"🇯🇵 flag", "\U0001F1EF\U0001F1F5"},
		{"👨‍💻 ZWJ", "\U0001F468‍\U0001F4BB"},
		{"👍🏻 skin", "\U0001F44D\U0001F3FB"},
		{"🏳️‍🌈 rainbow", "\U0001F3F3️‍\U0001F308"},
		{"🙂", "\U0001F642"},
		{"🤔", "\U0001F914"},
		{"🔥", "\U0001F525"},
		{"🌍", "\U0001F30D"},
		{"👩‍🔬", "\U0001F469‍\U0001F52C"},
		{"👨‍👩‍👧‍👦 family", "\U0001F468‍\U0001F469‍\U0001F467‍\U0001F466"},
		{"🤦", "\U0001F926"},
		{"🐱", "\U0001F431"},
		{"🍎", "\U0001F34E"},
	}
	pxPerEm := fixed.I(20)
	queries := []font.Font{
		{Typeface: "Inter," + widgets.EmojiTypeface},
		{Typeface: widgets.MonoTypeface},
		{Typeface: ""},
	}
	for _, q := range queries {
		t.Run(string(q.Typeface), func(t *testing.T) {
			for _, tc := range cases {
				shaper.LayoutString(text.Parameters{
					PxPerEm:  pxPerEm,
					MaxWidth: 1 << 20,
					Locale:   system.Locale{Language: "en", Direction: system.LTR},
					Font:     q,
				}, tc.s)
				var advance fixed.Int26_6
				faces := map[int]int{}
				for {
					g, ok := shaper.NextGlyph()
					if !ok {
						break
					}
					if g.Advance == 0 && g.Runes == 0 {
						continue
					}
					advance += g.Advance
					faces[faceIdxFromGlyph(uint64(g.ID))]++
				}
				t.Logf("  %-25s faces=%v advance=%v", tc.name, faces, advance)
				if advance == 0 {
					t.Errorf("    zero advance for %q", tc.s)
				}
				for idx, cnt := range faces {
					if idx != emojiFaceIdx && cnt > 0 {
						t.Errorf("    %s: face[%d]=%s used (want emoji face[%d])",
							tc.name, idx, faceNames[idx], emojiFaceIdx)
					}
				}
			}
		})
	}
}

// TestDigitsAndTextStayInTextFont guarantees ordinary text — digits,
// punctuation, Latin/Cyrillic letters — keeps using Inter / JetBrains Mono,
// not NotoColorEmoji (whose cmap includes keycap-base codepoints like 0-9).
func TestDigitsAndTextStayInTextFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	cases := []struct {
		name        string
		s           string
		query       font.Font
		wantTextIdx int
	}{
		{"digits/Inter", "1234567890", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"hash/Inter", "#", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"asterisk/Inter", "*", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"latin/Inter", "Hello", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"cyrillic/Inter", "Привет", font.Font{Typeface: "Inter," + widgets.EmojiTypeface}, interRegularIdx},
		{"digits/Mono", "1234567890", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"hash/Mono", "#", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"asterisk/Mono", "*", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"latin/Mono", "Hello", font.Font{Typeface: widgets.MonoTypeface}, jbmRegularIdx},
		{"digits/Empty", "1234567890", font.Font{}, interRegularIdx},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shaper.LayoutString(text.Parameters{
				PxPerEm:  fixed.I(20),
				MaxWidth: 1 << 20,
				Locale:   system.Locale{Language: "en", Direction: system.LTR},
				Font:     tc.query,
			}, tc.s)
			var adv fixed.Int26_6
			faces := map[int]int{}
			for {
				g, ok := shaper.NextGlyph()
				if !ok {
					break
				}
				if g.Advance == 0 && g.Runes == 0 {
					continue
				}
				adv += g.Advance
				faces[faceIdxFromGlyph(uint64(g.ID))]++
			}
			t.Logf("%s faces=%v advance=%v", tc.name, faces, adv)
			if adv == 0 {
				t.Errorf("%q: zero advance", tc.s)
			}
			if n := faces[emojiFaceIdx]; n > 0 {
				t.Errorf("%q: %d glyphs went through NotoColorEmoji — should be %s",
					tc.s, n, faceNames[tc.wantTextIdx])
			}
		})
	}
}

// TestDualUseBMPEmojiNowGoToEmojiFont covers BMP codepoints that used to
// leak through Inter / JBM (they had monochrome glyphs for ❤ ⚠ ☀ ⚡ ⬜ etc.).
// After subsetting those text fonts, the resolver must fall through to
// NotoColorEmoji.
func TestDualUseBMPEmojiNowGoToEmojiFont(t *testing.T) {
	shaper, faceNames := buildShaper(t)
	cases := []struct {
		name string
		s    string
	}{
		{"heart ❤", "❤"},
		{"warning ⚠", "⚠"},
		{"sun ☀", "☀"},
		{"snowman ☃", "☃"},
		{"snowflake ❄", "❄"},
		{"lightning ⚡", "⚡"},
		{"white square ⬜", "⬜"},
		{"black square ⬛", "⬛"},
		{"telephone ☎", "☎"},
		{"airplane ✈", "✈"},
		{"hot bev ☕", "☕"},
		{"copyright ©", "©"},
		{"registered ®", "®"},
		{"tm ™", "™"},
		{"star ⭐", "⭐"},
	}
	queries := []font.Font{
		{Typeface: "Inter," + widgets.EmojiTypeface},
		{Typeface: widgets.MonoTypeface},
		{Typeface: "Inter," + widgets.EmojiTypeface, Weight: font.Bold},
		{Typeface: widgets.MonoTypeface, Style: font.Italic},
		{Typeface: ""},
	}
	for _, q := range queries {
		t.Run(string(q.Typeface)+"/"+q.Weight.String()+"/"+q.Style.String(), func(t *testing.T) {
			for _, tc := range cases {
				shaper.LayoutString(text.Parameters{
					PxPerEm:  fixed.I(20),
					MaxWidth: 1 << 20,
					Locale:   system.Locale{Language: "en", Direction: system.LTR},
					Font:     q,
				}, tc.s)
				var adv fixed.Int26_6
				faces := map[int]int{}
				for {
					g, ok := shaper.NextGlyph()
					if !ok {
						break
					}
					if g.Advance == 0 && g.Runes == 0 {
						continue
					}
					adv += g.Advance
					faces[faceIdxFromGlyph(uint64(g.ID))]++
				}
				t.Logf("  %-20s faces=%v advance=%v", tc.name, faces, adv)
				if adv == 0 {
					t.Errorf("%s: zero advance", tc.name)
				}
				for idx, cnt := range faces {
					if idx != emojiFaceIdx && cnt > 0 {
						t.Errorf("%s: face[%d]=%s used (want NotoColorEmoji)",
							tc.name, idx, faceNames[idx])
					}
				}
			}
		})
	}
}

func TestNonEmojiUnicodeStillWorks(t *testing.T) {
	shaper, _ := buildShaper(t)
	cases := []struct {
		name string
		s    string
	}{
		{"latin", "Hello world"},
		{"cyrillic", "Привет мир"},
		{"greek", "Γειά σου κόσμε"},
		{"punctuation", "[]{}—«»…"},
		{"numbers", "1234567890"},
		{"latin+emoji", "hi 🚀"},
	}
	for _, tc := range cases {
		shaper.LayoutString(text.Parameters{
			PxPerEm:  fixed.I(20),
			MaxWidth: 1 << 20,
			Locale:   system.Locale{Language: "en", Direction: system.LTR},
			Font:     font.Font{Typeface: "Inter," + widgets.EmojiTypeface},
		}, tc.s)
		var adv fixed.Int26_6
		var gc int
		for {
			g, ok := shaper.NextGlyph()
			if !ok {
				break
			}
			gc++
			adv += g.Advance
		}
		t.Logf("%-20s glyphs=%d advance=%v", tc.name, gc, adv)
		if adv == 0 {
			t.Errorf("%q produced zero advance", tc.s)
		}
	}
}
