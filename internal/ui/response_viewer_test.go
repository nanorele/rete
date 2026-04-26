package ui

import "testing"

func TestByteToRuneIdx(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		byteIdx int
		want    int
	}{
		{"ascii start", "abcdef", 0, 0},
		{"ascii mid", "abcdef", 3, 3},
		{"ascii end", "abcdef", 6, 6},
		{"cyrillic start", "привет", 0, 0},
		{"cyrillic after first rune (2 bytes)", "привет", 2, 1},
		{"cyrillic after third rune (6 bytes)", "привет", 6, 3},
		{"cyrillic full (12 bytes)", "привет", 12, 6},
		{"mixed: ab + 'и' (2 bytes) at byte 4", "abиcd", 4, 3},
		{"emoji 4-byte", "a\xf0\x9f\x98\x80b", 5, 2},
		{"past end clamps", "ab", 10, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := byteToRuneIdx([]byte(tc.text), tc.byteIdx)
			if got != tc.want {
				t.Errorf("byteToRuneIdx(%q, %d) = %d, want %d", tc.text, tc.byteIdx, got, tc.want)
			}
		})
	}
}

func TestRuneIdxToByte(t *testing.T) {
	cases := []struct {
		name    string
		text    string
		runeIdx int
		want    int
	}{
		{"ascii zero", "abcdef", 0, 0},
		{"ascii mid", "abcdef", 3, 3},
		{"ascii past end clamps", "abcdef", 100, 6},
		{"cyrillic 1 rune = 2 bytes", "привет", 1, 2},
		{"cyrillic 3 runes = 6 bytes", "привет", 3, 6},
		{"cyrillic 6 runes = 12 bytes", "привет", 6, 12},
		{"emoji 1 rune = 4 bytes", "\xf0\x9f\x98\x80x", 1, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runeIdxToByte([]byte(tc.text), tc.runeIdx)
			if got != tc.want {
				t.Errorf("runeIdxToByte(%q, %d) = %d, want %d", tc.text, tc.runeIdx, got, tc.want)
			}
		})
	}
}

// TestWordBoundsAt covers the double-click word-selection helper. The
// range returned should bracket a contiguous run of same-kind runes
// (word vs separator) around the click point, regardless of whether
// the click landed on a word char or whitespace.
func TestWordBoundsAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world\nfoo.bar")
	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
	}{
		// "hello world\nfoo.bar"  (indices: h=0..o=4, ' '=5, w=6..d=10, \n=11, f=12..o=14, .=15, b=16..r=18)
		{"start of first word", 0, 0, 5},     // 'h' → "hello"
		{"middle of word", 2, 0, 5},          // 'l' → "hello"
		{"on space (separator run)", 5, 5, 6}, // ' ' → just the space
		{"on word 'w' (start of second word)", 6, 6, 11}, // 'w' → "world"
		{"on newline (separator run)", 11, 11, 12},       // '\n' → just the newline
		{"start of foo (after newline)", 12, 12, 15},     // 'f' → "foo"
		{"on dot separator", 15, 15, 16},                 // '.' → just the dot
		{"on bar word", 17, 16, 19},                      // 'a' → "bar"
		{"at EOF (walks back into trailing word)", 19, 16, 19}, // EOF → "bar"
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.wordBoundsAt(tc.byteOff)
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("wordBoundsAt(%d) = (%d,%d); want (%d,%d) — sel=%q",
					tc.byteOff, gotS, gotE, tc.wantStart, tc.wantEnd, string(v.text[gotS:gotE]))
			}
		})
	}
}

// TestSourceLineBoundsAt covers the triple-click line-selection
// helper. Should return [start, end) of the source line containing
// byteOff, with start AFTER the previous '\n' (or 0) and end BEFORE
// the next '\n' (or len(text)).
// TestWordBoundsAt_QuotesAndHyphens verifies the user-requested
// behavior: double-click on a JSON string value selects the inner
// text without the surrounding quotes; hyphens (e.g. Content-Type,
// my-key) are treated as word characters so the whole identifier is
// selected as one word.
func TestWordBoundsAt_QuotesAndHyphens(t *testing.T) {
	v := NewResponseViewer()
	// "my-key": "Content-Type"
	// indices: " =0, m=1..y=2, -=3, k=4..y=6, " =7, : =8, ' '=9, " =10,
	//          C=11..t=17, -=18, T=19..e=22, " =23
	v.SetText(`"my-key": "Content-Type"`)

	cases := []struct {
		name               string
		byteOff            int
		wantStart, wantEnd int
		wantSel            string
	}{
		{"on opening quote", 0, 0, 1, `"`},
		{"on m of my-key", 1, 1, 7, "my-key"},
		{"on hyphen of my-key", 3, 1, 7, "my-key"},
		{"on k of key", 4, 1, 7, "my-key"},
		{"on closing quote of my-key", 7, 7, 8, `"`},
		{"on C of Content-Type", 11, 11, 23, "Content-Type"},
		{"on hyphen of Content-Type", 18, 11, 23, "Content-Type"},
		{"on T of Type", 19, 11, 23, "Content-Type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.wordBoundsAt(tc.byteOff)
			gotSel := string(v.text[gotS:gotE])
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("wordBoundsAt(%d) = (%d,%d) %q; want (%d,%d) %q",
					tc.byteOff, gotS, gotE, gotSel, tc.wantStart, tc.wantEnd, tc.wantSel)
			}
		})
	}
}

func TestSourceLineBoundsAt(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("line one\nsecond\r\nthird")
	cases := []struct {
		name              string
		byteOff           int
		wantStart, wantEnd int
	}{
		{"first line start", 0, 0, 8},
		{"first line middle", 4, 0, 8},
		{"first line end", 8, 0, 8},
		{"after first newline (= second line start)", 9, 9, 15}, // "second" — \r stripped
		{"middle of second line", 12, 9, 15},
		{"after second newline", 17, 17, 22},
		{"in third line", 19, 17, 22},
		{"at EOF", 22, 17, 22},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotS, gotE := v.sourceLineBoundsAt(tc.byteOff)
			if gotS != tc.wantStart || gotE != tc.wantEnd {
				t.Errorf("sourceLineBoundsAt(%d) = (%d,%d); want (%d,%d) — sel=%q",
					tc.byteOff, gotS, gotE, tc.wantStart, tc.wantEnd, string(v.text[gotS:gotE]))
			}
		})
	}
}

func TestSelectAll(t *testing.T) {
	v := NewResponseViewer()
	v.SetText("hello world")
	v.SelectAll()
	if v.selStart != 0 || v.selEnd != 11 {
		t.Errorf("SelectAll: got selection [%d,%d), want [0,11)", v.selStart, v.selEnd)
	}
	if got := v.SelectedText(); got != "hello world" {
		t.Errorf("SelectAll: SelectedText = %q, want full text", got)
	}
}

// Round-trip property: byteToRuneIdx(t, runeIdxToByte(t, n)) == n for
// any n in [0, runeCount(t)]. Catches off-by-one in either helper.
func TestRuneByteRoundTrip(t *testing.T) {
	texts := []string{"abc", "привет мир", "a\xf0\x9f\x98\x80b\xf0\x9f\x98\x81c", "{\"имя\":\"значение\"}"}
	for _, txt := range texts {
		bs := []byte(txt)
		// Walk every rune index from 0 up to count.
		for r := 0; ; r++ {
			b := runeIdxToByte(bs, r)
			gotR := byteToRuneIdx(bs, b)
			if gotR != r && b < len(bs) {
				t.Errorf("round-trip mismatch on %q at rune %d: byte=%d back-rune=%d", txt, r, b, gotR)
			}
			if b >= len(bs) {
				break
			}
		}
	}
}
