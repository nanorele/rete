package theme

import "image/color"

var (
	Bg             = color.NRGBA{R: 31, G: 31, B: 31, A: 255}
	BgDark         = color.NRGBA{R: 24, G: 24, B: 24, A: 255}
	BgField        = color.NRGBA{R: 49, G: 49, B: 49, A: 255}
	BgMenu         = color.NRGBA{R: 37, G: 37, B: 38, A: 255}
	BgPopup        = color.NRGBA{R: 35, G: 35, B: 35, A: 255}
	BgHover        = color.NRGBA{R: 42, G: 45, B: 46, A: 255}
	BgSecondary    = color.NRGBA{R: 55, G: 55, B: 55, A: 255}
	BgLoadMore     = color.NRGBA{R: 50, G: 50, B: 50, A: 255}
	BgDragHolder   = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	BgDragGhost    = color.NRGBA{R: 31, G: 31, B: 31, A: 240}
	Border         = color.NRGBA{R: 43, G: 45, B: 49, A: 255}
	BorderLight    = color.NRGBA{R: 60, G: 60, B: 60, A: 255}
	Fg             = color.NRGBA{R: 204, G: 204, B: 204, A: 255}
	FgMuted        = color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	FgDim          = color.NRGBA{R: 140, G: 140, B: 140, A: 255}
	FgHint         = color.NRGBA{R: 170, G: 170, B: 170, A: 255}
	FgDisabled     = color.NRGBA{R: 80, G: 80, B: 80, A: 255}
	White          = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	Accent         = color.NRGBA{R: 14, G: 99, B: 156, A: 255}
	AccentHover    = color.NRGBA{R: 20, G: 120, B: 180, A: 255}
	AccentDim      = color.NRGBA{R: 14, G: 99, B: 156, A: 40}
	Selection      = color.NRGBA{R: 14, G: 99, B: 156, A: 96}
	AccentFg       = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	DangerFg       = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	Danger         = color.NRGBA{R: 194, G: 64, B: 56, A: 255}
	Cancel         = color.NRGBA{R: 180, G: 40, B: 40, A: 255}
	CloseHover     = color.NRGBA{R: 232, G: 17, B: 35, A: 255}
	ScrollThumb    = color.NRGBA{R: 75, G: 75, B: 75, A: 255}
	EditorScroll   = color.NRGBA{R: 170, G: 170, B: 170, A: 255}
	VarFound       = color.NRGBA{R: 40, G: 110, B: 160, A: 100}
	VarMissing     = color.NRGBA{R: 130, G: 60, B: 60, A: 100}
	DividerLight   = color.NRGBA{R: 255, G: 255, B: 255, A: 60}
	Transparent    = color.NRGBA{}
	Syntax         SyntaxPalette
	MethodGet      = MethodDark.Get
	MethodPost     = MethodDark.Post
	MethodPut      = MethodDark.Put
	MethodDelete   = MethodDark.Delete
	MethodHead     = MethodDark.Head
	MethodPatch    = MethodDark.Patch
	MethodOptions  = MethodDark.Options
	MethodFallback = MethodDark.Fallback
)

var MethodDark = MethodPalette{
	Get:      color.NRGBA{R: 12, G: 187, B: 82, A: 255},
	Post:     color.NRGBA{R: 255, G: 180, B: 0, A: 255},
	Put:      color.NRGBA{R: 9, G: 123, B: 237, A: 255},
	Delete:   color.NRGBA{R: 235, G: 32, B: 19, A: 255},
	Head:     color.NRGBA{R: 217, G: 90, B: 165, A: 255},
	Patch:    color.NRGBA{R: 186, G: 85, B: 211, A: 255},
	Options:  color.NRGBA{R: 13, G: 184, B: 214, A: 255},
	Fallback: color.NRGBA{R: 150, G: 150, B: 150, A: 255},
}

var MethodLight = MethodPalette{
	Get:      color.NRGBA{R: 38, G: 138, B: 70, A: 255},
	Post:     color.NRGBA{R: 200, G: 130, B: 0, A: 255},
	Put:      color.NRGBA{R: 9, G: 105, B: 180, A: 255},
	Delete:   color.NRGBA{R: 200, G: 30, B: 30, A: 255},
	Head:     color.NRGBA{R: 180, G: 60, B: 130, A: 255},
	Patch:    color.NRGBA{R: 140, G: 60, B: 170, A: 255},
	Options:  color.NRGBA{R: 15, G: 130, B: 160, A: 255},
	Fallback: color.NRGBA{R: 100, G: 100, B: 100, A: 255},
}

func ApplyMethod(p MethodPalette) {
	MethodGet = p.Get
	MethodPost = p.Post
	MethodPut = p.Put
	MethodDelete = p.Delete
	MethodHead = p.Head
	MethodPatch = p.Patch
	MethodOptions = p.Options
	MethodFallback = p.Fallback
}

func MethodFor(bg color.NRGBA) MethodPalette {
	if RelLuminance(bg) > 0.45 {
		return MethodLight
	}
	return MethodDark
}

func MethodColor(method string) color.NRGBA {
	switch method {
	case "GET":
		return MethodGet
	case "POST":
		return MethodPost
	case "PUT":
		return MethodPut
	case "DELETE":
		return MethodDelete
	case "HEAD":
		return MethodHead
	case "PATCH":
		return MethodPatch
	case "OPTIONS":
		return MethodOptions
	default:
		return MethodFallback
	}
}

func Apply(p Palette) {
	Bg = p.Bg
	BgDark = p.BgDark
	BgField = p.BgField
	BgMenu = p.BgMenu
	BgPopup = p.BgPopup
	BgHover = p.BgHover
	BgSecondary = p.BgSecondary
	BgLoadMore = p.BgLoadMore
	BgDragHolder = p.BgDragHolder
	BgDragGhost = p.BgDragGhost
	Border = p.Border
	BorderLight = p.BorderLight
	Fg = p.Fg
	FgMuted = p.FgMuted
	FgDim = p.FgDim
	FgHint = p.FgHint
	FgDisabled = p.FgDisabled
	White = p.White
	Accent = p.Accent
	AccentHover = p.AccentHover
	AccentDim = p.AccentDim
	AccentFg = p.AccentFg
	if (AccentFg == color.NRGBA{}) {
		AccentFg = ContrastOn(Accent)
	}
	Danger = p.Danger
	DangerFg = p.DangerFg
	if (DangerFg == color.NRGBA{}) {
		DangerFg = ContrastOn(Danger)
	}
	Cancel = p.Cancel
	CloseHover = p.CloseHover
	ScrollThumb = p.ScrollThumb
	VarFound = p.VarFound
	VarMissing = p.VarMissing
	DividerLight = p.DividerLight
	Syntax = p.Syntax
	if (Syntax.Plain == color.NRGBA{}) {
		Syntax = DeriveSyntax(p)
	}
	ApplyMethod(MethodFor(p.Bg))
}
