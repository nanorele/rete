package fontsubset

import (
	"errors"
	"fmt"
)

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

func SubsetEmoji(ttf []byte) ([]byte, error) {
	return Subset(ttf, IsEmojiCodepoint)
}
