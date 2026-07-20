package mitm

import (
	"fmt"
	"image"
	"strings"

	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

var (
	mrTypeVals  = []string{MRRequest, MRResponse}
	mrAreaVals  = []string{MRHeader, MRBody, MRFirstLine}
	scopeKinds  = []string{ScopeInclude, ScopeExclude}
	scopeFields = []string{"host", "protocol", "port", "path"}
	condFields  = []string{CondHost, CondIP, CondMethod, CondURL, CondFileType, CondMIME, CondStatus, CondParam, CondHeader, CondScope}
)

func condHint(field string) string {
	switch field {
	case CondHost:
		return "example.com"
	case CondIP:
		return "127.0.0.1"
	case CondMethod:
		return "POST"
	case CondURL:
		return "/api/login"
	case CondFileType:
		return "js"
	case CondMIME:
		return "application/json"
	case CondStatus:
		return "404"
	case CondParam:
		return "token"
	case CondHeader:
		return "Authorization"
	case CondScope:
		return "(in scope — no value)"
	}
	return "value"
}

// ---------------------------------------------------------------------------
// 4.2 TLS / CA
// ---------------------------------------------------------------------------

func (s *UIState) secTLS() []layout.Widget {
	ca := s.Proxy.CA()
	installed := TrustInstalled()

	caState := "not installed"
	if installed {
		caState = "loaded · trusted"
	} else if ca != nil {
		caState = "loaded · not trusted"
	}

	rows := []layout.Widget{
		s.secHeader(&s.SecTLSHdr, "TLS / CA", caState, &s.SecTLSOpen),
	}
	if !s.SecTLSOpen {
		return rows
	}

	// HTTPS decryption toggle group.
	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return switchRow(gtx, s.host.Theme, &s.DecryptSwitch, "Decrypt HTTPS (forward)")
				}),
				layout.Rigid(vSpace(4)),
				layout.Rigid(s.smallLabel("Terminate forward-mode TLS with the local CA to see request/response bodies. Reverse targets set TLS per-domain.")),
			)
		})
	}))

	// Root certificate management group.
	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.fieldLabel("Root certificate")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.caStatusLine(gtx, ca, installed) }),
				layout.Rigid(vSpace(8)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return btnWide(gtx, s.host.Theme, &s.GenCABtn, genLabel(ca), widgets.IconShield, theme.Border, s.host.Theme.Fg)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							bg := theme.BtnPrimary
							fg := theme.BtnPrimaryFg
							if ca == nil {
								bg = theme.Border
								fg = theme.FgDim
							}
							return btnWide(gtx, s.host.Theme, &s.InstallCABtn, "Install", nil, bg, fg)
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.ExportPEMBtn, "Export PEM", nil, theme.Border, s.host.Theme.Fg, ca != nil)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.ExportDERBtn, "Export DER", nil, theme.Border, s.host.Theme.Fg, ca != nil)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return btn(gtx, s.host.Theme, &s.RemoveCABtn, "Remove", nil, theme.Border, theme.Danger, installed)
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := "Browser import guide  ▾"
					if s.HelpOpen {
						label = "Browser import guide  ▴"
					}
					return btnWide(gtx, s.host.Theme, &s.HelpBtn, label, nil, theme.Border, s.host.Theme.Fg)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if s.CABanner == "" {
						return layout.Dimensions{}
					}
					col := theme.FgMuted
					switch {
					case strings.HasPrefix(s.CABanner, "CA generated"), strings.HasPrefix(s.CABanner, "CA installed"), strings.HasPrefix(s.CABanner, "Exported"):
						col = theme.MethodGet
					case strings.Contains(s.CABanner, "failed"), strings.Contains(strings.ToLower(s.CABanner), "administrator"):
						col = theme.Danger
					}
					lbl := material.Label(s.host.Theme, unit.Sp(10), s.CABanner)
					lbl.Color = col
					lbl.MaxLines = 3
					return layout.Inset{Top: unit.Dp(6)}.Layout(gtx, lbl.Layout)
				}),
			)
		})
	}))

	if s.HelpOpen {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions { return s.importGuide(gtx) })
	}
	return rows
}

// caStatusLine shows a coloured dot + текст describing CA trust state.
func (s *UIState) caStatusLine(gtx layout.Context, ca *CA, installed bool) layout.Dimensions {
	dot := theme.FgMuted
	txt := "Not generated"
	switch {
	case installed:
		dot = theme.MethodGet
		txt = "Generated & trusted by the system"
	case ca != nil:
		dot = theme.MethodPost
		txt = "Generated — not installed in trust store"
	}
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d := gtx.Dp(unit.Dp(8))
			paint.FillShape(gtx.Ops, dot, clip.Ellipse{Max: image.Pt(d, d)}.Op(gtx.Ops))
			return layout.Dimensions{Size: image.Pt(d, d)}
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(s.host.Theme, unit.Sp(11), txt)
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		}),
	)
}

// ---------------------------------------------------------------------------
// 4.3 Intercept rules
// ---------------------------------------------------------------------------

func (s *UIState) secIRules() []layout.Widget {
	enabled, conds := s.Proxy.IRules.Snapshot(s.IRulesActive)

	rows := []layout.Widget{
		s.secHeader(&s.SecIRulesHdr, "Intercept rules", fmt.Sprintf("%d", len(conds)), &s.SecIRulesOpen),
	}
	if !s.SecIRulesOpen {
		return rows
	}

	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tab(gtx, s.host.Theme, &s.IRulesReqTab, "Requests", s.IRulesActive == HeldRequest)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return tab(gtx, s.host.Theme, &s.IRulesRespTab, "Responses", s.IRulesActive == HeldResponse)
			}),
		)
	}))

	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			s.IRuleEnableSw.Value = enabled
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return switchRow(gtx, s.host.Theme, &s.IRuleEnableSw, "Only pause matching messages")
				}),
				layout.Rigid(vSpace(4)),
				layout.Rigid(s.smallLabel("With the Intercept view on, pause only messages that match the conditions below. Off = pause everything.")),
			)
		})
	}))

	// add condition
	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		opLabel := "AND"
		if s.IRuleOr {
			opLabel = "OR"
		}
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.fieldLabel("Add condition")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Match", unit.Dp(48))),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.IRuleOrBtn, opLabel)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.IRuleFieldBtn, condFields[s.IRuleFieldSel])
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Value", unit.Dp(48))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return widgets.TextField(gtx, s.host.Theme, &s.IRuleValInput, condHint(condFields[s.IRuleFieldSel]), true, nil, 0, unit.Sp(11))
						}),
					)
				}),
				layout.Rigid(vSpace(8)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.IRuleAddBtn, "Add condition", widgets.IconAdd, theme.BtnPrimary, theme.BtnPrimaryFg)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.IRulePresetImg, "Preset: skip images / CSS / JS", nil, theme.Border, s.host.Theme.Fg)
				}),
			)
		})
	}))

	if len(conds) > 0 {
		rows = append(rows, pad(s.fieldLabel("Conditions")))
	}
	for len(s.IRuleRows) < len(conds) {
		s.IRuleRows = append(s.IRuleRows, &CondRow{})
	}
	for i := range conds {
		i := i
		c := conds[i]
		r := s.IRuleRows[i]
		rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
			r.Enable.Value = c.Enabled
			op := "AND"
			if c.Or {
				op = "OR"
			}
			if i == 0 {
				op = ""
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.CheckBox(s.host.Theme, &r.Enable, "").Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), strings.TrimSpace(op+" "+c.Field+" = "+c.Value))
					lbl.Font.Typeface = widgets.MonoTypeface
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return iconBtn(gtx, s.host.Theme, &r.Remove, widgets.IconDel)
				}),
			)
		}))
	}
	return rows
}

// ---------------------------------------------------------------------------
// 4.4 Match & Replace
// ---------------------------------------------------------------------------

func (s *UIState) secMR() []layout.Widget {
	mrs := s.Proxy.MR.Snapshot()

	rows := []layout.Widget{
		s.secHeader(&s.SecMRHdr, "Match & Replace", fmt.Sprintf("%d", len(mrs)), &s.SecMROpen),
	}
	if !s.SecMROpen {
		return rows
	}

	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.fieldLabel("Add rule")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("In", unit.Dp(58))),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.MRTypeBtn, mrTypeVals[s.MRTypeSel])
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.MRAreaBtn, mrAreaVals[s.MRAreaSel])
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return switchRow(gtx, s.host.Theme, &s.MRRegexSw, "regex")
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Find", unit.Dp(58))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							hint := "text to find"
							if mrAreaVals[s.MRAreaSel] == MRHeader {
								hint = "Header-Name"
							}
							return widgets.TextField(gtx, s.host.Theme, &s.MRPatInput, hint, true, nil, 0, unit.Sp(11))
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Replace", unit.Dp(58))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return widgets.TextField(gtx, s.host.Theme, &s.MRReplInput, "new value  (empty = delete)", true, nil, 0, unit.Sp(11))
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Note", unit.Dp(58))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return widgets.TextField(gtx, s.host.Theme, &s.MRCommInput, "optional comment", true, nil, 0, unit.Sp(11))
						}),
					)
				}),
				layout.Rigid(vSpace(8)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.MRAddBtn, "Add rule", widgets.IconAdd, theme.BtnPrimary, theme.BtnPrimaryFg)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.MRPresetCSP, "Preset: strip CSP / X-Frame-Options", nil, theme.Border, s.host.Theme.Fg)
				}),
			)
		})
	}))

	if len(mrs) > 0 {
		rows = append(rows, pad(s.fieldLabel("Rules (applied in order)")))
	}
	for len(s.MRRows) < len(mrs) {
		s.MRRows = append(s.MRRows, &MRRow{})
	}
	for i := range mrs {
		i := i
		m := mrs[i]
		r := s.MRRows[i]
		rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
			r.Enable.Value = m.Enabled
			repl := m.Replacement
			if repl == "" {
				repl = "∅"
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.CheckBox(s.host.Theme, &r.Enable, "").Layout(gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(s.host.Theme, unit.Sp(10), fmt.Sprintf("%s · %s", m.Type, m.Area))
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(s.host.Theme, unit.Sp(11), m.Pattern+" → "+repl)
							lbl.Font.Typeface = widgets.MonoTypeface
							lbl.MaxLines = 1
							lbl.Truncator = "…"
							return lbl.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return iconBtn(gtx, s.host.Theme, &r.Up, widgets.IconExpandLess)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return iconBtn(gtx, s.host.Theme, &r.Down, widgets.IconExpandMore)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return iconBtn(gtx, s.host.Theme, &r.Remove, widgets.IconDel)
				}),
			)
		}))
	}
	return rows
}

// ---------------------------------------------------------------------------
// 4.5 Scope
// ---------------------------------------------------------------------------

func (s *UIState) secScope() []layout.Widget {
	scopes := s.Proxy.ScopeR.Snapshot()

	rows := []layout.Widget{
		s.secHeader(&s.SecScopeHdr, "Scope", fmt.Sprintf("%d", len(scopes)), &s.SecScopeOpen),
	}
	if !s.SecScopeOpen {
		return rows
	}

	rows = append(rows, pad(s.smallLabel("Limit the module to certain hosts/paths. With any Include rule, only matching traffic is in scope; Exclude always wins.")))
	rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
		return group(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(s.fieldLabel("Add scope rule")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Rule", unit.Dp(48))),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.ScopeKindBtn, scopeKinds[s.ScopeKindSel])
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return cycleBtn(gtx, s.host.Theme, &s.ScopeFieldBtn, scopeFields[s.ScopeFieldSel])
						}),
					)
				}),
				layout.Rigid(vSpace(6)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(s.inlineLabel("Match", unit.Dp(48))),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return widgets.TextField(gtx, s.host.Theme, &s.ScopePatInput, "substring or regex", true, nil, 0, unit.Sp(11))
						}),
					)
				}),
				layout.Rigid(vSpace(8)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return btnWide(gtx, s.host.Theme, &s.ScopeAddBtn, "Add scope rule", widgets.IconAdd, theme.BtnPrimary, theme.BtnPrimaryFg)
				}),
			)
		})
	}))

	for len(s.ScopeRows) < len(scopes) {
		s.ScopeRows = append(s.ScopeRows, &ScopeRow{})
	}
	for i := range scopes {
		i := i
		sc := scopes[i]
		r := s.ScopeRows[i]
		rows = append(rows, pad(func(gtx layout.Context) layout.Dimensions {
			r.Enable.Value = sc.Enabled
			col := theme.MethodGet
			if sc.Kind == ScopeExclude {
				col = theme.Danger
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return material.CheckBox(s.host.Theme, &r.Enable, "").Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(10), sc.Kind)
					lbl.Color = col
					return layout.Inset{Right: unit.Dp(6)}.Layout(gtx, lbl.Layout)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(11), sc.Field+": "+sc.Pattern)
					lbl.Font.Typeface = widgets.MonoTypeface
					lbl.MaxLines = 1
					lbl.Truncator = "…"
					return lbl.Layout(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return iconBtn(gtx, s.host.Theme, &r.Remove, widgets.IconDel)
				}),
			)
		}))
	}
	return rows
}

// ---------------------------------------------------------------------------
// CA import guide
// ---------------------------------------------------------------------------

func (s *UIState) importGuide(gtx layout.Context) layout.Dimensions {
	path := CACertPath(MITMDir())
	ffEnabled := FirefoxEnterpriseRootsEnabled()
	winInstalled := TrustInstalled()

	return layout.Inset{Top: unit.Dp(6), Bottom: unit.Dp(6), Left: unit.Dp(10), Right: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return boxed(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(s.host.Theme, unit.Sp(11), "Import the root certificate")
						lbl.Font.Weight = font.Bold
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(4)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(s.host.Theme, unit.Sp(10), path)
						lbl.Font.Typeface = widgets.MonoTypeface
						lbl.Color = theme.FgMuted
						lbl.MaxLines = 2
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return btn(gtx, s.host.Theme, &s.RevealBtn, "Reveal", nil, theme.Border, s.host.Theme.Fg, true)
							}),
							layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return btn(gtx, s.host.Theme, &s.CopyPathBtn, "Copy path", nil, theme.Border, s.host.Theme.Fg, true)
							}),
						)
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.guideBrowser(gtx, "Chrome / Edge (Windows trust)", winInstalled, chromeEdgeSteps(winInstalled))
					}),
					layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.guideBrowser(gtx, "Firefox", ffEnabled, firefoxSteps())
					}),
				)
			})
		})
	})
}

func (s *UIState) guideBrowser(gtx layout.Context, title string, ok bool, steps []string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			mark := "▸"
			col := theme.FgMuted
			if ok {
				mark = "✓"
				col = theme.MethodGet
			}
			lbl := material.Label(s.host.Theme, unit.Sp(11), mark+" "+title)
			lbl.Color = col
			lbl.Font.Weight = font.Bold
			return lbl.Layout(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(steps))
			for i, step := range steps {
				i, step := i, step
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					lbl := material.Label(s.host.Theme, unit.Sp(10), fmt.Sprintf("%d. %s", i+1, step))
					lbl.Color = theme.FgMuted
					return layout.Inset{Left: unit.Dp(10), Top: unit.Dp(1), Bottom: unit.Dp(1)}.Layout(gtx, lbl.Layout)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}),
	)
}

func chromeEdgeSteps(installed bool) []string {
	if installed {
		return []string{
			"Already trusted — Install added our root to Windows LocalMachine\\Root.",
			"Restart open Chrome / Edge windows to re-read the trust store.",
		}
	}
	return []string{
		"Click Install (admin). Chrome and Edge share the Windows trust store.",
		"Restart open browser windows.",
	}
}

func firefoxSteps() []string {
	return []string{
		"Firefox → Settings → Privacy & Security → Certificates → View Certificates.",
		"Authorities tab → Import → pick tracto-ca.crt.",
		"Check \"Trust this CA to identify websites\" → OK.",
		"Restart Firefox.",
	}
}
