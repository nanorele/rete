package mitm

import (
	"os"

	"tracto/internal/persist"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/widget"
)

type UIState struct {
	Store *Store
	Proxy *Proxy

	StartBtn widget.Clickable
	StopBtn  widget.Clickable
	ClearBtn widget.Clickable

	List         widget.List
	RowClicks    []*widget.Clickable
	Selected     uint64
	StatusBanner string

	HeadersList widget.List

	SplitRatio float32
	SplitDrag  gesture.Drag
	SplitDragX float32

	TabReq  widget.Clickable
	TabResp widget.Clickable
	ActTab  int

	ReqHeadersList  widget.List
	RespHeadersList widget.List

	BindAddr widget.Editor

	GenCABtn        widget.Clickable
	InstallCABtn    widget.Clickable
	RemoveCABtn     widget.Clickable
	InterceptBtn    widget.Clickable
	HelpBtn         widget.Clickable
	RevealBtn       widget.Clickable
	CopyPathBtn     widget.Clickable
	CABanner        string
	HelpOpen        bool
	caLoadAttempted bool

	RulesOpen        bool
	RulesBtn         widget.Clickable
	RulesList        widget.List
	RuleHostInput    widget.Editor
	RuleHostBox      widget.Clickable
	RuleTimeoutInput widget.Editor
	RuleTimeoutBox   widget.Clickable
	RuleDoHCheck     widget.Bool
	RuleAddBtn       widget.Clickable
	RuleRowRemove    map[string]*widget.Clickable
	RuleBanner       string
}

func (s *UIState) Ensure() {
	if s.Store == nil {
		s.Store = NewStore()
	}
	if s.Proxy == nil {
		s.Proxy = NewProxy(s.Store)
	}
	if s.SplitRatio <= 0 {
		s.SplitRatio = 0.45
	}
	if s.BindAddr.Text() == "" {
		s.BindAddr.SetText(DefaultAddr)
	}
	s.BindAddr.SingleLine = true
	s.RuleHostInput.SingleLine = true
	s.RuleTimeoutInput.SingleLine = true
	if s.RuleRowRemove == nil {
		s.RuleRowRemove = make(map[string]*widget.Clickable)
	}
	if s.Proxy != nil && s.Proxy.Rules == nil {
		s.Proxy.Rules = NewRules()
	}

	if !s.caLoadAttempted {
		s.caLoadAttempted = true
		dir := persist.MITMDir()
		if _, err := os.Stat(CACertPath(dir)); err == nil {
			if ca, err := LoadCA(dir); err == nil {
				s.Proxy.SetCA(ca)
			}
		}
	}
}

func MITMDir() string { return persist.MITMDir() }
