package syntax

func TokenizeXML(src []byte) []Token {
	return tokenizeMarkup(src, false)
}

func TokenizeHTML(src []byte) []Token {
	return tokenizeMarkup(src, true)
}

func tokenizeMarkup(src []byte, html bool) []Token {
	if len(src) == 0 {
		return nil
	}
	out := make([]Token, 0, len(src)/12+8)
	depth := uint8(0)

	emit := func(start, end int, kind TokenKind, d uint8) {
		if start >= end {
			return
		}
		out = appendToken(out, start, end, kind, d)
	}

	i := 0
	for i < len(src) {
		c := src[i]
		if c != '<' {
			j := i + 1
			for j < len(src) && src[j] != '<' {
				j++
			}
			i = j
			continue
		}

		if i+3 < len(src) && src[i+1] == '!' && src[i+2] == '-' && src[i+3] == '-' {
			start := i
			i += 4
			for i+2 < len(src) {
				if src[i] == '-' && src[i+1] == '-' && src[i+2] == '>' {
					i += 3
					break
				}
				i++
			}
			if i+2 >= len(src) {
				i = len(src)
			}
			emit(start, i, TokComment, 0)
			continue
		}

		if i+8 < len(src) && hasBytes(src, i+1, "![CDATA[") {
			start := i
			i += 9
			for i+2 < len(src) {
				if src[i] == ']' && src[i+1] == ']' && src[i+2] == '>' {
					i += 3
					break
				}
				i++
			}
			if i+2 >= len(src) {
				i = len(src)
			}
			emit(start, i, TokString, 0)
			continue
		}

		if i+1 < len(src) && (src[i+1] == '?' || src[i+1] == '!') {
			start := i
			closer := byte('>')
			i += 2
			for i < len(src) && src[i] != closer {
				i++
			}
			if i < len(src) {
				i++
			}
			emit(start, i, TokKeyword, 0)
			continue
		}

		isClose := i+1 < len(src) && src[i+1] == '/'
		if isClose && depth > 0 {
			depth--
		}
		emit(i, i+1, TokBracket, depth)
		i++
		if isClose {
			i++
			emit(i-1, i, TokPunctuation, 0)
		}

		nameStart := i
		for i < len(src) {
			b := src[i]
			if isXMLNameByte(b) {
				i++
				continue
			}
			break
		}
		if i > nameStart {
			emit(nameStart, i, TokKeyword, 0)
		}
		name := src[nameStart:i]
		var scriptType []byte

		for i < len(src) && src[i] != '>' && (src[i] != '/' || i+1 >= len(src) || src[i+1] != '>') {
			loopStart := i
			b := src[i]
			if b == ' ' || b == '\t' || b == '\n' || b == '\r' {
				i++
				continue
			}
			attrStart := i
			for i < len(src) {
				ab := src[i]
				if isXMLNameByte(ab) {
					i++
					continue
				}
				break
			}
			attrName := src[attrStart:i]
			if i > attrStart {
				emit(attrStart, i, TokKey, 0)
			}
			if i < len(src) && src[i] == '=' {
				emit(i, i+1, TokPunctuation, 0)
				i++
			}
			if i < len(src) && (src[i] == '"' || src[i] == '\'') {
				quote := src[i]
				vstart := i
				i++
				for i < len(src) && src[i] != quote {
					i++
				}
				if i < len(src) {
					i++
				}
				if html && equalFold(attrName, "type") {
					scriptType = src[vstart+1 : i-1]
				}
				emit(vstart, i, TokString, 0)
				continue
			}
			if i < len(src) && src[i] != '>' && src[i] != '/' && src[i] != ' ' && src[i] != '\t' && src[i] != '\n' && src[i] != '\r' {
				vstart := i
				for i < len(src) {
					vb := src[i]
					if vb == ' ' || vb == '\t' || vb == '\n' || vb == '\r' || vb == '>' || vb == '/' {
						break
					}
					i++
				}
				if html && equalFold(attrName, "type") {
					scriptType = src[vstart:i]
				}
				emit(vstart, i, TokString, 0)
				continue
			}
			if i == loopStart {
				i++
				continue
			}
			if i < len(src) && src[i] != '>' && src[i] != '/' {
				i++
			}
		}

		selfClose := false
		if i+1 < len(src) && src[i] == '/' && src[i+1] == '>' {
			emit(i, i+1, TokPunctuation, 0)
			i++
			selfClose = true
		}
		if i < len(src) && src[i] == '>' {
			emit(i, i+1, TokBracket, depth)
			i++
		}
		if isClose || selfClose {
			continue
		}
		if !html {
			depth++
			continue
		}
		if isHTMLVoid(name) {
			continue
		}
		depth++
		if !isHTMLRawText(name) {
			continue
		}
		bodyStart := i
		bodyEnd := findRawTextEnd(src, i, name)
		if equalFold(name, "script") {
			out = appendOffset(out, scriptTokens(src[bodyStart:bodyEnd], scriptType), bodyStart)
		}
		i = bodyEnd
	}

	return out
}

func scriptTokens(body []byte, typ []byte) []Token {
	if len(typ) == 0 {
		return TokenizeJS(body)
	}
	t := toLowerBytes(typ, len(typ))
	for i := 0; i < len(t); i++ {
		if t[i] == ';' {
			t = t[:i]
			break
		}
	}
	ts := trimSpace(string(t))
	switch {
	case ts == "module", ts == "importmap", endsWith(ts, "javascript"), endsWith(ts, "ecmascript"):
		return TokenizeJS(body)
	case ts == "application/json", endsWith(ts, "+json"):
		return TokenizeJSON(body)
	}
	return nil
}

func appendOffset(out, toks []Token, off int) []Token {
	for _, t := range toks {
		t.Start += int32(off)
		out = append(out, t)
	}
	return out
}

func findRawTextEnd(src []byte, i int, name []byte) int {
	n := len(name)
	for ; i+1+n < len(src); i++ {
		if src[i] != '<' || src[i+1] != '/' {
			continue
		}
		if !equalFold(src[i+2:i+2+n], string(name)) {
			continue
		}
		if j := i + 2 + n; j < len(src) && isXMLNameByte(src[j]) {
			continue
		}
		return i
	}
	return len(src)
}

func isHTMLRawText(name []byte) bool {
	return equalFold(name, "script") || equalFold(name, "style") ||
		equalFold(name, "textarea") || equalFold(name, "title")
}

func isHTMLVoid(name []byte) bool {
	switch len(name) {
	case 2:
		return equalFold(name, "br") || equalFold(name, "hr")
	case 3:
		return equalFold(name, "img") || equalFold(name, "col") || equalFold(name, "wbr")
	case 4:
		return equalFold(name, "meta") || equalFold(name, "link") || equalFold(name, "base") ||
			equalFold(name, "area")
	case 5:
		return equalFold(name, "input") || equalFold(name, "embed") || equalFold(name, "param") ||
			equalFold(name, "track")
	case 6:
		return equalFold(name, "source")
	}
	return false
}

func equalFold(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		x, y := b[i], s[i]
		if x >= 'A' && x <= 'Z' {
			x += 'a' - 'A'
		}
		if y >= 'A' && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

func isXMLNameByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '-' || b == '_' || b == '.' || b == ':'
}
