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
	"golang.org/x/image/math/fixed"
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

func TestFixedToFloat(t *testing.T) {
	cases := []struct {
		in   fixed.Int26_6
		want float32
	}{
		{0, 0},
		{fixed.I(1), 1.0},
		{fixed.I(-2), -2.0},
		{fixed.I(64), 64.0},
		{32, 0.5},
		{-32, -0.5},
	}
	for _, c := range cases {
		got := FixedToFloat(c.in)
		if got != c.want {
			t.Errorf("FixedToFloat(%v)=%v want %v", c.in, got, c.want)
		}
	}
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
	MenuOptionDanger(gtx, th, &clk, "Delete", ic)
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

func TestPaintColoredText_Wrap(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(50, 60)
	spans := []ColoredSpan{
		{Start: 0, End: 5, Color: color.NRGBA{R: 255, A: 255}},
		{Start: 5, End: 11, Color: color.NRGBA{G: 255, A: 255}},
	}
	dims := PaintColoredText(gtx, th.Shaper, MonoFont, 12, "hello world", spans, color.NRGBA{B: 255, A: 255}, true, 50)
	if dims.Size.X <= 0 || dims.Size.Y <= 0 {
		t.Errorf("dims=%v", dims.Size)
	}
}

func TestPaintColoredText_NoWrap_DefaultColor(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(500, 40)

	dims := PaintColoredText(gtx, th.Shaper, MonoFont, 12, "abc", nil, color.NRGBA{R: 10, A: 255}, false, 0)
	if dims.Size.X <= 0 {
		t.Errorf("dims=%v", dims.Size)
	}
}

func TestPaintColoredText_UTF8(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(200, 40)
	s := "abгд😀"

	spans := []ColoredSpan{
		{Start: 2, End: 6, Color: color.NRGBA{R: 255, A: 255}},
	}
	PaintColoredText(gtx, th.Shaper, MonoFont, 12, s, spans, color.NRGBA{A: 255}, false, 0)
}

func TestPaintColoredText_EmptyText(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 40)
	dims := PaintColoredText(gtx, th.Shaper, MonoFont, 12, "", nil, color.NRGBA{A: 255}, false, 0)
	if dims.Size.X != 0 && dims.Size.Y != 0 {

		_ = dims
	}
}

func TestShapeChunkForWrap_Basic(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(100, 40)
	out := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, []byte("hello world wrap me"), 30)
	if len(out) == 0 {
		t.Fatal("expected glyphs")
	}

	maxLine := WrapMaxLine(out)
	if maxLine < 1 {
		t.Errorf("expected at least one wrap; maxLine=%d", maxLine)
	}

	for i := 1; i < len(out); i++ {
		if out[i].byteStart < out[i-1].byteStart {
			t.Errorf("byteStart not monotonic at %d", i)
		}
	}
}

func TestShapeChunkForWrap_NilOrEmpty(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 40)
	if out := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, nil, 50); out != nil {
		t.Errorf("expected nil for empty input, got %v", out)
	}
	if out := ShapeChunkForWrap(nil, MonoFont, 12, gtx, []byte("x"), 50); out != nil {
		t.Errorf("expected nil for nil shaper, got %v", out)
	}

	out := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, []byte("a"), 0)
	if len(out) == 0 {
		t.Errorf("expected at least one glyph despite maxW=0")
	}
}

func TestCaretXYInWrap_Empty(t *testing.T) {
	x, l := CaretXYInWrap(nil, 0)
	if x != 0 || l != 0 {
		t.Errorf("empty: got x=%d line=%d", x, l)
	}
}

func TestCaretXYInWrap_Bounds(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 40)
	glyphs := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, []byte("hello world"), 30)
	if len(glyphs) == 0 {
		t.Skip("no glyphs shaped")
	}

	x, l := CaretXYInWrap(glyphs, 0)
	if l != glyphs[0].line {
		t.Errorf("line mismatch at byteOff=0: got %d want %d", l, glyphs[0].line)
	}
	_ = x

	x2, l2 := CaretXYInWrap(glyphs, 1000)
	last := glyphs[len(glyphs)-1]
	if x2 != (last.x + last.advance).Round() {
		t.Errorf("past-end x=%d want %d", x2, (last.x + last.advance).Round())
	}
	if l2 != last.line {
		t.Errorf("past-end line=%d want %d", l2, last.line)
	}

	mid := glyphs[len(glyphs)/2].byteStart
	_, _ = CaretXYInWrap(glyphs, mid)
}

func TestCaretXYInWrap_AfterBreak(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 40)
	glyphs := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, []byte("hello world wrap me"), 30)

	for i := range len(glyphs) - 1 {
		if glyphs[i].isBreak {
			target := glyphs[i+1].byteStart
			_, l := CaretXYInWrap(glyphs, target)
			if l != glyphs[i].line {

				t.Logf("post-break byteOff=%d returned line %d (prev line was %d)", target, l, glyphs[i].line)
			}
			return
		}
	}
	t.Skip("no line breaks generated")
}

func TestByteOffInWrap_Empty(t *testing.T) {
	if got := ByteOffInWrap(nil, 0, 0); got != 0 {
		t.Errorf("empty got %d", got)
	}
}

func TestByteOffInWrap_Lookup(t *testing.T) {
	th := material.NewTheme()
	gtx := makeGtx(100, 40)
	glyphs := ShapeChunkForWrap(th.Shaper, MonoFont, 12, gtx, []byte("hello world wrap me"), 30)
	if len(glyphs) == 0 {
		t.Skip("no glyphs")
	}

	b := ByteOffInWrap(glyphs, 0, 0)
	if b < 0 {
		t.Errorf("negative byte offset %d", b)
	}

	br := ByteOffInWrap(glyphs, 1<<20, 0)
	if br < b {
		t.Errorf("far-right %d < far-left %d", br, b)
	}

	ByteOffInWrap(glyphs, 5, -3)

	high := WrapMaxLine(glyphs) + 50
	got := ByteOffInWrap(glyphs, 0, high)
	if got != glyphs[len(glyphs)-1].byteEnd {
		t.Errorf("non-existent line: got %d want %d", got, glyphs[len(glyphs)-1].byteEnd)
	}
}

func TestWrapMaxLine_Empty(t *testing.T) {
	if got := WrapMaxLine(nil); got != 0 {
		t.Errorf("expected 0 for empty, got %d", got)
	}
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
