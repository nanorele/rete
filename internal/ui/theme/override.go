package theme

import (
	"image/color"

	"tracto/internal/model"
)

type PaletteColorEntry struct {
	Label   string
	GetBase func(p Palette) color.NRGBA
	GetOv   func(o model.ThemeColorOverride) string
	SetOv   func(o *model.ThemeColorOverride, hex string)
}

var PaletteColorTable = []PaletteColorEntry{
	{Label: "Background", GetBase: func(p Palette) color.NRGBA { return p.Bg }, GetOv: func(o model.ThemeColorOverride) string { return o.Bg }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Bg = h }},
	{Label: "Background — dark (sidebar)", GetBase: func(p Palette) color.NRGBA { return p.BgDark }, GetOv: func(o model.ThemeColorOverride) string { return o.BgDark }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgDark = h }},
	{Label: "Background — field (input)", GetBase: func(p Palette) color.NRGBA { return p.BgField }, GetOv: func(o model.ThemeColorOverride) string { return o.BgField }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgField = h }},
	{Label: "Background — menu", GetBase: func(p Palette) color.NRGBA { return p.BgMenu }, GetOv: func(o model.ThemeColorOverride) string { return o.BgMenu }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgMenu = h }},
	{Label: "Background — popup", GetBase: func(p Palette) color.NRGBA { return p.BgPopup }, GetOv: func(o model.ThemeColorOverride) string { return o.BgPopup }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgPopup = h }},
	{Label: "Background — hover", GetBase: func(p Palette) color.NRGBA { return p.BgHover }, GetOv: func(o model.ThemeColorOverride) string { return o.BgHover }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgHover = h }},
	{Label: "Background — secondary", GetBase: func(p Palette) color.NRGBA { return p.BgSecondary }, GetOv: func(o model.ThemeColorOverride) string { return o.BgSecondary }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgSecondary = h }},
	{Label: "Background — load more", GetBase: func(p Palette) color.NRGBA { return p.BgLoadMore }, GetOv: func(o model.ThemeColorOverride) string { return o.BgLoadMore }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgLoadMore = h }},
	{Label: "Background — drag holder", GetBase: func(p Palette) color.NRGBA { return p.BgDragHolder }, GetOv: func(o model.ThemeColorOverride) string { return o.BgDragHolder }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgDragHolder = h }},
	{Label: "Background — drag ghost", GetBase: func(p Palette) color.NRGBA { return p.BgDragGhost }, GetOv: func(o model.ThemeColorOverride) string { return o.BgDragGhost }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BgDragGhost = h }},
	{Label: "Border", GetBase: func(p Palette) color.NRGBA { return p.Border }, GetOv: func(o model.ThemeColorOverride) string { return o.Border }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Border = h }},
	{Label: "Border — light", GetBase: func(p Palette) color.NRGBA { return p.BorderLight }, GetOv: func(o model.ThemeColorOverride) string { return o.BorderLight }, SetOv: func(o *model.ThemeColorOverride, h string) { o.BorderLight = h }},
	{Label: "Foreground (text)", GetBase: func(p Palette) color.NRGBA { return p.Fg }, GetOv: func(o model.ThemeColorOverride) string { return o.Fg }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Fg = h }},
	{Label: "Foreground — muted", GetBase: func(p Palette) color.NRGBA { return p.FgMuted }, GetOv: func(o model.ThemeColorOverride) string { return o.FgMuted }, SetOv: func(o *model.ThemeColorOverride, h string) { o.FgMuted = h }},
	{Label: "Foreground — dim", GetBase: func(p Palette) color.NRGBA { return p.FgDim }, GetOv: func(o model.ThemeColorOverride) string { return o.FgDim }, SetOv: func(o *model.ThemeColorOverride, h string) { o.FgDim = h }},
	{Label: "Foreground — hint", GetBase: func(p Palette) color.NRGBA { return p.FgHint }, GetOv: func(o model.ThemeColorOverride) string { return o.FgHint }, SetOv: func(o *model.ThemeColorOverride, h string) { o.FgHint = h }},
	{Label: "Foreground — disabled", GetBase: func(p Palette) color.NRGBA { return p.FgDisabled }, GetOv: func(o model.ThemeColorOverride) string { return o.FgDisabled }, SetOv: func(o *model.ThemeColorOverride, h string) { o.FgDisabled = h }},
	{Label: "White / contrast", GetBase: func(p Palette) color.NRGBA { return p.White }, GetOv: func(o model.ThemeColorOverride) string { return o.White }, SetOv: func(o *model.ThemeColorOverride, h string) { o.White = h }},
	{Label: "Accent", GetBase: func(p Palette) color.NRGBA { return p.Accent }, GetOv: func(o model.ThemeColorOverride) string { return o.Accent }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Accent = h }},
	{Label: "Accent — hover", GetBase: func(p Palette) color.NRGBA { return p.AccentHover }, GetOv: func(o model.ThemeColorOverride) string { return o.AccentHover }, SetOv: func(o *model.ThemeColorOverride, h string) { o.AccentHover = h }},
	{Label: "Accent — dim", GetBase: func(p Palette) color.NRGBA { return p.AccentDim }, GetOv: func(o model.ThemeColorOverride) string { return o.AccentDim }, SetOv: func(o *model.ThemeColorOverride, h string) { o.AccentDim = h }},
	{Label: "Accent — fg", GetBase: func(p Palette) color.NRGBA { return p.AccentFg }, GetOv: func(o model.ThemeColorOverride) string { return o.AccentFg }, SetOv: func(o *model.ThemeColorOverride, h string) { o.AccentFg = h }},
	{Label: "Danger", GetBase: func(p Palette) color.NRGBA { return p.Danger }, GetOv: func(o model.ThemeColorOverride) string { return o.Danger }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Danger = h }},
	{Label: "Danger — fg", GetBase: func(p Palette) color.NRGBA { return p.DangerFg }, GetOv: func(o model.ThemeColorOverride) string { return o.DangerFg }, SetOv: func(o *model.ThemeColorOverride, h string) { o.DangerFg = h }},
	{Label: "Cancel", GetBase: func(p Palette) color.NRGBA { return p.Cancel }, GetOv: func(o model.ThemeColorOverride) string { return o.Cancel }, SetOv: func(o *model.ThemeColorOverride, h string) { o.Cancel = h }},
	{Label: "Close — hover", GetBase: func(p Palette) color.NRGBA { return p.CloseHover }, GetOv: func(o model.ThemeColorOverride) string { return o.CloseHover }, SetOv: func(o *model.ThemeColorOverride, h string) { o.CloseHover = h }},
	{Label: "Scroll thumb", GetBase: func(p Palette) color.NRGBA { return p.ScrollThumb }, GetOv: func(o model.ThemeColorOverride) string { return o.ScrollThumb }, SetOv: func(o *model.ThemeColorOverride, h string) { o.ScrollThumb = h }},
	{Label: "Variable found bg", GetBase: func(p Palette) color.NRGBA { return p.VarFound }, GetOv: func(o model.ThemeColorOverride) string { return o.VarFound }, SetOv: func(o *model.ThemeColorOverride, h string) { o.VarFound = h }},
	{Label: "Variable missing bg", GetBase: func(p Palette) color.NRGBA { return p.VarMissing }, GetOv: func(o model.ThemeColorOverride) string { return o.VarMissing }, SetOv: func(o *model.ThemeColorOverride, h string) { o.VarMissing = h }},
	{Label: "Divider — light", GetBase: func(p Palette) color.NRGBA { return p.DividerLight }, GetOv: func(o model.ThemeColorOverride) string { return o.DividerLight }, SetOv: func(o *model.ThemeColorOverride, h string) { o.DividerLight = h }},
}

func ApplyOverride(p Palette, ov model.ThemeColorOverride) Palette {
	if c, ok := ParseHex(ov.Bg); ok {
		p.Bg = c
	}
	if c, ok := ParseHex(ov.BgDark); ok {
		p.BgDark = c
	}
	if c, ok := ParseHex(ov.BgField); ok {
		p.BgField = c
	}
	if c, ok := ParseHex(ov.BgMenu); ok {
		p.BgMenu = c
	}
	if c, ok := ParseHex(ov.BgPopup); ok {
		p.BgPopup = c
	}
	if c, ok := ParseHex(ov.BgHover); ok {
		p.BgHover = c
	}
	if c, ok := ParseHex(ov.BgSecondary); ok {
		p.BgSecondary = c
	}
	if c, ok := ParseHex(ov.BgLoadMore); ok {
		p.BgLoadMore = c
	}
	if c, ok := ParseHex(ov.BgDragHolder); ok {
		p.BgDragHolder = c
	}
	if c, ok := ParseHex(ov.BgDragGhost); ok {
		p.BgDragGhost = c
	}
	if c, ok := ParseHex(ov.Border); ok {
		p.Border = c
	}
	if c, ok := ParseHex(ov.BorderLight); ok {
		p.BorderLight = c
	}
	if c, ok := ParseHex(ov.Fg); ok {
		p.Fg = c
	}
	if c, ok := ParseHex(ov.FgMuted); ok {
		p.FgMuted = c
	}
	if c, ok := ParseHex(ov.FgDim); ok {
		p.FgDim = c
	}
	if c, ok := ParseHex(ov.FgHint); ok {
		p.FgHint = c
	}
	if c, ok := ParseHex(ov.FgDisabled); ok {
		p.FgDisabled = c
	}
	if c, ok := ParseHex(ov.White); ok {
		p.White = c
	}
	if c, ok := ParseHex(ov.Accent); ok {
		p.Accent = c
	}
	if c, ok := ParseHex(ov.AccentHover); ok {
		p.AccentHover = c
	}
	if c, ok := ParseHex(ov.AccentDim); ok {
		p.AccentDim = c
	}
	if c, ok := ParseHex(ov.AccentFg); ok {
		p.AccentFg = c
	}
	if c, ok := ParseHex(ov.Danger); ok {
		p.Danger = c
	}
	if c, ok := ParseHex(ov.DangerFg); ok {
		p.DangerFg = c
	}
	if c, ok := ParseHex(ov.Cancel); ok {
		p.Cancel = c
	}
	if c, ok := ParseHex(ov.CloseHover); ok {
		p.CloseHover = c
	}
	if c, ok := ParseHex(ov.ScrollThumb); ok {
		p.ScrollThumb = c
	}
	if c, ok := ParseHex(ov.VarFound); ok {
		p.VarFound = c
	}
	if c, ok := ParseHex(ov.VarMissing); ok {
		p.VarMissing = c
	}
	if c, ok := ParseHex(ov.DividerLight); ok {
		p.DividerLight = c
	}
	return p
}

type TokenColorEntry struct {
	Label   string
	GetBase func(s SyntaxPalette) color.NRGBA
	GetOv   func(o model.ThemeSyntaxOverride) string
	SetOv   func(o *model.ThemeSyntaxOverride, hex string)
}

var TokenColorTable = []TokenColorEntry{
	{
		Label:   "Plain text",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Plain },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Plain },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Plain = h },
	},
	{
		Label:   "String",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.String },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.String },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.String = h },
	},
	{
		Label:   "Number",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Number },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Number },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Number = h },
	},
	{
		Label:   "Boolean",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Bool },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Bool },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Bool = h },
	},
	{
		Label:   "Null",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Null },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Null },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Null = h },
	},
	{
		Label:   "Property / Key",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Key },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Key },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Key = h },
	},
	{
		Label:   "Punctuation",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Punctuation },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Punctuation },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Punctuation = h },
	},
	{
		Label:   "Operator",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Operator },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Operator },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Operator = h },
	},
	{
		Label:   "Keyword",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Keyword },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Keyword },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Keyword = h },
	},
	{
		Label:   "Type",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Type },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Type },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Type = h },
	},
	{
		Label:   "Comment",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Comment },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Comment },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Comment = h },
	},
	{
		Label:   "Bracket 1 (depth 0)",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Brackets[0] },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Bracket0 },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Bracket0 = h },
	},
	{
		Label:   "Bracket 2 (depth 1)",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Brackets[1] },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Bracket1 },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Bracket1 = h },
	},
	{
		Label:   "Bracket 3 (depth 2)",
		GetBase: func(s SyntaxPalette) color.NRGBA { return s.Brackets[2] },
		GetOv:   func(o model.ThemeSyntaxOverride) string { return o.Bracket2 },
		SetOv:   func(o *model.ThemeSyntaxOverride, h string) { o.Bracket2 = h },
	},
}

func ApplySyntaxOverride(base SyntaxPalette, ov model.ThemeSyntaxOverride) SyntaxPalette {
	if c, ok := ParseHex(ov.Plain); ok {
		base.Plain = c
	}
	if c, ok := ParseHex(ov.String); ok {
		base.String = c
	}
	if c, ok := ParseHex(ov.Number); ok {
		base.Number = c
	}
	if c, ok := ParseHex(ov.Bool); ok {
		base.Bool = c
	}
	if c, ok := ParseHex(ov.Null); ok {
		base.Null = c
	}
	if c, ok := ParseHex(ov.Key); ok {
		base.Key = c
	}
	if c, ok := ParseHex(ov.Punctuation); ok {
		base.Punctuation = c
	}
	if c, ok := ParseHex(ov.Operator); ok {
		base.Operator = c
	}
	if c, ok := ParseHex(ov.Keyword); ok {
		base.Keyword = c
	}
	if c, ok := ParseHex(ov.Type); ok {
		base.Type = c
	}
	if c, ok := ParseHex(ov.Comment); ok {
		base.Comment = c
	}
	if c, ok := ParseHex(ov.Bracket0); ok {
		base.Brackets[0] = c
	}
	if c, ok := ParseHex(ov.Bracket1); ok {
		base.Brackets[1] = c
	}
	if c, ok := ParseHex(ov.Bracket2); ok {
		base.Brackets[2] = c
	}
	return base
}
