package har

import (
	"bytes"
	"encoding/json"
	"strings"
)

func Pretty(body []byte, mime string) ([]byte, bool) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return body, false
	}
	if !looksLikeJSON(trimmed, mime) {
		return body, false
	}
	var out bytes.Buffer
	if err := json.Indent(&out, trimmed, "", "  "); err != nil {
		return body, false
	}
	return out.Bytes(), true
}

func PrettyCode(body []byte, mime string) ([]byte, bool) {
	if out, ok := Pretty(body, mime); ok {
		return out, true
	}
	m := strings.ToLower(mime)
	switch {
	case isMarkupMime(m):
		return beautifyMarkup(body)
	case isCSSMime(m):
		return beautifyBraces(body, true)
	case isJSMime(m):
		return beautifyBraces(body, false)
	}
	trimmed := bytes.TrimSpace(body)
	if looksLikeMarkupContent(trimmed) {
		return beautifyMarkup(body)
	}
	if looksLikeBraceCode(body, mime) {
		return beautifyBraces(body, false)
	}
	return body, false
}

func isMarkupMime(m string) bool {
	return strings.Contains(m, "html") || strings.Contains(m, "xml") ||
		strings.Contains(m, "svg") || strings.Contains(m, "xhtml")
}

func isCSSMime(m string) bool { return strings.Contains(m, "css") }

func isJSMime(m string) bool {
	return strings.Contains(m, "javascript") || strings.Contains(m, "ecmascript") ||
		strings.Contains(m, "typescript") || strings.Contains(m, "jscript")
}

func looksLikeMarkupContent(trimmed []byte) bool {
	if len(trimmed) == 0 || trimmed[0] != '<' {
		return false
	}
	tags := 0
	limit := len(trimmed)
	if limit > 4096 {
		limit = 4096
	}
	for i := 0; i < limit-1; i++ {
		if trimmed[i] == '<' {
			c := trimmed[i+1]
			if c == '/' || c == '!' || c == '?' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				tags++
			}
		}
	}
	return tags >= 2
}

func looksLikeJSON(trimmed []byte, mime string) bool {
	if strings.Contains(strings.ToLower(mime), "json") {
		return true
	}
	if len(trimmed) == 0 {
		return false
	}
	switch trimmed[0] {
	case '{', '[':
		return true
	}
	return false
}

func looksLikeBraceCode(body []byte, mime string) bool {
	m := strings.ToLower(mime)
	if strings.Contains(m, "javascript") || strings.Contains(m, "ecmascript") ||
		strings.Contains(m, "typescript") || strings.Contains(m, "/css") {
		return true
	}
	nl, braces, semis := 0, 0, 0
	limit := len(body)
	if limit > 4096 {
		limit = 4096
	}
	for i := 0; i < limit; i++ {
		switch body[i] {
		case '\n':
			nl++
		case '{', '}':
			braces++
		case ';':
			semis++
		}
	}
	return braces+semis >= 6 && nl <= limit/200
}

func regexCanStart(last byte) bool {
	switch last {
	case 0, '(', ',', '=', ':', '[', '!', '&', '|', '?', '{', '}', ';', '+', '-', '*', '%', '<', '>', '^', '~':
		return true
	}
	return false
}

func beautifyBraces(src []byte, css bool) ([]byte, bool) {
	var out []byte
	indent := 0
	n := len(src)

	writeIndent := func() {
		for i := 0; i < indent; i++ {
			out = append(out, ' ', ' ')
		}
	}
	trimInlineTrailing := func() {
		for len(out) > 0 && (out[len(out)-1] == ' ' || out[len(out)-1] == '\t') {
			out = out[:len(out)-1]
		}
	}
	newline := func() {
		trimInlineTrailing()
		out = append(out, '\n')
		writeIndent()
	}
	atLineStart := func() bool {
		for i := len(out) - 1; i >= 0; i-- {
			switch out[i] {
			case ' ', '\t':
				continue
			case '\n':
				return true
			default:
				return false
			}
		}
		return true
	}
	peekNonSpace := func(i int) byte {
		for i < n {
			if src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
				return src[i]
			}
			i++
		}
		return 0
	}
	isWord := func(b byte) bool {
		return b == '_' || b == '$' || (b >= 'a' && b <= 'z') ||
			(b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
	}
	peekWord := func(i int) string {
		for i < n && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
			i++
		}
		start := i
		for i < n && isWord(src[i]) {
			i++
		}
		return string(src[start:i])
	}

	var paren int
	var lastSig byte
	i := 0
	for i < n {
		c := src[i]

		if c == '"' || c == '\'' || c == '`' {
			quote := c
			out = append(out, c)
			i++
			for i < n {
				d := src[i]
				out = append(out, d)
				i++
				if d == '\\' && i < n {
					out = append(out, src[i])
					i++
					continue
				}
				if d == quote {
					break
				}
			}
			lastSig = quote
			continue
		}

		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				out = append(out, src[i])
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '*' {
			out = append(out, '/', '*')
			i += 2
			for i < n {
				if src[i] == '*' && i+1 < n && src[i+1] == '/' {
					out = append(out, '*', '/')
					i += 2
					break
				}
				out = append(out, src[i])
				i++
			}
			continue
		}
		if c == '/' && regexCanStart(lastSig) {
			out = append(out, '/')
			i++
			inClass := false
			for i < n {
				d := src[i]
				out = append(out, d)
				i++
				if d == '\\' && i < n {
					out = append(out, src[i])
					i++
					continue
				}
				switch d {
				case '[':
					inClass = true
				case ']':
					inClass = false
				case '/':
					if !inClass {
						lastSig = '/'
						goto regexDone
					}
				}
			}
		regexDone:
			continue
		}

		switch c {
		case '{':
			out = append(out, '{')
			indent++
			i++
			newline()
			lastSig = '{'
		case '}':
			if indent > 0 {
				indent--
			}
			if atLineStart() {
				trimInlineTrailing()
				writeIndent()
			} else {
				newline()
			}
			out = append(out, '}')
			lastSig = '}'
			i++
			switch nx := peekNonSpace(i); {
			case nx == ')' || nx == ']' || nx == ',' || nx == ';' || nx == '.' && !css || nx == 0:
			case !css && (nx == '=' || nx == '&' || nx == '|' || nx == '?' || nx == ':' ||
				nx == '+' || nx == '-' || nx == '*' || nx == '%' || nx == '<' || nx == '>' ||
				nx == '(' || nx == '`'):
			default:
				switch peekWord(i) {
				case "else", "catch", "finally", "while":
					out = append(out, ' ')
				default:
					newline()
				}
			}
		case ';':
			out = append(out, ';')
			i++
			if paren > 0 {
				if nx := peekNonSpace(i); nx != 0 && nx != ')' {
					out = append(out, ' ')
				}
				lastSig = ';'
				continue
			}
			newline()
			lastSig = ';'
		case '(':
			out = append(out, '(')
			paren++
			i++
			lastSig = '('
		case ')':
			out = append(out, ')')
			if paren > 0 {
				paren--
			}
			i++
			lastSig = ')'
		case ' ', '\t':
			if !atLineStart() {
				out = append(out, ' ')
			}
			i++
		case '\n', '\r':
			i++
		default:
			out = append(out, c)
			lastSig = c
			i++
		}
	}

	res := bytes.TrimRight(collapseBlankLines(out), "\n")
	return res, !bytes.Equal(res, bytes.TrimRight(src, "\n"))
}

func collapseBlankLines(b []byte) []byte {
	var out []byte
	nlRun := 0
	for _, c := range b {
		if c == '\n' {
			nlRun++
			if nlRun > 2 {
				continue
			}
		} else {
			nlRun = 0
		}
		out = append(out, c)
	}
	return out
}
