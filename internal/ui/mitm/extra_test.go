package mitm

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ---------- pure helpers ----------

func TestCanonicalHost(t *testing.T) {
	cases := map[string]string{
		"localhost":   "127.0.0.1",
		"LOCALHOST":   "127.0.0.1",
		"  Localhost ": "127.0.0.1",
		"::1":         "127.0.0.1",
		"[::1]":       "127.0.0.1",
		"0.0.0.0":     "127.0.0.1",
		"example.com": "example.com",
		"EXAMPLE.COM": "example.com",
	}
	for in, want := range cases {
		if got := canonicalHost(in); got != want {
			t.Errorf("canonicalHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSameHost(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"127.0.0.1:8888", "localhost:8888", true},
		{"127.0.0.1:8888", "127.0.0.1:9999", false},
		{"example.com", "EXAMPLE.com", true},
		{"example.com:80", "example.com:81", false},
		{"example.com", "other.com", false},
		// One side missing port — compare host only.
		{"example.com:80", "example.com", true},
	}
	for _, c := range cases {
		if got := sameHost(c.a, c.b); got != c.want {
			t.Errorf("sameHost(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSplitHostPort(t *testing.T) {
	h, p, err := splitHostPort("example.com:8080", "80")
	if err != nil || h != "example.com" || p != "8080" {
		t.Fatalf("got %q %q %v", h, p, err)
	}
	// No port → default.
	h, p, err = splitHostPort("example.com", "443")
	if err != nil || h != "example.com" || p != "443" {
		t.Fatalf("default port: got %q %q %v", h, p, err)
	}
	// Bracketed IPv6 with port — handled by net.SplitHostPort.
	h, p, err = splitHostPort("[::1]:9999", "443")
	if err != nil || h != "::1" || p != "9999" {
		t.Fatalf("ipv6: got %q %q %v", h, p, err)
	}
}

func TestStripHopByHop(t *testing.T) {
	h := http.Header{}
	h.Set("Connection", "X-Custom, Keep-Alive")
	h.Set("X-Custom", "drop me")
	h.Set("Keep-Alive", "timeout=5")
	h.Set("Proxy-Authenticate", "Basic")
	h.Set("Transfer-Encoding", "chunked")
	h.Set("Upgrade", "websocket")
	h.Set("X-Keep", "kept")
	h.Set("Trailer", "Expires")
	h.Set("Te", "trailers")
	h.Set("Proxy-Authorization", "creds")

	stripHopByHop(h)

	for _, k := range []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Transfer-Encoding",
		"Upgrade", "X-Custom", "Trailer", "Te", "Proxy-Authorization",
	} {
		if v := h.Get(k); v != "" {
			t.Errorf("expected %s removed, still %q", k, v)
		}
	}
	if h.Get("X-Keep") != "kept" {
		t.Errorf("X-Keep wrongly dropped")
	}
}

func TestStripHopByHopEmptyConnection(t *testing.T) {
	h := http.Header{}
	h.Set("X-Real", "ok")
	stripHopByHop(h) // no Connection header, just exercises the fallthrough.
	if h.Get("X-Real") != "ok" {
		t.Fatal("non-hop-by-hop header lost")
	}
}

func TestCollectHeaders(t *testing.T) {
	h := http.Header{}
	h.Add("X-Multi", "a")
	h.Add("X-Multi", "b")
	h.Set("X-Single", "z")
	out := collectHeaders(h)
	// 3 total entries.
	if len(out) != 3 {
		t.Fatalf("expected 3, got %d (%v)", len(out), out)
	}
	count := map[string]int{}
	for _, kv := range out {
		count[kv[0]+"="+kv[1]]++
	}
	for _, want := range []string{"X-Multi=a", "X-Multi=b", "X-Single=z"} {
		if count[want] != 1 {
			t.Errorf("missing entry %q in %v", want, out)
		}
	}
}

func TestReadLimited(t *testing.T) {
	// Nil reader returns nil, nil.
	b, err := readLimited(nil, 16)
	if err != nil || b != nil {
		t.Fatalf("nil reader: got %v %v", b, err)
	}
	// Cap enforced.
	src := bytes.NewReader([]byte("0123456789ABCDEF"))
	b, err = readLimited(src, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "0123" {
		t.Errorf("got %q", b)
	}
	// Body smaller than cap.
	src2 := bytes.NewReader([]byte("hi"))
	b, err = readLimited(src2, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "hi" {
		t.Errorf("got %q", b)
	}
}

func TestWriteStatus(t *testing.T) {
	p1, p2 := net.Pipe()
	defer p2.Close()
	go func() {
		defer p1.Close()
		writeStatus(p1, 502, "boom")
	}()
	r := bufio.NewReader(p2)
	resp, err := http.ReadResponse(r, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "boom" {
		t.Errorf("body = %q", body)
	}
	// net/http hoists "Connection: close" out of Header into resp.Close.
	if !resp.Close {
		t.Errorf("expected Close=true; headers=%v", resp.Header)
	}
}

func TestBridgeCountsBytes(t *testing.T) {
	// Use real TCP so CloseWrite() works (net.Pipe has no half-close).
	pair := func() (net.Conn, net.Conn) {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		type result struct {
			c   net.Conn
			err error
		}
		ch := make(chan result, 1)
		go func() {
			c, err := l.Accept()
			ch <- result{c, err}
		}()
		client, err := net.Dial("tcp", l.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		r := <-ch
		if r.err != nil {
			t.Fatal(r.err)
		}
		return client, r.c
	}
	clientA, serverA := pair()
	clientB, serverB := pair()
	defer clientA.Close()
	defer serverA.Close()
	defer clientB.Close()
	defer serverB.Close()

	// Writer on the a-side sends 5 bytes then half-closes.
	go func() {
		_, _ = serverA.Write([]byte("hello"))
		_ = serverA.(*net.TCPConn).CloseWrite()
	}()
	// Writer on the b-side reads first then sends 6 bytes and half-closes.
	go func() {
		buf := make([]byte, 16)
		_, _ = serverB.Read(buf)
		_, _ = serverB.Write([]byte("WORLD!"))
		_ = serverB.(*net.TCPConn).CloseWrite()
	}()
	in, out := bridge(clientA, clientB)
	if out != 5 {
		t.Errorf("out=%d, want 5", out)
	}
	if in != 6 {
		t.Errorf("in=%d, want 6", in)
	}
}

// ---------- Store ----------

func TestStoreClearAndNotify(t *testing.T) {
	s := NewStore()
	var calls int64
	s.SetNotify(func() { atomic.AddInt64(&calls, 1) })
	s.Add(&Flow{Method: "GET"})
	s.Add(&Flow{Method: "POST"})
	if s.Len() != 2 {
		t.Fatalf("len = %d", s.Len())
	}
	s.Clear()
	if s.Len() != 0 {
		t.Fatalf("len after Clear = %d", s.Len())
	}
	// Add + Add + Clear = 3 notifications.
	if got := atomic.LoadInt64(&calls); got != 3 {
		t.Errorf("notify count = %d, want 3", got)
	}
	// SetNotify(nil) clears the callback.
	s.SetNotify(nil)
	s.Add(&Flow{})
	if got := atomic.LoadInt64(&calls); got != 3 {
		t.Errorf("notify after clear = %d, want 3", got)
	}
}

func TestStoreAtBounds(t *testing.T) {
	s := NewStore()
	if s.At(-1) != nil || s.At(0) != nil {
		t.Fatal("out-of-range At must return nil")
	}
	s.Add(&Flow{Method: "GET"})
	if s.At(0) == nil {
		t.Fatal("At(0) should return a flow")
	}
	if s.At(1) != nil {
		t.Fatal("At(1) on size-1 store must be nil")
	}
}

func TestFlowLive(t *testing.T) {
	f := &Flow{}
	if !f.Live() {
		t.Fatal("zero Ended must be Live")
	}
	f.Ended = time.Now()
	if f.Live() {
		t.Fatal("set Ended must not be Live")
	}
}

func TestStoreMarkAllEnded(t *testing.T) {
	s := NewStore()
	already := time.Now().Add(-time.Hour)
	f1 := s.Add(&Flow{Method: "GET"})
	f2 := s.Add(&Flow{Method: "POST", Ended: already})
	s.MarkAllEnded()
	if f1.Ended.IsZero() {
		t.Fatal("live flow must be marked ended")
	}
	if !f2.Ended.Equal(already) {
		t.Fatal("MarkAllEnded must not overwrite preset Ended")
	}
}

func TestStoreSnapshotIsValueCopy(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Method: "GET", URL: "https://example.com"})
	snap := s.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snap len = %d", len(snap))
	}
	// Mutating returned pointer must NOT affect the live flow's scalar fields.
	snap[0].URL = "https://hacked.example"
	if s.At(0).URL != "https://example.com" {
		t.Fatalf("scalar field leaked to snapshot: %q", s.At(0).URL)
	}
}

func TestStoreSnapshotSlicesAreDeepCopied(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{
		ReqBody:     []byte("abc"),
		RespBody:    []byte("xyz"),
		ReqHeaders:  [][2]string{{"K", "V"}},
		RespHeaders: [][2]string{{"X", "Y"}},
	})
	snap := s.Snapshot()
	snap[0].ReqBody[0] = 'Z'
	snap[0].RespBody[0] = 'Z'
	snap[0].ReqHeaders[0] = [2]string{"Modified", "Modified"}
	snap[0].RespHeaders[0] = [2]string{"Modified", "Modified"}
	if got := s.At(0).ReqBody[0]; got != 'a' {
		t.Errorf("ReqBody must be deep-copied; live store mutated to %q", got)
	}
	if got := s.At(0).RespBody[0]; got != 'x' {
		t.Errorf("RespBody must be deep-copied; live store mutated to %q", got)
	}
	if s.At(0).ReqHeaders[0][0] != "K" {
		t.Errorf("ReqHeaders must be deep-copied; live store mutated")
	}
	if s.At(0).RespHeaders[0][0] != "X" {
		t.Errorf("RespHeaders must be deep-copied; live store mutated")
	}
}

// ---------- CA ----------

func TestLoadCAMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCA(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

	// Cert exists, key missing.
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CACertPath(dir), ca.CertPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = LoadCA(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist for missing key, got %v", err)
	}
}

func TestLoadCAGarbageFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(CACertPath(dir), []byte("not pem at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(CAKeyPath(dir), []byte("nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadCA(dir)
	if err == nil {
		t.Fatal("expected error decoding garbage")
	}
}

func TestCAPathHelpers(t *testing.T) {
	dir := filepath.Join("a", "b")
	if got := CACertPath(dir); !strings.HasSuffix(got, caCertFile) {
		t.Errorf("CACertPath = %q", got)
	}
	if got := CAKeyPath(dir); !strings.HasSuffix(got, caKeyFile) {
		t.Errorf("CAKeyPath = %q", got)
	}
}

func TestCAGenerateProperties(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	if !ca.Cert.IsCA {
		t.Error("Cert.IsCA must be true")
	}
	if !ca.Cert.BasicConstraintsValid {
		t.Error("BasicConstraintsValid must be true")
	}
	if !ca.Cert.MaxPathLenZero {
		t.Error("MaxPathLenZero must be true (no intermediate CAs allowed)")
	}
	if ca.Cert.MaxPathLen != 0 {
		t.Errorf("MaxPathLen = %d, want 0", ca.Cert.MaxPathLen)
	}
	if ca.Cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("CA must have KeyUsageCertSign")
	}
	if len(ca.Cert.SubjectKeyId) == 0 {
		t.Error("CA must populate SubjectKeyId for AKI chaining")
	}
	// 10y validity ± grace.
	dur := ca.Cert.NotAfter.Sub(ca.Cert.NotBefore)
	want := caValidity + 1*time.Minute // -1m skew
	if dur < want-time.Minute || dur > want+time.Minute {
		t.Errorf("validity = %v, want ~%v", dur, want)
	}
	// Subject identity.
	if ca.Cert.Subject.CommonName != caCommonName {
		t.Errorf("CN = %q", ca.Cert.Subject.CommonName)
	}
}

func TestCAFingerprintNonEmpty(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	fp := ca.Fingerprint()
	if fp == "" {
		t.Fatal("fingerprint empty")
	}
	// SHA-1 hex = 20 bytes = 19 colons + 40 hex chars = 59 chars.
	if len(fp) != 59 {
		t.Errorf("len(fp) = %d, want 59 (%q)", len(fp), fp)
	}
	// Empty CA cert -> empty fingerprint.
	if (&CA{}).Fingerprint() != "" {
		t.Error("empty CA must return empty fingerprint")
	}
}

func TestCALeafForVariants(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	// Empty host rejected.
	if _, err := ca.LeafFor(""); err == nil {
		t.Error("empty host should error")
	}
	// Whitespace-only.
	if _, err := ca.LeafFor("   "); err == nil {
		t.Error("whitespace-only host should error")
	}

	// Hostname → DNS SAN.
	leaf, err := ca.LeafFor("example.com")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(leaf.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.DNSNames) != 1 || parsed.DNSNames[0] != "example.com" {
		t.Errorf("DNS SANs = %v", parsed.DNSNames)
	}
	if len(parsed.IPAddresses) != 0 {
		t.Errorf("unexpected IP SANs = %v", parsed.IPAddresses)
	}
	// Includes server + client EKUs.
	hasServer := false
	hasClient := false
	for _, u := range parsed.ExtKeyUsage {
		switch u {
		case x509.ExtKeyUsageServerAuth:
			hasServer = true
		case x509.ExtKeyUsageClientAuth:
			hasClient = true
		}
	}
	if !hasServer || !hasClient {
		t.Errorf("EKUs = %v, want both ServerAuth+ClientAuth", parsed.ExtKeyUsage)
	}
	// AKI must chain to CA SKI.
	if !bytes.Equal(parsed.AuthorityKeyId, ca.Cert.SubjectKeyId) {
		t.Errorf("AKI %x != CA SKI %x", parsed.AuthorityKeyId, ca.Cert.SubjectKeyId)
	}
	// 1y validity.
	dur := parsed.NotAfter.Sub(parsed.NotBefore)
	want := leafValidity + 1*time.Minute
	if dur < want-time.Minute || dur > want+time.Minute {
		t.Errorf("leaf validity = %v, want ~%v", dur, want)
	}
	if parsed.IsCA {
		t.Error("leaf must not be CA")
	}

	// IP literal → IP SAN.
	leaf2, err := ca.LeafFor("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	p2, _ := x509.ParseCertificate(leaf2.Certificate[0])
	if len(p2.IPAddresses) != 1 || !p2.IPAddresses[0].Equal(net.ParseIP("127.0.0.1")) {
		t.Errorf("IP SANs = %v", p2.IPAddresses)
	}
	if len(p2.DNSNames) != 0 {
		t.Errorf("unexpected DNS SANs for IP leaf: %v", p2.DNSNames)
	}

	// host:port stripped before minting.
	leaf3, err := ca.LeafFor("Example.com:8443")
	if err != nil {
		t.Fatal(err)
	}
	p3, _ := x509.ParseCertificate(leaf3.Certificate[0])
	if len(p3.DNSNames) != 1 || p3.DNSNames[0] != "example.com" {
		t.Errorf("DNS SAN with port stripped = %v", p3.DNSNames)
	}

	// Chain validates: leaf -> root.
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	if _, err := parsed.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "example.com",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Errorf("leaf chain failed to verify: %v", err)
	}
}

func TestCALeafForCaching(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	a, err := ca.LeafFor("Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	b, err := ca.LeafFor("example.com")
	if err != nil {
		t.Fatal(err)
	}
	// Cache keyed on lowercased host — same cert pointer.
	if a != b {
		t.Error("LeafFor must cache and return same *tls.Certificate for case-insensitive host")
	}
}

func TestCALeafCacheEviction(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	// Fill cache to the limit using cheap hosts.
	for i := 0; i < leafCacheLimit; i++ {
		if _, err := ca.LeafFor(fmt.Sprintf("h%d.example", i)); err != nil {
			t.Fatal(err)
		}
	}
	ca.mu.Lock()
	got := len(ca.leaves)
	ca.mu.Unlock()
	if got != leafCacheLimit {
		t.Fatalf("cache size pre-eviction = %d, want %d", got, leafCacheLimit)
	}
	// One more triggers wholesale clear, then re-insert of the new one.
	if _, err := ca.LeafFor("trigger.example"); err != nil {
		t.Fatal(err)
	}
	ca.mu.Lock()
	got = len(ca.leaves)
	ca.mu.Unlock()
	if got != 1 {
		t.Fatalf("cache size post-eviction = %d, want 1", got)
	}
}

func TestCASaveAndReload(t *testing.T) {
	dir := t.TempDir()
	gen, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	if err := gen.Save(dir); err != nil {
		t.Fatal(err)
	}
	// Cert file is world-readable; key file is owner-only.
	if st, err := os.Stat(CACertPath(dir)); err == nil {
		_ = st // permissions check is platform-specific; existence is enough on Windows.
	} else {
		t.Fatalf("cert file missing: %v", err)
	}
	if _, err := os.Stat(CAKeyPath(dir)); err != nil {
		t.Fatalf("key file missing: %v", err)
	}

	// Reload and compare fingerprint AND private key modulus.
	loaded, err := LoadCA(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Fingerprint() != gen.Fingerprint() {
		t.Errorf("fingerprint mismatch after reload")
	}
	if loaded.Key.N.Cmp(gen.Key.N) != 0 {
		t.Errorf("private key not preserved across save/load")
	}

	// Leaf signed by reloaded CA chains to original CA cert.
	leaf, err := loaded.LeafFor("re.example")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := x509.ParseCertificate(leaf.Certificate[0])
	roots := x509.NewCertPool()
	roots.AddCert(gen.Cert)
	if _, err := parsed.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   "re.example",
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}); err != nil {
		t.Errorf("leaf from reloaded CA does not chain to original: %v", err)
	}
}

// ---------- Proxy / SetIntercept ----------

func TestProxyInterceptingRequiresCA(t *testing.T) {
	p := NewProxy(NewStore())
	if p.Intercepting() {
		t.Fatal("default should be off")
	}
	// No CA installed — SetIntercept(true) is a no-op.
	p.SetIntercept(true)
	if p.Intercepting() {
		t.Fatal("intercept must remain off when CA is nil")
	}
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	p.SetCA(ca)
	p.SetIntercept(true)
	if !p.Intercepting() {
		t.Fatal("intercept must turn on once CA is set")
	}
	// Removing CA forces intercept off.
	p.SetCA(nil)
	if p.Intercepting() {
		t.Fatal("SetCA(nil) must force intercept off")
	}
	if p.CA() != nil {
		t.Fatal("CA() after SetCA(nil) must be nil")
	}
}

func TestProxyStartInvalidAddr(t *testing.T) {
	p := NewProxy(NewStore())
	// Port 0 with bogus host should fail.
	if err := p.Start("invalid host:::not a port"); err == nil {
		t.Fatal("expected error from invalid Start addr")
	}
	if p.Running() {
		t.Fatal("Running should stay false after failed Start")
	}
}

func TestProxyStartDefaultAddrUsedWhenEmpty(t *testing.T) {
	// Bind to :0-equivalent isn't possible with the default const, so we
	// only exercise the empty-addr branch by starting and immediately
	// stopping. If DefaultAddr is already taken, skip — this is a sanity
	// check, not a guarantee.
	p := NewProxy(NewStore())
	if err := p.Start(""); err != nil {
		t.Skipf("DefaultAddr %s unavailable: %v", DefaultAddr, err)
	}
	defer p.Stop()
	if !strings.HasPrefix(p.Addr(), "127.0.0.1") {
		t.Errorf("addr = %q", p.Addr())
	}
}

// Listener that immediately closes — exercises serve() loop exit path.
func TestProxyServeExitsOnAcceptError(t *testing.T) {
	p := NewProxy(NewStore())
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	// Stop closes the listener; serve() must return promptly.
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked > 2s after listener close")
	}
}

// CONNECT to an address that won't be dialable → expect 502 in flow + over wire.
func TestProxyConnectDialFailure(t *testing.T) {
	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// Pick a port we know is closed: bind to :0, capture port, close.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	closedAddr := l.Addr().String()
	_ = l.Close()

	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", closedAddr, closedAddr)
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("dial-fail status = %d, want 502", resp.StatusCode)
	}

	// Flow recorded.
	deadline := time.Now().Add(time.Second)
	var f *Flow
	for time.Now().Before(deadline) {
		if store.Len() > 0 {
			f = store.At(0)
			if f != nil && f.StatusCode != 0 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if f == nil || f.StatusCode != 502 {
		t.Fatalf("flow not recorded with 502: %+v", f)
	}
	if f.Error == "" {
		t.Errorf("expected Error on dial failure")
	}
}

func TestProxyMalformedConnectHost(t *testing.T) {
	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// Empty Host header on CONNECT — splitHostPort with default keeps host
	// empty, which is fine but the dial will fail with "missing host". We
	// expect either a 400 or a 502.
	fmt.Fprint(c, "CONNECT  HTTP/1.1\r\nHost: \r\n\r\n")
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return // connection torn down is acceptable
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx/5xx for malformed CONNECT, got %d", resp.StatusCode)
	}
}

func TestProxyHTTPUpstreamError(t *testing.T) {
	// Forward to a closed port → upstream error path.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := l.Addr().String()
	_ = l.Close()

	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   3 * time.Second,
	}
	req, _ := http.NewRequest("GET", "http://"+deadAddr+"/", nil)
	resp, err := cl.Do(req)
	if err != nil {
		// Some Go versions surface the proxy 502 as an error; that's fine.
		return
	}
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	deadline := time.Now().Add(time.Second)
	var f *Flow
	for time.Now().Before(deadline) {
		if store.Len() > 0 {
			f = store.At(0)
			if f != nil && f.StatusCode != 0 {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	if f == nil || f.StatusCode != 502 || f.Error == "" {
		t.Fatalf("expected 502 flow with Error, got %+v", f)
	}
}

func TestProxyClearsCapturedBodyAtLimit(t *testing.T) {
	// Upstream returns a body larger than maxCaptureBody but smaller than
	// maxBodyForward. The wire body is full; the captured RespBody is
	// truncated to maxCaptureBody.
	const respLen = maxCaptureBody + 1024
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := bytes.Repeat([]byte("x"), respLen)
		_, _ = w.Write(buf)
	}))
	defer upstream.Close()

	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   10 * time.Second,
	}
	resp, err := cl.Get(upstream.URL + "/big")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(body) != respLen {
		t.Fatalf("wire body len = %d, want %d", len(body), respLen)
	}

	deadline := time.Now().Add(2 * time.Second)
	var f *Flow
	for time.Now().Before(deadline) {
		if store.Len() > 0 {
			f = store.At(0)
			if f != nil && f.StatusCode != 0 && f.RespSize > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if f == nil {
		t.Fatal("no flow recorded")
	}
	if int64(len(f.RespBody)) > maxCaptureBody {
		t.Errorf("captured body len = %d > cap %d", len(f.RespBody), maxCaptureBody)
	}
	if f.RespSize != int64(respLen) {
		t.Errorf("RespSize = %d, want %d", f.RespSize, respLen)
	}
}

// ---------- Intercept-only edge: 502 on bad upstream ----------

func TestProxyInterceptUpstreamUnreachable(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore()
	p := NewProxy(store)
	p.SetCA(ca)
	p.SetIntercept(true)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	// Pick a free port and let it close so the inner request dial fails.
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	deadAddr := l.Addr().String()
	_ = l.Close()

	clientPool := x509.NewCertPool()
	clientPool.AddCert(ca.Cert)

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: clientPool, ServerName: "ignored"},
		},
		Timeout: 5 * time.Second,
	}
	resp, err := cl.Get("https://" + deadAddr + "/x")
	if err != nil {
		// Network failure is also acceptable — we just want no panic.
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("status = %d body=%q, want 502", resp.StatusCode, body)
	}

	// At least the parent CONNECT flow should exist; the inner HTTP flow
	// may exist too and have Error set.
	if store.Len() == 0 {
		t.Fatal("no flow recorded")
	}
	flows := store.Snapshot()
	sawError := false
	for _, f := range flows {
		if f.Error != "" {
			sawError = true
			break
		}
	}
	if !sawError {
		t.Errorf("expected at least one flow with Error: %+v", flows)
	}
}

// ---------- store concurrency ----------

func TestStoreConcurrentAddUpdate(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				f := s.Add(&Flow{Method: "GET"})
				s.Update(func() {
					f.Status = "200 OK"
					f.StatusCode = 200
				})
			}
		}()
	}
	// Concurrent reads.
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			_ = s.Snapshot()
			_ = s.Len()
		}
	}()
	wg.Wait()
	if s.Len() != 800 {
		t.Errorf("len = %d, want 800", s.Len())
	}
}

// ---------- Save error path ----------

func TestCASaveBadDir(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	// On Windows, NUL is reserved; on POSIX, a NUL byte in path is invalid.
	// Use a path under a file (so MkdirAll has to create dir under a file).
	parent := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Now ask Save to put files under that regular file → MkdirAll fails.
	bad := filepath.Join(parent, "sub")
	if err := ca.Save(bad); err == nil {
		t.Fatal("expected error saving under non-directory parent")
	}
}
