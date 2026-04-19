package ui

import (
	"os"
	"strings"
	"testing"
	"time"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget"
)

func TestLooksLikeJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"object", `{"a": 1}`, true},
		{"array", `[1, 2, 3]`, true},
		{"spaces before object", "   \t\n  {\"a\": 1}", true},
		{"not json string", `"string"`, false},
		{"not json num", "123", false},
		{"not json html", "<html></html>", false},
		{"empty string", "", false},
		{"only spaces", "   ", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := looksLikeJSON([]byte(tc.input))
			if result != tc.expected {
				t.Errorf("expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestFormatJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		state    *JSONFormatterState
		expected string
	}{
		{
			name:  "simple object",
			input: `{"a":1}`,
			state: &JSONFormatterState{},
			expected: "{\n  \"a\": 1\n}",
		},
		{
			name:  "nested object",
			input: `{"a":{"b":2}}`,
			state: &JSONFormatterState{},
			expected: "{\n  \"a\": {\n    \"b\": 2\n  }\n}",
		},
		{
			name:  "array",
			input: `[1, 2]`,
			state: &JSONFormatterState{},
			expected: "[\n  1,\n  2\n]",
		},
		{
			name:  "empty array",
			input: `[]`,
			state: &JSONFormatterState{},
			expected: "[]",
		},
		{
			name:  "empty object",
			input: `{}`,
			state: &JSONFormatterState{},
			expected: "{}",
		},
		{
			name:  "string with nested chars",
			input: `{"key": "value with { and [ and ,"}`,
			state: &JSONFormatterState{},
			expected: "{\n  \"key\": \"value with { and [ and ,\"\n}",
		},
		{
			name:  "numbers and bools",
			input: `{"a": 1, "b": true, "c": null}`,
			state: &JSONFormatterState{},
			expected: "{\n  \"a\": 1,\n  \"b\": true,\n  \"c\": null\n}",
		},
		{
			name:  "unquoted values",
			input: `{"a": unquoted}`,
			state: &JSONFormatterState{},
			expected: "{\n  \"a\": unquoted\n}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := formatJSON([]byte(tc.input), tc.state)
			if result != tc.expected {
				t.Errorf("expected:\n%q\n\ngot:\n%q", tc.expected, result)
			}
		})
	}
}

func TestFormatJSON_DeepNesting(t *testing.T) {
	// Test indentTable limit
	depth := 65
	input := strings.Repeat("[", depth) + strings.Repeat("]", depth)
	result := formatJSON([]byte(input), &JSONFormatterState{})
	if !strings.Contains(result, "[]") {
		t.Errorf("expected empty array at depth")
	}
}

func TestLoadPreviewFromFile(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	
	content := `{"a": 1}`
	os.WriteFile(tmpPath, []byte(content), 0644)
	
	result, n, isJSON := loadPreviewFromFile(tmpPath, int64(len(content)), &JSONFormatterState{})
	
	if result != "{\n  \"a\": 1\n}" {
		t.Errorf("expected formatted JSON, got %q", result)
	}
	if n != int64(len(content)) {
		t.Errorf("expected read size %d, got %d", len(content), n)
	}
	if !isJSON {
		t.Errorf("expected isJSON true")
	}
}

func TestEditorInsertWorks(t *testing.T) {
	var ed widget.Editor
	ed.Insert("hello")
	if ed.Text() != "hello" {
		t.Errorf("expected hello, got %q", ed.Text())
	}
}

func TestLoadMorePreview(t *testing.T) {
	tmp, _ := os.CreateTemp("", "preview")
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	
	content := "line1\nline2\n"
	os.WriteFile(tmpPath, []byte(content), 0644)
	
	tab := NewRequestTab("test")
	tab.window = new(app.Window)
	tab.respFile = tmpPath
	tab.respSize = int64(len(content))
	tab.previewLoaded = 6 // after "line1\n"
	tab.respIsJSON = false
	
	tab.loadMorePreview()
	
	// Wait and poll for channel
	success := false
	var lastText string
	for i := 0; i < 200; i++ {
		// Manual check of channel
		select {
		case text := <-tab.appendChan:
			tab.RespEditor.Insert(text)
		default:
		}
		
		lastText = tab.RespEditor.Text()
		if lastText == "line2\n" {
			success = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	
	if !success {
		// Check if file is readable
		data, _ := os.ReadFile(tmpPath)
		t.Errorf("expected line2, got %q (respSize=%d, previewLoaded=%d, fileData=%q)", lastText, tab.respSize, tab.previewLoaded, string(data))
	}
}

func TestOpenFileHelpers(t *testing.T) {
	// Skip actual execution tests
}
