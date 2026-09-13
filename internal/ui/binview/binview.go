package binview

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"

	"rete/internal/utils"
)

type Mode uint8

const (
	ModeHexDump Mode = iota
	ModeHex
	ModeBase64
	ModeBits
	ModeText
	modeCount
)

var modeLabels = [modeCount]string{"Hex dump", "Hex", "Base64", "Bits", "Text"}

func (m Mode) Label() string {
	if m < modeCount {
		return modeLabels[m]
	}
	return "?"
}

const MaxRender = 256 * 1024

const sniffWindow = 8 * 1024

func IsBinary(b []byte, mime string) bool {
	if len(b) == 0 {
		return false
	}
	sample := b
	if len(sample) > sniffWindow {
		sample = sample[:sniffWindow]
	}
	if utils.SniffCharsetBOM(sample) != "" {
		return false
	}
	switch mimeClass(mime) {
	case mimeText:
		return bytes.IndexByte(sample, 0) >= 0
	case mimeBinary:
		return true
	}
	control := 0
	for _, c := range sample {
		if c == 0 {
			return true
		}
		if c < 0x09 || (c > 0x0d && c < 0x20 && c != 0x1b) {
			control++
		}
	}
	return control*100/len(sample) >= 8
}

type mimeKind uint8

const (
	mimeUnknown mimeKind = iota
	mimeText
	mimeBinary
)

func mimeClass(mime string) mimeKind {
	mt := strings.ToLower(strings.TrimSpace(mime))
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = strings.TrimSpace(mt[:i])
	}
	if mt == "" {
		return mimeUnknown
	}
	if strings.HasPrefix(mt, "text/") ||
		strings.HasSuffix(mt, "+json") || strings.HasSuffix(mt, "+xml") ||
		strings.Contains(mt, "json") || strings.Contains(mt, "xml") ||
		strings.Contains(mt, "javascript") || strings.Contains(mt, "ecmascript") ||
		strings.Contains(mt, "x-www-form-urlencoded") || strings.Contains(mt, "yaml") ||
		strings.Contains(mt, "graphql") || strings.Contains(mt, "csv") ||
		mt == "image/svg+xml" || mt == "application/x-sh" || mt == "application/sql" {
		return mimeText
	}
	if strings.HasPrefix(mt, "image/") || strings.HasPrefix(mt, "audio/") ||
		strings.HasPrefix(mt, "video/") || strings.HasPrefix(mt, "font/") ||
		mt == "application/octet-stream" || mt == "application/pdf" ||
		strings.Contains(mt, "zip") || strings.Contains(mt, "gzip") ||
		strings.Contains(mt, "protobuf") || strings.Contains(mt, "msgpack") ||
		strings.Contains(mt, "wasm") || strings.Contains(mt, "x-tar") ||
		strings.Contains(mt, "x-7z") || strings.Contains(mt, "x-rar") ||
		strings.Contains(mt, "vnd.openxmlformats") || strings.Contains(mt, "msword") ||
		strings.Contains(mt, "vnd.ms-") || strings.Contains(mt, "x-sqlite") ||
		strings.Contains(mt, "x-executable") || strings.Contains(mt, "x-msdownload") {
		return mimeBinary
	}
	return mimeUnknown
}

func Format(b []byte, m Mode) string {
	return FormatTotal(b, m, len(b))
}

// FormatTotal renders at most MaxRender bytes of b and, when total says the
// source is longer than what was rendered, ends with a line saying so.
func FormatTotal(b []byte, m Mode, total int) string {
	if len(b) > MaxRender {
		b = b[:MaxRender]
	}
	if total < len(b) {
		total = len(b)
	}
	var out string
	switch m {
	case ModeHex:
		out = hexPlain(b)
	case ModeBase64:
		out = base64Wrapped(b)
	case ModeBits:
		out = bits(b)
	case ModeText:
		out = utils.SanitizeBytes(b)
	default:
		out = HexDump(b)
	}
	if total > len(b) {
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		out += "… showing first " + strconv.Itoa(len(b)) + " of " + strconv.Itoa(total) + " bytes"
	}
	return out
}

const hexChars = "0123456789abcdef"

func HexDump(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	const perLine = 16
	var sb strings.Builder
	sb.Grow((len(b)/perLine + 1) * 80)
	var off [8]byte
	for pos := 0; pos < len(b); pos += perLine {
		end := min(pos+perLine, len(b))
		v := pos
		for i := 7; i >= 0; i-- {
			off[i] = hexChars[v&0xf]
			v >>= 4
		}
		sb.Write(off[:])
		sb.WriteString("  ")
		for i := pos; i < pos+perLine; i++ {
			if i < end {
				sb.WriteByte(hexChars[b[i]>>4])
				sb.WriteByte(hexChars[b[i]&0xf])
				sb.WriteByte(' ')
			} else {
				sb.WriteString("   ")
			}
			if i == pos+7 {
				sb.WriteByte(' ')
			}
		}
		sb.WriteString(" |")
		for i := pos; i < end; i++ {
			c := b[i]
			if c >= 0x20 && c < 0x7f {
				sb.WriteByte(c)
			} else {
				sb.WriteByte('.')
			}
		}
		sb.WriteString("|\n")
	}
	return sb.String()
}

func hexPlain(b []byte) string {
	const perLine = 32
	var sb strings.Builder
	sb.Grow(len(b)*2 + len(b)/perLine + 1)
	for i, c := range b {
		if i > 0 && i%perLine == 0 {
			sb.WriteByte('\n')
		}
		sb.WriteByte(hexChars[c>>4])
		sb.WriteByte(hexChars[c&0xf])
	}
	return sb.String()
}

func base64Wrapped(b []byte) string {
	const perLine = 76
	enc := base64.StdEncoding.EncodeToString(b)
	if len(enc) <= perLine {
		return enc
	}
	var sb strings.Builder
	sb.Grow(len(enc) + len(enc)/perLine + 1)
	for i := 0; i < len(enc); i += perLine {
		end := min(i+perLine, len(enc))
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(enc[i:end])
	}
	return sb.String()
}

func bits(b []byte) string {
	const perLine = 8
	var sb strings.Builder
	sb.Grow(len(b) * 9)
	for i, c := range b {
		if i > 0 {
			if i%perLine == 0 {
				sb.WriteByte('\n')
			} else {
				sb.WriteByte(' ')
			}
		}
		for bit := 7; bit >= 0; bit-- {
			if c&(1<<uint(bit)) != 0 {
				sb.WriteByte('1')
			} else {
				sb.WriteByte('0')
			}
		}
	}
	return sb.String()
}
