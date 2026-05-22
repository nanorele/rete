package utils

import "testing"

func TestParseContentDispositionFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", `attachment; filename="report.pdf"`, "report.pdf"},
		{"unquoted", `attachment; filename=report.pdf`, "report.pdf"},
		{"inline", `inline; filename="page.html"`, "page.html"},
		{"rfc5987 utf-8", `attachment; filename*=UTF-8''%D0%B4%D0%BE%D0%BA.txt`, "док.txt"},
		{"both, ext wins", `attachment; filename="fallback.txt"; filename*=UTF-8''real.txt`, "real.txt"},
		{"empty header", ``, ""},
		{"no filename", `attachment`, ""},
		{"path traversal stripped", `attachment; filename="../../etc/passwd"`, "passwd"},
		{"windows path stripped", `attachment; filename="C:\windows\foo.txt"`, "foo.txt"},
		{"dot only", `attachment; filename="."`, ""},
	}
	for _, c := range cases {
		got := ParseContentDispositionFilename(c.in)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestFilenameFromURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://example.com/files/report.pdf", "report.pdf"},
		{"https://example.com/files/report.pdf?x=1", "report.pdf"},
		{"https://example.com/", ""},
		{"https://example.com", ""},
		{"https://example.com/a/b/c", "c"},
		{"", ""},
		{"::not-a-url", ""},
		{"https://example.com/foo%20bar.txt", "foo bar.txt"},
	}
	for _, c := range cases {
		got := FilenameFromURL(c.in)
		if got != c.want {
			t.Errorf("FilenameFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
