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

func vstackGtx(sz image.Point) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
		Now:         time.Unix(1700000000, 0),
	}
}

func TestVStackRequestPaneShrinksToHeader(t *testing.T) {
	for _, expanded := range []bool{false, true} {
		name := "compact-headers"
		if expanded {
			name = "expanded-headers"
		}
		t.Run(name, func(t *testing.T) {
			tab := NewRequestTab("T1")
			tab.Method = "POST"
			tab.URLInput.SetText("http://example.com")
			tab.ReqEditor.SetText("{\n  \"a\": 1\n}")
			tab.LayoutMode = LayoutModeVert
			tab.HeadersExpanded = expanded
			tab.VStackRatio = 0.01

			win := new(app.Window)
			th := material.NewTheme()
			th.Shaper = material.NewTheme().Shaper

			render := func() {
				gtx := vstackGtx(image.Pt(800, 600))
				tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
			}
			render()
			if tab.reqHeaderH <= 0 {
				t.Fatalf("request header height was not recorded during layout")
			}
			tab.VStackRatio = 0.01
			render()

			gtx := vstackGtx(image.Pt(800, 600))
			extent := tab.stackedSplitExtent(gtx)
			minPx := tab.stackedReqPaneMinPx(gtx)
			gotPx := tab.VStackRatio * extent

			if diff := gotPx - float32(minPx); diff < -2 || diff > 2 {
				t.Errorf("clamped request pane height = %.1fpx, want ~%dpx (min without editor)", gotPx, minPx)
			}

			nonEditor := float32(tab.reqPaneAboveHeadersPx(gtx) + tab.reqPaneBelowHeadersPx(gtx) - gtx.Dp(unit.Dp(3)))
			if expanded {
				if got := tab.headersRenderH; !near(got, gtx.Dp(unit.Dp(tab.HeadersAbsHeight)), 2) {
					t.Errorf("headers must not be squeezed at the pane minimum: rendered %d, stored %ddp", got, tab.HeadersAbsHeight)
				}
				nonEditor += float32(tab.headersRenderH)
			} else {
				nonEditor -= float32(gtx.Dp(unit.Dp(4)) + gtx.Dp(unit.Dp(1)))
			}
			editorPx := gotPx - nonEditor
			if editorPx < 0 || editorPx > float32(gtx.Dp(unit.Dp(10))) {
				t.Errorf("space left beside header chrome at min pane height = %.1fpx, want ~0", editorPx)
			}
		})
	}
}
