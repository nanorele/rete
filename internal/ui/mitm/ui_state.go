package mitm

import (
	"os"
	"time"

	"tracto/internal/persist"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/widget"
)

// View identifiers for the central area (zone C).
const (
	ViewIntercept  = "intercept"
	ViewHistory    = "history"
	ViewWebSockets = "websockets"
)

type UIState struct {
	Store *Store
	Proxy *Proxy

	host *Host

	AutoStart     bool
	AutoInstallCA bool
	AutoRemoveCA  bool

	// ---- runtime / sub-header ----
	StartBtn   widget.Clickable
	ClearBtn   widget.Clickable
	View       string
	SegHistory widget.Clickable
	SegInterc  widget.Clickable
	SegWS      widget.Clickable
	Filter     widget.Editor
	FilterClr  widget.Clickable

	// clear confirmation modal
	ClearConfirmOpen bool
	ClearYesBtn      widget.Clickable
	ClearNoBtn       widget.Clickable

	// ---- history (zone C) ----
	List         widget.List
	RowClicks    []*widget.Clickable
	RowMore      []*widget.Clickable
	Selected     uint64
	StatusBanner string
	SortColumn   string
	SortAsc      bool
	SortClicks   [8]widget.Clickable
	HideNoiseSw  widget.Bool
	// row context menu
	CtxOpen      bool
	CtxFlowID    uint64
	CtxPos       struct{ X, Y int }
	LocalPtr     struct{ X, Y int }
	PtrTag       int
	OverlayCatch int
	CtxCopyURL, CtxCopyCurl, CtxCopyReq, CtxRepeat, CtxAddScope,
	CtxDelete, CtxAnnotate, CtxToRepeater widget.Clickable
	// annotate popup
	AnnotateOpen    bool
	AnnotateFlowID  uint64
	AnnotateComment widget.Editor
	AnnotateColors  [6]widget.Clickable
	AnnotateSave    widget.Clickable

	// ---- inspector (zone D) ----
	SplitRatio float32
	SplitDrag  gesture.Drag
	SplitDragX float32
	SplitPx    float32
	LeftDrawn  int

	InspectorCollapsed bool
	InspectorToggle    widget.Clickable

	TabReq  widget.Clickable
	TabResp widget.Clickable
	ActTab  int // 0=request 1=response

	ViewRaw, ViewPretty, ViewHex, ViewRender widget.Clickable
	RenderMode                               int // 0=raw 1=pretty 2=hex 3=render

	SecHeaders, SecBody, SecParams, SecCookies widget.Clickable
	SecTab                                     int // 0=headers 1=body 2=params 3=cookies

	ReqHeadersList                                                                  widget.List
	RespHeadersList                                                                 widget.List
	BodyList                                                                        widget.List
	InspSendRepeater, InspSendIntruder, InspSendComparer, InspSendDecoder, InspCopy widget.Clickable

	// ---- websockets (zone C) ----
	WSList     widget.List
	WSRowClk   []*widget.Clickable
	WSSelected uint64

	// ---- intercept (zone C) ----
	InterceptSwitch widget.Bool
	InterceptRespSw widget.Bool
	HeldEditor      widget.Editor
	HeldEditorFor   uint64
	ForwardBtn      widget.Clickable
	DropBtn         widget.Clickable
	ActionBtn       widget.Clickable

	BindAddr widget.Editor

	// ---- TLS / CA (zone B) ----
	GenCABtn        widget.Clickable
	InstallCABtn    widget.Clickable
	RemoveCABtn     widget.Clickable
	ExportPEMBtn    widget.Clickable
	ExportDERBtn    widget.Clickable
	DecryptSwitch   widget.Bool
	HelpBtn         widget.Clickable
	RevealBtn       widget.Clickable
	CopyPathBtn     widget.Clickable
	CABanner        string
	HelpOpen        bool
	caLoadAttempted bool
	TrustNotifySet  bool
	NotifySet       bool

	// ---- accordion section expand state (zone B) ----
	SecTargetsOpen bool
	SecTLSOpen     bool
	SecIRulesOpen  bool
	SecMROpen      bool
	SecScopeOpen   bool
	SecTargetsHdr  widget.Clickable
	SecTLSHdr      widget.Clickable
	SecIRulesHdr   widget.Clickable
	SecMRHdr       widget.Clickable
	SecScopeHdr    widget.Clickable
	SidebarList    widget.List

	// ---- Targets section (zone B) ----
	TargetInput  widget.Editor
	TargetAddBtn widget.Clickable
	TargetBanner string
	TargetRows   map[string]*TargetRow

	// ---- Intercept rules section (zone B) ----
	IRulesReqTab   widget.Clickable
	IRulesRespTab  widget.Clickable
	IRulesActive   string // HeldRequest | HeldResponse
	IRuleFieldBtn  widget.Clickable
	IRuleFieldSel  int
	IRuleValInput  widget.Editor
	IRuleOrBtn     widget.Clickable
	IRuleOr        bool
	IRuleAddBtn    widget.Clickable
	IRuleEnableSw  widget.Bool
	IRulePresetImg widget.Clickable
	IRuleRows      []*CondRow

	// ---- Match & Replace section (zone B) ----
	MRTypeBtn   widget.Clickable
	MRTypeSel   int
	MRAreaBtn   widget.Clickable
	MRAreaSel   int
	MRPatInput  widget.Editor
	MRReplInput widget.Editor
	MRCommInput widget.Editor
	MRRegexSw   widget.Bool
	MRAddBtn    widget.Clickable
	MRPresetCSP widget.Clickable
	MRRows      []*MRRow

	// ---- Scope section (zone B) ----
	ScopeKindBtn  widget.Clickable
	ScopeKindSel  int
	ScopeFieldBtn widget.Clickable
	ScopeFieldSel int
	ScopePatInput widget.Editor
	ScopeAddBtn   widget.Clickable
	ScopeRows     []*ScopeRow

	// ---- persistence ----
	Config       Config
	configLoaded bool
	dirty        bool
	saveAt       time.Time
	savePending  bool
}

type TargetRow struct {
	Edit, Remove, Copy, Expand   widget.Clickable
	Expanded                     bool
	UpstreamAuto, UpstreamManual widget.Clickable
	TLSDecrypt, TLSTunnel        widget.Clickable
	AddrInput, DelayInput        widget.Editor
	DoH                          widget.Bool
}

type CondRow struct {
	Enable widget.Bool
	Remove widget.Clickable
	Op     widget.Clickable
}

type MRRow struct {
	Enable widget.Bool
	Remove widget.Clickable
	Up     widget.Clickable
	Down   widget.Clickable
}

type ScopeRow struct {
	Enable widget.Bool
	Remove widget.Clickable
}

func (s *UIState) Ensure() {
	if s.Store == nil {
		s.Store = NewStore()
	}
	if s.Proxy == nil {
		s.Proxy = NewProxy(s.Store)
	}
	if s.SplitRatio <= 0 {
		s.SplitRatio = 0.62
	}
	if s.View == "" {
		s.View = ViewHistory
	}
	if s.IRulesActive == "" {
		s.IRulesActive = HeldRequest
	}
	if s.SortColumn == "" {
		s.SortColumn = "#"
		s.SortAsc = true
	}
	if s.BindAddr.Text() == "" {
		s.BindAddr.SetText(DefaultAddr)
	}
	s.BindAddr.SingleLine = true
	s.Filter.SingleLine = true
	s.TargetInput.SingleLine = true
	s.IRuleValInput.SingleLine = true
	s.MRPatInput.SingleLine = true
	s.MRReplInput.SingleLine = true
	s.MRCommInput.SingleLine = true
	s.ScopePatInput.SingleLine = true
	if s.TargetRows == nil {
		s.TargetRows = make(map[string]*TargetRow)
	}
	if s.Proxy != nil && s.Proxy.Rules == nil {
		s.Proxy.Rules = NewRules()
	}

	if !s.configLoaded {
		s.configLoaded = true
		s.Config = LoadConfig()
		s.applyLoadedConfig()
		// default: expand the two primary sections on first paint
		s.SecTargetsOpen = true
		s.SecTLSOpen = true
	}

	if !s.caLoadAttempted {
		s.caLoadAttempted = true
		dir := persist.MITMDir()
		if _, err := os.Stat(CACertPath(dir)); err == nil {
			if ca, err := LoadCA(dir); err == nil {
				s.Proxy.SetCA(ca)
			}
		}
		// Restore the decrypt toggle now that a CA may be loaded.
		if s.DecryptSwitch.Value && s.Proxy.CA() != nil {
			s.Proxy.SetIntercept(true)
		} else {
			s.DecryptSwitch.Value = s.Proxy.Intercepting()
		}
	}
}

func (s *UIState) applyLoadedConfig() {
	c := s.Config
	if c.BindAddr != "" {
		s.BindAddr.SetText(c.BindAddr)
	}
	if c.View != "" {
		s.View = c.View
	}
	if c.InspectorWidthPx > 0 {
		// stored as ratio elsewhere; width handled by shell
	}
	s.InspectorCollapsed = c.InspectorCollapsed
	if c.SortColumn != "" {
		s.SortColumn = c.SortColumn
		s.SortAsc = c.SortAsc
	}
	c.ApplyTo(s.Proxy)
	if c.Decrypt {
		// applied by shell once a CA is present
		s.DecryptSwitch.Value = true
	}
	s.InterceptRespSw.Value = c.InterceptResponses
	s.Proxy.Manual.SetInterceptResponses(c.InterceptResponses)
}

// MarkDirty flags the config for a debounced save.
func (s *UIState) MarkDirty() { s.dirty = true }

// Dirty reports and clears the dirty flag.
func (s *UIState) Dirty() bool {
	d := s.dirty
	s.dirty = false
	return d
}

// SnapshotConfig captures the current proxy + UI state into the config.
func (s *UIState) SnapshotConfig() Config {
	c := s.Config
	c.BindAddr = s.BindAddr.Text()
	c.View = s.View
	c.Decrypt = s.DecryptSwitch.Value
	c.InspectorCollapsed = s.InspectorCollapsed
	c.SortColumn = s.SortColumn
	c.SortAsc = s.SortAsc
	c.InterceptResponses = s.InterceptRespSw.Value
	c.CaptureFrom(s.Proxy)
	s.Config = c
	return c
}

func MITMDir() string { return persist.MITMDir() }
