package utils

import (
	"mime"
	"net/url"
	"path"
	"strings"
)

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
