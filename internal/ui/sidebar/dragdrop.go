package sidebar

import (
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/unit"
)

type nodeDropTarget struct {
	parent    *collections.CollectionNode
	insertIdx int
	intoNode  *collections.CollectionNode
	lineIdx   int
	lineDepth int
}

func siblingIndex(n *collections.CollectionNode) int {
	if n == nil || n.Parent == nil {
		return -1
	}
	for i, c := range n.Parent.Children {
		if c == n {
			return i
		}
	}
	return -1
}

func isAncestorOrSelf(ancestor, n *collections.CollectionNode) bool {
	for cur := n; cur != nil; cur = cur.Parent {
		if cur == ancestor {
			return true
		}
	}
	return false
}

func dragNodeDrop(host *Host, metric unit.Metric) (drop nodeDropTarget, ok bool) {
	if *host.DraggedNode == nil || !*host.DragNodeActive || *host.ColRowH <= 0 {
		return nodeDropTarget{}, false
	}
	src := *host.DraggedNode
	visible := *host.VisibleCols

	srcStart := -1
	for i, n := range visible {
		if n == src {
			srcStart = i
			break
		}
	}
	if srcStart < 0 {
		return nodeDropTarget{}, false
	}

	rowYAt := func(idx int) int {
		if idx >= len(visible) {
			if *host.ColAfterLastY > 0 {
				return *host.ColAfterLastY
			}
			return idx * *host.ColRowH
		}
		if y, exists := (*host.ColRowYs)[idx]; exists {
			return y
		}
		return idx * *host.ColRowH
	}

	cursorY := rowYAt(srcStart) + int(*host.DragNodeCurrentY)
	cursorX := int(*host.DragNodeCurrentX)

	if src.Parent == nil {
		return dragRootDrop(host, cursorY, rowYAt)
	}
	return dragChildDrop(host, src, srcStart, cursorY, cursorX, metric, rowYAt)
}

func dragRootDrop(host *Host, cursorY int, rowYAt func(int) int) (nodeDropTarget, bool) {
	visible := *host.VisibleCols
	src := *host.DraggedNode

	type rootInfo struct {
		rowIdx int
	}
	var roots []rootInfo
	srcRootSibIdx := -1
	for i := 0; i < len(visible); i++ {
		if visible[i].Depth != 0 {
			continue
		}
		if visible[i] == src {
			srcRootSibIdx = len(roots)
		}
		roots = append(roots, rootInfo{rowIdx: i})
	}
	if srcRootSibIdx < 0 || len(roots) == 0 {
		return nodeDropTarget{}, false
	}

	bestGap := srcRootSibIdx
	bestDist := 1 << 30
	for gap := 0; gap <= len(roots); gap++ {
		var y int
		if gap < len(roots) {
			y = rowYAt(roots[gap].rowIdx)
		} else {
			y = *host.ColAfterLastY
		}
		d := cursorY - y
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			bestGap = gap
		}
	}

	insertIdx := bestGap
	if bestGap > srcRootSibIdx {
		insertIdx = bestGap - 1
	}

	var lineIdx int
	if bestGap < len(roots) {
		lineIdx = roots[bestGap].rowIdx
	} else {
		lineIdx = len(visible)
	}

	return nodeDropTarget{
		parent:    nil,
		insertIdx: insertIdx,
		lineIdx:   lineIdx,
		lineDepth: 0,
	}, true
}

func dragChildDrop(host *Host, src *collections.CollectionNode, srcStart, cursorY, cursorX int, metric unit.Metric, rowYAt func(int) int) (nodeDropTarget, bool) {
	visible := *host.VisibleCols

	srcEnd := srcStart + 1
	for srcEnd < len(visible) && visible[srcEnd].Depth > src.Depth {
		srcEnd++
	}

	depthPx := func(d int) int {
		return metric.Dp(unit.Dp(float32(d * 12)))
	}

	type slot struct {
		parent    *collections.CollectionNode
		insertIdx int
		intoNode  *collections.CollectionNode
		y         int
		x         int
		lineIdx   int
		lineDepth int
	}
	var slots []slot

	for i, node := range visible {
		if i >= srcStart && i < srcEnd {
			continue
		}
		if node.Collection != src.Collection {
			continue
		}
		yTop := rowYAt(i)
		yBot := rowYAt(i + 1)
		if yBot <= yTop {
			yBot = yTop + *host.ColRowH
		}

		if node.Parent != nil {
			sibIdx := siblingIndex(node)
			if sibIdx >= 0 {
				slots = append(slots, slot{
					parent:    node.Parent,
					insertIdx: sibIdx,
					y:         yTop,
					x:         depthPx(node.Depth),
					lineIdx:   i,
					lineDepth: node.Depth,
				})
			}
		}

		if node.IsFolder && !isAncestorOrSelf(src, node) {
			slots = append(slots, slot{
				parent:    node,
				insertIdx: 0,
				intoNode:  node,
				y:         (yTop + yBot) / 2,
				x:         depthPx(node.Depth + 1),
				lineIdx:   -1,
				lineDepth: node.Depth + 1,
			})
		}

		nextDepth := 0
		if i+1 < len(visible) {
			nextDepth = visible[i+1].Depth
		}
		if node.Parent != nil && node.Depth > nextDepth {
			for ancestor := node; ancestor != nil && ancestor.Parent != nil && ancestor.Depth > nextDepth; ancestor = ancestor.Parent {
				sibIdx := siblingIndex(ancestor)
				if sibIdx < 0 {
					continue
				}
				slots = append(slots, slot{
					parent:    ancestor.Parent,
					insertIdx: sibIdx + 1,
					y:         yBot,
					x:         depthPx(ancestor.Depth),
					lineIdx:   i + 1,
					lineDepth: ancestor.Depth,
				})
			}
		}
	}

	if len(slots) == 0 {
		return nodeDropTarget{}, false
	}

	bestIdx := -1
	bestDist := int64(1) << 60
	for i, s := range slots {
		dy := int64(cursorY - s.y)
		dx := int64(cursorX - s.x)
		dist := dy*dy*16 + dx*dx
		if dist < bestDist {
			bestDist = dist
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nodeDropTarget{}, false
	}
	chosen := slots[bestIdx]
	return nodeDropTarget{
		parent:    chosen.parent,
		insertIdx: chosen.insertIdx,
		intoNode:  chosen.intoNode,
		lineIdx:   chosen.lineIdx,
		lineDepth: chosen.lineDepth,
	}, true
}

func commitNodeDrop(host *Host, src *collections.CollectionNode, metric unit.Metric) {
	if src == nil {
		return
	}
	drop, ok := dragNodeDrop(host, metric)
	if !ok {
		return
	}

	if src.Parent == nil {
		if drop.parent != nil {
			return
		}
		oldIdx := -1
		var movedColUI *collections.CollectionUI
		for i, c := range *host.Collections {
			if c != nil && c.Data != nil && c.Data.Root == src {
				movedColUI = c
				oldIdx = i
				break
			}
		}
		if movedColUI == nil || oldIdx < 0 {
			return
		}
		insertIdx := drop.insertIdx
		if insertIdx == oldIdx {
			return
		}
		if insertIdx < 0 {
			insertIdx = 0
		}
		if insertIdx > len(*host.Collections) {
			insertIdx = len(*host.Collections)
		}
		*host.Collections = append((*host.Collections)[:oldIdx], (*host.Collections)[oldIdx+1:]...)
		if insertIdx > len(*host.Collections) {
			insertIdx = len(*host.Collections)
		}
		*host.Collections = append((*host.Collections)[:insertIdx], append([]*collections.CollectionUI{movedColUI}, (*host.Collections)[insertIdx:]...)...)
	} else {
		newParent := drop.parent
		if newParent == nil {
			return
		}
		if isAncestorOrSelf(src, newParent) {
			return
		}
		if newParent.Collection != src.Collection {
			return
		}

		oldParent := src.Parent
		oldIdx := siblingIndex(src)
		if oldIdx < 0 {
			return
		}

		insertIdx := drop.insertIdx
		if oldParent == newParent {
			if insertIdx == oldIdx || insertIdx == oldIdx+1 {
				return
			}
			oldParent.Children = append(oldParent.Children[:oldIdx], oldParent.Children[oldIdx+1:]...)
			if insertIdx > oldIdx {
				insertIdx--
			}
			if insertIdx < 0 {
				insertIdx = 0
			}
			if insertIdx > len(oldParent.Children) {
				insertIdx = len(oldParent.Children)
			}
			oldParent.Children = append(oldParent.Children[:insertIdx], append([]*collections.CollectionNode{src}, oldParent.Children[insertIdx:]...)...)
		} else {
			oldParent.Children = append(oldParent.Children[:oldIdx], oldParent.Children[oldIdx+1:]...)
			src.Parent = newParent
			if insertIdx < 0 {
				insertIdx = 0
			}
			if insertIdx > len(newParent.Children) {
				insertIdx = len(newParent.Children)
			}
			newParent.Children = append(newParent.Children[:insertIdx], append([]*collections.CollectionNode{src}, newParent.Children[insertIdx:]...)...)
			recalcDepth(src, newParent.Depth+1)
			if drop.intoNode != nil && !newParent.Expanded {
				newParent.Expanded = true
			}
		}
		host.MarkCollectionDirty(src.Collection)
	}
	host.UpdateVisibleCols()
	host.SaveState()
	host.Window.Invalidate()
}
