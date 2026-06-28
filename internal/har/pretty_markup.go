package har

import (
	"bytes"
	"strings"
)

var markupVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var markupRawElements = map[string]bool{
	"script": true, "style": true, "pre": true, "textarea": true,
}

type mtokKind uint8

const (
	mtokText mtokKind = iota
	mtokOpen
	mtokClose
	mtokSelf
	mtokComment
	mtokDecl
	mtokRaw
)

type mtok struct {
	kind mtokKind
	raw  string
	name string
}

func beautifyMarkup(src []byte) ([]byte, bool) {
	toks := scanMarkup(src)
	if len(toks) == 0 {
		return src, false
	}

	var out []byte
	indent := 0
	writeLine := func(s string) {
		if len(out) > 0 {
			out = append(out, '\n')
		}
		for i := 0; i < indent; i++ {
			out = append(out, ' ', ' ')
		}
		out = append(out, s...)
	}

	for i := 0; i < len(toks); i++ {
		t := toks[i]
		switch t.kind {
		case mtokClose:
			if indent > 0 {
				indent--
			}
			writeLine(t.raw)
		case mtokOpen:
			if i+2 < len(toks) && toks[i+1].kind == mtokText &&
				toks[i+2].kind == mtokClose && toks[i+2].name == t.name {
				writeLine(t.raw + toks[i+1].raw + toks[i+2].raw)
				i += 2
				continue
			}
			if i+1 < len(toks) && toks[i+1].kind == mtokClose && toks[i+1].name == t.name {
				writeLine(t.raw + toks[i+1].raw)
				i++
				continue
			}
			writeLine(t.raw)
			indent++
		case mtokRaw:
			for _, line := range strings.Split(t.raw, "\n") {
				writeLine(line)
			}
		default:
			writeLine(t.raw)
		}
	}

	res := bytes.TrimRight(out, "\n")
	return res, !bytes.Equal(res, bytes.TrimRight(src, "\n"))
}

func scanMarkup(src []byte) []mtok {
	var toks []mtok
	n := len(src)
	i := 0
	for i < n {
		if src[i] != '<' {
			start := i
			for i < n && src[i] != '<' {
				i++
			}
			if txt := strings.TrimSpace(string(src[start:i])); txt != "" {
				toks = append(toks, mtok{kind: mtokText, raw: txt})
			}
			continue
		}

		if bytes.HasPrefix(src[i:], []byte("<!--")) {
			end := indexFrom(src, i+4, []byte("-->"))
			if end < 0 {
				end = n
			} else {
				end += 3
			}
			toks = append(toks, mtok{kind: mtokComment, raw: string(src[i:end])})
			i = end
			continue
		}
		if bytes.HasPrefix(src[i:], []byte("<![CDATA[")) {
			end := indexFrom(src, i+9, []byte("]]>"))
			if end < 0 {
				end = n
			} else {
				end += 3
			}
			toks = append(toks, mtok{kind: mtokDecl, raw: string(src[i:end])})
			i = end
			continue
		}
		if i+1 < n && (src[i+1] == '!' || src[i+1] == '?') {
			end := scanTagEnd(src, i)
			toks = append(toks, mtok{kind: mtokDecl, raw: strings.TrimSpace(string(src[i:end]))})
			i = end
			continue
		}

		end := scanTagEnd(src, i)
		raw := strings.TrimSpace(string(src[i:end]))
		name := tagName(raw)
		closing := strings.HasPrefix(raw, "</")
		selfClose := strings.HasSuffix(raw, "/>") || markupVoidElements[name]

		switch {
		case closing:
			toks = append(toks, mtok{kind: mtokClose, raw: raw, name: name})
			i = end
		case selfClose:
			toks = append(toks, mtok{kind: mtokSelf, raw: raw, name: name})
			i = end
		default:
			toks = append(toks, mtok{kind: mtokOpen, raw: raw, name: name})
			i = end
			if markupRawElements[name] {
				closeTag := "</" + name
				bodyEnd := indexFromFold(src, i, []byte(closeTag))
				if bodyEnd < 0 {
					bodyEnd = n
				}
				body := src[i:bodyEnd]
				if r := rawElementBody(name, body); r != "" {
					toks = append(toks, mtok{kind: mtokRaw, raw: r})
				}
				i = bodyEnd
			}
		}
	}
	return toks
}

func rawElementBody(name string, body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return ""
	}
	if name == "script" || name == "style" {
		if out, ok := beautifyBraces(trimmed, name == "style"); ok {
			return string(bytes.TrimRight(out, "\n"))
		}
	}
	return string(trimmed)
}

func scanTagEnd(src []byte, i int) int {
	n := len(src)
	i++
	for i < n {
		c := src[i]
		if c == '"' || c == '\'' {
			i++
			for i < n && src[i] != c {
				i++
			}
			i++
			continue
		}
		if c == '>' {
			return i + 1
		}
		i++
	}
	return n
}

func tagName(raw string) string {
	s := strings.TrimPrefix(raw, "<")
	s = strings.TrimPrefix(s, "/")
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
		start++
	}
	end := start
	for end < len(s) {
		c := s[end]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '>' || c == '/' {
			break
		}
		end++
	}
	return strings.ToLower(s[start:end])
}

func indexFrom(src []byte, from int, sep []byte) int {
	if from < 0 {
		from = 0
	}
	if from >= len(src) {
		return -1
	}
	idx := bytes.Index(src[from:], sep)
	if idx < 0 {
		return -1
	}
	return from + idx
}

func indexFromFold(src []byte, from int, sep []byte) int {
	if from < 0 {
		from = 0
	}
	lower := bytes.ToLower(src[from:])
	idx := bytes.Index(lower, bytes.ToLower(sep))
	if idx < 0 {
		return -1
	}
	return from + idx
}
