package har

import (
	"strings"
	"testing"
)

func TestPrettyCode_JSON(t *testing.T) {
	out, ok := PrettyCode([]byte(`{"a":1,"b":[2,3]}`), "application/json")
	if !ok || !strings.Contains(string(out), "\n") {
		t.Fatalf("JSON should prettify: ok=%v out=%q", ok, out)
	}
}

func TestPrettyCode_MinifiedJS(t *testing.T) {
	src := `function f(a){if(a){return a+1;}else{return 0;}}var x={k:1,j:2};`
	out, ok := PrettyCode([]byte(src), "application/javascript")
	if !ok {
		t.Fatalf("minified JS should beautify")
	}
	s := string(out)
	if !strings.Contains(s, "\n") {
		t.Fatalf("beautified JS must contain newlines:\n%s", s)
	}
	if !strings.Contains(s, "\n  ") {
		t.Errorf("expected indentation in:\n%s", s)
	}
	t.Logf("beautified:\n%s", s)
}

func TestBeautify_DoesNotBreakStrings(t *testing.T) {
	src := `var s="a;b{c}d";f();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `"a;b{c}d"`) {
		t.Errorf("string literal was mangled:\n%s", out)
	}
}

func TestBeautify_DoesNotBreakRegex(t *testing.T) {
	src := `var re=/a;b{2}/g;f();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `/a;b{2}/g`) {
		t.Errorf("regex literal was mangled:\n%s", out)
	}
}

func TestBeautify_DoesNotBreakComments(t *testing.T) {
	src := `a();/* x;y{z} */b();// trailing;{}
c();`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), `/* x;y{z} */`) {
		t.Errorf("block comment mangled:\n%s", out)
	}
	if !strings.Contains(string(out), `// trailing;{}`) {
		t.Errorf("line comment mangled:\n%s", out)
	}
}

func TestBeautify_TemplateLiteral(t *testing.T) {
	src := "var t=`a;b{c}`;f();"
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "`a;b{c}`") {
		t.Errorf("template literal mangled:\n%s", out)
	}
}

func TestBeautify_BalancedBraces(t *testing.T) {
	src := `if(x){a();if(y){b();}}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	if strings.Count(s, "{") != strings.Count(src, "{") || strings.Count(s, "}") != strings.Count(src, "}") {
		t.Errorf("brace count changed:\n%s", s)
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	last := lines[len(lines)-1]
	if last != "}" {
		t.Errorf("last line should be a dedented '}', got %q\nfull:\n%s", last, s)
	}
}

func TestBeautify_ForHeaderStaysOnOneLine(t *testing.T) {
	src := `function f(){for(let e=0;e<n;e++)g(e);for(;x;)h();}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	if !strings.Contains(s, "for(let e=0; e<n; e++)") {
		t.Errorf("for header was split:\n%s", s)
	}
	if !strings.Contains(s, "for(; x;)") {
		t.Errorf("empty-init for header was split:\n%s", s)
	}
	if !strings.Contains(s, "g(e);\n") {
		t.Errorf("statement after for did not break:\n%s", s)
	}
}

func TestBeautify_ElseCatchFinallyStayAttached(t *testing.T) {
	src := `function f(){try{a();}catch{b();}finally{c();}if(x){d();}else{e();}}`
	out, _ := beautifyBraces([]byte(src), false)
	s := string(out)
	for _, want := range []string{"} catch{", "} finally{", "} else{"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q to stay on the closing-brace line:\n%s", want, s)
		}
	}
	if strings.Contains(s, "}\nelse") || strings.Contains(s, "}\ncatch") || strings.Contains(s, "}\nfinally") {
		t.Errorf("else/catch/finally dangled onto its own line:\n%s", s)
	}
}

func TestBeautify_DoWhileStaysAttached(t *testing.T) {
	src := `function f(){do{a();}while(x);}`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "} while(x)") {
		t.Errorf("do/while tail dangled:\n%s", out)
	}
}

func TestBeautify_DestructuringAssignmentNotSplit(t *testing.T) {
	src := `function f(r){let{body:e,...t}=JSON.parse(r);return e;}`
	out, _ := beautifyBraces([]byte(src), false)
	if !strings.Contains(string(out), "}=JSON.parse(r)") {
		t.Errorf("destructuring `}=` was split:\n%s", out)
	}
}

func TestPrettyCode_SvelteKitBundle(t *testing.T) {
	src := "import{a as e}from\"./x.js\";var c=class{constructor(e){this.s=e}" +
		"toString(){return JSON.stringify(this.s)}};function g(...e){let t=5381;" +
		"for(let n of e)if(typeof n==`string`){let e=n.length;for(;e;)t=t*33^n.charCodeAt(--e)}" +
		"else throw TypeError(`bad`);return(t>>>0).toString(36)}"
	out, ok := PrettyCode([]byte(src), "application/javascript")
	if !ok {
		t.Fatal("minified bundle should beautify")
	}
	s := string(out)
	if strings.Count(s, "{") != strings.Count(src, "{") || strings.Count(s, "}") != strings.Count(src, "}") {
		t.Errorf("brace count changed:\n%s", s)
	}
	if !strings.Contains(s, "for(; e;)") {
		t.Errorf("for header inside bundle was split:\n%s", s)
	}
	if !strings.Contains(s, "} else throw") {
		t.Errorf("else clause dangled in bundle:\n%s", s)
	}
}

func TestPrettyCode_PlainTextUnchanged(t *testing.T) {
	if out, ok := PrettyCode([]byte("just some words here"), "text/plain"); ok || string(out) != "just some words here" {
		t.Errorf("plain text must pass through, got ok=%v out=%q", ok, out)
	}
}

func TestLooksLikeBraceCode(t *testing.T) {
	if !looksLikeBraceCode([]byte("x"), "application/javascript") {
		t.Error("javascript mime should be code")
	}
	if !looksLikeBraceCode([]byte("a{b:1}"), "text/css") {
		t.Error("css mime should be code")
	}
	if !looksLikeBraceCode([]byte(`a();b();c();d={x:1};e();f();`), "text/plain") {
		t.Error("minified single line should be detected")
	}
	if looksLikeBraceCode([]byte("the quick brown fox\njumps over\nthe lazy dog"), "text/plain") {
		t.Error("prose should not be code")
	}
}
