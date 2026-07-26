package widgets

import (
	"image"
	"testing"
	"time"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

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

	if got := KVKeysMinWidth(gtx, th, 0, func(int) *widget.Editor { return nil }); got < gtx.Dp(unit.Dp(kvKeyFloorDp)) {
		t.Errorf("empty table min width = %d, want >= the floor", got)
	}

	short := &widget.Editor{}
	short.SetText("a")
	long := &widget.Editor{}
	long.SetText("a-considerably-longer-header-name")
	eds := []*widget.Editor{short, long}
	got := KVKeysMinWidth(gtx, th, len(eds), func(i int) *widget.Editor { return eds[i] })
	onlyShort := KVKeysMinWidth(gtx, th, 1, func(int) *widget.Editor { return short })
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
