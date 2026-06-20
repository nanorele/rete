package fontsubset_test

import (
	"testing"
	"tracto/internal/ui/fontsubset"
)

func TestIsEmojiCodepoint(t *testing.T) {
	cases := []struct {
		r    rune
		want bool
	}{
		{'0', false}, {'5', false}, {'9', false},
		{'#', false}, {'*', false},
		{'A', false}, {'я', false}, {'α', false}, {' ', false},
		{'.', false}, {',', false}, {';', false},
		{0x00A9, true},
		{0x2764, true},
		{0x26A0, true},
		{0x2600, true},
		{0x2603, true},
		{0x26C4, true},
		{0x26A1, true},
		{0x2B1C, true},
		{0x260E, true},
		{0x2708, true},
		{0x2615, true},
		{0x1F600, true},
		{0x1F680, true},
		{0x1F1FA, true},
	}
	for _, c := range cases {
		if got := fontsubset.IsEmojiCodepoint(c.r); got != c.want {
			t.Errorf("IsEmojiCodepoint(U+%04X) = %v, want %v", c.r, got, c.want)
		}
	}
}
