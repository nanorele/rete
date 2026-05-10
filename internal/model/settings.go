package model

type DefaultHeader struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type AppSettings struct {
	Theme        string  `json:"theme"`
	UITextSize   int     `json:"ui_text_size"`
	BodyTextSize int     `json:"body_text_size"`
	HideTabBar   bool    `json:"hide_tab_bar"`
	HideSidebar  bool    `json:"hide_sidebar"`
	UIScale      float32 `json:"ui_scale"`

	RequestTimeoutSec      int             `json:"request_timeout_sec"`
	ConnectTimeoutSec      int             `json:"connect_timeout_sec"`
	TLSHandshakeTimeoutSec int             `json:"tls_handshake_timeout_sec"`
	IdleConnTimeoutSec     int             `json:"idle_conn_timeout_sec"`
	UserAgent              string          `json:"user_agent"`
	DefaultMethod          string          `json:"default_method"`
	FollowRedirects        bool            `json:"follow_redirects"`
	MaxRedirects           int             `json:"max_redirects"`
	VerifySSL              bool            `json:"verify_ssl"`
	KeepAlive              bool            `json:"keep_alive"`
	DisableHTTP2           bool            `json:"disable_http2"`
	CookieJarEnabled       bool            `json:"cookie_jar_enabled"`
	SendConnectionClose    bool            `json:"send_connection_close"`
	DefaultAcceptEncoding  string          `json:"default_accept_encoding"`
	MaxConnsPerHost        int             `json:"max_conns_per_host"`
	Proxy                  string          `json:"proxy"`
	DefaultHeaders         []DefaultHeader `json:"default_headers"`

	JSONIndentSpaces        int     `json:"json_indent_spaces"`
	WrapLinesDefault        bool    `json:"wrap_lines_default"`
	PreviewMaxMB            int     `json:"preview_max_mb"`
	ResponseBodyPadding     int     `json:"response_body_padding"`
	DefaultSplitRatio       float32 `json:"default_split_ratio"`
	AutoFormatJSON          bool    `json:"auto_format_json"`
	AutoFormatJSONRequest   bool    `json:"auto_format_json_request"`
	StripJSONComments       bool    `json:"strip_json_comments"`
	TrimTrailingWhitespace  bool    `json:"trim_trailing_whitespace"`
	BracketPairColorization bool    `json:"bracket_pair_colorization"`
	StackBreakpointDp       int     `json:"stack_breakpoint_dp,omitempty"`
	DefaultSidebarWidthPx   int     `json:"default_sidebar_width_px,omitempty"`
	RestoreTabsOnStartup    bool    `json:"restore_tabs_on_startup"`

	SyntaxOverrides map[string]ThemeSyntaxOverride `json:"syntax_overrides,omitempty"`

	ThemeOverrides map[string]ThemeColorOverride `json:"theme_overrides,omitempty"`

	CustomThemes []CustomTheme `json:"custom_themes,omitempty"`
}

type CustomTheme struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	BasedOn string              `json:"based_on,omitempty"`
	Palette ThemeColorOverride  `json:"palette"`
	Syntax  ThemeSyntaxOverride `json:"syntax,omitempty"`
}

type ThemeColorOverride struct {
	Bg           string `json:"bg,omitempty"`
	BgDark       string `json:"bg_dark,omitempty"`
	BgField      string `json:"bg_field,omitempty"`
	BgMenu       string `json:"bg_menu,omitempty"`
	BgPopup      string `json:"bg_popup,omitempty"`
	BgHover      string `json:"bg_hover,omitempty"`
	BgSecondary  string `json:"bg_secondary,omitempty"`
	BgLoadMore   string `json:"bg_load_more,omitempty"`
	BgDragHolder string `json:"bg_drag_holder,omitempty"`
	BgDragGhost  string `json:"bg_drag_ghost,omitempty"`
	Border       string `json:"border,omitempty"`
	BorderLight  string `json:"border_light,omitempty"`
	Fg           string `json:"fg,omitempty"`
	FgMuted      string `json:"fg_muted,omitempty"`
	FgDim        string `json:"fg_dim,omitempty"`
	FgHint       string `json:"fg_hint,omitempty"`
	FgDisabled   string `json:"fg_disabled,omitempty"`
	White        string `json:"white,omitempty"`
	Accent       string `json:"accent,omitempty"`
	AccentHover  string `json:"accent_hover,omitempty"`
	AccentDim    string `json:"accent_dim,omitempty"`
	AccentFg     string `json:"accent_fg,omitempty"`
	Danger       string `json:"danger,omitempty"`
	DangerFg     string `json:"danger_fg,omitempty"`
	Cancel       string `json:"cancel,omitempty"`
	CloseHover   string `json:"close_hover,omitempty"`
	ScrollThumb  string `json:"scroll_thumb,omitempty"`
	VarFound     string `json:"var_found,omitempty"`
	VarMissing   string `json:"var_missing,omitempty"`
	DividerLight string `json:"divider_light,omitempty"`
}

type ThemeSyntaxOverride struct {
	Plain       string `json:"plain,omitempty"`
	String      string `json:"string,omitempty"`
	Number      string `json:"number,omitempty"`
	Bool        string `json:"bool,omitempty"`
	Null        string `json:"null,omitempty"`
	Key         string `json:"key,omitempty"`
	Punctuation string `json:"punctuation,omitempty"`
	Operator    string `json:"operator,omitempty"`
	Keyword     string `json:"keyword,omitempty"`
	Type        string `json:"type,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Bracket0    string `json:"bracket0,omitempty"`
	Bracket1    string `json:"bracket1,omitempty"`
	Bracket2    string `json:"bracket2,omitempty"`
}

func DefaultSettings() AppSettings {
	return AppSettings{
		Theme:        "dark",
		UITextSize:   14,
		BodyTextSize: 13,
		HideTabBar:   false,
		HideSidebar:  false,
		UIScale:      1.0,

		RequestTimeoutSec:      30,
		ConnectTimeoutSec:      10,
		TLSHandshakeTimeoutSec: 10,
		IdleConnTimeoutSec:     90,
		UserAgent:              "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
		DefaultMethod:          "GET",
		FollowRedirects:        true,
		MaxRedirects:           10,
		VerifySSL:              true,
		KeepAlive:              true,
		DisableHTTP2:           false,
		CookieJarEnabled:       false,
		SendConnectionClose:    false,
		DefaultAcceptEncoding:  "gzip",
		MaxConnsPerHost:        0,
		Proxy:                  "",
		DefaultHeaders:         nil,

		JSONIndentSpaces:        2,
		WrapLinesDefault:        false,
		PreviewMaxMB:            100,
		ResponseBodyPadding:     4,
		DefaultSplitRatio:       0.5,
		AutoFormatJSON:          true,
		AutoFormatJSONRequest:   false,
		StripJSONComments:       true,
		TrimTrailingWhitespace:  false,
		BracketPairColorization: true,
		StackBreakpointDp:       700,
		DefaultSidebarWidthPx:   250,
		RestoreTabsOnStartup:    true,
	}
}
