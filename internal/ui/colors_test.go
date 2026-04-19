package ui

import (
	"image/color"
	"testing"
)

func TestGetMethodColor(t *testing.T) {
	tests := []struct {
		method   string
		expected color.NRGBA
	}{
		{"GET", colorMethodGet},
		{"POST", colorMethodPost},
		{"PUT", colorMethodPut},
		{"DELETE", colorMethodDelete},
		{"PATCH", colorMethodPatch},
		{"OPTIONS", colorMethodOptions},
		{"UNKNOWN", colorMethodFallback},
		{"", colorMethodFallback},
	}

	for _, tc := range tests {
		color := getMethodColor(tc.method)
		if color != tc.expected {
			t.Errorf("expected %v for %s, got %v", tc.expected, tc.method, color)
		}
	}
}
