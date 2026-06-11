package collections

import (
	"testing"
	"tracto/internal/model"
)

func TestCloneNodeSuffixOnlyTopLevel(t *testing.T) {
	folder := &CollectionNode{Name: "Auth", IsFolder: true}
	child := &CollectionNode{
		Name:    "Login",
		Parent:  folder,
		Request: &model.ParsedRequest{Name: "Login", Method: "POST"},
	}
	folder.Children = []*CollectionNode{child}

	dup := CloneNode(folder, nil)
	if dup.Name != "Auth Copy" {
		t.Errorf("top-level name = %q, want %q", dup.Name, "Auth Copy")
	}
	if len(dup.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(dup.Children))
	}
	if got := dup.Children[0].Name; got != "Login" {
		t.Errorf("child name = %q, want unchanged %q", got, "Login")
	}
	if got := dup.Children[0].Request.Name; got != "Login" {
		t.Errorf("child request name = %q, want %q", got, "Login")
	}
}

func TestCloneNodeCopiesExamples(t *testing.T) {
	node := &CollectionNode{
		Name: "Req",
		Request: &model.ParsedRequest{
			Name:     "Req",
			Method:   "GET",
			Examples: []model.ParsedExample{{Name: "ex1"}, {Name: "ex2"}},
		},
	}
	dup := CloneNode(node, nil)
	if len(dup.Request.Examples) != 2 {
		t.Fatalf("expected 2 examples copied, got %d", len(dup.Request.Examples))
	}
	// Ensure it's an independent slice.
	dup.Request.Examples[0].Name = "changed"
	if node.Request.Examples[0].Name != "ex1" {
		t.Error("clone shares Examples backing array with original")
	}
}
