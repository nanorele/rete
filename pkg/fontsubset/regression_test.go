package fontsubset

import (
	"encoding/binary"
	"testing"
	"time"
	"unicode"
)

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
