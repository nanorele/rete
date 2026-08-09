package workspace

import (
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

func (rig *searchRig) queuePointer(ev pointer.Event) {
	ev.Source = pointer.Mouse
	rig.r.Queue(ev)
	rig.frame()
}

func TestSelectionDrag_WheelScrollStillWorks(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 90), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	if !c.dragActive {
		t.Fatal("precondition: dragging must be active after press+move")
	}
	if c.selStart == c.selEnd {
		t.Fatal("precondition: dragging must have selected something")
	}
	selBefore := c.selEnd

	rig.queuePointer(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary, Scroll: f32.Pt(0, 300)})
	rig.frame()
	if c.scrollY == 0 {
		t.Fatal("wheel scroll during an active selection drag must scroll the viewport")
	}
	if c.selEnd == selBefore {
		t.Error("scrolling under a held pointer must extend the selection to the new content")
	}
	if c.selStart == c.selEnd {
		t.Error("selection must survive the scroll")
	}
}

func TestSelectionDrag_EdgeAutoScrollsViewport(t *testing.T) {
	rig := newSearchRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(60, 350), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	for i := 0; i < 30 && c.scrollY == 0; i++ {
		rig.frame()
	}
	if c.scrollY == 0 {
		t.Fatal("dragging past the bottom edge must auto-scroll the viewport")
	}
	prev := c.scrollY
	for i := 0; i < 10; i++ {
		rig.frame()
	}
	if c.scrollY <= prev {
		t.Error("auto-scroll must keep going while the pointer stays past the edge")
	}
	if c.selStart == c.selEnd {
		t.Error("auto-scrolling must keep extending the selection")
	}

	rig.queuePointer(pointer.Event{Kind: pointer.Release, Position: f32.Pt(60, 350)})
	stopped := c.scrollY
	rig.frame()
	rig.frame()
	if c.scrollY != stopped {
		t.Error("auto-scroll must stop on release")
	}
}

func TestEditorSelectionDrag_WheelScrollStillWorks(t *testing.T) {
	rig := newSearchEditorRig(prettyJSON(200), true)
	rig.frame()
	rig.frame()

	rig.queuePointer(pointer.Event{Kind: pointer.Press, Position: f32.Pt(50, 40), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(120, 90), Buttons: pointer.ButtonPrimary})
	rig.queuePointer(pointer.Event{Kind: pointer.Move, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary})

	c := rig.core()
	if !c.dragActive || c.selStart == c.selEnd {
		t.Fatal("precondition: dragging must be active and selecting")
	}

	rig.queuePointer(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(121, 91), Buttons: pointer.ButtonPrimary, Scroll: f32.Pt(0, 300)})
	rig.frame()
	if c.scrollY == 0 {
		t.Fatal("wheel scroll during an active editor selection drag must scroll the viewport")
	}
	if c.selStart == c.selEnd {
		t.Error("selection must survive the scroll")
	}
}
