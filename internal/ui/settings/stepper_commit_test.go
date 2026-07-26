package settings

import (
	"testing"

	"github.com/nanorele/gio/widget"
)

func markBlurred(t *testing.T, ed *widget.Editor) {
	t.Helper()
	stepperWasFocused[ed] = true
	t.Cleanup(func() { delete(stepperWasFocused, ed) })
}

func TestIntStepperUpdate_BlurCommits(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		current  int
		lo, hi   int
		wantVal  int
		wantOK   bool
		wantText string
	}{
		{"in-range", "20", 14, 10, 28, 20, true, "20"},
		{"clamped-high", "999", 14, 10, 28, 28, true, "28"},
		{"clamped-low", "-5", 14, 10, 28, 10, true, "10"},
		{"percent-suffix", "70%", 50, 20, 80, 70, true, "70"},
		{"whitespace", "  17  ", 14, 10, 28, 17, true, "17"},
		{"same-value", "14", 14, 10, 28, 14, false, "14"},
		{"unparseable", "abc", 14, 10, 28, 14, false, "14"},
		{"empty", "", 14, 10, 28, 14, false, "14"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := makeGtx(200, 30)
			ed := new(widget.Editor)
			ed.SetText(tc.typed)
			markBlurred(t, ed)

			v, ok := intStepperUpdate(gtx, ed, tc.current, tc.lo, tc.hi)
			if v != tc.wantVal || ok != tc.wantOK {
				t.Fatalf("got (%d, %v), want (%d, %v)", v, ok, tc.wantVal, tc.wantOK)
			}
			if ed.Text() != tc.wantText {
				t.Errorf("editor text = %q, want %q", ed.Text(), tc.wantText)
			}
		})
	}
}

func TestIntStepperUpdate_BlurLatchClearsAfterCommit(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := new(widget.Editor)
	ed.SetText("22")
	markBlurred(t, ed)

	if v, ok := intStepperUpdate(gtx, ed, 14, 10, 28); !ok || v != 22 {
		t.Fatalf("first call: got (%d, %v), want (22, true)", v, ok)
	}
	if stepperWasFocused[ed] {
		t.Fatal("blur latch should be cleared after the commit frame")
	}
	if _, ok := intStepperUpdate(gtx, ed, 22, 10, 28); ok {
		t.Fatal("second call must not re-commit")
	}
}

func TestFloatStepperUpdate_BlurCommits(t *testing.T) {
	cases := []struct {
		name     string
		typed    string
		current  float32
		lo, hi   float32
		format   string
		mult     float32
		wantVal  float32
		wantOK   bool
		wantText string
	}{
		{"scale-in-range", "1.50", 1.0, 0.75, 2.0, "%.2f", 1.0, 1.5, true, "1.50"},
		{"scale-clamp-high", "9", 1.0, 0.75, 2.0, "%.2f", 1.0, 2.0, true, "2.00"},
		{"scale-clamp-low", "0.1", 1.0, 0.75, 2.0, "%.2f", 1.0, 0.75, true, "0.75"},
		{"scale-x-suffix", "1.25x", 1.0, 0.75, 2.0, "%.2f", 1.0, 1.25, true, "1.25"},
		{"ratio-percent", "70%", 0.5, 0.2, 0.8, "%.0f", 100, 0.7, true, "70"},
		{"ratio-clamp-high", "95", 0.5, 0.2, 0.8, "%.0f", 100, 0.8, true, "80"},
		{"ratio-clamp-low", "5", 0.5, 0.2, 0.8, "%.0f", 100, 0.2, true, "20"},
		{"same-value", "50", 0.5, 0.2, 0.8, "%.0f", 100, 0.5, false, "50"},
		{"unparseable", "wide", 0.5, 0.2, 0.8, "%.0f", 100, 0.5, false, "50"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gtx := makeGtx(200, 30)
			ed := new(widget.Editor)
			ed.SetText(tc.typed)
			markBlurred(t, ed)

			v, ok := floatStepperUpdate(gtx, ed, tc.current, tc.lo, tc.hi, tc.format, tc.mult)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (v=%v)", ok, tc.wantOK, v)
			}
			if d := v - tc.wantVal; d > 1e-6 || d < -1e-6 {
				t.Fatalf("value = %v, want %v", v, tc.wantVal)
			}
			if ed.Text() != tc.wantText {
				t.Errorf("editor text = %q, want %q", ed.Text(), tc.wantText)
			}
		})
	}
}

func TestStepperUpdate_ForcesSingleLineSubmit(t *testing.T) {
	gtx := makeGtx(200, 30)
	ed := new(widget.Editor)
	intStepperUpdate(gtx, ed, 1, 0, 10)
	if !ed.SingleLine || !ed.Submit {
		t.Error("intStepperUpdate must force SingleLine and Submit")
	}
	fed := new(widget.Editor)
	floatStepperUpdate(gtx, fed, 1, 0, 10, "%.2f", 1)
	if !fed.SingleLine || !fed.Submit {
		t.Error("floatStepperUpdate must force SingleLine and Submit")
	}
}

func TestDefaultLabelHelpers(t *testing.T) {
	if got := defaultShownHidden(true); got != "Default: hidden." {
		t.Errorf("defaultShownHidden(true) = %q", got)
	}
	if got := defaultShownHidden(false); got != "Default: shown." {
		t.Errorf("defaultShownHidden(false) = %q", got)
	}
	if got := defaultOnOff(true); got != "Default: on." {
		t.Errorf("defaultOnOff(true) = %q", got)
	}
	if got := defaultOnOff(false); got != "Default: off." {
		t.Errorf("defaultOnOff(false) = %q", got)
	}
	if got := defaultTimeout(0, "never"); got != "Default: never." {
		t.Errorf("defaultTimeout(0) = %q", got)
	}
	if got := defaultTimeout(30, "never"); got != "Default: 30 s." {
		t.Errorf("defaultTimeout(30) = %q", got)
	}
}

func TestTextToHeaders_SkipsBlankKeys(t *testing.T) {
	got := textToHeaders("   : value\n:\nOK: 1\n")
	if len(got) != 1 || got[0].Key != "OK" || got[0].Value != "1" {
		t.Fatalf("textToHeaders = %+v, want a single OK:1 header", got)
	}
	if textToHeaders("   \n\n") != nil {
		t.Error("blank input should yield nil")
	}
}
