package workspace

import (
	"image"
	"io"
	"strings"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (t *RequestTab) layoutGraphQLBody(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string, ratio *float32, stacked, isDragging bool) layout.Dimensions {
	t.EnsureGQL()

	flexAxis := layout.Horizontal
	leftInset := layout.Inset{Right: unit.Dp(1)}
	rightInset := layout.Inset{Left: unit.Dp(1)}
	if stacked {
		flexAxis = layout.Vertical
		leftInset = layout.Inset{Bottom: unit.Dp(1)}
		rightInset = layout.Inset{Top: unit.Dp(1)}
	}

	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBar(gtx, th, &t.LayoutHorizBtn, &t.LayoutVertBtn, stacked)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: flexAxis}.Layout(gtx,
					layout.Flexed(*ratio, func(gtx layout.Context) layout.Dimensions {
						return leftInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return t.layoutGraphQLComposerPane(gtx, th, win, activeEnv)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						thick := gtx.Dp(unit.Dp(4))
						var size image.Point
						var cursor pointer.Cursor
						if stacked {
							size = image.Point{X: gtx.Constraints.Min.X, Y: thick}
							cursor = pointer.CursorRowResize
						} else {
							size = image.Point{X: thick, Y: gtx.Constraints.Min.Y}
							cursor = pointer.CursorColResize
						}
						rect := clip.Rect{Max: size}
						defer rect.Push(gtx.Ops).Pop()
						cursor.Add(gtx.Ops)
						t.SplitDrag.Add(gtx.Ops)
						for {
							_, ok := gtx.Event(pointer.Filter{Target: &t.SplitDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
							if !ok {
								break
							}
						}
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1-*ratio, func(gtx layout.Context) layout.Dimensions {
						return rightInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return t.layoutGraphQLResponsePane(gtx, th, win, isDragging)
						})
					}),
				)
			}),
		)
	})
}

func (t *RequestTab) layoutGraphQLComposerPane(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string) layout.Dimensions {
	g := t.EnsureGQL()

	for g.QueryCopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(g.Query.Text())),
		})
	}
	for g.VarsCopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(strings.NewReader(g.Variables.Text())),
		})
	}

	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutGraphQLHeadersHeader(gtx, th)
			}),
			layout.Rigid(wsHLine),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !t.HeadersExpanded {
					return layout.Dimensions{}
				}
				return t.layoutGraphQLHeadersList(gtx, th, activeEnv)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !t.HeadersExpanded {
					return layout.Dimensions{}
				}
				return wsHLine(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return t.layoutGraphQLQueryVars(gtx, th, win, activeEnv)
			}),
		)
	})
}

func (t *RequestTab) layoutGraphQLHeadersHeader(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(12), "Headers")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			}),
			layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return widgets.SquareBtn(gtx, &t.AddHeaderBtn, widgets.IconAdd, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				toggleIcon := widgets.IconExpandMore
				if t.HeadersExpanded {
					toggleIcon = widgets.IconExpandLess
				}
				return widgets.SquareBtn(gtx, &t.ViewGeneratedBtn, toggleIcon, th)
			}),
		)
	})
}

func (t *RequestTab) layoutGraphQLHeadersList(gtx layout.Context, th *material.Theme, env map[string]string) layout.Dimensions {
	h := gtx.Dp(unit.Dp(120))
	half := gtx.Constraints.Max.Y / 2
	if minH := gtx.Dp(unit.Dp(48)); half > minH && h > half {
		h = half
	}
	gtx.Constraints.Min.Y = h
	gtx.Constraints.Max.Y = h
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		if len(t.Headers) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := widgets.MonoLabel(th, unit.Sp(11), "No headers")
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			})
		}
		minKey := widgets.KVKeysMinWidth(gtx, th, len(t.Headers), func(i int) *widget.Editor { return &t.Headers[i].Key })
		return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return t.HeadersList.Layout(gtx, len(t.Headers), func(gtx layout.Context, i int) layout.Dimensions {
				hd := t.Headers[i]
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: unit.Dp(1), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return widgets.KVRow(gtx, th, &hd.Key, &hd.Value, &hd.DelBtn, &t.HeaderKeyW, &hd.SplitDrag, &hd.splitLastX, &t.HeaderKeyBelowMin, minKey, env)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i >= len(t.Headers)-1 {
							return layout.Dimensions{}
						}
						return rowDivider(gtx)
					}),
				)
			})
		})
	})
}

func (t *RequestTab) layoutGraphQLQueryVars(gtx layout.Context, th *material.Theme, win *app.Window, activeEnv map[string]string) layout.Dimensions {
	g := t.EnsureGQL()

	flexExtent := float32(gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(4)))
	var moved bool
	var finalPos float32
	var released bool
	for {
		e, ok := g.VarsSplitDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			g.VarsSplitDragX = e.Position.Y
		case pointer.Drag:
			finalPos = e.Position.Y
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}
	const minR, maxR = 0.15, 0.85
	if g.VarsSplitRatio < minR {
		g.VarsSplitRatio = minR
	}
	if g.VarsSplitRatio > maxR {
		g.VarsSplitRatio = maxR
	}
	if moved && flexExtent > 0 {
		delta := finalPos - g.VarsSplitDragX
		oldRatio := g.VarsSplitRatio
		g.VarsSplitRatio += delta / flexExtent
		if g.VarsSplitRatio < minR {
			g.VarsSplitRatio = minR
		} else if g.VarsSplitRatio > maxR {
			g.VarsSplitRatio = maxR
		}
		g.VarsSplitDragX = finalPos - ((g.VarsSplitRatio - oldRatio) * flexExtent)
		win.Invalidate()
	}
	if released {
		win.Invalidate()
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(g.VarsSplitRatio, func(gtx layout.Context) layout.Dimensions {
			return gqlEditorPanel(gtx, th, "Query", &g.Query, &g.QueryCopyBtn, "query { ... }")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			thick := gtx.Dp(unit.Dp(4))
			size := image.Point{X: gtx.Constraints.Min.X, Y: thick}
			rect := clip.Rect{Max: size}
			defer rect.Push(gtx.Ops).Pop()
			pointer.CursorRowResize.Add(gtx.Ops)
			g.VarsSplitDrag.Add(gtx.Ops)
			return layout.Dimensions{Size: size}
		}),
		layout.Flexed(1-g.VarsSplitRatio, func(gtx layout.Context) layout.Dimensions {
			return gqlEditorPanel(gtx, th, "Variables (JSON)", &g.Variables, &g.VarsCopyBtn, "{ }")
		}),
	)
}

func gqlEditorPanel(gtx layout.Context, th *material.Theme, title string, ed *widget.Editor, copyBtn *widget.Clickable, hint string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(12), title)
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						})
					}),
					layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.SquareBtn(gtx, copyBtn, iconCopy, th)
					}),
				)
			})
		}),
		layout.Rigid(wsHLine),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			bdr := gtx.Dp(unit.Dp(1))
			sz := gtx.Constraints.Max
			paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
			inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
			paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(inner).Op())
			gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
			gtx.Constraints.Max = gtx.Constraints.Min
			op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
			return layout.UniformInset(unit.Dp(6)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				e := material.Editor(th, ed, hint)
				e.TextSize = unit.Sp(12)
				e.HintColor = theme.FgMuted
				e.Font.Typeface = widgets.MonoTypeface
				widgets.HandleEditorShortcuts(gtx, ed)
				return e.Layout(gtx)
			})
		}),
	)
}

func (t *RequestTab) layoutGraphQLResponsePane(gtx layout.Context, th *material.Theme, win *app.Window, isDragging bool) layout.Dimensions {
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(2),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(28))
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								statusText := t.Status
								if t.isRequesting {
									if dl := t.downloadedBytes.Load(); dl > 0 {
										statusText = "Downloading... " + formatSize(dl)
									}
								}
								lbl := widgets.MonoLabel(th, unit.Sp(12), statusText)
								lbl.Font.Weight = font.Bold
								return lbl.Layout(gtx)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.SquareBtn(gtx, &t.SearchBtn, widgets.IconSearch, th)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.SquareBtn(gtx, &t.WrapBtn, iconWrap, th)
								}),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return widgets.SquareBtn(gtx, &t.CopyBtn, iconCopy, th)
								}),
							)
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
				return layout.Dimensions{Size: size}
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						return t.layoutResponseBody(gtx, th, win, isDragging)
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						return t.layoutSearchOverlay(gtx, th, &t.RespSearch)
					}),
				)
			}),
		)
	})
}
