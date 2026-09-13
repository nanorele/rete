package sidebar

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

func (rig *sideRig) rightClick(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonSecondary})
	rig.frame()
	rig.frame()
}

func TestEnvRowRightClickOpensMenu(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	a := rig.addEnv("e1", "Dev")
	b := rig.addEnv("e2", "Prod")
	rig.frames(3)

	hitY := float32(-1)
	for y := float32(0); y < 700 && hitY < 0; y += 2 {
		a.MenuOpen, b.MenuOpen = false, false
		rig.frame()
		rig.rightClick(40, y)
		if b.MenuOpen {
			hitY = y
		}
	}
	if hitY < 0 {
		t.Fatal("right-clicking the environment row never opened its menu")
	}
	if !b.CtxMenu.AtPointer {
		t.Error("environment right-click menu must anchor at the pointer")
	}
	if a.MenuOpen {
		t.Error("right-click on one environment left another menu open")
	}
	if *rig.host.ActiveEnvID != "" {
		t.Errorf("right-click must not select the environment, ActiveEnvID=%q", *rig.host.ActiveEnvID)
	}
	if *rig.host.DraggedEnv != nil {
		t.Error("right-click must not start an environment drag")
	}
	if *rig.host.EditingEnv != nil {
		t.Error("right-click must not open the environment editor")
	}

	rig.click(&b.MenuBtn)
	rig.click(&b.MenuBtn)
	if !b.MenuOpen || b.CtxMenu.AtPointer {
		t.Errorf("⋯ button after a right-click: open=%v atPointer=%v, want open and anchored to the button", b.MenuOpen, b.CtxMenu.AtPointer)
	}
}

func TestScriptRowRightClickOpensMenu(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.ScriptsExpanded = true
	a := rig.addScript("s1", "first")
	b := rig.addScript("s2", "second")
	rig.frames(3)

	hitY := float32(-1)
	for y := float32(0); y < 700 && hitY < 0; y += 2 {
		a.MenuOpen, b.MenuOpen = false, false
		rig.frame()
		rig.rightClick(40, y)
		if b.MenuOpen {
			hitY = y
		}
	}
	if hitY < 0 {
		t.Fatal("right-clicking the script row never opened its menu")
	}
	if !b.CtxMenu.AtPointer {
		t.Error("script right-click menu must anchor at the pointer")
	}
	if a.MenuOpen {
		t.Error("right-click on one script left another menu open")
	}
	if len(rig.opened) != 0 {
		t.Errorf("right-click must not open the script, OpenScript calls=%v", rig.opened)
	}

	rig.click(&b.MenuBtn)
	rig.click(&b.MenuBtn)
	if !b.MenuOpen || b.CtxMenu.AtPointer {
		t.Errorf("⋯ button after a right-click: open=%v atPointer=%v, want open and anchored to the button", b.MenuOpen, b.CtxMenu.AtPointer)
	}
}
