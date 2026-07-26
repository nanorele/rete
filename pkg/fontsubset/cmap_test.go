package fontsubset

import (
	"encoding/binary"
	"testing"
)

type fmt4Seg struct {
	end, start    uint16
	delta         int16
	idRangeOffset uint16
}

func buildFormat4(segs []fmt4Seg, glyphIDArray []uint16) []byte {
	segCount := len(segs)
	total := 14 + segCount*2 + 2 + segCount*6 + len(glyphIDArray)*2
	b := make([]byte, total)
	binary.BigEndian.PutUint16(b[0:2], 4)
	binary.BigEndian.PutUint16(b[2:4], uint16(total))
	binary.BigEndian.PutUint16(b[4:6], 0)
	binary.BigEndian.PutUint16(b[6:8], uint16(segCount*2))

	endOff := 14
	startOff := endOff + segCount*2 + 2
	deltaOff := startOff + segCount*2
	rangeOff := deltaOff + segCount*2
	glyphOff := rangeOff + segCount*2

	for i, s := range segs {
		binary.BigEndian.PutUint16(b[endOff+i*2:], s.end)
		binary.BigEndian.PutUint16(b[startOff+i*2:], s.start)
		binary.BigEndian.PutUint16(b[deltaOff+i*2:], uint16(s.delta))
		binary.BigEndian.PutUint16(b[rangeOff+i*2:], s.idRangeOffset)
	}
	for i, g := range glyphIDArray {
		binary.BigEndian.PutUint16(b[glyphOff+i*2:], g)
	}
	return b
}

type fmt12Group struct {
	start, end, startGlyph uint32
}

func buildFormat12(groups []fmt12Group) []byte {
	total := 16 + len(groups)*12
	b := make([]byte, total)
	binary.BigEndian.PutUint16(b[0:2], 12)
	binary.BigEndian.PutUint32(b[4:8], uint32(total))
	binary.BigEndian.PutUint32(b[12:16], uint32(len(groups)))
	for i, g := range groups {
		off := 16 + i*12
		binary.BigEndian.PutUint32(b[off:], g.start)
		binary.BigEndian.PutUint32(b[off+4:], g.end)
		binary.BigEndian.PutUint32(b[off+8:], g.startGlyph)
	}
	return b
}

type encRec struct {
	platformID, encodingID uint16
	sub                    []byte
	rawOffset              uint32
	useRawOffset           bool
}

func buildCmap(recs []encRec) []byte {
	head := 4 + len(recs)*8
	body := head
	offsets := make([]uint32, len(recs))
	for i, r := range recs {
		offsets[i] = uint32(body)
		body += len(r.sub)
	}
	b := make([]byte, body)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(recs)))
	for i, r := range recs {
		off := 4 + i*8
		binary.BigEndian.PutUint16(b[off:], r.platformID)
		binary.BigEndian.PutUint16(b[off+2:], r.encodingID)
		o := offsets[i]
		if r.useRawOffset {
			o = r.rawOffset
		}
		binary.BigEndian.PutUint32(b[off+4:], o)
		copy(b[offsets[i]:], r.sub)
	}
	return b
}

func pairsToMap(pairs []cmapPair) map[rune]uint32 {
	m := make(map[rune]uint32, len(pairs))
	for _, p := range pairs {
		m[p.codepoint] = p.glyphID
	}
	return m
}

func TestIsUnicodeEncoding(t *testing.T) {
	cases := []struct {
		name          string
		platform, enc uint16
		want          bool
	}{
		{"unicode any enc 0", 0, 0, true},
		{"unicode any enc 3", 0, 3, true},
		{"unicode any enc 6", 0, 6, true},
		{"macintosh", 1, 0, false},
		{"iso deprecated", 2, 1, false},
		{"windows symbol", 3, 0, false},
		{"windows bmp", 3, 1, true},
		{"windows shiftjis", 3, 2, false},
		{"windows full", 3, 10, true},
		{"custom", 4, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isUnicodeEncoding(c.platform, c.enc); got != c.want {
				t.Errorf("isUnicodeEncoding(%d, %d) = %v, want %v", c.platform, c.enc, got, c.want)
			}
		})
	}
}

func TestParseFormat4(t *testing.T) {
	truncatedLength := buildFormat4([]fmt4Seg{{end: 0x41, start: 0x41}}, nil)
	binary.BigEndian.PutUint16(truncatedLength[2:4], uint16(len(truncatedLength)+8))

	shortArrays := buildFormat4([]fmt4Seg{{end: 0x41, start: 0x41}}, nil)
	binary.BigEndian.PutUint16(shortArrays[2:4], 16)

	cases := []struct {
		name    string
		sub     []byte
		wantErr bool
		want    map[rune]uint32
	}{
		{
			name:    "header truncated",
			sub:     make([]byte, 13),
			wantErr: true,
		},
		{
			name:    "declared length past EOF",
			sub:     truncatedLength,
			wantErr: true,
		},
		{
			name:    "arrays past declared length",
			sub:     shortArrays,
			wantErr: true,
		},
		{
			name: "zero segments",
			sub:  buildFormat4(nil, nil),
			want: map[rune]uint32{},
		},
		{
			name: "single range with delta",
			sub: buildFormat4([]fmt4Seg{
				{start: 'A', end: 'C', delta: 10},
				{start: 0xFFFF, end: 0xFFFF, delta: 1},
			}, nil),
			want: map[rune]uint32{'A': 'A' + 10, 'B': 'B' + 10, 'C': 'C' + 10},
		},
		{
			name: "delta wraps modulo 65536",
			sub: buildFormat4([]fmt4Seg{
				{start: 0xFFF0, end: 0xFFF1, delta: 0x20},
			}, nil),
			want: map[rune]uint32{0xFFF0: 0x10, 0xFFF1: 0x11},
		},
		{
			name: "glyph zero is dropped",
			sub: buildFormat4([]fmt4Seg{
				{start: 0x20, end: 0x21, delta: -0x20},
			}, nil),
			want: map[rune]uint32{0x21: 1},
		},
		{
			name: "sentinel segment skipped",
			sub: buildFormat4([]fmt4Seg{
				{start: 0xFFFF, end: 0xFFFF, delta: 5},
			}, nil),
			want: map[rune]uint32{},
		},
		{
			name: "start greater than end yields nothing",
			sub: buildFormat4([]fmt4Seg{
				{start: 0x50, end: 0x40, delta: 1},
			}, nil),
			want: map[rune]uint32{},
		},
		{
			name: "id range offset lookup",
			sub: buildFormat4([]fmt4Seg{
				{start: 'a', end: 'c', idRangeOffset: 4},
				{start: 0xFFFF, end: 0xFFFF, idRangeOffset: 0},
			}, []uint16{77, 0, 78}),
			want: map[rune]uint32{'a': 77, 'c': 78},
		},
		{
			name: "id range offset plus delta",
			sub: buildFormat4([]fmt4Seg{
				{start: 'a', end: 'a', delta: 3, idRangeOffset: 4},
				{start: 0xFFFF, end: 0xFFFF},
			}, []uint16{100}),
			want: map[rune]uint32{'a': 103},
		},
		{
			name: "id range offset past declared length",
			sub: buildFormat4([]fmt4Seg{
				{start: 'a', end: 'a', idRangeOffset: 0x400},
			}, []uint16{100}),
			want: map[rune]uint32{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := make(map[rune]uint32)
			err := parseFormat4(c.sub, got)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseFormat4() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFormat4() error = %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("parseFormat4() = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("glyph for U+%04X = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestParseFormat12(t *testing.T) {
	pastEOF := buildFormat12([]fmt12Group{{start: 1, end: 1, startGlyph: 1}})
	binary.BigEndian.PutUint32(pastEOF[4:8], uint32(len(pastEOF)+4))

	groupsPastLength := buildFormat12([]fmt12Group{{start: 1, end: 1, startGlyph: 1}})
	binary.BigEndian.PutUint32(groupsPastLength[12:16], 5)

	cases := []struct {
		name    string
		sub     []byte
		wantErr bool
		want    map[rune]uint32
	}{
		{"header truncated", make([]byte, 15), true, nil},
		{"declared length past EOF", pastEOF, true, nil},
		{"group array past declared length", groupsPastLength, true, nil},
		{"no groups", buildFormat12(nil), false, map[rune]uint32{}},
		{
			name: "single codepoint group",
			sub:  buildFormat12([]fmt12Group{{start: 0x1F600, end: 0x1F600, startGlyph: 42}}),
			want: map[rune]uint32{0x1F600: 42},
		},
		{
			name: "contiguous group",
			sub:  buildFormat12([]fmt12Group{{start: 'A', end: 'D', startGlyph: 7}}),
			want: map[rune]uint32{'A': 7, 'B': 8, 'C': 9, 'D': 10},
		},
		{
			name: "multiple groups",
			sub: buildFormat12([]fmt12Group{
				{start: 'A', end: 'B', startGlyph: 1},
				{start: 0x4E00, end: 0x4E01, startGlyph: 100},
			}),
			want: map[rune]uint32{'A': 1, 'B': 2, 0x4E00: 100, 0x4E01: 101},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := make(map[rune]uint32)
			err := parseFormat12(c.sub, got)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseFormat12() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFormat12() error = %v", err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("parseFormat12() = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("glyph for U+%04X = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestParseFormat12OverwritesExisting(t *testing.T) {
	m := map[rune]uint32{'A': 1}
	if err := parseFormat12(buildFormat12([]fmt12Group{{start: 'A', end: 'A', startGlyph: 9}}), m); err != nil {
		t.Fatalf("parseFormat12() error = %v", err)
	}
	if m['A'] != 9 {
		t.Errorf("m['A'] = %d, want 9 (format 12 must win over format 4)", m['A'])
	}
}

func TestParseUnicodeCmap(t *testing.T) {
	fmt4Sub := buildFormat4([]fmt4Seg{
		{start: 'A', end: 'B', delta: 0},
		{start: 0xFFFF, end: 0xFFFF},
	}, nil)
	fmt12Sub := buildFormat12([]fmt12Group{{start: 0x1F600, end: 0x1F601, startGlyph: 500}})

	badFmt4 := buildFormat4([]fmt4Seg{{start: 'A', end: 'A'}}, nil)
	binary.BigEndian.PutUint16(badFmt4[2:4], uint16(len(badFmt4)+8))

	badFmt12 := buildFormat12([]fmt12Group{{start: 1, end: 1, startGlyph: 1}})
	binary.BigEndian.PutUint32(badFmt12[4:8], uint32(len(badFmt12)+4))

	cases := []struct {
		name    string
		data    []byte
		wantErr bool
		want    map[rune]uint32
	}{
		{"header truncated", []byte{0, 0, 0}, true, nil},
		{
			name:    "encoding records truncated",
			data:    []byte{0, 0, 0, 2, 0, 0},
			wantErr: true,
		},
		{"no encoding records", buildCmap(nil), false, map[rune]uint32{}},
		{
			name: "format 4 only",
			data: buildCmap([]encRec{{platformID: 3, encodingID: 1, sub: fmt4Sub}}),
			want: map[rune]uint32{'A': 'A', 'B': 'B'},
		},
		{
			name: "format 12 only",
			data: buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: fmt12Sub}}),
			want: map[rune]uint32{0x1F600: 500, 0x1F601: 501},
		},
		{
			name: "format 4 and 12 merged",
			data: buildCmap([]encRec{
				{platformID: 3, encodingID: 1, sub: fmt4Sub},
				{platformID: 3, encodingID: 10, sub: fmt12Sub},
			}),
			want: map[rune]uint32{'A': 'A', 'B': 'B', 0x1F600: 500, 0x1F601: 501},
		},
		{
			name: "non unicode encoding ignored",
			data: buildCmap([]encRec{{platformID: 1, encodingID: 0, sub: fmt4Sub}}),
			want: map[rune]uint32{},
		},
		{
			name: "unsupported subtable format ignored",
			data: buildCmap([]encRec{{platformID: 0, encodingID: 3, sub: []byte{0, 6, 0, 10, 0, 0, 0, 0, 0, 0}}}),
			want: map[rune]uint32{},
		},
		{
			name: "offset past EOF skipped",
			data: buildCmap([]encRec{{platformID: 3, encodingID: 1, sub: fmt4Sub, useRawOffset: true, rawOffset: 0xFFFF}}),
			want: map[rune]uint32{},
		},
		{
			name: "glyph id zero is not emitted",
			data: buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: buildFormat12([]fmt12Group{
				{start: 'A', end: 'B', startGlyph: 0},
				{start: 'Z', end: 'Z', startGlyph: 4},
			})}}),
			want: map[rune]uint32{'B': 1, 'Z': 4},
		},
		{
			name:    "format 4 parse error propagates",
			data:    buildCmap([]encRec{{platformID: 3, encodingID: 1, sub: badFmt4}}),
			wantErr: true,
		},
		{
			name:    "format 12 parse error propagates",
			data:    buildCmap([]encRec{{platformID: 3, encodingID: 10, sub: badFmt12}}),
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pairs, err := parseUnicodeCmap(c.data)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseUnicodeCmap() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseUnicodeCmap() error = %v", err)
			}
			got := pairsToMap(pairs)
			if len(got) != len(c.want) {
				t.Fatalf("parseUnicodeCmap() = %v, want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("glyph for U+%04X = %d, want %d", k, got[k], v)
				}
			}
			for i := 1; i < len(pairs); i++ {
				if pairs[i-1].codepoint >= pairs[i].codepoint {
					t.Fatalf("pairs not sorted ascending at %d: %v", i, pairs)
				}
			}
		})
	}
}

func TestBuildFormat12Cmap(t *testing.T) {
	cases := []struct {
		name       string
		pairs      []cmapPair
		wantGroups int
	}{
		{"empty", nil, 0},
		{"single", []cmapPair{{'A', 1}}, 1},
		{"contiguous merges", []cmapPair{{'A', 1}, {'B', 2}, {'C', 3}}, 1},
		{"codepoint gap splits", []cmapPair{{'A', 1}, {'C', 2}}, 2},
		{"glyph gap splits", []cmapPair{{'A', 1}, {'B', 5}}, 2},
		{"unsorted input is sorted", []cmapPair{{'C', 3}, {'A', 1}, {'B', 2}}, 1},
		{"mixed", []cmapPair{{'A', 1}, {'B', 2}, {0x1F600, 40}, {0x1F601, 41}, {0x4E00, 9}}, 3},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := append([]cmapPair(nil), c.pairs...)
			out := buildFormat12Cmap(in)

			if got := binary.BigEndian.Uint16(out[0:2]); got != 0 {
				t.Errorf("cmap version = %d, want 0", got)
			}
			if got := binary.BigEndian.Uint16(out[2:4]); got != 2 {
				t.Errorf("numTables = %d, want 2", got)
			}
			subOffset := binary.BigEndian.Uint32(out[8:12])
			if other := binary.BigEndian.Uint32(out[16:20]); other != subOffset {
				t.Errorf("second encoding record offset = %d, want %d", other, subOffset)
			}
			if p, e := binary.BigEndian.Uint16(out[4:6]), binary.BigEndian.Uint16(out[6:8]); p != 0 || e != 4 {
				t.Errorf("first encoding = (%d,%d), want (0,4)", p, e)
			}
			if p, e := binary.BigEndian.Uint16(out[12:14]), binary.BigEndian.Uint16(out[14:16]); p != 3 || e != 10 {
				t.Errorf("second encoding = (%d,%d), want (3,10)", p, e)
			}

			sub := out[subOffset:]
			if got := binary.BigEndian.Uint16(sub[0:2]); got != 12 {
				t.Errorf("subtable format = %d, want 12", got)
			}
			if got := binary.BigEndian.Uint32(sub[4:8]); int(got) != len(sub) {
				t.Errorf("subtable length = %d, want %d", got, len(sub))
			}
			if got := binary.BigEndian.Uint32(sub[12:16]); int(got) != c.wantGroups {
				t.Errorf("numGroups = %d, want %d", got, c.wantGroups)
			}

			round, err := parseUnicodeCmap(out)
			if err != nil {
				t.Fatalf("parseUnicodeCmap(buildFormat12Cmap(...)) error = %v", err)
			}
			got := pairsToMap(round)
			want := pairsToMap(c.pairs)
			if len(got) != len(want) {
				t.Fatalf("round trip = %v, want %v", got, want)
			}
			for k, v := range want {
				if got[k] != v {
					t.Errorf("round trip glyph for U+%04X = %d, want %d", k, got[k], v)
				}
			}
		})
	}
}

func TestBuildFormat12CmapGroupBounds(t *testing.T) {
	out := buildFormat12Cmap([]cmapPair{{'A', 1}, {'B', 2}, {'C', 3}})
	subOffset := binary.BigEndian.Uint32(out[8:12])
	sub := out[subOffset:]
	start := binary.BigEndian.Uint32(sub[16:20])
	end := binary.BigEndian.Uint32(sub[20:24])
	glyph := binary.BigEndian.Uint32(sub[24:28])
	if start != 'A' || end != 'C' || glyph != 1 {
		t.Errorf("group = (%#x, %#x, %d), want (0x41, 0x43, 1)", start, end, glyph)
	}
}
