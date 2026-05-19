package utils

import (
	"bytes"
	"testing"
)

func TestCharsetFromContentType(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"text/html", ""},
		{"text/html; charset=UTF-8", "utf-8"},
		{"text/html; charset=Windows-1251", "windows-1251"},
		{"application/json; charset=us-ascii", "us-ascii"},
		{"text/plain;charset=\"ISO-8859-1\"", "iso-8859-1"},
		{"not a media type", ""},
	}
	for _, c := range cases {
		got := CharsetFromContentType(c.in)
		if got != c.want {
			t.Errorf("CharsetFromContentType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDecodeBody_UTF8Passthrough(t *testing.T) {
	in := []byte("Привет, мир — hello 🌍")
	out := DecodeBody(in, "text/plain; charset=utf-8")
	if !bytes.Equal(out, in) {
		t.Errorf("utf-8 should pass through unchanged")
	}
	out = DecodeBody(in, "text/plain")
	if !bytes.Equal(out, in) {
		t.Errorf("no charset should pass through unchanged")
	}
}

func TestDecodeBody_Windows1251Cyrillic(t *testing.T) {
	// "Привет" in CP1251: П=0xCF, р=0xF0, и=0xE8, в=0xE2, е=0xE5, т=0xF2
	in := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	out := DecodeBody(in, "text/plain; charset=windows-1251")
	if string(out) != "Привет" {
		t.Errorf("windows-1251 decode: got %q want %q", out, "Привет")
	}
}

func TestDecodeBody_KOI8R(t *testing.T) {
	// "тест" in KOI8-R: т=0xD4, е=0xC5, с=0xD3, т=0xD4
	in := []byte{0xD4, 0xC5, 0xD3, 0xD4}
	out := DecodeBody(in, "text/plain; charset=koi8-r")
	if string(out) != "тест" {
		t.Errorf("koi8-r decode: got %q want %q", out, "тест")
	}
}

func TestDecodeBody_Latin1(t *testing.T) {
	// "café" in ISO-8859-1: c=0x63 a=0x61 f=0x66 é=0xE9
	in := []byte{0x63, 0x61, 0x66, 0xE9}
	out := DecodeBody(in, "text/plain; charset=iso-8859-1")
	if string(out) != "café" {
		t.Errorf("iso-8859-1 decode: got %q want %q", out, "café")
	}
}

func TestDecodeBody_ShiftJIS(t *testing.T) {
	// "あ" in Shift_JIS: 0x82, 0xA0
	in := []byte{0x82, 0xA0}
	out := DecodeBody(in, "text/html; charset=Shift_JIS")
	if string(out) != "あ" {
		t.Errorf("shift_jis decode: got %q want %q", out, "あ")
	}
}

func TestDecodeBody_UnknownCharset(t *testing.T) {
	in := []byte("plain bytes")
	out := DecodeBody(in, "text/plain; charset=made-up-encoding-xyz")
	if !bytes.Equal(out, in) {
		t.Errorf("unknown charset should leave bytes untouched")
	}
}

func TestSniffCharsetHTML_MetaCharset(t *testing.T) {
	html := []byte(`<!DOCTYPE html><html><head><meta charset="windows-1251"><title>x</title></head>`)
	got := SniffCharsetHTML(html)
	if got != "windows-1251" {
		t.Errorf("sniff <meta charset>: got %q", got)
	}
}

func TestSniffCharsetHTML_MetaHttpEquiv(t *testing.T) {
	html := []byte(`<html><head><meta http-equiv="Content-Type" content="text/html; charset=koi8-r"></head>`)
	got := SniffCharsetHTML(html)
	if got != "koi8-r" {
		t.Errorf("sniff <meta http-equiv>: got %q", got)
	}
}

func TestSniffCharsetHTML_None(t *testing.T) {
	html := []byte(`<html><head><title>nope</title></head><body>x</body></html>`)
	got := SniffCharsetHTML(html)
	if got != "" {
		t.Errorf("expected empty sniff, got %q", got)
	}
}

func TestDecodeBody_HTMLMetaFallback(t *testing.T) {
	// CP1251 "Привет" inside an HTML document with <meta charset> but no
	// charset on Content-Type. Browsers fall back to <meta charset>; we
	// should too.
	cp1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	html := append([]byte(`<html><head><meta charset="windows-1251"></head><body>`), cp1251...)
	html = append(html, []byte(`</body></html>`)...)
	out := DecodeBody(html, "text/html")
	if !bytes.Contains(out, []byte("Привет")) {
		t.Errorf("html meta fallback didn't decode cyrillic; out=%q", out)
	}
}

func TestDecodeBody_UTF16LE_BOM(t *testing.T) {
	// UTF-16LE BOM (FF FE) + "Hi"
	in := []byte{0xFF, 0xFE, 'H', 0x00, 'i', 0x00}
	out := DecodeBody(in, "text/plain; charset=utf-16")
	if string(out) != "Hi" {
		t.Errorf("utf-16 BOM decode: got %q want %q", out, "Hi")
	}
}

func TestSniffCharsetBOM(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"utf-8 BOM", []byte{0xEF, 0xBB, 0xBF, 'x'}, "utf-8"},
		{"utf-16le BOM", []byte{0xFF, 0xFE, 'x', 0x00}, "utf-16le"},
		{"utf-16be BOM", []byte{0xFE, 0xFF, 0x00, 'x'}, "utf-16be"},
		{"utf-32le BOM", []byte{0xFF, 0xFE, 0x00, 0x00}, "utf-32le"},
		{"utf-32be BOM", []byte{0x00, 0x00, 0xFE, 0xFF}, "utf-32be"},
		{"no BOM", []byte("hello"), ""},
		{"too short", []byte{0xFF}, ""},
	}
	for _, c := range cases {
		got := SniffCharsetBOM(c.in)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSniffCharsetXML(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"double-quoted", `<?xml version="1.0" encoding="windows-1251"?><root/>`, "windows-1251"},
		{"single-quoted", `<?xml version='1.0' encoding='ISO-8859-1'?>`, "iso-8859-1"},
		{"no encoding attr", `<?xml version="1.0"?>`, ""},
		{"not xml", `<html><head>`, ""},
		{"empty", ``, ""},
		{"no closing", `<?xml version="1.0" encoding="utf-8"`, ""},
	}
	for _, c := range cases {
		got := SniffCharsetXML([]byte(c.in))
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDecodeBody_BOMSniff_UTF16LE(t *testing.T) {
	// UTF-16LE BOM + "ok", Content-Type has no charset
	in := []byte{0xFF, 0xFE, 'o', 0x00, 'k', 0x00}
	out := DecodeBody(in, "text/plain")
	if string(out) != "ok" {
		t.Errorf("got %q want %q", out, "ok")
	}
}

func TestDecodeBody_BOMSniff_UTF8(t *testing.T) {
	// UTF-8 BOM should be stripped even when Content-Type is bare.
	in := []byte("\xEF\xBB\xBFhello")
	out := DecodeBody(in, "text/plain")
	if string(out) != "hello" {
		t.Errorf("utf-8 BOM not stripped: got %q", out)
	}
}

func TestDecodeBody_XMLDeclSniff(t *testing.T) {
	// CP1251 "Привет" inside an XML document, no charset on Content-Type.
	cp1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	doc := append([]byte(`<?xml version="1.0" encoding="windows-1251"?><root>`), cp1251...)
	doc = append(doc, []byte(`</root>`)...)
	out := DecodeBody(doc, "application/xml")
	if !bytes.Contains(out, []byte("Привет")) {
		t.Errorf("xml decl sniff didn't decode cyrillic; out=%q", out)
	}
}
