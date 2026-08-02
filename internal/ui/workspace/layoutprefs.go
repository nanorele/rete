package workspace

type LayoutPrefs struct {
	Valid             bool
	SplitRatio        float32
	VStackRatio       float32
	LayoutMode        int
	HeaderKeyW        float32
	HeadersAbsHeight  int
	HeadersManualDp   int
	HeadersResized    bool
	ReqBodyCollapsed  bool
	RespBodyCollapsed bool
	ReqRatioSaved     float32
	RespRatioSaved    float32
	WSSplitRatio      float32
	WSComposerRatio   float32
	GQLVarsRatio      float32
	WSHeadersColl     bool
	WSComposeColl     bool
	WSMessagesColl    bool
	WSComposeSaved    float32
	WSMsgsSaved       float32
}

func (t *RequestTab) MergeLayoutPrefs(p *LayoutPrefs) {
	if t == nil || p == nil {
		return
	}
	p.Valid = true
	p.SplitRatio = t.SplitRatio
	p.VStackRatio = t.VStackRatio
	p.LayoutMode = t.LayoutMode
	p.HeaderKeyW = t.HeaderKeyW
	p.ReqBodyCollapsed = t.ReqBodyCollapsed
	p.RespBodyCollapsed = t.RespBodyCollapsed
	p.ReqRatioSaved = t.reqRatioSaved
	p.RespRatioSaved = t.respRatioSaved
	if t.hbUserResized {
		p.HeadersResized = true
		p.HeadersAbsHeight = t.HeadersAbsHeight
		p.HeadersManualDp = t.hbManualDp
	}
	if t.WS != nil {
		if t.WS.SplitRatio > 0 {
			p.WSSplitRatio = t.WS.SplitRatio
		}
		if t.WS.ComposerRatio > 0 {
			p.WSComposerRatio = t.WS.ComposerRatio
		}
		p.WSHeadersColl = t.WS.HeadersCollapsed
		p.WSComposeColl = t.WS.ComposeCollapsed
		p.WSMessagesColl = t.WS.MessagesCollapsed
		p.WSComposeSaved = t.WS.composeSavedRatio
		p.WSMsgsSaved = t.WS.msgsSavedRatio
	}
	if t.GQL != nil && t.GQL.VarsSplitRatio > 0 {
		p.GQLVarsRatio = t.GQL.VarsSplitRatio
	}
}

func (t *RequestTab) ApplyLayoutPrefs(p LayoutPrefs) {
	if t == nil || !p.Valid {
		return
	}
	if p.SplitRatio > 0 {
		t.SplitRatio = p.SplitRatio
	}
	if p.VStackRatio > 0 {
		t.VStackRatio = p.VStackRatio
	}
	t.LayoutMode = p.LayoutMode
	t.HeaderKeyW = p.HeaderKeyW
	t.ReqBodyCollapsed = p.ReqBodyCollapsed
	t.RespBodyCollapsed = p.RespBodyCollapsed
	t.reqRatioSaved = p.ReqRatioSaved
	t.respRatioSaved = p.RespRatioSaved
	if p.HeadersResized && p.HeadersAbsHeight > 0 {
		t.hbUserResized = true
		t.HeadersAbsHeight = p.HeadersAbsHeight
		t.hbManualDp = p.HeadersManualDp
		if t.hbManualDp <= 0 {
			t.hbManualDp = p.HeadersAbsHeight
		}
	}
	if t.WS != nil {
		if p.WSSplitRatio > 0 {
			t.WS.SplitRatio = p.WSSplitRatio
		}
		if p.WSComposerRatio > 0 {
			t.WS.ComposerRatio = p.WSComposerRatio
		}
		t.WS.HeadersCollapsed = p.WSHeadersColl
		t.WS.ComposeCollapsed = p.WSComposeColl
		t.WS.MessagesCollapsed = p.WSMessagesColl
		t.WS.composeSavedRatio = p.WSComposeSaved
		t.WS.msgsSavedRatio = p.WSMsgsSaved
	}
	if t.GQL != nil && p.GQLVarsRatio > 0 {
		t.GQL.VarsSplitRatio = p.GQLVarsRatio
	}
	t.splitPaneRec = 0
	t.splitRespRec = 0
}
