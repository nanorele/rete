//go:build windows

package mitm

import (
	"strings"
	"testing"
)

func TestHasArg(t *testing.T) {
	if hasArg(nil, "x") {
		t.Error("hasArg on nil slice should be false")
	}
	if !hasArg([]string{"--foo", "--bar"}, "--bar") {
		t.Error("should find --bar")
	}
	if hasArg([]string{"--foo"}, "--bar") {
		t.Error("should not find --bar")
	}

	if hasArg([]string{"--mitm-start-now"}, "--mitm-start") {
		t.Error("hasArg must be exact-match, not substring")
	}
}

func TestQuoteArg(t *testing.T) {
	cases := []struct {
		in, want string
	}{

		{"simple", "simple"},
		{"a-b_c", "a-b_c"},

		{"with space", `"with space"`},

		{"with\ttab", "\"with\ttab\""},

		{`a"b`, `"a\"b"`},

		{"", `""`},

		{`mid\path`, `mid\path`},

		{`with space\`, `"with space\\"`},
	}
	for _, c := range cases {
		got := quoteArg(c.in)
		if got != c.want {
			t.Errorf("quoteArg(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	got := quoteArg(`pre\"post`)
	const want = `"pre\\\"post"`
	if got != want {
		t.Errorf("quoteArg(`pre\\\"post`) = %q, want %q (CommandLineToArgvW spec)", got, want)
	}
}

func TestJoinCmdLine(t *testing.T) {
	got := joinCmdLine([]string{"a", "b c", `d"e`})
	want := `a "b c" "d\"e"`
	if got != want {
		t.Errorf("joinCmdLine = %q, want %q", got, want)
	}

	if joinCmdLine(nil) != "" {
		t.Errorf("nil → %q", joinCmdLine(nil))
	}

	out := joinCmdLine([]string{"foo"})
	if strings.HasPrefix(out, " ") || strings.HasSuffix(out, " ") {
		t.Errorf("stray space: %q", out)
	}
}

func TestCanRequestElevation(t *testing.T) {

	if !CanRequestElevation() {
		t.Error("expected true on Windows")
	}
}

func TestIsAdminDoesNotPanic(t *testing.T) {

	a := IsAdmin()
	b := IsAdmin()
	if a != b {
		t.Errorf("IsAdmin returned inconsistent values: %v then %v", a, b)
	}
}

func TestFirefoxEnterpriseRootsEnabledReadsRegistry(t *testing.T) {

	_ = FirefoxEnterpriseRootsEnabled()
}

func TestUACShieldPNGDecodes(t *testing.T) {
	b, err := UACShieldPNG()
	if err != nil {

		t.Skipf("UAC shield unavailable in this environment: %v", err)
	}
	if len(b) < 8 {
		t.Fatalf("too short: %d bytes", len(b))
	}

	want := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := range want {
		if b[i] != want[i] {
			t.Fatalf("missing PNG signature byte %d: got 0x%02X, want 0x%02X", i, b[i], want[i])
		}
	}

	b2, err2 := UACShieldPNG()
	if err2 != err {
		t.Fatalf("second call error differs: %v vs %v", err, err2)
	}
	if &b[0] != &b2[0] {
		t.Error("expected cached identical slice header")
	}
}
