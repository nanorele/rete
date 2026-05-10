package settings

import (
	"tracto/internal/model"
	"tracto/internal/ui/theme"
)

var Methods = []string{"GET", "POST", "PUT", "DELETE", "HEAD", "PATCH", "OPTIONS"}

func Sanitize(s model.AppSettings) model.AppSettings {
	if !theme.IsValidID(s.Theme, s.CustomThemes) {
		s.Theme = "dark"
	}
	if s.UITextSize < 10 {
		s.UITextSize = 14
	}
	if s.UITextSize > 28 {
		s.UITextSize = 28
	}
	if s.BodyTextSize < 10 {
		s.BodyTextSize = 13
	}
	if s.BodyTextSize > 28 {
		s.BodyTextSize = 28
	}
	if s.UIScale <= 0 {
		s.UIScale = 1.0
	}
	if s.UIScale < 0.75 {
		s.UIScale = 0.75
	}
	if s.UIScale > 2.0 {
		s.UIScale = 2.0
	}

	if s.RequestTimeoutSec < 0 {
		s.RequestTimeoutSec = 30
	}
	if s.RequestTimeoutSec > 3600 {
		s.RequestTimeoutSec = 3600
	}
	if s.ConnectTimeoutSec < 0 {
		s.ConnectTimeoutSec = 0
	}
	if s.ConnectTimeoutSec > 600 {
		s.ConnectTimeoutSec = 600
	}
	if s.TLSHandshakeTimeoutSec < 0 {
		s.TLSHandshakeTimeoutSec = 0
	}
	if s.TLSHandshakeTimeoutSec > 600 {
		s.TLSHandshakeTimeoutSec = 600
	}
	if s.IdleConnTimeoutSec < 0 {
		s.IdleConnTimeoutSec = 0
	}
	if s.IdleConnTimeoutSec > 3600 {
		s.IdleConnTimeoutSec = 3600
	}
	switch s.DefaultAcceptEncoding {
	case "", "identity", "gzip", "deflate", "br", "gzip, deflate", "gzip, deflate, br":
	default:
		s.DefaultAcceptEncoding = "gzip"
	}
	if s.UserAgent == "" {
		s.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	if s.MaxRedirects < 0 {
		s.MaxRedirects = 0
	}
	if s.MaxRedirects > 50 {
		s.MaxRedirects = 50
	}

	if s.JSONIndentSpaces < 0 {
		s.JSONIndentSpaces = 2
	}
	if s.JSONIndentSpaces > 8 {
		s.JSONIndentSpaces = 8
	}
	if s.PreviewMaxMB < 1 {
		s.PreviewMaxMB = 100
	}
	if s.PreviewMaxMB > 500 {
		s.PreviewMaxMB = 500
	}
	if s.ResponseBodyPadding < 0 {
		s.ResponseBodyPadding = 0
	}
	if s.ResponseBodyPadding > 32 {
		s.ResponseBodyPadding = 32
	}

	validMethod := false
	for _, m := range Methods {
		if s.DefaultMethod == m {
			validMethod = true
			break
		}
	}
	if !validMethod {
		s.DefaultMethod = "GET"
	}
	if s.DefaultSplitRatio < 0.2 {
		s.DefaultSplitRatio = 0.5
	}
	if s.DefaultSplitRatio > 0.8 {
		s.DefaultSplitRatio = 0.8
	}
	if s.MaxConnsPerHost < 0 {
		s.MaxConnsPerHost = 0
	}
	if s.MaxConnsPerHost > 10000 {
		s.MaxConnsPerHost = 10000
	}

	if s.StackBreakpointDp < 0 {
		s.StackBreakpointDp = 0
	}
	if s.StackBreakpointDp > 0 && s.StackBreakpointDp < 400 {
		s.StackBreakpointDp = 400
	}
	if s.StackBreakpointDp > 2000 {
		s.StackBreakpointDp = 2000
	}

	if s.DefaultSidebarWidthPx < 0 {
		s.DefaultSidebarWidthPx = 0
	}
	if s.DefaultSidebarWidthPx > 0 && s.DefaultSidebarWidthPx < 160 {
		s.DefaultSidebarWidthPx = 160
	}
	if s.DefaultSidebarWidthPx > 1000 {
		s.DefaultSidebarWidthPx = 1000
	}

	return s
}
