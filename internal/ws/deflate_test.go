package ws

import (
	"bytes"
	"testing"
)

func TestDeflateInflateRoundtripNoContext(t *testing.T) {
	d, err := NewDeflater(true)
	if err != nil {
		t.Fatalf("NewDeflater: %v", err)
	}
	defer d.Close()
	in := NewInflater(true)
	defer in.Close()

	for _, payload := range [][]byte{
		[]byte("hello"),
		bytes.Repeat([]byte("AB"), 1000),
		[]byte("permessage-deflate test payload with some redundancy permessage-deflate"),
	} {
		compressed, err := d.Deflate(payload)
		if err != nil {
			t.Fatalf("Deflate: %v", err)
		}
		got, err := in.Inflate(compressed)
		if err != nil {
			t.Fatalf("Inflate: %v", err)
		}
		if !bytes.Equal(got, payload) {
			t.Errorf("roundtrip mismatch: got %q want %q", got, payload)
		}
	}
}

func TestDeflateInflateRoundtripContextTakeover(t *testing.T) {
	d, err := NewDeflater(false)
	if err != nil {
		t.Fatalf("NewDeflater: %v", err)
	}
	defer d.Close()
	in := NewInflater(false)
	defer in.Close()

	msgs := []string{"one two three", "one two three four", "one two three four five"}
	for _, m := range msgs {
		compressed, err := d.Deflate([]byte(m))
		if err != nil {
			t.Fatalf("Deflate: %v", err)
		}
		got, err := in.Inflate(compressed)
		if err != nil {
			t.Fatalf("Inflate: %v", err)
		}
		if string(got) != m {
			t.Errorf("got %q want %q", got, m)
		}
	}
}

func TestParseExtensions(t *testing.T) {
	cases := []struct {
		in    string
		want  ExtParams
	}{
		{"", ExtParams{}},
		{"permessage-deflate", ExtParams{Negotiated: true}},
		{"permessage-deflate; client_no_context_takeover", ExtParams{Negotiated: true, ClientNoContextTakeover: true}},
		{"permessage-deflate; server_no_context_takeover; client_no_context_takeover", ExtParams{Negotiated: true, ServerNoContextTakeover: true, ClientNoContextTakeover: true}},
		{"permessage-deflate; client_max_window_bits=10", ExtParams{Negotiated: true, ClientMaxWindowBits: 10}},
		{"other-ext, permessage-deflate", ExtParams{Negotiated: true}},
		{"deflate-stream", ExtParams{}},
	}
	for _, c := range cases {
		got := ParseExtensions(c.in)
		if got != c.want {
			t.Errorf("ParseExtensions(%q) = %+v want %+v", c.in, got, c.want)
		}
	}
}
