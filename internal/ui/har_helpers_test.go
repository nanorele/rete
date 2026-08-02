package ui

import (
	"testing"

	"tracto/internal/har"
)

func TestHarSkipHeader(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{":authority", true},
		{":method", true},
		{":path", true},
		{"content-length", true},
		{"Content-Length", true},
		{"CONTENT-LENGTH", true},
		{"host", true},
		{"Host", true},
		{"content-type", false},
		{"Authorization", false},
		{"", false},
		{"x-content-length", false},
		{"hostname", false},
	}
	for _, c := range cases {
		if got := harSkipHeader(c.name); got != c.want {
			t.Errorf("harSkipHeader(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestHarWSURL(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"https://example.com/socket", "wss://example.com/socket"},
		{"http://example.com/socket", "ws://example.com/socket"},
		{"wss://example.com/socket", "wss://example.com/socket"},
		{"ws://example.com/socket", "ws://example.com/socket"},
		{"", ""},
		{"example.com", "example.com"},
		{"HTTPS://example.com", "HTTPS://example.com"},
		{"https://", "wss://"},
	}
	for _, c := range cases {
		if got := harWSURL(c.raw); got != c.want {
			t.Errorf("harWSURL(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestHarRunTitle(t *testing.T) {
	cases := []struct {
		method, reqURL, want string
	}{
		{"GET", "https://example.com/a/b", "GET example.com"},
		{"POST", "https://example.com:8443/x?q=1", "POST example.com:8443"},
		{"GET", "wss://example.com/socket", "GET example.com"},
		{"GET", "not a url", "GET not a url"},
		{"", "https://example.com/a", "example.com"},
		{"GET", "", "GET"},
	}
	for _, c := range cases {
		e := &har.Entry{Request: har.Request{Method: c.method, URL: c.reqURL}}
		if got := harRunTitle(e, c.reqURL); got != c.want {
			t.Errorf("harRunTitle(%q, %q) = %q, want %q", c.method, c.reqURL, got, c.want)
		}
	}
}

func TestLoadEmbeddedTTF(t *testing.T) {
	b, err := loadEmbeddedTTF("Inter-Regular.ttf")
	if err != nil {
		t.Fatalf("loadEmbeddedTTF(Inter-Regular.ttf) error: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("loadEmbeddedTTF returned empty font")
	}
	if _, err := loadEmbeddedTTF("DoesNotExist.ttf"); err == nil {
		t.Error("loadEmbeddedTTF(DoesNotExist.ttf) = nil error, want error")
	}
}

func TestEmbeddedFontsDecompress(t *testing.T) {
	for name := range embeddedFonts {
		b, err := loadEmbeddedTTF(name)
		if err != nil {
			t.Errorf("loadEmbeddedTTF(%q) error: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("loadEmbeddedTTF(%q) decompressed to 0 bytes", name)
		}
	}
}

func TestFallbackFontFilesAreEmbedded(t *testing.T) {
	specs := append([]lazyFontSpec{emojiFontSpec}, fallbackFontSpecs...)
	for _, spec := range specs {
		if _, ok := embeddedFonts[spec.file]; !ok {
			t.Errorf("fallback font %q has no embedded entry", spec.file)
		}
	}
}
