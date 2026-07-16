package workspace

import (
	"image"
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
