package sidebar

import (
	"image"
	"testing"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/unit"
)

type drow struct {
	Drag  gesture.Drag
	Hover gesture.Hover
}

// Models the production loop: a frame whose scroll position changed schedules
// one follow-up frame (the Invalidate). With the prepass keeping handlers alive
// AND the follow-up frame draining the frame-time Leave, the hover settles on
// its own — no user input and no unrelated invalidate needed.
func TestHoverSettlesAfterScroll(t *testing.T) {
	const N = 10
	const rowH = 20
	const viewH = 100
	rows := make([]*drow, N)
	for i := range rows {
		rows[i] = &drow{}
	}
	var list layout.List
	list.Axis = layout.Vertical
	r := new(input.Router)
	var prevFirst, prevOffset int

	// renderFrame returns true if the scroll position changed (i.e. production
	// would call Invalidate, scheduling another frame).
	renderFrame := func() bool {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(100, viewH)),
			Source:      r.Source(),
		}
		for _, rw := range rows { // prepass
			rw.Hover.Update(gtx.Source)
		}
		list.Layout(gtx, N, func(gtx layout.Context, i int) layout.Dimensions {
			rw := rows[i]
			rw.Hover.Update(gtx.Source)
			size := image.Pt(gtx.Constraints.Max.X, rowH)
			st := clip.Rect{Max: size}.Push(gtx.Ops)
			rw.Hover.Add(gtx.Ops)
			st.Pop()
			return layout.Dimensions{Size: size}
		})
		r.Frame(gtx.Ops)
		scrolled := list.Position.First != prevFirst || list.Position.Offset != prevOffset
		prevFirst, prevOffset = list.Position.First, list.Position.Offset
		return scrolled
	}

	// drive emulates the window loop: keep rendering while frames are scheduled
	// (by Invalidate-on-scroll), with a safety cap.
	drive := func() {
		for i := 0; i < 20; i++ {
			if !renderFrame() {
				return
			}
		}
		t.Fatal("frames never settled (possible invalidate loop)")
	}

	entered := func() []int {
		var e []int
		for i, rw := range rows {
			if en, _ := peekHover(&rw.Hover); en {
				e = append(e, i)
			}
		}
		return e
	}

	drive()
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(50, 10), Source: pointer.Mouse})
	drive()
	if got := entered(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("expected only row 0 hovered, got %v", got)
	}

	// jump-scroll (fast wheel): row 0 leaves the cursor; row 5 lands under it.
	list.Position.First = 5
	list.Position.Offset = 0
	r.Queue(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(50, 10), Source: pointer.Mouse, Scroll: f32.Pt(0, 0)})
	drive() // loop settles via invalidate-on-scroll; NO extra user input
	got := entered()
	t.Logf("after scroll settled: entered=%v", got)
	if len(got) != 1 || got[0] != 5 {
		t.Errorf("stale hover: expected only row 5 hovered after settle, got %v", got)
	}
}
