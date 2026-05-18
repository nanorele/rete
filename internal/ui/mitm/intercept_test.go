package mitm

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestProxyHTTPSIntercept brings up an httptest TLS server, points an
// HTTP client through the proxy with HTTPS interception enabled, and
// verifies (a) the client gets the real response, (b) the proxy captured
// the inner request and response with full headers and body.
func TestProxyHTTPSIntercept(t *testing.T) {
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Inner", "decrypted")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("inner:" + string(body)))
	}))
	defer upstream.Close()

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	store := NewStore()
	p := NewProxy(store)
	p.SetCA(ca)
	p.SetIntercept(true)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// Client trusts our CA AND the upstream's self-signed cert (the proxy
	// makes the real outbound call, so it also needs to trust upstream).
	clientPool := x509.NewCertPool()
	clientPool.AddCert(ca.Cert)
	upstreamPool := x509.NewCertPool()
	upstreamPool.AddCert(upstream.Certificate())

	// The proxy verifies upstream against the system roots by default,
	// which won't include httptest's cert. Inject a custom RootCAs into
	// the proxy's intercept transport via the test-only hook below.
	interceptDialRoots = upstreamPool
	defer func() { interceptDialRoots = nil }()

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: clientPool},
		},
		Timeout: 5 * time.Second,
	}

	resp, err := cl.Post(upstream.URL+"/x", "text/plain", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "inner:hello" {
		t.Fatalf("body = %q", body)
	}
	if resp.Header.Get("X-Inner") != "decrypted" {
		t.Fatalf("X-Inner header missing: %v", resp.Header)
	}

	// We should see at least 2 flows: the parent CONNECT (Kind=tunnel)
	// and the inner intercepted POST (Kind=http).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if store.Len() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	flows := store.Snapshot()
	if len(flows) < 2 {
		t.Fatalf("expected ≥2 flows, got %d", len(flows))
	}

	var inner *Flow
	for _, f := range flows {
		if f.Kind == FlowHTTP && f.Method == "POST" {
			inner = f
			break
		}
	}
	if inner == nil {
		t.Fatalf("no intercepted HTTP flow: %+v", flows)
	}
	if inner.StatusCode != 200 {
		t.Fatalf("inner status = %d", inner.StatusCode)
	}
	if string(inner.ReqBody) != "hello" {
		t.Fatalf("captured req body = %q", inner.ReqBody)
	}
	if string(inner.RespBody) != "inner:hello" {
		t.Fatalf("captured resp body = %q", inner.RespBody)
	}
	gotInner := false
	for _, h := range inner.RespHeaders {
		if h[0] == "X-Inner" && h[1] == "decrypted" {
			gotInner = true
		}
	}
	if !gotInner {
		t.Fatalf("X-Inner not captured in resp headers: %v", inner.RespHeaders)
	}
}

func TestCAGenerateAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	gen, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Save(dir); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint() != gen.Fingerprint() {
		t.Fatalf("fingerprint mismatch: got %s want %s", loaded.Fingerprint(), gen.Fingerprint())
	}

	leaf, err := loaded.LeafFor("example.com")
	if err != nil {
		t.Fatal(err)
	}
	if len(leaf.Certificate) != 2 {
		t.Fatalf("expected leaf+CA chain (len=2), got %d", len(leaf.Certificate))
	}
}
