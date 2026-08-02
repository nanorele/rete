package theme

import (
	"image/color"
	"testing"
)

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
