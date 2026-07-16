package workspace

import (
	"testing"

	"golang.org/x/image/math/fixed"
)

func TestCoordToByteOffset_NoWrap_RoundsToNearestChar(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("abcdef")
	v.padChunkHeights()
	for i := range v.chunkHeights {
		v.chunkHeights[i] = 10
	}
	gtx := makeTestGtx()
	adv := fixed.I(8)

	if got := v.coordToByteOffset(gtx, 22, 0, adv, 10, 200, false); got != 3 {
		t.Errorf("click on right half of a glyph should place caret after it; got %d, want 3", got)
	}
	if got := v.coordToByteOffset(gtx, 17, 0, adv, 10, 200, false); got != 2 {
		t.Errorf("click on left half of a glyph should place caret before it; got %d, want 2", got)
	}
}
