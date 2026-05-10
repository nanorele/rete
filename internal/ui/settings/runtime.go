package settings

import (
	"net/http"

	"tracto/internal/model"

	"github.com/nanorele/gio/unit"
)

var BodyTextSize = unit.Sp(13)

var (
	UserAgent             = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	DefaultHeaders        []model.DefaultHeader
	JSONIndent            = 2
	PreviewMaxMB          = 100
	RespBodyPad           = unit.Dp(4)
	DefaultMethod         = "GET"
	DefaultSplitRatio     = float32(0.5)
	AutoFormatJSON        = true
	AutoFormatJSONRequest = false
	StripJSONComments     = true
	TrimTrailingWS        = false
	SendConnClose         = false
	AcceptEncoding        = "gzip"
	BracketColorization   = true
	StackBreakpointDp     = 700
)

var HTTPClient *http.Client = buildHTTPClient(model.DefaultSettings())
