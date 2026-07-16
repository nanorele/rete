package workspace

import (
	"strings"
	"testing"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
)

func TestSearchReseedsFromNewSelection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText(`{"name":"alice","role":"admin"}`)

	var r input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: r.Source()}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("name")
	tab.ReqSearch.refresh(&tab.ReqEditor, tab.ReqEditor.Text(), true)
	if got := len(tab.ReqSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for %q, got %d", "name", got)
	}

	idx := strings.Index(tab.ReqEditor.Text(), "role")
	tab.ReqEditor.selStart = idx
	tab.ReqEditor.selEnd = idx + len("role")
	if got := tab.ReqEditor.SelectedText(); got != "role" {
		t.Fatalf("selection setup failed, got %q", got)
	}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	if got := tab.ReqSearch.Editor.Text(); got != "role" {
		t.Fatalf("search box not reseeded from new selection: got %q, want %q", got, "role")
	}
	if got := len(tab.ReqSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for reseeded query, got %d", got)
	}
}

func TestSearchKeepsQueryWithoutSelection(t *testing.T) {
	tab := NewRequestTab("t")
	tab.ReqEditor.SetText(`{"name":"alice"}`)

	var r input.Router
	gtx := layout.Context{Ops: new(op.Ops), Source: r.Source()}

	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	tab.ReqSearch.Editor.SetText("name")
	tab.ReqSearch.closeOn(&tab.ReqEditor)

	tab.ReqEditor.SetCaret(0, 0)
	tab.toggleSearch(gtx, &tab.ReqSearch, &tab.ReqEditor)
	if got := tab.ReqSearch.Editor.Text(); got != "name" {
		t.Fatalf("search box query should be preserved when no selection: got %q", got)
	}
}
