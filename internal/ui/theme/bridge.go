package theme

import (
	"tracto/internal/model"
)

func PaletteFor(id string, customs []model.CustomTheme) Palette {
	for _, t := range Registry {
		if t.ID == id {
			return t.Palette
		}
	}
	for _, c := range customs {
		if c.ID == id {
			base := Dark
			for _, t := range Registry {
				if t.ID == c.BasedOn {
					base = t.Palette
					break
				}
			}
			p := ApplyOverride(base, c.Palette)
			p.Syntax = ApplySyntaxOverride(base.Syntax, c.Syntax)
			return p
		}
	}
	return Dark
}

func IsValidID(id string, customs []model.CustomTheme) bool {
	for _, t := range Registry {
		if t.ID == id {
			return true
		}
	}
	for _, c := range customs {
		if c.ID == id {
			return true
		}
	}
	return false
}

func PaletteToOverride(p Palette) model.ThemeColorOverride {
	return model.ThemeColorOverride{
		Bg: HexFromColor(p.Bg), BgDark: HexFromColor(p.BgDark), BgField: HexFromColor(p.BgField),
		BgMenu: HexFromColor(p.BgMenu), BgPopup: HexFromColor(p.BgPopup), BgHover: HexFromColor(p.BgHover),
		BgSecondary: HexFromColor(p.BgSecondary), BgLoadMore: HexFromColor(p.BgLoadMore),
		BgDragHolder: HexFromColor(p.BgDragHolder), BgDragGhost: HexFromColor(p.BgDragGhost),
		Border: HexFromColor(p.Border), BorderLight: HexFromColor(p.BorderLight),
		Fg: HexFromColor(p.Fg), FgMuted: HexFromColor(p.FgMuted), FgDim: HexFromColor(p.FgDim),
		FgHint: HexFromColor(p.FgHint), FgDisabled: HexFromColor(p.FgDisabled),
		White:  HexFromColor(p.White),
		Accent: HexFromColor(p.Accent), AccentHover: HexFromColor(p.AccentHover),
		AccentDim: HexFromColor(p.AccentDim), AccentFg: HexFromColor(p.AccentFg),
		Danger: HexFromColor(p.Danger), DangerFg: HexFromColor(p.DangerFg),
		Cancel: HexFromColor(p.Cancel), CloseHover: HexFromColor(p.CloseHover),
		ScrollThumb: HexFromColor(p.ScrollThumb),
		VarFound:    HexFromColor(p.VarFound), VarMissing: HexFromColor(p.VarMissing),
		DividerLight: HexFromColor(p.DividerLight),
	}
}

func SyntaxToOverride(s SyntaxPalette) model.ThemeSyntaxOverride {
	return model.ThemeSyntaxOverride{
		Plain:       HexFromColor(s.Plain),
		String:      HexFromColor(s.String),
		Number:      HexFromColor(s.Number),
		Bool:        HexFromColor(s.Bool),
		Null:        HexFromColor(s.Null),
		Key:         HexFromColor(s.Key),
		Punctuation: HexFromColor(s.Punctuation),
		Operator:    HexFromColor(s.Operator),
		Keyword:     HexFromColor(s.Keyword),
		Type:        HexFromColor(s.Type),
		Comment:     HexFromColor(s.Comment),
		Bracket0:    HexFromColor(s.Brackets[0]),
		Bracket1:    HexFromColor(s.Brackets[1]),
		Bracket2:    HexFromColor(s.Brackets[2]),
	}
}
