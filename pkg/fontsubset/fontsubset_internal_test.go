package fontsubset

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode"
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

func TestSerializeShortHeadDoesNotPanic(t *testing.T) {
	cases := []struct {
		name string
		head []byte
	}{
		{"empty head", []byte{}},
		{"4-byte head", []byte{0, 0, 0, 0}},
		{"11-byte head", make([]byte, 11)},
		{"12-byte head", make([]byte, 12)},
		{"full head", make([]byte, 54)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &sfntFile{
				sfntVersion: 0x00010000,
				tables: map[uint32]*sfntTable{
					tagHead: {tag: tagHead, data: c.head},
				},
			}
			out, err := f.serialize()
			if err != nil {
				return
			}
			if len(out) == 0 {
				t.Error("serialize returned no data and no error")
			}
		})
	}
}

func format12Sub(groups [][3]uint32) []byte {
	sub := make([]byte, 16+len(groups)*12)
	binary.BigEndian.PutUint16(sub[0:2], 12)
	binary.BigEndian.PutUint32(sub[4:8], uint32(len(sub)))
	binary.BigEndian.PutUint32(sub[12:16], uint32(len(groups)))
	for i, g := range groups {
		off := 16 + i*12
		binary.BigEndian.PutUint32(sub[off:off+4], g[0])
		binary.BigEndian.PutUint32(sub[off+4:off+8], g[1])
		binary.BigEndian.PutUint32(sub[off+8:off+12], g[2])
	}
	return sub
}

func TestParseFormat12RejectsUnboundedGroups(t *testing.T) {
	cases := []struct {
		name   string
		groups [][3]uint32
	}{
		{"full uint32 range", [][3]uint32{{0, 0xFFFFFFFF, 1}}},
		{"end at max uint32", [][3]uint32{{0xFFFFFF00, 0xFFFFFFFF, 1}}},
		{"start above MaxRune", [][3]uint32{{0x110000, 0x110010, 1}}},
		{"end above MaxRune", [][3]uint32{{0x10FFF0, 0x200000, 1}}},
		{"end before start", [][3]uint32{{100, 50, 1}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := map[rune]uint32{}
			done := make(chan error, 1)
			go func() {
				done <- parseFormat12(format12Sub(c.groups), out)
			}()
			select {
			case err := <-done:
				if err != nil {
					return
				}
			case <-timeoutCh():
				t.Fatal("parseFormat12 did not terminate on an unbounded group")
			}
			for r := range out {
				if r < 0 || r > unicode.MaxRune {
					t.Errorf("parseFormat12 emitted invalid rune %d", r)
				}
			}
			if len(out) > unicode.MaxRune+1 {
				t.Errorf("parseFormat12 produced %d entries", len(out))
			}
		})
	}
}

func TestParseFormat12ValidGroupStillWorks(t *testing.T) {
	out := map[rune]uint32{}
	if err := parseFormat12(format12Sub([][3]uint32{{0x41, 0x43, 10}}), out); err != nil {
		t.Fatalf("parseFormat12: %v", err)
	}
	want := map[rune]uint32{0x41: 10, 0x42: 11, 0x43: 12}
	if len(out) != len(want) {
		t.Fatalf("out = %v, want %v", out, want)
	}
	for r, g := range want {
		if out[r] != g {
			t.Errorf("out[%d] = %d, want %d", r, out[r], g)
		}
	}
}

func TestParseFormat12ClampsToMaxRune(t *testing.T) {
	out := map[rune]uint32{}
	if err := parseFormat12(format12Sub([][3]uint32{{unicode.MaxRune - 2, 0xFFFFFFF0, 7}}), out); err != nil {
		t.Fatalf("parseFormat12: %v", err)
	}
	if len(out) != 3 {
		t.Errorf("len(out) = %d, want 3 entries clamped at MaxRune", len(out))
	}
	if _, ok := out[unicode.MaxRune]; !ok {
		t.Error("MaxRune missing from the clamped range")
	}
}

func timeoutCh() <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Second)
		close(ch)
	}()
	return ch
}

func tag(s string) uint32 {
	return binary.BigEndian.Uint32([]byte(s))
}

type rawTable struct {
	tag  string
	data []byte
}

func buildSFNT(version uint32, tables []rawTable) []byte {
	headerSize := 12 + len(tables)*16
	pos := headerSize
	offsets := make([]int, len(tables))
	for i, t := range tables {
		pos = (pos + 3) &^ 3
		offsets[i] = pos
		pos += len(t.data)
	}
	pos = (pos + 3) &^ 3
	b := make([]byte, pos)
	binary.BigEndian.PutUint32(b[0:4], version)
	binary.BigEndian.PutUint16(b[4:6], uint16(len(tables)))
	for i, t := range tables {
		entry := 12 + i*16
		binary.BigEndian.PutUint32(b[entry:], tag(t.tag))
		binary.BigEndian.PutUint32(b[entry+8:], uint32(offsets[i]))
		binary.BigEndian.PutUint32(b[entry+12:], uint32(len(t.data)))
		copy(b[offsets[i]:], t.data)
	}
	return b
}

func newHead() []byte {
	h := make([]byte, 54)
	binary.BigEndian.PutUint32(h[0:4], 0x00010000)
	binary.BigEndian.PutUint32(h[12:16], 0x5F0F3CF5)
	binary.BigEndian.PutUint16(h[18:20], 1000)
	return h
}

func TestParseSFNT(t *testing.T) {
	good := buildSFNT(0x00010000, []rawTable{
		{"cmap", []byte{1, 2, 3, 4, 5}},
		{"head", newHead()},
	})

	pastEOF := buildSFNT(0x00010000, []rawTable{{"cmap", []byte{1, 2, 3, 4}}})
	binary.BigEndian.PutUint32(pastEOF[12+12:], 0xFFFF)

	dirTruncated := buildSFNT(0x00010000, []rawTable{{"cmap", []byte{1, 2, 3, 4}}})
	binary.BigEndian.PutUint16(dirTruncated[4:6], 100)

	cases := []struct {
		name    string
		in      []byte
		wantErr bool
	}{
		{"nil", nil, true},
		{"header truncated", make([]byte, 11), true},
		{"unsupported version", buildSFNT(0x74746366, nil), true},
		{"woff rejected", buildSFNT(0x774F4646, nil), true},
		{"truetype 1.0", buildSFNT(0x00010000, nil), false},
		{"otto", buildSFNT(0x4F54544F, nil), false},
		{"true", buildSFNT(0x74727565, nil), false},
		{"table directory truncated", dirTruncated, true},
		{"table past EOF", pastEOF, true},
		{"valid font", good, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := parseSFNT(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseSFNT() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSFNT() error = %v", err)
			}
			if f == nil {
				t.Fatal("parseSFNT() returned nil file")
			}
		})
	}

	f, err := parseSFNT(good)
	if err != nil {
		t.Fatalf("parseSFNT() error = %v", err)
	}
	if f.sfntVersion != 0x00010000 {
		t.Errorf("sfntVersion = %#x, want 0x00010000", f.sfntVersion)
	}
	if len(f.tables) != 2 {
		t.Fatalf("len(tables) = %d, want 2", len(f.tables))
	}
	if got := f.tables[tagCmap].data; !bytes.Equal(got, []byte{1, 2, 3, 4, 5}) {
		t.Errorf("cmap data = %v, want [1 2 3 4 5]", got)
	}
	if f.tables[tagHead] == nil {
		t.Error("head table missing")
	}
	if f.tables[tagCmap].tag != tagCmap {
		t.Errorf("cmap tag = %#x, want %#x", f.tables[tagCmap].tag, tagCmap)
	}
}

func TestParseSFNTCopiesData(t *testing.T) {
	raw := buildSFNT(0x00010000, []rawTable{{"cmap", []byte{9, 9, 9, 9}}})
	f, err := parseSFNT(raw)
	if err != nil {
		t.Fatalf("parseSFNT() error = %v", err)
	}
	for i := range raw {
		raw[i] = 0
	}
	if !bytes.Equal(f.tables[tagCmap].data, []byte{9, 9, 9, 9}) {
		t.Errorf("table data aliases the input buffer: %v", f.tables[tagCmap].data)
	}
}

func TestSerializeRoundTrip(t *testing.T) {
	cases := []struct {
		name   string
		tables []rawTable
	}{
		{"no tables", nil},
		{"single table", []rawTable{{"cmap", []byte{1, 2, 3, 4}}}},
		{"unaligned lengths", []rawTable{{"cmap", []byte{1, 2, 3}}, {"glyf", []byte{7}}}},
		{"with head", []rawTable{{"cmap", []byte{1, 2, 3, 4, 5}}, {"head", newHead()}, {"name", []byte{0xAA}}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := parseSFNT(buildSFNT(0x00010000, c.tables))
			if err != nil {
				t.Fatalf("parseSFNT() error = %v", err)
			}
			out, err := f.serialize()
			if err != nil {
				t.Fatalf("serialize() error = %v", err)
			}
			if len(out)%4 != 0 {
				t.Errorf("len(out) = %d, want a multiple of 4", len(out))
			}

			back, err := parseSFNT(out)
			if err != nil {
				t.Fatalf("parseSFNT(serialize()) error = %v", err)
			}
			if len(back.tables) != len(c.tables) {
				t.Fatalf("len(tables) = %d, want %d", len(back.tables), len(c.tables))
			}
			for _, tb := range c.tables {
				got := back.tables[tag(tb.tag)]
				if got == nil {
					t.Fatalf("table %q missing after round trip", tb.tag)
				}
				gotData := got.data
				if tag(tb.tag) == tagHead && len(gotData) >= 12 {
					gotData = append([]byte(nil), gotData...)
					binary.BigEndian.PutUint32(gotData[8:12], 0)
				}
				if !bytes.Equal(gotData, tb.data) {
					t.Errorf("table %q data = %v, want %v", tb.tag, gotData, tb.data)
				}
			}

			numTables := int(binary.BigEndian.Uint16(out[4:6]))
			prevTag := uint32(0)
			for i := 0; i < numTables; i++ {
				entry := 12 + i*16
				tg := binary.BigEndian.Uint32(out[entry:])
				off := binary.BigEndian.Uint32(out[entry+8:])
				length := binary.BigEndian.Uint32(out[entry+12:])
				if i > 0 && tg <= prevTag {
					t.Errorf("table directory not sorted at %d: %#x after %#x", i, tg, prevTag)
				}
				prevTag = tg
				if off%4 != 0 {
					t.Errorf("table %#x offset %d is not 4-byte aligned", tg, off)
				}
				want := tableChecksum(out[off : off+length])
				if tg == tagHead {
					continue
				}
				if got := binary.BigEndian.Uint32(out[entry+4:]); got != want {
					t.Errorf("table %#x checksum = %#x, want %#x", tg, got, want)
				}
			}
		})
	}
}

func TestSerializeHeadChecksumAdjustment(t *testing.T) {
	f, err := parseSFNT(buildSFNT(0x00010000, []rawTable{
		{"cmap", []byte{1, 2, 3, 4, 5, 6, 7}},
		{"head", newHead()},
	}))
	if err != nil {
		t.Fatalf("parseSFNT() error = %v", err)
	}
	out, err := f.serialize()
	if err != nil {
		t.Fatalf("serialize() error = %v", err)
	}
	if got := tableChecksum(out); got != 0xB1B0AFBA {
		t.Errorf("whole-font checksum = %#x, want 0xB1B0AFBA", got)
	}
}

func TestSerializeNoHeadLeavesNoAdjustment(t *testing.T) {
	f, err := parseSFNT(buildSFNT(0x00010000, []rawTable{{"cmap", []byte{1, 2, 3, 4}}}))
	if err != nil {
		t.Fatalf("parseSFNT() error = %v", err)
	}
	out, err := f.serialize()
	if err != nil {
		t.Fatalf("serialize() error = %v", err)
	}
	back, err := parseSFNT(out)
	if err != nil {
		t.Fatalf("parseSFNT(serialize()) error = %v", err)
	}
	if _, ok := back.tables[tagHead]; ok {
		t.Error("head table appeared out of nowhere")
	}
}

func TestTableChecksum(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want uint32
	}{
		{"empty", nil, 0},
		{"one word", []byte{0, 0, 0, 1}, 1},
		{"two words", []byte{0, 0, 0, 1, 0, 0, 0, 2}, 3},
		{"one tail byte zero padded", []byte{0x01}, 0x01000000},
		{"three tail bytes zero padded", []byte{0x00, 0x00, 0x01}, 0x00000100},
		{"word plus tail", []byte{0, 0, 0, 1, 0x02}, 0x02000001},
		{"overflow wraps", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 0, 2}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := tableChecksum(c.in); got != c.want {
				t.Errorf("tableChecksum(%v) = %#x, want %#x", c.in, got, c.want)
			}
		})
	}
}

func TestDirSearchParams(t *testing.T) {
	cases := []struct {
		n                                      uint16
		searchRange, entrySelector, rangeShift uint16
	}{
		{0, 0, 0, 0},
		{1, 16, 0, 0},
		{2, 32, 1, 0},
		{3, 32, 1, 16},
		{4, 64, 2, 0},
		{5, 64, 2, 16},
		{9, 128, 3, 16},
		{16, 256, 4, 0},
		{17, 256, 4, 16},
	}
	for _, c := range cases {
		sr, es, rs := dirSearchParams(c.n)
		if sr != c.searchRange || es != c.entrySelector || rs != c.rangeShift {
			t.Errorf("dirSearchParams(%d) = (%d, %d, %d), want (%d, %d, %d)",
				c.n, sr, es, rs, c.searchRange, c.entrySelector, c.rangeShift)
		}
		if c.n > 0 && sr+rs != c.n*16 {
			t.Errorf("dirSearchParams(%d): searchRange+rangeShift = %d, want %d", c.n, sr+rs, c.n*16)
		}
	}
}

func TestSerializeWritesSearchParams(t *testing.T) {
	f, err := parseSFNT(buildSFNT(0x00010000, []rawTable{
		{"cmap", []byte{1}}, {"glyf", []byte{2}}, {"head", newHead()},
	}))
	if err != nil {
		t.Fatalf("parseSFNT() error = %v", err)
	}
	out, err := f.serialize()
	if err != nil {
		t.Fatalf("serialize() error = %v", err)
	}
	wantSR, wantES, wantRS := dirSearchParams(3)
	if got := binary.BigEndian.Uint16(out[6:8]); got != wantSR {
		t.Errorf("searchRange = %d, want %d", got, wantSR)
	}
	if got := binary.BigEndian.Uint16(out[8:10]); got != wantES {
		t.Errorf("entrySelector = %d, want %d", got, wantES)
	}
	if got := binary.BigEndian.Uint16(out[10:12]); got != wantRS {
		t.Errorf("rangeShift = %d, want %d", got, wantRS)
	}
}

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
