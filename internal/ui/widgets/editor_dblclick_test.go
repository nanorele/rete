package widgets

import (
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

func (rig *fieldKeyRig) doubleClickAt(p f32.Point) {
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: p, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5000 * time.Millisecond},
		pointer.Event{Kind: pointer.Release, Position: p, Source: pointer.Mouse, Time: 5020 * time.Millisecond},
		pointer.Event{Kind: pointer.Press, Position: p, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5100 * time.Millisecond},
		pointer.Event{Kind: pointer.Release, Position: p, Source: pointer.Mouse, Time: 5120 * time.Millisecond},
	)
	rig.frame()
}

func TestFieldDoubleClickSelectsWordInsideBrackets(t *testing.T) {
	rig := newFieldKeyRig("example (word) end")
	rig.focusEnd()

	rig.ed.SetCaret(11, 11)
	rig.frame()
	pos := rig.ed.CaretCoords()

	rig.doubleClickAt(f32.Pt(pos.X+1, pos.Y))

	start, end := rig.ed.Selection()
	if start > end {
		start, end = end, start
	}
	if start != 9 || end != 13 {
		t.Fatalf("double-click inside (word) selected [%d,%d), want [9,13)", start, end)
	}
}

func (rig *fieldKeyRig) doublePressHold(p f32.Point) {
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: p, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5000 * time.Millisecond},
		pointer.Event{Kind: pointer.Release, Position: p, Source: pointer.Mouse, Time: 5020 * time.Millisecond},
		pointer.Event{Kind: pointer.Press, Position: p, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5100 * time.Millisecond},
	)
	rig.frame()
}

func TestFieldDoubleClickPastLineEndAfterSeparator(t *testing.T) {
	rig := newFieldKeyRig("word)")
	rig.focusEnd()

	rig.doubleClickAt(f32.Pt(300, 10))

	start, end := rig.ed.Selection()
	if start != end {
		t.Fatalf("double-click past line end after ')' should select nothing, got [%d,%d)", start, end)
	}
}

func TestFieldDoubleClickDragExtendsByWord(t *testing.T) {
	rig := newFieldKeyRig("one two three")
	rig.focusEnd()

	rig.ed.SetCaret(10, 10)
	rig.frame()
	target := rig.ed.CaretCoords()
	rig.ed.SetCaret(1, 1)
	rig.frame()
	src := rig.ed.CaretCoords()

	rig.doublePressHold(f32.Pt(src.X+1, src.Y))
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(target.X+1, target.Y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5150 * time.Millisecond})
	rig.frame()

	if start, end := rig.ed.Selection(); start != 13 || end != 0 {
		t.Fatalf("word-drag from \"one\" into \"three\" selected (%d,%d), want (13,0)", start, end)
	}

	rig.r.Queue(
		pointer.Event{Kind: pointer.Move, Position: f32.Pt(src.X+1, src.Y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: 5200 * time.Millisecond},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(src.X+1, src.Y), Source: pointer.Mouse, Time: 5220 * time.Millisecond},
	)
	rig.frame()

	if start, end := rig.ed.Selection(); start != 3 || end != 0 {
		t.Fatalf("word-drag back inside anchor selected (%d,%d), want (3,0)", start, end)
	}
}

func TestFieldDoubleClickOnBracketSelectsBracket(t *testing.T) {
	rig := newFieldKeyRig("example (word) end")
	rig.focusEnd()

	rig.ed.SetCaret(8, 8)
	rig.frame()
	pos := rig.ed.CaretCoords()

	rig.doubleClickAt(f32.Pt(pos.X+1, pos.Y))

	start, end := rig.ed.Selection()
	if start > end {
		start, end = end, start
	}
	if start != 8 || end != 9 {
		t.Fatalf("double-click on '(' selected [%d,%d), want [8,9)", start, end)
	}
}
