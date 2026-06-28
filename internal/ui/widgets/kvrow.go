package widgets

import (
	"image"
	"image/color"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

const (
	kvDividerHitDp = 8
	kvKeyFloorDp   = 24
	kvValueMinDp   = 40
)

func KVKeysMinWidth(gtx layout.Context, th *material.Theme, n int, keyAt func(i int) *widget.Editor) int {
	pad := gtx.Dp(unit.Dp(4))*2 + gtx.Dp(unit.Dp(2))
	maxW := 0
	for i := 0; i < n; i++ {
		w := MeasureTextWidthCached(gtx, th, unit.Sp(11), MonoFont, keyAt(i).Text())
		if w > maxW {
			maxW = w
		}
	}
	minW := maxW + pad
	if floor := gtx.Dp(unit.Dp(kvKeyFloorDp)); minW < floor {
		minW = floor
	}
	return minW
}

func KVSurface() color.NRGBA {
	return theme.Mix(theme.Bg, theme.BgField, 0.4)
}

func DeleteButtonInside(gtx layout.Context) layout.Dimensions {
	bg := theme.Mix(theme.Bg, theme.Danger, 0.6)
	fg := theme.ContrastOn(bg)
	sz := gtx.Constraints.Min
	rect := clip.UniformRRect(image.Rectangle{Max: sz}, 2)
	paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		is := gtx.Dp(unit.Dp(14))
		gtx.Constraints.Min = image.Point{X: is, Y: is}
		return IconDel.Layout(gtx, fg)
	})
}

func KVRow(gtx layout.Context, th *material.Theme, key, value *widget.Editor, del *widget.Clickable, keyW *float32, drag *gesture.Drag, lastX *float32, belowMin *bool, minKey int, env map[string]string) layout.Dimensions {
	fieldH := gtx.Dp(unit.Dp(26))
	dividerW := gtx.Dp(unit.Dp(kvDividerHitDp))
	spacerW := gtx.Dp(unit.Dp(2))
	delW := gtx.Dp(unit.Dp(20))
	valueMin := gtx.Dp(unit.Dp(kvValueMinDp))
	dragFloor := gtx.Dp(unit.Dp(8))

	flexTotal := gtx.Constraints.Max.X - dividerW - spacerW - delW
	if flexTotal < 2 {
		flexTotal = 2
	}

	resolveKeyW := func(stored float32) int {
		floored := belowMin == nil || !*belowMin
		w := int(stored)
		if stored <= 0 {
			w = minKey
		}
		floor := 0
		if floored {
			floor = minKey
			if w < minKey {
				w = minKey
			}
		}
		if maxKey := flexTotal - valueMin; w > maxKey {
			if maxKey >= floor {
				w = maxKey
			} else {
				w = floor
			}
		}
		if w > flexTotal {
			w = flexTotal
		}
		if w < 0 {
			w = 0
		}
		return w
	}

	if drag != nil && keyW != nil {
		for {
			ev, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
			if !ok {
				break
			}
			switch ev.Kind {
			case pointer.Press:
				*lastX = ev.Position.X
				*keyW = float32(resolveKeyW(*keyW))
			case pointer.Drag:
				d := ev.Position.X - *lastX
				*lastX = ev.Position.X
				nw := *keyW + d
				if nw < float32(dragFloor) {
					nw = float32(dragFloor)
				}
				if mx := float32(flexTotal - valueMin); nw > mx {
					nw = mx
				}
				*keyW = nw
				if belowMin != nil {
					*belowMin = int(nw) < minKey
				}
			}
		}
	}

	stored := float32(0)
	if keyW != nil {
		stored = *keyW
	}
	kw := resolveKeyW(stored)
	valueW := flexTotal - kw

	cell := func(w int, wdg layout.Widget) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Pt(w, fieldH)
			gtx.Constraints.Max = gtx.Constraints.Min
			return wdg(gtx)
		})
	}
	gap := func(w int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(w, fieldH)}
		})
	}

	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		cell(kw, func(gtx layout.Context) layout.Dimensions {
			return TextFieldOverlayBg(gtx, th, key, "Key", false, env, 0, unit.Sp(11), KVSurface())
		}),
		gap(dividerW),
		cell(valueW, func(gtx layout.Context) layout.Dimensions {
			return TextFieldOverlayBg(gtx, th, value, "Value", false, env, 0, unit.Sp(11), KVSurface())
		}),
		gap(spacerW),
		cell(delW, func(gtx layout.Context) layout.Dimensions {
			return del.Layout(gtx, DeleteButtonInside)
		}),
	)

	if drag != nil {
		area := image.Rect(kw, 0, kw+dividerW, fieldH)
		st := clip.Rect(area).Push(gtx.Ops)
		pointer.CursorColResize.Add(gtx.Ops)
		drag.Add(gtx.Ops)
		event.Op(gtx.Ops, drag)
		st.Pop()
	}
	line := gtx.Dp(unit.Dp(1))
	col := theme.BorderLight
	if drag != nil && drag.Dragging() {
		col = theme.Accent
	}
	cx := kw + dividerW/2
	paint.FillShape(gtx.Ops, col, clip.Rect{Min: image.Pt(cx-line/2, 0), Max: image.Pt(cx-line/2+line, fieldH)}.Op())

	return dims
}
