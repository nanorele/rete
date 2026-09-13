package workspace

import (
	"bytes"
	"context"
	"io"
	"rete/internal/ui/settings"
	"strings"
	"testing"
)

type chunkedReader struct {
	data  []byte
	pos   int
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := r.chunk
	if n > len(p) {
		n = len(p)
	}
	if r.pos+n > len(r.data) {
		n = len(r.data) - r.pos
	}
	copy(p, r.data[r.pos:r.pos+n])
	r.pos += n
	return n, nil
}

func (t *RequestTab) drainAppendText() string {
	var sb strings.Builder
	for {
		select {
		case c := <-t.appendChan:
			sb.WriteString(c.text)
		default:
			return sb.String()
		}
	}
}

func TestStreamResponse_LivePreviewFormatsJSONWhileLoading(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = true
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := `{"user":{"id":42,"name":"Иван","tags":["a","b"],"note":"с \"кавычками\" и {скобками}"},"items":[1,2,3]}`
	for _, chunk := range []int{3, 7, 4096} {
		tab := NewRequestTab("stream-fmt")
		reqID := tab.requestID.Load()
		var sink bytes.Buffer
		src := &chunkedReader{data: []byte(raw), chunk: chunk}
		total, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "application/json")
		if err != nil {
			t.Fatalf("chunk=%d: streamResponse error: %v", chunk, err)
		}
		if total != int64(len(raw)) {
			t.Fatalf("chunk=%d: total = %d, want %d", chunk, total, len(raw))
		}
		if sink.String() != raw {
			t.Errorf("chunk=%d: file sink must receive raw bytes", chunk)
		}
		want := formatJSON([]byte(raw), &JSONFormatterState{})
		if got := tab.drainAppendText(); got != want {
			t.Errorf("chunk=%d: live preview must be formatted while streaming\ngot:  %q\nwant: %q", chunk, got, want)
		}
	}
}

func TestStreamResponse_NonJSONStaysRaw(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = true
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := "plain text body: {not json because of prefix}"
	tab := NewRequestTab("stream-plain")
	reqID := tab.requestID.Load()
	var sink bytes.Buffer
	src := &chunkedReader{data: []byte(raw), chunk: 5}
	if _, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "text/plain"); err != nil {
		t.Fatalf("streamResponse error: %v", err)
	}
	if got := tab.drainAppendText(); got != raw {
		t.Errorf("non-JSON body must stream unformatted\ngot:  %q\nwant: %q", got, raw)
	}
}

func TestStreamResponse_AutoFormatOffStaysRaw(t *testing.T) {
	oldFmt := settings.AutoFormatJSON
	settings.AutoFormatJSON = false
	defer func() { settings.AutoFormatJSON = oldFmt }()

	raw := `{"a":1}`
	tab := NewRequestTab("stream-off")
	reqID := tab.requestID.Load()
	var sink bytes.Buffer
	src := &chunkedReader{data: []byte(raw), chunk: 3}
	if _, err := tab.streamResponse(context.Background(), reqID, src, &sink, nil, true, "application/json"); err != nil {
		t.Fatalf("streamResponse error: %v", err)
	}
	if got := tab.drainAppendText(); got != raw {
		t.Errorf("with AutoFormatJSON off the stream must stay raw, got %q", got)
	}
}
