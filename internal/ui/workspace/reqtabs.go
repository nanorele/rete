package workspace

import (
	"encoding/base64"
	"image"
	"strings"

	"tracto/internal/model"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/font"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/op/paint"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget"
	"github.com/nanorele/gio/widget/material"
)

const (
	reqSubHeaders = iota
	reqSubParams
	reqSubAuth
	reqSubCookies
)

const (
	authNone = iota
	authBearer
	authBasic
)

var authTypeNames = [3]string{"No Auth", "Bearer Token", "Basic Auth"}

func authTypeToModel(i int) string {
	switch i {
	case authBearer:
		return "bearer"
	case authBasic:
		return "basic"
	default:
		return ""
	}
}

func authTypeFromModel(s string) int {
	switch s {
	case "bearer":
		return authBearer
	case "basic":
		return authBasic
	default:
		return authNone
	}
}

func (t *RequestTab) AuthModel() model.ParsedAuth {
	return model.ParsedAuth{
		Type:     authTypeToModel(t.AuthType),
		Token:    t.AuthToken.Text(),
		Username: t.AuthUser.Text(),
		Password: t.AuthPass.Text(),
	}
}

func (t *RequestTab) ApplyAuth(a model.ParsedAuth) {
	t.AuthType = authTypeFromModel(a.Type)
	t.AuthToken.SetText(a.Token)
	t.AuthUser.SetText(a.Username)
	t.AuthPass.SetText(a.Password)
}

func (t *RequestTab) CookieModels() []model.ParsedKV {
	var out []model.ParsedKV
	for _, c := range t.Cookies {
		if c.Key.Text() == "" {
			continue
		}
		out = append(out, model.ParsedKV{Key: c.Key.Text(), Value: c.Value.Text()})
	}
	return out
}

func (t *RequestTab) ApplyCookies(cs []model.ParsedKV) {
	t.Cookies = t.Cookies[:0]
	for _, c := range cs {
		t.addCookie(c.Key, c.Value)
	}
}

func (t *RequestTab) addParam(k, v string) {
	it := &HeaderItem{}
	it.Key.SetText(k)
	it.Value.SetText(v)
	t.Params = append(t.Params, it)
}

func (t *RequestTab) addCookie(k, v string) {
	it := &HeaderItem{}
	it.Key.SetText(k)
	it.Value.SetText(v)
	t.Cookies = append(t.Cookies, it)
}

func (t *RequestTab) activeKVItems() []*HeaderItem {
	switch t.ReqSubTab {
	case reqSubParams:
		return t.Params
	case reqSubCookies:
		return t.Cookies
	default:
		return t.Headers
	}
}

func (t *RequestTab) activeKVList() *widget.List {
	switch t.ReqSubTab {
	case reqSubParams:
		return &t.ParamsList
	case reqSubCookies:
		return &t.CookiesList
	default:
		return &t.HeadersList
	}
}

func splitURLQuery(u string) (base, query, frag string) {
	if i := strings.IndexByte(u, '#'); i >= 0 {
		frag = u[i:]
		u = u[:i]
	}
	if i := strings.IndexByte(u, '?'); i >= 0 {
		return u[:i], u[i+1:], frag
	}
	return u, "", frag
}

func (t *RequestTab) syncParamsFromURL() {
	u := t.URLInput.Text()
	_, query, _ := splitURLQuery(u)
	t.Params = t.Params[:0]
	if query != "" {
		for _, pair := range strings.Split(query, "&") {
			if pair == "" {
				continue
			}
			k, v := pair, ""
			if eq := strings.IndexByte(pair, '='); eq >= 0 {
				k, v = pair[:eq], pair[eq+1:]
			}
			t.addParam(k, v)
		}
	}
	t.paramsSynced = u
}

func (t *RequestTab) syncURLFromParams() {
	base, _, frag := splitURLQuery(t.URLInput.Text())
	var parts []string
	for _, p := range t.Params {
		k := p.Key.Text()
		v := p.Value.Text()
		if k == "" && v == "" {
			continue
		}
		parts = append(parts, k+"="+v)
	}
	newURL := base
	if len(parts) > 0 {
		newURL += "?" + strings.Join(parts, "&")
	}
	newURL += frag
	if newURL != t.URLInput.Text() {
		t.URLInput.SetText(newURL)
	}
	t.paramsSynced = newURL
}

func (t *RequestTab) authHeaderValue(env map[string]string) string {
	switch t.AuthType {
	case authBearer:
		tok := strings.TrimSpace(processTemplate(t.AuthToken.Text(), env))
		if tok == "" {
			return ""
		}
		return "Bearer " + tok
	case authBasic:
		u := processTemplate(t.AuthUser.Text(), env)
		p := processTemplate(t.AuthPass.Text(), env)
		if u == "" && p == "" {
			return ""
		}
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(u+":"+p))
	}
	return ""
}

func (t *RequestTab) cookieHeaderValue(env map[string]string) string {
	var parts []string
	for _, c := range t.Cookies {
		k := strings.TrimSpace(processTemplate(c.Key.Text(), env))
		if k == "" {
			continue
		}
		v := strings.TrimSpace(processTemplate(c.Value.Text(), env))
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "; ")
}

func pumpKVEvents(gtx layout.Context, items []*HeaderItem, onChange func()) {
	for _, it := range items {
		for {
			ev, ok := it.Key.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok && onChange != nil {
				onChange()
			}
		}
		for {
			ev, ok := it.Value.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok && onChange != nil {
				onChange()
			}
		}
	}
}

func (t *RequestTab) updateReqSubTabs(gtx layout.Context) {
	selectTab := func(sub int) {
		prev := 0
		if t.HeadersExpanded {
			prev = t.HeadersAbsHeight
		}
		t.ReqSubTab = sub
		t.HeadersExpanded = true
		t.fitPrevHeadersDp = prev
		t.fitHeadersExact = true
	}
	for t.HeadersTabBtn.Clicked(gtx) {
		selectTab(reqSubHeaders)
	}
	for t.ParamsTabBtn.Clicked(gtx) {
		selectTab(reqSubParams)
	}
	for t.AuthTabBtn.Clicked(gtx) {
		selectTab(reqSubAuth)
	}
	for t.CookiesTabBtn.Clicked(gtx) {
		selectTab(reqSubCookies)
	}

	for i := 0; i < len(t.Params); i++ {
		if t.Params[i].DelBtn.Clicked(gtx) {
			t.Params = append(t.Params[:i], t.Params[i+1:]...)
			i--
			t.syncURLFromParams()
		}
	}
	for i := 0; i < len(t.Cookies); i++ {
		if t.Cookies[i].DelBtn.Clicked(gtx) {
			t.Cookies = append(t.Cookies[:i], t.Cookies[i+1:]...)
			i--
			t.dirtyCheckNeeded = true
		}
	}

	pumpKVEvents(gtx, t.Params, t.syncURLFromParams)
	pumpKVEvents(gtx, t.Cookies, func() { t.dirtyCheckNeeded = true })

	if u := t.URLInput.Text(); u != t.paramsSynced {
		t.syncParamsFromURL()
		pumpKVEvents(gtx, t.Params, nil)
	}

	for t.AuthTypeBtn.Clicked(gtx) {
		t.AuthTypeOpen = !t.AuthTypeOpen
	}
	for i := range t.AuthTypeChoices {
		for t.AuthTypeChoices[i].Clicked(gtx) {
			t.AuthType = i
			t.AuthTypeOpen = false
			prev := 0
			if t.HeadersExpanded {
				prev = t.HeadersAbsHeight
			}
			t.fitPrevHeadersDp = prev
			t.fitHeadersExact = true
			t.dirtyCheckNeeded = true
		}
	}
	for _, ed := range []*widget.Editor{&t.AuthToken, &t.AuthUser, &t.AuthPass} {
		for {
			ev, ok := ed.Update(gtx)
			if !ok {
				break
			}
			if _, ok := ev.(widget.ChangeEvent); ok {
				t.dirtyCheckNeeded = true
			}
		}
	}
}

func (t *RequestTab) reqTab(gtx layout.Context, th *material.Theme, clk *widget.Clickable, label string, active bool) layout.Dimensions {
	return material.Clickable(gtx, clk, func(gtx layout.Context) layout.Dimensions {
		if !active {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		return layout.Inset{Top: unit.Dp(2), Bottom: unit.Dp(2), Left: unit.Dp(5), Right: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			lbl := widgets.MonoLabel(th, unit.Sp(12), label)
			lbl.MaxLines = 1
			if active {
				lbl.Font.Weight = font.Bold
				lbl.Color = theme.Fg
			} else {
				lbl.Color = theme.FgMuted
			}
			return lbl.Layout(gtx)
		})
	})
}

func reqTabSeparator(gtx layout.Context, th *material.Theme) layout.Dimensions {
	lbl := widgets.MonoLabel(th, unit.Sp(12), "|")
	lbl.Color = theme.Border
	return lbl.Layout(gtx)
}

func (t *RequestTab) reqTabsChildren(gtx layout.Context, th *material.Theme) []layout.FlexChild {
	sep := layout.Rigid(func(gtx layout.Context) layout.Dimensions { return reqTabSeparator(gtx, th) })
	return []layout.FlexChild{
		layout.Rigid(layout.Spacer{Width: unit.Dp(4)}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return t.reqTab(gtx, th, &t.HeadersTabBtn, "Headers", t.ReqSubTab == reqSubHeaders)
		}),
		sep,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return t.reqTab(gtx, th, &t.ParamsTabBtn, "Params", t.ReqSubTab == reqSubParams)
		}),
		sep,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return t.reqTab(gtx, th, &t.AuthTabBtn, "Auth", t.ReqSubTab == reqSubAuth)
		}),
		sep,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return t.reqTab(gtx, th, &t.CookiesTabBtn, "Cookies", t.ReqSubTab == reqSubCookies)
		}),
	}
}

func (t *RequestTab) layoutAuthPanel(gtx layout.Context, th *material.Theme, env map[string]string) layout.Dimensions {
	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						lbl := widgets.MonoLabel(th, unit.Sp(11), "Type")
						lbl.Color = theme.FgMuted
						return lbl.Layout(gtx)
					}),
					layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return t.layoutAuthTypeSelector(gtx, th)
					}),
				)
			}),
			layout.Rigid(layout.Spacer{Height: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				switch t.AuthType {
				case authBearer:
					return t.authField(gtx, th, "Token", &t.AuthToken, env)
				case authBasic:
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return t.authField(gtx, th, "Username", &t.AuthUser, env)
						}),
						layout.Rigid(layout.Spacer{Height: unit.Dp(6)}.Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return t.authField(gtx, th, "Password", &t.AuthPass, env)
						}),
					)
				default:
					lbl := widgets.MonoLabel(th, unit.Sp(11), "This request does not use authorization.")
					lbl.Color = theme.FgMuted
					return lbl.Layout(gtx)
				}
			}),
		)
	})
}

func (t *RequestTab) authField(gtx layout.Context, th *material.Theme, label string, ed *widget.Editor, env map[string]string) layout.Dimensions {
	labelW := gtx.Dp(unit.Dp(76))
	fieldH := gtx.Dp(unit.Dp(26))
	return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = labelW
			gtx.Constraints.Max.X = labelW
			lbl := widgets.MonoLabel(th, unit.Sp(11), label)
			lbl.Color = theme.FgMuted
			return lbl.Layout(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = fieldH
			gtx.Constraints.Max.Y = fieldH
			return widget.Border{Color: theme.Border, CornerRadius: unit.Dp(2), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return widgets.TextFieldOverlayBg(gtx, th, ed, label, false, env, 0, unit.Sp(11), widgets.KVSurface())
			})
		}),
	)
}

func (t *RequestTab) layoutAuthTypeSelector(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return layout.Stack{Alignment: layout.NW}.Layout(gtx,
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			if !t.AuthTypeOpen {
				return layout.Dimensions{}
			}
			items := make([]widgets.MenuItem, len(authTypeNames))
			for i, name := range authTypeNames {
				items[i] = widgets.MenuItem{
					Label:   name,
					Click:   &t.AuthTypeChoices[i],
					Checked: t.AuthType == i,
					Mono:    true,
				}
			}
			anchor := widgets.MenuAnchor{Pt: image.Pt(0, gtx.Dp(unit.Dp(26)))}
			widgets.DeferMenuAt(gtx, th, &t.AuthTypeOpen, anchor, 160, items)
			return layout.Dimensions{}
		}),
		layout.Stacked(func(gtx layout.Context) layout.Dimensions {
			return material.Clickable(gtx, &t.AuthTypeBtn, func(gtx layout.Context) layout.Dimensions {
				pointer.CursorPointer.Add(gtx.Ops)
				macro := op.Record(gtx.Ops)
				dim := layout.Inset{Top: unit.Dp(5), Bottom: unit.Dp(5), Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							lbl := widgets.MonoLabel(th, unit.Sp(11), authTypeNames[t.AuthType])
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
				border := widget.Border{Color: theme.Border, Width: unit.Dp(1)}
				return border.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if t.AuthTypeBtn.Hovered() {
						paint.FillShape(gtx.Ops, theme.BgHover, clip.Rect{Max: dim.Size}.Op())
					}
					call.Add(gtx.Ops)
					return dim
				})
			})
		}),
	)
}
