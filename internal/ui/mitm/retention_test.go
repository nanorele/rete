package mitm

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFlowStoreCapsRetainedBytes(t *testing.T) {
	s := NewStore()
	const chunk = 8 << 20
	for i := 0; i < 64; i++ {
		s.Add(&Flow{Host: "h", RespBody: make([]byte, chunk)})
	}
	var total int64
	for _, f := range s.Snapshot() {
		total += int64(len(f.ReqBody) + len(f.RespBody))
	}
	if total > MaxFlowBytes {
		t.Fatalf("retained %d bytes, cap is %d", total, MaxFlowBytes)
	}
	if s.Len() == 0 {
		t.Fatal("byte cap must not empty the store")
	}
}

func TestWSStoreCapsRetainedBytes(t *testing.T) {
	s := NewWSStore()
	const chunk = 4 << 20
	for i := 0; i < 64; i++ {
		s.Add(&WSMessage{Payload: make([]byte, chunk)})
	}
	var total int
	for _, m := range s.Snapshot() {
		total += len(m.Payload)
	}
	if int64(total) > maxWSBytes {
		t.Fatalf("retained %d bytes, cap is %d", total, maxWSBytes)
	}
	if s.Len() == 0 {
		t.Fatal("byte cap must not empty the store")
	}
}

func TestStoreDropsReferencesBeyondLength(t *testing.T) {
	s := NewStore()
	for i := 0; i < MaxFlows+16; i++ {
		s.Add(&Flow{Host: "h", RespBody: make([]byte, 1024)})
	}
	s.mu.RLock()
	tail := s.flows[len(s.flows):cap(s.flows)]
	s.mu.RUnlock()
	for i, f := range tail {
		if f != nil {
			t.Fatalf("trimmed slot %d still references flow %d", i, f.ID)
		}
	}
}

func TestSnapshotMetaCachedUntilStoreChanges(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a", RespBody: []byte("body")})
	first := s.SnapshotMeta()
	if &first[0] != &s.SnapshotMeta()[0] {
		t.Fatal("unchanged store must reuse the meta snapshot")
	}
	if first[0].RespBody != nil || first[0].RespHeaders != nil {
		t.Fatal("meta snapshot must not carry bodies or headers")
	}
	s.Add(&Flow{Host: "b"})
	if len(s.SnapshotMeta()) != 2 {
		t.Fatal("meta snapshot must refresh after Add")
	}
	s.SetAnnotation(1, "red", "note")
	if got := s.SnapshotMeta()[0].Highlight; got != "red" {
		t.Fatalf("meta snapshot stale after SetAnnotation: %q", got)
	}
}

func TestFindByIDSharesFinishedBodies(t *testing.T) {
	s := NewStore()
	live := s.Add(&Flow{Host: "live", RespBody: []byte("partial")})
	got := s.FindByID(live.ID)
	if &got.RespBody[0] == &live.RespBody[0] {
		t.Fatal("a live flow must be copied, the proxy may still write to it")
	}

	s.Update(live, func() { live.Ended = live.Started.Add(1) })
	got = s.FindByID(live.ID)
	if &got.RespBody[0] != &live.RespBody[0] {
		t.Fatal("a finished flow should share its body instead of copying per frame")
	}
}

func TestSplitPaneLinesPreservesContent(t *testing.T) {
	cases := []string{
		"short\nlines\nhere",
		strings.Repeat("x", paneLineChunk*3+7),
		strings.Repeat("привет", paneLineChunk),
		"head\n" + strings.Repeat("y", paneLineChunk+1) + "\ntail",
	}
	for _, txt := range cases {
		lines := splitPaneLines(txt)
		for i, ln := range lines {
			if len(ln) > paneLineChunk {
				t.Fatalf("line %d is %d bytes, over the %d chunk", i, len(ln), paneLineChunk)
			}
			if !utf8.ValidString(ln) {
				t.Fatalf("line %d split mid-rune", i)
			}
		}
		var b strings.Builder
		for i, ln := range strings.Split(txt, "\n") {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(ln)
		}
		if joinPaneLines(lines, txt) != b.String() {
			t.Fatalf("chunking lost content for a %d byte input", len(txt))
		}
	}
}

// joinPaneLines rebuilds the original text: chunks of one logical line join
// with nothing, separate logical lines with a newline.
func joinPaneLines(lines []string, orig string) string {
	want := strings.Split(orig, "\n")
	var b strings.Builder
	li := 0
	for i, w := range want {
		if i > 0 {
			b.WriteString("\n")
		}
		n := 0
		for n < len(w) && li < len(lines) {
			b.WriteString(lines[li])
			n += len(lines[li])
			li++
		}
		if len(w) == 0 && li < len(lines) && lines[li] == "" {
			li++
		}
	}
	return b.String()
}

func TestWSPreviewIsBounded(t *testing.T) {
	payload := []byte(strings.Repeat("ab\n", 1<<16))
	got := wsPreview(payload)
	if n := utf8.RuneCountInString(got); n != wsPreviewRunes {
		t.Fatalf("preview is %d runes, want %d", n, wsPreviewRunes)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("preview must fold newlines into spaces")
	}
	if got := wsPreview([]byte("привет мир")); got != "привет мир" {
		t.Fatalf("short multibyte preview = %q", got)
	}
}

func TestWSMatchesKeepsFilterSemantics(t *testing.T) {
	m := &WSMessage{URL: "wss://Example.com/socket", Opcode: 0x1, Payload: []byte("Hello WORLD")}
	oldMatch := func(q string) bool {
		hay := strings.ToLower(m.URL + " " + string(m.Payload) + " " + WSOpcodeName(m.Opcode))
		return strings.Contains(hay, q)
	}
	for _, q := range []string{
		"", "example", "socket", "world", "hello world", "text",
		"socket hello", "d text", "missing", "привет",
	} {
		if got, want := wsMatches(m, q), oldMatch(q); got != want {
			t.Errorf("wsMatches(%q) = %v, want %v", q, got, want)
		}
	}
}
