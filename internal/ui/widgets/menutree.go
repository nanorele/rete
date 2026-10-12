package widgets

import (
	"image"

	"rete/internal/ui/theme"

	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type MenuNode struct {
	Item     MenuItem
	Children []MenuNode
}

type MenuTreeState struct {
	path []int
}

func (s *MenuTreeState) Reset() {
	if s != nil {
		s.path = s.path[:0]
	}
}

func (s *MenuTreeState) OpenPath() []int {
	if s == nil {
		return nil
	}
	return s.path
}

const submenuOverlapDp = 3

type menuRowGeom struct {
	y, h int
}

func menuListPad() unit.Dp {
	if theme.CompactMenus {
		return 0
	}
	return unit.Dp(MenuListPadDp)
}

func (m menuStyler) treeContent(nodes []MenuNode, geom *[]menuRowGeom) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		rowMinX := gtx.Constraints.Min.X
		pad := menuListPad()
		*geom = (*geom)[:0]
		return layout.Inset{Top: pad, Bottom: pad}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			y := 0
			w := rowMinX
			for i := range nodes {
				it := nodes[i].Item
				hasKids := len(nodes[i].Children) > 0
				if hasKids {
					it.Shortcut = " "
				}
				rg := gtx
				rg.Constraints.Min.X = rowMinX
				rg.Constraints.Min.Y = 0
				off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
				d := m.Row(rg, it)
				if hasKids {
					isz := gtx.Dp(unit.Dp(14))
					cx := d.Size.X - gtx.Dp(unit.Dp(MenuRowPadHDp)) - isz
					cy := (d.Size.Y - isz) / 2
					io := op.Offset(image.Pt(cx, cy)).Push(gtx.Ops)
					ig := gtx
					ig.Constraints.Min = image.Pt(isz, isz)
					ig.Constraints.Max = ig.Constraints.Min
					col := m.Colors.Fg
					if it.Disabled {
						col = m.Colors.Disabled
					}
					IconChevronR.Layout(ig, col)
					io.Pop()
				}
				off.Pop()
				*geom = append(*geom, menuRowGeom{y: y, h: d.Size.Y})
				y += d.Size.Y
				if d.Size.X > w {
					w = d.Size.X
				}
			}
			return layout.Dimensions{Size: image.Pt(w, y)}
		})
	}
}

func menuTreeUpdate(gtx layout.Context, nodes []MenuNode, st *MenuTreeState) {
	level := 0
	cur := nodes
	for cur != nil {
		next := -1
		for i := range cur {
			it := cur[i].Item
			if it.Click == nil || it.Disabled {
				continue
			}
			hasKids := len(cur[i].Children) > 0
			clicked := false
			if hasKids {
				for it.Click.Clicked(gtx) {
					clicked = true
				}
			}
			if clicked || it.Click.Hovered() {
				if hasKids {
					if level < len(st.path) && st.path[level] == i {
						continue
					}
					st.path = append(st.path[:level], i)
				} else {
					st.path = st.path[:level]
				}
			}
		}
		if level < len(st.path) {
			next = st.path[level]
		}
		if next < 0 || next >= len(cur) {
			st.path = st.path[:level]
			return
		}
		cur = cur[next].Children
		level++
	}
}

func DeferMenuTreeAt(gtx layout.Context, th *material.Theme, tag event.Tag, anchor MenuAnchor, minWidthDp int, nodes []MenuNode, st *MenuTreeState) layout.Dimensions {
	if st == nil {
		st = &MenuTreeState{}
	}
	menuTreeUpdate(gtx, nodes, st)
	m := menuStyle(th, minWidthDp)

	var geom []menuRowGeom
	mGtx := gtx
	mGtx.Constraints.Min = image.Point{}
	rec := op.Record(gtx.Ops)
	dims := m.surface(mGtx, tag, m.treeContent(nodes, &geom))
	call := rec.Stop()
	pos := anchor.Resolve(dims.Size)
	macro := op.Record(gtx.Ops)
	op.Offset(pos).Add(gtx.Ops)
	call.Add(gtx.Ops)
	op.Defer(gtx.Ops, macro.Stop())

	parentPos, parentW := pos, dims.Size.X
	parentGeom := geom
	cur := nodes
	overlap := gtx.Dp(unit.Dp(submenuOverlapDp))
	for level := 0; level < len(st.path); level++ {
		idx := st.path[level]
		if idx < 0 || idx >= len(cur) || idx >= len(parentGeom) {
			break
		}
		kids := cur[idx].Children
		if len(kids) == 0 {
			break
		}
		var subGeom []menuRowGeom
		sRec := op.Record(gtx.Ops)
		sDims := m.surface(mGtx, tag, m.treeContent(kids, &subGeom))
		sCall := sRec.Stop()

		x := parentPos.X + parentW - overlap
		if anchor.Clamp.X > 0 && x+sDims.Size.X > anchor.Clamp.X {
			x = parentPos.X - sDims.Size.X + overlap
			if x < 0 {
				x = 0
			}
		}
		y := parentPos.Y + parentGeom[idx].y
		if anchor.Clamp.Y > 0 && y+sDims.Size.Y > anchor.Clamp.Y {
			y = anchor.Clamp.Y - sDims.Size.Y
		}
		if y < 0 {
			y = 0
		}
		sPos := image.Pt(x, y)
		sMacro := op.Record(gtx.Ops)
		op.Offset(sPos).Add(gtx.Ops)
		sCall.Add(gtx.Ops)
		op.Defer(gtx.Ops, sMacro.Stop())

		parentPos, parentW = sPos, sDims.Size.X
		parentGeom = subGeom
		cur = kids
	}
	return dims
}

func (s *MenuTreeState) SetPath(path ...int) {
	if s != nil {
		s.path = append(s.path[:0], path...)
	}
}
