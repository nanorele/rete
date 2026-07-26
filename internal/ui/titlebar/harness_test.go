package titlebar

import (
	"image"
	"testing"
	"time"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

type rig struct {
	b   *Bar
	th  *material.Theme
	win *app.Window
	r   input.Router
	sz  image.Point
	now time.Time

	title      string
	bugURL     string
	settings   bool
	onSettings func()
}

func shapedTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func newRig(t *testing.T, w int) *rig {
	t.Helper()
	return &rig{
		b:     &Bar{},
		th:    shapedTheme(),
		win:   new(app.Window),
		sz:    image.Pt(w, 30),
		now:   time.Unix(1700000000, 0),
		title: "Tracto",
	}
}

func (rg *rig) gtx() layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rg.sz),
		Source:      rg.r.Source(),
		Now:         rg.now,
	}
}

func (rg *rig) frame() layout.Dimensions {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	dims := rg.b.Layout(gtx, rg.th, rg.win, rg.title, rg.bugURL, rg.settings, rg.onSettings)
	rg.r.Frame(gtx.Ops)
	return dims
}

func (rg *rig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for range n {
		d = rg.frame()
	}
	return d
}

func (rg *rig) press(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) drag(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) move(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) release(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frames(2)
}

func TestWindowButtonsPerformActions(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)

	rg.b.BtnMinimize.Click()
	rg.frame()

	rg.b.BtnClose.Click()
	rg.frame()

	if rg.b.Maximized {
		t.Fatal("precondition: bar starts unmaximized")
	}
	rg.b.BtnMaximize.Click()
	rg.frame()
	if !rg.b.Maximized {
		t.Error("clicking maximize must latch Maximized")
	}
	rg.b.BtnMaximize.Click()
	rg.frame()
	if rg.b.Maximized {
		t.Error("clicking maximize again must unlatch Maximized")
	}
}

func TestWindowButtonsWithNilWindow(t *testing.T) {
	rg := newRig(t, 900)
	rg.win = nil
	rg.frames(2)
	rg.b.BtnMaximize.Click()
	rg.frame()
	if rg.b.Maximized {
		t.Error("with no window the maximize state must not change")
	}
}

func TestSettingsButtonInvokesToggle(t *testing.T) {
	rg := newRig(t, 900)
	var calls int
	rg.onSettings = func() { calls++ }
	rg.frames(2)

	rg.b.SettingsBtn.Click()
	rg.frame()
	if calls != 1 {
		t.Errorf("onToggleSettings called %d times, want 1", calls)
	}

	rg.settings = true
	rg.frames(2)
	rg.b.SettingsBtn.Click()
	rg.frame()
	if calls != 2 {
		t.Errorf("onToggleSettings called %d times, want 2", calls)
	}
}

func TestSettingsButtonClickWithNilCallback(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)
	rg.b.SettingsBtn.Click()
	rg.frames(2)
}

func TestBugButtonClickWithEmptyURL(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)
	rg.b.BugReportBtn.Click()
	rg.frames(2)
}

func TestNetBadgeToggleAndCancel(t *testing.T) {
	rg := newRig(t, 1200)
	var toggled, canceled int
	rg.b.NetActive = true
	rg.b.OnNetToggle = func() { toggled++ }
	rg.b.OnNetCancel = func() { canceled++ }
	rg.frames(2)

	rg.b.BtnNetToggle.Click()
	rg.frame()
	if toggled != 1 {
		t.Errorf("OnNetToggle called %d times, want 1", toggled)
	}

	rg.b.BtnNetCancel.Click()
	rg.frame()
	if canceled != 1 {
		t.Errorf("OnNetCancel called %d times, want 1", canceled)
	}
}

func TestNetBadgeClicksWithNilCallbacks(t *testing.T) {
	rg := newRig(t, 1200)
	rg.b.NetPaused = true
	rg.frames(2)
	rg.b.BtnNetToggle.Click()
	rg.b.BtnNetCancel.Click()
	rg.frames(2)
}

func TestNetBadgeHoverPaintsBothStates(t *testing.T) {
	rg := newRig(t, 1200)
	rg.b.NetActive = true
	rg.frames(2)

	hoveredToggle, hoveredCancel := false, false
	for x := float32(300); x < float32(rg.sz.X-200); x += 2 {
		rg.move(x, 15)
		if rg.b.BtnNetToggle.Hovered() {
			hoveredToggle = true
		}
		if rg.b.BtnNetCancel.Hovered() {
			hoveredCancel = true
		}
		if hoveredToggle && hoveredCancel {
			break
		}
	}
	if !hoveredToggle {
		t.Error("the net toggle button was never hoverable")
	}
	if !hoveredCancel {
		t.Error("the net cancel button was never hoverable")
	}
}

func TestMITMBadgeActiveAndStopped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active bool
		flows  int
		addr   string
	}{
		{"active", true, 42, "127.0.0.1:8080"},
		{"stopped", false, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rg := newRig(t, 1400)
			rg.b.MITMShow = true
			rg.b.MITMActive = tc.active
			rg.b.MITMFlows = tc.flows
			rg.b.MITMAddr = tc.addr
			if d := rg.frames(2); d.Size.X != 1400 {
				t.Fatalf("width = %d, want 1400", d.Size.X)
			}
		})
	}
}

func TestMITMBadgeClickInvokesToggle(t *testing.T) {
	rg := newRig(t, 1400)
	var calls int
	rg.b.MITMShow = true
	rg.b.MITMActive = true
	rg.b.MITMAddr = "127.0.0.1:8080"
	rg.b.OnMITMToggle = func() { calls++ }
	rg.frames(2)

	rg.b.BtnMITM.Click()
	rg.frame()
	if calls != 1 {
		t.Errorf("OnMITMToggle called %d times, want 1", calls)
	}
}

func TestMITMBadgeClickWithNilCallback(t *testing.T) {
	rg := newRig(t, 1400)
	rg.b.MITMShow = true
	rg.frames(2)
	rg.b.BtnMITM.Click()
	rg.frames(2)
}

func TestMITMBadgeTakesPrecedenceOverNetBadge(t *testing.T) {
	rg := newRig(t, 1400)
	rg.b.MITMShow = true
	rg.b.MITMActive = true
	rg.b.MITMAddr = "127.0.0.1:8080"
	rg.b.NetActive = true
	var mitm, net int
	rg.b.OnMITMToggle = func() { mitm++ }
	rg.b.OnNetToggle = func() { net++ }
	rg.frames(2)

	clicked := false
	for x := float32(300); x < 1100; x += 3 {
		rg.press(x, 15)
		rg.release(x, 15)
		if mitm > 0 {
			clicked = true
			break
		}
	}
	if !clicked {
		t.Error("the MITM badge was never clickable while shown")
	}
	if net != 0 {
		t.Errorf("the net badge must not be reachable while MITM is shown (%d clicks)", net)
	}
}

func TestBadgeSuppressedWhenMidRegionTooNarrow(t *testing.T) {
	rg := newRig(t, 400)
	rg.b.MITMShow = true
	rg.b.MITMActive = true
	rg.b.MITMAddr = "127.0.0.1:8080"
	if d := rg.frames(2); d.Size.X != 400 {
		t.Fatalf("width = %d, want 400", d.Size.X)
	}
}

func TestTitleDragMovesWindow(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)
	rg.press(6, 15)
	rg.drag(60, 15)
	rg.drag(120, 15)
	rg.release(120, 15)
	if d := rg.frames(2); d.Size.Y != 30 {
		t.Fatalf("height = %d, want 30", d.Size.Y)
	}
}

func TestTitleDoubleClickTogglesMaximize(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)

	rg.press(6, 15)
	rg.release(6, 15)
	if rg.b.Maximized {
		t.Fatal("a single press must not maximize")
	}
	if rg.b.lastClick.IsZero() {
		t.Fatal("the first press must record lastClick")
	}

	rg.press(6, 15)
	rg.release(6, 15)
	if !rg.b.Maximized {
		t.Fatal("a second press within the double-click window must maximize")
	}
	if !rg.b.lastClick.IsZero() {
		t.Error("a completed double click must reset lastClick")
	}

	rg.press(6, 15)
	rg.release(6, 15)
	rg.press(6, 15)
	rg.release(6, 15)
	if rg.b.Maximized {
		t.Error("a second double click must unmaximize")
	}
}

func TestTitleDoubleClickOutsideWindowIsSafe(t *testing.T) {
	rg := newRig(t, 900)
	rg.win = nil
	rg.frames(2)
	rg.press(6, 15)
	rg.release(6, 15)
	rg.press(6, 15)
	rg.release(6, 15)
	if rg.b.Maximized {
		t.Error("without a window the double click must not latch Maximized")
	}
}

func TestTitleSlowClicksDoNotMaximize(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)
	rg.press(6, 15)
	rg.release(6, 15)
	time.Sleep(320 * time.Millisecond)
	rg.press(6, 15)
	rg.release(6, 15)
	if rg.b.Maximized {
		t.Error("presses more than 300ms apart must not count as a double click")
	}
}

func TestSecondaryButtonPressIsIgnored(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)
	for range 2 {
		rg.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(6, 15), Buttons: pointer.ButtonSecondary, Source: pointer.Mouse})
		rg.frame()
		rg.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(6, 15), Source: pointer.Mouse})
		rg.frames(2)
	}
	if rg.b.Maximized {
		t.Error("right-clicking the title bar must not maximize")
	}
}

func TestWindowButtonHoverStates(t *testing.T) {
	rg := newRig(t, 900)
	rg.frames(2)

	btns := []struct {
		name  string
		check func() bool
	}{
		{"minimize", func() bool { return rg.b.BtnMinimize.Hovered() }},
		{"maximize", func() bool { return rg.b.BtnMaximize.Hovered() }},
		{"close", func() bool { return rg.b.BtnClose.Hovered() }},
		{"settings", func() bool { return rg.b.SettingsBtn.Hovered() }},
		{"bug", func() bool { return rg.b.BugReportBtn.Hovered() }},
	}
	seen := map[string]bool{}
	for x := float32(2); x < 900; x += 2 {
		rg.move(x, 15)
		for _, b := range btns {
			if b.check() {
				seen[b.name] = true
			}
		}
	}
	for _, b := range btns {
		if !seen[b.name] {
			t.Errorf("the %s button was never hoverable", b.name)
		}
	}
}

func TestMaximizedIconVariant(t *testing.T) {
	rg := newRig(t, 900)
	rg.b.Maximized = true
	if d := rg.frames(2); d.Size.X != 900 {
		t.Fatalf("width = %d, want 900", d.Size.X)
	}
	rg.b.Maximized = false
	rg.frames(2)
}

func TestNarrowBarHidesLeftRegion(t *testing.T) {
	for _, w := range []int{1, 20, 100, 138, 139, 240, 400} {
		rg := newRig(t, w)
		rg.b.MITMShow = true
		rg.b.NetActive = true
		if d := rg.frames(2); d.Size.X != w {
			t.Errorf("width %d: got %d", w, d.Size.X)
		}
	}
}

func TestDragZoneIgnoresNonPositiveWidth(t *testing.T) {
	rg := newRig(t, 900)
	gtx := rg.gtx()
	rg.b.dragZone(gtx, 0, 0, 30)
	rg.b.dragZone(gtx, 0, -10, 30)
	rg.b.dragZone(gtx, 0, 10, 30)
}

func TestItoa(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{{0, "0"}, {7, "7"}, {-3, "-3"}, {123456, "123456"}} {
		if got := itoa(tc.in); got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEmptyTitleFallsBackToTracto(t *testing.T) {
	rg := newRig(t, 900)
	rg.title = ""
	if d := rg.frames(2); d.Size.X != 900 {
		t.Fatalf("width = %d, want 900", d.Size.X)
	}
	if rg.title != "" {
		t.Error("Layout must not mutate the caller's title argument")
	}
}
