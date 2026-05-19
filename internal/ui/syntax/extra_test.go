package syntax

import (
	"testing"
)

func runNoHang(t *testing.T, name string, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-timeout():
		t.Fatalf("%s hung", name)
	}
}

func validateTokens(t *testing.T, src []byte, toks []Token) {
	t.Helper()
	for i, tok := range toks {
		if tok.Start < 0 || tok.End > len(src) || tok.Start > tok.End {
			t.Errorf("invalid range tokens[%d]=%+v len(src)=%d", i, tok, len(src))
		}
	}
}

func TestTokenize_Dispatch(t *testing.T) {
	cases := []struct {
		lang    Lang
		src     []byte
		wantNil bool
	}{
		{LangJSON, []byte(`{"a":1}`), false},
		{LangXML, []byte(`<a/>`), false},
		{LangHTML, []byte(`<html></html>`), false},
		{LangYAML, []byte("a: 1\n"), false},
		{LangForm, []byte(`a=1`), false},
		{LangPlain, []byte(`whatever`), true},
		{Lang(99), []byte(`xxx`), true},
	}
	for _, c := range cases {
		got := Tokenize(c.lang, c.src)
		if c.wantNil && got != nil {
			t.Errorf("Tokenize(%v) = %v, want nil", c.lang, got)
		}
		if !c.wantNil && got == nil {
			t.Errorf("Tokenize(%v) returned nil, want tokens", c.lang)
		}
	}
}

func TestTokenizeJSON_Comments(t *testing.T) {
	cases := [][]byte{
		[]byte(`// line comment` + "\n" + `{"a":1}`),
		[]byte(`/* block */{"a":1}`),
		[]byte(`/* unterminated`),
		[]byte(`// unterminated trailing`),
		[]byte(`/*`),
		[]byte(`/`),
		[]byte(`/x`),
	}
	for _, src := range cases {
		toks := TokenizeJSON(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeJSON_NumberEdgeCases(t *testing.T) {
	// JSON disallows .5 and 5., trailing exponents, leading +. The tokenizer is permissive
	// and treats `-`, digits, `.`, `e`, `E`, `+`, `-` greedily.
	// TODO bug: json.go:119-133 accepts malformed numbers like "1e+", "--1", "1.2.3", "1ee5".
	cases := [][]byte{
		[]byte(`-0`),
		[]byte(`-`),
		[]byte(`1e+999`),
		[]byte(`1e`),
		[]byte(`1.2.3`),
		[]byte(`--1`),
		[]byte(`1ee5`),
		[]byte(`0.`),
		[]byte(`.5`),
	}
	for _, src := range cases {
		toks := TokenizeJSON(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeJSON_StringEscapes(t *testing.T) {
	cases := [][]byte{
		[]byte(`"a\nb"`),
		[]byte(`"aÿb"`),
		[]byte(`"a\\"`),
		[]byte(`"unterminated`),
		[]byte(`"line` + "\n" + `break"`),
		[]byte(`"\`),
		[]byte(`"\"`),
		[]byte(`""`),
	}
	for _, src := range cases {
		toks := TokenizeJSON(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeJSON_UnknownByteEmitsTokPlain(t *testing.T) {
	src := []byte(`@#$`)
	toks := TokenizeJSON(src)
	found := false
	for _, tok := range toks {
		if tok.Kind == TokPlain && tok.Start >= 0 && tok.End <= len(src) && tok.End > tok.Start {
			found = true
		}
	}
	if !found {
		t.Errorf("garbage bytes must surface as TokPlain (not silently dropped), got %+v", toks)
	}
}

func TestTokenizeJSON_DeepNesting(t *testing.T) {
	// depth is uint8; ensure no panic on very deep nesting.
	var src []byte
	for range 300 {
		src = append(src, '[')
	}
	for range 300 {
		src = append(src, ']')
	}
	runNoHang(t, "deep json", func() { _ = TokenizeJSON(src) })
}

func TestTokenizeJSON_TrailingComma(t *testing.T) {
	src := []byte(`{"a":1,}`)
	toks := TokenizeJSON(src)
	validateTokens(t, src, toks)
}

func TestTokenizeJSON_NonUTF8(t *testing.T) {
	src := []byte{'{', '"', 0xff, 0xfe, '"', ':', '1', '}'}
	runNoHang(t, "non-utf8 json", func() {
		toks := TokenizeJSON(src)
		validateTokens(t, src, toks)
	})
}

func TestTokenizeJSON_WhitespaceOnly(t *testing.T) {
	for _, src := range [][]byte{[]byte(""), []byte(" \t\n\r")} {
		toks := TokenizeJSON(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeXML_CDATA(t *testing.T) {
	src := []byte(`<a><![CDATA[hello <world> & stuff]]></a>`)
	toks := TokenizeXML(src)
	var hasCdata bool
	for _, tok := range toks {
		if tok.Kind == TokString && tok.End-tok.Start > 5 {
			hasCdata = true
		}
	}
	if !hasCdata {
		t.Error("expected CDATA emitted as TokString")
	}
}

func TestTokenizeXML_CDATAAtExactEnd(t *testing.T) {
	src := []byte(`<![CDATA[`)
	runNoHang(t, "cdata-at-end", func() {
		toks := TokenizeXML(src)
		validateTokens(t, src, toks)
	})
}

func TestTokenizeXML_CommentAtExactEnd(t *testing.T) {
	src := []byte(`<!--`)
	runNoHang(t, "comment-at-end", func() {
		toks := TokenizeXML(src)
		validateTokens(t, src, toks)
	})
}

func TestTokenizeXML_ProcessingInstruction(t *testing.T) {
	src := []byte(`<?xml version="1.0" encoding="UTF-8"?><root/>`)
	toks := TokenizeXML(src)
	var hasPI bool
	for _, tok := range toks {
		if tok.Kind == TokKeyword && tok.Start == 0 {
			text := string(src[tok.Start:tok.End])
			if len(text) > 4 && text[0] == '<' && text[1] == '?' {
				hasPI = true
			}
		}
	}
	if !hasPI {
		t.Error("expected processing instruction emitted as TokKeyword")
	}
}

func TestTokenizeXML_Doctype(t *testing.T) {
	src := []byte(`<!DOCTYPE html><html></html>`)
	toks := TokenizeXML(src)
	validateTokens(t, src, toks)
}

func TestTokenizeXML_Namespaces(t *testing.T) {
	src := []byte(`<ns:root xmlns:ns="urn:x"><ns:child/></ns:root>`)
	toks := TokenizeXML(src)
	var foundNamespaceTag bool
	for _, tok := range toks {
		if tok.Kind == TokKeyword && string(src[tok.Start:tok.End]) == "ns:root" {
			foundNamespaceTag = true
		}
	}
	if !foundNamespaceTag {
		t.Error("expected namespaced tag name to be tokenized as one keyword")
	}
}

func TestTokenizeXML_SelfClosing(t *testing.T) {
	src := []byte(`<a/><b x="1"/><c><d/></c>`)
	toks := TokenizeXML(src)
	maxDepth := uint8(0)
	for _, tok := range toks {
		if tok.Depth > maxDepth {
			maxDepth = tok.Depth
		}
	}
	if maxDepth > 1 {
		t.Errorf("self-closing tags should not increase depth beyond 1, got %d", maxDepth)
	}
}

func TestTokenizeXML_UnquotedAttr(t *testing.T) {
	src := []byte(`<a x=1 y=hello z="q">body</a>`)
	toks := TokenizeXML(src)
	validateTokens(t, src, toks)
}

func TestTokenizeXML_SingleQuoteAttr(t *testing.T) {
	src := []byte(`<a x='1' y='hello'>body</a>`)
	toks := TokenizeXML(src)
	var foundSingleQuoted bool
	for _, tok := range toks {
		if tok.Kind == TokString && tok.End > tok.Start && src[tok.Start] == '\'' {
			foundSingleQuoted = true
		}
	}
	if !foundSingleQuoted {
		t.Error("expected single-quoted attr value")
	}
}

func TestTokenizeXML_NestedAndMixed(t *testing.T) {
	src := []byte(`<root><a><b><c/></b></a></root>`)
	toks := TokenizeXML(src)
	var maxDepth uint8
	for _, tok := range toks {
		if tok.Depth > maxDepth {
			maxDepth = tok.Depth
		}
	}
	if maxDepth < 3 {
		t.Errorf("expected depth >=3, got %d", maxDepth)
	}
}

func TestTokenizeXML_UnclosedDeep(t *testing.T) {
	src := []byte(`<a><b><c>`)
	runNoHang(t, "unclosed xml", func() {
		_ = TokenizeXML(src)
	})
}

func TestTokenizeXML_LonelyBracket(t *testing.T) {
	src := []byte(`<`)
	runNoHang(t, "single-<", func() {
		toks := TokenizeXML(src)
		validateTokens(t, src, toks)
	})
}

func TestTokenizeYAML_BlockScalars(t *testing.T) {
	src := []byte("desc: |\n  line one\n  line two\nother: >\n  folded text\n")
	runNoHang(t, "block-scalar yaml", func() {
		toks := TokenizeYAML(src)
		validateTokens(t, src, toks)
	})
}

func TestTokenizeYAML_AnchorsAliases(t *testing.T) {
	src := []byte("base: &b\n  a: 1\nuser: *b\n")
	toks := TokenizeYAML(src)
	var anchors, aliases int
	for _, tok := range toks {
		if tok.Kind == TokOperator {
			if tok.End > tok.Start {
				switch src[tok.Start] {
				case '&':
					anchors++
				case '*':
					aliases++
				}
			}
		}
	}
	if anchors == 0 {
		t.Error("expected anchor token")
	}
	if aliases == 0 {
		t.Error("expected alias token")
	}
}

func TestTokenizeYAML_Tags(t *testing.T) {
	src := []byte("v: !!str hello\nx: !custom value\n")
	toks := TokenizeYAML(src)
	var tags int
	for _, tok := range toks {
		if tok.Kind == TokType {
			tags++
		}
	}
	if tags < 2 {
		t.Errorf("expected >=2 tag tokens, got %d", tags)
	}
}

func TestTokenizeYAML_MultiDoc(t *testing.T) {
	src := []byte("---\na: 1\n...\n---\nb: 2\n")
	toks := TokenizeYAML(src)
	var seps int
	for _, tok := range toks {
		if tok.Kind == TokKeyword && tok.End-tok.Start == 3 {
			seps++
		}
	}
	if seps < 3 {
		t.Errorf("expected >=3 doc separators, got %d", seps)
	}
}

func TestTokenizeYAML_FlowMapping(t *testing.T) {
	src := []byte("obj: {a: 1, b: 2, c: [1,2,3]}\n")
	toks := TokenizeYAML(src)
	var brackets int
	for _, tok := range toks {
		if tok.Kind == TokBracket {
			brackets++
		}
	}
	if brackets != 4 {
		t.Errorf("expected 4 bracket tokens (2 pairs), got %d", brackets)
	}
}

func TestTokenizeYAML_QuotedStrings(t *testing.T) {
	cases := [][]byte{
		[]byte(`x: "hello world"` + "\n"),
		[]byte(`x: 'single quoted'` + "\n"),
		[]byte(`x: "escape \" quote"` + "\n"),
		[]byte(`x: "unterminated` + "\n"),
		[]byte(`x: 'unterminated` + "\n"),
		[]byte(`x: "with\nnewline"` + "\n"),
		[]byte(`x: "\`),
	}
	for _, src := range cases {
		toks := TokenizeYAML(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeYAML_ScalarClassification(t *testing.T) {
	cases := []struct {
		in   string
		kind TokenKind
	}{
		{"true", TokBool},
		{"True", TokBool},
		{"YES", TokBool},
		{"off", TokBool},
		{"null", TokNull},
		{"~", TokNull},
		{"NULL", TokNull},
		{"-42", TokNumber},
		{"+3.14", TokNumber},
		{"1e10", TokNumber},
		{"abc", TokString},
		{"", TokString},
		{"-", TokString},
		{"+", TokString},
	}
	for _, c := range cases {
		got := classifyYAMLScalar([]byte(c.in))
		if got != c.kind {
			t.Errorf("classifyYAMLScalar(%q) = %v, want %v", c.in, got, c.kind)
		}
	}
}

func TestTokenizeYAML_EmptyAndWhitespace(t *testing.T) {
	for _, src := range [][]byte{[]byte(""), []byte("\n\n\n"), []byte("   \t\r\n")} {
		toks := TokenizeYAML(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeYAML_CommentOnly(t *testing.T) {
	src := []byte("# just a comment\n# another\n")
	toks := TokenizeYAML(src)
	var comments int
	for _, tok := range toks {
		if tok.Kind == TokComment {
			comments++
		}
	}
	if comments != 2 {
		t.Errorf("expected 2 comments, got %d", comments)
	}
}

func TestTokenizeYAML_InlineComment(t *testing.T) {
	src := []byte("k: v # trailing\nx: 1\n")
	toks := TokenizeYAML(src)
	var comments int
	for _, tok := range toks {
		if tok.Kind == TokComment {
			comments++
		}
	}
	if comments != 1 {
		t.Errorf("expected 1 inline comment, got %d", comments)
	}
}

func TestTokenizeYAML_DashWithoutSpace(t *testing.T) {
	// "-foo" at line start is not a list marker (requires space) — should not emit punctuation.
	src := []byte("-foo\n")
	toks := TokenizeYAML(src)
	for _, tok := range toks {
		if tok.Kind == TokPunctuation && tok.End-tok.Start == 1 && src[tok.Start] == '-' {
			t.Errorf("'-foo' incorrectly treated as list marker: %+v", tok)
		}
	}
}

func TestTokenizeYAML_URLValue(t *testing.T) {
	// "url: http://example.com" — the value contains `:`. Make sure parser doesn't choke.
	src := []byte("url: http://example.com\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
}

func TestTokenizeYAML_BlockScalarLiteral(t *testing.T) {
	src := []byte("key: |\n  line1\n  line2\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var indicator, body *Token
	for i := range toks {
		tk := &toks[i]
		if tk.Kind == TokKeyword && tk.End-tk.Start >= 1 && src[tk.Start] == '|' {
			indicator = tk
		}
		if tk.Kind == TokString && tk.End-tk.Start > 4 && src[tk.Start] == ' ' {
			body = tk
		}
	}
	if indicator == nil {
		t.Fatal("expected TokKeyword for '|' indicator")
	}
	if string(src[indicator.Start:indicator.End]) != "|" {
		t.Errorf("indicator = %q, want %q", src[indicator.Start:indicator.End], "|")
	}
	if body == nil {
		t.Fatal("expected TokString covering indented body")
	}
	if !bytesContain(src[body.Start:body.End], "line1") || !bytesContain(src[body.Start:body.End], "line2") {
		t.Errorf("body did not cover both lines: %q", src[body.Start:body.End])
	}
}

func TestTokenizeYAML_BlockScalarFolded(t *testing.T) {
	src := []byte("key: >\n  text\n  more\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var indicator *Token
	for i := range toks {
		tk := &toks[i]
		if tk.Kind == TokKeyword && tk.End-tk.Start >= 1 && src[tk.Start] == '>' {
			indicator = tk
		}
	}
	if indicator == nil {
		t.Fatal("expected TokKeyword for '>' indicator")
	}
	if string(src[indicator.Start:indicator.End]) != ">" {
		t.Errorf("indicator = %q, want %q", src[indicator.Start:indicator.End], ">")
	}
}

func TestTokenizeYAML_BlockScalarChomping(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{"key: |-\n  x\n", "|-"},
		{"key: |+\n  y\n", "|+"},
		{"key: >-\n  z\n", ">-"},
		{"key: >+\n  w\n", ">+"},
	}
	for _, c := range cases {
		src := []byte(c.src)
		toks := TokenizeYAML(src)
		validateTokens(t, src, toks)
		var found bool
		for _, tk := range toks {
			if tk.Kind == TokKeyword && string(src[tk.Start:tk.End]) == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("indicator %q not found in tokens for src %q: %+v", c.want, c.src, toks)
		}
	}
}

func TestTokenizeYAML_BlockScalarIndentDigit(t *testing.T) {
	src := []byte("key: |2\n  z\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var found bool
	for _, tk := range toks {
		if tk.Kind == TokKeyword && string(src[tk.Start:tk.End]) == "|2" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected indicator '|2', got tokens=%+v", toks)
	}
}

func TestTokenizeYAML_RegularScalarUnchanged(t *testing.T) {
	src := []byte("key: value\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var key, punct, val *Token
	for i := range toks {
		tk := &toks[i]
		switch tk.Kind {
		case TokKey:
			key = tk
		case TokPunctuation:
			punct = tk
		case TokString:
			val = tk
		}
	}
	if key == nil || string(src[key.Start:key.End]) != "key" {
		t.Errorf("want TokKey 'key', got %+v", key)
	}
	if punct == nil || string(src[punct.Start:punct.End]) != ":" {
		t.Errorf("want TokPunctuation ':', got %+v", punct)
	}
	if val == nil || string(src[val.Start:val.End]) != "value" {
		t.Errorf("want TokString 'value', got %+v", val)
	}
}

func TestTokenizeYAML_BlockScalarEndOfInput(t *testing.T) {
	srcs := [][]byte{
		[]byte("key: |"),
		[]byte("key: >"),
		[]byte("key: |-"),
		[]byte("key: |\n"),
		[]byte("key: |2"),
	}
	for _, src := range srcs {
		runNoHang(t, "block-scalar-eof "+string(src), func() {
			toks := TokenizeYAML(src)
			validateTokens(t, src, toks)
		})
	}
}

func TestTokenizeYAML_BlockScalarTerminatedByDedent(t *testing.T) {
	src := []byte("a: |\n  body line\nb: 2\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var sawB bool
	for _, tk := range toks {
		if tk.Kind == TokKey && string(src[tk.Start:tk.End]) == "b" {
			sawB = true
		}
	}
	if !sawB {
		t.Errorf("expected key 'b' after block scalar dedent, got %+v", toks)
	}
}

func bytesContain(b []byte, s string) bool {
	if len(s) == 0 {
		return true
	}
	for i := 0; i+len(s) <= len(b); i++ {
		match := true
		for j := range len(s) {
			if b[i+j] != s[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestTokenizeForm_EdgeCases(t *testing.T) {
	cases := [][]byte{
		[]byte(``),
		[]byte(`=value`),
		[]byte(`key=`),
		[]byte(`&&&`),
		[]byte(`a=1&&b=2`),
		[]byte(`a=1;b=2`),
		[]byte(`a%20b=c%21d`),
		[]byte(`solo`),
		[]byte(`=`),
		[]byte(`a==b`),
	}
	for _, src := range cases {
		toks := TokenizeForm(src)
		validateTokens(t, src, toks)
	}
}

func TestTokenizeForm_SemicolonSep(t *testing.T) {
	src := []byte(`a=1;b=2`)
	toks := TokenizeForm(src)
	var seps int
	for _, tok := range toks {
		if tok.Kind == TokPunctuation && src[tok.Start] == ';' {
			seps++
		}
	}
	if seps != 1 {
		t.Errorf("expected 1 semicolon separator, got %d", seps)
	}
}

func TestTokenizeForm_NonUTF8(t *testing.T) {
	src := []byte{0xff, 0xfe, '=', 0xff, 0xfe, '&', 'b', '=', '1'}
	runNoHang(t, "non-utf8 form", func() {
		toks := TokenizeForm(src)
		validateTokens(t, src, toks)
	})
}

func TestDetect_AllContentTypes(t *testing.T) {
	cases := []struct {
		ct   string
		want Lang
	}{
		{"text/xml", LangXML},
		{"text/xml; charset=utf-8", LangXML},
		{"application/atom+xml", LangXML},
		{"text/html; charset=utf-8", LangHTML},
		{"application/xhtml+xml", LangHTML},
		{"application/yaml", LangYAML},
		{"text/yaml", LangYAML},
		{"application/x-yaml", LangYAML},
		{"application/x-www-form-urlencoded", LangForm},
		{"APPLICATION/JSON", LangJSON},
		{"  application/json  ", LangJSON},
		{"text/plain", LangPlain},
		{"", LangPlain},
		{";", LangPlain},
		{"application/octet-stream", LangPlain},
	}
	for _, c := range cases {
		if got := Detect(c.ct, nil); got != c.want {
			t.Errorf("Detect(%q) = %v, want %v", c.ct, got, c.want)
		}
	}
}

func TestDetect_BodySniffEdges(t *testing.T) {
	cases := []struct {
		body []byte
		want Lang
	}{
		{nil, LangPlain},
		{[]byte(""), LangPlain},
		{[]byte("   \t\r\n  "), LangPlain},
		{[]byte("%YAML 1.2\n"), LangYAML},
		{[]byte("# comment\nkey: val\n"), LangYAML},
		{[]byte("---\nname: A\n"), LangYAML},
		{[]byte("key: val\n"), LangYAML},
		{[]byte("a=1&b=2"), LangForm},
		{[]byte("just plain text"), LangPlain},
		{[]byte("-x"), LangPlain},
		{[]byte("<svg></svg>"), LangXML},
		{[]byte("<HTML>"), LangHTML},
		{[]byte("<htm"), LangXML},
	}
	for _, c := range cases {
		if got := Detect("", c.body); got != c.want {
			t.Errorf("Detect(_, %q) = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestHelpers_HasASCII(t *testing.T) {
	src := []byte("truefoo")
	if hasASCII(src, 0, "true") {
		t.Error("hasASCII should reject when followed by letter")
	}
	src2 := []byte("true,")
	if !hasASCII(src2, 0, "true") {
		t.Error("hasASCII should accept when followed by punctuation")
	}
	src3 := []byte("true")
	if !hasASCII(src3, 0, "true") {
		t.Error("hasASCII should accept at EOF")
	}
	if hasASCII([]byte("tru"), 0, "true") {
		t.Error("hasASCII should reject when src is too short")
	}
	if hasASCII([]byte("xrue"), 0, "true") {
		t.Error("hasASCII should reject byte mismatch")
	}
	if hasASCII([]byte("true1"), 0, "true") {
		t.Error("hasASCII should reject when followed by digit")
	}
	if hasASCII([]byte("true_"), 0, "true") {
		t.Error("hasASCII should reject when followed by underscore")
	}
}

func TestHelpers_HasBytes(t *testing.T) {
	if !hasBytes([]byte("abcdef"), 1, "bcd") {
		t.Error("hasBytes should match")
	}
	if hasBytes([]byte("abc"), 1, "bcd") {
		t.Error("hasBytes should reject when too short")
	}
	if hasBytes([]byte("abcdef"), 1, "xyz") {
		t.Error("hasBytes should reject mismatch")
	}
}

func TestHelpers_TrimLowerEnds(t *testing.T) {
	if trimSpace("   ") != "" {
		t.Error("trimSpace all whitespace")
	}
	if trimSpace("\tabc\t") != "abc" {
		t.Errorf("trimSpace tab: %q", trimSpace("\tabc\t"))
	}
	if trimSpace("") != "" {
		t.Error("trimSpace empty")
	}
	if toLower("ABC") != "abc" {
		t.Error("toLower ABC")
	}
	if toLower("") != "" {
		t.Error("toLower empty")
	}
	if string(toLowerBytes([]byte("ABCDEF"), 100)) != "abcdef" {
		t.Error("toLowerBytes n>len")
	}
	if string(toLowerBytes([]byte("ABC"), 0)) != "" {
		t.Error("toLowerBytes n=0")
	}
	if !endsWith("foobar", "bar") {
		t.Error("endsWith match")
	}
	if endsWith("foo", "foobar") {
		t.Error("endsWith should reject when suffix longer")
	}
	if !hasPrefix([]byte("abcdef"), "abc") {
		t.Error("hasPrefix match")
	}
	if hasPrefix([]byte("ab"), "abc") {
		t.Error("hasPrefix should reject when shorter")
	}
	if hasPrefix([]byte("xbcdef"), "abc") {
		t.Error("hasPrefix should reject mismatch")
	}
}
