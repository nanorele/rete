package fontsubset

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func fontWithCmap(cmap []byte, extra ...rawTable) []byte {
	tables := append([]rawTable{{"cmap", cmap}, {"head", newHead()}}, extra...)
	return buildSFNT(0x00010000, tables)
}

func subsetPairs(t *testing.T, out []byte) map[rune]uint32 {
	t.Helper()
	f, err := parseSFNT(out)
	if err != nil {
		t.Fatalf("parseSFNT(Subset(...)) error = %v", err)
	}
	cm, ok := f.tables[tagCmap]
	if !ok {
		t.Fatal("Subset() output has no cmap table")
	}
	pairs, err := parseUnicodeCmap(cm.data)
	if err != nil {
		t.Fatalf("parseUnicodeCmap(Subset(...)) error = %v", err)
	}
	return pairsToMap(pairs)
}

func TestSubsetErrors(t *testing.T) {
	sub := buildFormat12([]fmt12Group{{start: 'A', end: 'A', startGlyph: 1}})
	badCmap := buildFormat12([]fmt12Group{{start: 'A', end: 'A', startGlyph: 1}})
	binary.BigEndian.PutUint32(badCmap[4:8], uint32(len(badCmap)+4))

	cases := []struct {
		name    string
		ttf     []byte
		fn      func(rune) bool
		wantSub string
	}{
		{
			name:    "nil predicate",
			ttf:     fontWithCmap(buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: sub}})),
			fn:      nil,
			wantSub: "shouldRemove is nil",
		},
		{
			name:    "not a font",
			ttf:     []byte("not a font at all"),
			fn:      func(rune) bool { return false },
			wantSub: "unsupported version",
		},
		{
			name:    "truncated header",
			ttf:     []byte{0, 1},
			fn:      func(rune) bool { return false },
			wantSub: "header truncated",
		},
		{
			name:    "cmap missing",
			ttf:     buildSFNT(0x00010000, []rawTable{{"head", newHead()}}),
			fn:      func(rune) bool { return false },
			wantSub: "cmap table missing",
		},
		{
			name:    "cmap unparsable",
			ttf:     fontWithCmap(buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: badCmap}})),
			fn:      func(rune) bool { return false },
			wantSub: "past EOF",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := Subset(c.ttf, c.fn)
			if err == nil {
				t.Fatalf("Subset() error = nil, want error containing %q", c.wantSub)
			}
			if out != nil {
				t.Errorf("Subset() = %v, want nil on error", out)
			}
			if !strings.Contains(err.Error(), c.wantSub) {
				t.Errorf("Subset() error = %q, want it to contain %q", err, c.wantSub)
			}
			if !strings.HasPrefix(err.Error(), "fontsubset: ") {
				t.Errorf("Subset() error = %q, want a %q prefix", err, "fontsubset: ")
			}
		})
	}
}

func TestSubsetFiltering(t *testing.T) {
	cmap := buildCmap([]encRec{
		{platformID: 3, encodingID: 1, sub: buildFormat4([]fmt4Seg{
			{start: 'A', end: 'C', delta: 0},
			{start: 0xFFFF, end: 0xFFFF},
		}, nil)},
		{platformID: 3, encodingID: 10, sub: buildFormat12([]fmt12Group{
			{start: 0x1F600, end: 0x1F601, startGlyph: 900},
			{start: 0x4E00, end: 0x4E00, startGlyph: 500},
		})},
	})

	cases := []struct {
		name string
		fn   func(rune) bool
		want map[rune]uint32
	}{
		{
			name: "keep everything",
			fn:   func(rune) bool { return false },
			want: map[rune]uint32{'A': 'A', 'B': 'B', 'C': 'C', 0x1F600: 900, 0x1F601: 901, 0x4E00: 500},
		},
		{
			name: "drop everything",
			fn:   func(rune) bool { return true },
			want: map[rune]uint32{},
		},
		{
			name: "drop emoji",
			fn:   IsEmojiCodepoint,
			want: map[rune]uint32{'A': 'A', 'B': 'B', 'C': 'C', 0x4E00: 500},
		},
		{
			name: "drop ascii",
			fn:   func(r rune) bool { return r < 0x80 },
			want: map[rune]uint32{0x1F600: 900, 0x1F601: 901, 0x4E00: 500},
		},
		{
			name: "drop a single codepoint",
			fn:   func(r rune) bool { return r == 'B' },
			want: map[rune]uint32{'A': 'A', 'C': 'C', 0x1F600: 900, 0x1F601: 901, 0x4E00: 500},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ttf := fontWithCmap(cmap, rawTable{"glyf", []byte{1, 2, 3, 4, 5}})
			out, err := Subset(ttf, c.fn)
			if err != nil {
				t.Fatalf("Subset() error = %v", err)
			}
			got := subsetPairs(t, out)
			if len(got) != len(c.want) {
				t.Fatalf("Subset() cmap = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("glyph for U+%04X = %d, want %d", k, got[k], v)
				}
			}

			f, err := parseSFNT(out)
			if err != nil {
				t.Fatalf("parseSFNT() error = %v", err)
			}
			if g := f.tables[tag("glyf")]; g == nil || !bytes.Equal(g.data, []byte{1, 2, 3, 4, 5}) {
				t.Errorf("glyf table was not preserved: %v", g)
			}
			if _, ok := f.tables[tagHead]; !ok {
				t.Error("head table was not preserved")
			}
			if got := tableChecksum(out); got != 0xB1B0AFBA {
				t.Errorf("whole-font checksum = %#x, want 0xB1B0AFBA", got)
			}
		})
	}
}

func TestSubsetDoesNotMutateInput(t *testing.T) {
	ttf := fontWithCmap(buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: buildFormat12([]fmt12Group{
		{start: 0x1F600, end: 0x1F605, startGlyph: 10},
	})}}))
	orig := append([]byte(nil), ttf...)
	if _, err := Subset(ttf, func(rune) bool { return true }); err != nil {
		t.Fatalf("Subset() error = %v", err)
	}
	if !bytes.Equal(ttf, orig) {
		t.Error("Subset() mutated its input buffer")
	}
}

func TestSubsetEmoji(t *testing.T) {
	cmap := buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: buildFormat12([]fmt12Group{
		{start: 'A', end: 'A', startGlyph: 1},
		{start: 0x2764, end: 0x2764, startGlyph: 2},
		{start: 0x4E2D, end: 0x4E2D, startGlyph: 3},
		{start: 0x1F600, end: 0x1F600, startGlyph: 4},
	})}})
	out, err := SubsetEmoji(fontWithCmap(cmap))
	if err != nil {
		t.Fatalf("SubsetEmoji() error = %v", err)
	}
	got := subsetPairs(t, out)
	want := map[rune]uint32{'A': 1, 0x4E2D: 3}
	if len(got) != len(want) {
		t.Fatalf("SubsetEmoji() cmap = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("glyph for U+%04X = %d, want %d", k, got[k], v)
		}
	}
}

func TestSubsetEmojiPropagatesError(t *testing.T) {
	if _, err := SubsetEmoji(nil); err == nil {
		t.Fatal("SubsetEmoji(nil) error = nil, want error")
	}
}

func TestEmojiRangesInvariants(t *testing.T) {
	for i, r := range emojiRanges {
		if r[0] > r[1] {
			t.Errorf("emojiRanges[%d] = %#x..%#x is inverted", i, r[0], r[1])
		}
		if r[1] > 0x10FFFF {
			t.Errorf("emojiRanges[%d] end %#x is not a valid rune", i, r[1])
		}
		if i > 0 && r[0] <= emojiRanges[i-1][1] {
			t.Errorf("emojiRanges[%d] start %#x overlaps or is unsorted vs previous end %#x",
				i, r[0], emojiRanges[i-1][1])
		}
	}
}

func TestIsEmojiCodepointMatchesLinearScan(t *testing.T) {
	inRange := func(r rune) bool {
		for _, rg := range emojiRanges {
			if r >= rg[0] && r <= rg[1] {
				return true
			}
		}
		return false
	}
	for r := rune(0); r <= 0x1FBFF; r++ {
		want := inRange(r)
		if r == '#' || r == '*' || (r >= '0' && r <= '9') {
			want = false
		}
		if got := IsEmojiCodepoint(r); got != want {
			t.Fatalf("IsEmojiCodepoint(U+%04X) = %v, want %v", r, got, want)
		}
	}
}

func TestIsEmojiCodepointBoundaries(t *testing.T) {
	cases := []struct {
		name string
		r    rune
		want bool
	}{
		{"below first range", 0x00A8, false},
		{"first range start", 0x00A9, true},
		{"gap after first range", 0x00AA, false},
		{"last range end", 0x1FAF8, true},
		{"above last range", 0x1FAF9, false},
		{"far above", 0x10FFFF, false},
		{"negative", -1, false},
		{"zero", 0, false},
		{"range start", 0x2194, true},
		{"range end", 0x2199, true},
		{"inside range", 0x2196, true},
		{"just before range", 0x2193, false},
		{"just after range", 0x219A, false},
		{"keycap hash excluded", '#', false},
		{"keycap star excluded", '*', false},
		{"keycap zero excluded", '0', false},
		{"keycap nine excluded", '9', false},
		{"cjk not emoji", 0x4E2D, false},
		{"cyrillic not emoji", 0x0416, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IsEmojiCodepoint(c.r); got != c.want {
				t.Errorf("IsEmojiCodepoint(U+%04X) = %v, want %v", c.r, got, c.want)
			}
		})
	}
}
