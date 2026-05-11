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

	tab.SearchOpen = true
	tab.SearchEditor.SetText("hello")
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

	tab.appendChan <- "more"
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})

	tab.FileSaveChan <- &failingWriteCloser{}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
}
