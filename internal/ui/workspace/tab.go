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
	"github.com/nanorele/gio/io/event"
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
}

type tabResponse struct {
	requestID     uint64
	status        string
	body          string
	respSize      int64
	respFile      string
	previewLoaded int64
	isJSON        bool
}

type previewResult struct {
	body          string
	previewLoaded int64
	isJSON        bool
}

type RequestTab struct {
	Title            string
	TabBtn           widget.Clickable
	CloseBtn         widget.Clickable
	Method           string
	MethodBtn        widget.Clickable
	MethodListOpen   bool
	MethodClickables []widget.Clickable
	URLInput         widget.Editor
	SendBtn          widget.Clickable
	Headers          []*HeaderItem
	HeadersExpanded  bool
	AddHeaderBtn     widget.Clickable
	ViewGeneratedBtn widget.Clickable
	HeadersList      widget.List
	ReqEditor        RequestEditor
	RespListH        widget.List
	WrapBtn          widget.Clickable
	WrapEnabled      bool
	CopyBtn          widget.Clickable
	Status           string
	RespEditor       *ResponseViewer
	SplitRatio       float32
	VStackRatio      float32
	LayoutMode       int
	LayoutHorizBtn   widget.Clickable
	LayoutVertBtn    widget.Clickable
	SplitDrag        gesture.Drag
	SplitDragX       float32
	ScrollDrag       gesture.Drag
	ScrollDragY      float32
	ReqScrollDrag    gesture.Drag
	ReqScrollDragY   float32
	HScrollDrag      gesture.Drag
	HScrollDragX     float32
	ReqHScrollDrag   gesture.Drag
	ReqHScrollDragX  float32

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
	Closed   atomic.Bool
	FileSaveMu      sync.Mutex
	isRequesting    bool
	cancelFn        context.CancelFunc
	respSize        int64
	respFile        string
	respIsJSON      bool
	downloadedBytes atomic.Int64
	previewLoaded   atomic.Int64

	CancelBtn      widget.Clickable
	SendMenuBtn    widget.Clickable
	SendMenuOpen   bool
	SaveToFileBtn  widget.Clickable
	SaveToFilePath string
	ShowPreviewBtn widget.Clickable
	PreviewEnabled bool
	LoadMoreBtn    widget.Clickable
	OpenFileBtn    widget.Clickable
	PropertiesBtn  widget.Clickable

	ReqWrapEnabled   bool
	jsonFmtState     *JSONFormatterState
	ReqWrapBtn       widget.Clickable
	ReqListH         widget.List
	HeaderSplitRatio float32
	HeaderSplitDrag  gesture.Drag
	HeaderSplitDragX float32

	SearchOpen       bool
	SearchEditor     widget.Editor
	SearchBtn        widget.Clickable
	SearchNextBtn    widget.Clickable
	SearchPrevBtn    widget.Clickable
	SearchCloseBtn   widget.Clickable
	searchQuery      string
	searchResults    []int
	searchCurrent    int
	searchCache      string
	searchCacheDirty bool

	URLSubmitted      bool
	FileSaveChan      chan io.WriteCloser
	dirtyCheckNeeded  bool
	visibleHeadersBuf []*HeaderItem

	appendChan       chan string
	window           *app.Window
	pendingRespWidth int
	pendingReqWidth  int
	reqWidthTimer    *time.Timer
	respWidthTimer   *time.Timer
	LastReqHeight    int
	LastRespHeight   int
	reqHeightTimer   *time.Timer
	respHeightTimer  *time.Timer

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
		Status:           "Ready",
		RespEditor:       NewResponseViewer(),
		MethodClickables: make([]widget.Clickable, len(methods)),
		responseChan:     make(chan tabResponse, 1),
		previewChan:      make(chan previewResult, 1),
		FileSaveChan:     make(chan io.WriteCloser, 1),
		appendChan:       make(chan string, 1024),
		SplitRatio:       splitRatio,
		VStackRatio:      0.5,
		WrapEnabled:      true,
		ReqWrapEnabled:   true,
		jsonFmtState:     &JSONFormatterState{},
		HeadersExpanded:  false,
		HeaderSplitRatio: 0.35,
		BodyType:         model.BodyRaw,
		formPartFileChan: make(chan formPartFileResult, 8),
		binaryFileChan:   make(chan binaryFileResult, 1),
	}
	t.URLInput.Submit = true
	t.HeadersList.Axis = layout.Vertical
	t.RespListH.Axis = layout.Horizontal
	t.ReqListH.Axis = layout.Horizontal
	t.SearchEditor.SingleLine = true
	t.SearchEditor.Submit = true
	return t
}

func (t *RequestTab) responseLang() syntax.Lang {
	if !settings.AutoFormatJSON {
		return syntax.LangPlain
	}
	if t.respIsJSON {
		return syntax.LangJSON
	}
	head := t.RespEditor.Bytes()
	if len(head) > 256 {
		head = head[:256]
	}
	return syntax.Detect("", head)
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
	body := t.ReqEditor.Bytes()
	head := body
	if len(head) > 256 {
		head = head[:256]
	}
	return syntax.Detect("", head)
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
			macro := op.Record(gtx.Ops)
			layout.Inset{Top: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widget.Border{
					Color:        theme.BorderLight,
					CornerRadius: unit.Dp(2),
					Width:        unit.Dp(1),
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Stack{}.Layout(gtx,
						layout.Expanded(func(gtx layout.Context) layout.Dimensions {
							rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
							paint.FillShape(gtx.Ops, theme.BgMenu, rect.Op(gtx.Ops))
							return layout.Dimensions{Size: gtx.Constraints.Min}
						}),
						layout.Stacked(func(gtx layout.Context) layout.Dimensions {
							rowW := gtx.Dp(unit.Dp(180))
							children := make([]layout.FlexChild, 0, len(bodyTypeChoices))
							for i, bt := range bodyTypeChoices {
								idx := i
								typ := bt
								children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.X = rowW
									gtx.Constraints.Max.X = rowW
									return material.Clickable(gtx, &t.BodyTypeChoices[idx], func(gtx layout.Context) layout.Dimensions {
										if t.BodyTypeChoices[idx].Hovered() {
											paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: image.Pt(rowW, gtx.Dp(unit.Dp(28)))}.Op())
										}
										return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											name := typ.String()
											if t.BodyType == typ {
												name = "✓ " + name
											} else {
												name = "  " + name
											}
											lbl := widgets.MonoLabel(th, unit.Sp(11), name)
											return lbl.Layout(gtx)
										})
									})
								}))
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
						}),
					)
				})
			})
			op.Defer(gtx.Ops, macro.Stop())
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &t.BodyTypeBtn, func(gtx layout.Context) layout.Dimensions {
				bg := theme.BgField
				if t.BodyTypeBtn.Hovered() {
					bg = theme.BgHover
				}
				macro := op.Record(gtx.Ops)
				dim := layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), "Type:")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
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
				rrFill := clip.UniformRRect(image.Rectangle{Max: dim.Size}, gtx.Dp(unit.Dp(4)))
				paint.FillShape(gtx.Ops, bg, rrFill.Op(gtx.Ops))
				widgets.PaintBorder1px(gtx, dim.Size, theme.Border)
				call.Add(gtx.Ops)
				return dim
			})
		}),
	)
}

func (t *RequestTab) layoutModeBarHeight(gtx layout.Context) int {
	return gtx.Dp(unit.Dp(26)) + gtx.Dp(unit.Dp(4))
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
		bg := theme.BgField
		if btn.Hovered() {
			bg = theme.BgHover
		}
		if active {
			bg = theme.AccentDim
		}
		paint.FillShape(gtx.Ops, bg, clip.Rect{Max: gtx.Constraints.Min}.Op())
		widgets.PaintBorder1px(gtx, gtx.Constraints.Min, theme.Border)
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			isz := gtx.Dp(unit.Dp(12))
			gtx.Constraints.Min = image.Pt(isz, isz)
			gtx.Constraints.Max = gtx.Constraints.Min
			paintLayoutSplitIcon(gtx, isz, theme.FgMuted, vertical)
			return layout.Dimensions{Size: image.Pt(isz, isz)}
		})
	})
}

func (t *RequestTab) layoutModeBar(gtx layout.Context, hBtn, vBtn *widget.Clickable, stacked bool) layout.Dimensions {
	barH := t.layoutModeBarHeight(gtx)
	gtx.Constraints.Min.Y = barH
	gtx.Constraints.Max.Y = barH
	size := image.Pt(gtx.Constraints.Max.X, barH)
	widgets.PaintBorder1px(gtx, size, theme.Border)
	return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(2), Right: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBtn(gtx, hBtn, false, !stacked)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.layoutModeBtn(gtx, vBtn, true, stacked)
			}),
		)
	})
}

func (t *RequestTab) headersRowMinWidth(gtx layout.Context, th *material.Theme) int {
	leftInset := gtx.Dp(unit.Dp(6))
	headerW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, "Headers")
	btnPad := gtx.Dp(unit.Dp(12))
	addW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, "Add") + btnPad
	showText := "Show Generated"
	if t.HeadersExpanded {
		showText = "Hide Generated"
	}
	showW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, showText) + btnPad
	gap := gtx.Dp(unit.Dp(4))
	safety := gtx.Dp(unit.Dp(12))
	return leftInset + headerW + gap + addW + gap + showW + safety
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
	leftInset := gtx.Dp(unit.Dp(6))
	requestW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(12), widgets.MonoFont, "Request")
	gapBetween := gtx.Dp(unit.Dp(8))

	selectorPad := gtx.Dp(unit.Dp(16))
	typeLbl := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(11), widgets.MonoFont, "Type:")
	typeNameW := widgets.MeasureTextWidthCached(gtx, th, unit.Sp(11), widgets.MonoFont, typeName)
	iconW := gtx.Dp(unit.Dp(12))
	innerGap := gtx.Dp(unit.Dp(4))
	selectorW := selectorPad + typeLbl + innerGap + typeNameW + innerGap + iconW

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
	if t.URLInput.Len() != len(req.URL) {
		t.IsDirty = true
		return
	}
	if t.ReqEditor.Len() != len(req.Body) {
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
	if t.URLInput.Text() != req.URL {
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
	t.IsDirty = false
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
	req.FormParts = req.FormParts[:0]
	for _, p := range t.FormParts {
		k := p.Key.Text()
		if k == "" {
			continue
		}
		fp := model.ParsedFormPart{Key: k, Value: p.Value.Text(), Kind: p.Kind, FilePath: p.FilePath}
		req.FormParts = append(req.FormParts, fp)
	}
	req.URLEncoded = req.URLEncoded[:0]
	for _, p := range t.URLEncoded {
		k := p.Key.Text()
		if k == "" {
			continue
		}
		req.URLEncoded = append(req.URLEncoded, model.ParsedKV{Key: k, Value: p.Value.Text()})
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
	t.searchCacheDirty = true
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

func (t *RequestTab) performSearch() {
	query := t.SearchEditor.Text()
	t.searchQuery = query
	t.searchResults = t.searchResults[:0]
	t.searchCurrent = -1
	if query == "" {
		return
	}
	if t.searchCacheDirty || t.searchCache == "" {
		t.searchCache = asciiToLower(t.RespEditor.Text())
		t.searchCacheDirty = false
	}
	q := asciiToLower(query)
	qLen := len(q)
	text := t.searchCache
	offset := 0
	for offset <= len(text)-qLen {
		idx := strings.Index(text[offset:], q)
		if idx < 0 {
			break
		}
		t.searchResults = append(t.searchResults, offset+idx)
		offset += idx + qLen
	}
}

func (t *RequestTab) searchNavigate(dir int) {
	if len(t.searchResults) == 0 {
		return
	}
	t.searchCurrent += dir
	if t.searchCurrent >= len(t.searchResults) {
		t.searchCurrent = 0
	}
	if t.searchCurrent < 0 {
		t.searchCurrent = len(t.searchResults) - 1
	}
	pos := t.searchResults[t.searchCurrent]
	t.RespEditor.SetCaret(pos, pos+len(t.searchQuery))
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
		// Content-Type with boundary set by buildBody at send time; don't show stale auto-header here.
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

func (t *RequestTab) Layout(gtx layout.Context, th *material.Theme, win *app.Window, exp *explorer.Explorer, activeEnv map[string]string, isAppDragging bool, onSave func(), onCollectionDirty func(*collections.ParsedCollection)) layout.Dimensions {
	t.window = win

	select {
	case chunk := <-t.appendChan:
		var buf strings.Builder
		buf.WriteString(chunk)
	drainLoop:
		for {
			select {
			case more := <-t.appendChan:
				buf.WriteString(more)
			default:
				break drainLoop
			}
		}
		appended := buf.String()
		t.RespEditor.Append(appended)
		t.invalidateSearchCache()
	default:
	}

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

	if t.ReqEditor.Changed() {
		t.UpdateSystemHeaders()
		t.dirtyCheckNeeded = true
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
			t.isRequesting = false
			t.cancelFn = nil
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
		t.previewLoaded.Store(pr.previewLoaded)
		t.respIsJSON = pr.isJSON
		t.RespEditor.SetText(pr.body)
		t.invalidateSearchCache()
		th.Shaper.ResetLayoutCache()
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
		t.SearchOpen = !t.SearchOpen
	}
	for t.SearchCloseBtn.Clicked(gtx) {
		t.SearchOpen = false
		t.searchResults = nil
	}
	for t.SearchNextBtn.Clicked(gtx) {
		t.searchNavigate(1)
	}
	for t.SearchPrevBtn.Clicked(gtx) {
		t.searchNavigate(-1)
	}
	for {
		ev, ok := t.SearchEditor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			t.performSearch()
			t.searchNavigate(1)
		}
	}
	if t.SearchOpen && t.SearchEditor.Text() != t.searchQuery {
		t.performSearch()
	}

	for t.MethodBtn.Clicked(gtx) {
		t.MethodListOpen = !t.MethodListOpen
	}
	for i := range t.MethodClickables {
		for t.MethodClickables[i].Clicked(gtx) {
			t.Method = methods[i]
			t.MethodListOpen = false
			t.dirtyCheckNeeded = true
		}
	}

	for t.AddHeaderBtn.Clicked(gtx) {
		t.AddHeader("", "")
		t.dirtyCheckNeeded = true
	}

	for t.ViewGeneratedBtn.Clicked(gtx) {
		t.HeadersExpanded = !t.HeadersExpanded
	}

	for i := 0; i < len(t.Headers); i++ {
		if t.Headers[i].DelBtn.Clicked(gtx) {
			t.Headers = append(t.Headers[:i], t.Headers[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
		}
	}

	if t.CopyBtn.Clicked(gtx) {
		var reader io.ReadCloser
		if t.respFile != "" {
			if fi, err := os.Stat(t.respFile); err == nil && fi.Size() > 0 {
				if f, ferr := os.Open(t.respFile); ferr == nil {
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
		if !h.IsGenerated || t.HeadersExpanded {
			visibleHeaders = append(visibleHeaders, h)
		}
	}
	t.visibleHeadersBuf = visibleHeaders

	for t.LayoutHorizBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeHoriz {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeHoriz
		}
		win.Invalidate()
	}
	for t.LayoutVertBtn.Clicked(gtx) {
		if t.LayoutMode == LayoutModeVert {
			t.LayoutMode = LayoutModeAuto
		} else {
			t.LayoutMode = LayoutModeVert
		}
		win.Invalidate()
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
	} else {
		ratio = &t.SplitRatio
		flexExtent = float32(gtx.Constraints.Max.X - gtx.Dp(unit.Dp(8)))
		dragAxis = gesture.Horizontal
		reqMinDp = float32(defaultMin)
		respMinDp = float32(gtx.Dp(unit.Dp(200)))
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
			t.SplitDragX = pos
			t.IsDraggingSplit = true
		case pointer.Drag:
			finalX = pos
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
		oldRatio := *ratio
		*ratio += delta / flexExtent
		if *ratio < minReqRatio {
			*ratio = minReqRatio
		} else if *ratio > maxReqRatio {
			*ratio = maxReqRatio
		}
		t.SplitDragX = finalX - ((*ratio - oldRatio) * flexExtent)
		win.Invalidate()
	}
	if released {
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
								if !t.MethodListOpen {
									return layout.Dimensions{}
								}
								macro := op.Record(gtx.Ops)
								dropGtx := gtx
								dropGtx.Constraints.Min = image.Point{}
								dropGtx.Constraints.Max.Y = 1 << 24
								layout.Inset{Top: unit.Dp(32)}.Layout(dropGtx, func(gtx layout.Context) layout.Dimensions {
									return widget.Border{
										Color:        theme.BorderLight,
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										return layout.Stack{}.Layout(gtx,
											layout.Expanded(func(gtx layout.Context) layout.Dimensions {
												rect := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2)
												paint.FillShape(gtx.Ops, theme.BgMenu, rect.Op(gtx.Ops))
												return layout.Dimensions{Size: gtx.Constraints.Min}
											}),
											layout.Stacked(func(gtx layout.Context) layout.Dimensions {
												rowW := gtx.Dp(unit.Dp(96))
												children := make([]layout.FlexChild, 0, len(methods))
												for i, m := range methods {
													idx := i
													methodName := m
													children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
														gtx.Constraints.Min.X = rowW
														gtx.Constraints.Max.X = rowW
														return material.Clickable(gtx, &t.MethodClickables[idx], func(gtx layout.Context) layout.Dimensions {
															if t.MethodClickables[idx].Hovered() {
																paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: image.Pt(rowW, gtx.Dp(unit.Dp(34)))}.Op())
															}
															return layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(12), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																lbl := widgets.MonoLabel(th, unit.Sp(12), methodName)
																lbl.Color = theme.MethodColor(methodName)
																return lbl.Layout(gtx)
															})
														})
													}))
												}
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
											}),
										)
									})
								})
								op.Defer(gtx.Ops, macro.Stop())
								return layout.Dimensions{}
							}),
							layout.Stacked(func(gtx layout.Context) layout.Dimensions {
								return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									gtx.Constraints.Min.Y = 0
									return widgets.Bordered1px(gtx, unit.Dp(4), theme.Border, func(gtx layout.Context) layout.Dimensions {
										btn := widgets.MonoButton(th, &t.MethodBtn, t.Method)
										btn.Background = theme.BgSecondary
										btn.Color = theme.MethodColor(t.Method)
										btn.TextSize = unit.Sp(12)
										btn.Inset = layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(7), Left: unit.Dp(8), Right: unit.Dp(8)}
										return btn.Layout(gtx)
									})
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
						return widgets.TextFieldOverlay(gtx, th, &t.URLInput, "https://api.example.com", true, activeEnv, frozenURLWidth, unit.Sp(12))
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if t.LinkedNode == nil {
							return layout.Dimensions{}
						}
						iconColor := theme.FgDisabled
						if t.IsDirty {
							iconColor = th.ContrastBg
						}
						gtx.Constraints.Min = image.Point{X: btnH, Y: btnH}
						gtx.Constraints.Max = gtx.Constraints.Min
						return t.SaveToColBtn.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(2)))
							paint.FillShape(gtx.Ops, theme.BgField, rr.Op(gtx.Ops))
							widgets.PaintBorder1px(gtx, gtx.Constraints.Min, theme.Border)
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								s := gtx.Dp(unit.Dp(18))
								gtx.Constraints.Min = image.Point{X: s, Y: s}
								return widgets.IconSave.Layout(gtx, iconColor)
							})
						})
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = btnH
						gtx.Constraints.Max.Y = btnH
						btnMinW := gtx.Dp(unit.Dp(90))
						if t.isRequesting {
							gtx.Constraints.Min.X = btnMinW
							return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								btn := widgets.MonoButton(th, &t.CancelBtn, "CANCEL")
								btn.Background = theme.Cancel
								btn.Color = theme.DangerFg
								btn.TextSize = unit.Sp(12)
								btn.Inset = layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(16)}
								return btn.Layout(gtx)
							})
						}

						bgColor := theme.VarFound
						cornerR := gtx.Dp(unit.Dp(4))
						gtx.Constraints.Min.X = btnMinW

						sendMacro := op.Record(gtx.Ops)
						sendDims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SendBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(7), Bottom: unit.Dp(6), Left: unit.Dp(16), Right: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										lbl := material.Label(th, unit.Sp(12), "SEND")
										lbl.Color = th.Fg
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
									return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(0), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										is := gtx.Dp(unit.Dp(20))
										gtx.Constraints.Min = image.Point{X: is, Y: is}
										gtx.Constraints.Max = gtx.Constraints.Min
										return widgets.IconDropDown.Layout(gtx, th.Fg)
									})
								})
							}),
						)
						sendCall := sendMacro.Stop()

						sz := sendDims.Size
						paint.FillShape(gtx.Ops, bgColor, clip.UniformRRect(image.Rectangle{Max: sz}, cornerR).Op(gtx.Ops))
						sendCall.Add(gtx.Ops)

						if t.SendMenuOpen {
							macro := op.Record(gtx.Ops)
							menuGtx := gtx
							menuGtx.Constraints.Min = image.Point{}
							menuGtx.Constraints.Max = image.Pt(gtx.Dp(unit.Dp(160)), gtx.Dp(unit.Dp(100)))

							rec := op.Record(gtx.Ops)
							menuDims := layout.UniformInset(unit.Dp(4)).Layout(menuGtx, func(gtx layout.Context) layout.Dimensions {
								return material.Clickable(gtx, &t.SaveToFileBtn, func(gtx layout.Context) layout.Dimensions {
									return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										gtx.Constraints.Min.X = gtx.Dp(unit.Dp(130))
										lbl := widgets.MonoLabel(th, unit.Sp(12), "Save to file...")
										return lbl.Layout(gtx)
									})
								})
							})
							menuCall := rec.Stop()

							msz := menuDims.Size
							menuX := sz.X - msz.X
							op.Offset(image.Pt(menuX, sz.Y+gtx.Dp(unit.Dp(2)))).Add(gtx.Ops)

							paint.FillShape(gtx.Ops, theme.BgPopup, clip.UniformRRect(image.Rectangle{Max: msz}, 4).Op(gtx.Ops))
							b := max(1, gtx.Dp(unit.Dp(1)))
							paint.FillShape(gtx.Ops, theme.BorderLight, clip.Stroke{Path: clip.UniformRRect(image.Rectangle{Max: msz}, 4).Path(gtx.Ops), Width: float32(b)}.Op())
							menuCall.Add(gtx.Ops)

							call := macro.Stop()
							op.Defer(gtx.Ops, call)
						}

						return sendDims
					}),
				)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			flexAxis := layout.Horizontal
			leftInset := layout.Inset{Right: unit.Dp(1)}
			rightInset := layout.Inset{Left: unit.Dp(1)}
			if stacked {
				flexAxis = layout.Vertical
				leftInset = layout.Inset{Bottom: unit.Dp(1)}
				rightInset = layout.Inset{Top: unit.Dp(1)}
			}
			return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(1), Left: unit.Dp(4), Right: unit.Dp(4)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return t.layoutModeBar(gtx, &t.LayoutHorizBtn, &t.LayoutVertBtn, stacked)
					}),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: flexAxis}.Layout(gtx,
							layout.Flexed(*ratio, func(gtx layout.Context) layout.Dimensions {
								return leftInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return widget.Border{
										Color:        theme.Border,
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																lbl := widgets.MonoLabel(th, unit.Sp(12), "Headers")
																lbl.Font.Weight = font.Bold
																return lbl.Layout(gtx)
															})
														}),
														layout.Flexed(1, layout.Spacer{Width: unit.Dp(1)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
															return widgets.Bordered1px(gtx, unit.Dp(4), theme.Border, func(gtx layout.Context) layout.Dimensions {
																btn := widgets.MonoButton(th, &t.AddHeaderBtn, "Add")
																btn.TextSize = unit.Sp(12)
																btn.Background = theme.BgField
																btn.Color = th.Fg
																btn.Inset = layout.UniformInset(unit.Dp(6))
																return btn.Layout(gtx)
															})
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
															btnText := "Show Generated"
															if t.HeadersExpanded {
																btnText = "Hide Generated"
															}
															return widgets.Bordered1px(gtx, unit.Dp(4), theme.Border, func(gtx layout.Context) layout.Dimensions {
																btn := widgets.MonoButton(th, &t.ViewGeneratedBtn, btnText)
																btn.TextSize = unit.Sp(12)
																btn.Background = theme.BgField
																btn.Color = th.Fg
																btn.Inset = layout.UniformInset(unit.Dp(6))
																return btn.Layout(gtx)
															})
														}),
													)
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
												paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
												return layout.Dimensions{Size: size}
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if len(visibleHeaders) == 0 {
													return layout.Dimensions{}
												}
												return layout.UniformInset(unit.Dp(4)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return t.HeadersList.Layout(gtx, len(visibleHeaders), func(gtx layout.Context, i int) layout.Dimensions {
														h := visibleHeaders[i]
														return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																return layout.Inset{Top: unit.Dp(1), Bottom: unit.Dp(0), Left: unit.Dp(1), Right: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																	return kvRow(gtx, th, &h.Key, &h.Value, &h.DelBtn, t.HeaderSplitRatio, activeEnv)
																})
															}),
															layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																if i >= len(visibleHeaders)-1 {
																	return layout.Dimensions{}
																}
																size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
																paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
																return layout.Dimensions{Size: size}
															}),
														)
													})
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if len(visibleHeaders) == 0 {
													return layout.Dimensions{}
												}
												size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
												paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
												return layout.Dimensions{Size: size}
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																lbl := widgets.MonoLabel(th, unit.Sp(12), "Request")
																lbl.Font.Weight = font.Bold
																return lbl.Layout(gtx)
															})
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															return t.layoutBodyTypeSelector(gtx, th)
														}),
														layout.Flexed(1, layout.Spacer{}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															if t.BodyType != model.BodyRaw {
																return layout.Dimensions{}
															}
															return widgets.SquareBtnSlim(gtx, &t.ReqWrapBtn, iconWrap, th)
														}),
													)
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
												paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
												return layout.Dimensions{Size: size}
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												bdr := gtx.Dp(unit.Dp(1))
												sz := gtx.Constraints.Max
												paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: sz}.Op())
												inner := image.Rect(bdr, bdr, sz.X-bdr, sz.Y-bdr)
												bodyBg := theme.Bg
												if t.BodyType == model.BodyRaw {
													bodyBg = theme.BgField
												}
												paint.FillShape(gtx.Ops, bodyBg, clip.Rect(inner).Op())
												gtx.Constraints.Min = image.Pt(inner.Dx(), inner.Dy())
												gtx.Constraints.Max = gtx.Constraints.Min
												op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
												drawRaw := func(gtx layout.Context) layout.Dimensions {
													return layout.Stack{}.Layout(gtx,
														layout.Expanded(func(gtx layout.Context) layout.Dimensions {
															style := RequestEditorStyle{
																Viewer:         &t.ReqEditor,
																Shaper:         th.Shaper,
																Font:           widgets.MonoFont,
																TextSize:       settings.BodyTextSize,
																Color:          theme.Fg,
																HighlightColor: theme.AccentDim,
																SelectionColor: theme.Selection,
																Wrap:           t.ReqWrapEnabled,
																Padding:        settings.RespBodyPad,
																Env:            activeEnv,
																Lang:           t.requestLang(),
																Syntax:         theme.Syntax,
																BracketCycle:   settings.BracketColorization,
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
													)
												}
												return t.layoutBody(gtx, th, win, exp, activeEnv, drawRaw)
											}),
										)
									})
								})
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
								event.Op(gtx.Ops, &t.SplitDrag)
								for {
									_, ok := gtx.Event(pointer.Filter{Target: &t.SplitDrag, Kinds: pointer.Move | pointer.Enter | pointer.Leave})
									if !ok {
										break
									}
								}
								return layout.Dimensions{Size: size}
							}),
							layout.Flexed(1-*ratio, func(gtx layout.Context) layout.Dimensions {
								return rightInset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return widget.Border{
										Color:        theme.Border,
										CornerRadius: unit.Dp(2),
										Width:        unit.Dp(1),
									}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
										paint.FillShape(gtx.Ops, theme.Bg, clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, 2).Op(gtx.Ops))
										return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
														layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
																statusText := t.Status
																if t.isRequesting {
																	dl := t.downloadedBytes.Load()
																	if dl > 0 {
																		statusText = "Downloading... " + formatSize(dl)
																	}
																}
																lbl := widgets.MonoLabel(th, unit.Sp(12), statusText)
																return lbl.Layout(gtx)
															})
														}),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
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
																	defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
																	return widgets.SquareBtn(gtx, &t.SearchBtn, widgets.IconSearch, th)
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
																	return widgets.SquareBtn(gtx, &t.WrapBtn, iconWrap, th)
																}),
																layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
																layout.Rigid(func(gtx layout.Context) layout.Dimensions {
																	defer op.Offset(image.Pt(-1, 1)).Push(gtx.Ops).Pop()
																	return widgets.SquareBtn(gtx, &t.CopyBtn, iconCopy, th)
																}),
															)
														}),
													)
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												if !t.SearchOpen {
													return layout.Dimensions{}
												}
												return layout.Inset{Left: unit.Dp(4), Right: unit.Dp(4), Top: unit.Dp(2), Bottom: unit.Dp(2)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
													return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
														layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
															return widgets.TextField(gtx, th, &t.SearchEditor, "Search...", true, nil, 0, unit.Sp(11))
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															cur := 0
															if len(t.searchResults) > 0 {
																cur = t.searchCurrent + 1
															}
															lbl := widgets.MonoLabel(th, unit.Sp(10), strconv.Itoa(cur)+"/"+strconv.Itoa(len(t.searchResults)))
															lbl.Color = theme.FgDim
															return lbl.Layout(gtx)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															btn := widgets.MonoButton(th, &t.SearchPrevBtn, "▲")
															btn.TextSize = unit.Sp(8)
															btn.Background = theme.BgSecondary
															btn.Inset = layout.UniformInset(unit.Dp(4))
															return btn.Layout(gtx)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															btn := widgets.MonoButton(th, &t.SearchNextBtn, "▼")
															btn.TextSize = unit.Sp(8)
															btn.Background = theme.BgSecondary
															btn.Inset = layout.UniformInset(unit.Dp(4))
															return btn.Layout(gtx)
														}),
														layout.Rigid(layout.Spacer{Width: unit.Dp(2)}.Layout),
														layout.Rigid(func(gtx layout.Context) layout.Dimensions {
															btn := widgets.MonoButton(th, &t.SearchCloseBtn, "✕")
															btn.TextSize = unit.Sp(8)
															btn.Background = theme.BgSecondary
															btn.Inset = layout.UniformInset(unit.Dp(4))
															return btn.Layout(gtx)
														}),
													)
												})
											}),
											layout.Rigid(func(gtx layout.Context) layout.Dimensions {
												size := image.Point{X: gtx.Constraints.Max.X, Y: gtx.Dp(unit.Dp(1))}
												paint.FillShape(gtx.Ops, theme.Border, clip.Rect{Max: size}.Op())
												return layout.Dimensions{Size: size}
											}),
											layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
												return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
													layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
														return t.layoutResponseBody(gtx, th, win, isDragging)
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
									})
								})
							}),
						)
					}),
				)
			})
		}),
	)
}

func (t *RequestTab) layoutResponseBody(gtx layout.Context, th *material.Theme, win *app.Window, isDragging bool) layout.Dimensions {
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
	rInner := image.Rect(bdr, bdr, rsz.X-bdr, rsz.Y-bdr)
	paint.FillShape(gtx.Ops, theme.BgField, clip.Rect(rInner).Op())
	op.Offset(image.Pt(bdr, bdr)).Add(gtx.Ops)
	gtx.Constraints.Min = image.Pt(rInner.Dx(), rInner.Dy())
	gtx.Constraints.Max = gtx.Constraints.Min

	return layout.Stack{}.Layout(gtx,
		layout.Expanded(func(gtx layout.Context) layout.Dimensions {
			lang := t.responseLang()
			vs := ResponseViewerStyle{
				Viewer:         t.RespEditor,
				Shaper:         th.Shaper,
				Font:           widgets.MonoFont,
				TextSize:       settings.BodyTextSize,
				Color:          theme.Fg,
				HighlightColor: theme.AccentDim,
				SelectionColor: theme.Selection,
				Wrap:           t.WrapEnabled,
				Padding:        settings.RespBodyPad,
				Lang:           lang,
				Syntax:         theme.Syntax,
				BracketCycle:   settings.BracketColorization,
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

			rect := image.Rect(
				gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
				int(thumbY),
				gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
				int(thumbY+thumbH),
			)
			paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if t.WrapEnabled {
				return layout.Dimensions{}
			}
			return t.layoutRespHScrollbar(gtx, win)
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
					btn := material.Button(th, &t.LoadFromFileBtn, "Load from file…")
					btn.Background = theme.Accent
					btn.Color = theme.AccentFg
					btn.TextSize = unit.Sp(11)
					btn.Inset = layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(8), Right: unit.Dp(8)}
					return btn.Layout(gtx)
				}),
				layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					btn := material.Button(th, &t.DismissOversizeBtn, "Dismiss")
					btn.Background = theme.Border
					btn.Color = th.Fg
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
	return layoutHScrollbar(gtx, win, t.ReqEditor.GetMaxLineWidth(), t.ReqEditor.GetScrollX(), &t.ReqHScrollDrag, &t.ReqHScrollDragX, func(x int) {
		t.ReqEditor.SetScrollX(x)
	})
}

func (t *RequestTab) layoutRespHScrollbar(gtx layout.Context, win *app.Window) layout.Dimensions {
	return layoutHScrollbar(gtx, win, t.RespEditor.GetMaxLineWidth(), t.RespEditor.GetScrollX(), &t.HScrollDrag, &t.HScrollDragX, func(x int) {
		t.RespEditor.SetScrollX(x)
	})
}

func layoutHScrollbar(gtx layout.Context, win *app.Window, totalW int, currentX int, drag *gesture.Drag, dragOriginX *float32, setX func(int)) layout.Dimensions {
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

	rect := image.Rect(
		int(thumbX),
		gtx.Constraints.Max.Y-int(thumbHeight)-gtx.Dp(unit.Dp(2)),
		int(thumbX+thumbW),
		gtx.Constraints.Max.Y-gtx.Dp(unit.Dp(2)),
	)
	paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

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

	rect := image.Rect(
		gtx.Constraints.Max.X-int(thumbWidth)-gtx.Dp(unit.Dp(2)),
		int(thumbY),
		gtx.Constraints.Max.X-gtx.Dp(unit.Dp(2)),
		int(thumbY+thumbH),
	)
	paint.FillShape(gtx.Ops, theme.ScrollThumb, clip.UniformRRect(rect, gtx.Dp(unit.Dp(3))).Op(gtx.Ops))

	return layout.Dimensions{}
}

func formatSize(n int64) string {
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
