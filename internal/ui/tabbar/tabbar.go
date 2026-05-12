package tabbar

import (
	"image"
	"strings"
	"time"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/ui/workspace"
	"tracto/internal/utils"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type cachedTab struct {
	title string
	width int
	ppdp  float32
}

type tabInfo struct {
	Idx        int
	NatWidth   int
	FinalWidth int
}

type Strip struct {
	AddTabBtn        widget.Clickable
	tabDragTag       struct{}
	TabDragIdx       int
	TabDragging      bool
	TabDragOriginX   float32
	TabDragOriginY   float32
	TabDragPressX    float32
	TabDragPressY    float32
	TabDragCurrentX  float32
	TabDragCurrentY  float32
	TabDragPressTime time.Time

	TabCtxMenuOpen bool
	TabCtxMenuIdx  int
	TabCtxMenuPos  f32.Point

	widthCache map[*workspace.RequestTab]cachedTab
	infoBuf    []tabInfo
	rowsBuf    [][]int
	rowBuf     []int
}

func NewStrip() *Strip {
	return &Strip{
		TabDragIdx: -1,
		widthCache: make(map[*workspace.RequestTab]cachedTab),
	}
}

func (s *Strip) Forget(tab *workspace.RequestTab) {
	delete(s.widthCache, tab)
}

func measureTabWidth(gtx layout.Context, th *material.Theme, cleanTitle string) int {
	words := strings.Fields(cleanTitle)

	var maxW int
	if len(words) <= 1 {
		if len(words) == 0 {
			cleanTitle = "New request"
		}
		maxW = widgets.MeasureTextWidth(gtx, th, unit.Sp(12), font.Font{}, cleanTitle)
	} else {
		mid := (len(words) + 1) / 2
		line1 := strings.Join(words[:mid], " ")
		line2 := strings.Join(words[mid:], " ")
		w1 := widgets.MeasureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line1)
		w2 := widgets.MeasureTextWidth(gtx, th, unit.Sp(12), font.Font{}, line2)
		maxW = max(w1, w2)
	}

	totalW := maxW + gtx.Dp(unit.Dp(52))
	maxWidthLimit := gtx.Dp(unit.Dp(200))
	if totalW > maxWidthLimit {
		return maxWidthLimit
	}
	return totalW
}

func (s *Strip) Layout(
	gtx layout.Context,
	th *material.Theme,
	tabs *[]*workspace.RequestTab,
	activeIdx *int,
	onRevealLinkedNode func(*workspace.RequestTab),
	onSave func(),
) layout.Dimensions {
	return layout.Inset{
		Top:    unit.Dp(8),
		Bottom: unit.Dp(8),
		Left:   unit.Dp(4),
		Right:  unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		tabHeight := gtx.Dp(unit.Dp(36))
		closeBtnWidth := gtx.Dp(unit.Dp(28))
		addBtnW := gtx.Dp(unit.Dp(36))
		maxWidth := max(gtx.Constraints.Max.X-2, 0)

		infos := s.infoBuf[:0]
		for i, tab := range *tabs {
			cache, ok := s.widthCache[tab]
			if !ok || cache.title != tab.Title || cache.ppdp != gtx.Metric.PxPerDp {
				natW := measureTabWidth(gtx, th, tab.GetCleanTitle())
				s.widthCache[tab] = cachedTab{title: tab.Title, width: natW, ppdp: gtx.Metric.PxPerDp}
				cache = s.widthCache[tab]
			}
			infos = append(infos, tabInfo{Idx: i, NatWidth: cache.width})
		}
		s.infoBuf = infos

		rows := s.rowsBuf[:0]
		var currentX int
		currentRow := s.rowBuf[:0]

		for i, t := range infos {
			w := t.NatWidth
			if currentX > 0 && currentX+w > maxWidth {
				rows = append(rows, currentRow)
				currentRow = nil
				currentX = 0
			}
			currentRow = append(currentRow, i)
			currentX += w
		}

		if currentX > 0 && currentX+addBtnW > maxWidth {
			rows = append(rows, currentRow)
			currentRow = nil
		}
		currentRow = append(currentRow, -1)
		rows = append(rows, currentRow)
		s.rowsBuf = rows
		s.rowBuf = currentRow

		for rIdx, row := range rows {
			isLastRow := rIdx == len(rows)-1

			rowTabsNatW := 0
			rowHasAddBtn := false
			for _, i := range row {
				if i >= 0 {
					rowTabsNatW += infos[i].NatWidth
				} else {
					rowHasAddBtn = true
				}
			}

			rowTotalNatW := rowTabsNatW
			if rowHasAddBtn {
				rowTotalNatW += addBtnW
			}

			if isLastRow {
				for _, i := range row {
					if i >= 0 {
						infos[i].FinalWidth = infos[i].NatWidth
					}
				}
				continue
			}

			extraSpace := maxWidth - rowTotalNatW
			if extraSpace > 0 && rowTabsNatW > 0 {
				allocated := 0
				lastTabInRowIdx := -1
				for j, i := range row {
					if i >= 0 {
						lastTabInRowIdx = j
					}
				}

				for j, i := range row {
					if i >= 0 {
						var add int
						if j == lastTabInRowIdx {
							add = extraSpace - allocated
						} else {
							share := float32(infos[i].NatWidth) / float32(rowTabsNatW)
							add = int(float32(extraSpace) * share)
						}
						allocated += add
						infos[i].FinalWidth = infos[i].NatWidth + add
					}
				}
			} else {
				for _, i := range row {
					if i >= 0 {
						infos[i].FinalWidth = infos[i].NatWidth
					}
				}
			}
		}

		thf := float32(tabHeight)

		tabIdxAtXY := func(x, y float32) int {
			rowIdx := int(y / thf)
			if rowIdx < 0 {
				rowIdx = 0
			}
			if rowIdx >= len(rows) {
				rowIdx = len(rows) - 1
			}
			row := rows[rowIdx]
			acc := float32(0)
			for _, tIdx := range row {
				var w float32
				if tIdx < 0 {
					w = float32(addBtnW)
				} else {
					w = float32(infos[tIdx].FinalWidth)
				}
				if x < acc+w {
					return tIdx
				}
				acc += w
			}
			if len(row) > 0 {
				last := row[len(row)-1]
				if last == -1 && len(row) > 1 {
					last = row[len(row)-2]
				}
				if last >= 0 {
					return last
				}
			}
			return -1
		}

		tabPosInRow := func(idx int) (row int, xOff float32) {
			for r, rr := range rows {
				x := float32(0)
				for _, tIdx := range rr {
					if tIdx == idx {
						return r, x
					}
					if tIdx >= 0 {
						x += float32(infos[tIdx].FinalWidth)
					}
				}
			}
			return 0, 0
		}

		swapTabs := func(a, b int) {
			(*tabs)[a], (*tabs)[b] = (*tabs)[b], (*tabs)[a]
			infos[a], infos[b] = infos[b], infos[a]
			switch *activeIdx {
			case a:
				*activeIdx = b
			case b:
				*activeIdx = a
			}
		}

		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: &s.tabDragTag,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
			})
			if !ok {
				break
			}
			if pe, ok := ev.(pointer.Event); ok {
				switch pe.Kind {
				case pointer.Press:
					if pe.Buttons.Contain(pointer.ButtonPrimary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							hitRow, xOff := tabPosInRow(hit)
							s.TabDragIdx = hit
							s.TabDragOriginX = pe.Position.X - xOff
							s.TabDragOriginY = pe.Position.Y - float32(hitRow)*thf
							s.TabDragPressX = pe.Position.X
							s.TabDragPressY = pe.Position.Y
							s.TabDragCurrentX = pe.Position.X
							s.TabDragCurrentY = pe.Position.Y
							s.TabDragging = false
							s.TabDragPressTime = gtx.Now
						}
					} else if pe.Buttons.Contain(pointer.ButtonSecondary) {
						hit := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if hit >= 0 {
							s.TabCtxMenuOpen = true
							s.TabCtxMenuIdx = hit
							s.TabCtxMenuPos = pe.Position
						}
					}
				case pointer.Drag:
					s.TabDragCurrentX = pe.Position.X
					s.TabDragCurrentY = pe.Position.Y
					if !s.TabDragging && s.TabDragIdx >= 0 {
						elapsed := gtx.Now.Sub(s.TabDragPressTime)
						dx := pe.Position.X - s.TabDragPressX
						dy := pe.Position.Y - s.TabDragPressY
						dist := dx*dx + dy*dy
						if elapsed > 150*time.Millisecond && dist > 100 {
							s.TabDragging = true
						}
					}
					if s.TabDragging && s.TabDragIdx >= 0 && s.TabDragIdx < len(*tabs) {
						target := tabIdxAtXY(pe.Position.X, pe.Position.Y)
						if target >= 0 && target != s.TabDragIdx {
							old := s.TabDragIdx
							if target > old {
								for i := old; i < target; i++ {
									swapTabs(i, i+1)
								}
							} else {
								for i := old; i > target; i-- {
									swapTabs(i, i-1)
								}
							}
							s.TabDragIdx = target
						}
					}
				case pointer.Release, pointer.Cancel:
					if s.TabDragging && onSave != nil {
						onSave()
					}
					s.TabDragging = false
					s.TabDragIdx = -1
				}
			}
		}

		tabBarHeight := len(rows) * tabHeight
		clipStack := clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, tabBarHeight)}.Push(gtx.Ops)
		passStack := pointer.PassOp{}.Push(gtx.Ops)
		event.Op(gtx.Ops, &s.tabDragTag)

		var dragTabOX, dragTabOY int
		var dragTabW int

		rowChildren := make([]layout.FlexChild, len(rows))
		for i := range rows {
			rIdx := i
			row := rows[rIdx]
			rowChildren[rIdx] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				children := make([]layout.FlexChild, 0, len(row))

				for j, tIdx := range row {
					if tIdx >= 0 {
						idx := tIdx
						finalW := infos[idx].FinalWidth
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = finalW
							gtx.Constraints.Max.X = finalW
							gtx.Constraints.Min.Y = tabHeight
							gtx.Constraints.Max.Y = tabHeight

							if s.TabDragging && s.TabDragIdx == idx {
								dragTabOX = int(s.TabDragCurrentX - s.TabDragOriginX)
								dragTabOY = int(s.TabDragCurrentY - s.TabDragOriginY)
								dragTabW = finalW
								paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(finalW, tabHeight)}.Op())
								t := max(gtx.Dp(1), 1)
								paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, tabHeight-t), Max: image.Pt(finalW, tabHeight)}.Op())
								paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(finalW-t, 0), Max: image.Pt(finalW, tabHeight)}.Op())
								if rIdx == 0 {
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(finalW, t)}.Op())
								}
								if j == 0 {
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, tabHeight)}.Op())
								}
								return layout.Dimensions{Size: image.Pt(finalW, tabHeight)}
							}

							tab := (*tabs)[idx]
							if tab.TabBtn.Clicked(gtx) {
								if !s.TabDragging {
									*activeIdx = idx
									s.TabCtxMenuOpen = false
									if onRevealLinkedNode != nil {
										onRevealLinkedNode(tab)
									}
								}
							}

							bgColor := theme.BgDark
							fgColor := theme.FgMuted
							if idx == *activeIdx {
								bgColor = theme.Bg
								fgColor = theme.Fg
							}

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: gtx.Constraints.Min}.Op())
									if idx == *activeIdx {
										paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Point{X: gtx.Constraints.Min.X, Y: gtx.Dp(unit.Dp(2))}}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
											gtx.Constraints.Min.X = gtx.Constraints.Max.X
											return material.Clickable(gtx, &tab.TabBtn, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min = gtx.Constraints.Max
												pointer.CursorPointer.Add(gtx.Ops)
												return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													topPad, bottomPad := unit.Dp(2), unit.Dp(2)
													if idx == *activeIdx {
														topPad, bottomPad = unit.Dp(3), unit.Dp(1)
													}
													return layout.Inset{Top: topPad, Bottom: bottomPad, Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
														cleanTitle := tab.GetCleanTitle()
														if tab.IsDirty {
															cleanTitle = "● " + cleanTitle
														}
														lbl := material.Label(th, unit.Sp(12), cleanTitle)
														lbl.Color = fgColor
														lbl.MaxLines = 2
														lbl.Truncator = "..."
														return lbl.Layout(gtx)
													})
												})
											})
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											gtx.Constraints.Min.X = closeBtnWidth
											gtx.Constraints.Max.X = closeBtnWidth
											gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
											return material.Clickable(gtx, &tab.CloseBtn, func(gtx layout.Context) layout.Dimensions {
												gtx.Constraints.Min = gtx.Constraints.Max
												pointer.CursorPointer.Add(gtx.Ops)
												return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													size := gtx.Dp(unit.Dp(16))
													gtx.Constraints.Min = image.Point{X: size, Y: size}
													gtx.Constraints.Max = gtx.Constraints.Min
													return widgets.IconClose.Layout(gtx, fgColor)
												})
											})
										}),
									)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := max(gtx.Dp(1), 1)
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
							)
						}))
					} else {
						children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.X = gtx.Dp(unit.Dp(36))
							gtx.Constraints.Max.X = gtx.Constraints.Min.X
							gtx.Constraints.Min.Y = tabHeight
							gtx.Constraints.Max.Y = tabHeight

							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: gtx.Constraints.Min}.Op())
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									btn := material.Button(th, &s.AddTabBtn, "+")
									btn.Background = theme.BgDark
									btn.Color = th.Fg
									btn.TextSize = unit.Sp(16)
									btn.CornerRadius = unit.Dp(0)
									btn.Inset = layout.Inset{}
									gtx.Constraints.Min = gtx.Constraints.Max
									defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
									pointer.CursorPointer.Add(gtx.Ops)
									return btn.Layout(gtx)
								}),
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									maxX := gtx.Constraints.Min.X
									maxY := gtx.Constraints.Min.Y
									t := max(gtx.Dp(1), 1)
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, maxY-t), Max: image.Pt(maxX, maxY)}.Op())
									paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(maxX-t, 0), Max: image.Pt(maxX, maxY)}.Op())
									if rIdx == 0 {
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(maxX, t)}.Op())
									}
									if j == 0 {
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(0, 0), Max: image.Pt(t, maxY)}.Op())
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
							)
						}))
					}
				}
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
			})
		}
		listDims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, rowChildren...)

		passStack.Pop()
		clipStack.Pop()

		if s.TabDragging && s.TabDragIdx >= 0 && s.TabDragIdx < len(*tabs) {
			dragMacro := op.Record(gtx.Ops)
			op.Offset(image.Pt(dragTabOX, dragTabOY)).Add(gtx.Ops)
			dIdx := s.TabDragIdx
			dTab := (*tabs)[dIdx]
			dW := dragTabW
			if dW <= 0 {
				dW = infos[dIdx].FinalWidth
			}
			dGtx := gtx
			dGtx.Constraints.Min = image.Pt(dW, tabHeight)
			dGtx.Constraints.Max = dGtx.Constraints.Min
			paint.FillShape(dGtx.Ops, theme.BgDragGhost, clip.Rect{Max: dGtx.Constraints.Min}.Op())
			paint.FillShape(dGtx.Ops, theme.Accent, clip.Rect{Max: image.Point{X: dW, Y: dGtx.Dp(unit.Dp(2))}}.Op())
			layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(1), Left: unit.Dp(10), Right: unit.Dp(6)}.Layout(dGtx, func(gtx layout.Context) layout.Dimensions {
				t := utils.SanitizeText(dTab.Title)
				t = strings.ReplaceAll(t, "\n", " ")
				if strings.TrimSpace(t) == "" {
					t = "New request"
				}
				if dTab.IsDirty {
					t = "● " + t
				}
				lbl := material.Label(th, unit.Sp(12), t)
				lbl.Color = theme.Fg
				lbl.MaxLines = 2
				lbl.Truncator = "..."
				return lbl.Layout(gtx)
			})
			op.Defer(gtx.Ops, dragMacro.Stop())
		}

		return listDims
	})
}
