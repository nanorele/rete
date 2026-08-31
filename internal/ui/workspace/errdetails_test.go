package workspace

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"

	"tracto/internal/ui/collections"
)

func errTabTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	return th
}

func TestErrorDetailsPanelTogglesAndFitsFullMessage(t *testing.T) {
	tab := NewRequestTab("T1")
	tab.Method = "GET"
	tab.URLInput.SetText("http://example.com")
	tab.Status = "Error: " + strings.Repeat("Get \"http://example.com\": dial tcp: lookup example.com: no such host; ", 6)

	win := new(app.Window)
	th := errTabTheme()
	frame := func() {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Exact(image.Pt(800, 600)),
			Now:         time.Now(),
		}
		tab.Layout(gtx, th, win, nil, nil, false, func() {}, func(*collections.ParsedCollection) {})
	}
	panelHeight := func() int {
		gtx := layout.Context{
			Ops:         new(op.Ops),
			Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
			Constraints: layout.Constraints{Max: image.Pt(800, 600)},
			Now:         time.Now(),
		}
		return tab.layoutErrorDetails(gtx, th).Size.Y
	}

	frame()
	if h := panelHeight(); h != 0 {
		t.Fatalf("collapsed error details should take no space, got %d", h)
	}

	tab.ErrDetailsBtn.Click()
	frame()
	if !tab.ErrDetailsOpen {
		t.Fatal("clicking the details button should expand the error")
	}
	openH := panelHeight()
	if openH <= 0 {
		t.Fatal("expanded error details rendered nothing")
	}

	// The panel wraps: a message six times as long must not fit in the height a
	// single truncated status line would take.
	tab.Status = "Error: short"
	if shortH := panelHeight(); shortH >= openH {
		t.Errorf("long error panel (%d) should be taller than a short one (%d)", openH, shortH)
	}

	tab.Status = "200 OK  10ms  1 B"
	if h := panelHeight(); h != 0 {
		t.Errorf("a non-error status must not render an error panel, got %d", h)
	}
}
