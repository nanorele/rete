package widgets

import (
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/op"
)

// Hover tracks whether the pointer is inside the area it is added to, and the
// pointer's last position in that area's local coordinate space.
//
// Pos() is what makes hover lag-free for scrolling content: the area itself
// (e.g. a list viewport) stays put while its rows scroll underneath, so the
// cached local position remains valid between pointer events and a caller can
// recompute which row is under the pointer every frame from current geometry —
// instead of relying on per-row Enter/Leave events, which the gio router only
// reconciles after layout and therefore lag a content shift by one frame.
type Hover struct {
	entered bool
	pos     f32.Point
}

func (h *Hover) Add(ops *op.Ops) { event.Op(ops, h) }

func (h *Hover) Update(q input.Source) bool {
	for {
		ev, ok := q.Event(pointer.Filter{
			Target: h,
			Kinds:  pointer.Enter | pointer.Leave | pointer.Cancel | pointer.Move | pointer.Drag,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Enter:
			h.entered = true
			h.pos = e.Position
		case pointer.Move, pointer.Drag:
			h.pos = e.Position
		case pointer.Leave, pointer.Cancel:
			h.entered = false
		}
	}
	return h.entered
}

func (h *Hover) Hovered() bool { return h.entered }

// Pos reports the pointer's last known position in the area's local coordinates.
// Only meaningful while Hovered() is true.
func (h *Hover) Pos() f32.Point { return h.pos }
