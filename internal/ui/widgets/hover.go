package widgets

import (
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/op"
)

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

func (h *Hover) Pos() f32.Point { return h.pos }
