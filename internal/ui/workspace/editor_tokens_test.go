package workspace

import (
	"image/color"
	"reflect"
	"testing"

	"tracto/internal/ui/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
)

func tokensSnapshot(v *RequestEditor) []syntax.Token {
	out := make([]syntax.Token, len(v.tokens))
	copy(out, v.tokens)
	return out
}

func setTokens(v *RequestEditor, toks []syntax.Token) {
	v.tokens = append(v.tokens[:0], toks...)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false
}

func TestShiftTokens_InsertBeforeAllTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`"hello"`)
	setTokens(v, []syntax.Token{{Start: 0, End: 7, Kind: syntax.TokString}})

	v.Insert(0, "X")
	if v.Text() != `X"hello"` {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 1, End: 8, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at 0:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertAtTokenEndDoesNotExtend(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	setTokens(v, []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}})

	v.Insert(3, "X")
	if v.Text() != "abcX" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at End boundary:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertAtTokenStartPushesToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	setTokens(v, []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}})

	v.Insert(0, " ")
	if v.Text() != " abc" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 1, End: 4, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert at Start:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertInsideTokenExtends(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("hello")
	setTokens(v, []syntax.Token{{Start: 0, End: 5, Kind: syntax.TokKeyword}})

	v.Insert(2, "XY")
	if v.Text() != "heXYllo" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 7, Kind: syntax.TokKeyword}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after insert inside token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_InsertBetweenTwoTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("AA BB")
	setTokens(v, []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 3, End: 5, Kind: syntax.TokKeyword},
	})

	v.Insert(2, "X")
	if v.Text() != "AAX BB" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 4, End: 6, Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after between-tokens insert:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteEntirelyAfterToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc def")
	setTokens(v, []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}})

	v.DeleteRange(4, 7)
	if v.Text() != "abc " {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete-after-token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteEntirelyBeforeToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc def")
	setTokens(v, []syntax.Token{{Start: 4, End: 7, Kind: syntax.TokString}})

	v.DeleteRange(0, 4)
	if v.Text() != "def" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 3, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete-before-token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteInsideTokenShrinks(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcdef")
	setTokens(v, []syntax.Token{{Start: 0, End: 6, Kind: syntax.TokString}})

	v.DeleteRange(2, 4)
	if v.Text() != "abef" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 4, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after inside-token delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteFullyContainsToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("aaXXbb")
	setTokens(v, []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 2, End: 4, Kind: syntax.TokString},
		{Start: 4, End: 6, Kind: syntax.TokKeyword},
	})

	v.DeleteRange(2, 4)
	if v.Text() != "aabb" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 2, End: 4, Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete that contains token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteSpanningPartialToken(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcDEFGHIjkl")
	setTokens(v, []syntax.Token{{Start: 3, End: 9, Kind: syntax.TokString}})

	v.DeleteRange(5, 12)
	if v.Text() != "abcDE" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 3, End: 5, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after partial-tail delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteSpanningTokenLeftEdge(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abcDEFGHIjkl")
	setTokens(v, []syntax.Token{{Start: 3, End: 9, Kind: syntax.TokString}})

	v.DeleteRange(0, 5)
	if v.Text() != "FGHIjkl" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{{Start: 0, End: 4, Kind: syntax.TokString}}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after partial-head delete:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_NoOpWhenNoTokens(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("abc")
	v.Insert(1, "X")
	if len(v.tokens) != 0 {
		t.Fatalf("expected no tokens, got %+v", v.tokens)
	}
	v.DeleteRange(0, 1)
	if len(v.tokens) != 0 {
		t.Fatalf("expected no tokens, got %+v", v.tokens)
	}
}

func TestShiftTokens_ReplaceShiftsCorrectly(t *testing.T) {
	v := NewRequestEditor()
	v.SetText("aa bb cc")
	setTokens(v, []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 3, End: 5, Kind: syntax.TokKeyword},
		{Start: 6, End: 8, Kind: syntax.TokKeyword},
	})

	v.Replace(2, 6, "X")
	if v.Text() != "aaXcc" {
		t.Fatalf("text: %q", v.Text())
	}
	want := []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 3, End: 5, Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after Replace:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestSpansAfterInsert_PreservesTrailingColor(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"name":"hello"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	var helloTok *syntax.Token
	for i := range v.tokens {
		if v.tokens[i].Kind == syntax.TokString && v.tokens[i].Start == 8 {
			helloTok = &v.tokens[i]
			break
		}
	}
	if helloTok == nil || helloTok.End != 15 {
		t.Fatalf("setup tokenization changed: tokens=%+v", v.tokens)
	}

	v.Insert(3, "X")
	if v.Text() != `{"nXame":"hello"}` {
		t.Fatalf("text after insert: %q", v.Text())
	}

	var got *syntax.Token
	for i := range v.tokens {
		if v.tokens[i].Kind == syntax.TokString && v.tokens[i].Start == 9 {
			got = &v.tokens[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("could not find shifted hello token; tokens=%+v", v.tokens)
	}
	if got.End != 16 {
		t.Fatalf("hello token End should be 16 (covers \"hello\" in new text), got %d", got.End)
	}
	if v.text[got.End-1] != '"' {
		t.Fatalf("expected closing quote at byte End-1, got %q (text=%q)",
			v.text[got.End-1], v.Text())
	}
}

func TestSpansForChunk_NoTrailingDropAfterInsert(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	v.Insert(5, "Y")
	if v.Text() != `{"k":Y"abc","m":"def"}` {
		t.Fatalf("text: %q", v.Text())
	}

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	spans := v.spansForChunk(0, len(v.text), palette, false)

	covered := make([]bool, len(v.text))
	colored := make([]color.NRGBA, len(v.text))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(covered); i++ {
			covered[i] = true
			colored[i] = sp.Color
		}
	}

	for i := 6; i <= 10; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of first string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	for i := 16; i <= 20; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of second string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	if covered[5] {
		t.Errorf("inserted byte at position 5 unexpectedly carries a syntax color")
	}
}

func TestSpansForChunk_NoTrailingDropAfterDelete(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	v.DeleteRange(1, 5)
	if v.Text() != `{"abc","m":"def"}` {
		t.Fatalf("text: %q", v.Text())
	}

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	spans := v.spansForChunk(0, len(v.text), palette, false)
	covered := make([]bool, len(v.text))
	for _, sp := range spans {
		for i := sp.Start; i < sp.End && i < len(covered); i++ {
			covered[i] = true
		}
	}
	for i := 11; i <= 15; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) lost its color after delete: text=%q", i, v.text[i], v.Text())
		}
	}
}

func TestSpansForChunk_StableWithoutEdits(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"v"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	palette := theme.SyntaxPalette{
		String: color.NRGBA{R: 1, A: 255},
		Key:    color.NRGBA{R: 2, A: 255},
	}
	want := v.spansForChunk(0, len(v.text), palette, false)
	got := v.spansForChunk(0, len(v.text), palette, false)
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("spansForChunk non-deterministic: %+v vs %+v", want, got)
	}
	if len(got) == 0 {
		t.Fatalf("expected non-empty spans for %q", v.Text())
	}
}

var _ = widgets.ColoredSpan{}
