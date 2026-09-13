package sidebar

import (
	"image"
	"strings"
	"time"

	"rete/internal/ui/environments"
	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/gesture"
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

	Drag            gesture.Drag
	MenuBtn         widget.Clickable
	RenameBtn       widget.Clickable
	DupBtn          widget.Clickable
	DelBtn          widget.Clickable
	MenuOpen        bool
	MenuClickY      float32
	CtxMenu         environments.CtxMenuState
	IsRenaming      bool
	RenamingFocused bool
	NameEd          widget.Editor
	LastClickAt     time.Time

	RowHovered      bool
	MenuHovered     bool
	ContentHeightPx int
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

func scriptClick(gtx layout.Context, host *Host, row *ScriptRow, flowsMode bool) {
	if row.IsRenaming {
		return
	}
	isDouble := !row.LastClickAt.IsZero() && gtx.Now.Sub(row.LastClickAt) < 300*time.Millisecond
	if !flowsMode {
		if isDouble {
			row.LastClickAt = time.Time{}
			if host.OpenScript != nil {
				host.OpenScript(row.ID)
			}
			return
		}
		row.LastClickAt = gtx.Now
		return
	}
	if isDouble {
		row.startRename()
		row.LastClickAt = time.Time{}
		return
	}
	row.LastClickAt = gtx.Now
	if host.OpenScript != nil {
		host.OpenScript(row.ID)
	}
}

func scriptDragEvents(gtx layout.Context, host *Host, row *ScriptRow, flowsMode bool) {
	slop := float32(gtx.Dp(unit.Dp(4)))
	for {
		e, ok := row.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			*host.DraggedScript = row
			*host.DragScriptOriginY = e.Position.Y
			*host.DragScriptCurrentY = e.Position.Y
			*host.DragScriptActive = false
		case pointer.Drag:
			if *host.DraggedScript != row {
				continue
			}
			*host.DragScriptCurrentY = e.Position.Y
			dy := *host.DragScriptCurrentY - *host.DragScriptOriginY
			if dy < 0 {
				dy = -dy
			}
			if !*host.DragScriptActive && dy > slop {
				*host.DragScriptActive = true
				*host.DragScriptOriginY = *host.DragScriptCurrentY
			}
		case pointer.Release:
			if *host.DraggedScript != row {
				continue
			}
			if *host.DragScriptActive {
				*host.DragScriptCurrentY = e.Position.Y
				commitScriptDrop(host, row)
			} else {
				scriptClick(gtx, host, row, flowsMode)
			}
			*host.DraggedScript = nil
			*host.DragScriptActive = false
		case pointer.Cancel:
			if *host.DraggedScript == row {
				*host.DraggedScript = nil
				*host.DragScriptActive = false
			}
		}
	}
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
		anchor := image.Pt(headerDims.Size.X, headerDims.Size.Y+gtx.Dp(unit.Dp(2)))
		widgets.DeferMenu(gtx, host.Theme, host.ScriptsMenuOpen, anchor, widgets.MenuMinWidthDp, []widgets.MenuItem{
			{Label: "Import", Click: host.ImportScriptBtn, Icon: widgets.IconDownload},
		})
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
	scriptsCut := listGutter(gtx, host.Theme, host.ScriptList)
	blockHovered := host.ScriptsBodyHover.Update(gtx.Source) || anyScriptMenuOpen ||
		host.ScriptList.Scrollbar.Dragging() || host.ScriptList.Scrollbar.IndicatorHovered() || host.ScriptList.Scrollbar.TrackHovered()
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

	scrollBarWheel(gtx, host.ScriptBarScroll, host.ScriptList)

	if dragged := *host.DraggedScript; dragged != nil {
		scriptDragEvents(gtx, host, dragged, flowsMode)
	}

	draggedSrcIdx := -1
	if *host.DraggedScript != nil && *host.DragScriptActive {
		for i, r := range rows {
			if r == *host.DraggedScript {
				draggedSrcIdx = i
				break
			}
		}
	}
	draggingScript := draggedSrcIdx >= 0

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
		scriptsScrollable := host.ScriptList.Position.First > 0 || host.ScriptList.Position.OffsetLast < 0
		overScrollbar := scriptsScrollable && scrollbarZoneHovered(gtx, pos.X, gtx.Constraints.Max.X)
		rel := pos.Y + float32(host.ScriptList.Position.Offset)
		if !overScrollbar && rel >= 0 {
			if idx := host.ScriptList.Position.First + int(rel)/rowH; idx >= 0 && idx < len(rows) {
				rows[idx].RowHovered = true
				rows[idx].MenuHovered = menuZoneHovered(gtx, pos.X, gtx.Constraints.Max.X)
			}
		}
	}

	scriptBarW := sidebarBarWidth(gtx, host.Theme, host.ScriptList)
	listMacro := op.Record(gtx.Ops)
	dim := host.ScriptList.List.Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		row := rows[i]
		isActive := row.ID == activeID

		scriptDragEvents(gtx, host, row, flowsMode)
		isPlaceholder := draggingScript && row == *host.DraggedScript

		for row.MenuBtn.Clicked(gtx) {
			if !row.MenuOpen {
				for _, r := range rows {
					r.MenuOpen = false
				}
			}
			row.MenuOpen = !row.MenuOpen
			row.CtxMenu.AtPointer = false
			if row.MenuOpen {
				row.MenuClickY = widgets.GlobalPointerPos.Y
			}
		}
		if pos, ok := pollCtxPress(gtx, &row.CtxMenu); ok {
			for _, r := range rows {
				r.MenuOpen = false
			}
			row.MenuOpen = true
			row.CtxMenu = environments.CtxMenuState{AtPointer: true, Pos: pos}
			row.MenuClickY = widgets.GlobalPointerPos.Y
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
			widgets.HandleEditorShortcuts(gtx, &row.NameEd)
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
					size := gtx.Constraints.Min
					surf := rowSurface(size, scriptsCut)
					if isPlaceholder {
						paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: surf}.Op())
						return layout.Dimensions{Size: size}
					}
					switch {
					case isActive:
						paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: surf}.Op())
					case rowHovered:
						paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: surf}.Op())
					case row.MenuOpen:
						paint.FillShape(gtx.Ops, menuRowBg(theme.BgDark), clip.Rect{Max: surf}.Op())
					}
					if row.MenuOpen {
						paintMenuOutline(gtx, surf)
					}
					defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
					row.Drag.Add(gtx.Ops)
					return layout.Dimensions{Size: size}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					if isPlaceholder {
						rowH := *host.ScriptRowH
						if rowH <= 0 {
							rowH = gtx.Dp(unit.Dp(24))
						}
						return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, rowH)}
					}
					d := layout.Inset{Left: unit.Dp(8), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								cd := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											size := gtx.Dp(unit.Dp(14))
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
								})
								row.ContentHeightPx = cd.Size.Y
								return cd
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								iconCol := theme.FgMuted
								if row.MenuHovered {
									iconCol = host.Theme.Fg
								}
								return rowIconBtn(gtx, &row.MenuBtn, row.MenuHovered, row.ContentHeightPx, fade, rowIcon(widgets.IconMore, fadeAlpha(iconCol, fade)))
							}),
						)
					})
					addCtxArea(gtx, &row.CtxMenu, d.Size)
					return d
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					if !row.MenuOpen {
						return layout.Dimensions{}
					}
					menuHeight := gtx.Dp(unit.Dp(90))
					menuY := gtx.Dp(unit.Dp(24))
					windowH := host.WindowSize.Y
					flip := windowH > 0 && int(row.MenuClickY)+menuHeight > windowH
					if flip {
						menuY = -menuHeight - gtx.Dp(unit.Dp(4))
					}
					anchor := widgets.MenuAnchor{
						Pt:         image.Pt(gtx.Constraints.Max.X, menuY),
						AlignRight: true,
						Clamp:      image.Pt(gtx.Constraints.Max.X, 0),
					}
					if row.CtxMenu.AtPointer {
						anchor = ctxMenuAnchor(gtx, row.CtxMenu.Pos, flip)
					}
					widgets.DeferMenuAt(gtx, host.Theme, &row.MenuOpen, anchor, widgets.MenuMinWidthDp, []widgets.MenuItem{
						{Label: "Rename", Click: &row.RenameBtn, Icon: widgets.IconRename},
						{Label: "Duplicate", Click: &row.DupBtn, Icon: widgets.IconDup},
						{Separator: true},
						{Label: "Delete", Click: &row.DelBtn, Icon: widgets.IconDel, Danger: true},
					})
					return layout.Dimensions{}
				}),
			)
		})
		if i == 0 && rowDim.Size.Y > 0 {
			*host.ScriptRowH = rowDim.Size.Y
		}
		return rowDim
	})
	listCall := listMacro.Stop()
	sbMacro := op.Record(gtx.Ops)
	layoutSidebarScrollbar(gtx, host.Theme, host.ScriptList, len(rows), dim.Size.Y, host.ScriptsBodyFade.Value())
	op.Defer(gtx.Ops, sbMacro.Stop())
	listCall.Add(gtx.Ops)
	if draggingScript && *host.ScriptRowH > 0 {
		layoutScriptDragOverlay(gtx, host, dim, draggedSrcIdx)
	}

	pass := pointer.PassOp{}.Push(gtx.Ops)
	ov := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	host.ScriptsBodyHover.Add(gtx.Ops)
	ov.Pop()
	pass.Pop()

	addScrollBarStrip(gtx, host.ScriptBarScroll, dim.Size, scriptBarW)

	return dim
}

func layoutScriptDragOverlay(gtx layout.Context, host *Host, dim layout.Dimensions, srcIdx int) {
	rowH := *host.ScriptRowH
	rowW := dim.Size.X
	if rowW <= 0 {
		rowW = gtx.Constraints.Max.X
	}
	srcOverlayY := (srcIdx-host.ScriptList.Position.First)*rowH - host.ScriptList.Position.Offset
	hitMacro := op.Record(gtx.Ops)
	hitOff := op.Offset(image.Pt(0, srcOverlayY)).Push(gtx.Ops)
	hitClip := clip.Rect{Max: image.Pt(rowW, rowH)}.Push(gtx.Ops)
	(*host.DraggedScript).Drag.Add(gtx.Ops)
	hitClip.Pop()
	hitOff.Pop()
	op.Defer(gtx.Ops, hitMacro.Stop())

	ghostY := srcOverlayY + int(*host.DragScriptCurrentY-*host.DragScriptOriginY)
	ghostY = max(0, ghostY)
	if maxGhost := dim.Size.Y - rowH; maxGhost > 0 && ghostY > maxGhost {
		ghostY = maxGhost
	}
	ghostMacro := op.Record(gtx.Ops)
	ghostOff := op.Offset(image.Pt(0, ghostY)).Push(gtx.Ops)
	ghostGtx := gtx
	ghostGtx.Constraints.Min = image.Pt(rowW, 0)
	ghostGtx.Constraints.Max = image.Pt(rowW, rowH)
	renderScriptGhost(ghostGtx, host.Theme, *host.DraggedScript)
	ghostOff.Pop()
	op.Defer(gtx.Ops, ghostMacro.Stop())

	target := dragScriptDropTargetIdx(host)
	if target < 0 {
		return
	}
	dropY := target * rowH
	if target > srcIdx {
		dropY = (target + 1) * rowH
	}
	lineH := max(1, gtx.Dp(unit.Dp(2)))
	lineTop := max(0, dropY-lineH/2)
	if maxLine := dim.Size.Y - lineH; maxLine > 0 && lineTop > maxLine {
		lineTop = maxLine
	}
	lineMacro := op.Record(gtx.Ops)
	lineOff := op.Offset(image.Pt(0, lineTop)).Push(gtx.Ops)
	paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Pt(rowW, lineH)}.Op())
	lineOff.Pop()
	op.Defer(gtx.Ops, lineMacro.Stop())
}

func renderScriptGhost(gtx layout.Context, th *material.Theme, row *ScriptRow) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	rowH := gtx.Constraints.Max.Y
	if rowH <= 0 {
		rowH = gtx.Dp(unit.Dp(24))
	}
	size := image.Pt(gtx.Constraints.Max.X, rowH)
	paint.FillShape(gtx.Ops, theme.BgDragGhost, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
	widgets.PaintBorder1px(gtx, size, theme.Accent)
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(rowIcon(widgets.IconLab, theme.FgMuted)),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), row.Name)
				lbl.MaxLines = 1
				lbl.Truncator = "…"
				lbl.LineHeightScale = 1.0
				return layout.W.Layout(gtx, lbl.Layout)
			}),
		)
	})
}
