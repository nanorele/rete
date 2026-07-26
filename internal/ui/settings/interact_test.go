package settings

import (
	"image"
	"strings"
	"testing"
	"time"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

type editorRig struct {
	e      *Editor
	host   *Host
	cur    *model.AppSettings
	r      input.Router
	sz     image.Point
	now    time.Time
	saves  int
	closes int
}

func newEditorRig(t *testing.T, sz image.Point) *editorRig {
	t.Helper()
	resetHTTPClient(t)
	dir := t.TempDir()
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	cur := model.DefaultSettings()
	open := true
	rig := &editorRig{cur: &cur, sz: sz, now: time.Unix(1700000000, 0)}
	rig.host = &Host{
		Theme:   material.NewTheme(),
		Current: &cur,
		Open:    &open,
		OnClose: func() { rig.closes++ },
		OnSave:  func() { rig.saves++ },
	}
	rig.e = NewEditor(cur)
	return rig
}

func (rig *editorRig) gtx() layout.Context {
	rig.now = rig.now.Add(16 * time.Millisecond)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
}

func (rig *editorRig) frame() layout.Dimensions {
	gtx := rig.gtx()
	d := rig.e.Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return d
}

func (rig *editorRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *editorRig) clickN(b *widget.Clickable, n int) {
	for i := 0; i < n; i++ {
		b.Click()
		rig.frame()
	}
}

func TestEditorRig_EveryCategoryRendersFully(t *testing.T) {
	for cat := range settingsCategories {
		name := settingsCategories[cat]
		t.Run(name, func(t *testing.T) {
			rig := newEditorRig(t, image.Pt(1000, 6000))
			rig.e.Category = cat
			d := rig.frames(2)
			if d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Fatalf("category %s produced no dimensions: %+v", name, d.Size)
			}
		})
	}
}

func TestEditorRig_AppearanceExpandedRendersAllColorRows(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.e.SyntaxColorsExpanded = true
	rig.e.NewThemeDialogOpen = true
	rig.e.NewThemeBaseID = theme.Registry[0].ID
	rig.e.Draft.CustomThemes = []model.CustomTheme{{ID: "custom-x", Name: "Mine", BasedOn: "dark"}}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("expanded appearance produced no dimensions")
	}
}

func TestEditorRig_NarrowViewportSingleColumnGrids(t *testing.T) {
	rig := newEditorRig(t, image.Pt(120, 4000))
	rig.e.Category = 0
	rig.e.NewThemeDialogOpen = true
	rig.frames(2)
	rig.e.Category = 2
	if d := rig.frames(2); d.Size.X <= 0 {
		t.Fatal("narrow viewport produced no dimensions")
	}
}

func TestEditorRig_BackButtonClosesWithoutRendering(t *testing.T) {
	rig := newEditorRig(t, image.Pt(800, 600))
	rig.frame()
	rig.e.BackBtn.Click()
	rig.frame()
	if rig.closes != 1 {
		t.Fatalf("OnClose calls = %d, want 1", rig.closes)
	}
}

func TestEditorRig_CategoryButtonsSwitchAndResetScroll(t *testing.T) {
	rig := newEditorRig(t, image.Pt(800, 400))
	rig.frame()
	rig.e.ContentList.Position.First = 5
	rig.clickN(&rig.e.CategoryBtn[2], 1)
	if rig.e.Category != 2 {
		t.Fatalf("Category = %d, want 2", rig.e.Category)
	}
	if rig.e.ContentList.Position.First != 0 {
		t.Errorf("switching category must reset scroll, got First=%d", rig.e.ContentList.Position.First)
	}
	rig.e.ContentList.Position.First = 7
	rig.clickN(&rig.e.CategoryBtn[2], 1)
	if rig.e.ContentList.Position.First != 7 {
		t.Errorf("re-clicking the active category must not reset scroll, got First=%d", rig.e.ContentList.Position.First)
	}
}

func TestEditorRig_ResetButtonRestoresDefaults(t *testing.T) {
	rig := newEditorRig(t, image.Pt(800, 600))
	rig.e.Draft.UITextSize = 27
	rig.e.Draft.Theme = "light"
	rig.frame()
	rig.clickN(&rig.e.ResetBtn, 1)
	def := model.DefaultSettings()
	if rig.e.Draft.UITextSize != def.UITextSize || rig.e.Draft.Theme != def.Theme {
		t.Fatalf("Reset did not restore defaults: %+v", rig.e.Draft)
	}
	if rig.saves == 0 {
		t.Error("Reset should trigger OnSave")
	}
}

func TestEditorRig_ThemeButtonsSelectAndDedupe(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 700))
	rig.frame()
	for i := range theme.Registry {
		rig.clickN(&rig.e.ThemeBtns[i], 1)
		if rig.e.Draft.Theme != theme.Registry[i].ID {
			t.Fatalf("theme btn %d: Draft.Theme = %q, want %q", i, rig.e.Draft.Theme, theme.Registry[i].ID)
		}
	}
	saves := rig.saves
	rig.clickN(&rig.e.ThemeBtns[len(theme.Registry)-1], 1)
	if rig.saves != saves {
		t.Error("re-selecting the active theme must not mark the draft changed")
	}
}

func TestEditorRig_NewThemeDialogCreateCancel(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.frame()

	rig.clickN(&rig.e.NewThemeBtn, 1)
	if !rig.e.NewThemeDialogOpen {
		t.Fatal("NewThemeBtn should open the dialog")
	}
	rig.clickN(&rig.e.NewThemeCancelBtn, 1)
	if rig.e.NewThemeDialogOpen {
		t.Fatal("Cancel should close the dialog")
	}

	rig.clickN(&rig.e.NewThemeBtn, 1)
	rig.clickN(&rig.e.NewThemeCreateBtn, 1)
	if len(rig.e.Draft.CustomThemes) != 0 {
		t.Fatal("Create with an empty name must not add a theme")
	}
	if !rig.e.NewThemeDialogOpen {
		t.Error("Create with an empty name should leave the dialog open")
	}

	if len(rig.e.NewThemeBaseBtns) < 2 {
		t.Fatalf("NewThemeBaseBtns not sized: %d", len(rig.e.NewThemeBaseBtns))
	}
	rig.clickN(&rig.e.NewThemeBaseBtns[1], 1)
	if rig.e.NewThemeBaseID != theme.Registry[1].ID {
		t.Fatalf("NewThemeBaseID = %q, want %q", rig.e.NewThemeBaseID, theme.Registry[1].ID)
	}
	rig.e.NewThemeNameEditor.SetText("Ocean")
	rig.clickN(&rig.e.NewThemeCreateBtn, 1)
	if len(rig.e.Draft.CustomThemes) != 1 {
		t.Fatalf("CustomThemes = %d, want 1", len(rig.e.Draft.CustomThemes))
	}
	ct := rig.e.Draft.CustomThemes[0]
	if ct.Name != "Ocean" || ct.BasedOn != theme.Registry[1].ID {
		t.Errorf("custom theme = %+v", ct)
	}
	if !strings.HasPrefix(ct.ID, "custom-") {
		t.Errorf("custom theme ID = %q, want custom- prefix", ct.ID)
	}
	if rig.e.Draft.Theme != ct.ID {
		t.Errorf("creating a theme should activate it, got %q", rig.e.Draft.Theme)
	}
	if rig.e.NewThemeDialogOpen {
		t.Error("Create should close the dialog")
	}
}

func TestEditorRig_CreateThemeDefaultsBaseToDark(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.frame()
	rig.e.NewThemeDialogOpen = true
	rig.e.NewThemeBaseID = ""
	rig.e.NewThemeNameEditor.SetText("Fallback")
	rig.clickN(&rig.e.NewThemeCreateBtn, 1)
	if len(rig.e.Draft.CustomThemes) != 1 {
		t.Fatalf("CustomThemes = %d, want 1", len(rig.e.Draft.CustomThemes))
	}
	if got := rig.e.Draft.CustomThemes[0].BasedOn; got != "dark" {
		t.Errorf("empty base should fall back to dark, got %q", got)
	}
}

func TestEditorRig_CustomThemeSelectAndDelete(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Draft.CustomThemes = []model.CustomTheme{
		{ID: "custom-a", Name: "A", BasedOn: "dark"},
		{ID: "custom-b", Name: "B", BasedOn: "light"},
	}
	rig.frames(2)
	if len(rig.e.CustomThemeBtns) != 2 {
		t.Fatalf("CustomThemeBtns = %d, want 2", len(rig.e.CustomThemeBtns))
	}
	rig.clickN(&rig.e.CustomThemeBtns[1], 1)
	if rig.e.Draft.Theme != "custom-b" {
		t.Fatalf("Draft.Theme = %q, want custom-b", rig.e.Draft.Theme)
	}
	rig.clickN(&rig.e.CustomThemeDelBtns[1], 1)
	if len(rig.e.Draft.CustomThemes) != 1 || rig.e.Draft.CustomThemes[0].ID != "custom-a" {
		t.Fatalf("after delete: %+v", rig.e.Draft.CustomThemes)
	}
	if rig.e.Draft.Theme != "dark" {
		t.Errorf("deleting the active custom theme should fall back to dark, got %q", rig.e.Draft.Theme)
	}
}

func TestEditorRig_DeleteInactiveCustomThemeKeepsSelection(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Draft.CustomThemes = []model.CustomTheme{
		{ID: "custom-a", Name: "A", BasedOn: "dark"},
		{ID: "custom-b", Name: "B", BasedOn: "dark"},
	}
	rig.e.Draft.Theme = "custom-b"
	rig.frames(2)
	rig.clickN(&rig.e.CustomThemeDelBtns[0], 1)
	if rig.e.Draft.Theme != "custom-b" {
		t.Errorf("deleting another theme must not change selection, got %q", rig.e.Draft.Theme)
	}
	if len(rig.e.Draft.CustomThemes) != 1 || rig.e.Draft.CustomThemes[0].ID != "custom-b" {
		t.Errorf("after delete: %+v", rig.e.Draft.CustomThemes)
	}
}

func TestEditorRig_MethodButtons(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Category = 2
	rig.frames(2)
	for i, m := range Methods {
		rig.clickN(&rig.e.DefaultMethodBtn[i], 1)
		if rig.e.Draft.DefaultMethod != m {
			t.Fatalf("method btn %d: got %q, want %q", i, rig.e.Draft.DefaultMethod, m)
		}
	}
}

func TestEditorRig_AcceptEncodingButtons(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Category = 2
	rig.frames(2)
	for i, opt := range acceptEncodingOptions {
		rig.clickN(&rig.e.AcceptEncodingBtn[i], 1)
		want := Sanitize(model.AppSettings{DefaultAcceptEncoding: opt.Value}).DefaultAcceptEncoding
		if rig.e.Draft.DefaultAcceptEncoding != want {
			t.Fatalf("accept-encoding btn %d (%q): draft = %q, want %q",
				i, opt.Value, rig.e.Draft.DefaultAcceptEncoding, want)
		}
	}
}

func TestEditorRig_IntSteppers(t *testing.T) {
	cases := []struct {
		name     string
		dec, inc func(*Editor) *widget.Clickable
		get      func(*Editor) int
		set      func(*Editor, int)
		start    int
		lo, hi   int
		step     int
	}{
		{"UITextSize",
			func(e *Editor) *widget.Clickable { return &e.UISizeDec },
			func(e *Editor) *widget.Clickable { return &e.UISizeInc },
			func(e *Editor) int { return e.Draft.UITextSize },
			func(e *Editor, v int) { e.Draft.UITextSize = v }, 14, 10, 28, 1},
		{"BodyTextSize",
			func(e *Editor) *widget.Clickable { return &e.BodySizeDec },
			func(e *Editor) *widget.Clickable { return &e.BodySizeInc },
			func(e *Editor) int { return e.Draft.BodyTextSize },
			func(e *Editor, v int) { e.Draft.BodyTextSize = v }, 13, 10, 28, 1},
		{"ResponseBodyPadding",
			func(e *Editor) *widget.Clickable { return &e.BodyPaddingDec },
			func(e *Editor) *widget.Clickable { return &e.BodyPaddingInc },
			func(e *Editor) int { return e.Draft.ResponseBodyPadding },
			func(e *Editor, v int) { e.Draft.ResponseBodyPadding = v }, 4, 0, 32, 1},
		{"ConnectTimeoutSec",
			func(e *Editor) *widget.Clickable { return &e.ConnectTimeoutDec },
			func(e *Editor) *widget.Clickable { return &e.ConnectTimeoutInc },
			func(e *Editor) int { return e.Draft.ConnectTimeoutSec },
			func(e *Editor, v int) { e.Draft.ConnectTimeoutSec = v }, 5, 0, 600, 1},
		{"TLSHandshakeTimeoutSec",
			func(e *Editor) *widget.Clickable { return &e.TLSTimeoutDec },
			func(e *Editor) *widget.Clickable { return &e.TLSTimeoutInc },
			func(e *Editor) int { return e.Draft.TLSHandshakeTimeoutSec },
			func(e *Editor, v int) { e.Draft.TLSHandshakeTimeoutSec = v }, 5, 0, 600, 1},
		{"MaxRedirects",
			func(e *Editor) *widget.Clickable { return &e.MaxRedirectsDec },
			func(e *Editor) *widget.Clickable { return &e.MaxRedirectsInc },
			func(e *Editor) int { return e.Draft.MaxRedirects },
			func(e *Editor, v int) { e.Draft.MaxRedirects = v }, 10, 0, 50, 1},
		{"JSONIndentSpaces",
			func(e *Editor) *widget.Clickable { return &e.JSONIndentDec },
			func(e *Editor) *widget.Clickable { return &e.JSONIndentInc },
			func(e *Editor) int { return e.Draft.JSONIndentSpaces },
			func(e *Editor, v int) { e.Draft.JSONIndentSpaces = v }, 2, 0, 8, 1},
		{"PreviewMaxMB",
			func(e *Editor) *widget.Clickable { return &e.PreviewMaxDec },
			func(e *Editor) *widget.Clickable { return &e.PreviewMaxInc },
			func(e *Editor) int { return e.Draft.PreviewMaxMB },
			func(e *Editor, v int) { e.Draft.PreviewMaxMB = v }, 100, 1, 500, 1},
		{"SyntaxHighlightMaxMB",
			func(e *Editor) *widget.Clickable { return &e.SyntaxHLMaxDec },
			func(e *Editor) *widget.Clickable { return &e.SyntaxHLMaxInc },
			func(e *Editor) int { return e.Draft.SyntaxHighlightMaxMB },
			func(e *Editor, v int) { e.Draft.SyntaxHighlightMaxMB = v }, 100, 1, 500, 1},
		{"StickyMaxLines",
			func(e *Editor) *widget.Clickable { return &e.StickyMaxDec },
			func(e *Editor) *widget.Clickable { return &e.StickyMaxInc },
			func(e *Editor) int { return e.Draft.StickyMaxLines },
			func(e *Editor, v int) { e.Draft.StickyMaxLines = v }, 5, 1, 12, 1},
		{"MaxTabRows",
			func(e *Editor) *widget.Clickable { return &e.MaxTabRowsDec },
			func(e *Editor) *widget.Clickable { return &e.MaxTabRowsInc },
			func(e *Editor) int { return e.Draft.MaxTabRows },
			func(e *Editor, v int) { e.Draft.MaxTabRows = v }, 3, 1, 10, 1},
		{"DefaultSidebarWidthPx",
			func(e *Editor) *widget.Clickable { return &e.SidebarWidthDec },
			func(e *Editor) *widget.Clickable { return &e.SidebarWidthInc },
			func(e *Editor) int { return e.Draft.DefaultSidebarWidthPx },
			func(e *Editor, v int) { e.Draft.DefaultSidebarWidthPx = v }, 300, 160, 1000, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newEditorRig(t, image.Pt(900, 4000))
			tc.set(rig.e, tc.start)
			rig.frame()

			rig.clickN(tc.inc(rig.e), 1)
			if got := tc.get(rig.e); got != tc.start+tc.step {
				t.Fatalf("inc: got %d, want %d", got, tc.start+tc.step)
			}
			rig.clickN(tc.dec(rig.e), 1)
			if got := tc.get(rig.e); got != tc.start {
				t.Fatalf("dec: got %d, want %d", got, tc.start)
			}

			tc.set(rig.e, tc.hi)
			rig.frame()
			rig.clickN(tc.inc(rig.e), 2)
			if got := tc.get(rig.e); got != tc.hi {
				t.Errorf("inc past max: got %d, want clamp at %d", got, tc.hi)
			}

			tc.set(rig.e, tc.lo)
			rig.frame()
			rig.clickN(tc.dec(rig.e), 2)
			if got := tc.get(rig.e); got != tc.lo {
				t.Errorf("dec past min: got %d, want clamp at %d", got, tc.lo)
			}
		})
	}
}

func TestEditorRig_TimeoutStepperUsesVariableStep(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 4000))
	rig.e.Draft.RequestTimeoutSec = 5
	rig.frame()
	rig.clickN(&rig.e.TimeoutInc, 1)
	if rig.e.Draft.RequestTimeoutSec != 6 {
		t.Fatalf("5+step(1) = %d, want 6", rig.e.Draft.RequestTimeoutSec)
	}
	rig.e.Draft.RequestTimeoutSec = 60
	rig.frame()
	rig.clickN(&rig.e.TimeoutInc, 1)
	if rig.e.Draft.RequestTimeoutSec != 90 {
		t.Fatalf("60+step(30) = %d, want 90", rig.e.Draft.RequestTimeoutSec)
	}
	rig.e.Draft.RequestTimeoutSec = 3590
	rig.frame()
	rig.clickN(&rig.e.TimeoutInc, 1)
	if rig.e.Draft.RequestTimeoutSec != 3600 {
		t.Fatalf("clamp at max: got %d, want 3600", rig.e.Draft.RequestTimeoutSec)
	}
	rig.clickN(&rig.e.TimeoutInc, 1)
	if rig.e.Draft.RequestTimeoutSec != 3600 {
		t.Fatalf("inc at max: got %d, want 3600", rig.e.Draft.RequestTimeoutSec)
	}
	rig.e.Draft.RequestTimeoutSec = 3
	rig.frame()
	rig.clickN(&rig.e.TimeoutDec, 5)
	if rig.e.Draft.RequestTimeoutSec != 0 {
		t.Fatalf("dec to floor: got %d, want 0", rig.e.Draft.RequestTimeoutSec)
	}
}

func TestEditorRig_IdleTimeoutStepper(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 4000))
	rig.e.Draft.IdleConnTimeoutSec = 300
	rig.frame()
	rig.clickN(&rig.e.IdleTimeoutInc, 1)
	if rig.e.Draft.IdleConnTimeoutSec != 360 {
		t.Fatalf("300+step(60) = %d, want 360", rig.e.Draft.IdleConnTimeoutSec)
	}
	rig.clickN(&rig.e.IdleTimeoutDec, 1)
	if rig.e.Draft.IdleConnTimeoutSec != 300 {
		t.Fatalf("360-step(60) = %d, want 300", rig.e.Draft.IdleConnTimeoutSec)
	}
	rig.e.Draft.IdleConnTimeoutSec = 3590
	rig.frame()
	rig.clickN(&rig.e.IdleTimeoutInc, 1)
	if rig.e.Draft.IdleConnTimeoutSec != 3600 {
		t.Fatalf("clamp: got %d, want 3600", rig.e.Draft.IdleConnTimeoutSec)
	}
	rig.e.Draft.IdleConnTimeoutSec = 0
	rig.frame()
	rig.clickN(&rig.e.IdleTimeoutDec, 1)
	if rig.e.Draft.IdleConnTimeoutSec != 0 {
		t.Fatalf("dec at floor: got %d, want 0", rig.e.Draft.IdleConnTimeoutSec)
	}
}

func TestEditorRig_MaxConnsStepper(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 4000))
	rig.e.Draft.MaxConnsPerHost = 100
	rig.frame()
	rig.clickN(&rig.e.MaxConnsInc, 1)
	if rig.e.Draft.MaxConnsPerHost != 150 {
		t.Fatalf("100+step(50) = %d, want 150", rig.e.Draft.MaxConnsPerHost)
	}
	rig.e.Draft.MaxConnsPerHost = 9950
	rig.frame()
	rig.clickN(&rig.e.MaxConnsInc, 1)
	if rig.e.Draft.MaxConnsPerHost != 10000 {
		t.Fatalf("clamp: got %d, want 10000", rig.e.Draft.MaxConnsPerHost)
	}
	rig.clickN(&rig.e.MaxConnsInc, 1)
	if rig.e.Draft.MaxConnsPerHost != 10000 {
		t.Fatalf("inc at max: got %d", rig.e.Draft.MaxConnsPerHost)
	}
	rig.e.Draft.MaxConnsPerHost = 0
	rig.frame()
	rig.clickN(&rig.e.MaxConnsDec, 1)
	if rig.e.Draft.MaxConnsPerHost != 0 {
		t.Fatalf("dec at floor: got %d, want 0", rig.e.Draft.MaxConnsPerHost)
	}
	rig.e.Draft.MaxConnsPerHost = 5
	rig.frame()
	rig.clickN(&rig.e.MaxConnsDec, 5)
	if rig.e.Draft.MaxConnsPerHost != 0 {
		t.Fatalf("dec to floor: got %d, want 0", rig.e.Draft.MaxConnsPerHost)
	}
}

func TestEditorRig_StackBreakpointStepper(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 4000))
	rig.e.Draft.StackBreakpointDp = 0
	rig.frame()
	rig.clickN(&rig.e.StackBpInc, 1)
	if rig.e.Draft.StackBreakpointDp != 400 {
		t.Fatalf("0 -> inc = %d, want 400", rig.e.Draft.StackBreakpointDp)
	}
	rig.clickN(&rig.e.StackBpInc, 1)
	if rig.e.Draft.StackBreakpointDp != 450 {
		t.Fatalf("400 -> inc = %d, want 450", rig.e.Draft.StackBreakpointDp)
	}
	rig.clickN(&rig.e.StackBpDec, 1)
	if rig.e.Draft.StackBreakpointDp != 400 {
		t.Fatalf("450 -> dec = %d, want 400", rig.e.Draft.StackBreakpointDp)
	}
	rig.clickN(&rig.e.StackBpDec, 1)
	if rig.e.Draft.StackBreakpointDp != 0 {
		t.Fatalf("400 -> dec = %d, want 0 (off)", rig.e.Draft.StackBreakpointDp)
	}
	rig.e.Draft.StackBreakpointDp = 2000
	rig.frame()
	rig.clickN(&rig.e.StackBpInc, 1)
	if rig.e.Draft.StackBreakpointDp != 2000 {
		t.Fatalf("inc at max = %d, want 2000", rig.e.Draft.StackBreakpointDp)
	}
}

func TestEditorRig_FloatSteppers(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 4000))
	rig.e.Draft.UIScale = 1.0
	rig.frame()
	rig.clickN(&rig.e.UIScaleInc, 1)
	if got := rig.e.Draft.UIScale; got <= 1.0 || got > 1.1 {
		t.Fatalf("UIScale inc = %v, want ~1.05", got)
	}
	rig.e.Draft.UIScale = 2.0
	rig.frame()
	rig.clickN(&rig.e.UIScaleInc, 1)
	if rig.e.Draft.UIScale != 2.0 {
		t.Errorf("UIScale inc at max = %v, want 2.0", rig.e.Draft.UIScale)
	}
	rig.e.Draft.UIScale = 0.75
	rig.frame()
	rig.clickN(&rig.e.UIScaleDec, 1)
	if rig.e.Draft.UIScale != 0.75 {
		t.Errorf("UIScale dec at min = %v, want 0.75", rig.e.Draft.UIScale)
	}

	rig.e.Draft.DefaultSplitRatio = 0.5
	rig.frame()
	rig.clickN(&rig.e.SplitRatioInc, 1)
	if got := rig.e.Draft.DefaultSplitRatio; got <= 0.5 || got > 0.6 {
		t.Fatalf("SplitRatio inc = %v, want ~0.55", got)
	}
	rig.e.Draft.DefaultSplitRatio = 0.8
	rig.frame()
	rig.clickN(&rig.e.SplitRatioInc, 1)
	if rig.e.Draft.DefaultSplitRatio != 0.8 {
		t.Errorf("SplitRatio inc at max = %v, want 0.8", rig.e.Draft.DefaultSplitRatio)
	}
	rig.e.Draft.DefaultSplitRatio = 0.2
	rig.frame()
	rig.clickN(&rig.e.SplitRatioDec, 1)
	if rig.e.Draft.DefaultSplitRatio != 0.2 {
		t.Errorf("SplitRatio dec at min = %v, want 0.2", rig.e.Draft.DefaultSplitRatio)
	}
}

func TestEditorRig_SyntaxSwatchTogglesPicker(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.clickN(&rig.e.SyntaxSwatchBtns[0], 1)
	if rig.e.ColorPicker.Kind != colorpicker.KindSyntax || rig.e.ColorPicker.OpenIdx != 0 {
		t.Fatalf("swatch click should open the syntax picker: kind=%v idx=%d",
			rig.e.ColorPicker.Kind, rig.e.ColorPicker.OpenIdx)
	}
	rig.clickN(&rig.e.SyntaxSwatchBtns[0], 1)
	if rig.e.ColorPicker.IsOpen() {
		t.Fatal("clicking the same swatch again should close the picker")
	}
	rig.clickN(&rig.e.SyntaxSwatchBtns[0], 1)
	rig.clickN(&rig.e.SyntaxSwatchBtns[1], 1)
	if rig.e.ColorPicker.OpenIdx != 1 {
		t.Fatalf("clicking a different swatch should retarget: idx=%d", rig.e.ColorPicker.OpenIdx)
	}
}

func TestEditorRig_ThemeSwatchTogglesPicker(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.clickN(&rig.e.ThemeColorSwatchBtns[0], 1)
	if rig.e.ColorPicker.Kind != colorpicker.KindTheme || rig.e.ColorPicker.OpenIdx != 0 {
		t.Fatalf("swatch click should open the theme picker: kind=%v idx=%d",
			rig.e.ColorPicker.Kind, rig.e.ColorPicker.OpenIdx)
	}
	rig.clickN(&rig.e.ThemeColorSwatchBtns[0], 1)
	if rig.e.ColorPicker.IsOpen() {
		t.Fatal("clicking the same swatch again should close the picker")
	}
}

func TestEditorRig_ColorPickerCloseButton(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)
	rig.clickN(&rig.e.ThemeColorSwatchBtns[0], 1)
	if !rig.e.ColorPicker.IsOpen() {
		t.Fatal("precondition: picker must be open")
	}
	rig.clickN(&rig.e.ColorPicker.CloseBtn, 1)
	if rig.e.ColorPicker.IsOpen() {
		t.Fatal("CloseBtn should close the picker")
	}
}

func TestEditorRig_PickerHSVWritesOverride(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.clickN(&rig.e.ThemeColorSwatchBtns[0], 1)
	rig.e.ColorPicker.H, rig.e.ColorPicker.S, rig.e.ColorPicker.V = 0.5, 1, 1
	rig.frame()

	hex := rig.e.ThemeColorEditors[0].Text()
	if !strings.HasPrefix(hex, "#") {
		t.Fatalf("dragging the picker should write a hex override, got %q", hex)
	}
	if rig.e.Draft.ThemeOverrides == nil {
		t.Fatal("ThemeOverrides map should have been created")
	}

	rig.clickN(&rig.e.SyntaxSwatchBtns[0], 1)
	rig.e.ColorPicker.H, rig.e.ColorPicker.S, rig.e.ColorPicker.V = 0.25, 1, 1
	rig.frame()
	if !strings.HasPrefix(rig.e.SyntaxOverrideEditors[0].Text(), "#") {
		t.Fatalf("syntax picker should write a hex override, got %q", rig.e.SyntaxOverrideEditors[0].Text())
	}
	if rig.e.Draft.SyntaxOverrides == nil {
		t.Fatal("SyntaxOverrides map should have been created")
	}
}

func TestEditorRig_ResetButtonsClearOverrides(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.e.ThemeColorEditors[0].SetText("#123456")
	rig.e.putThemeOverride(0, "#123456")
	rig.e.SyntaxOverrideEditors[0].SetText("#654321")
	rig.e.putOverride(0, "#654321")
	rig.frame()

	rig.clickN(&rig.e.ThemeColorResetBtns[0], 1)
	if rig.e.ThemeColorEditors[0].Text() != "" {
		t.Errorf("theme reset should clear the editor, got %q", rig.e.ThemeColorEditors[0].Text())
	}
	rig.clickN(&rig.e.SyntaxResetBtns[0], 1)
	if rig.e.SyntaxOverrideEditors[0].Text() != "" {
		t.Errorf("syntax reset should clear the editor, got %q", rig.e.SyntaxOverrideEditors[0].Text())
	}
	if rig.e.Draft.ThemeOverrides != nil {
		t.Errorf("last theme override removed should nil the map, got %+v", rig.e.Draft.ThemeOverrides)
	}
	if rig.e.Draft.SyntaxOverrides != nil {
		t.Errorf("last syntax override removed should nil the map, got %+v", rig.e.Draft.SyntaxOverrides)
	}
}

func TestEditorRig_ResetButtonClosesOpenPickerForSameIndex(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.clickN(&rig.e.ThemeColorSwatchBtns[1], 1)
	rig.clickN(&rig.e.ThemeColorResetBtns[1], 1)
	if rig.e.ColorPicker.IsOpen() {
		t.Error("resetting the swatch under the open picker should close it")
	}

	rig.clickN(&rig.e.SyntaxSwatchBtns[1], 1)
	rig.clickN(&rig.e.SyntaxResetBtns[1], 1)
	if rig.e.ColorPicker.IsOpen() {
		t.Error("resetting the syntax swatch under the open picker should close it")
	}

	rig.clickN(&rig.e.SyntaxSwatchBtns[1], 1)
	rig.clickN(&rig.e.SyntaxResetBtns[2], 1)
	if !rig.e.ColorPicker.IsOpen() {
		t.Error("resetting a different swatch must leave the picker open")
	}
}

func TestEditorRig_ResetAllButtons(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.e.putThemeOverride(0, "#111111")
	rig.e.putOverride(0, "#222222")
	rig.e.ThemeColorEditors[0].SetText("#111111")
	rig.e.SyntaxOverrideEditors[0].SetText("#222222")
	rig.frame()

	rig.clickN(&rig.e.ThemeColorResetAllBtn, 1)
	if rig.e.Draft.ThemeOverrides != nil {
		t.Errorf("reset all should drop theme overrides, got %+v", rig.e.Draft.ThemeOverrides)
	}
	for i := range rig.e.ThemeColorEditors {
		if rig.e.ThemeColorEditors[i].Text() != "" {
			t.Fatalf("theme editor %d not cleared: %q", i, rig.e.ThemeColorEditors[i].Text())
		}
	}

	rig.clickN(&rig.e.SyntaxResetAllBtn, 1)
	if rig.e.Draft.SyntaxOverrides != nil {
		t.Errorf("reset all should drop syntax overrides, got %+v", rig.e.Draft.SyntaxOverrides)
	}
	for i := range rig.e.SyntaxOverrideEditors {
		if rig.e.SyntaxOverrideEditors[i].Text() != "" {
			t.Fatalf("syntax editor %d not cleared: %q", i, rig.e.SyntaxOverrideEditors[i].Text())
		}
	}
}

func TestEditorRig_ResetAllOnEmptyOverridesIsNoop(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.frame()
	rig.clickN(&rig.e.ThemeColorResetAllBtn, 1)
	rig.clickN(&rig.e.SyntaxResetAllBtn, 1)
	if rig.e.Draft.ThemeOverrides != nil || rig.e.Draft.SyntaxOverrides != nil {
		t.Fatal("reset all on empty overrides should keep the maps nil")
	}
}

func TestEditorRig_SpoilerHeadersToggle(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Category = 0
	rig.frames(2)
	rig.clickN(&rig.e.ThemeColorsHeaderBtn, 1)
	if !rig.e.ThemeColorsExpanded {
		t.Fatal("header click should expand the theme colors spoiler")
	}
	rig.clickN(&rig.e.ThemeColorsHeaderBtn, 1)
	if rig.e.ThemeColorsExpanded {
		t.Fatal("second header click should collapse the spoiler")
	}
	rig.clickN(&rig.e.SyntaxColorsHeaderBtn, 1)
	if !rig.e.SyntaxColorsExpanded {
		t.Fatal("header click should expand the syntax colors spoiler")
	}
}

func TestEditorRig_SwitchingThemeResyncsColorEditors(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 20000))
	rig.e.Category = 0
	rig.e.ThemeColorsExpanded = true
	rig.frames(2)

	rig.e.Draft.ThemeOverrides = map[string]model.ThemeColorOverride{}
	rig.e.putThemeOverride(0, "#abcdef")
	rig.e.themeEditorsThemeID = ""
	rig.frame()
	if got := rig.e.ThemeColorEditors[0].Text(); got != "#abcdef" {
		t.Fatalf("editor should show the stored override, got %q", got)
	}

	other := theme.Registry[0].ID
	if other == rig.e.Draft.Theme {
		other = theme.Registry[1].ID
	}
	rig.e.Draft.Theme = other
	rig.frame()
	if got := rig.e.ThemeColorEditors[0].Text(); got != "" {
		t.Fatalf("switching theme should clear the editor for a theme with no override, got %q", got)
	}
}

func TestEditorRig_TextEditorChangesPropagateToDraft(t *testing.T) {
	rig := newEditorRig(t, image.Pt(1000, 4000))
	rig.e.Category = 2
	rig.frames(2)
	rig.e.UserAgentEditor.SetText("  custom-agent  ")
	rig.e.ProxyEditor.SetText(" http://proxy.local:8080 ")
	rig.e.Apply(rig.host)

	if rig.e.Draft.UserAgent != "custom-agent" {
		t.Errorf("UserAgent = %q", rig.e.Draft.UserAgent)
	}
	if rig.e.Draft.Proxy != "http://proxy.local:8080" {
		t.Errorf("Proxy = %q", rig.e.Draft.Proxy)
	}
	if rig.cur.UserAgent != "custom-agent" {
		t.Errorf("Apply should write through to host.Current, got %q", rig.cur.UserAgent)
	}
}

func TestEditorRig_EmptyUserAgentFallsBackOnApply(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 600))
	rig.frame()
	rig.e.UserAgentEditor.SetText("   ")
	rig.e.Apply(rig.host)
	if rig.e.Draft.UserAgent == "" {
		t.Fatal("blank User-Agent should be replaced by the default")
	}
	if !strings.Contains(rig.e.Draft.UserAgent, "Mozilla") {
		t.Errorf("UserAgent = %q, want the built-in default", rig.e.Draft.UserAgent)
	}
}

func TestEditorRig_DefaultHeadersRoundTrip(t *testing.T) {
	src := []model.DefaultHeader{
		{Key: "Accept", Value: "application/json"},
		{Key: "X-Trace", Value: "1"},
	}
	txt := headersToText(src)
	if txt != "Accept: application/json\nX-Trace: 1" {
		t.Fatalf("headersToText = %q", txt)
	}
	got := textToHeaders("# comment\n" + txt + "\nbad-line\n: novalue\n\n")
	if len(got) != len(src) {
		t.Fatalf("textToHeaders returned %d headers, want %d: %+v", len(got), len(src), got)
	}
	for i := range src {
		if got[i] != src[i] {
			t.Errorf("header %d = %+v, want %+v", i, got[i], src[i])
		}
	}
}

func TestEditorRig_NilEditorLayoutDoesNotPanic(t *testing.T) {
	rig := newEditorRig(t, image.Pt(800, 600))
	var nilE *Editor
	gtx := rig.gtx()
	d := nilE.Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	if d.Size.X <= 0 {
		t.Fatalf("nil editor layout returned %+v", d.Size)
	}
	nilE.Apply(rig.host)
	nilE.Reset()
}

func TestEditorRig_LayoutIsStableAcrossManyFrames(t *testing.T) {
	rig := newEditorRig(t, image.Pt(900, 700))
	for cat := range settingsCategories {
		rig.e.Category = cat
		rig.frames(3)
	}
	saves := rig.saves
	rig.frames(5)
	if rig.saves != saves {
		t.Fatalf("idle frames must not trigger saves: %d -> %d", saves, rig.saves)
	}
}
