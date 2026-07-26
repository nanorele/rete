package theme

import (
	"image/color"
	"testing"

	"tracto/internal/model"
)

func TestIsValidID(t *testing.T) {
	customs := []model.CustomTheme{{ID: "my-custom"}}

	if !IsValidID("dark", nil) {
		t.Error("builtin 'dark' should be valid")
	}
	if !IsValidID("light", nil) {
		t.Error("builtin 'light' should be valid")
	}
	if !IsValidID("monokai", customs) {
		t.Error("builtin 'monokai' should be valid")
	}
	if !IsValidID("my-custom", customs) {
		t.Error("custom theme should be valid")
	}
	if IsValidID("my-custom", nil) {
		t.Error("custom not in list should be invalid")
	}
	if IsValidID("not-a-theme", customs) {
		t.Error("unknown id should be invalid")
	}
	if IsValidID("", customs) {
		t.Error("empty id should be invalid")
	}
	if IsValidID("", nil) {
		t.Error("empty id should be invalid (no customs)")
	}
}

func TestPaletteFor_Builtin(t *testing.T) {
	p := PaletteFor("dark", nil)
	if p.Bg != Dark.Bg {
		t.Errorf("expected Dark palette for 'dark', got Bg=%v", p.Bg)
	}
	p = PaletteFor("light", nil)
	if p.Bg != Light.Bg {
		t.Errorf("expected Light palette for 'light', got Bg=%v", p.Bg)
	}
}

func TestPaletteFor_UnknownFallsBackToDark(t *testing.T) {
	p := PaletteFor("does-not-exist", nil)
	if p.Bg != Dark.Bg {
		t.Errorf("unknown id should fall back to Dark, got Bg=%v", p.Bg)
	}
}

func TestPaletteFor_CustomWithKnownBase(t *testing.T) {
	customs := []model.CustomTheme{{
		ID:      "custom-light",
		Name:    "Custom Light",
		BasedOn: "light",
		Palette: model.ThemeColorOverride{Bg: "#123456"},
	}}
	p := PaletteFor("custom-light", customs)
	want := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 255}
	if p.Bg != want {
		t.Errorf("override not applied: got Bg=%v want %v", p.Bg, want)
	}

	if p.Fg != Light.Fg {
		t.Errorf("non-overridden Fg should come from base Light, got %v", p.Fg)
	}
}

func TestPaletteFor_CustomMissingBaseDefaultsToDark(t *testing.T) {
	customs := []model.CustomTheme{{
		ID:      "orphan",
		BasedOn: "no-such-theme",
	}}
	p := PaletteFor("orphan", customs)
	if p.Bg != Dark.Bg {
		t.Errorf("missing base should default to Dark, got Bg=%v", p.Bg)
	}
}

func TestPaletteFor_CustomSelfBasedOnDoesNotRecurse(t *testing.T) {

	customs := []model.CustomTheme{{ID: "loop", BasedOn: "loop"}}
	p := PaletteFor("loop", customs)
	if p.Bg != Dark.Bg {
		t.Errorf("self-based-on should fall back to Dark, got %v", p.Bg)
	}
}

func TestApplyOverride_Empty(t *testing.T) {
	got := ApplyOverride(Dark, model.ThemeColorOverride{})
	if got != Dark {
		t.Error("empty override should return base unchanged")
	}
}

func TestApplyOverride_PartialAndInvalidIgnored(t *testing.T) {
	ov := model.ThemeColorOverride{
		Bg:     "#112233",
		Fg:     "not-a-hex",
		Accent: "#abc",
	}
	got := ApplyOverride(Dark, ov)
	if got.Bg != (color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 255}) {
		t.Errorf("Bg override failed: %v", got.Bg)
	}
	if got.Fg != Dark.Fg {
		t.Errorf("invalid Fg should keep base, got %v", got.Fg)
	}
	if got.Accent != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 255}) {
		t.Errorf("3-char Accent failed: %v", got.Accent)
	}

	if got.BgDark != Dark.BgDark {
		t.Error("BgDark must be preserved")
	}
}

func TestApplyOverride_AllFields(t *testing.T) {
	ov := model.ThemeColorOverride{
		Bg: "#010101", BgDark: "#020202", BgField: "#030303",
		BgMenu: "#040404", BgPopup: "#050505", BgHover: "#060606",
		BgSecondary: "#070707", BgLoadMore: "#080808",
		BgDragHolder: "#090909", BgDragGhost: "#0a0a0a",
		Border: "#0b0b0b", BorderLight: "#0c0c0c",
		Fg: "#0d0d0d", FgMuted: "#0e0e0e", FgDim: "#0f0f0f",
		FgHint: "#101010", FgDisabled: "#111111", White: "#121212",
		Accent: "#131313", AccentHover: "#141414",
		AccentDim: "#151515", AccentFg: "#161616",
		Danger: "#171717", DangerFg: "#181818",
		Cancel: "#191919", CloseHover: "#1a1a1a",
		ScrollThumb: "#1b1b1b", VarFound: "#1c1c1c",
		VarMissing: "#1d1d1d", DividerLight: "#1e1e1e",
	}
	got := ApplyOverride(Palette{}, ov)
	if got.Bg.R != 0x01 || got.BgDark.R != 0x02 || got.DividerLight.R != 0x1e {
		t.Errorf("not all fields applied: Bg=%v BgDark=%v DividerLight=%v",
			got.Bg, got.BgDark, got.DividerLight)
	}
	if got.AccentFg != (color.NRGBA{R: 0x16, G: 0x16, B: 0x16, A: 255}) {
		t.Errorf("AccentFg not applied: %v", got.AccentFg)
	}
}

func TestApplySyntaxOverride_Empty(t *testing.T) {
	got := ApplySyntaxOverride(DarkPlusSyntax, model.ThemeSyntaxOverride{})
	if got != DarkPlusSyntax {
		t.Error("empty override should return base unchanged")
	}
}

func TestApplySyntaxOverride_Partial(t *testing.T) {
	ov := model.ThemeSyntaxOverride{
		Plain:    "#111111",
		Bracket0: "#222222",
		Type:     "garbage",
	}
	got := ApplySyntaxOverride(DarkPlusSyntax, ov)
	if got.Plain != (color.NRGBA{R: 0x11, G: 0x11, B: 0x11, A: 255}) {
		t.Errorf("Plain not overridden: %v", got.Plain)
	}
	if got.Brackets[0] != (color.NRGBA{R: 0x22, G: 0x22, B: 0x22, A: 255}) {
		t.Errorf("Bracket0 not overridden: %v", got.Brackets[0])
	}
	if got.Type != DarkPlusSyntax.Type {
		t.Errorf("invalid Type override should keep base, got %v", got.Type)
	}
	if got.String != DarkPlusSyntax.String {
		t.Error("untouched fields must be preserved")
	}
}

func TestApplySyntaxOverride_AllFields(t *testing.T) {
	ov := model.ThemeSyntaxOverride{
		Plain: "#010101", String: "#020202", Number: "#030303",
		Bool: "#040404", Null: "#050505", Key: "#060606",
		Punctuation: "#070707", Operator: "#080808",
		Keyword: "#090909", Type: "#0a0a0a", Comment: "#0b0b0b",
		Bracket0: "#0c0c0c", Bracket1: "#0d0d0d", Bracket2: "#0e0e0e",
	}
	got := ApplySyntaxOverride(SyntaxPalette{}, ov)
	if got.Plain.R != 0x01 || got.Comment.R != 0x0b {
		t.Errorf("not all syntax fields applied: %+v", got)
	}
	if got.Brackets[2].R != 0x0e {
		t.Errorf("Bracket2 not applied: %v", got.Brackets[2])
	}
}

func TestPaletteToOverride_RoundTripsHexFromColor(t *testing.T) {
	ov := PaletteToOverride(Dark)
	if ov.Bg != HexFromColor(Dark.Bg) {
		t.Errorf("Bg mismatch: %s vs %s", ov.Bg, HexFromColor(Dark.Bg))
	}
	if ov.Accent != HexFromColor(Dark.Accent) {
		t.Errorf("Accent mismatch: %s", ov.Accent)
	}
	if ov.DividerLight != HexFromColor(Dark.DividerLight) {
		t.Errorf("DividerLight mismatch: %s", ov.DividerLight)
	}
}

func TestSyntaxToOverride(t *testing.T) {
	ov := SyntaxToOverride(DarkPlusSyntax)
	if ov.Plain != HexFromColor(DarkPlusSyntax.Plain) {
		t.Errorf("Plain mismatch")
	}
	if ov.Bracket2 != HexFromColor(DarkPlusSyntax.Brackets[2]) {
		t.Errorf("Bracket2 mismatch: %s", ov.Bracket2)
	}
}

func TestApply_GlobalsAndDerivedFallbacks(t *testing.T) {

	saveBg, saveFg, saveAcc, saveAccFg := Bg, Fg, Accent, AccentFg
	saveDan, saveDanFg, saveSyn := Danger, DangerFg, Syntax
	defer func() {
		Bg, Fg, Accent, AccentFg = saveBg, saveFg, saveAcc, saveAccFg
		Danger, DangerFg, Syntax = saveDan, saveDanFg, saveSyn
	}()

	p := Palette{
		Bg:     color.NRGBA{R: 30, G: 30, B: 30, A: 255},
		Fg:     color.NRGBA{R: 200, G: 200, B: 200, A: 255},
		Accent: color.NRGBA{R: 14, G: 99, B: 156, A: 255},
		Danger: color.NRGBA{R: 200, G: 50, B: 50, A: 255},
	}
	Apply(p)
	if Bg != p.Bg || Fg != p.Fg {
		t.Error("Apply did not copy core fields")
	}
	if (AccentFg == color.NRGBA{}) {
		t.Error("AccentFg should be derived when zero")
	}
	if (DangerFg == color.NRGBA{}) {
		t.Error("DangerFg should be derived when zero")
	}
	if (Syntax == SyntaxPalette{}) {
		t.Error("Syntax should be derived when zero")
	}

	p2 := p
	p2.AccentFg = color.NRGBA{R: 7, G: 8, B: 9, A: 255}
	p2.DangerFg = color.NRGBA{R: 17, G: 18, B: 19, A: 255}
	p2.Syntax = MonokaiSyntax
	Apply(p2)
	if AccentFg != p2.AccentFg {
		t.Errorf("AccentFg should be preserved, got %v", AccentFg)
	}
	if DangerFg != p2.DangerFg {
		t.Errorf("DangerFg should be preserved, got %v", DangerFg)
	}
	if Syntax != MonokaiSyntax {
		t.Error("Syntax should be preserved")
	}
}

func TestApplyMethodAndMethodFor(t *testing.T) {
	save := MethodPalette{
		Get: MethodGet, Post: MethodPost, Put: MethodPut,
		Delete: MethodDelete, Head: MethodHead, Patch: MethodPatch,
		Options: MethodOptions, Fallback: MethodFallback,
	}
	defer ApplyMethod(save)

	ApplyMethod(MethodLight)
	if MethodGet != MethodLight.Get || MethodFallback != MethodLight.Fallback {
		t.Error("ApplyMethod did not propagate to globals")
	}
	ApplyMethod(MethodDark)
	if MethodGet != MethodDark.Get {
		t.Error("ApplyMethod (dark) did not propagate")
	}

	if MethodFor(color.NRGBA{R: 240, G: 240, B: 240, A: 255}) != MethodLight {
		t.Error("light bg should pick MethodLight")
	}
	if MethodFor(color.NRGBA{R: 20, G: 20, B: 20, A: 255}) != MethodDark {
		t.Error("dark bg should pick MethodDark")
	}
}

func TestPaletteColorTable_RoundTrip(t *testing.T) {
	const sentinel = "#ff00ff"
	wantBase := color.NRGBA{R: 0xff, G: 0x00, B: 0xff, A: 0xff}
	seenLabels := map[string]bool{}
	for i, e := range PaletteColorTable {
		if e.Label == "" {
			t.Errorf("entry %d has empty Label", i)
		}
		if seenLabels[e.Label] {
			t.Errorf("duplicate Label %q at index %d", e.Label, i)
		}
		seenLabels[e.Label] = true
		if e.GetBase == nil || e.GetOv == nil || e.SetOv == nil {
			t.Fatalf("entry %d (%s) has nil closure", i, e.Label)
		}

		var p Palette
		var pp = &p
		setAllPaletteFields(pp, func(idx int) color.NRGBA {
			if idx == i {
				return wantBase
			}
			return color.NRGBA{R: byte(idx + 1), A: 255}
		})
		if got := e.GetBase(p); got != wantBase {
			t.Errorf("[%d %s] GetBase: got %v want %v", i, e.Label, got, wantBase)
		}

		var ov model.ThemeColorOverride
		e.SetOv(&ov, sentinel)
		if got := e.GetOv(ov); got != sentinel {
			t.Errorf("[%d %s] SetOv/GetOv round-trip: got %q want %q",
				i, e.Label, got, sentinel)
		}

		var empty model.ThemeColorOverride
		if got := e.GetOv(empty); got != "" {
			t.Errorf("[%d %s] GetOv(empty) = %q, want \"\"", i, e.Label, got)
		}
	}
}

func TestTokenColorTable_RoundTrip(t *testing.T) {
	const sentinel = "#abcdef"
	wantBase := color.NRGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0xff}
	seenLabels := map[string]bool{}
	for i, e := range TokenColorTable {
		if e.Label == "" {
			t.Errorf("entry %d has empty Label", i)
		}
		if seenLabels[e.Label] {
			t.Errorf("duplicate Label %q at index %d", e.Label, i)
		}
		seenLabels[e.Label] = true
		if e.GetBase == nil || e.GetOv == nil || e.SetOv == nil {
			t.Fatalf("entry %d (%s) has nil closure", i, e.Label)
		}

		var s SyntaxPalette
		setAllSyntaxFields(&s, func(idx int) color.NRGBA {
			if idx == i {
				return wantBase
			}
			return color.NRGBA{R: byte(idx + 1), A: 255}
		})
		if got := e.GetBase(s); got != wantBase {
			t.Errorf("[%d %s] GetBase: got %v want %v", i, e.Label, got, wantBase)
		}

		var ov model.ThemeSyntaxOverride
		e.SetOv(&ov, sentinel)
		if got := e.GetOv(ov); got != sentinel {
			t.Errorf("[%d %s] SetOv/GetOv round-trip: got %q want %q",
				i, e.Label, got, sentinel)
		}

		var empty model.ThemeSyntaxOverride
		if got := e.GetOv(empty); got != "" {
			t.Errorf("[%d %s] GetOv(empty) = %q, want \"\"", i, e.Label, got)
		}
	}
}

func TestPaletteColorTable_ApplyOverrideAgreement(t *testing.T) {
	const hex = "#123456"
	want := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff}
	for i, e := range PaletteColorTable {
		var ov model.ThemeColorOverride
		e.SetOv(&ov, hex)
		p := ApplyOverride(Palette{}, ov)
		if got := e.GetBase(p); got != want {
			t.Errorf("[%d %s] SetOv->ApplyOverride->GetBase mismatch: got %v want %v",
				i, e.Label, got, want)
		}
	}
}

func TestTokenColorTable_ApplySyntaxOverrideAgreement(t *testing.T) {
	const hex = "#654321"
	want := color.NRGBA{R: 0x65, G: 0x43, B: 0x21, A: 0xff}
	for i, e := range TokenColorTable {
		var ov model.ThemeSyntaxOverride
		e.SetOv(&ov, hex)
		s := ApplySyntaxOverride(SyntaxPalette{}, ov)
		if got := e.GetBase(s); got != want {
			t.Errorf("[%d %s] SetOv->ApplySyntaxOverride->GetBase mismatch: got %v want %v",
				i, e.Label, got, want)
		}
	}
}

func setAllPaletteFields(p *Palette, f func(idx int) color.NRGBA) {
	setters := []func(c color.NRGBA){
		func(c color.NRGBA) { p.Bg = c },
		func(c color.NRGBA) { p.BgDark = c },
		func(c color.NRGBA) { p.BgField = c },
		func(c color.NRGBA) { p.BgMenu = c },
		func(c color.NRGBA) { p.BgPopup = c },
		func(c color.NRGBA) { p.BgHover = c },
		func(c color.NRGBA) { p.BgSecondary = c },
		func(c color.NRGBA) { p.BgLoadMore = c },
		func(c color.NRGBA) { p.BgDragHolder = c },
		func(c color.NRGBA) { p.BgDragGhost = c },
		func(c color.NRGBA) { p.Border = c },
		func(c color.NRGBA) { p.BorderLight = c },
		func(c color.NRGBA) { p.Fg = c },
		func(c color.NRGBA) { p.FgMuted = c },
		func(c color.NRGBA) { p.FgDim = c },
		func(c color.NRGBA) { p.FgHint = c },
		func(c color.NRGBA) { p.FgDisabled = c },
		func(c color.NRGBA) { p.White = c },
		func(c color.NRGBA) { p.Accent = c },
		func(c color.NRGBA) { p.AccentHover = c },
		func(c color.NRGBA) { p.AccentDim = c },
		func(c color.NRGBA) { p.AccentFg = c },
		func(c color.NRGBA) { p.Danger = c },
		func(c color.NRGBA) { p.DangerFg = c },
		func(c color.NRGBA) { p.Cancel = c },
		func(c color.NRGBA) { p.CloseHover = c },
		func(c color.NRGBA) { p.ScrollThumb = c },
		func(c color.NRGBA) { p.VarFound = c },
		func(c color.NRGBA) { p.VarMissing = c },
		func(c color.NRGBA) { p.DividerLight = c },
	}
	for i, s := range setters {
		s(f(i))
	}
}

func setAllSyntaxFields(s *SyntaxPalette, f func(idx int) color.NRGBA) {
	setters := []func(c color.NRGBA){
		func(c color.NRGBA) { s.Plain = c },
		func(c color.NRGBA) { s.String = c },
		func(c color.NRGBA) { s.Number = c },
		func(c color.NRGBA) { s.Bool = c },
		func(c color.NRGBA) { s.Null = c },
		func(c color.NRGBA) { s.Key = c },
		func(c color.NRGBA) { s.Punctuation = c },
		func(c color.NRGBA) { s.Operator = c },
		func(c color.NRGBA) { s.Keyword = c },
		func(c color.NRGBA) { s.Type = c },
		func(c color.NRGBA) { s.Comment = c },
		func(c color.NRGBA) { s.Brackets[0] = c },
		func(c color.NRGBA) { s.Brackets[1] = c },
		func(c color.NRGBA) { s.Brackets[2] = c },
	}
	for i, st := range setters {
		st(f(i))
	}
}

func TestRegistry_AllIDsValid(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("Registry must be non-empty")
	}
	seen := map[string]bool{}
	for _, d := range Registry {
		if d.ID == "" {
			t.Errorf("registry entry has empty ID: %+v", d)
		}
		if seen[d.ID] {
			t.Errorf("duplicate ID in Registry: %s", d.ID)
		}
		seen[d.ID] = true
		if !IsValidID(d.ID, nil) {
			t.Errorf("IsValidID should accept %q", d.ID)
		}
	}
}
