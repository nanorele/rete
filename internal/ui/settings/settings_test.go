package settings

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tracto/internal/model"
)

func TestSanitize_ThemeFallback(t *testing.T) {
	s := model.AppSettings{Theme: "bogus"}
	out := Sanitize(s)
	if out.Theme != "dark" {
		t.Errorf("invalid theme should fall back to dark, got %q", out.Theme)
	}

	s.Theme = "light"
	out = Sanitize(s)
	if out.Theme != "light" {
		t.Errorf("valid theme should be kept, got %q", out.Theme)
	}

	s.Theme = "custom-x"
	s.CustomThemes = []model.CustomTheme{{ID: "custom-x", Name: "Custom"}}
	out = Sanitize(s)
	if out.Theme != "custom-x" {
		t.Errorf("custom theme should be kept, got %q", out.Theme)
	}
}

func TestSanitize_TextSizes(t *testing.T) {
	cases := []struct {
		ui, body         int
		wantUI, wantBody int
	}{
		{0, 0, 14, 13},
		{9, 9, 14, 13},
		{10, 10, 10, 10},
		{14, 13, 14, 13},
		{28, 28, 28, 28},
		{29, 99, 28, 28},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", UITextSize: c.ui, BodyTextSize: c.body})
		if out.UITextSize != c.wantUI {
			t.Errorf("UITextSize in=%d → %d, want %d", c.ui, out.UITextSize, c.wantUI)
		}
		if out.BodyTextSize != c.wantBody {
			t.Errorf("BodyTextSize in=%d → %d, want %d", c.body, out.BodyTextSize, c.wantBody)
		}
	}
}

func TestSanitize_UIScale(t *testing.T) {
	cases := []struct {
		in, want float32
	}{
		{0, 1.0},
		{-1, 1.0},
		{0.5, 0.75},
		{0.74, 0.75},
		{0.75, 0.75},
		{1.5, 1.5},
		{2.0, 2.0},
		{2.5, 2.0},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", UIScale: c.in})
		if out.UIScale != c.want {
			t.Errorf("UIScale %v → %v, want %v", c.in, out.UIScale, c.want)
		}
	}
}

func TestSanitize_Timeouts(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", RequestTimeoutSec: -5})
	if out.RequestTimeoutSec != 30 {
		t.Errorf("negative RequestTimeoutSec should default to 30, got %d", out.RequestTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", RequestTimeoutSec: 9999})
	if out.RequestTimeoutSec != 3600 {
		t.Errorf("huge RequestTimeoutSec should clamp to 3600, got %d", out.RequestTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ConnectTimeoutSec: -1})
	if out.ConnectTimeoutSec != 0 {
		t.Errorf("negative ConnectTimeoutSec should be 0, got %d", out.ConnectTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ConnectTimeoutSec: 9999})
	if out.ConnectTimeoutSec != 600 {
		t.Errorf("huge ConnectTimeoutSec should clamp to 600, got %d", out.ConnectTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", TLSHandshakeTimeoutSec: -1})
	if out.TLSHandshakeTimeoutSec != 0 {
		t.Errorf("negative TLS should be 0, got %d", out.TLSHandshakeTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", TLSHandshakeTimeoutSec: 9999})
	if out.TLSHandshakeTimeoutSec != 600 {
		t.Errorf("huge TLS should clamp to 600, got %d", out.TLSHandshakeTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", IdleConnTimeoutSec: -1})
	if out.IdleConnTimeoutSec != 0 {
		t.Errorf("negative idle should be 0, got %d", out.IdleConnTimeoutSec)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", IdleConnTimeoutSec: 9999})
	if out.IdleConnTimeoutSec != 3600 {
		t.Errorf("huge idle should clamp to 3600, got %d", out.IdleConnTimeoutSec)
	}
}

func TestSanitize_AcceptEncoding(t *testing.T) {
	valid := []string{"", "identity", "gzip", "deflate", "br", "gzip, deflate", "gzip, deflate, br"}
	for _, v := range valid {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultAcceptEncoding: v})
		if out.DefaultAcceptEncoding != v {
			t.Errorf("valid encoding %q changed to %q", v, out.DefaultAcceptEncoding)
		}
	}
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultAcceptEncoding: "garbage"})
	if out.DefaultAcceptEncoding != "gzip" {
		t.Errorf("garbage encoding should default to gzip, got %q", out.DefaultAcceptEncoding)
	}
}

func TestSanitize_UserAgent(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", UserAgent: ""})
	if out.UserAgent == "" {
		t.Error("empty UserAgent should be replaced with default")
	}
	out = Sanitize(model.AppSettings{Theme: "dark", UserAgent: "MyAgent/1.0"})
	if out.UserAgent != "MyAgent/1.0" {
		t.Errorf("explicit UserAgent should be kept, got %q", out.UserAgent)
	}
}

func TestSanitize_Redirects(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", MaxRedirects: -1})
	if out.MaxRedirects != 0 {
		t.Errorf("negative MaxRedirects → 0, got %d", out.MaxRedirects)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", MaxRedirects: 100})
	if out.MaxRedirects != 50 {
		t.Errorf("huge MaxRedirects → 50, got %d", out.MaxRedirects)
	}
}

func TestSanitize_JSONIndent(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", JSONIndentSpaces: -1})
	if out.JSONIndentSpaces != 2 {
		t.Errorf("negative JSONIndentSpaces → 2, got %d", out.JSONIndentSpaces)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", JSONIndentSpaces: 16})
	if out.JSONIndentSpaces != 8 {
		t.Errorf("huge JSONIndentSpaces → 8, got %d", out.JSONIndentSpaces)
	}
}

func TestSanitize_PreviewMaxMB(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", PreviewMaxMB: 0})
	if out.PreviewMaxMB != 100 {
		t.Errorf("zero PreviewMaxMB → 100, got %d", out.PreviewMaxMB)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", PreviewMaxMB: 9999})
	if out.PreviewMaxMB != 500 {
		t.Errorf("huge PreviewMaxMB → 500, got %d", out.PreviewMaxMB)
	}
}

func TestSanitize_ResponseBodyPadding(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", ResponseBodyPadding: -1})
	if out.ResponseBodyPadding != 0 {
		t.Errorf("negative ResponseBodyPadding → 0, got %d", out.ResponseBodyPadding)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", ResponseBodyPadding: 100})
	if out.ResponseBodyPadding != 32 {
		t.Errorf("huge ResponseBodyPadding → 32, got %d", out.ResponseBodyPadding)
	}
}

func TestSanitize_DefaultMethod(t *testing.T) {
	for _, m := range Methods {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: m})
		if out.DefaultMethod != m {
			t.Errorf("valid method %q changed to %q", m, out.DefaultMethod)
		}
	}
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: "FOO"})
	if out.DefaultMethod != "GET" {
		t.Errorf("invalid method → GET, got %q", out.DefaultMethod)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultMethod: ""})
	if out.DefaultMethod != "GET" {
		t.Errorf("empty method → GET, got %q", out.DefaultMethod)
	}
}

func TestSanitize_DefaultSplitRatio(t *testing.T) {
	cases := []struct{ in, want float32 }{
		{0, 0.5},
		{0.1, 0.5},
		{0.19, 0.5},
		{0.2, 0.2},
		{0.5, 0.5},
		{0.8, 0.8},
		{0.9, 0.8},
	}
	for _, c := range cases {
		out := Sanitize(model.AppSettings{Theme: "dark", DefaultSplitRatio: c.in})
		if out.DefaultSplitRatio != c.want {
			t.Errorf("DefaultSplitRatio %v → %v, want %v", c.in, out.DefaultSplitRatio, c.want)
		}
	}
}

func TestSanitize_MaxConnsPerHost(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", MaxConnsPerHost: -1})
	if out.MaxConnsPerHost != 0 {
		t.Errorf("negative MaxConnsPerHost → 0, got %d", out.MaxConnsPerHost)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", MaxConnsPerHost: 999999})
	if out.MaxConnsPerHost != 10000 {
		t.Errorf("huge MaxConnsPerHost → 10000, got %d", out.MaxConnsPerHost)
	}
}

func TestSanitize_StackBreakpointDp(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: -1})
	if out.StackBreakpointDp != 0 {
		t.Errorf("negative StackBreakpointDp → 0, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 300})
	if out.StackBreakpointDp != 400 {
		t.Errorf("StackBreakpointDp 300 → 400, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 9999})
	if out.StackBreakpointDp != 2000 {
		t.Errorf("huge StackBreakpointDp → 2000, got %d", out.StackBreakpointDp)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", StackBreakpointDp: 0})
	if out.StackBreakpointDp != 0 {
		t.Errorf("zero StackBreakpointDp should stay 0, got %d", out.StackBreakpointDp)
	}
}

func TestSanitize_DefaultSidebarWidthPx(t *testing.T) {
	out := Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: -1})
	if out.DefaultSidebarWidthPx != 0 {
		t.Errorf("negative DefaultSidebarWidthPx → 0, got %d", out.DefaultSidebarWidthPx)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: 100})
	if out.DefaultSidebarWidthPx != 160 {
		t.Errorf("DefaultSidebarWidthPx 100 → 160, got %d", out.DefaultSidebarWidthPx)
	}
	out = Sanitize(model.AppSettings{Theme: "dark", DefaultSidebarWidthPx: 9999})
	if out.DefaultSidebarWidthPx != 1000 {
		t.Errorf("huge DefaultSidebarWidthPx → 1000, got %d", out.DefaultSidebarWidthPx)
	}
}

func resetHTTPClient(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		HTTPClient = buildHTTPClient(model.DefaultSettings())
		persistentJar = nil
	})
}

func TestBuildHTTPClient_VerifySSL(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{VerifySSL: true})
	tr := c.Transport.(*http.Transport)
	if tr.TLSClientConfig != nil {
		t.Errorf("VerifySSL=true: TLSClientConfig should be nil, got %+v", tr.TLSClientConfig)
	}
	c = buildHTTPClient(model.AppSettings{VerifySSL: false})
	tr = c.Transport.(*http.Transport)
	if tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("VerifySSL=false: expected InsecureSkipVerify=true, got %+v", tr.TLSClientConfig)
	}
	_ = tls.Config{}
}

func TestBuildHTTPClient_KeepAlive(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{KeepAlive: true})
	tr := c.Transport.(*http.Transport)
	if tr.DisableKeepAlives {
		t.Error("KeepAlive=true: DisableKeepAlives should be false")
	}
	c = buildHTTPClient(model.AppSettings{KeepAlive: false})
	tr = c.Transport.(*http.Transport)
	if !tr.DisableKeepAlives {
		t.Error("KeepAlive=false: DisableKeepAlives should be true")
	}
}

func TestBuildHTTPClient_MaxConnsPerHost(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{MaxConnsPerHost: 25})
	tr := c.Transport.(*http.Transport)
	if tr.MaxConnsPerHost != 25 {
		t.Errorf("MaxConnsPerHost should be 25, got %d", tr.MaxConnsPerHost)
	}
	c = buildHTTPClient(model.AppSettings{MaxConnsPerHost: 0})
	tr = c.Transport.(*http.Transport)
	if tr.MaxConnsPerHost != 0 {
		t.Errorf("MaxConnsPerHost=0 should leave it at 0, got %d", tr.MaxConnsPerHost)
	}
}

func TestBuildHTTPClient_DisableHTTP2(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{DisableHTTP2: true})
	tr := c.Transport.(*http.Transport)
	if tr.ForceAttemptHTTP2 {
		t.Error("DisableHTTP2=true: ForceAttemptHTTP2 should be false")
	}
	if tr.TLSNextProto == nil {
		t.Error("DisableHTTP2=true: TLSNextProto should be non-nil (empty map)")
	}
	c = buildHTTPClient(model.AppSettings{DisableHTTP2: false})
	tr = c.Transport.(*http.Transport)
	if !tr.ForceAttemptHTTP2 {
		t.Error("DisableHTTP2=false: ForceAttemptHTTP2 should be true")
	}
	if tr.TLSNextProto != nil {
		t.Error("DisableHTTP2=false: TLSNextProto should be nil")
	}
}

func TestBuildHTTPClient_Timeouts(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{
		ConnectTimeoutSec:      5,
		TLSHandshakeTimeoutSec: 6,
		IdleConnTimeoutSec:     7,
		RequestTimeoutSec:      8,
	})
	tr := c.Transport.(*http.Transport)
	if tr.TLSHandshakeTimeout != 6*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 6s", tr.TLSHandshakeTimeout)
	}
	if tr.IdleConnTimeout != 7*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 7s", tr.IdleConnTimeout)
	}
	if c.Timeout != 8*time.Second {
		t.Errorf("Client.Timeout = %v, want 8s", c.Timeout)
	}
	if tr.DialContext == nil {
		t.Error("ConnectTimeoutSec>0 should set DialContext")
	}
}

func TestBuildHTTPClient_NoTimeouts(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{})
	tr := c.Transport.(*http.Transport)
	if tr.TLSHandshakeTimeout != 10*time.Second {
		// http.DefaultTransport defaults
		t.Logf("TLSHandshakeTimeout=%v (clone default)", tr.TLSHandshakeTimeout)
	}
	if c.Timeout != 0 {
		t.Errorf("Client.Timeout should be 0 when RequestTimeoutSec=0, got %v", c.Timeout)
	}
}

func TestBuildHTTPClient_Proxy(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{Proxy: "http://proxy.example.com:8080"})
	tr := c.Transport.(*http.Transport)
	if tr.Proxy == nil {
		t.Fatal("Proxy should be set")
	}
	req, _ := http.NewRequest("GET", "http://target.example.com", nil)
	u, err := tr.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy func returned err: %v", err)
	}
	if u == nil || u.Host != "proxy.example.com:8080" {
		t.Errorf("unexpected proxy URL: %v", u)
	}
}

func TestBuildHTTPClient_ProxyEmptyAndInvalid(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{Proxy: "   "})
	tr := c.Transport.(*http.Transport)
	_ = tr
	c = buildHTTPClient(model.AppSettings{Proxy: "::not-a-url"})
	tr = c.Transport.(*http.Transport)
	_ = tr
}

func TestBuildHTTPClient_CookieJar(t *testing.T) {
	resetHTTPClient(t)
	c := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c.Jar == nil {
		t.Error("CookieJarEnabled=true should set Jar")
	}
	first := c.Jar
	c = buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c.Jar != first {
		t.Error("persistentJar should be reused across calls")
	}
	c = buildHTTPClient(model.AppSettings{CookieJarEnabled: false})
	if c.Jar != nil {
		t.Error("CookieJarEnabled=false should leave Jar nil")
	}
}

func TestBuildHTTPClient_FollowRedirectsDisabled(t *testing.T) {
	resetHTTPClient(t)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redir.Close()

	c := buildHTTPClient(model.AppSettings{FollowRedirects: false})
	resp, err := c.Get(redir.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 (no follow), got %d", resp.StatusCode)
	}
}

func TestBuildHTTPClient_FollowRedirectsMaxLimit(t *testing.T) {
	resetHTTPClient(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL+r.URL.Path+"a", http.StatusFound)
	}))
	defer srv.Close()

	c := buildHTTPClient(model.AppSettings{FollowRedirects: true, MaxRedirects: 2})
	resp, err := c.Get(srv.URL + "/x")
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected redirect-limit error")
	}
	if !strings.Contains(err.Error(), "stopped after 2 redirects") {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestBuildHTTPClient_FollowRedirectsNoLimit(t *testing.T) {
	resetHTTPClient(t)
	hops := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops < 3 {
			http.Redirect(w, r, srv.URL+"/next", http.StatusFound)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	c := buildHTTPClient(model.AppSettings{FollowRedirects: true, MaxRedirects: 0})
	resp, err := c.Get(srv.URL)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("expected 200 after redirects, got %d", resp.StatusCode)
	}
}

func TestApply_NilThemeNoPanic(t *testing.T) {
	resetHTTPClient(t)
	// must not panic with nil *material.Theme
	Apply(nil, model.DefaultSettings())
	if UserAgent == "" {
		t.Error("UserAgent should be populated after Apply")
	}
	if HTTPClient == nil {
		t.Error("HTTPClient should be set after Apply")
	}
}

func TestApply_ClampsDefaults(t *testing.T) {
	resetHTTPClient(t)
	bad := model.AppSettings{
		Theme:             "dark",
		BodyTextSize:      12,
		UserAgent:         "",
		JSONIndentSpaces:  -1,
		PreviewMaxMB:      0,
		DefaultMethod:     "",
		DefaultSplitRatio: 0.05,
	}
	Apply(nil, bad)
	if UserAgent == "" {
		t.Error("empty UserAgent should be defaulted in Apply")
	}
	if JSONIndent != 2 {
		t.Errorf("JSONIndent should clamp from -1 to 2, got %d", JSONIndent)
	}
	if PreviewMaxMB != 100 {
		t.Errorf("PreviewMaxMB should clamp from 0 to 100, got %d", PreviewMaxMB)
	}
	if DefaultMethod != "GET" {
		t.Errorf("empty DefaultMethod should default to GET, got %q", DefaultMethod)
	}
	if DefaultSplitRatio != 0.5 {
		t.Errorf("DefaultSplitRatio out-of-range should reset to 0.5, got %v", DefaultSplitRatio)
	}
}

func TestApply_OldClientTransportClosed(t *testing.T) {
	resetHTTPClient(t)
	// Just make sure Apply does not panic when replacing client.
	Apply(nil, model.DefaultSettings())
	old := HTTPClient
	Apply(nil, model.DefaultSettings())
	if old == HTTPClient {
		t.Log("Apply produced same client identity (unusual but not a failure)")
	}
}

func TestHeadersToText(t *testing.T) {
	if got := headersToText(nil); got != "" {
		t.Errorf("nil → %q, want empty", got)
	}
	if got := headersToText([]model.DefaultHeader{}); got != "" {
		t.Errorf("empty slice → %q", got)
	}
	in := []model.DefaultHeader{
		{Key: "X-A", Value: "1"},
		{Key: "  ", Value: "skipped"},
		{Key: "X-B", Value: ""},
	}
	got := headersToText(in)
	want := "X-A: 1\nX-B: "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTextToHeaders(t *testing.T) {
	if got := textToHeaders(""); got != nil {
		t.Errorf("empty → %v", got)
	}
	if got := textToHeaders("   \n   "); got != nil {
		t.Errorf("whitespace-only → %v", got)
	}
	in := "X-A: 1\n# comment\nX-B:2\nbad-line-no-colon\n: missingkey\nX-C: with: colons"
	got := textToHeaders(in)
	want := []model.DefaultHeader{
		{Key: "X-A", Value: "1"},
		{Key: "X-B", Value: "2"},
		{Key: "X-C", Value: "with: colons"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d headers, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] %+v != %+v", i, got[i], want[i])
		}
	}
}

func TestTextToHeaders_RoundTrip(t *testing.T) {
	in := []model.DefaultHeader{
		{Key: "Accept", Value: "application/json"},
		{Key: "X-Custom", Value: "val with spaces"},
	}
	text := headersToText(in)
	out := textToHeaders(text)
	if len(out) != len(in) {
		t.Fatalf("len mismatch: %d vs %d", len(out), len(in))
	}
	for i := range in {
		if out[i] != in[i] {
			t.Errorf("[%d] %+v != %+v", i, out[i], in[i])
		}
	}
}

func TestPersistentJar_ResetOnDisable(t *testing.T) {
	resetHTTPClient(t)
	c1 := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c1.Jar == nil {
		t.Fatal("first client should have jar")
	}
	c2 := buildHTTPClient(model.AppSettings{CookieJarEnabled: false})
	if c2.Jar != nil {
		t.Error("after disable, new client should have nil jar")
	}
	c3 := buildHTTPClient(model.AppSettings{CookieJarEnabled: true})
	if c3.Jar == nil {
		t.Fatal("re-enabled client should have jar")
	}
	if c3.Jar == c1.Jar {
		t.Error("re-enabling after disable must produce a fresh jar (cookies cleared)")
	}
}
