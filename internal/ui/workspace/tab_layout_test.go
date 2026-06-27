package workspace

import (
	"image"
	"testing"
	"time"
	"tracto/internal/ui/collections"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func TestTabLayout(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com")
	tab.ReqEditor.SetText("body")
	tab.AddHeader("Auth", "secret")
	tab.addSystemHeader("Content-Type", "application/json")

	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper

	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}

	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.RespSearch.Open = true
	tab.RespSearch.Editor.SetText("hello")
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = true
	tab.respSize = 1000
	tab.previewLoaded.Store(500)
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.MethodListOpen = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.SendMenuOpen = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.HeadersExpanded = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.IsDraggingSplit = true
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = false
	tab.respFile = "some-file"
	tab.respSize = 100
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.PreviewEnabled = true
	tab.RespEditor.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n")
	tab.layoutResponseBody(gtx, th, win, false)

	tab.WrapEnabled = false
	tab.layoutResponseBody(gtx, th, win, false)

	tab.isRequesting = true
	tab.downloadedBytes.Store(500)
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.responseChan <- tabResponse{status: "200 OK", respSize: 1000, body: "ok", requestID: tab.requestID.Load()}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.appendChan <- appendChunk{requestID: tab.requestID.Load(), text: "more"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.FileSaveChan <- &failingWriteCloser{}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
}

// TestTabLayoutAllMenusOpen renders the tab with each migrated select dropdown
// open, ensuring the unified menu component lays out without panicking for
// every variant (checkmarks, color-coded rows, mono rows, protocol/method/body
// type/example, and the websocket opcode/filter menus).
func TestTabLayoutAllMenusOpen(t *testing.T) {
	win := new(app.Window)
	th := material.NewTheme()
	th.Shaper = material.NewTheme().Shaper
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(900, 700)),
		Now:         time.Now(),
	}
	render := func(tab *RequestTab) {
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}

	// HTTP request: protocol, method, body type, send and example menus.
	httpTab := NewRequestTab("http")
	httpTab.Method = "POST"
	httpTab.URLInput.SetText("http://example.com")
	httpTab.ProtocolListOpen = true
	render(httpTab)
	httpTab.ProtocolListOpen = false
	httpTab.MethodListOpen = true
	render(httpTab)
	httpTab.MethodListOpen = false
	httpTab.BodyTypeOpen = true
	render(httpTab)
	httpTab.BodyTypeOpen = false
	httpTab.RunOpen = true
	httpTab.ExampleListOpen = true
	render(httpTab)

	// WebSocket: opcode and filter menus.
	wsTab := NewRequestTab("ws")
	wsTab.Method = MethodWS
	wsTab.URLInput.SetText("ws://example.com/socket")
	s := wsTab.EnsureWS()
	s.OpcodeMenuOpen = true
	render(wsTab)
	s.OpcodeMenuOpen = false
	s.FilterMenuOpen = true
	render(wsTab)
}
