package theme

import (
	"image/color"
	"testing"
)

func TestMethodColor(t *testing.T) {
	tests := []struct {
		method   string
		expected color.NRGBA
	}{
		{"GET", MethodGet},
		{"POST", MethodPost},
		{"PUT", MethodPut},
		{"DELETE", MethodDelete},
		{"HEAD", MethodHead},
		{"PATCH", MethodPatch},
		{"OPTIONS", MethodOptions},
		{"UNKNOWN", MethodFallback},
		{"", MethodFallback},
	}

	for _, tc := range tests {
		got := MethodColor(tc.method)
		if got != tc.expected {
			t.Errorf("expected %v for %s, got %v", tc.expected, tc.method, got)
		}
	}
}
