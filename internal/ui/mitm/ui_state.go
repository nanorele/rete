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

	// CA management
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

// MITMDir is exposed so UI code can pass a deterministic directory to
// CA save/load helpers without re-importing persist itself.
func MITMDir() string { return persist.MITMDir() }
