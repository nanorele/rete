package theme

import (
	"image/color"
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
