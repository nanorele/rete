package sidebar

import (
	"bytes"
	"image"
	"path/filepath"
	"strconv"
	"strings"
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

func abbrevMethod(m string) string {
	m = strings.ToUpper(strings.TrimSpace(m))
	switch m {
	case "DELETE":
		return "DEL"
	case "OPTIONS":
		return "OPT"
	case "PATCH":
		return "PAT"
	case "TRACE":
		return "TRC"
	case "CONNECT":
		return "CONN"
	}
	if len(m) > 4 {
		return m[:4]
	}
	return m
}

func sectionCount(th *material.Theme, n int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		lbl := material.Label(th, unit.Sp(11), strconv.Itoa(n))
		lbl.Color = theme.FgHint
		lbl.LineHeightScale = 1.0
		return lbl.Layout(gtx)
	}
}

func headerMenuItem(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string) layout.Dimensions {
	return clk.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(150))
		if clk.Hovered() {
			paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
		}
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(12), label)
			lbl.LineHeightScale = 1.0
			return lbl.Layout(gtx)
		})
	})
}

func collectionMethodSet(n *collections.CollectionNode, set map[string]bool) {
	if n == nil {
		return
	}
	if !n.IsFolder && n.Request != nil {
		set[abbrevMethod(n.Request.Method)] = true
	}
	for _, c := range n.Children {
		collectionMethodSet(c, set)
	}
}

var DebugSticky func(first int, names []string)

var DebugBand func(reserve, fullBandH int)

// DebugBandSolid reports the OPAQUE contiguous band height (the filled background),
// which is <= the full band height while a folder header is still sliding in/out
// over its own real row (the gap between them is transparent). Used by tests that
// verify the band never paints an opaque "empty space" region.
var DebugBandSolid func(solidH int)

// DebugBandGeom reports the actually-drawn band lines (names and their rendered Y
// top positions) and the band bottom. Unlike DebugSticky (which reports only the
// logical reach-up chain), this includes the incoming folder rendered sliding into
// a slot during a seamless transition. Tests use it to verify band geometry.
var DebugBandGeom func(names []string, ys []int, bottom int)

func recalcDepth(node *collections.CollectionNode, depth int) {
	if node == nil {
		return
	}
	node.Depth = depth
	for _, child := range node.Children {
		recalcDepth(child, depth+1)
	}
}

func Layout(gtx layout.Context, host *Host) layout.Dimensions {
	host.ensureScripts()
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: size}.Op())
	gtx.Constraints.Min = size

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)

	event.Op(gtx.Ops, transfer.TargetFilter{Target: host.SidebarDropTag, Type: "text/plain"})
	event.Op(gtx.Ops, transfer.TargetFilter{Target: host.SidebarDropTag, Type: "application/json"})
	event.Op(gtx.Ops, host.ColList)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: host.ColList,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		if _, ok := ev.(pointer.Event); ok && *host.RenamingNode != nil {
			gtx.Execute(key.FocusCmd{Tag: nil})
		}
	}

	borderLine := func(gtx layout.Context) layout.Dimensions {
		rect := clip.Rect{Max: image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}}
		paint.FillShape(gtx.Ops, theme.BorderSubtle, rect.Op())
		return layout.Dimensions{Size: rect.Max}
	}

	colsHeader := func(gtx layout.Context) layout.Dimensions {
		if host.ColsHeaderClick.Clicked(gtx) {
			*host.ColsExpanded = !*host.ColsExpanded
			host.Window.Invalidate()
		}
		for host.ImportBtn.Clicked(gtx) {
			*host.ColsMenuOpen = false
			go func() {
				data, err := host.ChooseJSONFile()
				if err != nil || data == nil {
					return
				}
				id := persist.NewRandomID()
				col, err := collections.ParseCollection(bytes.NewReader(data), id)
				if err == nil && col != nil {
					if werr := persist.AtomicWriteFile(filepath.Join(persist.CollectionsDir(), id+".json"), data); werr == nil {
						host.PushColLoaded(&collections.CollectionUI{Data: col})
					}
				}
			}()
		}
		for host.AddColBtn.Clicked(gtx) {
			addNewCollection(host)
		}
		for host.ColsMenuBtn.Clicked(gtx) {
			*host.ColsMenuOpen = !*host.ColsMenuOpen
		}
		for host.ColsExpandAll.Clicked(gtx) {
			*host.ColsMenuOpen = false
			setAllCollectionsExpanded(host, true)
		}
		for host.ColsCollapseAll.Clicked(gtx) {
			*host.ColsMenuOpen = false
			setAllCollectionsExpanded(host, false)
		}

		headerDims := layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(0), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					if host.ColsHeaderClick.Hovered() {
						paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
					}
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return host.ColsHeaderClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(26))
								pointer.CursorPointer.Add(gtx.Ops)
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
									layout.Rigid(sectionCount(host.Theme, len(*host.Collections))),
									layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										lbl := material.Label(host.Theme, unit.Sp(12), "Collections")
										lbl.LineHeightScale = 1.0
										return lbl.Layout(gtx)
									}),
								)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, host.AddColBtn, widgets.IconAdd, host.Theme, 26, 16)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, host.ColsMenuBtn, widgets.IconMore, host.Theme, 26, 16)
						}),
					)
				}),
			)
		})

		if *host.ColsMenuOpen {
			macro := op.Record(gtx.Ops)
			op.Offset(image.Pt(headerDims.Size.X, 0)).Add(gtx.Ops)

			menuGtx := gtx
			menuGtx.Constraints.Min = image.Point{}
			rec := op.Record(gtx.Ops)
			menuDims := layout.Flex{Axis: layout.Vertical}.Layout(menuGtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return headerMenuItem(gtx, host.Theme, host.ImportBtn, "Import")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return headerMenuItem(gtx, host.Theme, host.ColsExpandAll, "Expand all")
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return headerMenuItem(gtx, host.Theme, host.ColsCollapseAll, "Collapse all")
				}),
			)
			menuCall := rec.Stop()

			paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: menuDims.Size}, 4).Op(gtx.Ops))
			b := max(1, gtx.Dp(unit.Dp(1)))
			paint.FillShape(gtx.Ops, theme.BorderLight, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: menuDims.Size}, 4).Path(gtx.Ops), Width: float32(b)}.Op())
			menuCall.Add(gtx.Ops)

			op.Defer(gtx.Ops, macro.Stop())
		}

		return headerDims
	}

	colsBody := func(gtx layout.Context) layout.Dimensions {

		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		pointer.CursorDefault.Add(gtx.Ops)

		anyColMenuOpen := false
		for _, n := range *host.VisibleCols {
			if n.MenuOpen {
				anyColMenuOpen = true
				break
			}
		}
		blockHovered := host.ColsBodyHover.Update(gtx.Source) || anyColMenuOpen
		fade := host.ColsBodyFade.Update(gtx, blockHovered, 100*time.Millisecond)

		if len(*host.Collections) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(host.Theme, unit.Sp(12), "No collections loaded")
				lbl.Color = theme.FgMuted
				lbl.Alignment = text.Middle
				return lbl.Layout(gtx)
			})
		}

		commitRename := func(n *collections.CollectionNode) {
			if n == nil || !n.IsRenaming {
				return
			}
			newName := strings.TrimSpace(n.NameEditor.Text())
			if newName == "" {
				n.NameEditor.SetText(n.Name)
				n.IsRenaming = false
				n.RenamingFocused = false
				if *host.RenamingNode == n {
					*host.RenamingNode = nil
				}
				return
			}
			n.Name = newName
			if n.Request != nil {
				n.Request.Name = n.Name
			}
			if n.Parent == nil && n.Collection != nil {
				n.Collection.Name = n.Name
			}
			n.IsRenaming = false
			n.RenamingFocused = false
			if *host.RenamingNode == n {
				*host.RenamingNode = nil
			}
			host.MarkCollectionDirty(n.Collection)
		}

		var updateCols bool
		flowsMode := host.SidebarSection != nil && *host.SidebarSection == "flows"

		nodeClickFn := func(n *collections.CollectionNode, inTextZone bool) {
			if *host.RenamingNode != nil && *host.RenamingNode != n {
				commitRename(*host.RenamingNode)
			}
			if n.IsRenaming {
				return
			}
			if flowsMode {
				isDouble := !n.LastClickAt.IsZero() && gtx.Now.Sub(n.LastClickAt) < 300*time.Millisecond
				if isDouble {
					n.LastClickAt = time.Time{}
					if host.SwitchSection != nil {
						host.SwitchSection("requests")
					}
					if n.IsFolder {
						if !n.Expanded {
							n.Expanded = true
							updateCols = true
						}
					} else if n.Request != nil {
						host.OpenRequestInTab(n)
					}
					return
				}
				n.LastClickAt = gtx.Now
				if n.IsFolder {
					n.Expanded = !n.Expanded
					if !n.Expanded {
						n.ResetSubtreeHover()
					}
					updateCols = true
				}
				return
			}
			if inTextZone && !n.LastClickAt.IsZero() && gtx.Now.Sub(n.LastClickAt) < 300*time.Millisecond {
				n.IsRenaming = true
				n.NameEditor.SetText(n.Name)
				*host.RenamingNode = n
				n.LastClickAt = time.Time{}
				return
			}
			if inTextZone {
				n.LastClickAt = gtx.Now
			} else {
				n.LastClickAt = time.Time{}
			}
			if n.IsFolder {
				n.Expanded = !n.Expanded
				if !n.Expanded {
					n.ResetSubtreeHover()
				}
				updateCols = true
			} else if n.Request != nil && !flowsMode {
				host.OpenRequestInTab(n)
			}
		}

		measureLabelWidth := func(gtx layout.Context, th *material.Theme, s string, bold bool, sz unit.Sp) int {
			if s == "" {
				return 0
			}
			g := gtx
			g.Constraints.Min.X = 0
			g.Constraints.Max.X = 1 << 24
			mm := op.Record(gtx.Ops)
			lbl := material.Label(th, sz, s)
			lbl.Alignment = text.Start
			if bold {
				lbl.Font.Weight = font.Bold
			}
			d := lbl.Layout(g)
			mm.Stop()
			return d.Size.X
		}

		isTextHit := func(n *collections.CollectionNode, x float32) bool {
			if n.NameWidthPx <= 0 {
				return false
			}
			rightPad := float32(gtx.Dp(unit.Dp(6)))
			return x >= float32(n.NameLeftPx) && x <= float32(n.NameLeftPx+n.NameWidthPx)+rightPad
		}

		colMethodW := make(map[*collections.ParsedCollection]int, len(*host.Collections))
		for _, cu := range *host.Collections {
			if cu == nil || cu.Data == nil || cu.Data.Root == nil {
				continue
			}
			set := make(map[string]bool, 4)
			collectionMethodSet(cu.Data.Root, set)
			w := 0
			for m := range set {
				if mw := measureLabelWidth(gtx, host.Theme, m, false, unit.Sp(10)); mw > w {
					w = mw
				}
			}
			colMethodW[cu.Data] = w
		}

		renameFieldSized := func(gtx layout.Context, th *material.Theme, ed *widget.Editor, bold bool, sz unit.Sp) layout.Dimensions {
			txt := ed.Text()
			if txt == "" {
				txt = " "
			}
			measuredW := measureLabelWidth(gtx, th, txt, bold, sz)

			sidePad := gtx.Dp(unit.Dp(4))
			caretRoom := gtx.Dp(unit.Dp(8))
			desiredW := measuredW + 2*sidePad + caretRoom
			minW := gtx.Dp(unit.Dp(80))
			if desiredW < minW {
				desiredW = minW
			}
			if desiredW > gtx.Constraints.Max.X {
				desiredW = gtx.Constraints.Max.X
			}

			inGtx := gtx
			inGtx.Constraints.Min.X = desiredW
			inGtx.Constraints.Max.X = desiredW

			return widgets.InlineRenameFieldPadded(inGtx, th, ed, unit.Dp(2))
		}

		preDragSlop := float32(gtx.Dp(unit.Dp(4)))
		if dragged := *host.DraggedNode; dragged != nil {
			for {
				e, ok := dragged.Drag.Update(gtx.Metric, gtx.Source, gesture.Both)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Drag:
					if *host.DraggedNode != dragged {
						continue
					}
					*host.DragNodeCurrentY = e.Position.Y
					*host.DragNodeCurrentX = e.Position.X
					if host.DragNodeWinOrig != nil {
						*host.DragNodeWinPos = host.DragNodeWinOrig.Add(e.Position)
					}
					dy := *host.DragNodeCurrentY - *host.DragNodeOriginY
					if dy < 0 {
						dy = -dy
					}
					dx := *host.DragNodeCurrentX - *host.DragNodeOriginX
					if dx < 0 {
						dx = -dx
					}
					if !*host.DragNodeActive && (dy > preDragSlop || dx > preDragSlop) {
						*host.DragNodeActive = true
						*host.DragNodeOriginY = *host.DragNodeCurrentY
						*host.DragNodeOriginX = *host.DragNodeCurrentX
					}
				case pointer.Release:
					if *host.DraggedNode == dragged {
						if *host.DragNodeActive {
							*host.DragNodeCurrentY = e.Position.Y
							*host.DragNodeCurrentX = e.Position.X
							if host.DragNodeWinOrig != nil {
								*host.DragNodeWinPos = host.DragNodeWinOrig.Add(e.Position)
							}
							if (host.DropNodeExternal == nil || !host.DropNodeExternal(dragged)) && !flowsMode {
								commitNodeDrop(host, dragged, gtx.Metric)
							}
							updateCols = true
						} else {
							nodeClickFn(dragged, isTextHit(dragged, e.Position.X))
						}
						*host.DraggedNode = nil
						*host.DragNodeActive = false
					}
				case pointer.Cancel:
					if *host.DraggedNode == dragged {
						*host.DraggedNode = nil
						*host.DragNodeActive = false
					}
				}
			}
		}

		var draggingNode bool
		draggedNodeVisibleIdx := -1
		if *host.DraggedNode != nil && *host.DragNodeActive {
			for i, n := range *host.VisibleCols {
				if n == *host.DraggedNode {
					draggedNodeVisibleIdx = i
					break
				}
			}
			if draggedNodeVisibleIdx >= 0 {
				draggingNode = true
			}
		}

		colsSnapshot := *host.VisibleCols

		for _, n := range colsSnapshot {
			n.Hover.Update(gtx.Source)
		}
		if hoverDebug {
			labels := make([]string, len(colsSnapshot))
			hovers := make([]*widgets.Hover, len(colsSnapshot))
			for i, n := range colsSnapshot {
				labels[i] = n.Name
				hovers[i] = &n.Hover
			}
			logHoverStates("col", labels, hovers, host.ColList.Position.First, host.ColList.Position.Count)
		}

		listFirst := host.ColList.Position.First
		trackY := -host.ColList.Position.Offset
		(*host.ColRowYs) = make(map[int]int, len(colsSnapshot))
		*host.ColAfterLastY = trackY

		colList := material.List(host.Theme, host.ColList)
		colList.AnchorStrategy = material.Overlay
		colList.Indicator.Color.A = uint8(float32(colList.Indicator.Color.A) * fade)
		colList.Indicator.HoverColor.A = uint8(float32(colList.Indicator.HoverColor.A) * fade)
		dim := colList.Layout(gtx, len(colsSnapshot), func(gtx layout.Context, i int) layout.Dimensions {
			node := colsSnapshot[i]

			nodeClick := func(x float32) {
				nodeClickFn(node, isTextHit(node, x))
			}

			dragSlop := float32(gtx.Dp(unit.Dp(4)))
			for {
				e, ok := node.Drag.Update(gtx.Metric, gtx.Source, gesture.Both)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					*host.DraggedNode = node
					*host.DragNodeOriginY = e.Position.Y
					*host.DragNodeCurrentY = e.Position.Y
					*host.DragNodeOriginX = e.Position.X
					*host.DragNodeCurrentX = e.Position.X
					*host.DragNodeActive = false
					if host.DragNodeWinOrig != nil {
						*host.DragNodeWinOrig = widgets.GlobalPointerPos.Sub(e.Position)
						*host.DragNodeWinPos = widgets.GlobalPointerPos
					}
				case pointer.Drag:
					if *host.DraggedNode == node {
						*host.DragNodeCurrentY = e.Position.Y
						*host.DragNodeCurrentX = e.Position.X
						if host.DragNodeWinOrig != nil {
							*host.DragNodeWinPos = host.DragNodeWinOrig.Add(e.Position)
						}
						dy := *host.DragNodeCurrentY - *host.DragNodeOriginY
						if dy < 0 {
							dy = -dy
						}
						dx := *host.DragNodeCurrentX - *host.DragNodeOriginX
						if dx < 0 {
							dx = -dx
						}
						if !*host.DragNodeActive && (dy > dragSlop || dx > dragSlop) {
							*host.DragNodeActive = true
							*host.DragNodeOriginY = *host.DragNodeCurrentY
							*host.DragNodeOriginX = *host.DragNodeCurrentX
						}
					}
				case pointer.Release:
					if *host.DraggedNode == node {
						if *host.DragNodeActive {
							if host.DragNodeWinOrig != nil {
								*host.DragNodeWinPos = host.DragNodeWinOrig.Add(e.Position)
							}
							if (host.DropNodeExternal == nil || !host.DropNodeExternal(node)) && !flowsMode {
								commitNodeDrop(host, node, gtx.Metric)
							}
							updateCols = true
						} else {
							nodeClick(e.Position.X)
						}
					}
					*host.DraggedNode = nil
					*host.DragNodeActive = false
				case pointer.Cancel:
					*host.DraggedNode = nil
					*host.DragNodeActive = false
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
							if *host.RenamingNode == node {
								*host.RenamingNode = nil
							}
						} else {
							commitRename(node)
						}
					}
				}
			}

			if node.IsRenaming {
				*host.RenamingNode = node
				if gtx.Focused(&node.NameEditor) {
					node.RenamingFocused = true
				} else if node.RenamingFocused {
					commitRename(node)
				} else {
					gtx.Execute(key.FocusCmd{Tag: &node.NameEditor})
				}
			}

			for node.MenuBtn.Clicked(gtx) {
				if *host.RenamingNode != nil && *host.RenamingNode != node {
					commitRename(*host.RenamingNode)
				}
				if !node.MenuOpen {
					for _, n := range *host.VisibleCols {
						n.MenuOpen = false
					}
				}
				node.MenuOpen = !node.MenuOpen
				updateCols = true
			}

			if node.MenuOpen {
				for node.AddReqBtn.Clicked(gtx) {
					commitRename(*host.RenamingNode)
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
					*host.RenamingNode = newNode
					updateCols = true
					host.MarkCollectionDirty(node.Collection)
				}

				for node.AddFldBtn.Clicked(gtx) {
					commitRename(*host.RenamingNode)
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
					*host.RenamingNode = newNode
					updateCols = true
					host.MarkCollectionDirty(node.Collection)
				}

				for node.EditBtn.Clicked(gtx) {
					commitRename(*host.RenamingNode)
					node.IsRenaming = true
					node.NameEditor.SetText(node.Name)
					node.MenuOpen = false
					*host.RenamingNode = node
				}

				for node.DupBtn.Clicked(gtx) {
					commitRename(*host.RenamingNode)
					if node.Parent != nil {
						dup := collections.CloneNode(node, node.Parent)
						recalcDepth(dup, node.Depth)
						node.Parent.Children = append(node.Parent.Children, dup)
						dup.IsRenaming = true
						dup.NameEditor.SetText(dup.Name)
						dup.NameEditor.SetCaret(0, len([]rune(dup.Name)))
						*host.RenamingNode = dup
						host.MarkCollectionDirty(node.Collection)
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
						*host.Collections = append(*host.Collections, &collections.CollectionUI{Data: newCol})
						dupRoot.IsRenaming = true
						dupRoot.NameEditor.SetText(dupRoot.Name)
						dupRoot.NameEditor.SetCaret(0, len([]rune(dupRoot.Name)))
						*host.RenamingNode = dupRoot
						host.MarkCollectionDirty(newCol)
						host.SaveState()
					}
					node.MenuOpen = false
					updateCols = true
				}

				for node.DelBtn.Clicked(gtx) {
					if *host.RenamingNode != nil {
						if _, isRemoved := collections.CollectSubtree(node)[*host.RenamingNode]; isRemoved {
							*host.RenamingNode = nil
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
						host.MarkCollectionDirty(node.Collection)
					} else {
						colID := node.Collection.ID
						for idx, c := range *host.Collections {
							if c.Data == node.Collection {
								*host.Collections = append((*host.Collections)[:idx], (*host.Collections)[idx+1:]...)
								break
							}
						}
						host.DeleteCollection(colID)
						host.SaveState()
					}
					for i := len((*host.Tabs)) - 1; i >= 0; i-- {
						if _, ok := removed[(*host.Tabs)[i].LinkedNode]; ok {
							host.CloseTab(i)
						}
					}
					for n := range removed {
						widgets.ResetEditorHScroll(&n.NameEditor)
					}
					node.MenuOpen = false
					updateCols = true
				}
			}

			isPlaceholder := draggingNode && node == *host.DraggedNode

			rowDim := layout.Inset{
				Top: unit.Dp(0), Bottom: unit.Dp(0),
				Left: unit.Dp(0), Right: unit.Dp(0),
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				isActiveNode := false
				if !flowsMode && len((*host.Tabs)) > 0 && *host.ActiveIdx >= 0 && *host.ActiveIdx < len((*host.Tabs)) {
					isActiveNode = (*host.Tabs)[*host.ActiveIdx].LinkedNode == node
				}

				nodeHovered := node.Hover.Update(gtx.Source) || node.MenuBtn.Hovered()
				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						size := gtx.Constraints.Min
						if isPlaceholder {
							paint.FillShape(gtx.Ops, theme.BgDragHolder, clip.Rect{Max: size}.Op())
						} else {
							switch {
							case isActiveNode:
								paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: size}.Op())
							case nodeHovered:
								paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: size}.Op())
							}
							if node.Depth > 0 && fade > 0 {
								indent := gtx.Dp(unit.Dp(12))
								guideW := max(1, gtx.Dp(unit.Dp(1)))
								off := gtx.Dp(unit.Dp(7))
								gc := theme.BorderSubtle
								if nodeHovered || isActiveNode {
									gc = theme.FgDisabled
								}
								gc.A = uint8(float32(gc.A) * fade)
								for d := 0; d < node.Depth; d++ {
									x := d*indent + off
									if x+guideW > size.X {
										break
									}
									paint.FillShape(gtx.Ops, gc, clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+guideW, size.Y)}.Op())
								}
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
								leftDp := float32(node.Depth * 12)
								if !node.IsFolder && node.Request != nil {
									leftDp += 8
								}
								contentDimInner := layout.Inset{
									Top: unit.Dp(4), Bottom: unit.Dp(4),
									Left:  unit.Dp(leftDp),
									Right: unit.Dp(4),
								}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									methodColW := 0
									if node.IsFolder {
										node.NameLeftPx = gtx.Dp(unit.Dp(float32(node.Depth*12 + 18 + 4)))
										node.NameWidthPx = measureLabelWidth(gtx, host.Theme, node.Name, node.Depth == 0, unit.Sp(12))
									} else if node.Request != nil {
										methodColW = colMethodW[node.Collection]
										if methodColW <= 0 {
											methodColW = measureLabelWidth(gtx, host.Theme, abbrevMethod(node.Request.Method), false, unit.Sp(10))
										}
										node.NameLeftPx = gtx.Dp(unit.Dp(leftDp)) + methodColW + gtx.Dp(unit.Dp(6))
										node.NameWidthPx = measureLabelWidth(gtx, host.Theme, node.Name, false, unit.Sp(12))
									}
									children := make([]layout.FlexChild, 0, 3)
									if node.IsFolder {
										children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											ic := widgets.IconChevronR
											if node.Expanded {
												ic = widgets.IconChevronD
											}
											size := gtx.Dp(unit.Dp(14))
											gtx.Constraints.Min = image.Pt(size, size)
											gtx.Constraints.Max = gtx.Constraints.Min
											return ic.Layout(gtx, theme.FgMuted)
										}))
										children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
										children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if node.IsRenaming {
												return renameFieldSized(gtx, host.Theme, &node.NameEditor, node.Depth == 0, unit.Sp(12))
											}
											lbl := material.Label(host.Theme, unit.Sp(12), node.Name)
											lbl.Alignment = text.Start
											lbl.MaxLines = 2
											lbl.Truncator = "…"
											lbl.LineHeightScale = 1.0
											if node.Depth == 0 {
												lbl.Font.Weight = font.Bold
											}
											return layout.W.Layout(gtx, lbl.Layout)
										}))
									} else if node.Request != nil {
										children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											lbl := material.Label(host.Theme, unit.Sp(10), abbrevMethod(node.Request.Method))
											lbl.Color = theme.MethodColor(node.Request.Method)
											lbl.Alignment = text.Start
											lbl.MaxLines = 1
											lbl.LineHeightScale = 1.0
											gtx.Constraints.Min.X = methodColW
											gtx.Constraints.Max.X = methodColW
											return lbl.Layout(gtx)
										}))
										children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
										children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
											if node.IsRenaming {
												return renameFieldSized(gtx, host.Theme, &node.NameEditor, false, unit.Sp(12))
											}
											lbl := material.Label(host.Theme, unit.Sp(12), node.Name)
											lbl.Alignment = text.Start
											lbl.MaxLines = 2
											lbl.Truncator = "…"
											lbl.LineHeightScale = 1.0
											return layout.W.Layout(gtx, lbl.Layout)
										}))
									}
									return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
								})
								node.ContentHeightPx = contentDimInner.Size.Y
								return contentDimInner
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								dims := material.Clickable(gtx, &node.MenuBtn, func(gtx layout.Context) layout.Dimensions {
									w := gtx.Dp(unit.Dp(18))
									h := node.ContentHeightPx
									if h <= 0 {
										h = gtx.Dp(unit.Dp(16))
									}
									gtx.Constraints.Min = image.Pt(w, h)
									gtx.Constraints.Max = gtx.Constraints.Min
									iconCol := theme.FgMuted
									if node.MenuBtn.Hovered() {
										iconCol = host.Theme.Fg
									}
									iconCol.A = uint8(float32(iconCol.A) * fade)
									return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										isz := gtx.Dp(unit.Dp(14))
										gtx.Constraints.Min = image.Pt(isz, isz)
										gtx.Constraints.Max = gtx.Constraints.Min
										return widgets.IconMore.Layout(gtx, iconCol)
									})
								})
								node.MenuBtnWidth = dims.Size.X
								return dims
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
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
						menuOffsetY := node.RowHeightPx
						if menuOffsetY <= 0 {
							menuOffsetY = gtx.Dp(unit.Dp(18))
						}
						op.Offset(image.Pt(menuX, menuOffsetY)).Add(gtx.Ops)
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
												return widgets.MenuOption(gtx, host.Theme, &node.AddReqBtn, "Add Request", widgets.IconAddReq)
											}))
											actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, host.Theme, &node.AddFldBtn, "Add Folder", widgets.IconAddFld)
											}))
										}
										actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOption(gtx, host.Theme, &node.EditBtn, "Rename", widgets.IconRename)
										}))
										if node.Depth > 0 {
											actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, host.Theme, &node.DupBtn, "Duplicate", widgets.IconDup)
											}))
										}
										actions = append(actions, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											return widgets.MenuOption(gtx, host.Theme, &node.DelBtn, "Delete", widgets.IconDel)
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
				*host.ColRowH = rowDim.Size.Y
			}
			if rowDim.Size.Y > 0 {
				node.RowHeightPx = rowDim.Size.Y
			}
			if i >= listFirst {
				(*host.ColRowYs)[i] = trackY
				trackY += rowDim.Size.Y
				*host.ColAfterLastY = trackY
			}
			return rowDim
		})

		rowYAt := func(idx int) int {
			if y, ok := (*host.ColRowYs)[idx]; ok {
				return y
			}
			if idx >= len(colsSnapshot) {
				return *host.ColAfterLastY
			}
			return idx * *host.ColRowH
		}

		if draggingNode && *host.ColRowH > 0 && draggedNodeVisibleIdx >= 0 && *host.DraggedNode != nil {
			rowW := dim.Size.X
			if rowW <= 0 {
				rowW = gtx.Constraints.Max.X
			}
			srcOverlayY := rowYAt(draggedNodeVisibleIdx)
			draggedRowH := *host.ColRowH
			if draggedNodeVisibleIdx+1 < len(colsSnapshot) {
				if nextY, ok := (*host.ColRowYs)[draggedNodeVisibleIdx+1]; ok {
					if h := nextY - srcOverlayY; h > 0 {
						draggedRowH = h
					}
				}
			} else if h := *host.ColAfterLastY - srcOverlayY; h > 0 && draggedNodeVisibleIdx >= listFirst {
				draggedRowH = h
			}
			hitMacro := op.Record(gtx.Ops)
			hitOff := op.Offset(image.Pt(0, srcOverlayY)).Push(gtx.Ops)
			hitClip := clip.Rect{Max: image.Pt(rowW, draggedRowH)}.Push(gtx.Ops)
			(*host.DraggedNode).Drag.Add(gtx.Ops)
			hitClip.Pop()
			hitOff.Pop()
			op.Defer(gtx.Ops, hitMacro.Stop())

			ghostY := srcOverlayY + int(*host.DragNodeCurrentY-*host.DragNodeOriginY)
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
			renderNodeGhost(ghostGtx, host.Theme, *host.DraggedNode)
			ghostOff.Pop()
			op.Defer(gtx.Ops, ghostMacro.Stop())

			if drop, ok := dragNodeDrop(host, gtx.Metric); ok && !flowsMode {
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
						hH := *host.ColRowH
						if targetIdx+1 < len(colsSnapshot) {
							if nextY, ok := (*host.ColRowYs)[targetIdx+1]; ok {
								if h := nextY - hY; h > 0 {
									hH = h
								}
							}
						} else if h := *host.ColAfterLastY - hY; h > 0 {
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
			host.UpdateVisibleCols()
			host.Window.Invalidate()
		}

		pass := pointer.PassOp{}.Push(gtx.Ops)
		ov := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		host.ColsBodyHover.Add(gtx.Ops)
		ov.Pop()
		pass.Pop()

		return dim
	}

	// stickySolidH carries the OPAQUE contiguous band height (filled background)
	// out to colsArea's DebugBand call. stickyReserve is the height the LIST is
	// pushed DOWN by, so the band never covers content (VS Code list-offset model).
	stickySolidH := 0
	stickyHeaders := func(gtx layout.Context) layout.Dimensions {
		host.StickyRows = host.StickyRows[:0]
		*host.StickyBandH = 0
		stickySolidH = 0

		snap := *host.VisibleCols
		first := host.ColList.Position.First
		if first < 0 || first >= len(snap) {
			return layout.Dimensions{}
		}
		// Overlay model: the band is painted ON TOP of the list (the list is NOT
		// offset by it — see colsArea), so content scrolls 1:1 and the number of
		// pinned rows never moves it. Because the band COVERS the top rows, the
		// pinned chain must be the ancestors of the first row that is NOT covered
		// (the first row the user actually sees below the band), not of snap[First]
		// which is hidden underneath — else the band lags by the covered rows. The
		// uncovered row (`target`) is found below, once the row height is known.
		ancestorsOf := func(n *collections.CollectionNode) []*collections.CollectionNode {
			var a []*collections.CollectionNode
			for p := n.Parent; p != nil; p = p.Parent {
				a = append(a, p)
			}
			for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
				a[i], a[j] = a[j], a[i]
			}
			return a
		}

		w := gtx.Constraints.Max.X
		indent := gtx.Dp(unit.Dp(12))
		guideW := max(1, gtx.Dp(unit.Dp(1)))
		goff := gtx.Dp(unit.Dp(7))
		fade := host.ColsBodyFade.Value()

		renderRow := func(gtx layout.Context, node *collections.CollectionNode) layout.Dimensions {
			gtx.Constraints.Min.X = w
			gtx.Constraints.Max.X = w
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					size := gtx.Constraints.Min
					paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: size}.Op())
					if node.Depth > 0 && fade > 0 {
						gc := theme.BorderSubtle
						gc.A = uint8(float32(gc.A) * fade)
						for dd := 0; dd < node.Depth; dd++ {
							x := dd*indent + goff
							if x+guideW > size.X {
								break
							}
							paint.FillShape(gtx.Ops, gc, clip.Rect{Min: image.Pt(x, 0), Max: image.Pt(x+guideW, size.Y)}.Op())
						}
					}
					return layout.Dimensions{Size: size}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = w
					leftDp := float32(node.Depth * 12)
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(leftDp), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								isz := gtx.Dp(unit.Dp(14))
								gtx.Constraints.Min = image.Pt(isz, isz)
								gtx.Constraints.Max = gtx.Constraints.Min
								return widgets.IconChevronD.Layout(gtx, theme.FgMuted)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								lbl := material.Label(host.Theme, unit.Sp(12), node.Name)
								lbl.MaxLines = 1
								lbl.Truncator = "…"
								lbl.LineHeightScale = 1.0
								if node.Depth == 0 {
									lbl.Font.Weight = font.Bold
								}
								return lbl.Layout(gtx)
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								bw := gtx.Dp(unit.Dp(18))
								bh := gtx.Dp(unit.Dp(16))
								gtx.Constraints.Min = image.Pt(bw, bh)
								gtx.Constraints.Max = gtx.Constraints.Min
								iconCol := theme.FgMuted
								iconCol.A = uint8(float32(iconCol.A) * fade)
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									isz := gtx.Dp(unit.Dp(14))
									gtx.Constraints.Min = image.Pt(isz, isz)
									gtx.Constraints.Max = gtx.Constraints.Min
									return widgets.IconMore.Layout(gtx, iconCol)
								})
							}),
						)
					})
				}),
			)
		}

		maxRows := gtx.Constraints.Max.Y/max(1, gtx.Dp(unit.Dp(24))) - 2

		// bandFor returns the rows pinned when `i` is the top row: its ancestors, plus
		// `i` itself when it is a folder being entered (header at top, children below).
		bandFor := func(i int) []*collections.CollectionNode {
			a := ancestorsOf(snap[i])
			n := snap[i]
			if (n.IsFolder || n.Depth == 0) && n.Expanded &&
				i+1 < len(snap) && snap[i+1].Parent == n {
				a = append(a, n)
			}
			return a
		}
		// Row height of a single band row (uniform, single-line) for sizing.
		borderH := max(1, gtx.Dp(unit.Dp(1)))
		mgtx := gtx
		mgtx.Constraints.Min.Y = 0
		mrec := op.Record(gtx.Ops)
		bandRowH := renderRow(mgtx, snap[first]).Size.Y
		mrec.Stop()
		if bandRowH <= 0 {
			bandRowH = gtx.Dp(unit.Dp(24))
		}
		listRowH := func(i int) int {
			if i >= 0 && i < len(snap) {
				if h := snap[i].RowHeightPx; h > 0 {
					return h
				}
			}
			if host.ColRowH != nil && *host.ColRowH > 0 {
				return *host.ColRowH
			}
			return gtx.Dp(unit.Dp(24))
		}
		offset := host.ColList.Position.Offset
		isAncestorOrSelf := func(a, n *collections.CollectionNode) bool {
			for p := n; p != nil; p = p.Parent {
				if p == a {
					return true
				}
			}
			return false
		}

		// Overlay reach-up model: the band pins the ancestor chain of the TOP row
		// (`first`) plus `first` itself when it is a folder being entered. The list is
		// drawn under the band (not offset), so content scrolls 1:1 with no jump.
		chain := bandFor(first)
		if len(chain) == 0 {
			return layout.Dimensions{}
		}
		if maxRows >= 1 && len(chain) > maxRows {
			chain = chain[len(chain)-maxRows:]
		}
		m := len(chain)

		screenAt := func(idx int) int {
			y := -offset
			if idx >= first {
				for j := first; j < idx; j++ {
					y += listRowH(j)
				}
			} else {
				for j := idx; j < first; j++ {
					y -= listRowH(j)
				}
			}
			return y
		}
		idxOf := func(n *collections.CollectionNode) int {
			for i := 0; i < len(snap); i++ {
				if snap[i] == n {
					return i
				}
			}
			return -1
		}
		// endYAbs = the screen Y where folder c's subtree ends, independent of `first`.
		endYAbs := func(c *collections.CollectionNode) (int, bool) {
			i := idxOf(c)
			if i < 0 {
				return 0, false
			}
			for i < len(snap) && isAncestorOrSelf(c, snap[i]) {
				i++
			}
			if i >= len(snap) {
				return screenAt(i), false
			}
			return screenAt(i), true
		}
		// place = the Y at which folder F sits when pinned at slot s: it rides its real
		// row until that row reaches the docked slot (s*bandRowH) where it sticks; when
		// F's own scope ends, the next section's boundary row pushes it up and out.
		place := func(F *collections.CollectionNode, s int) int {
			dock := s * bandRowH
			y := screenAt(idxOf(F))
			if y < dock {
				y = dock
			}
			if e, found := endYAbs(F); found {
				if ceil := e - bandRowH; y > ceil {
					y = ceil
				}
			}
			return y
		}

		host.StickyRows = append(host.StickyRows[:0], chain...)
		if DebugSticky != nil {
			names := make([]string, len(chain))
			for i, a := range chain {
				names[i] = a.Name
			}
			DebugSticky(first, names)
		}

		// Drawn lines = the current chain plus the incoming descending chain (the next
		// row's first-child folder path), so a nested subfolder chain shows together
		// with the folder being entered (no per-level lag). Each rides at its real row
		// until it docks, so it does not opaquely cover the content below.
		type bandLine struct {
			node *collections.CollectionNode
			y    int
			slot int
		}
		lines := make([]bandLine, 0, m+2)
		for k := 0; k < m; k++ {
			lines = append(lines, bandLine{chain[k], place(chain[k], k), k})
		}
		inLine := func(n *collections.CollectionNode) bool {
			for _, ln := range lines {
				if ln.node == n {
					return true
				}
			}
			return false
		}
		// slotDocked reports whether some already-placed line occupies slot s AT its
		// dock (y == s*bandRowH) — i.e. that level has fully settled into the band.
		slotDocked := func(s int) bool {
			for _, ln := range lines {
				if ln.slot == s && ln.y == s*bandRowH {
					return true
				}
			}
			return false
		}
		// addIncoming renders the folders of bandFor(src) that differ from the current
		// chain, each riding into its slot — but only once each has risen to within a
		// row of its slot (rawY check), so a far-off folder is never shown early.
		addIncoming := func(src int) {
			if src <= first || src >= len(snap) {
				return
			}
			next := bandFor(src)
			if maxRows >= 1 && len(next) > maxRows {
				next = next[len(next)-maxRows:]
			}
			for k := 0; k < len(next); k++ {
				if k < m && next[k] == chain[k] {
					continue // stable slot
				}
				if inLine(next[k]) {
					continue // already drawn from another source
				}
				rawY := screenAt(idxOf(next[k]))
				if rawY > k*bandRowH+bandRowH {
					continue // not yet risen to its slot
				}
				// Reveal nested headers strictly top-down: a deeper incoming row must
				// not appear before the row directly above it has DOCKED at its slot.
				// This matches VS Code (a child sticky header never shows without its
				// parent stuck first) and staggers a deep chain — each row waits for its
				// parent to settle, instead of the whole chain crossing its rise
				// thresholds together in a single frame (the multi-row pop). Rows must be
				// added top-down (k ascending, which this loop does) for the check to see
				// the parent already placed.
				if k > 0 && !slotDocked(k-1) {
					continue
				}
				lines = append(lines, bandLine{next[k], place(next[k], k), k})
			}
		}
		// descendFirstChildChain follows the first-child folder path down from x: while
		// snap[x] is an entered folder whose immediately-following row is its own child
		// folder, step into that child. Returns the deepest such index, so addIncoming
		// pins the whole nested first-child chain (not just the outermost folder).
		descendFirstChildChain := func(x int) int {
			for x+1 < len(snap) {
				n := snap[x]
				entered := (n.IsFolder || n.Depth == 0) && n.Expanded
				child := snap[x+1]
				childFolder := (child.IsFolder || child.Depth == 0) && child.Expanded
				if entered && child.Parent == n && childFolder {
					x++
					continue
				}
				break
			}
			return x
		}
		// (a) The descending first-child folder chain of the next row, so a nested
		// subfolder chain shows together with the folder being entered (no per-level
		// lag for the FIRST child at each level).
		addIncoming(descendFirstChildChain(first + 1))
		// (a') Early pre-load for a DEEP upcoming chain: scan a little ahead for the
		// nearest folder that begins a multi-level chain (its first-child descent goes
		// deeper than the folder itself) and pre-load it, so its rows have lead time to
		// rise and STAGGER into the band instead of popping in all at once at the single
		// frame the top row reaches the folder (the non-seamless deep transition — e.g.
		// entering "Сервисные методы > Webhooks > Входящее сообщение > …"). addIncoming's
		// per-row rawY gate keeps rows hidden until each has risen to its slot, so this
		// only widens the band as rows actually approach.
		//
		// Restricted to DEEP chains: a FLAT sibling folder docks at its slot immediately
		// (its real row is already one row below the top), so bringing it in early would
		// instead grow a top-level band past two rows — it must push out, not grow (see
		// TestStickySiblingPushOutSlidesNotVanish). The deep-only guard leaves flat
		// sibling swaps to dock normally.
		for na := first + 1; na < len(snap); na++ {
			if screenAt(na) > (m+1)*bandRowH {
				break // the folder's row is still too far below to pre-load
			}
			n := snap[na]
			entered := (n.IsFolder || n.Depth == 0) && n.Expanded &&
				na+1 < len(snap) && snap[na+1].Parent == n
			if !entered {
				continue
			}
			if deep := descendFirstChildChain(na); deep > na {
				addIncoming(deep)
			}
			break // only the nearest upcoming folder section
		}
		// (b) The SUCCESSOR of the deepest pinned folder while it is leaving (its header
		// is above the top, i.e. `first` is strictly inside it). This is the sibling
		// subfolder swap — e.g. leaving "Получение уведомления" into its sibling
		// "Получение файлов" — so the slot stays filled (the band does not dip to fewer
		// rows and then pop back) and the successor slides into the slot smoothly.
		//
		// Only for a DEEP deepest folder (slot >= 2, i.e. m >= 3): there the successor's
		// real row is far above its slot by the time `first` reaches the boundary, so
		// without this it would POP straight to its slot. A top-level (d1) successor's
		// real row is already at its slot, so it docks cleanly via (a) — bringing it in
		// early here would instead make the d1 swap grow to three rows (it must push
		// out, not grow — see TestStickySiblingPushOutSlidesNotVanish).
		//
		// Descend the successor's own first-child chain too: when the successor is itself
		// a deeply-nested folder (e.g. "Отправленное сообщение" > "…с телефона" > "Медиа
		// сообщения"), its inner rows must slide in WITH it. Without the descent only the
		// successor's outermost row pre-loads, and the inner chain pops in all at once at
		// the step `first` finally reaches the boundary (band jumps several rows in one
		// frame — the non-seamless d3+ transition the user reported).
		if m >= 3 {
			deepest := chain[m-1]
			if di := idxOf(deepest); di >= 0 && di != first {
				j := di
				for j < len(snap) && isAncestorOrSelf(deepest, snap[j]) {
					j++
				}
				addIncoming(descendFirstChildChain(j))
			}
		}

		bandBottom := 0
		for _, ln := range lines {
			if b := ln.y + bandRowH; b > bandBottom {
				bandBottom = b
			}
		}
		if bandBottom < 0 {
			bandBottom = 0
		}
		solidH := 0
		for s := 0; ; s++ {
			docked := false
			for _, ln := range lines {
				if ln.slot == s && ln.y == s*bandRowH {
					docked = true
					break
				}
			}
			if !docked {
				break
			}
			solidH = (s + 1) * bandRowH
		}
		bandTotalH := bandBottom + borderH

		rg := gtx
		rg.Constraints.Min.Y = 0
		bandClip := clip.Rect{Max: image.Pt(w, bandTotalH)}.Push(gtx.Ops)
		if solidH > 0 {
			paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(w, solidH)}.Op())
		}
		for s := bandBottom/bandRowH + 1; s >= 0; s-- {
			for _, ln := range lines {
				if ln.slot != s {
					continue
				}
				ro := op.Offset(image.Pt(0, ln.y)).Push(gtx.Ops)
				renderRow(rg, ln.node)
				ro.Pop()
			}
		}
		bo := op.Offset(image.Pt(0, bandBottom)).Push(gtx.Ops)
		paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect{Max: image.Pt(w, borderH)}.Op())
		bo.Pop()
		bandClip.Pop()

		dims := layout.Dimensions{Size: image.Pt(w, bandTotalH)}
		*host.StickyBandH = bandTotalH
		stickySolidH = solidH
		if DebugBandGeom != nil {
			names := make([]string, len(lines))
			ys := make([]int, len(lines))
			for i, ln := range lines {
				names[i], ys[i] = ln.node.Name, ln.y
			}
			DebugBandGeom(names, ys, bandBottom)
		}

		sp := pointer.PassOp{}.Push(gtx.Ops)
		hc := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		host.ColsBodyHover.Add(gtx.Ops)
		hc.Pop()
		sp.Pop()
		return dims
	}

	colsArea := func(gtx layout.Context) layout.Dimensions {
		total := gtx.Constraints.Max.Y

		// Overlay (VS Code-style): the list is laid out at full height and is NOT
		// offset — content scrolls 1:1 (no sticky-induced jump, ever). The band is
		// recorded after the list (so it reads the current Position.First) and painted
		// on top, occluding the top rows. (An earlier experiment pushing the list down
		// by the band height made content lurch ~a row at every folder boundary — see
		// TestStickyRealCollectionScrollTopToBottom — so the overlay is kept.)
		lgtx := gtx
		lgtx.Constraints.Min.Y = 0
		lgtx.Constraints.Max.Y = total
		listMacro := op.Record(gtx.Ops)
		colsBody(lgtx)
		listCall := listMacro.Stop()

		bgtx := gtx
		bgtx.Constraints.Min.Y = 0
		bandMacro := op.Record(bgtx.Ops)
		bandDim := stickyHeaders(bgtx)
		bandCall := bandMacro.Stop()

		listCall.Add(gtx.Ops)
		bandCall.Add(gtx.Ops)

		if DebugBand != nil {
			DebugBand(0, bandDim.Size.Y) // overlay: list not offset, so reserve is 0
		}
		if DebugBandSolid != nil {
			DebugBandSolid(stickySolidH)
		}
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, total)}
	}

	envsHeader := func(gtx layout.Context) layout.Dimensions {
		if host.EnvsHeaderClick.Clicked(gtx) {
			*host.EnvsExpanded = !*host.EnvsExpanded
			host.Window.Invalidate()
		}
		for host.ImportEnvBtn.Clicked(gtx) {
			*host.EnvsMenuOpen = false
			go func() {
				data, err := host.ChooseJSONFile()
				if err != nil || data == nil {
					return
				}
				id := persist.NewRandomID()
				env, err := environments.ParseEnvironment(bytes.NewReader(data), id)
				if err == nil && env != nil {
					if werr := persist.AtomicWriteFile(filepath.Join(persist.EnvironmentsDir(), id+".json"), data); werr == nil {
						host.PushEnvLoaded(&environments.EnvironmentUI{Data: env})
					}
				}
			}()
		}
		for host.AddEnvBtn.Clicked(gtx) {
			addNewEnvironment(host)
		}
		for host.EnvsMenuBtn.Clicked(gtx) {
			*host.EnvsMenuOpen = !*host.EnvsMenuOpen
		}

		headerDims := layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(0), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Stack{}.Layout(gtx,
				layout.Expanded(func(gtx layout.Context) layout.Dimensions {
					if host.EnvsHeaderClick.Hovered() {
						paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
					}
					return layout.Dimensions{Size: gtx.Constraints.Min}
				}),
				layout.Stacked(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return host.EnvsHeaderClick.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(26))
								pointer.CursorPointer.Add(gtx.Ops)
								return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
									layout.Rigid(sectionCount(host.Theme, len(*host.Environments))),
									layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
									layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.Y = 0
										lbl := material.Label(host.Theme, unit.Sp(12), "Environments")
										lbl.LineHeightScale = 1.0
										return lbl.Layout(gtx)
									}),
								)
							})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, host.AddEnvBtn, widgets.IconAdd, host.Theme, 26, 16)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return widgets.SquareBtnSized(gtx, host.EnvsMenuBtn, widgets.IconMore, host.Theme, 26, 16)
						}),
					)
				}),
			)
		})

		if *host.EnvsMenuOpen {
			macro := op.Record(gtx.Ops)
			menuX := headerDims.Size.X
			menuY := 0
			op.Offset(image.Pt(menuX, menuY)).Add(gtx.Ops)

			menuGtx := gtx
			menuGtx.Constraints.Min = image.Point{}
			rec := op.Record(gtx.Ops)
			menuDims := material.Clickable(menuGtx, host.ImportEnvBtn, func(gtx layout.Context) layout.Dimensions {
				if host.ImportEnvBtn.Hovered() {
					paint.FillShape(gtx.Ops, theme.BgHover, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
				}
				pointer.CursorPointer.Add(gtx.Ops)
				return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(20), Right: unit.Dp(20)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(host.Theme, unit.Sp(12), "Import")
					return lbl.Layout(gtx)
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

	envsBody := func(gtx layout.Context) layout.Dimensions {

		defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
		pointer.CursorDefault.Add(gtx.Ops)

		anyEnvMenuOpen := false
		for _, e := range *host.Environments {
			if e.MenuOpen {
				anyEnvMenuOpen = true
				break
			}
		}
		blockHovered := host.EnvsBodyHover.Update(gtx.Source) || anyEnvMenuOpen
		fade := host.EnvsBodyFade.Update(gtx, blockHovered, 100*time.Millisecond)

		if len((*host.Environments)) == 0 {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(host.Theme, unit.Sp(12), "No environments loaded")
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
			if *host.ActiveEnvID == env.Data.ID {
				*host.ActiveEnvID = ""
			} else {
				*host.ActiveEnvID = env.Data.ID
			}
			*host.ActiveEnvDirty = true
			host.SaveState()
			host.Window.Invalidate()
		}

		preEnvSlop := float32(gtx.Dp(unit.Dp(4)))
		if dragged := *host.DraggedEnv; dragged != nil {
			for {
				e, ok := dragged.Drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Drag:
					if *host.DraggedEnv != dragged {
						continue
					}
					*host.DragEnvCurrentY = e.Position.Y
					dy := *host.DragEnvCurrentY - *host.DragEnvOriginY
					if dy < 0 {
						dy = -dy
					}
					if !*host.DragEnvActive && dy > preEnvSlop {
						*host.DragEnvActive = true
						*host.DragEnvOriginY = *host.DragEnvCurrentY
					}
				case pointer.Release:
					if *host.DraggedEnv == dragged {
						if *host.DragEnvActive {
							*host.DragEnvCurrentY = e.Position.Y
							commitEnvDrop(host, dragged)
						} else {
							envClickFn(dragged)
						}
						*host.DraggedEnv = nil
						*host.DragEnvActive = false
					}
				case pointer.Cancel:
					if *host.DraggedEnv == dragged {
						*host.DraggedEnv = nil
						*host.DragEnvActive = false
					}
				}
			}
		}

		envSnapshot := (*host.Environments)

		for _, e := range envSnapshot {
			e.Hover.Update(gtx.Source)
		}
		if hoverDebug {
			labels := make([]string, len(envSnapshot))
			hovers := make([]*widgets.Hover, len(envSnapshot))
			for i, e := range envSnapshot {
				labels[i] = e.Data.Name
				hovers[i] = &e.Hover
			}
			logHoverStates("env", labels, hovers, host.EnvList.Position.First, host.EnvList.Position.Count)
		}

		var draggingEnv bool
		draggedSrcIdx := -1
		if *host.DraggedEnv != nil && *host.DragEnvActive {
			for i, e := range *host.Environments {
				if e == *host.DraggedEnv {
					draggedSrcIdx = i
					break
				}
			}
			if draggedSrcIdx >= 0 {
				draggingEnv = true
			}
		}

		var envToDelete *environments.EnvironmentUI
		envList := material.List(host.Theme, host.EnvList)
		envList.AnchorStrategy = material.Overlay
		envList.Indicator.Color.A = uint8(float32(envList.Indicator.Color.A) * fade)
		envList.Indicator.HoverColor.A = uint8(float32(envList.Indicator.HoverColor.A) * fade)
		dim := envList.Layout(gtx, len(envSnapshot), func(gtx layout.Context, idx int) layout.Dimensions {
			if idx >= len(envSnapshot) {
				return layout.Dimensions{}
			}
			env := envSnapshot[idx]
			isActive := *host.ActiveEnvID == env.Data.ID

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
					*host.DraggedEnv = env
					*host.DragEnvOriginY = e.Position.Y
					*host.DragEnvCurrentY = e.Position.Y
					*host.DragEnvActive = false
				case pointer.Drag:
					if *host.DraggedEnv == env {
						*host.DragEnvCurrentY = e.Position.Y
						dy := *host.DragEnvCurrentY - *host.DragEnvOriginY
						if dy < 0 {
							dy = -dy
						}
						if !*host.DragEnvActive && dy > dragSlop {
							*host.DragEnvActive = true
							*host.DragEnvOriginY = *host.DragEnvCurrentY
						}
					}
				case pointer.Release:
					if *host.DraggedEnv == env {
						if *host.DragEnvActive {
							commitEnvDrop(host, env)
						} else {
							envClick()
						}
					}
					*host.DraggedEnv = nil
					*host.DragEnvActive = false
				case pointer.Cancel:
					*host.DraggedEnv = nil
					*host.DragEnvActive = false
				}
			}

			isEnvPlaceholder := draggingEnv && env == *host.DraggedEnv

			rowDim := layout.Inset{Top: unit.Dp(0), Bottom: unit.Dp(0), Left: unit.Dp(0), Right: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
					if *host.EditingEnv != nil && *host.EditingEnv != env {
						host.CommitEditingEnv()
					}
					*host.PendingEnvClose = nil
					*host.EditingEnv = env
					env.InitEditor()
					env.MenuOpen = false
					host.Window.Invalidate()
				}
				for env.DelBtn.Clicked(gtx) {
					envToDelete = env
					env.MenuOpen = false
				}

				envHovered := env.Hover.Update(gtx.Source) || env.MenuBtn.Hovered()
				bgColor := theme.BgDark
				if isActive {
					bgColor = theme.Bg
				}
				if envHovered {
					bgColor = theme.BgHover
				}

				for env.MenuBtn.Clicked(gtx) {
					if !env.MenuOpen {
						for _, e := range *host.Environments {
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
						duplicateEnvironment(host, env)
						env.MenuOpen = false
					}
				}

				return layout.Stack{}.Layout(gtx,
					layout.Expanded(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						size := gtx.Constraints.Min
						if !isEnvPlaceholder {
							paint.FillShape(gtx.Ops, bgColor, clip.Rect{Max: size}.Op())
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
							rowH := *host.EnvRowH
							if rowH <= 0 {
								rowH = gtx.Dp(unit.Dp(30))
							}
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, rowH)}
						}
						return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(0), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
								layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										if env.IsRenaming {
											return widgets.InlineRenameField(gtx, host.Theme, &env.InlineNameEd)
										}
										lbl := material.Label(host.Theme, unit.Sp(12), env.Data.Name)
										lbl.MaxLines = 1
										if isActive {
											lbl.Font.Weight = font.Bold
										}
										return env.NameScroll.Layout(gtx, host.Theme, lbl)
									})
								}),
								layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									for env.SelectBtn.Clicked(gtx) {
										if host.EnvColorPicker.IsOpen() && *host.EnvColorEnvID == env.Data.ID {
											host.EnvColorPicker.Close()
										} else {
											*host.EnvColorEnvID = env.Data.ID
											host.EnvColorPicker.Open(colorpicker.KindEnv, 0, environments.HighlightColor(env.Data), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
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
										w := gtx.Dp(18)
										h := *host.EnvRowH - 2*gtx.Dp(unit.Dp(4))
										if h <= 0 {
											h = w
										}
										gtx.Constraints.Min = image.Pt(w, h)
										gtx.Constraints.Max = gtx.Constraints.Min
										iconCol := theme.FgMuted
										if env.MenuBtn.Hovered() {
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
						if !env.MenuOpen {
							return layout.Dimensions{}
						}
						macro := op.Record(gtx.Ops)
						menuWidth := gtx.Dp(unit.Dp(158))
						menuHeight := gtx.Dp(unit.Dp(150))
						menuX := gtx.Constraints.Max.X - menuWidth
						if menuX < 0 {
							menuX = 0
						}
						menuY := gtx.Dp(unit.Dp(24))
						windowH := host.WindowSize.Y
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
												return widgets.MenuOption(gtx, host.Theme, &env.EditBtn, "Edit", widgets.IconSettings)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, host.Theme, &env.RenameBtn, "Rename", widgets.IconRename)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOption(gtx, host.Theme, &env.DupBtn, "Duplicate", widgets.IconDup)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.MenuOptionDanger(gtx, host.Theme, &env.DelBtn, "Delete", widgets.IconDel)
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
				*host.EnvRowH = rowDim.Size.Y
			}
			return rowDim
		})
		if draggingEnv && *host.EnvRowH > 0 && draggedSrcIdx >= 0 && *host.DraggedEnv != nil {
			rowW := dim.Size.X
			if rowW <= 0 {
				rowW = gtx.Constraints.Max.X
			}
			srcOverlayY := (draggedSrcIdx-host.EnvList.Position.First)**host.EnvRowH - host.EnvList.Position.Offset
			hitMacro := op.Record(gtx.Ops)
			hitOff := op.Offset(image.Pt(0, srcOverlayY)).Push(gtx.Ops)
			hitClip := clip.Rect{Max: image.Pt(rowW, *host.EnvRowH)}.Push(gtx.Ops)
			(*host.DraggedEnv).Drag.Add(gtx.Ops)
			hitClip.Pop()
			hitOff.Pop()
			op.Defer(gtx.Ops, hitMacro.Stop())

			ghostY := srcOverlayY + int(*host.DragEnvCurrentY-*host.DragEnvOriginY)
			if ghostY < 0 {
				ghostY = 0
			}
			if maxGhost := dim.Size.Y - *host.EnvRowH; maxGhost > 0 && ghostY > maxGhost {
				ghostY = maxGhost
			}
			ghostMacro := op.Record(gtx.Ops)
			ghostOff := op.Offset(image.Pt(0, ghostY)).Push(gtx.Ops)
			ghostGtx := gtx
			ghostGtx.Constraints.Min = image.Pt(rowW, 0)
			ghostGtx.Constraints.Max = image.Pt(rowW, *host.EnvRowH)
			renderEnvGhost(ghostGtx, host.Theme, *host.DraggedEnv)
			ghostOff.Pop()
			op.Defer(gtx.Ops, ghostMacro.Stop())

			if target := dragEnvDropTargetIdx(host); target >= 0 {
				var dropY int
				if target <= draggedSrcIdx {
					dropY = target * *host.EnvRowH
				} else {
					dropY = (target + 1) * *host.EnvRowH
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
			deleteEnvironment(host, envToDelete)
		}

		pass := pointer.PassOp{}.Push(gtx.Ops)
		ov := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
		host.EnvsBodyHover.Add(gtx.Ops)
		ov.Pop()
		pass.Pop()

		return dim
	}

	envDivider := func(gtx layout.Context) layout.Dimensions {
		vis := gtx.Dp(unit.Dp(1))
		grab := gtx.Dp(unit.Dp(6))
		w := gtx.Constraints.Max.X
		lineCol := theme.BorderSubtle
		if host.SidebarEnvDrag.Dragging() {
			lineCol = theme.Accent
		}
		paint.FillShape(gtx.Ops, lineCol, clip.Rect{Max: image.Pt(w, vis)}.Op())

		hitArea := clip.Rect{Min: image.Pt(0, vis-grab), Max: image.Pt(w, vis)}.Push(gtx.Ops)
		pointer.CursorRowResize.Add(gtx.Ops)
		host.SidebarEnvDrag.Add(gtx.Ops)
		event.Op(gtx.Ops, host.SidebarEnvDrag)
		for {
			_, ok := gtx.Event(pointer.Filter{Target: host.SidebarEnvDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
		}
		hitArea.Pop()
		return layout.Dimensions{Size: image.Pt(w, vis)}
	}

	scriptRowH := *host.ScriptRowH
	if scriptRowH <= 0 {
		scriptRowH = gtx.Dp(unit.Dp(24))
	}
	envRowH := *host.EnvRowH
	if envRowH <= 0 {
		envRowH = gtx.Dp(unit.Dp(30))
	}
	colRowH := *host.ColRowH
	if colRowH <= 0 {
		colRowH = gtx.Dp(unit.Dp(24))
	}

	avail := gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(81))
	if avail < scriptRowH+envRowH {
		avail = scriptRowH + envRowH
	}

	scriptsContent := len(*host.Scripts) * scriptRowH
	if scriptsContent < scriptRowH {
		scriptsContent = scriptRowH
	}
	envContent := len(*host.Environments) * envRowH
	if envContent < envRowH {
		envContent = envRowH
	}

	scriptsMin, envMin := 0, 0
	if *host.ScriptsExpanded {
		scriptsMin = min(scriptRowH, scriptsContent)
	}
	if *host.EnvsExpanded {
		envMin = min(envRowH, envContent)
	}

	colsMin := 0
	if *host.ColsExpanded {
		colsMin = colRowH
	}
	budget := avail - colsMin
	if budget < scriptsMin+envMin {
		budget = min(avail, scriptsMin+envMin)
	}

	scriptsPx := 0
	if *host.ScriptsExpanded {
		scriptsPx = scriptsContent
		if *host.ScriptsHeight > 0 {
			scriptsPx = *host.ScriptsHeight
		}
	}
	envPx := 0
	if *host.EnvsExpanded {
		envPx = envContent
		if *host.SidebarEnvHeight > 0 {
			envPx = *host.SidebarEnvHeight
		}
	}

	fit := func() {
		scriptsPx = max(scriptsPx, scriptsMin)
		envPx = max(envPx, envMin)
		if over := scriptsPx + envPx - budget; over > 0 {
			ds := min(over, scriptsPx-scriptsMin)
			scriptsPx -= ds
			over -= ds
			envPx -= min(over, envPx-envMin)
		}
	}
	fit()

	readDrag := func(d *gesture.Drag, startY *float32) (moved, released bool, finalY float32) {
		for {
			e, ok := d.Update(gtx.Metric, gtx.Source, gesture.Vertical)
			if !ok {
				break
			}
			switch e.Kind {
			case pointer.Press:
				*startY = e.Position.Y
			case pointer.Drag:
				finalY = e.Position.Y
				moved = true
			case pointer.Cancel, pointer.Release:
				released = true
			}
		}
		return
	}
	resizeScripts := func(delta int) int {
		if delta > 0 {
			delta = min(delta, (scriptsPx-scriptsMin)+(envPx-envMin))
			ds := min(delta, scriptsPx-scriptsMin)
			scriptsPx -= ds
			envPx -= delta - ds
			return delta
		}
		up := min(-delta, budget-scriptsPx-envPx)
		scriptsPx += up
		return -up
	}

	resizeEnv := func(delta int) int {
		if delta < 0 {
			slack := max(0, scriptsPx-scriptsContent)
			up := min(-delta, slack+(budget-scriptsPx-envPx))
			scriptsPx -= min(up, slack)
			envPx += up
			return -up
		}
		down := min(delta, envPx-envMin)
		envPx -= down
		return down
	}

	resizeScriptsBottom := func(delta int) int {
		lo, hi := scriptsMin, budget-envMin
		want := scriptsPx + delta
		if want < lo {
			want = lo
		}
		if want > hi {
			want = hi
		}
		applied := want - scriptsPx
		scriptsPx = want
		return applied
	}

	storeHeights := func() {
		if *host.ScriptsExpanded {
			*host.ScriptsHeight = scriptsPx
		}
		if *host.EnvsExpanded {
			*host.SidebarEnvHeight = envPx
		}
	}

	if *host.ScriptsExpanded {
		moved, released, finalY := readDrag(host.ScriptsDrag, host.ScriptsDragY)
		if moved {
			*host.ScriptsDragY = finalY - float32(resizeScripts(int(finalY-*host.ScriptsDragY)))
			storeHeights()
			host.Window.Invalidate()
		}
		if released {
			storeHeights()
			host.SaveState()
		}
	}

	if *host.EnvsExpanded {
		moved, released, finalY := readDrag(host.SidebarEnvDrag, host.SidebarEnvDragY)
		if moved {
			delta := int(finalY - *host.SidebarEnvDragY)
			var applied int
			if !*host.ColsExpanded && *host.ScriptsExpanded {
				applied = resizeScriptsBottom(delta)
			} else {
				applied = resizeEnv(delta)
			}
			*host.SidebarEnvDragY = finalY - float32(applied)
			storeHeights()
			host.Window.Invalidate()
		}
		if released {
			storeHeights()
			host.SaveState()
		}
	}
	fit()
	scriptsDivider := func(gtx layout.Context) layout.Dimensions {
		vis := gtx.Dp(unit.Dp(1))
		grab := gtx.Dp(unit.Dp(6))
		w := gtx.Constraints.Max.X
		lineCol := theme.BorderSubtle
		if host.ScriptsDrag.Dragging() {
			lineCol = theme.Accent
		}
		paint.FillShape(gtx.Ops, lineCol, clip.Rect{Max: image.Pt(w, vis)}.Op())

		hitArea := clip.Rect{Min: image.Pt(0, vis-grab), Max: image.Pt(w, vis)}.Push(gtx.Ops)
		pointer.CursorRowResize.Add(gtx.Ops)
		host.ScriptsDrag.Add(gtx.Ops)
		event.Op(gtx.Ops, host.ScriptsDrag)
		for {
			_, ok := gtx.Event(pointer.Filter{Target: host.ScriptsDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
			if !ok {
				break
			}
		}
		hitArea.Pop()
		return layout.Dimensions{Size: image.Pt(w, vis)}
	}
	scriptsChildren := func() []layout.FlexChild {
		var out []layout.FlexChild
		if *host.ColsExpanded && *host.ScriptsExpanded {
			out = append(out, layout.Rigid(scriptsDivider))
		} else {
			out = append(out, layout.Rigid(borderLine))
		}
		out = append(out, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return scriptsHeader(gtx, host)
		}))
		if *host.ScriptsExpanded {
			out = append(out, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = scriptsPx
				gtx.Constraints.Max.Y = scriptsPx
				return scriptsBody(gtx, host)
			}))
		}
		return out
	}

	spacer := layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Min.Y)}
	})
	envBody := layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = envPx
		gtx.Constraints.Max.Y = envPx
		return envsBody(gtx)
	})

	netlimitMode := host.SidebarSection != nil && *host.SidebarSection == "netlimit"

	children := []layout.FlexChild{
		layout.Rigid(borderLine),
		layout.Rigid(colsHeader),
	}

	switch {
	case *host.ColsExpanded && *host.EnvsExpanded:
		children = append(children, layout.Flexed(1, colsArea))
		children = append(children, scriptsChildren()...)
		children = append(children,
			layout.Rigid(envDivider),
			layout.Rigid(envsHeader),
			envBody,
		)
	case *host.ColsExpanded:
		children = append(children, layout.Flexed(1, colsArea))
		children = append(children, scriptsChildren()...)
		children = append(children,
			layout.Rigid(borderLine),
			layout.Rigid(envsHeader),
		)
	case *host.EnvsExpanded:
		children = append(children, scriptsChildren()...)
		if *host.ScriptsExpanded {
			children = append(children, layout.Rigid(envDivider))
		} else {
			children = append(children, layout.Rigid(borderLine))
		}
		children = append(children,
			layout.Rigid(envsHeader),
			layout.Flexed(1, envsBody),
		)
	default:
		children = append(children, scriptsChildren()...)
		children = append(children,
			layout.Rigid(borderLine),
			layout.Rigid(envsHeader),
			spacer,
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
		secBtnH := gtx.Dp(unit.Dp(40))
		innerW := gutterW
		if !host.HideSidebar() {
			innerW = gutterW - gtx.Dp(unit.Dp(1))
		}
		layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(borderLine),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = image.Pt(innerW, btnH)
				gtx.Constraints.Max = image.Pt(innerW, btnH)
				return host.LayoutToggleBtn(gtx)
			}),
			layout.Rigid(borderLine),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if host.LayoutSectionRequests == nil {
					return layout.Dimensions{}
				}
				gtx.Constraints.Min = image.Pt(innerW, secBtnH)
				gtx.Constraints.Max = image.Pt(innerW, secBtnH)
				return host.LayoutSectionRequests(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if host.LayoutSectionFlows == nil {
					return layout.Dimensions{}
				}
				gtx.Constraints.Min = image.Pt(innerW, secBtnH)
				gtx.Constraints.Max = image.Pt(innerW, secBtnH)
				return host.LayoutSectionFlows(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if host.LayoutSectionMITM == nil {
					return layout.Dimensions{}
				}
				gtx.Constraints.Min = image.Pt(innerW, secBtnH)
				gtx.Constraints.Max = image.Pt(innerW, secBtnH)
				return host.LayoutSectionMITM(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if host.LayoutSectionNetlimit == nil {
					return layout.Dimensions{}
				}
				gtx.Constraints.Min = image.Pt(innerW, secBtnH)
				gtx.Constraints.Max = image.Pt(innerW, secBtnH)
				return host.LayoutSectionNetlimit(gtx)
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)}
			}),
		)
		if !host.HideSidebar() {
			line := gtx.Dp(unit.Dp(1))
			paint.FillShape(gtx.Ops, theme.BorderSubtle, clip.Rect{Min: image.Pt(gutterW-line, 0), Max: image.Pt(gutterW, h)}.Op())
		}
		return layout.Dimensions{Size: image.Pt(gutterW, h)}
	}

	if host.HideSidebar() {
		return gutter(gtx)
	}

	if netlimitMode && host.LayoutNetlimitBody != nil {
		children = []layout.FlexChild{
			layout.Rigid(borderLine),
			layout.Flexed(1, host.LayoutNetlimitBody),
		}
	}

	mitmMode := host.SidebarSection != nil && *host.SidebarSection == "mitm"
	if mitmMode && host.LayoutMITMRules != nil {
		children = []layout.FlexChild{
			layout.Rigid(borderLine),
			layout.Flexed(1, host.LayoutMITMRules),
		}
	}

	dims := layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(gutter),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
	if host.ScriptsDrag.Dragging() || host.ScriptsDrag.Pressed() ||
		host.SidebarEnvDrag.Dragging() || host.SidebarEnvDrag.Pressed() {
		ca := clip.Rect{Max: size}.Push(gtx.Ops)
		pointer.CursorRowResize.Add(gtx.Ops)
		ca.Pop()
	}
	return dims
}

func renderNodeGhost(gtx layout.Context, th *material.Theme, node *collections.CollectionNode) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	rowH := gtx.Constraints.Max.Y
	if rowH <= 0 {
		rowH = gtx.Dp(unit.Dp(18))
	}
	size := image.Pt(gtx.Constraints.Max.X, rowH)
	if size.Y <= 0 {
		size.Y = gtx.Dp(unit.Dp(16))
	}
	rect := clip.UniformRRect(image.Rectangle{Max: size}, 4)
	paint.FillShape(gtx.Ops, theme.BgDragGhost, rect.Op(gtx.Ops))
	widgets.PaintBorder1px(gtx, size, theme.Accent)
	gtx.Constraints.Min = size
	gtx.Constraints.Max = size
	leftDp := float32(node.Depth * 12)
	if !node.IsFolder && node.Request != nil {
		leftDp += 8
	}
	return layout.Inset{
		Top: unit.Dp(4), Bottom: unit.Dp(4),
		Left:  unit.Dp(leftDp),
		Right: unit.Dp(4),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, 3)
		if node.IsFolder {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ic := widgets.IconChevronR
				if node.Expanded {
					ic = widgets.IconChevronD
				}
				sz := gtx.Dp(unit.Dp(14))
				gtx.Constraints.Min = image.Pt(sz, sz)
				gtx.Constraints.Max = gtx.Constraints.Min
				return ic.Layout(gtx, theme.FgMuted)
			}))
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout))
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), node.Name)
				lbl.Alignment = text.Start
				lbl.MaxLines = 2
				lbl.Truncator = "…"
				lbl.LineHeightScale = 1.0
				if node.Depth == 0 {
					lbl.Font.Weight = font.Bold
				}
				return layout.W.Layout(gtx, lbl.Layout)
			}))
		} else if node.Request != nil {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(10), abbrevMethod(node.Request.Method))
				lbl.Color = theme.MethodColor(node.Request.Method)
				lbl.LineHeightScale = 1.0
				return lbl.Layout(gtx)
			}))
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout))
			children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), node.Name)
				lbl.Alignment = text.Start
				lbl.MaxLines = 2
				lbl.Truncator = "…"
				lbl.LineHeightScale = 1.0
				return layout.W.Layout(gtx, lbl.Layout)
			}))
		}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
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
	return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
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
