package ui

import (
	"testing"
	"tracto/internal/ui/fontsubset"

	"github.com/nanorele/gio/font/opentype"
)

type embeddedFont struct {
	name string
	data []byte
}

func textFontPayloads(t *testing.T) []embeddedFont {
	files := []struct{ label, file string }{
		{"Inter-Regular", "Inter-Regular.ttf"},
		{"Inter-Bold", "Inter-Bold.ttf"},
		{"JetBrainsMono-Regular", "JetBrainsMono-Regular.ttf"},
		{"JetBrainsMono-Bold", "JetBrainsMono-Bold.ttf"},
		{"JetBrainsMono-Italic", "JetBrainsMono-Italic.ttf"},
		{"JetBrainsMono-BoldItalic", "JetBrainsMono-BoldItalic.ttf"},
	}
	out := make([]embeddedFont, 0, len(files))
	for _, f := range files {
		b, err := loadEmbeddedTTF(f.file)
		if err != nil {
			t.Fatalf("load %s: %v", f.file, err)
		}
		out = append(out, embeddedFont{f.label, b})
	}
	return out
}

func TestSubsetRemovesEmojiCoverage(t *testing.T) {
	emojisToCheck := []rune{
		0x00A9,
		0x00AE,
		0x2122,
		0x2600,
		0x2603,
		0x2614,
		0x2615,
		0x2618,
		0x2620,
		0x26A0,
		0x26A1,
		0x26C4,
		0x26FD,
		0x2705,
		0x2708,
		0x2728,
		0x2744,
		0x274C,
		0x2753,
		0x2757,
		0x2764,
		0x2B1C,
		0x2B50,
		0x2B55,
	}
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("SubsetEmoji: %v", err)
			}
			face, err := opentype.Parse(out)
			if err != nil {
				t.Fatalf("Parse subsetted: %v", err)
			}
			fnt := face.Face().Font
			for _, r := range emojisToCheck {
				if gid, ok := fnt.Cmap.Lookup(r); ok && gid != 0 {
					t.Errorf("U+%04X still mapped to glyph %d after subset", r, gid)
				}
			}
		})
	}
}

func TestSubsetKeepsTextCoverage(t *testing.T) {
	mustCover := []rune{
		'0', '1', '5', '9',
		'#', '*',
		'A', 'M', 'z',
		'(', ')', ',', '.', ' ',
		'я', 'А', 'Ё',
		'α', 'Ω',
	}
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("SubsetEmoji: %v", err)
			}
			face, err := opentype.Parse(out)
			if err != nil {
				t.Fatalf("Parse subsetted: %v", err)
			}
			fnt := face.Face().Font
			for _, r := range mustCover {
				gid, ok := fnt.Cmap.Lookup(r)
				if !ok || gid == 0 {
					t.Errorf("U+%04X (%q) lost coverage (gid=%d ok=%v)", r, string(r), gid, ok)
				}
			}
		})
	}
}

func TestSubsetRoundTripParses(t *testing.T) {
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			out, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("subset: %v", err)
			}
			if _, err := opentype.Parse(out); err != nil {
				t.Fatalf("opentype.Parse: %v", err)
			}
		})
	}
}

func TestSubsetIdempotent(t *testing.T) {
	for _, fc := range textFontPayloads(t) {
		t.Run(fc.name, func(t *testing.T) {
			once, err := fontsubset.SubsetEmoji(fc.data)
			if err != nil {
				t.Fatalf("first subset: %v", err)
			}
			twice, err := fontsubset.SubsetEmoji(once)
			if err != nil {
				t.Fatalf("second subset: %v", err)
			}
			if len(twice) > len(once) {
				t.Errorf("size grew on re-subset: %d → %d", len(once), len(twice))
			}
			face, err := opentype.Parse(twice)
			if err != nil {
				t.Fatalf("parse twice: %v", err)
			}
			f := face.Face().Font
			if gid, ok := f.Cmap.Lookup(0x2764); ok && gid != 0 {
				t.Errorf("❤ reappeared after re-subset")
			}
			if gid, ok := f.Cmap.Lookup('0'); !ok || gid == 0 {
				t.Errorf("digit 0 lost on re-subset")
			}
		})
	}
}
