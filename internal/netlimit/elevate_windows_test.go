//go:build windows

package netlimit

import "testing"

func TestQuoteArg(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "server", "server"},
		{"empty string", "", `""`},
		{"flag", "--elevated", "--elevated"},
		{"path without spaces", `C:\tools\app.exe`, `C:\tools\app.exe`},
		{"trailing backslash without spaces", `C:\tools\`, `C:\tools\`},
		{"space", "hello world", `"hello world"`},
		{"tab", "a\tb", "\"a\tb\""},
		{"newline", "a\nb", "\"a\nb\""},
		{"vertical tab", "a\vb", "\"a\vb\""},
		{"embedded quote", `a"b`, `"a\"b"`},
		{"backslash before quote is doubled", `a\"b`, `"a\\\"b"`},
		{"two backslashes before quote", `a\\"b`, `"a\\\\\"b"`},
		{"path with spaces and trailing backslash", `C:\my dir\`, `"C:\my dir\\"`},
		{"path with spaces", `C:\Program Files\app.exe`, `"C:\Program Files\app.exe"`},
		{"interior backslash with space", `a\ b`, `"a\ b"`},
		{"only a quote", `"`, `"\""`},
		{"only backslashes with space", `\\ `, `"\\ "`},
		{"unicode passes through", "прокси", "прокси"},
		{"unicode with space", "про кси", `"про кси"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := quoteArg(tt.in); got != tt.want {
				t.Fatalf("quoteArg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestJoinCmdLine(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"nil", nil, ""},
		{"empty slice", []string{}, ""},
		{"single plain", []string{"a"}, "a"},
		{"single empty arg", []string{""}, `""`},
		{"two plain", []string{"a", "b"}, "a b"},
		{"quoted middle", []string{"a", "b c", "d"}, `a "b c" d`},
		{"path argument", []string{"--file", `C:\Program Files\x.txt`}, `--file "C:\Program Files\x.txt"`},
		{"empty among plain", []string{"a", "", "b"}, `a "" b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinCmdLine(tt.args); got != tt.want {
				t.Fatalf("joinCmdLine(%q) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestHasArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
		ok   bool
	}{
		{"nil slice", nil, "-x", false},
		{"present first", []string{"-x", "-y"}, "-x", true},
		{"present last", []string{"-y", "-x"}, "-x", true},
		{"absent", []string{"-y", "-z"}, "-x", false},
		{"case sensitive", []string{"-X"}, "-x", false},
		{"prefix is not a match", []string{"-xx"}, "-x", false},
		{"empty needle absent", []string{"-x"}, "", false},
		{"empty needle present", []string{"-x", ""}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasArg(tt.args, tt.want); got != tt.ok {
				t.Fatalf("hasArg(%q, %q) = %v, want %v", tt.args, tt.want, got, tt.ok)
			}
		})
	}
}

func TestIsElevatedIsCached(t *testing.T) {
	first := IsElevated()
	if second := IsElevated(); second != first {
		t.Fatalf("IsElevated returned %v then %v", first, second)
	}
}

func TestErrUACDenied(t *testing.T) {
	if ErrUACDenied == nil || ErrUACDenied.Error() == "" {
		t.Fatal("ErrUACDenied must carry a message")
	}
}
