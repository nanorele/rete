package widgets

import (
	"image"
	"testing"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func TestIsSeparator(t *testing.T) {
	tests := []struct {
		r        rune
		expected bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'.', true},
		{',', true},
		{':', true},
		{';', true},
		{'!', true},
		{'?', true},
		{'(', true},
		{')', true},
		{'[', true},
		{']', true},
		{'{', true},
		{'}', true},
		{'"', true},
		{'\'', true},
		{'`', true},
		{'-', false},
		{'a', false},
		{'1', false},
		{'_', false},
	}

	for _, tc := range tests {
		result := IsSeparator(tc.r)
		if result != tc.expected {
			t.Errorf("expected %v for %q, got %v", tc.expected, string(tc.r), result)
		}
	}
}

func TestMoveWord(t *testing.T) {
	s := "hello, world! this is a test."

	testsRight := []struct {
		pos      int
		expected int
	}{
		{0, 5},
		{2, 5},
		{5, 12},
		{12, 18},
		{28, 29},
		{29, 29},
	}

	for _, tc := range testsRight {
		result := MoveWord(s, tc.pos, 1)
		if result != tc.expected {
			t.Errorf("Right: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}

	testsLeft := []struct {
		pos      int
		expected int
	}{
		{29, 24},
		{24, 22},
		{12, 7},
		{5, 0},
		{0, 0},
		{-1, 0},
	}

	for _, tc := range testsLeft {
		result := MoveWord(s, tc.pos, -1)
		if result != tc.expected {
			t.Errorf("Left: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}
}

func TestUIWidgetsLayout(t *testing.T) {
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(500, 500)),
	}

	var ed widget.Editor
	ed.SetText("test {{var}} and {{missing}}")
	env := map[string]string{"var": "val"}

	TextFieldOverlay(gtx, th, &ed, "hint", true, env, 0, 12)
	TextFieldOverlay(gtx, th, &ed, "hint", false, env, 200, 12)

	TextField(gtx, th, &ed, "hint", true, env, 0, 12)
	TextField(gtx, th, &ed, "hint", false, env, 200, 12)

	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)
	SquareBtn(gtx, &btn, ic, th)

	MenuOption(gtx, th, &btn, "Option", ic)

	HandleEditorShortcuts(gtx, &ed)

	MeasureTextWidth(gtx, th, 12, MonoFont, "test")

	LineMetrics(gtx, th, 12)
}

func TestMoveWordEdgeCases(t *testing.T) {

	if p := MoveWord("", 0, 1); p != 0 {
		t.Errorf("expected 0 for empty string, got %d", p)
	}

	s := "   ,,,   "
	if p := MoveWord(s, 0, 1); p != 9 {
		t.Errorf("expected end of string for only separators, got %d", p)
	}

	s = "hello"
	if p := MoveWord(s, 0, 1); p != 5 {
		t.Errorf("expected end of word, got %d", p)
	}
	if p := MoveWord(s, 5, -1); p != 0 {
		t.Errorf("expected start of word, got %d", p)
	}
}

func TestMoveWordRussian(t *testing.T) {
	s := "Привет, мир! Это тест."

	rightCases := []struct {
		pos      int
		expected int
	}{
		{0, 6},
		{3, 6},
		{6, 11},
		{11, 16},
		{14, 16},
		{17, 21},
	}
	for _, tc := range rightCases {
		got := MoveWord(s, tc.pos, 1)
		if got != tc.expected {
			t.Errorf("Right: from %d expected %d, got %d", tc.pos, tc.expected, got)
		}
	}

	leftCases := []struct {
		pos      int
		expected int
	}{
		{21, 17},
		{17, 13},
		{6, 0},
		{0, 0},
	}
	for _, tc := range leftCases {
		got := MoveWord(s, tc.pos, -1)
		if got != tc.expected {
			t.Errorf("Left: from %d expected %d, got %d", tc.pos, tc.expected, got)
		}
	}
}

func TestMoveWordEmoji(t *testing.T) {
	s := "Hello 🚀 World 🔥"

	rightFromZero := MoveWord(s, 0, 1)
	if rightFromZero != 5 {
		t.Errorf("from 0 expected 5 (end of Hello), got %d", rightFromZero)
	}

	rightFromSpace := MoveWord(s, 6, 1)
	if rightFromSpace != 7 {
		t.Errorf("from 6 (rocket) expected 7 (after rocket), got %d", rightFromSpace)
	}

	leftFromEnd := MoveWord(s, 16, -1)
	if leftFromEnd != 14 {
		t.Errorf("from end expected 14 (fire start), got %d", leftFromEnd)
	}
}

func TestMoveWordZWJ(t *testing.T) {
	s := "a 👨‍👩‍👧‍👦 b"

	rightFromZero := MoveWord(s, 0, 1)
	if rightFromZero != 1 {
		t.Errorf("from 0 expected 1 (end of 'a'), got %d", rightFromZero)
	}

	rightFromA := MoveWord(s, 2, 1)
	totalRunes := 0
	for range s {
		totalRunes++
	}
	if rightFromA <= 2 || rightFromA > totalRunes {
		t.Errorf("from 2 (family) expected position past family, got %d (total runes %d)", rightFromA, totalRunes)
	}
}

func TestMoveWordCJK(t *testing.T) {
	s := "你好 世界 测试"

	right1 := MoveWord(s, 0, 1)
	if right1 != 2 {
		t.Errorf("from 0 expected 2 (end of word), got %d", right1)
	}

	right2 := MoveWord(s, 3, 1)
	if right2 != 5 {
		t.Errorf("from 3 expected 5, got %d", right2)
	}

	left := MoveWord(s, 8, -1)
	if left != 6 {
		t.Errorf("from 8 expected 6, got %d", left)
	}
}

func TestMoveWordMixedScripts(t *testing.T) {
	s := "hello мир 你好 🚀end"

	pos := 0
	pos = MoveWord(s, pos, 1)
	if pos != 5 {
		t.Errorf("first word: expected 5, got %d", pos)
	}

	pos = MoveWord(s, pos, 1)
	if pos != 9 {
		t.Errorf("second word: expected 9 (мир end), got %d", pos)
	}

	pos = MoveWord(s, pos, 1)
	if pos != 12 {
		t.Errorf("third word: expected 12 (你好 end), got %d", pos)
	}
}

func TestMoveWordRoundTrip(t *testing.T) {
	inputs := []string{
		"hello world",
		"Привет мир",
		"a 🚀 b",
		"你好 世界",
		"mixed: AAA bbb 你 🔥 final",
	}
	for _, s := range inputs {
		totalRunes := 0
		for range s {
			totalRunes++
		}
		for pos := 0; pos <= totalRunes; pos++ {
			r := MoveWord(s, pos, 1)
			if r < pos {
				t.Errorf("%q: right from %d went backward to %d", s, pos, r)
			}
			if r > totalRunes {
				t.Errorf("%q: right from %d went past totalRunes %d to %d", s, pos, totalRunes, r)
			}
			l := MoveWord(s, pos, -1)
			if l > pos {
				t.Errorf("%q: left from %d went forward to %d", s, pos, l)
			}
			if l < 0 {
				t.Errorf("%q: left from %d went negative to %d", s, pos, l)
			}
		}
	}
}

func TestTextField_VarDetection(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(500, 50)),
	}

	ed := &widget.Editor{}
	env := map[string]string{"var": "val"}

	texts := []string{
		"a {{var}} b",
		"a {{missing}} b",
		"unterminated {{var",
		"nested {{{{var}}}}",
		"multiple {{a}} {{b}}",
	}

	for _, text := range texts {
		ed.SetText(text)
		TextField(gtx, th, ed, "hint", true, env, 0, 12)
		TextFieldOverlay(gtx, th, ed, "hint", true, env, 0, 12)
	}
}

func TestTextField_NoWrap(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(100, 50)),
	}

	ed := &widget.Editor{}
	ed.SetText("a very long line that should scroll horizontally")

	TextField(gtx, th, ed, "hint", false, nil, 0, 12)
}

func TestSquareBtn_Layout(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(50, 50)),
	}
	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)

	SquareBtn(gtx, &btn, ic, th)

	MenuOption(gtx, th, &btn, "Option", ic)
}
