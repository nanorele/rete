package theme

import (
	"image/color"

	"tracto/internal/ui/syntax"
)

type Palette struct {
	Bg           color.NRGBA
	BgDark       color.NRGBA
	BgField      color.NRGBA
	BgMenu       color.NRGBA
	BgPopup      color.NRGBA
	BgHover      color.NRGBA
	BgSecondary  color.NRGBA
	BgLoadMore   color.NRGBA
	BgDragHolder color.NRGBA
	BgDragGhost  color.NRGBA
	Border       color.NRGBA
	BorderLight  color.NRGBA
	Fg           color.NRGBA
	FgMuted      color.NRGBA
	FgDim        color.NRGBA
	FgHint       color.NRGBA
	FgDisabled   color.NRGBA
	White        color.NRGBA
	Accent       color.NRGBA
	AccentHover  color.NRGBA
	AccentDim    color.NRGBA
	AccentFg     color.NRGBA
	Danger       color.NRGBA
	DangerFg     color.NRGBA
	Cancel       color.NRGBA
	CloseHover   color.NRGBA
	ScrollThumb  color.NRGBA
	VarFound     color.NRGBA
	VarMissing   color.NRGBA
	DividerLight color.NRGBA
	Syntax       SyntaxPalette
}

type MethodPalette struct {
	Get      color.NRGBA
	Post     color.NRGBA
	Put      color.NRGBA
	Delete   color.NRGBA
	Head     color.NRGBA
	Patch    color.NRGBA
	Options  color.NRGBA
	Fallback color.NRGBA
}

type SyntaxPalette struct {
	Plain       color.NRGBA
	String      color.NRGBA
	Number      color.NRGBA
	Bool        color.NRGBA
	Null        color.NRGBA
	Key         color.NRGBA
	Punctuation color.NRGBA
	Operator    color.NRGBA
	Keyword     color.NRGBA
	Type        color.NRGBA
	Comment     color.NRGBA
	Brackets    [3]color.NRGBA
}

func (sp SyntaxPalette) ColorForToken(kind syntax.TokenKind, depth uint8, bracketCycle bool) color.NRGBA {
	switch kind {
	case syntax.TokString:
		return sp.String
	case syntax.TokNumber:
		return sp.Number
	case syntax.TokBool:
		return sp.Bool
	case syntax.TokNull:
		return sp.Null
	case syntax.TokKey:
		return sp.Key
	case syntax.TokPunctuation:
		return sp.Punctuation
	case syntax.TokBracket:
		if bracketCycle {
			return sp.Brackets[int(depth)%3]
		}
		return sp.Punctuation
	case syntax.TokOperator:
		return sp.Operator
	case syntax.TokKeyword:
		return sp.Keyword
	case syntax.TokType:
		return sp.Type
	case syntax.TokComment:
		return sp.Comment
	}
	return sp.Plain
}
