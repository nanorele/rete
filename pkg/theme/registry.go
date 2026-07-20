package theme

import (
	"image/color"
)

type Def struct {
	ID      string
	Name    string
	Palette Palette
}

func fieldSurface(bg color.NRGBA, isLight bool) color.NRGBA {
	if !isLight {
		return Shade(bg, 0.08)
	}
	lift := func(v uint8) uint8 {
		x := int(v) + 10
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	f := color.NRGBA{R: lift(bg.R), G: lift(bg.G), B: lift(bg.B), A: bg.A}
	d := func(a, b uint8) int {
		v := int(a) - int(b)
		if v < 0 {
			v = -v
		}
		return v
	}
	if d(f.R, bg.R) < 6 && d(f.G, bg.G) < 6 && d(f.B, bg.B) < 6 {
		return Shade(bg, -0.035)
	}
	return f
}

func MakeTheme(bg, fg, accent, danger color.NRGBA, isLight bool) Palette {
	var (
		bgDirDark, menuDir, popupDir, hoverDir, secDir float32
		borderMix, borderLightMix                      float32
		borderMinRatio, borderLightMinRatio            float32
	)
	if isLight {
		bgDirDark = -0.06
		menuDir = 0.02
		popupDir = 0.03
		hoverDir = -0.06
		secDir = -0.04
		borderMix = 0.17
		borderLightMix = 0.27
		borderMinRatio = 1.38
		borderLightMinRatio = 1.70
	} else {
		bgDirDark = -0.18
		menuDir = -0.06
		popupDir = 0.05
		hoverDir = 0.12
		secDir = 0.08
		borderMix = 0.08
		borderLightMix = 0.17
		borderMinRatio = 1.19
		borderLightMinRatio = 1.49
	}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	divider := color.NRGBA{R: 255, G: 255, B: 255, A: 60}
	if isLight {
		white = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
		divider = color.NRGBA{R: 0, G: 0, B: 0, A: 40}
	}
	accentHover := Shade(accent, 0.14)
	cancel := Shade(danger, -0.1)
	accentFg := BestTextOn(accent, accentHover)
	dangerFg := BestTextOn(danger, cancel)
	accent = AdjustForContrast(accent, accentFg, 4.0)
	accentHover = AdjustForContrast(accentHover, accentFg, 4.0)
	cancel = AdjustForContrast(cancel, dangerFg, 4.0)
	p := Palette{
		Bg:           bg,
		BgDark:       Shade(bg, bgDirDark),
		BgField:      fieldSurface(bg, isLight),
		BgMenu:       Shade(bg, menuDir),
		BgPopup:      Shade(bg, popupDir),
		BgHover:      Shade(bg, hoverDir),
		BgSecondary:  Shade(bg, secDir),
		BgLoadMore:   Shade(bg, secDir*1.2),
		BgDragHolder: Shade(bg, bgDirDark*1.4),
		BgDragGhost:  WithAlpha(bg, 240),
		Border:       AdjustForContrast(Mix(bg, fg, borderMix), bg, borderMinRatio),
		BorderLight:  AdjustForContrast(Mix(bg, fg, borderLightMix), bg, borderLightMinRatio),
		Fg:           fg,
		FgMuted:      AdjustForContrast(Mix(bg, fg, 0.72), bg, 3.2),
		FgDim:        AdjustForContrast(Mix(bg, fg, 0.62), bg, 3.0),
		FgHint:       AdjustForContrast(Mix(bg, fg, 0.82), bg, 3.4),
		FgDisabled:   Mix(bg, fg, 0.35),
		White:        white,
		Accent:       accent,
		AccentHover:  accentHover,
		AccentDim:    WithAlpha(accent, 40),
		AccentFg:     accentFg,
		Danger:       danger,
		DangerFg:     dangerFg,
		Cancel:       cancel,
		CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
		ScrollThumb:  Mix(bg, fg, 0.32),
		VarFound:     WithAlpha(accent, 100),
		VarMissing:   WithAlpha(danger, 100),
		DividerLight: divider,
	}
	p.Syntax = DeriveSyntax(p)
	return p
}

var Dark = Palette{
	Bg:           color.NRGBA{R: 31, G: 31, B: 31, A: 255},
	BgDark:       color.NRGBA{R: 24, G: 24, B: 24, A: 255},
	BgField:      color.NRGBA{R: 49, G: 49, B: 49, A: 255},
	BgMenu:       color.NRGBA{R: 37, G: 37, B: 38, A: 255},
	BgPopup:      color.NRGBA{R: 35, G: 35, B: 35, A: 255},
	BgHover:      color.NRGBA{R: 42, G: 45, B: 46, A: 255},
	BgSecondary:  color.NRGBA{R: 55, G: 55, B: 55, A: 255},
	BgLoadMore:   color.NRGBA{R: 50, G: 50, B: 50, A: 255},
	BgDragHolder: color.NRGBA{R: 20, G: 20, B: 20, A: 255},
	BgDragGhost:  color.NRGBA{R: 31, G: 31, B: 31, A: 240},
	Border:       color.NRGBA{R: 43, G: 45, B: 49, A: 255},
	BorderLight:  color.NRGBA{R: 60, G: 60, B: 60, A: 255},
	Fg:           color.NRGBA{R: 204, G: 204, B: 204, A: 255},
	FgMuted:      color.NRGBA{R: 150, G: 150, B: 150, A: 255},
	FgDim:        color.NRGBA{R: 140, G: 140, B: 140, A: 255},
	FgHint:       color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	FgDisabled:   color.NRGBA{R: 80, G: 80, B: 80, A: 255},
	White:        color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Accent:       color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	AccentHover:  color.NRGBA{R: 20, G: 120, B: 180, A: 255},
	AccentDim:    color.NRGBA{R: 14, G: 99, B: 156, A: 40},
	AccentFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Danger:       color.NRGBA{R: 194, G: 64, B: 56, A: 255},
	DangerFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Cancel:       color.NRGBA{R: 180, G: 40, B: 40, A: 255},
	CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
	ScrollThumb:  color.NRGBA{R: 75, G: 75, B: 75, A: 255},
	VarFound:     color.NRGBA{R: 40, G: 110, B: 160, A: 100},
	VarMissing:   color.NRGBA{R: 130, G: 60, B: 60, A: 100},
	DividerLight: color.NRGBA{R: 255, G: 255, B: 255, A: 60},
	Syntax:       DarkPlusSyntax,
}

var Light = Palette{
	Bg:           color.NRGBA{R: 245, G: 245, B: 245, A: 255},
	BgDark:       color.NRGBA{R: 230, G: 230, B: 230, A: 255},
	BgField:      color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	BgMenu:       color.NRGBA{R: 250, G: 250, B: 250, A: 255},
	BgPopup:      color.NRGBA{R: 252, G: 252, B: 252, A: 255},
	BgHover:      color.NRGBA{R: 220, G: 228, B: 235, A: 255},
	BgSecondary:  color.NRGBA{R: 238, G: 238, B: 238, A: 255},
	BgLoadMore:   color.NRGBA{R: 230, G: 230, B: 230, A: 255},
	BgDragHolder: color.NRGBA{R: 200, G: 200, B: 200, A: 255},
	BgDragGhost:  color.NRGBA{R: 245, G: 245, B: 245, A: 240},
	Border:       color.NRGBA{R: 210, G: 210, B: 214, A: 255},
	BorderLight:  color.NRGBA{R: 190, G: 190, B: 190, A: 255},
	Fg:           color.NRGBA{R: 40, G: 40, B: 40, A: 255},
	FgMuted:      color.NRGBA{R: 100, G: 100, B: 100, A: 255},
	FgDim:        color.NRGBA{R: 120, G: 120, B: 120, A: 255},
	FgHint:       color.NRGBA{R: 130, G: 130, B: 130, A: 255},
	FgDisabled:   color.NRGBA{R: 180, G: 180, B: 180, A: 255},
	White:        color.NRGBA{R: 20, G: 20, B: 20, A: 255},
	Accent:       color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	AccentHover:  color.NRGBA{R: 20, G: 120, B: 180, A: 255},
	AccentDim:    color.NRGBA{R: 14, G: 99, B: 156, A: 40},
	AccentFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Danger:       color.NRGBA{R: 194, G: 64, B: 56, A: 255},
	DangerFg:     color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Cancel:       color.NRGBA{R: 180, G: 40, B: 40, A: 255},
	CloseHover:   color.NRGBA{R: 232, G: 17, B: 35, A: 255},
	ScrollThumb:  color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	VarFound:     color.NRGBA{R: 40, G: 110, B: 160, A: 80},
	VarMissing:   color.NRGBA{R: 130, G: 60, B: 60, A: 80},
	DividerLight: color.NRGBA{R: 0, G: 0, B: 0, A: 40},
	Syntax:       LightPlusSyntax,
}

var Registry = []Def{
	{ID: "dark", Name: "Dark+ (default dark)", Palette: Dark},
	{ID: "light", Name: "Light+ (default light)", Palette: Light},
	{ID: "monokai", Name: "Monokai", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 39, G: 40, B: 34, A: 255},
		color.NRGBA{R: 248, G: 248, B: 242, A: 255},
		color.NRGBA{R: 166, G: 226, B: 46, A: 255},
		color.NRGBA{R: 249, G: 38, B: 114, A: 255},
		false,
	), MonokaiSyntax)},
	{ID: "monokai-dimmed", Name: "Monokai Dimmed", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 30, G: 30, B: 30, A: 255},
		color.NRGBA{R: 193, G: 193, B: 193, A: 255},
		color.NRGBA{R: 155, G: 184, B: 75, A: 255},
		color.NRGBA{R: 204, G: 102, B: 102, A: 255},
		false,
	), MonokaiDimmedSyntax)},
	{ID: "solarized-dark", Name: "Solarized Dark", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 0, G: 43, B: 54, A: 255},
		color.NRGBA{R: 147, G: 161, B: 161, A: 255},
		color.NRGBA{R: 38, G: 139, B: 210, A: 255},
		color.NRGBA{R: 220, G: 50, B: 47, A: 255},
		false,
	), SolarizedDarkSyntax)},
	{ID: "solarized-light", Name: "Solarized Light", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 253, G: 246, B: 227, A: 255},
		color.NRGBA{R: 88, G: 110, B: 117, A: 255},
		color.NRGBA{R: 38, G: 139, B: 210, A: 255},
		color.NRGBA{R: 220, G: 50, B: 47, A: 255},
		true,
	), SolarizedLightSyntax)},
	{ID: "dracula", Name: "Dracula", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 40, G: 42, B: 54, A: 255},
		color.NRGBA{R: 248, G: 248, B: 242, A: 255},
		color.NRGBA{R: 189, G: 147, B: 249, A: 255},
		color.NRGBA{R: 255, G: 85, B: 85, A: 255},
		false,
	), DraculaSyntax)},
	{ID: "abyss", Name: "Abyss", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 0, G: 12, B: 24, A: 255},
		color.NRGBA{R: 108, G: 149, B: 235, A: 255},
		color.NRGBA{R: 0, G: 139, B: 139, A: 255},
		color.NRGBA{R: 210, G: 50, B: 50, A: 255},
		false,
	), AbyssSyntax)},
	{ID: "kimbie-dark", Name: "Kimbie Dark", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 34, G: 26, B: 15, A: 255},
		color.NRGBA{R: 211, G: 175, B: 134, A: 255},
		color.NRGBA{R: 136, G: 155, B: 74, A: 255},
		color.NRGBA{R: 220, G: 62, B: 42, A: 255},
		false,
	), KimbieDarkSyntax)},
	{ID: "tomorrow-night-blue", Name: "Tomorrow Night Blue", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 0, G: 36, B: 81, A: 255},
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		color.NRGBA{R: 114, G: 133, B: 183, A: 255},
		color.NRGBA{R: 255, G: 157, B: 132, A: 255},
		false,
	), TomorrowNightBlueSyntax)},
	{ID: "red", Name: "Red", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 57, G: 10, B: 9, A: 255},
		color.NRGBA{R: 243, G: 224, B: 224, A: 255},
		color.NRGBA{R: 255, G: 104, B: 66, A: 255},
		color.NRGBA{R: 215, G: 40, B: 40, A: 255},
		false,
	), RedSyntax)},
	{ID: "quiet-light", Name: "Quiet Light", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 245, G: 245, B: 245, A: 255},
		color.NRGBA{R: 51, G: 51, B: 51, A: 255},
		color.NRGBA{R: 154, G: 103, B: 0, A: 255},
		color.NRGBA{R: 210, G: 40, B: 50, A: 255},
		true,
	), QuietLightSyntax)},
	{ID: "one-dark", Name: "One Dark Pro", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 40, G: 44, B: 52, A: 255},
		color.NRGBA{R: 171, G: 178, B: 191, A: 255},
		color.NRGBA{R: 97, G: 175, B: 239, A: 255},
		color.NRGBA{R: 224, G: 108, B: 117, A: 255},
		false,
	), OneDarkSyntax)},
	{ID: "github-dark", Name: "GitHub Dark", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 13, G: 17, B: 23, A: 255},
		color.NRGBA{R: 201, G: 209, B: 217, A: 255},
		color.NRGBA{R: 88, G: 166, B: 255, A: 255},
		color.NRGBA{R: 248, G: 81, B: 73, A: 255},
		false,
	), GithubDarkSyntax)},
	{ID: "github-light", Name: "GitHub Light", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 255, G: 255, B: 255, A: 255},
		color.NRGBA{R: 36, G: 41, B: 47, A: 255},
		color.NRGBA{R: 9, G: 105, B: 218, A: 255},
		color.NRGBA{R: 207, G: 34, B: 46, A: 255},
		true,
	), GithubLightSyntax)},
	{ID: "nord", Name: "Nord", Palette: WithSyntax(MakeTheme(
		color.NRGBA{R: 46, G: 52, B: 64, A: 255},
		color.NRGBA{R: 216, G: 222, B: 233, A: 255},
		color.NRGBA{R: 136, G: 192, B: 208, A: 255},
		color.NRGBA{R: 191, G: 97, B: 106, A: 255},
		false,
	), NordSyntax)},
}
