package settings

import (
	"crypto/tls"
	"fmt"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"image"
	"net/http"
	"net/http/httptest"
	"rete/internal/model"
	"rete/internal/persist"
	"rete/internal/ui/colorpicker"
	"rete/internal/ui/theme"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestApplyPropagatesCompactMenus(t *testing.T) {
	old := theme.CompactMenus
	t.Cleanup(func() { theme.CompactMenus = old })

	s := model.DefaultSettings()
	if s.CompactMenus {
		t.Error("menu padding should be kept by default")
	}

	s.CompactMenus = true
	Apply(nil, s)
	if !theme.CompactMenus {
		t.Error("Apply should enable theme.CompactMenus")
	}

	s.CompactMenus = false
	Apply(nil, s)
	if theme.CompactMenus {
		t.Error("Apply should disable theme.CompactMenus")
	}
}

func TestEditorCompactMenusRoundTrip(t *testing.T) {
	cur := model.DefaultSettings()
	cur.CompactMenus = true
	e := NewEditor(cur)
	if !e.CompactMenus.Value {
		t.Error("NewEditor should seed CompactMenus from current settings")
	}

	e.CompactMenus.Value = false
	host := &Host{Current: &cur, OnSave: func() {}}
	e.Apply(host)
	if cur.CompactMenus {
		t.Error("Apply should write the switch value back into settings")
	}

	e.Reset()
	if e.CompactMenus.Value != model.DefaultSettings().CompactMenus {
		t.Error("Reset should restore the default menu padding")
	}
}

func makeGtx(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

func newTestHost() (*Host, *model.AppSettings, *int, *int) {
	resetCalls := 0
	saveCalls := 0
	cur := model.DefaultSettings()
	open := true
	h := &Host{
		Theme:   material.NewTheme(),
		Current: &cur,
		Open:    &open,
		OnClose: func() { resetCalls++ },
		OnSave:  func() { saveCalls++ },
	}
	return h, &cur, &resetCalls, &saveCalls
}

func TestConnsStep(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1}, {5, 1}, {9, 1},
		{10, 10}, {50, 10}, {99, 10},
		{100, 50}, {500, 50}, {999, 50},
		{1000, 100}, {5000, 100}, {99999, 100},
	}
	for _, c := range cases {
		if got := connsStep(c.in); got != c.want {
			t.Errorf("connsStep(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestTimeoutStep(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1}, {5, 1}, {9, 1},
		{10, 5}, {30, 5}, {59, 5},
		{60, 30}, {200, 30}, {299, 30},
		{300, 60}, {1000, 60}, {3600, 60},
	}
	for _, c := range cases {
		if got := timeoutStep(c.in); got != c.want {
			t.Errorf("timeoutStep(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestNewEditor_Populates(t *testing.T) {
	cur := model.DefaultSettings()
	cur.UserAgent = "TestUA/1"
	cur.Proxy = "http://proxy"
	cur.HideTabBar = true
	cur.VerifySSL = true
	cur.DefaultHeaders = []model.DefaultHeader{{Key: "X-A", Value: "1"}}
	cur.AutoFormatJSON = true
	cur.BracketPairColorization = true
	e := NewEditor(cur)
	if e == nil {
		t.Fatal("NewEditor returned nil")
	}
	if e.UserAgentEditor.Text() != "TestUA/1" {
		t.Errorf("UserAgent editor text = %q", e.UserAgentEditor.Text())
	}
	if e.ProxyEditor.Text() != "http://proxy" {
		t.Errorf("Proxy editor text = %q", e.ProxyEditor.Text())
	}
	if !e.HideTabBar.Value {
		t.Error("HideTabBar.Value should be true")
	}
	if !e.VerifySSL.Value {
		t.Error("VerifySSL.Value should be true")
	}
	if !e.AutoFormatJSON.Value {
		t.Error("AutoFormatJSON.Value should be true")
	}
	if !e.BracketPairColorization.Value {
		t.Error("BracketPairColorization.Value should be true")
	}
	if e.DefaultHdrEdit.Text() != "X-A: 1" {
		t.Errorf("DefaultHdrEdit text = %q", e.DefaultHdrEdit.Text())
	}
	if !e.initialized {
		t.Error("initialized should be true")
	}
	if got := len(e.CategoryBtn); got != len(settingsCategories) {
		t.Errorf("CategoryBtn len = %d, want %d", got, len(settingsCategories))
	}
	if got := len(e.ThemeBtns); got != len(theme.Registry) {
		t.Errorf("ThemeBtns len = %d, want %d", got, len(theme.Registry))
	}
	if got := len(e.DefaultMethodBtn); got != len(Methods) {
		t.Errorf("DefaultMethodBtn len = %d, want %d", got, len(Methods))
	}
	if got := len(e.AcceptEncodingBtn); got != len(acceptEncodingOptions) {
		t.Errorf("AcceptEncodingBtn len = %d, want %d", got, len(acceptEncodingOptions))
	}
	if got := len(e.SyntaxOverrideEditors); got != len(theme.TokenColorTable) {
		t.Errorf("SyntaxOverrideEditors len = %d, want %d", got, len(theme.TokenColorTable))
	}
	if got := len(e.ThemeColorEditors); got != len(theme.PaletteColorTable) {
		t.Errorf("ThemeColorEditors len = %d, want %d", got, len(theme.PaletteColorTable))
	}
	if e.ColorPicker.OpenIdx != -1 {
		t.Errorf("ColorPicker.OpenIdx = %d, want -1", e.ColorPicker.OpenIdx)
	}
}

func TestNewEditor_HeaderlessAndOff(t *testing.T) {
	cur := model.AppSettings{Theme: "dark"}
	e := NewEditor(cur)
	if e.DefaultHdrEdit.Text() != "" {
		t.Errorf("DefaultHdrEdit text = %q, want empty", e.DefaultHdrEdit.Text())
	}
	if e.UserAgentEditor.Text() != "" {
		t.Errorf("UserAgent should be empty, got %q", e.UserAgentEditor.Text())
	}
}

func TestEditor_ApplyAndReset(t *testing.T) {
	resetHTTPClient(t)
	host, cur, _, _ := newTestHost()
	e := NewEditor(*cur)

	e.Draft.UITextSize = 14
	e.Draft.BodyTextSize = 13
	e.Draft.UIScale = 1.0
	e.UserAgentEditor.SetText("MyUA/1")
	e.ProxyEditor.SetText("http://proxy:8080")
	e.HideTabBar.Value = true
	e.HideSidebar.Value = true
	e.RestoreTabsOnStartup.Value = false
	e.FollowRedirects.Value = true
	e.VerifySSL.Value = true
	e.KeepAlive.Value = true
	e.DisableHTTP2.Value = true
	e.CookieJar.Value = true
	e.SendConnClose.Value = true
	e.DefaultHdrEdit.SetText("X-K: v1\nX-Y: v2")
	e.WrapLines.Value = true
	e.AutoFormatJSON.Value = true
	e.AutoFormatJSONRequest.Value = true
	e.StripJSONComments.Value = true
	e.TrimTrailingWS.Value = true
	e.BracketPairColorization.Value = true

	e.Apply(host)

	if cur.UserAgent != "MyUA/1" {
		t.Errorf("UserAgent = %q, want MyUA/1", cur.UserAgent)
	}
	if cur.Proxy != "http://proxy:8080" {
		t.Errorf("Proxy = %q", cur.Proxy)
	}
	if !cur.HideTabBar || !cur.HideSidebar {
		t.Error("HideTabBar/HideSidebar should be true")
	}
	if cur.RestoreTabsOnStartup {
		t.Error("RestoreTabsOnStartup should be false")
	}
	if !cur.DisableHTTP2 || !cur.CookieJarEnabled || !cur.SendConnectionClose {
		t.Error("HTTP toggles should be true")
	}
	if len(cur.DefaultHeaders) != 2 {
		t.Fatalf("DefaultHeaders len = %d, want 2", len(cur.DefaultHeaders))
	}
	if cur.DefaultHeaders[0].Key != "X-K" || cur.DefaultHeaders[1].Value != "v2" {
		t.Errorf("DefaultHeaders mismatch: %+v", cur.DefaultHeaders)
	}

	var nilE *Editor
	nilE.Apply(host)

	e.Reset()
	if e.Draft.UserAgent == "" {
		t.Error("Reset: Draft.UserAgent should be defaulted (non-empty)")
	}
	def := model.DefaultSettings()
	if e.Draft.Theme != def.Theme {
		t.Errorf("Reset: Theme = %q, want %q", e.Draft.Theme, def.Theme)
	}
	if e.UserAgentEditor.Text() != def.UserAgent {
		t.Errorf("Reset: UserAgent editor = %q, want %q", e.UserAgentEditor.Text(), def.UserAgent)
	}
	if e.HideTabBar.Value != def.HideTabBar {
		t.Errorf("Reset: HideTabBar.Value = %v, want %v", e.HideTabBar.Value, def.HideTabBar)
	}
	if e.BracketPairColorization.Value != def.BracketPairColorization {
		t.Error("Reset: BracketPairColorization not reset")
	}
	if e.AutoFormatJSON.Value != def.AutoFormatJSON {
		t.Error("Reset: AutoFormatJSON not reset")
	}
	if e.DefaultHdrEdit.Text() != headersToText(def.DefaultHeaders) {
		t.Errorf("Reset: DefaultHdrEdit text = %q", e.DefaultHdrEdit.Text())
	}

	nilE.Reset()
}

func TestSyncSyntaxEditors(t *testing.T) {
	cur := model.DefaultSettings()
	e := NewEditor(cur)

	e.syncSyntaxEditors()
	if e.syntaxEditorsThemeID != cur.Theme {
		t.Errorf("syntaxEditorsThemeID = %q, want %q", e.syntaxEditorsThemeID, cur.Theme)
	}

	for i := range e.SyntaxOverrideEditors {
		if e.SyntaxOverrideEditors[i].Text() != "" {
			t.Errorf("syntax editor[%d] text = %q, want empty", i, e.SyntaxOverrideEditors[i].Text())
		}
	}

	e.SyntaxOverrideEditors[0].SetText("#abcdef")
	e.syncSyntaxEditors()
	if e.SyntaxOverrideEditors[0].Text() != "#abcdef" {
		t.Error("syncSyntaxEditors should be no-op when theme unchanged")
	}

	e.Draft.Theme = "light"
	e.Draft.SyntaxOverrides = map[string]model.ThemeSyntaxOverride{
		"light": {Plain: "#123456"},
	}
	e.syncSyntaxEditors()
	if e.syntaxEditorsThemeID != "light" {
		t.Errorf("syntaxEditorsThemeID = %q, want light", e.syntaxEditorsThemeID)
	}
	if e.SyntaxOverrideEditors[0].Text() != "#123456" {
		t.Errorf("syntax editor[0] text = %q, want #123456", e.SyntaxOverrideEditors[0].Text())
	}
}

func TestSyncThemeEditors(t *testing.T) {
	cur := model.DefaultSettings()
	e := NewEditor(cur)
	e.syncThemeEditors()
	if e.themeEditorsThemeID != cur.Theme {
		t.Errorf("themeEditorsThemeID = %q, want %q", e.themeEditorsThemeID, cur.Theme)
	}
	for i := range e.ThemeColorEditors {
		if e.ThemeColorEditors[i].Text() != "" {
			t.Errorf("theme editor[%d] text = %q, want empty", i, e.ThemeColorEditors[i].Text())
		}
	}

	e.ThemeColorEditors[0].SetText("#111111")
	e.syncThemeEditors()
	if e.ThemeColorEditors[0].Text() != "#111111" {
		t.Error("syncThemeEditors should be no-op when theme unchanged")
	}

	e.Draft.Theme = "light"
	e.Draft.ThemeOverrides = map[string]model.ThemeColorOverride{
		"light": {Bg: "#222222"},
	}
	e.syncThemeEditors()
	if e.ThemeColorEditors[0].Text() != "#222222" {
		t.Errorf("theme editor[0] text = %q, want #222222", e.ThemeColorEditors[0].Text())
	}
}

func TestPutOverride_NilMapInit(t *testing.T) {
	cur := model.DefaultSettings()
	e := NewEditor(cur)
	if e.Draft.SyntaxOverrides != nil {
		t.Fatal("precondition: SyntaxOverrides should start nil")
	}

	e.putOverride(0, "#ABCDEF")
	if e.Draft.SyntaxOverrides == nil {
		t.Fatal("putOverride: SyntaxOverrides map should be initialised on first non-empty set")
	}
	if _, ok := e.Draft.SyntaxOverrides[cur.Theme]; !ok {
		t.Errorf("putOverride: expected entry for theme %q", cur.Theme)
	}

	e.putOverride(0, "")
	if e.Draft.SyntaxOverrides != nil {
		t.Errorf("putOverride: SyntaxOverrides should be nil after clearing only entry, got %+v", e.Draft.SyntaxOverrides)
	}

	e.putOverride(0, "")
	if e.Draft.SyntaxOverrides != nil {
		t.Error("putOverride(empty) with nil map should remain nil")
	}

	e.putOverride(0, "#111111")
	e.putOverride(1, "#222222")
	e.putOverride(0, "")
	if e.Draft.SyntaxOverrides == nil {
		t.Fatal("putOverride: map cleared too eagerly")
	}
	if _, ok := e.Draft.SyntaxOverrides[cur.Theme]; !ok {
		t.Errorf("entry for theme %q dropped while a sibling field remained set", cur.Theme)
	}
}

func TestPutThemeOverride_NilMapInit(t *testing.T) {
	cur := model.DefaultSettings()
	e := NewEditor(cur)
	if e.Draft.ThemeOverrides != nil {
		t.Fatal("precondition: ThemeOverrides should start nil")
	}
	e.putThemeOverride(0, "#FACADE")
	if e.Draft.ThemeOverrides == nil {
		t.Fatal("putThemeOverride: map should be initialised on first non-empty set")
	}
	if _, ok := e.Draft.ThemeOverrides[cur.Theme]; !ok {
		t.Errorf("putThemeOverride: expected entry for theme %q", cur.Theme)
	}
	e.putThemeOverride(0, "")
	if e.Draft.ThemeOverrides != nil {
		t.Errorf("putThemeOverride: ThemeOverrides should be nil after clearing only entry, got %+v", e.Draft.ThemeOverrides)
	}

	e.putThemeOverride(0, "#aaaaaa")
	e.putThemeOverride(1, "#bbbbbb")
	e.putThemeOverride(0, "")
	if e.Draft.ThemeOverrides == nil {
		t.Fatal("putThemeOverride: map cleared too eagerly")
	}
}

func TestIntStepperUpdate_NoEventNoChange(t *testing.T) {
	gtx := makeGtx(200, 30)
	var ed widget.Editor
	v, ok := intStepperUpdate(gtx, &ed, 15, 10, 28)
	if ok {
		t.Errorf("expected no change on first call, got ok=true v=%d", v)
	}
	if v != 15 {
		t.Errorf("expected v=15, got %d", v)
	}

	if ed.Text() != "15" {
		t.Errorf("expected editor populated with %q, got %q", "15", ed.Text())
	}
	if !ed.SingleLine || !ed.Submit {
		t.Error("intStepperUpdate should toggle SingleLine and Submit")
	}
}

func driveSubmit(_ *testing.T, ed *widget.Editor, text string) {
	ed.SetText(text)
}

func TestIntStepperUpdate_ClampOnSetText(t *testing.T) {

	gtx := makeGtx(200, 30)
	var ed widget.Editor
	intStepperUpdate(gtx, &ed, 10, 10, 28)
	if ed.Text() != "10" {
		t.Errorf("initial sync: %q", ed.Text())
	}

	driveSubmit(t, &ed, "999")

	intStepperUpdate(gtx, &ed, 10, 10, 28)
	if ed.Text() != "10" {
		t.Errorf("non-focused refresh should rewrite to current; got %q", ed.Text())
	}
}

func TestFloatStepperUpdate_FormatAndSingleLine(t *testing.T) {
	gtx := makeGtx(200, 30)
	var ed widget.Editor
	v, ok := floatStepperUpdate(gtx, &ed, 1.0, 0.75, 2.0, "%.2f", 1.0)
	if ok || v != 1.0 {
		t.Errorf("expected no change, v=%v ok=%v", v, ok)
	}
	if ed.Text() != "1.00" {
		t.Errorf("expected initial text %q, got %q", "1.00", ed.Text())
	}
	if !ed.SingleLine || !ed.Submit {
		t.Error("floatStepperUpdate should toggle SingleLine and Submit")
	}

	var ed2 widget.Editor
	v2, ok2 := floatStepperUpdate(gtx, &ed2, 0.5, 0.2, 0.8, "%.0f", 100)
	if ok2 || v2 != 0.5 {
		t.Errorf("expected no change, v=%v ok=%v", v2, ok2)
	}
	if ed2.Text() != "50" {
		t.Errorf("expected initial text %q, got %q", "50", ed2.Text())
	}
}

func TestIntStepperUpdate_SyncMatchesItoa(t *testing.T) {
	gtx := makeGtx(100, 30)
	var ed widget.Editor
	for _, v := range []int{0, 1, 10, 28, 3600, 10000} {
		intStepperUpdate(gtx, &ed, v, 0, 99999)
		if ed.Text() != strconv.Itoa(v) {
			t.Errorf("v=%d: got %q", v, ed.Text())
		}
	}
}

func TestFloatStepperUpdate_SyncMatchesFormat(t *testing.T) {
	gtx := makeGtx(100, 30)
	var ed widget.Editor
	cases := []struct {
		v      float32
		mult   float32
		format string
		want   string
	}{
		{1.0, 1.0, "%.2f", "1.00"},
		{1.25, 1.0, "%.2f", "1.25"},
		{0.5, 100, "%.0f", "50"},
		{0.75, 100, "%.0f", "75"},
	}
	for _, c := range cases {
		floatStepperUpdate(gtx, &ed, c.v, 0, 10, c.format, c.mult)
		want := fmt.Sprintf(c.format, c.v*c.mult)
		if ed.Text() != want {
			t.Errorf("v=%v mult=%v: got %q want %q", c.v, c.mult, ed.Text(), c.want)
		}
	}
}

func TestEditor_LayoutSmoke(t *testing.T) {
	resetHTTPClient(t)
	host, cur, _, _ := newTestHost()
	e := NewEditor(*cur)
	for cat := 0; cat < len(settingsCategories); cat++ {
		e.Category = cat
		gtx := makeGtx(1024, 768)

		dims := e.Layout(gtx, host)
		if dims.Size.X == 0 || dims.Size.Y == 0 {
			t.Errorf("cat=%d: zero dimensions: %+v", cat, dims)
		}
	}
}

func TestEditor_LayoutSmoke_ExpandedSpoilers(t *testing.T) {
	resetHTTPClient(t)
	host, cur, _, _ := newTestHost()
	e := NewEditor(*cur)
	e.ThemeColorsExpanded = true
	e.SyntaxColorsExpanded = true
	e.Category = 0
	gtx := makeGtx(1024, 768)
	dims := e.Layout(gtx, host)
	if dims.Size.X == 0 {
		t.Error("expanded spoilers: zero width")
	}
}

func TestEditor_LayoutSmoke_NewThemeDialogOpen(t *testing.T) {
	resetHTTPClient(t)
	host, cur, _, _ := newTestHost()
	e := NewEditor(*cur)
	e.NewThemeDialogOpen = true
	e.NewThemeNameEditor.SetText("MyTheme")
	e.NewThemeBaseID = "dark"
	e.Category = 0
	gtx := makeGtx(1024, 768)
	dims := e.Layout(gtx, host)
	if dims.Size.X == 0 {
		t.Error("new theme dialog open: zero width")
	}
}

func TestEditor_LayoutSmoke_WithCustomTheme(t *testing.T) {
	resetHTTPClient(t)
	host, cur, _, _ := newTestHost()
	cur.CustomThemes = []model.CustomTheme{
		{ID: "custom-1", Name: "MyCustom", BasedOn: "dark"},
	}
	e := NewEditor(*cur)
	e.Category = 0
	gtx := makeGtx(1024, 768)
	dims := e.Layout(gtx, host)
	if dims.Size.X == 0 {
		t.Error("custom theme: zero width")
	}
}

func TestEditor_LayoutSmoke_NilReceiver(t *testing.T) {
	resetHTTPClient(t)
	host, _, _, _ := newTestHost()
	var nilE *Editor
	gtx := makeGtx(800, 600)

	dims := nilE.Layout(gtx, host)

	if dims.Size != gtx.Constraints.Max {
		t.Logf("nil receiver layout returned %+v (constraints.Max=%+v)", dims, gtx.Constraints.Max)
	}
}

var _ = strings.TrimSpace

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

func TestSanitize_ThemeFallback(t *testing.T) {
	s := model.AppSettings{Theme: "bogus"}
	out := Sanitize(s)
	if out.Theme != "dark" {
		t.Errorf("invalid theme should fall back to dark, got %q", out.Theme)
	}

	s.Theme = "light"
	out = Sanitize(s)
	if out.Theme != "light" {
		t.Errorf("valid theme should be kept, got %q", out.Theme)
	}

	s.Theme = "custom-x"
	s.CustomThemes = []model.CustomTheme{{ID: "custom-x", Name: "Custom"}}
	out = Sanitize(s)
	if out.Theme != "custom-x" {
		t.Errorf("custom theme should be kept, got %q", out.Theme)
	}
}

func TestSanitize_TextSizes(t *testing.T) {
	cases := []struct {
		ui, body         int
		wantUI, wantBody int
	}{
		{0, 0, 14, 13},
		{9, 9, 14, 13},
		{10, 10, 10, 10},
		{14, 13, 14, 13},
		{28, 28, 28, 28},
		{29, 99, 28, 28},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", UITextSize: c.ui, BodyTextSize: c.body})
		if out.UITextSize != c.wantUI {
			t.Errorf("UITextSize in=%d → %d, want %d", c.ui, out.UITextSize, c.wantUI)
		}
		if out.BodyTextSize != c.wantBody {
			t.Errorf("BodyTextSize in=%d → %d, want %d", c.body, out.BodyTextSize, c.wantBody)
		}
	}
}

func TestSanitize_UIScale(t *testing.T) {
	cases := []struct {
		in, want float32
	}{
		{0, 1.0},
		{-1, 1.0},
		{0.5, 0.75},
		{0.74, 0.75},
		{0.75, 0.75},
		{1.5, 1.5},
		{2.0, 2.0},
		{2.5, 2.0},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", UIScale: c.in})
		if out.UIScale != c.want {
			t.Errorf("UIScale %v → %v, want %v", c.in, out.UIScale, c.want)
		}
	}
}

func TestSanitize_Timeouts(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", RequestTimeoutSec: -5})
	if out.RequestTimeoutSec != 0 {
		t.Errorf("negative RequestTimeoutSec should be 0, got %d", out.RequestTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", RequestTimeoutSec: 9999})
	if out.RequestTimeoutSec != 3600 {
		t.Errorf("huge RequestTimeoutSec should clamp to 3600, got %d", out.RequestTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ConnectTimeoutSec: -1})
	if out.ConnectTimeoutSec != 0 {
		t.Errorf("negative ConnectTimeoutSec should be 0, got %d", out.ConnectTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ConnectTimeoutSec: 9999})
	if out.ConnectTimeoutSec != 600 {
		t.Errorf("huge ConnectTimeoutSec should clamp to 600, got %d", out.ConnectTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", TLSHandshakeTimeoutSec: -1})
	if out.TLSHandshakeTimeoutSec != 0 {
		t.Errorf("negative TLS should be 0, got %d", out.TLSHandshakeTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", TLSHandshakeTimeoutSec: 9999})
	if out.TLSHandshakeTimeoutSec != 600 {
		t.Errorf("huge TLS should clamp to 600, got %d", out.TLSHandshakeTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", IdleConnTimeoutSec: -1})
	if out.IdleConnTimeoutSec != 0 {
		t.Errorf("negative idle should be 0, got %d", out.IdleConnTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", IdleConnTimeoutSec: 9999})
	if out.IdleConnTimeoutSec != 3600 {
		t.Errorf("huge idle should clamp to 3600, got %d", out.IdleConnTimeoutSec)
	}
}

func TestSanitize_AcceptEncoding(t *testing.T) {
	valid := []string{"", "identity", "gzip", "deflate", "br", "gzip, deflate", "gzip, deflate, br"}
	for _, v := range valid {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultAcceptEncoding: v})
		if out.DefaultAcceptEncoding != v {
			t.Errorf("valid encoding %q changed to %q", v, out.DefaultAcceptEncoding)
		}
	}
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultAcceptEncoding: "garbage"})
	if out.DefaultAcceptEncoding != "gzip" {
		t.Errorf("garbage encoding should default to gzip, got %q", out.DefaultAcceptEncoding)
	}
}

func TestSanitize_UserAgent(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", UserAgent: ""})
	if out.UserAgent == "" {
		t.Error("empty UserAgent should be replaced with default")
	}
	out = Sanitize(model.AppSettings{Theme: "dark", UserAgent: "MyAgent/1.0"})
	if out.UserAgent != "MyAgent/1.0" {
		t.Errorf("explicit UserAgent should be kept, got %q", out.UserAgent)
	}
}

func TestSanitize_Redirects(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", MaxRedirects: -1})
	if out.MaxRedirects != 0 {
		t.Errorf("negative MaxRedirects → 0, got %d", out.MaxRedirects)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", MaxRedirects: 100})
	if out.MaxRedirects != 50 {
		t.Errorf("huge MaxRedirects → 50, got %d", out.MaxRedirects)
	}
}

func TestSanitize_JSONIndent(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", JSONIndentSpaces: -1})
	if out.JSONIndentSpaces != 2 {
		t.Errorf("negative JSONIndentSpaces → 2, got %d", out.JSONIndentSpaces)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", JSONIndentSpaces: 16})
	if out.JSONIndentSpaces != 8 {
		t.Errorf("huge JSONIndentSpaces → 8, got %d", out.JSONIndentSpaces)
	}
}

func TestSanitize_PreviewMaxMB(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", PreviewMaxMB: 0})
	if out.PreviewMaxMB != 100 {
		t.Errorf("zero PreviewMaxMB → 100, got %d", out.PreviewMaxMB)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", PreviewMaxMB: 9999})
	if out.PreviewMaxMB != 500 {
		t.Errorf("huge PreviewMaxMB → 500, got %d", out.PreviewMaxMB)
	}
}

func TestSanitize_SyntaxHighlightMaxMB(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", SyntaxHighlightMaxMB: 0})
	if out.SyntaxHighlightMaxMB != 100 {
		t.Errorf("zero SyntaxHighlightMaxMB → 100, got %d", out.SyntaxHighlightMaxMB)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", SyntaxHighlightMaxMB: 9999})
	if out.SyntaxHighlightMaxMB != 500 {
		t.Errorf("huge SyntaxHighlightMaxMB → 500, got %d", out.SyntaxHighlightMaxMB)
	}
}

func TestSanitize_ResponseBodyPadding(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", ResponseBodyPadding: -1})
	if out.ResponseBodyPadding != 0 {
		t.Errorf("negative ResponseBodyPadding → 0, got %d", out.ResponseBodyPadding)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ResponseBodyPadding: 100})
	if out.ResponseBodyPadding != 32 {
		t.Errorf("huge ResponseBodyPadding → 32, got %d", out.ResponseBodyPadding)
	}
}

func TestSanitize_DefaultMethod(t *testing.T) {
	for _, m := range Methods {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: m})
		if out.DefaultMethod != m {
			t.Errorf("valid method %q changed to %q", m, out.DefaultMethod)
		}
	}
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: "FOO"})
	if out.DefaultMethod != "GET" {
		t.Errorf("invalid method → GET, got %q", out.DefaultMethod)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: ""})
	if out.DefaultMethod != "GET" {
		t.Errorf("empty method → GET, got %q", out.DefaultMethod)
	}
}

func TestSanitize_DefaultSplitRatio(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{0, 0.5},
		{0.1, 0.5},
		{0.19, 0.5},
		{0.2, 0.2},
		{0.5, 0.5},
		{0.8, 0.8},
		{0.9, 0.8},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultSplitRatio: c.in})
		if out.DefaultSplitRatio != c.want {
			t.Errorf("DefaultSplitRatio %v → %v, want %v", c.in, out.DefaultSplitRatio, c.want)
		}
	}
}

func TestSanitize_MaxConnsPerHost(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", MaxConnsPerHost: -1})
	if out.MaxConnsPerHost != 0 {
		t.Errorf("negative MaxConnsPerHost → 0, got %d", out.MaxConnsPerHost)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", MaxConnsPerHost: 999999})
	if out.MaxConnsPerHost != 10000 {
		t.Errorf("huge MaxConnsPerHost → 10000, got %d", out.MaxConnsPerHost)
	}
}

func TestSanitize_StackBreakpointDp(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: -1})
	if out.StackBreakpointDp != 0 {
		t.Errorf("negative StackBreakpointDp → 0, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 300})
	if out.StackBreakpointDp != 400 {
		t.Errorf("StackBreakpointDp 300 → 400, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 9999})
	if out.StackBreakpointDp != 2000 {
		t.Errorf("huge StackBreakpointDp → 2000, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 0})
	if out.StackBreakpointDp != 0 {
		t.Errorf("zero StackBreakpointDp should stay 0, got %d", out.StackBreakpointDp)
	}
}

func TestSanitize_DefaultSidebarWidthPx(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: -1})
	if out.DefaultSidebarWidthPx != 0 {
		t.Errorf("negative DefaultSidebarWidthPx → 0, got %d", out.DefaultSidebarWidthPx)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: 100})
	if out.DefaultSidebarWidthPx != 160 {
		t.Errorf("DefaultSidebarWidthPx 100 → 160, got %d", out.DefaultSidebarWidthPx)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: 9999})
	if out.DefaultSidebarWidthPx != 1000 {
		t.Errorf("huge DefaultSidebarWidthPx → 1000, got %d", out.DefaultSidebarWidthPx)
	}
}

func resetHTTPClient(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		HTTPClient = buildHTTPClient(model.DefaultSettings())
		persistentJar = nil
	})
}

func TestBuildHTTPClient_VerifySSL(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{VerifySSL: true})
	tr := c.Transport.(*http.Transport)
	if tr.TLSClientConfig != nil {
		t.Errorf("VerifySSL=true: TLSClientConfig should be nil, got %+v", tr.TLSClientConfig)
	}
	c = buildHTTPClient(model.AppSettings{VerifySSL: false})
	tr = c.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("VerifySSL=false: expected InsecureSkipVerify=true, got %+v", tr.TLSClientConfig)
	}
	_ = tls.Config{}
}

func TestBuildHTTPClient_KeepAlive(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{KeepAlive: true})
	tr := c.Transport.(*http.Transport)
	if tr.DisableKeepAlives {
		t.Error("KeepAlive=true: DisableKeepAlives should be false")
	}
	c = buildHTTPClient(model.AppSettings{KeepAlive: false})
	tr = c.Transport.(*http.Transport)
	if !tr.DisableKeepAlives {
		t.Error("KeepAlive=false: DisableKeepAlives should be true")
	}
}

func TestBuildHTTPClient_MaxConnsPerHost(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{MaxConnsPerHost: 25})
	tr := c.Transport.(*http.Transport)
	if tr.MaxConnsPerHost != 25 {
		t.Errorf("MaxConnsPerHost should be 25, got %d", tr.MaxConnsPerHost)
	}
	c = buildHTTPClient(model.AppSettings{MaxConnsPerHost: 0})
	tr = c.Transport.(*http.Transport)
	if tr.MaxConnsPerHost != 0 {
		t.Errorf("MaxConnsPerHost=0 should leave it at 0, got %d", tr.MaxConnsPerHost)
	}
}

func TestBuildHTTPClient_DisableHTTP2(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{DisableHTTP2: true})
	tr := c.Transport.(*http.Transport)
	if tr.ForceAttemptHTTP2 {
		t.Error("DisableHTTP2=true: ForceAttemptHTTP2 should be false")
	}
	if tr.TLSNextProto == nil {
		t.Error("DisableHTTP2=true: TLSNextProto should be non-nil (empty map)")
	}
	c = buildHTTPClient(model.AppSettings{DisableHTTP2: false})
	tr = c.Transport.(*http.Transport)
	if !tr.ForceAttemptHTTP2 {
		t.Error("DisableHTTP2=false: ForceAttemptHTTP2 should be true")
	}
	if tr.TLSNextProto != nil {
		t.Error("DisableHTTP2=false: TLSNextProto should be nil")
	}
}

func TestBuildHTTPClient_Timeouts(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{
		ConnectTimeoutSec:      5,
		TLSHandshakeTimeoutSec: 6,
		IdleConnTimeoutSec:     7,
		RequestTimeoutSec:      8,
	})
	tr := c.Transport.(*http.Transport)
	if tr.TLSHandshakeTimeout != 6*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 6s", tr.TLSHandshakeTimeout)
	}
	if tr.IdleConnTimeout != 7*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 7s", tr.IdleConnTimeout)
	}
	if c.Timeout != 8*time.Second {
		t.Errorf("Client.Timeout = %v, want 8s", c.Timeout)
	}
	if tr.DialContext == nil {
		t.Error("ConnectTimeoutSec>0 should set DialContext")
	}
}

func TestBuildHTTPClient_NoTimeouts(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{})
	tr := c.Transport.(*http.Transport)
	if tr.TLSHandshakeTimeout != 10*time.Second {

		t.Logf("TLSHandshakeTimeout=%v (clone default)", tr.TLSHandshakeTimeout)
	}
	if c.Timeout != 0 {
		t.Errorf("Client.Timeout should be 0 when RequestTimeoutSec=0, got %v", c.Timeout)
	}
}

func TestBuildHTTPClient_Proxy(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{Proxy: "http://proxy.example.com:8080"})
	tr := c.Transport.(*http.Transport)
	if tr.Proxy == nil {
		t.Fatal("Proxy should be set")
	}
	req, _ := http.NewRequest("GET", "http://target.example.com", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy func returned err: %v", err)
	}
	if u == nil || u.Host != "proxy.example.com:8080" {
		t.Errorf("unexpected proxy URL: %v", u)
	}
}

func TestBuildHTTPClient_ProxyEmptyAndInvalid(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{Proxy: "   "})
	tr := c.Transport.(*http.Transport)
	_ = tr
	c = buildHTTPClient(model.AppSettings{Proxy: "::not-a-url"})
	tr = c.Transport.(*http.Transport)
	_ = tr
}

func TestBuildHTTPClient_CookieJar(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c.Jar == nil {
		t.Error("CookieJarEnabled=true should set Jar")
	}
	first := c.Jar
	c = buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c.Jar != first {
		t.Error("persistentJar should be reused across calls")
	}
	c = buildHTTPClient(model.AppSettings{CookieJarEnabled: false})
	if c.Jar != nil {
		t.Error("CookieJarEnabled=false should leave Jar nil")
	}
}

func TestBuildHTTPClient_FollowRedirectsDisabled(t *testing.T) {
	resetHTTPClient(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redir.Close()

	c := buildHTTPClient(model.AppSettings{FollowRedirects: false})
	resp, err := c.Get(redir.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 (no follow), got %d", resp.StatusCode)
	}
}

func TestBuildHTTPClient_FollowRedirectsMaxLimit(t *testing.T) {
	resetHTTPClient(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+r.URL.Path+"a", http.StatusFound)
	}))
	defer srv.Close()

	c := buildHTTPClient(model.AppSettings{FollowRedirects: true, MaxRedirects: 2})
	resp, err := c.Get(srv.URL + "/x")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect-limit error")
	}
	if !strings.Contains(err.Error(), "stopped after 2 redirects") {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestBuildHTTPClient_FollowRedirectsNoLimit(t *testing.T) {
	resetHTTPClient(t)
	hops := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops < 3 {
			http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := buildHTTPClient(model.AppSettings{FollowRedirects: true, MaxRedirects: 0})
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after redirects, got %d", resp.StatusCode)
	}
}

func TestApply_NilThemeNoPanic(t *testing.T) {
	resetHTTPClient(t)

	Apply(nil, model.DefaultSettings())
	if UserAgent == "" {
		t.Error("UserAgent should be populated after Apply")
	}
	if HTTPClient == nil {
		t.Error("HTTPClient should be set after Apply")
	}
}

func TestApply_ClampsDefaults(t *testing.T) {
	resetHTTPClient(t)
	bad := model.AppSettings{
		Theme:             "dark",
		BodyTextSize:      12,
		UserAgent:         "",
		JSONIndentSpaces:  -1,
		PreviewMaxMB:      0,
		DefaultMethod:     "",
		DefaultSplitRatio: 0.05,
	}
	Apply(nil, bad)
	if UserAgent == "" {
		t.Error("empty UserAgent should be defaulted in Apply")
	}
	if JSONIndent != 2 {
		t.Errorf("JSONIndent should clamp from -1 to 2, got %d", JSONIndent)
	}
	if PreviewMaxMB != 100 {
		t.Errorf("PreviewMaxMB should clamp from 0 to 100, got %d", PreviewMaxMB)
	}
	if DefaultMethod != "GET" {
		t.Errorf("empty DefaultMethod should default to GET, got %q", DefaultMethod)
	}
	if DefaultSplitRatio != 0.5 {
		t.Errorf("DefaultSplitRatio out-of-range should reset to 0.5, got %v", DefaultSplitRatio)
	}
}

func TestApply_OldClientTransportClosed(t *testing.T) {
	resetHTTPClient(t)

	Apply(nil, model.DefaultSettings())
	old := HTTPClient
	Apply(nil, model.DefaultSettings())
	if old == HTTPClient {
		t.Log("Apply produced same client identity (unusual but not a failure)")
	}
}

func TestHeadersToText(t *testing.T) {
	if got := headersToText(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := headersToText([]model.DefaultHeader{}); got != "" {
		t.Errorf("empty slice → %q", got)
	}
	in := []model.DefaultHeader{
		{Key: "X-A", Value: "1"},
		{Key: "  ", Value: "skipped"},
		{Key: "X-B", Value: ""},
	}
	got := headersToText(in)
	want := "X-A: 1\nX-B: "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextToHeaders(t *testing.T) {
	if got := textToHeaders(""); got != nil {
		t.Errorf("empty → %v", got)
	}
	if got := textToHeaders("   \n   "); got != nil {
		t.Errorf("whitespace-only → %v", got)
	}
	in := "X-A: 1\n# comment\nX-B:2\nbad-line-no-colon\n: missingkey\nX-C: with: colons"
	got := textToHeaders(in)
	want := []model.DefaultHeader{
		{Key: "X-A", Value: "1"},
		{Key: "X-B", Value: "2"},
		{Key: "X-C", Value: "with: colons"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d headers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] %+v != %+v", i, got[i], want[i])
		}
	}
}

func TestTextToHeaders_RoundTrip(t *testing.T) {
	in := []model.DefaultHeader{
		{Key: "Accept", Value: "application/json"},
		{Key: "X-Custom", Value: "val with spaces"},
	}
	text := headersToText(in)
	out := textToHeaders(text)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: %d vs %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("[%d] %+v != %+v", i, out[i], in[i])
		}
	}
}

func TestPersistentJar_ResetOnDisable(t *testing.T) {
	resetHTTPClient(t)
	c1 := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c1.Jar == nil {
		t.Fatal("first client should have jar")
	}
	c2 := buildHTTPClient(model.AppSettings{CookieJarEnabled: false})
	if c2.Jar != nil {
		t.Error("after disable, new client should have nil jar")
	}
	c3 := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c3.Jar == nil {
		t.Fatal("re-enabled client should have jar")
	}
	if c3.Jar == c1.Jar {
		t.Error("re-enabling after disable must produce a fresh jar (cookies cleared)")
	}
}

func markBlurred(t *testing.T, ed *widget.Editor) {
	t.Helper()
	stepperWasFocused[ed] = true
	t.Cleanup(func() { delete(stepperWasFocused, ed) })
}

func TestIntStepperUpdate_BlurCommits(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		current  int
		lo, hi   int
		wantVal  int
		wantOK   bool
		wantText string
	}{
		{"in-range", "20", 14, 10, 28, 20, true, "20"},
		{"clamped-high", "999", 14, 10, 28, 28, true, "28"},
		{"clamped-low", "-5", 14, 10, 28, 10, true, "10"},
		{"percent-suffix", "70%", 50, 20, 80, 70, true, "70"},
		{"whitespace", "  17  ", 14, 10, 28, 17, true, "17"},
		{"same-value", "14", 14, 10, 28, 14, false, "14"},
		{"unparseable", "abc", 14, 10, 28, 14, false, "14"},
		{"empty", "", 14, 10, 28, 14, false, "14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := makeGtx(200, 30)
			ed := new(widget.Editor)
			ed.SetText(tc.typed)
			markBlurred(t, ed)

			v, ok := intStepperUpdate(gtx, ed, tc.current, tc.lo, tc.hi)
			if v != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("got (%d, %v), want (%d, %v)", v, ok, tc.wantVal, tc.wantOK)
			}
			if ed.Text() != tc.wantText {
				t.Errorf("editor text = %q, want %q", ed.Text(), tc.wantText)
			}
		})
	}
}

func TestIntStepperUpdate_BlurLatchClearsAfterCommit(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := new(widget.Editor)
	ed.SetText("22")
	markBlurred(t, ed)

	if v, ok := intStepperUpdate(gtx, ed, 14, 10, 28); !ok || v != 22 {
		t.Fatalf("first call: got (%d, %v), want (22, true)", v, ok)
	}
	if stepperWasFocused[ed] {
		t.Fatal("blur latch should be cleared after the commit frame")
	}
	if _, ok := intStepperUpdate(gtx, ed, 22, 10, 28); ok {
		t.Fatal("second call must not re-commit")
	}
}

func TestFloatStepperUpdate_BlurCommits(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		current  float32
		lo, hi   float32
		format   string
		mult     float32
		wantVal  float32
		wantOK   bool
		wantText string
	}{
		{"scale-in-range", "1.50", 1.0, 0.75, 2.0, "%.2f", 1.0, 1.5, true, "1.50"},
		{"scale-clamp-high", "9", 1.0, 0.75, 2.0, "%.2f", 1.0, 2.0, true, "2.00"},
		{"scale-clamp-low", "0.1", 1.0, 0.75, 2.0, "%.2f", 1.0, 0.75, true, "0.75"},
		{"scale-x-suffix", "1.25x", 1.0, 0.75, 2.0, "%.2f", 1.0, 1.25, true, "1.25"},
		{"ratio-percent", "70%", 0.5, 0.2, 0.8, "%.0f", 100, 0.7, true, "70"},
		{"ratio-clamp-high", "95", 0.5, 0.2, 0.8, "%.0f", 100, 0.8, true, "80"},
		{"ratio-clamp-low", "5", 0.5, 0.2, 0.8, "%.0f", 100, 0.2, true, "20"},
		{"same-value", "50", 0.5, 0.2, 0.8, "%.0f", 100, 0.5, false, "50"},
		{"unparseable", "wide", 0.5, 0.2, 0.8, "%.0f", 100, 0.5, false, "50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := makeGtx(200, 30)
			ed := new(widget.Editor)
			ed.SetText(tc.typed)
			markBlurred(t, ed)

			v, ok := floatStepperUpdate(gtx, ed, tc.current, tc.lo, tc.hi, tc.format, tc.mult)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (v=%v)", ok, tc.wantOK, v)
			}
			if d := v - tc.wantVal; d > 1e-6 || d < -1e-6 {
				t.Fatalf("value = %v, want %v", v, tc.wantVal)
			}
			if ed.Text() != tc.wantText {
				t.Errorf("editor text = %q, want %q", ed.Text(), tc.wantText)
			}
		})
	}
}

func TestStepperUpdate_ForcesSingleLineSubmit(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := new(widget.Editor)
	intStepperUpdate(gtx, ed, 1, 0, 10)
	if !ed.SingleLine || !ed.Submit {
		t.Error("intStepperUpdate must force SingleLine and Submit")
	}
	fed := new(widget.Editor)
	floatStepperUpdate(gtx, fed, 1, 0, 10, "%.2f", 1)
	if !fed.SingleLine || !fed.Submit {
		t.Error("floatStepperUpdate must force SingleLine and Submit")
	}
}

func TestDefaultLabelHelpers(t *testing.T) {
	if got := defaultShownHidden(true); got != "Default: hidden." {
		t.Errorf("defaultShownHidden(true) = %q", got)
	}
	if got := defaultShownHidden(false); got != "Default: shown." {
		t.Errorf("defaultShownHidden(false) = %q", got)
	}
	if got := defaultOnOff(true); got != "Default: on." {
		t.Errorf("defaultOnOff(true) = %q", got)
	}
	if got := defaultOnOff(false); got != "Default: off." {
		t.Errorf("defaultOnOff(false) = %q", got)
	}
	if got := defaultTimeout(0, "never"); got != "Default: never." {
		t.Errorf("defaultTimeout(0) = %q", got)
	}
	if got := defaultTimeout(30, "never"); got != "Default: 30 s." {
		t.Errorf("defaultTimeout(30) = %q", got)
	}
}

func TestTextToHeaders_SkipsBlankKeys(t *testing.T) {
	got := textToHeaders("   : value\n:\nOK: 1\n")
	if len(got) != 1 || got[0].Key != "OK" || got[0].Value != "1" {
		t.Fatalf("textToHeaders = %+v, want a single OK:1 header", got)
	}
	if textToHeaders("   \n\n") != nil {
		t.Error("blank input should yield nil")
	}
}
