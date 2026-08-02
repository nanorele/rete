package ui

import (
	"sync"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/opentype"
	"github.com/nanorele/gio/text"

	"tracto/pkg/fontsubset"
)

type lazyFontSpec struct {
	file     string
	typeface font.Typeface
	ranges   []text.RuneRange
}

func rr(lo, hi rune) text.RuneRange { return text.RuneRange{Lo: lo, Hi: hi} }

var emojiFontSpec = lazyFontSpec{
	file:     "NotoColorEmoji.ttf",
	typeface: "Noto Color Emoji",
	ranges:   emojiClaimRanges(),
}

// emojiClaimRanges mirrors the codepoints fontsubset strips from the UI and
// mono faces, so the color face is loaded exactly where those faces stopped
// providing glyphs.
func emojiClaimRanges() []text.RuneRange {
	src := fontsubset.EmojiRanges()
	out := make([]text.RuneRange, len(src))
	for i, r := range src {
		out[i] = rr(r[0], r[1])
	}
	return out
}

var fallbackFontSpecs = []lazyFontSpec{
	{"NotoSansHebrew-Regular.ttf", "Noto Sans Hebrew", []text.RuneRange{
		rr(0x0590, 0x05FF), rr(0xFB1D, 0xFB4F),
	}},
	{"NotoSansArabic-Regular.ttf", "Noto Sans Arabic", []text.RuneRange{
		rr(0x0600, 0x06FF), rr(0x0750, 0x077F), rr(0x08A0, 0x08FF),
		rr(0xFB50, 0xFDFF), rr(0xFE70, 0xFEFF),
	}},
	{"NotoSansThai-Regular.ttf", "Noto Sans Thai", []text.RuneRange{
		rr(0x0E00, 0x0E7F),
	}},
	{"NotoSansDevanagari-Regular.ttf", "Noto Sans Devanagari", []text.RuneRange{
		rr(0x0900, 0x097F), rr(0xA8E0, 0xA8FF), rr(0x1CD0, 0x1CFF),
	}},
	{"NotoSansBengali-Regular.ttf", "Noto Sans Bengali", []text.RuneRange{
		rr(0x0980, 0x09FF),
	}},
	{"NotoSansTamil-Regular.ttf", "Noto Sans Tamil", []text.RuneRange{
		rr(0x0B80, 0x0BFF), rr(0x11FC0, 0x11FFF),
	}},
	{"NotoSansTelugu-Regular.ttf", "Noto Sans Telugu", []text.RuneRange{
		rr(0x0C00, 0x0C7F),
	}},
	{"NotoSansKannada-Regular.ttf", "Noto Sans Kannada", []text.RuneRange{
		rr(0x0C80, 0x0CFF),
	}},
	{"NotoSansMalayalam-Regular.ttf", "Noto Sans Malayalam", []text.RuneRange{
		rr(0x0D00, 0x0D7F),
	}},
	{"NotoSansGujarati-Regular.ttf", "Noto Sans Gujarati", []text.RuneRange{
		rr(0x0A80, 0x0AFF),
	}},
	{"NotoSansGurmukhi-Regular.ttf", "Noto Sans Gurmukhi", []text.RuneRange{
		rr(0x0A00, 0x0A7F),
	}},
	{"NotoSansSinhala-Regular.ttf", "Noto Sans Sinhala", []text.RuneRange{
		rr(0x0D80, 0x0DFF), rr(0x111E0, 0x111FF),
	}},
	{"NotoSansGeorgian-Regular.ttf", "Noto Sans Georgian", []text.RuneRange{
		rr(0x10A0, 0x10FF), rr(0x1C90, 0x1CBF), rr(0x2D00, 0x2D2F),
	}},
	{"NotoSansArmenian-Regular.ttf", "Noto Sans Armenian", []text.RuneRange{
		rr(0x0530, 0x058F), rr(0xFB13, 0xFB17),
	}},
	{"NotoSansKhmer-Regular.ttf", "Noto Sans Khmer", []text.RuneRange{
		rr(0x1780, 0x17FF), rr(0x19E0, 0x19FF),
	}},
	{"NotoSansLao-Regular.ttf", "Noto Sans Lao", []text.RuneRange{
		rr(0x0E80, 0x0EFF), rr(0xAA80, 0xAADF),
	}},
	{"NotoSansMyanmar-Regular.ttf", "Noto Sans Myanmar", []text.RuneRange{
		rr(0x1000, 0x109F), rr(0xA9E0, 0xA9FF), rr(0xAA60, 0xAA7F),
	}},
	{"NotoSansEthiopic-Regular.ttf", "Noto Sans Ethiopic", []text.RuneRange{
		rr(0x1200, 0x139F), rr(0x2D80, 0x2DDF), rr(0xAB00, 0xAB2F),
	}},
	{"NotoSansCJK-Regular.otf", "Noto Sans CJK SC", []text.RuneRange{
		rr(0x1100, 0x11FF), rr(0x2E80, 0x2EFF), rr(0x2F00, 0x2FDF),
		rr(0x3000, 0x30FF), rr(0x3100, 0x312F), rr(0x3130, 0x318F),
		rr(0x31A0, 0x31FF), rr(0x3200, 0x33FF), rr(0x3400, 0x4DBF),
		rr(0x4E00, 0x9FFF), rr(0xA960, 0xA97F), rr(0xAC00, 0xD7FF),
		rr(0xF900, 0xFAFF), rr(0xFE10, 0xFE4F), rr(0xFF00, 0xFFEF),
		rr(0x1B000, 0x1B16F), rr(0x20000, 0x2A6DF), rr(0x2A700, 0x2EBEF),
		rr(0x2F800, 0x2FA1F), rr(0x30000, 0x3134F),
	}},
}

var (
	lazyFontsOnce sync.Once
	lazyFonts     []text.LazyFace
)

func appLazyFontFaces() []text.LazyFace {
	lazyFontsOnce.Do(func() { lazyFonts = buildLazyFontFaces() })
	return lazyFonts
}

func buildLazyFontFaces() []text.LazyFace {
	specs := append([]lazyFontSpec{emojiFontSpec}, fallbackFontSpecs...)
	out := make([]text.LazyFace, 0, len(specs))
	for _, spec := range specs {
		out = append(out, text.LazyFace{
			Typeface: spec.typeface,
			Ranges:   spec.ranges,
			Load:     lazyFontLoader(spec.file),
		})
	}
	return out
}

func lazyFontLoader(name string) func() (font.FontFace, error) {
	return func() (font.FontFace, error) {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			return font.FontFace{}, err
		}
		face, err := opentype.Parse(b)
		if err != nil {
			return font.FontFace{}, err
		}
		return font.FontFace{Font: face.Font(), Face: face}, nil
	}
}
