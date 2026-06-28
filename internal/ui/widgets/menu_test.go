package widgets

import (
	"image"
	"image/color"
	"testing"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/widget"
)

func TestMenuListWidthAtLeastMin(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(800, 600)
	var c1, c2 widget.Clickable
	items := []MenuItem{
		{Label: "A", Click: &c1},
		{Label: "B", Click: &c2},
	}
	dims := MenuList(gtx, th, nil, MenuMinWidthDp, items)
	if dims.Size.X < MenuMinWidthDp {
		t.Errorf("menu width=%d, want >= %d", dims.Size.X, MenuMinWidthDp)
	}
	if dims.Size.Y <= 0 {
		t.Errorf("menu height=%d, want > 0", dims.Size.Y)
	}
	if dims.Size.X > MenuMinWidthDp+40 {
		t.Errorf("menu width=%d should hug content (~%d), not span the window (800)", dims.Size.X, MenuMinWidthDp)
	}
}

func TestMenuListWithSeparatorAndIconStaysNarrow(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(1000, 700)
	var c1, c2 widget.Clickable
	dims := MenuList(gtx, th, nil, MenuMinWidthDp, []MenuItem{
		{Label: "Rename", Click: &c1, Icon: IconRename},
		{Separator: true},
		{Label: "Delete", Click: &c2, Icon: IconDel, Danger: true},
	})
	if dims.Size.X > MenuMinWidthDp+40 {
		t.Errorf("menu width=%d should hug content, not span the 1000px window", dims.Size.X)
	}
}

func TestMenuListWidthGrowsWithLongLabel(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(2000, 600)
	var c widget.Clickable
	long := "This is a very long menu item label that should exceed the minimum width"
	dims := MenuList(gtx, th, nil, MenuMinWidthDp, []MenuItem{{Label: long, Click: &c}})
	if dims.Size.X <= MenuMinWidthDp {
		t.Errorf("long-label menu width=%d, want > %d", dims.Size.X, MenuMinWidthDp)
	}
}

func TestMenuListClampsToMaxWidth(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(100, 600)
	var c widget.Clickable
	dims := MenuList(gtx, th, nil, MenuMinWidthDp, []MenuItem{{Label: "Item", Click: &c}})
	if dims.Size.X > 100 {
		t.Errorf("clamped menu width=%d, want <= 100", dims.Size.X)
	}
}

func TestMenuRowVariants(t *testing.T) {
	th := newTestTheme()
	var c widget.Clickable
	cases := []MenuItem{
		{Label: "Normal", Click: &c},
		{Label: "Danger", Click: &c, Danger: true},
		{Label: "Disabled", Click: &c, Disabled: true},
		{Label: "Checked", Click: &c, Checked: true},
		{Label: "Icon", Click: &c, Icon: IconRename},
		{Label: "Mono", Click: &c, Mono: true},
		{Label: "Bold", Click: &c, Bold: true},
		{Label: "Shortcut", Click: &c, Shortcut: "Ctrl+W"},
		{Label: "Colored", Click: &c, LabelCol: color.NRGBA{R: 200, G: 100, A: 255}},
		{Label: "NoClick"},
		{Separator: true},
	}
	for i, it := range cases {
		gtx := makeGtx(300, 100)
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("MenuRow case %d panicked: %v", i, r)
				}
			}()
			dims := MenuRow(gtx, th, it)
			if dims.Size.Y <= 0 {
				t.Errorf("case %d (%q) height=%d, want > 0", i, it.Label, dims.Size.Y)
			}
		}()
	}
}

func TestMenuSeparatorIsThin(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(300, 100)
	sep := MenuRow(gtx, th, MenuItem{Separator: true})
	var c widget.Clickable
	row := MenuRow(gtx, th, MenuItem{Label: "Item", Click: &c})
	if sep.Size.Y >= row.Size.Y {
		t.Errorf("separator height=%d should be less than row height=%d", sep.Size.Y, row.Size.Y)
	}
}

func TestMenuShadowZeroSizeNoPanic(t *testing.T) {
	gtx := makeGtx(300, 100)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MenuShadow with zero size panicked: %v", r)
		}
	}()
	MenuShadow(gtx, image.Point{})
}

func TestMenuSurfaceWithTagNoPanic(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(800, 600)
	tag := new(int)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MenuSurface with tag panicked: %v", r)
		}
	}()
	var c widget.Clickable
	MenuList(gtx, th, tag, MenuMinWidthDp, []MenuItem{{Label: "X", Click: &c}})
}

func TestMenuAnchorResolveClamp(t *testing.T) {
	bounds := image.Pt(800, 600)
	cases := []struct {
		anchor, size, want image.Point
	}{
		{image.Pt(10, 10), image.Pt(100, 100), image.Pt(10, 10)},
		{image.Pt(750, 10), image.Pt(100, 100), image.Pt(700, 10)},
		{image.Pt(10, 550), image.Pt(100, 100), image.Pt(10, 500)},
		{image.Pt(-20, -20), image.Pt(100, 100), image.Pt(0, 0)},
		{image.Pt(700, 500), image.Pt(900, 700), image.Pt(0, 0)},
	}
	for i, c := range cases {
		got := MenuAnchor{Pt: c.anchor, Clamp: bounds}.resolve(c.size)
		if got != c.want {
			t.Errorf("case %d: resolve(%v,%v)=%v, want %v", i, c.anchor, c.size, got, c.want)
		}
	}
}

func TestMenuAnchorAlign(t *testing.T) {
	size := image.Pt(120, 80)
	got := MenuAnchor{Pt: image.Pt(300, 40), AlignRight: true}.resolve(size)
	if got != image.Pt(180, 40) {
		t.Errorf("AlignRight resolve=%v, want (180,40)", got)
	}
	got = MenuAnchor{Pt: image.Pt(10, 50), AlignBottom: true}.resolve(size)
	if got != image.Pt(10, -30) {
		t.Errorf("AlignBottom resolve=%v, want (10,-30)", got)
	}
	got = MenuAnchor{Pt: image.Pt(500, -20), AlignRight: true, Clamp: image.Pt(400, 0)}.resolve(size)
	if got.X != 280 || got.Y != -20 {
		t.Errorf("mixed resolve=%v, want (280,-20)", got)
	}
}

func TestDeferMenuNoPanic(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(800, 600)
	var c1, c2 widget.Clickable
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DeferMenu panicked: %v", r)
		}
	}()
	dims := DeferMenu(gtx, th, new(int), image.Pt(790, 590), MenuMinWidthDp, []MenuItem{
		{Label: "Close", Click: &c1},
		{Separator: true},
		{Label: "Delete", Click: &c2, Danger: true},
	})
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("DeferMenu dims=%v, want positive", dims.Size)
	}
}

func TestMenuRowDangerColor(t *testing.T) {
	if theme.Danger == (color.NRGBA{}) {
		t.Fatal("theme.Danger is zero")
	}
}
