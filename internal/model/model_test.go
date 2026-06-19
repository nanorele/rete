package model

import (
	"encoding/json"
	"testing"
)

func TestBodyType_String(t *testing.T) {
	cases := []struct {
		in   BodyType
		want string
	}{
		{BodyNone, "none"},
		{BodyRaw, "raw"},
		{BodyFormData, "form-data"},
		{BodyURLEncoded, "x-www-form-urlencoded"},
		{BodyBinary, "binary"},
		{BodyType(99), "raw"},
	}
	for _, c := range cases {
		if got := c.in.String(); got != c.want {
			t.Errorf("BodyType(%d).String() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBodyType_PostmanMode(t *testing.T) {
	cases := []struct {
		in   BodyType
		want string
	}{
		{BodyNone, "none"},
		{BodyRaw, "raw"},
		{BodyFormData, "formdata"},
		{BodyURLEncoded, "urlencoded"},
		{BodyBinary, "file"},
		{BodyType(99), "raw"},
	}
	for _, c := range cases {
		if got := c.in.PostmanMode(); got != c.want {
			t.Errorf("BodyType(%d).PostmanMode() = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBodyTypeFromMode(t *testing.T) {
	cases := []struct {
		in   string
		want BodyType
	}{
		{"", BodyNone},
		{"none", BodyNone},
		{"formdata", BodyFormData},
		{"form-data", BodyFormData},
		{"urlencoded", BodyURLEncoded},
		{"x-www-form-urlencoded", BodyURLEncoded},
		{"file", BodyBinary},
		{"binary", BodyBinary},
		{"raw", BodyRaw},
		{"something-unknown", BodyRaw},
	}
	for _, c := range cases {
		if got := BodyTypeFromMode(c.in); got != c.want {
			t.Errorf("BodyTypeFromMode(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestBodyType_PostmanRoundTrip(t *testing.T) {
	values := []BodyType{BodyNone, BodyRaw, BodyFormData, BodyURLEncoded, BodyBinary}
	for _, v := range values {
		if got := BodyTypeFromMode(v.PostmanMode()); got != v {
			t.Errorf("round-trip via PostmanMode for %d: got %d", v, got)
		}
	}
}

func TestBodyType_StringRoundTrip(t *testing.T) {
	values := []BodyType{BodyNone, BodyRaw, BodyFormData, BodyURLEncoded, BodyBinary}
	for _, v := range values {
		if got := BodyTypeFromMode(v.String()); got != v {
			t.Errorf("round-trip via String for %d: got %d", v, got)
		}
	}
}

func TestBodyType_EnumOrder(t *testing.T) {
	if BodyNone != 0 || BodyRaw != 1 || BodyFormData != 2 || BodyURLEncoded != 3 || BodyBinary != 4 {
		t.Fatalf("BodyType enum order changed: None=%d Raw=%d FormData=%d URLEncoded=%d Binary=%d",
			BodyNone, BodyRaw, BodyFormData, BodyURLEncoded, BodyBinary)
	}
}

func TestFormPartKind_EnumOrder(t *testing.T) {
	if FormPartText != 0 || FormPartFile != 1 {
		t.Fatalf("FormPartKind enum order changed: Text=%d File=%d", FormPartText, FormPartFile)
	}
}

func TestDefaultSettings_Values(t *testing.T) {
	s := DefaultSettings()
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"Theme", s.Theme, "dark"},
		{"UITextSize", s.UITextSize, 14},
		{"BodyTextSize", s.BodyTextSize, 13},
		{"HideTabBar", s.HideTabBar, false},
		{"HideSidebar", s.HideSidebar, false},
		{"UIScale", s.UIScale, float32(1.0)},
		{"RequestTimeoutSec", s.RequestTimeoutSec, 0},
		{"ConnectTimeoutSec", s.ConnectTimeoutSec, 0},
		{"TLSHandshakeTimeoutSec", s.TLSHandshakeTimeoutSec, 0},
		{"IdleConnTimeoutSec", s.IdleConnTimeoutSec, 0},
		{"DefaultMethod", s.DefaultMethod, "GET"},
		{"FollowRedirects", s.FollowRedirects, true},
		{"MaxRedirects", s.MaxRedirects, 10},
		{"VerifySSL", s.VerifySSL, true},
		{"KeepAlive", s.KeepAlive, true},
		{"DisableHTTP2", s.DisableHTTP2, false},
		{"CookieJarEnabled", s.CookieJarEnabled, false},
		{"SendConnectionClose", s.SendConnectionClose, false},
		{"DefaultAcceptEncoding", s.DefaultAcceptEncoding, "gzip"},
		{"MaxConnsPerHost", s.MaxConnsPerHost, 0},
		{"Proxy", s.Proxy, ""},
		{"JSONIndentSpaces", s.JSONIndentSpaces, 2},
		{"WrapLinesDefault", s.WrapLinesDefault, false},
		{"PreviewMaxMB", s.PreviewMaxMB, 100},
		{"SyntaxHighlightMaxMB", s.SyntaxHighlightMaxMB, 100},
		{"ResponseBodyPadding", s.ResponseBodyPadding, 4},
		{"DefaultSplitRatio", s.DefaultSplitRatio, float32(0.5)},
		{"AutoFormatJSON", s.AutoFormatJSON, true},
		{"AutoFormatJSONRequest", s.AutoFormatJSONRequest, false},
		{"StripJSONComments", s.StripJSONComments, true},
		{"TrimTrailingWhitespace", s.TrimTrailingWhitespace, false},
		{"BracketPairColorization", s.BracketPairColorization, true},
		{"StackBreakpointDp", s.StackBreakpointDp, 700},
		{"DefaultSidebarWidthPx", s.DefaultSidebarWidthPx, 250},
		{"RestoreTabsOnStartup", s.RestoreTabsOnStartup, true},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	if s.UserAgent == "" {
		t.Error("UserAgent must not be empty")
	}
	if s.DefaultHeaders != nil {
		t.Errorf("DefaultHeaders = %v, want nil", s.DefaultHeaders)
	}
	if s.SyntaxOverrides != nil || s.ThemeOverrides != nil || s.CustomThemes != nil {
		t.Error("override maps and CustomThemes must be nil by default")
	}
}

func TestAppSettings_JSONRoundTrip(t *testing.T) {
	in := DefaultSettings()
	in.DefaultHeaders = []DefaultHeader{{Key: "X-Test", Value: "1"}}
	in.SyntaxOverrides = map[string]ThemeSyntaxOverride{
		"dark": {String: "#abcdef", Bracket0: "#fff"},
	}
	in.ThemeOverrides = map[string]ThemeColorOverride{
		"dark": {Bg: "#101010", Accent: "#22aaff"},
	}
	in.CustomThemes = []CustomTheme{{
		ID: "my", Name: "My", BasedOn: "dark",
		Palette: ThemeColorOverride{Bg: "#000"},
		Syntax:  ThemeSyntaxOverride{Key: "#0f0"},
	}}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out AppSettings
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Theme != in.Theme || out.UITextSize != in.UITextSize {
		t.Errorf("basic fields differ after round-trip")
	}
	if len(out.DefaultHeaders) != 1 || out.DefaultHeaders[0].Key != "X-Test" {
		t.Errorf("DefaultHeaders lost: %+v", out.DefaultHeaders)
	}
	if out.SyntaxOverrides["dark"].String != "#abcdef" {
		t.Errorf("SyntaxOverrides lost: %+v", out.SyntaxOverrides)
	}
	if out.ThemeOverrides["dark"].Accent != "#22aaff" {
		t.Errorf("ThemeOverrides lost: %+v", out.ThemeOverrides)
	}
	if len(out.CustomThemes) != 1 || out.CustomThemes[0].ID != "my" {
		t.Errorf("CustomThemes lost: %+v", out.CustomThemes)
	}
}

func TestThemeOverrides_Omitempty(t *testing.T) {
	empty := ThemeColorOverride{}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("empty ThemeColorOverride marshalled to %s, want {}", data)
	}
	emptySyntax := ThemeSyntaxOverride{}
	data, err = json.Marshal(emptySyntax)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("empty ThemeSyntaxOverride marshalled to %s, want {}", data)
	}
}

func TestExtBody_OmitemptyMode(t *testing.T) {
	b := ExtBody{Mode: "raw", Raw: "hello"}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundtrip ExtBody
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if roundtrip.Mode != "raw" || roundtrip.Raw != "hello" {
		t.Errorf("round-trip lost fields: %+v", roundtrip)
	}
}

func TestExtCollection_Unmarshal(t *testing.T) {
	src := `{
		"info": {"name": "My Coll"},
		"item": [
			{"name": "req1", "request": {"method": "GET", "url": "https://example.com"}},
			{"name": "folder", "item": [
				{"name": "nested", "request": {"method": "POST"}}
			]}
		]
	}`
	var c ExtCollection
	if err := json.Unmarshal([]byte(src), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Info.Name != "My Coll" {
		t.Errorf("Info.Name = %q", c.Info.Name)
	}
	if len(c.Item) != 2 {
		t.Fatalf("len Item = %d", len(c.Item))
	}
	if c.Item[0].Name != "req1" || len(c.Item[0].Request) == 0 {
		t.Errorf("first item wrong: %+v", c.Item[0])
	}
	if len(c.Item[1].Item) != 1 || c.Item[1].Item[0].Name != "nested" {
		t.Errorf("nested item missing: %+v", c.Item[1])
	}
}

func TestExtEnvironment_Unmarshal(t *testing.T) {
	src := `{
		"name": "Dev",
		"highlight_color": "#ff8800",
		"values": [
			{"key": "host", "value": "localhost"},
			{"key": "port", "value": "8080", "enabled": true},
			{"key": "off", "value": "x", "enabled": false}
		]
	}`
	var e ExtEnvironment
	if err := json.Unmarshal([]byte(src), &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Name != "Dev" || e.HighlightColor != "#ff8800" {
		t.Errorf("header fields wrong: %+v", e)
	}
	if len(e.Values) != 3 {
		t.Fatalf("len Values = %d", len(e.Values))
	}
	if e.Values[0].Key != "host" || e.Values[0].Value != "localhost" {
		t.Errorf("values[0] = %+v", e.Values[0])
	}
	if e.Values[2].Key != "off" || e.Values[2].Value != "x" {
		t.Errorf("values[2] = %+v", e.Values[2])
	}
}

func TestEnvVar_JSONRoundTrip(t *testing.T) {
	v := EnvVar{Key: "k", Value: "v"}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out EnvVar
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != v {
		t.Errorf("round-trip mismatch: %+v vs %+v", out, v)
	}
}

func TestParsedRequest_ZeroValue(t *testing.T) {
	var r ParsedRequest
	if r.Headers != nil || r.FormParts != nil || r.URLEncoded != nil {
		t.Error("ParsedRequest zero value should have nil slices/maps")
	}
	if r.BodyType != BodyNone {
		t.Errorf("zero BodyType should be BodyNone, got %d", r.BodyType)
	}
}

func TestDefaultHeader_JSONRoundTrip(t *testing.T) {
	h := DefaultHeader{Key: "X-A", Value: "B"}
	data, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(data) != `{"key":"X-A","value":"B"}` {
		t.Errorf("unexpected marshal: %s", data)
	}
	var out DefaultHeader
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out != h {
		t.Errorf("round-trip mismatch")
	}
}
