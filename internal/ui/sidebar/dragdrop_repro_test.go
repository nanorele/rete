package sidebar

import (
	"testing"

	"tracto/internal/ui/collections"
)

func twoCollections(withB func(rB *collections.CollectionNode)) (rA, a0, a1, rB *collections.CollectionNode, colA, colB *collections.ParsedCollection) {
	rA = mkNode("A", true)
	rA.Expanded = true
	a0 = mkNode("a0", false)
	a1 = mkNode("a1", false)
	rA.Children = []*collections.CollectionNode{a0, a1}
	colA = &collections.ParsedCollection{ID: "A", Root: rA}
	collections.AssignParents(rA, nil, colA)
	recalcDepth(rA, 0)

	rB = mkNode("B", true)
	colB = &collections.ParsedCollection{ID: "B", Root: rB}
	collections.AssignParents(rB, nil, colB)
	recalcDepth(rB, 0)
	if withB != nil {
		withB(rB)
	}
	return
}

func TestDragChildStaysInCollectionWhenOtherRootOffscreen(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	rA, a0, a1, rB, colA, colB := twoCollections(func(rB *collections.CollectionNode) { rB.Expanded = true })

	*host.Collections = []*collections.CollectionUI{{Data: colB}, {Data: colA}}
	visible := []*collections.CollectionNode{rB, rA, a0, a1}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	// rB(0) and rA(1) are scrolled above the viewport: only a0(2) and a1(3) laid out.
	(*host.ColRowYs)[2] = 0
	(*host.ColRowYs)[3] = 20
	*host.ColAfterLastY = 40

	*host.DraggedNode = a0
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 12
	*host.DragNodeCurrentX = 12
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 6

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent == rB || (drop.parent != nil && drop.parent.Collection == colB) {
		t.Fatalf("child fell into off-screen collection B; parent=%q", parentName(drop.parent))
	}
	if drop.parent == nil || drop.parent.Collection != colA {
		t.Fatalf("child should stay within collection A; got parent=%q", parentName(drop.parent))
	}
}

func TestDragChildIntoCollapsedTargetCollection(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()

	rA, a0, a1, rB, colA, colB := twoCollections(nil)

	*host.Collections = []*collections.CollectionUI{{Data: colA}, {Data: colB}}
	visible := []*collections.CollectionNode{rA, a0, a1, rB}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	for i := range visible {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = 80

	*host.DraggedNode = a0
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 12
	*host.DragNodeCurrentX = 12
	*host.DragNodeOriginY = 0
	*host.DragNodeCurrentY = 43

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != rB {
		t.Fatalf("child dropped on collection B header should go INTO B; got parent=%q", parentName(drop.parent))
	}
}

func parentName(n *collections.CollectionNode) string {
	if n == nil {
		return "<nil>"
	}
	return n.Name
}
