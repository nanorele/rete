package workspace

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"

	"tracto/internal/ui/widgets"
)

type respRig struct {
	r      input.Router
	ops    *op.Ops
	shaper *text.Shaper
	v      *ResponseViewer
	size   image.Point
	wrap   bool
}

func newRespRig(txt string, wrap bool) *respRig {
	rig := &respRig{
		ops:    new(op.Ops),
		shaper: text.NewShaper(text.WithCollection(gofont.Collection())),
		v:      NewResponseViewer(),
		size:   image.Pt(400, 300),
		wrap:   wrap,
	}
	rig.v.SetText(txt)
	return rig
}

func (rig *respRig) frame(now time.Time) {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         now,
		Source:      rig.r.Source(),
	}
	ResponseViewerStyle{
		Viewer:   rig.v,
		Shaper:   rig.shaper,
		TextSize: unit.Sp(13),
		Wrap:     rig.wrap,
		Padding:  unit.Dp(4),
	}.Layout(gtx)
	rig.r.Frame(rig.ops)
}

func (rig *respRig) click(x, y int, at time.Duration) {
	pos := f32.Pt(float32(x), float32(y))
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: at},
		pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: at},
	)
	rig.frame(time.Unix(1700000000, 0).Add(at))
}

func TestDoubleClickSelectsAcrossOSDoubleClickWindow(t *testing.T) {
	resp := `{"receiptId":3813,"body":{"typeWebhook":"incomingMessageReceived"}}`
	rig := newRespRig(resp, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	base := time.Duration(0)
	for _, gap := range []time.Duration{50, 199, 250, 350, 480} {
		base += 5 * time.Second
		rig.v.selStart, rig.v.selEnd = 0, 0
		rig.v.lastClickTime = time.Time{}
		rig.v.multiClickN = 0
		rig.click(x, y, base)
		rig.click(x, y, base+gap*time.Millisecond)
		if rig.v.selStart == rig.v.selEnd {
			t.Errorf("gap=%dms: double-click within the OS double-click window must select a word, got empty", gap)
		} else if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "receiptId" {
			t.Errorf("gap=%dms: expected word %q, got %q", gap, "receiptId", got)
		}
	}
}

func TestSlowClicksDoNotSelect(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+700*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("clicks 700ms apart must not word-select; got %q", string(rig.v.text[rig.v.selStart:rig.v.selEnd]))
	}
}

func TestClicksAtDifferentPositionsDoNotSelect(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813,"body":{"a":1}}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	y := 4 + rig.v.lastLineHeight/2

	rig.click(30, y, time.Second)
	rig.click(120, y, time.Second+80*time.Millisecond)
	if rig.v.selStart != rig.v.selEnd {
		t.Errorf("two fast clicks at far-apart positions must not word-select; got %q", string(rig.v.text[rig.v.selStart:rig.v.selEnd]))
	}
}

func (rig *respRig) clickAt(x, y int, evTime, frameNow time.Duration) {
	pos := f32.Pt(float32(x), float32(y))
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: evTime},
		pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: evTime},
	)
	rig.frame(time.Unix(1700000000, 0).Add(frameNow))
}

func TestDoubleClickSurvivesSlowFrame(t *testing.T) {
	rig := newRespRig(`{"receiptId":3813}`, true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 60, 4+rig.v.lastLineHeight/2

	rig.clickAt(x, y, 1*time.Second, 1*time.Second)
	rig.clickAt(x, y, 1300*time.Millisecond, 2800*time.Millisecond)
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("double-click 300ms apart must select even when the second press lands in a frame delayed by 1.5s")
	}
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "receiptId" {
		t.Errorf("expected word %q, got %q", "receiptId", got)
	}
}

func TestDoubleClickAfterWheelScroll(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "line%03d word%03d\n", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 20, 4+rig.v.lastLineHeight/2
	pos := f32.Pt(float32(x), float32(y))

	rig.r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, PointerID: 1, Position: pos, Scroll: f32.Pt(0, float32(rig.v.lastLineHeight)), Time: 900 * time.Millisecond})
	rig.frame(time.Unix(1700000000, 0).Add(900 * time.Millisecond))

	for _, at := range []time.Duration{time.Second, time.Second + 200*time.Millisecond} {
		rig.r.Queue(
			pointer.Event{Kind: pointer.Press, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, PointerID: 1, Time: at},
			pointer.Event{Kind: pointer.Release, Position: pos, Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, PointerID: 1, Time: at},
		)
		rig.frame(time.Unix(1700000000, 0).Add(at))
	}
	if rig.v.selStart == rig.v.selEnd {
		t.Fatal("double-click after a wheel scroll with the same pointer ID must still select a word")
	}
}

func TestCoordToByteOffsetLongLineMatchesFullShape(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 20000; i++ {
		fmt.Fprintf(&b, "\"key%04d\":\"value%04d\",", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	lineH := rig.v.lastLineHeight
	if lineH <= 0 {
		t.Fatal("no line height measured")
	}
	const pad = 4
	innerW := rig.size.X - 2*pad

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
	full := widgets.ShapeChunkForWrap(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx, rig.v.text, innerW)
	charAdv := measureCharAdvance(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx)

	for _, scrollLines := range []int{0, 70, 200} {
		rig.v.SetScrollY(scrollLines * lineH)
		rig.frame(time.Unix(1700000000, 0))
		for _, x := range []int{10, 90, 250} {
			for _, row := range []int{0, 3, 9} {
				y := pad + row*lineH + lineH/2
				got := rig.v.coordToByteOffset(gtx, x-pad, y-pad, charAdv, lineH, innerW, true)
				wrapLine := (y - pad + rig.v.scrollY) / lineH
				want := widgets.ByteOffInWrap(full, x-pad, wrapLine)
				if got != want {
					t.Errorf("scroll=%d x=%d row=%d: coordToByteOffset=%d, full-shape reference=%d", scrollLines, x, row, got, want)
				}
			}
		}
	}
}

func TestWrapNavLongLineMatchesFullShape(t *testing.T) {
	var b strings.Builder
	for i := 0; b.Len() < 20000; i++ {
		fmt.Fprintf(&b, "\"key%04d\":\"value%04d\",", i, i)
	}
	rig := newRespRig(b.String(), true)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	const pad = 4
	innerW := rig.size.X - 2*pad
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.size),
		Now:         time.Unix(1700000000, 0),
		Source:      rig.r.Source(),
	}
	full := widgets.ShapeChunkForWrap(rig.shaper, rig.v.layoutFont, rig.v.layoutSize, gtx, rig.v.text, innerW)
	isLineStart := map[int]bool{}
	for _, s := range widgets.WrapLineStarts(full) {
		isLineStart[s] = true
	}
	maxSub := widgets.WrapMaxLine(full)
	n := len(rig.v.text)

	for _, off := range []int{0, 777, 5003, 12347, n - 5, n} {
		for off > 0 && off < n && isLineStart[off] {
			off++
		}
		gotX := rig.v.visualXAt(off, gtx, innerW)
		wantX, wantSub := widgets.CaretXYInWrap(full, off)
		if gotX != wantX {
			t.Errorf("off=%d: visualXAt=%d, full-shape reference=%d", off, gotX, wantX)
		}
		for _, dir := range []int{-1, 1} {
			got := rig.v.wrapLineMoveX(off, 37, dir, gtx, innerW)
			var want int
			if dir < 0 {
				if wantSub > 0 {
					want = widgets.ByteOffInWrap(full, 37, wantSub-1)
				}
			} else {
				want = n
				if wantSub < maxSub {
					want = widgets.ByteOffInWrap(full, 37, wantSub+1)
				}
			}
			if got != want {
				t.Errorf("off=%d dir=%d: wrapLineMoveX=%d, full-shape reference=%d", off, dir, got, want)
			}
		}
	}
}

func TestTripleClickSelectsLine(t *testing.T) {
	rig := newRespRig("first line\nsecond line\nthird line", false)
	for i := 0; i < 3; i++ {
		rig.frame(time.Unix(1700000000, 0))
	}
	x, y := 40, 4+rig.v.lastLineHeight+rig.v.lastLineHeight/2

	rig.click(x, y, time.Second)
	rig.click(x, y, time.Second+120*time.Millisecond)
	rig.click(x, y, time.Second+240*time.Millisecond)
	if got := string(rig.v.text[rig.v.selStart:rig.v.selEnd]); got != "second line" {
		t.Errorf("triple-click should select the whole line; got %q", got)
	}
}
