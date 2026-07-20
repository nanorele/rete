package widgets

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

func makeGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

func newTestTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func TestMonoLabelAndMonoButton(t *testing.T) {
	th := material.NewTheme()
	lbl := MonoLabel(th, 12, "x")
	if lbl.Font.Typeface != MonoTypeface {
		t.Errorf("MonoLabel typeface = %q, want %q", lbl.Font.Typeface, MonoTypeface)
	}
	if lbl.Text != "x" {
		t.Errorf("MonoLabel text = %q", lbl.Text)
	}

	var clk widget.Clickable
	btn := MonoButton(th, &clk, "go")
	if btn.Font.Typeface != MonoTypeface {
		t.Errorf("MonoButton typeface = %q", btn.Font.Typeface)
	}
}

func TestMeasureTextWidthCached_Empty(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 100)
	if w := MeasureTextWidthCached(gtx, th, 12, MonoFont, ""); w != 0 {
		t.Errorf("empty string width = %d, want 0", w)
	}
}

func TestMeasureTextWidthCached_Hit(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(100, 100)

	widthCache = make(map[widthCacheKey]int, 512)
	w1 := MeasureTextWidthCached(gtx, th, 12, MonoFont, "abc123")
	w2 := MeasureTextWidthCached(gtx, th, 12, MonoFont, "abc123")
	if w1 != w2 {
		t.Errorf("cache returned different widths: %d vs %d", w1, w2)
	}
	if w1 <= 0 {
		t.Errorf("expected positive width, got %d", w1)
	}
}

func TestMeasureTextWidthCached_Eviction(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 100)

	for i := range widthCacheLimit + 10 {
		s := "k" + string(rune('a'+(i%26))) + string(rune('0'+(i%10)))
		MeasureTextWidthCached(gtx, th, unit.Sp(8+(i%5)), MonoFont, s)
	}
}

func TestCaretIndexAtX(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(500, 50)

	if got := CaretIndexAtX(gtx, th, 12, "", 0); got != 0 {
		t.Errorf("empty string: got %d, want 0", got)
	}
	if got := CaretIndexAtX(gtx, th, 12, "abc", 0); got != 0 {
		t.Errorf("x=0: got %d, want 0", got)
	}
	if got := CaretIndexAtX(gtx, th, 12, "abc", -5); got != 0 {
		t.Errorf("negative x: got %d, want 0", got)
	}
	full := MeasureTextWidth(gtx, th, 12, MonoFont, "abcdef")
	if got := CaretIndexAtX(gtx, th, 12, "abcdef", full*4); got != 6 {
		t.Errorf("far right: got %d, want 6", got)
	}
	mid := CaretIndexAtX(gtx, th, 12, "abcdef", full/2)
	if mid < 1 || mid > 5 {
		t.Errorf("middle: got %d, want between 1 and 5", mid)
	}
}

func TestResetEditorHScroll(t *testing.T) {
	ed := &widget.Editor{}
	s := GetHScroll(ed)
	if s == nil {
		t.Fatal("GetHScroll returned nil")
	}
	if _, ok := editorHScrolls[ed]; !ok {
		t.Fatal("editor not registered")
	}
	ResetEditorHScroll(ed)
	if _, ok := editorHScrolls[ed]; ok {
		t.Error("expected entry to be deleted")
	}

	ResetEditorHScroll(ed)
}

func TestGetHScroll_Cleanup(t *testing.T) {
	for k := range editorHScrolls {
		delete(editorHScrolls, k)
	}
	old := &widget.Editor{}
	s := GetHScroll(old)
	s.lastSeen = time.Now().Add(-10 * time.Minute)

	for range hScrollCleanupThreshold + 2 {
		ed := &widget.Editor{}
		_ = GetHScroll(ed)
	}
	if _, ok := editorHScrolls[old]; ok {
		t.Error("expected stale entry to be evicted")
	}
}

func TestArmInvalidateTimer_NilTimer(t *testing.T) {
	var timer *time.Timer
	win := new(app.Window)
	ArmInvalidateTimer(&timer, win, 1*time.Hour)
	if timer == nil {
		t.Fatal("expected timer to be set")
	}
	timer.Stop()
}

func TestArmInvalidateTimer_Replaces(t *testing.T) {
	var timer *time.Timer
	win := new(app.Window)
	ArmInvalidateTimer(&timer, win, 1*time.Hour)
	first := timer
	ArmInvalidateTimer(&timer, win, 1*time.Hour)
	if timer == nil {
		t.Fatal("timer nil after re-arm")
	}
	if timer == first {
		t.Log("note: timer pointer reused (allowed)")
	}
	timer.Stop()
}

func TestPaintBorder1px_ZeroSize(t *testing.T) {
	gtx := makeGtx(100, 100)
	PaintBorder1px(gtx, image.Pt(0, 10), color.NRGBA{R: 1})
	PaintBorder1px(gtx, image.Pt(10, 0), color.NRGBA{R: 1})
	PaintBorder1px(gtx, image.Pt(-1, -1), color.NRGBA{R: 1})
	PaintBorder1px(gtx, image.Pt(10, 10), color.NRGBA{R: 1, A: 255})
}

func TestBordered1px(t *testing.T) {
	gtx := makeGtx(100, 100)
	dims := Bordered1px(gtx, unit.Dp(1), color.NRGBA{R: 255, A: 255}, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(40, 20)}
	})
	if dims.Size.X != 40 || dims.Size.Y != 20 {
		t.Errorf("Bordered1px size = %v, want (40,20)", dims.Size)
	}
}

func TestSquareBtnSlim(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(60, 60)
	var clk widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)
	dims := SquareBtnSlim(gtx, &clk, ic, th)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("SquareBtnSlim dims = %v", dims.Size)
	}
}

func TestMenuOptionDanger(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(300, 60)
	var clk widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionDelete)
	MenuRow(gtx, th, MenuItem{Label: "Delete", Click: &clk, Icon: ic, Danger: true})
}

func TestInlineRenameField(t *testing.T) {
	th := material.NewTheme()
	ed := &widget.Editor{}
	ed.SetText("name")

	gtx := makeGtx(200, 30)
	dims := InlineRenameField(gtx, th, ed)
	if dims.Size.X <= 0 {
		t.Errorf("dims.Size.X = %d", dims.Size.X)
	}

	gtx2 := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(0, 0)),
	}
	d := InlineRenameField(gtx2, th, ed)
	if d.Size.X != 0 || d.Size.Y != 0 {
		t.Errorf("expected zero dims for zero width, got %v", d.Size)
	}
}

func TestScrollLabel_NoScrollAndScroll(t *testing.T) {
	th := material.NewTheme()
	var sl ScrollLabel

	gtx := makeGtx(500, 40)
	lbl := MonoLabel(th, 12, "hi")
	sl.Layout(gtx, th, lbl)
	if sl.scrollX != 0 {
		t.Errorf("expected scrollX=0 after non-scrolling layout, got %d", sl.scrollX)
	}

	gtxN := makeGtx(20, 40)
	lblL := MonoLabel(th, 12, "this is a fairly long line of text that must scroll")
	dim := sl.Layout(gtxN, th, lblL)
	if dim.Size.X != 20 {
		t.Errorf("expected viewW=20, got %d", dim.Size.X)
	}

	sl.scrollX = -100
	sl.Layout(gtxN, th, lblL)
	if sl.scrollX < 0 {
		t.Errorf("expected scrollX clamped to >=0, got %d", sl.scrollX)
	}
	sl.scrollX = 1 << 20
	sl.Layout(gtxN, th, lblL)
	if sl.scrollX < 0 {
		t.Errorf("scrollX should be clamped, got %d", sl.scrollX)
	}
}

func TestUpdateHScroll_NoScrollNeeded(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := &widget.Editor{}
	ed.SetText("short")
	sx, ms, addG := UpdateHScroll(gtx, ed, 200, 50)
	if sx != 0 {
		t.Errorf("scrollX should be 0, got %d", sx)
	}
	if ms != 0 {
		t.Errorf("maxScroll should be 0, got %d", ms)
	}
	if addG == nil {
		t.Fatal("addGesture is nil")
	}
	addG()
}

func TestUpdateHScroll_ScrollNeeded(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := &widget.Editor{}
	ed.SetText("some text content")
	_, ms, addG := UpdateHScroll(gtx, ed, 100, 500)
	if ms != 400 {
		t.Errorf("maxScroll = %d, want 400", ms)
	}
	addG()
}

func TestDrawHScrollbar_NoOp(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := &widget.Editor{}

	DrawHScrollbar(gtx, ed, 50, 0, image.Pt(100, 30), 100, 4, 1)

	DrawHScrollbar(gtx, ed, 200, 0, image.Pt(100, 30), 0, 4, 1)

	DrawHScrollbar(gtx, ed, 200, 0, image.Pt(100, 2), 80, 4, 1)

	DrawHScrollbar(gtx, ed, 200, 0, image.Pt(10, 30), 80, 50, 1)
}

func TestDrawHScrollbar_Renders(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := &widget.Editor{}
	DrawHScrollbar(gtx, ed, 500, 100, image.Pt(200, 30), 100, 4, 1)

	DrawHScrollbar(gtx, ed, 500, -50, image.Pt(200, 30), 100, 4, 1)

	DrawHScrollbar(gtx, ed, 500, 10000, image.Pt(200, 30), 100, 4, 1)
}

func TestHandleFieldFallbackClick_NoEvent(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(200, 30)
	ed := &widget.Editor{}
	ed.SetText("abc")
	HandleFieldFallbackClick(gtx, th, ed, image.Pt(200, 30), image.Rect(4, 4, 196, 26), 0, 12)
}

func TestTextFieldOverlay_MinHeight(t *testing.T) {
	th := material.NewTheme()

	gtx := layout.Context{
		Ops:    new(op.Ops),
		Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{
			Min: image.Pt(200, 200),
			Max: image.Pt(400, 400),
		},
	}
	ed := &widget.Editor{}
	ed.SetText("text")
	TextFieldOverlay(gtx, th, ed, "h", true, nil, 0, 12)
}

func TestTextField_ZeroWidth(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(0, 30)),
	}
	ed := &widget.Editor{}
	dims := TextField(gtx, th, ed, "hint", true, nil, 0, 12)
	if dims.Size.X != 0 || dims.Size.Y != 0 {
		t.Errorf("expected zero dims, got %v", dims.Size)
	}

	dims2 := TextFieldOverlay(gtx, th, ed, "hint", true, nil, 0, 12)
	if dims2.Size.X != 0 || dims2.Size.Y != 0 {
		t.Errorf("overlay: expected zero dims, got %v", dims2.Size)
	}
}

func TestMustIcon_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on bad icon data")
		}
	}()
	_ = mustIcon([]byte("not valid icon data"))
}

func TestMustIcon_OK(t *testing.T) {
	ic := mustIcon(icons.ActionBuild)
	if ic == nil {
		t.Error("expected non-nil icon")
	}
}

func TestIconsInitialized(t *testing.T) {

	all := []*widget.Icon{
		IconClose, IconSettings, IconSave, IconBack, IconAddReq, IconAddFld,
		IconRename, IconDup, IconDel, IconSearch, IconBug, IconDropDown,
		IconChevronR, IconChevronL, IconChevronD, IconRefresh, IconRequests,
		IconMITM, IconShield, IconPlay, IconStop,
	}
	for i, ic := range all {
		if ic == nil {
			t.Errorf("icon index %d is nil", i)
		}
	}
}

func TestMonoFontConstants(t *testing.T) {
	if MonoFamilyName != "JetBrains Mono" {
		t.Errorf("MonoFamilyName=%q", MonoFamilyName)
	}
	if EmojiTypeface != "Noto Color Emoji" {
		t.Errorf("EmojiTypeface=%q", EmojiTypeface)
	}
	if MonoTypeface != MonoFamilyName+","+EmojiTypeface {
		t.Errorf("MonoTypeface=%q expected mono+emoji multi-family", MonoTypeface)
	}
	if MonoFont.Typeface != MonoTypeface {
		t.Errorf("MonoFont.Typeface=%q", MonoFont.Typeface)
	}
}

func TestMeasureTextWidthCached_FontWeightCollision(t *testing.T) {

	th := material.NewTheme()
	gtx := makeGtx(200, 30)
	f1 := font.Font{Typeface: MonoTypeface}
	f2 := font.Font{Typeface: MonoTypeface, Weight: font.Bold}
	w1 := MeasureTextWidthCached(gtx, th, 12, f1, "weighty")
	w2 := MeasureTextWidthCached(gtx, th, 12, f2, "weighty")

	_ = w1
	_ = w2
}
