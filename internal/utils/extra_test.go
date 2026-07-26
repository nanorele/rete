package utils

import (
	"bytes"
	"strings"
	"testing"

	"golang.org/x/text/transform"
)

func TestCharsetDecoder(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantNil     bool
	}{
		{"empty", "", true},
		{"utf-8 needs no decoder", "text/html; charset=utf-8", true},
		{"utf8 alias", "text/plain; charset=utf8", true},
		{"us-ascii", "text/plain; charset=us-ascii", true},
		{"ascii alias", "text/plain; charset=ascii", true},
		{"no charset param", "text/html", true},
		{"unknown charset", "text/html; charset=definitely-not-a-charset", true},
		{"utf-8 alias via html index", "text/plain; charset=unicode-1-1-utf-8", true},
		{"utf-8 alias csutf8", "text/plain; charset=csutf8", true},
		{"malformed content type", "text/html;;;charset=", true},
		{"latin1", "text/html; charset=iso-8859-1", false},
		{"windows-1251", "text/html; charset=windows-1251", false},
		{"shift_jis", "text/html; charset=shift_jis", false},
		{"koi8-r", "text/plain; charset=koi8-r", false},
		{"utf-16", "text/plain; charset=utf-16", false},
		{"utf-16le", "text/plain; charset=utf-16le", false},
		{"utf-16be", "text/plain; charset=utf-16be", false},
		{"uppercase charset", "text/html; charset=WINDOWS-1252", false},
		{"quoted charset", `text/html; charset="iso-8859-2"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := CharsetDecoder(tt.contentType)
			if (dec == nil) != tt.wantNil {
				t.Fatalf("CharsetDecoder(%q) nil = %v, want nil = %v", tt.contentType, dec == nil, tt.wantNil)
			}
		})
	}
}

func TestCharsetDecoderDecodesCorrectly(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		in          []byte
		want        string
	}{
		{"latin1", "text/plain; charset=iso-8859-1", []byte{0xE9, 0xE8}, "éè"},
		{"windows-1251", "text/plain; charset=windows-1251", []byte{0xCF, 0xF0}, "Пр"},
		{"koi8-r", "text/plain; charset=koi8-r", []byte{0xF0, 0xD2}, "Пр"},
		{"utf-16le", "text/plain; charset=utf-16le", []byte{0x41, 0x00, 0x42, 0x00}, "AB"},
		{"utf-16be", "text/plain; charset=utf-16be", []byte{0x00, 0x41, 0x00, 0x42}, "AB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := CharsetDecoder(tt.contentType)
			if dec == nil {
				t.Fatal("expected a decoder")
			}
			got, _, err := transform.Bytes(dec, tt.in)
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("decoded %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCharsetDecoderForBody(t *testing.T) {
	tests := []struct {
		name        string
		probe       []byte
		contentType string
		wantNil     bool
	}{
		{"content-type wins", []byte("<?xml version=\"1.0\" encoding=\"utf-8\"?>"),
			"text/xml; charset=windows-1251", false},
		{"utf-8 content type", nil, "text/plain; charset=utf-8", true},
		{"bom sniffed utf-16le", []byte{0xFF, 0xFE, 0x41, 0x00}, "", false},
		{"bom sniffed utf-16be", []byte{0xFE, 0xFF, 0x00, 0x41}, "", false},
		{"bom sniffed utf-8 needs no decoder", []byte{0xEF, 0xBB, 0xBF, 'h'}, "", true},
		{"xml decl sniffed", []byte(`<?xml version="1.0" encoding="windows-1251"?><a/>`), "text/xml", false},
		{"xml decl utf-8 needs no decoder", []byte(`<?xml version="1.0" encoding="utf-8"?><a/>`), "text/xml", true},
		{"html meta sniffed", []byte(`<html><meta charset="iso-8859-2"></html>`), "text/html", false},
		{"html meta ignored for non-html type", []byte(`<html><meta charset="iso-8859-2"></html>`), "text/plain", true},
		{"nothing to sniff", []byte("plain ascii body"), "text/plain", true},
		{"empty probe", nil, "", true},
		{"unknown sniffed charset", []byte(`<?xml version="1.0" encoding="bogus-999"?>`), "text/xml", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := CharsetDecoderForBody(tt.probe, tt.contentType)
			if (dec == nil) != tt.wantNil {
				t.Fatalf("nil = %v, want nil = %v", dec == nil, tt.wantNil)
			}
		})
	}
}

func TestCharsetDecoderForBodyMatchesDecodeBody(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		contentType string
	}{
		{"latin1 declared", []byte{0xE9, 0xE8, 'a'}, "text/plain; charset=iso-8859-1"},
		{"windows-1251 declared", []byte{0xCF, 0xF0}, "text/html; charset=windows-1251"},
		{"xml sniffed", []byte(`<?xml version="1.0" encoding="windows-1251"?>` + "\xCF\xF0"), "text/xml"},
		{"html sniffed", []byte(`<meta charset="iso-8859-1">` + "\xE9"), "text/html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dec := CharsetDecoderForBody(tt.body, tt.contentType)
			if dec == nil {
				t.Fatal("expected a decoder")
			}
			streamed, _, err := transform.Bytes(dec, tt.body)
			if err != nil {
				t.Fatalf("transform: %v", err)
			}
			whole := DecodeBody(tt.body, tt.contentType)
			if !bytes.Equal(bytes.TrimPrefix(streamed, []byte{0xEF, 0xBB, 0xBF}), whole) {
				t.Errorf("streamed decode %q != DecodeBody %q", streamed, whole)
			}
		})
	}
}

func TestIsHTMLContentType(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"", false},
		{"text/html", true},
		{"TEXT/HTML", true},
		{"text/html; charset=utf-8", true},
		{"application/xhtml+xml", true},
		{"application/xhtml+xml; charset=utf-8", true},
		{"text/plain", false},
		{"text/xml", false},
		{"application/json", false},
		{"application/xml", false},
		{"not a media type at all ///", false},
		{"text/html; charset", false},
		{"  text/html  ", true},
	}
	for _, tt := range tests {
		if got := isHTMLContentType(tt.ct); got != tt.want {
			t.Errorf("isHTMLContentType(%q) = %v, want %v", tt.ct, got, tt.want)
		}
	}
}

func TestSniffCharsetXMLEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"too short", "<?xml", ""},
		{"not a declaration", "<html>", ""},
		{"missing question mark", "<xml version='1.0'?>", ""},
		{"no closing", `<?xml version="1.0" encoding="utf-8"`, ""},
		{"no encoding attr", `<?xml version="1.0"?>`, ""},
		{"encoding without equals", `<?xml version="1.0" encoding "utf-8"?>`, ""},
		{"encoding without quotes", `<?xml version="1.0" encoding=utf-8?>`, ""},
		{"encoding value missing", `<?xml v="1" encoding=  ?>`, ""},
		{"encoding is last token", `<?xml v="1" encoding?>`, ""},
		{"unterminated quote", `<?xml version="1.0" encoding="utf-8?>`, ""},
		{"double quotes", `<?xml version="1.0" encoding="windows-1251"?>`, "windows-1251"},
		{"single quotes", `<?xml version='1.0' encoding='koi8-r'?>`, "koi8-r"},
		{"spaces around equals", `<?xml version="1.0" encoding = "utf-8"?>`, "utf-8"},
		{"tabs around equals", "<?xml version=\"1.0\" encoding\t=\t\"utf-8\"?>", "utf-8"},
		{"uppercase lowered", `<?XML VERSION="1.0" ENCODING="UTF-8"?>`, "utf-8"},
		{"encoding after 256 byte window", `<?xml ` + strings.Repeat(" ", 300) + `encoding="utf-8"?>`, ""},
		{"declaration then content", `<?xml version="1.0" encoding="euc-jp"?><doc>hello</doc>`, "euc-jp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffCharsetXML([]byte(tt.in)); got != tt.want {
				t.Errorf("SniffCharsetXML(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSniffCharsetHTMLEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no meta", "<html><body>hi</body></html>", ""},
		{"meta charset double quoted", `<meta charset="iso-8859-1">`, "iso-8859-1"},
		{"meta charset single quoted", `<meta charset='iso-8859-1'>`, "iso-8859-1"},
		{"meta charset unquoted", `<meta charset=iso-8859-1>`, "iso-8859-1"},
		{"meta charset with spaces", `<meta charset = "iso-8859-1">`, "iso-8859-1"},
		{"http-equiv form", `<meta http-equiv="Content-Type" content="text/html; charset=shift_jis">`, "shift_jis"},
		{"self closing", `<meta charset="utf-8"/>`, "utf-8"},
		{"uppercase", `<META CHARSET="UTF-8">`, "utf-8"},
		{"first meta has no charset", `<meta name="viewport" content="x"><meta charset="koi8-r">`, "koi8-r"},
		{"two metas neither has charset", `<meta name="a"><meta name="b">`, ""},
		{"meta without closing bracket", `<meta charset="utf-8"`, ""},
		{"charset at very end of tag", `<meta content="text/html;charset=big5">`, "big5"},
		{"beyond 4096 window", strings.Repeat(" ", 4100) + `<meta charset="utf-8">`, ""},
		{"just inside 4096 window", strings.Repeat(" ", 4000) + `<meta charset="utf-8">`, "utf-8"},
		{"meta charset empty value", `<meta charset="">`, ""},
		{"falls through malformed charset attr to next meta", `<meta charsetx><meta charset="utf-8">`, "utf-8"},
		{"malformed charset attr alone", `<meta charsetx>`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffCharsetHTML([]byte(tt.in)); got != tt.want {
				t.Errorf("SniffCharsetHTML(%q) = %q, want %q", truncate(tt.in), got, tt.want)
			}
		})
	}
}

func truncate(s string) string {
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

func TestSniffCharsetPrecedence(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType string
		want        string
	}{
		{
			name:        "bom beats xml declaration",
			data:        append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<?xml version="1.0" encoding="windows-1251"?>`)...),
			contentType: "text/xml",
			want:        "utf-8",
		},
		{
			name:        "xml declaration beats html meta",
			data:        []byte(`<?xml version="1.0" encoding="koi8-r"?><meta charset="utf-8">`),
			contentType: "text/html",
			want:        "koi8-r",
		},
		{
			name:        "html meta only for html content types",
			data:        []byte(`<meta charset="koi8-r">`),
			contentType: "text/plain",
			want:        "",
		},
		{
			name:        "html meta used for xhtml",
			data:        []byte(`<meta charset="koi8-r">`),
			contentType: "application/xhtml+xml",
			want:        "koi8-r",
		},
		{
			name:        "nothing found",
			data:        []byte("just some text"),
			contentType: "text/html",
			want:        "",
		},
		{
			name:        "utf-32be bom",
			data:        []byte{0x00, 0x00, 0xFE, 0xFF, 'a'},
			contentType: "",
			want:        "utf-32be",
		},
		{
			name:        "utf-32le bom takes priority over utf-16le prefix",
			data:        []byte{0xFF, 0xFE, 0x00, 0x00, 'a'},
			contentType: "",
			want:        "utf-32le",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffCharset(tt.data, tt.contentType); got != tt.want {
				t.Errorf("SniffCharset = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDecodeBodyIdempotentForASCII(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
	}{
		{"no content type", ""},
		{"plain", "text/plain"},
		{"utf-8", "text/plain; charset=utf-8"},
		{"unknown charset", "text/plain; charset=nonsense-42"},
		{"malformed", "!!!"},
	}
	body := []byte("plain ascii stays untouched")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecodeBody(body, tt.contentType)
			if !bytes.Equal(got, body) {
				t.Errorf("DecodeBody = %q, want %q", got, body)
			}
		})
	}
}

func TestDecodeBodyEmptyInput(t *testing.T) {
	for _, ct := range []string{"", "text/html", "text/html; charset=windows-1251", "text/plain; charset=utf-16le"} {
		if got := DecodeBody(nil, ct); len(got) != 0 {
			t.Errorf("DecodeBody(nil, %q) = %q, want empty", ct, got)
		}
	}
}

func TestDecodeBodyStripsUTF8BOMOnce(t *testing.T) {
	in := append([]byte{0xEF, 0xBB, 0xBF, 0xEF, 0xBB, 0xBF}, []byte("hi")...)
	got := DecodeBody(in, "")
	want := append([]byte{0xEF, 0xBB, 0xBF}, []byte("hi")...)
	if !bytes.Equal(got, want) {
		t.Errorf("DecodeBody = %q, want exactly one BOM stripped (%q)", got, want)
	}
}

func TestCharsetFromContentTypeEdgeCases(t *testing.T) {
	tests := []struct {
		ct   string
		want string
	}{
		{"", ""},
		{"text/html", ""},
		{"text/html; charset=UTF-8", "utf-8"},
		{`text/html; charset="UTF-8"`, "utf-8"},
		{"text/html; charset=  utf-8  ", "utf-8"},
		{"text/html; CHARSET=utf-8", "utf-8"},
		{"garbage///", ""},
		{"text/html; charset=", ""},
		{"text/html; boundary=x; charset=latin1", "latin1"},
	}
	for _, tt := range tests {
		if got := CharsetFromContentType(tt.ct); got != tt.want {
			t.Errorf("CharsetFromContentType(%q) = %q, want %q", tt.ct, got, tt.want)
		}
	}
}

func TestParseContentDispositionFilenameEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"empty", "", ""},
		{"no filename", "attachment", ""},
		{"simple", `attachment; filename="report.pdf"`, "report.pdf"},
		{"unquoted", "attachment; filename=report.pdf", "report.pdf"},
		{"inline", `inline; filename="a.txt"`, "a.txt"},
		{"path stripped forward slash", `attachment; filename="/etc/passwd"`, "passwd"},
		{"path stripped backslash", `attachment; filename="C:\\temp\\evil.exe"`, "evil.exe"},
		{"traversal stripped", `attachment; filename="../../secret.txt"`, "secret.txt"},
		{"bare dotdot rejected", `attachment; filename=".."`, ""},
		{"bare dot rejected", `attachment; filename="."`, ""},
		{"whitespace only rejected", `attachment; filename="   "`, ""},
		{"trailing slash rejected", `attachment; filename="dir/"`, ""},
		{"rfc5987 utf-8", `attachment; filename*=UTF-8''%D1%84%D0%B0%D0%B9%D0%BB.txt`, "файл.txt"},
		{"malformed header", "attachment; filename", ""},
		{"empty filename value", `attachment; filename=""`, ""},
		{"spaces trimmed", `attachment; filename="  spaced.txt  "`, "spaced.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseContentDispositionFilename(tt.header); got != tt.want {
				t.Errorf("ParseContentDispositionFilename(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestFilenameFromURLEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"empty", "", ""},
		{"simple", "https://example.com/file.zip", "file.zip"},
		{"with query", "https://example.com/file.zip?token=abc", "file.zip"},
		{"with fragment", "https://example.com/file.zip#top", "file.zip"},
		{"nested path", "https://example.com/a/b/c/doc.pdf", "doc.pdf"},
		{"trailing slash", "https://example.com/a/b/", "b"},
		{"root only", "https://example.com/", ""},
		{"no path", "https://example.com", ""},
		{"percent encoded", "https://example.com/my%20file.txt", "my file.txt"},
		{"leading and trailing spaces", "  https://example.com/x.txt  ", "x.txt"},
		{"dot path", "https://example.com/.", ""},
		{"dotdot path", "https://example.com/..", ""},
		{"encoded slash", "https://example.com/a%2Fb", "b"},
		{"unparsable", "://bad", ""},
		{"relative path", "dir/file.txt", "file.txt"},
		{"bare filename", "file.txt", "file.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FilenameFromURL(tt.url); got != tt.want {
				t.Errorf("FilenameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestStripJSONCommentsEdgeCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no comments", `{"a":1}`, `{"a":1}`},
		{"empty", "", ""},
		{"line comment", "{\n// note\n\"a\":1}", "{\n\n\"a\":1}"},
		{"line comment at eof", `{"a":1} // trailing`, `{"a":1} `},
		{"block comment", `{/* note */"a":1}`, `{"a":1}`},
		{"multiline block", "{\n/* a\nb */\n\"a\":1}", "{\n\n\"a\":1}"},
		{"unterminated block", `{"a":1 /* forever`, `{"a":1 `},
		{"unterminated line", `{"a":1 //`, `{"a":1 `},
		{"slashes inside string kept", `{"url":"http://x.com/a"}`, `{"url":"http://x.com/a"}`},
		{"block marker inside string kept", `{"s":"/* not a comment */"}`, `{"s":"/* not a comment */"}`},
		{"escaped quote then comment", `{"s":"a\"b"} // c`, `{"s":"a\"b"} `},
		{"escaped backslash before quote", `{"s":"a\\"} // c`, `{"s":"a\\"} `},
		{"double escaped backslash", `{"s":"a\\\\"} // c`, `{"s":"a\\\\"} `},
		{"lone slash kept", `{"a":1/2}`, `{"a":1/2}`},
		{"slash at end of input", `{"a":1}/`, `{"a":1}/`},
		{"comment between keys", `{"a":1,/*x*/"b":2}`, `{"a":1,"b":2}`},
		{"consecutive comments", `{}//a
//b`, `{}
`},
		{"block then line", `{/*a*/}//b`, `{}`},
		{"star not ending block", `{/* a * b */}`, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripJSONComments(tt.in); got != tt.want {
				t.Errorf("StripJSONComments(%q) =\n  %q\nwant\n  %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeBytesFastPath(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"pure ascii", []byte("hello world"), "hello world"},
		{"ascii with newlines", []byte("a\nb\nc"), "a\nb\nc"},
		{"empty", nil, ""},
		{"tab triggers expansion", []byte("a\tb"), "a    b"},
		{"cr alone becomes newline", []byte("a\rb"), "a\nb"},
		{"crlf collapses to lf", []byte("a\r\nb"), "a\nb"},
		{"del stripped", []byte("a\x7Fb"), "ab"},
		{"nul stripped", []byte("a\x00b"), "ab"},
		{"valid utf-8 kept", []byte("héllo"), "héllo"},
		{"invalid utf-8 replaced", []byte{'a', 0xFF, 'b'}, "a�b"},
		{"zero width space stripped", []byte("a\u200Bb"), "ab"},
		{"zero width non-joiner stripped", []byte("a\u200Cb"), "ab"},
		{"left-to-right mark stripped", []byte("a\u200Eb"), "ab"},
		{"right-to-left mark stripped", []byte("a\u200Fb"), "ab"},
		{"word joiner stripped", []byte("a\u2060b"), "ab"},
		{"bidi isolates stripped", []byte("a\u2066b\u2067c\u2068d\u2069e"), "abcde"},
		{"bom stripped", []byte("a\uFEFFb"), "ab"},
		{"soft hyphen stripped", []byte("a\u00ADb"), "ab"},
		{"line separator becomes newline", []byte("a\u2028b"), "a\nb"},
		{"paragraph separator becomes newline", []byte("a\u2029b"), "a\nb"},
		{"c1 control stripped", []byte("a\u0085b"), "ab"},
		{"c1 high stripped", []byte("a\u009Fb"), "ab"},
		{"emoji kept", []byte("a\U0001F389b"), "a\U0001F389b"},
		{"cjk kept", []byte("日本語"), "日本語"},
		{"trailing cr", []byte("abc\r"), "abc\n"},
		{"lone continuation byte", []byte{0x80}, "�"},
		{"truncated multibyte", []byte{'a', 0xE2, 0x82}, "a��"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeBytes(tt.in); got != tt.want {
				t.Errorf("SanitizeBytes(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if got := SanitizeText(string(tt.in)); got != tt.want {
				t.Errorf("SanitizeText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeBytesTextParityOnBinaryCorpus(t *testing.T) {
	corpus := [][]byte{
		{},
		{0x00},
		{0xFF, 0xFE, 0xFD},
		{0xC2},
		{0xC2, 0xA0},
		{0xE2, 0x80},
		{0xE2, 0x80, 0xA8},
		{0xEF, 0xBB, 0xBF},
		{0xEF},
		[]byte("\r\n\r\n"),
		[]byte("\t\t\t"),
		bytes.Repeat([]byte{0x1B, '['}, 20),
	}
	x := uint32(0x5EED)
	for range 200 {
		n := int(x%64) + 1
		b := make([]byte, n)
		for i := range b {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
			b[i] = byte(x)
		}
		corpus = append(corpus, b)
	}
	for i, in := range corpus {
		fromBytes := SanitizeBytes(in)
		fromText := SanitizeText(string(in))
		if fromBytes != fromText {
			t.Errorf("case %d (%x): SanitizeBytes = %q, SanitizeText = %q", i, in, fromBytes, fromText)
		}
		for _, r := range fromBytes {
			if r < 0x20 && r != '\n' {
				t.Errorf("case %d (%x): output retained control rune %#x", i, in, r)
			}
			if r >= 0x7F && r <= 0x9F {
				t.Errorf("case %d (%x): output retained C1 control %#x", i, in, r)
			}
		}
	}
}
