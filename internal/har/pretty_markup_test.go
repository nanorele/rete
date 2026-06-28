package har

import (
	"strings"
	"testing"
)

func TestPrettyCode_HTML(t *testing.T) {
	src := `<!DOCTYPE html><html><head><title>x</title></head><body><div class="a"><p>hi</p></div></body></html>`
	out, ok := PrettyCode([]byte(src), "text/html")
	if !ok {
		t.Fatal("minified HTML should beautify")
	}
	s := string(out)
	if !strings.Contains(s, "\n") {
		t.Fatalf("HTML must gain line breaks:\n%s", s)
	}
	if !strings.Contains(s, "<title>x</title>") {
		t.Errorf("short leaf element should be inline:\n%s", s)
	}
	if !strings.Contains(s, "<p>hi</p>") {
		t.Errorf("short leaf element should be inline:\n%s", s)
	}
	if !strings.Contains(s, "\n      <p>hi</p>") {
		t.Errorf("expected indentation for nested <p>:\n%s", s)
	}
}

func TestPrettyMarkup_VoidElements(t *testing.T) {
	src := `<body><img src="a.png"><br><input type="text"/><p>x</p></body>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	for _, want := range []string{"\n  <img src=\"a.png\">", "\n  <br>", "\n  <input type=\"text\"/>", "\n  <p>x</p>"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected %q at body depth (void elements must not nest):\n%s", want, s)
		}
	}
}

func TestPrettyMarkup_AttributesWithBrackets(t *testing.T) {
	src := `<div data-x="a>b" title='c<d'><p>hi</p></div>`
	out, _ := beautifyMarkup([]byte(src))
	if !strings.Contains(string(out), `<div data-x="a>b" title='c<d'>`) {
		t.Errorf("attribute with brackets was mis-parsed:\n%s", out)
	}
}

func TestPrettyMarkup_CommentsAndDoctype(t *testing.T) {
	src := `<!DOCTYPE html><div><!-- keep me --><p>x</p></div>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	if !strings.Contains(s, "<!DOCTYPE html>") {
		t.Errorf("doctype lost:\n%s", s)
	}
	if !strings.Contains(s, "<!-- keep me -->") {
		t.Errorf("comment lost:\n%s", s)
	}
}

func TestPrettyMarkup_EmbeddedScriptAndStyle(t *testing.T) {
	src := `<head><style>body{margin:0}</style><script>function f(){return 1;}</script></head>`
	out, _ := beautifyMarkup([]byte(src))
	s := string(out)
	if !strings.Contains(s, "body{\n") || !strings.Contains(s, "margin:0") {
		t.Errorf("embedded CSS not beautified:\n%s", s)
	}
	if !strings.Contains(s, "function f(){\n") || !strings.Contains(s, "return 1;") {
		t.Errorf("embedded JS not beautified:\n%s", s)
	}
	if !strings.Contains(s, "</style>") || !strings.Contains(s, "</script>") {
		t.Errorf("raw-text element close tags lost:\n%s", s)
	}
}

func TestPrettyMarkup_CaseInsensitiveRawClose(t *testing.T) {
	src := `<SCRIPT>var a=1;</SCRIPT>`
	out, _ := beautifyMarkup([]byte(src))
	if !strings.Contains(string(out), "</SCRIPT>") {
		t.Errorf("upper-case </SCRIPT> not matched:\n%s", out)
	}
}

func TestPrettyCode_XML(t *testing.T) {
	src := `<?xml version="1.0"?><root><item id="1"><name>a</name></item></root>`
	out, ok := PrettyCode([]byte(src), "application/xml")
	if !ok {
		t.Fatal("minified XML should beautify")
	}
	s := string(out)
	if !strings.Contains(s, `<?xml version="1.0"?>`) {
		t.Errorf("XML declaration lost:\n%s", s)
	}
	if !strings.Contains(s, "<name>a</name>") {
		t.Errorf("leaf element should be inline:\n%s", s)
	}
}

func TestPrettyCode_SVGByMime(t *testing.T) {
	src := `<svg viewBox="0 0 1 1"><rect x="0" y="0"/></svg>`
	if _, ok := PrettyCode([]byte(src), "image/svg+xml"); !ok {
		t.Error("svg mime should route to markup beautifier")
	}
}

func TestPrettyCode_MarkupSniffWithoutMime(t *testing.T) {
	src := `<ul><li>a</li><li>b</li></ul>`
	if _, ok := PrettyCode([]byte(src), ""); !ok {
		t.Error("markup should be detected from content when mime is absent")
	}
	if out, ok := PrettyCode([]byte("a < b and c < d"), "text/plain"); ok {
		t.Errorf("prose with '<' must pass through, got:\n%s", out)
	}
}

func TestPrettyCode_AlreadyFormattedMarkupIsNoop(t *testing.T) {
	src := "<div>\n  <p>x</p>\n</div>"
	if out, ok := PrettyCode([]byte(src), "text/html"); ok {
		t.Errorf("already-formatted markup should report no change, got:\n%s", out)
	}
}

func TestPrettyCode_CSSRulesBreakOnNewSelector(t *testing.T) {
	src := `body{margin:0}.a,.b{color:red}@media(max-width:600px){.a{display:none}}`
	out, ok := PrettyCode([]byte(src), "text/css")
	if !ok {
		t.Fatal("minified CSS should beautify")
	}
	s := string(out)
	if strings.Contains(s, "}.a") {
		t.Errorf("new CSS selector glued to previous rule's '}':\n%s", s)
	}
	if !strings.Contains(s, "\n.a,.b{") {
		t.Errorf("expected '.a,.b' selector on its own line:\n%s", s)
	}
}
