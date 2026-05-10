package theme

import "image/color"

func DeriveSyntax(p Palette) SyntaxPalette {
	isLight := RelLuminance(p.Bg) > 0.5

	shift := func(c color.NRGBA, dark, light float32) color.NRGBA {
		if isLight {
			return Shade(c, light)
		}
		return Shade(c, dark)
	}

	stringC := Mix(p.Accent, color.NRGBA{R: 152, G: 195, B: 121, A: 255}, 0.65)
	if isLight {
		stringC = Mix(stringC, color.NRGBA{R: 0, G: 100, B: 0, A: 255}, 0.4)
	}

	syn := SyntaxPalette{
		Plain:       p.Fg,
		String:      stringC,
		Number:      shift(p.Accent, 0.2, -0.25),
		Bool:        shift(p.Accent, 0.0, -0.15),
		Null:        shift(p.Accent, 0.0, -0.15),
		Key:         shift(p.Accent, 0.15, -0.2),
		Punctuation: p.FgMuted,
		Operator:    p.FgDim,
		Keyword:     shift(p.Accent, 0.1, -0.2),
		Type:        shift(p.Accent, 0.2, -0.3),
		Comment:     Mix(p.Bg, p.Fg, 0.4),
	}
	if isLight {
		syn.Brackets = [3]color.NRGBA{
			{R: 0, G: 122, B: 204, A: 255},
			{R: 200, G: 100, B: 0, A: 255},
			{R: 130, G: 90, B: 200, A: 255},
		}
	} else {
		syn.Brackets = [3]color.NRGBA{
			{R: 255, G: 215, B: 0, A: 255},
			{R: 218, G: 112, B: 214, A: 255},
			{R: 23, G: 159, B: 255, A: 255},
		}
	}
	return syn
}

func WithSyntax(p Palette, s SyntaxPalette) Palette {
	p.Syntax = s
	return p
}

var DarkPlusSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	String:      color.NRGBA{R: 206, G: 145, B: 120, A: 255},
	Number:      color.NRGBA{R: 181, G: 206, B: 168, A: 255},
	Bool:        color.NRGBA{R: 86, G: 156, B: 214, A: 255},
	Null:        color.NRGBA{R: 86, G: 156, B: 214, A: 255},
	Key:         color.NRGBA{R: 156, G: 220, B: 254, A: 255},
	Punctuation: color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	Operator:    color.NRGBA{R: 212, G: 212, B: 212, A: 255},
	Keyword:     color.NRGBA{R: 197, G: 134, B: 192, A: 255},
	Type:        color.NRGBA{R: 78, G: 201, B: 176, A: 255},
	Comment:     color.NRGBA{R: 106, G: 153, B: 85, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 215, B: 0, A: 255},
		{R: 218, G: 112, B: 214, A: 255},
		{R: 23, G: 159, B: 255, A: 255},
	},
}

var LightPlusSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	String:      color.NRGBA{R: 163, G: 21, B: 21, A: 255},
	Number:      color.NRGBA{R: 9, G: 134, B: 88, A: 255},
	Bool:        color.NRGBA{R: 0, G: 0, B: 255, A: 255},
	Null:        color.NRGBA{R: 0, G: 0, B: 255, A: 255},
	Key:         color.NRGBA{R: 4, G: 81, B: 165, A: 255},
	Punctuation: color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Operator:    color.NRGBA{R: 0, G: 0, B: 0, A: 255},
	Keyword:     color.NRGBA{R: 175, G: 0, B: 219, A: 255},
	Type:        color.NRGBA{R: 38, G: 127, B: 153, A: 255},
	Comment:     color.NRGBA{R: 0, G: 128, B: 0, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 0, G: 65, B: 159, A: 255},
		{R: 178, G: 99, B: 0, A: 255},
		{R: 113, G: 36, B: 165, A: 255},
	},
}

var MonokaiSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	String:      color.NRGBA{R: 230, G: 219, B: 116, A: 255},
	Number:      color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Bool:        color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Null:        color.NRGBA{R: 174, G: 129, B: 255, A: 255},
	Key:         color.NRGBA{R: 166, G: 226, B: 46, A: 255},
	Punctuation: color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	Operator:    color.NRGBA{R: 249, G: 38, B: 114, A: 255},
	Keyword:     color.NRGBA{R: 249, G: 38, B: 114, A: 255},
	Type:        color.NRGBA{R: 102, G: 217, B: 239, A: 255},
	Comment:     color.NRGBA{R: 117, G: 113, B: 94, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 249, G: 38, B: 114, A: 255},
		{R: 166, G: 226, B: 46, A: 255},
		{R: 102, G: 217, B: 239, A: 255},
	},
}

var DraculaSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	String:      color.NRGBA{R: 241, G: 250, B: 140, A: 255},
	Number:      color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Bool:        color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Null:        color.NRGBA{R: 189, G: 147, B: 249, A: 255},
	Key:         color.NRGBA{R: 139, G: 233, B: 253, A: 255},
	Punctuation: color.NRGBA{R: 248, G: 248, B: 242, A: 255},
	Operator:    color.NRGBA{R: 255, G: 121, B: 198, A: 255},
	Keyword:     color.NRGBA{R: 255, G: 121, B: 198, A: 255},
	Type:        color.NRGBA{R: 80, G: 250, B: 123, A: 255},
	Comment:     color.NRGBA{R: 98, G: 114, B: 164, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 121, B: 198, A: 255},
		{R: 80, G: 250, B: 123, A: 255},
		{R: 139, G: 233, B: 253, A: 255},
	},
}

var OneDarkSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 171, G: 178, B: 191, A: 255},
	String:      color.NRGBA{R: 152, G: 195, B: 121, A: 255},
	Number:      color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Bool:        color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Null:        color.NRGBA{R: 209, G: 154, B: 102, A: 255},
	Key:         color.NRGBA{R: 224, G: 108, B: 117, A: 255},
	Punctuation: color.NRGBA{R: 171, G: 178, B: 191, A: 255},
	Operator:    color.NRGBA{R: 86, G: 182, B: 194, A: 255},
	Keyword:     color.NRGBA{R: 198, G: 120, B: 221, A: 255},
	Type:        color.NRGBA{R: 229, G: 192, B: 123, A: 255},
	Comment:     color.NRGBA{R: 92, G: 99, B: 112, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 198, G: 120, B: 221, A: 255},
		{R: 209, G: 154, B: 102, A: 255},
		{R: 86, G: 182, B: 194, A: 255},
	},
}

var SolarizedDarkSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	String:      color.NRGBA{R: 42, G: 161, B: 152, A: 255},
	Number:      color.NRGBA{R: 211, G: 54, B: 130, A: 255},
	Bool:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Null:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Key:         color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Punctuation: color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	Operator:    color.NRGBA{R: 203, G: 75, B: 22, A: 255},
	Keyword:     color.NRGBA{R: 133, G: 153, B: 0, A: 255},
	Type:        color.NRGBA{R: 181, G: 137, B: 0, A: 255},
	Comment:     color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 211, G: 54, B: 130, A: 255},
		{R: 133, G: 153, B: 0, A: 255},
		{R: 38, G: 139, B: 210, A: 255},
	},
}

var SolarizedLightSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	String:      color.NRGBA{R: 42, G: 161, B: 152, A: 255},
	Number:      color.NRGBA{R: 211, G: 54, B: 130, A: 255},
	Bool:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Null:        color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Key:         color.NRGBA{R: 38, G: 139, B: 210, A: 255},
	Punctuation: color.NRGBA{R: 88, G: 110, B: 117, A: 255},
	Operator:    color.NRGBA{R: 203, G: 75, B: 22, A: 255},
	Keyword:     color.NRGBA{R: 133, G: 153, B: 0, A: 255},
	Type:        color.NRGBA{R: 181, G: 137, B: 0, A: 255},
	Comment:     color.NRGBA{R: 147, G: 161, B: 161, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 211, G: 54, B: 130, A: 255},
		{R: 133, G: 153, B: 0, A: 255},
		{R: 38, G: 139, B: 210, A: 255},
	},
}

var GithubDarkSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 201, G: 209, B: 217, A: 255},
	String:      color.NRGBA{R: 165, G: 214, B: 255, A: 255},
	Number:      color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Bool:        color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Null:        color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Key:         color.NRGBA{R: 121, G: 192, B: 255, A: 255},
	Punctuation: color.NRGBA{R: 201, G: 209, B: 217, A: 255},
	Operator:    color.NRGBA{R: 255, G: 123, B: 114, A: 255},
	Keyword:     color.NRGBA{R: 255, G: 123, B: 114, A: 255},
	Type:        color.NRGBA{R: 255, G: 166, B: 87, A: 255},
	Comment:     color.NRGBA{R: 139, G: 148, B: 158, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 123, B: 114, A: 255},
		{R: 255, G: 166, B: 87, A: 255},
		{R: 121, G: 192, B: 255, A: 255},
	},
}

var GithubLightSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 36, G: 41, B: 47, A: 255},
	String:      color.NRGBA{R: 10, G: 48, B: 105, A: 255},
	Number:      color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Bool:        color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Null:        color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Key:         color.NRGBA{R: 5, G: 80, B: 174, A: 255},
	Punctuation: color.NRGBA{R: 36, G: 41, B: 47, A: 255},
	Operator:    color.NRGBA{R: 207, G: 34, B: 46, A: 255},
	Keyword:     color.NRGBA{R: 207, G: 34, B: 46, A: 255},
	Type:        color.NRGBA{R: 149, G: 53, B: 32, A: 255},
	Comment:     color.NRGBA{R: 106, G: 115, B: 125, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 207, G: 34, B: 46, A: 255},
		{R: 149, G: 53, B: 32, A: 255},
		{R: 5, G: 80, B: 174, A: 255},
	},
}

var MonokaiDimmedSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 193, G: 193, B: 193, A: 255},
	String:      color.NRGBA{R: 154, G: 168, B: 58, A: 255},
	Number:      color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Bool:        color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Null:        color.NRGBA{R: 96, G: 137, B: 180, A: 255},
	Key:         color.NRGBA{R: 157, G: 163, B: 154, A: 255},
	Punctuation: color.NRGBA{R: 193, G: 193, B: 193, A: 255},
	Operator:    color.NRGBA{R: 103, G: 104, B: 103, A: 255},
	Keyword:     color.NRGBA{R: 152, G: 118, B: 170, A: 255},
	Type:        color.NRGBA{R: 157, G: 163, B: 154, A: 255},
	Comment:     color.NRGBA{R: 154, G: 154, B: 154, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 204, G: 102, B: 102, A: 255},
		{R: 155, G: 184, B: 75, A: 255},
		{R: 96, G: 137, B: 180, A: 255},
	},
}

var AbyssSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 108, G: 149, B: 235, A: 255},
	String:      color.NRGBA{R: 34, G: 170, B: 68, A: 255},
	Number:      color.NRGBA{R: 242, G: 128, B: 208, A: 255},
	Bool:        color.NRGBA{R: 80, G: 138, B: 192, A: 255},
	Null:        color.NRGBA{R: 80, G: 138, B: 192, A: 255},
	Key:         color.NRGBA{R: 34, G: 153, B: 230, A: 255},
	Punctuation: color.NRGBA{R: 108, G: 149, B: 235, A: 255},
	Operator:    color.NRGBA{R: 230, G: 213, B: 84, A: 255},
	Keyword:     color.NRGBA{R: 34, G: 153, B: 230, A: 255},
	Type:        color.NRGBA{R: 221, G: 187, B: 136, A: 255},
	Comment:     color.NRGBA{R: 56, G: 72, B: 135, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 242, G: 128, B: 208, A: 255},
		{R: 34, G: 170, B: 68, A: 255},
		{R: 221, G: 187, B: 136, A: 255},
	},
}

var KimbieDarkSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 211, G: 175, B: 134, A: 255},
	String:      color.NRGBA{R: 136, G: 155, B: 74, A: 255},
	Number:      color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Bool:        color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Null:        color.NRGBA{R: 247, G: 154, B: 50, A: 255},
	Key:         color.NRGBA{R: 152, G: 103, B: 106, A: 255},
	Punctuation: color.NRGBA{R: 211, G: 175, B: 134, A: 255},
	Operator:    color.NRGBA{R: 240, G: 100, B: 49, A: 255},
	Keyword:     color.NRGBA{R: 152, G: 103, B: 106, A: 255},
	Type:        color.NRGBA{R: 240, G: 100, B: 49, A: 255},
	Comment:     color.NRGBA{R: 165, G: 122, B: 76, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 220, G: 62, B: 42, A: 255},
		{R: 136, G: 155, B: 74, A: 255},
		{R: 247, G: 154, B: 50, A: 255},
	},
}

var NordSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 216, G: 222, B: 233, A: 255},
	String:      color.NRGBA{R: 163, G: 190, B: 140, A: 255},
	Number:      color.NRGBA{R: 180, G: 142, B: 173, A: 255},
	Bool:        color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Null:        color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Key:         color.NRGBA{R: 143, G: 188, B: 187, A: 255},
	Punctuation: color.NRGBA{R: 216, G: 222, B: 233, A: 255},
	Operator:    color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Keyword:     color.NRGBA{R: 129, G: 161, B: 193, A: 255},
	Type:        color.NRGBA{R: 143, G: 188, B: 187, A: 255},
	Comment:     color.NRGBA{R: 97, G: 110, B: 136, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 191, G: 97, B: 106, A: 255},
		{R: 235, G: 203, B: 139, A: 255},
		{R: 136, G: 192, B: 208, A: 255},
	},
}

var TomorrowNightBlueSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	String:      color.NRGBA{R: 209, G: 241, B: 169, A: 255},
	Number:      color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Bool:        color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Null:        color.NRGBA{R: 255, G: 197, B: 143, A: 255},
	Key:         color.NRGBA{R: 255, G: 238, B: 173, A: 255},
	Punctuation: color.NRGBA{R: 255, G: 255, B: 255, A: 255},
	Operator:    color.NRGBA{R: 255, G: 157, B: 132, A: 255},
	Keyword:     color.NRGBA{R: 235, G: 187, B: 255, A: 255},
	Type:        color.NRGBA{R: 255, G: 238, B: 173, A: 255},
	Comment:     color.NRGBA{R: 114, G: 133, B: 183, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 255, G: 157, B: 164, A: 255},
		{R: 255, G: 238, B: 173, A: 255},
		{R: 187, G: 218, B: 255, A: 255},
	},
}

var RedSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 243, G: 224, B: 224, A: 255},
	String:      color.NRGBA{R: 255, G: 201, B: 161, A: 255},
	Number:      color.NRGBA{R: 243, G: 58, B: 21, A: 255},
	Bool:        color.NRGBA{R: 255, G: 137, B: 112, A: 255},
	Null:        color.NRGBA{R: 255, G: 137, B: 112, A: 255},
	Key:         color.NRGBA{R: 255, G: 255, B: 137, A: 255},
	Punctuation: color.NRGBA{R: 243, G: 224, B: 224, A: 255},
	Operator:    color.NRGBA{R: 251, G: 154, B: 75, A: 255},
	Keyword:     color.NRGBA{R: 251, G: 154, B: 75, A: 255},
	Type:        color.NRGBA{R: 218, G: 239, B: 163, A: 255},
	Comment:     color.NRGBA{R: 230, G: 70, B: 64, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 251, G: 154, B: 75, A: 255},
		{R: 255, G: 255, B: 137, A: 255},
		{R: 255, G: 142, B: 142, A: 255},
	},
}

var QuietLightSyntax = SyntaxPalette{
	Plain:       color.NRGBA{R: 51, G: 51, B: 51, A: 255},
	String:      color.NRGBA{R: 68, G: 140, B: 39, A: 255},
	Number:      color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Bool:        color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Null:        color.NRGBA{R: 171, G: 101, B: 38, A: 255},
	Key:         color.NRGBA{R: 79, G: 118, B: 172, A: 255},
	Punctuation: color.NRGBA{R: 51, G: 51, B: 51, A: 255},
	Operator:    color.NRGBA{R: 119, G: 119, B: 119, A: 255},
	Keyword:     color.NRGBA{R: 75, G: 131, B: 205, A: 255},
	Type:        color.NRGBA{R: 122, G: 62, B: 157, A: 255},
	Comment:     color.NRGBA{R: 170, G: 170, B: 170, A: 255},
	Brackets: [3]color.NRGBA{
		{R: 210, G: 40, B: 50, A: 255},
		{R: 154, G: 103, B: 0, A: 255},
		{R: 111, G: 66, B: 193, A: 255},
	},
}
