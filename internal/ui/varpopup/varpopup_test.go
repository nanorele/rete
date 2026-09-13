package varpopup

import (
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"rete/internal/model"
	"rete/internal/ui/environments"
	"testing"
	"time"
)

type rig struct {
	s    *State
	host *Host
	r    input.Router
	sz   image.Point
	now  time.Time

	dismissed int
	selected  []string
	refreshed int
	saved     int
}

func testTheme() *material.Theme {
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	return th
}

func newRig(t *testing.T, envs []*environments.EnvironmentUI, activeID string, sz image.Point) *rig {
	t.Helper()
	rg := &rig{s: &State{}, sz: sz, now: time.Unix(1700000000, 0)}
	envsCopy := envs
	id := activeID
	rg.host = &Host{
		Theme:        testTheme(),
		Window:       new(app.Window),
		Environments: &envsCopy,
		ActiveEnvID:  &id,
		ActiveEnvVar: func(name string) (string, bool) {
			for _, e := range envsCopy {
				if e.Data.ID != id {
					continue
				}
				for _, v := range e.Data.Vars {
					if v.Key == name {
						return v.Value, true
					}
				}
			}
			return "", false
		},
		OnDismiss: func() { rg.dismissed++ },
		OnSelectEnv: func(envID string) {
			rg.selected = append(rg.selected, envID)
			id = envID
		},
		RefreshActiveEnv: func() { rg.refreshed++ },
		SaveState:        func() { rg.saved++ },
	}
	return rg
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

func (rg *rig) frame() {
	rg.now = rg.now.Add(16 * time.Millisecond)
	gtx := rg.gtx()
	rg.s.Layout(gtx, rg.host)
	rg.r.Frame(gtx.Ops)
}

func (rg *rig) frames(n int) {
	for range n {
		rg.frame()
	}
}

func (rg *rig) press(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rg.frame()
}

func (rg *rig) release(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frames(2)
}

func (rg *rig) click(x, y float32) {
	rg.press(x, y)
	rg.release(x, y)
}

func (rg *rig) keyPress(name key.Name) {
	rg.r.Queue(key.Event{Name: name, State: key.Press})
	rg.frame()
}

func twoEnvs() []*environments.EnvironmentUI {
	return []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "dev-tok"}}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod", Vars: []model.EnvVar{{Key: "other", Value: "x"}}}},
	}
}

func openRig(t *testing.T, envs []*environments.EnvironmentUI, activeID string) *rig {
	t.Helper()
	rg := newRig(t, envs, activeID, image.Pt(900, 700))
	rg.host.ChipHovered = func() bool { return true }
	rg.s.OpenAt("token", "dev-tok", nil, struct{ Start, End int }{Start: 1, End: 5}, f32.Pt(100, 100), activeID)
	rg.frames(2)
	return rg
}

func TestEscapeDismisses(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.keyPress(key.NameEscape)
	if rg.s.Open {
		t.Error("Escape must close the popup")
	}
	if rg.dismissed == 0 {
		t.Error("Escape must invoke OnDismiss")
	}
}

func TestReturnAndEnterDismiss(t *testing.T) {
	for _, name := range []key.Name{key.NameReturn, key.NameEnter} {
		t.Run(string(name), func(t *testing.T) {
			rg := openRig(t, twoEnvs(), "e1")
			rg.s.EnvMenuOpen = true
			rg.frames(2)
			rg.keyPress(name)
			if rg.s.Open {
				t.Errorf("%q must close the popup", name)
			}
			if rg.s.EnvMenuOpen {
				t.Errorf("%q must close the env menu", name)
			}
			if rg.dismissed == 0 {
				t.Errorf("%q must invoke OnDismiss", name)
			}
		})
	}
}

func TestKeyReleaseIsIgnored(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.r.Queue(key.Event{Name: key.NameEscape, State: key.Release})
	rg.frames(2)
	if !rg.s.Open {
		t.Error("a key release must not close the popup")
	}
	if rg.dismissed != 0 {
		t.Errorf("OnDismiss called %d times on key release", rg.dismissed)
	}
}

func (rg *rig) moveTo(x, y float32) {
	rg.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rg.frame()
}

func TestHoverAwayCloses(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = nil
	rg.frames(25)
	if rg.s.Open {
		t.Error("the popup must close once the cursor stays away past the grace period")
	}
	if rg.dismissed != 0 {
		t.Errorf("closing with an unchanged value must not save, got %d OnDismiss calls", rg.dismissed)
	}
}

func TestHoverAwayClosesSavesChangedValue(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = nil
	rg.s.Editor.SetText("edited")
	rg.frames(25)
	if rg.s.Open {
		t.Error("the popup must close once the cursor stays away past the grace period")
	}
	if rg.dismissed == 0 {
		t.Error("a modified value must be saved when the popup closes on hover-away")
	}
}

func TestHoverInsideKeepsOpen(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = nil
	for i := 0; i < 30; i++ {
		rg.moveTo(150, 130)
	}
	if !rg.s.Open {
		t.Error("the popup must stay open while the cursor hovers it")
	}
	if rg.dismissed != 0 {
		t.Errorf("OnDismiss fired %d times while hovered", rg.dismissed)
	}
}

func TestValueFieldPressClosesEnvMenu(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frames(2)
	rg.click(150, 165)
	if rg.s.EnvMenuOpen {
		t.Error("a press on the value field must close the environment list")
	}
	if !rg.s.Open {
		t.Error("closing the environment list must not close the popup")
	}
}

func TestFocusedEditorKeepsOpen(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = nil

	gtx := rg.gtx()
	gtx.Execute(key.FocusCmd{Tag: &rg.s.Editor})
	rg.s.Layout(gtx, rg.host)
	rg.r.Frame(gtx.Ops)

	rg.frames(25)
	if !rg.s.Open {
		t.Fatal("the popup must stay open while the value editor is focused (e.g. selecting text)")
	}

	gtx = rg.gtx()
	gtx.Execute(key.FocusCmd{Tag: nil})
	rg.s.Layout(gtx, rg.host)
	rg.r.Frame(gtx.Ops)

	rg.frames(25)
	if rg.s.Open {
		t.Error("once focus leaves the editor and the cursor is away, the popup must close")
	}
}

func TestChipHoveredKeepsOpen(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = func() bool { return true }
	rg.frames(30)
	if !rg.s.Open {
		t.Error("the popup must stay open while the source chip is hovered")
	}
}

func TestPressInsideKeepsOpen(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.click(150, 130)
	if !rg.s.Open {
		t.Error("a press inside the popup must not close it")
	}
	if rg.dismissed != 0 {
		t.Errorf("OnDismiss fired %d times for an inside press", rg.dismissed)
	}
}

func TestDismissWithoutCallbacks(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.OnDismiss = nil
	rg.host.Window = nil
	rg.keyPress(key.NameEscape)
	if rg.s.Open {
		t.Error("Escape must close the popup even without OnDismiss")
	}
}

func TestEnvButtonTogglesMenu(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	if rg.s.EnvMenuOpen {
		t.Fatal("precondition: env menu starts closed")
	}
	rg.s.EnvBtn.Click()
	rg.frames(2)
	if !rg.s.EnvMenuOpen {
		t.Fatal("clicking the env button must open the menu")
	}
	rg.s.EnvBtn.Click()
	rg.frames(2)
	if rg.s.EnvMenuOpen {
		t.Error("clicking the env button again must close the menu")
	}
}

func TestEnvMenuGrowsPopupHeight(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.ChipHovered = nil
	rg.s.EnvMenuOpen = true
	rg.frames(2)
	for i := 0; i < 30; i++ {
		rg.moveTo(150, 290)
	}
	if !rg.s.Open {
		t.Error("y=290 is inside the grown popup; hovering there must keep it open")
	}

	rg.s.EnvMenuOpen = false
	for i := 0; i < 30; i++ {
		rg.moveTo(150, 290)
	}
	if rg.s.Open {
		t.Error("with the env menu closed y=290 is below the auto-sized popup; it must close after the grace period")
	}
}

func TestEnvRowClickSelectsEnvironment(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frames(2)
	if len(rg.s.EnvClicks) < 3 {
		t.Fatalf("EnvClicks len=%d, want >=3", len(rg.s.EnvClicks))
	}

	rg.s.EnvClicks[2].Click()
	rg.frames(2)

	if len(rg.selected) == 0 {
		t.Fatal("clicking an environment row never invoked OnSelectEnv")
	}
	if got := rg.selected[len(rg.selected)-1]; got != "e2" {
		t.Errorf("OnSelectEnv(%q), want \"e2\"", got)
	}
	if rg.s.EnvID != "e2" {
		t.Errorf("EnvID=%q, want \"e2\"", rg.s.EnvID)
	}
	if rg.s.EnvMenuOpen {
		t.Error("selecting an environment must close the env menu")
	}
	if rg.refreshed == 0 {
		t.Error("RefreshActiveEnv was never invoked")
	}
	if rg.saved == 0 {
		t.Error("SaveState was never invoked")
	}
}

func TestEnvRowClickNoEnvironmentEntry(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frames(2)

	rg.s.EnvClicks[0].Click()
	rg.frames(2)

	if len(rg.selected) == 0 || rg.selected[len(rg.selected)-1] != "" {
		t.Errorf("row 0 must select the empty environment, got %v", rg.selected)
	}
	if rg.s.EnvID != "" {
		t.Errorf("EnvID=%q, want empty", rg.s.EnvID)
	}
	if rg.s.Editor.Text() != "" {
		t.Errorf("Editor should be cleared for the empty environment, got %q", rg.s.Editor.Text())
	}
}

func TestEnvRowClickCopiesValueIntoEditor(t *testing.T) {
	rg := openRig(t, twoEnvs(), "")
	rg.s.Editor.SetText("stale")
	rg.s.EnvMenuOpen = true
	rg.frames(2)

	rg.s.EnvClicks[1].Click()
	rg.frames(2)

	if rg.s.Editor.Text() != "dev-tok" {
		t.Errorf("Editor=%q, want %q from the newly selected environment", rg.s.Editor.Text(), "dev-tok")
	}
}

func TestEnvRowClickWithoutCallbacks(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.host.OnSelectEnv = nil
	rg.host.RefreshActiveEnv = nil
	rg.host.ActiveEnvVar = nil
	rg.host.SaveState = nil
	rg.host.Window = nil
	rg.s.EnvMenuOpen = true
	rg.frames(2)

	rg.s.EnvClicks[1].Click()
	rg.frames(2)

	if rg.s.EnvID != "e1" {
		t.Errorf("EnvID=%q, want \"e1\"", rg.s.EnvID)
	}
	if rg.s.Editor.Text() != "" {
		t.Errorf("without ActiveEnvVar the editor must be cleared, got %q", rg.s.Editor.Text())
	}
	if rg.s.EnvMenuOpen {
		t.Error("selecting must close the env menu")
	}
}

func TestSubmitClosesPopup(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frames(2)

	gtx := rg.gtx()
	gtx.Execute(key.FocusCmd{Tag: &rg.s.Editor})
	rg.s.Layout(gtx, rg.host)
	rg.r.Frame(gtx.Ops)

	rg.r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	rg.frame()

	if rg.s.Open {
		t.Error("submitting the editor must close the popup")
	}
	if rg.dismissed == 0 {
		t.Error("submit must invoke OnDismiss")
	}
}

func TestEnvClicksSliceGrowsWithEnvironments(t *testing.T) {
	envs := twoEnvs()
	rg := newRig(t, envs, "e1", image.Pt(900, 700))
	rg.s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(10, 10), "e1")
	rg.s.EnvMenuOpen = true
	rg.frames(2)
	if len(rg.s.EnvClicks) != 3 {
		t.Fatalf("EnvClicks len=%d, want 3", len(rg.s.EnvClicks))
	}

	envs = append(envs, &environments.EnvironmentUI{Data: &model.ParsedEnvironment{ID: "e3", Name: "QA"}})
	*rg.host.Environments = envs
	rg.frames(2)
	if len(rg.s.EnvClicks) < 4 {
		t.Errorf("EnvClicks len=%d after adding an environment, want >=4", len(rg.s.EnvClicks))
	}
}

func TestPopupClampsWithinViewport(t *testing.T) {
	cases := []struct {
		name string
		pos  f32.Point
		sz   image.Point
	}{
		{"bottom-right", f32.Pt(880, 690), image.Pt(900, 700)},
		{"top-left-negative", f32.Pt(-500, -500), image.Pt(900, 700)},
		{"viewport-smaller-than-popup", f32.Pt(10, 10), image.Pt(200, 100)},
		{"exact-fit", f32.Pt(0, 0), image.Pt(360, 184)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rg := newRig(t, twoEnvs(), "e1", tc.sz)
			rg.s.OpenAt("token", "v", nil, struct{ Start, End int }{}, tc.pos, "e1")
			rg.frames(2)
			if !rg.s.Open {
				t.Error("popup closed itself during layout")
			}
			if rg.s.Pos != tc.pos {
				t.Errorf("Layout mutated Pos: %v -> %v", tc.pos, rg.s.Pos)
			}
		})
	}
}

func TestLayoutWithNoEnvironments(t *testing.T) {
	rg := newRig(t, nil, "", image.Pt(900, 700))
	rg.s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(50, 50), "")
	rg.s.EnvMenuOpen = true
	rg.frames(2)
	if len(rg.s.EnvClicks) != 1 {
		t.Errorf("EnvClicks len=%d, want 1 for the no-environment entry", len(rg.s.EnvClicks))
	}
	rg.s.EnvClicks[0].Click()
	rg.frames(2)
	if len(rg.selected) == 0 {
		t.Error("the no-environment row must still be clickable")
	}
}

func TestClosedPopupIgnoresEvents(t *testing.T) {
	rg := newRig(t, twoEnvs(), "e1", image.Pt(900, 700))
	rg.frames(2)
	rg.keyPress(key.NameEscape)
	rg.click(400, 400)
	if rg.dismissed != 0 {
		t.Errorf("a closed popup must not dismiss, got %d calls", rg.dismissed)
	}
}

func manyEnvs(n int) []*environments.EnvironmentUI {
	out := make([]*environments.EnvironmentUI, 0, n)
	for i := 0; i < n; i++ {
		id := string(rune('a'+i%26)) + string(rune('0'+i/26))
		out = append(out, &environments.EnvironmentUI{
			Data: &model.ParsedEnvironment{
				ID:   id,
				Name: "env-" + id,
				Vars: []model.EnvVar{{Key: "token", Value: "tok-" + id}},
			},
		})
	}
	return out
}

func TestEnvClickDrainsForRowOutsideViewport(t *testing.T) {
	envs := manyEnvs(60)
	rg := openRig(t, envs, envs[0].Data.ID)
	rg.s.EnvMenuOpen = true
	rg.frame()

	last := len(envs)
	if len(rg.s.EnvClicks) <= last {
		t.Fatalf("len(EnvClicks) = %d, want > %d", len(rg.s.EnvClicks), last)
	}

	rg.s.EnvClicks[last].Click()
	rg.frame()

	wantID := envs[last-1].Data.ID
	if len(rg.selected) == 0 {
		t.Fatal("a click on a row scrolled out of the viewport was dropped; poll clicks before List.Layout")
	}
	if got := rg.selected[len(rg.selected)-1]; got != wantID {
		t.Errorf("selected env = %q, want %q", got, wantID)
	}
	if rg.s.EnvMenuOpen {
		t.Error("selecting an environment must close the menu")
	}
}

func TestEnvClickNoEnvironmentRow(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = true
	rg.frame()

	rg.s.EnvClicks[0].Click()
	rg.frame()

	if len(rg.selected) == 0 {
		t.Fatal("the (no environment) row click was dropped")
	}
	if got := rg.selected[len(rg.selected)-1]; got != "" {
		t.Errorf("selected env = %q, want the empty id", got)
	}
	if rg.s.EnvID != "" {
		t.Errorf("EnvID = %q, want empty", rg.s.EnvID)
	}
}

func TestEnvClickWhileMenuClosedIsDiscarded(t *testing.T) {
	rg := openRig(t, twoEnvs(), "e1")
	rg.s.EnvMenuOpen = false
	rg.frame()

	rg.s.EnvClicks[1].Click()
	rg.frame()

	if len(rg.selected) != 0 {
		t.Errorf("a click while the menu is closed must not select an environment, got %v", rg.selected)
	}

	rg.s.EnvMenuOpen = true
	rg.frame()
	if len(rg.selected) != 0 {
		t.Errorf("a stranded click fired late when the menu reopened, got %v", rg.selected)
	}
}

func newGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
		Now:         time.Now(),
	}
}

func makeHost(envs []*environments.EnvironmentUI, activeID string) *Host {
	envsCopy := envs
	id := activeID
	return &Host{
		Theme:        material.NewTheme(),
		Environments: &envsCopy,
		ActiveEnvID:  &id,
		ActiveEnvVar: func(name string) (string, bool) {
			for _, e := range envsCopy {
				if e.Data.ID == id {
					for _, v := range e.Data.Vars {
						if v.Key == name {
							return v.Value, true
						}
					}
				}
			}
			return "", false
		},
	}
}

func TestOpenAtSetsState(t *testing.T) {
	var s State
	rng := struct{ Start, End int }{Start: 2, End: 7}
	srcEd := struct{}{}
	pos := f32.Pt(10, 20)
	s.OpenAt("token", "secret", srcEd, rng, pos, "env-1")
	if !s.Open {
		t.Fatalf("Open should be true after OpenAt")
	}
	if s.Name != "token" {
		t.Errorf("Name=%q, want 'token'", s.Name)
	}
	if s.Editor.Text() != "secret" {
		t.Errorf("Editor.Text()=%q, want 'secret'", s.Editor.Text())
	}
	if s.EnvID != "env-1" {
		t.Errorf("EnvID=%q, want 'env-1'", s.EnvID)
	}
	if s.Range.Start != 2 || s.Range.End != 7 {
		t.Errorf("Range=%+v, want {2,7}", s.Range)
	}
	if s.Pos.X != 10 || s.Pos.Y != 20 {
		t.Errorf("Pos=%+v, want (10,20)", s.Pos)
	}
	if s.SrcEditor == nil {
		t.Errorf("SrcEditor should be set")
	}
	if s.EnvMenuOpen {
		t.Errorf("EnvMenuOpen should be reset to false")
	}
}

func TestOpenAtResetsEnvMenuOpen(t *testing.T) {
	var s State
	s.EnvMenuOpen = true
	s.OpenAt("v", "val", nil, struct{ Start, End int }{}, f32.Pt(0, 0), "")
	if s.EnvMenuOpen {
		t.Errorf("OpenAt should reset EnvMenuOpen to false even when already open")
	}
}

func TestCloseResetsFlags(t *testing.T) {
	var s State
	s.Open = true
	s.EnvMenuOpen = true
	s.Close()
	if s.Open {
		t.Errorf("Open should be false after Close")
	}
	if s.EnvMenuOpen {
		t.Errorf("EnvMenuOpen should be false after Close")
	}
}

func TestCloseOnNonOpenStateIsNoop(t *testing.T) {
	var s State
	s.Name = "preserved"
	s.Editor.SetText("preserved-value")
	s.EnvID = "env-x"
	s.Close()
	if s.Open || s.EnvMenuOpen {
		t.Errorf("Open/EnvMenuOpen should remain false")
	}

	if s.Name != "preserved" {
		t.Errorf("Close should not wipe Name; got %q", s.Name)
	}
	if s.Editor.Text() != "preserved-value" {
		t.Errorf("Close should not wipe Editor; got %q", s.Editor.Text())
	}
	if s.EnvID != "env-x" {
		t.Errorf("Close should not wipe EnvID; got %q", s.EnvID)
	}
}

func TestLayoutNilStateReturns(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Layout on nil *State should not panic; got %v", r)
		}
	}()
	var s *State
	gtx := newGtx(800, 600)
	s.Layout(gtx, nil)
}

func TestLayoutClosedReturns(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Layout on closed State should not panic; got %v", r)
		}
	}()
	var s State
	gtx := newGtx(800, 600)

	s.Layout(gtx, nil)
}

func TestLayoutOpenBasic(t *testing.T) {
	var s State
	s.OpenAt("token", "abc", nil, struct{ Start, End int }{}, f32.Pt(50, 50), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutOpenWithEnvironments(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "dev-tok"}}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod", Vars: []model.EnvVar{{Key: "token", Value: "prod-tok"}}}},
	}
	var s State
	s.OpenAt("token", "dev-tok", nil, struct{ Start, End int }{}, f32.Pt(100, 100), "e1")
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutEnvMenuOpen(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "dev-tok"}}}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod"}},
	}
	var s State
	s.OpenAt("token", "dev-tok", nil, struct{ Start, End int }{}, f32.Pt(100, 100), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutPositionClampingRight(t *testing.T) {

	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(700, 100), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)

	if s.Pos.X != 700 {
		t.Errorf("Pos.X mutated by Layout: %v", s.Pos.X)
	}
}

func TestLayoutPositionClampingBottom(t *testing.T) {

	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(10, 580), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutNegativePosition(t *testing.T) {
	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(-50, -50), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutTinyConstraintsForcesPyZero(t *testing.T) {

	var s State
	s.OpenAt("v", "value", nil, struct{ Start, End int }{}, f32.Pt(10, 10), "")
	host := makeHost(nil, "")
	gtx := newGtx(400, 100)
	s.Layout(gtx, host)
}

func TestLayoutEnvMenuOpenWithEmptyActive(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "token", Value: "x"}}}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	s.EnvMenuOpen = true
	host := makeHost(envs, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutEnvClicksAllocation(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev"}},
		{Data: &model.ParsedEnvironment{ID: "e2", Name: "Prod"}},
		{Data: &model.ParsedEnvironment{ID: "e3", Name: "QA"}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e2")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)

	if len(s.EnvClicks) < 4 {
		t.Errorf("EnvClicks len=%d, want >=4", len(s.EnvClicks))
	}
}

func TestLayoutEnvAxisIsVerticalAfterLayout(t *testing.T) {
	var s State
	s.OpenAt("v", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "")
	host := makeHost(nil, "")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
	if s.EnvList.Axis != layout.Vertical {
		t.Errorf("EnvList.Axis=%v, want Vertical", s.EnvList.Axis)
	}
}

func TestLayoutWithVarNotInEnv(t *testing.T) {
	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{{Key: "other", Value: "x"}}}},
	}
	var s State
	s.OpenAt("missing", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}

func TestLayoutWithEmptyValueVar(t *testing.T) {

	envs := []*environments.EnvironmentUI{
		{Data: &model.ParsedEnvironment{ID: "e1", Name: "Dev", Vars: []model.EnvVar{
			{Key: "token", Value: ""},
			{Key: "token", Value: "second"},
		}}},
	}
	var s State
	s.OpenAt("token", "", nil, struct{ Start, End int }{}, f32.Pt(20, 20), "e1")
	s.EnvMenuOpen = true
	host := makeHost(envs, "e1")
	gtx := newGtx(800, 600)
	s.Layout(gtx, host)
}
