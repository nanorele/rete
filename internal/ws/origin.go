package ws

import (
	"net"
	"net/url"
	"strings"
)

func DefaultOrigin(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := "https"
	if s := strings.ToLower(u.Scheme); s == "ws" || s == "http" {
		scheme = "http"
	}
	host := u.Host
	if h, p, err := net.SplitHostPort(host); err == nil {
		if (scheme == "https" && p == "443") || (scheme == "http" && p == "80") {
			host = h
		}
	}
	return scheme + "://" + host
}
