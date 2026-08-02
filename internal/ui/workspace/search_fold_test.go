package workspace

import (
	"strings"
	"testing"
)

func TestFoldForSearch_PreservesByteLength(t *testing.T) {
	for _, s := range []string{
		"Hello World",
		"ПРИВЕТ Мир",
		"Grüße, Straße",
		"ΑΘΗΝΑ",
		"İstanbul KELVIN K",
		"🚀 emoji ЖЖ",
		"",
	} {
		if got := foldForSearch(s); len(got) != len(s) {
			t.Errorf("foldForSearch(%q) changed byte length: %d -> %d", s, len(s), len(got))
		}
	}
}

func TestSearch_CaseInsensitiveCyrillic(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText(`{"имя": "Привет", "город": "МОСКВА", "note": "привет again"}`)

	for _, q := range []string{"привет", "ПРИВЕТ", "ПрИвЕт"} {
		tab.RespSearch.Editor.SetText(q)
		tab.invalidateSearchCache()
		tab.RespSearch.recompute(tab.RespEditor.Text())
		if got := len(tab.RespSearch.spans); got != 2 {
			t.Errorf("query %q: expected 2 matches, got %d", q, got)
		}
		for _, m := range tab.RespSearch.spans {
			if !strings.EqualFold(tab.RespEditor.Text()[m.start:m.end], q) {
				t.Errorf("query %q: span [%d,%d) covers %q, not the match",
					q, m.start, m.end, tab.RespEditor.Text()[m.start:m.end])
			}
		}
	}

	tab.RespSearch.Editor.SetText("москва")
	tab.invalidateSearchCache()
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match for москва, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := tab.RespEditor.Text()[m.start:m.end]; got != "МОСКВА" {
		t.Errorf("span [%d,%d) covers %q, want МОСКВА", m.start, m.end, got)
	}
}

// A rune whose lowercase form is a different byte length used to shift every
// offset after it, so the highlight landed on unrelated text further down.
func TestSearch_OffsetsSurviveLengthChangingRunes(t *testing.T) {
	tab := NewRequestTab("t")
	body := "Kİ padding TARGET tail"
	tab.RespEditor.SetText(body)

	tab.RespSearch.Editor.SetText("target")
	tab.invalidateSearchCache()
	tab.RespSearch.recompute(body)
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := body[m.start:m.end]; got != "TARGET" {
		t.Errorf("span [%d,%d) covers %q, want TARGET", m.start, m.end, got)
	}
}

func TestSearch_StaleCacheRebuiltOnTextChange(t *testing.T) {
	tab := NewRequestTab("t")
	tab.RespEditor.SetText("alpha beta")
	tab.RespSearch.Editor.SetText("alpha")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match, got %d", got)
	}

	// A viewer that swaps its text without announcing it must not keep matching
	// against the previous document.
	tab.RespEditor.SetText("gamma delta alpha")
	tab.RespSearch.recompute(tab.RespEditor.Text())
	if got := len(tab.RespSearch.spans); got != 1 {
		t.Fatalf("expected 1 match after the swap, got %d", got)
	}
	m := tab.RespSearch.spans[0]
	if got := tab.RespEditor.Text()[m.start:m.end]; got != "alpha" {
		t.Errorf("stale cache: span [%d,%d) covers %q", m.start, m.end, got)
	}
}
