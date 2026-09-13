package theme

import (
	"image/color"
	"math"
	"rete/pkg/syntax"
	"testing"
)

func blendOver(fg, bg color.NRGBA) color.NRGBA {
	a := float32(fg.A) / 255
	mix := func(f, b uint8) uint8 {
		return uint8(float32(f)*a + float32(b)*(1-a))
	}
	return color.NRGBA{R: mix(fg.R, bg.R), G: mix(fg.G, bg.G), B: mix(fg.B, bg.B), A: 255}
}

type pair struct {
	what string
	fg   func(p Palette) color.NRGBA
	bg   func(p Palette) color.NRGBA
	min  float32
}

func auditPairs() []pair {
	return []pair{
		{"Fg on BtnPrimary", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA {
			return ContrastBgFor(Composite(p.VarFound, p.Bg), p.Fg, 4.0)
		}, 4.0},
		{"AccentFg on Accent", func(p Palette) color.NRGBA { return p.AccentFg }, func(p Palette) color.NRGBA { return p.Accent }, 4.0},
		{"AccentFg on AccentHover", func(p Palette) color.NRGBA { return p.AccentFg }, func(p Palette) color.NRGBA { return p.AccentHover }, 4.0},
		{"DangerFg on Danger", func(p Palette) color.NRGBA { return p.DangerFg }, func(p Palette) color.NRGBA { return p.Danger }, 4.0},
		{"DangerFg on Cancel", func(p Palette) color.NRGBA { return p.DangerFg }, func(p Palette) color.NRGBA { return p.Cancel }, 4.0},
		{"Fg on Border", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.Border }, 3.5},
		{"Fg on BgSecondary", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.BgSecondary }, 3.5},
		{"Fg on BgHover", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.BgHover }, 3.5},
		{"Fg on Bg", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.Bg }, 4.0},
		{"Fg on BgField", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.BgField }, 4.0},
		{"Fg on BgMenu", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.BgMenu }, 4.0},
		{"Fg on BgPopup", func(p Palette) color.NRGBA { return p.Fg }, func(p Palette) color.NRGBA { return p.BgPopup }, 4.0},
		{"FgMuted on Bg", func(p Palette) color.NRGBA { return p.FgMuted }, func(p Palette) color.NRGBA { return p.Bg }, 3.0},
		{"FgDim on Bg", func(p Palette) color.NRGBA { return p.FgDim }, func(p Palette) color.NRGBA { return p.Bg }, 3.0},
		{"FgHint on Bg", func(p Palette) color.NRGBA { return p.FgHint }, func(p Palette) color.NRGBA { return p.Bg }, 3.0},
		{"FgHint on BgField", func(p Palette) color.NRGBA { return p.FgHint }, func(p Palette) color.NRGBA { return p.BgField }, 3.0},
	}
}

func maxChannelDelta(a, b color.NRGBA) int {
	d := func(x, y uint8) int {
		v := int(x) - int(y)
		if v < 0 {
			v = -v
		}
		return v
	}
	m := d(a.R, b.R)
	if v := d(a.G, b.G); v > m {
		m = v
	}
	if v := d(a.B, b.B); v > m {
		m = v
	}
	return m
}

func TestFieldSurfaceDistinctFromBg(t *testing.T) {
	for _, def := range Registry {
		def := def
		t.Run(def.ID, func(t *testing.T) {
			p := def.Palette
			if d := maxChannelDelta(p.BgField, p.Bg); d < 8 {
				t.Errorf("%s: BgField %v too close to Bg %v (delta %d < 8)", def.Name, p.BgField, p.Bg, d)
			}
		})
	}
}

func TestBorderContrastFloor(t *testing.T) {
	for _, def := range Registry {
		def := def
		t.Run(def.ID, func(t *testing.T) {
			p := def.Palette
			borderMin, borderLightMin := float32(1.185), float32(1.485)
			if RelLuminance(p.Bg) > 0.5 {
				borderMin, borderLightMin = 1.375, 1.695
			}
			if r := ContrastRatio(p.Border, p.Bg); r < borderMin {
				t.Errorf("%s: Border contrast %.3f < %.3f (border=%v bg=%v)", def.Name, r, borderMin, p.Border, p.Bg)
			}
			if r := ContrastRatio(p.BorderLight, p.Bg); r < borderLightMin {
				t.Errorf("%s: BorderLight contrast %.3f < %.3f (border=%v bg=%v)", def.Name, r, borderLightMin, p.BorderLight, p.Bg)
			}
		})
	}
}

func TestThemeContrast(t *testing.T) {
	for _, def := range Registry {
		def := def
		t.Run(def.ID, func(t *testing.T) {
			p := def.Palette
			for _, pr := range auditPairs() {
				bg := pr.bg(p)
				fg := blendOver(pr.fg(p), bg)
				got := ContrastRatio(fg, bg)
				if got < pr.min {
					t.Errorf("%s: %s contrast %.2f < %.2f (fg=%v bg=%v)",
						def.Name, pr.what, got, pr.min, pr.fg(p), bg)
				}
			}
		})
	}
}

func TestParseHex(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
		want color.NRGBA
	}{
		{"empty", "", false, color.NRGBA{}},
		{"hash only", "#", false, color.NRGBA{}},
		{"6 lower", "#abcdef", true, color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 255}},
		{"6 upper", "ABCDEF", true, color.NRGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 255}},
		{"6 mixed", "#AbCdEf", true, color.NRGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 255}},
		{"3 short", "#abc", true, color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 255}},
		{"3 short no hash", "f0f", true, color.NRGBA{R: 0xff, G: 0x00, B: 0xff, A: 255}},
		{"with surrounding spaces", "  #abcdef  ", true, color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 255}},
		{"4 char invalid", "#abcd", false, color.NRGBA{}},
		{"5 char invalid", "#abcde", false, color.NRGBA{}},
		{"7 char invalid", "#abcdef0", false, color.NRGBA{}},
		{"8 char alpha not supported", "#abcdef00", false, color.NRGBA{}},
		{"non-hex chars", "#zzzzzz", false, color.NRGBA{}},
		{"black", "#000000", true, color.NRGBA{R: 0, G: 0, B: 0, A: 255}},
		{"white", "#ffffff", true, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseHex(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok=%v want %v (in=%q got=%v)", ok, tc.ok, tc.in, got)
			}
			if ok && got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestHexFromColor(t *testing.T) {
	cases := []struct {
		c    color.NRGBA
		want string
	}{
		{color.NRGBA{R: 0, G: 0, B: 0, A: 255}, "#000000"},
		{color.NRGBA{R: 255, G: 255, B: 255, A: 255}, "#ffffff"},
		{color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 255}, "#abcdef"},
		{color.NRGBA{R: 1, G: 2, B: 3, A: 0}, "#010203"},
	}
	for _, tc := range cases {
		if got := HexFromColor(tc.c); got != tc.want {
			t.Errorf("got %q want %q", got, tc.want)
		}
	}
}

func TestHexRoundTrip(t *testing.T) {
	colors := []color.NRGBA{
		{R: 31, G: 31, B: 31, A: 255},
		{R: 0, G: 0, B: 0, A: 255},
		{R: 255, G: 255, B: 255, A: 255},
		{R: 14, G: 99, B: 156, A: 255},
		{R: 1, G: 16, B: 17, A: 255},
	}
	for _, c := range colors {
		h := HexFromColor(c)
		back, ok := ParseHex(h)
		if !ok {
			t.Fatalf("parse failed for %q", h)
		}
		if back.R != c.R || back.G != c.G || back.B != c.B {
			t.Fatalf("roundtrip mismatch in=%v out=%v hex=%s", c, back, h)
		}
	}
}

func TestShade(t *testing.T) {
	c := color.NRGBA{R: 100, G: 100, B: 100, A: 200}
	if got := Shade(c, 0); got != c {
		t.Errorf("amt=0 should be identity, got %v", got)
	}
	light := Shade(c, 0.5)
	if light.R <= c.R || light.G <= c.G || light.B <= c.B {
		t.Errorf("positive amt should lighten: %v -> %v", c, light)
	}
	if light.A != c.A {
		t.Errorf("alpha must be preserved: %v", light)
	}
	dark := Shade(c, -0.5)
	if dark.R >= c.R || dark.G >= c.G || dark.B >= c.B {
		t.Errorf("negative amt should darken: %v -> %v", c, dark)
	}

	hi := Shade(color.NRGBA{R: 250, G: 250, B: 250, A: 255}, 1.0)
	if hi.R != 255 || hi.G != 255 || hi.B != 255 {
		t.Errorf("expected clamp to 255, got %v", hi)
	}

	lo := Shade(color.NRGBA{R: 5, G: 5, B: 5, A: 255}, -2.0)
	if lo.R != 0 || lo.G != 0 || lo.B != 0 {
		t.Errorf("expected clamp to 0, got %v", lo)
	}
}

func TestMix(t *testing.T) {
	a := color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	b := color.NRGBA{R: 200, G: 100, B: 50, A: 255}
	if got := Mix(a, b, 0); got != (color.NRGBA{R: 0, G: 0, B: 0, A: 255}) {
		t.Errorf("t=0 should be a: %v", got)
	}
	if got := Mix(a, b, 1); got != (color.NRGBA{R: 200, G: 100, B: 50, A: 255}) {
		t.Errorf("t=1 should be b: %v", got)
	}
	mid := Mix(a, b, 0.5)
	if mid.R != 100 || mid.G != 50 || mid.B != 25 {
		t.Errorf("midpoint wrong: %v", mid)
	}
	if mid.A != 255 {
		t.Errorf("Mix must force A=255, got %d", mid.A)
	}
}

func TestWithAlpha(t *testing.T) {
	c := color.NRGBA{R: 10, G: 20, B: 30, A: 255}
	got := WithAlpha(c, 128)
	if got.R != 10 || got.G != 20 || got.B != 30 || got.A != 128 {
		t.Errorf("unexpected: %v", got)
	}
}

func TestRelLuminance_sRGB(t *testing.T) {

	if l := RelLuminance(color.NRGBA{R: 0, G: 0, B: 0, A: 255}); l != 0 {
		t.Errorf("black luminance want 0, got %v", l)
	}
	if l := RelLuminance(color.NRGBA{R: 255, G: 255, B: 255, A: 255}); math.Abs(float64(l-1)) > 1e-4 {
		t.Errorf("white luminance want ~1, got %v", l)
	}

	mid := RelLuminance(color.NRGBA{R: 128, G: 128, B: 128, A: 255})
	if math.Abs(float64(mid-0.2159)) > 0.01 {
		t.Errorf("mid-grey luminance want ~0.2159 (sRGB gamma), got %v", mid)
	}

	r := RelLuminance(color.NRGBA{R: 255, A: 255})
	g := RelLuminance(color.NRGBA{G: 255, A: 255})
	b := RelLuminance(color.NRGBA{B: 255, A: 255})
	if math.Abs(float64(r-0.2126)) > 1e-3 {
		t.Errorf("red luminance want 0.2126, got %v", r)
	}
	if math.Abs(float64(g-0.7152)) > 1e-3 {
		t.Errorf("green luminance want 0.7152, got %v", g)
	}
	if math.Abs(float64(b-0.0722)) > 1e-3 {
		t.Errorf("blue luminance want 0.0722, got %v", b)
	}

	low := RelLuminance(color.NRGBA{R: 10, A: 255})
	wantLow := 0.2126 * (10.0 / 255.0) / 12.92
	if math.Abs(float64(low)-wantLow) > 1e-4 {
		t.Errorf("low-channel red want %v, got %v", wantLow, low)
	}
}

func TestContrastOn(t *testing.T) {

	got := ContrastOn(color.NRGBA{R: 20, G: 20, B: 20, A: 255})
	if got != (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Errorf("dark bg want white text, got %v", got)
	}

	got = ContrastOn(color.NRGBA{R: 240, G: 240, B: 240, A: 255})
	if got != (color.NRGBA{R: 20, G: 20, B: 20, A: 255}) {
		t.Errorf("light bg want dark text, got %v", got)
	}
}

func TestColorForToken(t *testing.T) {
	sp := SyntaxPalette{
		Plain:       color.NRGBA{R: 1},
		String:      color.NRGBA{R: 2},
		Number:      color.NRGBA{R: 3},
		Bool:        color.NRGBA{R: 4},
		Null:        color.NRGBA{R: 5},
		Key:         color.NRGBA{R: 6},
		Punctuation: color.NRGBA{R: 7},
		Operator:    color.NRGBA{R: 8},
		Keyword:     color.NRGBA{R: 9},
		Type:        color.NRGBA{R: 10},
		Comment:     color.NRGBA{R: 11},
		Brackets: [3]color.NRGBA{
			{R: 20}, {R: 21}, {R: 22},
		},
	}
	cases := []struct {
		kind syntax.TokenKind
		want color.NRGBA
	}{
		{syntax.TokString, sp.String},
		{syntax.TokNumber, sp.Number},
		{syntax.TokBool, sp.Bool},
		{syntax.TokNull, sp.Null},
		{syntax.TokKey, sp.Key},
		{syntax.TokPunctuation, sp.Punctuation},
		{syntax.TokOperator, sp.Operator},
		{syntax.TokKeyword, sp.Keyword},
		{syntax.TokType, sp.Type},
		{syntax.TokComment, sp.Comment},
	}
	for _, tc := range cases {
		if got := sp.ColorForToken(tc.kind, 0, false); got != tc.want {
			t.Errorf("kind=%v got=%v want=%v", tc.kind, got, tc.want)
		}
	}

	if got := sp.ColorForToken(syntax.TokenKind(255), 0, false); got != sp.Plain {
		t.Errorf("unknown kind should return Plain, got %v", got)
	}

	if got := sp.ColorForToken(syntax.TokBracket, 0, true); got != sp.Brackets[0] {
		t.Errorf("bracket depth 0 cycle: %v", got)
	}
	if got := sp.ColorForToken(syntax.TokBracket, 1, true); got != sp.Brackets[1] {
		t.Errorf("bracket depth 1 cycle: %v", got)
	}
	if got := sp.ColorForToken(syntax.TokBracket, 4, true); got != sp.Brackets[1] {
		t.Errorf("bracket depth 4 should wrap to index 1: %v", got)
	}

	if got := sp.ColorForToken(syntax.TokBracket, 0, false); got != sp.Punctuation {
		t.Errorf("bracket no-cycle should be Punctuation, got %v", got)
	}
}

func TestMakeTheme_BasicShape(t *testing.T) {
	bg := color.NRGBA{R: 30, G: 30, B: 30, A: 255}
	fg := color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	accent := color.NRGBA{R: 14, G: 99, B: 156, A: 255}
	danger := color.NRGBA{R: 200, G: 50, B: 50, A: 255}
	p := MakeTheme(bg, fg, accent, danger, false)
	if p.Bg != bg || p.Fg != fg || p.Accent != accent || p.Danger != danger {
		t.Error("MakeTheme should pass through the seeds")
	}
	if (p.Syntax == SyntaxPalette{}) {
		t.Error("MakeTheme should derive a non-zero Syntax")
	}

	pl := MakeTheme(
		color.NRGBA{R: 240, G: 240, B: 240, A: 255},
		color.NRGBA{R: 30, G: 30, B: 30, A: 255},
		accent, danger, true,
	)

	if pl.White == (color.NRGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Error("light theme should use dark 'White'")
	}
}

func TestWithSyntax(t *testing.T) {
	p := Palette{Bg: color.NRGBA{R: 1}}
	out := WithSyntax(p, MonokaiSyntax)
	if out.Bg != p.Bg {
		t.Error("WithSyntax must preserve palette fields")
	}
	if out.Syntax != MonokaiSyntax {
		t.Error("WithSyntax must set Syntax")
	}
}

func TestDeriveSyntax_BranchesByLuminance(t *testing.T) {
	dark := Palette{
		Bg:      color.NRGBA{R: 20, G: 20, B: 20, A: 255},
		Fg:      color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		FgMuted: color.NRGBA{R: 150, G: 150, B: 150, A: 255},
		FgDim:   color.NRGBA{R: 120, G: 120, B: 120, A: 255},
		Accent:  color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	}
	light := Palette{
		Bg:      color.NRGBA{R: 245, G: 245, B: 245, A: 255},
		Fg:      color.NRGBA{R: 40, G: 40, B: 40, A: 255},
		FgMuted: color.NRGBA{R: 100, G: 100, B: 100, A: 255},
		FgDim:   color.NRGBA{R: 120, G: 120, B: 120, A: 255},
		Accent:  color.NRGBA{R: 14, G: 99, B: 156, A: 255},
	}
	ds := DeriveSyntax(dark)
	ls := DeriveSyntax(light)
	if ds.Brackets == ls.Brackets {
		t.Error("dark and light should choose different bracket sets")
	}
	if ds.Plain != dark.Fg || ls.Plain != light.Fg {
		t.Error("Plain should be Fg in both branches")
	}
}

func hex(t *testing.T, s string) color.NRGBA {
	t.Helper()
	c, ok := ParseHex(s)
	if !ok {
		t.Fatalf("bad hex %q", s)
	}
	return c
}

// VS Code Dark+ puts findMatchBackground about 4.4:1 below the default
// foreground and about 2.4:1 above the editor background. A derived fill has to
// land in the same band for every token colour it is asked about.
func TestSearchFill_MatchesVSCodeLevels(t *testing.T) {
	bg := hex(t, "#1f1f1f")
	for _, tc := range []struct {
		name string
		text string
	}{
		{"default fg", "#cccccc"},
		{"json string", "#ce9178"},
		{"json key", "#9cdcfe"},
		{"number", "#b5cea8"},
		{"keyword", "#569cd6"},
		{"comment", "#6a9955"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := hex(t, tc.text)
			fill := SearchFill(text, bg, true)

			// VS Code's own fixed #515C6A ranges from 4.4:1 under the default
			// foreground down to about 2:1 under a green comment, so that is
			// the band a derived fill has to stay inside too.
			if got := ContrastRatio(fill, text); got < 1.9 {
				t.Errorf("fill %s vs text %s: contrast %.2f, the glyphs sink into it",
					HexFromColor(fill), tc.text, got)
			}
			if got := ContrastRatio(fill, bg); got < 2.0 || got > 2.9 {
				t.Errorf("fill %s vs bg: contrast %.2f, off the VS Code level",
					HexFromColor(fill), got)
			}
			if fill.A != 255 {
				t.Errorf("the current match must be opaque, got alpha %d", fill.A)
			}
		})
	}
}

func TestSearchFill_InactiveIsTheSameToneTranslucent(t *testing.T) {
	bg := hex(t, "#1f1f1f")
	text := hex(t, "#ce9178")
	active := SearchFill(text, bg, true)
	inactive := SearchFill(text, bg, false)

	if inactive.R != active.R || inactive.G != active.G || inactive.B != active.B {
		t.Errorf("inactive %s must be the same tone as active %s",
			HexFromColor(inactive), HexFromColor(active))
	}
	if inactive.A >= active.A || inactive.A == 0 {
		t.Errorf("inactive alpha %d must be translucent", inactive.A)
	}
	// composited over the page it still has to clear the background
	if got := ContrastRatio(Composite(inactive, bg), bg); got < 1.25 {
		t.Errorf("composited inactive fill vs bg: contrast %.2f, too faint to spot", got)
	}
}

// The whole point of deriving from the glyph colour is that the fill follows the
// hue of the token it covers instead of being one accent for everything.
func TestSearchFill_FollowsTheGlyphHue(t *testing.T) {
	bg := hex(t, "#1f1f1f")
	warm := SearchFill(hex(t, "#ce9178"), bg, true)
	cool := SearchFill(hex(t, "#9cdcfe"), bg, true)

	if warm.R <= warm.B {
		t.Errorf("a warm string colour should give a warm fill, got %s", HexFromColor(warm))
	}
	if cool.B <= cool.R {
		t.Errorf("a cool key colour should give a cool fill, got %s", HexFromColor(cool))
	}
}

func TestSearchFill_LightBackgroundStaysReadable(t *testing.T) {
	bg := hex(t, "#ffffff")
	text := hex(t, "#a31515") // VS Code light string colour
	fill := SearchFill(text, bg, true)

	if RelLuminance(fill) <= RelLuminance(text) {
		t.Errorf("on a light page the fill %s must be lighter than the text", HexFromColor(fill))
	}
	if got := ContrastRatio(fill, text); got < 3.0 {
		t.Errorf("fill %s vs text: contrast %.2f is unreadable", HexFromColor(fill), got)
	}
	if got := ContrastRatio(fill, bg); got < 1.4 {
		t.Errorf("fill %s vs a white page: contrast %.2f, invisible", HexFromColor(fill), got)
	}
}

func TestSearchFill_HandlesBlackText(t *testing.T) {
	fill := SearchFill(color.NRGBA{A: 255}, hex(t, "#ffffff"), true)
	if fill.A != 255 {
		t.Fatalf("expected an opaque fill, got %+v", fill)
	}
	if fill.R == 0 && fill.G == 0 && fill.B == 0 {
		t.Error("pure black text must not produce a black fill")
	}
}
