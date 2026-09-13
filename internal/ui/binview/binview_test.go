package binview

import (
	"bytes"
	"strings"
	"testing"
)

func TestIsBinary(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		mime string
		want bool
	}{
		{"empty", nil, "", false},
		{"plain", []byte("hello world\r\n\tplain"), "", false},
		{"json", []byte(`{"a":1}`), "application/json", false},
		{"nul", []byte("abc\x00def"), "", true},
		{"nul in declared text", []byte("abc\x00def"), "text/plain", true},
		{"png by mime", []byte("\x89PNG\r\n\x1a\n"), "image/png", true},
		{"octet-stream by mime", []byte("plain looking"), "application/octet-stream", true},
		{"mostly control", bytes.Repeat([]byte{0x01}, 100), "", true},
		{"few control", append(bytes.Repeat([]byte("a"), 100), 0x01), "", false},
		{"utf16 bom", []byte{0xFF, 0xFE, 'h', 0, 'i', 0}, "", false},
		{"utf8 bom", []byte{0xEF, 0xBB, 0xBF, 'h', 'i'}, "", false},
		{"cp1251 text", []byte("\xcf\xf0\xe8\xe2\xe5\xf2, \xec\xe8\xf0"), "", false},
		{"svg is text", []byte("<svg/>"), "image/svg+xml", false},
		{"charset param", []byte("<html>"), "text/html; charset=utf-8", false},
		{"large binary", append(bytes.Repeat([]byte{0x02}, 9000), 'a'), "", true},
	}
	for _, c := range cases {
		if got := IsBinary(c.in, c.mime); got != c.want {
			t.Errorf("IsBinary(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestFormatModes(t *testing.T) {
	in := []byte("Hello\x00\xff")
	want := map[Mode]string{
		ModeHexDump: "00000000  48 65 6c 6c 6f 00 ff                              |Hello..|\n",
		ModeHex:     "48656c6c6f00ff",
		ModeBase64:  "SGVsbG8A/w==",
		ModeBits:    "01001000 01100101 01101100 01101100 01101111 00000000 11111111",
		ModeText:    "Hello�",
	}
	for m, w := range want {
		if got := Format(in, m); got != w {
			t.Errorf("%s: got %q, want %q", m.Label(), got, w)
		}
	}
}

func TestHexDumpLines(t *testing.T) {
	in := make([]byte, 20)
	for i := range in {
		in[i] = byte(i)
	}
	got := HexDump(in)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("20 bytes must dump as 2 lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "00000000  00 01 02 03 04 05 06 07  08 09 0a 0b 0c 0d 0e 0f  |") {
		t.Errorf("first line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "00000010  10 11 12 13 ") || !strings.HasSuffix(lines[1], "|....|") {
		t.Errorf("second line = %q", lines[1])
	}
	if len(lines[0]) != len(lines[1])-4+16 {
		t.Errorf("hex columns must stay aligned on the short last line:\n%q\n%q", lines[0], lines[1])
	}
}

func TestFormatTruncates(t *testing.T) {
	in := bytes.Repeat([]byte{0xAB}, MaxRender+10)
	got := Format(in, ModeHex)
	if !strings.HasSuffix(got, "… showing first 262144 of 262154 bytes") {
		t.Errorf("truncation note missing: %q", got[len(got)-60:])
	}
	got = FormatTotal(in[:16], ModeHexDump, 1<<20)
	if !strings.Contains(got, "showing first 16 of 1048576 bytes") {
		t.Errorf("FormatTotal must report the source size: %q", got)
	}
	if got := Format([]byte("abc"), ModeHex); strings.Contains(got, "showing") {
		t.Errorf("no note when nothing was cut: %q", got)
	}
}

func TestHexAndBase64Wrap(t *testing.T) {
	in := bytes.Repeat([]byte{0x11}, 70)
	if lines := strings.Split(Format(in, ModeHex), "\n"); len(lines) != 3 || len(lines[0]) != 64 {
		t.Errorf("hex must wrap at 32 bytes per line, got %d lines, first %d chars", len(lines), len(lines[0]))
	}
	if lines := strings.Split(Format(in, ModeBase64), "\n"); len(lines) != 2 || len(lines[0]) != 76 {
		t.Errorf("base64 must wrap at 76 chars, got %d lines, first %d chars", len(lines), len(lines[0]))
	}
	if lines := strings.Split(Format(in[:9], ModeBits), "\n"); len(lines) != 2 || len(strings.Fields(lines[0])) != 8 {
		t.Errorf("bits must wrap at 8 bytes per line: %q", lines)
	}
}
