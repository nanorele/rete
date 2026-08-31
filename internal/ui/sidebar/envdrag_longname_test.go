package sidebar

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
)

const longEnvName = "Production environment with a very long overflowing name"

func TestSidebarRig_EnvDragWorksWithOverflowingName(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 800))
	a := rig.addEnv("e1", longEnvName)
	rig.addEnv("e2", "Prod")
	rig.addEnv("e3", "QA")
	rig.frames(2)

	h := *rig.host.EnvRowH
	y := float32(rig.envRowY(0))
	rig.r.Queue(pointer.Event{
		Kind: pointer.Press, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y),
	})
	rig.frame()
	if *rig.host.DraggedEnv != a {
		t.Fatalf("press on a row with an overflowing name should latch the drag, got %v", *rig.host.DraggedEnv)
	}
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, y+10),
	})
	rig.frame()
	if !*rig.host.DragEnvActive {
		t.Fatal("moving past the slop threshold should activate the env drag")
	}
	end := y + 10 + float32(2*h)
	rig.r.Queue(pointer.Event{
		Kind: pointer.Move, Source: pointer.Mouse,
		Buttons: pointer.ButtonPrimary, Position: f32.Pt(60, end),
	})
	rig.frame()
	rig.r.Queue(pointer.Event{
		Kind: pointer.Release, Source: pointer.Mouse, Position: f32.Pt(60, end),
	})
	rig.frames(2)
	if (*rig.host.Environments)[0] == a {
		t.Errorf("dragging down should reorder: %q still first", (*rig.host.Environments)[0].Data.Name)
	}
}
