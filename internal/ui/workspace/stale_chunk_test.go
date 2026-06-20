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

func newLayoutCtx() (layout.Context, *material.Theme, *app.Window) {
	win := new(app.Window)
	th := material.NewTheme()
	gtx := layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 600)),
		Now:         time.Now(),
	}
	return gtx, th, win
}

func TestLayoutDropsStaleAppendChunk(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.PreviewEnabled = true
	gtx, th, win := newLayoutCtx()

	curID := tab.requestID.Load()
	tab.RespEditor.SetText("")

	tab.appendChan <- appendChunk{requestID: curID - 1, text: "STALE"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "" {
		t.Fatalf("stale chunk was applied: %q", got)
	}

	tab.appendChan <- appendChunk{requestID: curID, text: "FRESH"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "FRESH" {
		t.Fatalf("fresh chunk not applied: %q", got)
	}
}

func TestLayoutDropsStalePreviewResult(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.PreviewEnabled = true
	gtx, th, win := newLayoutCtx()

	curID := tab.requestID.Load()
	tab.RespEditor.SetText("keep")

	tab.previewChan <- previewResult{requestID: curID - 1, body: "STALE-PREVIEW"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got == "STALE-PREVIEW" {
		t.Fatalf("stale preview overwrote response: %q", got)
	}

	tab.previewChan <- previewResult{requestID: curID, body: "FRESH-PREVIEW"}
	tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	if got := tab.RespEditor.Text(); got != "FRESH-PREVIEW" {
		t.Fatalf("fresh preview not applied: %q", got)
	}
}
