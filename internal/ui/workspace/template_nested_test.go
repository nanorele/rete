package workspace

import (
	"testing"

	"rete/internal/ui/widgets"
)

func TestProcessTemplateNestedBraces(t *testing.T) {
	env := map[string]string{"sdgsgds": "X"}
	in := `'[[[['''''[[[[[[[[{{{{{{{{{{{{{sdgsgds}}}}}}}}}}}}}]]]]]]]]''''']]]]'`
	want := `'[[[['''''[[[[[[[[{{{{{{{{{{{X}}}}}}}}}}}]]]]]]]]''''']]]]'`
	if got := processTemplate(in, env); got != want {
		t.Fatalf("processTemplate = %q, want %q", got, want)
	}
	if got := processTemplate("{{a}b}} {{a}}", map[string]string{"a": "1"}); got != "{{a}b}} 1" {
		t.Fatalf("a brace inside a name must not form a variable, got %q", got)
	}
}

func TestClipSpansToVarsUsesInnermostVariable(t *testing.T) {
	chunk := []byte("{{{{v}}}}")
	spans := []widgets.ColoredSpan{{Start: 0, End: len(chunk)}}
	got := clipSpansToVars(spans, chunk)
	if len(got) != 2 || got[0].End != 2 || got[1].Start != 7 {
		t.Fatalf("spans should be cut around {{v}} only, got %+v", got)
	}
}
