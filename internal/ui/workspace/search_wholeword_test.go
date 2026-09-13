package workspace

import "testing"

func spanTexts(text string, spans []matchSpan) []string {
	out := make([]string, 0, len(spans))
	for _, m := range spans {
		out = append(out, text[m.start:m.end])
	}
	return out
}

func TestSearch_WholeWord(t *testing.T) {
	tab := NewRequestTab("t")
	body := "cat concatenate cat. Cat-cat _cat /cat/ кот, коты"
	tab.RespEditor.SetText(body)
	box := &tab.RespSearch

	box.Editor.SetText("cat")
	tab.invalidateSearchCache()
	box.recompute(body)
	if got := len(box.spans); got != 7 {
		t.Fatalf("substring mode: expected 7 matches, got %d %v", got, spanTexts(body, box.spans))
	}

	box.WholeWord = true
	box.recompute(body)
	want := []string{"cat", "cat", "Cat", "cat", "cat"}
	got := spanTexts(body, box.spans)
	if len(got) != len(want) {
		t.Fatalf("whole word: expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("whole word match %d = %q, want %q", i, got[i], want[i])
		}
	}
	for _, m := range box.spans {
		if m.start > 0 && body[m.start-1] == '_' {
			t.Errorf("underscore is a word character; _cat must not match whole-word")
		}
	}

	box.Editor.SetText("кот")
	box.recompute(body)
	if got := spanTexts(body, box.spans); len(got) != 1 || got[0] != "кот" {
		t.Errorf("cyrillic whole word: got %v, want [кот]", got)
	}

	box.Editor.SetText("cat.")
	box.recompute(body)
	if got := spanTexts(body, box.spans); len(got) != 1 {
		t.Errorf("a query ending in a separator is whole at its own edge: got %v", got)
	}
}

func TestSearch_PerDocumentState(t *testing.T) {
	tab := NewRequestTab("t")
	box := &tab.RespSearch
	ed := tab.RespEditor

	docA := "a a a"
	docB := "b b"

	ed.SetText(docA)
	box.SetDocument("A", ed)
	box.Open = true
	box.Editor.SetText("a")
	box.refresh(ed, ed.Text(), true)
	box.navigate(1, ed)
	if box.current != 1 || len(box.spans) != 3 {
		t.Fatalf("precondition: doc A on match 2/3, got %d/%d", box.current+1, len(box.spans))
	}

	ed.SetText(docB)
	box.SetDocument("B", ed)
	if box.Open || box.Editor.Text() != "" || box.current != -1 {
		t.Fatalf("a never-searched document must start closed and empty, got open=%v query=%q current=%d", box.Open, box.Editor.Text(), box.current)
	}
	if len(ed.searchSpans) != 0 {
		t.Errorf("switching to a closed document must clear the viewer's match spans")
	}
	box.Open = true
	box.Editor.SetText("b")
	box.refresh(ed, ed.Text(), true)
	if box.current != 0 || len(box.spans) != 2 {
		t.Fatalf("doc B: expected 1/2, got %d/%d", box.current+1, len(box.spans))
	}

	ed.SetText(docA)
	box.SetDocument("A", ed)
	if !box.Open || box.Editor.Text() != "a" {
		t.Fatalf("doc A must come back open with its own query, got open=%v query=%q", box.Open, box.Editor.Text())
	}
	if !box.cacheDirty {
		t.Errorf("switching documents must mark the fold cache dirty")
	}
	box.refresh(ed, ed.Text(), false)
	if box.current != 1 || len(box.spans) != 3 {
		t.Errorf("doc A must resume on match 2/3, got %d/%d", box.current+1, len(box.spans))
	}
	if box.query != "a" {
		t.Errorf("restored query must not read as a fresh edit: query=%q", box.query)
	}

	ed.SetText(docB)
	box.SetDocument("B", ed)
	box.refresh(ed, ed.Text(), false)
	if !box.Open || box.Editor.Text() != "b" || box.current != 0 {
		t.Errorf("doc B must resume on 1/2 with query b, got open=%v query=%q current=%d", box.Open, box.Editor.Text(), box.current)
	}

	box.closeOn(ed)
	ed.SetText(docA)
	box.SetDocument("A", ed)
	if !box.Open {
		t.Errorf("closing doc B must not close doc A")
	}
	box.SetDocument("A", ed)
	if !box.Open || box.Editor.Text() != "a" {
		t.Errorf("re-setting the same key must be a no-op")
	}
}

func TestSearch_DocStateEviction(t *testing.T) {
	tab := NewRequestTab("t")
	box := &tab.RespSearch
	ed := tab.RespEditor
	ed.SetText("x")
	for i := 0; i < maxSearchDocs+20; i++ {
		box.SetDocument(string(rune('a'+i%26))+string(rune('a'+i/26)), ed)
		box.Open = true
		box.Editor.SetText("x")
	}
	if len(box.docs) > maxSearchDocs {
		t.Errorf("parked document states must stay bounded, got %d", len(box.docs))
	}
}
