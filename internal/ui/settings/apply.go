package settings

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"tracto/internal/model"
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
)

func Apply(th *material.Theme, s model.AppSettings) {
	p := theme.PaletteFor(s.Theme, s.CustomThemes)
	if ov, ok := s.ThemeOverrides[s.Theme]; ok {
		p = theme.ApplyOverride(p, ov)
	}
	if ov, ok := s.SyntaxOverrides[s.Theme]; ok {
		p.Syntax = theme.ApplySyntaxOverride(p.Syntax, ov)
	}
	theme.Apply(p)
	BodyTextSize = unit.Sp(float32(s.BodyTextSize))
	UserAgent = s.UserAgent
	if UserAgent == "" {
		UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	DefaultHeaders = append(DefaultHeaders[:0], s.DefaultHeaders...)
	JSONIndent = s.JSONIndentSpaces
	if JSONIndent < 0 {
		JSONIndent = 2
	}
	PreviewMaxMB = s.PreviewMaxMB
	if PreviewMaxMB < 1 {
		PreviewMaxMB = 100
	}
	RespBodyPad = unit.Dp(s.ResponseBodyPadding)
	DefaultMethod = s.DefaultMethod
	if DefaultMethod == "" {
		DefaultMethod = "GET"
	}
	DefaultSplitRatio = s.DefaultSplitRatio
	if DefaultSplitRatio < 0.2 || DefaultSplitRatio > 0.8 {
		DefaultSplitRatio = 0.5
	}
	AutoFormatJSON = s.AutoFormatJSON
	AutoFormatJSONRequest = s.AutoFormatJSONRequest
	StripJSONComments = s.StripJSONComments
	TrimTrailingWS = s.TrimTrailingWhitespace
	SendConnClose = s.SendConnectionClose
	AcceptEncoding = s.DefaultAcceptEncoding
	BracketColorization = s.BracketPairColorization
	StackBreakpointDp = s.StackBreakpointDp
	HTTPClient = buildHTTPClient(s)
	if th != nil {
		th.Bg = theme.Bg
		th.Fg = theme.Fg
		th.ContrastBg = theme.Accent
		th.ContrastFg = theme.AccentFg
		th.TextSize = unit.Sp(float32(s.UITextSize))
	}
}

func buildHTTPClient(s model.AppSettings) *http.Client {
	base, _ := http.DefaultTransport.(*http.Transport)
	var tr *http.Transport
	if base != nil {
		tr = base.Clone()
	} else {
		tr = &http.Transport{}
	}
	if !s.VerifySSL {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	} else {
		tr.TLSClientConfig = nil
	}
	tr.DisableKeepAlives = !s.KeepAlive
	if s.MaxConnsPerHost > 0 {
		tr.MaxConnsPerHost = s.MaxConnsPerHost
	} else {
		tr.MaxConnsPerHost = 0
	}
	if s.DisableHTTP2 {
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	} else {
		tr.ForceAttemptHTTP2 = true
		tr.TLSNextProto = nil
	}
	if s.ConnectTimeoutSec > 0 {
		dialer := &net.Dialer{
			Timeout:   time.Duration(s.ConnectTimeoutSec) * time.Second,
			KeepAlive: 30 * time.Second,
		}
		tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		}
	}
	if s.TLSHandshakeTimeoutSec > 0 {
		tr.TLSHandshakeTimeout = time.Duration(s.TLSHandshakeTimeoutSec) * time.Second
	}
	if s.IdleConnTimeoutSec > 0 {
		tr.IdleConnTimeout = time.Duration(s.IdleConnTimeoutSec) * time.Second
	}
	if strings.TrimSpace(s.Proxy) != "" {
		if u, err := url.Parse(strings.TrimSpace(s.Proxy)); err == nil && u.Host != "" {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	c := &http.Client{Transport: tr}
	if s.RequestTimeoutSec > 0 {
		c.Timeout = time.Duration(s.RequestTimeoutSec) * time.Second
	}
	if s.CookieJarEnabled {
		if jar, err := cookiejar.New(nil); err == nil {
			c.Jar = jar
		}
	}
	switch {
	case !s.FollowRedirects:
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	case s.MaxRedirects > 0:
		maxR := s.MaxRedirects
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxR {
				return fmt.Errorf("stopped after %d redirects", maxR)
			}
			return nil
		}
	}
	return c
}
