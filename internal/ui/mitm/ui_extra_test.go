package mitm

import (
	"strings"
	"testing"
	"time"
)

func TestShortFingerprint(t *testing.T) {
	if got := shortFingerprint(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	short := "ab:cd:ef"
	if got := shortFingerprint(short); got != short {
		t.Errorf("short passthrough: got %q want %q", got, short)
	}

	bd := strings.Repeat("a", 17)
	if got := shortFingerprint(bd); got != bd {
		t.Errorf("len17 passthrough: got %q", got)
	}

	in := "0123456789abcdef00"
	got := shortFingerprint(in)
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in %q", got)
	}
	if !strings.HasPrefix(got, in[:8]) || !strings.HasSuffix(got, in[len(in)-8:]) {
		t.Errorf("expected first 8 and last 8 chars in %q", got)
	}
}

func TestGenLabel(t *testing.T) {
	if genLabel(nil) != "Generate CA" {
		t.Errorf("nil CA should give Generate CA")
	}

	ca, err := GenerateCA()
	if err != nil {
		t.Skipf("GenerateCA failed: %v", err)
	}
	if genLabel(ca) != "Regenerate" {
		t.Errorf("non-nil CA should give Regenerate")
	}
}

func TestChromeEdgeAndFirefoxSteps(t *testing.T) {
	for _, installed := range []bool{true, false} {
		steps := chromeEdgeSteps(installed)
		if len(steps) == 0 {
			t.Errorf("expected chromeEdgeSteps non-empty for %v", installed)
		}
		joined := strings.ToLower(strings.Join(steps, " "))
		if installed && !strings.Contains(joined, "trusted") {
			t.Errorf("installed=true should mention trusted")
		}
		if !installed && !strings.Contains(joined, "install") {
			t.Errorf("installed=false should mention install")
		}
	}
	ff := firefoxSteps()
	if len(ff) < 3 {
		t.Errorf("firefoxSteps too short: %d", len(ff))
	}
	if !strings.Contains(strings.ToLower(strings.Join(ff, " ")), "firefox") {
		t.Errorf("firefoxSteps should mention firefox")
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1024 * 1024, "1.0M"},
		{1536, "1.5K"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q want %q", c.n, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	f := &Flow{}
	if got := humanDuration(f); got != "-" {
		t.Errorf("zero Started: got %q want -", got)
	}
	now := time.Now()
	f = &Flow{Started: now.Add(-500 * time.Microsecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "µs") {
		t.Errorf("expected microseconds suffix, got %q", got)
	}
	f = &Flow{Started: now.Add(-50 * time.Millisecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "ms") {
		t.Errorf("expected ms, got %q", got)
	}
	f = &Flow{Started: now.Add(-2 * time.Second), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "s") {
		t.Errorf("expected s, got %q", got)
	}

	f = &Flow{Started: time.Now().Add(-10 * time.Millisecond)}
	if got := humanDuration(f); got == "-" {
		t.Errorf("live flow should compute, got %q", got)
	}
}

func TestTunnelStatusText(t *testing.T) {
	if got := tunnelStatusText(&Flow{}); got != "…" {
		t.Errorf("empty: got %q", got)
	}
	if got := tunnelStatusText(&Flow{Status: "200 OK"}); got != "200 OK" {
		t.Errorf("status only: got %q", got)
	}
	got := tunnelStatusText(&Flow{Status: "200 OK", Error: "boom"})
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "boom") {
		t.Errorf("status+err: got %q", got)
	}

}

func TestMITMStatusLine(t *testing.T) {
	now := time.Now()
	f := &Flow{Status: "200 OK", ReqSize: 100, RespSize: 200, Started: now.Add(-20 * time.Millisecond), Ended: now}
	got := statusLine(f)
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "req") || !strings.Contains(got, "resp") {
		t.Errorf("status line missing components: %q", got)
	}

	f2 := &Flow{}
	got2 := statusLine(f2)
	if got2 != "-" {
		t.Errorf("expected just '-' for empty flow, got %q", got2)
	}
}

func TestMITMFindByID(t *testing.T) {
	s := NewStore()
	if got := s.FindByID(1); got != nil {
		t.Errorf("empty store: expected nil")
	}
	added := s.Add(&Flow{Method: "GET", Host: "h"})
	if got := s.FindByID(added.ID); got == nil || got.ID != added.ID {
		t.Errorf("expected to find by ID %d", added.ID)
	}
	if got := s.FindByID(9999); got != nil {
		t.Errorf("expected nil for missing ID")
	}
}

func TestConsumeStartupFlags_NoAdmin(t *testing.T) {
	setupTestConfigDir(t)
	var st UIState
	st.Ensure()

	st.AutoStart = true
	st.AutoInstallCA = true
	st.AutoRemoveCA = true
	st.consumeStartupFlags()
	if st.AutoStart || st.AutoInstallCA || st.AutoRemoveCA {
		t.Errorf("startup flags should be consumed (reset to false) regardless of admin state")
	}
}
