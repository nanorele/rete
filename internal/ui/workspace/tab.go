package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/settings"
	"tracto/internal/ui/syntax"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/internal/utils"

	"github.com/nanorele/gio-x/explorer"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/io/clipboard"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var methods = []string{"GET", "POST", "PUT", "DELETE", "HEAD", "PATCH", "OPTIONS"}

var protocols = []string{"HTTP", "WS", "GraphQL"}

var (
	iconCopy *widget.Icon
	iconWrap *widget.Icon
)

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 256*1024)
		return &b
	},
}

var bodyReplacer = strings.NewReplacer("\u2003", "\t", "\uFEFF", "")

func init() {
	iconCopy, _ = widget.NewIcon(icons.ContentContentCopy)
	iconWrap, _ = widget.NewIcon(icons.EditorWrapText)
}

type HeaderItem struct {
	Key         widget.Editor
	Value       widget.Editor
	DelBtn      widget.Clickable
	IsGenerated bool
	LastAutoKey string
	LastAutoVal string
	SplitDrag   gesture.Drag
	splitLastX  float32
	RowHover    widgets.Hover
	RowFade     widgets.Fade
}

type tabResponse struct {
	requestID     uint64
	status        string
	body          string
	respSize      int64
	respFile      string
	previewLoaded int64
	isJSON        bool
	contentType   string
	filename      string
	timings       Timings
}

type previewResult struct {
	requestID     uint64
	body          string
	previewLoaded int64
	isJSON        bool
}

type appendChunk struct {
	requestID uint64
	text      string
}

type RequestTab struct {
	Title              string
	TabBtn             widget.Clickable
	CloseBtn           widget.Clickable
	Method             string
	MethodBtn          widget.Clickable
	MethodListOpen     bool
	MethodClickables   []widget.Clickable
	ProtocolBtn        widget.Clickable
	ProtocolListOpen   bool
	ProtocolClickables [3]widget.Clickable
	LastHTTPMethod     string
	URLInput           widget.Editor
	urlClick           gesture.Click
	SendBtn            widget.Clickable
	Headers            []*HeaderItem
	HeadersExpanded    bool
	AddHeaderBtn       widget.Clickable
	ViewGeneratedBtn   widget.Clickable
	HeadersList        widget.List
	ReqEditor          RequestEditor
	RespListH          widget.List
	WrapBtn            widget.Clickable
	WrapEnabled        bool
	CopyBtn            widget.Clickable
	Status             string
	RespEditor         *ResponseViewer
	SplitRatio         float32
	VStackRatio        float32
	HeadersAbsHeight   int
	FitHeaders         bool
	LayoutMode         int
	LayoutHorizBtn     widget.Clickable
	LayoutVertBtn      widget.Clickable
	SplitDrag          gesture.Drag
	SplitDragX         float32
	HeadersBodyDrag    gesture.Drag
	HeadersBodyDragX   float32
	ScrollDrag         gesture.Drag
	ScrollDragY        float32
	ReqScrollDrag      gesture.Drag
	ReqScrollDragY     float32
	HScrollDrag        gesture.Drag
	HScrollDragX       float32
	ReqHScrollDrag     gesture.Drag
	ReqHScrollDragX    float32

	LoadFromFileBtn    widget.Clickable
	DismissOversizeBtn widget.Clickable
	LastReqWidth       int
	LastRespWidth      int
	IsDraggingSplit    bool
	LastURLWidth       int
	LinkedNode         *collections.CollectionNode
	SaveToColBtn       widget.Clickable
	IsDirty            bool
	PendingColID       string
	PendingNodePath    []int

	responseChan    chan tabResponse
	previewChan     chan previewResult
	previewLoading  atomic.Bool
	requestID       atomic.Uint64
	respMu          sync.Mutex
	jsonStateMu     sync.Mutex
	Closed          atomic.Bool
	FileSaveMu      sync.Mutex
	isRequesting    bool
	cancelFn        context.CancelFunc
	respSize        int64
	respFile        string
	respIsJSON      bool
	respContentType string
	ReqLangHint     syntax.Lang
	downloadedBytes atomic.Int64
	previewLoaded   atomic.Int64

	CancelBtn      widget.Clickable
	SendMenuBtn    widget.Clickable
	SendMenuOpen   bool
	SaveToFileBtn  widget.Clickable
	SaveToFilePath string
	SuggestedFile  string
	CopyAsCurlBtn  widget.Clickable
	ShowPreviewBtn widget.Clickable
	PreviewEnabled bool
	LoadMoreBtn    widget.Clickable
	OpenFileBtn    widget.Clickable
	PropertiesBtn  widget.Clickable
	LastTimings    Timings

	ReqWrapEnabled    bool
	jsonFmtState      *JSONFormatterState
	ReqWrapBtn        widget.Clickable
	ReqCopyBtn        widget.Clickable
	ReqListH          widget.List
	HeaderKeyW        float32
	HeaderSplitDrag   gesture.Drag
	HeaderSplitDragX  float32
	HeaderKeyBelowMin bool
	ReqCollapseBtn    widget.Clickable
	ReqBodyCollapsed  bool
	RespCollapseBtn   widget.Clickable
	RespBodyCollapsed bool
	reqRatioSaved     float32
	respRatioSaved    float32
	respHeaderH       int

	ReqSubTab     int
	HeadersTabBtn widget.Clickable
	ParamsTabBtn  widget.Clickable
	AuthTabBtn    widget.Clickable
	CookiesTabBtn widget.Clickable

	Params       []*HeaderItem
	ParamsList   widget.List
	paramsSynced string

	Cookies     []*HeaderItem
	CookiesList widget.List

	AuthType        int
	AuthTypeBtn     widget.Clickable
	AuthTypeOpen    bool
	AuthTypeChoices [3]widget.Clickable
	AuthToken       widget.Editor
	AuthUser        widget.Editor
	AuthPass        widget.Editor

	SearchBtn    widget.Clickable
	ReqSearchBtn widget.Clickable
	ReqSearch    SearchBox
	RespSearch   SearchBox

	URLSubmitted      bool
	FileSaveChan      chan io.WriteCloser
	dirtyCheckNeeded  bool
	visibleHeadersBuf []*HeaderItem

	appendChan       chan appendChunk
	window           *app.Window
	pendingRespWidth int
	pendingReqWidth  int
	reqWidthTimer    *time.Timer
	respWidthTimer   *time.Timer
	LastReqHeight    int
	LastRespHeight   int
	reqHeightTimer   *time.Timer
	respHeightTimer  *time.Timer
	reqHeaderH       int
	headersRowH      int
	headersRenderH   int
	reqPaneH         int
	hbEditorPx       int
	hbHeadersPx      float32
	splitPaneRec     int
	splitRespRec     int
	splitPanePx      float32
	PaneDrawnH       int
	hbSliderY        int
	hbUserResized    bool
	hbManualDp       int
	fitHeadersExact  bool
	fitPrevHeadersDp int
	prevStacked      bool
	prevStackedInit  bool
	reqHugPending    bool
	reqPaneBoxH      int
	respPaneBoxH     int

	cleanTitle    string
	cleanTitleSrc string

	BodyType        model.BodyType
	FormParts       []*FormDataPart
	URLEncoded      []*URLEncodedPart
	BinaryFilePath  string
	BinaryFileSize  int64
	BodyTypeBtn     widget.Clickable
	BodyTypeOpen    bool
	BodyTypeChoices [5]widget.Clickable
	AddFormPartBtn  widget.Clickable
	AddUEPartBtn    widget.Clickable
	ChooseBinaryBtn widget.Clickable

	formPartFileChan chan formPartFileResult
	binaryFileChan   chan binaryFileResult

	WS     *WSSession
	WSHost WSHostFuncs

	GQL *GQLSession

	Run         *RequestRunner
	RunOpen     bool
	SingleBtn   widget.Clickable
	MultipleBtn widget.Clickable

	Examples        []model.ParsedExample
	ExampleSel      int
	ExampleBtn      widget.Clickable
	ExampleListOpen bool
	ExampleChoices  []widget.Clickable
	BaseState       exampleBaseState
}

func NewRequestTab(title string) *RequestTab {
	method := settings.DefaultMethod
	if method == "" {
		method = "GET"
	}
	splitRatio := settings.DefaultSplitRatio
	if splitRatio < 0.2 || splitRatio > 0.8 {
		splitRatio = 0.5
	}
	t := &RequestTab{
		Title:            title,
		Method:           method,
		LastHTTPMethod:   "GET",
		Status:           "Ready",
		RespEditor:       NewResponseViewer(),
		MethodClickables: make([]widget.Clickable, len(methods)),
		responseChan:     make(chan tabResponse, 1),
		previewChan:      make(chan previewResult, 1),
		FileSaveChan:     make(chan io.WriteCloser, 1),
		appendChan:       make(chan appendChunk, 1024),
		SplitRatio:       splitRatio,
		VStackRatio:      0.5,
		HeadersAbsHeight: 0,
		WrapEnabled:      true,
		ReqWrapEnabled:   true,
		jsonFmtState:     &JSONFormatterState{},
		HeadersExpanded:  false,
		BodyType:         model.BodyRaw,
		formPartFileChan: make(chan formPartFileResult, 64),
		binaryFileChan:   make(chan binaryFileResult, 8),
		ExampleSel:       -1,
	}
	t.URLInput.Submit = true
	t.HeadersList.Axis = layout.Vertical
	t.ParamsList.Axis = layout.Vertical
	t.CookiesList.Axis = layout.Vertical
	t.RespListH.Axis = layout.Horizontal
	t.ReqListH.Axis = layout.Horizontal
	t.ReqSearch.Editor.SingleLine = true
	t.ReqSearch.Editor.Submit = true
	t.RespSearch.Editor.SingleLine = true
	t.RespSearch.Editor.Submit = true
	return t
}

func (t *RequestTab) responseLang() syntax.Lang {
	if t.respIsJSON {
		return syntax.LangJSON
	}
	return syntax.Detect(t.respContentType, t.RespEditor.Bytes())
}

func (t *RequestTab) requestLang() syntax.Lang {
	for _, h := range t.Headers {
		if strings.EqualFold(h.Key.Text(), "Content-Type") {
			if l := syntax.Detect(h.Value.Text(), nil); l != syntax.LangPlain {
				return l
			}
			break
		}
	}
	if t.ReqLangHint != syntax.LangPlain {
		return t.ReqLangHint
	}
	return syntax.Detect("", t.ReqEditor.Bytes())
}

var bodyTypeChoices = [5]model.BodyType{model.BodyNone, model.BodyRaw, model.BodyFormData, model.BodyURLEncoded, model.BodyBinary}

const (
	LayoutModeAuto  = 0
	LayoutModeHoriz = 1
	LayoutModeVert  = 2
)

func (t *RequestTab) layoutBodyTypeSelector(gtx layout.Context, th *material.Theme) layout.Dimensions {
	for t.BodyTypeBtn.Clicked(gtx) {
		t.BodyTypeOpen = !t.BodyTypeOpen
	}
	for i := range t.BodyTypeChoices {
		for t.BodyTypeChoices[i].Clicked(gtx) {
			next := bodyTypeChoices[i]
			if t.BodyType != next {
				t.BodyType = next
				t.UpdateSystemHeaders()
				t.dirtyCheckNeeded = true
			}
			t.BodyTypeOpen = false
		}
	}

	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !t.BodyTypeOpen {
				return layout.Dimensions{}
			}
			items := make([]widgets.MenuItem, len(bodyTypeChoices))
			for i, bt := range bodyTypeChoices {
				items[i] = widgets.MenuItem{
					Label:   bt.String(),
					Click:   &t.BodyTypeChoices[i],
					Checked: t.BodyType == bt,
					Mono:    true,
				}
			}
			anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(28)))}
			widgets.DeferMenuAt(gtx, th, &t.BodyTypeOpen, anchor, 180, items)
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &t.BodyTypeBtn, func(gtx layout.Context) layout.Dimensions {
				pointer.CursorPointer.Add(gtx.Ops)
				macro := op.Record(gtx.Ops)
				dim := layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(7), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), t.BodyType.String())
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							s := gtx.Dp(unit.Dp(12))
							gtx.Constraints.Min = image.Pt(s, s)
							gtx.Constraints.Max = gtx.Constraints.Min
							return widgets.IconDropDown.Layout(gtx, theme.FgMuted)
						}),
					)
				})
				call := macro.Stop()
				if t.BodyTypeBtn.Hovered() {
					paint.FillShape(gtx.Ops, theme.BgHover, clip.UniformRRect(image.Rectangle{Max: dim.Size}, gtx.Dp(unit.Dp(2))).Op(gtx.Ops))
				}
				call.Add(gtx.Ops)
				return dim
			})
		}),
	)
}

func (t *RequestTab) layoutModeBarHeight(gtx layout.Context) int {
	return gtx.Dp(unit.Dp(26)) + gtx.Dp(unit.Dp(4))
}

func (t *RequestTab) stackedSplitExtent(gtx layout.Context) float32 {
	urlRowH := gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(28)) + gtx.Dp(unit.Dp(8))
	contentInsetH := 2 * gtx.Dp(unit.Dp(1))
	dividerH := gtx.Dp(unit.Dp(4))
	ext := gtx.Constraints.Max.Y - urlRowH - contentInsetH - t.layoutModeBarHeight(gtx) - dividerH
	if ext < 1 {
		ext = 1
	}
	return float32(ext)
}

func collapseChevron(gtx layout.Context, th *material.Theme, btn *widget.Clickable, collapsed bool) layout.Dimensions {
	icon := widgets.IconExpandLess
	if collapsed {
		icon = widgets.IconExpandMore
	}
	return widgets.SquareBtn(gtx, btn, icon, th)
}

func (t *RequestTab) reqHeaderRowPx(gtx layout.Context) int {
	if t.reqHeaderH > 0 {
		return t.reqHeaderH
	}
	return gtx.Dp(unit.Dp(34))
}

func (t *RequestTab) headersRowPx(gtx layout.Context) int {
	if t.headersRowH > 0 {
		return t.headersRowH
	}
	return t.reqHeaderRowPx(gtx)
}

func (t *RequestTab) reqPaneAboveHeadersPx(gtx layout.Context) int {
	return t.headersRowPx(gtx) + gtx.Dp(unit.Dp(1))
}

func (t *RequestTab) reqPaneBelowHeadersContentPx(gtx layout.Context) int {
	row := t.reqHeaderRowPx(gtx)
	line := gtx.Dp(unit.Dp(1))
	h := gtx.Dp(unit.Dp(4)) + line + row
	if !t.ReqBodyCollapsed {
		h += line
	}
	return h
}

func (t *RequestTab) reqPaneBelowHeadersPx(gtx layout.Context) int {
	return t.reqPaneBelowHeadersContentPx(gtx) + gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(2))
}

func (t *RequestTab) respCollapsedMinPx(gtx layout.Context) int {
	h := t.respHeaderH
	if h <= 0 {
		h = gtx.Dp(unit.Dp(60))
	}
	return h + 2*gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(2))
}

func (t *RequestTab) stackedReqPaneMinPx(gtx layout.Context) int {
	row := t.reqHeaderRowPx(gtx)
	line := gtx.Dp(unit.Dp(1))
	h := t.headersRowPx(gtx)
	if t.HeadersExpanded {
		hDp := t.HeadersAbsHeight
		if hDp <= 0 {
			hDp = 120
		}
		h += line + gtx.Dp(unit.Dp(hDp))
	}
	h += gtx.Dp(unit.Dp(4)) + line + row
	if !t.ReqBodyCollapsed {
		h += line
	}
	return h + gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(2))
}

func (t *RequestTab) headersFitDp(activeKV []*HeaderItem) int {
	if t.ReqSubTab == reqSubAuth {
		if t.AuthType == authBasic {
			return 150
		}
		return 100
	}
	rows := len(activeKV)
	if rows < 1 {
		rows = 1
	}
	return rows*28 + 4
}

func paintLayoutSplitIcon(gtx layout.Context, sz int, color color.NRGBA, vertical bool) {
	if sz <= 0 {
		return
	}
	widgets.PaintBorder1px(gtx, image.Pt(sz, sz), color)
	if vertical {
		midY := sz / 2
		paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(0, midY), Max: image.Pt(sz, midY+1)}.Op())
	} else {
		midX := sz / 2
		paint.FillShape(gtx.Ops, color, clip.Rect{Min: image.Pt(midX, 0), Max: image.Pt(midX+1, sz)}.Op())
	}
}

func (t *RequestTab) layoutModeBtn(gtx layout.Context, btn *widget.Clickable, vertical bool, active bool) layout.Dimensions {
	return material.Clickable(gtx, btn, func(gtx layout.Context) layout.Dimensions {
		s := gtx.Dp(unit.Dp(22))
		gtx.Constraints.Min = image.Pt(s, s)
		gtx.Constraints.Max = gtx.Constraints.Min
		switch {
		case active:
			paint.FillShape(gtx.Ops, theme.AccentDim, clip.Rect{Max: gtx.Constraints.Min}.Op())
		case btn.Hovered():
			paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
		}
		pointer.CursorPointer.Add(gtx.Ops)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			isz := gtx.Dp(unit.Dp(12))
			gtx.Constraints.Min = image.Pt(isz, isz)
			gtx.Constraints.Max = gtx.Constraints.Min
			paintLayoutSplitIcon(gtx, isz, theme.FgMuted, vertical)
			return layout.Dimensions{Size: image.Pt(isz, isz)}
		})
	})
}

func (t *RequestTab) layoutModeBar(gtx layout.Context, th *material.Theme, hBtn, vBtn *widget.Clickable, stacked bool) layout.Dimensions {
	barH := t.layoutModeBarHeight(gtx)
	gtx.Constraints.Min.Y = barH
	gtx.Constraints.Max.Y = barH
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBtn(gtx, hBtn, false, !stacked)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBtn(gtx, vBtn, true, stacked)
			}),
			layout.Flexed(1, layout.Spacer{}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if t.Method == MethodWS || t.Method == MethodGraphQL {
					return layout.Dimensions{}
				}
				return t.layoutExampleSelector(gtx, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if t.Method == MethodWS || t.Method == MethodGraphQL {
					return layout.Dimensions{}
				}
				return layout.Spacer{Width: unit.Dp(100)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if t.Method == MethodWS {
					return layout.Dimensions{}
				}
				return t.layoutRunModeTabs(gtx, th)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if t.Method == MethodWS {
					return layout.Dimensions{}
				}
				return layout.Spacer{Width: unit.Dp(100)}.Layout(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if t.LinkedNode == nil {
					return layout.Dimensions{}
				}
				iconColor := theme.FgDisabled
				if t.IsDirty {
					iconColor = theme.Accent
				}
				return t.SaveToColBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					s := gtx.Dp(unit.Dp(22))
					gtx.Constraints.Min = image.Pt(s, s)
					gtx.Constraints.Max = gtx.Constraints.Min
					if t.SaveToColBtn.Hovered() {
						paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: gtx.Constraints.Min}.Op())
					}
					pointer.CursorPointer.Add(gtx.Ops)
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						isz := gtx.Dp(unit.Dp(16))
						gtx.Constraints.Min = image.Pt(isz, isz)
						gtx.Constraints.Max = gtx.Constraints.Min
						return widgets.IconSave.Layout(gtx, iconColor)
					})
				})
			}),
		)
	})
}

func (t *RequestTab) headersRowMinWidth(gtx layout.Context, th *material.Theme) int {
	leftInset := gtx.Dp(unit.Dp(4))
	tabPad := gtx.Dp(unit.Dp(10))
	sepW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, "|")
	tabsW := 0
	for _, label := range []string{"Headers", "Params", "Auth", "Cookies"} {
		tabsW += widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, label) + tabPad
	}
	tabsW += 3 * sepW
	btnW := gtx.Dp(unit.Dp(28))
	gap := gtx.Dp(unit.Dp(4))
	safety := gtx.Dp(unit.Dp(12))
	return leftInset + tabsW + gap + btnW + gap + btnW + safety
}

func (t *RequestTab) bodyTypeRowMinWidth(gtx layout.Context, th *material.Theme) int {
	return computeBodyTypeRowMinWidth(gtx, th, t.BodyType.String())
}

func (t *RequestTab) defaultPaneMinWidth(gtx layout.Context, th *material.Theme) int {
	headersMin := t.headersRowMinWidth(gtx, th)
	bodyTypeMin := computeBodyTypeRowMinWidth(gtx, th, "x-www-form-urlencoded")
	threshold := headersMin
	if bodyTypeMin > threshold {
		threshold = bodyTypeMin
	}
	return threshold + gtx.Dp(unit.Dp(1))
}

func computeBodyTypeRowMinWidth(gtx layout.Context, th *material.Theme, typeName string) int {
	leftInset := gtx.Dp(unit.Dp(9))
	requestW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, "Request")
	gapBetween := gtx.Dp(unit.Dp(8))

	selectorPad := gtx.Dp(unit.Dp(16))
	typeNameW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(11), widgets.MonoFont, typeName)
	iconW := gtx.Dp(unit.Dp(12))
	innerGap := gtx.Dp(unit.Dp(4))
	selectorW := selectorPad + typeNameW + innerGap + iconW

	safety := gtx.Dp(unit.Dp(12))
	return leftInset + requestW + gapBetween + selectorW + safety
}

func (t *RequestTab) GetCleanTitle() string {
	if t.cleanTitleSrc == t.Title && t.cleanTitle != "" {
		return t.cleanTitle
	}
	s := utils.SanitizeText(t.Title)
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.TrimSpace(s) == "" {
		s = "New request"
	}
	t.cleanTitle = s
	t.cleanTitleSrc = t.Title
	return s
}

func (t *RequestTab) checkDirty() {
	if t.LinkedNode == nil || t.LinkedNode.Request == nil {
		t.IsDirty = false
		return
	}
	req := t.LinkedNode.Request
	if t.Method != req.Method {
		t.IsDirty = true
		return
	}
	if t.URLInput.Text() != req.URL {
		t.IsDirty = true
		return
	}
	if t.ReqEditor.Text() != req.Body {
		t.IsDirty = true
		return
	}
	if t.BodyType != req.BodyType {
		t.IsDirty = true
		return
	}
	if t.BinaryFilePath != req.BinaryPath {
		t.IsDirty = true
		return
	}
	userHeaders := 0
	for _, h := range t.Headers {
		if !h.IsGenerated && h.Key.Len() > 0 {
			userHeaders++
		}
	}
	if userHeaders != len(req.Headers) {
		t.IsDirty = true
		return
	}
	for _, h := range t.Headers {
		if !h.IsGenerated && h.Key.Len() > 0 {
			k := h.Key.Text()
			if v, ok := req.Headers[k]; !ok || v != h.Value.Text() {
				t.IsDirty = true
				return
			}
		}
	}
	if t.formPartsDirty(req) || t.urlEncodedDirty(req) {
		t.IsDirty = true
		return
	}
	if t.AuthModel() != req.Auth {
		t.IsDirty = true
		return
	}
	if t.cookiesDirty(req) {
		t.IsDirty = true
		return
	}
	t.IsDirty = false
}

func (t *RequestTab) cookiesDirty(req *model.ParsedRequest) bool {
	cm := t.CookieModels()
	if len(cm) != len(req.Cookies) {
		return true
	}
	for i, c := range cm {
		if c != req.Cookies[i] {
			return true
		}
	}
	return false
}

func (t *RequestTab) formPartsDirty(req *model.ParsedRequest) bool {
	var parts []model.ParsedFormPart
	for _, p := range t.FormParts {
		if p.Key.Text() == "" {
			continue
		}
		parts = append(parts, model.ParsedFormPart{
			Key: p.Key.Text(), Value: p.Value.Text(), Kind: p.Kind,
			FilePath: p.FilePath, Disabled: p.Disabled,
		})
	}
	if len(parts) != len(req.FormParts) {
		return true
	}
	for i, p := range parts {
		if p != req.FormParts[i] {
			return true
		}
	}
	return false
}

func (t *RequestTab) urlEncodedDirty(req *model.ParsedRequest) bool {
	var parts []model.ParsedKV
	for _, p := range t.URLEncoded {
		if p.Key.Text() == "" {
			continue
		}
		parts = append(parts, model.ParsedKV{Key: p.Key.Text(), Value: p.Value.Text(), Disabled: p.Disabled})
	}
	if len(parts) != len(req.URLEncoded) {
		return true
	}
	for i, p := range parts {
		if p != req.URLEncoded[i] {
			return true
		}
	}
	return false
}

func (t *RequestTab) SaveToCollection() *collections.ParsedCollection {
	if t.LinkedNode == nil || t.LinkedNode.Request == nil {
		return nil
	}
	req := t.LinkedNode.Request
	req.URL = t.URLInput.Text()
	req.Method = t.Method
	req.Body = t.ReqEditor.Text()
	req.Name = t.Title
	req.Headers = make(map[string]string, len(t.Headers))
	rawArr := make([]map[string]string, 0, len(t.Headers))
	for _, h := range t.Headers {
		if h.IsGenerated {
			continue
		}
		k := h.Key.Text()
		if k == "" {
			continue
		}
		v := h.Value.Text()
		req.Headers[k] = v
		rawArr = append(rawArr, map[string]string{"key": k, "value": v})
	}
	if data, err := json.Marshal(rawArr); err == nil {
		req.RawHeaders = data
	} else {
		req.RawHeaders = nil
	}
	req.BodyType = t.BodyType
	req.BinaryPath = t.BinaryFilePath
	req.Auth = t.AuthModel()
	req.Cookies = t.CookieModels()
	req.FormParts = req.FormParts[:0]
	for _, p := range t.FormParts {
		k := p.Key.Text()
		if k == "" {
			continue
		}
		fp := model.ParsedFormPart{Key: k, Value: p.Value.Text(), Kind: p.Kind, FilePath: p.FilePath, Disabled: p.Disabled}
		req.FormParts = append(req.FormParts, fp)
	}
	req.URLEncoded = req.URLEncoded[:0]
	for _, p := range t.URLEncoded {
		k := p.Key.Text()
		if k == "" {
			continue
		}
		req.URLEncoded = append(req.URLEncoded, model.ParsedKV{Key: k, Value: p.Value.Text(), Disabled: p.Disabled})
	}
	t.IsDirty = false
	return t.LinkedNode.Collection
}

func processTemplate(input string, env map[string]string) string {
	if env == nil || !strings.Contains(input, "{{") {
		return input
	}
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); {
		start := strings.Index(input[i:], "{{")
		if start == -1 {
			b.WriteString(input[i:])
			break
		}
		b.WriteString(input[i : i+start])
		rest := input[i+start:]
		end := strings.Index(rest[2:], "}}")
		if end == -1 {
			b.WriteString(rest)
			break
		}
		end += 4
		k := strings.TrimSpace(rest[2 : end-2])
		if v, ok := env[k]; ok {
			b.WriteString(v)
		} else {
			b.WriteString(rest[:end])
		}
		i += start + end
	}
	return b.String()
}

func (t *RequestTab) invalidateSearchCache() {
	t.RespSearch.invalidate()
}

func asciiToLower(s string) string {
	asciiOnly := true
	hasUpper := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			asciiOnly = false
			break
		}
		if c >= 'A' && c <= 'Z' {
			hasUpper = true
		}
	}
	if !asciiOnly {
		return strings.ToLower(s)
	}
	if !hasUpper {
		return s
	}
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func (t *RequestTab) AddHeader(k, v string) {
	h := &HeaderItem{IsGenerated: false}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) addSystemHeader(k, v string) {
	h := &HeaderItem{
		IsGenerated: true,
		LastAutoKey: k,
		LastAutoVal: v,
	}
	h.Key.SetText(k)
	h.Value.SetText(v)
	t.Headers = append(t.Headers, h)
}

func (t *RequestTab) UpdateSystemHeaders() {
	for _, h := range t.Headers {
		if h.IsGenerated {
			if h.Key.Text() != h.LastAutoKey || h.Value.Text() != h.LastAutoVal {
				h.IsGenerated = false
			}
		}
	}

	ua := settings.UserAgent
	if ua == "" {
		ua = model.DefaultSettings().UserAgent
	}
	sysHeaders := map[string]string{
		"User-Agent": ua,
	}
	switch t.BodyType {
	case model.BodyNone:
	case model.BodyURLEncoded:
		sysHeaders["Content-Type"] = "application/x-www-form-urlencoded"
	case model.BodyFormData:

	case model.BodyBinary:
		sysHeaders["Content-Type"] = "application/octet-stream"
	default:
		autoCT := "text/plain"
		if t.ReqEditor.Len() > 0 {
			body := t.ReqEditor.Bytes()
			i := 0
			if len(body) >= 3 && body[0] == 0xEF && body[1] == 0xBB && body[2] == 0xBF {
				i = 3
			}
			for i < len(body) && (body[i] == ' ' || body[i] == '\t' || body[i] == '\r' || body[i] == '\n') {
				i++
			}
			if i < len(body) && (body[i] == '{' || body[i] == '[') {
				autoCT = "application/json"
			}
		}
		sysHeaders["Content-Type"] = autoCT
	}

	for _, h := range t.Headers {
		if !h.IsGenerated {
			k := h.Key.Text()
			for sysK := range sysHeaders {
				if strings.EqualFold(k, sysK) {
					delete(sysHeaders, sysK)
				}
			}
		}
	}

	n := 0
	for _, h := range t.Headers {
		keep := !h.IsGenerated
		if !keep {
			_, keep = sysHeaders[h.Key.Text()]
		}
		if keep {
			t.Headers[n] = h
			n++
		}
	}
	t.Headers = t.Headers[:n]

	for k, v := range sysHeaders {
		found := false
		for _, h := range t.Headers {
			if h.IsGenerated && h.Key.Text() == k {
				if h.Value.Text() != v {
					h.Value.SetText(v)
					h.LastAutoVal = v
				}
				found = true
				break
			}
		}
		if !found {
			t.addSystemHeader(k, v)
		}
	}
}

func isURLWordSep(r rune) bool {
	if r <= ' ' {
		return true
	}
	switch r {
	case '/', '\\', ':', '?', '#', '&', '=', '.', ',', ';', '@', '(', ')', '[', ']', '{', '}', '"', '\'', '`', '<', '>', '|', '!':
		return true
	}
	return false
}

type urlVarSpan struct{ start, end int }

func findURLVarSpans(runes []rune) []urlVarSpan {
	var spans []urlVarSpan
	n := len(runes)
	for i := 0; i <= n-2; i++ {
		if runes[i] != '{' || runes[i+1] != '{' {
			continue
		}
		for j := i + 2; j <= n-2; j++ {
			if runes[j] == '}' && runes[j+1] == '}' {
				spans = append(spans, urlVarSpan{i, j + 2})
				i = j + 1
				break
			}
		}
	}
	return spans
}

func moveURLWord(s string, pos int, dir int) int {
	runes := []rune(s)
	n := len(runes)
	if pos < 0 {
		pos = 0
	}
	if pos > n {
		pos = n
	}
	spans := findURLVarSpans(runes)

	varAt := func(p int) *urlVarSpan {
		for i := range spans {
			v := &spans[i]
			if p >= v.start && p < v.end {
				return v
			}
		}
		return nil
	}
	isSep := func(p int) bool {
		if p < 0 || p >= n {
			return false
		}
		if varAt(p) != nil {
			return false
		}
		return isURLWordSep(runes[p])
	}

	if dir > 0 {
		if v := varAt(pos); v != nil {
			return v.end
		}
		for pos < n && isSep(pos) {
			pos++
		}
		if v := varAt(pos); v != nil {
			return v.end
		}
		for pos < n && !isSep(pos) {
			if varAt(pos) != nil {
				break
			}
			pos++
		}
		return pos
	}

	if v := varAt(pos); v != nil && pos > v.start {
		return v.start
	}
	if pos > 0 {
		if v := varAt(pos - 1); v != nil {
			return v.start
		}
	}
	for pos > 0 && isSep(pos-1) {
		pos--
	}
	if pos > 0 {
		if v := varAt(pos - 1); v != nil {
			return v.start
		}
	}
	for pos > 0 && !isSep(pos-1) {
		pos--
	}
	return pos
}

func urlWordBounds(s string, pos int) (int, int) {
	runes := []rune(s)
	n := len(runes)
	if pos < 0 {
		pos = 0
	}
	if pos > n {
		pos = n
	}
	spans := findURLVarSpans(runes)
	varAt := func(p int) *urlVarSpan {
		for i := range spans {
			v := &spans[i]
			if p >= v.start && p < v.end {
				return v
			}
		}
		return nil
	}
	isSep := func(p int) bool {
		if p < 0 || p >= n {
			return false
		}
		if varAt(p) != nil {
			return false
		}
		return isURLWordSep(runes[p])
	}

	ref := pos
	sepRun := false
	switch {
	case pos < n && !isSep(pos):
		if v := varAt(pos); v != nil {
			return v.start, v.end
		}
		ref = pos
	case pos > 0 && !isSep(pos-1):
		if v := varAt(pos - 1); v != nil {
			return v.start, v.end
		}
		ref = pos - 1
	default:
		sepRun = true
	}

	if sepRun {
		s := pos
		for s > 0 && isSep(s-1) {
			s--
		}
		e := pos
		for e < n && isSep(e) {
			e++
		}
		return s, e
	}

	start := ref
	for start > 0 {
		if v := varAt(start - 1); v != nil {
			start = v.end
			break
		}
		if isSep(start - 1) {
			break
		}
		start--
	}
	end := ref
	for end < n {
		if v := varAt(end); v != nil {
			break
		}
		if isSep(end) {
			break
		}
		end++
	}
	return start, end
}

func (t *RequestTab) handleURLMultiClick(gtx layout.Context, th *material.Theme, textSize unit.Sp) {
	for {
		ev, ok := t.urlClick.Update(gtx.Source)
		if !ok {
			break
		}
		if ev.Kind != gesture.KindPress || ev.Source != pointer.Mouse || ev.NumClicks < 2 {
			continue
		}
		txt := t.URLInput.Text()
		n := utf8.RuneCountInString(txt)
		if ev.NumClicks >= 3 {
			t.URLInput.SetCaret(0, n)
			gtx.Execute(key.FocusCmd{Tag: &t.URLInput})
			continue
		}
		paddingX := gtx.Dp(unit.Dp(4))
		scrollX := widgets.GetEditorScrollX(&t.URLInput)
		textX := ev.Position.X - paddingX + scrollX
		if textX < 0 {
			textX = 0
		}
		pos := widgets.CaretIndexAtX(gtx, th, textSize, txt, textX)
		start, end := urlWordBounds(txt, pos)
		t.URLInput.SetCaret(start, end)
		gtx.Execute(key.FocusCmd{Tag: &t.URLInput})
	}
}

func (t *RequestTab) handleURLWordJump(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &t.URLInput, Name: key.NameLeftArrow, Required: key.ModShortcut, Optional: key.ModShift},
			key.Filter{Focus: &t.URLInput, Name: key.NameRightArrow, Required: key.ModShortcut, Optional: key.ModShift},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		extend := ke.Modifiers.Contain(key.ModShift)
		start, end := t.URLInput.Selection()
		text := t.URLInput.Text()
		var newPos int
		switch ke.Name {
		case key.NameLeftArrow:
			newPos = moveURLWord(text, end, -1)
		case key.NameRightArrow:
			newPos = moveURLWord(text, end, 1)
		default:
			continue
		}
		if extend {
			t.URLInput.SetCaret(start, newPos)
		} else {
			t.URLInput.SetCaret(newPos, newPos)
		}
	}
}

func (t *RequestTab) handleURLWordDelete(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(
			key.Filter{Focus: &t.URLInput, Name: key.NameDeleteBackward, Required: key.ModShortcut},
			key.Filter{Focus: &t.URLInput, Name: key.NameDeleteForward, Required: key.ModShortcut},
		)
		if !ok {
			break
		}
		ke, ok := ev.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		start, end := t.URLInput.Selection()
		if start != end {
			t.URLInput.Insert("")
			continue
		}
		text := t.URLInput.Text()
		var newPos int
		switch ke.Name {
		case key.NameDeleteBackward:
			newPos = moveURLWord(text, end, -1)
		case key.NameDeleteForward:
			newPos = moveURLWord(text, end, 1)
		default:
			continue
		}
		if newPos == end {
			continue
		}
		if newPos < end {
			t.URLInput.SetCaret(newPos, end)
		} else {
			t.URLInput.SetCaret(end, newPos)
		}
		t.URLInput.Insert("")
	}
}

func (t *RequestTab) Layout(gtx layout.Context, th *material.Theme, win *app.Window, exp *explorer.Explorer, activeEnv map[string]string, isAppDragging bool, onSave func(), onCollectionDirty func(*collections.ParsedCollection)) layout.Dimensions {
	t.window = win

	select {
	case chunk := <-t.appendChan:
		curID := t.requestID.Load()
		var buf strings.Builder
		if chunk.requestID == curID {
			buf.WriteString(chunk.text)
		}
	drainLoop:
		for {
			select {
			case more := <-t.appendChan:
				if more.requestID == curID {
					buf.WriteString(more.text)
				}
			default:
				break drainLoop
			}
		}
		if appended := buf.String(); appended != "" {
			t.RespEditor.Append(appended)
			t.invalidateSearchCache()
		}
	default:
	}

	t.handleURLWordJump(gtx)
	t.handleURLWordDelete(gtx)

	for {
		ev, ok := t.URLInput.Update(gtx)
		if !ok {
			break
		}
		switch ev.(type) {
		case widget.SubmitEvent:
			t.URLSubmitted = true
		case widget.ChangeEvent:
			t.dirtyCheckNeeded = true
		}
	}

	t.handleURLMultiClick(gtx, th, unit.Sp(12))

	if t.ReqEditor.Changed() {
		t.UpdateSystemHeaders()
		t.dirtyCheckNeeded = true
		t.ReqSearch.invalidate()
	}

	select {
	case res := <-t.responseChan:
		if res.requestID == t.requestID.Load() {
			t.drainAppendChan()
			t.Status = res.status
			t.respSize = res.respSize
			t.respFile = res.respFile
			t.previewLoaded.Store(res.previewLoaded)
			t.respIsJSON = res.isJSON
			t.respContentType = res.contentType
			if res.filename != "" {
				t.SuggestedFile = res.filename
			}
			t.LastTimings = res.timings
			t.isRequesting = false
			if t.cancelFn != nil {
				t.cancelFn()
				t.cancelFn = nil
			}
			t.invalidateSearchCache()
			if t.PreviewEnabled && res.body != "" {
				if t.RespEditor.Len() != len(res.body) || !bytes.Equal(t.RespEditor.Bytes(), []byte(res.body)) {
					t.RespEditor.SetText(res.body)
				}
			} else if !t.PreviewEnabled {
				t.RespEditor.SetText("")
			}
			th.Shaper.ResetLayoutCache()
		}
	default:
	}

	select {
	case pr := <-t.previewChan:
		t.previewLoading.Store(false)
		if pr.requestID == t.requestID.Load() {
			t.previewLoaded.Store(pr.previewLoaded)
			t.respIsJSON = pr.isJSON
			t.RespEditor.SetText(pr.body)
			t.invalidateSearchCache()
			th.Shaper.ResetLayoutCache()
		}
	default:
	}

	for t.SendMenuBtn.Clicked(gtx) {
		t.SendMenuOpen = !t.SendMenuOpen
	}
	for t.ShowPreviewBtn.Clicked(gtx) {
		t.loadPreviewForSavedFile()
	}
	for t.LoadMoreBtn.Clicked(gtx) {
		t.loadMorePreview()
	}
	for t.OpenFileBtn.Clicked(gtx) {
		if t.SaveToFilePath != "" {
			go OpenFile(t.SaveToFilePath)
		}
	}
	for t.PropertiesBtn.Clicked(gtx) {
		if t.SaveToFilePath != "" {
			go openFileInExplorer(t.SaveToFilePath)
		}
	}

	for t.WrapBtn.Clicked(gtx) {
		t.WrapEnabled = !t.WrapEnabled
		th.Shaper.ResetLayoutCache()
		t.LastRespWidth = 0
		t.pendingRespWidth = 0
	}
	for t.ReqWrapBtn.Clicked(gtx) {
		t.ReqWrapEnabled = !t.ReqWrapEnabled
		th.Shaper.ResetLayoutCache()
		t.LastReqWidth = 0
		t.pendingReqWidth = 0
	}
	for t.SearchBtn.Clicked(gtx) {
		t.toggleSearch(gtx, &t.RespSearch, t.RespEditor)
	}
	for t.ReqSearchBtn.Clicked(gtx) {
		t.toggleSearch(gtx, &t.ReqSearch, &t.ReqEditor)
	}
	t.updateSearch(gtx, &t.RespSearch, t.RespEditor)
	t.updateSearch(gtx, &t.ReqSearch, &t.ReqEditor)

	for t.MethodBtn.Clicked(gtx) {
		t.MethodListOpen = !t.MethodListOpen
		t.ProtocolListOpen = false
	}
	for i := range t.MethodClickables {
		for t.MethodClickables[i].Clicked(gtx) {
			t.Method = methods[i]
			if t.Method != MethodWS {
				t.LastHTTPMethod = t.Method
			}
			t.MethodListOpen = false
			t.dirtyCheckNeeded = true
		}
	}

	for t.ProtocolBtn.Clicked(gtx) {
		t.ProtocolListOpen = !t.ProtocolListOpen
		t.MethodListOpen = false
	}
	for i := range t.ProtocolClickables {
		for t.ProtocolClickables[i].Clicked(gtx) {
			switch protocols[i] {
			case "WS":
				if t.Method != MethodWS {
					if t.Method != MethodGraphQL {
						t.LastHTTPMethod = t.Method
					}
					t.EnsureWS().SplitRatio = t.SplitRatio
					t.Method = MethodWS
				}
			case "GraphQL":
				if t.Method != MethodGraphQL {
					if t.Method == MethodWS {
						if t.WS != nil {
							t.SplitRatio = t.WS.SplitRatio
						}
					} else {
						t.LastHTTPMethod = t.Method
					}
					t.Method = MethodGraphQL
				}
			default:
				if t.Method == MethodWS || t.Method == MethodGraphQL {
					if t.Method == MethodWS && t.WS != nil {
						t.SplitRatio = t.WS.SplitRatio
					}
					if t.LastHTTPMethod == "" {
						t.LastHTTPMethod = "GET"
					}
					t.Method = t.LastHTTPMethod
				}
			}
			t.ProtocolListOpen = false
			t.dirtyCheckNeeded = true
		}
	}

	for t.AddHeaderBtn.Clicked(gtx) {
		switch t.ReqSubTab {
		case reqSubParams:
			t.addParam("", "")
			t.syncURLFromParams()
		case reqSubCookies:
			t.addCookie("", "")
		default:
			t.AddHeader("", "")
		}
		t.HeadersExpanded = true
		t.FitHeaders = true
		t.dirtyCheckNeeded = true
	}

	for t.ViewGeneratedBtn.Clicked(gtx) {
		t.HeadersExpanded = !t.HeadersExpanded
		t.reqHugPending = true
	}

	t.updateReqSubTabs(gtx)

	for i := 0; i < len(t.Headers); i++ {
		if t.Headers[i].DelBtn.Clicked(gtx) {
			t.Headers = append(t.Headers[:i], t.Headers[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
		}
	}

	if t.ReqCopyBtn.Clicked(gtx) {
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: io.NopCloser(bytes.NewReader(t.ReqEditor.Bytes())),
		})
	}

	if t.CopyBtn.Clicked(gtx) {
		var reader io.ReadCloser
		if t.respFile != "" {
			if fi, err := os.Stat(t.respFile); err == nil && fi.Size() > 0 {
				if t.respIsJSON {
					pr, pw := io.Pipe()
					respFile := t.respFile
					contentType := t.respContentType
					go func() {
						data, rerr := os.ReadFile(respFile)
						if rerr != nil {
							_ = pw.CloseWithError(rerr)
							return
						}
						decoded := utils.DecodeBody(data, contentType)
						formatted := formatJSON(decoded, &JSONFormatterState{})
						_, _ = io.WriteString(pw, formatted)
						_ = pw.Close()
					}()
					reader = pr
				} else if f, ferr := os.Open(t.respFile); ferr == nil {
					reader = f
				}
			}
		}
		if reader == nil {
			reader = io.NopCloser(bytes.NewReader(t.RespEditor.Bytes()))
		}
		gtx.Execute(clipboard.WriteCmd{
			Type: "application/text",
			Data: reader,
		})
	}

	if t.CopyAsCurlBtn.Clicked(gtx) {
		t.SendMenuOpen = false
		cmd := BuildCurlCommand(t, activeEnv)
		if cmd != "" {
			gtx.Execute(clipboard.WriteCmd{
				Type: "application/text",
				Data: io.NopCloser(strings.NewReader(cmd)),
			})
		}
	}

	if t.SaveToColBtn.Clicked(gtx) {
		if col := t.SaveToCollection(); col != nil && onCollectionDirty != nil {
			onCollectionDirty(col)
		}
	}

	if t.dirtyCheckNeeded && t.LinkedNode != nil {
		t.dirtyCheckNeeded = false
		t.checkDirty()
	}

	visibleHeaders := t.visibleHeadersBuf[:0]
	for _, h := range t.Headers {
		for {
			ev, ok := h.Key.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				t.dirtyCheckNeeded = true
			}
		}
		for {
			ev, ok := h.Value.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				t.dirtyCheckNeeded = true
			}
		}
		if t.HeadersExpanded {
			visibleHeaders = append(visibleHeaders, h)
		}
	}
	t.visibleHeadersBuf = visibleHeaders

	activeKV := visibleHeaders
	if t.HeadersExpanded {
		switch t.ReqSubTab {
		case reqSubParams:
			activeKV = t.Params
		case reqSubCookies:
			activeKV = t.Cookies
		case reqSubAuth:
			activeKV = nil
		}
	}

	for t.LayoutHorizBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeHoriz {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeHoriz
		}

		t.IsDraggingSplit = false
		t.SplitDrag = gesture.Drag{}
		win.Invalidate()
	}
	for t.LayoutVertBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeVert {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeVert
		}
		t.IsDraggingSplit = false
		t.SplitDrag = gesture.Drag{}
		win.Invalidate()
	}

	if t.Method != MethodWS {
		r := t.EnsureRun()
		for t.SingleBtn.Clicked(gtx) {
			t.RunOpen = false
			win.Invalidate()
		}
		for t.MultipleBtn.Clicked(gtx) {
			t.RunOpen = true
			win.Invalidate()
		}
		for r.AddVarBtn.Clicked(gtx) {
			r.addVar()
		}
		for i := len(r.Variables) - 1; i >= 0; i-- {
			if r.Variables[i].DelBtn.Clicked(gtx) {
				r.Variables = append(r.Variables[:i], r.Variables[i+1:]...)
			}
		}
		for r.ModeIterBtn.Clicked(gtx) {
			r.Mode = runByIterations
		}
		for r.ModeTimeBtn.Clicked(gtx) {
			r.Mode = runByDuration
		}
		for i := range r.SortBtns {
			if r.SortBtns[i].Clicked(gtx) {
				if r.SortCol == i {
					r.SortAsc = !r.SortAsc
				} else {
					r.SortCol = i
					r.SortAsc = false
				}
			}
		}
	}

	defaultMin := t.defaultPaneMinWidth(gtx, th)

	overflow := false
	{
		flexExtentH := float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
		if flexExtentH > 0 {
			splitR := t.SplitRatio
			minR := float32(defaultMin) / flexExtentH
			maxR := 1.0 - float32(gtx.Dp(unit.Dp(200)))/flexExtentH
			if minR > maxR {
				minR, maxR = 0.5, 0.5
			}
			if splitR < minR {
				splitR = minR
			} else if splitR > maxR {
				splitR = maxR
			}
			leftPaneInner := int(splitR * flexExtentH)
			if leftPaneInner < t.headersRowMinWidth(gtx, th) {
				overflow = true
			} else if t.BodyType == model.BodyURLEncoded && leftPaneInner < t.bodyTypeRowMinWidth(gtx, th) {
				overflow = true
			}
		}
	}

	var stacked bool
	switch t.LayoutMode {
	case LayoutModeHoriz:
		stacked = overflow
	case LayoutModeVert:
		stacked = true
	default:
		stacked = (settings.StackBreakpointDp > 0 && gtx.Constraints.Max.X < gtx.Dp(unit.Dp(float32(settings.StackBreakpointDp)))) || overflow
	}

	layoutSwitched := t.prevStackedInit && stacked != t.prevStacked
	t.prevStackedInit = true
	t.prevStacked = stacked
	if layoutSwitched {
		t.splitPaneRec = 0
		t.splitRespRec = 0
	}

	var ratio *float32
	var flexExtent float32
	var dragAxis gesture.Axis
	var reqMinDp, respMinDp float32

	if stacked {
		ratio = &t.VStackRatio
		flexExtent = float32(gtx.Constraints.Max.Y - gtx.Dp(unit.Dp(8)))
		dragAxis = gesture.Vertical
		reqMinDp = float32(gtx.Dp(unit.Dp(180)))
		respMinDp = float32(gtx.Dp(unit.Dp(120)))
		if t.Method != MethodWS && t.Method != MethodGraphQL {
			flexExtent = t.stackedSplitExtent(gtx)
			if pool := t.splitPaneRec + t.splitRespRec; pool > 0 {
				flexExtent = float32(pool)
			}
			reqMinDp = float32(t.stackedReqPaneMinPx(gtx))
			if t.RespBodyCollapsed {
				respMinDp = float32(t.respCollapsedMinPx(gtx))
			}
		}
	} else {
		ratio = &t.SplitRatio
		flexExtent = float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
		dragAxis = gesture.Horizontal
		reqMinDp = float32(defaultMin)
		respMinDp = float32(gtx.Dp(unit.Dp(200)))
		if pool := t.splitPaneRec + t.splitRespRec; pool > 0 && t.Method != MethodWS && t.Method != MethodGraphQL {
			flexExtent = float32(pool)
		}
	}

	if layoutSwitched && stacked && flexExtent > 0 && t.Method != MethodWS && t.Method != MethodGraphQL {
		switch {
		case t.ReqBodyCollapsed:
			minR := float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
			if *ratio > minR && !t.RespBodyCollapsed {
				t.reqRatioSaved = *ratio
			}
			*ratio = minR
		case t.RespBodyCollapsed:
			maxR := 1 - float32(t.respCollapsedMinPx(gtx))/flexExtent
			if *ratio < maxR {
				t.respRatioSaved = *ratio
			}
			*ratio = maxR
		default:
			minOpen := (float32(t.stackedReqPaneMinPx(gtx)) + float32(gtx.Dp(unit.Dp(120)))) / flexExtent
			if *ratio < minOpen {
				restore := t.reqRatioSaved
				if restore < minOpen {
					restore = minOpen
				}
				*ratio = restore
			}
		}
	}

	if t.Method != MethodWS && t.Method != MethodGraphQL {
		for t.ReqCollapseBtn.Clicked(gtx) {
			t.ReqBodyCollapsed = !t.ReqBodyCollapsed
			if stacked && flexExtent > 0 {
				if t.ReqBodyCollapsed {
					if !t.RespBodyCollapsed {
						t.reqRatioSaved = *ratio
					}
					*ratio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
				} else if t.RespBodyCollapsed {
					*ratio = 1 - float32(t.respCollapsedMinPx(gtx))/flexExtent
				} else {
					restore := t.reqRatioSaved
					minOpen := (float32(t.stackedReqPaneMinPx(gtx)) + float32(gtx.Dp(unit.Dp(120)))) / flexExtent
					if restore < minOpen {
						restore = minOpen
					}
					*ratio = restore
				}
				reqMinDp = float32(t.stackedReqPaneMinPx(gtx))
			}
			win.Invalidate()
		}
		for t.RespCollapseBtn.Clicked(gtx) {
			t.RespBodyCollapsed = !t.RespBodyCollapsed
			if stacked && flexExtent > 0 {
				if t.RespBodyCollapsed {
					if !t.ReqBodyCollapsed {
						t.respRatioSaved = *ratio
					}
					respMinDp = float32(t.respCollapsedMinPx(gtx))
					if t.ReqBodyCollapsed {
						*ratio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
					} else {
						*ratio = 1 - respMinDp/flexExtent
					}
				} else {
					respMinDp = float32(gtx.Dp(unit.Dp(120)))
					if t.ReqBodyCollapsed {
						*ratio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
					} else {
						restore := t.respRatioSaved
						if restore <= 0 {
							restore = t.reqRatioSaved
						}
						maxOpen := 1 - respMinDp/flexExtent
						minOpen := (float32(t.stackedReqPaneMinPx(gtx)) + float32(gtx.Dp(unit.Dp(120)))) / flexExtent
						if restore <= 0 || restore > maxOpen {
							restore = maxOpen
						}
						if restore < minOpen {
							restore = minOpen
						}
						*ratio = restore
					}
				}
			}
			win.Invalidate()
		}
	}

	if t.reqHugPending {
		t.reqHugPending = false
		if stacked && flexExtent > 0 && t.ReqBodyCollapsed && t.Method != MethodWS && t.Method != MethodGraphQL {
			*ratio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
			reqMinDp = float32(t.stackedReqPaneMinPx(gtx))
			win.Invalidate()
		}
	}

	if t.fitHeadersExact && t.Method != MethodWS && t.Method != MethodGraphQL {
		t.fitHeadersExact = false
		fit := t.headersFitDp(activeKV)
		if t.hbUserResized {
			manual := t.hbManualDp
			if manual <= 0 {
				manual = t.HeadersAbsHeight
			}
			if t.ReqSubTab == reqSubAuth && fit > manual {
				t.HeadersAbsHeight = fit
			} else {
				t.HeadersAbsHeight = manual
			}
		} else {
			prev := t.fitPrevHeadersDp
			t.HeadersAbsHeight = fit
			if stacked && flexExtent > 0 {
				deltaPx := gtx.Dp(unit.Dp(fit)) - gtx.Dp(unit.Dp(prev))
				if prev == 0 {
					deltaPx += gtx.Dp(unit.Dp(1))
				}
				*ratio += float32(deltaPx) / flexExtent
			}
		}
		if stacked {
			reqMinDp = float32(t.stackedReqPaneMinPx(gtx))
			if flexExtent > 0 && t.ReqBodyCollapsed {
				*ratio = reqMinDp / flexExtent
			}
		}
	}

	var moved bool
	var finalX float32
	var released bool

	for {
		e, ok := t.SplitDrag.Update(gtx.Metric, gtx.Source, dragAxis)
		if !ok {
			break
		}
		var pos float32
		if stacked {
			pos = e.Position.Y
		} else {
			pos = e.Position.X
		}
		switch e.Kind {
		case pointer.Press:
			t.SplitDragX = pos + float32(t.PaneDrawnH)
			t.IsDraggingSplit = true
			t.splitPanePx = *ratio * flexExtent
		case pointer.Drag:
			finalX = pos + float32(t.PaneDrawnH)
			moved = true
		case pointer.Cancel, pointer.Release:
			t.IsDraggingSplit = false
			released = true
		}
	}

	var minReqRatio, maxReqRatio float32
	if flexExtent > 0 {
		minReqRatio = reqMinDp / flexExtent
		maxReqRatio = 1.0 - (respMinDp / flexExtent)
	}
	if minReqRatio > maxReqRatio {
		minReqRatio = 0.5
		maxReqRatio = 0.5
	}

	if *ratio < minReqRatio {
		*ratio = minReqRatio
	} else if *ratio > maxReqRatio {
		*ratio = maxReqRatio
	}

	if moved && flexExtent > 0 {
		delta := finalX - t.SplitDragX
		oldSnap := int(*ratio*flexExtent + 0.5)
		newPane := t.splitPanePx + delta
		if stacked && t.Method != MethodWS && t.Method != MethodGraphQL {
			if !t.ReqBodyCollapsed && newPane < float32(t.stackedReqPaneMinPx(gtx))-0.5 {
				t.ReqBodyCollapsed = true
				minReqRatio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
			} else if t.ReqBodyCollapsed && newPane > float32(t.stackedReqPaneMinPx(gtx))+float32(gtx.Dp(unit.Dp(6))) {
				t.ReqBodyCollapsed = false
				minReqRatio = float32(t.stackedReqPaneMinPx(gtx)) / flexExtent
			}
			if t.RespBodyCollapsed && flexExtent-newPane > float32(t.respCollapsedMinPx(gtx))+float32(gtx.Dp(unit.Dp(6))) {
				t.RespBodyCollapsed = false
				maxReqRatio = 1 - float32(gtx.Dp(unit.Dp(120)))/flexExtent
			}
			if minReqRatio > maxReqRatio {
				minReqRatio, maxReqRatio = 0.5, 0.5
			}
		}
		t.splitPanePx = newPane
		if newPane < minReqRatio*flexExtent {
			newPane = minReqRatio * flexExtent
		} else if newPane > maxReqRatio*flexExtent {
			newPane = maxReqRatio * flexExtent
		}
		snap := oldSnap
		if d := newPane - float32(oldSnap); d >= 0.75 || d <= -0.75 {
			snap = int(newPane + 0.5)
		}
		*ratio = float32(snap) / flexExtent
		t.SplitDragX = finalX
		win.Invalidate()
	}
	if released {
		if onSave != nil {
			onSave()
		}
		win.Invalidate()
	}

	var hbMoved bool
	var hbFinalPos float32
	var hbReleased bool
	for {
		e, ok := t.HeadersBodyDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		pos := e.Position.Y + float32(t.hbSliderY)
		switch e.Kind {
		case pointer.Press:
			t.HeadersBodyDragX = pos
			t.hbHeadersPx = 0
			if t.HeadersExpanded {
				t.hbHeadersPx = float32(t.headersRenderH)
			}
			if stacked && flexExtent > 0 {
				above := t.headersRowPx(gtx)
				headersNow := 0
				if t.HeadersExpanded {
					above += gtx.Dp(unit.Dp(1))
					headersNow = t.headersRenderH
				}
				t.hbEditorPx = int(*ratio*flexExtent) - above - headersNow - t.reqPaneBelowHeadersPx(gtx)
				if t.hbEditorPx < 0 {
					t.hbEditorPx = 0
				}
			}
		case pointer.Drag:
			hbFinalPos = pos
			hbMoved = true
		case pointer.Cancel, pointer.Release:
			hbReleased = true
		}
	}

	if hbMoved {
		t.hbUserResized = true
		delta := hbFinalPos - t.HeadersBodyDragX
		oldSnap := float32(0)
		if t.HeadersExpanded {
			oldSnap = float32(gtx.Dp(unit.Dp(t.HeadersAbsHeight)))
		}
		newH := t.hbHeadersPx + delta
		row := float32(t.headersRowPx(gtx))
		line := float32(gtx.Dp(unit.Dp(1)))
		below := float32(t.reqPaneBelowHeadersPx(gtx))
		hbMaxPx := newH
		if stacked && flexExtent > 0 {
			hbMaxPx = flexExtent - respMinDp - row - line - below - float32(t.hbEditorPx)
		} else if t.reqPaneH > 0 {
			hbMaxPx = float32(t.reqPaneH) - row - line - below
		}
		if hbMaxPx < 0 {
			hbMaxPx = 0
		}
		t.hbHeadersPx = newH
		if newH > hbMaxPx {
			newH = hbMaxPx
		}
		if newH < 0 {
			newH = 0
		}
		newSnap := oldSnap
		if newH <= 0.5 {
			if t.HeadersExpanded {
				t.HeadersExpanded = false
				t.HeadersAbsHeight = 120
			}
			newSnap = 0
			if stacked && flexExtent > 0 {
				*ratio = (row + below + float32(t.hbEditorPx)) / flexExtent
			}
		} else {
			wasExpanded := t.HeadersExpanded
			t.HeadersExpanded = true
			if d := newH - oldSnap; !wasExpanded || d >= 0.75 || d <= -0.75 {
				t.HeadersAbsHeight = int(newH/gtx.Metric.PxPerDp + 0.5)
			}
			newSnap = float32(gtx.Dp(unit.Dp(t.HeadersAbsHeight)))
			if stacked && flexExtent > 0 {
				*ratio = (row + line + newSnap + below + float32(t.hbEditorPx)) / flexExtent
			}
		}
		t.hbManualDp = t.HeadersAbsHeight
		t.HeadersBodyDragX = hbFinalPos
		win.Invalidate()
	}
	if hbReleased {
		if onSave != nil {
			onSave()
		}
		win.Invalidate()
	}

	isDragging := isAppDragging || t.IsDraggingSplit

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(8), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				btnH := gtx.Dp(unit.Dp(28))
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = btnH
						gtx.Constraints.Max.Y = btnH
						return layout.Stack{Alignment: layout.NW}.Layout(gtx,
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if !t.ProtocolListOpen {
									return layout.Dimensions{}
								}
								items := make([]widgets.MenuItem, len(protocols))
								curProto := "HTTP"
								switch t.Method {
								case MethodWS:
									curProto = "WS"
								case MethodGraphQL:
									curProto = "GraphQL"
								}
								for i, p := range protocols {
									items[i] = widgets.MenuItem{
										Label:   p,
										Click:   &t.ProtocolClickables[i],
										Checked: p == curProto,
										Mono:    true,
									}
								}
								anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(32)))}
								widgets.DeferMenuAt(gtx, th, &t.ProtocolListOpen, anchor, 120, items)
								return layout.Dimensions{}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									protoLabel := "HTTP"
									switch t.Method {
									case MethodWS:
										protoLabel = "WS"
									case MethodGraphQL:
										protoLabel = "GraphQL"
									}
									btn := widgets.MonoButton(th, &t.ProtocolBtn, protoLabel)
									btn.Background = theme.BgSecondary
									btn.Color = th.Fg
									btn.TextSize = unit.Sp(12)
									btn.CornerRadius = unit.Dp(0)
									btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(7), Left: unit.Dp(8), Right: unit.Dp(8)}
									return btn.Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if t.Method == MethodWS || t.Method == MethodGraphQL {
							return layout.Dimensions{}
						}
						return layout.Spacer{Width: unit.Dp(4)}.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if t.Method == MethodWS || t.Method == MethodGraphQL {
							return layout.Dimensions{}
						}
						gtx.Constraints.Min.Y = btnH
						gtx.Constraints.Max.Y = btnH
						return layout.Stack{Alignment: layout.NW}.Layout(gtx,
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								if !t.MethodListOpen {
									return layout.Dimensions{}
								}
								items := make([]widgets.MenuItem, len(methods))
								for i, m := range methods {
									items[i] = widgets.MenuItem{
										Label:    m,
										Click:    &t.MethodClickables[i],
										Checked:  m == t.Method,
										Mono:     true,
										LabelCol: theme.MethodColor(m),
									}
								}
								anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(32)))}
								widgets.DeferMenuAt(gtx, th, &t.MethodListOpen, anchor, 120, items)
								return layout.Dimensions{}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									btn := widgets.MonoButton(th, &t.MethodBtn, t.Method)
									btn.Background = theme.BgSecondary
									btn.Color = theme.MethodColor(t.Method)
									btn.TextSize = unit.Sp(12)
									btn.CornerRadius = unit.Dp(0)
									btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(7), Left: unit.Dp(8), Right: unit.Dp(8)}
									return btn.Layout(gtx)
								})
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = btnH
						gtx.Constraints.Max.Y = btnH
						frozenURLWidth := 0
						if isDragging && t.LastURLWidth > 0 {
							frozenURLWidth = t.LastURLWidth
						} else {
							t.LastURLWidth = gtx.Constraints.Max.X
						}
						urlHint := "https://api.example.com"
						if t.Method == MethodWS {
							urlHint = "ws://example.com/socket or wss://example.com/socket"
						}
						dims := widgets.TextFieldOverlay(gtx, th, &t.URLInput, urlHint, true, activeEnv, frozenURLWidth, unit.Sp(12))
						pass := pointer.PassOp{}.Push(gtx.Ops)
						cl := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
						t.urlClick.Add(gtx.Ops)
						cl.Pop()
						pass.Pop()
						return dims
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = btnH
						gtx.Constraints.Max.Y = btnH
						btnMinW := gtx.Dp(unit.Dp(90))
						sendLabelW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), font.Font{Typeface: th.Face}, "RERUN")
						actionBtnW := gtx.Dp(unit.Dp(16)) + sendLabelW + gtx.Dp(unit.Dp(12)) + gtx.Dp(unit.Dp(1)) + gtx.Dp(unit.Dp(24))
						if actionBtnW < btnMinW {
							actionBtnW = btnMinW
						}
						if t.isRequesting {
							gtx.Constraints.Min.X = actionBtnW
							gtx.Constraints.Max.X = actionBtnW
							return t.CancelBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								pointer.CursorPointer.Add(gtx.Ops)
								macro := op.Record(gtx.Ops)
								dims := layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(th, unit.Sp(12), "CANCEL")
									lbl.Color = theme.DangerFg
									return lbl.Layout(gtx)
								})
								call := macro.Stop()
								rr := clip.UniformRRect(image.Rectangle{Max: dims.Size}, gtx.Dp(unit.Dp(4)))
								paint.FillShape(gtx.Ops, theme.Cancel, rr.Op(gtx.Ops))
								call.Add(gtx.Ops)
								return dims
							})
						}

						bgColor := theme.BtnPrimary
						sendFg := theme.BtnPrimaryFg
						sendLabel := "SEND"
						if t.RunOpen {
							sendLabel, bgColor = t.runnerSendLabel()
						}
						cornerR := gtx.Dp(unit.Dp(4))
						gtx.Constraints.Min.X = actionBtnW

						if t.RunOpen {
							runnerBtnW := sendLabelW + 2*gtx.Dp(unit.Dp(14))
							gtx.Constraints.Min.X = runnerBtnW
							gtx.Constraints.Max.X = runnerBtnW
							return t.SendBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								pointer.CursorPointer.Add(gtx.Ops)
								sz := image.Pt(runnerBtnW, btnH)
								paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(image.Rectangle{Max: sz}, cornerR).Op(gtx.Ops))
								layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									lbl := material.Label(th, unit.Sp(12), sendLabel)
									lbl.Color = sendFg
									lbl.Alignment = text.Middle
									lbl.MaxLines = 1
									return lbl.Layout(gtx)
								})
								return layout.Dimensions{Size: sz}
							})
						}

						sendMacro := op.Record(gtx.Ops)
						sendDims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendBtn, func(gtx layout.Context) layout.Dimensions {
									pointer.CursorPointer.Add(gtx.Ops)
									return layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = sendLabelW
										gtx.Constraints.Max.X = sendLabelW
										lbl := material.Label(th, unit.Sp(12), sendLabel)
										lbl.Color = sendFg
										lbl.Alignment = text.Middle
										return lbl.Layout(gtx)
									})
								})
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								h := gtx.Dp(unit.Dp(20))
								w := gtx.Dp(unit.Dp(1))
								paint.FillShape(gtx.Ops, theme.DividerLight, clip.Rect{Max: image.Pt(w, h)}.Op())
								return layout.Dimensions{Size: image.Pt(w, h)}
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendMenuBtn, func(gtx layout.Context) layout.Dimensions {
									pointer.CursorPointer.Add(gtx.Ops)
									return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(0), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										is := gtx.Dp(unit.Dp(20))
										gtx.Constraints.Min = image.Point{X: is, Y: is}
										gtx.Constraints.Max = gtx.Constraints.Min
										return widgets.IconDropDown.Layout(gtx, sendFg)
									})
								})
							}),
						)
						sendCall := sendMacro.Stop()

						sz := sendDims.Size
						paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(image.Rectangle{Max: sz}, cornerR).Op(gtx.Ops))
						sendCall.Add(gtx.Ops)

						if t.SendMenuOpen {
							anchor := widgets.MenuAnchor{
								Pt:         image.Pt(sz.X, sz.Y+gtx.Dp(unit.Dp(2))),
								AlignRight: true,
							}
							widgets.DeferMenuAt(gtx, th, &t.SendMenuOpen, anchor, widgets.MenuMinWidthDp, []widgets.MenuItem{
								{Label: "Save to file…", Click: &t.SaveToFileBtn, Icon: widgets.IconSave},
								{Label: "Copy as cURL", Click: &t.CopyAsCurlBtn, Icon: widgets.IconDup},
							})
						}

						return sendDims
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if t.Method == MethodWS {
				return t.layoutWSBody(gtx, th, win, activeEnv)
			}
			if t.Method == MethodGraphQL {
				return t.layoutGraphQLBody(gtx, th, win, activeEnv, ratio, stacked, isDragging)
			}
			flexAxis := layout.Horizontal
			if stacked {
				flexAxis = layout.Vertical
			}
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return t.layoutModeBar(gtx, th, &t.LayoutHorizBtn, &t.LayoutVertBtn, stacked)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: flexAxis}.Layout(gtx,
							layout.Flexed(*ratio, func(gtx layout.Context) layout.Dimensions {
								if stacked {
									t.splitPaneRec = gtx.Constraints.Max.Y
								} else {
									t.splitPaneRec = gtx.Constraints.Max.X
								}
								d := func(gtx layout.Context) layout.Dimensions {
									t.reqPaneH = gtx.Constraints.Max.Y
									bottomAnchored := t.ReqBodyCollapsed && !stacked
									if bottomAnchored && !t.HeadersExpanded {
										compact := t.headersRowPx(gtx) + t.reqPaneBelowHeadersContentPx(gtx)
										if gtx.Constraints.Max.Y > compact {
											gtx.Constraints.Max.Y = compact
										}
										gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
									}
									headersRowAt := func(gtx layout.Context, vInset unit.Dp) layout.Dimensions {
										return layout.Inset{Top: vInset, Bottom: vInset}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											children := t.reqTabsChildren(gtx, th)
											children = append(children,
												layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if t.ReqSubTab == reqSubAuth {
														return layout.Dimensions{}
													}
													return widgets.SquareBtn(gtx, &t.AddHeaderBtn, widgets.IconAdd, th)
												}),
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													return collapseChevron(gtx, th, &t.ViewGeneratedBtn, !t.HeadersExpanded)
												}),
											)
											return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx, children...)
										})
									}

									kvBody := func(gtx layout.Context) layout.Dimensions {
										bdr := gtx.Dp(unit.Dp(1))
										sz := gtx.Constraints.Max
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
										inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
										paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.Rect(inner).Op())
										gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
										gtx.Constraints.Max = gtx.Constraints.Min
										op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
										if t.ReqSubTab == reqSubAuth {
											return t.layoutAuthPanel(gtx, th, activeEnv)
										}
										kvList := t.activeKVList()
										if len(activeKV) == 0 {
											return layout.Dimensions{Size: gtx.Constraints.Min}
										}
										minKey := widgets.KVKeysMinWidth(gtx, th, len(activeKV), func(i int) *widget.Editor { return &activeKV[i].Key })
										return layout.UniformInset(unit.Dp(2)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return widgets.VScrollList(gtx, th, kvList, len(activeKV), func(gtx layout.Context, i int) layout.Dimensions {
												h := activeKV[i]
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(0), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return widgets.KVRow(gtx, th, &h.Key, &h.Value, &h.DelBtn, &t.HeaderKeyW, &h.SplitDrag, &h.splitLastX, &t.HeaderKeyBelowMin, minKey, activeEnv, &h.RowHover, &h.RowFade)
														})
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														if i >= len(activeKV)-1 {
															return layout.Dimensions{}
														}
														return rowDivider(gtx)
													}),
												)
											})
										})
									}

									sliderHandle := func(gtx layout.Context) layout.Dimensions {
										thick := gtx.Dp(unit.Dp(4))
										size := image.Point{X: gtx.Constraints.Max.X, Y: thick}
										rect := clip.Rect{Max: size}
										defer rect.Push(gtx.Ops).Pop()
										pointer.CursorRowResize.Add(gtx.Ops)
										t.HeadersBodyDrag.Add(gtx.Ops)
										for {
											_, ok := gtx.Event(pointer.Filter{Target: &t.HeadersBodyDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
											if !ok {
												break
											}
										}
										return layout.Dimensions{Size: size}
									}

									reqHeaderRow := func(gtx layout.Context) layout.Dimensions {
										rowH := t.headersRowPx(gtx)
										inner := gtx
										inner.Constraints.Min.Y = 0
										inner.Constraints.Max.Y = rowH
										macro := op.Record(gtx.Ops)
										dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(inner,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													lbl := widgets.MonoLabel(th, unit.Sp(12), "Request")
													lbl.Font.Weight = font.Bold
													return lbl.Layout(gtx)
												})
											}),
											layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return t.layoutBodyTypeSelector(gtx, th)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if t.BodyType != model.BodyRaw {
													return layout.Dimensions{}
												}
												return widgets.SquareBtn(gtx, &t.ReqWrapBtn, iconWrap, th)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if t.BodyType != model.BodyRaw {
													return layout.Dimensions{}
												}
												return widgets.SquareBtn(gtx, &t.ReqSearchBtn, widgets.IconSearch, th)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return widgets.SquareBtn(gtx, &t.ReqCopyBtn, iconCopy, th)
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return collapseChevron(gtx, th, &t.ReqCollapseBtn, t.ReqBodyCollapsed)
											}),
										)
										call := macro.Stop()
										off := (rowH - dims.Size.Y) / 2
										if off < 0 {
											off = 0
										}
										st := op.Offset(image.Pt(0, off)).Push(gtx.Ops)
										call.Add(gtx.Ops)
										st.Pop()
										t.reqHeaderH = rowH
										return layout.Dimensions{Size: image.Pt(dims.Size.X, rowH)}
									}

									editorBody := func(gtx layout.Context) layout.Dimensions {
										bdr := gtx.Dp(unit.Dp(1))
										sz := gtx.Constraints.Max
										paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
										inner := image.Rect(bdr, 0, sz.X-bdr, sz.Y-bdr)
										paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.Rect(inner).Op())
										gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
										gtx.Constraints.Max = gtx.Constraints.Min
										op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
										drawRaw := func(gtx layout.Context) layout.Dimensions {
											return layout.Stack{}.Layout(gtx,
												layout.Expanded(func(gtx layout.Context) layout.Dimensions {
													style := RequestEditorStyle{
														Viewer:           &t.ReqEditor,
														Shaper:           th.Shaper,
														Font:             widgets.MonoFont,
														TextSize:         settings.BodyTextSize,
														Color:            theme.Fg,
														HighlightColor:   theme.WithAlpha(theme.Accent, 150),
														SearchMatchColor: theme.WithAlpha(theme.Accent, 60),
														SelectionColor:   theme.Selection,
														Wrap:             t.ReqWrapEnabled,
														Padding:          settings.RespBodyPad,
														Env:              activeEnv,
														Lang:             t.requestLang(),
														Syntax:           theme.Syntax,
														BracketCycle:     settings.BracketColorization,
													}
													return style.Layout(gtx)
												}),
												layout.Stacked(func(gtx layout.Context) layout.Dimensions {
													return t.layoutReqScrollbar(gtx, win)
												}),
												layout.Stacked(func(gtx layout.Context) layout.Dimensions {
													if t.ReqWrapEnabled {
														return layout.Dimensions{}
													}
													return t.layoutReqHScrollbar(gtx, win)
												}),
												layout.Stacked(func(gtx layout.Context) layout.Dimensions {
													return t.layoutOversizeBanner(gtx, th)
												}),
												layout.Stacked(func(gtx layout.Context) layout.Dimensions {
													return t.layoutSearchOverlay(gtx, th, &t.ReqSearch)
												}),
												layout.Stacked(func(gtx layout.Context) layout.Dimensions {
													return t.ReqEditor.LayoutScrollbarHover(gtx, t.ReqScrollDrag.Dragging() || t.ReqHScrollDrag.Dragging())
												}),
											)
										}
										return t.layoutBody(gtx, th, win, exp, activeEnv, drawRaw)
									}

									sliderTop := 0
									children := []layout.FlexChild{
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											d := headersRowAt(gtx, unit.Dp(2))
											t.headersRowH = d.Size.Y
											sliderTop = d.Size.Y
											return d
										}),
									}
									if t.HeadersExpanded {
										children = append(children,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												d := wsHLine(gtx)
												sliderTop += d.Size.Y
												return d
											}),
										)
										if bottomAnchored {
											children = append(children,
												layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
													h := gtx.Constraints.Max.Y
													t.headersRenderH = h
													t.hbSliderY = sliderTop + h
													gtx.Constraints.Min.Y = h
													gtx.Constraints.Max.Y = h
													d := kvBody(gtx)
													d.Size.Y = h
													return d
												}),
											)
										} else {
											children = append(children,
												layout.Rigid(func(gtx layout.Context) layout.Dimensions {
													if t.HeadersAbsHeight <= 0 {
														t.HeadersAbsHeight = 120
													}
													if t.FitHeaders {
														if fit := t.headersFitDp(activeKV); fit > t.HeadersAbsHeight {
															t.HeadersAbsHeight = fit
														}
														t.FitHeaders = false
													}
													h := gtx.Dp(unit.Dp(t.HeadersAbsHeight))
													available := t.reqPaneH - t.reqPaneAboveHeadersPx(gtx) - t.reqPaneBelowHeadersContentPx(gtx)
													if available < 0 {
														available = 0
													}
													if h > available {
														h = available
													}
													if h < 0 {
														h = 0
													}
													t.headersRenderH = h
													gtx.Constraints.Min.Y = h
													gtx.Constraints.Max.Y = h
													d := kvBody(gtx)
													d.Size.Y = h
													sliderTop += d.Size.Y
													return d
												}),
											)
										}
									}
									children = append(children,
										layout.Rigid(func(gtx layout.Context) layout.Dimensions {
											if !(bottomAnchored && t.HeadersExpanded) {
												t.hbSliderY = sliderTop
											}
											return sliderHandle(gtx)
										}),
									)
									children = append(children,
										layout.Rigid(wsHLine),
										layout.Rigid(reqHeaderRow),
									)
									if !t.ReqBodyCollapsed {
										children = append(children,
											layout.Rigid(wsHLine),
											layout.Flexed(1, editorBody),
										)
									}
									paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
									dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
									widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
									return dims
								}(gtx)
								t.reqPaneBoxH = d.Size.Y
								if stacked {
									t.PaneDrawnH = d.Size.Y
								} else {
									t.PaneDrawnH = d.Size.X
								}
								return d
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								thick := gtx.Dp(unit.Dp(4))
								var size image.Point
								var cursor pointer.Cursor
								if stacked {
									size = image.Point{X: gtx.Constraints.Min.X, Y: thick}
									cursor = pointer.CursorRowResize
								} else {
									size = image.Point{X: thick, Y: gtx.Constraints.Min.Y}
									cursor = pointer.CursorColResize
								}
								rect := clip.Rect{Max: size}
								defer rect.Push(gtx.Ops).Pop()
								cursor.Add(gtx.Ops)

								t.SplitDrag.Add(gtx.Ops)
								for {
									_, ok := gtx.Event(pointer.Filter{Target: &t.SplitDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
									if !ok {
										break
									}
								}
								return layout.Dimensions{Size: size}
							}),
							layout.Flexed(1-*ratio, func(gtx layout.Context) layout.Dimensions {
								if stacked {
									t.splitRespRec = gtx.Constraints.Max.Y
								} else {
									t.splitRespRec = gtx.Constraints.Max.X
								}
								d := func(gtx layout.Context) layout.Dimensions {
									if stacked && t.RespBodyCollapsed && t.respHeaderH > 0 {
										capped := t.respHeaderH + 2*gtx.Dp(unit.Dp(1))
										if gtx.Constraints.Max.Y > capped {
											gtx.Constraints.Max.Y = capped
										}
										gtx.Constraints.Min.Y = gtx.Constraints.Max.Y
									}
									paint.FillShape(gtx.Ops, theme.Bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
									respHdrH := 0
									dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												d := t.layoutExampleNameRow(gtx, th)
												respHdrH = d.Size.Y
												return d
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												rowH := t.headersRowPx(gtx)
												inner := gtx
												inner.Constraints.Min.Y = 0
												inner.Constraints.Max.Y = rowH
												macro := op.Record(gtx.Ops)
												fd := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(inner,
														layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															gtx.Constraints.Min.Y = 0
															return layout.Inset{Left: unit.Dp(9)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																statusText := t.Status
																if t.RunOpen {
																	statusText = t.runnerStatusText()
																} else if t.isRequesting {
																	dl := t.downloadedBytes.Load()
																	if dl > 0 {
																		statusText = "Downloading... " + formatSize(dl)
																	}
																}
																lbl := widgets.MonoLabel(th, unit.Sp(12), statusText)
																lbl.Font.Weight = font.Bold
																lbl.MaxLines = 1
																lbl.Truncator = "…"
																return lbl.Layout(gtx)
															})
														}),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if t.RunOpen {
																return layout.Dimensions{}
															}
															if t.SaveToFilePath != "" && !t.PreviewEnabled {
																return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
																	layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																		btn := widgets.MonoButton(th, &t.OpenFileBtn, "Open")
																		btn.TextSize = unit.Sp(10)
																		btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
																		return btn.Layout(gtx)
																	}),
																	layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
																	layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																		btn := widgets.MonoButton(th, &t.PropertiesBtn, "Location")
																		btn.TextSize = unit.Sp(10)
																		btn.Background = theme.BgSecondary
																		btn.Inset = layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}
																		return btn.Layout(gtx)
																	}),
																)
															}
															return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	return widgets.SquareBtn(gtx, &t.SearchBtn, widgets.IconSearch, th)
																}),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	return widgets.SquareBtn(gtx, &t.WrapBtn, iconWrap, th)
																}),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	return widgets.SquareBtn(gtx, &t.CopyBtn, iconCopy, th)
																}),
															)
														}),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if !stacked {
																return layout.Dimensions{}
															}
															return collapseChevron(gtx, th, &t.RespCollapseBtn, t.RespBodyCollapsed)
														}),
												)
												call := macro.Stop()
												off := (rowH - fd.Size.Y) / 2
												if off < 0 {
													off = 0
												}
												st := op.Offset(image.Pt(0, off)).Push(gtx.Ops)
												call.Add(gtx.Ops)
												st.Pop()
												d := layout.Dimensions{Size: image.Pt(fd.Size.X, rowH)}
												respHdrH += d.Size.Y
												t.respHeaderH = respHdrH
												return d
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if stacked && t.RespBodyCollapsed {
													return layout.Dimensions{}
												}
												return wsHLine(gtx)
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												if stacked && t.RespBodyCollapsed {
													return layout.Dimensions{}
												}
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return layout.Stack{}.Layout(gtx,
															layout.Expanded(func(gtx layout.Context) layout.Dimensions {
																return t.layoutResponseBody(gtx, th, win, isDragging)
															}),
															layout.Stacked(func(gtx layout.Context) layout.Dimensions {
																return t.layoutSearchOverlay(gtx, th, &t.RespSearch)
															}),
														)
													}),
													layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														loaded := t.previewLoaded.Load()
														if !t.PreviewEnabled || loaded == 0 || loaded >= t.respSize {
															return layout.Dimensions{}
														}
														return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
															return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																remaining := t.respSize - loaded
																label := "Load more (" + formatSize(remaining) + " remaining)"
																btn := widgets.MonoButton(th, &t.LoadMoreBtn, label)
																btn.TextSize = unit.Sp(11)
																btn.Background = theme.BgLoadMore
																btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(12), Right: unit.Dp(12)}
																return btn.Layout(gtx)
															})
														})
													}),
												)
											}),
										)
									widgets.PaintBorder1px(gtx, dims.Size, theme.Border)
									return dims
								}(gtx)
								t.respPaneBoxH = d.Size.Y
								return d
							}),
						)
					}),
				)
			})
		}),
	)
}

func (t *RequestTab) layoutResponseBody(gtx layout.Context, th *material.Theme, win *app.Window, isDragging bool) layout.Dimensions {
	if t.RunOpen {
		return t.layoutRunner(gtx, th, win)
	}
	if !t.PreviewEnabled && !t.isRequesting && t.respSize > 0 {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					msg := "Response saved to file (" + formatSize(t.respSize) + ")"
					if t.SaveToFilePath != "" {
						msg += "\n" + filepath.Base(t.SaveToFilePath)
					}
					lbl := widgets.MonoLabel(th, unit.Sp(13), msg)
					lbl.Alignment = text.Middle
					lbl.Color = theme.FgHint
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(12)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if t.respFile == "" {
						return layout.Dimensions{}
					}
					btn := widgets.MonoButton(th, &t.ShowPreviewBtn, "Show in app")
					btn.TextSize = unit.Sp(12)
					btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
					return btn.Layout(gtx)
				}),
			)
		})
	}

	bdr := gtx.Dp(unit.Dp(1))
	rsz := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: rsz}.Op())
	rInner := image.Rect(bdr, 0, rsz.X-bdr, rsz.Y-bdr)
	paint.FillShape(gtx.Ops, widgets.KVSurface(), clip.Rect(rInner).Op())
	op.Offset(image.Pt(bdr, 0)).Add(gtx.Ops)
	gtx.Constraints.Min = image.Pt(rInner.Dx(), rInner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			lang := t.responseLang()
			vs := ResponseViewerStyle{
				Viewer:           t.RespEditor,
				Shaper:           th.Shaper,
				Font:             widgets.MonoFont,
				TextSize:         settings.BodyTextSize,
				Color:            theme.Fg,
				HighlightColor:   theme.WithAlpha(theme.Accent, 150),
				SearchMatchColor: theme.WithAlpha(theme.Accent, 60),
				SelectionColor:   theme.Selection,
				Wrap:             t.WrapEnabled,
				Padding:          settings.RespBodyPad,
				Lang:             lang,
				Syntax:           theme.Syntax,
				BracketCycle:     settings.BracketColorization,
			}
			return vs.Layout(gtx)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			bounds := t.RespEditor.GetScrollBounds()
			totalH := float32(bounds.Max.Y)
			viewH := float32(gtx.Constraints.Max.Y)

			if totalH <= viewH || totalH == 0 {
				return layout.Dimensions{}
			}

			scrollY := float32(t.RespEditor.GetScrollY())
			maxScroll := totalH - viewH
			if maxScroll <= 0 {
				maxScroll = 1
			}

			scrollFraction := scrollY / maxScroll
			if scrollFraction < 0 {
				scrollFraction = 0
			}
			if scrollFraction > 1 {
				scrollFraction = 1
			}

			thumbH := viewH * (viewH / totalH)
			if thumbH < 20 {
				thumbH = 20
			}

			thumbY := scrollFraction * (viewH - thumbH)
			trackWidth := float32(gtx.Dp(unit.Dp(10)))
			thumbWidth := float32(gtx.Dp(unit.Dp(6)))

			trackRect := image.Rect(
				gtx.Constraints.Max.X-int(trackWidth), 0,
				gtx.Constraints.Max.X, gtx.Constraints.Max.Y,
			)

			stack := clip.Rect(trackRect).Push(gtx.Ops)
			for {
				e, ok := t.ScrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
				if !ok {
					break
				}
				switch e.Kind {
				case pointer.Press:
					t.ScrollDragY = e.Position.Y
				case pointer.Drag:
					delta := e.Position.Y - t.ScrollDragY
					t.ScrollDragY = e.Position.Y
					var contentDelta float32
					if viewH > thumbH {
						contentDelta = delta / (viewH - thumbH) * maxScroll
					}
					scrollY += contentDelta
					newScrollY := int(scrollY)
					if newScrollY < 0 {
						newScrollY = 0
					}
					t.RespEditor.SetScrollY(newScrollY)
					win.Invalidate()
				}
			}
			pointer.CursorDefault.Add(gtx.Ops)
			t.ScrollDrag.Add(gtx.Ops)
			stack.Pop()

			fade := t.RespEditor.ScrollbarFade()
			if fade <= 0 {
				return layout.Dimensions{}
			}
			col := theme.ScrollThumb
			col.A = uint8(float32(col.A) * fade)
			rect := image.Rect(
				gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
				int(thumbY),
				gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
				int(thumbY+thumbH),
			)
			paint.FillShape(gtx.Ops, col, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if t.WrapEnabled {
				return layout.Dimensions{}
			}
			return t.layoutRespHScrollbar(gtx, win)
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return t.RespEditor.LayoutScrollbarHover(gtx, t.ScrollDrag.Dragging() || t.HScrollDrag.Dragging())
		}),
	)
}

func (t *RequestTab) layoutOversizeBanner(gtx layout.Context, th *material.Theme) layout.Dimensions {
	msg := t.ReqEditor.OversizeMsg()
	if msg == "" {
		return layout.Dimensions{}
	}

	for t.DismissOversizeBtn.Clicked(gtx) {
		t.ReqEditor.DismissOversize()
	}

	bg := theme.Danger
	fg := theme.DangerFg

	return layout.Inset{Top: unit.Dp(0)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		macro := op.Record(gtx.Ops)
		dim := layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(th, unit.Sp(12), "⚠ "+msg)
					lbl.Color = fg
					lbl.MaxLines = 2
					return lbl.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := widgets.PrimaryButton(th, &t.LoadFromFileBtn, "Load from file…")
					btn.TextSize = unit.Sp(11)
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := widgets.FilledButton(th, &t.DismissOversizeBtn, "Dismiss", theme.Border, th.Fg)
					btn.TextSize = unit.Sp(11)
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
			)
		})
		call := macro.Stop()

		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, dim.Size.Y)}.Op())
		call.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, dim.Size.Y)}
	})
}

func (t *RequestTab) layoutReqHScrollbar(gtx layout.Context, win *app.Window) layout.Dimensions {
	return layoutHScrollbar(gtx, win, t.ReqEditor.GetMaxLineWidth(), t.ReqEditor.GetScrollX(), &t.ReqHScrollDrag, &t.ReqHScrollDragX, t.ReqEditor.ScrollbarFade(), func(x int) {
		t.ReqEditor.SetScrollX(x)
	})
}

func (t *RequestTab) layoutRespHScrollbar(gtx layout.Context, win *app.Window) layout.Dimensions {
	return layoutHScrollbar(gtx, win, t.RespEditor.GetMaxLineWidth(), t.RespEditor.GetScrollX(), &t.HScrollDrag, &t.HScrollDragX, t.RespEditor.ScrollbarFade(), func(x int) {
		t.RespEditor.SetScrollX(x)
	})
}

func layoutHScrollbar(gtx layout.Context, win *app.Window, totalW int, currentX int, drag *gesture.Drag, dragOriginX *float32, fade float32, setX func(int)) layout.Dimensions {
	viewW := float32(gtx.Constraints.Max.X)
	totalWf := float32(totalW)
	if totalWf <= viewW || totalWf == 0 {
		return layout.Dimensions{}
	}
	maxScroll := totalWf - viewW
	if maxScroll <= 0 {
		maxScroll = 1
	}
	scrollX := float32(currentX)
	scrollFraction := scrollX / maxScroll
	if scrollFraction < 0 {
		scrollFraction = 0
	}
	if scrollFraction > 1 {
		scrollFraction = 1
	}
	thumbW := viewW * (viewW / totalWf)
	if thumbW < 20 {
		thumbW = 20
	}
	thumbX := scrollFraction * (viewW - thumbW)

	trackHeight := float32(gtx.Dp(unit.Dp(10)))
	thumbHeight := float32(gtx.Dp(unit.Dp(6)))

	trackRect := image.Rect(
		0, gtx.Constraints.Max.Y-int(trackHeight),
		gtx.Constraints.Max.X, gtx.Constraints.Max.Y,
	)

	stack := clip.Rect(trackRect).Push(gtx.Ops)
	for {
		e, ok := drag.Update(gtx.Metric, gtx.Source, gesture.Horizontal)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			*dragOriginX = e.Position.X
		case pointer.Drag:
			delta := e.Position.X - *dragOriginX
			*dragOriginX = e.Position.X
			var contentDelta float32
			if viewW > thumbW {
				contentDelta = delta / (viewW - thumbW) * maxScroll
			}
			scrollX += contentDelta
			newScrollX := int(scrollX)
			if newScrollX < 0 {
				newScrollX = 0
			}
			if float32(newScrollX) > maxScroll {
				newScrollX = int(maxScroll)
			}
			setX(newScrollX)
			win.Invalidate()
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	drag.Add(gtx.Ops)
	stack.Pop()

	if fade <= 0 {
		return layout.Dimensions{}
	}
	col := theme.ScrollThumb
	col.A = uint8(float32(col.A) * fade)
	rect := image.Rect(
		int(thumbX),
		gtx.Constraints.Max.Y-int(thumbHeight)-gtx.Dp(unit.Dp(2)),
		int(thumbX+thumbW),
		gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(2)),
	)
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

	return layout.Dimensions{}
}

func (t *RequestTab) layoutReqScrollbar(gtx layout.Context, win *app.Window) layout.Dimensions {
	bounds := t.ReqEditor.GetScrollBounds()
	totalH := float32(bounds.Max.Y)
	viewH := float32(gtx.Constraints.Max.Y)

	if totalH <= viewH || totalH == 0 {
		return layout.Dimensions{}
	}

	scrollY := float32(t.ReqEditor.GetScrollY())
	maxScroll := totalH - viewH
	if maxScroll <= 0 {
		maxScroll = 1
	}

	scrollFraction := scrollY / maxScroll
	if scrollFraction < 0 {
		scrollFraction = 0
	}
	if scrollFraction > 1 {
		scrollFraction = 1
	}

	thumbH := viewH * (viewH / totalH)
	if thumbH < 20 {
		thumbH = 20
	}

	thumbY := scrollFraction * (viewH - thumbH)
	trackWidth := float32(gtx.Dp(unit.Dp(10)))
	thumbWidth := float32(gtx.Dp(unit.Dp(6)))

	trackRect := image.Rect(
		gtx.Constraints.Max.X-int(trackWidth), 0,
		gtx.Constraints.Max.X, gtx.Constraints.Max.Y,
	)

	stack := clip.Rect(trackRect).Push(gtx.Ops)
	for {
		e, ok := t.ReqScrollDrag.Update(gtx.Metric, gtx.Source, gesture.Vertical)
		if !ok {
			break
		}
		switch e.Kind {
		case pointer.Press:
			t.ReqScrollDragY = e.Position.Y
		case pointer.Drag:
			delta := e.Position.Y - t.ReqScrollDragY
			t.ReqScrollDragY = e.Position.Y
			var contentDelta float32
			if viewH > thumbH {
				contentDelta = delta / (viewH - thumbH) * maxScroll
			}
			scrollY += contentDelta
			newScrollY := int(scrollY)
			if newScrollY < 0 {
				newScrollY = 0
			}
			t.ReqEditor.SetScrollY(newScrollY)
			win.Invalidate()
		}
	}
	pointer.CursorDefault.Add(gtx.Ops)
	t.ReqScrollDrag.Add(gtx.Ops)
	stack.Pop()

	fade := t.ReqEditor.ScrollbarFade()
	if fade <= 0 {
		return layout.Dimensions{}
	}
	col := theme.ScrollThumb
	col.A = uint8(float32(col.A) * fade)
	rect := image.Rect(
		gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
		int(thumbY),
		gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, col, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

	return layout.Dimensions{}
}

func formatSize(n int64) string {
	if n < 0 {
		n = 0
	}
	switch {
	case n >= 1<<30:
		return strconv.FormatFloat(float64(n)/float64(1<<30), 'f', 2, 64) + " GB"
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/float64(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatFloat(float64(n)/float64(1<<10), 'f', 1, 64) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}
