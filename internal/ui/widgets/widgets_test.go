package widgets

import (
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
	"image"
	"image/color"
	"rete/internal/ui/theme"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestDebounce(t *testing.T) {
	var timer *time.Timer
	win := new(app.Window)
	ArmInvalidateTimer(&timer, win, 1*time.Millisecond)
	if timer == nil {
		t.Errorf("expected timer to be armed")
	}

	ArmInvalidateTimer(&timer, win, 1*time.Millisecond)

	var timer2 *time.Timer
	ArmInvalidateTimer(&timer2, nil, 1*time.Millisecond)
	if timer2 != nil {
		t.Errorf("expected timer not to be armed")
	}
}

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

func TestFieldDoubleClickSelectsURLPathSegment(t *testing.T) {
	const url = "{{apiUrl}}/waInstance{{idInstance}}/getStatusInstance/{{apiTokenInstance}}"
	rig := newFieldKeyRig(url)
	rig.focusEnd()

	wordStart := strings.Index(url, "getStatusInstance")
	wordEnd := wordStart + len("getStatusInstance")
	rig.ed.SetCaret(wordStart+5, wordStart+5)
	rig.frame()
	pos := rig.ed.CaretCoords()

	rig.doubleClickAt(f32.Pt(pos.X+1, pos.Y))

	start, end := rig.ed.Selection()
	if start > end {
		start, end = end, start
	}
	if start != wordStart || end != wordEnd {
		t.Fatalf("double-click on getStatusInstance selected %q [%d,%d), want [%d,%d)", url[start:end], start, end, wordStart, wordEnd)
	}

	rig = newFieldKeyRig(url)
	rig.focusEnd()
	varStart := strings.Index(url, "idInstance")
	rig.ed.SetCaret(varStart+2, varStart+2)
	rig.frame()
	pos = rig.ed.CaretCoords()
	rig.doubleClickAt(f32.Pt(pos.X+1, pos.Y))
	start, end = rig.ed.Selection()
	if start > end {
		start, end = end, start
	}
	if url[start:end] != "idInstance" {
		t.Fatalf("double-click inside {{idInstance}} selected %q, want idInstance", url[start:end])
	}
}

type fieldKeyRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	ed  *widget.Editor
}

func newFieldKeyRig(text string) *fieldKeyRig {
	rig := &fieldKeyRig{
		ops: new(op.Ops),
		th:  newTestTheme(),
		ed:  &widget.Editor{SingleLine: true},
	}
	rig.ed.SetText(text)
	return rig
}

func (rig *fieldKeyRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(400, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	HandleEditorShortcuts(gtx, rig.ed)
	for {
		if _, ok := rig.ed.Update(gtx); !ok {
			break
		}
	}
	material.Editor(rig.th, rig.ed, "").Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *fieldKeyRig) focusEnd() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
	n := utf8.RuneCountInString(rig.ed.Text())
	rig.ed.SetCaret(n, n)
}

func (rig *fieldKeyRig) pressKey(name key.Name, mods key.Modifiers) {
	rig.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rig.frame()
}

func TestFieldCtrlBackspaceDeletesWord(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.ed.Text(); got != "hello " {
		t.Fatalf("Ctrl+Backspace should delete trailing word, got %q", got)
	}
}

func TestFieldCtrlBackspaceDeletesSelection(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()
	rig.ed.SetCaret(0, utf8.RuneCountInString("hello world"))

	rig.pressKey(key.NameDeleteBackward, key.ModShortcut)
	if got := rig.ed.Text(); got != "" {
		t.Fatalf("Ctrl+Backspace with a selection should delete it, got %q", got)
	}
}

func TestFieldCtrlDeleteDeletesForwardWord(t *testing.T) {
	rig := newFieldKeyRig("hello world")
	rig.focusEnd()
	rig.ed.SetCaret(0, 0)

	rig.pressKey(key.NameDeleteForward, key.ModShortcut)
	if got := rig.ed.Text(); got != " world" {
		t.Fatalf("Ctrl+Delete should delete the leading word, got %q", got)
	}
}

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

type rig struct {
	r   input.Router
	th  *material.Theme
	sz  image.Point
	now time.Time
	w   func(layout.Context) layout.Dimensions
}

func newRig(t *testing.T, sz image.Point, w func(layout.Context) layout.Dimensions) *rig {
	t.Helper()
	return &rig{th: newTestTheme(), sz: sz, now: time.Unix(1700000000, 0), w: w}
}

func (rg *rig) gtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rg.sz),
		Source:      rg.r.Source(),
		Now:         rg.now,
	}
}

func (rg *rig) frame() layout.Dimensions {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	dims := rg.w(gtx)
	rg.r.Frame(gtx.Ops)
	return dims
}

func (rg *rig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for range n {
		d = rg.frame()
	}
	return d
}

func (rg *rig) press(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) dragTo(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) move(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) release(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frames(2)
}

func (rg *rig) click(x, y float32) {
	rg.press(x, y)
	rg.release(x, y)
}

func (rg *rig) keyPress(name key.Name, mods key.Modifiers) {
	rg.r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	rg.frame()
}

func (rg *rig) focus(tag any) {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	gtx.Execute(key.FocusCmd{Tag: tag})
	rg.w(gtx)
	rg.r.Frame(gtx.Ops)
}

func TestHandleEditorShortcuts_WordMotion(t *testing.T) {
	cases := []struct {
		name  string
		text  string
		caret int
		name2 key.Name
		want  int
	}{
		{"left-from-end", "alpha beta gamma", 16, key.NameLeftArrow, 11},
		{"left-mid-word", "alpha beta gamma", 14, key.NameLeftArrow, 11},
		{"left-at-start", "alpha beta", 0, key.NameLeftArrow, 0},
		{"right-from-start", "alpha beta gamma", 0, key.NameRightArrow, 5},
		{"right-over-space", "alpha beta gamma", 5, key.NameRightArrow, 10},
		{"right-at-end", "alpha beta", 10, key.NameRightArrow, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText(tc.text)
			var rg *rig
			rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
				return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
			})
			rg.frame()
			rg.focus(ed)
			ed.SetCaret(tc.caret, tc.caret)

			rg.keyPress(tc.name2, key.ModShortcut)
			got, end := ed.Selection()
			if got != tc.want || end != tc.want {
				t.Errorf("caret = (%d,%d), want (%d,%d)", got, end, tc.want, tc.want)
			}
		})
	}
}

func TestHandleEditorShortcuts_WordDelete(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		start, end int
		key        key.Name
		want       string
	}{
		{"backward-word", "alpha beta gamma", 16, 16, key.NameDeleteBackward, "alpha beta "},
		{"backward-at-start", "alpha beta", 0, 0, key.NameDeleteBackward, "alpha beta"},
		{"backward-selection", "alpha beta gamma", 0, 5, key.NameDeleteBackward, " beta gamma"},
		{"forward-word", "alpha beta gamma", 0, 0, key.NameDeleteForward, " beta gamma"},
		{"forward-at-end", "alpha beta", 10, 10, key.NameDeleteForward, "alpha beta"},
		{"forward-selection", "alpha beta gamma", 6, 10, key.NameDeleteForward, "alpha  gamma"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText(tc.text)
			var rg *rig
			rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
				return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
			})
			rg.frame()
			rg.focus(ed)
			ed.SetCaret(tc.start, tc.end)

			rg.keyPress(tc.key, key.ModShortcut)
			if got := ed.Text(); got != tc.want {
				t.Errorf("text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHandleEditorShortcuts_IgnoresKeyRelease(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("alpha beta")
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frame()
	rg.focus(ed)
	ed.SetCaret(10, 10)

	rg.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut, State: key.Release})
	rg.frames(2)
	if s, e := ed.Selection(); s != 10 || e != 10 {
		t.Errorf("a key release must not move the caret, got (%d,%d)", s, e)
	}
	if ed.Text() != "alpha beta" {
		t.Errorf("a key release must not edit the text, got %q", ed.Text())
	}
}

func TestHandleEditorShortcuts_UnfocusedEditorIgnoresKeys(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("alpha beta")
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)
	ed.SetCaret(10, 10)

	rg.keyPress(key.NameLeftArrow, key.ModShortcut)
	if s, _ := ed.Selection(); s != 10 {
		t.Errorf("an unfocused editor must not react, caret = %d", s)
	}
}

func TestFieldFallbackClickPlacesCaret(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("alpha beta gamma delta")
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)
	ed.SetCaret(0, 0)

	dims := rg.frames(1)
	rg.click(4, float32(dims.Size.Y)-1)
	if s, e := ed.Selection(); s != e {
		t.Errorf("a fallback click must collapse the selection, got (%d,%d)", s, e)
	}

	rg.click(float32(rg.sz.X)-2, float32(dims.Size.Y)-1)
	end, _ := ed.Selection()
	if end < 0 || end > ed.Len() {
		t.Errorf("caret %d out of range for length %d", end, ed.Len())
	}
}

func TestFieldFallbackClickIgnoresMultiClick(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("alpha beta gamma")
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	dims := rg.frames(2)
	y := float32(dims.Size.Y) - 1

	for range 3 {
		rg.press(float32(rg.sz.X)-2, y)
		rg.release(float32(rg.sz.X)-2, y)
	}
	if s, e := ed.Selection(); s < 0 || e < 0 {
		t.Errorf("selection went negative after repeated clicks: (%d,%d)", s, e)
	}
}

func TestVarClickAndHoverGlobals(t *testing.T) {
	GlobalVarClick, GlobalVarHover = nil, nil
	t.Cleanup(func() { GlobalVarClick, GlobalVarHover = nil, nil })

	ed := &widget.Editor{}
	ed.SetText("{{token}} tail")
	env := map[string]string{"token": "v"}
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, env, 0, 12)
	})
	rg.frames(2)

	entered := false
	for x := float32(5); x < 90 && !entered; x += 2 {
		rg.move(x, 10)
		if GlobalVarHover != nil {
			entered = true
		}
	}
	if !entered {
		t.Fatal("hovering the variable chip never set GlobalVarHover")
	}
	if GlobalVarHover.Name != "token" {
		t.Errorf("GlobalVarHover.Name = %q, want \"token\"", GlobalVarHover.Name)
	}
	if GlobalVarHover.Editor != ed {
		t.Error("GlobalVarHover.Editor must point at the source editor")
	}
	if GlobalVarHover.Range.Start != 0 || GlobalVarHover.Range.End != 9 {
		t.Errorf("GlobalVarHover.Range = %+v, want {0,9}", GlobalVarHover.Range)
	}

	hoverX := float32(0)
	for x := float32(5); x < 90; x += 2 {
		rg.move(x, 10)
		if GlobalVarHover != nil {
			hoverX = x
			break
		}
	}
	rg.click(hoverX, 10)
	if GlobalVarClick == nil {
		t.Fatal("clicking the variable chip never set GlobalVarClick")
	}
	if GlobalVarClick.Name != "token" {
		t.Errorf("GlobalVarClick.Name = %q, want \"token\"", GlobalVarClick.Name)
	}

	rg.move(float32(rg.sz.X)-2, 35)
	rg.frames(2)
	if GlobalVarHover != nil {
		t.Errorf("leaving the chip must clear GlobalVarHover, got %+v", GlobalVarHover)
	}
}

func TestVarClickInOverlayField(t *testing.T) {
	GlobalVarClick, GlobalVarHover = nil, nil
	t.Cleanup(func() { GlobalVarClick, GlobalVarHover = nil, nil })

	ed := &widget.Editor{}
	ed.SetText("{{missing}}")
	var rg *rig
	rg = newRig(t, image.Pt(600, 60), func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 50
		return TextFieldOverlay(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)

	for y := float32(4); y < 56 && GlobalVarClick == nil; y += 3 {
		for x := float32(5); x < 110 && GlobalVarClick == nil; x += 2 {
			rg.click(x, y)
		}
	}
	if GlobalVarClick == nil {
		t.Fatal("clicking a variable chip in the overlay field never set GlobalVarClick")
	}
	if GlobalVarClick.Name != "missing" {
		t.Errorf("GlobalVarClick.Name = %q, want \"missing\"", GlobalVarClick.Name)
	}
}

func TestVarSecondaryClickDoesNotSetGlobal(t *testing.T) {
	GlobalVarClick, GlobalVarHover = nil, nil
	t.Cleanup(func() { GlobalVarClick, GlobalVarHover = nil, nil })

	ed := &widget.Editor{}
	ed.SetText("{{token}}")
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)

	for x := float32(5); x < 90; x += 2 {
		rg.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, 10), Buttons: pointer.ButtonSecondary, Source: pointer.Mouse})
		rg.frame()
		rg.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, 10), Source: pointer.Mouse})
		rg.frames(2)
	}
	if GlobalVarClick != nil {
		t.Errorf("a secondary-button press must not set GlobalVarClick, got %+v", GlobalVarClick)
	}
}

func TestMultilineVarRects(t *testing.T) {
	GlobalVarClick, GlobalVarHover = nil, nil
	t.Cleanup(func() { GlobalVarClick, GlobalVarHover = nil, nil })

	ed := &widget.Editor{}
	ed.SetText("line one {{a}}\nline two {{b}}\n{{c}}")
	env := map[string]string{"a": "1", "c": "3"}
	var rg *rig
	rg = newRig(t, image.Pt(600, 200), func(gtx layout.Context) layout.Dimensions {
		return TextFieldOverlay(gtx, rg.th, ed, "", true, env, 0, 12)
	})
	if d := rg.frames(2); d.Size.X != 600 {
		t.Fatalf("width = %d, want 600", d.Size.X)
	}
}

func TestUnterminatedAndEmptyVarSpans(t *testing.T) {
	for _, txt := range []string{"{{unterminated", "{{}}", "{{ spaced }}", "a{{x}}b{{y}}c", "{{{{nested}}}}"} {
		ed := &widget.Editor{}
		ed.SetText(txt)
		var rg *rig
		rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
			return TextField(gtx, rg.th, ed, "", true, map[string]string{"x": "1"}, 0, 12)
		})
		if d := rg.frames(2); d.Size.X != 600 {
			t.Errorf("%q: width = %d, want 600", txt, d.Size.X)
		}
	}
}

func TestHScrollThumbDrag(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("the quick brown fox jumps over the lazy dog and keeps running far past the visible edge")
	var rg *rig
	rg = newRig(t, image.Pt(140, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	dims := rg.frames(3)
	ed.SetCaret(0, 0)
	rg.frames(2)

	if GetEditorScrollX(ed) != 0 {
		t.Fatalf("precondition: scroll must start at 0, got %d", GetEditorScrollX(ed))
	}

	y := float32(dims.Size.Y) - 3
	before := GetEditorScrollX(ed)
	rg.press(20, y)
	rg.dragTo(120, y)
	rg.release(120, y)
	if GetEditorScrollX(ed) == before {
		t.Errorf("dragging the h-scrollbar thumb never changed the scroll offset (%d)", before)
	}
	if got := GetEditorScrollX(ed); got < 0 {
		t.Errorf("scroll offset went negative: %d", got)
	}
}

func TestGetEditorScrollXUnknownEditor(t *testing.T) {
	ed := &widget.Editor{}
	ResetEditorHScroll(ed)
	if got := GetEditorScrollX(ed); got != 0 {
		t.Errorf("GetEditorScrollX on an unseen editor = %d, want 0", got)
	}
}

func TestScrollbarFadesInOnHover(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("the quick brown fox jumps over the lazy dog and keeps running far past the edge")
	var rg *rig
	rg = newRig(t, image.Pt(140, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)
	if got := GetHScroll(ed).fade.Value(); got != 0 {
		t.Fatalf("precondition: fade must start at 0, got %v", got)
	}

	for range 20 {
		rg.move(70, 10)
	}
	if got := GetHScroll(ed).fade.Value(); got <= 0 {
		t.Errorf("hovering the field must fade the scrollbar in, got %v", got)
	}

	for range 20 {
		rg.move(-50, -50)
	}
	if got := GetHScroll(ed).fade.Value(); got != 0 {
		t.Errorf("leaving the field must fade the scrollbar out, got %v", got)
	}
}

func TestUpdateHScrollFollowsCaret(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("0123456789 0123456789 0123456789 0123456789 0123456789")
	var rg *rig
	rg = newRig(t, image.Pt(120, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(3)

	ed.SetCaret(ed.Len(), ed.Len())
	rg.frames(2)
	atEnd := GetEditorScrollX(ed)
	if atEnd <= 0 {
		t.Errorf("moving the caret to the end must scroll right, got %d", atEnd)
	}

	ed.SetCaret(0, 0)
	rg.frames(2)
	if got := GetEditorScrollX(ed); got != 0 {
		t.Errorf("moving the caret home must scroll back to 0, got %d", got)
	}
}

func TestInlineRenameFieldScrollsAndClicks(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("a fairly long inline rename value that overflows the field")
	var rg *rig
	rg = newRig(t, image.Pt(120, 40), func(gtx layout.Context) layout.Dimensions {
		return InlineRenameFieldPadded(gtx, rg.th, ed, unit.Dp(3))
	})
	dims := rg.frames(3)
	if dims.Size.X != 120 {
		t.Fatalf("width = %d, want 120", dims.Size.X)
	}

	before := GetEditorScrollX(ed)
	y := float32(dims.Size.Y) - 3
	rg.press(20, y)
	rg.dragTo(100, y)
	rg.release(100, y)
	if GetEditorScrollX(ed) == before {
		t.Errorf("dragging the rename-field scrollbar never moved the offset (%d)", before)
	}
}

func TestScrollLabelWheelScrolls(t *testing.T) {
	var sl ScrollLabel
	var rg *rig
	rg = newRig(t, image.Pt(60, 30), func(gtx layout.Context) layout.Dimensions {
		return sl.Layout(gtx, rg.th, MonoLabel(rg.th, 12, "a long label that will not fit inside sixty pixels"))
	})
	rg.frames(2)
	if sl.scrollX != 0 {
		t.Fatalf("precondition: scrollX must start at 0, got %d", sl.scrollX)
	}

	rg.r.Queue(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(30, 15), Source: pointer.Mouse, Scroll: f32.Pt(40, 0)})
	rg.frames(2)
	if sl.scrollX <= 0 {
		t.Errorf("a horizontal wheel event must scroll the label, got %d", sl.scrollX)
	}

	rg.r.Queue(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(30, 15), Source: pointer.Mouse, Scroll: f32.Pt(100000, 0)})
	rg.frames(2)
	max := MeasureTextWidthCached(rg.gtx(), rg.th, 12, MonoFont, "a long label that will not fit inside sixty pixels") - 60
	if sl.scrollX > max {
		t.Errorf("scrollX = %d exceeds the maximum %d", sl.scrollX, max)
	}
}

func TestKVKeysMinWidth(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(600, 60)

	var cache KeyWidthCache
	if got := KVKeysMinWidth(gtx, th, &cache, 0, func(int) *widget.Editor { return nil }); got < gtx.Dp(unit.Dp(kvKeyFloorDp)) {
		t.Errorf("empty table min width = %d, want >= the floor", got)
	}

	short := &widget.Editor{}
	short.SetText("a")
	long := &widget.Editor{}
	long.SetText("a-considerably-longer-header-name")
	eds := []*widget.Editor{short, long}
	got := KVKeysMinWidth(gtx, th, &cache, len(eds), func(i int) *widget.Editor { return eds[i] })
	onlyShort := KVKeysMinWidth(gtx, th, &cache, 1, func(int) *widget.Editor { return short })
	if got <= onlyShort {
		t.Errorf("the longest key must drive the width: %d vs %d", got, onlyShort)
	}
	if got <= gtx.Dp(unit.Dp(kvKeyFloorDp)) {
		t.Errorf("a long key must exceed the floor, got %d", got)
	}
}

func TestKVSurfaceIsBetweenBgAndField(t *testing.T) {
	got := KVSurface()
	if got == theme.Bg && got == theme.BgField {
		t.Error("KVSurface must be a distinct mix")
	}
	if got.A == 0 {
		t.Error("KVSurface must be opaque")
	}
}

func TestDeleteButtonInsideAlpha(t *testing.T) {
	gtx := makeGtx(24, 24)
	gtx.Constraints.Min = image.Pt(20, 20)
	for _, reveal := range []float32{0, 0.5, 1, 4} {
		d := DeleteButtonInsideAlpha(gtx, reveal)
		if d.Size.X <= 0 || d.Size.Y <= 0 {
			t.Errorf("reveal %v produced no dimensions", reveal)
		}
	}
	if d := DeleteButtonInside(gtx); d.Size.X <= 0 {
		t.Error("DeleteButtonInside produced no dimensions")
	}
}

type kvState struct {
	key, value widget.Editor
	del        widget.Clickable
	keyW       float32
	drag       gesture.Drag
	lastX      float32
	belowMin   bool
	hover      Hover
	fade       Fade
}

func newKVRig(t *testing.T, st *kvState, minKey int, env map[string]string, withHover bool) *rig {
	t.Helper()
	var rg *rig
	rg = newRig(t, image.Pt(400, 40), func(gtx layout.Context) layout.Dimensions {
		var h *Hover
		var f *Fade
		if withHover {
			h, f = &st.hover, &st.fade
		}
		return KVRow(gtx, rg.th, &st.key, &st.value, &st.del,
			&st.keyW, &st.drag, &st.lastX, &st.belowMin, minKey, env, h, f)
	})
	return rg
}

func TestKVRowRendersAndDefaultsKeyWidth(t *testing.T) {
	st := &kvState{}
	st.key.SetText("Content-Type")
	st.value.SetText("application/json")
	rg := newKVRig(t, st, 80, nil, true)
	d := rg.frames(2)
	if d.Size.X <= 0 || d.Size.Y <= 0 {
		t.Fatalf("KVRow produced no dimensions: %v", d.Size)
	}
	if d.Size.X > 400 {
		t.Errorf("KVRow width %d exceeds the 400px constraint", d.Size.X)
	}
}

func TestKVRowWithoutHoverAndDrag(t *testing.T) {
	st := &kvState{}
	st.key.SetText("K")
	var rg *rig
	rg = newRig(t, image.Pt(400, 40), func(gtx layout.Context) layout.Dimensions {
		return KVRow(gtx, rg.th, &st.key, &st.value, &st.del, nil, nil, nil, nil, 60, nil, nil, nil)
	})
	if d := rg.frames(2); d.Size.X <= 0 {
		t.Fatal("KVRow with no drag or hover produced no dimensions")
	}
}

func TestKVRowDividerDragResizesKeyColumn(t *testing.T) {
	st := &kvState{}
	st.key.SetText("Key")
	st.value.SetText("Value")
	rg := newKVRig(t, st, 80, nil, true)
	rg.frames(2)

	before := st.keyW
	x := float32(80) + 4
	rg.press(x, 12)
	rg.dragTo(x+60, 12)
	rg.release(x+60, 12)
	if st.keyW <= before {
		t.Fatalf("dragging the divider right must grow keyW: %v -> %v", before, st.keyW)
	}
	if st.belowMin {
		t.Error("a widened key column must not be flagged below the minimum")
	}

	wide := st.keyW
	x = st.keyW + 4
	rg.press(x, 12)
	rg.dragTo(10, 12)
	rg.release(10, 12)
	if st.keyW >= wide {
		t.Errorf("dragging the divider left must shrink keyW: %v -> %v", wide, st.keyW)
	}
	if !st.belowMin {
		t.Error("dragging below the minimum key width must set belowMin")
	}
	if st.keyW < 8 {
		t.Errorf("keyW must stay at or above the 8dp drag floor, got %v", st.keyW)
	}
	if d := rg.frames(2); d.Size.X <= 0 {
		t.Fatal("layout broke after the divider drag")
	}
}

func TestKVRowClampsKeyWidthToAvailableSpace(t *testing.T) {
	cases := []struct {
		name     string
		keyW     float32
		belowMin bool
		minKey   int
	}{
		{"huge", 100000, false, 80},
		{"negative", -50, false, 80},
		{"zero-defaults-to-min", 0, false, 80},
		{"below-min-allowed", 5, true, 80},
		{"min-larger-than-row", 20, false, 100000},
		{"min-larger-than-row-below", 20, true, 100000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := &kvState{keyW: tc.keyW, belowMin: tc.belowMin}
			st.key.SetText("K")
			st.value.SetText("V")
			rg := newKVRig(t, st, tc.minKey, nil, true)
			d := rg.frames(2)
			if d.Size.X <= 0 || d.Size.X > 400 {
				t.Errorf("dimensions %v out of range for a 400px row", d.Size)
			}
		})
	}
}

func TestKVRowDeleteButtonClick(t *testing.T) {
	st := &kvState{}
	st.key.SetText("K")
	st.value.SetText("V")
	rg := newKVRig(t, st, 80, nil, true)
	rg.frames(2)

	st.del.Click()
	rg.frames(2)
	if st.del.Clicked(rg.gtx()) {
		t.Log("delete click consumed")
	}
}

func TestKVRowHoverRevealsDeleteButton(t *testing.T) {
	st := &kvState{}
	st.key.SetText("K")
	st.value.SetText("V")
	rg := newKVRig(t, st, 80, nil, true)
	rg.frames(2)

	for range 20 {
		rg.move(200, 12)
	}
	if !st.hover.Hovered() {
		t.Fatal("the KV row never reported hover")
	}
	if got := st.fade.Value(); got <= 0 {
		t.Errorf("hovering must fade the delete button in, got %v", got)
	}

	for range 20 {
		rg.move(-100, -100)
	}
	if got := st.fade.Value(); got != 0 {
		t.Errorf("leaving must fade the delete button out, got %v", got)
	}
	if d := rg.frames(2); d.Size.X <= 0 {
		t.Fatal("layout broke with the delete button hidden")
	}
}

func TestKVRowWithVariables(t *testing.T) {
	st := &kvState{}
	st.key.SetText("{{hdr}}")
	st.value.SetText("{{val}}")
	rg := newKVRig(t, st, 80, map[string]string{"hdr": "X"}, true)
	if d := rg.frames(2); d.Size.X <= 0 {
		t.Fatal("KVRow with variables produced no dimensions")
	}
}

func TestKVRowInNarrowRow(t *testing.T) {
	for _, w := range []int{1, 10, 40, 80, 400} {
		st := &kvState{}
		st.key.SetText("Key")
		st.value.SetText("Value")
		var rg *rig
		rg = newRig(t, image.Pt(w, 40), func(gtx layout.Context) layout.Dimensions {
			return KVRow(gtx, rg.th, &st.key, &st.value, &st.del,
				&st.keyW, &st.drag, &st.lastX, &st.belowMin, 80, nil, &st.hover, &st.fade)
		})
		if d := rg.frames(2); d.Size.X < 0 {
			t.Errorf("width %d produced negative dimensions %v", w, d.Size)
		}
	}
}

func TestMenuSurfaceVariants(t *testing.T) {
	gtx := makeGtx(800, 600)
	tag := new(int)
	content := func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(120, 60)}
	}
	if d := MenuSurface(gtx, tag, MenuMinWidthDp, content); d.Size.X <= 0 || d.Size.Y <= 0 {
		t.Errorf("MenuSurface dims = %v", d.Size)
	}
	if d := DeferMenuSurface(gtx, tag, image.Pt(700, 550), MenuMinWidthDp, content); d.Size.X <= 0 {
		t.Errorf("DeferMenuSurface dims = %v", d.Size)
	}
	anchor := MenuAnchor{Pt: image.Pt(10, 10), Clamp: image.Pt(800, 600), AlignRight: true}
	if d := DeferMenuSurfaceAt(gtx, tag, anchor, MenuMinWidthDp, content); d.Size.X <= 0 {
		t.Errorf("DeferMenuSurfaceAt dims = %v", d.Size)
	}
}

func TestDeferMenuAtVariants(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(800, 600)
	var c widget.Clickable
	items := []MenuItem{{Label: "One", Click: &c}}
	for _, a := range []MenuAnchor{
		{Pt: image.Pt(0, 0), Clamp: image.Pt(800, 600)},
		{Pt: image.Pt(790, 590), Clamp: image.Pt(800, 600)},
		{Pt: image.Pt(400, 300), Clamp: image.Pt(800, 600), AlignRight: true, AlignBottom: true},
	} {
		if d := DeferMenuAt(gtx, th, new(int), a, MenuMinWidthDp, items); d.Size.X <= 0 {
			t.Errorf("anchor %+v produced no dimensions", a)
		}
	}
}

func TestFilledPrimaryDangerButtons(t *testing.T) {
	th := newTestTheme()
	var clk widget.Clickable

	p := PrimaryButton(th, &clk, "Send")
	if p.Background != theme.BtnPrimary || p.Color != theme.BtnPrimaryFg {
		t.Errorf("PrimaryButton colors = %v/%v", p.Background, p.Color)
	}
	d := DangerButton(th, &clk, "Delete")
	if d.Background != theme.Danger || d.Color != theme.DangerFg {
		t.Errorf("DangerButton colors = %v/%v", d.Background, d.Color)
	}
	f := FilledButton(th, &clk, "Go", theme.Accent, theme.White)
	if f.Background != theme.Accent || f.Color != theme.White {
		t.Errorf("FilledButton colors = %v/%v", f.Background, f.Color)
	}
	if f.TextSize != unit.Sp(12) {
		t.Errorf("FilledButton TextSize = %v, want 12sp", f.TextSize)
	}
	if f.Inset.Left != unit.Dp(10) || f.Inset.Top != unit.Dp(6) {
		t.Errorf("FilledButton inset = %+v", f.Inset)
	}

	gtx := makeGtx(200, 60)
	if dim := p.Layout(gtx); dim.Size.X <= 0 {
		t.Error("PrimaryButton produced no dimensions")
	}
}

func TestSquareBtnHoverPaintsBackground(t *testing.T) {
	var clk widget.Clickable
	var rg *rig
	rg = newRig(t, image.Pt(40, 40), func(gtx layout.Context) layout.Dimensions {
		return SquareBtn(gtx, &clk, IconClose, rg.th)
	})
	rg.frames(2)

	rg.move(14, 14)
	if !clk.Hovered() {
		t.Fatal("the square button never reported hover")
	}
	if d := rg.frames(2); d.Size.X <= 0 {
		t.Fatal("the hovered square button produced no dimensions")
	}

	rg.move(-10, -10)
	if clk.Hovered() {
		t.Error("moving away must clear hover")
	}
}

func TestLineMetricsCacheEviction(t *testing.T) {
	th := newTestTheme()
	for i := range metricsCache {
		metricsCache[i] = cachedMetrics{}
	}
	metricsLRU = 0

	first, firstSpacing := 0, 0
	for i := range len(metricsCache) + 8 {
		gtx := makeGtx(200, 40)
		h, sp := LineMetrics(gtx, th, unit.Sp(6+i))
		if h <= 0 {
			t.Fatalf("size %d: line height = %d", 6+i, h)
		}
		if sp <= 0 {
			t.Fatalf("size %d: line spacing = %d", 6+i, sp)
		}
		if i == 0 {
			first, firstSpacing = h, sp
		}
	}

	gtx := makeGtx(200, 40)
	h, sp := LineMetrics(gtx, th, unit.Sp(6))
	if h != first || sp != firstSpacing {
		t.Errorf("re-measuring after eviction gave (%d,%d), want (%d,%d)", h, sp, first, firstSpacing)
	}
}

func TestMeasureTextWidthMonotonic(t *testing.T) {
	th := newTestTheme()
	gtx := makeGtx(400, 40)
	prev := 0
	for _, s := range []string{"a", "aa", "aaa", "aaaa"} {
		w := MeasureTextWidth(gtx, th, 12, MonoFont, s)
		if w <= prev {
			t.Errorf("%q width %d must exceed %d", s, w, prev)
		}
		prev = w
	}
	if got := MeasureTextWidth(gtx, th, 12, MonoFont, ""); got != 0 {
		t.Errorf("empty width = %d, want 0", got)
	}
}

func TestTextFieldFrozenWidth(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("some text that is wider than the frozen width")
	var rg *rig
	rg = newRig(t, image.Pt(400, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 120, 12)
	})
	d := rg.frames(2)
	if d.Size.X != 400 {
		t.Errorf("a frozen text width must not change the field width: %d", d.Size.X)
	}
	if GetEditorScrollX(ed) < 0 {
		t.Errorf("scroll offset went negative: %d", GetEditorScrollX(ed))
	}
}

func TestTextFieldOverlayClampsToMaxHeight(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("x")
	th := newTestTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Min: image.Pt(200, 0), Max: image.Pt(200, 4)},
	}
	d := TextFieldOverlay(gtx, th, ed, "", true, nil, 0, 12)
	if d.Size.Y > 4 {
		t.Errorf("height %d exceeds the 4px maximum", d.Size.Y)
	}
}

func TestTextFieldFocusedBorder(t *testing.T) {
	ed := &widget.Editor{}
	ed.SetText("focus me")
	var rg *rig
	rg = newRig(t, image.Pt(300, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frames(2)
	rg.focus(ed)
	if d := rg.frames(2); d.Size.X != 300 {
		t.Fatalf("width = %d, want 300", d.Size.X)
	}

	var rg2 *rig
	rg2 = newRig(t, image.Pt(300, 40), func(gtx layout.Context) layout.Dimensions {
		return TextFieldOverlay(gtx, rg2.th, ed, "", false, nil, 0, 12)
	})
	rg2.frames(2)
	rg2.focus(ed)
	if d := rg2.frames(2); d.Size.X != 300 {
		t.Fatalf("overlay width = %d, want 300", d.Size.X)
	}
}

func TestAddFieldHoverIgnoresEmptySize(t *testing.T) {
	gtx := makeGtx(100, 40)
	ed := &widget.Editor{}
	AddFieldHover(gtx, ed, image.Pt(0, 10))
	AddFieldHover(gtx, ed, image.Pt(10, 0))
	AddFieldHover(gtx, ed, image.Pt(-1, -1))
	AddFieldHover(gtx, ed, image.Pt(10, 10))
}

func TestScrollLabelPassesPressToRowBelow(t *testing.T) {
	var sl ScrollLabel
	var drag gesture.Drag
	pressed := false
	var rg *rig
	rg = newRig(t, image.Pt(60, 30), func(gtx layout.Context) layout.Dimensions {
		for {
			e, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
			if !ok {
				break
			}
			if e.Kind == pointer.Press {
				pressed = true
			}
		}
		st := clip.Rect{Max: image.Pt(60, 30)}.Push(gtx.Ops)
		drag.Add(gtx.Ops)
		st.Pop()
		return sl.Layout(gtx, rg.th, MonoLabel(rg.th, 12, "a long label that will not fit inside sixty pixels"))
	})
	rg.frames(2)
	rg.press(30, 15)
	rg.frame()
	if !pressed {
		t.Error("an overflowing ScrollLabel must not swallow presses aimed at the row below it")
	}
}

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
		got := MenuAnchor{Pt: c.anchor, Clamp: bounds}.Resolve(c.size)
		if got != c.want {
			t.Errorf("case %d: resolve(%v,%v)=%v, want %v", i, c.anchor, c.size, got, c.want)
		}
	}
}

func TestMenuAnchorAlign(t *testing.T) {
	size := image.Pt(120, 80)
	got := MenuAnchor{Pt: image.Pt(300, 40), AlignRight: true}.Resolve(size)
	if got != image.Pt(180, 40) {
		t.Errorf("AlignRight resolve=%v, want (180,40)", got)
	}
	got = MenuAnchor{Pt: image.Pt(10, 50), AlignBottom: true}.Resolve(size)
	if got != image.Pt(10, -30) {
		t.Errorf("AlignBottom resolve=%v, want (10,-30)", got)
	}
	got = MenuAnchor{Pt: image.Pt(500, -20), AlignRight: true, Clamp: image.Pt(400, 0)}.Resolve(size)
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

func shortcutRig(t *testing.T, ed *widget.Editor) *rig {
	t.Helper()
	var rg *rig
	rg = newRig(t, image.Pt(600, 40), func(gtx layout.Context) layout.Dimensions {
		return TextField(gtx, rg.th, ed, "", true, nil, 0, 12)
	})
	rg.frame()
	rg.focus(ed)
	return rg
}

func TestHandleEditorShortcuts_ExtendMovesCaretNotAnchor(t *testing.T) {
	cases := []struct {
		name        string
		text        string
		caret       int
		anchor      int
		key         key.Name
		wantCaret   int
		wantAnchor  int
		wantSelText string
	}{
		{
			name: "shrink ltr selection from the left", text: "alpha beta gamma",
			caret: 10, anchor: 0, key: key.NameLeftArrow,
			wantCaret: 6, wantAnchor: 0, wantSelText: "alpha ",
		},
		{
			name: "grow ltr selection to the right", text: "alpha beta gamma",
			caret: 5, anchor: 0, key: key.NameRightArrow,
			wantCaret: 10, wantAnchor: 0, wantSelText: "alpha beta",
		},
		{
			name: "grow rtl selection to the left", text: "alpha beta gamma",
			caret: 11, anchor: 16, key: key.NameLeftArrow,
			wantCaret: 6, wantAnchor: 16, wantSelText: "beta gamma",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText(tc.text)
			rg := shortcutRig(t, ed)
			ed.SetCaret(tc.caret, tc.anchor)

			rg.keyPress(tc.key, key.ModShortcut|key.ModShift)

			caret, anchor := ed.Selection()
			if caret != tc.wantCaret {
				t.Errorf("caret = %d, want %d (the caret must move, not the anchor)", caret, tc.wantCaret)
			}
			if anchor != tc.wantAnchor {
				t.Errorf("anchor = %d, want %d (the anchor must stay put)", anchor, tc.wantAnchor)
			}
			if got := ed.SelectedText(); got != tc.wantSelText {
				t.Errorf("selection = %q, want %q", got, tc.wantSelText)
			}
		})
	}
}

func TestHandleEditorShortcuts_PlainMoveUsesCaretOrigin(t *testing.T) {
	cases := []struct {
		name   string
		caret  int
		anchor int
		key    key.Name
		want   int
	}{
		{"left from caret of a ltr selection", 10, 0, key.NameLeftArrow, 6},
		{"right from caret of a ltr selection", 6, 0, key.NameRightArrow, 10},
		{"left from caret of a rtl selection", 6, 16, key.NameLeftArrow, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ed := &widget.Editor{}
			ed.SetText("alpha beta gamma")
			rg := shortcutRig(t, ed)
			ed.SetCaret(tc.caret, tc.anchor)

			rg.keyPress(tc.key, key.ModShortcut)

			caret, anchor := ed.Selection()
			if caret != tc.want || anchor != tc.want {
				t.Errorf("caret = (%d,%d), want (%d,%d) collapsed at the caret origin",
					caret, anchor, tc.want, tc.want)
			}
		})
	}
}

func tableGtx(r *input.Router, w int) layout.Context {
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, 400)),
	}
	if r != nil {
		gtx.Source = r.Source()
	}
	return gtx
}

func sampleCols() []TableColumn {
	return []TableColumn{
		{Title: "A", Width: unit.Dp(50), Min: unit.Dp(20), Align: text.Start},
		{Title: "B", Width: 0, Align: text.Start},
		{Title: "C", Width: unit.Dp(60), Align: text.End},
	}
}

func TestNewTable_ColumnsRoundTrip(t *testing.T) {
	cols := sampleCols()
	tbl := NewTable(cols)
	got := tbl.Columns()
	if len(got) != len(cols) {
		t.Fatalf("Columns() len = %d, want %d", len(got), len(cols))
	}
	for i := range cols {
		if got[i] != cols[i] {
			t.Errorf("Columns()[%d] = %+v, want %+v", i, got[i], cols[i])
		}
	}
}

func TestTable_HeaderAndRowRender(t *testing.T) {
	tbl := NewTable(sampleCols())
	th := material.NewTheme()
	var r input.Router

	for i := 0; i < 2; i++ {
		gtx := tableGtx(&r, 800)
		hd := tbl.Header(gtx, th)
		if hd.Size.X != 800 || hd.Size.Y <= 0 {
			t.Fatalf("header dims = %+v, want full width and positive height", hd.Size)
		}
		rd := tbl.Row(gtx, func(i int) layout.Widget {
			return func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), tbl.Columns()[i].Title)
				return lbl.Layout(gtx)
			}
		})
		if rd.Size.X <= 0 {
			t.Fatalf("row dims = %+v", rd.Size)
		}
		r.Frame(gtx.Ops)
	}
}

func TestFindVarNestedBraces(t *testing.T) {
	s := `'[[[['''''[[[[[[[[{{{{{{{{{{{{{sdgsgds}}}}}}}}}}}}}]]]]]]]]''''']]]]'`
	start, end, ok := FindVar(s, 0)
	if !ok {
		t.Fatal("expected a variable")
	}
	if got := s[start:end]; got != "{{sdgsgds}}" {
		t.Fatalf("variable boundary = %q, want {{sdgsgds}}", got)
	}
	if _, _, ok := FindVar(s, end); ok {
		t.Fatal("no second variable expected")
	}
	if bs, be, ok := FindVar([]byte(s), 0); !ok || bs != start || be != end {
		t.Fatalf("byte variant = (%d,%d,%v), want (%d,%d,true)", bs, be, ok, start, end)
	}
}

func TestFindVarCases(t *testing.T) {
	cases := []struct {
		in   string
		from int
		want string
		ok   bool
	}{
		{"{{a}}", 0, "{{a}}", true},
		{"x{{ a }}y{{b}}", 0, "{{ a }}", true},
		{"x{{ a }}y{{b}}", 3, "{{b}}", true},
		{"{{a}b}}", 0, "", false},
		{"{{a{b}}", 0, "", false},
		{"{{{a}}}", 0, "{{a}}", true},
		{"{{}}", 0, "{{}}", true},
		{"{{a", 0, "", false},
		{"a}}", 0, "", false},
		{"", 0, "", false},
	}
	for _, c := range cases {
		s, e, ok := FindVar(c.in, c.from)
		if ok != c.ok || (ok && c.in[s:e] != c.want) {
			got := ""
			if ok {
				got = c.in[s:e]
			}
			t.Errorf("FindVar(%q, %d) = %q,%v want %q,%v", c.in, c.from, got, ok, c.want, c.ok)
		}
	}
}

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
		{'(', true},
		{')', true},
		{'[', true},
		{']', true},
		{'{', true},
		{'}', true},
		{'"', true},
		{'\'', true},
		{'`', true},
		{'@', true},
		{'-', true},
		{'/', true},
		{'=', true},
		{'&', true},
		{'+', true},
		{'<', true},
		{'>', true},
		{'|', true},
		{'#', true},
		{'$', true},
		{'%', true},
		{'^', true},
		{'*', true},
		{'~', true},
		{'a', false},
		{'1', false},
		{'_', false},
		{'\u0441', false},
	}

	for _, tc := range tests {
		result := IsSeparator(tc.r)
		if result != tc.expected {
			t.Errorf("expected %v for %q, got %v", tc.expected, string(tc.r), result)
		}
	}
}

func TestMoveWord(t *testing.T) {
	s := "hello, world! this is a test."

	testsRight := []struct {
		pos      int
		expected int
	}{
		{0, 5},
		{2, 5},
		{5, 12},
		{12, 18},
		{28, 29},
		{29, 29},
	}

	for _, tc := range testsRight {
		result := MoveWord(s, tc.pos, 1)
		if result != tc.expected {
			t.Errorf("Right: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}

	testsLeft := []struct {
		pos      int
		expected int
	}{
		{29, 24},
		{24, 22},
		{12, 7},
		{5, 0},
		{0, 0},
		{-1, 0},
	}

	for _, tc := range testsLeft {
		result := MoveWord(s, tc.pos, -1)
		if result != tc.expected {
			t.Errorf("Left: expected %d for pos %d, got %d", tc.expected, tc.pos, result)
		}
	}
}

func TestUIWidgetsLayout(t *testing.T) {
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(500, 500)),
	}

	var ed widget.Editor
	ed.SetText("test {{var}} and {{missing}}")
	env := map[string]string{"var": "val"}

	TextFieldOverlay(gtx, th, &ed, "hint", true, env, 0, 12)
	TextFieldOverlay(gtx, th, &ed, "hint", false, env, 200, 12)

	TextField(gtx, th, &ed, "hint", true, env, 0, 12)
	TextField(gtx, th, &ed, "hint", false, env, 200, 12)

	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)
	SquareBtn(gtx, &btn, ic, th)

	MenuRow(gtx, th, MenuItem{Label: "Option", Click: &btn, Icon: ic})

	HandleEditorShortcuts(gtx, &ed)

	MeasureTextWidth(gtx, th, 12, MonoFont, "test")

	LineMetrics(gtx, th, 12)
}

func TestMoveWordEdgeCases(t *testing.T) {

	if p := MoveWord("", 0, 1); p != 0 {
		t.Errorf("expected 0 for empty string, got %d", p)
	}

	s := "   ,,,   "
	if p := MoveWord(s, 0, 1); p != 9 {
		t.Errorf("expected end of string for only separators, got %d", p)
	}

	s = "hello"
	if p := MoveWord(s, 0, 1); p != 5 {
		t.Errorf("expected end of word, got %d", p)
	}
	if p := MoveWord(s, 5, -1); p != 0 {
		t.Errorf("expected start of word, got %d", p)
	}
}

func TestMoveWordRussian(t *testing.T) {
	s := "Привет, мир! Это тест."

	rightCases := []struct {
		pos      int
		expected int
	}{
		{0, 6},
		{3, 6},
		{6, 11},
		{11, 16},
		{14, 16},
		{17, 21},
	}
	for _, tc := range rightCases {
		got := MoveWord(s, tc.pos, 1)
		if got != tc.expected {
			t.Errorf("Right: from %d expected %d, got %d", tc.pos, tc.expected, got)
		}
	}

	leftCases := []struct {
		pos      int
		expected int
	}{
		{21, 17},
		{17, 13},
		{6, 0},
		{0, 0},
	}
	for _, tc := range leftCases {
		got := MoveWord(s, tc.pos, -1)
		if got != tc.expected {
			t.Errorf("Left: from %d expected %d, got %d", tc.pos, tc.expected, got)
		}
	}
}

func TestMoveWordEmoji(t *testing.T) {
	s := "Hello 🚀 World 🔥"

	rightFromZero := MoveWord(s, 0, 1)
	if rightFromZero != 5 {
		t.Errorf("from 0 expected 5 (end of Hello), got %d", rightFromZero)
	}

	rightFromSpace := MoveWord(s, 6, 1)
	if rightFromSpace != 7 {
		t.Errorf("from 6 (rocket) expected 7 (after rocket), got %d", rightFromSpace)
	}

	leftFromEnd := MoveWord(s, 16, -1)
	if leftFromEnd != 14 {
		t.Errorf("from end expected 14 (fire start), got %d", leftFromEnd)
	}
}

func TestMoveWordZWJ(t *testing.T) {
	s := "a 👨‍👩‍👧‍👦 b"

	rightFromZero := MoveWord(s, 0, 1)
	if rightFromZero != 1 {
		t.Errorf("from 0 expected 1 (end of 'a'), got %d", rightFromZero)
	}

	rightFromA := MoveWord(s, 2, 1)
	totalRunes := 0
	for range s {
		totalRunes++
	}
	if rightFromA <= 2 || rightFromA > totalRunes {
		t.Errorf("from 2 (family) expected position past family, got %d (total runes %d)", rightFromA, totalRunes)
	}
}

func TestMoveWordCJK(t *testing.T) {
	s := "你好 世界 测试"

	right1 := MoveWord(s, 0, 1)
	if right1 != 2 {
		t.Errorf("from 0 expected 2 (end of word), got %d", right1)
	}

	right2 := MoveWord(s, 3, 1)
	if right2 != 5 {
		t.Errorf("from 3 expected 5, got %d", right2)
	}

	left := MoveWord(s, 8, -1)
	if left != 6 {
		t.Errorf("from 8 expected 6, got %d", left)
	}
}

func TestMoveWordMixedScripts(t *testing.T) {
	s := "hello мир 你好 🚀end"

	pos := 0
	pos = MoveWord(s, pos, 1)
	if pos != 5 {
		t.Errorf("first word: expected 5, got %d", pos)
	}

	pos = MoveWord(s, pos, 1)
	if pos != 9 {
		t.Errorf("second word: expected 9 (мир end), got %d", pos)
	}

	pos = MoveWord(s, pos, 1)
	if pos != 12 {
		t.Errorf("third word: expected 12 (你好 end), got %d", pos)
	}
}

func TestMoveWordRoundTrip(t *testing.T) {
	inputs := []string{
		"hello world",
		"Привет мир",
		"a 🚀 b",
		"你好 世界",
		"mixed: AAA bbb 你 🔥 final",
	}
	for _, s := range inputs {
		totalRunes := 0
		for range s {
			totalRunes++
		}
		for pos := 0; pos <= totalRunes; pos++ {
			r := MoveWord(s, pos, 1)
			if r < pos {
				t.Errorf("%q: right from %d went backward to %d", s, pos, r)
			}
			if r > totalRunes {
				t.Errorf("%q: right from %d went past totalRunes %d to %d", s, pos, totalRunes, r)
			}
			l := MoveWord(s, pos, -1)
			if l > pos {
				t.Errorf("%q: left from %d went forward to %d", s, pos, l)
			}
			if l < 0 {
				t.Errorf("%q: left from %d went negative to %d", s, pos, l)
			}
		}
	}
}

func TestTextField_VarDetection(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(500, 50)),
	}

	ed := &widget.Editor{}
	env := map[string]string{"var": "val"}

	texts := []string{
		"a {{var}} b",
		"a {{missing}} b",
		"unterminated {{var",
		"nested {{{{var}}}}",
		"multiple {{a}} {{b}}",
	}

	for _, text := range texts {
		ed.SetText(text)
		TextField(gtx, th, ed, "hint", true, env, 0, 12)
		TextFieldOverlay(gtx, th, ed, "hint", true, env, 0, 12)
	}
}

func TestTextField_NoWrap(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(100, 50)),
	}

	ed := &widget.Editor{}
	ed.SetText("a very long line that should scroll horizontally")

	TextField(gtx, th, ed, "hint", false, nil, 0, 12)
}

func TestSquareBtn_Layout(t *testing.T) {
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Constraints: layout.Exact(image.Pt(50, 50)),
	}
	var btn widget.Clickable
	ic, _ := widget.NewIcon(icons.ActionBuild)

	SquareBtn(gtx, &btn, ic, th)

	MenuRow(gtx, th, MenuItem{Label: "Option", Click: &btn, Icon: ic})
}
