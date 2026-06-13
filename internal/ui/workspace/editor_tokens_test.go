package workspace

import (
	"image/color"
	"reflect"
	"testing"

	"tracto/internal/ui/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
)

// tokensSnapshot returns a shallow copy of v.tokens for assertions.
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
	// Token "abc" covers [0,3). Insert at 3 (just past the token).
	// The inserted byte must NOT be absorbed into the token; coloring of
	// the original three chars must be preserved exactly.
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
	// Insert at the exact Start of a token: the token shifts forward,
	// the inserted byte sits outside the token.
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
	// Insert STRICTLY inside a token: token grows by len(inserted).
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
	// "AA BB" with tokens {0,2} and {3,5}. Insert "X" at position 2
	// (exact End of first token == one before Start of second).
	//
	// Expected: first token must NOT extend (insert is at its End boundary),
	// second token must shift forward because its Start (3) > pos (2).
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
	// Middle token collapses to empty -> dropped. Trailing token shifts back.
	want := []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 2, End: 4, Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after delete that contains token:\n  got  %+v\n  want %+v", got, want)
	}
}

func TestShiftTokens_DeleteSpanningPartialToken(t *testing.T) {
	// Token [3, 9). Delete [5, 12). Original token chars 5..8 deleted;
	// chars 3..4 survive at positions 3..4. New token: [3, 5).
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
	// Token [3, 9). Delete [0, 5). Original chars 0..2 and 3..4 deleted;
	// token chars 5..8 survive at positions 0..3. New token: [0, 4).
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
	// Replace is DeleteRange + Insert under the hood; both must apply.
	v := NewRequestEditor()
	v.SetText("aa bb cc")
	setTokens(v, []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 3, End: 5, Kind: syntax.TokKeyword},
		{Start: 6, End: 8, Kind: syntax.TokKeyword},
	})

	// Replace " bb " (positions 2..6) with "X" -> "aaXcc".
	v.Replace(2, 6, "X")
	if v.Text() != "aaXcc" {
		t.Fatalf("text: %q", v.Text())
	}
	// Middle token fully inside delete -> dropped.
	// Last token shifts back by 3 (delete removed 4, then insert added 1).
	want := []syntax.Token{
		{Start: 0, End: 2, Kind: syntax.TokKeyword},
		{Start: 3, End: 5, Kind: syntax.TokKeyword},
	}
	if got := tokensSnapshot(v); !reflect.DeepEqual(got, want) {
		t.Fatalf("tokens after Replace:\n  got  %+v\n  want %+v", got, want)
	}
}

// TestSpansAfterInsert_PreservesTrailingColor is the regression test for the
// reported bug: after typing inside a JSON value, the last byte(s) of every
// token to the right of the cursor must keep their syntax color until the
// debounced re-tokenize fires.
func TestSpansAfterInsert_PreservesTrailingColor(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"name":"hello"}`)
	// Tokenize once so v.tokens is populated.
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	// Sanity: the "hello" string token (including the quotes) ends at 15.
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

	// Type 'X' at position 3 (inside the key "name"). After the insert,
	// before re-tokenize, the right-hand string token must still cover its
	// entire range in the NEW text.
	v.Insert(3, "X")
	if v.Text() != `{"nXame":"hello"}` {
		t.Fatalf("text after insert: %q", v.Text())
	}

	// Find the string token that originally was the value "hello".
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
	// Verify byte at End-1 is the closing quote (i.e., we cover the FULL word).
	if v.text[got.End-1] != '"' {
		t.Fatalf("expected closing quote at byte End-1, got %q (text=%q)",
			v.text[got.End-1], v.Text())
	}
}

// TestSpansForChunk_NoTrailingDropAfterInsert exercises the public path the
// renderer takes (spansForChunk) to prove that PaintColoredText receives
// spans that cover every byte the user expects, including the LAST byte of
// each word/token to the right of the edit point.
func TestSpansForChunk_NoTrailingDropAfterInsert(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	// Insert at position 4 (just before the first value's opening quote).
	// New text: {"k":Y"abc","m":"def"}
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

	// Every byte of "abc" (positions 7..9 in new text, plus surrounding quotes
	// at 6 and 10) must be covered by the string color.
	for i := 6; i <= 10; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of first string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	// Every byte of "def" must also be covered (this is the token furthest to
	// the right of the edit -- the most likely victim of stale offsets).
	for i := 16; i <= 20; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) of second string lost its color: text=%q", i, v.text[i], v.Text())
		}
		if colored[i] != palette.String {
			t.Errorf("byte %d (%q) wrong color: got %+v want %+v", i, v.text[i], colored[i], palette.String)
		}
	}
	// Newly inserted byte 'Y' (position 5) must NOT be claimed by any token
	// -- it sits outside every existing span.
	if covered[5] {
		t.Errorf("inserted byte at position 5 unexpectedly carries a syntax color")
	}
}

// TestSpansForChunk_NoTrailingDropAfterDelete covers the deletion side: after
// deleting a chunk, tokens to the right must shift back so the last byte of
// every word is still inside its span.
func TestSpansForChunk_NoTrailingDropAfterDelete(t *testing.T) {
	v := NewRequestEditor()
	v.SetText(`{"k":"abc","m":"def"}`)
	v.tokens = syntax.Tokenize(syntax.LangJSON, v.text)
	v.tokensLang = syntax.LangJSON
	v.tokensTxt = len(v.text)
	v.tokensDirty = false

	// Delete `"k":` (positions 1..5) -> {"abc","m":"def"} no wait,
	// positions 1..5 is `"k":` so result is `{"abc","m":"def"}`.
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
	// "def" (positions 12..14, surrounded by quotes at 11 and 15) must remain
	// fully covered after the delete.
	for i := 11; i <= 15; i++ {
		if !covered[i] {
			t.Errorf("byte %d (%q) lost its color after delete: text=%q", i, v.text[i], v.Text())
		}
	}
}

// guard against drift: a no-op token list mutation must not change spans.
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
	// And a couple of byte-level invariants on those spans, just to make
	// sure the helper above isn't comparing two empties.
	if len(got) == 0 {
		t.Fatalf("expected non-empty spans for %q", v.Text())
	}
}

// ensure the linker keeps widgets imported even if a future refactor removes
// all direct usages above; the spansForChunk return type is ColoredSpan, so
// this is here as documentation.
var _ = widgets.ColoredSpan{}
