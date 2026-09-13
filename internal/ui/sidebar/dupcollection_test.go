package sidebar

import (
	"encoding/json"
	"image"
	"testing"

	"rete/internal/ui/collections"
)

func TestCollectionRootMenuOffersDuplicate(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	req := rig.addRequest(root, "req", "GET")
	rig.frames(2)

	has := func(n *collections.CollectionNode, label string) bool {
		for _, it := range nodeMenuItems(n) {
			if it.Label == label {
				return true
			}
		}
		return false
	}
	if !has(root, "Duplicate") {
		t.Fatal("collection root menu has no Duplicate item")
	}
	if !has(req, "Duplicate") {
		t.Fatal("request menu lost its Duplicate item")
	}
	for _, it := range nodeMenuItems(root) {
		if it.Label == "Duplicate" && it.Click != &root.DupBtn {
			t.Fatal("root Duplicate item is not wired to root.DupBtn")
		}
	}
}

func TestDuplicateCollectionCopiesExtras(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 900))
	root := rig.addCollection("Col")
	root.Collection.InfoExtras = map[string]json.RawMessage{
		"_postman_id": json.RawMessage(`"abc"`),
		"schema":      json.RawMessage(`"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"`),
	}
	root.Collection.TopExtras = map[string]json.RawMessage{
		"auth":     json.RawMessage(`{"type":"bearer"}`),
		"variable": json.RawMessage(`[{"key":"host","value":"x"}]`),
	}
	rig.frames(2)

	rig.click(&root.MenuBtn)
	rig.click(&root.DupBtn)
	if len(*rig.host.Collections) != 2 {
		t.Fatalf("collections = %d, want 2", len(*rig.host.Collections))
	}
	dup := (*rig.host.Collections)[1].Data
	if _, ok := dup.InfoExtras["_postman_id"]; ok {
		t.Error("duplicate must not reuse the source _postman_id")
	}
	if string(dup.InfoExtras["schema"]) != string(root.Collection.InfoExtras["schema"]) {
		t.Errorf("schema not copied: %s", dup.InfoExtras["schema"])
	}
	for k, v := range root.Collection.TopExtras {
		if string(dup.TopExtras[k]) != string(v) {
			t.Errorf("TopExtras[%q] = %s, want %s", k, dup.TopExtras[k], v)
		}
	}
	dup.TopExtras["auth"][0] = 'X'
	if root.Collection.TopExtras["auth"][0] == 'X' {
		t.Error("TopExtras shared with the source instead of copied")
	}
}
