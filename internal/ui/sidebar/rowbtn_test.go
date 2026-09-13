package sidebar

import (
	"image"
	"testing"

	"rete/internal/ui/theme"
	"rete/internal/ui/widgets"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
)

func TestRowIconBtnGeometry(t *testing.T) {
	draw := rowIcon(widgets.IconMore, theme.FgMuted)
	var btn widget.Clickable
	for _, tc := range []struct{ h, want int }{{23, 23}, {40, 40}, {0, 16}} {
		for _, b := range []*widget.Clickable{&btn, nil} {
			for _, hovered := range []bool{true, false} {
				gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(200, 200)}}
				d := rowIconBtn(gtx, b, hovered, tc.h, 1, draw)
				if d.Size != image.Pt(18, tc.want) {
					t.Errorf("h=%d btn=%v hovered=%v: size %v, want 18x%d", tc.h, b != nil, hovered, d.Size, tc.want)
				}
			}
		}
	}
}

func TestRowButtonsSpanTheirRows(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	*rig.host.ScriptsExpanded = true
	root := rig.addCollection("Col A")
	req := rig.addRequest(root, "get-users", "GET")
	env := rig.addEnv("e1", "Dev")
	scr := rig.addScript("s1", "first")
	rig.frames(4)

	check := func(label string, btnH, rowH int) {
		if btnH <= 0 || btnH != rowH {
			t.Errorf("%s: menu button height %d, row height %d", label, btnH, rowH)
		}
	}
	check("collection root", root.ContentHeightPx, root.RowHeightPx)
	check("request", req.ContentHeightPx, req.RowHeightPx)
	check("environment", env.ContentHeightPx, *rig.host.EnvRowH)
	check("script", scr.ContentHeightPx, *rig.host.ScriptRowH)
}

func (rig *sideRig) moveTo(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *sideRig) pressAt(x, y float32) {
	rig.moveTo(x, y)
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Source: pointer.Mouse, Buttons: pointer.ButtonPrimary})
	rig.frame()
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frame()
	rig.frame()
}

func (rig *sideRig) scanY(x float32, cond func() bool) float32 {
	for y := float32(0); y < float32(rig.sz.Y); y += 2 {
		rig.moveTo(x, y)
		if cond() {
			return y
		}
	}
	return -1
}

func TestEnvRowButtonsHitTestAndHover(t *testing.T) {
	rig := newSideRig(t, image.Pt(260, 700))
	*rig.host.EnvsExpanded = true
	rig.addEnv("e1", "Dev")
	b := rig.addEnv("e2", "Prod")
	w := float32(rig.frames(3).Size.X)
	menuX, swatchX, gearX := w-19, w-41, w-63

	y := rig.scanY(menuX, func() bool { return b.MenuHovered && b.MenuBtn.Hovered() })
	if y < 0 {
		t.Fatal("env ⋯ button never hovered")
	}
	rig.moveTo(gearX, y)
	if !b.QuickEditBtn.Hovered() || b.MenuBtn.Hovered() {
		t.Fatalf("gear at x=%v: gear hovered=%v, menu hovered=%v", gearX, b.QuickEditBtn.Hovered(), b.MenuBtn.Hovered())
	}
	rig.moveTo(swatchX, y)
	if !b.SelectBtn.Hovered() || b.QuickEditBtn.Hovered() {
		t.Fatalf("swatch at x=%v: swatch hovered=%v, gear hovered=%v", swatchX, b.SelectBtn.Hovered(), b.QuickEditBtn.Hovered())
	}

	rig.pressAt(gearX, y)
	if *rig.host.EditingEnv != b {
		t.Fatal("pressing the gear must open the environment editor")
	}
	*rig.host.EditingEnv = nil
	rig.frame()

	rig.pressAt(swatchX, y)
	if !rig.host.EnvColorPicker.IsOpen() || *rig.host.EnvColorEnvID != "e2" {
		t.Fatalf("pressing the swatch must open the color picker for e2: open=%v id=%q", rig.host.EnvColorPicker.IsOpen(), *rig.host.EnvColorEnvID)
	}
}

func TestStickyRowMenuButtonHovers(t *testing.T) {
	host, frame, r, _, fld2, _, bandY := buildStickyMenuHost(t)

	frame()
	host.ColList.Position.First = 41
	host.ColList.Position.Offset = 0
	frame()
	frame()
	if _, ok := bandY(fld2); !ok {
		t.Fatalf("fld2 is not in the band at First=41")
	}
	const menuX = 240 - 19
	hitY := float32(-1)
	for y := float32(0); y < 200; y++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(menuX, y), Source: pointer.Mouse})
		frame()
		frame()
		if fld2.StickyHovered && fld2.StickyMenuBtn.Hovered() {
			hitY = y
			break
		}
	}
	if hitY < 0 {
		t.Fatal("the sticky row ⋯ button never reported hover")
	}
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(menuX, hitY+2), Source: pointer.Mouse})
	frame()
	frame()
	if !fld2.StickyMenuBtn.Hovered() {
		t.Fatalf("sticky ⋯ button lost hover 2px below the first hit at y=%v", hitY)
	}
}

func TestListGutterOnlyWhenScrollable(t *testing.T) {
	host, cleanup := newTestHost()
	defer cleanup()
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	list := &widget.List{}
	if g := listGutter(gtx, host.Theme, list); g != 0 {
		t.Fatalf("gutter for a non-scrollable list = %d, want 0", g)
	}
	list.Position.OffsetLast = -5
	g := listGutter(gtx, host.Theme, list)
	if g <= 0 {
		t.Fatalf("gutter for a scrollable list = %d, want > 0", g)
	}
	if s := rowSurface(image.Pt(250, 23), g); s != image.Pt(250-g, 23) {
		t.Fatalf("row surface %v, want %v", s, image.Pt(250-g, 23))
	}
	if s := rowSurface(image.Pt(1, 23), g); s.X != 0 {
		t.Fatalf("row surface narrower than the gutter must clamp to 0, got %v", s)
	}
}
