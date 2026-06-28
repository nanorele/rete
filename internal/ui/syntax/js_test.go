package syntax

import "testing"

func kindOf(t *testing.T, src, sub string) TokenKind {
	t.Helper()
	idx := indexOf(src, sub)
	if idx < 0 {
		t.Fatalf("substring %q not in %q", sub, src)
	}
	toks := TokenizeJS([]byte(src))
	for _, tk := range toks {
		if tk.Start <= idx && idx < tk.End {
			return tk.Kind
		}
	}
	return TokPlain
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestJS_Keywords(t *testing.T) {
	src := `function f(){const x=1;return x;}`
	for _, kw := range []string{"function", "const", "return"} {
		if k := kindOf(t, src, kw); k != TokKeyword {
			t.Errorf("%q kind=%d, want keyword", kw, k)
		}
	}
}

func TestJS_StringsAndTemplate(t *testing.T) {
	if k := kindOf(t, `var s="hi";`, `"hi"`); k != TokString {
		t.Errorf("double-quote string kind=%d", k)
	}
	if k := kindOf(t, `var s='hi';`, `'hi'`); k != TokString {
		t.Errorf("single-quote string kind=%d", k)
	}
	if k := kindOf(t, "var s=`a${b}c`;", "`a${b}c`"); k != TokTemplate {
		t.Errorf("template kind=%d", k)
	}
}

func TestJS_Regex(t *testing.T) {
	if k := kindOf(t, `var re=/ab+c/gi;`, `/ab+c/gi`); k != TokRegex {
		t.Errorf("regex kind=%d, want regex", k)
	}
	if k := kindOf(t, `var x=a/b;`, "/"); k != TokOperator {
		t.Errorf("division '/' kind=%d, want operator", k)
	}
	toks := TokenizeJS([]byte(`x=/[/]/;`))
	foundRegex := false
	for _, tk := range toks {
		if tk.Kind == TokRegex {
			foundRegex = true
		}
	}
	if !foundRegex {
		t.Error("regex with char class not detected")
	}
}

func TestJS_Numbers(t *testing.T) {
	for _, num := range []string{"0xFF", "0b1010", "0o17", "1_000", "3.14", "1e10", "42n", "1.5e-3"} {
		src := "var x=" + num + ";"
		if k := kindOf(t, src, num); k != TokNumber {
			t.Errorf("number %q kind=%d, want number", num, k)
		}
	}
}

func TestJS_Comments(t *testing.T) {
	if k := kindOf(t, "a();// note\nb();", "// note"); k != TokComment {
		t.Errorf("line comment kind=%d", k)
	}
	if k := kindOf(t, "a();/* note */b();", "/* note */"); k != TokComment {
		t.Errorf("block comment kind=%d", k)
	}
}

func TestJS_Booleans_Null_Constants(t *testing.T) {
	if k := kindOf(t, "var x=true;", "true"); k != TokBool {
		t.Errorf("true kind=%d", k)
	}
	if k := kindOf(t, "var x=null;", "null"); k != TokNull {
		t.Errorf("null kind=%d", k)
	}
	if k := kindOf(t, "var x=undefined;", "undefined"); k != TokConstant {
		t.Errorf("undefined kind=%d, want constant", k)
	}
	if k := kindOf(t, "var x=NaN;", "NaN"); k != TokConstant {
		t.Errorf("NaN kind=%d, want constant", k)
	}
}

func TestJS_BuiltinsAndContext(t *testing.T) {
	if k := kindOf(t, "console.log(x)", "console"); k != TokType {
		t.Errorf("console kind=%d, want type", k)
	}
	if k := kindOf(t, "console.log(x)", "log"); k != TokFunction {
		t.Errorf("log kind=%d, want function", k)
	}
	if k := kindOf(t, "var n=obj.name;", "name"); k != TokProperty {
		t.Errorf("name kind=%d, want property", k)
	}
	if k := kindOf(t, "doThing(1)", "doThing"); k != TokFunction {
		t.Errorf("doThing kind=%d, want function", k)
	}
	if k := kindOf(t, "function myFn(){}", "myFn"); k != TokFunction {
		t.Errorf("myFn kind=%d, want function", k)
	}
	if k := kindOf(t, "class Widget {}", "Widget"); k != TokType {
		t.Errorf("Widget kind=%d, want type", k)
	}
	if k := kindOf(t, "var total = a + b;", "total"); k != TokPlain {
		t.Errorf("total kind=%d, want plain", k)
	}
}

func TestJS_Operators(t *testing.T) {
	src := "a===b&&c=>d"
	toks := TokenizeJS([]byte(src))
	want := map[string]bool{"===": false, "&&": false, "=>": false}
	for _, tk := range toks {
		if tk.Kind == TokOperator {
			s := src[tk.Start:tk.End]
			if _, ok := want[s]; ok {
				want[s] = true
			}
		}
	}
	for op, found := range want {
		if !found {
			t.Errorf("operator %q not tokenized", op)
		}
	}
}

func TestJS_BracketDepthCycles(t *testing.T) {
	toks := TokenizeJS([]byte(`a({b:[1]})`))
	var depths []uint8
	for _, tk := range toks {
		if tk.Kind == TokBracket {
			depths = append(depths, tk.Depth)
		}
	}
	want := []uint8{0, 1, 2, 2, 1, 0}
	if len(depths) != len(want) {
		t.Fatalf("bracket depths = %v, want %v", depths, want)
	}
	for i := range want {
		if depths[i] != want[i] {
			t.Errorf("bracket %d depth=%d, want %d (all=%v)", i, depths[i], want[i], depths)
		}
	}
}

func TestJS_NoOverlapAndOrdered(t *testing.T) {
	src := `function f(a,b){ return a/b + /re/.test(s); } // tail`
	toks := TokenizeJS([]byte(src))
	prev := 0
	for _, tk := range toks {
		if tk.Start < prev {
			t.Fatalf("tokens overlap/out of order at %d (prev end %d): %+v", tk.Start, prev, tk)
		}
		if tk.End < tk.Start || tk.End > len(src) {
			t.Fatalf("token out of bounds: %+v (len %d)", tk, len(src))
		}
		prev = tk.End
	}
}

func TestJS_PunctuationTerminates(t *testing.T) {
	src := `let o={a:1,b:2};f(a,b);`
	toks := TokenizeJS([]byte(src))
	for _, p := range []string{";", ",", ":"} {
		if k := kindOf(t, src, p); k != TokPunctuation {
			t.Errorf("%q kind=%d, want punctuation", p, k)
		}
	}
	for _, tk := range toks {
		if tk.End <= tk.Start {
			t.Fatalf("zero-width token: %+v", tk)
		}
	}
}

func TestDetect_JS(t *testing.T) {
	for _, ct := range []string{
		"application/javascript", "text/javascript", "application/x-javascript",
		"application/ecmascript", "text/javascript; charset=utf-8", "application/typescript",
	} {
		if l := Detect(ct, nil); l != LangJS {
			t.Errorf("Detect(%q) = %d, want LangJS", ct, l)
		}
	}
	if l := Detect("text/plain", nil); l == LangJS {
		t.Error("text/plain must not detect as JS")
	}
}

func TestTokenizeDispatchesJS(t *testing.T) {
	toks := Tokenize(LangJS, []byte(`const x=1;`))
	if len(toks) == 0 {
		t.Fatal("Tokenize(LangJS) returned no tokens")
	}
}
