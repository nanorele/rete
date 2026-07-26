package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestEditorSizeBytesAndSoftLimit(t *testing.T) {
	ed := NewRequestEditor()
	if ed.SizeBytes() != 0 {
		t.Errorf("SizeBytes() = %d, want 0", ed.SizeBytes())
	}
	if ed.IsOverSoftLimit() {
		t.Error("an empty editor must not be over the limit")
	}
	ed.SetText("héllo")
	if ed.SizeBytes() != len("héllo") {
		t.Errorf("SizeBytes() = %d, want the byte length %d", ed.SizeBytes(), len("héllo"))
	}
	if ed.IsOverSoftLimit() {
		t.Error("a small body must not be over the limit")
	}
}

func TestEditorSetTextRejectsOversize(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	big := strings.Repeat("x", RequestBodyMaxBytes+1)
	if ed.SetText(big) {
		t.Fatal("SetText must reject a body over 100 MB")
	}
	if ed.Text() != "keep" {
		t.Errorf("a rejected SetText must leave the old text: %q", ed.Text())
	}
	if ed.OversizeMsg() == "" {
		t.Error("a rejected SetText must explain itself")
	}
	ed.DismissOversize()
	if ed.OversizeMsg() != "" {
		t.Errorf("DismissOversize left %q", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReader(t *testing.T) {
	ed := NewRequestEditor()
	if err := ed.LoadFromReader(strings.NewReader("line1\nline2\n")); err != nil {
		t.Fatalf("LoadFromReader: %v", err)
	}
	if ed.Text() != "line1\nline2\n" {
		t.Errorf("Text() = %q", ed.Text())
	}
	if ed.SizeBytes() != 12 {
		t.Errorf("SizeBytes() = %d, want 12", ed.SizeBytes())
	}
	if ed.OversizeMsg() != "" {
		t.Errorf("a successful load must clear the message, got %q", ed.OversizeMsg())
	}
	if got := len(ed.lineStarts); got != 3 {
		t.Errorf("line index = %d entries, want 3", got)
	}
}

func TestEditorLoadFromReaderEmpty(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("previous")
	if err := ed.LoadFromReader(strings.NewReader("")); err != nil {
		t.Fatalf("LoadFromReader(empty): %v", err)
	}
	if ed.Text() != "" {
		t.Errorf("Text() = %q, want empty", ed.Text())
	}
}

func TestEditorLoadFromReaderPropagatesReadError(t *testing.T) {
	ed := NewRequestEditor()
	want := errors.New("disk on fire")
	err := ed.LoadFromReader(failingReader{err: want})
	if !errors.Is(err, want) {
		t.Errorf("LoadFromReader error = %v, want %v", err, want)
	}
	if !strings.Contains(ed.OversizeMsg(), "disk on fire") {
		t.Errorf("OversizeMsg = %q, want the read error surfaced", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReaderResetsViewState(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText(strings.Repeat("abcdef\n", 50))
	ed.SetCaret(3, 9)
	ed.SetScrollY(40)
	if err := ed.LoadFromReader(strings.NewReader("fresh")); err != nil {
		t.Fatalf("LoadFromReader: %v", err)
	}
	if ed.GetScrollY() != 0 {
		t.Errorf("GetScrollY() = %d, want 0 after a load", ed.GetScrollY())
	}
	if ed.SelectedText() != "" {
		t.Errorf("SelectedText() = %q, want the selection cleared", ed.SelectedText())
	}
}

func TestEditorLoadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "body.json")
	if err := os.WriteFile(path, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ed := NewRequestEditor()
	if err := ed.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	if ed.Text() != `{"a":1}` {
		t.Errorf("Text() = %q", ed.Text())
	}
	if ed.OversizeMsg() != "" {
		t.Errorf("OversizeMsg = %q, want empty", ed.OversizeMsg())
	}
}

func TestEditorLoadFromFileMissing(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	err := ed.LoadFromFile(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("LoadFromFile must fail for a missing path")
	}
	if !strings.HasPrefix(ed.OversizeMsg(), "Load failed: ") {
		t.Errorf("OversizeMsg = %q, want a 'Load failed' message", ed.OversizeMsg())
	}
	if ed.Text() != "keep" {
		t.Errorf("a failed load must leave the old text: %q", ed.Text())
	}
}

func TestEditorLoadFromFileUnreadableDirectory(t *testing.T) {
	dir := t.TempDir()
	ed := NewRequestEditor()
	if err := ed.LoadFromFile(dir); err == nil {
		t.Error("LoadFromFile must fail when handed a directory")
	}
	if ed.OversizeMsg() == "" {
		t.Error("a directory load failure must be reported")
	}
}

func TestErrBodyTooLargeMessage(t *testing.T) {
	if errBodyTooLarge.Error() == "" {
		t.Error("errBodyTooLarge must carry a message")
	}
	if !errors.Is(errBodyTooLarge, errBodyTooLarge) {
		t.Error("errBodyTooLarge must compare equal to itself")
	}
}

func TestEditorAppend(t *testing.T) {
	ed := NewRequestEditor()
	if !ed.Append("") {
		t.Error("appending an empty string must succeed")
	}
	if ed.SizeBytes() != 0 {
		t.Errorf("SizeBytes() = %d after an empty append", ed.SizeBytes())
	}

	if !ed.Append("a\nb") {
		t.Fatal("Append failed")
	}
	if !ed.Append("c\nd\n") {
		t.Fatal("second Append failed")
	}
	if ed.Text() != "a\nbc\nd\n" {
		t.Errorf("Text() = %q, want %q", ed.Text(), "a\nbc\nd\n")
	}
	if got := len(ed.lineStarts); got != 4 {
		t.Errorf("line index = %d entries, want 4 for %q", got, ed.Text())
	}
}

func TestEditorAppendKeepsLineIndexConsistent(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("one\ntwo")
	ed.Append("\nthree\nfour")

	built := NewRequestEditor()
	built.SetText(ed.Text())
	if len(ed.lineStarts) != len(built.lineStarts) {
		t.Fatalf("appended line index = %v, want %v (same text loaded whole)", ed.lineStarts, built.lineStarts)
	}
	for i := range ed.lineStarts {
		if ed.lineStarts[i] != built.lineStarts[i] {
			t.Fatalf("appended line index = %v, want %v", ed.lineStarts, built.lineStarts)
		}
	}
}

func TestEditorAppendRejectsOverflow(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText(strings.Repeat("x", 64))
	stub := ed.SizeBytes()
	huge := strings.Repeat("y", RequestBodyMaxBytes)
	if ed.Append(huge) {
		t.Fatal("Append must refuse to cross the 100 MB ceiling")
	}
	if ed.SizeBytes() != stub {
		t.Errorf("a rejected Append must not grow the buffer: %d -> %d", stub, ed.SizeBytes())
	}
	if !strings.Contains(ed.OversizeMsg(), "Append rejected") {
		t.Errorf("OversizeMsg = %q, want an append-rejected message", ed.OversizeMsg())
	}
}

func TestEditorLoadFromReaderRejectsOversizeStream(t *testing.T) {
	ed := NewRequestEditor()
	ed.SetText("keep")
	r := io.LimitReader(zeroReader{}, int64(RequestBodyMaxBytes)+1)
	err := ed.LoadFromReader(r)
	if !errors.Is(err, errBodyTooLarge) {
		t.Fatalf("LoadFromReader error = %v, want errBodyTooLarge", err)
	}
	if !strings.Contains(ed.OversizeMsg(), "exceeds 100 MB") {
		t.Errorf("OversizeMsg = %q", ed.OversizeMsg())
	}
	if ed.Text() != "keep" {
		t.Errorf("a rejected stream load must leave the old text: %q", ed.Text())
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'z'
	}
	return len(p), nil
}
