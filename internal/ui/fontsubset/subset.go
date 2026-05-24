// Package fontsubset rewrites a TrueType / OpenType font's cmap so the
// resulting face stops claiming coverage for a chosen set of codepoints,
// without touching glyph data. The intended use is to remove emoji-property
// codepoints from text fonts (Inter, JetBrains Mono) so the gio shaper's
// fallback chain routes them to a dedicated emoji font (NotoColorEmoji).
//
// Subset returns rewritten bytes; nothing is written to disk.
package fontsubset

import (
	"errors"
	"fmt"
)

// Subset returns a copy of ttf whose cmap omits every codepoint for which
// shouldRemove returns true. Glyph storage is preserved (those glyphs simply
// become unreachable). Only Unicode subtables (format 4 and format 12) are
// read; the rewritten cmap exposes a single format-12 subtable suitable for
// gio / fontscan.
func Subset(ttf []byte, shouldRemove func(r rune) bool) ([]byte, error) {
	if shouldRemove == nil {
		return nil, errors.New("fontsubset: shouldRemove is nil")
	}
	f, err := parseSFNT(ttf)
	if err != nil {
		return nil, fmt.Errorf("fontsubset: %w", err)
	}
	cmapT, ok := f.tables[tagCmap]
	if !ok {
		return nil, errors.New("fontsubset: cmap table missing")
	}
	pairs, err := parseUnicodeCmap(cmapT.data)
	if err != nil {
		return nil, fmt.Errorf("fontsubset: %w", err)
	}
	kept := pairs[:0]
	for _, p := range pairs {
		if !shouldRemove(p.codepoint) {
			kept = append(kept, p)
		}
	}
	cmapT.data = buildFormat12Cmap(kept)
	return f.serialize()
}

// SubsetEmoji is a convenience wrapper that strips Unicode 15.1 Emoji-property
// codepoints except '#', '*' and '0'..'9'. See IsEmojiCodepoint for the
// exact predicate.
func SubsetEmoji(ttf []byte) ([]byte, error) {
	return Subset(ttf, IsEmojiCodepoint)
}
