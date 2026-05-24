package ui

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"tracto/internal/ui/mitm"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

func (ui *AppUI) mitmRulesSection(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM

	for st.RuleAddBtn.Clicked(gtx) {
		ui.mitmAddRuleFromForm()
	}

	rules := st.Proxy.Rules.Snapshot()
	for _, r := range rules {
		clk, ok := st.RuleRowRemove[r.Host]
		if !ok {
			clk = &widget.Clickable{}
			st.RuleRowRemove[r.Host] = clk
		}
		for clk.Clicked(gtx) {
			st.Proxy.Rules.Remove(r.Host)
			delete(st.RuleRowRemove, r.Host)
			st.RuleBanner = "Rule removed: " + r.Host
		}
	}
	for h := range st.RuleRowRemove {
		found := false
		for _, r := range rules {
			if r.Host == h {
				found = true
				break
			}
		}
		if !found {
			delete(st.RuleRowRemove, h)
		}
	}

	return mitmBgBar(gtx, theme.BgSecondary, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(13), "Host rules")
							lbl.Font.Weight = font.Bold
							return lbl.Layout(gtx)
						}),
						layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := material.Label(ui.Theme, unit.Sp(11), "Apply DoH lookup and/or per-host delay for matching upstream hosts (exact match).")
							lbl.Color = theme.FgMuted
							return lbl.Layout(gtx)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if st.RuleBanner == "" {
								return layout.Dimensions{}
							}
							lbl := material.Label(ui.Theme, unit.Sp(11), st.RuleBanner)
							col := theme.FgMuted
							switch {
							case strings.HasPrefix(st.RuleBanner, "Rule added"), strings.HasPrefix(st.RuleBanner, "Rule updated"):
								col = theme.VarFound
							case strings.HasPrefix(st.RuleBanner, "Rule removed"):
								col = theme.FgMuted
							case strings.Contains(st.RuleBanner, "invalid"), strings.Contains(st.RuleBanner, "empty"):
								col = theme.Danger
							}
							lbl.Color = col
							return lbl.Layout(gtx)
						}),
					)
				}),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ui.mitmRulesEditorRow(gtx) }),
				layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return ui.mitmRulesTable(gtx, rules)
				}),
			)
		})
	})
}

func (ui *AppUI) mitmRulesEditorRow(gtx layout.Context) layout.Dimensions {
	st := &ui.MITM
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(ui.Theme, &st.RuleHostInput, "host (e.g. example.com)")
					ed.TextSize = unit.Sp(12)
					ed.HintColor = theme.FgMuted
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(6)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Dp(unit.Dp(110))
			gtx.Constraints.Max.X = gtx.Constraints.Min.X
			return mitmBoxed(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: unit.Dp(4), Bottom: unit.Dp(4), Left: unit.Dp(6), Right: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					ed := material.Editor(ui.Theme, &st.RuleTimeoutInput, "timeout ms")
					ed.TextSize = unit.Sp(12)
					ed.HintColor = theme.FgMuted
					return ed.Layout(gtx)
				})
			})
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			sw := material.Switch(ui.Theme, &st.RuleDoHCheck, "")
			return sw.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(ui.Theme, unit.Sp(11), "DoH")
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		}),
		layout.Rigid(layout.Spacer{Width: unit.Dp(10)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return mitmBtn(gtx, ui.Theme, &st.RuleAddBtn, "Add / update", nil, theme.Accent, ui.Theme.ContrastFg, true)
		}),
	)
}

func (ui *AppUI) mitmRulesTable(gtx layout.Context, rules []mitm.HostRuleEntry) layout.Dimensions {
	st := &ui.MITM
	st.RulesList.Axis = layout.Vertical

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			paint.FillShape(gtx.Ops, theme.BgDark, clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(22)))}.Op())
			return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					ruleColHeader(ui.Theme, "Host", 0, text.Start),
					ruleColHeader(ui.Theme, "Timeout", 90, text.End),
					ruleColHeader(ui.Theme, "DoH", 50, text.End),
					ruleColHeader(ui.Theme, "", 70, text.End),
				)
			})
		}),
		layout.Rigid(mitmHLine),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(rules) == 0 {
				return layout.Inset{Top: unit.Dp(12), Bottom: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						lbl := material.Label(ui.Theme, unit.Sp(11), "No host rules — traffic passes through unmodified")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					})
				})
			}
			gtx.Constraints.Max.Y = gtx.Dp(unit.Dp(180))
			return material.List(ui.Theme, &st.RulesList).Layout(gtx, len(rules), func(gtx layout.Context, i int) layout.Dimensions {
				r := rules[i]
				clk := st.RuleRowRemove[r.Host]
				return mitmRuleRow(gtx, ui.Theme, r, clk)
			})
		}),
	)
}

func mitmRuleRow(gtx layout.Context, th *material.Theme, r mitm.HostRuleEntry, removeClk *widget.Clickable) layout.Dimensions {
	rowH := gtx.Dp(unit.Dp(24))
	gtx.Constraints.Min.Y = rowH
	return layout.Inset{Top: unit.Dp(3), Bottom: unit.Dp(3), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				lbl := material.Label(th, unit.Sp(11), r.Host)
				lbl.Font.Typeface = widgets.MonoTypeface
				lbl.MaxLines = 1
				return lbl.Layout(gtx)
			}),
			ruleCell(th, fmt.Sprintf("%d ms", r.Delay.Milliseconds()), 90, text.End),
			ruleCell(th, boolMark(r.UseDoH), 50, text.End),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(unit.Dp(70))
				gtx.Constraints.Max.X = gtx.Constraints.Min.X
				if removeClk == nil {
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Min.X, 0)}
				}
				return mitmBtn(gtx, th, removeClk, "Remove", nil, theme.Border, th.Fg, true)
			}),
		)
	})
}

func boolMark(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func ruleColHeader(th *material.Theme, s string, w int, al text.Alignment) layout.FlexChild {
	if w == 0 {
		return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lbl := material.Label(th, unit.Sp(10), s)
			lbl.Color = theme.FgMuted
			lbl.Font.Weight = font.Bold
			lbl.Alignment = al
			lbl.MaxLines = 1
			return lbl.Layout(gtx)
		})
	}
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		lbl := material.Label(th, unit.Sp(10), s)
		lbl.Color = theme.FgMuted
		lbl.Font.Weight = font.Bold
		lbl.Alignment = al
		lbl.MaxLines = 1
		return lbl.Layout(gtx)
	})
}

func ruleCell(th *material.Theme, s string, w int, al text.Alignment) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Dp(unit.Dp(float32(w)))
		gtx.Constraints.Max.X = gtx.Constraints.Min.X
		lbl := material.Label(th, unit.Sp(11), s)
		lbl.Color = theme.FgMuted
		lbl.Alignment = al
		lbl.MaxLines = 1
		lbl.Font.Typeface = widgets.MonoTypeface
		return lbl.Layout(gtx)
	})
}

func (ui *AppUI) mitmAddRuleFromForm() {
	st := &ui.MITM
	host := strings.TrimSpace(st.RuleHostInput.Text())
	if host == "" {
		st.RuleBanner = "Host is empty"
		return
	}
	timeoutText := strings.TrimSpace(st.RuleTimeoutInput.Text())
	var ms int64
	if timeoutText != "" {
		v, err := strconv.ParseInt(timeoutText, 10, 64)
		if err != nil || v < 0 {
			st.RuleBanner = "Timeout invalid: must be a non-negative integer (ms)"
			return
		}
		ms = v
	}
	rule := mitm.HostRule{
		Delay:  time.Duration(ms) * time.Millisecond,
		UseDoH: st.RuleDoHCheck.Value,
	}
	_, existed := st.Proxy.Rules.Get(host)
	st.Proxy.Rules.Set(host, rule)
	if existed {
		st.RuleBanner = "Rule updated: " + host
	} else {
		st.RuleBanner = "Rule added: " + host
	}
	st.RuleHostInput.SetText("")
	st.RuleTimeoutInput.SetText("")
	st.RuleDoHCheck.Value = false
}
