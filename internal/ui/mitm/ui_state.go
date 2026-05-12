package mitm

import (
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
}
