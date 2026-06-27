package workspace

import (
	"image"
	"testing"
	"time"

	"tracto/internal/ui/collections"
	"tracto/internal/wsproto"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func TestWSTabLayoutSmoke(t *testing.T) {
	tab := NewRequestTab("WS")
	tab.Method = MethodWS
	tab.URLInput.SetText("wss://api.oneme.ru/websocket")
	tab.AddHeader("Origin", "https://web.max.ru")

	s := tab.EnsureWS()
	s.OptionsExpanded = true
	s.UseMsgpackProto = true
	s.AddSubprotocol("graphql-transport-ws")
	s.ComposerEditor.SetText(`{"hello":"world"}`)
	raw, _, err := wsproto.Encode(wsproto.Frame{Cmd: 1, Seq: 2, Opcode: 3, Payload: map[string]any{"hi": "there"}})
	if err != nil {
		t.Fatal(err)
	}
	s.Messages = append(s.Messages, WSDisplayMessage{
		Time:   time.Now(),
		Opcode: 2,
		Proto:  decodeProtoView(raw),
	})
	s.Selected = 0

	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	render := func(w, h int) {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(w, h)),
			Now:         time.Now(),
		}
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}

	render(1100, 700)
	render(420, 360)

	s.ListMode = wsListSubprotos
	render(1100, 700)

	s.ListMode = wsListHeaders
	s.OptionsExpanded = false
	render(1100, 700)
}
