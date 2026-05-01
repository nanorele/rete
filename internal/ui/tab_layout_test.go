package ui

import (
	"image"
	"testing"
	"time"

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
	tab.addHeader("Auth", "secret")
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


	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})
	
	// Test Search
	tab.SearchOpen = true
	tab.SearchEditor.SetText("hello")
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})
	
	// Test preview mode
	tab.PreviewEnabled = true
	tab.respSize = 1000
	tab.previewLoaded = 500
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test method list open
	tab.MethodListOpen = true
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test send menu open
	tab.SendMenuOpen = true
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test headers expanded
	tab.HeadersExpanded = true
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test dragging
	tab.IsDraggingSplit = true
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test response saved to file but preview disabled
	tab.PreviewEnabled = false
	tab.respFile = "some-file"
	tab.respSize = 100
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test layoutResponseBody with content
	tab.PreviewEnabled = true
	tab.RespEditor.SetText("line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n")
	tab.layoutResponseBody(gtx, th, win, false)
	
	// Test without wrap
	tab.WrapEnabled = false
	tab.layoutResponseBody(gtx, th, win, false)

	// Test isRequesting
	tab.isRequesting = true
	tab.downloadedBytes.Store(500)
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})
	
	// Test response arriving
	tab.responseChan <- tabResponse{status: "200 OK", respSize: 1000, body: "ok", requestID: tab.requestID.Load()}
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})
	
	// Test append arriving
	tab.appendChan <- "more"
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})

	// Test FileSaveChan arriving
	tab.FileSaveChan <- &failingWriteCloser{}
	tab.layout(gtx, th, win, nil, false, func(){}, func(*ParsedCollection){})
}
