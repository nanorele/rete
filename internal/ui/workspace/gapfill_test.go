package workspace

import (
	"bytes"
	"image"
	"time"
	"unicode/utf8"

	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/key"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/op/clip"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"

	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tracto/internal/model"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"
	"tracto/pkg/syntax"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

func TestClipSpansToVars_NoVarsIsIdentity(t *testing.T) {
	spans := []widgets.ColoredSpan{{Start: 0, End: 5}, {Start: 6, End: 9}}
	got := clipSpansToVars(spans, []byte("hello world"))
	if len(got) != 2 || got[0] != spans[0] || got[1] != spans[1] {
		t.Fatalf("spans should pass through unchanged: %+v", got)
	}
}

func TestClipSpansToVars_UnterminatedVarIsIgnored(t *testing.T) {
	spans := []widgets.ColoredSpan{{Start: 0, End: 10}}
	got := clipSpansToVars(spans, []byte("abc{{unclosed"))
	if len(got) != 1 || got[0].End != 10 {
		t.Fatalf("an unterminated {{ must leave spans alone: %+v", got)
	}
}

func TestClipSpansToVars_SplitsAroundVariable(t *testing.T) {
	chunk := []byte("aa{{v}}bb")
	red := color.NRGBA{R: 255, A: 255}
	got := clipSpansToVars([]widgets.ColoredSpan{{Start: 0, End: len(chunk), Color: red}}, chunk)
	want := []widgets.ColoredSpan{
		{Start: 0, End: 2, Color: red},
		{Start: 7, End: 9, Color: red},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestClipSpansToVars_SpanFullyInsideVariableIsDropped(t *testing.T) {
	chunk := []byte("{{token}}")
	got := clipSpansToVars([]widgets.ColoredSpan{{Start: 2, End: 7}}, chunk)
	if len(got) != 0 {
		t.Fatalf("a span entirely inside a variable must be dropped: %+v", got)
	}
}

func TestClipSpansToVars_MultipleVarsAndDisjointSpan(t *testing.T) {
	chunk := []byte("x{{a}}y{{b}}z")
	got := clipSpansToVars([]widgets.ColoredSpan{
		{Start: 0, End: len(chunk)},
		{Start: 12, End: 13},
	}, chunk)
	wantStarts := []int{0, 6, 12, 12}
	wantEnds := []int{1, 7, 13, 13}
	if len(got) != len(wantStarts) {
		t.Fatalf("got %+v", got)
	}
	for i := range wantStarts {
		if got[i].Start != wantStarts[i] || got[i].End != wantEnds[i] {
			t.Fatalf("segment %d = [%d,%d), want [%d,%d)", i, got[i].Start, got[i].End, wantStarts[i], wantEnds[i])
		}
	}
}

func TestBytesIndexAndTwoBraces(t *testing.T) {
	if got := bytesIndex([]byte("abc"), ""); got != 0 {
		t.Errorf("empty needle should return 0, got %d", got)
	}
	if got := bytesIndex([]byte("hello"), "ll"); got != 2 {
		t.Errorf("bytesIndex = %d, want 2", got)
	}
	if got := bytesIndex([]byte("hello"), "zz"); got != -1 {
		t.Errorf("missing needle should return -1, got %d", got)
	}
	if bytesContainsTwoBraces([]byte("{")) {
		t.Error("a single brace is not a variable opener")
	}
	if bytesContainsTwoBraces([]byte("{a{b")) {
		t.Error("non-adjacent braces are not a variable opener")
	}
	if !bytesContainsTwoBraces([]byte("x{{y")) {
		t.Error("adjacent braces should be detected")
	}
}

func gzipBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func zlibBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zlib.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func rawDeflateBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := flate.NewWriter(&b, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate.NewWriter: %v", err)
	}
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func brotliBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := brotli.NewWriter(&b)
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func zstdBytes(t *testing.T, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w, err := zstd.NewWriter(&b)
	if err != nil {
		t.Fatalf("zstd.NewWriter: %v", err)
	}
	_, _ = w.Write([]byte(s))
	_ = w.Close()
	return b.Bytes()
}

func respWith(enc string, body []byte) *http.Response {
	h := http.Header{}
	if enc != "" {
		h.Set("Content-Encoding", enc)
	}
	return &http.Response{Header: h, Body: io.NopCloser(bytes.NewReader(body))}
}

func TestDecompressBody_Encodings(t *testing.T) {
	const payload = "the quick brown fox jumps over the lazy dog"

	var doubled bytes.Buffer
	gw := gzip.NewWriter(&doubled)
	_, _ = gw.Write(brotliBytes(t, payload))
	_ = gw.Close()

	cases := []struct {
		name string
		enc  string
		body []byte
	}{
		{"gzip", "gzip", gzipBytes(t, payload)},
		{"x-gzip", "x-gzip", gzipBytes(t, payload)},
		{"deflate-zlib", "deflate", zlibBytes(t, payload)},
		{"deflate-raw", "deflate", rawDeflateBytes(t, payload)},
		{"br", "br", brotliBytes(t, payload)},
		{"zstd", "zstd", zstdBytes(t, payload)},
		{"uppercase-and-spaces", "  GZIP  ", gzipBytes(t, payload)},
		{"chained-gzip-br", "br, gzip", doubled.Bytes()},
		{"empty-element", "identity, gzip", gzipBytes(t, payload)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rc := decompressBody(respWith(c.enc, c.body))
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Errorf("close: %v", err)
			}
			if string(got) != payload {
				t.Fatalf("got %q, want %q", got, payload)
			}
		})
	}
}

func TestDecompressBody_PassThroughCases(t *testing.T) {
	if got := decompressBody(&http.Response{Header: http.Header{}}); got != nil {
		t.Error("nil body must be returned as-is")
	}

	raw := []byte("plain")
	r := respWith("gzip", raw)
	r.Uncompressed = true
	if got := decompressBody(r); got != r.Body {
		t.Error("an already-decompressed response must not be wrapped again")
	}

	for _, enc := range []string{"", "identity", "exotic-codec"} {
		r := respWith(enc, raw)
		if got := decompressBody(r); got != r.Body {
			t.Errorf("encoding %q should pass through untouched", enc)
		}
	}
}

func TestDecompressBody_CorruptStreamFallsBackToRawBody(t *testing.T) {
	for _, enc := range []string{"gzip", "deflate"} {
		r := respWith(enc, []byte{0x78, 0x00, 0x00, 0x00, 0x00, 0x00})
		got := decompressBody(r)
		if got != r.Body {
			t.Errorf("encoding %q: a corrupt stream must fall back to the raw body", enc)
		}
	}
}

func TestCleanupRespFile_RemovesTempAndDrainsChannels(t *testing.T) {
	tab := NewRequestTab("cleanup")

	dir := t.TempDir()
	main := filepath.Join(dir, "main.tmp")
	queued := filepath.Join(dir, "queued.tmp")
	for _, p := range []string{main, queued} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}

	f, err := os.CreateTemp(dir, "writer-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tab.FileSaveChan <- f
	tab.responseChan <- tabResponse{respFile: queued}
	tab.respFile = main

	tab.cleanupRespFile()

	if _, err := os.Stat(main); !os.IsNotExist(err) {
		t.Error("the tab's own temp response file must be removed")
	}
	if _, err := os.Stat(queued); !os.IsNotExist(err) {
		t.Error("a response still queued on the channel must have its temp file removed")
	}
	if tab.respFile != "" {
		t.Errorf("respFile should be cleared, got %q", tab.respFile)
	}

	tab.cleanupRespFile()
}

func TestTextCore_PadAndInvalidateHelpers(t *testing.T) {
	var v RequestEditor
	v.SetText("a\nb\nc\nd")

	v.chunkHeights = []int{1, 2, 3, 4, 5, 6}
	v.padChunkHeights()
	if len(v.chunkHeights) != len(v.lineStarts) {
		t.Fatalf("padChunkHeights should truncate to %d, got %d", len(v.lineStarts), len(v.chunkHeights))
	}
	v.chunkHeights = v.chunkHeights[:1]
	v.padChunkHeights()
	if len(v.chunkHeights) != len(v.lineStarts) {
		t.Fatalf("padChunkHeights should grow to %d, got %d", len(v.lineStarts), len(v.chunkHeights))
	}

	v.wrapPlans = make([]*wrapPlan, len(v.lineStarts)+3)
	v.padWrapPlans()
	if len(v.wrapPlans) != len(v.lineStarts) {
		t.Fatalf("padWrapPlans should truncate to %d, got %d", len(v.lineStarts), len(v.wrapPlans))
	}
	v.wrapPlans = nil
	v.padWrapPlans()
	if len(v.wrapPlans) != len(v.lineStarts) {
		t.Fatalf("padWrapPlans should grow to %d, got %d", len(v.lineStarts), len(v.wrapPlans))
	}

	for i := range v.wrapPlans {
		v.wrapPlans[i] = &wrapPlan{valid: true}
	}
	v.invalidateWrapPlansFrom(2)
	if !v.wrapPlans[0].valid || !v.wrapPlans[1].valid {
		t.Error("lines before the invalidation point must stay valid")
	}
	for i := 2; i < len(v.wrapPlans); i++ {
		if v.wrapPlans[i].valid {
			t.Errorf("line %d should have been invalidated", i)
		}
	}

	for i := range v.wrapPlans {
		v.wrapPlans[i] = &wrapPlan{valid: true}
	}
	v.invalidateWrapPlansFrom(-5)
	for i := range v.wrapPlans {
		if v.wrapPlans[i].valid {
			t.Fatalf("a negative start must clamp to 0 and invalidate line %d", i)
		}
	}

	v.invalidateChunkHeights()
	if len(v.chunkHeights) != 0 {
		t.Error("invalidateChunkHeights must empty the slice")
	}
}

func TestTextCore_SelectedTextClampsOutOfRange(t *testing.T) {
	var v RequestEditor
	v.SetText("abcdef")

	if got := v.SelectedText(); got != "" {
		t.Errorf("an empty selection must yield %q, got %q", "", got)
	}
	v.selStart, v.selEnd = 4, 1
	if got := v.SelectedText(); got != "bcd" {
		t.Errorf("a reversed selection should normalize, got %q", got)
	}
	v.selStart, v.selEnd = -3, 100
	if got := v.SelectedText(); got != "abcdef" {
		t.Errorf("out-of-range bounds should clamp, got %q", got)
	}
}

func TestTextCore_SpansForChunk(t *testing.T) {
	var v RequestEditor
	v.SetText("abcdefghij")
	v.tokens = []syntax.Token{
		{Start: 0, End: 3},
		{Start: 3, End: 6},
		{Start: 6, End: 10},
	}
	pal := theme.Syntax

	if got := v.spansForChunk(5, 5, pal, false); got != nil {
		t.Error("an empty chunk range must produce no spans")
	}
	if got := v.spansForChunk(20, 30, pal, false); got != nil {
		t.Error("a chunk past every token must produce no spans")
	}

	got := v.spansForChunk(2, 7, pal, false)
	if len(got) != 3 {
		t.Fatalf("want 3 clipped spans, got %d (%+v)", len(got), got)
	}
	if got[0].Start != 0 || got[0].End != 1 {
		t.Errorf("first span should be clipped to the chunk start: %+v", got[0])
	}
	if got[2].Start != 4 || got[2].End != 5 {
		t.Errorf("last span should be clipped to the chunk end: %+v", got[2])
	}

	v.tokens = nil
	if got := v.spansForChunk(0, 5, pal, false); got != nil {
		t.Error("no tokens must produce no spans")
	}
}

func TestTextCore_SetScrollCaretIsInert(t *testing.T) {
	var v RequestEditor
	v.SetText("hello")
	v.scrollY = 17
	v.SetScrollCaret(true)
	v.SetScrollCaret(false)
	if v.scrollY != 17 {
		t.Errorf("SetScrollCaret must not move the viewport, scrollY = %d", v.scrollY)
	}
}

func TestEditor_SetCaretClampsAndScrolls(t *testing.T) {
	var v RequestEditor
	v.SetText(strings.Repeat("line\n", 200))

	v.SetCaret(-5, -9)
	if s, e := v.Selection(); s != 0 || e != 0 {
		t.Errorf("negative offsets must clamp to 0, got (%d,%d)", s, e)
	}

	v.SetCaret(1<<20, 1<<20)
	s, e := v.Selection()
	if s != v.Len() || e != v.Len() {
		t.Errorf("offsets past the end must clamp to len, got (%d,%d) len=%d", s, e, v.Len())
	}

	v.scrollY = 999
	v.lastLineHeight = 0
	v.SetCaret(10, 10)
	if v.scrollY != 999 {
		t.Error("with no measured line height the viewport must not move")
	}

	v.lastLineHeight = 20
	v.lastViewportH = 100
	v.SetCaret(v.Len(), v.Len())
	if v.scrollY <= 0 {
		t.Errorf("scrolling to the end should produce a positive offset, got %d", v.scrollY)
	}

	v.lastViewportH = 0
	v.SetCaret(0, 0)
	if v.scrollY != 0 {
		t.Errorf("scrolling to offset 0 should land at 0, got %d", v.scrollY)
	}
}

func TestExampleSelLabel(t *testing.T) {
	tab := NewRequestTab("ex")
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("with no selection want %q, got %q", "Examples", got)
	}
	tab.Examples = []model.ParsedExample{{Name: "one"}, {Name: "two"}}
	tab.ExampleSel = 1
	if got := tab.exampleSelLabel(); got != exampleMenuLabel(1) {
		t.Errorf("selected label = %q", got)
	}
	tab.ExampleSel = 9
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("an out-of-range selection must fall back, got %q", got)
	}
	tab.ExampleSel = -1
	if got := tab.exampleSelLabel(); got != "Examples" {
		t.Errorf("a negative selection must fall back, got %q", got)
	}
}

func newLinkedTab(t *testing.T) (*RequestTab, *model.ParsedRequest) {
	t.Helper()
	tab := NewRequestTab("linked")
	req := &model.ParsedRequest{
		Name:    "linked",
		Method:  "GET",
		URL:     "http://example.com",
		Headers: map[string]string{},
	}
	tab.Method = req.Method
	tab.URLInput.SetText(req.URL)
	tab.ReqEditor.SetText(req.Body)
	tab.BodyType = req.BodyType
	tab.LinkedNode = &collections.CollectionNode{Request: req}
	tab.checkDirty()
	if tab.IsDirty {
		t.Fatalf("a freshly linked tab must be clean")
	}
	return tab, req
}

func TestCheckDirty_UnlinkedTabIsNeverDirty(t *testing.T) {
	tab := NewRequestTab("free")
	tab.IsDirty = true
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("a tab with no linked node must be clean")
	}
	tab.LinkedNode = &collections.CollectionNode{}
	tab.IsDirty = true
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("a linked node with no request must leave the tab clean")
	}
}

func TestCheckDirty_FieldByField(t *testing.T) {
	cases := []struct {
		name  string
		dirty func(*RequestTab, *model.ParsedRequest)
	}{
		{"method", func(tab *RequestTab, _ *model.ParsedRequest) { tab.Method = "POST" }},
		{"url", func(tab *RequestTab, _ *model.ParsedRequest) { tab.URLInput.SetText("http://other") }},
		{"body", func(tab *RequestTab, _ *model.ParsedRequest) { tab.ReqEditor.SetText("changed") }},
		{"body type", func(tab *RequestTab, _ *model.ParsedRequest) { tab.BodyType = model.BodyFormData }},
		{"binary path", func(tab *RequestTab, _ *model.ParsedRequest) { tab.BinaryFilePath = "/tmp/x" }},
		{"header added", func(tab *RequestTab, _ *model.ParsedRequest) { tab.AddHeader("X-New", "1") }},
		{"header value", func(tab *RequestTab, req *model.ParsedRequest) {
			req.Headers["X-A"] = "1"
			tab.AddHeader("X-A", "2")
		}},
		{"header key", func(tab *RequestTab, req *model.ParsedRequest) {
			req.Headers["X-A"] = "1"
			tab.AddHeader("X-B", "1")
		}},
		{"auth", func(tab *RequestTab, _ *model.ParsedRequest) {
			tab.AuthType = authBearer
			tab.AuthToken.SetText("tok")
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tab, req := newLinkedTab(t)
			c.dirty(tab, req)
			tab.checkDirty()
			if !tab.IsDirty {
				t.Fatalf("changing %s must mark the tab dirty", c.name)
			}
		})
	}
}

func TestCheckDirty_GeneratedAndBlankHeadersAreIgnored(t *testing.T) {
	tab, _ := newLinkedTab(t)
	tab.AddHeader("Content-Length", "12")
	tab.Headers[len(tab.Headers)-1].IsGenerated = true
	tab.AddHeader("", "orphan-value")
	tab.checkDirty()
	if tab.IsDirty {
		t.Error("generated and key-less header rows must not count as user edits")
	}
}

func TestCheckDirty_FormPartsAndURLEncodedAndCookies(t *testing.T) {
	t.Run("form part added", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.FormParts[0].Key.SetText("file")
		tab.FormParts[0].Value.SetText("v")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a form part must mark the tab dirty")
		}
	})

	t.Run("blank form part ignored", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.checkDirty()
		if tab.IsDirty {
			t.Fatal("a form part with a blank key must be ignored")
		}
	})

	t.Run("form part value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		req.FormParts = []model.ParsedFormPart{{Key: "k", Value: "saved"}}
		tab.FormParts = append(tab.FormParts, NewFormPart("", "", model.FormPartText, "", 0))
		tab.FormParts[0].Key.SetText("k")
		tab.FormParts[0].Value.SetText("edited")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited form-part value must mark the tab dirty")
		}
	})

	t.Run("urlencoded added", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.URLEncoded[0].Key.SetText("a")
		tab.URLEncoded[0].Value.SetText("1")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a urlencoded pair must mark the tab dirty")
		}
	})

	t.Run("blank urlencoded ignored", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.checkDirty()
		if tab.IsDirty {
			t.Fatal("a urlencoded row with a blank key must be ignored")
		}
	})

	t.Run("urlencoded value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		req.URLEncoded = []model.ParsedKV{{Key: "a", Value: "saved"}}
		tab.URLEncoded = append(tab.URLEncoded, NewURLEncodedPart("", ""))
		tab.URLEncoded[0].Key.SetText("a")
		tab.URLEncoded[0].Value.SetText("edited")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited urlencoded value must mark the tab dirty")
		}
	})

	t.Run("cookie count differs", func(t *testing.T) {
		tab, _ := newLinkedTab(t)
		tab.addCookie("sid", "1")
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("adding a cookie must mark the tab dirty")
		}
	})

	t.Run("cookie value differs", func(t *testing.T) {
		tab, req := newLinkedTab(t)
		tab.addCookie("sid", "edited")
		req.Cookies = tab.CookieModels()
		req.Cookies[0].Value = "saved"
		tab.checkDirty()
		if !tab.IsDirty {
			t.Fatal("an edited cookie value must mark the tab dirty")
		}
	})
}

func TestWSSend_NotConnectedRecordsError(t *testing.T) {
	tab := NewRequestTab("ws")
	tab.Method = MethodWS

	tab.WSSendText("hi")
	tab.WSSendBinary([]byte{1, 2})
	tab.WSSendPing()
	tab.WSSendProto(`{"a":1}`)

	s := tab.EnsureWS()
	s.sessionMu.Lock()
	msgs := append([]WSDisplayMessage(nil), s.Messages...)
	s.sessionMu.Unlock()
	if len(msgs) != 4 {
		t.Fatalf("want 4 error entries, got %d", len(msgs))
	}
	for i, m := range msgs {
		if m.Error != "Not connected" {
			t.Errorf("entry %d error = %q, want %q", i, m.Error, "Not connected")
		}
	}
}

func TestProtoHeaderFields(t *testing.T) {
	tab := NewRequestTab("ws")
	tab.Method = MethodWS
	s := tab.EnsureWS()

	s.ProtoCmdEditor.SetText("7")
	s.ProtoSeqEditor.SetText("-9")
	s.ProtoOpcodeEditor.SetText("300")
	cmd, seq, op, err := s.protoHeaderFields()
	if err != nil {
		t.Fatalf("valid fields returned an error: %v", err)
	}
	if cmd != 7 || seq != -9 || op != 300 {
		t.Fatalf("parsed (%d,%d,%d), want (7,-9,300)", cmd, seq, op)
	}

	cases := []struct {
		name   string
		cmd    string
		seq    string
		opcode string
		prefix string
	}{
		{"bad cmd", "nope", "0", "0", "cmd: "},
		{"cmd out of range", "256", "0", "0", "cmd: "},
		{"bad seq", "1", "zz", "0", "seq: "},
		{"seq out of range", "1", "32768", "0", "seq: "},
		{"bad opcode", "1", "0", "!!", "opcode: "},
		{"opcode out of range", "1", "0", "-32769", "opcode: "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s.ProtoCmdEditor.SetText(c.cmd)
			s.ProtoSeqEditor.SetText(c.seq)
			s.ProtoOpcodeEditor.SetText(c.opcode)
			_, _, _, err := s.protoHeaderFields()
			if err == nil {
				t.Fatalf("want an error for %s", c.name)
			}
			if !strings.HasPrefix(err.Error(), c.prefix) {
				t.Fatalf("error %q should start with %q", err, c.prefix)
			}
		})
	}
}

type urlNavRig struct {
	r   input.Router
	ops *op.Ops
	th  *material.Theme
	tab *RequestTab
	clk time.Duration
}

func newURLNavRig(text string) *urlNavRig {
	rig := &urlNavRig{ops: new(op.Ops), th: material.NewTheme(), tab: &RequestTab{}}
	rig.tab.URLInput.SingleLine = true
	rig.tab.URLInput.SetText(text)
	return rig
}

func (rig *urlNavRig) frame() {
	rig.ops.Reset()
	gtx := layout.Context{
		Ops:         rig.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(600, 40)),
		Now:         time.Now(),
		Source:      rig.r.Source(),
	}
	rig.tab.handleURLWordJump(gtx)
	for {
		if _, ok := rig.tab.URLInput.Update(gtx); !ok {
			break
		}
	}
	rig.tab.handleURLMultiClick(gtx, rig.th, unit.Sp(12))
	dims := material.Editor(rig.th, &rig.tab.URLInput, "").Layout(gtx)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	cl := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	rig.tab.urlClick.Add(gtx.Ops)
	cl.Pop()
	pass.Pop()
	rig.r.Frame(rig.ops)
}

func (rig *urlNavRig) focus() {
	rig.frame()
	rig.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
		pointer.Event{Kind: pointer.Release, Position: f32.Pt(5, 5), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse},
	)
	rig.frame()
}

func (rig *urlNavRig) clickN(x float32, n int) {
	for i := 0; i < n; i++ {
		rig.clk += 40 * time.Millisecond
		rig.r.Queue(
			pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, 10), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse, Time: rig.clk},
			pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, 10), Source: pointer.Mouse, Time: rig.clk},
		)
		rig.frame()
	}
	rig.frame()
}

func TestHandleURLWordJump_MovesAndExtends(t *testing.T) {
	const url = "https://example.com/a/b"
	rig := newURLNavRig(url)
	rig.focus()

	n := utf8.RuneCountInString(url)
	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()

	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	_, afterLeft := rig.tab.URLInput.Selection()
	if afterLeft >= n {
		t.Fatalf("ctrl+left should move the caret left of %d, got %d", n, afterLeft)
	}

	rig.r.Queue(key.Event{Name: key.NameRightArrow, Modifiers: key.ModShortcut, State: key.Press})
	rig.frame()
	_, afterRight := rig.tab.URLInput.Selection()
	if afterRight <= afterLeft {
		t.Fatalf("ctrl+right should move the caret right of %d, got %d", afterLeft, afterRight)
	}

	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()
	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut | key.ModShift, State: key.Press})
	rig.frame()
	lo, hi := selRange(rig.tab)
	if lo == hi {
		t.Fatalf("ctrl+shift+left must extend the selection, got (%d,%d)", lo, hi)
	}
	if hi != n {
		t.Errorf("the selection anchor should stay at %d, got %d", n, hi)
	}
}

func TestHandleURLWordJump_IgnoresKeyRelease(t *testing.T) {
	const url = "https://example.com/path"
	rig := newURLNavRig(url)
	rig.focus()
	n := utf8.RuneCountInString(url)
	rig.tab.URLInput.SetCaret(n, n)
	rig.frame()

	rig.r.Queue(key.Event{Name: key.NameLeftArrow, Modifiers: key.ModShortcut, State: key.Release})
	rig.frame()
	if _, end := rig.tab.URLInput.Selection(); end != n {
		t.Fatalf("a key release must not move the caret, got %d", end)
	}
}

func TestHandleURLMultiClick_TripleClickSelectsAll(t *testing.T) {
	const url = "https://example.com/alpha/beta"
	rig := newURLNavRig(url)
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 3)

	lo, hi := selRange(rig.tab)
	n := utf8.RuneCountInString(url)
	if lo != 0 || hi != n {
		t.Fatalf("triple click should select the whole URL, got (%d,%d) want (0,%d)", lo, hi, n)
	}
}

func TestHandleURLMultiClick_DoubleClickSelectsWord(t *testing.T) {
	const url = "https://example.com/alpha/beta"
	rig := newURLNavRig(url)
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 2)

	lo, hi := selRange(rig.tab)
	if lo == hi {
		t.Fatalf("double click should select a word, got an empty selection at %d", lo)
	}
	n := utf8.RuneCountInString(url)
	if lo < 0 || hi > n {
		t.Fatalf("selection (%d,%d) is out of range for a %d-rune URL", lo, hi, n)
	}
	sel := []rune(url)[lo:hi]
	if strings.ContainsAny(string(sel), "/:.") {
		t.Errorf("a double-click word should not span URL separators, got %q", string(sel))
	}
}

func TestHandleURLMultiClick_SingleClickLeavesSelectionEmpty(t *testing.T) {
	rig := newURLNavRig("https://example.com/alpha")
	rig.frame()
	rig.clk += 3 * time.Second
	rig.clickN(60, 1)
	rig.tab.URLInput.SetCaret(0, 0)
	rig.frame()

	if lo, hi := selRange(rig.tab); lo != hi {
		t.Fatalf("a single click must not create a selection, got (%d,%d)", lo, hi)
	}
}

func selRange(tab *RequestTab) (int, int) {
	a, b := tab.URLInput.Selection()
	if a > b {
		return b, a
	}
	return a, b
}

func TestEditor_ReplaceRangeAndClamp(t *testing.T) {
	var v RequestEditor
	v.SetText("hello world")

	if !v.Replace(6, 11, "there") {
		t.Fatal("Replace should succeed")
	}
	if v.Text() != "hello there" {
		t.Fatalf("after replace = %q", v.Text())
	}

	if !v.Replace(11, 6, "!") {
		t.Fatal("reversed range should be normalized and succeed")
	}
	if v.Text() != "hello !" {
		t.Fatalf("after reversed replace = %q", v.Text())
	}

	if !v.Replace(-5, 100, "reset") {
		t.Fatal("out-of-range bounds should clamp and succeed")
	}
	if v.Text() != "reset" {
		t.Fatalf("after clamped replace = %q", v.Text())
	}

	if !v.Replace(0, 0, "") {
		t.Fatal("an empty no-op replace should return true")
	}
}

func TestEditor_ReplaceRejectsOversize(t *testing.T) {
	var v RequestEditor
	v.SetText("x")
	huge := strings.Repeat("a", RequestBodyMaxBytes+10)
	if v.Replace(0, 1, huge) {
		t.Fatal("a replacement over the size limit must be rejected")
	}
	if v.Text() != "x" {
		t.Fatalf("rejected replace must not mutate the buffer, got len %d", v.Len())
	}
	if v.oversizeMsg == "" {
		t.Error("an oversize rejection should set a user message")
	}
}

func TestAutoSurroundPair(t *testing.T) {
	pairs := map[string][2]string{
		"(": {"(", ")"}, "[": {"[", "]"}, "{": {"{", "}"}, "<": {"<", ">"},
		"\"": {"\"", "\""}, "'": {"'", "'"}, "`": {"`", "`"},
	}
	for in, want := range pairs {
		o, c, ok := autoSurroundPair(in)
		if !ok || o != want[0] || c != want[1] {
			t.Errorf("autoSurroundPair(%q) = (%q,%q,%v), want (%q,%q,true)", in, o, c, ok, want[0], want[1])
		}
	}
	for _, in := range []string{"a", ")", "", "ab"} {
		if _, _, ok := autoSurroundPair(in); ok {
			t.Errorf("autoSurroundPair(%q) should not surround", in)
		}
	}
}

func TestCountRunesBetween(t *testing.T) {
	text := []byte("héllo")
	if got := countRunesBetween(text, 3, 3); got != 0 {
		t.Errorf("equal bounds = %d, want 0", got)
	}
	if got := countRunesBetween(text, 5, 2); got != 0 {
		t.Errorf("reversed bounds = %d, want 0", got)
	}
	if got := countRunesBetween(text, 0, len(text)); got != 5 {
		t.Errorf("full range = %d, want 5 runes", got)
	}
	if got := countRunesBetween(text, 0, 999); got != 5 {
		t.Errorf("past-end bound should clamp, got %d", got)
	}
}

func TestEditor_ByteToRuneASCIIAndUnicode(t *testing.T) {
	var a RequestEditor
	a.SetText("abcdef")
	if got := a.byteToRune(3); got != 3 {
		t.Errorf("ascii byteToRune(3) = %d, want 3", got)
	}
	if got := a.byteToRune(100); got != 6 {
		t.Errorf("ascii byteToRune past end should clamp to len, got %d", got)
	}

	var u RequestEditor
	u.SetText("héllo")
	if got := u.byteToRune(len("hé")); got != 2 {
		t.Errorf("unicode byteToRune after 'hé' = %d, want 2 runes", got)
	}
}

func TestEditor_ShiftRangesInsertAndDelete(t *testing.T) {
	var v RequestEditor
	v.SetText("0123456789")
	v.selStart, v.selEnd = 4, 7
	v.highlightStart, v.highlightEnd = 5, 8

	v.shiftRanges(3, 2)
	if v.selStart != 6 || v.selEnd != 9 {
		t.Errorf("insert shift sel = (%d,%d), want (6,9)", v.selStart, v.selEnd)
	}
	if v.highlightStart != 7 || v.highlightEnd != 10 {
		t.Errorf("insert shift highlight = (%d,%d), want (7,10)", v.highlightStart, v.highlightEnd)
	}

	var d RequestEditor
	d.SetText("0123456789")
	d.selStart, d.selEnd = 2, 9
	d.shiftRanges(8, -3)
	if d.selStart != 2 {
		t.Errorf("offset before the deleted [5,8) should be unchanged, got %d", d.selStart)
	}
	if d.selEnd != 6 {
		t.Errorf("offset after the cut should move left by 3, got %d", d.selEnd)
	}

	var c RequestEditor
	c.SetText("0123456789")
	c.selStart, c.selEnd = 6, 9
	c.shiftRanges(8, -3)
	if c.selStart != 5 {
		t.Errorf("an offset inside the deleted range should collapse to its start (5), got %d", c.selStart)
	}
	if c.selEnd != 6 {
		t.Errorf("offset after the cut should move left by 3, got %d", c.selEnd)
	}
}
