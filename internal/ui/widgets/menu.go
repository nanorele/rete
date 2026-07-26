package widgets

import (
	"image"

	"tracto/internal/ui/theme"

	"github.com/nanorele/gio-x/component"

	"github.com/nanorele/gio/io/event"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget/material"
)

const (
	MenuRadiusDp    = component.MenuRadiusDp
	MenuRowRadiusDp = component.MenuRowRadiusDp
	MenuBorderDp    = component.MenuBorderDp
	MenuListPadDp   = component.MenuListPadDp
	MenuRowPadVDp   = component.MenuRowPadVDp
	MenuRowPadHDp   = component.MenuRowPadHDp
	MenuGutterDp    = component.MenuGutterDp
	MenuMinWidthDp  = component.MenuMinWidthDp
)

type MenuItem = component.MenuEntry

type MenuAnchor = component.MenuAnchor

func menuColors(th *material.Theme) component.MenuColors {
	fg := theme.FgMuted
	if th != nil {
		fg = th.Fg
	}
	return component.MenuColors{
		Surface:  theme.BgMenu,
		Border:   MenuBorderColor(),
		Hover:    theme.BgHover,
		Divider:  theme.DividerLight,
		Fg:       fg,
		Muted:    theme.FgMuted,
		Disabled: theme.FgDisabled,
		Danger:   theme.Danger,
	}
}

func menuStyle(th *material.Theme, minWidthDp int) menuStyler {
	return menuStyler{component.AnchoredMenu{
		Theme:      th,
		MinWidthDp: minWidthDp,
		MonoFace:   MonoTypeface,
		CheckIcon:  IconCheck,
		Colors:     menuColors(th),
	}}
}

func MenuShadow(gtx layout.Context, sz image.Point) {
	component.MenuShadow(gtx, sz)
}

func MenuSurface(gtx layout.Context, tag event.Tag, minWidthDp int, content layout.Widget) layout.Dimensions {
	return menuStyle(nil, minWidthDp).surface(gtx, tag, content)
}

func MenuList(gtx layout.Context, th *material.Theme, tag event.Tag, minWidthDp int, items []MenuItem) layout.Dimensions {
	return menuStyle(th, minWidthDp).list(gtx, tag, items)
}

func DeferMenuSurfaceAt(gtx layout.Context, tag event.Tag, anchor MenuAnchor, minWidthDp int, content layout.Widget) layout.Dimensions {
	return menuStyle(nil, minWidthDp).deferSurfaceAt(gtx, tag, anchor, content)
}

func DeferMenuAt(gtx layout.Context, th *material.Theme, tag event.Tag, anchor MenuAnchor, minWidthDp int, items []MenuItem) layout.Dimensions {
	return menuStyle(th, minWidthDp).deferAt(gtx, tag, anchor, items)
}

func DeferMenu(gtx layout.Context, th *material.Theme, tag event.Tag, anchor image.Point, minWidthDp int, items []MenuItem) layout.Dimensions {
	return DeferMenuAt(gtx, th, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, items)
}

func DeferMenuSurface(gtx layout.Context, tag event.Tag, anchor image.Point, minWidthDp int, content layout.Widget) layout.Dimensions {
	return DeferMenuSurfaceAt(gtx, tag, MenuAnchor{Pt: anchor, Clamp: gtx.Constraints.Max}, minWidthDp, content)
}

func MenuRow(gtx layout.Context, th *material.Theme, it MenuItem) layout.Dimensions {
	return menuStyle(th, MenuMinWidthDp).Row(gtx, it)
}
