package settings

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/colorpicker"
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

type Editor struct {
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

func NewEditor(current model.AppSettings) *Editor {
	s := &Editor{
		Draft:                 current,
		CategoryBtn:           make([]widget.Clickable, len(settingsCategories)),
		ThemeBtns:             make([]widget.Clickable, len(theme.Registry)),
		DefaultMethodBtn:      make([]widget.Clickable, len(Methods)),
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

func (e *Editor) Apply(host *Host) {
	if e == nil {
		return
	}
	e.Draft.HideTabBar = e.HideTabBar.Value
	e.Draft.HideSidebar = e.HideSidebar.Value
	e.Draft.RestoreTabsOnStartup = e.RestoreTabsOnStartup.Value

	e.Draft.UserAgent = strings.TrimSpace(e.UserAgentEditor.Text())
	e.Draft.Proxy = strings.TrimSpace(e.ProxyEditor.Text())
	e.Draft.FollowRedirects = e.FollowRedirects.Value
	e.Draft.VerifySSL = e.VerifySSL.Value
	e.Draft.KeepAlive = e.KeepAlive.Value
	e.Draft.DisableHTTP2 = e.DisableHTTP2.Value
	e.Draft.CookieJarEnabled = e.CookieJar.Value
	e.Draft.SendConnectionClose = e.SendConnClose.Value
	e.Draft.DefaultHeaders = textToHeaders(e.DefaultHdrEdit.Text())
	e.Draft.WrapLinesDefault = e.WrapLines.Value
	e.Draft.AutoFormatJSON = e.AutoFormatJSON.Value
	e.Draft.AutoFormatJSONRequest = e.AutoFormatJSONRequest.Value
	e.Draft.StripJSONComments = e.StripJSONComments.Value
	e.Draft.TrimTrailingWhitespace = e.TrimTrailingWS.Value
	e.Draft.BracketPairColorization = e.BracketPairColorization.Value

	e.Draft = Sanitize(e.Draft)
	(*host.Current) = e.Draft
	Apply(host.Theme, (*host.Current))
}


func (e *Editor) syncSyntaxEditors() {
	if e.syntaxEditorsThemeID == e.Draft.Theme {
		return
	}
	e.syntaxEditorsThemeID = e.Draft.Theme
	ov := e.Draft.SyntaxOverrides[e.Draft.Theme]
	for i, entry := range theme.TokenColorTable {
		e.SyntaxOverrideEditors[i].SetText(entry.GetOv(ov))
	}
}

func (e *Editor) putOverride(i int, h string) {
	themeID := e.Draft.Theme
	ov := e.Draft.SyntaxOverrides[themeID]
	theme.TokenColorTable[i].SetOv(&ov, h)
	if ov == (model.ThemeSyntaxOverride{}) {
		if e.Draft.SyntaxOverrides != nil {
			delete(e.Draft.SyntaxOverrides, themeID)
			if len(e.Draft.SyntaxOverrides) == 0 {
				e.Draft.SyntaxOverrides = nil
			}
		}
		return
	}
	if e.Draft.SyntaxOverrides == nil {
		e.Draft.SyntaxOverrides = map[string]model.ThemeSyntaxOverride{}
	}
	e.Draft.SyntaxOverrides[themeID] = ov
}

func (e *Editor) syncThemeEditors() {
	if e.themeEditorsThemeID == e.Draft.Theme {
		return
	}
	e.themeEditorsThemeID = e.Draft.Theme
	ov := e.Draft.ThemeOverrides[e.Draft.Theme]
	for i, entry := range theme.PaletteColorTable {
		e.ThemeColorEditors[i].SetText(entry.GetOv(ov))
	}
}

func (e *Editor) putThemeOverride(i int, h string) {
	themeID := e.Draft.Theme
	ov := e.Draft.ThemeOverrides[themeID]
	theme.PaletteColorTable[i].SetOv(&ov, h)
	if ov == (model.ThemeColorOverride{}) {
		if e.Draft.ThemeOverrides != nil {
			delete(e.Draft.ThemeOverrides, themeID)
			if len(e.Draft.ThemeOverrides) == 0 {
				e.Draft.ThemeOverrides = nil
			}
		}
		return
	}
	if e.Draft.ThemeOverrides == nil {
		e.Draft.ThemeOverrides = map[string]model.ThemeColorOverride{}
	}
	e.Draft.ThemeOverrides[themeID] = ov
}

func (e *Editor) Reset() {
	if e == nil {
		return
	}
	def := model.DefaultSettings()
	e.Draft = def
	e.HideTabBar.Value = def.HideTabBar
	e.HideSidebar.Value = def.HideSidebar
	e.RestoreTabsOnStartup.Value = def.RestoreTabsOnStartup

	e.UserAgentEditor.SetText(def.UserAgent)
	e.ProxyEditor.SetText(def.Proxy)
	e.FollowRedirects.Value = def.FollowRedirects
	e.VerifySSL.Value = def.VerifySSL
	e.KeepAlive.Value = def.KeepAlive
	e.DisableHTTP2.Value = def.DisableHTTP2
	e.CookieJar.Value = def.CookieJarEnabled
	e.SendConnClose.Value = def.SendConnectionClose
	e.DefaultHdrEdit.SetText(headersToText(def.DefaultHeaders))
	e.WrapLines.Value = def.WrapLinesDefault
	e.AutoFormatJSON.Value = def.AutoFormatJSON
	e.AutoFormatJSONRequest.Value = def.AutoFormatJSONRequest
	e.StripJSONComments.Value = def.StripJSONComments
	e.TrimTrailingWS.Value = def.TrimTrailingWhitespace
	e.BracketPairColorization.Value = def.BracketPairColorization
}

func (e *Editor) Layout(gtx layout.Context, host *Host) layout.Dimensions {
	if e == nil {
		e = NewEditor((*host.Current))
	}

	for e.BackBtn.Clicked(gtx) {
		host.OnClose()
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	resetChanged := false
	for e.ResetBtn.Clicked(gtx) {
		e.Reset()
		resetChanged = true
	}

	for i := range e.CategoryBtn {
		if e.CategoryBtn[i].Clicked(gtx) {
			if e.Category != i {
				e.Category = i
				e.ContentList.Position = layout.Position{}
			}
		}
	}

	changed := false
	for i := range e.ThemeBtns {
		for e.ThemeBtns[i].Clicked(gtx) {
			tid := theme.Registry[i].ID
			if e.Draft.Theme != tid {
				e.Draft.Theme = tid
				e.ColorPicker.Close()
				changed = true
			}
		}
	}
	if len(e.NewThemeBaseBtns) != len(theme.Registry) {
		e.NewThemeBaseBtns = make([]widget.Clickable, len(theme.Registry))
	}
	if len(e.CustomThemeBtns) != len(e.Draft.CustomThemes) {
		e.CustomThemeBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
		e.CustomThemeDelBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
	}
	for i := range e.CustomThemeBtns {
		for e.CustomThemeBtns[i].Clicked(gtx) {
			if i < len(e.Draft.CustomThemes) {
				tid := e.Draft.CustomThemes[i].ID
				if e.Draft.Theme != tid {
					e.Draft.Theme = tid
					e.ColorPicker.Close()
					changed = true
				}
			}
		}
	}
	deleteIdx := -1
	for i := range e.CustomThemeDelBtns {
		if i >= len(e.CustomThemeDelBtns) {
			break
		}
		if e.CustomThemeDelBtns[i].Clicked(gtx) {
			deleteIdx = i
			for e.CustomThemeDelBtns[i].Clicked(gtx) {
			}
			break
		}
	}
	if deleteIdx >= 0 && deleteIdx < len(e.Draft.CustomThemes) {
		deletedID := e.Draft.CustomThemes[deleteIdx].ID
		e.Draft.CustomThemes = append(e.Draft.CustomThemes[:deleteIdx], e.Draft.CustomThemes[deleteIdx+1:]...)
		e.CustomThemeBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
		e.CustomThemeDelBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
		if e.Draft.Theme == deletedID {
			e.Draft.Theme = "dark"
		}
		changed = true
	}
	for e.NewThemeBtn.Clicked(gtx) {
		e.NewThemeDialogOpen = !e.NewThemeDialogOpen
		if e.NewThemeDialogOpen {
			e.NewThemeNameEditor.SingleLine = true
			e.NewThemeNameEditor.Submit = true
			e.NewThemeNameEditor.SetText("")
			e.NewThemeBaseID = e.Draft.Theme
		}
	}
	for i := range e.NewThemeBaseBtns {
		for e.NewThemeBaseBtns[i].Clicked(gtx) {
			e.NewThemeBaseID = theme.Registry[i].ID
		}
	}
	for e.NewThemeCancelBtn.Clicked(gtx) {
		e.NewThemeDialogOpen = false
	}
	for e.NewThemeCreateBtn.Clicked(gtx) {
		name := strings.TrimSpace(e.NewThemeNameEditor.Text())
		if name == "" {
			continue
		}
		baseID := e.NewThemeBaseID
		if baseID == "" {
			baseID = "dark"
		}
		basePalette := theme.PaletteFor(baseID, e.Draft.CustomThemes)
		newID := "custom-" + persist.NewRandomID()[:8]
		ct := model.CustomTheme{
			ID:      newID,
			Name:    name,
			BasedOn: baseID,
			Palette: theme.PaletteToOverride(basePalette),
			Syntax:  theme.SyntaxToOverride(basePalette.Syntax),
		}
		e.Draft.CustomThemes = append(e.Draft.CustomThemes, ct)
		e.Draft.Theme = newID
		e.NewThemeDialogOpen = false
		e.NewThemeNameEditor.SetText("")
		e.CustomThemeBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
		e.CustomThemeDelBtns = make([]widget.Clickable, len(e.Draft.CustomThemes))
		e.ColorPicker.Close()
		changed = true
	}
	e.syncSyntaxEditors()

	for e.UISizeDec.Clicked(gtx) {
		if e.Draft.UITextSize > 10 {
			e.Draft.UITextSize--
			changed = true
		}
	}
	for e.UISizeInc.Clicked(gtx) {
		if e.Draft.UITextSize < 28 {
			e.Draft.UITextSize++
			changed = true
		}
	}
	for e.BodySizeDec.Clicked(gtx) {
		if e.Draft.BodyTextSize > 10 {
			e.Draft.BodyTextSize--
			changed = true
		}
	}
	for e.BodySizeInc.Clicked(gtx) {
		if e.Draft.BodyTextSize < 28 {
			e.Draft.BodyTextSize++
			changed = true
		}
	}
	for e.UIScaleDec.Clicked(gtx) {
		if e.Draft.UIScale > 0.75 {
			e.Draft.UIScale -= 0.05
			changed = true
		}
	}
	for e.UIScaleInc.Clicked(gtx) {
		if e.Draft.UIScale < 2.0 {
			e.Draft.UIScale += 0.05
			changed = true
		}
	}
	for e.BodyPaddingDec.Clicked(gtx) {
		if e.Draft.ResponseBodyPadding > 0 {
			e.Draft.ResponseBodyPadding--
			changed = true
		}
	}
	for e.BodyPaddingInc.Clicked(gtx) {
		if e.Draft.ResponseBodyPadding < 32 {
			e.Draft.ResponseBodyPadding++
			changed = true
		}
	}
	for e.SplitRatioDec.Clicked(gtx) {
		if e.Draft.DefaultSplitRatio > 0.2 {
			e.Draft.DefaultSplitRatio -= 0.05
			if e.Draft.DefaultSplitRatio < 0.2 {
				e.Draft.DefaultSplitRatio = 0.2
			}
			changed = true
		}
	}
	for e.SplitRatioInc.Clicked(gtx) {
		if e.Draft.DefaultSplitRatio < 0.8 {
			e.Draft.DefaultSplitRatio += 0.05
			if e.Draft.DefaultSplitRatio > 0.8 {
				e.Draft.DefaultSplitRatio = 0.8
			}
			changed = true
		}
	}
	for e.StackBpDec.Clicked(gtx) {
		if e.Draft.StackBreakpointDp <= 400 {
			e.Draft.StackBreakpointDp = 0
		} else {
			e.Draft.StackBreakpointDp -= 50
		}
		changed = true
	}
	for e.StackBpInc.Clicked(gtx) {
		if e.Draft.StackBreakpointDp == 0 {
			e.Draft.StackBreakpointDp = 400
		} else if e.Draft.StackBreakpointDp < 2000 {
			e.Draft.StackBreakpointDp += 50
			if e.Draft.StackBreakpointDp > 2000 {
				e.Draft.StackBreakpointDp = 2000
			}
		}
		changed = true
	}
	for i := range e.DefaultMethodBtn {
		for e.DefaultMethodBtn[i].Clicked(gtx) {
			if e.Draft.DefaultMethod != Methods[i] {
				e.Draft.DefaultMethod = Methods[i]
				changed = true
			}
		}
	}
	for e.MaxConnsDec.Clicked(gtx) {
		step := connsStep(e.Draft.MaxConnsPerHost)
		if e.Draft.MaxConnsPerHost > 0 {
			e.Draft.MaxConnsPerHost -= step
			if e.Draft.MaxConnsPerHost < 0 {
				e.Draft.MaxConnsPerHost = 0
			}
			changed = true
		}
	}
	for e.MaxConnsInc.Clicked(gtx) {
		step := connsStep(e.Draft.MaxConnsPerHost)
		if e.Draft.MaxConnsPerHost < 10000 {
			e.Draft.MaxConnsPerHost += step
			if e.Draft.MaxConnsPerHost > 10000 {
				e.Draft.MaxConnsPerHost = 10000
			}
			changed = true
		}
	}

	for e.TimeoutDec.Clicked(gtx) {
		step := timeoutStep(e.Draft.RequestTimeoutSec)
		if e.Draft.RequestTimeoutSec > 0 {
			e.Draft.RequestTimeoutSec -= step
			if e.Draft.RequestTimeoutSec < 0 {
				e.Draft.RequestTimeoutSec = 0
			}
			changed = true
		}
	}
	for e.TimeoutInc.Clicked(gtx) {
		step := timeoutStep(e.Draft.RequestTimeoutSec)
		if e.Draft.RequestTimeoutSec < 3600 {
			e.Draft.RequestTimeoutSec += step
			if e.Draft.RequestTimeoutSec > 3600 {
				e.Draft.RequestTimeoutSec = 3600
			}
			changed = true
		}
	}
	for e.ConnectTimeoutDec.Clicked(gtx) {
		if e.Draft.ConnectTimeoutSec > 0 {
			e.Draft.ConnectTimeoutSec--
			changed = true
		}
	}
	for e.ConnectTimeoutInc.Clicked(gtx) {
		if e.Draft.ConnectTimeoutSec < 600 {
			e.Draft.ConnectTimeoutSec++
			changed = true
		}
	}
	for e.TLSTimeoutDec.Clicked(gtx) {
		if e.Draft.TLSHandshakeTimeoutSec > 0 {
			e.Draft.TLSHandshakeTimeoutSec--
			changed = true
		}
	}
	for e.TLSTimeoutInc.Clicked(gtx) {
		if e.Draft.TLSHandshakeTimeoutSec < 600 {
			e.Draft.TLSHandshakeTimeoutSec++
			changed = true
		}
	}
	for e.IdleTimeoutDec.Clicked(gtx) {
		step := timeoutStep(e.Draft.IdleConnTimeoutSec)
		if e.Draft.IdleConnTimeoutSec > 0 {
			e.Draft.IdleConnTimeoutSec -= step
			if e.Draft.IdleConnTimeoutSec < 0 {
				e.Draft.IdleConnTimeoutSec = 0
			}
			changed = true
		}
	}
	for e.IdleTimeoutInc.Clicked(gtx) {
		step := timeoutStep(e.Draft.IdleConnTimeoutSec)
		if e.Draft.IdleConnTimeoutSec < 3600 {
			e.Draft.IdleConnTimeoutSec += step
			if e.Draft.IdleConnTimeoutSec > 3600 {
				e.Draft.IdleConnTimeoutSec = 3600
			}
			changed = true
		}
	}
	for e.SidebarWidthDec.Clicked(gtx) {
		if e.Draft.DefaultSidebarWidthPx > 160 {
			e.Draft.DefaultSidebarWidthPx -= 10
			if e.Draft.DefaultSidebarWidthPx < 160 {
				e.Draft.DefaultSidebarWidthPx = 160
			}
			changed = true
		}
	}
	for e.SidebarWidthInc.Clicked(gtx) {
		if e.Draft.DefaultSidebarWidthPx < 1000 {
			e.Draft.DefaultSidebarWidthPx += 10
			if e.Draft.DefaultSidebarWidthPx > 1000 {
				e.Draft.DefaultSidebarWidthPx = 1000
			}
			changed = true
		}
	}
	for i := range e.AcceptEncodingBtn {
		for e.AcceptEncodingBtn[i].Clicked(gtx) {
			if e.Draft.DefaultAcceptEncoding != acceptEncodingOptions[i].Value {
				e.Draft.DefaultAcceptEncoding = acceptEncodingOptions[i].Value
				changed = true
			}
		}
	}
	for e.MaxRedirectsDec.Clicked(gtx) {
		if e.Draft.MaxRedirects > 0 {
			e.Draft.MaxRedirects--
			changed = true
		}
	}
	for e.MaxRedirectsInc.Clicked(gtx) {
		if e.Draft.MaxRedirects < 50 {
			e.Draft.MaxRedirects++
			changed = true
		}
	}
	for e.JSONIndentDec.Clicked(gtx) {
		if e.Draft.JSONIndentSpaces > 0 {
			e.Draft.JSONIndentSpaces--
			changed = true
		}
	}
	for e.JSONIndentInc.Clicked(gtx) {
		if e.Draft.JSONIndentSpaces < 8 {
			e.Draft.JSONIndentSpaces++
			changed = true
		}
	}
	for e.PreviewMaxDec.Clicked(gtx) {
		if e.Draft.PreviewMaxMB > 1 {
			e.Draft.PreviewMaxMB--
			changed = true
		}
	}
	for e.PreviewMaxInc.Clicked(gtx) {
		if e.Draft.PreviewMaxMB < 500 {
			e.Draft.PreviewMaxMB++
			changed = true
		}
	}

	if v, ok := intStepperUpdate(gtx, &e.UISizeEditor, e.Draft.UITextSize, 10, 28); ok {
		e.Draft.UITextSize = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.BodySizeEditor, e.Draft.BodyTextSize, 10, 28); ok {
		e.Draft.BodyTextSize = v
		changed = true
	}
	if v, ok := floatStepperUpdate(gtx, &e.UIScaleEditor, e.Draft.UIScale, 0.75, 2.0, "%.2f", 1.0); ok {
		e.Draft.UIScale = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.BodyPaddingEditor, e.Draft.ResponseBodyPadding, 0, 32); ok {
		e.Draft.ResponseBodyPadding = v
		changed = true
	}
	if v, ok := floatStepperUpdate(gtx, &e.SplitRatioEditor, e.Draft.DefaultSplitRatio, 0.2, 0.8, "%.0f", 100); ok {
		e.Draft.DefaultSplitRatio = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.StackBpEditor, e.Draft.StackBreakpointDp, 0, 2000); ok {
		if v > 0 && v < 400 {
			v = 400
		}
		e.Draft.StackBreakpointDp = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.TimeoutEditor, e.Draft.RequestTimeoutSec, 0, 3600); ok {
		e.Draft.RequestTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.ConnectTimeoutEditor, e.Draft.ConnectTimeoutSec, 0, 600); ok {
		e.Draft.ConnectTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.TLSTimeoutEditor, e.Draft.TLSHandshakeTimeoutSec, 0, 600); ok {
		e.Draft.TLSHandshakeTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.IdleTimeoutEditor, e.Draft.IdleConnTimeoutSec, 0, 3600); ok {
		e.Draft.IdleConnTimeoutSec = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.MaxRedirectsEditor, e.Draft.MaxRedirects, 0, 50); ok {
		e.Draft.MaxRedirects = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.MaxConnsEditor, e.Draft.MaxConnsPerHost, 0, 10000); ok {
		e.Draft.MaxConnsPerHost = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.SidebarWidthEditor, e.Draft.DefaultSidebarWidthPx, 160, 1000); ok {
		e.Draft.DefaultSidebarWidthPx = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.JSONIndentEditor, e.Draft.JSONIndentSpaces, 0, 8); ok {
		e.Draft.JSONIndentSpaces = v
		changed = true
	}
	if v, ok := intStepperUpdate(gtx, &e.PreviewMaxEditor, e.Draft.PreviewMaxMB, 1, 500); ok {
		e.Draft.PreviewMaxMB = v
		changed = true
	}

	for _, ed := range []*widget.Editor{&e.UserAgentEditor, &e.ProxyEditor, &e.DefaultHdrEdit} {
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

	for i := range e.SyntaxOverrideEditors {
		ed := &e.SyntaxOverrideEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				e.putOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range e.SyntaxResetBtns {
		for e.SyntaxResetBtns[i].Clicked(gtx) {
			e.putOverride(i, "")
			e.SyntaxOverrideEditors[i].SetText("")
			if e.ColorPicker.Kind == colorpicker.KindSyntax && e.ColorPicker.OpenIdx == i {
				e.ColorPicker.Close()
			}
			changed = true
		}
	}
	for i := range e.SyntaxSwatchBtns {
		for e.SyntaxSwatchBtns[i].Clicked(gtx) {
			if e.ColorPicker.Kind == colorpicker.KindSyntax && e.ColorPicker.OpenIdx == i {
				e.ColorPicker.Close()
			} else {
				base := theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes).Syntax
				if ov, ok := e.Draft.SyntaxOverrides[e.Draft.Theme]; ok {
					base = theme.ApplySyntaxOverride(base, ov)
				}
				e.ColorPicker.Open(colorpicker.KindSyntax, i, theme.TokenColorTable[i].GetBase(base), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	if e.ColorPicker.IsOpen() {
		cur := [3]float32{e.ColorPicker.H, e.ColorPicker.S, e.ColorPicker.V}
		if cur != e.ColorPicker.LastHSV {
			hex := theme.HexFromColor(e.ColorPicker.Color())
			idx := e.ColorPicker.OpenIdx
			switch e.ColorPicker.Kind {
			case colorpicker.KindSyntax:
				if idx >= 0 && idx < len(e.SyntaxOverrideEditors) && e.SyntaxOverrideEditors[idx].Text() != hex {
					e.SyntaxOverrideEditors[idx].SetText(hex)
					e.putOverride(idx, hex)
					changed = true
				}
			case colorpicker.KindTheme:
				if idx >= 0 && idx < len(e.ThemeColorEditors) && e.ThemeColorEditors[idx].Text() != hex {
					e.ThemeColorEditors[idx].SetText(hex)
					e.putThemeOverride(idx, hex)
					changed = true
				}
			}
		}
		e.ColorPicker.LastHSV = cur
	}
	for e.ColorPicker.CloseBtn.Clicked(gtx) {
		e.ColorPicker.Close()
		changed = true
	}
	e.syncThemeEditors()
	for i := range e.ThemeColorEditors {
		ed := &e.ThemeColorEditors[i]
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				e.putThemeOverride(i, strings.TrimSpace(ed.Text()))
				changed = true
			}
			if _, ok := ev.(widget.SubmitEvent); ok {
				changed = true
			}
		}
	}
	for i := range e.ThemeColorResetBtns {
		for e.ThemeColorResetBtns[i].Clicked(gtx) {
			e.putThemeOverride(i, "")
			e.ThemeColorEditors[i].SetText("")
			if e.ColorPicker.Kind == colorpicker.KindTheme && e.ColorPicker.OpenIdx == i {
				e.ColorPicker.Close()
			}
			changed = true
		}
	}
	for i := range e.ThemeColorSwatchBtns {
		for e.ThemeColorSwatchBtns[i].Clicked(gtx) {
			if e.ColorPicker.Kind == colorpicker.KindTheme && e.ColorPicker.OpenIdx == i {
				e.ColorPicker.Close()
			} else {
				base := theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes)
				if ov, ok := e.Draft.ThemeOverrides[e.Draft.Theme]; ok {
					base = theme.ApplyOverride(base, ov)
				}
				e.ColorPicker.Open(colorpicker.KindTheme, i, theme.PaletteColorTable[i].GetBase(base), colorpicker.Anchor{X: widgets.GlobalPointerPos.X, Y: widgets.GlobalPointerPos.Y})
			}
			changed = true
		}
	}
	for e.ThemeColorResetAllBtn.Clicked(gtx) {
		if e.Draft.ThemeOverrides != nil {
			delete(e.Draft.ThemeOverrides, e.Draft.Theme)
			if len(e.Draft.ThemeOverrides) == 0 {
				e.Draft.ThemeOverrides = nil
			}
		}
		for i := range e.ThemeColorEditors {
			e.ThemeColorEditors[i].SetText("")
		}
		changed = true
	}
	for e.ThemeColorsHeaderBtn.Clicked(gtx) {
		e.ThemeColorsExpanded = !e.ThemeColorsExpanded
	}
	for e.SyntaxColorsHeaderBtn.Clicked(gtx) {
		e.SyntaxColorsExpanded = !e.SyntaxColorsExpanded
	}

	for e.SyntaxResetAllBtn.Clicked(gtx) {
		if e.Draft.SyntaxOverrides != nil {
			delete(e.Draft.SyntaxOverrides, e.Draft.Theme)
			if len(e.Draft.SyntaxOverrides) == 0 {
				e.Draft.SyntaxOverrides = nil
			}
		}
		for i := range e.SyntaxOverrideEditors {
			e.SyntaxOverrideEditors[i].SetText("")
		}
		changed = true
	}
	if e.HideTabBar.Update(gtx) {
		changed = true
	}
	if e.HideSidebar.Update(gtx) {
		changed = true
	}
	if e.RestoreTabsOnStartup.Update(gtx) {
		changed = true
	}
	if e.FollowRedirects.Update(gtx) {
		changed = true
	}
	if e.VerifySSL.Update(gtx) {
		changed = true
	}
	if e.KeepAlive.Update(gtx) {
		changed = true
	}
	if e.DisableHTTP2.Update(gtx) {
		changed = true
	}
	if e.CookieJar.Update(gtx) {
		changed = true
	}
	if e.SendConnClose.Update(gtx) {
		changed = true
	}
	if e.WrapLines.Update(gtx) {
		changed = true
	}
	if e.AutoFormatJSON.Update(gtx) {
		changed = true
	}
	if e.AutoFormatJSONRequest.Update(gtx) {
		changed = true
	}
	if e.StripJSONComments.Update(gtx) {
		changed = true
	}
	if e.TrimTrailingWS.Update(gtx) {
		changed = true
	}
	if e.BracketPairColorization.Update(gtx) {
		changed = true
	}

	if changed || resetChanged {
		e.Apply(host)
		host.OnSave()
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
				return e.layoutHeader(gtx, host)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(16)}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Dp(unit.Dp(200))
						gtx.Constraints.Max.X = gtx.Dp(unit.Dp(220))
						return e.layoutCategories(gtx, host)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(1, gtx.Constraints.Max.Y)
						paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
						return layout.Dimensions{Size: size}
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return e.layoutContent(gtx, host)
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

func methodGrid(th *material.Theme, e *Editor, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(2))
	children := make([]layout.FlexChild, 0, len(Methods)*2)
	for i, m := range Methods {
		i, m := i, m
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &e.DefaultMethodBtn[i], func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				borderC := theme.Border
				borderW := gtx.Dp(unit.Dp(1))
				active := e.Draft.DefaultMethod == m
				if active {
					borderC = theme.Accent
					borderW = gtx.Dp(unit.Dp(2))
				} else if e.DefaultMethodBtn[i].Hovered() {
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
		if i < len(Methods)-1 {
			children = append(children, layout.Rigid(layout.Spacer{Width: unit.Dp(float32(gap) / gtx.Metric.PxPerDp)}.Layout))
		}
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
}

func acceptEncodingGrid(th *material.Theme, e *Editor, gtx layout.Context) layout.Dimensions {
	height := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(4))
	children := make([]layout.FlexChild, 0, len(acceptEncodingOptions)*2)
	for i, opt := range acceptEncodingOptions {
		i, opt := i, opt
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &e.AcceptEncodingBtn[i], func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Constraints.Max.X, height)
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				borderC := theme.Border
				borderW := gtx.Dp(unit.Dp(1))
				active := e.Draft.DefaultAcceptEncoding == opt.Value
				if active {
					borderC = theme.Accent
					borderW = gtx.Dp(unit.Dp(2))
				} else if e.AcceptEncodingBtn[i].Hovered() {
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

func (e *Editor) layoutHeader(gtx layout.Context, host *Host) layout.Dimensions {
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &e.BackBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(28)), gtx.Dp(unit.Dp(28)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := theme.Border
				if e.BackBtn.Hovered() {
					bg = theme.BorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min = image.Pt(gtx.Dp(16), gtx.Dp(16))
					return widgets.IconBack.Layout(gtx, host.Theme.Fg)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(12)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(host.Theme, unit.Sp(18), "Settings")
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &e.ResetBtn, func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(140)), gtx.Dp(unit.Dp(32)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				bg := theme.Border
				if e.ResetBtn.Hovered() {
					bg = theme.BorderLight
				}
				paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: size}, 4).Op(gtx.Ops))
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(host.Theme, unit.Sp(13), "Reset to defaults")
					lbl.Color = host.Theme.Fg
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				})
			})
		}),
	)
}

func (e *Editor) layoutCategories(gtx layout.Context, host *Host) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(settingsCategories))
	for i, name := range settingsCategories {
		i, name := i, name
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &e.CategoryBtn[i], func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					bg := theme.Transparent
					fg := theme.FgMuted
					if e.Category == i {
						bg = theme.BgHover
						fg = host.Theme.Fg
					} else if e.CategoryBtn[i].Hovered() {
						bg = theme.BgSecondary
					}
					rect := clip.UniformRRect(image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(32)))}, 4)
					paint.FillShape(gtx.Ops, bg, rect.Op(gtx.Ops))
					return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(host.Theme, unit.Sp(13), name)
						lbl.Color = fg
						if e.Category == i {
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

func (e *Editor) layoutContent(gtx layout.Context, host *Host) layout.Dimensions {
	var sections []layout.Widget
	switch e.Category {
	case 0:
		sections = e.sectionsAppearance(host)
	case 1:
		sections = e.sectionsSizes(host)
	case 2:
		sections = e.sectionsHTTP(host)
	case 3:
		sections = e.sectionsAdvanced(host)
	}
	return material.List(host.Theme, &e.ContentList).Layout(gtx, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Inset{Right: unit.Dp(8)}.Layout(gtx, sections[i])
	})
}

func (e *Editor) sectionsAppearance(host *Host) []layout.Widget {
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
		if t.ID == e.Draft.Theme {
			activeThemeName = t.Name
			break
		}
	}
	for _, c := range e.Draft.CustomThemes {
		if c.ID == e.Draft.Theme {
			activeThemeName = c.Name
			break
		}
	}
	widgets := []layout.Widget{
		settingsSectionTitle(host.Theme, "Visibility"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.HideTabBar)
			return settingsSwitchRow(host.Theme, "Hide tab bar", tabHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.HideSidebar)
			return settingsSwitchRow(host.Theme, "Hide sidebar", sideHint, sw.Layout)(gtx)
		},
		spacerH(20),
		settingsSectionTitle(host.Theme, "Startup"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.RestoreTabsOnStartup)
			return settingsSwitchRow(host.Theme, "Restore tabs on startup", restoreHint, sw.Layout)(gtx)
		},
		spacerH(20),
		settingsSectionTitle(host.Theme, "Color theme"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("VS Code–inspired themes. Default: %s.", defName)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return themeGrid(host.Theme, e, gtx)
		},
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return e.layoutNewThemeRow(gtx, host)
		},
		spacerH(20),
		spoilerHeader(host.Theme, &e.ThemeColorsHeaderBtn, &e.ThemeColorResetAllBtn,
			"Customize colors — "+activeThemeName, e.ThemeColorsExpanded),
	}
	if e.ThemeColorsExpanded {
		widgets = append(widgets,
			spacerH(4),
			settingsHint(host.Theme, "Type a hex color (e.g. #1F1F1F) or click the swatch for a picker. Empty = theme default."),
			spacerH(8),
		)
		for i := range theme.PaletteColorTable {
			idx := i
			widgets = append(widgets, themeColorRow(host.Theme, e, idx))
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
				lbl := material.Label(host.Theme, unit.Sp(11), "Syntax")
				lbl.Color = theme.FgMuted
				lbl.Font.Weight = font.Bold
				return lbl.Layout(gtx)
			},
			spacerH(8),
		)
		for i := range theme.TokenColorTable {
			idx := i
			widgets = append(widgets, syntaxColorRow(host.Theme, e, idx))
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

func themeColorRow(th *material.Theme, e *Editor, idx int) layout.Widget {
	entry := theme.PaletteColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		base := theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes)
		if ov, ok := e.Draft.ThemeOverrides[e.Draft.Theme]; ok {
			base = theme.ApplyOverride(base, ov)
		}
		swatchColor := entry.GetBase(base)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &e.ThemeColorSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if e.ColorPicker.Kind == colorpicker.KindTheme && e.ColorPicker.OpenIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, theme.Accent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := theme.BorderLight
						if e.ThemeColorSwatchBtns[idx].Hovered() {
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
				return widgets.TextField(gtx, th, &e.ThemeColorEditors[idx], theme.HexFromColor(entry.GetBase(theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes))), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &e.ThemeColorResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := theme.BgField
					if e.ThemeColorResetBtns[idx].Hovered() {
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

func syntaxColorRow(th *material.Theme, e *Editor, idx int) layout.Widget {
	entry := theme.TokenColorTable[idx]
	return func(gtx layout.Context) layout.Dimensions {
		basePalette := theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes).Syntax
		if ov, ok := e.Draft.SyntaxOverrides[e.Draft.Theme]; ok {
			basePalette = theme.ApplySyntaxOverride(basePalette, ov)
		}
		swatchColor := entry.GetBase(basePalette)

		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(20)), gtx.Dp(unit.Dp(20)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &e.SyntaxSwatchBtns[idx], func(gtx layout.Context) layout.Dimensions {
					border := gtx.Dp(unit.Dp(1))
					if e.ColorPicker.Kind == colorpicker.KindSyntax && e.ColorPicker.OpenIdx == idx {
						border = gtx.Dp(unit.Dp(2))
						paint.FillShape(gtx.Ops, theme.Accent, clip.UniformRRect(image.Rectangle{Max: size}, 3).Op(gtx.Ops))
					} else {
						borderC := theme.BorderLight
						if e.SyntaxSwatchBtns[idx].Hovered() {
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
				return widgets.TextField(gtx, th, &e.SyntaxOverrideEditors[idx], theme.HexFromColor(entry.GetBase(theme.PaletteFor(e.Draft.Theme, e.Draft.CustomThemes).Syntax)), true, nil, 0, unit.Sp(11))
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(unit.Dp(22)), gtx.Dp(unit.Dp(22)))
				gtx.Constraints.Min = size
				gtx.Constraints.Max = size
				return material.Clickable(gtx, &e.SyntaxResetBtns[idx], func(gtx layout.Context) layout.Dimensions {
					bg := theme.BgField
					if e.SyntaxResetBtns[idx].Hovered() {
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

func (e *Editor) sectionsSizes(host *Host) []layout.Widget {
	def := model.DefaultSettings()
	return []layout.Widget{
		settingsSectionTitle(host.Theme, "UI text size"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Scales all UI text. Default: %d pt.", def.UITextSize)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.UISizeDec, &e.UISizeInc, &e.UISizeEditor, "pt"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "Body text size"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Size of the request and response body editors. Default: %d pt.", def.BodyTextSize)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.BodySizeDec, &e.BodySizeInc, &e.BodySizeEditor, "pt"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "UI scale"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Overall size of layout spacing and controls. Default: %.2fx.", def.UIScale)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.UIScaleDec, &e.UIScaleInc, &e.UIScaleEditor, "x"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "Response body padding"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Inner padding around the response body text. Same for wrap and no-wrap modes. Default: %d px.", def.ResponseBodyPadding)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.BodyPaddingDec, &e.BodyPaddingInc, &e.BodyPaddingEditor, "px"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "Default request/response split"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Initial width ratio of the request pane in new tabs. Default: %.0f%%.", def.DefaultSplitRatio*100)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.SplitRatioDec, &e.SplitRatioInc, &e.SplitRatioEditor, "%"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "Adaptive stack breakpoint"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Stack request and response panes vertically when the tab content area is narrower than this width. Set to 0 to always keep them side-by-side. Default: %d dp.", def.StackBreakpointDp)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.StackBpDec, &e.StackBpInc, &e.StackBpEditor, "dp"),
		spacerH(20),
		settingsSectionTitle(host.Theme, "Default sidebar width"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Initial width of the collections/environments sidebar on first launch. Existing windows keep their dragged width. Default: %d px.", def.DefaultSidebarWidthPx)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.SidebarWidthDec, &e.SidebarWidthInc, &e.SidebarWidthEditor, "px"),
	}
}

func (e *Editor) sectionsHTTP(host *Host) []layout.Widget {
	def := model.DefaultSettings()
	redirectHint := "Follow HTTP 3xx redirects automatically. " + defaultOnOff(def.FollowRedirects)
	verifyHint := "Verify TLS certificates for HTTPS requests. Disable only for local dev against self-signed certs. " + defaultOnOff(def.VerifySSL)
	keepAliveHint := "Reuse TCP connections across requests to the same host. " + defaultOnOff(def.KeepAlive)
	http2Hint := "Force HTTP/1.1 only — disables HTTP/2 ALPN negotiation on TLS connections. " + defaultOnOff(def.DisableHTTP2)
	cookieHint := "Persist cookies set by the server and resend them on subsequent requests to the same host (in-memory only, cleared on app exit). " + defaultOnOff(def.CookieJarEnabled)
	connCloseHint := "Send Connection: close on every request and tear down the TCP connection after the response. Useful for debugging. " + defaultOnOff(def.SendConnectionClose)
	return []layout.Widget{
		settingsSectionTitle(host.Theme, "Request timeout"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Cancel a request if no response arrives in this many seconds. 0 = no timeout. Default: %d s.", def.RequestTimeoutSec)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.TimeoutDec, &e.TimeoutInc, &e.TimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Connect timeout"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Maximum time to establish a TCP connection. 0 = system default. Default: %d s.", def.ConnectTimeoutSec)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.ConnectTimeoutDec, &e.ConnectTimeoutInc, &e.ConnectTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(host.Theme, "TLS handshake timeout"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Maximum time waiting for the TLS handshake. 0 = system default. Default: %d s.", def.TLSHandshakeTimeoutSec)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.TLSTimeoutDec, &e.TLSTimeoutInc, &e.TLSTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Idle connection timeout"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Close idle keep-alive connections after this many seconds. 0 = never. Default: %d s.", def.IdleConnTimeoutSec)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.IdleTimeoutDec, &e.IdleTimeoutInc, &e.IdleTimeoutEditor, "s"),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Default request method"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Method assigned to newly created tabs. Default: %s.", def.DefaultMethod)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return methodGrid(host.Theme, e, gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Default User-Agent"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Sent on every request unless overridden by a per-request header. Default: %s.", def.UserAgent)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return widgets.TextField(gtx, host.Theme, &e.UserAgentEditor, "User-Agent", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Redirects"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.FollowRedirects)
			return settingsSwitchRow(host.Theme, "Follow redirects", redirectHint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(host.Theme, fmt.Sprintf("Maximum redirect chain length. 0 = unlimited. Default: %d.", def.MaxRedirects)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.MaxRedirectsDec, &e.MaxRedirectsInc, &e.MaxRedirectsEditor, ""),
		spacerH(20),

		settingsSectionTitle(host.Theme, "TLS"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.VerifySSL)
			return settingsSwitchRow(host.Theme, "Verify SSL certificates", verifyHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Connection"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.KeepAlive)
			return settingsSwitchRow(host.Theme, "Keep-Alive", keepAliveHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.DisableHTTP2)
			return settingsSwitchRow(host.Theme, "Disable HTTP/2", http2Hint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.SendConnClose)
			return settingsSwitchRow(host.Theme, "Send Connection: close", connCloseHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.CookieJar)
			return settingsSwitchRow(host.Theme, "Cookie jar", cookieHint, sw.Layout)(gtx)
		},
		spacerH(12),
		settingsHint(host.Theme, fmt.Sprintf("Maximum concurrent connections per host. 0 = unlimited. Default: %d.", def.MaxConnsPerHost)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.MaxConnsDec, &e.MaxConnsInc, &e.MaxConnsEditor, ""),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Default Accept-Encoding"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Sent on every request unless overridden by a per-request header. \"off\" omits the header (Go will then add gzip automatically and transparently decode it). Default: %q.", def.DefaultAcceptEncoding)),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			return acceptEncodingGrid(host.Theme, e, gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "HTTP proxy"),
		spacerH(4),
		settingsHint(host.Theme, "Send all requests through this proxy. Format: http://host:port or http://user:pass@host:port. Leave empty to disable."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(360))
			return widgets.TextField(gtx, host.Theme, &e.ProxyEditor, "http://proxy.local:8080", true, nil, 0, unit.Sp(13))
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Default headers"),
		spacerH(4),
		settingsHint(host.Theme, "One per line, format \"Header: value\". Added to every request unless the tab sets the same header. Lines starting with # are comments."),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = gtx.Dp(unit.Dp(480))
			gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(96))
			return widgets.TextField(gtx, host.Theme, &e.DefaultHdrEdit, "Accept: application/json", true, nil, 0, unit.Sp(13))
		},
	}
}

func (e *Editor) sectionsAdvanced(host *Host) []layout.Widget {
	def := model.DefaultSettings()
	wrapHint := "Wrap long lines by default in new editors. " + defaultOnOff(def.WrapLinesDefault)
	autoFmtHint := "Pretty-print JSON responses in the preview viewer. Disable to display raw bytes as received. " + defaultOnOff(def.AutoFormatJSON)
	autoFmtReqHint := "Pretty-print the JSON request body before sending if it parses as valid JSON. Uses the JSON indent setting. " + defaultOnOff(def.AutoFormatJSONRequest)
	stripHint := "Remove // line comments from JSON request bodies before sending if the result is valid JSON. " + defaultOnOff(def.StripJSONComments)
	trimHint := "Strip trailing spaces and tabs from each line of the request body before sending. " + defaultOnOff(def.TrimTrailingWhitespace)
	bracketHint := "Color matched brackets in nested JSON by depth, like VS Code. " + defaultOnOff(def.BracketPairColorization)
	return []layout.Widget{
		settingsSectionTitle(host.Theme, "JSON indent"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Spaces per level in the JSON pretty-printer. 0 = minified. Default: %d.", def.JSONIndentSpaces)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.JSONIndentDec, &e.JSONIndentInc, &e.JSONIndentEditor, ""),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Response preview cap"),
		spacerH(4),
		settingsHint(host.Theme, fmt.Sprintf("Maximum response size loaded into the preview editor before 'Load more' is required. Default: %d MB.", def.PreviewMaxMB)),
		spacerH(8),
		stepperEditableRow(host.Theme, &e.PreviewMaxDec, &e.PreviewMaxInc, &e.PreviewMaxEditor, "MB"),
		spacerH(20),

		settingsSectionTitle(host.Theme, "Editors"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.WrapLines)
			return settingsSwitchRow(host.Theme, "Wrap long lines by default", wrapHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "JSON handling"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.AutoFormatJSON)
			return settingsSwitchRow(host.Theme, "Auto-format JSON responses", autoFmtHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.AutoFormatJSONRequest)
			return settingsSwitchRow(host.Theme, "Auto-format JSON request before send", autoFmtReqHint, sw.Layout)(gtx)
		},
		spacerH(12),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.StripJSONComments)
			return settingsSwitchRow(host.Theme, "Strip // comments before send", stripHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Body editor"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.TrimTrailingWS)
			return settingsSwitchRow(host.Theme, "Trim trailing whitespace before send", trimHint, sw.Layout)(gtx)
		},
		spacerH(20),

		settingsSectionTitle(host.Theme, "Syntax coloring"),
		spacerH(8),
		func(gtx layout.Context) layout.Dimensions {
			sw := styledSwitch(host.Theme, &e.BracketPairColorization)
			return settingsSwitchRow(host.Theme, "Bracket pair colorization", bracketHint, sw.Layout)(gtx)
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

func (e *Editor) layoutNewThemeRow(gtx layout.Context, host *Host) layout.Dimensions {
	if !e.NewThemeDialogOpen {
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
					lbl := material.Label(host.Theme, unit.Sp(13), "Create new theme")
					lbl.Font.Weight = font.Bold
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(host.Theme, unit.Sp(11), "Name")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(2)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return widgets.TextField(gtx, host.Theme, &e.NewThemeNameEditor, "My theme", true, nil, 0, unit.Sp(12))
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(host.Theme, unit.Sp(11), "Based on")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return e.layoutBaseThemePicker(gtx, host)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(10)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(host.Theme, &e.NewThemeCreateBtn, "Create")
							btn.TextSize = unit.Sp(12)
							btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(14), Right: unit.Dp(14)}
							return btn.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							btn := material.Button(host.Theme, &e.NewThemeCancelBtn, "Cancel")
							btn.Background = theme.Border
							btn.Color = host.Theme.Fg
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

func (e *Editor) layoutBaseThemePicker(gtx layout.Context, host *Host) layout.Dimensions {
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
					return material.Clickable(gtx, &e.NewThemeBaseBtns[baseIdx+j], func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Pt(tileW, tileH)
						gtx.Constraints.Max = gtx.Constraints.Min
						bg := t.Palette.Bg
						paint.FillShape(gtx.Ops, bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Op(gtx.Ops))
						if e.NewThemeBaseID == t.ID {
							paint.FillShape(gtx.Ops, theme.Accent, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 4).Path(gtx.Ops), Width: 2}.Op())
						} else {
							widgets.PaintBorder1px(gtx, gtx.Constraints.Min, theme.BorderLight)
						}
						return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(host.Theme, unit.Sp(11), t.Name)
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

func themeGrid(th *material.Theme, e *Editor, gtx layout.Context) layout.Dimensions {
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
		entries = append(entries, tileEntry{def: theme.Registry[i], btn: &e.ThemeBtns[i]})
	}
	for i, c := range e.Draft.CustomThemes {
		if i >= len(e.CustomThemeBtns) || i >= len(e.CustomThemeDelBtns) {
			break
		}
		entries = append(entries, tileEntry{
			def:      theme.Def{ID: c.ID, Name: c.Name, Palette: theme.PaletteFor(c.ID, e.Draft.CustomThemes)},
			btn:      &e.CustomThemeBtns[i],
			delBtn:   &e.CustomThemeDelBtns[i],
			isCustom: true,
		})
	}
	entries = append(entries, tileEntry{btn: &e.NewThemeBtn, isAdd: true})

	var rows []layout.FlexChild
	for i := 0; i < len(entries); i += perRow {
		end := i + perRow
		if end > len(entries) {
			end = len(entries)
		}
		slice := entries[i:end]
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cols []layout.FlexChild
			for j, te := range slice {
				j, te := j, te
				var w layout.Widget
				if te.isAdd {
					w = themeTileFixedNew(th, te.btn, e.NewThemeDialogOpen, tileW, tileH)
				} else {
					w = themeTileFixedCustom(th, te.btn, te.delBtn, te.def, e.Draft.Theme == te.def.ID, te.isCustom, tileW, tileH)
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
