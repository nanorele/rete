package widgets

import (
	"unicode/utf8"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"golang.org/x/image/math/fixed"
)

func WrapLineStartsFor(
	shaper *text.Shaper,
	fnt font.Font,
	size unit.Sp,
	gtx layout.Context,
	chunkText []byte,
	maxW int,
	out []int,
) []int {
	out = out[:0]
	if len(chunkText) == 0 || shaper == nil {
		return append(out, 0)
	}
	if maxW < 1 {
		maxW = 1
	}
	shaper.LayoutString(text.Parameters{
		Font:       fnt,
		PxPerEm:    fixed.I(gtx.Sp(size)),
		Locale:     gtx.Locale,
		WrapPolicy: text.WrapGraphemes,
		MaxWidth:   maxW,
	}, string(chunkText))

	out = append(out, 0)
	byteAccum := 0
	broke := false
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		if broke {
			out = append(out, byteAccum)
			broke = false
		}
		runeBytes := 0
		for r := uint32(0); r < g.Runes && byteAccum+runeBytes < len(chunkText); r++ {
			_, sz := utf8.DecodeRune(chunkText[byteAccum+runeBytes:])
			runeBytes += sz
		}
		byteAccum += runeBytes
		if g.Flags&text.FlagLineBreak != 0 {
			broke = true
		}
	}
	return out
}
