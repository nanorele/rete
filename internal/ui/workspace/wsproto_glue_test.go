package workspace

import (
	"strings"
	"testing"

	"tracto/internal/wsproto"
)

func TestParseProtoInt(t *testing.T) {
	cases := []struct {
		in      string
		lo, hi  int
		want    int
		wantErr bool
	}{
		{"", 0, 255, 0, false},
		{"  42 ", 0, 255, 42, false},
		{"-5", -32768, 32767, -5, false},
		{"256", 0, 255, 0, true},
		{"x", 0, 255, 0, true},
	}
	for _, c := range cases {
		got, err := parseProtoInt(c.in, c.lo, c.hi)
		if c.wantErr {
			if err == nil {
				t.Fatalf("parseProtoInt(%q) expected error", c.in)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Fatalf("parseProtoInt(%q) = %d, %v; want %d", c.in, got, err, c.want)
		}
	}
}

func TestDecodeProtoViewRoundTrip(t *testing.T) {
	raw, _, err := wsproto.Encode(wsproto.Frame{
		Cmd:     5,
		Seq:     17,
		Opcode:  3,
		Payload: map[string]any{"hello": "world", "n": int64(9)},
	})
	if err != nil {
		t.Fatal(err)
	}
	view := decodeProtoView(raw)
	if view.DecodeErr != "" {
		t.Fatalf("unexpected decode error: %s", view.DecodeErr)
	}
	if view.Cmd != 5 || view.Seq != 17 || view.Opcode != 3 {
		t.Fatalf("header mismatch: %+v", view)
	}
	if !strings.Contains(view.JSON, "\"hello\": \"world\"") {
		t.Fatalf("json missing field: %s", view.JSON)
	}
	if !strings.Contains(previewProto(view), "cmd=5 seq=17 op=3") {
		t.Fatalf("preview mismatch: %s", previewProto(view))
	}
	detail := protoDetailText(view)
	if !strings.Contains(detail, "cmd=5") || !strings.Contains(detail, "\"hello\": \"world\"") {
		t.Fatalf("detail mismatch: %s", detail)
	}
}

func TestDecodeProtoViewBadFrame(t *testing.T) {
	view := decodeProtoView([]byte{1, 2, 3})
	if view.DecodeErr == "" {
		t.Fatal("expected decode error for short frame")
	}
	if !strings.Contains(previewProto(view), "⚠") {
		t.Fatalf("preview should flag error: %s", previewProto(view))
	}
}
