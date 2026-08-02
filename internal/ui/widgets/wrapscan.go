package widgets

import (
	"github.com/nanorele/gio-x/component"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
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
	return component.WrapLineStartsFor(shaper, fnt, size, gtx, chunkText, maxW, out)
}
