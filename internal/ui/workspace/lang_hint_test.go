package workspace

import (
	"testing"

	"tracto/internal/ui/syntax"
)

func TestRequestLang_PrefersHint(t *testing.T) {
	tab := NewRequestTab("test")
	tab.ReqEditor.SetText("not obviously any language")

	if got := tab.requestLang(); got != syntax.LangPlain {
		t.Fatalf("precondition: plain body should sniff as LangPlain, got %v", got)
	}

	tab.ReqLangHint = syntax.LangJSON
	if got := tab.requestLang(); got != syntax.LangJSON {
		t.Errorf("requestLang() = %v, want LangJSON from the hint", got)
	}
}

func TestRequestLang_ContentTypeBeatsHint(t *testing.T) {
	tab := NewRequestTab("test")
	tab.AddHeader("Content-Type", "application/json")
	tab.ReqLangHint = syntax.LangPlain
	if got := tab.requestLang(); got != syntax.LangJSON {
		t.Errorf("requestLang() = %v, want LangJSON from the Content-Type header", got)
	}
}
