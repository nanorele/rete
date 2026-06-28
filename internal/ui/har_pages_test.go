package ui

import (
	"image"
	"testing"

	"github.com/nanorele/gio/io/input"
)

const harPagesDoc = `{
  "log": {
    "version": "1.2",
    "pages": [
      {"id":"page_1","title":"Home","startedDateTime":"2024-01-01T10:00:00Z"},
      {"id":"page_2","title":"About","startedDateTime":"2024-01-01T10:01:00Z"}
    ],
    "entries": [
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/a"},"response":{"status":200,"content":{"mimeType":"text/html","text":"a"}}},
      {"pageref":"page_1","request":{"method":"GET","url":"https://x/b"},"response":{"status":200,"content":{"mimeType":"text/css","text":"b"}}},
      {"pageref":"page_2","request":{"method":"GET","url":"https://x/c"},"response":{"status":200,"content":{"mimeType":"application/json","text":"c"}}}
    ]
  }
}`

func harPagesState(t *testing.T) *harState {
	t.Helper()
	st := &harState{}
	st.ensure()
	st.applyLoad([]byte(harPagesDoc), "p.har", nil)
	if st.Doc == nil || len(st.Doc.Pages) != 2 || len(st.Doc.Entries) != 3 {
		t.Fatalf("precondition: doc must load 2 pages / 3 entries, got %+v", st.Doc)
	}
	return st
}

func TestHarPages_FilterByPage(t *testing.T) {
	st := harPagesState(t)

	if got := st.visibleIndices(); len(got) != 3 {
		t.Fatalf("no filter must show all 3 entries, got %v", got)
	}
	st.selectPage("page_1")
	if got := st.visibleIndices(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Errorf("page_1 filter = %v, want [0 1]", got)
	}
	st.selectPage("page_2")
	if got := st.visibleIndices(); len(got) != 1 || got[0] != 2 {
		t.Errorf("page_2 filter = %v, want [2]", got)
	}
	st.selectPage("")
	if got := st.visibleIndices(); len(got) != 3 {
		t.Errorf("cleared filter = %v, want all 3", got)
	}
}

func TestHarPages_SelectMovesSelection(t *testing.T) {
	st := harPagesState(t)
	st.selectPage("page_2")
	if st.SelReq != 2 {
		t.Errorf("SelReq = %d, want 2 (first entry of page_2)", st.SelReq)
	}
	st.selectPage("")
	if st.SelReq != 0 {
		t.Errorf("SelReq after reset = %d, want 0", st.SelReq)
	}
}

func TestHarReqLabel_ReflectsPageFilter(t *testing.T) {
	st := harPagesState(t)
	if got := harReqLabel(st); got != "3" {
		t.Errorf("unfiltered req label = %q, want 3", got)
	}
	st.selectPage("page_1")
	if got := harReqLabel(st); got != "2" {
		t.Errorf("page_1 req label = %q, want 2", got)
	}
	if got := harPagesLabel(st); got != "2" {
		t.Errorf("pages label = %q, want 2", got)
	}
}

func TestHarPageRequestCount(t *testing.T) {
	st := harPagesState(t)
	if got := st.pageRequestCount("page_1"); got != 2 {
		t.Errorf("page_1 count = %d, want 2", got)
	}
	if got := st.pageRequestCount("page_2"); got != 1 {
		t.Errorf("page_2 count = %d, want 1", got)
	}
}

func TestHarPages_ResetOnReload(t *testing.T) {
	st := harPagesState(t)
	st.selectPage("page_2")
	st.applyLoad([]byte(harPagesDoc), "p2.har", nil)
	if st.SelPageID != "" {
		t.Errorf("reload must clear the page filter, got %q", st.SelPageID)
	}
	if got := st.visibleIndices(); len(got) != 3 {
		t.Errorf("reload must show all entries, got %v", got)
	}
}

func TestHARSection_PagesTabRenders(t *testing.T) {
	ui := harTestUI(t)
	ui.HARView.ensure()
	ui.HARView.applyLoad([]byte(harPagesDoc), "p.har", nil)
	ui.HARView.TopTab = harTabPages

	var r input.Router
	if d := layoutHARTwice(&r, image.Pt(1100, 620), ui.layoutHARSection); d.Size.Y <= 0 {
		t.Fatal("pages tab failed to render")
	}
}
