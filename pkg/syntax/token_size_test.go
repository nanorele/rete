package syntax

import (
	"strings"
	"testing"
	"unsafe"
)

func TestTokenStaysEightBytes(t *testing.T) {
	if got := unsafe.Sizeof(Token{}); got != 8 {
		t.Errorf("Token is %d bytes; a large document holds one per few bytes of text", got)
	}
}

func TestLongRunSplitsIntoWholeTokens(t *testing.T) {
	src := []byte(`{"k":"` + strings.Repeat("x", 200000) + `"}`)
	toks := TokenizeJSON(src)
	var covered int
	for i, tk := range toks {
		if int(tk.Start) < covered {
			t.Fatalf("token %d overlaps the previous one", i)
		}
		if int(tk.End()) > len(src) {
			t.Fatalf("token %d ends past the source", i)
		}
		covered = int(tk.End())
	}
	var stringBytes int
	for _, tk := range toks {
		if tk.Kind == TokString {
			stringBytes += int(tk.Len)
		}
	}
	if stringBytes < 200000 {
		t.Errorf("long string covered by %d bytes of tokens, want the whole 200002", stringBytes)
	}
}
