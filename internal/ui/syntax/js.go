package syntax

func TokenizeJS(src []byte) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/8+8)

	var depth uint8
	var lastKind TokenKind = TokPunctuation
	var lastByte byte
	var haveLast bool
	var pendingName TokenKind

	emit := func(start, end int, kind TokenKind, d uint8) {
		if start >= end {
			return
		}
		out = append(out, Token{Start: start, End: end, Kind: kind, Depth: d})
		lastKind = kind
		lastByte = src[end-1]
		haveLast = true
	}

	regexAllowed := func() bool {
		if !haveLast {
			return true
		}
		switch lastKind {
		case TokPlain, TokNumber, TokString, TokTemplate, TokRegex,
			TokProperty, TokFunction, TokType, TokConstant, TokBool:
			return false
		case TokBracket:
			return lastByte == '{' || lastByte == '}'
		}
		return true
	}

	i := 0
	n := len(src)
	for i < n {
		c := src[i]

		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			i++
			continue
		}

		if c == '/' && i+1 < n && src[i+1] == '/' {
			start := i
			i += 2
			for i < n && src[i] != '\n' {
				i++
			}
			emit(start, i, TokComment, 0)
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '*' {
			start := i
			i += 2
			for i+1 < n {
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			if i+1 >= n {
				i = n
			}
			emit(start, i, TokComment, 0)
			continue
		}

		if c == '/' && regexAllowed() {
			start := i
			i++
			inClass := false
			ok := false
			for i < n {
				d := src[i]
				if d == '\n' {
					break
				}
				if d == '\\' && i+1 < n {
					i += 2
					continue
				}
				if d == '[' {
					inClass = true
				} else if d == ']' {
					inClass = false
				} else if d == '/' && !inClass {
					i++
					ok = true
					break
				}
				i++
			}
			if ok {
				for i < n && isIdentPart(src[i]) {
					i++
				}
				emit(start, i, TokRegex, 0)
				continue
			}
			i = start
		}

		if c == '"' || c == '\'' {
			start := i
			quote := c
			i++
			for i < n {
				b := src[i]
				if b == '\\' && i+1 < n {
					i += 2
					continue
				}
				if b == quote || b == '\n' {
					if b == quote {
						i++
					}
					break
				}
				i++
			}
			emit(start, i, TokString, 0)
			continue
		}
		if c == '`' {
			start := i
			i++
			for i < n {
				b := src[i]
				if b == '\\' && i+1 < n {
					i += 2
					continue
				}
				if b == '`' {
					i++
					break
				}
				i++
			}
			emit(start, i, TokTemplate, 0)
			continue
		}

		if c >= '0' && c <= '9' || (c == '.' && i+1 < n && src[i+1] >= '0' && src[i+1] <= '9') {
			end := scanJSNumber(src, i)
			if end > i {
				emit(i, end, TokNumber, 0)
				i = end
				continue
			}
		}

		if c == '{' || c == '[' || c == '(' {
			emit(i, i+1, TokBracket, depth)
			depth++
			i++
			continue
		}
		if c == '}' || c == ']' || c == ')' {
			if depth > 0 {
				depth--
			}
			emit(i, i+1, TokBracket, depth)
			i++
			continue
		}

		if c == ';' || c == ',' || c == ':' {
			emit(i, i+1, TokPunctuation, 0)
			i++
			continue
		}
		if c == '.' && !(i+1 < n && src[i+1] == '.') {
			emit(i, i+1, TokPunctuation, 0)
			i++
			continue
		}

		if isIdentStart(c) {
			start := i
			i++
			for i < n && isIdentPart(src[i]) {
				i++
			}
			word := string(src[start:i])
			kind := classifyIdent(word, lastByte, pendingName, peekNonSpace(src, i))
			pendingName = 0
			if jsDefiners[word] {
				if word == "class" {
					pendingName = TokType
				} else {
					pendingName = TokFunction
				}
			}
			emit(start, i, kind, 0)
			continue
		}

		if oplen := matchJSOperator(src, i); oplen > 0 {
			emit(i, i+oplen, TokOperator, 0)
			i += oplen
			continue
		}

		emit(i, i+1, TokPlain, 0)
		i++
	}

	return out
}

func classifyIdent(word string, lastByte byte, pendingName TokenKind, next byte) TokenKind {
	if pendingName != 0 {
		return pendingName
	}
	switch {
	case jsKeywords[word]:
		return TokKeyword
	case word == "true" || word == "false":
		return TokBool
	case word == "null":
		return TokNull
	case jsConstants[word]:
		return TokConstant
	case jsBuiltins[word]:
		return TokType
	}
	if lastByte == '.' {
		if next == '(' {
			return TokFunction
		}
		return TokProperty
	}
	if next == '(' {
		return TokFunction
	}
	return TokPlain
}

func peekNonSpace(src []byte, i int) byte {
	for i < len(src) {
		c := src[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' {
			return c
		}
		i++
	}
	return 0
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func scanJSNumber(src []byte, i int) int {
	n := len(src)
	start := i
	if src[i] == '0' && i+1 < n {
		switch src[i+1] {
		case 'x', 'X':
			i += 2
			for i < n && (isHex(src[i]) || src[i] == '_') {
				i++
			}
			if i < n && src[i] == 'n' {
				i++
			}
			return i
		case 'b', 'B', 'o', 'O':
			i += 2
			for i < n && (src[i] == '0' || src[i] == '1' || (src[i] >= '0' && src[i] <= '7') || src[i] == '_') {
				i++
			}
			if i < n && src[i] == 'n' {
				i++
			}
			return i
		}
	}
	for i < n && (isDigit(src[i]) || src[i] == '_') {
		i++
	}
	if i < n && src[i] == '.' {
		i++
		for i < n && (isDigit(src[i]) || src[i] == '_') {
			i++
		}
	}
	if i < n && (src[i] == 'e' || src[i] == 'E') {
		j := i + 1
		if j < n && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j < n && isDigit(src[j]) {
			i = j
			for i < n && (isDigit(src[i]) || src[i] == '_') {
				i++
			}
		}
	}
	if i < n && src[i] == 'n' {
		i++
	}
	if i == start {
		return start
	}
	return i
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func matchJSOperator(src []byte, i int) int {
	rest := src[i:]
	for _, op := range jsOperators {
		if hasBytes(rest, 0, op) {
			return len(op)
		}
	}
	return 0
}

var jsOperators = []string{
	">>>=", "===", "!==", "**=", "<<=", ">>=", ">>>", "&&=", "||=", "??=", "...",
	"=>", "==", "!=", "<=", ">=", "&&", "||", "??", "?.", "**", "++", "--",
	"+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<", ">>",
	"+", "-", "*", "/", "%", "=", "<", ">", "!", "&", "|", "^", "~", "?",
}

var jsDefiners = map[string]bool{
	"function": true, "class": true, "new": true, "get": true, "set": true,
}

var jsKeywords = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "export": true, "extends": true, "finally": true, "for": true,
	"function": true, "if": true, "import": true, "in": true, "instanceof": true,
	"let": true, "new": true, "return": true, "super": true, "switch": true,
	"this": true, "throw": true, "try": true, "typeof": true, "var": true,
	"void": true, "while": true, "with": true, "yield": true, "async": true,
	"await": true, "static": true, "get": true, "set": true, "of": true, "as": true,
	"from": true, "enum": true, "implements": true, "interface": true, "package": true,
	"private": true, "protected": true, "public": true, "readonly": true, "declare": true,
	"namespace": true, "type": true, "abstract": true, "satisfies": true, "keyof": true, "infer": true,
}

var jsConstants = map[string]bool{
	"NaN": true, "Infinity": true, "undefined": true, "globalThis": true, "arguments": true,
}

var jsBuiltins = map[string]bool{
	"console": true, "window": true, "document": true, "Math": true, "JSON": true,
	"Object": true, "Array": true, "String": true, "Number": true, "Boolean": true,
	"Symbol": true, "BigInt": true, "Promise": true, "Map": true, "Set": true,
	"WeakMap": true, "WeakSet": true, "Date": true, "RegExp": true, "Error": true,
	"TypeError": true, "RangeError": true, "SyntaxError": true, "ReferenceError": true,
	"Function": true, "Proxy": true, "Reflect": true, "Intl": true, "ArrayBuffer": true,
	"DataView": true, "Uint8Array": true, "Int8Array": true, "Uint16Array": true,
	"Int16Array": true, "Uint32Array": true, "Int32Array": true, "Float32Array": true,
	"Float64Array": true, "BigInt64Array": true, "BigUint64Array": true,
	"localStorage": true, "sessionStorage": true, "navigator": true, "location": true,
	"history": true, "fetch": true, "setTimeout": true, "setInterval": true,
	"clearTimeout": true, "clearInterval": true, "requestAnimationFrame": true,
	"queueMicrotask": true, "structuredClone": true, "URL": true, "URLSearchParams": true,
	"Headers": true, "Request": true, "Response": true, "FormData": true, "Blob": true,
}
