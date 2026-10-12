package ui

import (
	"image"
	"testing"

	"rete/internal/model"
	"rete/internal/ui/collections"
	"rete/internal/ui/tabbar"
	"rete/internal/ui/workspace"
)

func linkedTab(t *testing.T, u *AppUI, name string) *workspace.RequestTab {
	t.Helper()
	root := &collections.CollectionNode{Name: "col", IsFolder: true, Expanded: true}
	col := &collections.ParsedCollection{ID: "col-" + name, Name: "col", Root: root}
	node := &collections.CollectionNode{Name: name, Depth: 1, Request: &model.ParsedRequest{Name: name, Method: "GET", URL: "https://old.test", Headers: map[string]string{}}}
	root.Children = []*collections.CollectionNode{node}
	collections.AssignParents(root, nil, col)
	u.Collections = append(u.Collections, &collections.CollectionUI{Data: col})
	tab := workspace.NewRequestTab(name)
	tab.LinkedNode = node
	tab.URLInput.SetText("https://new.test")
	u.Tabs = append(u.Tabs, tab)
	return tab
}

func TestCloseDirtyTabAsksBeforeClosing(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	u := rig.ui
	u.TabBar = tabbar.NewStrip()
	clean := workspace.NewRequestTab("clean")
	u.Tabs = append(u.Tabs, clean)
	dirty := linkedTab(t, u, "dirty")
	dirty.IsDirty = true
	u.ActiveIdx = 1

	u.requestCloseTab(0)
	if u.closeConfirm.Open {
		t.Fatal("closing a clean tab must not ask")
	}
	if len(u.Tabs) != 1 || u.Tabs[0] != dirty {
		t.Fatalf("clean tab not closed: %d", len(u.Tabs))
	}

	u.requestCloseTab(0)
	if !u.closeConfirm.Open {
		t.Fatal("closing a dirty linked tab must open the confirmation")
	}
	if len(u.Tabs) != 1 {
		t.Fatal("tab must stay open until confirmed")
	}
	msg, names := closeConfirmMessage(u.closeConfirm.Dirty)
	if msg != "Do you want to save the changes you made to dirty?" || names != nil {
		t.Errorf("message = %q %v", msg, names)
	}

	u.cancelCloseConfirm()
	if u.closeConfirm.Open || len(u.Tabs) != 1 {
		t.Fatal("cancel must keep the tab")
	}

	u.requestCloseTab(0)
	u.resolveCloseConfirm(false)
	if len(u.Tabs) != 0 {
		t.Fatalf("discard must close the tab: %d", len(u.Tabs))
	}
	if dirty.LinkedNode.Request.URL != "https://old.test" {
		t.Errorf("discard must not save, url = %q", dirty.LinkedNode.Request.URL)
	}
}

func TestCloseDirtyTabSaveWritesCollection(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	u := rig.ui
	u.TabBar = tabbar.NewStrip()
	dirty := linkedTab(t, u, "dirty")
	dirty.IsDirty = true
	u.ActiveIdx = 0

	u.requestCloseTab(0)
	u.resolveCloseConfirm(true)
	if len(u.Tabs) != 0 {
		t.Fatalf("save must close the tab: %d", len(u.Tabs))
	}
	if got := dirty.LinkedNode.Request.URL; got != "https://new.test" {
		t.Errorf("save must write the tab into the collection, url = %q", got)
	}
	if _, ok := u.dirtyCollections["col-dirty"]; !ok {
		t.Error("saving must mark the collection dirty for flushing")
	}
}

func TestCloseOthersKeepsActiveAndAsksOnce(t *testing.T) {
	rig := newUIRig(t, image.Pt(800, 600))
	u := rig.ui
	u.TabBar = tabbar.NewStrip()
	keep := workspace.NewRequestTab("keep")
	u.Tabs = append(u.Tabs, keep)
	d1 := linkedTab(t, u, "one")
	d2 := linkedTab(t, u, "two")
	d1.IsDirty, d2.IsDirty = true, true
	clean := workspace.NewRequestTab("clean")
	u.Tabs = append(u.Tabs, clean)
	u.ActiveIdx = 0

	u.requestCloseTabs([]*workspace.RequestTab{d1, d2, clean})
	if !u.closeConfirm.Open || len(u.closeConfirm.Dirty) != 2 {
		t.Fatalf("expected one confirmation for two dirty tabs: open=%v dirty=%d", u.closeConfirm.Open, len(u.closeConfirm.Dirty))
	}
	msg, names := closeConfirmMessage(u.closeConfirm.Dirty)
	if msg != "Do you want to save the changes to the following 2 requests?" || len(names) != 2 {
		t.Errorf("message = %q %v", msg, names)
	}
	u.resolveCloseConfirm(false)
	if len(u.Tabs) != 1 || u.Tabs[0] != keep || u.ActiveIdx != 0 {
		t.Fatalf("close others must leave only the kept tab active: %d active=%d", len(u.Tabs), u.ActiveIdx)
	}
}
