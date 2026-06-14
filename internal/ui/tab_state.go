package ui

import (
	"os"

	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/workspace"
	"tracto/internal/ws"
)

func (ui *AppUI) loadTabFromState(ts persist.TabState) *workspace.RequestTab {
	rt := workspace.NewRequestTab(ts.Title)
	if rt.Title == "" {
		rt.Title = "New request"
	}
	method := ts.Method
	if ts.Kind == workspace.TabKindWebSocket {
		method = workspace.MethodWS
	}
	if ts.Kind == workspace.TabKindGraphQL {
		method = workspace.MethodGraphQL
	}
	if method == "" {
		method = "GET"
	}
	rt.Method = method
	rt.URLInput.SetText(ts.URL)
	rt.ReqEditor.SetText(ts.Body)
	for _, hs := range ts.Headers {
		rt.AddHeader(hs.Key, hs.Value)
	}
	rt.HeadersExpanded = ts.HeadersExpanded
	rt.HeadersAbsHeight = ts.HeadersAbsHeight
	if ts.SplitRatio > 0 {
		rt.SplitRatio = ts.SplitRatio
	}
	if ts.VStackRatio > 0 {
		rt.VStackRatio = ts.VStackRatio
	}
	rt.LayoutMode = ts.LayoutMode
	if ts.HeaderSplitRatio > 0 {
		rt.HeaderSplitRatio = ts.HeaderSplitRatio
	}
	if ts.ReqWrapEnabled != nil {
		rt.ReqWrapEnabled = *ts.ReqWrapEnabled
	}
	rt.PendingColID = ts.CollectionID
	rt.PendingNodePath = ts.NodePath
	rt.BodyType = model.BodyTypeFromMode(ts.BodyType)
	for _, fp := range ts.FormParts {
		kind := model.FormPartText
		if fp.Kind == "file" {
			kind = model.FormPartFile
		}
		var size int64
		if kind == model.FormPartFile && fp.FilePath != "" {
			if fi, err := os.Stat(fp.FilePath); err == nil {
				size = fi.Size()
			}
		}
		rt.FormParts = append(rt.FormParts, workspace.NewFormPart(fp.Key, fp.Value, kind, fp.FilePath, size))
	}
	for _, ue := range ts.URLEncoded {
		rt.URLEncoded = append(rt.URLEncoded, workspace.NewURLEncodedPart(ue.Key, ue.Value))
	}
	rt.BinaryFilePath = ts.BinaryPath
	if ts.BinaryPath != "" {
		if fi, err := os.Stat(ts.BinaryPath); err == nil {
			rt.BinaryFileSize = fi.Size()
		}
	}
	rt.UpdateSystemHeaders()
	if ts.WS != nil {
		ws := rt.EnsureWS()
		for _, sp := range ts.WS.Subprotocols {
			ws.AddSubprotocol(sp)
		}
		ws.OptionsExpanded = ts.WS.OptionsExpanded
		ws.SubprotosAbsHeight = ts.WS.SubprotosAbsHeight
		ws.OfferDeflate = ts.WS.OfferDeflate
		ws.InsecureSkipVerify = ts.WS.InsecureSkipVerify
		ws.UseTractoCA = ts.WS.UseTractoCA
		if ts.WS.SplitRatio > 0 {
			ws.SplitRatio = ts.WS.SplitRatio
		}
		if ts.WS.ComposerRatio > 0 {
			ws.ComposerRatio = ts.WS.ComposerRatio
		}
		for _, s := range ts.WS.SavedSends {
			ws.AppendSavedSend(s.Name, s.Text, opcodeFromString(s.Opcode))
		}
	}
	if ts.GQL != nil {
		g := rt.EnsureGQL()
		g.Query.SetText(ts.GQL.Query)
		g.Variables.SetText(ts.GQL.Variables)
		if ts.GQL.VarsSplitRatio > 0 {
			g.VarsSplitRatio = ts.GQL.VarsSplitRatio
		}
	}
	return rt
}

func (ui *AppUI) tabStateFromTab(rt *workspace.RequestTab) persist.TabState {
	reqWrap := rt.ReqWrapEnabled
	kind := workspace.TabKindHTTP
	method := rt.Method
	if rt.Method == workspace.MethodWS {
		kind = workspace.TabKindWebSocket
	}
	if rt.Method == workspace.MethodGraphQL {
		kind = workspace.TabKindGraphQL
	}
	ts := persist.TabState{
		Kind:             kind,
		Title:            rt.Title,
		Method:           method,
		URL:              rt.URLInput.Text(),
		Body:             rt.ReqEditor.Text(),
		SplitRatio:       rt.SplitRatio,
		VStackRatio:      rt.VStackRatio,
		LayoutMode:       rt.LayoutMode,
		HeaderSplitRatio: rt.HeaderSplitRatio,
		HeadersExpanded:  rt.HeadersExpanded,
		HeadersAbsHeight: rt.HeadersAbsHeight,
		ReqWrapEnabled:   &reqWrap,
		BodyType:         rt.BodyType.PostmanMode(),
		BinaryPath:       rt.BinaryFilePath,
	}
	for _, p := range rt.FormParts {
		k := "text"
		if p.Kind == model.FormPartFile {
			k = "file"
		}
		ts.FormParts = append(ts.FormParts, persist.FormPartState{
			Key:      p.Key.Text(),
			Kind:     k,
			Value:    p.Value.Text(),
			FilePath: p.FilePath,
		})
	}
	for _, ue := range rt.URLEncoded {
		ts.URLEncoded = append(ts.URLEncoded, persist.HeaderState{
			Key:   ue.Key.Text(),
			Value: ue.Value.Text(),
		})
	}
	if rt.LinkedNode != nil && rt.LinkedNode.Collection != nil {
		ts.CollectionID = rt.LinkedNode.Collection.ID
		ts.NodePath = collections.NodePathFrom(rt.LinkedNode.Collection.Root, rt.LinkedNode)
	}
	ts.Headers = make([]persist.HeaderState, 0, len(rt.Headers))
	for _, h := range rt.Headers {
		if !h.IsGenerated {
			k := h.Key.Text()
			if k != "" {
				ts.Headers = append(ts.Headers, persist.HeaderState{Key: k, Value: h.Value.Text()})
			}
		}
	}
	if rt.WS != nil {
		wsState := &persist.WSTabState{
			Subprotocols:       rt.WS.SubprotocolList(),
			OptionsExpanded:    rt.WS.OptionsExpanded,
			SubprotosAbsHeight: rt.WS.SubprotosAbsHeight,
			OfferDeflate:       rt.WS.OfferDeflate,
			InsecureSkipVerify: rt.WS.InsecureSkipVerify,
			UseTractoCA:        rt.WS.UseTractoCA,
			SplitRatio:         rt.WS.SplitRatio,
			ComposerRatio:      rt.WS.ComposerRatio,
		}
		for _, s := range rt.WS.SavedSends {
			wsState.SavedSends = append(wsState.SavedSends, persist.WSSavedSend{
				Name:   s.Name,
				Opcode: opcodeToString(s.Opcode),
				Text:   s.Text,
			})
		}
		ts.WS = wsState
	}
	if rt.GQL != nil {
		ts.GQL = &persist.GQLTabState{
			Query:          rt.GQL.Query.Text(),
			Variables:      rt.GQL.Variables.Text(),
			VarsSplitRatio: rt.GQL.VarsSplitRatio,
		}
	}
	return ts
}

func opcodeFromString(s string) ws.Opcode {
	if s == "BIN" || s == "binary" {
		return ws.OpBinary
	}
	return ws.OpText
}

func opcodeToString(op ws.Opcode) string {
	if op == ws.OpBinary {
		return "BIN"
	}
	return "TEXT"
}
