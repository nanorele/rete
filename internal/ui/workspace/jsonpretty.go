package workspace

import (
	"bytes"
	"strings"
	"unsafe"
)

const (
	maxIndentDepth   = 63
	jsonParallelMin  = 2 << 20
	jsonMinChunkSize = 256 << 10
	jsonMaxWorkers   = 16
)

type JSONFormatterState struct {
	Indent     int
	InString   bool
	NeedIndent bool
	EscapeNext bool
}

var indentTable [maxIndentDepth + 1]string

var jsonTokenEnd [256]bool

func init() {
	for i := range indentTable {
		indentTable[i] = "\n" + strings.Repeat("  ", i)
	}
	for _, c := range []byte{',', '}', ']', ':', ' ', '\t', '\n', '\r'} {
		jsonTokenEnd[c] = true
	}
}

func appendIndent(out []byte, indent int) []byte {
	if indent < 0 {
		return out
	}
	if indent > maxIndentDepth {
		indent = maxIndentDepth
	}
	return append(out, indentTable[indent]...)
}

func appendFormatJSON(out, data []byte, st *JSONFormatterState) []byte {
	n := len(data)
	i := 0
	for i < n {
		if st.InString {
			j := i
			if st.EscapeNext {
				st.EscapeNext = false
				j++
			}
			for {
				if j >= n {
					out = append(out, data[i:]...)
					i = n
					break
				}
				q := bytes.IndexByte(data[j:], '"')
				end := n
				if q >= 0 {
					end = j + q
				}
				if bs := bytes.IndexByte(data[j:end], '\\'); bs >= 0 {
					j += bs + 2
					if j > n {
						st.EscapeNext = true
						out = append(out, data[i:]...)
						i = n
						break
					}
					continue
				}
				if q < 0 {
					out = append(out, data[i:]...)
					i = n
					break
				}
				out = append(out, data[i:end+1]...)
				i = end + 1
				st.InString = false
				break
			}
			continue
		}

		b := data[i]
		i++
		switch b {
		case '"':
			if st.NeedIndent {
				out = appendIndent(out, st.Indent)
				st.NeedIndent = false
			}
			out = append(out, '"')
			st.InString = true
		case '{', '[':
			if st.NeedIndent {
				out = appendIndent(out, st.Indent)
				st.NeedIndent = false
			}
			out = append(out, b)
			j := i
			for j < n && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < n && ((b == '{' && data[j] == '}') || (b == '[' && data[j] == ']')) {
				out = append(out, data[j])
				i = j + 1
				continue
			}
			st.Indent++
			st.NeedIndent = true
		case '}', ']':
			st.Indent--
			if st.Indent < 0 {
				st.Indent = 0
			}
			out = appendIndent(out, st.Indent)
			out = append(out, b)
		case ',':
			out = append(out, ',')
			st.NeedIndent = true
		case ':':
			out = append(out, ':', ' ')
		case ' ', '\t', '\n', '\r':
		default:
			if st.NeedIndent {
				out = appendIndent(out, st.Indent)
				st.NeedIndent = false
			}
			start := i - 1
			for i < n && !jsonTokenEnd[data[i]] {
				i++
			}
			out = append(out, data[start:i]...)
		}
	}
	return out
}

func formatJSON(data []byte, state *JSONFormatterState) string {
	if len(data) >= jsonParallelMin {
		if s, ok := formatJSONParallel(data, state); ok {
			return s
		}
	}
	out := appendFormatJSON(make([]byte, 0, len(data)*3), data, state)
	if len(out) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(out), len(out))
}
