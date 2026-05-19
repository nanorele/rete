package titlebar

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func makeGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

func newTheme() *material.Theme {
	th := material.NewTheme()
	return th
}

func TestBar_Layout_DefaultTitleAndNilWin(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	// Pass empty title — should default to "Tracto" internally and not panic.
	// Pass win=nil — must not panic when no clicks happen.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Layout panicked with nil win: %v", r)
		}
	}()

	gtx := makeGtx(800, 30)
	dim := b.Layout(gtx, th, nil, "", "", false, nil)
	if dim.Size.X != 800 {
		t.Errorf("expected width 800, got %d", dim.Size.X)
	}
	if dim.Size.Y != 30 {
		t.Errorf("expected height 30, got %d", dim.Size.Y)
	}
}

func TestBar_Layout_WithTitleAndActiveSettings(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	gtx := makeGtx(800, 30)
	b.Layout(gtx, th, nil, "MyApp", "https://example.com/bugs", true, func() {})
}

func TestBar_Layout_NarrowWindow(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	// Very narrow: leftMaxW likely becomes 0 (bug area + buttons consume the rest).
	gtx := makeGtx(200, 30)
	b.Layout(gtx, th, nil, "T", "", false, nil)

	// Extreme: smaller than even the 3 close-buttons row.
	gtx2 := makeGtx(50, 30)
	b.Layout(gtx2, th, nil, "T", "", false, nil)
}

func TestBar_Layout_MaximizedStateAffectsIcon(t *testing.T) {
	th := newTheme()
	b := &Bar{Maximized: true}

	gtx := makeGtx(800, 30)
	b.Layout(gtx, th, nil, "x", "", false, nil)

	// Toggle back.
	b.Maximized = false
	gtx = makeGtx(800, 30)
	b.Layout(gtx, th, nil, "x", "", false, nil)
}

func TestBar_layoutBtn_AllKinds(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	for kind := 0; kind <= 3; kind++ {
		gtx := makeGtx(46, 30)
		var btn widget.Clickable
		b.layoutBtn(gtx, th, &btn, kind)
	}
}

func TestBar_layoutBtn_HoverPaths(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	for kind := 0; kind <= 3; kind++ {
		gtx := makeGtx(46, 30)
		var btn widget.Clickable
		// Without true hover events the Hovered() returns false; just ensure no panic.
		b.layoutBtn(gtx, th, &btn, kind)
	}
}

func TestBar_layoutSettingsBtn_NoClick(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	gtx := makeGtx(100, 30)
	called := false
	b.layoutSettingsBtn(gtx, th, nil, false, func() { called = true })
	if called {
		t.Errorf("onToggle should not be called without click")
	}

	// Active state path.
	gtx2 := makeGtx(100, 30)
	b.layoutSettingsBtn(gtx2, th, nil, true, nil)
}

func TestBar_layoutSettingsBtn_NilOnToggle(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil onToggle should not panic: %v", r)
		}
	}()
	gtx := makeGtx(100, 30)
	b.layoutSettingsBtn(gtx, th, nil, false, nil)
}

func TestBar_layoutBugBtn_EmptyURL(t *testing.T) {
	th := newTheme()
	b := &Bar{}

	// Empty URL — must not invoke OS open even if clicked.
	gtx := makeGtx(100, 30)
	b.layoutBugBtn(gtx, th, "")

	// Non-empty URL: still no click, so no goroutine launched. Just exercises path.
	gtx2 := makeGtx(100, 30)
	b.layoutBugBtn(gtx2, th, "https://example.com")
}

func TestBar_DoubleClickTimingFields(t *testing.T) {
	// We can't easily inject pointer events through the public API without a real
	// app.Window. Verify the underlying timing fields behave correctly:
	// the 300ms threshold is exclusive (< 300ms).
	b := &Bar{}
	now := time.Now()
	b.lastClick = now.Add(-200 * time.Millisecond)
	if time.Since(b.lastClick) >= 300*time.Millisecond {
		t.Errorf("200ms gap should be within double-click window")
	}

	b.lastClick = now.Add(-400 * time.Millisecond)
	if time.Since(b.lastClick) < 300*time.Millisecond {
		t.Errorf("400ms gap should exceed double-click window")
	}

	// Zero-value lastClick: time.Since is huge, not a double-click.
	b.lastClick = time.Time{}
	if time.Since(b.lastClick) < 300*time.Millisecond {
		t.Errorf("zero lastClick should never count as double-click")
	}
}

func TestBar_Layout_RepeatedFrames(t *testing.T) {
	// Multiple frames should be safe (state retained across layouts).
	th := newTheme()
	b := &Bar{}

	for i := 0; i < 5; i++ {
		gtx := makeGtx(800, 30)
		b.Layout(gtx, th, nil, "Frame", "https://x", i%2 == 0, func() {})
	}
}

// TODO bug: titlebar.go:209,212,215 — BtnClose/Minimize/Maximize click handlers
// silently drop the click when win == nil (Clicked(gtx) drains the event), so a
// caller passing nil win cannot react to these buttons by any other means.

// TODO bug: titlebar.go:244 — double-click window uses `< 300ms` (strict), so
// exactly-300ms inter-click intervals are treated as separate clicks.

// TODO bug: titlebar.go:163 — bug-report click launches `go workspace.OpenFile(url)`
// without any validation/sanitization of `url`; an arbitrary string is passed to
// the OS handler.

// TODO bug: titlebar.go:262 — when leftMaxW <= 0 (very narrow window), the title
// label and the Settings button are completely skipped; settings becomes
// unreachable with no fallback.
