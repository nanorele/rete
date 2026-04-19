package ui

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
		{'-', true},
		{'(', true},
		{')', true},
		{'[', true},
		{']', true},
		{'{', true},
		{'}', true},
		{'a', false},
		{'1', false},
		{'_', false},
	}

	for _, tc := range tests {
		result := isSeparator(tc.r)
		if result != tc.expected {
			t.Errorf("expected %v for %q, got %v", tc.expected, string(tc.r), result)
		}
	}
}

func TestMoveWord(t *testing.T) {
	s := "hello, world! this is a test."
	
	// Right
	testsRight := []struct {
		pos      int
		expected int
	}{
		{0, 5},   // "hello"
		{2, 5},   // inside "hello" -> end of "hello"
		{5, 12},  // at "," -> end of "world"
		{12, 18}, // at "!" -> end of "this"
		{28, 29}, // end -> end
		{29, 29}, // out of bounds -> end
	}

	for _, tc := range testsRight {
		result := moveWord(s, tc.pos, 1)
		if result != tc.expected {
			t.Errorf("Right: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}

	// Left
	testsLeft := []struct {
		pos      int
		expected int
	}{
		{29, 24}, // end -> start of "test"
		{24, 22}, // start of "test" -> start of "a"
		{12, 7},  // end of "world" -> start of "world"
		{5, 0},   // end of "hello" -> start of "hello"
		{0, 0},   // start -> start
		{-1, 0},  // out of bounds -> start
	}

	for _, tc := range testsLeft {
		result := moveWord(s, tc.pos, -1)
		if result != tc.expected {
			t.Errorf("Left: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}
}

func TestUIWidgetsLayout(t *testing.T) {
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper
	
	gtx := layout.Context{
		Ops: new(op.Ops),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(500, 500)),
	}

	var ed widget.Editor
	ed.SetText("test {{var}} and {{missing}}")
	env := map[string]string{"var": "val"}
	
	// Test TextFieldOverlay
	TextFieldOverlay(gtx, th, &ed, "hint", true, env, 0, 12)
	TextFieldOverlay(gtx, th, &ed, "hint", false, env, 200, 12)
	
	// Test TextField
	TextField(gtx, th, &ed, "hint", true, env, 0, 12)
	TextField(gtx, th, &ed, "hint", false, env, 200, 12)

	// Test SquareBtn
	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)
	SquareBtn(gtx, &btn, ic, th)

	// Test menuOption
	menuOption(gtx, th, &btn, "Option", ic)

	// Test handleEditorShortcuts
	handleEditorShortcuts(gtx, &ed)
	
	// Test measureTextWidth
	measureTextWidth(gtx, th, 12, monoFont, "test")
	
	// Test getLineMetrics
	getLineMetrics(gtx, th, 12)
}
