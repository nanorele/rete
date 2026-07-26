package ui

import (
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/workspace"
)

func reqNode(name string, depth int) *collections.CollectionNode {
	return &collections.CollectionNode{
		Name:    name,
		Depth:   depth,
		Request: &model.ParsedRequest{Name: name},
	}
}

func colWithTree(id string, root *collections.CollectionNode) *collections.CollectionUI {
	pc := &collections.ParsedCollection{ID: id, Name: id, Root: root}
	var mark func(n *collections.CollectionNode)
	mark = func(n *collections.CollectionNode) {
		n.Collection = pc
		for _, c := range n.Children {
			c.Parent = n
			mark(c)
		}
	}
	mark(root)
	return &collections.CollectionUI{Data: pc}
}

func TestRelinkTabs_ResolvesPendingNode(t *testing.T) {
	leaf := reqNode("get-user", 1)
	root := node("root", true, true, 0, leaf)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.PendingColID = "c1"
	tab.PendingNodePath = []int{0}

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.relinkTabs()

	if tab.LinkedNode != leaf {
		t.Fatalf("LinkedNode = %v, want the leaf node", tab.LinkedNode)
	}
	if tab.PendingColID != "" {
		t.Errorf("PendingColID = %q, want cleared", tab.PendingColID)
	}
	if tab.PendingNodePath != nil {
		t.Errorf("PendingNodePath = %v, want nil", tab.PendingNodePath)
	}
}

func TestRelinkTabs_LeavesUnresolvable(t *testing.T) {
	cases := []struct {
		name    string
		colID   string
		path    []int
		wantCol string
	}{
		{"unknown collection", "nope", []int{0}, "nope"},
		{"path out of range", "c1", []int{5}, "c1"},
		{"path into missing child", "c1", []int{0, 3}, "c1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := node("root", true, true, 0, reqNode("get-user", 1))
			col := colWithTree("c1", root)

			tab := workspace.NewRequestTab("t")
			tab.PendingColID = c.colID
			tab.PendingNodePath = c.path

			ui := &AppUI{}
			ui.Collections = []*collections.CollectionUI{col}
			ui.Tabs = []*workspace.RequestTab{tab}
			ui.relinkTabs()

			if tab.LinkedNode != nil {
				t.Error("LinkedNode was set from an unresolvable path")
			}
			if tab.PendingColID != c.wantCol {
				t.Errorf("PendingColID = %q, want %q retained for a later retry", tab.PendingColID, c.wantCol)
			}
		})
	}
}

func TestRelinkTabs_SkipsFolderNodes(t *testing.T) {
	folder := node("fld", true, false, 1)
	root := node("root", true, true, 0, folder)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.PendingColID = "c1"
	tab.PendingNodePath = []int{0}

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.relinkTabs()

	if tab.LinkedNode != nil {
		t.Error("a folder node (no Request) was linked to a request tab")
	}
}

func TestRelinkTabs_IgnoresAlreadyLinkedAndUnpending(t *testing.T) {
	leaf := reqNode("a", 1)
	other := reqNode("b", 1)
	root := node("root", true, true, 0, leaf, other)
	col := colWithTree("c1", root)

	linked := workspace.NewRequestTab("linked")
	linked.LinkedNode = other
	linked.PendingColID = "c1"
	linked.PendingNodePath = []int{0}

	plain := workspace.NewRequestTab("plain")

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{linked, plain}
	ui.relinkTabs()

	if linked.LinkedNode != other {
		t.Error("relinkTabs overwrote an already-linked tab")
	}
	if plain.LinkedNode != nil {
		t.Error("relinkTabs linked a tab with no pending reference")
	}
}

func TestRevealLinkedNode_ExpandsAncestors(t *testing.T) {
	leaf := reqNode("deep", 2)
	folder := node("fld", true, false, 1, leaf)
	root := node("root", true, false, 0, folder)
	col := colWithTree("c1", root)

	tab := workspace.NewRequestTab("t")
	tab.LinkedNode = leaf

	ui := &AppUI{}
	ui.Collections = []*collections.CollectionUI{col}
	ui.Tabs = []*workspace.RequestTab{tab}
	ui.revealLinkedNode(tab)

	if !folder.Expanded {
		t.Error("parent folder was not expanded")
	}
	if !root.Expanded {
		t.Error("root was not expanded")
	}
	if len(ui.VisibleCols) == 0 {
		t.Error("visible collections were not rebuilt after expanding")
	}
}

func TestRevealLinkedNode_NoopCases(t *testing.T) {
	ui := &AppUI{}
	ui.revealLinkedNode(nil)

	tab := workspace.NewRequestTab("t")
	ui.revealLinkedNode(tab)

	orphan := reqNode("orphan", 1)
	tab.LinkedNode = orphan
	ui.revealLinkedNode(tab)

	if len(ui.VisibleCols) != 0 {
		t.Error("revealLinkedNode rebuilt visible list for an unreachable node")
	}
}
