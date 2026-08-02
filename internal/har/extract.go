package har

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Resource struct {
	EntryIndex int
	URL        string
	Host       string
	ZipPath    string
	MimeType   string
	Method     string
	Status     int

	// Body holds the content of a resource assembled by hand. Resources built
	// from an archive leave it nil and decode through [Resource.Bytes]
	// instead: an archive yields one resource per entry, so keeping a decoded
	// copy of each doubled the memory a loaded HAR needs.
	Body []byte
	// Size is the decoded length, known without materialising the body.
	Size int

	entry *Entry
	ws    bool
}

// Bytes returns the resource content, decoding it from the source entry when
// the resource was built from an archive.
func (r Resource) Bytes() []byte {
	if r.Body != nil || r.entry == nil {
		return r.Body
	}
	if r.ws {
		return WSTranscript(r.entry)
	}
	body, _, err := r.entry.DecodeBody()
	if err != nil {
		return []byte(r.entry.Response.Content.Text)
	}
	return body
}

func (e Entry) DecodeBody() ([]byte, bool, error) {
	text := e.Response.Content.Text
	if text == "" {
		return nil, false, nil
	}
	if strings.EqualFold(e.Response.Content.Encoding, "base64") {
		dec, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, true, err
		}
		return dec, true, nil
	}
	return []byte(text), true, nil
}

func (e Entry) IsJavaScript() bool {
	mime := strings.ToLower(e.ContentType())
	if strings.Contains(mime, "javascript") || strings.Contains(mime, "ecmascript") {
		return true
	}
	if u, err := url.Parse(e.Request.URL); err == nil {
		if strings.HasSuffix(strings.ToLower(u.Path), ".js") {
			return true
		}
	}
	return false
}

func ZipPath(rawURL, mime string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u == nil {
		return path.Join("_invalid", sanitize(rawURL))
	}

	host := u.Host
	if host == "" {
		host = "_nohost"
	}
	host = sanitizeSegment(host)

	p := u.Path
	if p == "" || strings.HasSuffix(p, "/") {
		p += "index"
	}
	if pathExt(p) == "" && strings.Contains(strings.ToLower(mime), "javascript") {
		p += ".js"
	}
	if u.RawQuery != "" {
		p += "__" + sanitize(u.RawQuery)
	}

	segs := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segs {
		segs[i] = sanitizeSegment(s)
	}
	return path.Join(append([]string{host}, segs...)...)
}

func pathExt(p string) string {
	base := p
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		base = p[i+1:]
	}
	if i := strings.LastIndexByte(base, '.'); i > 0 {
		return base[i:]
	}
	return ""
}

func (h *HAR) Resources(jsOnly bool) []Resource {
	var out []Resource
	for i := range h.Entries {
		e := &h.Entries[i]
		if jsOnly && !e.IsJavaScript() {
			continue
		}
		if r, ok := e.bodyResource(i); ok {
			out = append(out, r)
		}
		if jsOnly {
			continue
		}
		if r, ok := e.wsResource(i); ok {
			out = append(out, r)
		}
	}
	return out
}

func hostOf(rawURL string) string {
	if u, err := url.Parse(rawURL); err == nil && u != nil {
		return u.Host
	}
	return ""
}

func (e *Entry) bodyResource(i int) (Resource, bool) {
	text := e.Response.Content.Text
	if text == "" {
		return Resource{}, false
	}
	size := len(text)
	if strings.EqualFold(e.Response.Content.Encoding, "base64") {
		if n, ok := base64DecodedSize(text); ok {
			size = n
		}
	}
	return Resource{
		EntryIndex: i,
		URL:        e.Request.URL,
		Host:       hostOf(e.Request.URL),
		ZipPath:    ZipPath(e.Request.URL, e.ContentType()),
		MimeType:   e.ContentType(),
		Method:     e.Request.Method,
		Status:     e.Response.Status,
		Size:       size,
		entry:      e,
	}, true
}

// base64DecodedSize counts the decoded length without decoding: the count of
// alphabet characters determines it, and inputs [Entry.DecodeBody] would
// reject are reported as not base64 so Size falls back to the raw length.
func base64DecodedSize(text string) (int, bool) {
	n, pad := 0, 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '\r' || c == '\n':
		case c == '=':
			n++
			pad++
		case pad > 0:
			return 0, false
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z',
			c >= '0' && c <= '9', c == '+', c == '/':
			n++
		default:
			return 0, false
		}
	}
	if n%4 != 0 || pad > 2 {
		return 0, false
	}
	return n/4*3 - pad, true
}

func (e *Entry) wsResource(i int) (Resource, bool) {
	if len(e.WebSocketMessages) == 0 {
		return Resource{}, false
	}
	return Resource{
		EntryIndex: i,
		URL:        e.Request.URL,
		Host:       hostOf(e.Request.URL),
		ZipPath:    ZipPath(e.Request.URL, "text/plain") + ".ws.txt",
		MimeType:   "text/plain",
		Method:     "WS",
		Status:     e.Response.Status,
		Size:       len(WSTranscript(e)),
		entry:      e,
		ws:         true,
	}, true
}

func WSTranscript(e *Entry) []byte {
	var b bytes.Buffer
	last := len(e.WebSocketMessages) - 1
	for i, m := range e.WebSocketMessages {
		dir := "<< receive"
		if m.Sent() {
			dir = ">> send"
		}
		kind := "text"
		if m.Binary() {
			kind = "binary"
		}
		b.WriteString("#")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" ")
		b.WriteString(dir)
		b.WriteString(" [")
		b.WriteString(kind)
		b.WriteString("] t=")
		b.WriteString(strconv.FormatFloat(m.Time, 'f', -1, 64))
		b.WriteByte('\n')
		if m.Binary() {
			if raw, derr := base64.StdEncoding.DecodeString(m.Data); derr == nil {
				b.Write(raw)
			} else {
				b.WriteString(m.Data)
			}
		} else {
			b.WriteString(m.Data)
		}
		b.WriteByte('\n')
		if i != last {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}

func dedupePaths(res []Resource) []Resource {
	seen := map[string]int{}
	out := make([]Resource, len(res))
	copy(out, res)
	for i := range out {
		p := out[i].ZipPath
		if n, ok := seen[p]; ok {
			seen[p] = n + 1
			out[i].ZipPath = suffixBeforeExt(p, n+1)
		} else {
			seen[p] = 0
		}
	}
	return out
}

func suffixBeforeExt(p string, n int) string {
	ext := pathExt(p)
	stem := strings.TrimSuffix(p, ext)
	return stem + "-" + itoa(n) + ext
}

func WriteZip(w io.Writer, resources []Resource) (int, error) {
	zw := zip.NewWriter(w)
	written := 0
	for _, r := range dedupePaths(resources) {
		name := strings.TrimPrefix(r.ZipPath, "/")
		if name == "" {
			continue
		}
		f, err := zw.Create(name)
		if err != nil {
			continue
		}
		if _, err := f.Write(r.Bytes()); err != nil {
			continue
		}
		written++
	}
	if err := zw.Close(); err != nil {
		return written, err
	}
	return written, nil
}

func (h *HAR) ExportAll(w io.Writer) (int, error) {
	return WriteZip(w, h.Resources(false))
}

func WriteDir(dir string, resources []Resource, mkdirAll func(string) error, writeFile func(string, []byte) error) (int, error) {
	written := 0
	for _, r := range dedupePaths(resources) {
		rel := strings.TrimPrefix(r.ZipPath, "/")
		if rel == "" {
			continue
		}
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := mkdirAll(filepath.Dir(full)); err != nil {
			continue
		}
		if err := writeFile(full, r.Bytes()); err != nil {
			continue
		}
		written++
	}
	return written, nil
}

func WriteDirOS(dir string, resources []Resource) (int, error) {
	return WriteDir(dir, resources,
		func(p string) error { return os.MkdirAll(p, 0o755) },
		func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) },
	)
}

func sanitize(s string) string {
	repl := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"?", "_",
		"*", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	out := repl.Replace(s)
	if len(out) > 100 {
		out = out[:100]
	}
	return out
}

func sanitizeSegment(s string) string {
	repl := strings.NewReplacer(
		"\\", "_",
		":", "_",
		"?", "_",
		"*", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	out := repl.Replace(s)
	switch out {
	case "", ".", "..":
		out = "_"
	}
	if len(out) > 150 {
		out = out[:150]
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func sortResourcesByPath(res []Resource) []Resource {
	out := make([]Resource, len(res))
	copy(out, res)
	sort.SliceStable(out, func(i, j int) bool { return out[i].ZipPath < out[j].ZipPath })
	return out
}
