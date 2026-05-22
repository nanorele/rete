package utils

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

// ParseContentDispositionFilename extracts a filename from a
// Content-Disposition header. Go's mime.ParseMediaType already decodes
// RFC 5987 `filename*=charset''percent-encoded` into the regular
// `filename` parameter, so we just consume that. The returned string
// has any path separators stripped so a malicious server cannot direct
// our save into another directory.
func ParseContentDispositionFilename(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err != nil {
		return ""
	}
	name := params["filename"]
	if name == "" {
		return ""
	}
	return sanitizeFilename(name)
}

// FilenameFromURL derives a sensible default filename from the last
// segment of a URL path. Returns "" when the URL has no path component
// (e.g. https://example.com/).
func FilenameFromURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	name := path.Base(u.Path)
	if name == "" || name == "/" || name == "." {
		return ""
	}
	return sanitizeFilename(name)
}

func sanitizeFilename(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSpace(s)
	if s == "" || s == "." || s == ".." {
		return ""
	}
	return s
}
