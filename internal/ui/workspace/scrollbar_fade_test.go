package workspace

import (
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
)

func TestResponseScrollbarFadeOnHover(t *testing.T) {
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "line of response content that is long enough to overflow horizontally too")
	}
	rig := newRespRig(strings.Join(lines, "\n"), false)

	var r input.Router
	ops := new(op.Ops)
	size := rig.size
	keepVisible := false
	now := time.Unix(1700000000, 0)
	frame := func() {
		now = now.Add(40 * time.Millisecond)
		ops.Reset()
		gtx := layout.Context{
			Ops:         ops,
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(size),
			Now:         now,
			Source:      r.Source(),
		}
		layout.Stack{}.Layout(gtx,
			layout.Expanded(func(gtx layout.Context) layout.Dimensions {
				return ResponseViewerStyle{
					Viewer:   rig.v,
					Shaper:   rig.shaper,
					TextSize: unit.Sp(13),
					Wrap:     false,
					Padding:  unit.Dp(4),
				}.Layout(gtx)
			}),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return rig.v.LayoutScrollbarHover(gtx, keepVisible)
			}),
		)
		r.Frame(ops)
	}

	for i := 0; i < 3; i++ {
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 0 {
		t.Fatalf("fade should start at 0 (not hovered), got %v", f)
	}

	cx, cy := size.X/2, size.Y/2
	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(cx), float32(cy)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f <= 0 {
		t.Fatalf("fade should rise above 0 while hovering the editor, got %v", f)
	}

	rightX := size.X - 3
	for i := 0; i < 6; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(rightX), float32(cy)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f <= 0 {
		t.Fatalf("fade must stay > 0 while the cursor is over the vertical scrollbar, got %v", f)
	}

	keepVisible = true
	for i := 0; i < 12; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(size.X+500), float32(size.Y+500)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 1 {
		t.Fatalf("fade must stay fully visible while dragging even with the pointer outside, got %v", f)
	}

	keepVisible = false
	for i := 0; i < 12; i++ {
		r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(size.X+500), float32(size.Y+500)), Source: pointer.Mouse})
		frame()
	}
	if f := rig.v.ScrollbarFade(); f != 0 {
		t.Fatalf("fade should fall back to 0 after the pointer leaves and drag ends, got %v", f)
	}
}
