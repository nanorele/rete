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
	// Exact-match (no substring).
	if hasArg([]string{"--mitm-start-now"}, "--mitm-start") {
		t.Error("hasArg must be exact-match, not substring")
	}
}

func TestQuoteArg(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// No special chars → unquoted passthrough.
		{"simple", "simple"},
		{"a-b_c", "a-b_c"},
		// Space forces quoting.
		{"with space", `"with space"`},
		// Tab forces quoting.
		{"with\ttab", "\"with\ttab\""},
		// Embedded double-quote is escaped.
		{`a"b`, `"a\"b"`},
		// Empty string gets quoted (so CommandLineToArgvW gets an empty arg).
		{"", `""`},
		// Trailing backslash without other specials is left unquoted — fine,
		// since a bare backslash has no special meaning when not preceding a quote.
		{`mid\path`, `mid\path`},
		// Trailing backslash before closing quote is doubled.
		{`with space\`, `"with space\\"`},
	}
	for _, c := range cases {
		got := quoteArg(c.in)
		if got != c.want {
			t.Errorf("quoteArg(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// Per CommandLineToArgvW: N backslashes followed by a literal '"' must
	// emit 2N+1 backslashes. For input `pre\"post` (1 backslash + '"'),
	// expected output is `"pre\\\"post"` (3 backslashes + escaped quote).
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
	// Empty slice → empty string.
	if joinCmdLine(nil) != "" {
		t.Errorf("nil → %q", joinCmdLine(nil))
	}
	// Single arg → no leading/trailing space.
	out := joinCmdLine([]string{"foo"})
	if strings.HasPrefix(out, " ") || strings.HasSuffix(out, " ") {
		t.Errorf("stray space: %q", out)
	}
}

func TestCanRequestElevation(t *testing.T) {
	// On Windows we always allow the UAC prompt — failure is reported later.
	if !CanRequestElevation() {
		t.Error("expected true on Windows")
	}
}

func TestIsAdminDoesNotPanic(t *testing.T) {
	// We don't know whether the test runner is elevated; just ensure the
	// call returns and is cached/stable across invocations.
	a := IsAdmin()
	b := IsAdmin()
	if a != b {
		t.Errorf("IsAdmin returned inconsistent values: %v then %v", a, b)
	}
}

func TestFirefoxEnterpriseRootsEnabledReadsRegistry(t *testing.T) {
	// Just exercises the registry read path; the value depends on system
	// policy and we have no control over it. Test must not panic.
	_ = FirefoxEnterpriseRootsEnabled()
}

func TestUACShieldPNGDecodes(t *testing.T) {
	b, err := UACShieldPNG()
	if err != nil {
		// May fail if the test environment lacks the desktop session APIs.
		t.Skipf("UAC shield unavailable in this environment: %v", err)
	}
	if len(b) < 8 {
		t.Fatalf("too short: %d bytes", len(b))
	}
	// PNG signature: 89 50 4E 47 0D 0A 1A 0A
	want := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := range want {
		if b[i] != want[i] {
			t.Fatalf("missing PNG signature byte %d: got 0x%02X, want 0x%02X", i, b[i], want[i])
		}
	}
	// Second call returns cached identical bytes.
	b2, err2 := UACShieldPNG()
	if err2 != err {
		t.Fatalf("second call error differs: %v vs %v", err, err2)
	}
	if &b[0] != &b2[0] {
		t.Error("expected cached identical slice header")
	}
}
