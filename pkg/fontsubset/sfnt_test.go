package fontsubset

import (
	"bytes"
	"encoding/binary"
	"testing"
)

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
