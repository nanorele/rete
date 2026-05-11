package utils

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "clean ascii",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "valid utf8",
			input:    []byte("привет мир 😊"),
			expected: "привет мир 😊",
		},
		{
			name:     "invalid utf8 sequence",
			input:    []byte("hello\xffworld"),
			expected: "hello\ufffdworld",
		},
		{
			name:     "control chars removed",
			input:    []byte("hello\x01\x1fworld"),
			expected: "helloworld",
		},
		{
			name:     "tabs expanded",
			input:    []byte("hello\tworld"),
			expected: "hello    world",
		},
		{
			name:     "crlf normalized",
			input:    []byte("hello\r\nworld"),
			expected: "hello\nworld",
		},
		{
			name:     "cr normalized",
			input:    []byte("hello\rworld"),
			expected: "hello\nworld",
		},
		{
			name:     "special spaces normalized",
			input:    []byte("hello\u2028world\u2029"),
			expected: "hello\nworld\n",
		},
		{
			name:     "zero width chars removed",
			input:    []byte("hello\u200b\ufeffworld"),
			expected: "helloworld",
		},
		{
			name:     "del and above removed",
			input:    []byte("hello\x7f\x1fworld"),
			expected: "helloworld",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeBytes(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean ascii",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "valid utf8",
			input:    "привет мир 😊",
			expected: "привет мир 😊",
		},
		{
			name:     "invalid utf8 sequence",
			input:    "hello\xffworld",
			expected: "hello\ufffdworld",
		},
		{
			name:     "control chars removed",
			input:    "hello\x01\x1fworld",
			expected: "helloworld",
		},
		{
			name:     "tabs expanded",
			input:    "hello\tworld",
			expected: "hello    world",
		},
		{
			name:     "crlf normalized",
			input:    "hello\r\nworld",
			expected: "hello\nworld",
		},
		{
			name:     "cr normalized",
			input:    "hello\rworld",
			expected: "hello\nworld",
		},
		{
			name:     "special spaces normalized",
			input:    "hello\u2028world\u2029",
			expected: "hello\nworld\n",
		},
		{
			name:     "zero width chars removed",
			input:    "hello\u200b\ufeffworld",
			expected: "helloworld",
		},
		{
			name:     "del and above removed",
			input:    "hello\x7f\x9fworld",
			expected: "hello\ufffdworld",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeText(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "line comment at end",
			input:    `{"key": "value"} // comment`,
			expected: `{"key": "value"} `,
		},
		{
			name:     "line comment in middle",
			input:    "{\n  // comment\n  \"key\": \"value\"\n}",
			expected: "{\n  \n  \"key\": \"value\"\n}",
		},
		{
			name:     "comment inside string",
			input:    `{"key": "http://example.com"}`,
			expected: `{"key": "http://example.com"}`,
		},
		{
			name:     "escaped quote in string",
			input:    `{"key": "http://example.com\" // not comment"}`,
			expected: `{"key": "http://example.com\" // not comment"}`,
		},
		{
			name:     "escaped backslash before quote",
			input:    `{"key": "http://example.com\\" // comment}`,
			expected: `{"key": "http://example.com\\" `,
		},
		{
			name:     "multiple comments",
			input:    `// c1` + "\n" + `{"a": 1} // c2` + "\n" + `// c3`,
			expected: "\n" + `{"a": 1} ` + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := StripJSONComments(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSanitizeBytesUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "russian text preserved",
			input:    []byte("Привет, как дела?"),
			expected: "Привет, как дела?",
		},
		{
			name:     "single emoji preserved",
			input:    []byte("Hello 🚀"),
			expected: "Hello 🚀",
		},
		{
			name:     "multiple emoji",
			input:    []byte("🔥 💯 ✨"),
			expected: "🔥 💯 ✨",
		},
		{
			name:     "zwj family emoji",
			input:    []byte("👨‍👩‍👧‍👦"),
			expected: "👨‍👩‍👧‍👦",
		},
		{
			name:     "regional indicator flag",
			input:    []byte("🇷🇺 🇺🇸"),
			expected: "🇷🇺 🇺🇸",
		},
		{
			name:     "skin tone modifier",
			input:    []byte("👍🏼"),
			expected: "👍🏼",
		},
		{
			name:     "chinese text",
			input:    []byte("你好世界"),
			expected: "你好世界",
		},
		{
			name:     "japanese mixed",
			input:    []byte("こんにちは漢字カタカナ"),
			expected: "こんにちは漢字カタカナ",
		},
		{
			name:     "korean",
			input:    []byte("안녕하세요"),
			expected: "안녕하세요",
		},
		{
			name:     "arabic rtl",
			input:    []byte("مرحبا بالعالم"),
			expected: "مرحبا بالعالم",
		},
		{
			name:     "combining diacritics",
			input:    []byte("é (é via combining)"),
			expected: "é (é via combining)",
		},
		{
			name:     "mathematical bold",
			input:    []byte("𝐇𝐞𝐥𝐥𝐨"),
			expected: "𝐇𝐞𝐥𝐥𝐨",
		},
		{
			name:     "truncated rune at end",
			input:    []byte("hello\xc3"),
			expected: "hello�",
		},
		{
			name:     "truncated rune mid",
			input:    []byte("hello\xc3world"),
			expected: "hello�world",
		},
		{
			name:     "lone continuation byte",
			input:    []byte("hello\x80world"),
			expected: "hello�world",
		},
		{
			name:     "overlong encoding rejected",
			input:    []byte("\xc0\x80"),
			expected: "��",
		},
		{
			name:     "BOM at start",
			input:    []byte("\xef\xbb\xbfhello"),
			expected: "hello",
		},
		{
			name:     "russian with control chars",
			input:    []byte("Привет\x01мир"),
			expected: "Приветмир",
		},
		{
			name:     "emoji with embedded control",
			input:    []byte("🚀\x01end"),
			expected: "🚀end",
		},
		{
			name:     "empty input",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "nil input",
			input:    nil,
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeBytes(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
			if !utf8.ValidString(result) {
				t.Errorf("output is not valid UTF-8: %q", result)
			}
		})
	}
}

func TestSanitizeTextUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "russian text preserved",
			input:    "Здравствуй, мир!",
			expected: "Здравствуй, мир!",
		},
		{
			name:     "emoji preserved",
			input:    "Hello 🌍",
			expected: "Hello 🌍",
		},
		{
			name:     "zwj family preserved",
			input:    "👨‍👨‍👦",
			expected: "👨‍👨‍👦",
		},
		{
			name:     "skin tones",
			input:    "👋🏻 👋🏼 👋🏽 👋🏾 👋🏿",
			expected: "👋🏻 👋🏼 👋🏽 👋🏾 👋🏿",
		},
		{
			name:     "cjk preserved",
			input:    "中文日本語한국어",
			expected: "中文日本語한국어",
		},
		{
			name:     "mixed scripts and emoji",
			input:    "Hello мир 🚀 你好",
			expected: "Hello мир 🚀 你好",
		},
		{
			name:     "line separator becomes newline",
			input:    "a b",
			expected: "a\nb",
		},
		{
			name:     "paragraph separator becomes newline",
			input:    "a b",
			expected: "a\nb",
		},
		{
			name:     "tab expanded",
			input:    "a\tб",
			expected: "a    б",
		},
		{
			name:     "BOM stripped from middle too",
			input:    "abc\ufeffdef",
			expected: "abcdef",
		},
		{
			name:     "non-breaking space preserved",
			input:    "a b",
			expected: "a b",
		},
		{
			name:     "soft hyphen removed",
			input:    "abc­def",
			expected: "abcdef",
		},
		{
			name:     "russian with CRLF",
			input:    "строка1\r\nстрока2",
			expected: "строка1\nстрока2",
		},
		{
			name:     "emoji with newline",
			input:    "🚀\n🔥",
			expected: "🚀\n🔥",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeText(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
			if !utf8.ValidString(result) {
				t.Errorf("output is not valid UTF-8: %q", result)
			}
		})
	}
}

func TestSanitizeBytesAndTextAgree(t *testing.T) {
	cases := []string{
		"Hello world",
		"Привет",
		"🚀 emoji",
		"漢字テスト",
		"Mixed: ASCII + кириллица + 🇷🇺",
		"line1\nline2",
		"with\ttab",
		"control\x01char",
		"DEL\x7fchar",
	}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			byteResult := SanitizeBytes([]byte(s))
			textResult := SanitizeText(s)
			if byteResult != textResult {
				t.Errorf("SanitizeBytes(%q)=%q vs SanitizeText(%q)=%q", s, byteResult, s, textResult)
			}
		})
	}
}

func TestStripJSONCommentsUnicode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "russian in string",
			input:    `{"key": "Привет"}`,
			expected: `{"key": "Привет"}`,
		},
		{
			name:     "emoji in string",
			input:    `{"reaction": "🔥"}`,
			expected: `{"reaction": "🔥"}`,
		},
		{
			name:     "comment with russian",
			input:    `{"a": 1} // комментарий`,
			expected: `{"a": 1} `,
		},
		{
			name:     "block comment with emoji",
			input:    `{"a": 1} /* 🔥 */`,
			expected: `{"a": 1} `,
		},
		{
			name:     "// inside russian-containing string",
			input:    `{"url": "Привет//мир"}`,
			expected: `{"url": "Привет//мир"}`,
		},
		{
			name:     "unicode quote escape",
			input:    `{"emoji": "🚀"test"}`,
			expected: `{"emoji": "🚀"test"}`,
		},
		{
			name:     "empty json",
			input:    `{}`,
			expected: `{}`,
		},
		{
			name:     "only comment",
			input:    `// just a comment`,
			expected: ``,
		},
		{
			name:     "unterminated block comment",
			input:    `{"a": 1} /* not closed`,
			expected: `{"a": 1} `,
		},
		{
			name:     "unterminated string preserves rest",
			input:    `{"unclosed: "value"} // comment`,
			expected: `{"unclosed: "value"} // comment`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := StripJSONComments(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestStripJSONCommentsPreservesContentLength(t *testing.T) {
	in := `{"data": ` + strings.Repeat("x", 1000) + `}`
	out := StripJSONComments(in)
	if out != in {
		t.Errorf("input without comments should pass through unchanged")
	}
}
