package ui

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/colorpicker"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

var settingsCategories = []string{"Appearance", "Sizes", "HTTP", "Advanced"}

var acceptEncodingOptions = []struct {
	Value string
	Label string
}{
	{"", "off"},
	{"identity", "identity"},
	{"gzip", "gzip"},
	{"deflate", "deflate"},
	{"br", "br"},
	{"gzip, deflate", "gzip+deflate"},
	{"gzip, deflate, br", "all"},
}

type SettingsEditorState struct {
	Draft model.AppSettings

	Category    int
	CategoryBtn []widget.Clickable

	BackBtn     widget.Clickable
	ResetBtn    widget.Clickable
	ContentList widget.List

	ThemeBtns []widget.Clickable

	UISizeDec         widget.Clickable
	UISizeInc         widget.Clickable
	UISizeEditor      widget.Editor
	BodySizeDec       widget.Clickable
	BodySizeInc       widget.Clickable
	BodySizeEditor    widget.Editor
	UIScaleDec        widget.Clickable
	UIScaleInc        widget.Clickable
	UIScaleEditor     widget.Editor
	BodyPaddingDec    widget.Clickable
	BodyPaddingInc    widget.Clickable
	BodyPaddingEditor widget.Editor
	SplitRatioDec     widget.Clickable
	SplitRatioInc     widget.Clickable
	SplitRatioEditor  widget.Editor
	StackBpDec        widget.Clickable
	StackBpInc        widget.Clickable
	StackBpEditor     widget.Editor

	HideTabBar           widget.Bool
	HideSidebar          widget.Bool
	RestoreTabsOnStartup widget.Bool

	SidebarWidthDec    widget.Clickable
	SidebarWidthInc    widget.Clickable
	SidebarWidthEditor widget.Editor

	TimeoutDec           widget.Clickable
	TimeoutInc           widget.Clickable
	TimeoutEditor        widget.Editor
	ConnectTimeoutDec    widget.Clickable
	ConnectTimeoutInc    widget.Clickable
	ConnectTimeoutEditor widget.Editor
	TLSTimeoutDec        widget.Clickable
	TLSTimeoutInc        widget.Clickable
	TLSTimeoutEditor     widget.Editor
	IdleTimeoutDec       widget.Clickable
	IdleTimeoutInc       widget.Clickable
	IdleTimeoutEditor    widget.Editor
	MaxRedirectsDec      widget.Clickable
	MaxRedirectsInc      widget.Clickable
	MaxRedirectsEditor   widget.Editor
	MaxConnsDec          widget.Clickable
	MaxConnsInc          widget.Clickable
	MaxConnsEditor       widget.Editor
	FollowRedirects      widget.Bool
	VerifySSL            widget.Bool
	KeepAlive            widget.Bool
	DisableHTTP2         widget.Bool
	CookieJar            widget.Bool
	SendConnClose        widget.Bool
	UserAgentEditor      widget.Editor
	ProxyEditor          widget.Editor
	DefaultHdrEdit       widget.Editor
	DefaultMethodBtn     []widget.Clickable
	AcceptEncodingBtn    []widget.Clickable

	JSONIndentDec           widget.Clickable
	JSONIndentInc           widget.Clickable
	JSONIndentEditor        widget.Editor
	PreviewMaxDec           widget.Clickable
	PreviewMaxInc           widget.Clickable
	PreviewMaxEditor        widget.Editor
	WrapLines               widget.Bool
	AutoFormatJSON          widget.Bool
	AutoFormatJSONRequest   widget.Bool
	StripJSONComments       widget.Bool
	TrimTrailingWS          widget.Bool
	BracketPairColorization widget.Bool

	SyntaxOverrideEditors []widget.Editor
	SyntaxResetBtns       []widget.Clickable
	SyntaxSwatchBtns      []widget.Clickable
	SyntaxResetAllBtn     widget.Clickable
	syntaxEditorsThemeID  string

	ThemeColorEditors     []widget.Editor
	ThemeColorResetBtns   []widget.Clickable
	ThemeColorSwatchBtns  []widget.Clickable
	ThemeColorResetAllBtn widget.Clickable
	themeEditorsThemeID   string

	ThemeColorsExpanded   bool
	SyntaxColorsExpanded  bool
	ThemeColorsHeaderBtn  widget.Clickable
	SyntaxColorsHeaderBtn widget.Clickable
	SyntaxSplitRatio      float32
	SyntaxSplitDrag       gesture.Drag
	SyntaxSplitDragY      float32
	ThemeColorsList       widget.List
	SyntaxColorsList      widget.List

	ColorPicker colorpicker.State

	NewThemeBtn        widget.Clickable
	NewThemeDialogOpen bool
	NewThemeNameEditor widget.Editor
	NewThemeBaseBtns   []widget.Clickable
	NewThemeBaseID     string
	NewThemeCreateBtn  widget.Clickable
	NewThemeCancelBtn  widget.Clickable
	CustomThemeBtns    []widget.Clickable
	CustomThemeDelBtns []widget.Clickable

	initialized bool
}

func newSettingsEditorState(current model.AppSettings) *SettingsEditorState {
	s := &SettingsEditorState{
		Draft:                 current,
		CategoryBtn:           make([]widget.Clickable, len(settingsCategories)),
		ThemeBtns:             make([]widget.Clickable, len(theme.Registry)),
		DefaultMethodBtn:      make([]widget.Clickable, len(settings.Methods)),
		AcceptEncodingBtn:     make([]widget.Clickable, len(acceptEncodingOptions)),
		SyntaxOverrideEditors: make([]widget.Editor, len(theme.TokenColorTable)),
		SyntaxResetBtns:       make([]widget.Clickable, len(theme.TokenColorTable)),
		SyntaxSwatchBtns:      make([]widget.Clickable, len(theme.TokenColorTable)),
		ThemeColorEditors:     make([]widget.Editor, len(theme.PaletteColorTable)),
		ThemeColorResetBtns:   make([]widget.Clickable, len(theme.PaletteColorTable)),
		ThemeColorSwatchBtns:  make([]widget.Clickable, len(theme.PaletteColorTable)),
	}
	s.ColorPicker.Kind = colorpicker.KindNone
	s.ColorPicker.OpenIdx = -1
	for i := range s.SyntaxOverrideEditors {
		s.SyntaxOverrideEditors[i].SingleLine = true
		s.SyntaxOverrideEditors[i].Submit = true
	}
	for i := range s.ThemeColorEditors {
		s.ThemeColorEditors[i].SingleLine = true
		s.ThemeColorEditors[i].Submit = true
	}
	s.ContentList.Axis = layout.Vertical

	s.UserAgentEditor.SingleLine = true
	s.UserAgentEditor.Submit = true
	s.UserAgentEditor.SetText(current.UserAgent)

	s.ProxyEditor.SingleLine = true
	s.ProxyEditor.Submit = true
	s.ProxyEditor.SetText(current.Proxy)

	s.DefaultHdrEdit.SetText(headersToText(current.DefaultHeaders))

	s.HideTabBar.Value = current.HideTabBar
	s.HideSidebar.Value = current.HideSidebar
	s.RestoreTabsOnStartup.Value = current.RestoreTabsOnStartup
	s.FollowRedirects.Value = current.FollowRedirects
	s.VerifySSL.Value = current.VerifySSL
	s.KeepAlive.Value = current.KeepAlive
	s.DisableHTTP2.Value = current.DisableHTTP2
	s.CookieJar.Value = current.CookieJarEnabled
	s.SendConnClose.Value = current.SendConnectionClose
	s.WrapLines.Value = current.WrapLinesDefault
	s.AutoFormatJSON.Value = current.AutoFormatJSON
	s.AutoFormatJSONRequest.Value = current.AutoFormatJSONRequest
	s.StripJSONComments.Value = current.StripJSONComments
	s.TrimTrailingWS.Value = current.TrimTrailingWhitespace
	s.BracketPairColorization.Value = current.BracketPairColorization

	s.initialized = true
	return s
}

func headersToText(hs []model.DefaultHeader) string {
	if len(hs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, h := range hs {
		if strings.TrimSpace(h.Key) == "" {
			continue
		}
		b.WriteString(h.Key)
		b.WriteString(": ")
		b.WriteString(h.Value)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func textToHeaders(s string) []model.DefaultHeader {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []model.DefaultHeader
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:idx])
		v := strings.TrimSpace(line[idx+1:])
		if k == "" {
			continue
		}
		out = append(out, model.DefaultHeader{Key: k, Value: v})
	}
	return out
}

func (ui *AppUI) applyDraftSettings() {
	if ui.SettingsState == nil {
		return
	}
	st := ui.SettingsState
	st.Draft.HideTabBar = st.HideTabBar.Value
	st.Draft.HideSidebar = st.HideSidebar.Value
	st.Draft.RestoreTabsOnStartup = st.RestoreTabsOnStartup.Value

	st.Draft.UserAgent = strings.TrimSpace(st.UserAgentEditor.Text())
	st.Draft.Proxy = strings.TrimSpace(st.ProxyEditor.Text())
	st.Draft.FollowRedirects = st.FollowRedirects.Value
	st.Draft.VerifySSL = st.VerifySSL.Value
	st.Draft.KeepAlive = st.KeepAlive.Value
	st.Draft.DisableHTTP2 = st.DisableHTTP2.Value
	st.Draft.CookieJarEnabled = st.CookieJar.Value
	st.Draft.SendConnectionClose = st.SendConnClose.Value
	st.Draft.DefaultHeaders = textToHeaders(st.DefaultHdrEdit.Text())
	st.Draft.WrapLinesDefault = st.WrapLines.Value
	st.Draft.AutoFormatJSON = st.AutoFormatJSON.Value
	st.Draft.AutoFormatJSONRequest = st.AutoFormatJSONRequest.Value
	st.Draft.StripJSONComments = st.StripJSONComments.Value
	st.Draft.TrimTrailingWhitespace = st.TrimTrailingWS.Value
	st.Draft.BracketPairColorization = st.BracketPairColorization.Value

	st.Draft = settings.Sanitize(st.Draft)
	ui.Settings = st.Draft
	settings.Apply(ui.Theme, ui.Settings)
}

func (ui *AppUI) closeSettings() {
	ui.SettingsOpen = false
	ui.SettingsState = nil
	if ui.Window != nil {
		ui.Window.Invalidate()
	}
}

func (st *SettingsEditorState) syncSyntaxEditors() {
	if st.syntaxEditorsThemeID == st.Draft.Theme {
		return
	}
	st.syntaxEditorsThemeID = st.Draft.Theme
	ov := st.Draft.SyntaxOverrides[st.Draft.Theme]
	for i, entry := range theme.TokenColorTable {
		st.SyntaxOverrideEditors[i].SetText(entry.GetOv(ov))
	}
}

func (st *SettingsEditorState) putOverride(i int, h string) {
	themeID := st.Draft.Theme
	ov := st.Draft.SyntaxOverrides[themeID]
	theme.TokenColorTable[i].SetOv(&ov, h)
	if ov == (model.ThemeSyntaxOverride{}) {
		if st.Draft.SyntaxOverrides != nil {
			delete(st.Draft.SyntaxOverrides, themeID)
			if len(st.Draft.SyntaxOverrides) == 0 {
				st.Draft.SyntaxOverrides = nil
			}
		}
		return
	}
	if st.Draft.SyntaxOverrides == nil {
		st.Draft.SyntaxOverrides = map[string]model.ThemeSyntaxOverride{}
	}
	st.Draft.SyntaxOverrides[themeID] = ov
}

func (st *SettingsEditorState) syncThemeEditors() {
	if st.themeEditorsThemeID == st.Draft.Theme {
		return
	}
	st.themeEditorsThemeID = st.Draft.Theme
	ov := st.Draft.ThemeOverrides[st.Draft.Theme]
	for i, entry := range theme.PaletteColorTable {
		st.ThemeColorEditors[i].SetText(entry.GetOv(ov))
	}
}

func (st *SettingsEditorState) putThemeOverride(i int, h string) {
	themeID := st.Draft.Theme
	ov := st.Draft.ThemeOverrides[themeID]
	theme.PaletteColorTable[i].SetOv(&ov, h)
	if ov == (model.ThemeColorOverride{}) {
		if st.Draft.ThemeOverrides != nil {
			delete(st.Draft.ThemeOverrides, themeID)
			if len(st.Draft.ThemeOverrides) == 0 {
				st.Draft.ThemeOverrides = nil
			}
		}
		return
	}
	if st.Draft.ThemeOverrides == nil {
		st.Draft.ThemeOverrides = map[string]model.ThemeColorOverride{}
	}
	st.Draft.ThemeOverrides[themeID] = ov
}

func (ui *AppUI) resetSettings() {
	if ui.SettingsState == nil {
		return
	}
	def := model.DefaultSettings()
	st := ui.SettingsState
	st.Draft = def
	st.HideTabBar.Value = def.HideTabBar
	st.HideSidebar.Value = def.HideSidebar
	st.RestoreTabsOnStartup.Value = def.RestoreTabsOnStartup

	st.UserAgentEditor.SetText(def.UserAgent)
	st.ProxyEditor.SetText(def.Proxy)
	st.FollowRedirects.Value = def.FollowRedirects
	st.VerifySSL.Value = def.VerifySSL
	st.KeepAlive.Value = def.KeepAlive
	st.DisableHTTP2.Value = def.DisableHTTP2
	st.CookieJar.Value = def.CookieJarEnabled
	st.SendConnClose.Value = def.SendConnectionClose
	st.DefaultHdrEdit.SetText(headersToText(def.DefaultHeaders))
	st.WrapLines.Value = def.WrapLinesDefault
	st.AutoFormatJSON.Value = def.AutoFormatJSON
	st.AutoFormatJSONRequest.Value = def.AutoFormatJSONRequest
	st.StripJSONComments.Value = def.StripJSONComments
	st.TrimTrailingWS.Value = def.TrimTrailingWhitespace
	st.BracketPairColorization.Value = def.BracketPairColorization
}

func (ui *AppUI) layoutSettings(gtx layout.Context) layout.Dimensions {
	if ui.SettingsState == nil {
		ui.SettingsState = newSettingsEditorState(ui.Settings)
	}
	st := ui.SettingsState

	for st.BackBtn.Clicked(gtx) {
		ui.closeSettings()
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	resetChanged := false
	for st.ResetBtn.Clicked(gtx) {
		ui.resetSettings()
		resetChanged = true
	}

	for i := range st.CategoryBtn {
		if st.CategoryBtn[i].Clicked(gtx) {
			st.Category = i
		}
	}

	changed := false
	for i := range st.ThemeBtns {
		for st.ThemeBtns[i].Clicked(gtx) {
			tid := theme.Registry[i].ID
			if st.Draft.Theme != tid {
				st.Draft.Theme = tid
				st.ColorPicker.Close()
				changed = true
			}
		}
	}
	if len(st.NewThemeBaseBtns) != len(theme.Registry) {
		st.NewThemeBaseBtns = make([]widget.Clickable, len(theme.Registry))
	}
	if len(st.CustomThemeBtns) != len(st.Draft.CustomThemes) {
		st.CustomThemeBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
		st.CustomThemeDelBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
	}
	for i := range st.CustomThemeBtns {
		for st.CustomThemeBtns[i].Clicked(gtx) {
			if i < len(st.Draft.CustomThemes) {
				tid := st.Draft.CustomThemes[i].ID
				if st.Draft.Theme != tid {
					st.Draft.Theme = tid
					st.ColorPicker.Close()
					changed = true
				}
			}
		}
	}
	deleteIdx := -1
	for i := range st.CustomThemeDelBtns {
		if i >= len(st.CustomThemeDelBtns) {
			break
		}
		if st.CustomThemeDelBtns[i].Clicked(gtx) {
			deleteIdx = i
			for st.CustomThemeDelBtns[i].Clicked(gtx) {
			}
			break
		}
	}
	if deleteIdx >= 0 && deleteIdx < len(st.Draft.CustomThemes) {
		deletedID := st.Draft.CustomThemes[deleteIdx].ID
		st.Draft.CustomThemes = append(st.Draft.CustomThemes[:deleteIdx], st.Draft.CustomThemes[deleteIdx+1:]...)
		st.CustomThemeBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
		st.CustomThemeDelBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
		if st.Draft.Theme == deletedID {
			st.Draft.Theme = "dark"
		}
		changed = true
	}
	for st.NewThemeBtn.Clicked(gtx) {
		st.NewThemeDialogOpen = !st.NewThemeDialogOpen
		if st.NewThemeDialogOpen {
			st.NewThemeNameEditor.SingleLine = true
			st.NewThemeNameEditor.Submit = true
			st.NewThemeNameEditor.SetText("")
			st.NewThemeBaseID = st.Draft.Theme
		}
	}
	for i := range st.NewThemeBaseBtns {
		for st.NewThemeBaseBtns[i].Clicked(gtx) {
			st.NewThemeBaseID = theme.Registry[i].ID
		}
	}
	for st.NewThemeCancelBtn.Clicked(gtx) {
		st.NewThemeDialogOpen = false
	}
	for st.NewThemeCreateBtn.Clicked(gtx) {
		name := strings.TrimSpace(st.NewThemeNameEditor.Text())
		if name == "" {
			continue
		}
		baseID := st.NewThemeBaseID
		if baseID == "" {
			baseID = "dark"
		}
		basePalette := theme.PaletteFor(baseID, st.Draft.CustomThemes)
		newID := "custom-" + persist.NewRandomID()[:8]
		ct := model.CustomTheme{
			ID:      newID,
			Name:    name,
			BasedOn: baseID,
			Palette: theme.PaletteToOverride(basePalette),
			Syntax:  theme.SyntaxToOverride(basePalette.Syntax),
		}
		st.Draft.CustomThemes = append(st.Draft.CustomThemes, ct)
		st.Draft.Theme = newID
		st.NewThemeDialogOpen = false
		st.NewThemeNameEditor.SetText("")
		st.CustomThemeBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
		st.CustomThemeDelBtns = make([]widget.Clickable, len(st.Draft.CustomThemes))
		st.ColorPicker.Close()
		changed = true
	}
	st.syncSyntaxEditors()

	for st.UISizeDec.Clicked(gtx) {
		if st.Draft.UITextSize > 10 {
			st.Draft.UITextSize--
			changed = true
		}
	}
	for st.UISizeInc.Clicked(gtx) {
		if st.Draft.UITextSize < 28 {
			st.Draft.UITextSize++
			changed = true
		}
	}
	for st.BodySizeDec.Clicked(gtx) {
		if st.Draft.BodyTextSize > 10 {
			st.Draft.BodyTextSize--
			changed = true
		}
	}
	for st.BodySizeInc.Clicked(gtx) {
		if st.Draft.BodyTextSize < 28 {
			st.Draft.BodyTextSize++
			changed = true
		}
	}
	for st.UIScaleDec.Clicked(gtx) {
		if st.Draft.UIScale > 0.75 {
			st.Draft.UIScale -= 0.05
			changed = true
		}
	}
	for st.UIScaleInc.Clicked(gtx) {
		if st.Draft.UIScale < 2.0 {
			st.Draft.UIScale += 0.05
			changed = true
		}
	}
	for st.BodyPaddingDec.Clicked(gtx) {
		if st.Draft.ResponseBodyPadding > 0 {
			st.Draft.ResponseBodyPadding--
			changed = true
		}
	}
	for st.BodyPaddingInc.Clicked(gtx) {
		if st.Draft.ResponseBodyPadding < 32 {
			st.Draft.ResponseBodyPadding++
			changed = true
		}
	}
	for st.SplitRatioDec.Clicked(gtx) {
		if st.Draft.DefaultSplitRatio > 0.2 {
			st.Draft.DefaultSplitRatio -= 0.05
			if st.Draft.DefaultSplitRatio < 0.2 {
				st.Draft.DefaultSplitRatio = 0.2
			}
			changed = true
		}
	}
	for st.SplitRatioInc.Clicked(gtx) {
		if st.Draft.DefaultSplitRatio < 0.8 {
			st.Draft.DefaultSplitRatio += 0.05
			if st.Draft.DefaultSplitRatio > 0.8 {
				st.Draft.DefaultSplitRatio = 0.8
			}
			changed = true
		}
	}
	for st.StackBpDec.Clicked(gtx) {
		if st.Draft.StackBreakpointDp <= 400 {
			st.Draft.StackBreakpointDp = 0
		} else {
			st.Draft.StackBreakpointDp -= 50
		}
		changed = true
	}
	for st.StackBpInc.Clicked(gtx) {
		if st.Draft.StackBreakpointDp == 0 {
			st.Draft.StackBreakpointDp = 400
		} else if st.Draft.StackBreakpointDp < 2000 {
			st.Draft.StackBreakpointDp += 50
			if st.Draft.StackBreakpointDp > 2000 {
				st.Draft.StackBreakpointDp = 2000
			}
		}
		changed = true
	}
	for i := range st.DefaultMethodBtn {
		for st.DefaultMethodBtn[i].Clicked(gtx) {
			if st.Draft.DefaultMethod != settings.Methods[i] {
				st.Draft.DefaultMethod = settings.Methods[i]
				changed = true
			}
		}
	}
	for st.MaxConnsDec.Clicked(gtx) {
		step := connsStep(st.Draft.MaxConnsPerHost)
		if st.Draft.MaxConnsPerHost > 0 {
			st.Draft.MaxConnsPerHost -= step
			if st.Draft.MaxConnsPerHost < 0 {
				st.Draft.MaxConnsPerHost = 0
			}
			changed = true
		}
	}
	for st.MaxConnsInc.Clicked(gtx) {
		step := connsStep(st.Draft.MaxConnsPerHost)
		if st.Draft.MaxConnsPerHost < 10000 {
			st.Draft.MaxConnsPerHost += step
			if st.Draft.MaxConnsPerHost > 10000 {
				st.Draft.MaxConnsPerHost = 10000
			}
			changed = true
		}
	}

	for st.TimeoutDec.Clicked(gtx) {
		step := timeoutStep(st.Draft.RequestTimeoutSec)
		if st.Draft.RequestTimeoutSec > 0 {
			st.Draft.RequestTimeoutSec -= step
			if st.Draft.RequestTimeoutSec < 0 {
				st.Draft.RequestTimeoutSec = 0
			}
			changed = true
		}
	}
	for st.TimeoutInc.Clicked(gtx) {
		step := timeoutStep(st.Draft.RequestTimeoutSec)
		if st.Draft.RequestTimeoutSec < 3600 {
			st.Draft.RequestTimeoutSec += step
			if st.Draft.RequestTimeoutSec > 3600 {
				st.Draft.RequestTimeoutSec = 3600
			}
			changed = true
		}
	}
	for st.ConnectTimeoutDec.Clicked(gtx) {
		if st.Draft.ConnectTimeoutSec > 0 {
			st.Draft.ConnectTimeoutSec--
			changed = true
		}
	}
	for st.ConnectTimeoutInc.Clicked(gtx) {
		if st.Draft.ConnectTimeoutSec < 600 {
			st.Draft.ConnectTimeoutSec++
			changed = true
		}
	}
	for st.TLSTimeoutDec.Clicked(gtx) {
		if st.Draft.TLSHandshakeTimeoutSec > 0 {
			st.Draft.TLSHandshakeTimeoutSec--
			changed = true
		}
	}
	for st.TLSTimeoutInc.Clicked(gtx) {
		if st.Draft.TLSHandshakeTimeoutSec < 600 {
			st.Draft.TLSHandshakeTimeoutSec++
			changed = true
		}
	}
	for st.IdleTimeoutDec.Clicked(gtx) {
		step := timeoutStep(st.Draft.IdleConnTimeoutSec)
		if st.Draft.IdleConnTimeoutSec > 0 {
			st.Draft.IdleConnTimeoutSec -= step
			if st.Draft.IdleConnTimeoutSec < 0 {
				st.Draft.IdleConnTimeoutSec = 0
			}
			changed = true
		}
	}
	for st.IdleTimeoutInc.Clicked(gtx) {
		step := timeoutStep(st.Draft.IdleConnTimeoutSec)
		if st.Draft.IdleConnTimeoutSec < 3600 {
			st.Draft.IdleConnTimeoutSec += step
			if st.Draft.IdleConnTimeoutSec > 3600 {
				st.Draft.IdleConnTimeoutSec = 3600
			}
			changed = true
		}
	}
	for st.SidebarWidthDec.Clicked(gtx) {
		if st.Draft.DefaultSidebarWidthPx > 160 {
			st.Draft.DefaultSidebarWidthPx -= 10
			if st.Draft.DefaultSidebarWidthPx < 160 {
				st.Draft.DefaultSidebarWidthPx = 160
			}
			changed = true
		}
	}
	for st.SidebarWidthInc.Clicked(gtx) {
		if st.Draft.DefaultSidebarWidthPx < 1000 {
			st.Draft.DefaultSidebarWidthPx += 10
			if st.Draft.DefaultSidebarWidthPx > 1000 {
				st.Draft.DefaultSidebarWidthPx = 1000
			}
			changed = true
		}
	}
	for i := range st.AcceptEncodingBtn {
		for st.AcceptEncodingBtn[i].Clicked(gtx) {
			if st.Draft.DefaultAcceptEncoding != acceptEncodingOptions[i].Value {
				st.Draft.DefaultAcceptEncoding = acceptEncodingOptions[i].Value
				changed = true
			}
		}
	}
	for st.MaxRedirectsDec.Clicked(gtx) {
		if st.Draft.MaxRedirects > 0 {
			st.Draft.MaxRedirects--
			changed = true
		}
	}
	for st.MaxRedirectsInc.Clicked(gtx) {
		if st.Draft.MaxRedirects < 50 {
			st.Draft.MaxRedirects++
			changed = true
		}
	}
	for st.JSONIndentDec.Clicked(gtx) {
		if st.Draft.JSONIndentSpaces > 0 {
			st.Draft.JSONIndentSpaces--
			changed = true
		}
	}
	for st.JSONIndentInc.Clicked(gtx) {
		if st.Draft.JSONIndentSpaces < 8 {
			st.Draft.JSONIndentSpaces++
			changed = true
		}
	}
	for st.PreviewMaxDec.Clicked(gtx) {
		if st.Draft.PreviewMaxMB > 1 {
			st.Draft.PreviewMaxMB--
			changed = true
		}
	}
	for st.PreviewMaxInc.Clicked(gtx) {
		if st.Draft.PreviewMaxMB < 500 {
			st.Draft.PreviewMaxMB++
			changed = true
		}
	}

	if v, ok := intStepperUpdate(gtx, &st.UISizeEditor, st.Draft.UITextSize, 10, 28); ok {
		st.Draft.UITextSize = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.BodySizeEditor, st.Draft.BodyTextSize, 10, 28); ok {
		st.Draft.BodyTextSize = v
		changed = true
	}
	if v, ok := floatStepperUpdate(gtx, &st.UIScaleEditor, st.Draft.UIScale, 0.75, 2.0, "%.2f", 1.0); ok {
		st.Draft.UIScale = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.BodyPaddingEditor, st.Draft.ResponseBodyPadding, 0, 32); ok {
		st.Draft.ResponseBodyPadding = v
		changed = true
	}
	if v, ok := floatStepperUpdate(gtx, &st.SplitRatioEditor, st.Draft.DefaultSplitRatio, 0.2, 0.8, "%.0f", 100); ok {
		st.Draft.DefaultSplitRatio = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.StackBpEditor, st.Draft.StackBreakpointDp, 0, 2000); ok {
		if v > 0 && v < 400 {
			v = 400
		}
		st.Draft.StackBreakpointDp = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.TimeoutEditor, st.Draft.RequestTimeoutSec, 0, 3600); ok {
		st.Draft.RequestTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.ConnectTimeoutEditor, st.Draft.ConnectTimeoutSec, 0, 600); ok {
		st.Draft.ConnectTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.TLSTimeoutEditor, st.Draft.TLSHandshakeTimeoutSec, 0, 600); ok {
		st.Draft.TLSHandshakeTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.IdleTimeoutEditor, st.Draft.IdleConnTimeoutSec, 0, 3600); ok {
		st.Draft.IdleConnTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.MaxRedirectsEditor, st.Draft.MaxRedirects, 0, 50); ok {
		st.Draft.MaxRedirects = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.MaxConnsEditor, st.Draft.MaxConnsPerHost, 0, 10000); ok {
		st.Draft.MaxConnsPerHost = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.SidebarWidthEditor, st.Draft.DefaultSidebarWidthPx, 160, 1000); ok {
		st.Draft.DefaultSidebarWidthPx = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.JSONIndentEditor, st.Draft.JSONIndentSpaces, 0, 8); ok {
		st.Draft.JSONIndentSpaces = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &st.PreviewMaxEditor, st.Draft.PreviewMaxMB, 1, 500); ok {
		st.Draft.PreviewMaxMB = v
		changed = true
	}

	for _, ed := range []*widget.Editor{&st.UserAgentEditor, &st.ProxyEditor, &st.DefaultHdrEdit} {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}

	for i := range st.SyntaxOverrideEditors {
		ed := &st.SyntaxOverrideEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				st.putOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range st.SyntaxResetBtns {
		for st.SyntaxResetBtns[i].Clicked(gtx) {
			st.putOverride(i, "")
			st.SyntaxOverrideEditors[i].SetText("")
			if st.ColorPicker.Kind == colorpicker.KindSyntax && st.ColorPicker.OpenIdx == i {
				st.ColorPicker.Close()
			}
			changed = true
		}
	}
	for i := range st.SyntaxSwatchBtns {
		for st.SyntaxSwatchBtns[i].Clicked(gtx) {
			if st.ColorPicker.Kind == colorpicker.KindSyntax && st.ColorPicker.OpenIdx == i {
				st.ColorPicker.Close()
			} else {
				base := theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes).Syntax
				if ov, ok := st.Draft.SyntaxOverrides[st.Draft.Theme]; ok {
					base = theme.ApplySyntaxOverride(base, ov)
				}
				st.ColorPicker.Open(colorpicker.KindSyntax, i, theme.TokenColorTable[i].GetBase(base), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	if st.ColorPicker.IsOpen() {
		cur := [3]float32{st.ColorPicker.H, st.ColorPicker.S, st.ColorPicker.V}
		if cur != st.ColorPicker.LastHSV {
			hex := theme.HexFromColor(st.ColorPicker.Color())
			idx := st.ColorPicker.OpenIdx
			switch st.ColorPicker.Kind {
			case colorpicker.KindSyntax:
				st.SyntaxOverrideEditors[idx].SetText(hex)
				st.putOverride(idx, hex)
			case colorpicker.KindTheme:
				if idx >= 0 && idx < len(st.ThemeColorEditors) {
					st.ThemeColorEditors[idx].SetText(hex)
					st.putThemeOverride(idx, hex)
				}
			}
			changed = true
		}
		st.ColorPicker.LastHSV = cur
	}
	for st.ColorPicker.CloseBtn.Clicked(gtx) {
		st.ColorPicker.Close()
		changed = true
	}
	st.syncThemeEditors()
	for i := range st.ThemeColorEditors {
		ed := &st.ThemeColorEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				st.putThemeOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range st.ThemeColorResetBtns {
		for st.ThemeColorResetBtns[i].Clicked(gtx) {
			st.putThemeOverride(i, "")
			st.ThemeColorEditors[i].SetText("")
			if st.ColorPicker.Kind == colorpicker.KindTheme && st.ColorPicker.OpenIdx == i {
				st.ColorPicker.Close()
			}
			changed = true
		}
	}
	for i := range st.ThemeColorSwatchBtns {
		for st.ThemeColorSwatchBtns[i].Clicked(gtx) {
			if st.ColorPicker.Kind == colorpicker.KindTheme && st.ColorPicker.OpenIdx == i {
				st.ColorPicker.Close()
			} else {
				base := theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes)
				if ov, ok := st.Draft.ThemeOverrides[st.Draft.Theme]; ok {
					base = theme.ApplyOverride(base, ov)
				}
				st.ColorPicker.Open(colorpicker.KindTheme, i, theme.PaletteColorTable[i].GetBase(base), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	for st.ThemeColorResetAllBtn.Clicked(gtx) {
		if st.Draft.ThemeOverrides != nil {
			delete(st.Draft.ThemeOverrides, st.Draft.Theme)
			if len(st.Draft.ThemeOverrides) == 0 {
				st.Draft.ThemeOverrides = nil
			}
		}
		for i := range st.ThemeColorEditors {
			st.ThemeColorEditors[i].SetText("")
		}
		changed = true
	}
	for st.ThemeColorsHeaderBtn.Clicked(gtx) {
		st.ThemeColorsExpanded = !st.ThemeColorsExpanded
	}
	for st.SyntaxColorsHeaderBtn.Clicked(gtx) {
		st.SyntaxColorsExpanded = !st.SyntaxColorsExpanded
	}

	for st.SyntaxResetAllBtn.Clicked(gtx) {
		if st.Draft.SyntaxOverrides != nil {
			delete(st.Draft.SyntaxOverrides, st.Draft.Theme)
			if len(st.Draft.SyntaxOverrides) == 0 {
				st.Draft.SyntaxOverrides = nil
			}
		}
		for i := range st.SyntaxOverrideEditors {
			st.SyntaxOverrideEditors[i].SetText("")
		}
		changed = true
	}
	if st.HideTabBar.Update(gtx) {
		changed = true
	}
	if st.HideSidebar.Update(gtx) {
		changed = true
	}
	if st.RestoreTabsOnStartup.Update(gtx) {
		changed = true
	}
	if st.FollowRedirects.Update(gtx) {
		changed = true
	}
	if st.VerifySSL.Update(gtx) {
		changed = true
	}
	if st.KeepAlive.Update(gtx) {
		changed = true
	}
	if st.DisableHTTP2.Update(gtx) {
		changed = true
	}
	if st.CookieJar.Update(gtx) {
		changed = true
	}
	if st.SendConnClose.Update(gtx) {
		changed = true
	}
	if st.WrapLines.Update(gtx) {
		changed = true
	}
	if st.AutoFormatJSON.Update(gtx) {
		changed = true
	}
	if st.AutoFormatJSONRequest.Update(gtx) {
		changed = true
	}
	if st.StripJSONComments.Update(gtx) {
		changed = true
	}
	if st.TrimTrailingWS.Update(gtx) {
		changed = true
	}
	if st.BracketPairColorization.Update(gtx) {
		changed = true
	}

	if changed || resetChanged {
		ui.applyDraftSettings()
		ui.saveState()
	}

	// Anchor the whole settings screen to CursorDefault. The settings
	// content is full of widget.Editor instances (Text Fields) whose
	// hit-area can extend past their visible bounds via hint-inflated
	// gtx.Constraints.Min in material.EditorStyle.Layout. Without this
	// anchor, those areas leak CursorText into adjacent widgets that
	// don't set a cursor of their own (header buttons, category list,
	// the divider, etc.).
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	pointer.CursorDefault.Add(gtx.Ops)

	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return ui.layoutSettingsHeader(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(220))
						return ui.layoutSettingsCategories(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(1, gtx.Constraints.Max.Y)
						paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return ui.layoutSettingsContent(gtx)
						})
					}),
				)
			}),
		)
	})
}

func timeoutStep(current int) int {
	switch {
	case current < 10:
		return 1
	case current < 60:
		return 5
	case current < 300:
		return 30
	default:
		return 60
	}
}

func connsStep(current int) int {
	switch {
	case current < 10:
		return 1
	case current < 100:
		return 10
	case current < 1000:
		return 50
	default:
		return 100
	}
}

func methodGrid(th *material.Theme, st *SettingsEditorState, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(2))
	children := make([]layout.FlexChild, 0, len(settings.Methods)*2)
	for i, m := range settings.Methods {
		i, m := i, m
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.DefaultMethodBtn[i], func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				borderC := theme.Border
				borderW := gtx.Dp(unit.Dp(1))
				active := st.Draft.DefaultMethod == m
				if active {
					borderC = theme.Accent
					borderW = gtx.Dp(unit.Dp(2))
				} else if st.DefaultMethodBtn[i].Hovered() {
					borderC = theme.BorderLight
				}
				outer := clip.UniformRRect(image.Rectangle{Max: size}, 4)
				paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
				inner := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
				paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(inner, 3).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := widgets.MonoLabel(th, unit.Sp(11), m)
					lbl.Color = theme.MethodColor(m)
					if active {
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				})
			})
		}))
		if i < len(settings.Methods)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func acceptEncodingGrid(th *material.Theme, st *SettingsEditorState, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(4))
	children := make([]layout.FlexChild, 0, len(acceptEncodingOptions)*2)
	for i, opt := range acceptEncodingOptions {
		i, opt := i, opt
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.AcceptEncodingBtn[i], func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				borderC := theme.Border
				borderW := gtx.Dp(unit.Dp(1))
				active := st.Draft.DefaultAcceptEncoding == opt.Value
				if active {
					borderC = theme.Accent
					borderW = gtx.Dp(unit.Dp(2))
				} else if st.AcceptEncodingBtn[i].Hovered() {
					borderC = theme.BorderLight
				}
				outer := clip.UniformRRect(image.Rectangle{Max: size}, 4)
				paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
				inner := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
				paint.FillShape(gtx.Ops, theme.BgField, clip.UniformRRect(inner, 3).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(11), opt.Label)
					lbl.Color = theme.Fg
					if active {
						lbl.Font.Weight = font.Bold
					}
					return lbl.Layout(gtx)
				})
			})
		}))
		if i < len(acceptEncodingOptions)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func (ui *AppUI) layoutSettingsHeader(gtx layout.Context) layout.Dimensions {
	st := ui.SettingsState
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.BackBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := theme.Border
				if st.BackBtn.Hovered() {
					bg = theme.BorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
					return widgets.IconBack.Layout(gtx, ui.Theme.Fg)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(18), "Settings")
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.ResetBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(140)), gtx.Dp(unit.Dp(32)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := theme.Border
				if st.ResetBtn.Hovered() {
					bg = theme.BorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(13), "Reset to defaults")
					lbl.Color = ui.Theme.Fg
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			})
		}),
	)
}

func (ui *AppUI) layoutSettingsCategories(gtx layout.Context) layout.Dimensions {
	st := ui.SettingsState
	children := make([]layout.FlexChild, 0, len(settingsCategories))
	for i, name := range settingsCategories {
		i, name := i, name
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &st.CategoryBtn[i], func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					bg := theme.Transparent
					fg := theme.FgMuted
					if st.Category == i {
						bg = theme.BgHover
						fg = ui.Theme.Fg
					} else if st.CategoryBtn[i].Hovered() {
						bg = theme.BgSecondary
					}
					rect := clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(32)))}, 4)
					paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(13), name)
						lbl.Color = fg
						if st.Category == i {
							lbl.Font.Weight = font.Bold
						}
						return lbl.Layout(gtx)
					})
				})
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (ui *AppUI) layoutSettingsContent(gtx layout.Context) layout.Dimensions {
	var sections []layout.Widget
	switch ui.SettingsState.Category {
	case 0:
		sections = ui.sectionsAppearance()
	case 1:
		sections = ui.sectionsSizes()
	case 2:
		sections = ui.sectionsHTTP()
	case 3:
		sections = ui.sectionsAdvanced()
	}
	return material.List(ui.Theme, &ui.SettingsState.ContentList).Layout(gtx, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, sections[i])
	})
}

func (ui *AppUI) sectionsAppearance() []layout.Widget {
	st := ui.SettingsState
	def := model.DefaultSettings()
	defName := "Dark+"
	for _, t := range theme.Registry {
		if t.ID == def.Theme {
			defName = t.Name
			break
		}
	}
	tabHint := "Hide the row of request tabs above the editor. " + defaultShownHidden(def.HideTabBar)
	sideHint := "Hide the collections/environments sidebar. " + defaultShownHidden(def.HideSidebar)
	restoreHint := "Reopen previously open tabs when the app starts. " + defaultOnOff(def.RestoreTabsOnStartup)
	activeThemeName := defName
	for _, t := range theme.Registry {
		if t.ID == st.Draft.Theme {
			activeThemeName = t.Name
			break
		}
	}
	for _, c := range st.Draft.CustomThemes {
		if c.ID == st.Draft.Theme {
			activeThemeName = c.Name
			break
		}
	}
	widgets := []layout.Widget{
		settingsSectionTitle(ui.Theme, "Visibility"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.HideTabBar)
			return settingsSwitchRow(ui.Theme, "Hide tab bar", tabHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.HideSidebar)
			return settingsSwitchRow(ui.Theme, "Hide sidebar", sideHint, sw.Layout)(gtx)
		},
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Startup"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.RestoreTabsOnStartup)
			return settingsSwitchRow(ui.Theme, "Restore tabs on startup", restoreHint, sw.Layout)(gtx)
		},
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Color theme"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("VS Code–inspired themes. Default: %s.", defName)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return themeGrid(ui.Theme, st, gtx)
		},
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return ui.layoutNewThemeRow(gtx, st)
		},
		spacerH(20),
		spoilerHeader(ui.Theme, &st.ThemeColorsHeaderBtn, &st.ThemeColorResetAllBtn,
			"Customize colors — "+activeThemeName, st.ThemeColorsExpanded),
	}
	if st.ThemeColorsExpanded {
		widgets = append(widgets,
			spacerH(4),
			settingsHint(ui.Theme, "Type a hex color (e.g. #1F1F1F) or click the swatch for a picker. Empty = theme default."),
			spacerH(8),
		)
		for i := range theme.PaletteColorTable {
			idx := i
			widgets = append(widgets, themeColorRow(ui.Theme, st, idx))
			widgets = append(widgets, spacerH(4))
		}
		widgets = append(widgets,
			spacerH(4),
			func(gtx layout.Context) layout.Dimensions {
				size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
				paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
				return layout.Dimensions{Size: size}
			},
			spacerH(8),
			func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(ui.Theme, unit.Sp(11), "Syntax")
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			},
			spacerH(8),
		)
		for i := range theme.TokenColorTable {
			idx := i
			widgets = append(widgets, syntaxColorRow(ui.Theme, st, idx))
			if idx < len(theme.TokenColorTable)-1 {
				widgets = append(widgets, spacerH(4))
			}
		}
	}
	return widgets
}

func spoilerHeader(th *material.Theme, headerBtn, resetBtn *widget.Clickable, title string, expanded bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, headerBtn, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							chev := "▶"
							if expanded {
								chev = "▼"
							}
							lbl := material.Label(th, unit.Sp(10), chev)
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(th, unit.Sp(13), title)
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
					)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return material.Clickable(gtx, resetBtn, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "Reset all")
						lbl.Color = theme.FgMuted
						if resetBtn.Hovered() {
							lbl.Color = theme.Accent
						}
						return lbl.Layout(gtx)
					})
				})
			}),
		)
	}
}

func themeColorRow(th *material.Theme, st *SettingsEditorState, idx int) layout.Widget {
	entry := theme.PaletteColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		base := theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes)
		if ov, ok := st.Draft.ThemeOverrides[st.Draft.Theme]; ok {
			base = theme.ApplyOverride(base, ov)
		}
		swatchColor := entry.GetBase(base)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.ThemeColorSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if st.ColorPicker.Kind == colorpicker.KindTheme && st.ColorPicker.OpenIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, theme.Accent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := theme.BorderLight
						if st.ThemeColorSwatchBtns[idx].Hovered() {
							borderC = theme.Accent
						}
						paint.FillShape(gtx.Ops, borderC, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					}
					inner := image.Rect(border, border, size.X-border, size.Y-border)
					paint.FillShape(gtx.Ops, swatchColor, clip.UniformRRect(inner, 2).Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), entry.Label)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(110))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return widgets.TextField(gtx, th, &st.ThemeColorEditors[idx], theme.HexFromColor(entry.GetBase(theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes))), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.ThemeColorResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := theme.BgField
					if st.ThemeColorResetBtns[idx].Hovered() {
						bg = theme.BgHover
					}
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						isz := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(isz, isz)
						gtx.Constraints.Max = gtx.Constraints.Min
						return widgets.IconRefresh.Layout(gtx, theme.FgMuted)
					})
				})
			}),
		)
	}
}

func syntaxColorRow(th *material.Theme, st *SettingsEditorState, idx int) layout.Widget {
	entry := theme.TokenColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		basePalette := theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes).Syntax
		if ov, ok := st.Draft.SyntaxOverrides[st.Draft.Theme]; ok {
			basePalette = theme.ApplySyntaxOverride(basePalette, ov)
		}
		swatchColor := entry.GetBase(basePalette)

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.SyntaxSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if st.ColorPicker.Kind == colorpicker.KindSyntax && st.ColorPicker.OpenIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, theme.Accent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := theme.BorderLight
						if st.SyntaxSwatchBtns[idx].Hovered() {
							borderC = theme.Accent
						}
						paint.FillShape(gtx.Ops, borderC, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					}
					inner := image.Rect(border, border, size.X-border, size.Y-border)
					paint.FillShape(gtx.Ops, swatchColor, clip.UniformRRect(inner, 2).Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				})
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), entry.Label)
				return lbl.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.X = gtx.Dp(unit.Dp(110))
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return widgets.TextField(gtx, th, &st.SyntaxOverrideEditors[idx], theme.HexFromColor(entry.GetBase(theme.PaletteFor(st.Draft.Theme, st.Draft.CustomThemes).Syntax)), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &st.SyntaxResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := theme.BgField
					if st.SyntaxResetBtns[idx].Hovered() {
						bg = theme.BgHover
					}
					paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						isz := gtx.Dp(unit.Dp(14))
						gtx.Constraints.Min = image.Pt(isz, isz)
						gtx.Constraints.Max = gtx.Constraints.Min
						return widgets.IconRefresh.Layout(gtx, theme.FgMuted)
					})
				})
			}),
		)
	}
}

func (ui *AppUI) sectionsSizes() []layout.Widget {
	st := ui.SettingsState
	def := model.DefaultSettings()
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "UI text size"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Scales all UI text. Default: %d pt.", def.UITextSize)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.UISizeDec, &st.UISizeInc, &st.UISizeEditor, "pt"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Body text size"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Size of the request and response body editors. Default: %d pt.", def.BodyTextSize)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.BodySizeDec, &st.BodySizeInc, &st.BodySizeEditor, "pt"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "UI scale"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Overall size of layout spacing and controls. Default: %.2fx.", def.UIScale)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.UIScaleDec, &st.UIScaleInc, &st.UIScaleEditor, "x"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Response body padding"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Inner padding around the response body text. Same for wrap and no-wrap modes. Default: %d px.", def.ResponseBodyPadding)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.BodyPaddingDec, &st.BodyPaddingInc, &st.BodyPaddingEditor, "px"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Default request/response split"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Initial width ratio of the request pane in new tabs. Default: %.0f%%.", def.DefaultSplitRatio*100)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.SplitRatioDec, &st.SplitRatioInc, &st.SplitRatioEditor, "%"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Adaptive stack breakpoint"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Stack request and response panes vertically when the tab content area is narrower than this width. Set to 0 to always keep them side-by-side. Default: %d dp.", def.StackBreakpointDp)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.StackBpDec, &st.StackBpInc, &st.StackBpEditor, "dp"),
		spacerH(20),
		settingsSectionTitle(ui.Theme, "Default sidebar width"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Initial width of the collections/environments sidebar on first launch. Existing windows keep their dragged width. Default: %d px.", def.DefaultSidebarWidthPx)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.SidebarWidthDec, &st.SidebarWidthInc, &st.SidebarWidthEditor, "px"),
	}
}

func (ui *AppUI) sectionsHTTP() []layout.Widget {
	st := ui.SettingsState
	def := model.DefaultSettings()
	redirectHint := "Follow HTTP 3xx redirects automatically. " + defaultOnOff(def.FollowRedirects)
	verifyHint := "Verify TLS certificates for HTTPS requests. Disable only for local dev against self-signed certs. " + defaultOnOff(def.VerifySSL)
	keepAliveHint := "Reuse TCP connections across requests to the same host. " + defaultOnOff(def.KeepAlive)
	http2Hint := "Force HTTP/1.1 only — disables HTTP/2 ALPN negotiation on TLS connections. " + defaultOnOff(def.DisableHTTP2)
	cookieHint := "Persist cookies set by the server and resend them on subsequent requests to the same host (in-memory only, cleared on app exit). " + defaultOnOff(def.CookieJarEnabled)
	connCloseHint := "Send Connection: close on every request and tear down the TCP connection after the response. Useful for debugging. " + defaultOnOff(def.SendConnectionClose)
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "Request timeout"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Cancel a request if no response arrives in this many seconds. 0 = no timeout. Default: %d s.", def.RequestTimeoutSec)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.TimeoutDec, &st.TimeoutInc, &st.TimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Connect timeout"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum time to establish a TCP connection. 0 = system default. Default: %d s.", def.ConnectTimeoutSec)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.ConnectTimeoutDec, &st.ConnectTimeoutInc, &st.ConnectTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "TLS handshake timeout"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum time waiting for the TLS handshake. 0 = system default. Default: %d s.", def.TLSHandshakeTimeoutSec)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.TLSTimeoutDec, &st.TLSTimeoutInc, &st.TLSTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Idle connection timeout"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Close idle keep-alive connections after this many seconds. 0 = never. Default: %d s.", def.IdleConnTimeoutSec)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.IdleTimeoutDec, &st.IdleTimeoutInc, &st.IdleTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default request method"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Method assigned to newly created tabs. Default: %s.", def.DefaultMethod)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return methodGrid(ui.Theme, st, gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default User-Agent"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Sent on every request unless overridden by a per-request header. Default: %s.", def.UserAgent)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return widgets.TextField(gtx, ui.Theme, &st.UserAgentEditor, "User-Agent", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Redirects"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.FollowRedirects)
			return settingsSwitchRow(ui.Theme, "Follow redirects", redirectHint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum redirect chain length. 0 = unlimited. Default: %d.", def.MaxRedirects)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.MaxRedirectsDec, &st.MaxRedirectsInc, &st.MaxRedirectsEditor, ""),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "TLS"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.VerifySSL)
			return settingsSwitchRow(ui.Theme, "Verify SSL certificates", verifyHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Connection"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.KeepAlive)
			return settingsSwitchRow(ui.Theme, "Keep-Alive", keepAliveHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.DisableHTTP2)
			return settingsSwitchRow(ui.Theme, "Disable HTTP/2", http2Hint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.SendConnClose)
			return settingsSwitchRow(ui.Theme, "Send Connection: close", connCloseHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.CookieJar)
			return settingsSwitchRow(ui.Theme, "Cookie jar", cookieHint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum concurrent connections per host. 0 = unlimited. Default: %d.", def.MaxConnsPerHost)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.MaxConnsDec, &st.MaxConnsInc, &st.MaxConnsEditor, ""),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default Accept-Encoding"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Sent on every request unless overridden by a per-request header. \"off\" omits the header (Go will then add gzip automatically and transparently decode it). Default: %q.", def.DefaultAcceptEncoding)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return acceptEncodingGrid(ui.Theme, st, gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "HTTP proxy"),
		spacerH(4),
		settingsHint(ui.Theme, "Send all requests through this proxy. Format: http://host:port or http://user:pass@host:port. Leave empty to disable."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(360))
			return widgets.TextField(gtx, ui.Theme, &st.ProxyEditor, "http://proxy.local:8080", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Default headers"),
		spacerH(4),
		settingsHint(ui.Theme, "One per line, format \"Header: value\". Added to every request unless the tab sets the same header. Lines starting with # are comments."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(480))
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(96))
			return widgets.TextField(gtx, ui.Theme, &st.DefaultHdrEdit, "Accept: application/json", true, nil, 0, unit.Sp(13))
		},
	}
}

func (ui *AppUI) sectionsAdvanced() []layout.Widget {
	st := ui.SettingsState
	def := model.DefaultSettings()
	wrapHint := "Wrap long lines by default in new editors. " + defaultOnOff(def.WrapLinesDefault)
	autoFmtHint := "Pretty-print JSON responses in the preview viewer. Disable to display raw bytes as received. " + defaultOnOff(def.AutoFormatJSON)
	autoFmtReqHint := "Pretty-print the JSON request body before sending if it parses as valid JSON. Uses the JSON indent setting. " + defaultOnOff(def.AutoFormatJSONRequest)
	stripHint := "Remove // line comments from JSON request bodies before sending if the result is valid JSON. " + defaultOnOff(def.StripJSONComments)
	trimHint := "Strip trailing spaces and tabs from each line of the request body before sending. " + defaultOnOff(def.TrimTrailingWhitespace)
	bracketHint := "Color matched brackets in nested JSON by depth, like VS Code. " + defaultOnOff(def.BracketPairColorization)
	return []layout.Widget{
		settingsSectionTitle(ui.Theme, "JSON indent"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Spaces per level in the JSON pretty-printer. 0 = minified. Default: %d.", def.JSONIndentSpaces)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.JSONIndentDec, &st.JSONIndentInc, &st.JSONIndentEditor, ""),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Response preview cap"),
		spacerH(4),
		settingsHint(ui.Theme, fmt.Sprintf("Maximum response size loaded into the preview editor before 'Load more' is required. Default: %d MB.", def.PreviewMaxMB)),
		spacerH(8),
		stepperEditableRow(ui.Theme, &st.PreviewMaxDec, &st.PreviewMaxInc, &st.PreviewMaxEditor, "MB"),
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Editors"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.WrapLines)
			return settingsSwitchRow(ui.Theme, "Wrap long lines by default", wrapHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "JSON handling"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.AutoFormatJSON)
			return settingsSwitchRow(ui.Theme, "Auto-format JSON responses", autoFmtHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.AutoFormatJSONRequest)
			return settingsSwitchRow(ui.Theme, "Auto-format JSON request before send", autoFmtReqHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.StripJSONComments)
			return settingsSwitchRow(ui.Theme, "Strip // comments before send", stripHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Body editor"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.TrimTrailingWS)
			return settingsSwitchRow(ui.Theme, "Trim trailing whitespace before send", trimHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(ui.Theme, "Syntax coloring"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(ui.Theme, &st.BracketPairColorization)
			return settingsSwitchRow(ui.Theme, "Bracket pair colorization", bracketHint, sw.Layout)(gtx)
		},
	}
}

func spacerH(h int) layout.Widget {
	return layout.Spacer{Height: unit.Dp(float32(h))}.Layout
}

func settingsHint(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(11), text)
		lbl.Color = theme.FgMuted
		return lbl.Layout(gtx)
	}
}

func defaultShownHidden(hidden bool) string {
	if hidden {
		return "Default: hidden."
	}
	return "Default: shown."
}

func defaultOnOff(on bool) string {
	if on {
		return "Default: on."
	}
	return "Default: off."
}

func settingsSectionTitle(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		lbl := material.Label(th, unit.Sp(13), text)
		lbl.Font.Weight = font.Bold
		return lbl.Layout(gtx)
	}
}

func stepperEditableRow(th *material.Theme, dec, inc *widget.Clickable, ed *widget.Editor, unit_ string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(stepperBtn(th, dec, "-")),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(80))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				return widgets.TextField(gtx, th, ed, "", true, nil, 0, unit.Sp(13))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if unit_ == "" {
					return layout.Dimensions{}
				}
				lbl := material.Label(th, unit.Sp(12), unit_)
				lbl.Color = theme.FgMuted
				return lbl.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(stepperBtn(th, inc, "+")),
		)
	}
}

func intStepperUpdate(gtx layout.Context, ed *widget.Editor, current, lo, hi int) (int, bool) {
	if !ed.SingleLine {
		ed.SingleLine = true
		ed.Submit = true
	}
	if !gtx.Focused(ed) {
		txt := strconv.Itoa(current)
		if ed.Text() != txt {
			ed.SetText(txt)
		}
	}
	for {
		ev, ok := ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			s := strings.TrimSpace(ed.Text())
			s = strings.TrimSuffix(s, "%")
			if v, err := strconv.Atoi(s); err == nil {
				if v < lo {
					v = lo
				}
				if v > hi {
					v = hi
				}
				ed.SetText(strconv.Itoa(v))
				if v != current {
					return v, true
				}
			} else {
				ed.SetText(strconv.Itoa(current))
			}
		}
	}
	return current, false
}

func floatStepperUpdate(gtx layout.Context, ed *widget.Editor, current, lo, hi float32, format string, multiplier float32) (float32, bool) {
	if !ed.SingleLine {
		ed.SingleLine = true
		ed.Submit = true
	}
	displayed := current * multiplier
	if !gtx.Focused(ed) {
		txt := fmt.Sprintf(format, displayed)
		if ed.Text() != txt {
			ed.SetText(txt)
		}
	}
	for {
		ev, ok := ed.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			s := strings.TrimSpace(ed.Text())
			s = strings.TrimSuffix(s, "%")
			s = strings.TrimSuffix(s, "x")
			s = strings.TrimSpace(s)
			if v, err := strconv.ParseFloat(s, 32); err == nil {
				fv := float32(v) / multiplier
				if fv < lo {
					fv = lo
				}
				if fv > hi {
					fv = hi
				}
				ed.SetText(fmt.Sprintf(format, fv*multiplier))
				if fv != current {
					return fv, true
				}
			} else {
				ed.SetText(fmt.Sprintf(format, displayed))
			}
		}
	}
	return current, false
}

func stepperBtn(th *material.Theme, btn *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			bg := theme.Border
			if btn.Hovered() {
				bg = theme.BorderLight
			}
			paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(14), label)
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			})
		})
	}
}

func styledSwitch(th *material.Theme, b *widget.Bool) material.SwitchStyle {
	sw := material.Switch(th, b, "")
	sw.Color.Disabled = theme.Mix(theme.Bg, theme.Fg, 0.55)
	sw.Color.Track = theme.Mix(theme.Bg, theme.Fg, 0.3)
	return sw
}

func settingsSwitchRow(th *material.Theme, title, hint string, control layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(13), title)
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), hint)
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
				)
			}),
			layout.Rigid(control),
		)
	}
}

func (ui *AppUI) layoutNewThemeRow(gtx layout.Context, st *SettingsEditorState) layout.Dimensions {
	if !st.NewThemeDialogOpen {
		return layout.Dimensions{}
	}
	return widget.Border{
		Color:        theme.Border,
		CornerRadius: unit.Dp(4),
		Width:        unit.Dp(1),
	}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(10)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(13), "Create new theme")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(11), "Name")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.TextField(gtx, ui.Theme, &st.NewThemeNameEditor, "My theme", true, nil, 0, unit.Sp(12))
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(ui.Theme, unit.Sp(11), "Based on")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.layoutBaseThemePicker(gtx, st)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.Theme, &st.NewThemeCreateBtn, "Create")
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
							return btn.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(ui.Theme, &st.NewThemeCancelBtn, "Cancel")
							btn.Background = theme.Border
							btn.Color = ui.Theme.Fg
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
							return btn.Layout(gtx)
						}),
					)
				}),
			)
		})
	})
}

func (ui *AppUI) layoutBaseThemePicker(gtx layout.Context, st *SettingsEditorState) layout.Dimensions {
	tileW := gtx.Dp(unit.Dp(110))
	tileH := gtx.Dp(unit.Dp(40))
	gap := gtx.Dp(unit.Dp(6))
	perRow := (gtx.Constraints.Max.X + gap) / (tileW + gap)
	if perRow < 1 {
		perRow = 1
	}
	var rows []layout.FlexChild
	for i := 0; i < len(theme.Registry); i += perRow {
		end := i + perRow
		if end > len(theme.Registry) {
			end = len(theme.Registry)
		}
		slice := theme.Registry[i:end]
		baseIdx := i
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cols []layout.FlexChild
			for j, t := range slice {
				j, t := j, t
				cols = append(cols, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.Clickable(gtx, &st.NewThemeBaseBtns[baseIdx+j], func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(tileW, tileH)
						gtx.Constraints.Max = gtx.Constraints.Min
						bg := t.Palette.Bg
						paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
						if st.NewThemeBaseID == t.ID {
							paint.FillShape(gtx.Ops, theme.Accent, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Path(gtx.Ops), Width: 2}.Op())
						} else {
							widgets.PaintBorder1px(gtx, gtx.Constraints.Min, theme.BorderLight)
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(11), t.Name)
							lbl.Color = t.Palette.Fg
							return lbl.Layout(gtx)
						})
					})
				}))
				if j < len(slice)-1 {
					cols = append(cols, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cols...)
		}))
		rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func themeGrid(th *material.Theme, st *SettingsEditorState, gtx layout.Context) layout.Dimensions {
	tileW := gtx.Dp(unit.Dp(150))
	tileH := gtx.Dp(unit.Dp(90))
	gap := gtx.Dp(unit.Dp(10))
	perRow := (gtx.Constraints.Max.X + gap) / (tileW + gap)
	if perRow < 1 {
		perRow = 1
	}

	type tileEntry struct {
		def      theme.Def
		btn      *widget.Clickable
		delBtn   *widget.Clickable
		isCustom bool
		isAdd    bool
	}
	var entries []tileEntry
	for i := range theme.Registry {
		entries = append(entries, tileEntry{def: theme.Registry[i], btn: &st.ThemeBtns[i]})
	}
	for i, c := range st.Draft.CustomThemes {
		if i >= len(st.CustomThemeBtns) || i >= len(st.CustomThemeDelBtns) {
			break
		}
		entries = append(entries, tileEntry{
			def:      theme.Def{ID: c.ID, Name: c.Name, Palette: theme.PaletteFor(c.ID, st.Draft.CustomThemes)},
			btn:      &st.CustomThemeBtns[i],
			delBtn:   &st.CustomThemeDelBtns[i],
			isCustom: true,
		})
	}
	entries = append(entries, tileEntry{btn: &st.NewThemeBtn, isAdd: true})

	var rows []layout.FlexChild
	for i := 0; i < len(entries); i += perRow {
		end := i + perRow
		if end > len(entries) {
			end = len(entries)
		}
		slice := entries[i:end]
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cols []layout.FlexChild
			for j, e := range slice {
				j, e := j, e
				var w layout.Widget
				if e.isAdd {
					w = themeTileFixedNew(th, e.btn, st.NewThemeDialogOpen, tileW, tileH)
				} else {
					w = themeTileFixedCustom(th, e.btn, e.delBtn, e.def, st.Draft.Theme == e.def.ID, e.isCustom, tileW, tileH)
				}
				cols = append(cols, layout.Rigid(w))
				if j < len(slice)-1 {
					cols = append(cols, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
				}
			}
			return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, cols...)
		}))
		rows = append(rows, layout.Rigid(layout.Spacer{Height: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

func themeTileFixedNew(th *material.Theme, btn *widget.Clickable, active bool, tileW, tileH int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(tileW, tileH)
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			borderC := theme.Border
			borderW := gtx.Dp(unit.Dp(1))
			if active {
				borderC = theme.Accent
				borderW = gtx.Dp(unit.Dp(2))
			} else if btn.Hovered() {
				borderC = theme.BorderLight
			}
			outer := clip.UniformRRect(image.Rectangle{Max: size}, 6)
			paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
			innerRect := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
			inner := clip.UniformRRect(innerRect, 5)
			paint.FillShape(gtx.Ops, theme.BgField, inner.Op(gtx.Ops))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(28), "+")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(th, unit.Sp(11), "New theme")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
				)
			})
		})
	}
}

func themeTileFixedCustom(th *material.Theme, btn *widget.Clickable, delBtn *widget.Clickable, def theme.Def, active bool, isCustom bool, tileW, tileH int) layout.Widget {
	tileFn := themeTileFixed(th, btn, def, active, tileW, tileH)
	if !isCustom || delBtn == nil {
		return tileFn
	}
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Stack{}.Layout(gtx,
			layout.Stacked(tileFn),
			layout.Stacked(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Right: unit.Dp(4), Left: unit.Dp(float32(tileW-22-4) / gtx.Metric.PxPerDp)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					sz := gtx.Dp(unit.Dp(20))
					gtx.Constraints.Min = image.Pt(sz, sz)
					gtx.Constraints.Max = gtx.Constraints.Min
					return material.Clickable(gtx, delBtn, func(gtx layout.Context) layout.Dimensions {
						bg := theme.BgField
						if delBtn.Hovered() {
							bg = theme.Danger
						}
						paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 3).Op(gtx.Ops))
						widgets.PaintBorder1px(gtx, gtx.Constraints.Min, theme.Border)
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							isz := gtx.Dp(unit.Dp(12))
							gtx.Constraints.Min = image.Pt(isz, isz)
							gtx.Constraints.Max = gtx.Constraints.Min
							col := theme.FgMuted
							if delBtn.Hovered() {
								col = theme.DangerFg
							}
							return widgets.IconClose.Layout(gtx, col)
						})
					})
				})
			}),
		)
	}
}

func themeTileFixed(th *material.Theme, btn *widget.Clickable, def theme.Def, active bool, tileW, tileH int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(tileW, tileH)
			gtx.Constraints.Min = size
			gtx.Constraints.Max = size
			borderC := theme.Border
			borderW := gtx.Dp(unit.Dp(1))
			if active {
				borderC = theme.Accent
				borderW = gtx.Dp(unit.Dp(2))
			} else if btn.Hovered() {
				borderC = theme.BorderLight
			}
			p := def.Palette
			outer := clip.UniformRRect(image.Rectangle{Max: size}, 6)
			paint.FillShape(gtx.Ops, borderC, outer.Op(gtx.Ops))
			innerRect := image.Rect(borderW, borderW, size.X-borderW, size.Y-borderW)
			inner := clip.UniformRRect(innerRect, 5)
			paint.FillShape(gtx.Ops, p.Bg, inner.Op(gtx.Ops))

			stripe := image.Rect(borderW, borderW, size.X-borderW, borderW+gtx.Dp(unit.Dp(16)))
			paint.FillShape(gtx.Ops, p.BgDark, clip.Rect(stripe).Op())

			dot := image.Rect(size.X-gtx.Dp(unit.Dp(20)), size.Y-gtx.Dp(unit.Dp(20)), size.X-gtx.Dp(unit.Dp(10)), size.Y-gtx.Dp(unit.Dp(10)))
			paint.FillShape(gtx.Ops, p.Accent, clip.UniformRRect(dot, 3).Op(gtx.Ops))

			return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(40)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(12), def.Name)
				lbl.Color = p.Fg
				lbl.Font.Weight = font.Bold
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			})
		})
	}
}
