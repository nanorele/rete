package theme

import (
	pkgtheme "tracto/pkg/theme"
)

type (
	Palette       = pkgtheme.Palette
	MethodPalette = pkgtheme.MethodPalette
	SyntaxPalette = pkgtheme.SyntaxPalette
	Def           = pkgtheme.Def
)

var (
	Shade             = pkgtheme.Shade
	Mix               = pkgtheme.Mix
	WithAlpha         = pkgtheme.WithAlpha
	RelLuminance      = pkgtheme.RelLuminance
	SearchFill        = pkgtheme.SearchFill
	GreyAt            = pkgtheme.GreyAt
	ContrastRatio     = pkgtheme.ContrastRatio
	ContrastOn        = pkgtheme.ContrastOn
	BestTextOn        = pkgtheme.BestTextOn
	AdjustForContrast = pkgtheme.AdjustForContrast
	Composite         = pkgtheme.Composite
	ContrastBgFor     = pkgtheme.ContrastBgFor
	ParseHex          = pkgtheme.ParseHex
	HexFromColor      = pkgtheme.HexFromColor
	MakeTheme         = pkgtheme.MakeTheme
	DeriveSyntax      = pkgtheme.DeriveSyntax
	WithSyntax        = pkgtheme.WithSyntax
	Dark              = pkgtheme.Dark
	Light             = pkgtheme.Light
	Registry          = pkgtheme.Registry
)

var (
	DarkPlusSyntax          = pkgtheme.DarkPlusSyntax
	LightPlusSyntax         = pkgtheme.LightPlusSyntax
	MonokaiSyntax           = pkgtheme.MonokaiSyntax
	DraculaSyntax           = pkgtheme.DraculaSyntax
	OneDarkSyntax           = pkgtheme.OneDarkSyntax
	SolarizedDarkSyntax     = pkgtheme.SolarizedDarkSyntax
	SolarizedLightSyntax    = pkgtheme.SolarizedLightSyntax
	GithubDarkSyntax        = pkgtheme.GithubDarkSyntax
	GithubLightSyntax       = pkgtheme.GithubLightSyntax
	MonokaiDimmedSyntax     = pkgtheme.MonokaiDimmedSyntax
	AbyssSyntax             = pkgtheme.AbyssSyntax
	KimbieDarkSyntax        = pkgtheme.KimbieDarkSyntax
	NordSyntax              = pkgtheme.NordSyntax
	TomorrowNightBlueSyntax = pkgtheme.TomorrowNightBlueSyntax
	RedSyntax               = pkgtheme.RedSyntax
	QuietLightSyntax        = pkgtheme.QuietLightSyntax
)
