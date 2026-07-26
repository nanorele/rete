package mitm

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget"
)

// handleEvents processes all module-level widget events once per frame.
func (s *UIState) handleEvents(gtx layout.Context) {
	// Start / Stop.
	for s.StartBtn.Clicked(gtx) {
		switch {
		case s.Proxy.Running():
			s.Proxy.Stop()
			s.StatusBanner = "Proxy stopped"
		case !IsAdmin():
			s.host.Elevate(&s.StatusBanner, "--mitm-start")
		default:
			addr := strings.TrimSpace(s.BindAddr.Text())
			if err := s.Proxy.Start(addr); err != nil {
				s.StatusBanner = "Start failed: " + err.Error()
			} else {
				s.StatusBanner = "Proxy listening on " + s.Proxy.Addr()
				s.MarkDirty()
			}
		}
		s.host.Window.Invalidate() // refresh title-bar proxy status
	}

	// Clear with confirmation.
	for s.ClearBtn.Clicked(gtx) {
		if s.Store.Len() > 0 || s.Proxy.WS.Len() > 0 {
			s.ClearConfirmOpen = true
		}
	}
	for s.ClearYesBtn.Clicked(gtx) {
		s.Store.Clear()
		s.Proxy.WS.Clear()
		s.Selected = 0
		s.WSSelected = 0
		s.ClearConfirmOpen = false
	}
	for s.ClearNoBtn.Clicked(gtx) {
		s.ClearConfirmOpen = false
	}

	// View switcher.
	for s.SegHistory.Clicked(gtx) {
		s.View = ViewHistory
		s.MarkDirty()
	}
	for s.SegInterc.Clicked(gtx) {
		s.View = ViewIntercept
		s.MarkDirty()
	}
	for s.SegWS.Clicked(gtx) {
		s.View = ViewWebSockets
		s.MarkDirty()
	}
	for s.FilterClr.Clicked(gtx) {
		s.Filter.SetText("")
	}

	// Inspector tabs / render modes / sections.
	for s.TabReq.Clicked(gtx) {
		s.ActTab = 0
	}
	for s.TabResp.Clicked(gtx) {
		s.ActTab = 1
	}
	for s.ViewRaw.Clicked(gtx) {
		s.RenderMode = 0
	}
	for s.ViewPretty.Clicked(gtx) {
		s.RenderMode = 1
	}
	for s.ViewHex.Clicked(gtx) {
		s.RenderMode = 2
	}
	for s.ViewRender.Clicked(gtx) {
		s.RenderMode = 3
	}
	for s.SecHeaders.Clicked(gtx) {
		s.SecTab = 0
	}
	for s.SecBody.Clicked(gtx) {
		s.SecTab = 1
	}
	for s.SecParams.Clicked(gtx) {
		s.SecTab = 2
	}
	for s.SecCookies.Clicked(gtx) {
		s.SecTab = 3
	}
	for s.InspectorToggle.Clicked(gtx) {
		s.InspectorCollapsed = !s.InspectorCollapsed
		s.MarkDirty()
	}
	// Inspector action row (bus stubs) + copy.
	for s.InspCopy.Clicked(gtx) {
		if f := s.Store.FindByID(s.Selected); f != nil {
			copyText(gtx, flowAsText(f, s.ActTab == 1))
			s.StatusBanner = "Copied to clipboard"
		}
	}
	s.handleSendBus(gtx)

	// Manual interception.
	if s.InterceptSwitch.Update(gtx) {
		s.Proxy.Manual.SetOn(s.InterceptSwitch.Value)
		if s.InterceptSwitch.Value {
			s.View = ViewIntercept
		}
		s.MarkDirty()
	}
	if s.InterceptRespSw.Update(gtx) {
		s.Proxy.Manual.SetInterceptResponses(s.InterceptRespSw.Value)
		s.MarkDirty()
	}
	s.handleInterceptDecisions(gtx)
	s.viewRowEvents(gtx)

	// Row context menu + annotate.
	s.handleContextEvents(gtx)
}

// viewRowEvents polls the central-view list rows. Like the sidebar, these
// MUST be polled here (before the view's material.List renders) rather than
// inside the List element closures, which never receive the events.
func (s *UIState) viewRowEvents(gtx layout.Context) {
	switch s.View {
	case ViewHistory:
		for i := range histCols {
			for s.SortClicks[i].Clicked(gtx) {
				if s.SortColumn == histCols[i] {
					s.SortAsc = !s.SortAsc
				} else {
					s.SortColumn = histCols[i]
					s.SortAsc = true
				}
				s.MarkDirty()
			}
		}
		if s.HideNoiseSw.Update(gtx) {
			// filter refresh only
		}
		flows := s.filteredFlows()
		for len(s.RowClicks) < len(flows) {
			s.RowClicks = append(s.RowClicks, &widget.Clickable{})
		}
		for len(s.RowMore) < len(flows) {
			s.RowMore = append(s.RowMore, &widget.Clickable{})
		}
		for i := range flows {
			for s.RowClicks[i].Clicked(gtx) {
				s.Selected = flows[i].ID
			}
			for s.RowMore[i].Clicked(gtx) {
				s.CtxOpen = true
				s.CtxFlowID = flows[i].ID
				s.CtxPos = s.LocalPtr
			}
		}
	case ViewWebSockets:
		msgs := s.filteredWS()
		for len(s.WSRowClk) < len(msgs) {
			s.WSRowClk = append(s.WSRowClk, &widget.Clickable{})
		}
		for i := range msgs {
			for s.WSRowClk[i].Clicked(gtx) {
				s.WSSelected = msgs[i].ID
			}
		}
	}
}

// sidebarEvents processes zone-B (accordion) widget events. It MUST run
// within the sidebar layout pass so the polled widgets are laid out in the same
// subtree; polling them from zone C (handleEvents) never receives events.
func (s *UIState) sidebarEvents(gtx layout.Context) {
	// Accordion section headers.
	for s.SecTargetsHdr.Clicked(gtx) {
		s.SecTargetsOpen = !s.SecTargetsOpen
	}
	for s.SecTLSHdr.Clicked(gtx) {
		s.SecTLSOpen = !s.SecTLSOpen
	}
	for s.SecIRulesHdr.Clicked(gtx) {
		s.SecIRulesOpen = !s.SecIRulesOpen
	}
	for s.SecMRHdr.Clicked(gtx) {
		s.SecMROpen = !s.SecMROpen
	}
	for s.SecScopeHdr.Clicked(gtx) {
		s.SecScopeOpen = !s.SecScopeOpen
	}

	s.handleCAEvents(gtx)

	// Decrypt HTTPS toggle.
	if s.DecryptSwitch.Update(gtx) {
		if s.DecryptSwitch.Value {
			if s.Proxy.CA() == nil {
				s.DecryptSwitch.Value = false
				s.CABanner = "Generate and install a CA before enabling HTTPS decryption"
			} else {
				s.Proxy.SetIntercept(true)
				s.MarkDirty()
			}
		} else {
			s.Proxy.SetIntercept(false)
			s.MarkDirty()
		}
	}

	s.targetsEvents(gtx)
	s.iRulesEvents(gtx)
	s.mrEvents(gtx)
	s.scopeEvents(gtx)
}

func (s *UIState) targetsEvents(gtx layout.Context) {
	for s.TargetAddBtn.Clicked(gtx) {
		s.addTarget()
	}
	for _, t := range s.Proxy.Targets.Snapshot() {
		row := s.TargetRows[t.Domain]
		if row == nil {
			continue
		}
		for row.Expand.Clicked(gtx) {
			row.Expanded = !row.Expanded
			if row.Expanded && row.AddrInput.Text() == "" {
				row.AddrInput.SetText(t.UpstreamAddr)
			}
		}
		for row.Remove.Clicked(gtx) {
			s.Proxy.Targets.Remove(t.Domain)
			delete(s.TargetRows, t.Domain)
			s.MarkDirty()
		}
		for row.Copy.Clicked(gtx) {
			copyText(gtx, HostsLine(t.Domain))
			s.StatusBanner = "Copied hosts line"
		}
		for row.UpstreamAuto.Clicked(gtx) {
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.Upstream = UpstreamAuto })
			s.MarkDirty()
		}
		for row.UpstreamManual.Clicked(gtx) {
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.Upstream = UpstreamManual })
			s.MarkDirty()
		}
		for row.TLSDecrypt.Clicked(gtx) {
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.TLS = TLSDecrypt })
			s.MarkDirty()
		}
		for row.TLSTunnel.Clicked(gtx) {
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.TLS = TLSTunnel })
			s.MarkDirty()
		}
		if row.DoH.Update(gtx) {
			doh := row.DoH.Value
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.DoH = doh })
			s.MarkDirty()
		}
		if addr := strings.TrimSpace(row.AddrInput.Text()); row.Expanded && addr != t.UpstreamAddr {
			s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.UpstreamAddr = addr })
			s.MarkDirty()
		}
		if ds := strings.TrimSpace(row.DelayInput.Text()); row.Expanded && ds != "" {
			if ms, err := strconv.Atoi(ds); err == nil && time.Duration(ms)*time.Millisecond != t.Delay {
				s.Proxy.Targets.Update(t.Domain, func(tg *Target) { tg.Delay = time.Duration(ms) * time.Millisecond })
				s.MarkDirty()
			}
		}
	}
}

func (s *UIState) iRulesEvents(gtx layout.Context) {
	for s.IRulesReqTab.Clicked(gtx) {
		s.IRulesActive = HeldRequest
	}
	for s.IRulesRespTab.Clicked(gtx) {
		s.IRulesActive = HeldResponse
	}
	if s.IRuleEnableSw.Update(gtx) {
		s.Proxy.IRules.SetEnabled(s.IRulesActive, s.IRuleEnableSw.Value)
		s.MarkDirty()
	}
	for s.IRulePresetImg.Clicked(gtx) {
		s.Proxy.IRules.Add(s.IRulesActive, InterceptCond{Enabled: true, Field: CondFileType, Value: "js"})
		s.Proxy.IRules.Add(s.IRulesActive, InterceptCond{Enabled: true, Or: true, Field: CondFileType, Value: "css"})
		s.MarkDirty()
	}
	for s.IRuleFieldBtn.Clicked(gtx) {
		s.IRuleFieldSel = (s.IRuleFieldSel + 1) % len(condFields)
	}
	for s.IRuleOrBtn.Clicked(gtx) {
		s.IRuleOr = !s.IRuleOr
	}
	for s.IRuleAddBtn.Clicked(gtx) {
		s.Proxy.IRules.Add(s.IRulesActive, InterceptCond{
			Enabled: true, Or: s.IRuleOr, Field: condFields[s.IRuleFieldSel], Value: strings.TrimSpace(s.IRuleValInput.Text()),
		})
		s.IRuleValInput.SetText("")
		s.MarkDirty()
	}
	_, conds := s.Proxy.IRules.Snapshot(s.IRulesActive)
	for i := range conds {
		if i >= len(s.IRuleRows) {
			break
		}
		r := s.IRuleRows[i]
		if r.Enable.Update(gtx) {
			en := r.Enable.Value
			s.Proxy.IRules.Update(s.IRulesActive, i, func(cc *InterceptCond) { cc.Enabled = en })
			s.MarkDirty()
		}
		for r.Remove.Clicked(gtx) {
			s.Proxy.IRules.Remove(s.IRulesActive, i)
			s.MarkDirty()
		}
	}
}

func (s *UIState) mrEvents(gtx layout.Context) {
	for s.MRTypeBtn.Clicked(gtx) {
		s.MRTypeSel = (s.MRTypeSel + 1) % len(mrTypeVals)
	}
	for s.MRAreaBtn.Clicked(gtx) {
		s.MRAreaSel = (s.MRAreaSel + 1) % len(mrAreaVals)
	}
	for s.MRAddBtn.Clicked(gtx) {
		pattern := strings.TrimSpace(s.MRPatInput.Text())
		if pattern == "" {
			continue
		}
		s.Proxy.MR.Add(MatchReplaceRule{
			Enabled: true, Type: mrTypeVals[s.MRTypeSel], Area: mrAreaVals[s.MRAreaSel],
			Pattern: pattern, Replacement: s.MRReplInput.Text(),
			IsRegex: s.MRRegexSw.Value, Comment: s.MRCommInput.Text(),
		})
		s.MRPatInput.SetText("")
		s.MRReplInput.SetText("")
		s.MRCommInput.SetText("")
		s.MarkDirty()
	}
	s.MRRegexSw.Update(gtx)
	for s.MRPresetCSP.Clicked(gtx) {
		for _, h := range []string{"Content-Security-Policy", "X-Frame-Options"} {
			s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: h, Replacement: "", Comment: "strip security header"})
		}
		s.MarkDirty()
	}
	mrs := s.Proxy.MR.Snapshot()
	for i := range mrs {
		if i >= len(s.MRRows) {
			break
		}
		r := s.MRRows[i]
		if r.Enable.Update(gtx) {
			en := r.Enable.Value
			s.Proxy.MR.Update(i, func(mm *MatchReplaceRule) { mm.Enabled = en })
			s.MarkDirty()
		}
		for r.Remove.Clicked(gtx) {
			s.Proxy.MR.Remove(i)
			s.MarkDirty()
		}
		for r.Up.Clicked(gtx) {
			s.Proxy.MR.Move(i, -1)
			s.MarkDirty()
		}
		for r.Down.Clicked(gtx) {
			s.Proxy.MR.Move(i, 1)
			s.MarkDirty()
		}
	}
}

func (s *UIState) scopeEvents(gtx layout.Context) {
	for s.ScopeKindBtn.Clicked(gtx) {
		s.ScopeKindSel = (s.ScopeKindSel + 1) % len(scopeKinds)
	}
	for s.ScopeFieldBtn.Clicked(gtx) {
		s.ScopeFieldSel = (s.ScopeFieldSel + 1) % len(scopeFields)
	}
	for s.ScopeAddBtn.Clicked(gtx) {
		s.Proxy.ScopeR.Add(ScopeRule{
			Enabled: true, Kind: scopeKinds[s.ScopeKindSel], Field: scopeFields[s.ScopeFieldSel],
			Pattern: strings.TrimSpace(s.ScopePatInput.Text()),
		})
		s.ScopePatInput.SetText("")
		s.MarkDirty()
	}
	scopes := s.Proxy.ScopeR.Snapshot()
	for i := range scopes {
		if i >= len(s.ScopeRows) {
			break
		}
		r := s.ScopeRows[i]
		if r.Enable.Update(gtx) {
			en := r.Enable.Value
			s.Proxy.ScopeR.Update(i, func(ss *ScopeRule) { ss.Enabled = en })
			s.MarkDirty()
		}
		for r.Remove.Clicked(gtx) {
			s.Proxy.ScopeR.Remove(i)
			s.MarkDirty()
		}
	}
}

func (s *UIState) handleCAEvents(gtx layout.Context) {
	for s.GenCABtn.Clicked(gtx) {
		ca, err := GenerateCA()
		if err != nil {
			s.CABanner = "Generate failed: " + err.Error()
		} else if err := ca.Save(MITMDir()); err != nil {
			s.CABanner = "Save failed: " + err.Error()
		} else {
			s.Proxy.SetCA(ca)
			s.CABanner = "CA generated • " + ca.Fingerprint()
		}
	}
	for s.InstallCABtn.Clicked(gtx) {
		ca := s.Proxy.CA()
		switch {
		case ca == nil:
			s.CABanner = "Generate a CA first"
		case !IsAdmin():
			s.host.Elevate(&s.CABanner, "--mitm-install-ca")
		default:
			if err := InstallTrust(CACertPath(MITMDir())); err != nil {
				s.CABanner = "Install failed: " + err.Error()
			} else {
				s.CABanner = "CA installed into Windows trust • Firefox needs manual import — see \"Import guide\""
				s.HelpOpen = true
			}
		}
	}
	for s.RemoveCABtn.Clicked(gtx) {
		if !IsAdmin() {
			s.host.Elevate(&s.CABanner, "--mitm-remove-ca")
		} else if err := UninstallTrust(); err != nil {
			s.CABanner = "Remove failed: " + err.Error()
		} else {
			s.CABanner = "CA removed from trust store"
		}
	}
	for s.ExportPEMBtn.Clicked(gtx) {
		s.exportCA(false)
	}
	for s.ExportDERBtn.Clicked(gtx) {
		s.exportCA(true)
	}
	for s.HelpBtn.Clicked(gtx) {
		s.HelpOpen = !s.HelpOpen
	}
	for s.RevealBtn.Clicked(gtx) {
		if err := RevealInExplorer(CACertPath(MITMDir())); err != nil {
			s.CABanner = "Reveal failed: " + err.Error()
		}
	}
	for s.CopyPathBtn.Clicked(gtx) {
		copyText(gtx, CACertPath(MITMDir()))
		s.CABanner = "Path copied to clipboard"
	}
}

func (s *UIState) exportCA(der bool) {
	ca := s.Proxy.CA()
	if ca == nil {
		s.CABanner = "Generate a CA first"
		return
	}
	var path string
	var data []byte
	if der {
		path = filepath.Join(MITMDir(), "tracto-ca.der")
		data = ca.Cert.Raw
	} else {
		path = filepath.Join(MITMDir(), "tracto-ca.pem")
		data = ca.CertPEM
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		s.CABanner = "Export failed: " + err.Error()
		return
	}
	s.CABanner = "Exported " + strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), ".")) + " → " + path
	_ = RevealInExplorer(path)
}

// handleSendBus wires the "Send to X" action buttons (shared bus stubs).
func (s *UIState) handleSendBus(gtx layout.Context) {
	for s.InspSendRepeater.Clicked(gtx) {
		s.StatusBanner = "Sent to Repeater (bus)"
	}
	for s.InspSendIntruder.Clicked(gtx) {
		s.StatusBanner = "Sent to Intruder (bus)"
	}
	for s.InspSendComparer.Clicked(gtx) {
		s.StatusBanner = "Sent to Comparer (bus)"
	}
	for s.InspSendDecoder.Clicked(gtx) {
		s.StatusBanner = "Sent to Decoder (bus)"
	}
}

// handleInterceptDecisions processes Forward/Drop on the head of the queue.
func (s *UIState) handleInterceptDecisions(gtx layout.Context) {
	q := s.Proxy.Manual.Queue()
	if len(q) == 0 {
		return
	}
	head := q[0]
	for s.ForwardBtn.Clicked(gtx) {
		edited := []byte(s.HeldEditor.Text())
		s.Proxy.Manual.Forward(head.ID, edited)
		s.HeldEditorFor = 0
	}
	for s.DropBtn.Clicked(gtx) {
		s.Proxy.Manual.Drop(head.ID)
		s.HeldEditorFor = 0
	}
}

// handleContextEvents handles the history row context menu + annotate popup.
func (s *UIState) handleContextEvents(gtx layout.Context) {
	for s.CtxCopyURL.Clicked(gtx) {
		if f := s.Store.FindByID(s.CtxFlowID); f != nil {
			copyText(gtx, f.URL)
			s.StatusBanner = "URL copied"
		}
		s.CtxOpen = false
	}
	for s.CtxCopyCurl.Clicked(gtx) {
		if f := s.Store.FindByID(s.CtxFlowID); f != nil {
			copyText(gtx, asCurl(f))
			s.StatusBanner = "curl copied"
		}
		s.CtxOpen = false
	}
	for s.CtxCopyReq.Clicked(gtx) {
		if f := s.Store.FindByID(s.CtxFlowID); f != nil {
			copyText(gtx, flowAsText(f, false))
			s.StatusBanner = "Request copied"
		}
		s.CtxOpen = false
	}
	for s.CtxRepeat.Clicked(gtx) {
		s.StatusBanner = "Repeat request (bus)"
		s.CtxOpen = false
	}
	for s.CtxToRepeater.Clicked(gtx) {
		s.StatusBanner = "Sent to Repeater (bus)"
		s.CtxOpen = false
	}
	for s.CtxAddScope.Clicked(gtx) {
		if f := s.Store.FindByID(s.CtxFlowID); f != nil && f.Host != "" {
			s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: f.Host})
			s.StatusBanner = "Added " + f.Host + " to scope"
			s.MarkDirty()
		}
		s.CtxOpen = false
	}
	for s.CtxDelete.Clicked(gtx) {
		s.Store.Delete(s.CtxFlowID)
		if s.Selected == s.CtxFlowID {
			s.Selected = 0
		}
		s.CtxOpen = false
	}
	for s.CtxAnnotate.Clicked(gtx) {
		s.AnnotateOpen = true
		s.AnnotateFlowID = s.CtxFlowID
		if f := s.Store.FindByID(s.CtxFlowID); f != nil {
			s.AnnotateComment.SetText(f.Comment)
		}
		s.CtxOpen = false
	}
	colors := annotateColorKeys()
	for i := range s.AnnotateColors {
		for s.AnnotateColors[i].Clicked(gtx) {
			s.Store.SetAnnotation(s.AnnotateFlowID, colors[i], s.AnnotateComment.Text())
		}
	}
	for s.AnnotateSave.Clicked(gtx) {
		if f := s.Store.FindByID(s.AnnotateFlowID); f != nil {
			s.Store.SetAnnotation(s.AnnotateFlowID, f.Highlight, s.AnnotateComment.Text())
		}
		s.AnnotateOpen = false
	}
}

// ---------------------------------------------------------------------------
// Config persistence (debounced)
// ---------------------------------------------------------------------------

func (s *UIState) flushConfig() {
	if s.Dirty() {
		s.saveAt = time.Now()
		s.savePending = true
	}
	if s.savePending && time.Since(s.saveAt) > 500*time.Millisecond {
		s.savePending = false
		cfg := s.SnapshotConfig()
		go func() { _ = SaveConfig(cfg) }()
	} else if s.savePending && s.host.Window != nil {
		win := s.host.Window
		time.AfterFunc(520*time.Millisecond, func() { win.Invalidate() })
	}
}
