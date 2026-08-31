package sidebar

import (
	"testing"

	"tracto/internal/ui/collections"
)

// f1 and f2 are expanded sibling folders; f3 is dragged from below into the
// gap between them, grabbed by its label (cursor X well right of the indent).
func expandedSiblingFolders() (host *Host, cleanup func(), root, f1, f2, f3 *collections.CollectionNode) {
	host, cleanup = newTestHost()

	root = mkNode("root", true)
	root.Expanded = true
	f1 = mkNode("f1", true)
	f1.Expanded = true
	c1 := mkNode("c1", false)
	f1.Children = []*collections.CollectionNode{c1}
	f2 = mkNode("f2", true)
	f2.Expanded = true
	c2 := mkNode("c2", false)
	f2.Children = []*collections.CollectionNode{c2}
	f3 = mkNode("f3", true)
	root.Children = []*collections.CollectionNode{f1, f2, f3}

	col := &collections.ParsedCollection{ID: "col", Name: "root", Root: root}
	collections.AssignParents(root, nil, col)
	recalcDepth(root, 0)

	*host.Collections = []*collections.CollectionUI{{Data: col}}
	visible := []*collections.CollectionNode{root, f1, c1, f2, c2, f3}
	*host.VisibleCols = visible
	*host.ColRowH = 20
	for i := range visible {
		(*host.ColRowYs)[i] = i * 20
	}
	*host.ColAfterLastY = len(visible) * 20
	return
}

func TestDropBetweenExpandedFolders(t *testing.T) {
	host, cleanup, root, _, f2, f3 := expandedSiblingFolders()
	defer cleanup()

	// f3 sits at row 5 (y=100); the gap between f1's subtree and f2 is y=60.
	// The pointer grabbed the row at its label, 60px from the row's left edge,
	// and never moved horizontally.
	*host.DraggedNode = f3
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 60
	*host.DragNodeCurrentX = 60
	*host.DragNodeOriginY = 10
	*host.DragNodeCurrentY = -40

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != root {
		t.Fatalf("drop between two expanded folders should stay in root; got parent=%q into=%q",
			parentName(drop.parent), parentName(drop.intoNode))
	}
	if drop.insertIdx != 1 {
		t.Fatalf("insertIdx = %d, want 1 (between f1 and f2)", drop.insertIdx)
	}

	commitNodeDrop(host, f3, unitMetric())
	if len(root.Children) != 3 || root.Children[1] != f3 {
		got := make([]string, 0, len(root.Children))
		for _, c := range root.Children {
			got = append(got, c.Name)
		}
		t.Fatalf("root children = %v, want [f1 f3 f2]", got)
	}
	if f2.Children[0].Name != "c2" || len(f2.Children) != 1 {
		t.Fatalf("f2 must be untouched, got %d children", len(f2.Children))
	}
}

// Moving the pointer right by one indent step still nests into the folder above.
func TestDropIntoFolderWithHorizontalIntent(t *testing.T) {
	host, cleanup, _, f1, _, f3 := expandedSiblingFolders()
	defer cleanup()

	*host.DraggedNode = f3
	*host.DragNodeActive = true
	*host.DragNodeOriginX = 60
	*host.DragNodeCurrentX = 60 + 12
	*host.DragNodeOriginY = 10
	*host.DragNodeCurrentY = -40

	drop, ok := dragNodeDrop(host, unitMetric())
	if !ok {
		t.Fatal("expected a drop target")
	}
	if drop.parent != f1 {
		t.Fatalf("dragging right of the gap should nest into f1; got parent=%q", parentName(drop.parent))
	}
}
