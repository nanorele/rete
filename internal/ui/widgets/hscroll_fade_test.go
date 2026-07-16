package widgets

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func TestTextFieldScrollbarFadeOnHover(t *testing.T) {
	th := newTestTheme()

	ed := &widget.Editor{}
	ed.SetText("this is a very long url that overflows the narrow text field and needs horizontal scrolling")
	ResetEditorHScroll(ed)

	const w, h = 120, 28
	r := new(input.Router)
	now := time.Unix(1000, 0)
	frame := func() {
		now = now.Add(40 * time.Millisecond)
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Now:         now,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(w, h)),
			Source:      r.Source(),
		}
		TextFieldOverlay(gtx, th, ed, "url", true, nil, 0, 12)
		r.Frame(gtx.Ops)
	}

	frame()
	if f := GetHScroll(ed).fade.Value(); f != 0 {
		t.Fatalf("fade should start at 0 (not hovered), got %v", f)
	}

	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(w/2, h/2), Source: pointer.Mouse})
		frame()
	}
	if !GetHScroll(ed).hover.Hovered() {
		t.Fatalf("field should register as hovered after pointer moves inside it")
	}
	if f := GetHScroll(ed).fade.Value(); f <= 0 {
		t.Fatalf("fade should rise above 0 while hovering the field, got %v", f)
	}

	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(w/2, h-1), Source: pointer.Mouse})
		frame()
	}
	if !GetHScroll(ed).hover.Hovered() {
		t.Fatalf("field must stay hovered while the cursor is over the scrollbar thumb")
	}
	if f := GetHScroll(ed).fade.Value(); f <= 0 {
		t.Fatalf("fade must stay > 0 while the cursor is over the scrollbar thumb, got %v", f)
	}

	r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(w/2, h-3), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	frame()
	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(w+500, h+500), Source: pointer.Mouse})
		frame()
	}
	if !GetHScroll(ed).thumbDrag.Dragging() {
		t.Fatal("precondition: thumb drag should be active after press-and-move")
	}
	if f := GetHScroll(ed).fade.Value(); f <= 0 {
		t.Fatalf("fade must stay > 0 while dragging the thumb even with the pointer outside, got %v", f)
	}

	r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(w+500, h+500), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	for i := 0; i < 12; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(w+500, h+500), Source: pointer.Mouse})
		frame()
	}
	if f := GetHScroll(ed).fade.Value(); f != 0 {
		t.Fatalf("fade should fall back to 0 after the pointer leaves and drag ends, got %v", f)
	}
}
