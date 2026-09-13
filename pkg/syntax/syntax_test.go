package syntax

import (
	"strings"
	"testing"
	"time"
	"unsafe"
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
		if tok.Start < 0 || int(tok.End()) > len(src) || tok.Start > tok.End() {
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
		if tok.Kind == TokPlain && tok.Start >= 0 && int(tok.End()) <= len(src) && tok.End() > tok.Start {
			found = true
		}
	}
	if !found {
		t.Errorf("garbage bytes must surface as TokPlain (not silently dropped), got %+v", toks)
	}
}

func TestTokenizeJSON_DeepNesting(t *testing.T) {

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
		if tok.Kind == TokString && tok.End()-tok.Start > 5 {
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
			text := string(src[tok.Start:tok.End()])
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
		if tok.Kind == TokKeyword && string(src[tok.Start:tok.End()]) == "ns:root" {
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
		if tok.Kind == TokString && tok.End() > tok.Start && src[tok.Start] == '\'' {
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
			if tok.End() > tok.Start {
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
		if tok.Kind == TokKeyword && tok.End()-tok.Start == 3 {
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

	src := []byte("-foo\n")
	toks := TokenizeYAML(src)
	for _, tok := range toks {
		if tok.Kind == TokPunctuation && tok.End()-tok.Start == 1 && src[tok.Start] == '-' {
			t.Errorf("'-foo' incorrectly treated as list marker: %+v", tok)
		}
	}
}

func TestTokenizeYAML_URLValue(t *testing.T) {

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
		if tk.Kind == TokKeyword && tk.End()-tk.Start >= 1 && src[tk.Start] == '|' {
			indicator = tk
		}
		if tk.Kind == TokString && tk.End()-tk.Start > 4 && src[tk.Start] == ' ' {
			body = tk
		}
	}
	if indicator == nil {
		t.Fatal("expected TokKeyword for '|' indicator")
	}
	if string(src[indicator.Start:indicator.End()]) != "|" {
		t.Errorf("indicator = %q, want %q", src[indicator.Start:indicator.End()], "|")
	}
	if body == nil {
		t.Fatal("expected TokString covering indented body")
	}
	if !bytesContain(src[body.Start:body.End()], "line1") || !bytesContain(src[body.Start:body.End()], "line2") {
		t.Errorf("body did not cover both lines: %q", src[body.Start:body.End()])
	}
}

func TestTokenizeYAML_BlockScalarFolded(t *testing.T) {
	src := []byte("key: >\n  text\n  more\n")
	toks := TokenizeYAML(src)
	validateTokens(t, src, toks)
	var indicator *Token
	for i := range toks {
		tk := &toks[i]
		if tk.Kind == TokKeyword && tk.End()-tk.Start >= 1 && src[tk.Start] == '>' {
			indicator = tk
		}
	}
	if indicator == nil {
		t.Fatal("expected TokKeyword for '>' indicator")
	}
	if string(src[indicator.Start:indicator.End()]) != ">" {
		t.Errorf("indicator = %q, want %q", src[indicator.Start:indicator.End()], ">")
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
			if tk.Kind == TokKeyword && string(src[tk.Start:tk.End()]) == c.want {
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
		if tk.Kind == TokKeyword && string(src[tk.Start:tk.End()]) == "|2" {
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
	if key == nil || string(src[key.Start:key.End()]) != "key" {
		t.Errorf("want TokKey 'key', got %+v", key)
	}
	if punct == nil || string(src[punct.Start:punct.End()]) != ":" {
		t.Errorf("want TokPunctuation ':', got %+v", punct)
	}
	if val == nil || string(src[val.Start:val.End()]) != "value" {
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
		if tk.Kind == TokKey && string(src[tk.Start:tk.End()]) == "b" {
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

func kindOf(t *testing.T, src, sub string) TokenKind {
	t.Helper()
	idx := indexOf(src, sub)
	if idx < 0 {
		t.Fatalf("substring %q not in %q", sub, src)
	}
	toks := TokenizeJS([]byte(src))
	for _, tk := range toks {
		if int(tk.Start) <= idx && idx < int(tk.End()) {
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
			s := src[tk.Start:tk.End()]
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
		if int(tk.Start) < prev {
			t.Fatalf("tokens overlap/out of order at %d (prev end %d): %+v", tk.Start, prev, tk)
		}
		if tk.End() < tk.Start || int(tk.End()) > len(src) {
			t.Fatalf("token out of bounds: %+v (len %d)", tk, len(src))
		}
		prev = int(tk.End())
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
		if tk.End() <= tk.Start {
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

func TestTokenizeJSON_Simple(t *testing.T) {
	src := []byte(`{"name": "Alice", "age": 30}`)
	tokens := TokenizeJSON(src)

	want := []Token{
		{Start: 0, Len: 1, Kind: TokBracket, Depth: 0},
		{Start: 1, Len: 6, Kind: TokKey},
		{Start: 7, Len: 1, Kind: TokPunctuation},
		{Start: 9, Len: 7, Kind: TokString},
		{Start: 16, Len: 1, Kind: TokPunctuation},
		{Start: 18, Len: 5, Kind: TokKey},
		{Start: 23, Len: 1, Kind: TokPunctuation},
		{Start: 25, Len: 2, Kind: TokNumber},
		{Start: 27, Len: 1, Kind: TokBracket, Depth: 0},
	}

	if len(tokens) != len(want) {
		t.Fatalf("len(tokens) = %d, want %d; got %+v", len(tokens), len(want), tokens)
	}
	for i, w := range want {
		g := tokens[i]
		if g.Start != w.Start || g.End() != w.End() || g.Kind != w.Kind || g.Depth != w.Depth {
			t.Errorf("tokens[%d] = %+v, want %+v", i, g, w)
		}
	}
}

func TestTokenizeJSON_BracketDepth(t *testing.T) {
	src := []byte(`{"a":[1,{"b":2}]}`)
	tokens := TokenizeJSON(src)

	wantBrackets := []struct {
		offset int
		depth  uint8
	}{
		{offset: 0, depth: 0},
		{offset: 5, depth: 1},
		{offset: 8, depth: 2},
		{offset: 14, depth: 2},
		{offset: 15, depth: 1},
		{offset: 16, depth: 0},
	}

	var got []struct {
		offset int
		depth  uint8
	}
	for _, tok := range tokens {
		if tok.Kind == TokBracket {
			got = append(got, struct {
				offset int
				depth  uint8
			}{offset: int(tok.Start), depth: tok.Depth})
		}
	}
	if len(got) != len(wantBrackets) {
		t.Fatalf("brackets: got %d, want %d (%+v)", len(got), len(wantBrackets), got)
	}
	for i, w := range wantBrackets {
		if got[i] != w {
			t.Errorf("bracket[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestTokenizeJSON_LiteralsAndNumbers(t *testing.T) {
	src := []byte(`[true, false, null, -3.14e+10]`)
	tokens := TokenizeJSON(src)

	kinds := []TokenKind{}
	for _, tok := range tokens {
		kinds = append(kinds, tok.Kind)
	}
	want := []TokenKind{
		TokBracket,
		TokBool,
		TokPunctuation,
		TokBool,
		TokPunctuation,
		TokNull,
		TokPunctuation,
		TokNumber,
		TokBracket,
	}
	if len(kinds) != len(want) {
		t.Fatalf("len(kinds) = %d, want %d (%v)", len(kinds), len(want), kinds)
	}
	for i, w := range want {
		if kinds[i] != w {
			t.Errorf("kinds[%d] = %v, want %v", i, kinds[i], w)
		}
	}
}

func TestTokenizeJSON_KeyVsString(t *testing.T) {
	src := []byte(`{"key":"val"}`)
	tokens := TokenizeJSON(src)
	var keyKind, valKind TokenKind
	for _, tok := range tokens {
		if tok.Start == 1 {
			keyKind = tok.Kind
		}
		if tok.Start == 7 {
			valKind = tok.Kind
		}
	}
	if keyKind != TokKey {
		t.Errorf("first string should be Key, got %v", keyKind)
	}
	if valKind != TokString {
		t.Errorf("second string should be String, got %v", valKind)
	}
}

func TestTokenizeJSON_UnicodeStrings(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"key": "Привет"}`),
		[]byte(`{"emoji": "🚀"}`),
		[]byte(`{"family": "👨‍👩‍👧‍👦"}`),
		[]byte(`{"cjk": "你好"}`),
		[]byte(`{"mixed": "a 🔥 б 漢"}`),
		[]byte(`{"escape": "line1\nline2"}`),
		[]byte(`{"escape": "tab\there"}`),
		[]byte(`{"escape": "quote\"end"}`),
		[]byte(`{"escape": "backslash\\end"}`),
		[]byte(`{"unicode": "é"}`),
		[]byte(`{"rtl": "مرحبا"}`),
	}
	for _, src := range cases {
		t.Run(string(src), func(t *testing.T) {
			done := make(chan struct{})
			var tokens []Token
			go func() {
				defer close(done)
				tokens = TokenizeJSON(src)
			}()
			select {
			case <-done:
			case <-timeout():
				t.Fatalf("TokenizeJSON hung on %q", src)
			}
			if len(tokens) == 0 {
				t.Errorf("no tokens for %q", src)
			}
			for _, tok := range tokens {
				if tok.Start < 0 || int(tok.End()) > len(src) || tok.Start > tok.End() {
					t.Errorf("invalid token range [%d,%d) for input %q", tok.Start, tok.End(), src)
				}
			}
		})
	}
}

func TestTokenizeJSON_MalformedNoHang(t *testing.T) {
	cases := [][]byte{
		[]byte(``),
		[]byte(`{`),
		[]byte(`}`),
		[]byte(`{"`),
		[]byte(`{"key`),
		[]byte(`{"key":`),
		[]byte(`{"key":}`),
		[]byte(`[`),
		[]byte(`[1,`),
		[]byte(`"\`),
		[]byte(`"\u`),
		[]byte(`"\u00`),
		[]byte(`{"a":1,}`),
		[]byte(`null,null`),
	}
	for _, src := range cases {
		t.Run(string(src), func(t *testing.T) {
			done := make(chan struct{})
			go func() {
				defer close(done)
				_ = TokenizeJSON(src)
			}()
			select {
			case <-done:
			case <-timeout():
				t.Fatalf("TokenizeJSON hung on %q", src)
			}
		})
	}
}

func TestDetect(t *testing.T) {
	cases := []struct {
		name     string
		ct       string
		body     []byte
		wantLang Lang
	}{
		{"json header", "application/json", nil, LangJSON},
		{"json with charset", "application/json; charset=utf-8", nil, LangJSON},
		{"vendor json suffix", "application/vnd.api+json", nil, LangJSON},
		{"xml header", "application/xml", nil, LangXML},
		{"sniff json object", "", []byte("  \n{\"x\":1}"), LangJSON},
		{"sniff json array", "", []byte("[1,2,3]"), LangJSON},
		{"sniff html", "", []byte("<!DOCTYPE html><html>"), LangHTML},
		{"sniff xml", "", []byte("<?xml version='1.0'?><root/>"), LangXML},
		{"plain text", "text/plain", []byte("hello world"), LangPlain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Detect(c.ct, c.body)
			if got != c.wantLang {
				t.Errorf("Detect(%q, %q) = %v, want %v", c.ct, c.body, got, c.wantLang)
			}
		})
	}
}

func TestTokenStaysEightBytes(t *testing.T) {
	if got := unsafe.Sizeof(Token{}); got != 8 {
		t.Errorf("Token is %d bytes; a large document holds one per few bytes of text", got)
	}
}

func TestLongRunSplitsIntoWholeTokens(t *testing.T) {
	src := []byte(`{"k":"` + strings.Repeat("x", 200000) + `"}`)
	toks := TokenizeJSON(src)
	var covered int
	for i, tk := range toks {
		if int(tk.Start) < covered {
			t.Fatalf("token %d overlaps the previous one", i)
		}
		if int(tk.End()) > len(src) {
			t.Fatalf("token %d ends past the source", i)
		}
		covered = int(tk.End())
	}
	var stringBytes int
	for _, tk := range toks {
		if tk.Kind == TokString {
			stringBytes += int(tk.Len)
		}
	}
	if stringBytes < 200000 {
		t.Errorf("long string covered by %d bytes of tokens, want the whole 200002", stringBytes)
	}
}

func timeout() <-chan time.Time {
	return time.After(2 * time.Second)
}

func TestTokenizeXML_Simple(t *testing.T) {
	src := []byte(`<root><item id="1">hello</item></root>`)
	tokens := TokenizeXML(src)

	var (
		gotKeyword []string
		gotKey     []string
		gotString  []string
		brackets   int
	)
	for _, tok := range tokens {
		switch tok.Kind {
		case TokKeyword:
			gotKeyword = append(gotKeyword, string(src[tok.Start:tok.End()]))
		case TokKey:
			gotKey = append(gotKey, string(src[tok.Start:tok.End()]))
		case TokString:
			gotString = append(gotString, string(src[tok.Start:tok.End()]))
		case TokBracket:
			brackets++
		}
	}
	wantKeywords := []string{"root", "item", "item", "root"}
	if len(gotKeyword) != len(wantKeywords) {
		t.Fatalf("keywords: got %v, want %v", gotKeyword, wantKeywords)
	}
	for i, w := range wantKeywords {
		if gotKeyword[i] != w {
			t.Errorf("keyword[%d] = %q, want %q", i, gotKeyword[i], w)
		}
	}
	if len(gotKey) != 1 || gotKey[0] != "id" {
		t.Errorf("keys: got %v, want [id]", gotKey)
	}
	if len(gotString) != 1 || gotString[0] != `"1"` {
		t.Errorf("strings: got %v, want [\"1\"]", gotString)
	}
	if brackets < 4 {
		t.Errorf("expected at least 4 bracket tokens, got %d", brackets)
	}
}

func TestTokenizeXML_Comment(t *testing.T) {
	src := []byte(`<a><!-- hello world --></a>`)
	tokens := TokenizeXML(src)
	var hasComment bool
	for _, tok := range tokens {
		if tok.Kind == TokComment {
			if string(src[tok.Start:tok.End()]) != `<!-- hello world -->` {
				t.Errorf("comment text mismatch: %q", src[tok.Start:tok.End()])
			}
			hasComment = true
		}
	}
	if !hasComment {
		t.Error("expected a comment token")
	}
}

func TestTokenizeYAML_CRLF_List(t *testing.T) {
	src := []byte("items:\r\n- apple\r\n- banana\r\n")
	tokens := TokenizeYAML(src)
	var dashes int
	for _, tok := range tokens {
		if tok.Kind == TokPunctuation && tok.End()-tok.Start == 1 && src[tok.Start] == '-' {
			dashes++
		}
	}
	if dashes != 2 {
		t.Errorf("expected 2 dashes (list items), got %d", dashes)
	}
}

func TestTokenizeYAML_Unicode(t *testing.T) {
	src := []byte("имя: Алиса\nemoji: 🚀\nkey: \"привет\"\n")
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = TokenizeYAML(src)
	}()
	select {
	case <-done:
	case <-timeout():
		t.Fatalf("TokenizeYAML hung on UTF-8")
	}
}

func TestTokenizeYAML_Basic(t *testing.T) {
	src := []byte("name: Alice\nage: 30\nactive: true\nitems:\n  - apple\n  - banana\n# comment\n")
	tokens := TokenizeYAML(src)

	kinds := map[TokenKind]int{}
	for _, tok := range tokens {
		kinds[tok.Kind]++
	}
	if kinds[TokKey] < 4 {
		t.Errorf("expected >=4 keys, got %d", kinds[TokKey])
	}
	if kinds[TokNumber] < 1 {
		t.Errorf("expected >=1 number, got %d", kinds[TokNumber])
	}
	if kinds[TokBool] < 1 {
		t.Errorf("expected >=1 bool, got %d", kinds[TokBool])
	}
	if kinds[TokComment] < 1 {
		t.Errorf("expected >=1 comment, got %d", kinds[TokComment])
	}
}

func TestTokenizeXML_TruncatedTagDoesNotHang(t *testing.T) {
	cases := [][]byte{
		[]byte(`<a`),
		[]byte(`<a x`),
		[]byte(`<a x=`),
		[]byte(`<a x=`),
		[]byte(`<a /`),
		[]byte(`<!--`),
		[]byte(`<!--unclosed`),
		[]byte(`<![CDATA[`),
		[]byte(`<a>bcd`),
		[]byte(`<a x=y`),
		[]byte(``),
		[]byte(`<`),
	}
	for _, src := range cases {
		done := make(chan struct{})
		go func(s []byte) {
			defer close(done)
			_ = TokenizeXML(s)
		}(src)
		select {
		case <-done:
		case <-timeout():
			t.Fatalf("TokenizeXML hung on %q", src)
		}
	}
}

func TestTokenizeXML_UTF8Tags(t *testing.T) {
	src := []byte(`<товар id="🚀">тест</товар>`)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = TokenizeXML(src)
	}()
	select {
	case <-done:
	case <-timeout():
		t.Fatalf("TokenizeXML hung on UTF-8 input")
	}
}

func TestTokenizeForm_Basic(t *testing.T) {
	src := []byte(`name=Alice&age=30&active=true`)
	tokens := TokenizeForm(src)

	wantSeq := []struct {
		kind TokenKind
		text string
	}{
		{TokKey, "name"},
		{TokOperator, "="},
		{TokString, "Alice"},
		{TokPunctuation, "&"},
		{TokKey, "age"},
		{TokOperator, "="},
		{TokString, "30"},
		{TokPunctuation, "&"},
		{TokKey, "active"},
		{TokOperator, "="},
		{TokString, "true"},
	}
	if len(tokens) != len(wantSeq) {
		t.Fatalf("len(tokens) = %d, want %d", len(tokens), len(wantSeq))
	}
	for i, w := range wantSeq {
		got := tokens[i]
		if got.Kind != w.kind || string(src[got.Start:got.End()]) != w.text {
			t.Errorf("[%d] = %+v %q, want %+v %q", i, got.Kind, src[got.Start:got.End()], w.kind, w.text)
		}
	}
}
