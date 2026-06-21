package sidebar

import (
	"image"
	"strings"
	"time"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type ScriptRow struct {
	ID   string
	Name string

	NameClick       widget.Clickable
	MenuBtn         widget.Clickable
	RenameBtn       widget.Clickable
	DupBtn          widget.Clickable
	DelBtn          widget.Clickable
	MenuOpen        bool
	MenuClickY      float32
	IsRenaming      bool
	RenamingFocused bool
	NameEd          widget.Editor
	LastClickAt     time.Time

	// RowHovered/MenuHovered are recomputed each frame from the live pointer
	// position and the list geometry (see scriptsBody), not from Enter/Leave
	// events.
	RowHovered  bool
	MenuHovered bool
}

func (r *ScriptRow) startRename() {
	r.IsRenaming = true
	r.RenamingFocused = false
	r.NameEd.SingleLine = true
	r.NameEd.Submit = true
	r.NameEd.SetText(r.Name)
	r.NameEd.SetCaret(0, len([]rune(r.Name)))
}

func commitScriptRename(host *Host, r *ScriptRow) {
	if !r.IsRenaming {
		return
	}
	name := strings.TrimSpace(r.NameEd.Text())
	if name != "" && name != r.Name {
		r.Name = name
		if host.RenameScript != nil {
			host.RenameScript(r.ID, name)
		}
	}
	r.IsRenaming = false
	r.RenamingFocused = false
}

func scriptsHeader(gtx layout.Context, host *Host) layout.Dimensions {
	if host.ScriptsHeaderClick.Clicked(gtx) {
		*host.ScriptsExpanded = !*host.ScriptsExpanded
		host.Window.Invalidate()
	}
	for host.AddScriptBtn.Clicked(gtx) {
		if host.NewScript != nil {
			host.NewScript()
		}
	}
	for host.ScriptsMenuBtn.Clicked(gtx) {
		*host.ScriptsMenuOpen = !*host.ScriptsMenuOpen
	}
	for host.ImportScriptBtn.Clicked(gtx) {
		*host.ScriptsMenuOpen = false
		go func() {
			data, err := host.ChooseJSONFile()
			if err != nil || data == nil {
				return
			}
			if host.ImportScript != nil {
				host.ImportScript(data)
			}
		}()
	}

	headerDims := layout.Inset{Bottom: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				if host.ScriptsHeaderClick.Hovered() {
					paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
				}
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return host.ScriptsHeaderClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(26))
							pointer.CursorPointer.Add(gtx.Ops)
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
								layout.Rigid(sectionCount(host.Theme, len(*host.Scripts))),
								layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									lbl := material.Label(host.Theme, unit.Sp(12), "Scripts")
									lbl.LineHeightScale = 1.0
									return lbl.Layout(gtx)
								}),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.SquareBtnSized(gtx, host.AddScriptBtn, widgets.IconAdd, host.Theme, 26, 16)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return widgets.SquareBtnSized(gtx, host.ScriptsMenuBtn, widgets.IconMore, host.Theme, 26, 16)
					}),
				)
			}),
		)
	})

	if *host.ScriptsMenuOpen {
		macro := op.Record(gtx.Ops)
		op.Offset(image.Pt(headerDims.Size.X, 0)).Add(gtx.Ops)

		menuGtx := gtx
		menuGtx.Constraints.Min = image.Point{}
		rec := op.Record(gtx.Ops)
		menuDims := material.Clickable(menuGtx, host.ImportScriptBtn, func(gtx layout.Context) layout.Dimensions {
			if host.ImportScriptBtn.Hovered() {
				paint.FillShape(gtx.Ops, theme.BgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
			}
			pointer.CursorPointer.Add(gtx.Ops)
			return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return material.Label(host.Theme, unit.Sp(12), "Import").Layout(gtx)
			})
		})
		menuCall := rec.Stop()

		paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: menuDims.Size}, 4).Op(gtx.Ops))
		b := max(1, gtx.Dp(unit.Dp(1)))
		paint.FillShape(gtx.Ops, theme.BorderLight, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: menuDims.Size}, 4).Path(gtx.Ops), Width: float32(b)}.Op())
		menuCall.Add(gtx.Ops)

		op.Defer(gtx.Ops, macro.Stop())
	}

	return headerDims
}

func scriptsBody(gtx layout.Context, host *Host) layout.Dimensions {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)

	anyScriptMenuOpen := false
	for _, r := range *host.Scripts {
		if r.MenuOpen {
			anyScriptMenuOpen = true
			break
		}
	}
	blockHovered := host.ScriptsBodyHover.Update(gtx.Source) || anyScriptMenuOpen
	fade := host.ScriptsBodyFade.Update(gtx, blockHovered, 100*time.Millisecond)

	rows := *host.Scripts
	if len(rows) == 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(host.Theme, unit.Sp(12), "No scripts yet")
			lbl.Color = theme.FgMuted
			lbl.Alignment = text.Middle
			return lbl.Layout(gtx)
		})
	}

	activeID := ""
	flowsMode := host.SidebarSection != nil && *host.SidebarSection == "flows"
	if flowsMode && host.ActiveScriptID != nil {
		activeID = host.ActiveScriptID()
	}

	// Geometric hover (see sidebar.colsBody): the row under the pointer is
	// recomputed each frame from the body-local pointer position and the uniform
	// row height, so the highlight never lags a content shift.
	for _, r := range rows {
		r.RowHovered = false
		r.MenuHovered = false
	}
	if host.ScriptsBodyHover.Hovered() {
		rowH := *host.ScriptRowH
		if rowH <= 0 {
			rowH = gtx.Dp(unit.Dp(24))
		}
		pos := host.ScriptsBodyHover.Pos()
		rel := pos.Y + float32(host.ScriptList.Position.Offset)
		if rel >= 0 {
			if idx := host.ScriptList.Position.First + int(rel)/rowH; idx >= 0 && idx < len(rows) {
				rows[idx].RowHovered = true
				rows[idx].MenuHovered = menuZoneHovered(gtx, pos.X, gtx.Constraints.Max.X)
			}
		}
	}

	list := material.List(host.Theme, host.ScriptList)
	list.AnchorStrategy = material.Overlay
	list.Indicator.Color.A = uint8(float32(list.Indicator.Color.A) * fade)
	list.Indicator.HoverColor.A = uint8(float32(list.Indicator.HoverColor.A) * fade)
	dim := list.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		row := rows[i]
		isActive := row.ID == activeID

		for row.NameClick.Clicked(gtx) {
			if row.IsRenaming {
				continue
			}
			isDouble := !row.LastClickAt.IsZero() && gtx.Now.Sub(row.LastClickAt) < 300*time.Millisecond
			if !flowsMode {
				if isDouble {
					row.LastClickAt = time.Time{}
					if host.OpenScript != nil {
						host.OpenScript(row.ID)
					}
					continue
				}
				row.LastClickAt = gtx.Now
				continue
			}
			if isDouble {
				row.startRename()
				row.LastClickAt = time.Time{}
				continue
			}
			row.LastClickAt = gtx.Now
			if host.OpenScript != nil {
				host.OpenScript(row.ID)
			}
		}

		for row.MenuBtn.Clicked(gtx) {
			if !row.MenuOpen {
				for _, r := range rows {
					r.MenuOpen = false
				}
			}
			row.MenuOpen = !row.MenuOpen
			if row.MenuOpen {
				row.MenuClickY = widgets.GlobalPointerPos.Y
			}
		}
		if row.MenuOpen {
			for row.RenameBtn.Clicked(gtx) {
				row.startRename()
				row.MenuOpen = false
			}
			for row.DupBtn.Clicked(gtx) {
				if host.DuplicateScript != nil {
					host.DuplicateScript(row.ID)
				}
				row.MenuOpen = false
			}
			for row.DelBtn.Clicked(gtx) {
				if host.DeleteScript != nil {
					host.DeleteScript(row.ID)
				}
				row.MenuOpen = false
			}
		}

		if row.IsRenaming {
			for {
				ev, ok := row.NameEd.Update(gtx)
				if !ok {
					break
				}
				if _, ok := ev.(widget.SubmitEvent); ok {
					commitScriptRename(host, row)
				}
			}
			for {
				ev, ok := gtx.Event(
					key.Filter{Focus: &row.NameEd, Name: key.NameEscape},
				)
				if !ok {
					break
				}
				if e, ok := ev.(key.Event); ok && e.State == key.Press && e.Name == key.NameEscape {
					row.IsRenaming = false
					row.RenamingFocused = false
				}
			}
			if row.IsRenaming {
				if gtx.Focused(&row.NameEd) {
					row.RenamingFocused = true
				} else if row.RenamingFocused {
					commitScriptRename(host, row)
				} else {
					gtx.Execute(key.FocusCmd{Tag: &row.NameEd})
				}
			}
		}

		rowHovered := row.RowHovered

		rowDim := layout.Inset{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return row.NameClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						size := gtx.Constraints.Min
						switch {
						case isActive:
							paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: size}.Op())
						case rowHovered:
							paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: size}.Op())
						}
						return layout.Dimensions{Size: size}
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(unit.Dp(16))
										gtx.Constraints.Min = image.Pt(size, size)
										gtx.Constraints.Max = gtx.Constraints.Min
										col := theme.FgMuted
										if isActive {
											col = theme.Accent
										}
										return widgets.IconLab.Layout(gtx, col)
									}),
									layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										if row.IsRenaming {
											return widgets.InlineRenameField(gtx, host.Theme, &row.NameEd)
										}
										lbl := material.Label(host.Theme, unit.Sp(12), row.Name)
										lbl.MaxLines = 1
										lbl.Truncator = "…"
										lbl.LineHeightScale = 1.0
										return lbl.Layout(gtx)
									}),
								)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &row.MenuBtn, func(gtx layout.Context) layout.Dimensions {
									w := gtx.Dp(18)
									h := *host.ScriptRowH - 2*gtx.Dp(unit.Dp(4))
									if h <= 0 {
										h = w
									}
									gtx.Constraints.Min = image.Pt(w, h)
									gtx.Constraints.Max = gtx.Constraints.Min
									iconCol := theme.FgMuted
									if row.MenuHovered {
										iconCol = host.Theme.Fg
									}
									iconCol.A = uint8(float32(iconCol.A) * fade)
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										isz := gtx.Dp(16)
										gtx.Constraints.Min = image.Pt(isz, isz)
										gtx.Constraints.Max = gtx.Constraints.Min
										return widgets.IconMore.Layout(gtx, iconCol)
									})
								})
							}),
						)
					})
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if !row.MenuOpen {
						return layout.Dimensions{}
					}
					macro := op.Record(gtx.Ops)
					menuWidth := gtx.Dp(unit.Dp(128))
					menuHeight := gtx.Dp(unit.Dp(90))
					menuX := gtx.Constraints.Max.X - menuWidth
					if menuX < 0 {
						menuX = 0
					}
					menuY := gtx.Dp(unit.Dp(24))
					windowH := host.WindowSize.Y
					if windowH > 0 && int(row.MenuClickY)+menuHeight > windowH {
						menuY = -menuHeight - gtx.Dp(unit.Dp(4))
					}
					op.Offset(image.Pt(menuX, menuY)).Add(gtx.Ops)
					widget.Border{
						Color:        theme.BorderLight,
						CornerRadius: unit.Dp(4),
						Width:        unit.Dp(1),
					}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Stack{}.Layout(gtx,
							layout.Expanded(func(gtx layout.Context) layout.Dimensions {
								paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
								defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
								event.Op(gtx.Ops, &row.MenuOpen)
								for {
									_, ok := gtx.Event(pointer.Filter{Target: &row.MenuOpen, Kinds: pointer.Press})
									if !ok {
										break
									}
								}
								return layout.Dimensions{Size: gtx.Constraints.Min}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOptionCompact(gtx, host.Theme, &row.RenameBtn, "Rename", widgets.IconRename)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOptionCompact(gtx, host.Theme, &row.DupBtn, "Duplicate", widgets.IconDup)
										}),
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOptionDangerCompact(gtx, host.Theme, &row.DelBtn, "Delete", widgets.IconDel)
										}),
									)
								})
							}),
						)
					})
					op.Defer(gtx.Ops, macro.Stop())
					return layout.Dimensions{}
				}),
			)
		})
		if i == 0 && rowDim.Size.Y > 0 {
			*host.ScriptRowH = rowDim.Size.Y
		}
		return rowDim
	})

	pass := pointer.PassOp{}.Push(gtx.Ops)
	ov := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	host.ScriptsBodyHover.Add(gtx.Ops)
	ov.Pop()
	pass.Pop()

	return dim
}
