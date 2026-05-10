package ui

import (
	"bytes"
	"image"
	"io"
	"os"
	"path/filepath"
	"time"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/environments"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/io/transfer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) layoutSidebar(gtx layout.Context) layout.Dimensions {
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size

	// Anchor the sidebar to a CursorDefault so children that don't set
	// a cursor of their own (material.Clickable / material.Button) don't
	// inherit one from a deeper hit-node — e.g. a widget.Editor whose
	// hit-area extends past its visible bounds via gtx.Constraints.Min
	// inflated by hint dimensions in material.EditorStyle.Layout.
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)

	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "text/plain"})
	event.Op(gtx.Ops, transfer.TargetFilter{Target: &ui.SidebarDropTag, Type: "application/json"})
	event.Op(gtx.Ops, &ui.ColList)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &ui.ColList,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok && ui.RenamingNode != nil {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}

	var moved bool
	var finalY float32
	var released bool

	for {
		e, ok := ui.SidebarEnvDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			ui.SidebarEnvDragY = e.Position.Y
		case pointer.Drag:
			finalY = e.Position.Y
			moved = true
		case pointer.Cancel, pointer.Release:
			released = true
		}
	}

	if moved {
		delta := finalY - ui.SidebarEnvDragY
		oldHeight := ui.SidebarEnvHeight
		ui.SidebarEnvHeight -= int(delta)
		minEnvHeight := gtx.Dp(unit.Dp(80))
		maxEnvHeight := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(80))
		if ui.SidebarEnvHeight < minEnvHeight {
			ui.SidebarEnvHeight = minEnvHeight
		}
		if ui.SidebarEnvHeight > maxEnvHeight && maxEnvHeight > minEnvHeight {
			ui.SidebarEnvHeight = maxEnvHeight
		}
		actualDelta := oldHeight - ui.SidebarEnvHeight
		ui.SidebarEnvDragY = finalY - float32(actualDelta)
		ui.Window.Invalidate()
	}
	if released {
		if ui.envRowH > 0 {
			snapped := ((ui.SidebarEnvHeight + ui.envRowH/2) / ui.envRowH) * ui.envRowH
			minEnvHeight := gtx.Dp(unit.Dp(80))
			maxEnvHeight := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(80))
			if snapped < minEnvHeight {
				snapped = minEnvHeight
			}
			if snapped > maxEnvHeight && maxEnvHeight > minEnvHeight {
				snapped = maxEnvHeight
			}
			ui.SidebarEnvHeight = snapped
		}
		ui.saveState()
	}

	borderLine := func(gtx layout.Context) layout.Dimensions {
		rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
		paint.FillShape(gtx.Ops, theme.Border, rect.Op())
		return layout.Dimensions{Size: rect.Max}
	}

	colsHeader := func(gtx layout.Context) layout.Dimensions {
		if ui.ColsHeaderClick.Clicked(gtx) {
			ui.ColsExpanded = !ui.ColsExpanded
		}
		for ui.ImportBtn.Clicked(gtx) {
			go func() {
				file, err := ui.Explorer.ChooseFile("json")
				if err == nil && file != nil {
					data, err := io.ReadAll(file)
					_ = file.Close()
					if err == nil {
						id := persist.NewRandomID()
						col, err := collections.ParseCollection(bytes.NewReader(data), id)
						if err == nil && col != nil {
							if werr := persist.AtomicWriteFile(filepath.Join(persist.CollectionsDir(), id+".json"), data); werr == nil {
								ui.ColLoadedChan <- &collections.CollectionUI{Data: col}
								ui.Window.Invalidate()
							}
						}
					}
				}
			}()
		}
		for ui.AddColBtn.Clicked(gtx) {
			ui.addNewCollection()
		}

		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &ui.ColsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								ic := widgets.IconChevronR
								if ui.ColsExpanded {
									ic = widgets.IconChevronD
								}
								size := gtx.Dp(unit.Dp(18))
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return ic.Layout(gtx, theme.FgMuted)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(12), "Collections")
								return lbl.Layout(gtx)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.Theme, &ui.AddColBtn, "+")
					btn.Background = theme.Border
					btn.Color = ui.Theme.Fg
					btn.TextSize = unit.Sp(11)
					btn.CornerRadius = unit.Dp(0)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(0)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.Theme, &ui.ImportBtn, "Import")
					btn.Background = theme.VarFound
					btn.Color = theme.Fg
					btn.TextSize = unit.Sp(11)
					btn.CornerRadius = unit.Dp(0)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)}
					return btn.Layout(gtx)
				}),
			)
		})
	}

	colsBody := func(gtx layout.Context) layout.Dimensions {
		// Anchor the collections list. A row in rename mode hosts a
		// widget.Editor whose hit-area can extend past the visible
		// row, leaking CursorText to neighbour rows that have no
		// cursor of their own (drag/hover hit-nodes don't set one).
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		pointer.CursorDefault.Add(gtx.Ops)

		if len(ui.Collections) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(12), "No collections loaded")
				lbl.Color = theme.FgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			})
		}

		commitRename := func(n *collections.CollectionNode) {
			if n == nil || !n.IsRenaming {
				return
			}
			n.Name = n.NameEditor.Text()
			if n.Request != nil {
				n.Request.Name = n.Name
			}
			if n.Parent == nil && n.Collection != nil {
				n.Collection.Name = n.Name
			}
			n.IsRenaming = false
			n.RenamingFocused = false
			if ui.RenamingNode == n {
				ui.RenamingNode = nil
			}
			ui.markCollectionDirty(n.Collection)
		}

		var updateCols bool

		nodeClickFn := func(n *collections.CollectionNode) {
			if ui.RenamingNode != nil && ui.RenamingNode != n {
				commitRename(ui.RenamingNode)
			}
			if n.IsRenaming {
				return
			}
			if !n.LastClickAt.IsZero() && gtx.Now.Sub(n.LastClickAt) < 300*time.Millisecond {
				n.IsRenaming = true
				n.NameEditor.SetText(n.Name)
				ui.RenamingNode = n
				n.LastClickAt = time.Time{}
				return
			}
			n.LastClickAt = gtx.Now
			if n.IsFolder {
				n.Expanded = !n.Expanded
				updateCols = true
			} else if n.Request != nil {
				ui.openRequestInTab(n)
			}
		}

		preDragSlop := float32(gtx.Dp(unit.Dp(4)))
		if dragged := ui.DraggedNode; dragged != nil {
			for {
				e, ok := dragged.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Drag:
					if ui.DraggedNode != dragged {
						continue
					}
					ui.DragNodeCurrentY = e.Position.Y
					ui.DragNodeCurrentX = e.Position.X
					dy := ui.DragNodeCurrentY - ui.DragNodeOriginY
					if dy < 0 {
						dy = -dy
					}
					if !ui.DragNodeActive && dy > preDragSlop {
						ui.DragNodeActive = true
						ui.DragNodeOriginY = ui.DragNodeCurrentY
						ui.DragNodeOriginX = ui.DragNodeCurrentX
					}
				case pointer.Release:
					if ui.DraggedNode == dragged {
						if ui.DragNodeActive {
							ui.DragNodeCurrentY = e.Position.Y
							ui.DragNodeCurrentX = e.Position.X
							ui.commitNodeDrop(dragged, gtx.Metric)
							updateCols = true
						} else {
							nodeClickFn(dragged)
						}
						ui.DraggedNode = nil
						ui.DragNodeActive = false
					}
				case pointer.Cancel:
					if ui.DraggedNode == dragged {
						ui.DraggedNode = nil
						ui.DragNodeActive = false
					}
				}
			}
		}

		var draggingNode bool
		draggedNodeVisibleIdx := -1
		if ui.DraggedNode != nil && ui.DragNodeActive {
			for i, n := range ui.VisibleCols {
				if n == ui.DraggedNode {
					draggedNodeVisibleIdx = i
					break
				}
			}
			if draggedNodeVisibleIdx >= 0 {
				draggingNode = true
			}
		}

		colsSnapshot := ui.VisibleCols
		if draggingNode {
			colsSnapshot = ui.buildDisplayVisibleCols()
		}

		listFirst := ui.ColList.Position.First
		trackY := -ui.ColList.Position.Offset
		ui.colRowYs = make(map[int]int, len(colsSnapshot))
		ui.colAfterLastY = trackY

		dim := material.List(ui.Theme, &ui.ColList).Layout(gtx, len(colsSnapshot), func(gtx layout.Context, i int) layout.Dimensions {
			node := colsSnapshot[i]

			nodeClick := func() {
				nodeClickFn(node)
			}

			dragSlop := float32(gtx.Dp(unit.Dp(4)))
			for {
				e, ok := node.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					ui.DraggedNode = node
					ui.DragNodeOriginY = e.Position.Y
					ui.DragNodeCurrentY = e.Position.Y
					ui.DragNodeOriginX = e.Position.X
					ui.DragNodeCurrentX = e.Position.X
					ui.DragNodeActive = false
				case pointer.Drag:
					if ui.DraggedNode == node {
						ui.DragNodeCurrentY = e.Position.Y
						ui.DragNodeCurrentX = e.Position.X
						dy := ui.DragNodeCurrentY - ui.DragNodeOriginY
						if dy < 0 {
							dy = -dy
						}
						if !ui.DragNodeActive && dy > dragSlop {
							ui.DragNodeActive = true
							ui.DragNodeOriginY = ui.DragNodeCurrentY
							ui.DragNodeOriginX = ui.DragNodeCurrentX
						}
					}
				case pointer.Release:
					if ui.DraggedNode == node {
						if ui.DragNodeActive {
							ui.commitNodeDrop(node, gtx.Metric)
							updateCols = true
						} else {
							nodeClick()
						}
					}
					ui.DraggedNode = nil
					ui.DragNodeActive = false
				case pointer.Cancel:
					ui.DraggedNode = nil
					ui.DragNodeActive = false
				}
			}

			if node.IsRenaming {
				for {
					ev, ok := node.NameEditor.Update(gtx)
					if !ok {
						break
					}
					if _, ok := ev.(widget.SubmitEvent); ok {
						commitRename(node)
					}
				}

				for {
					ev, ok := gtx.Event(
						key.Filter{Focus: &node.NameEditor, Name: "S", Required: key.ModShortcut},
						key.Filter{Focus: &node.NameEditor, Name: key.NameEscape},
					)
					if !ok {
						break
					}
					if e, ok := ev.(key.Event); ok && e.State == key.Press {
						if e.Name == key.NameEscape {
							node.IsRenaming = false
							node.RenamingFocused = false
							if ui.RenamingNode == node {
								ui.RenamingNode = nil
							}
						} else {
							commitRename(node)
						}
					}
				}
			}

			if node.IsRenaming {
				ui.RenamingNode = node
				if gtx.Focused(&node.NameEditor) {
					node.RenamingFocused = true
				} else if node.RenamingFocused {
					commitRename(node)
				} else {
					gtx.Execute(key.FocusCmd{Tag: &node.NameEditor})
				}
			}

			for node.MenuBtn.Clicked(gtx) {
				if ui.RenamingNode != nil && ui.RenamingNode != node {
					commitRename(ui.RenamingNode)
				}
				if !node.MenuOpen {
					for _, n := range ui.VisibleCols {
						n.MenuOpen = false
					}
				}
				node.MenuOpen = !node.MenuOpen
				updateCols = true
			}

			if node.MenuOpen {
				for node.AddReqBtn.Clicked(gtx) {
					commitRename(ui.RenamingNode)
					newNode := &collections.CollectionNode{
						Name:       "New Request",
						Request:    &model.ParsedRequest{Method: "GET"},
						Depth:      node.Depth + 1,
						Parent:     node,
						Collection: node.Collection,
						IsRenaming: true,
					}
					newNode.NameEditor.SingleLine = true
					newNode.NameEditor.Submit = true
					newNode.NameEditor.SetText("New Request")
					newNode.NameEditor.SetCaret(0, len([]rune(newNode.Name)))
					node.Children = append(node.Children, newNode)
					node.Expanded = true
					node.MenuOpen = false
					ui.RenamingNode = newNode
					updateCols = true
					ui.markCollectionDirty(node.Collection)
				}

				for node.AddFldBtn.Clicked(gtx) {
					commitRename(ui.RenamingNode)
					newNode := &collections.CollectionNode{
						Name:       "New Folder",
						IsFolder:   true,
						Depth:      node.Depth + 1,
						Parent:     node,
						Collection: node.Collection,
						IsRenaming: true,
					}
					newNode.NameEditor.SingleLine = true
					newNode.NameEditor.Submit = true
					newNode.NameEditor.SetText("New Folder")
					newNode.NameEditor.SetCaret(0, len([]rune(newNode.Name)))
					node.Children = append(node.Children, newNode)
					node.Expanded = true
					node.MenuOpen = false
					ui.RenamingNode = newNode
					updateCols = true
					ui.markCollectionDirty(node.Collection)
				}

				for node.EditBtn.Clicked(gtx) {
					commitRename(ui.RenamingNode)
					node.IsRenaming = true
					node.NameEditor.SetText(node.Name)
					node.MenuOpen = false
					ui.RenamingNode = node
				}

				for node.DupBtn.Clicked(gtx) {
					commitRename(ui.RenamingNode)
					if node.Parent != nil {
						dup := collections.CloneNode(node, node.Parent)
						recalcDepth(dup, node.Depth)
						node.Parent.Children = append(node.Parent.Children, dup)
						dup.IsRenaming = true
						dup.NameEditor.SetText(dup.Name)
						dup.NameEditor.SetCaret(0, len([]rune(dup.Name)))
						ui.RenamingNode = dup
						ui.markCollectionDirty(node.Collection)
					} else {
						newCol := &collections.ParsedCollection{
							ID:   persist.NewRandomID(),
							Name: node.Name + " Copy",
						}
						dupRoot := collections.CloneNode(node, nil)
						dupRoot.Collection = newCol
						newCol.Root = dupRoot
						collections.AssignParents(dupRoot, nil, newCol)
						recalcDepth(dupRoot, 0)
						ui.Collections = append(ui.Collections, &collections.CollectionUI{Data: newCol})
						dupRoot.IsRenaming = true
						dupRoot.NameEditor.SetText(dupRoot.Name)
						dupRoot.NameEditor.SetCaret(0, len([]rune(dupRoot.Name)))
						ui.RenamingNode = dupRoot
						ui.markCollectionDirty(newCol)
						ui.saveState()
					}
					node.MenuOpen = false
					updateCols = true
				}

				for node.DelBtn.Clicked(gtx) {
					if ui.RenamingNode != nil {
						if _, isRemoved := collections.CollectSubtree(node)[ui.RenamingNode]; isRemoved {
							ui.RenamingNode = nil
						}
					}
					removed := collections.CollectSubtree(node)
					if node.Parent != nil {
						for idx, c := range node.Parent.Children {
							if c == node {
								node.Parent.Children = append(node.Parent.Children[:idx], node.Parent.Children[idx+1:]...)
								break
							}
						}
						ui.markCollectionDirty(node.Collection)
					} else {
						colID := node.Collection.ID
						for idx, c := range ui.Collections {
							if c.Data == node.Collection {
								ui.Collections = append(ui.Collections[:idx], ui.Collections[idx+1:]...)
								break
							}
						}
						delete(ui.dirtyCollections, colID)
						if ui.deletedCollections == nil {
							ui.deletedCollections = make(map[string]struct{})
						}
						ui.deletedCollections[colID] = struct{}{}
						_ = os.Remove(filepath.Join(persist.CollectionsDir(), colID+".json"))
						ui.saveState()
					}
					for i := len(ui.Tabs) - 1; i >= 0; i-- {
						if _, ok := removed[ui.Tabs[i].LinkedNode]; ok {
							ui.closeTab(i)
						}
					}
					node.MenuOpen = false
					updateCols = true
				}
			}

			isPlaceholder := draggingNode && node == ui.DraggedNode

			rowDim := layout.Inset{
				Top: unit.Dp(1), Bottom: unit.Dp(1),
				Left: unit.Dp(0), Right: unit.Dp(0),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				isActiveNode := false
				if len(ui.Tabs) > 0 && ui.ActiveIdx >= 0 && ui.ActiveIdx < len(ui.Tabs) {
					isActiveNode = ui.Tabs[ui.ActiveIdx].LinkedNode == node
				}

				nodeHovered := node.Hover.Update(gtx.Source)
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						if !isPlaceholder {
							rect := clip.UniformRRect(image.Rectangle{Max: size}, 4)
							switch {
							case isActiveNode:
								paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: size}.Op())
							case nodeHovered:
								paint.FillShape(gtx.Ops, theme.BgHover, rect.Op(gtx.Ops))
							}
							defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
							node.Drag.Add(gtx.Ops)
							node.Hover.Add(gtx.Ops)
						}
						return layout.Dimensions{Size: size}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						contentMacro := op.Record(gtx.Ops)
						contentDim := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.X = gtx.Constraints.Max.X
								return layout.Inset{
									Top: unit.Dp(2), Bottom: unit.Dp(2),
									Left:  unit.Dp(float32(node.Depth * 12)),
									Right: unit.Dp(4),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									children := make([]layout.FlexChild, 0, 3)
									if node.IsFolder {
										children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											ic := widgets.IconChevronR
											if node.Expanded {
												ic = widgets.IconChevronD
											}
											size := gtx.Dp(unit.Dp(18))
											gtx.Constraints.Min = image.Pt(size, size)
											gtx.Constraints.Max = gtx.Constraints.Min
											return ic.Layout(gtx, theme.FgMuted)
										}))
										children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
										children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if node.IsRenaming {
												return widgets.InlineRenameField(gtx, ui.Theme, &node.NameEditor)
											}
											lbl := material.Label(ui.Theme, unit.Sp(12), node.Name)
											lbl.Alignment = text.Start
											if node.Depth == 0 {
												lbl.Font.Weight = font.Bold
											}
											return layout.W.Layout(gtx, lbl.Layout)
										}))
									} else if node.Request != nil {
										children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.Theme, unit.Sp(10), node.Request.Method)
											lbl.Color = theme.MethodColor(node.Request.Method)
											return lbl.Layout(gtx)
										}))
										children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
										children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if node.IsRenaming {
												return widgets.InlineRenameField(gtx, ui.Theme, &node.NameEditor)
											}
											lbl := material.Label(ui.Theme, unit.Sp(12), node.Name)
											lbl.Alignment = text.Start
											return layout.W.Layout(gtx, lbl.Layout)
										}))
									}
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								btn := material.Button(ui.Theme, &node.MenuBtn, "⋮")
								btn.Background = theme.Transparent
								btn.Color = ui.Theme.Fg
								btn.Inset = layout.UniformInset(unit.Dp(2))
								btn.TextSize = unit.Sp(14)
								dims := btn.Layout(gtx)
								node.MenuBtnWidth = dims.Size.X
								return dims
							}),
						)
						contentCall := contentMacro.Stop()
						if !isPlaceholder {
							contentCall.Add(gtx.Ops)
						}
						return contentDim
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if !node.MenuOpen {
							return layout.Dimensions{}
						}

						macro := op.Record(gtx.Ops)
						menuWidth := gtx.Dp(unit.Dp(166))
						menuX := gtx.Constraints.Max.X - menuWidth
						if menuX < 0 {
							menuX = 0
						}
						op.Offset(image.Pt(menuX, gtx.Dp(unit.Dp(24)))).Add(gtx.Ops)
						widget.Border{
							Color:        theme.BorderLight,
							CornerRadius: unit.Dp(4),
							Width:        unit.Dp(1),
						}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Stack{}.Layout(gtx,
								layout.Expanded(func(gtx layout.Context) layout.Dimensions {
									paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
									defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
									event.Op(gtx.Ops, &node.MenuOpen)
									for {
										_, ok := gtx.Event(pointer.Filter{Target: &node.MenuOpen, Kinds: pointer.Press})
										if !ok {
											break
										}
									}
									return layout.Dimensions{Size: gtx.Constraints.Min}
								}),
								layout.Stacked(func(gtx layout.Context) layout.Dimensions {
									return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										actions := make([]layout.FlexChild, 0, 5)
										if node.IsFolder || node.Depth == 0 {
											actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, ui.Theme, &node.AddReqBtn, "Add Request", widgets.IconAddReq)
											}))
											actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, ui.Theme, &node.AddFldBtn, "Add Folder", widgets.IconAddFld)
											}))
										}
										actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOption(gtx, ui.Theme, &node.EditBtn, "Rename", widgets.IconRename)
										}))
										if node.Depth > 0 {
											actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, ui.Theme, &node.DupBtn, "Duplicate", widgets.IconDup)
											}))
										}
										actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOption(gtx, ui.Theme, &node.DelBtn, "Delete", widgets.IconDel)
										}))
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx, actions...)
									})
								}),
							)
						})
						call := macro.Stop()
						op.Defer(gtx.Ops, call)

						return layout.Dimensions{}
					}),
				)
			})
			if i == 0 && rowDim.Size.Y > 0 {
				ui.colRowH = rowDim.Size.Y
			}
			if i >= listFirst {
				ui.colRowYs[i] = trackY
				trackY += rowDim.Size.Y
				ui.colAfterLastY = trackY
			}
			return rowDim
		})

		rowYAt := func(idx int) int {
			if y, ok := ui.colRowYs[idx]; ok {
				return y
			}
			if idx >= len(colsSnapshot) {
				return ui.colAfterLastY
			}
			return idx * ui.colRowH
		}

		if draggingNode && ui.colRowH > 0 && draggedNodeVisibleIdx >= 0 && ui.DraggedNode != nil {
			rowW := dim.Size.X
			if rowW <= 0 {
				rowW = gtx.Constraints.Max.X
			}
			srcOverlayY := rowYAt(draggedNodeVisibleIdx)
			draggedRowH := ui.colRowH
			if draggedNodeVisibleIdx+1 < len(colsSnapshot) {
				if nextY, ok := ui.colRowYs[draggedNodeVisibleIdx+1]; ok {
					if h := nextY - srcOverlayY; h > 0 {
						draggedRowH = h
					}
				}
			} else if h := ui.colAfterLastY - srcOverlayY; h > 0 && draggedNodeVisibleIdx >= listFirst {
				draggedRowH = h
			}
			hitMacro := op.Record(gtx.Ops)
			hitOff := op.Offset(image.Pt(0, srcOverlayY)).Push(gtx.Ops)
			hitClip := clip.Rect{Max: image.Pt(rowW, draggedRowH)}.Push(gtx.Ops)
			ui.DraggedNode.Drag.Add(gtx.Ops)
			hitClip.Pop()
			hitOff.Pop()
			op.Defer(gtx.Ops, hitMacro.Stop())

			ghostY := srcOverlayY + int(ui.DragNodeCurrentY-ui.DragNodeOriginY)
			if ghostY < 0 {
				ghostY = 0
			}
			if maxGhost := dim.Size.Y - draggedRowH; maxGhost > 0 && ghostY > maxGhost {
				ghostY = maxGhost
			}
			ghostMacro := op.Record(gtx.Ops)
			ghostOff := op.Offset(image.Pt(0, ghostY)).Push(gtx.Ops)
			ghostGtx := gtx
			ghostGtx.Constraints.Min = image.Pt(rowW, 0)
			ghostGtx.Constraints.Max = image.Pt(rowW, draggedRowH)
			renderNodeGhost(ghostGtx, ui.Theme, ui.DraggedNode)
			ghostOff.Pop()
			op.Defer(gtx.Ops, ghostMacro.Stop())

			if drop, ok := ui.dragNodeDrop(gtx.Metric); ok {
				if drop.intoNode != nil {
					targetIdx := -1
					for i, n := range colsSnapshot {
						if n == drop.intoNode {
							targetIdx = i
							break
						}
					}
					if targetIdx >= 0 {
						hY := rowYAt(targetIdx)
						hH := ui.colRowH
						if targetIdx+1 < len(colsSnapshot) {
							if nextY, ok := ui.colRowYs[targetIdx+1]; ok {
								if h := nextY - hY; h > 0 {
									hH = h
								}
							}
						} else if h := ui.colAfterLastY - hY; h > 0 {
							hH = h
						}
						hMacro := op.Record(gtx.Ops)
						hOff := op.Offset(image.Pt(0, hY)).Push(gtx.Ops)
						paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: image.Pt(rowW, hH)}.Op())
						hOff.Pop()
						op.Defer(gtx.Ops, hMacro.Stop())
					}
				} else {
					dropY := rowYAt(drop.lineIdx)
					lineH := gtx.Dp(unit.Dp(2))
					if lineH < 1 {
						lineH = 1
					}
					lineTop := dropY - lineH/2
					if lineTop < 0 {
						lineTop = 0
					}
					if maxLine := dim.Size.Y - lineH; maxLine > 0 && lineTop > maxLine {
						lineTop = maxLine
					}
					lineLeft := gtx.Dp(unit.Dp(float32(drop.lineDepth * 12)))
					if lineLeft >= rowW {
						lineLeft = 0
					}
					lineMacro := op.Record(gtx.Ops)
					lineOff := op.Offset(image.Pt(lineLeft, lineTop)).Push(gtx.Ops)
					paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Pt(rowW-lineLeft, lineH)}.Op())
					lineOff.Pop()
					op.Defer(gtx.Ops, lineMacro.Stop())
				}
			}
		}

		if updateCols {
			ui.updateVisibleCols()
		}

		return dim
	}

	envsHeader := func(gtx layout.Context) layout.Dimensions {
		if ui.EnvsHeaderClick.Clicked(gtx) {
			ui.EnvsExpanded = !ui.EnvsExpanded
		}
		for ui.ImportEnvBtn.Clicked(gtx) {
			go func() {
				file, err := ui.Explorer.ChooseFile("json")
				if err == nil && file != nil {
					data, err := io.ReadAll(file)
					_ = file.Close()
					if err == nil {
						id := persist.NewRandomID()
						env, err := environments.ParseEnvironment(bytes.NewReader(data), id)
						if err == nil && env != nil {
							if werr := persist.AtomicWriteFile(filepath.Join(persist.EnvironmentsDir(), id+".json"), data); werr == nil {
								ui.EnvLoadedChan <- &environments.EnvironmentUI{Data: env}
								ui.Window.Invalidate()
							}
						}
					}
				}
			}()
		}
		for ui.AddEnvBtn.Clicked(gtx) {
			ui.addNewEnvironment()
		}

		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &ui.EnvsHeaderClick, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								ic := widgets.IconChevronR
								if ui.EnvsExpanded {
									ic = widgets.IconChevronD
								}
								size := gtx.Dp(unit.Dp(18))
								gtx.Constraints.Min = image.Pt(size, size)
								gtx.Constraints.Max = gtx.Constraints.Min
								return ic.Layout(gtx, theme.FgMuted)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(ui.Theme, unit.Sp(12), "Environments")
								return lbl.Layout(gtx)
							}),
						)
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.Theme, &ui.AddEnvBtn, "+")
					btn.Background = theme.Border
					btn.Color = ui.Theme.Fg
					btn.TextSize = unit.Sp(11)
					btn.CornerRadius = unit.Dp(0)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(0)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(ui.Theme, &ui.ImportEnvBtn, "Import")
					btn.Background = theme.VarFound
					btn.Color = theme.Fg
					btn.TextSize = unit.Sp(11)
					btn.CornerRadius = unit.Dp(0)
					btn.Inset = layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)}
					return btn.Layout(gtx)
				}),
			)
		})
	}

	envsBody := func(gtx layout.Context) layout.Dimensions {
		// Same reason as colsBody — env rename uses widget.Editor.
		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		pointer.CursorDefault.Add(gtx.Ops)

		if len(ui.Environments) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(12), "No environments loaded")
				lbl.Color = theme.FgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			})
		}

		envClickFn := func(env *environments.EnvironmentUI) {
			if env.IsRenaming {
				return
			}
			if !env.LastClickAt.IsZero() && gtx.Now.Sub(env.LastClickAt) < 300*time.Millisecond {
				env.IsRenaming = true
				env.InlineNameEd.SingleLine = true
				env.InlineNameEd.Submit = true
				env.InlineNameEd.SetText(env.Data.Name)
				env.LastClickAt = time.Time{}
				return
			}
			env.LastClickAt = gtx.Now
			if ui.ActiveEnvID == env.Data.ID {
				ui.ActiveEnvID = ""
			} else {
				ui.ActiveEnvID = env.Data.ID
			}
			ui.activeEnvDirty = true
			ui.saveState()
			ui.Window.Invalidate()
		}

		preEnvSlop := float32(gtx.Dp(unit.Dp(4)))
		if dragged := ui.DraggedEnv; dragged != nil {
			for {
				e, ok := dragged.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Drag:
					if ui.DraggedEnv != dragged {
						continue
					}
					ui.DragEnvCurrentY = e.Position.Y
					dy := ui.DragEnvCurrentY - ui.DragEnvOriginY
					if dy < 0 {
						dy = -dy
					}
					if !ui.DragEnvActive && dy > preEnvSlop {
						ui.DragEnvActive = true
						ui.DragEnvOriginY = ui.DragEnvCurrentY
					}
				case pointer.Release:
					if ui.DraggedEnv == dragged {
						if ui.DragEnvActive {
							ui.DragEnvCurrentY = e.Position.Y
							ui.commitEnvDrop(dragged)
						} else {
							envClickFn(dragged)
						}
						ui.DraggedEnv = nil
						ui.DragEnvActive = false
					}
				case pointer.Cancel:
					if ui.DraggedEnv == dragged {
						ui.DraggedEnv = nil
						ui.DragEnvActive = false
					}
				}
			}
		}

		envSnapshot := ui.Environments

		var draggingEnv bool
		draggedSrcIdx := -1
		if ui.DraggedEnv != nil && ui.DragEnvActive {
			for i, e := range ui.Environments {
				if e == ui.DraggedEnv {
					draggedSrcIdx = i
					break
				}
			}
			if draggedSrcIdx >= 0 {
				draggingEnv = true
			}
		}

		var envToDelete *environments.EnvironmentUI
		envList := material.List(ui.Theme, &ui.EnvList)
		envList.AnchorStrategy = material.Overlay
		dim := envList.Layout(gtx, len(envSnapshot), func(gtx layout.Context, idx int) layout.Dimensions {
			if idx >= len(envSnapshot) {
				return layout.Dimensions{}
			}
			env := envSnapshot[idx]
			isActive := ui.ActiveEnvID == env.Data.ID

			envClick := func() {
				envClickFn(env)
			}

			dragSlop := float32(gtx.Dp(unit.Dp(4)))
			for {
				e, ok := env.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					ui.DraggedEnv = env
					ui.DragEnvOriginY = e.Position.Y
					ui.DragEnvCurrentY = e.Position.Y
					ui.DragEnvActive = false
				case pointer.Drag:
					if ui.DraggedEnv == env {
						ui.DragEnvCurrentY = e.Position.Y
						dy := ui.DragEnvCurrentY - ui.DragEnvOriginY
						if dy < 0 {
							dy = -dy
						}
						if !ui.DragEnvActive && dy > dragSlop {
							ui.DragEnvActive = true
							ui.DragEnvOriginY = ui.DragEnvCurrentY
						}
					}
				case pointer.Release:
					if ui.DraggedEnv == env {
						if ui.DragEnvActive {
							ui.commitEnvDrop(env)
						} else {
							envClick()
						}
					}
					ui.DraggedEnv = nil
					ui.DragEnvActive = false
				case pointer.Cancel:
					ui.DraggedEnv = nil
					ui.DragEnvActive = false
				}
			}

			isEnvPlaceholder := draggingEnv && env == ui.DraggedEnv

			rowDim := layout.Inset{Left: unit.Dp(0), Right: unit.Dp(0), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X

				commitEnvRename := func(e *environments.EnvironmentUI) {
					if !e.IsRenaming {
						return
					}
					name := e.InlineNameEd.Text()
					if name != "" {
						e.Data.Name = name
						_ = persist.SaveEnvironment(e.Data)
					}
					e.IsRenaming = false
					e.RenamingFocused = false
				}

				if env.IsRenaming {
					for {
						ev, ok := env.InlineNameEd.Update(gtx)
						if !ok {
							break
						}
						if _, ok := ev.(widget.SubmitEvent); ok {
							commitEnvRename(env)
						}
					}
					for {
						ev, ok := gtx.Event(
							key.Filter{Focus: &env.InlineNameEd, Name: key.NameEscape},
						)
						if !ok {
							break
						}
						if e, ok := ev.(key.Event); ok && e.State == key.Press && e.Name == key.NameEscape {
							env.IsRenaming = false
							env.RenamingFocused = false
						}
					}
					if gtx.Focused(&env.InlineNameEd) {
						env.RenamingFocused = true
					} else if env.RenamingFocused {
						commitEnvRename(env)
					} else {
						gtx.Execute(key.FocusCmd{Tag: &env.InlineNameEd})
					}
				}

				for env.EditBtn.Clicked(gtx) {
					if ui.EditingEnv != nil && ui.EditingEnv != env {
						ui.commitEditingEnv()
					}
					ui.pendingEnvClose = nil
					ui.EditingEnv = env
					env.InitEditor()
					env.MenuOpen = false
					ui.Window.Invalidate()
				}
				for env.DelBtn.Clicked(gtx) {
					envToDelete = env
					env.MenuOpen = false
				}

				envHovered := env.Hover.Update(gtx.Source)
				bgColor := theme.BgDark
				if isActive {
					bgColor = theme.Bg
				}
				if envHovered {
					bgColor = theme.BgHover
				}

				for env.MenuBtn.Clicked(gtx) {
					if !env.MenuOpen {
						for _, e := range ui.Environments {
							e.MenuOpen = false
						}
					}
					env.MenuOpen = !env.MenuOpen
					if env.MenuOpen {
						env.MenuClickY = widgets.GlobalPointerPos.Y
					}
				}
				if env.MenuOpen {
					for env.RenameBtn.Clicked(gtx) {
						env.IsRenaming = true
						env.InlineNameEd.SingleLine = true
						env.InlineNameEd.Submit = true
						env.InlineNameEd.SetText(env.Data.Name)
						env.MenuOpen = false
					}
					for env.DupBtn.Clicked(gtx) {
						ui.duplicateEnvironment(env)
						env.MenuOpen = false
					}
				}

				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						size := gtx.Constraints.Min
						rect := clip.UniformRRect(image.Rectangle{Max: size}, 4)
						if !isEnvPlaceholder {
							paint.FillShape(gtx.Ops, bgColor, rect.Op(gtx.Ops))
							if isActive {
								paint.FillShape(gtx.Ops, environments.HighlightColor(env.Data), clip.Rect{Max: image.Point{X: gtx.Dp(unit.Dp(2)), Y: size.Y}}.Op())
							}
							defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
							env.Drag.Add(gtx.Ops)
							env.Hover.Add(gtx.Ops)
						}
						return layout.Dimensions{Size: size}
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						if isEnvPlaceholder {
							rowH := ui.envRowH
							if rowH <= 0 {
								rowH = gtx.Dp(unit.Dp(30))
							}
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, rowH-gtx.Dp(unit.Dp(4)))}
						}
						return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										if env.IsRenaming {
											return widgets.InlineRenameField(gtx, ui.Theme, &env.InlineNameEd)
										}
										lbl := material.Label(ui.Theme, unit.Sp(12), env.Data.Name)
										lbl.MaxLines = 1
										if isActive {
											lbl.Font.Weight = font.Bold
										}
										return env.NameScroll.Layout(gtx, ui.Theme, lbl)
									})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									for env.SelectBtn.Clicked(gtx) {
										if ui.EnvColorPicker.IsOpen() && ui.EnvColorEnvID == env.Data.ID {
											ui.EnvColorPicker.Close()
										} else {
											ui.EnvColorEnvID = env.Data.ID
											ui.EnvColorPicker.Open(colorpicker.KindEnv, 0, environments.HighlightColor(env.Data), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
										}
									}
									return material.Clickable(gtx, &env.SelectBtn, func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(18)
										gtx.Constraints.Min = image.Pt(size, size)
										gtx.Constraints.Max = gtx.Constraints.Min
										swatch := environments.HighlightColor(env.Data)
										border := gtx.Dp(unit.Dp(1))
										if border < 1 {
											border = 1
										}
										paint.FillShape(gtx.Ops, theme.BorderLight, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 3).Op(gtx.Ops))
										inner := image.Rect(border, border, size-border, size-border)
										paint.FillShape(gtx.Ops, swatch, clip.UniformRRect(inner, 2).Op(gtx.Ops))
										return layout.Dimensions{Size: gtx.Constraints.Min}
									})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return material.Clickable(gtx, &env.MenuBtn, func(gtx layout.Context) layout.Dimensions {
										size := gtx.Dp(18)
										gtx.Constraints.Min = image.Pt(size, size)
										gtx.Constraints.Max = gtx.Constraints.Min
										iconCol := theme.FgMuted
										if env.MenuBtn.Hovered() {
											iconCol = ui.Theme.Fg
										}
										return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(ui.Theme, unit.Sp(14), "⋮")
											lbl.Color = iconCol
											return lbl.Layout(gtx)
										})
									})
								}),
							)
						})
					}),
					layout.Stacked(func(gtx layout.Context) layout.Dimensions {
						if !env.MenuOpen {
							return layout.Dimensions{}
						}
						macro := op.Record(gtx.Ops)
						menuWidth := gtx.Dp(unit.Dp(166))
						menuHeight := gtx.Dp(unit.Dp(150))
						menuX := gtx.Constraints.Max.X - menuWidth
						if menuX < 0 {
							menuX = 0
						}
						menuY := gtx.Dp(unit.Dp(24))
						windowH := ui.windowSize.Y
						if windowH > 0 && int(env.MenuClickY)+menuHeight > windowH {
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
									event.Op(gtx.Ops, &env.MenuOpen)
									for {
										_, ok := gtx.Event(pointer.Filter{Target: &env.MenuOpen, Kinds: pointer.Press})
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
												return widgets.MenuOption(gtx, ui.Theme, &env.EditBtn, "Edit", widgets.IconSettings)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, ui.Theme, &env.RenameBtn, "Rename", widgets.IconRename)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, ui.Theme, &env.DupBtn, "Duplicate", widgets.IconDup)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOptionDanger(gtx, ui.Theme, &env.DelBtn, "Delete", widgets.IconDel)
											}),
										)
									})
								}),
							)
						})
						call := macro.Stop()
						op.Defer(gtx.Ops, call)
						return layout.Dimensions{}
					}),
				)
			})
			if idx == 0 && rowDim.Size.Y > 0 {
				ui.envRowH = rowDim.Size.Y
			}
			return rowDim
		})
		if draggingEnv && ui.envRowH > 0 && draggedSrcIdx >= 0 && ui.DraggedEnv != nil {
			rowW := dim.Size.X
			if rowW <= 0 {
				rowW = gtx.Constraints.Max.X
			}
			srcOverlayY := draggedSrcIdx * ui.envRowH
			hitMacro := op.Record(gtx.Ops)
			hitOff := op.Offset(image.Pt(0, srcOverlayY)).Push(gtx.Ops)
			hitClip := clip.Rect{Max: image.Pt(rowW, ui.envRowH)}.Push(gtx.Ops)
			ui.DraggedEnv.Drag.Add(gtx.Ops)
			hitClip.Pop()
			hitOff.Pop()
			op.Defer(gtx.Ops, hitMacro.Stop())

			ghostY := srcOverlayY + int(ui.DragEnvCurrentY-ui.DragEnvOriginY)
			if ghostY < 0 {
				ghostY = 0
			}
			if maxGhost := dim.Size.Y - ui.envRowH; maxGhost > 0 && ghostY > maxGhost {
				ghostY = maxGhost
			}
			ghostMacro := op.Record(gtx.Ops)
			ghostOff := op.Offset(image.Pt(0, ghostY)).Push(gtx.Ops)
			ghostGtx := gtx
			ghostGtx.Constraints.Min = image.Pt(rowW, 0)
			ghostGtx.Constraints.Max = image.Pt(rowW, ui.envRowH)
			renderEnvGhost(ghostGtx, ui.Theme, ui.DraggedEnv)
			ghostOff.Pop()
			op.Defer(gtx.Ops, ghostMacro.Stop())

			if target := ui.dragEnvDropTargetIdx(); target >= 0 {
				var dropY int
				if target <= draggedSrcIdx {
					dropY = target * ui.envRowH
				} else {
					dropY = (target + 1) * ui.envRowH
				}
				lineH := gtx.Dp(unit.Dp(2))
				if lineH < 1 {
					lineH = 1
				}
				lineTop := dropY - lineH/2
				if lineTop < 0 {
					lineTop = 0
				}
				if maxLine := dim.Size.Y - lineH; maxLine > 0 && lineTop > maxLine {
					lineTop = maxLine
				}
				lineMacro := op.Record(gtx.Ops)
				lineOff := op.Offset(image.Pt(0, lineTop)).Push(gtx.Ops)
				paint.FillShape(gtx.Ops, theme.Accent, clip.Rect{Max: image.Pt(rowW, lineH)}.Op())
				lineOff.Pop()
				op.Defer(gtx.Ops, lineMacro.Stop())
			}
		}
		if envToDelete != nil {
			ui.deleteEnvironment(envToDelete)
		}
		return dim
	}

	envDivider := func(gtx layout.Context) layout.Dimensions {
		hit := gtx.Dp(unit.Dp(6))
		size := image.Point{X: gtx.Constraints.Max.X, Y: hit}
		lineCol := theme.Border
		if ui.SidebarEnvDrag.Dragging() {
			lineCol = theme.Accent
		}
		vis := gtx.Dp(unit.Dp(1))
		lineY := hit - vis
		paint.FillShape(gtx.Ops, lineCol, clip.Rect{Min: image.Pt(0, lineY), Max: image.Pt(size.X, lineY+vis)}.Op())

		defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
		pointer.CursorRowResize.Add(gtx.Ops)
		ui.SidebarEnvDrag.Add(gtx.Ops)
		event.Op(gtx.Ops, &ui.SidebarEnvDrag)
		for {
			_, ok := gtx.Event(pointer.Filter{Target: &ui.SidebarEnvDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
		}
		return layout.Dimensions{Size: size}
	}

	children := []layout.FlexChild{
		layout.Rigid(colsHeader),
		layout.Rigid(borderLine),
	}

	switch {
	case ui.ColsExpanded && ui.EnvsExpanded:
		remaining := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(62))
		if remaining < 2 {
			remaining = 2
		}
		envPx := ui.SidebarEnvHeight
		minPx := gtx.Dp(unit.Dp(80))
		if envPx < minPx {
			envPx = minPx
		}
		if envPx > remaining-minPx {
			envPx = remaining - minPx
		}
		if envPx < 1 {
			envPx = 1
		}
		colsWeight := float32(remaining - envPx)
		envsWeight := float32(envPx)
		children = append(children,
			layout.Flexed(colsWeight, colsBody),
			layout.Rigid(envDivider),
			layout.Rigid(envsHeader),
			layout.Rigid(borderLine),
			layout.Flexed(envsWeight, envsBody),
		)
	case ui.ColsExpanded:
		children = append(children,
			layout.Flexed(1, colsBody),
			layout.Rigid(envsHeader),
			layout.Rigid(borderLine),
		)
	case ui.EnvsExpanded:
		children = append(children,
			layout.Rigid(envsHeader),
			layout.Rigid(borderLine),
			layout.Flexed(1, envsBody),
		)
	default:
		children = append(children,
			layout.Rigid(envsHeader),
			layout.Rigid(borderLine),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y)}
			}),
		)
	}

	gutter := func(gtx layout.Context) layout.Dimensions {
		gutterW := gtx.Dp(unit.Dp(36))
		h := gtx.Constraints.Max.Y
		if h == 0 {
			h = gtx.Constraints.Min.Y
		}
		gtx.Constraints.Min = image.Pt(gutterW, h)
		gtx.Constraints.Max = image.Pt(gutterW, h)
		btnH := gtx.Dp(unit.Dp(52))
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(gutterW, btnH)
				gtx.Constraints.Max = image.Pt(gutterW, btnH)
				return ui.layoutSidebarToggleBtn(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}
			}),
		)
		if !ui.Settings.HideSidebar {
			line := gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Min: image.Pt(gutterW-line, 0), Max: image.Pt(gutterW, h)}.Op())
		}
		return layout.Dimensions{Size: image.Pt(gutterW, h)}
	}

	if ui.Settings.HideSidebar {
		return gutter(gtx)
	}

	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(gutter),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

func renderNodeGhost(gtx layout.Context, th *material.Theme, node *collections.CollectionNode) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	rowH := gtx.Constraints.Max.Y
	if rowH <= 0 {
		rowH = gtx.Dp(unit.Dp(24))
	}
	return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, rowH-gtx.Dp(unit.Dp(2)))
		if size.Y <= 0 {
			size.Y = gtx.Dp(unit.Dp(20))
		}
		rect := clip.UniformRRect(image.Rectangle{Max: size}, 4)
		paint.FillShape(gtx.Ops, theme.BgDragGhost, rect.Op(gtx.Ops))
		widgets.PaintBorder1px(gtx, size, theme.Accent)
		gtx.Constraints.Min = size
		gtx.Constraints.Max = size
		return layout.Inset{
			Top: unit.Dp(2), Bottom: unit.Dp(2),
			Left:  unit.Dp(float32(node.Depth * 12)),
			Right: unit.Dp(4),
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, 3)
			if node.IsFolder {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					ic := widgets.IconChevronR
					if node.Expanded {
						ic = widgets.IconChevronD
					}
					sz := gtx.Dp(unit.Dp(18))
					gtx.Constraints.Min = image.Pt(sz, sz)
					gtx.Constraints.Max = gtx.Constraints.Min
					return ic.Layout(gtx, theme.FgMuted)
				}))
				children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), node.Name)
					lbl.Alignment = text.Start
					if node.Depth == 0 {
						lbl.Font.Weight = font.Bold
					}
					return layout.W.Layout(gtx, lbl.Layout)
				}))
			} else if node.Request != nil {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(10), node.Request.Method)
					lbl.Color = theme.MethodColor(node.Request.Method)
					return lbl.Layout(gtx)
				}))
				children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout))
				children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), node.Name)
					lbl.Alignment = text.Start
					return layout.W.Layout(gtx, lbl.Layout)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
		})
	})
}

func renderEnvGhost(gtx layout.Context, th *material.Theme, env *environments.EnvironmentUI) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	rowH := gtx.Constraints.Max.Y
	if rowH <= 0 {
		rowH = gtx.Dp(unit.Dp(30))
	}
	size := image.Pt(gtx.Constraints.Max.X, rowH)
	rect := clip.UniformRRect(image.Rectangle{Max: size}, 4)
	paint.FillShape(gtx.Ops, theme.BgDragGhost, rect.Op(gtx.Ops))
	widgets.PaintBorder1px(gtx, size, theme.Accent)
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), env.Data.Name)
					lbl.MaxLines = 1
					return layout.W.Layout(gtx, lbl.Layout)
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				sw := gtx.Dp(18)
				gtx.Constraints.Min = image.Pt(sw, sw)
				gtx.Constraints.Max = gtx.Constraints.Min
				border := gtx.Dp(unit.Dp(1))
				if border < 1 {
					border = 1
				}
				paint.FillShape(gtx.Ops, theme.BorderLight, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 3).Op(gtx.Ops))
				inner := image.Rect(border, border, sw-border, sw-border)
				paint.FillShape(gtx.Ops, environments.HighlightColor(env.Data), clip.UniformRRect(inner, 2).Op(gtx.Ops))
				return layout.Dimensions{Size: gtx.Constraints.Min}
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
		)
	})
}
