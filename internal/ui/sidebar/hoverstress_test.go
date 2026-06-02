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

type srow struct {
	Hover gesture.Hover
}

// Real wheel-scroll stress: hover a row, fling up/down with many small wheel
// events (momentum), then move the cursor away. With the prepass disabling the
// router's self-heal Cancel, any count drift past 1 sticks. Models the
// production loop (invalidate-on-scroll -> follow-up frame).
func TestHoverWheelStress(t *testing.T) {
	const N = 10
	const rowH = 20
	const viewH = 100
	rows := make([]*srow, N)
	for i := range rows {
		rows[i] = &srow{}
	}
	var list layout.List
	list.Axis = layout.Vertical
	r := new(input.Router)
	var prevFirst, prevOffset int

	renderFrame := func() bool {
		ops := new(op.Ops)
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(100, viewH)),
			Source:      r.Source(),
		}
		for _, rw := range rows {
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
	drive := func() {
		for i := 0; i < 50; i++ {
			if !renderFrame() {
				return
			}
		}
	}
	maxCount := func() (int, int) {
		mi, mc := -1, 0
		for i, rw := range rows {
			_, c := peekHover(&rw.Hover)
			if c > mc {
				mc, mi = c, i
			}
		}
		return mi, mc
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
	// hover near top
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(50, 30), Source: pointer.Mouse})
	drive()

	// many small wheel steps down then up, cursor stationary at y=30
	for dir := 0; dir < 6; dir++ {
		dy := float32(8)
		if dir%2 == 1 {
			dy = -8
		}
		for s := 0; s < 12; s++ {
			r.Queue(pointer.Event{Kind: pointer.Scroll, Position: f32.Pt(50, 30), Source: pointer.Mouse, Scroll: f32.Pt(0, dy)})
			drive()
		}
		i, c := maxCount()
		t.Logf("dir=%d maxCount row=%d count=%d entered=%v first=%d", dir, i, c, entered(), list.Position.First)
		if c > 1 {
			t.Errorf("COUNT DRIFT: row %d has count=%d (>1) — will stick", i, c)
		}
	}

	// move cursor far outside and settle
	r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(50, 9999), Source: pointer.Mouse})
	drive()
	if e := entered(); len(e) > 0 {
		t.Errorf("STUCK after cursor left: entered=%v", e)
	}
}
