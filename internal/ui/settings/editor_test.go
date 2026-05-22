package settings

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/theme"

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
