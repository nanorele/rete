package utils

import (
	"bytes"
	"mime"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/htmlindex"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// CharsetFromContentType extracts the charset parameter from a
// Content-Type header value, normalised to lower case. Returns "" if the
// header is empty, unparseable, or has no charset parameter.
func CharsetFromContentType(ct string) string {
	if ct == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(params["charset"]))
}

// SniffCharsetBOM returns the encoding label implied by a byte-order
// mark at the start of data, or "" if no BOM is present. The detected
// labels are the same ones charsetEncoding understands.
func SniffCharsetBOM(data []byte) string {
	switch {
	case len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0xFE && data[3] == 0xFF:
		return "utf-32be"
	case len(data) >= 4 && data[0] == 0xFF && data[1] == 0xFE && data[2] == 0x00 && data[3] == 0x00:
		return "utf-32le"
	case len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF:
		return "utf-8"
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		return "utf-16be"
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		return "utf-16le"
	}
	return ""
}

// SniffCharsetXML looks for an `encoding="..."` attribute inside the
// leading XML declaration `<?xml ... ?>`. Returns "" if there is no
// declaration in the first 256 bytes or no encoding attribute on it.
// Bytes are assumed to be ASCII-compatible (no UTF-16 prologue) — if a
// UTF-16 BOM was present, SniffCharsetBOM should have caught it first.
func SniffCharsetXML(data []byte) string {
	const window = 256
	if len(data) > window {
		data = data[:window]
	}
	if len(data) < 6 || data[0] != '<' || data[1] != '?' {
		return ""
	}
	lower := bytes.ToLower(data)
	if !bytes.HasPrefix(lower, []byte("<?xml")) {
		return ""
	}
	end := bytes.Index(lower, []byte("?>"))
	if end < 0 {
		return ""
	}
	decl := lower[:end]
	idx := bytes.Index(decl, []byte("encoding"))
	if idx < 0 {
		return ""
	}
	rest := decl[idx+len("encoding"):]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	if len(rest) == 0 || rest[0] != '=' {
		return ""
	}
	rest = rest[1:]
	for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
		rest = rest[1:]
	}
	if len(rest) == 0 || (rest[0] != '"' && rest[0] != '\'') {
		return ""
	}
	quote := rest[0]
	rest = rest[1:]
	stop := bytes.IndexByte(rest, quote)
	if stop < 0 {
		return ""
	}
	return string(rest[:stop])
}

// SniffCharsetHTML scans the leading bytes of an HTML document for a
// charset declaration (<meta charset="..."> or <meta http-equiv="..."
// content="...; charset=...">). It only inspects the first 4 KiB, which
// is what browsers do. Returns "" if no declaration is found.
func SniffCharsetHTML(data []byte) string {
	const window = 4096
	if len(data) > window {
		data = data[:window]
	}
	lower := bytes.ToLower(data)

	for i := 0; i < len(lower); {
		idx := bytes.Index(lower[i:], []byte("<meta"))
		if idx < 0 {
			return ""
		}
		start := i + idx
		end := bytes.IndexByte(lower[start:], '>')
		if end < 0 {
			return ""
		}
		tag := lower[start : start+end]

		if cs := extractCharsetAttr(tag); cs != "" {
			return cs
		}
		i = start + end + 1
	}
	return ""
}

func extractCharsetAttr(tag []byte) string {
	if idx := bytes.Index(tag, []byte("charset")); idx >= 0 {
		rest := tag[idx+len("charset"):]
		for j := range rest {
			c := rest[j]
			if c == ' ' || c == '\t' {
				continue
			}
			if c != '=' {
				break
			}
			rest = rest[j+1:]
			for len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t' || rest[0] == '"' || rest[0] == '\'') {
				rest = rest[1:]
			}
			end := 0
			for end < len(rest) {
				c := rest[end]
				if c == ' ' || c == '\t' || c == '"' || c == '\'' || c == ';' || c == '/' || c == '>' {
					break
				}
				end++
			}
			return string(rest[:end])
		}
	}
	return ""
}

// charsetEncoding resolves a charset name to a Go encoding. It returns
// nil for UTF-8 (since no decoding is needed) and for unknown labels.
func charsetEncoding(name string) encoding.Encoding {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil
	}
	switch name {
	case "utf-8", "utf8", "us-ascii", "ascii":
		return nil
	case "utf-16":
		return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
	case "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM)
	case "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM)
	}
	enc, err := htmlindex.Get(name)
	if err != nil {
		return nil
	}
	if enc == unicode.UTF8 || enc == nil {
		return nil
	}
	return enc
}

// SniffCharset combines BOM, XML declaration, and HTML meta sniffing in
// the order browsers do. Used when Content-Type carries no charset.
func SniffCharset(data []byte, contentType string) string {
	if cs := SniffCharsetBOM(data); cs != "" {
		return cs
	}
	if cs := SniffCharsetXML(data); cs != "" {
		return cs
	}
	if isHTMLContentType(contentType) {
		if cs := SniffCharsetHTML(data); cs != "" {
			return cs
		}
	}
	return ""
}

// DecodeBody converts a response body from the encoding declared in its
// Content-Type header to UTF-8. If the charset is empty, utf-8, or
// unknown, the body is sniffed (BOM → <?xml encoding> → <meta charset>
// for HTML); the input is returned unchanged when no charset can be
// established.
func DecodeBody(data []byte, contentType string) []byte {
	cs := CharsetFromContentType(contentType)
	if cs == "" {
		cs = SniffCharset(data, contentType)
	}
	enc := charsetEncoding(cs)
	if enc == nil {
		return stripUTF8BOM(data)
	}
	out, _, err := transform.Bytes(enc.NewDecoder(), data)
	if err != nil {
		return data
	}
	return stripUTF8BOM(out)
}

// CharsetDecoder returns a streaming transformer for the encoding
// declared in contentType, or nil if no decoding is needed. The
// returned transformer is stateful — callers must keep using the same
// instance across chunks of the same response body.
func CharsetDecoder(contentType string) *encoding.Decoder {
	enc := charsetEncoding(CharsetFromContentType(contentType))
	if enc == nil {
		return nil
	}
	return enc.NewDecoder()
}

// CharsetDecoderForBody is like CharsetDecoder but, when contentType
// has no explicit charset, falls back to sniffing the leading bytes
// (BOM / XML decl / HTML meta). Pass the first few KiB of body so the
// sniff has something to look at.
func CharsetDecoderForBody(probe []byte, contentType string) *encoding.Decoder {
	cs := CharsetFromContentType(contentType)
	if cs == "" {
		cs = SniffCharset(probe, contentType)
	}
	enc := charsetEncoding(cs)
	if enc == nil {
		return nil
	}
	return enc.NewDecoder()
}

func stripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}

func isHTMLContentType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	mt = strings.ToLower(mt)
	return mt == "text/html" || mt == "application/xhtml+xml"
}
