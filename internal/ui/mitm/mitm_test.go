package mitm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/f32"
	"github.com/nanorele/gio/font/gofont"
	"github.com/nanorele/gio/io/input"
	"github.com/nanorele/gio/io/pointer"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/op"
	"github.com/nanorele/gio/text"
	"github.com/nanorele/gio/unit"
	"github.com/nanorele/gio/widget/material"
	"image"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"rete/internal/persist"
	"rete/internal/ws"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "rete-test")
	persist.SetConfigOverride(configPath)

	t.Cleanup(func() {
		persist.SetConfigOverride("")
	})

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tempDir)
	case "darwin":
		t.Setenv("HOME", tempDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}

func TestCanonicalHost(t *testing.T) {
	cases := map[string]string{
		"localhost":    "127.0.0.1",
		"LOCALHOST":    "127.0.0.1",
		"  Localhost ": "127.0.0.1",
		"::1":          "127.0.0.1",
		"[::1]":        "127.0.0.1",
		"0.0.0.0":      "127.0.0.1",
		"example.com":  "example.com",
		"EXAMPLE.COM":  "example.com",
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

	h, p, err = splitHostPort("example.com", "443")
	if err != nil || h != "example.com" || p != "443" {
		t.Fatalf("default port: got %q %q %v", h, p, err)
	}

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
	stripHopByHop(h)
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

	b, err := readLimited(nil, 16)
	if err != nil || b != nil {
		t.Fatalf("nil reader: got %v %v", b, err)
	}

	src := bytes.NewReader([]byte("0123456789ABCDEF"))
	b, err = readLimited(src, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "0123" {
		t.Errorf("got %q", b)
	}

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

	if !resp.Close {
		t.Errorf("expected Close=true; headers=%v", resp.Header)
	}
}

func TestBridgeCountsBytes(t *testing.T) {

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

	go func() {
		_, _ = serverA.Write([]byte("hello"))
		_ = serverA.(*net.TCPConn).CloseWrite()
	}()

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

	if got := atomic.LoadInt64(&calls); got != 3 {
		t.Errorf("notify count = %d, want 3", got)
	}

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

func TestLoadCAMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCA(dir)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected ErrNotExist, got %v", err)
	}

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

	dur := ca.Cert.NotAfter.Sub(ca.Cert.NotBefore)
	want := caValidity + 1*time.Minute
	if dur < want-time.Minute || dur > want+time.Minute {
		t.Errorf("validity = %v, want ~%v", dur, want)
	}

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

	if len(fp) != 59 {
		t.Errorf("len(fp) = %d, want 59 (%q)", len(fp), fp)
	}

	if (&CA{}).Fingerprint() != "" {
		t.Error("empty CA must return empty fingerprint")
	}
}

func TestCALeafForVariants(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := ca.LeafFor(""); err == nil {
		t.Error("empty host should error")
	}

	if _, err := ca.LeafFor("   "); err == nil {
		t.Error("whitespace-only host should error")
	}

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

	if !bytes.Equal(parsed.AuthorityKeyId, ca.Cert.SubjectKeyId) {
		t.Errorf("AKI %x != CA SKI %x", parsed.AuthorityKeyId, ca.Cert.SubjectKeyId)
	}

	dur := parsed.NotAfter.Sub(parsed.NotBefore)
	want := leafValidity + 1*time.Minute
	if dur < want-time.Minute || dur > want+time.Minute {
		t.Errorf("leaf validity = %v, want ~%v", dur, want)
	}
	if parsed.IsCA {
		t.Error("leaf must not be CA")
	}

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

	leaf3, err := ca.LeafFor("Example.com:8443")
	if err != nil {
		t.Fatal(err)
	}
	p3, _ := x509.ParseCertificate(leaf3.Certificate[0])
	if len(p3.DNSNames) != 1 || p3.DNSNames[0] != "example.com" {
		t.Errorf("DNS SAN with port stripped = %v", p3.DNSNames)
	}

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

	if a != b {
		t.Error("LeafFor must cache and return same *tls.Certificate for case-insensitive host")
	}
}

func TestCALeafCacheEviction(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}

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

	if st, err := os.Stat(CACertPath(dir)); err == nil {
		_ = st
	} else {
		t.Fatalf("cert file missing: %v", err)
	}
	if _, err := os.Stat(CAKeyPath(dir)); err != nil {
		t.Fatalf("key file missing: %v", err)
	}

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

func TestProxyInterceptingRequiresCA(t *testing.T) {
	p := NewProxy(NewStore())
	if p.Intercepting() {
		t.Fatal("default should be off")
	}

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

	if err := p.Start("invalid host:::not a port"); err == nil {
		t.Fatal("expected error from invalid Start addr")
	}
	if p.Running() {
		t.Fatal("Running should stay false after failed Start")
	}
}

func TestProxyStartDefaultAddrUsedWhenEmpty(t *testing.T) {

	p := NewProxy(NewStore())
	if err := p.Start(""); err != nil {
		t.Skipf("DefaultAddr %s unavailable: %v", DefaultAddr, err)
	}
	defer p.Stop()
	if !strings.HasPrefix(p.Addr(), "127.0.0.1") {
		t.Errorf("addr = %q", p.Addr())
	}
}

func TestProxyServeExitsOnAcceptError(t *testing.T) {
	p := NewProxy(NewStore())
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked > 2s after listener close")
	}
}

func TestProxyConnectDialFailure(t *testing.T) {
	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

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

	fmt.Fprint(c, "CONNECT  HTTP/1.1\r\nHost: \r\n\r\n")
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		return
	}
	resp.Body.Close()
	if resp.StatusCode < 400 {
		t.Fatalf("expected 4xx/5xx for malformed CONNECT, got %d", resp.StatusCode)
	}
}

func TestProxyHTTPUpstreamError(t *testing.T) {

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

		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 502 {
		t.Fatalf("status = %d body=%q, want 502", resp.StatusCode, body)
	}

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

func TestStoreConcurrentAddUpdate(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				f := s.Add(&Flow{Method: "GET"})
				s.Update(f, func() {
					f.Status = "200 OK"
					f.StatusCode = 200
				})
			}
		}()
	}

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

func TestCASaveBadDir(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}

	parent := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	bad := filepath.Join(parent, "sub")
	if err := ca.Save(bad); err == nil {
		t.Fatal("expected error saving under non-directory parent")
	}
}

// TestReverseProxyHTTP verifies an origin-form request to a matching reverse
// target is proxied to its manual upstream and captured as a rev flow.
func TestReverseProxyHTTP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Server", "up")
		_, _ = io.WriteString(w, "upstream-body:"+r.Host)
	}))
	defer upstream.Close()
	upAddr := strings.TrimPrefix(upstream.URL, "http://")

	store := NewStore()
	p := NewProxy(store)
	if ok := p.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamManual, UpstreamAddr: upAddr}); !ok {
		t.Fatal("add target failed")
	}
	// strip the security header on the way back
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"})

	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	defer p.Stop()

	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_, _ = io.WriteString(c, "GET /page HTTP/1.1\r\nHost: shop.example.com\r\nConnection: close\r\n\r\n")

	resp, err := http.ReadResponse(bufio.NewReader(c), nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(string(body), "upstream-body:") {
		t.Fatalf("unexpected body: %q", body)
	}
	if resp.Header.Get("X-Frame-Options") != "" {
		t.Fatalf("match&replace did not strip X-Frame-Options: %v", resp.Header)
	}

	// verify a reverse flow was captured with proxying status
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && store.Len() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	flows := store.Snapshot()
	if len(flows) == 0 {
		t.Fatal("no flow captured")
	}
	f := flows[0]
	if f.Src != SrcReverse || f.TargetDomain != "shop.example.com" {
		t.Fatalf("flow not tagged reverse: src=%q target=%q", f.Src, f.TargetDomain)
	}
	tv := p.Targets.Snapshot()
	if len(tv) != 1 || tv[0].Status != StatusProxying || tv[0].Requests == 0 {
		t.Fatalf("target status not updated: %+v", tv)
	}
}

// TestManualInterceptForwardDrop verifies Hold blocks until Forward/Drop.
func TestManualInterceptForwardDrop(t *testing.T) {
	in := NewInterceptor()
	in.SetOn(true)

	// Forward with edit
	done := make(chan struct{})
	var gotEdited []byte
	var gotDrop bool
	go func() {
		gotEdited, gotDrop = in.Hold(&Held{Kind: HeldRequest, Raw: []byte("original")})
		close(done)
	}()
	waitQueue(t, in, 1)
	q := in.Queue()
	in.Forward(q[0].ID, []byte("edited"))
	<-done
	if gotDrop || string(gotEdited) != "edited" {
		t.Fatalf("forward: drop=%v edited=%q", gotDrop, gotEdited)
	}

	// Drop
	done2 := make(chan struct{})
	var dropRes bool
	go func() {
		_, dropRes = in.Hold(&Held{Kind: HeldRequest, Raw: []byte("x")})
		close(done2)
	}()
	waitQueue(t, in, 1)
	q = in.Queue()
	in.Drop(q[0].ID)
	<-done2
	if !dropRes {
		t.Fatal("drop: expected drop=true")
	}

	// Off returns immediately without holding
	in.SetOn(false)
	edited, drop := in.Hold(&Held{Kind: HeldRequest, Raw: []byte("passthrough")})
	if drop || string(edited) != "passthrough" {
		t.Fatalf("off passthrough failed: drop=%v edited=%q", drop, edited)
	}
}

func waitQueue(t *testing.T, in *Interceptor, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if in.Len() >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("queue did not reach %d", n)
}

func TestScopeAndDomainMatch(t *testing.T) {
	if !domainMatches("*.example.com", "a.example.com") {
		t.Error("wildcard should match subdomain")
	}
	if domainMatches("*.example.com", "example.com") {
		t.Error("wildcard should not match apex")
	}
	if !ValidDomain("example.com") || !ValidDomain("*.example.com") || ValidDomain("nodot") {
		t.Error("ValidDomain logic wrong")
	}

	s := NewScope()
	s.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"})
	if !s.InScope(&Flow{Host: "api.example.com"}) {
		t.Error("should be in scope")
	}
	if s.InScope(&Flow{Host: "other.org"}) {
		t.Error("should be out of scope")
	}
}

func TestMatchReplaceBody(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "secret", Replacement: "REDACTED"})
	out := m.ApplyBody(MRResponse, []byte("a secret value"))
	if string(out) != "a REDACTED value" {
		t.Fatalf("body replace failed: %q", out)
	}
	hdrs := m.ApplyHeaders(MRResponse, [][2]string{{"X-Frame-Options", "DENY"}})
	_ = hdrs
}

func TestStoreCapsFlowCount(t *testing.T) {
	s := NewStore()
	total := MaxFlows + 500
	for i := 0; i < total; i++ {
		s.Add(&Flow{Host: "h", ReqBody: make([]byte, 1024)})
	}
	if got := s.Len(); got != MaxFlows {
		t.Fatalf("Len()=%d, want capped at %d", got, MaxFlows)
	}
	meta := s.SnapshotMeta()
	if len(meta) != MaxFlows {
		t.Fatalf("SnapshotMeta len=%d, want %d", len(meta), MaxFlows)
	}
	first := meta[0]
	if first.ID <= 500 {
		t.Errorf("oldest flow not evicted, first ID=%d", first.ID)
	}
}

func TestSnapshotMetaDropsBodies(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{
		Host:        "h",
		ReqBody:     []byte("request-body"),
		RespBody:    []byte("response-body"),
		ReqHeaders:  [][2]string{{"A", "B"}},
		RespHeaders: [][2]string{{"C", "D"}},
		ReqSize:     12,
		RespSize:    13,
	})
	meta := s.SnapshotMeta()
	if len(meta) != 1 {
		t.Fatalf("want 1 flow, got %d", len(meta))
	}
	f := meta[0]
	if f.ReqBody != nil || f.RespBody != nil || f.ReqHeaders != nil || f.RespHeaders != nil {
		t.Error("SnapshotMeta must not carry bodies/headers")
	}
	if f.ReqSize != 12 || f.RespSize != 13 || f.Host != "h" {
		t.Errorf("metadata lost: %+v", f)
	}
}

func TestFindByIDReturnsClone(t *testing.T) {
	s := NewStore()
	added := s.Add(&Flow{Host: "h", ReqBody: []byte("body")})
	got := s.FindByID(added.ID)
	if got == nil {
		t.Fatal("FindByID returned nil for existing flow")
	}
	if got == added {
		t.Error("FindByID must return a clone, not the stored pointer")
	}
	got.ReqBody[0] = 'X'
	again := s.FindByID(added.ID)
	if again.ReqBody[0] == 'X' {
		t.Error("mutation of clone leaked into store")
	}
	if s.FindByID(999999) != nil {
		t.Error("FindByID must return nil for unknown id")
	}
}

func mkWSFrame(op byte, payload []byte, masked bool) []byte {
	var b bytes.Buffer
	b.WriteByte(0x80 | op)
	n := len(payload)
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}
	switch {
	case n < 126:
		b.WriteByte(maskBit | byte(n))
	case n < 1<<16:
		b.WriteByte(maskBit | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		b.Write(ext[:])
	default:
		b.WriteByte(maskBit | 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		b.Write(ext[:])
	}
	if masked {
		key := [4]byte{0x11, 0x22, 0x33, 0x44}
		b.Write(key[:])
		enc := append([]byte(nil), payload...)
		for i := range enc {
			enc[i] ^= key[i&3]
		}
		b.Write(enc)
		return b.Bytes()
	}
	b.Write(payload)
	return b.Bytes()
}

type errWriter struct {
	n   int
	max int
}

func (w *errWriter) Write(p []byte) (int, error) {
	w.n++
	if w.n > w.max {
		return 0, errors.New("write refused")
	}
	return len(p), nil
}

func TestPumpWS_LogsFramesAndForwardsRaw(t *testing.T) {
	p := NewProxy(NewStore())

	var stream bytes.Buffer
	stream.Write(mkWSFrame(0x1, []byte("hello"), true))
	stream.Write(mkWSFrame(0x0, []byte("cont"), true))
	stream.Write(mkWSFrame(0x2, bytes.Repeat([]byte("b"), 200), false))
	stream.Write(mkWSFrame(0x8, []byte{0x03, 0xe8}, false))
	stream.Write(mkWSFrame(0x1, []byte("after-close"), false))

	raw := append([]byte(nil), stream.Bytes()...)
	var out bytes.Buffer
	p.pumpWS(bufio.NewReader(bytes.NewReader(raw)), &out, 42, "wss://x/y", true)

	closeEnd := len(mkWSFrame(0x1, []byte("hello"), true)) +
		len(mkWSFrame(0x0, []byte("cont"), true)) +
		len(mkWSFrame(0x2, bytes.Repeat([]byte("b"), 200), false)) +
		len(mkWSFrame(0x8, []byte{0x03, 0xe8}, false))
	if !bytes.Equal(out.Bytes(), raw[:closeEnd]) {
		t.Fatalf("forwarded bytes mismatch: got %d want %d", out.Len(), closeEnd)
	}

	msgs := p.WS.Snapshot()
	if len(msgs) != 3 {
		t.Fatalf("want 3 logged frames (continuation skipped, close logged), got %d", len(msgs))
	}
	if msgs[0].Opcode != 0x1 || string(msgs[0].Payload) != "hello" {
		t.Errorf("frame 0: op=%x payload=%q", msgs[0].Opcode, msgs[0].Payload)
	}
	if !msgs[0].ToServer || msgs[0].FlowID != 42 || msgs[0].URL != "wss://x/y" {
		t.Errorf("frame 0 metadata wrong: %+v", msgs[0])
	}
	if msgs[1].Opcode != 0x2 || len(msgs[1].Payload) != 200 {
		t.Errorf("frame 1: op=%x len=%d", msgs[1].Opcode, len(msgs[1].Payload))
	}
	if msgs[2].Opcode != 0x8 {
		t.Errorf("frame 2 should be close, got %x", msgs[2].Opcode)
	}
}

func TestPumpWS_StopsOnReadError(t *testing.T) {
	p := NewProxy(NewStore())
	truncated := mkWSFrame(0x1, []byte("hello"), false)
	truncated = truncated[:len(truncated)-2]

	var out bytes.Buffer
	p.pumpWS(bufio.NewReader(bytes.NewReader(truncated)), &out, 1, "wss://x", false)
	if p.WS.Len() != 0 {
		t.Fatalf("truncated frame must not be logged, got %d", p.WS.Len())
	}
}

func TestPumpWS_StopsOnWriteError(t *testing.T) {
	p := NewProxy(NewStore())
	var stream bytes.Buffer
	stream.Write(mkWSFrame(0x1, []byte("one"), false))
	stream.Write(mkWSFrame(0x1, []byte("two"), false))
	stream.Write(mkWSFrame(0x1, []byte("three"), false))

	w := &errWriter{max: 1}
	p.pumpWS(bufio.NewReader(bytes.NewReader(stream.Bytes())), w, 7, "wss://x", false)
	if p.WS.Len() != 1 {
		t.Fatalf("want exactly 1 logged frame before the write error, got %d", p.WS.Len())
	}
}

func TestPumpWS_ExtendedLengthFrames(t *testing.T) {
	p := NewProxy(NewStore())
	big := bytes.Repeat([]byte("z"), 70000)
	var stream bytes.Buffer
	stream.Write(mkWSFrame(0x2, big, false))
	stream.Write(mkWSFrame(0x8, nil, false))

	var out bytes.Buffer
	p.pumpWS(bufio.NewReader(bytes.NewReader(stream.Bytes())), &out, 3, "wss://big", false)

	msgs := p.WS.Snapshot()
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	if len(msgs[0].Payload) != len(big) {
		t.Errorf("64-bit length frame payload truncated: %d", len(msgs[0].Payload))
	}
}

func TestResolveDoH_IPLiteralShortCircuits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := resolveDoH(ctx, "203.0.113.7"); got != "203.0.113.7" {
		t.Fatalf("IP literal must be returned as-is without any network call, got %q", got)
	}
	if got := resolveDoH(ctx, "2001:db8::1"); got != "2001:db8::1" {
		t.Fatalf("IPv6 literal must be returned as-is, got %q", got)
	}
}

func TestResolveDoH_InvalidHostYieldsEmpty(t *testing.T) {
	ctx := context.Background()
	if got := resolveDoH(ctx, "bad\x7fhost"); got != "" {
		t.Fatalf("unparsable request URL must yield empty, got %q", got)
	}
}

func TestResolveDoH_CancelledContextYieldsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := resolveDoH(ctx, "example.invalid"); got != "" {
		t.Fatalf("cancelled context must yield empty, got %q", got)
	}
}

func TestResolveUpstream_Manual(t *testing.T) {
	p := NewProxy(NewStore())

	if _, err := p.resolveUpstream(&Target{Upstream: UpstreamManual}, "h.example.com", "80"); err == nil {
		t.Fatal("empty manual address must be an error")
	}

	addr, err := p.resolveUpstream(&Target{Upstream: UpstreamManual, UpstreamAddr: "10.0.0.5"}, "h.example.com", "8443")
	if err != nil || addr != "10.0.0.5:8443" {
		t.Fatalf("bare manual addr should gain the request port: %q %v", addr, err)
	}

	addr, err = p.resolveUpstream(&Target{Upstream: UpstreamManual, UpstreamAddr: "10.0.0.5:9000"}, "h.example.com", "80")
	if err != nil || addr != "10.0.0.5:9000" {
		t.Fatalf("manual host:port should pass through: %q %v", addr, err)
	}
}

func TestResolveUpstream_AutoWithIPHost(t *testing.T) {
	p := NewProxy(NewStore())
	addr, err := p.resolveUpstream(&Target{Upstream: UpstreamAuto}, "198.51.100.9", "443")
	if err != nil || addr != "198.51.100.9:443" {
		t.Fatalf("auto+IP must skip DoH: %q %v", addr, err)
	}
}

func TestResolveUpstream_AutoResolveFailure(t *testing.T) {
	p := NewProxy(NewStore())
	_, err := p.resolveUpstream(&Target{Upstream: UpstreamAuto}, "bad\x7fhost", "443")
	if err == nil {
		t.Fatal("auto resolve failure must be reported")
	}
	if !strings.Contains(err.Error(), "DoH resolve failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReteTrustPool(t *testing.T) {
	setupTestConfigDir(t)
	if pool := ReteTrustPool(); pool != nil {
		t.Fatal("no CA on disk must yield a nil pool")
	}

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := ca.Save(MITMDir()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if pool := ReteTrustPool(); pool == nil {
		t.Fatal("a saved CA must produce a pool")
	}

	if err := os.WriteFile(CACertPath(MITMDir()), []byte("not a pem"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if pool := ReteTrustPool(); pool != nil {
		t.Fatal("garbage cert file must yield a nil pool")
	}
}

func TestLoadCA_KeyBlockVariants(t *testing.T) {
	dir := t.TempDir()
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := ca.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := os.WriteFile(CAKeyPath(dir), []byte("-----BEGIN NOPE-----\nAAAA\n-----END NOPE-----\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCA(dir); err == nil || !strings.Contains(err.Error(), "unsupported key block") {
		t.Fatalf("want unsupported key block error, got %v", err)
	}

	if err := os.WriteFile(CAKeyPath(dir), []byte("-----BEGIN RSA PRIVATE KEY-----\nAAAA\n-----END RSA PRIVATE KEY-----\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadCA(dir); err == nil || !strings.Contains(err.Error(), "parse key") {
		t.Fatalf("want parse key error, got %v", err)
	}

	if err := os.Remove(CAKeyPath(dir)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, err := LoadCA(dir); err == nil {
		t.Fatal("missing key file must error")
	}
}

func TestCASave_MkdirFailure(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := ca.Save(filepath.Join(blocker, "sub")); err == nil {
		t.Fatal("saving under a regular file must fail")
	}
}

func TestSaveConfig_MkdirFailure(t *testing.T) {
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	persist.SetConfigOverride(blocker)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	if err := SaveConfig(Config{BindAddr: "x"}); err == nil {
		t.Fatal("SaveConfig must report a failure when its directory cannot be created")
	}
}

func TestRules_SetIgnoresEmptyHost(t *testing.T) {
	r := &Rules{}
	r.Set("   ", HostRule{Delay: time.Second})
	if r.Len() != 0 {
		t.Fatal("blank host must not create an entry")
	}
	r.Set("Example.COM:8443", HostRule{Delay: 5 * time.Millisecond})
	if r.Len() != 1 {
		t.Fatalf("want 1 rule after set on a nil map, got %d", r.Len())
	}
	if _, ok := r.Get("example.com"); !ok {
		t.Fatal("host should be normalized to a bare lowercase host")
	}
}

func TestTargets_MarkOnUnknownDomainIsNoop(t *testing.T) {
	tg := NewTargets()
	tg.markRequest("nobody.example.com")
	tg.markError("nobody.example.com", "boom")
	if tg.Len() != 0 {
		t.Fatal("marking an unknown domain must not create entries")
	}
	tg.Update("nobody.example.com", func(*Target) { t.Error("edit must not run for an unknown domain") })
}

func reverseTarget(addr string) *Target {
	return &Target{
		Domain:       "rev.example.com",
		Upstream:     UpstreamManual,
		UpstreamAddr: addr,
		TLS:          TLSDecrypt,
	}
}

func newReverseRequest(t *testing.T, method, uri, body string) *http.Request {
	t.Helper()
	raw := method + " " + uri + " HTTP/1.1\r\nHost: rev.example.com\r\nX-Probe: yes\r\n"
	raw += "Content-Length: " + itoaLen(body) + "\r\n\r\n" + body
	req, err := http.ReadRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	return req
}

func itoaLen(s string) string {
	n := len(s)
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}

func runReverse(t *testing.T, p *Proxy, tg *Target, req *http.Request) *http.Response {
	t.Helper()
	cli, srv := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.handleReverseHTTP(srv, bufio.NewReader(srv), req, tg)
		_ = srv.Close()
	}()
	_ = cli.SetDeadline(time.Now().Add(10 * time.Second))
	resp, err := http.ReadResponse(bufio.NewReader(cli), req)
	if err != nil {
		_ = cli.Close()
		<-done
		t.Fatalf("ReadResponse: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	_ = cli.Close()
	<-done
	return resp
}

func TestReverseHTTP_SuccessRecordsFlow(t *testing.T) {
	var gotMethod, gotProbe, gotHost string
	var gotBody []byte
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotProbe, gotHost = r.Method, r.Header.Get("X-Probe"), r.Host
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("X-Up", "1")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("upstream-said-hi"))
	}))
	defer up.Close()

	p := NewProxy(NewStore())
	tg := reverseTarget(strings.TrimPrefix(up.URL, "http://"))
	p.Targets.Add(tg)

	resp := runReverse(t, p, tg, newReverseRequest(t, "POST", "/api/v1?q=1", "payload"))
	if resp.StatusCode != 201 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "upstream-said-hi" {
		t.Fatalf("body = %q", body)
	}
	if resp.Header.Get("X-Up") != "1" {
		t.Error("upstream response header not relayed")
	}
	if gotMethod != "POST" || gotProbe != "yes" || string(gotBody) != "payload" {
		t.Errorf("upstream saw method=%q probe=%q body=%q", gotMethod, gotProbe, gotBody)
	}
	if gotHost != "rev.example.com" {
		t.Errorf("upstream Host should stay the target domain, got %q", gotHost)
	}

	flows := p.Store.Snapshot()
	if len(flows) != 1 {
		t.Fatalf("want 1 flow, got %d", len(flows))
	}
	f := flows[0]
	if f.Src != SrcReverse || f.TargetDomain != "rev.example.com" {
		t.Errorf("flow source metadata wrong: %+v", f)
	}
	if f.StatusCode != 201 || string(f.RespBody) != "upstream-said-hi" {
		t.Errorf("flow response not captured: %d %q", f.StatusCode, f.RespBody)
	}
	if string(f.ReqBody) != "payload" || f.ReqSize != 7 {
		t.Errorf("flow request not captured: %q %d", f.ReqBody, f.ReqSize)
	}
	if f.Ended.IsZero() {
		t.Error("flow must be marked ended")
	}

	views := p.Targets.Snapshot()
	if len(views) != 1 || views[0].Status != StatusProxying || views[0].Requests != 1 {
		t.Errorf("target status not updated: %+v", views)
	}
}

func TestReverseHTTP_ResolveFailureMarksTargetError(t *testing.T) {
	p := NewProxy(NewStore())
	tg := reverseTarget("")
	p.Targets.Add(tg)

	resp := runReverse(t, p, tg, newReverseRequest(t, "GET", "/", ""))
	if resp.StatusCode != 502 {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	views := p.Targets.Snapshot()
	if len(views) != 1 || views[0].Status != StatusError || views[0].LastErr == "" {
		t.Errorf("target should be in error state: %+v", views)
	}
	flows := p.Store.Snapshot()
	if len(flows) != 1 || flows[0].StatusCode != 502 || flows[0].Error == "" {
		t.Errorf("flow should record the resolve failure: %+v", flows)
	}
}

func TestReverseHTTP_UpstreamDialFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	dead := l.Addr().String()
	_ = l.Close()

	p := NewProxy(NewStore())
	tg := reverseTarget(dead)
	p.Targets.Add(tg)

	resp := runReverse(t, p, tg, newReverseRequest(t, "GET", "/", ""))
	if resp.StatusCode != 502 {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	views := p.Targets.Snapshot()
	if len(views) != 1 || views[0].Status != StatusError {
		t.Errorf("target should be in error state: %+v", views)
	}
}

func TestReverseHTTP_ManualInterceptDropsRequest(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream must not be reached for a dropped request")
		w.WriteHeader(200)
	}))
	defer up.Close()

	p := NewProxy(NewStore())
	tg := reverseTarget(strings.TrimPrefix(up.URL, "http://"))
	p.Targets.Add(tg)
	p.Manual.SetOn(true)

	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if q := p.Manual.Queue(); len(q) > 0 {
				p.Manual.Drop(q[0].ID)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()

	resp := runReverse(t, p, tg, newReverseRequest(t, "GET", "/secret", ""))
	if resp.StatusCode != 403 {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	flows := p.Store.Snapshot()
	if len(flows) != 1 || flows[0].Status != "dropped" {
		t.Errorf("flow should be marked dropped: %+v", flows)
	}
}

func TestReverseHTTP_MatchReplaceRewritesRequestAndResponse(t *testing.T) {
	var sawPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawPath = r.URL.Path
		_, _ = w.Write([]byte("original-body"))
	}))
	defer up.Close()

	p := NewProxy(NewStore())
	tg := reverseTarget(strings.TrimPrefix(up.URL, "http://"))
	p.Targets.Add(tg)
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRFirstLine, Pattern: "/old", Replacement: "/new"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "original", Replacement: "rewritten"})

	resp := runReverse(t, p, tg, newReverseRequest(t, "GET", "/old", ""))
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "rewritten-body" {
		t.Fatalf("response body = %q, want rewritten-body", body)
	}
	if sawPath != "/new" {
		t.Fatalf("upstream path = %q, want /new", sawPath)
	}
}

func TestReverseHTTP_TargetDelayIsApplied(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()

	p := NewProxy(NewStore())
	tg := reverseTarget(strings.TrimPrefix(up.URL, "http://"))
	tg.Delay = 60 * time.Millisecond
	p.Targets.Add(tg)

	start := time.Now()
	resp := runReverse(t, p, tg, newReverseRequest(t, "GET", "/", ""))
	if elapsed := time.Since(start); elapsed < 50*time.Millisecond {
		t.Fatalf("target delay not applied: %v", elapsed)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestWSStore_EvictsBeyondCap(t *testing.T) {
	s := NewWSStore()
	total := maxWSMessages + 25
	for i := 0; i < total; i++ {
		s.Add(&WSMessage{Opcode: 0x1, Payload: []byte{byte(i)}})
	}
	if got := s.Len(); got != maxWSMessages {
		t.Fatalf("store should cap at %d, got %d", maxWSMessages, got)
	}
	snap := s.Snapshot()
	if snap[0].ID != uint64(total-maxWSMessages+1) {
		t.Errorf("oldest surviving ID = %d, want %d", snap[0].ID, total-maxWSMessages+1)
	}
	if last := snap[len(snap)-1]; last.ID != uint64(total) {
		t.Errorf("newest ID = %d, want %d", last.ID, total)
	}
}

func TestWSStore_ClearAndNotify(t *testing.T) {
	s := NewWSStore()
	var n int
	s.SetNotify(func() { n++ })
	s.Add(&WSMessage{Opcode: 0x1})
	if n != 1 {
		t.Fatalf("Add should emit once, got %d", n)
	}
	s.Clear()
	if n != 2 {
		t.Fatalf("Clear should emit, got %d", n)
	}
	if s.Len() != 0 {
		t.Fatal("Clear should empty the store")
	}
	s.SetNotify(nil)
	s.Add(&WSMessage{Opcode: 0x2})
	if n != 2 {
		t.Fatalf("a nil notify must not fire, got %d", n)
	}
}

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

	clientPool := x509.NewCertPool()
	clientPool.AddCert(ca.Cert)
	upstreamPool := x509.NewCertPool()
	upstreamPool.AddCert(upstream.Certificate())

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

func TestProxyHTTPCapture(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Test", "hello")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("echo:" + string(body)))
	}))
	defer upstream.Close()

	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Stop()

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}

	req, _ := http.NewRequest(http.MethodPost, upstream.URL+"/x", strings.NewReader("payload"))
	req.Header.Set("X-From", "rete")
	resp, err := cl.Do(req)
	if err != nil {
		t.Fatalf("client do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("status = %d, want 418", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "echo:payload" {
		t.Fatalf("body = %q", body)
	}
	if got := resp.Header.Get("X-Test"); got != "hello" {
		t.Fatalf("X-Test = %q", got)
	}

	deadline := time.Now().Add(time.Second)
	var flow *Flow
	for time.Now().Before(deadline) {
		if store.Len() > 0 {
			flow = store.At(0)
			if flow != nil && flow.StatusCode != 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if flow == nil {
		t.Fatalf("no flow captured")
	}
	if flow.Method != "POST" || flow.StatusCode != http.StatusTeapot {
		t.Fatalf("flow = %+v", flow)
	}
	if string(flow.ReqBody) != "payload" {
		t.Fatalf("captured req body = %q", flow.ReqBody)
	}
	if string(flow.RespBody) != "echo:payload" {
		t.Fatalf("captured resp body = %q", flow.RespBody)
	}
}

func TestProxyDirectHitNoLoop(t *testing.T) {
	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Stop()

	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Get("http://" + p.Addr() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want 421", resp.StatusCode)
	}

	proxyURL, _ := url.Parse("http://" + p.Addr())
	cl2 := &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
	resp2, err := cl2.Get("http://" + p.Addr() + "/anything")
	if err != nil {
		t.Fatalf("loop get: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("loop status = %d, want 421", resp2.StatusCode)
	}
}

func TestProxyConnectTunnelMarksEndedOnStop(t *testing.T) {

	upL, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upL.Close()
	go func() {
		for {
			c, err := upL.Accept()
			if err != nil {
				return
			}

			go func(c net.Conn) {
				_, _ = io.Copy(io.Discard, c)
				_ = c.Close()
			}(c)
		}
	}()

	store := NewStore()
	p := NewProxy(store)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}

	c, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	fmt.Fprintf(c, "CONNECT %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: test\r\nProxy-Connection: keep-alive\r\n\r\n", upL.Addr().String(), upL.Addr().String())
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		t.Fatalf("read CONNECT resp: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("CONNECT status = %d", resp.StatusCode)
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f := store.At(0)
		if f != nil && !f.Ended.IsZero() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	f := store.At(0)
	if f == nil {
		t.Fatal("flow not recorded")
	}
	if f.Ended.IsZero() {
		t.Fatalf("Ended must be stamped right after handshake; flow=%+v", f)
	}
	if f.TunnelClosed {
		t.Fatalf("tunnel must still be open at this point; flow=%+v", f)
	}

	stopDone := make(chan struct{})
	go func() { p.Stop(); close(stopDone) }()
	select {
	case <-stopDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s — connection not force-closed")
	}
	f = store.At(0)
	if !f.TunnelClosed {
		t.Fatalf("tunnel should be marked closed after Stop: %+v", f)
	}
}

func TestProxyStartStop(t *testing.T) {
	p := NewProxy(NewStore())
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !p.Running() {
		t.Fatal("expected running")
	}
	if err := p.Start("127.0.0.1:0"); err == nil {
		t.Fatal("expected error on second start")
	}
	p.Stop()
	if p.Running() {
		t.Fatal("expected stopped")
	}
}

func TestMatchReplaceConcurrentApplyNoRace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N", Type: MRRequest})
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: false, Pattern: "secret", Replacement: "***", Type: MRRequest})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				mr.ApplyBody(MRRequest, []byte("id=123 secret=abc"))
			}
		}()
	}
	wg.Wait()
}

func TestMatchReplaceConcurrentApplyAndUpdateNoRace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N", Type: MRRequest})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					mr.ApplyBody(MRRequest, []byte("id=123"))
				}
			}
		}()
	}

	patterns := []string{`\d+`, `[a-z]+`, `id=\d+`, `x`}
	for i := 0; i < 200; i++ {
		p := patterns[i%len(patterns)]
		mr.Update(0, func(r *MatchReplaceRule) { r.Pattern = p })
	}
	close(stop)
	wg.Wait()
}

func TestScopeConcurrentInScopeNoRace(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^ex.*\.com$`, IsRegex: true})
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health", IsRegex: false})

	flows := []*Flow{
		{Host: "example.com", Path: "/api"},
		{Host: "other.org", Path: "/api"},
		{Host: "example.com", Path: "/health"},
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				sc.InScope(flows[(n+j)%len(flows)])
			}
		}(i)
	}
	wg.Wait()
}

func TestScopeConcurrentInScopeAndUpdateNoRace(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example", IsRegex: true})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					sc.InScope(&Flow{Host: "example.com", Path: "/api"})
				}
			}
		}()
	}
	for i := 0; i < 200; i++ {
		sc.Update(0, func(r *ScopeRule) { r.IsRegex = i%2 == 0 })
	}
	close(stop)
	wg.Wait()
}

func TestTargetsConcurrentMatchAndUpdateNoRace(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if m, ok := tg.Match("example.com"); ok {
						_ = m.UpstreamAddr
						_ = m.Upstream
						_ = m.TLS
						_ = m.Delay
					}
					tg.markRequest("example.com")
				}
			}
		}()
	}
	for i := 0; i < 300; i++ {
		addr := "2.2.2.2:90"
		if i%2 == 0 {
			addr = "3.3.3.3:70"
		}
		tg.Update("example.com", func(x *Target) { x.UpstreamAddr = addr })
	}
	close(stop)
	wg.Wait()
}

func targetByDomain(t *testing.T, s *UIState, domain string) Target {
	t.Helper()
	for _, v := range s.Proxy.Targets.Snapshot() {
		if v.Domain == domain {
			return v.Target
		}
	}
	t.Fatalf("target %q not found", domain)
	return Target{}
}

func TestTargetExpandPreservesUpstreamAddr(t *testing.T) {
	cases := []struct {
		name     string
		upstream string
		addr     string
	}{
		{"manual upstream", UpstreamManual, "127.0.0.1:8080"},
		{"auto upstream", UpstreamAuto, "10.0.0.5:9000"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 800))
			s := rig.s
			s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: c.upstream, UpstreamAddr: c.addr})
			row := &TargetRow{}
			s.TargetRows["example.com"] = row

			row.Expand.Click()
			s.targetsEvents(rig.gtx())

			if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != c.addr {
				t.Errorf("UpstreamAddr = %q after expanding the row, want %q preserved", got, c.addr)
			}
			if !row.Expanded {
				t.Error("row did not expand")
			}
			if row.AddrInput.Text() != c.addr {
				t.Errorf("AddrInput = %q, want the row seeded with %q", row.AddrInput.Text(), c.addr)
			}
		})
	}
}

func TestTargetEditAddrStillApplies(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	row := &TargetRow{}
	s.TargetRows["example.com"] = row

	row.Expand.Click()
	s.targetsEvents(rig.gtx())

	row.AddrInput.SetText("2.2.2.2:90")
	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "2.2.2.2:90" {
		t.Errorf("UpstreamAddr = %q, want the edited value applied", got)
	}
}

func TestTargetClearAddrStillApplies(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	row := &TargetRow{}
	s.TargetRows["example.com"] = row

	row.Expand.Click()
	s.targetsEvents(rig.gtx())

	row.AddrInput.SetText("")
	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "" {
		t.Errorf("UpstreamAddr = %q, want the user's explicit clear to apply", got)
	}
}

func TestTargetCollapsedRowDoesNotTouchAddr(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 800))
	s := rig.s
	s.Proxy.Targets.Add(&Target{Domain: "example.com", Upstream: UpstreamManual, UpstreamAddr: "1.1.1.1:80"})
	s.TargetRows["example.com"] = &TargetRow{}

	s.targetsEvents(rig.gtx())

	if got := targetByDomain(t, s, "example.com").UpstreamAddr; got != "1.1.1.1:80" {
		t.Errorf("UpstreamAddr = %q, want untouched while collapsed", got)
	}
}

func TestIsNoiseMatchesExtensionSuffixOnly(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/api/users.json", false},
		{"/v1/data.json", false},
		{"/api/list.jsonp", false},
		{"/graphql.json?q=1", false},
		{"/assets/app.js", true},
		{"/assets/app.js?v=3", true},
		{"/assets/app.js#frag", true},
		{"/style.css", true},
		{"/logo.png", true},
		{"/fonts/x.woff2", true},
		{"/api/jsonify", false},
		{"/csset", false},
		{"/a/.js/b", false},
		{"/", false},
		{"", false},
	}
	for _, c := range cases {
		f := &Flow{Path: c.path}
		if got := isNoise(f); got != c.want {
			t.Errorf("isNoise(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestAsCurlQuotesSingleQuotes(t *testing.T) {
	f := &Flow{
		Method:     "POST",
		URL:        "https://example.com/a?q=it's",
		ReqHeaders: [][2]string{{"X-Note", "it's here"}},
		ReqBody:    []byte(`{"msg":"it's"}`),
	}
	out := asCurl(f)
	if strings.Contains(out, "'it's") {
		t.Errorf("unescaped single quote leaked into curl output:\n%s", out)
	}
	for _, want := range []string{`'\''`} {
		if !strings.Contains(out, want) {
			t.Errorf("expected escaped quote %q in output:\n%s", want, out)
		}
	}
}

func TestAsCurlPlainValues(t *testing.T) {
	f := &Flow{
		Method:     "GET",
		URL:        "https://example.com/a",
		ReqHeaders: [][2]string{{"Accept", "application/json"}, {"Host", "example.com"}},
	}
	out := asCurl(f)
	if !strings.Contains(out, "curl -X GET 'https://example.com/a'") {
		t.Errorf("unexpected curl line:\n%s", out)
	}
	if strings.Contains(out, "Host:") {
		t.Errorf("host header must be omitted:\n%s", out)
	}
	if !strings.Contains(out, "-H 'Accept: application/json'") {
		t.Errorf("header missing:\n%s", out)
	}
}

func TestMatchReplaceInvalidRegexDoesNotFallBackToLiteral(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{
		Enabled:     true,
		IsRegex:     true,
		Pattern:     "([unclosed",
		Replacement: "x",
	})
	rules := mr.rules
	if len(rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1", len(rules))
	}
	in := "([unclosed here"
	if got := rules[0].applyString(in); got != in {
		t.Errorf("applyString with an invalid regex = %q, want the input unchanged", got)
	}
}

func TestMatchReplaceValidRegexApplies(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	if got := mr.rules[0].applyString("a1b22c"); got != "aNbNc" {
		t.Errorf("applyString = %q, want aNbNc", got)
	}
}

func TestMatchReplaceLiteralStillWorks(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: false, Pattern: "foo", Replacement: "bar"})
	if got := mr.rules[0].applyString("a foo b"); got != "a bar b" {
		t.Errorf("applyString = %q, want 'a bar b'", got)
	}
}

func TestMatchReplaceUpdateRecompiles(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	mr.Update(0, func(r *MatchReplaceRule) { r.Pattern = `[a-z]+` })
	if got := mr.rules[0].applyString("abc123"); got != "N123" {
		t.Errorf("applyString after Update = %q, want N123", got)
	}
}

func TestMatchReplaceRuleCompiledEagerly(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, IsRegex: true, Pattern: `\d+`, Replacement: "N"})
	if mr.rules[0].re == nil {
		t.Error("Add must compile the regex eagerly so read paths never write under RLock")
	}
}

func TestScopeRuleCompiledEagerly(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^ex.*\.com$`, IsRegex: true})
	if sc.rules[0].re == nil {
		t.Error("Add must compile the scope regex eagerly")
	}
	if !sc.InScope(&Flow{Host: "example.com"}) {
		t.Error("host should be in scope")
	}
	if sc.InScope(&Flow{Host: "other.org"}) {
		t.Error("host should not be in scope")
	}
}

func TestScopeInvalidRegexNeverMatches(t *testing.T) {
	sc := NewScope()
	sc.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "([bad", IsRegex: true})
	if sc.InScope(&Flow{Host: "([bad"}) {
		t.Error("an invalid scope regex must not fall back to a literal match")
	}
}

func TestSortClicksCoversEveryHistColumn(t *testing.T) {
	var s UIState
	if len(s.SortClicks) != len(histCols) {
		t.Errorf("len(SortClicks) = %d, len(histCols) = %d; every column needs a clickable",
			len(s.SortClicks), len(histCols))
	}
}

func TestApplyHeadersLeavesCallerSliceIntact(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "Cookie", Replacement: ""})

	in := [][2]string{{"Host", "example.com"}, {"Cookie", "secret=1"}, {"Accept", "*/*"}}
	before := append([][2]string(nil), in...)

	out := mr.ApplyHeaders(MRRequest, in)
	for i := range before {
		if in[i] != before[i] {
			t.Fatalf("caller's slice was rewritten in place: %v, want %v", in, before)
		}
	}
	if len(out) != 2 || out[0][0] != "Host" || out[1][0] != "Accept" {
		t.Errorf("out = %v, want Cookie removed", out)
	}
}

func TestApplyHeadersEmptyPatternIsSkipped(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "", Replacement: "x"})

	out := mr.ApplyHeaders(MRRequest, [][2]string{{"Host", "h"}})
	if len(out) != 1 || out[0][0] != "Host" {
		t.Fatalf("out = %v, want the input unchanged", out)
	}
	for _, h := range out {
		if h[0] == "" {
			t.Errorf("a rule naming no header injected a nameless header: %v", out)
		}
	}
}

func TestApplyHeadersCollapsesDuplicatesOnReplace(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Set-Cookie", Replacement: "a=1"})

	out := mr.ApplyHeaders(MRResponse, [][2]string{
		{"Set-Cookie", "a=old"},
		{"Content-Type", "text/html"},
		{"set-cookie", "b=old"},
	})
	seen := 0
	for _, h := range out {
		if strings.EqualFold(h[0], "Set-Cookie") {
			seen++
			if h[1] != "a=1" {
				t.Errorf("Set-Cookie = %q, want a=1", h[1])
			}
		}
	}
	if seen != 1 {
		t.Errorf("out = %v, want exactly one Set-Cookie header, got %d", out, seen)
	}
	if len(out) != 2 {
		t.Errorf("out = %v, want the untouched Content-Type kept alongside", out)
	}
}

func TestApplyHeadersDeletesEveryDuplicate(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Set-Cookie", Replacement: ""})

	out := mr.ApplyHeaders(MRResponse, [][2]string{
		{"Set-Cookie", "a=1"},
		{"set-cookie", "b=2"},
		{"Content-Type", "text/html"},
	})
	if len(out) != 1 || out[0][0] != "Content-Type" {
		t.Errorf("out = %v, want every Set-Cookie removed", out)
	}
}

func TestApplyHeadersAddsWhenAbsent(t *testing.T) {
	mr := NewMatchReplace()
	mr.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "X-Trace", Replacement: "1"})

	out := mr.ApplyHeaders(MRRequest, [][2]string{{"Host", "h"}})
	if len(out) != 2 || out[1][0] != "X-Trace" || out[1][1] != "1" {
		t.Errorf("out = %v, want X-Trace appended", out)
	}
}

func TestFlowStoreCapsRetainedBytes(t *testing.T) {
	s := NewStore()
	const chunk = 8 << 20
	for i := 0; i < 64; i++ {
		s.Add(&Flow{Host: "h", RespBody: make([]byte, chunk)})
	}
	var total int64
	for _, f := range s.Snapshot() {
		total += int64(len(f.ReqBody) + len(f.RespBody))
	}
	if total > MaxFlowBytes {
		t.Fatalf("retained %d bytes, cap is %d", total, MaxFlowBytes)
	}
	if s.Len() == 0 {
		t.Fatal("byte cap must not empty the store")
	}
}

func TestWSStoreCapsRetainedBytes(t *testing.T) {
	s := NewWSStore()
	const chunk = 4 << 20
	for i := 0; i < 64; i++ {
		s.Add(&WSMessage{Payload: make([]byte, chunk)})
	}
	var total int
	for _, m := range s.Snapshot() {
		total += len(m.Payload)
	}
	if int64(total) > maxWSBytes {
		t.Fatalf("retained %d bytes, cap is %d", total, maxWSBytes)
	}
	if s.Len() == 0 {
		t.Fatal("byte cap must not empty the store")
	}
}

func TestStoreDropsReferencesBeyondLength(t *testing.T) {
	s := NewStore()
	for i := 0; i < MaxFlows+16; i++ {
		s.Add(&Flow{Host: "h", RespBody: make([]byte, 1024)})
	}
	s.mu.RLock()
	tail := s.flows[len(s.flows):cap(s.flows)]
	s.mu.RUnlock()
	for i, f := range tail {
		if f != nil {
			t.Fatalf("trimmed slot %d still references flow %d", i, f.ID)
		}
	}
}

func TestSnapshotMetaCachedUntilStoreChanges(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a", RespBody: []byte("body")})
	first := s.SnapshotMeta()
	if &first[0] != &s.SnapshotMeta()[0] {
		t.Fatal("unchanged store must reuse the meta snapshot")
	}
	if first[0].RespBody != nil || first[0].RespHeaders != nil {
		t.Fatal("meta snapshot must not carry bodies or headers")
	}
	s.Add(&Flow{Host: "b"})
	if len(s.SnapshotMeta()) != 2 {
		t.Fatal("meta snapshot must refresh after Add")
	}
	s.SetAnnotation(1, "red", "note")
	if got := s.SnapshotMeta()[0].Highlight; got != "red" {
		t.Fatalf("meta snapshot stale after SetAnnotation: %q", got)
	}
}

func TestFindByIDSharesFinishedBodies(t *testing.T) {
	s := NewStore()
	live := s.Add(&Flow{Host: "live", RespBody: []byte("partial")})
	got := s.FindByID(live.ID)
	if &got.RespBody[0] == &live.RespBody[0] {
		t.Fatal("a live flow must be copied, the proxy may still write to it")
	}

	s.Update(live, func() { live.Ended = live.Started.Add(1) })
	got = s.FindByID(live.ID)
	if &got.RespBody[0] != &live.RespBody[0] {
		t.Fatal("a finished flow should share its body instead of copying per frame")
	}
}

func TestSplitPaneLinesPreservesContent(t *testing.T) {
	cases := []string{
		"short\nlines\nhere",
		strings.Repeat("x", paneLineChunk*3+7),
		strings.Repeat("привет", paneLineChunk),
		"head\n" + strings.Repeat("y", paneLineChunk+1) + "\ntail",
	}
	for _, txt := range cases {
		lines := splitPaneLines(txt)
		for i, ln := range lines {
			if len(ln) > paneLineChunk {
				t.Fatalf("line %d is %d bytes, over the %d chunk", i, len(ln), paneLineChunk)
			}
			if !utf8.ValidString(ln) {
				t.Fatalf("line %d split mid-rune", i)
			}
		}
		var b strings.Builder
		for i, ln := range strings.Split(txt, "\n") {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(ln)
		}
		if joinPaneLines(lines, txt) != b.String() {
			t.Fatalf("chunking lost content for a %d byte input", len(txt))
		}
	}
}

// joinPaneLines rebuilds the original text: chunks of one logical line join
// with nothing, separate logical lines with a newline.
func joinPaneLines(lines []string, orig string) string {
	want := strings.Split(orig, "\n")
	var b strings.Builder
	li := 0
	for i, w := range want {
		if i > 0 {
			b.WriteString("\n")
		}
		n := 0
		for n < len(w) && li < len(lines) {
			b.WriteString(lines[li])
			n += len(lines[li])
			li++
		}
		if len(w) == 0 && li < len(lines) && lines[li] == "" {
			li++
		}
	}
	return b.String()
}

func TestWSPreviewIsBounded(t *testing.T) {
	payload := []byte(strings.Repeat("ab\n", 1<<16))
	got := wsPreview(payload)
	if n := utf8.RuneCountInString(got); n != wsPreviewRunes {
		t.Fatalf("preview is %d runes, want %d", n, wsPreviewRunes)
	}
	if strings.Contains(got, "\n") {
		t.Fatal("preview must fold newlines into spaces")
	}
	if got := wsPreview([]byte("привет мир")); got != "привет мир" {
		t.Fatalf("short multibyte preview = %q", got)
	}
}

func TestWSMatchesKeepsFilterSemantics(t *testing.T) {
	m := &WSMessage{URL: "wss://Example.com/socket", Opcode: 0x1, Payload: []byte("Hello WORLD")}
	oldMatch := func(q string) bool {
		hay := strings.ToLower(m.URL + " " + string(m.Payload) + " " + WSOpcodeName(m.Opcode))
		return strings.Contains(hay, q)
	}
	for _, q := range []string{
		"", "example", "socket", "world", "hello world", "text",
		"socket hello", "d text", "missing", "привет",
	} {
		if got, want := wsMatches(m, q), oldMatch(q); got != want {
			t.Errorf("wsMatches(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestDelayConn_AppliesDelayOnce(t *testing.T) {
	r := NewRules()
	r.Set("slow.example.com", HostRule{Delay: 40 * time.Millisecond})

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() {
		buf := make([]byte, 4)
		_, _ = io.ReadFull(server, buf)
		_, _ = server.Write([]byte("pong"))
	}()

	dc := &delayConn{Conn: client, rules: r, host: "slow.example.com"}
	start := time.Now()
	if _, err := dc.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	first := time.Since(start)
	if first < 30*time.Millisecond {
		t.Errorf("the first write must apply the configured delay, took %v", first)
	}

	buf := make([]byte, 4)
	start = time.Now()
	if _, err := io.ReadFull(dc, buf); err != nil {
		t.Fatal(err)
	}
	if second := time.Since(start); second > 30*time.Millisecond {
		t.Errorf("the delay must apply only once, second op took %v", second)
	}
	if string(buf) != "pong" {
		t.Errorf("payload = %q", buf)
	}
}

func TestDelayConn_NoRuleNoDelay(t *testing.T) {
	r := NewRules()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	go func() { _, _ = io.ReadFull(server, make([]byte, 2)) }()

	dc := &delayConn{Conn: client, rules: r, host: "fast.example.com"}
	start := time.Now()
	if _, err := dc.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > 30*time.Millisecond {
		t.Errorf("an unconfigured host must not be delayed, took %v", took)
	}
}

func wsFrame(opcode byte, payload []byte, mask bool) []byte {
	var b bytes.Buffer
	b.WriteByte(0x80 | opcode)
	n := len(payload)
	maskBit := byte(0)
	if mask {
		maskBit = 0x80
	}
	switch {
	case n < 126:
		b.WriteByte(maskBit | byte(n))
	case n < 65536:
		b.WriteByte(maskBit | 126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(n))
		b.Write(ext[:])
	default:
		b.WriteByte(maskBit | 127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(n))
		b.Write(ext[:])
	}
	key := [4]byte{0xAA, 0xBB, 0xCC, 0xDD}
	if mask {
		b.Write(key[:])
		for i, c := range payload {
			b.WriteByte(c ^ key[i%4])
		}
		return b.Bytes()
	}
	b.Write(payload)
	return b.Bytes()
}

func TestReadWSFrame(t *testing.T) {
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
		mask    bool
	}{
		{"short-text", 0x1, []byte("hello"), false},
		{"short-masked", 0x1, []byte("hello"), true},
		{"binary", 0x2, []byte{0, 1, 2, 3}, false},
		{"empty", 0x9, nil, false},
		{"medium", 0x1, bytes.Repeat([]byte("x"), 300), false},
		{"medium-masked", 0x1, bytes.Repeat([]byte("y"), 300), true},
		{"large", 0x2, bytes.Repeat([]byte("z"), 70000), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frame := wsFrame(c.opcode, c.payload, c.mask)
			r := bufio.NewReader(bytes.NewReader(frame))
			op, payload, raw, err := readWSFrame(r)
			if err != nil {
				t.Fatalf("readWSFrame: %v", err)
			}
			if op != c.opcode {
				t.Errorf("opcode = %#x, want %#x", op, c.opcode)
			}
			if !bytes.Equal(payload, c.payload) && !(len(payload) == 0 && len(c.payload) == 0) {
				t.Errorf("payload len = %d, want %d", len(payload), len(c.payload))
			}
			if !bytes.Equal(raw, frame) {
				t.Errorf("raw must reproduce the wire bytes exactly (%d vs %d)", len(raw), len(frame))
			}
		})
	}
}

func TestReadWSFrame_Truncated(t *testing.T) {
	full := wsFrame(0x1, []byte("hello"), false)
	for n := 0; n < len(full); n++ {
		r := bufio.NewReader(bytes.NewReader(full[:n]))
		if _, _, _, err := readWSFrame(r); err == nil {
			t.Errorf("truncated frame of %d bytes must error", n)
		}
	}
}

func TestReadWSFrame_RejectsOversizeLength(t *testing.T) {
	var b bytes.Buffer
	b.WriteByte(0x82)
	b.WriteByte(127)
	var ext [8]byte
	binary.BigEndian.PutUint64(ext[:], 1<<40)
	b.Write(ext[:])
	r := bufio.NewReader(bytes.NewReader(b.Bytes()))
	if _, _, _, err := readWSFrame(r); err == nil {
		t.Error("an absurd frame length must be rejected")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProcessRequest_MatchReplaceOnly(t *testing.T) {
	p := NewProxy(NewStore())
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "User-Agent", Replacement: "rete"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRBody, Pattern: "old", Replacement: "new"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRFirstLine, Pattern: "/v1", Replacement: "/v2"})

	f := &Flow{ID: 1, Host: "example.com", URL: "https://example.com/v1/x"}
	method, uri, headers, body, drop := p.processRequest(f, "POST", "/v1/x", "HTTP/1.1",
		[][2]string{{"User-Agent", "curl"}}, []byte("old value"), true)

	if drop {
		t.Fatal("nothing should be dropped without manual interception")
	}
	if method != "POST" || uri != "/v2/x" {
		t.Errorf("method/uri = %q %q", method, uri)
	}
	if headerVal(headers, "User-Agent") != "rete" {
		t.Errorf("headers = %+v", headers)
	}
	if string(body) != "new value" {
		t.Errorf("body = %q", body)
	}
}

func TestProcessResponse_MatchReplaceOnly(t *testing.T) {
	p := NewProxy(NewStore())
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "secret", Replacement: "***"})
	p.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRFirstLine, Pattern: "200 OK", Replacement: "201 Created"})

	f := &Flow{ID: 1, Host: "example.com"}
	status, headers, body, drop := p.processResponse(f, "200 OK", "HTTP/1.1",
		[][2]string{{"X-Frame-Options", "DENY"}, {"Server", "nginx"}}, []byte("a secret b"), true)

	if drop {
		t.Fatal("nothing should be dropped without manual interception")
	}
	if status != "201 Created" {
		t.Errorf("status = %q", status)
	}
	if headerVal(headers, "X-Frame-Options") != "" {
		t.Errorf("X-Frame-Options was not stripped: %+v", headers)
	}
	if headerVal(headers, "Server") != "nginx" {
		t.Errorf("other headers must survive: %+v", headers)
	}
	if string(body) != "a *** b" {
		t.Errorf("body = %q", body)
	}
}

func TestProcessRequest_ManualForwardEdit(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	f := &Flow{ID: 7, Host: "example.com", URL: "https://example.com/a"}
	type result struct {
		method, uri string
		body        []byte
		drop        bool
	}
	res := make(chan result, 1)
	go func() {
		m, u, _, b, d := p.processRequest(f, "GET", "/a", "HTTP/1.1",
			[][2]string{{"Host", "example.com"}}, nil, true)
		res <- result{m, u, b, d}
	}()

	waitQueue(t, p.Manual, 1)
	q := p.Manual.Queue()
	if q[0].FlowID != 7 || q[0].Kind != HeldRequest {
		t.Fatalf("held message wrong: %+v", q[0])
	}
	if !strings.HasPrefix(string(q[0].Raw), "GET /a HTTP/1.1\r\n") {
		t.Errorf("held raw = %q", q[0].Raw)
	}
	p.Manual.Forward(q[0].ID, []byte("PUT /edited HTTP/1.1\r\nHost: example.com\r\nContent-Length: 8\r\n\r\nnew body"))

	got := <-res
	if got.drop {
		t.Fatal("forward must not drop")
	}
	if got.method != "PUT" || got.uri != "/edited" || string(got.body) != "new body" {
		t.Errorf("edits not applied: %+v", got)
	}
}

func TestProcessRequest_ManualDrop(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	dropped := make(chan bool, 1)
	go func() {
		_, _, _, _, d := p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, true)
		dropped <- d
	}()
	waitQueue(t, p.Manual, 1)
	p.Manual.Drop(p.Manual.Queue()[0].ID)
	if !<-dropped {
		t.Error("Drop must propagate to processRequest")
	}
}

func TestProcessRequest_SkippedWhenOutOfScope(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	done := make(chan struct{})
	go func() {
		p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("an out-of-scope request must not be held")
	}
}

func TestProcessResponse_SkippedWhenResponsesDisabled(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	p.Manual.SetInterceptResponses(false)
	defer p.Manual.SetOn(false)

	done := make(chan struct{})
	go func() {
		p.processResponse(&Flow{ID: 1, Host: "x"}, "200 OK", "HTTP/1.1", nil, nil, true)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("responses must not be held when response interception is off")
	}
}

func TestProcessResponse_ManualForward(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	p.Manual.SetInterceptResponses(true)
	defer p.Manual.SetOn(false)

	type result struct {
		status string
		body   []byte
	}
	res := make(chan result, 1)
	go func() {
		st, _, b, _ := p.processResponse(&Flow{ID: 3, Host: "x"}, "200 OK", "HTTP/1.1",
			[][2]string{{"Content-Type", "text/plain"}}, []byte("orig"), true)
		res <- result{st, b}
	}()
	waitQueue(t, p.Manual, 1)
	q := p.Manual.Queue()
	if q[0].Kind != HeldResponse {
		t.Fatalf("kind = %q", q[0].Kind)
	}
	p.Manual.Forward(q[0].ID, []byte("HTTP/1.1 404 Not Found\r\nContent-Length: 5\r\n\r\nedits"))

	got := <-res
	if got.status != "404 Not Found" || string(got.body) != "edits" {
		t.Errorf("edits not applied: %+v", got)
	}
}

func TestProcessRequest_UnparseableEditIsIgnored(t *testing.T) {
	p := NewProxy(NewStore())
	p.Manual.SetOn(true)
	defer p.Manual.SetOn(false)

	res := make(chan string, 1)
	go func() {
		m, _, _, _, _ := p.processRequest(&Flow{ID: 1, Host: "x"}, "GET", "/", "HTTP/1.1", nil, nil, true)
		res <- m
	}()
	waitQueue(t, p.Manual, 1)
	p.Manual.Forward(p.Manual.Queue()[0].ID, []byte("total garbage"))
	if got := <-res; got != "GET" {
		t.Errorf("an unparseable edit must leave the original in place, got %q", got)
	}
}

func TestInterceptor_DrainAllOnOff(t *testing.T) {
	in := NewInterceptor()
	in.SetOn(true)
	done := make(chan bool, 1)
	go func() {
		_, drop := in.Hold(&Held{Kind: HeldRequest, Raw: []byte("keep")})
		done <- drop
	}()
	waitQueue(t, in, 1)
	in.SetOn(false)
	select {
	case drop := <-done:
		if drop {
			t.Error("draining on shutdown must forward, not drop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SetOn(false) did not release the held message")
	}
	if in.Len() != 0 {
		t.Errorf("queue must be empty after draining, len=%d", in.Len())
	}
}

func TestInterceptor_ResolveUnknownIDIsNoop(t *testing.T) {
	in := NewInterceptor()
	in.Forward(9999, []byte("x"))
	in.Drop(9999)
	if in.Len() != 0 {
		t.Error("resolving an unknown ID must not change the queue")
	}
}

func TestInterceptor_ForwardNilKeepsOriginal(t *testing.T) {
	in := NewInterceptor()
	in.SetOn(true)
	defer in.SetOn(false)
	res := make(chan []byte, 1)
	go func() {
		edited, _ := in.Hold(&Held{Kind: HeldRequest, Raw: []byte("original")})
		res <- edited
	}()
	waitQueue(t, in, 1)
	in.Forward(in.Queue()[0].ID, nil)
	if got := string(<-res); got != "original" {
		t.Errorf("a nil edit must forward the original, got %q", got)
	}
}

func TestExportCA_NoCA(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	rig.s.exportCA(false)
	if rig.s.CABanner != "Generate a CA first" {
		t.Errorf("banner = %q", rig.s.CABanner)
	}
}

func TestUILayout_StatusBarVariants(t *testing.T) {
	banners := []string{
		"", "Proxy listening on 127.0.0.1:8080",
		"Start failed: address in use",
		"Administrator privileges required",
		"something neutral",
	}
	for _, b := range banners {
		rig := newUIRig(t, image.Pt(1200, 700))
		rig.s.StatusBanner = b
		if d := rig.frames(2); d.Size.Y <= 0 {
			t.Errorf("banner %q produced no dimensions", b)
		}
	}
}

func TestUILayout_SidebarHelpAndGuides(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTLSOpen = true
	rig.s.HelpOpen = true
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("the import guide produced no dimensions")
	}

	rig.s.HelpBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.HelpOpen {
		t.Error("the help button must toggle the guide closed")
	}
	rig.s.HelpBtn.Click()
	rig.sidebarFrames(2)
	if !rig.s.HelpOpen {
		t.Error("the help button must toggle the guide open")
	}
}

func TestUILayout_SidebarWithCA(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	rig.s.Proxy.SetCA(ca)
	rig.s.SecTLSOpen = true
	rig.s.CABanner = "CA generated"
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("the TLS section with a CA produced no dimensions")
	}
}

func TestUILayout_MRAndScopeRowVariants(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	for _, typ := range []string{MRRequest, MRResponse} {
		for _, area := range []string{MRFirstLine, MRHeader, MRBody} {
			rig.s.Proxy.MR.Add(MatchReplaceRule{
				Enabled: area == MRBody, Type: typ, Area: area,
				Pattern: "p", Replacement: "r", IsRegex: area == MRHeader, Comment: "note",
			})
		}
	}
	for _, kind := range []string{ScopeInclude, ScopeExclude} {
		for _, field := range []string{"host", "protocol", "port", "path"} {
			rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: kind == ScopeInclude, Kind: kind, Field: field, Pattern: "x"})
		}
	}
	if d := rig.sidebarFrames(3); d.Size.Y <= 0 {
		t.Fatal("populated MR/scope sections produced no dimensions")
	}
}

func TestShortFingerprint(t *testing.T) {
	if got := shortFingerprint(""); got != "" {
		t.Errorf("empty: got %q", got)
	}
	short := "ab:cd:ef"
	if got := shortFingerprint(short); got != short {
		t.Errorf("short passthrough: got %q want %q", got, short)
	}

	bd := strings.Repeat("a", 17)
	if got := shortFingerprint(bd); got != bd {
		t.Errorf("len17 passthrough: got %q", got)
	}

	in := "0123456789abcdef00"
	got := shortFingerprint(in)
	if !strings.Contains(got, "…") {
		t.Errorf("expected ellipsis in %q", got)
	}
	if !strings.HasPrefix(got, in[:8]) || !strings.HasSuffix(got, in[len(in)-8:]) {
		t.Errorf("expected first 8 and last 8 chars in %q", got)
	}
}

func TestGenLabel(t *testing.T) {
	if genLabel(nil) != "Generate CA" {
		t.Errorf("nil CA should give Generate CA")
	}

	ca, err := GenerateCA()
	if err != nil {
		t.Skipf("GenerateCA failed: %v", err)
	}
	if genLabel(ca) != "Regenerate" {
		t.Errorf("non-nil CA should give Regenerate")
	}
}

func TestChromeEdgeAndFirefoxSteps(t *testing.T) {
	for _, installed := range []bool{true, false} {
		steps := chromeEdgeSteps(installed)
		if len(steps) == 0 {
			t.Errorf("expected chromeEdgeSteps non-empty for %v", installed)
		}
		joined := strings.ToLower(strings.Join(steps, " "))
		if installed && !strings.Contains(joined, "trusted") {
			t.Errorf("installed=true should mention trusted")
		}
		if !installed && !strings.Contains(joined, "install") {
			t.Errorf("installed=false should mention install")
		}
	}
	ff := firefoxSteps()
	if len(ff) < 3 {
		t.Errorf("firefoxSteps too short: %d", len(ff))
	}
	if !strings.Contains(strings.ToLower(strings.Join(ff, " ")), "firefox") {
		t.Errorf("firefoxSteps should mention firefox")
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{-1, "-"},
		{0, "0B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1024 * 1024, "1.0M"},
		{1536, "1.5K"},
	}
	for _, c := range cases {
		if got := humanSize(c.n); got != c.want {
			t.Errorf("humanSize(%d) = %q want %q", c.n, got, c.want)
		}
	}
}

func TestHumanDuration(t *testing.T) {
	f := &Flow{}
	if got := humanDuration(f); got != "-" {
		t.Errorf("zero Started: got %q want -", got)
	}
	now := time.Now()
	f = &Flow{Started: now.Add(-500 * time.Microsecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "µs") {
		t.Errorf("expected microseconds suffix, got %q", got)
	}
	f = &Flow{Started: now.Add(-50 * time.Millisecond), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "ms") {
		t.Errorf("expected ms, got %q", got)
	}
	f = &Flow{Started: now.Add(-2 * time.Second), Ended: now}
	if got := humanDuration(f); !strings.HasSuffix(got, "s") {
		t.Errorf("expected s, got %q", got)
	}

	f = &Flow{Started: time.Now().Add(-10 * time.Millisecond)}
	if got := humanDuration(f); got == "-" {
		t.Errorf("live flow should compute, got %q", got)
	}
}

func TestTunnelStatusText(t *testing.T) {
	if got := tunnelStatusText(&Flow{}); got != "…" {
		t.Errorf("empty: got %q", got)
	}
	if got := tunnelStatusText(&Flow{Status: "200 OK"}); got != "200 OK" {
		t.Errorf("status only: got %q", got)
	}
	got := tunnelStatusText(&Flow{Status: "200 OK", Error: "boom"})
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "boom") {
		t.Errorf("status+err: got %q", got)
	}

}

func TestMITMStatusLine(t *testing.T) {
	now := time.Now()
	f := &Flow{Status: "200 OK", ReqSize: 100, RespSize: 200, Started: now.Add(-20 * time.Millisecond), Ended: now}
	got := statusLine(f)
	if !strings.Contains(got, "200 OK") || !strings.Contains(got, "req") || !strings.Contains(got, "resp") {
		t.Errorf("status line missing components: %q", got)
	}

	f2 := &Flow{}
	got2 := statusLine(f2)
	if got2 != "-" {
		t.Errorf("expected just '-' for empty flow, got %q", got2)
	}
}

func TestMITMFindByID(t *testing.T) {
	s := NewStore()
	if got := s.FindByID(1); got != nil {
		t.Errorf("empty store: expected nil")
	}
	added := s.Add(&Flow{Method: "GET", Host: "h"})
	if got := s.FindByID(added.ID); got == nil || got.ID != added.ID {
		t.Errorf("expected to find by ID %d", added.ID)
	}
	if got := s.FindByID(9999); got != nil {
		t.Errorf("expected nil for missing ID")
	}
}

func TestConsumeStartupFlags_NoAdmin(t *testing.T) {
	setupTestConfigDir(t)
	var st UIState
	st.Ensure()

	st.AutoStart = true
	st.AutoInstallCA = true
	st.AutoRemoveCA = true
	st.consumeStartupFlags()
	if st.AutoStart || st.AutoInstallCA || st.AutoRemoveCA {
		t.Errorf("startup flags should be consumed (reset to false) regardless of admin state")
	}
}

type uiRig struct {
	s    *UIState
	host *Host
	r    input.Router
	sz   image.Point
	now  time.Time
}

func newUIRig(t *testing.T, sz image.Point) *uiRig {
	t.Helper()
	setupTestConfigDir(t)
	rig := &uiRig{
		s:    &UIState{},
		host: &Host{Theme: material.NewTheme(), Window: new(app.Window)},
		sz:   sz,
		now:  time.Unix(1700000000, 0),
	}
	rig.s.Ensure()
	t.Cleanup(func() {
		if rig.s.Proxy != nil && rig.s.Proxy.Running() {
			rig.s.Proxy.Stop()
		}
	})
	return rig
}

func (rig *uiRig) gtx() layout.Context {
	rig.now = rig.now.Add(16 * time.Millisecond)
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(rig.sz),
		Source:      rig.r.Source(),
		Now:         rig.now,
	}
}

func (rig *uiRig) frame() layout.Dimensions {
	gtx := rig.gtx()
	dims := rig.s.Layout(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *uiRig) frames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.frame()
	}
	return d
}

func (rig *uiRig) sidebarFrame() layout.Dimensions {
	gtx := rig.gtx()
	gtx.Constraints = layout.Exact(image.Pt(300, rig.sz.Y))
	dims := rig.s.LayoutSidebar(gtx, rig.host)
	rig.r.Frame(gtx.Ops)
	return dims
}

func (rig *uiRig) sidebarFrames(n int) layout.Dimensions {
	var d layout.Dimensions
	for i := 0; i < n; i++ {
		d = rig.sidebarFrame()
	}
	return d
}

func (rig *uiRig) press(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Press, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *uiRig) move(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Move, Position: f32.Pt(x, y), Buttons: pointer.ButtonPrimary, Source: pointer.Mouse})
	rig.frame()
}

func (rig *uiRig) release(x, y float32) {
	rig.r.Queue(pointer.Event{Kind: pointer.Release, Position: f32.Pt(x, y), Source: pointer.Mouse})
	rig.frames(2)
}

func seedFlows(s *UIState) {
	base := time.Unix(1700000000, 0)
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Src: SrcForward, ClientAddr: "127.0.0.1:5511",
		Scheme: "https", Method: "GET", Host: "api.example.com", Port: "443",
		Path: "/v1/users?page=2&sort=name", URL: "https://api.example.com/v1/users?page=2&sort=name",
		Version: "HTTP/1.1",
		ReqHeaders: [][2]string{
			{"Host", "api.example.com"},
			{"Cookie", "sid=abc; theme=dark"},
			{"Content-Type", "application/x-www-form-urlencoded"},
		},
		ReqBody: []byte("a=1&b=2"), ReqSize: 7,
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "application/json"}, {"Set-Cookie", "sid=xyz; Path=/; HttpOnly"}},
		RespBody:    []byte(`{"users":[{"id":1,"name":"a"}]}`), RespSize: 31,
		Started: base, Ended: base.Add(120 * time.Millisecond),
		Highlight: "red", Comment: "interesting",
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Src: SrcReverse, TargetDomain: "shop.example.com",
		Scheme: "http", Method: "POST", Host: "shop.example.com", Port: "80",
		Path: "/checkout", URL: "http://shop.example.com/checkout",
		ReqHeaders: [][2]string{{"Content-Type", "application/json"}},
		ReqBody:    []byte(`{"cart":1}`),
		Status:     "500 Internal Server Error", StatusCode: 500,
		RespHeaders: [][2]string{{"Content-Type", "text/html"}},
		RespBody:    []byte("<html><body>Server <b>error</b></body></html>"),
		Started:     base.Add(time.Second), Ended: base.Add(time.Second + 3*time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "cdn.example.com",
		Path: "/assets/app.css", URL: "https://cdn.example.com/assets/app.css",
		StatusCode: 304, Status: "304 Not Modified",
		Started: base.Add(2 * time.Second), Ended: base.Add(2*time.Second + 5*time.Millisecond),
	})
	s.Store.Add(&Flow{
		Kind: FlowTunnel, Method: "CONNECT", Host: "secure.example.com", Port: "443",
		BytesIn: 4096, BytesOut: 2048, Started: base.Add(3 * time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "ws.example.com", Path: "/socket",
		URL: "https://ws.example.com/socket", WebSocket: true,
		StatusCode: 101, Status: "101 Switching Protocols",
		Started: base.Add(4 * time.Second),
	})
	s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "broken.example.com", Path: "/x",
		Error: "dial tcp: connection refused", Started: base.Add(5 * time.Second),
	})

	flows := s.Store.Snapshot()
	wsID := flows[4].ID
	for i := 0; i < 4; i++ {
		s.Proxy.WS.Add(&WSMessage{
			FlowID: wsID, URL: "wss://ws.example.com/socket",
			ToServer: i%2 == 0, Opcode: byte(1 + i%2),
			Payload: []byte(`{"frame":` + string(rune('0'+i)) + `}`),
			Time:    base.Add(time.Duration(i) * time.Second),
		})
	}
}

func TestUILayout_AllViewsRender(t *testing.T) {
	for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets, "unknown"} {
		t.Run(view, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1400, 800))
			seedFlows(rig.s)
			rig.s.View = view
			if d := rig.frames(2); d.Size.X <= 0 || d.Size.Y <= 0 {
				t.Fatalf("view %q produced no dimensions", view)
			}
		})
	}
}

func TestUILayout_EmptyStoreRenders(t *testing.T) {
	for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets} {
		t.Run(view, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1200, 700))
			rig.s.View = view
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatalf("empty view %q produced no dimensions", view)
			}
		})
	}
}

func TestUILayout_InspectorTabsAndModes(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)
	flows := rig.s.Store.Snapshot()

	for _, f := range flows {
		for actTab := 0; actTab < 2; actTab++ {
			for mode := 0; mode < 4; mode++ {
				for sec := 0; sec < 4; sec++ {
					rig.s.Selected = f.ID
					rig.s.ActTab = actTab
					rig.s.RenderMode = mode
					rig.s.SecTab = sec
					if d := rig.frames(1); d.Size.Y <= 0 {
						t.Fatalf("flow %d tab %d mode %d sec %d produced no dimensions", f.ID, actTab, mode, sec)
					}
				}
			}
		}
	}
}

func TestUILayout_InspectorCollapsed(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.InspectorCollapsed = true
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("collapsed inspector produced no dimensions")
	}
	rig.s.InspectorToggle.Click()
	rig.frames(2)
	if rig.s.InspectorCollapsed {
		t.Error("toggle must expand the inspector")
	}
	rig.s.InspectorToggle.Click()
	rig.frames(2)
	if !rig.s.InspectorCollapsed {
		t.Error("toggle must collapse the inspector again")
	}
}

func TestUILayout_UnselectedAndMissingFlow(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)
	rig.s.Selected = 0
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("no selection produced no dimensions")
	}
	rig.s.Selected = 999999
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("stale selection produced no dimensions")
	}
}

func TestUILayout_OverlaysRender(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*UIState)
	}{
		{"clear-confirm", func(s *UIState) { s.ClearConfirmOpen = true }},
		{"context-menu", func(s *UIState) {
			s.CtxOpen = true
			s.CtxFlowID = s.Store.Snapshot()[0].ID
			s.CtxPos.X, s.CtxPos.Y = 300, 200
		}},
		{"annotate", func(s *UIState) {
			s.AnnotateOpen = true
			s.AnnotateFlowID = s.Store.Snapshot()[0].ID
		}},
		{"help", func(s *UIState) { s.HelpOpen = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rig := newUIRig(t, image.Pt(1300, 800))
			seedFlows(rig.s)
			rig.frames(1)
			tc.setup(rig.s)
			if d := rig.frames(2); d.Size.Y <= 0 {
				t.Fatalf("overlay %q produced no dimensions", tc.name)
			}
		})
	}
}

func TestUILayout_ViewSwitcherButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	rig.s.SegInterc.Click()
	rig.frames(2)
	if rig.s.View != ViewIntercept {
		t.Errorf("View = %q, want %q", rig.s.View, ViewIntercept)
	}
	rig.s.SegWS.Click()
	rig.frames(2)
	if rig.s.View != ViewWebSockets {
		t.Errorf("View = %q, want %q", rig.s.View, ViewWebSockets)
	}
	rig.s.SegHistory.Click()
	rig.frames(2)
	if rig.s.View != ViewHistory {
		t.Errorf("View = %q, want %q", rig.s.View, ViewHistory)
	}
}

func TestUILayout_ClearFlowThroughConfirm(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	if rig.s.Store.Len() == 0 {
		t.Fatal("precondition: store must have flows")
	}

	rig.s.ClearBtn.Click()
	rig.frames(2)
	if !rig.s.ClearConfirmOpen {
		t.Fatal("Clear must open the confirmation modal")
	}
	rig.s.ClearNoBtn.Click()
	rig.frames(2)
	if rig.s.ClearConfirmOpen {
		t.Error("No must close the modal")
	}
	if rig.s.Store.Len() == 0 {
		t.Error("No must not clear the store")
	}

	rig.s.ClearBtn.Click()
	rig.frames(2)
	rig.s.ClearYesBtn.Click()
	rig.frames(2)
	if rig.s.Store.Len() != 0 {
		t.Errorf("Yes must clear the store, %d flows left", rig.s.Store.Len())
	}
	if rig.s.ClearConfirmOpen {
		t.Error("Yes must close the modal")
	}
}

func TestUILayout_FilterEditorNarrowsRows(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	all := len(rig.s.filteredFlows())
	rig.s.Filter.SetText("checkout")
	rig.frames(2)
	got := rig.s.filteredFlows()
	if len(got) != 1 || got[0].Path != "/checkout" {
		t.Fatalf("filter did not narrow to /checkout: %d rows", len(got))
	}

	rig.s.FilterClr.Click()
	rig.frames(2)
	if rig.s.Filter.Text() != "" {
		t.Errorf("clear button must empty the filter, got %q", rig.s.Filter.Text())
	}
	if len(rig.s.filteredFlows()) != all {
		t.Errorf("clearing the filter must restore all %d rows", all)
	}
}

func TestUILayout_HideNoiseSwitch(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)

	before := len(rig.s.filteredFlows())
	rig.s.HideNoiseSw.Value = true
	after := len(rig.s.filteredFlows())
	if after >= before {
		t.Errorf("hiding noise must drop the .css row: %d -> %d", before, after)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke with noise hidden")
	}
}

func TestUILayout_SortColumnButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	seen := map[string]bool{}
	for i := range rig.s.SortClicks {
		rig.s.SortClicks[i].Click()
		rig.frames(2)
		seen[rig.s.SortColumn] = true
	}
	if len(seen) < 3 {
		t.Errorf("clicking the header columns changed the sort column to only %v", seen)
	}

	rig.s.SortClicks[1].Click()
	rig.frames(2)
	if rig.s.SortColumn != histCols[1] || !rig.s.SortAsc {
		t.Fatalf("first click on a new column must select it ascending: %q asc=%v", rig.s.SortColumn, rig.s.SortAsc)
	}
	rig.s.SortClicks[1].Click()
	rig.frames(2)
	if rig.s.SortColumn != histCols[1] || rig.s.SortAsc {
		t.Errorf("re-clicking the active column must flip direction: %q asc=%v", rig.s.SortColumn, rig.s.SortAsc)
	}
}

func TestUILayout_RowClickSelects(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	sel := map[uint64]bool{}
	for y := float32(80); y < 300; y += 2 {
		rig.press(200, y)
		rig.release(200, y)
		sel[rig.s.Selected] = true
	}
	delete(sel, 0)
	if len(sel) < 3 {
		t.Errorf("clicking history rows selected only %d distinct flows: %v", len(sel), sel)
	}
}

func TestUILayout_SplitDragMovesRatio(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.frames(2)

	before := rig.s.SplitRatio
	x := float32(rig.s.LeftDrawn) + 3
	rig.press(x, 400)
	rig.move(x-150, 400)
	rig.release(x-150, 400)
	if rig.s.SplitRatio >= before {
		t.Errorf("dragging the split left must shrink SplitRatio: %v -> %v", before, rig.s.SplitRatio)
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("layout broke after the split drag")
	}
}

func TestUILayout_WebSocketViewAndSelection(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.View = ViewWebSockets
	rig.frames(2)

	msgs := rig.s.Proxy.WS.Snapshot()
	if len(msgs) == 0 {
		t.Fatal("precondition: WS store must have frames")
	}
	for _, m := range msgs {
		rig.s.WSSelected = m.ID
		if d := rig.frames(1); d.Size.Y <= 0 {
			t.Fatalf("WS message %d produced no dimensions", m.ID)
		}
	}
	rig.s.WSSelected = 999999
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("stale WS selection broke the layout")
	}

	rig.s.Filter.SetText("frame")
	if got := rig.s.filteredWS(); len(got) != len(msgs) {
		t.Errorf("filter 'frame' matched %d of %d frames", len(got), len(msgs))
	}
	rig.s.Filter.SetText("no-such-payload")
	if got := rig.s.filteredWS(); len(got) != 0 {
		t.Errorf("non-matching filter returned %d frames", len(got))
	}
	rig.frames(2)
}

func TestUILayout_InterceptView(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.View = ViewIntercept
	rig.frames(2)

	rig.s.Proxy.Manual.SetOn(true)
	rig.frames(2)
	if !rig.s.Proxy.Manual.On() {
		t.Fatal("manual interception must be on")
	}

	done := make(chan struct{})
	go func() {
		rig.s.Proxy.Manual.Hold(&Held{
			Kind: HeldRequest, Method: "GET", URL: "https://x/y", Host: "x",
			Raw: []byte("GET /y HTTP/1.1\r\nHost: x\r\n\r\n"),
		})
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && rig.s.Proxy.Manual.Len() == 0 {
		rig.frame()
		time.Sleep(2 * time.Millisecond)
	}
	if rig.s.Proxy.Manual.Len() == 0 {
		t.Fatal("held message never reached the queue")
	}
	if d := rig.frames(2); d.Size.Y <= 0 {
		t.Fatal("intercept view with a held message produced no dimensions")
	}

	rig.s.ForwardBtn.Click()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rig.frame()
		select {
		case <-done:
			rig.s.Proxy.Manual.SetOn(false)
			rig.frames(2)
			if rig.s.Proxy.Manual.On() {
				t.Error("SetOn(false) must disable manual interception")
			}
			return
		default:
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("Forward never released the held message")
}

func TestUILayout_SidebarSectionsRender(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamManual, UpstreamAddr: "127.0.0.1:9", TLS: TLSDecrypt})
	rig.s.Proxy.Targets.Add(&Target{Domain: "*.api.example.com", Upstream: UpstreamAuto, TLS: TLSTunnel, DoH: true})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: false, Type: MRRequest, Area: MRBody, Pattern: "a", Replacement: "b", IsRegex: true, Comment: "swap"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "example.com"})
	rig.s.Proxy.IRules.Add(HeldResponse, InterceptCond{Enabled: true, Or: true, Field: CondStatus, Value: "500"})

	all := []*bool{
		&rig.s.SecTargetsOpen, &rig.s.SecTLSOpen, &rig.s.SecIRulesOpen,
		&rig.s.SecMROpen, &rig.s.SecScopeOpen,
	}
	for _, open := range all {
		*open = false
	}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("collapsed sidebar produced no dimensions")
	}
	for _, open := range all {
		*open = true
	}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("expanded sidebar produced no dimensions")
	}

	rig.s.TargetRows["shop.example.com"] = &TargetRow{Expanded: true}
	if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
		t.Fatal("expanded target row produced no dimensions")
	}
	for _, kind := range []string{HeldRequest, HeldResponse} {
		rig.s.IRulesActive = kind
		if d := rig.sidebarFrames(2); d.Size.Y <= 0 {
			t.Fatalf("intercept rules (%s) produced no dimensions", kind)
		}
	}
}

func TestUILayout_SidebarAccordionHeaders(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.sidebarFrames(2)

	toggles := []struct {
		name  string
		click func()
		flag  func() bool
	}{
		{"targets", func() { rig.s.SecTargetsHdr.Click() }, func() bool { return rig.s.SecTargetsOpen }},
		{"tls", func() { rig.s.SecTLSHdr.Click() }, func() bool { return rig.s.SecTLSOpen }},
		{"irules", func() { rig.s.SecIRulesHdr.Click() }, func() bool { return rig.s.SecIRulesOpen }},
		{"mr", func() { rig.s.SecMRHdr.Click() }, func() bool { return rig.s.SecMROpen }},
		{"scope", func() { rig.s.SecScopeHdr.Click() }, func() bool { return rig.s.SecScopeOpen }},
	}
	for _, tc := range toggles {
		before := tc.flag()
		tc.click()
		rig.sidebarFrames(2)
		if tc.flag() == before {
			t.Errorf("clicking the %s header did not toggle its section", tc.name)
		}
	}
}

func TestUILayout_SidebarAddTarget(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.sidebarFrames(2)

	rig.s.TargetInput.SetText("added.example.com")
	rig.s.TargetAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 1 {
		t.Fatalf("target was not added, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetInput.Text() != "" {
		t.Errorf("the input must be cleared after adding, got %q", rig.s.TargetInput.Text())
	}

	rig.s.TargetInput.SetText("not-a-domain")
	rig.s.TargetAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 1 {
		t.Errorf("an invalid domain must not be added, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetBanner == "" {
		t.Error("an invalid domain must set a banner")
	}
}

func TestUILayout_SidebarAddScopeAndMRAndIRule(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecScopeOpen = true
	rig.s.SecMROpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.ScopePatInput.SetText("example.com")
	rig.s.ScopeAddBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.ScopeR.Len() != 1 {
		t.Errorf("scope rule was not added, len=%d", rig.s.Proxy.ScopeR.Len())
	}

	rig.s.MRPatInput.SetText("secret")
	rig.s.MRReplInput.SetText("REDACTED")
	rig.s.MRAddBtn.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) != 1 {
		t.Errorf("match&replace rule was not added, len=%d", len(rig.s.Proxy.MR.Snapshot()))
	}

	rig.s.IRuleValInput.SetText("example.com")
	rig.s.IRuleAddBtn.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(rig.s.IRulesActive); len(conds) != 1 {
		t.Errorf("intercept condition was not added, len=%d", len(conds))
	}
}

func TestUILayout_SidebarPresets(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.MRPresetCSP.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) == 0 {
		t.Error("the CSP preset must add at least one match&replace rule")
	}

	rig.s.IRulePresetImg.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(rig.s.IRulesActive); len(conds) == 0 {
		t.Error("the image preset must add at least one intercept condition")
	}
}

func TestUILayout_SidebarCycleSelectors(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	cases := []struct {
		name  string
		click func()
		get   func() int
	}{
		{"mr-type", func() { rig.s.MRTypeBtn.Click() }, func() int { return rig.s.MRTypeSel }},
		{"mr-area", func() { rig.s.MRAreaBtn.Click() }, func() int { return rig.s.MRAreaSel }},
		{"scope-kind", func() { rig.s.ScopeKindBtn.Click() }, func() int { return rig.s.ScopeKindSel }},
		{"scope-field", func() { rig.s.ScopeFieldBtn.Click() }, func() int { return rig.s.ScopeFieldSel }},
		{"irule-field", func() { rig.s.IRuleFieldBtn.Click() }, func() int { return rig.s.IRuleFieldSel }},
	}
	for _, tc := range cases {
		seen := map[int]bool{tc.get(): true}
		for i := 0; i < 12; i++ {
			tc.click()
			rig.sidebarFrames(1)
			seen[tc.get()] = true
		}
		if len(seen) < 2 {
			t.Errorf("%s selector never changed value", tc.name)
		}
		if tc.get() < 0 {
			t.Errorf("%s selector went negative: %d", tc.name, tc.get())
		}
	}
}

func TestUILayout_SidebarIRuleTabs(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	rig.s.IRulesRespTab.Click()
	rig.sidebarFrames(2)
	if rig.s.IRulesActive != HeldResponse {
		t.Errorf("IRulesActive = %q, want %q", rig.s.IRulesActive, HeldResponse)
	}
	rig.s.IRulesReqTab.Click()
	rig.sidebarFrames(2)
	if rig.s.IRulesActive != HeldRequest {
		t.Errorf("IRulesActive = %q, want %q", rig.s.IRulesActive, HeldRequest)
	}
}

func TestUILayout_NarrowViewport(t *testing.T) {
	for _, sz := range []image.Point{{X: 500, Y: 300}, {X: 800, Y: 400}, {X: 1920, Y: 1080}} {
		rig := newUIRig(t, sz)
		seedFlows(rig.s)
		rig.s.Selected = rig.s.Store.Snapshot()[0].ID
		for _, view := range []string{ViewHistory, ViewIntercept, ViewWebSockets} {
			rig.s.View = view
			if d := rig.frames(2); d.Size.X <= 0 {
				t.Errorf("size %v view %q produced no dimensions", sz, view)
			}
		}
	}
}

func TestUILayout_ConfigRoundTrip(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.Proxy.Targets.Add(&Target{Domain: "a.example.com", Upstream: UpstreamManual, UpstreamAddr: "1.2.3.4:80", TLS: TLSTunnel, Delay: 250 * time.Millisecond, DoH: true})
	rig.s.Proxy.Rules.Set("b.example.com", HostRule{Delay: 100 * time.Millisecond, UseDoH: true})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "p", Replacement: "q", Comment: "c"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondMethod, Value: "POST"})
	rig.s.Proxy.IRules.SetEnabled(HeldResponse, false)
	rig.s.BindAddr.SetText("127.0.0.1:9999")
	rig.s.SortColumn = "Status"
	rig.s.SortAsc = false
	rig.s.InspectorCollapsed = true
	rig.s.View = ViewWebSockets

	c := rig.s.SnapshotConfig()
	if err := SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded := LoadConfig()
	if loaded.BindAddr != "127.0.0.1:9999" || loaded.View != ViewWebSockets {
		t.Errorf("bind/view not persisted: %+v", loaded)
	}
	if loaded.SortColumn != "Status" || loaded.SortAsc {
		t.Errorf("sort not persisted: %q asc=%v", loaded.SortColumn, loaded.SortAsc)
	}
	if !loaded.InspectorCollapsed {
		t.Error("inspector collapse not persisted")
	}
	if len(loaded.Targets) != 1 || loaded.Targets[0].DelayMs != 250 || !loaded.Targets[0].DoH {
		t.Errorf("targets not persisted: %+v", loaded.Targets)
	}
	if len(loaded.Rules) != 1 || loaded.Rules[0].DelayMs != 100 {
		t.Errorf("rules not persisted: %+v", loaded.Rules)
	}
	if len(loaded.MatchReplace) != 1 || loaded.MatchReplace[0].Comment != "c" {
		t.Errorf("match&replace not persisted: %+v", loaded.MatchReplace)
	}
	if len(loaded.Scope) != 1 || loaded.Scope[0].Kind != ScopeExclude {
		t.Errorf("scope not persisted: %+v", loaded.Scope)
	}
	if len(loaded.InterceptReq) != 1 || loaded.InterceptReq[0].Value != "POST" {
		t.Errorf("intercept conditions not persisted: %+v", loaded.InterceptReq)
	}
	if loaded.IRespEnabled == nil || *loaded.IRespEnabled {
		t.Error("response ruleset enable flag not persisted")
	}

	p2 := NewProxy(NewStore())
	p2.Rules = NewRules()
	loaded.ApplyTo(p2)
	if p2.Targets.Len() != 1 || p2.Rules.Len() != 1 || len(p2.MR.Snapshot()) != 1 || p2.ScopeR.Len() != 1 {
		t.Errorf("ApplyTo did not restore the proxy state")
	}
	if enabled, conds := p2.IRules.Snapshot(HeldResponse); enabled || len(conds) != 0 {
		t.Errorf("response ruleset restored wrong: enabled=%v conds=%d", enabled, len(conds))
	}
}

func TestUILayout_DirtyFlag(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.frames(1)
	rig.s.MarkDirty()
	if !rig.s.Dirty() {
		t.Fatal("MarkDirty must set the flag")
	}
	if rig.s.Dirty() {
		t.Error("Dirty must clear the flag after reading")
	}
}

func TestUILayout_MRAddRejectsEmptyPattern(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.sidebarFrames(2)

	for _, pat := range []string{"", "   "} {
		rig.s.MRPatInput.SetText(pat)
		rig.s.MRReplInput.SetText("injected")
		rig.s.MRAddBtn.Click()
		rig.sidebarFrames(2)
		if n := len(rig.s.Proxy.MR.Snapshot()); n != 0 {
			t.Fatalf("pattern %q added %d rule(s); a rule naming no header would be applied to every message", pat, n)
		}
	}

	rig.s.MRPatInput.SetText("  X-Trace  ")
	rig.s.MRAddBtn.Click()
	rig.sidebarFrames(2)
	rules := rig.s.Proxy.MR.Snapshot()
	if len(rules) != 1 {
		t.Fatalf("a non-empty pattern must still be accepted, len=%d", len(rules))
	}
	if rules[0].Pattern != "X-Trace" {
		t.Errorf("Pattern = %q, want the trimmed name", rules[0].Pattern)
	}
}

func TestPipeline_SerializeRoundTrip(t *testing.T) {
	headers := [][2]string{{"Zeta", "1"}, {"Alpha", "2"}, {"Content-Length", "5"}}
	raw := serializeRequest("POST", "/submit", "HTTP/1.1", headers, []byte("hello"))
	s := string(raw)
	if !strings.HasPrefix(s, "POST /submit HTTP/1.1\r\n") {
		t.Fatalf("bad request line: %q", s)
	}
	if strings.Index(s, "Alpha:") > strings.Index(s, "Zeta:") {
		t.Error("writeHeaderPairs must emit headers in sorted order")
	}

	er, ok := parseRequestRaw(raw)
	if !ok {
		t.Fatal("parseRequestRaw failed on our own output")
	}
	if er.Method != "POST" || er.RequestURI != "/submit" {
		t.Errorf("method/uri = %q %q", er.Method, er.RequestURI)
	}
	if string(er.Body) != "hello" {
		t.Errorf("body = %q", er.Body)
	}
	if headerVal(er.Headers, "alpha") != "2" {
		t.Errorf("headers lost: %+v", er.Headers)
	}
}

func TestPipeline_SerializeResponseRoundTrip(t *testing.T) {
	headers := [][2]string{{"Content-Type", "text/plain"}, {"Content-Length", "2"}}
	raw := serializeResponse("200 OK", "HTTP/1.1", headers, []byte("ok"))
	if !strings.HasPrefix(string(raw), "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("bad status line: %q", raw)
	}
	er, ok := parseResponseRaw(raw)
	if !ok {
		t.Fatal("parseResponseRaw failed on our own output")
	}
	if er.Status != "200 OK" || string(er.Body) != "ok" {
		t.Errorf("status=%q body=%q", er.Status, er.Body)
	}
}

func TestPipeline_SerializeDefaultsProto(t *testing.T) {
	if got := string(serializeRequest("GET", "/", "", nil, nil)); !strings.HasPrefix(got, "GET / HTTP/1.1\r\n") {
		t.Errorf("empty proto must default to HTTP/1.1, got %q", got)
	}
	if got := string(serializeResponse("204 No Content", "", nil, nil)); !strings.HasPrefix(got, "HTTP/1.1 204") {
		t.Errorf("empty proto must default to HTTP/1.1, got %q", got)
	}
}

func TestPipeline_ParseRejectsGarbage(t *testing.T) {
	if _, ok := parseRequestRaw([]byte("this is not http")); ok {
		t.Error("garbage must not parse as a request")
	}
	if _, ok := parseResponseRaw([]byte("this is not http")); ok {
		t.Error("garbage must not parse as a response")
	}
}

func TestPipeline_NormalizeCRLF(t *testing.T) {
	cases := map[string]string{
		"a\nb":       "a\r\nb",
		"a\r\nb":     "a\r\nb",
		"a\r\n\nb":   "a\r\n\r\nb",
		"no newline": "no newline",
		"":           "",
	}
	for in, want := range cases {
		if got := string(normalizeCRLF([]byte(in))); got != want {
			t.Errorf("normalizeCRLF(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPipeline_ParseToleratesBareLF(t *testing.T) {
	raw := []byte("GET /x HTTP/1.1\nHost: example.com\nX-Test: 1\n\nbody")
	er, ok := parseRequestRaw(raw)
	if !ok {
		t.Fatal("bare-LF request must parse after normalisation")
	}
	if er.RequestURI != "/x" || headerVal(er.Headers, "X-Test") != "1" {
		t.Errorf("parsed wrong: %+v", er)
	}
}

func TestPipeline_PairsToHeader(t *testing.T) {
	h := pairsToHeader([][2]string{{"A", "1"}, {"A", "2"}, {"B", "3"}})
	if got := h.Values("A"); len(got) != 2 {
		t.Errorf("duplicate header keys must be preserved, got %v", got)
	}
	if h.Get("B") != "3" {
		t.Errorf("B = %q", h.Get("B"))
	}
}

func TestPipeline_IsWebSocketUpgrade(t *testing.T) {
	cases := []struct {
		upgrade, connection string
		want                bool
	}{
		{"websocket", "Upgrade", true},
		{"WebSocket", "keep-alive, Upgrade", true},
		{"websocket", "keep-alive", false},
		{"h2c", "Upgrade", false},
		{"", "", false},
	}
	for _, c := range cases {
		h := pairsToHeader([][2]string{{"Upgrade", c.upgrade}, {"Connection", c.connection}})
		if got := isWebSocketUpgrade(h); got != c.want {
			t.Errorf("isWebSocketUpgrade(%q,%q) = %v, want %v", c.upgrade, c.connection, got, c.want)
		}
	}
}

func TestMatchReplace_HeadersAddReplaceDelete(t *testing.T) {
	cases := []struct {
		name     string
		rule     MatchReplaceRule
		in, want [][2]string
	}{
		{
			"replace-existing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "Server", Replacement: "rete"},
			[][2]string{{"Server", "nginx"}, {"X", "1"}},
			[][2]string{{"Server", "rete"}, {"X", "1"}},
		},
		{
			"delete-existing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-Frame-Options"},
			[][2]string{{"X-Frame-Options", "DENY"}, {"X", "1"}},
			[][2]string{{"X", "1"}},
		},
		{
			"add-missing",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "X-New", Replacement: "v"},
			[][2]string{{"X", "1"}},
			[][2]string{{"X", "1"}, {"X-New", "v"}},
		},
		{
			"case-insensitive-name",
			MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRHeader, Pattern: "server", Replacement: "t"},
			[][2]string{{"SERVER", "nginx"}},
			[][2]string{{"SERVER", "t"}},
		},
		{
			"disabled-noop",
			MatchReplaceRule{Enabled: false, Type: MRResponse, Area: MRHeader, Pattern: "Server", Replacement: "t"},
			[][2]string{{"Server", "nginx"}},
			[][2]string{{"Server", "nginx"}},
		},
		{
			"wrong-type-noop",
			MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRHeader, Pattern: "Server", Replacement: "t"},
			[][2]string{{"Server", "nginx"}},
			[][2]string{{"Server", "nginx"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMatchReplace()
			m.Add(c.rule)
			got := m.ApplyHeaders(MRResponse, append([][2]string(nil), c.in...))
			if len(got) != len(c.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(c.want), got)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("header %d = %v, want %v", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestMatchReplace_BodyAndFirstLine(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "secret", Replacement: "REDACTED"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRRequest, Area: MRFirstLine, Pattern: "/old", Replacement: "/new"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: `v\d+`, IsRegex: true, Replacement: "vX"})

	if got := string(m.ApplyBody(MRResponse, []byte("a secret v12 b"))); got != "a REDACTED vX b" {
		t.Errorf("body = %q", got)
	}
	if got := m.ApplyFirstLine(MRRequest, "/old/path"); got != "/new/path" {
		t.Errorf("first line = %q", got)
	}

	orig := []byte("nothing to change")
	if got := m.ApplyBody(MRResponse, orig); &got[0] != &orig[0] {
		t.Error("an unchanged body must be returned without copying")
	}
	if got := m.ApplyFirstLine(MRResponse, "200 OK"); got != "200 OK" {
		t.Errorf("wrong-type first line must be untouched, got %q", got)
	}
}

func TestMatchReplace_InvalidRegexDoesNotPanic(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "([unclosed", IsRegex: true, Replacement: "x"})
	if got := string(m.ApplyBody(MRResponse, []byte("unrelated body"))); got != "unrelated body" {
		t.Errorf("an uncompilable regex must leave non-matching text alone, got %q", got)
	}
}

func TestMatchReplace_EmptyPatternIsNoop(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "", Replacement: "X"})
	if got := string(m.ApplyBody(MRResponse, []byte("abc"))); got != "abc" {
		t.Errorf("an empty pattern must be a no-op, got %q", got)
	}
}

func TestMatchReplace_RemoveUpdateMove(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a", Replacement: "1"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "b", Replacement: "2"})
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "c", Replacement: "3"})

	m.Move(0, 1)
	if got := m.Snapshot(); got[0].Pattern != "b" || got[1].Pattern != "a" {
		t.Errorf("Move(0,1) = %q,%q", got[0].Pattern, got[1].Pattern)
	}
	m.Move(0, -1)
	m.Move(2, 5)
	m.Move(-1, 1)
	if len(m.Snapshot()) != 3 {
		t.Error("out-of-range moves must not change the list")
	}

	m.Update(1, func(r *MatchReplaceRule) { r.Pattern = "z"; r.Enabled = false })
	if got := m.Snapshot(); got[1].Pattern != "z" || got[1].Enabled {
		t.Errorf("Update failed: %+v", got[1])
	}
	m.Update(99, func(r *MatchReplaceRule) { r.Pattern = "boom" })

	m.Remove(0)
	if len(m.Snapshot()) != 2 {
		t.Errorf("Remove failed, len=%d", len(m.Snapshot()))
	}
	m.Remove(99)
	m.Remove(-1)
	if len(m.Snapshot()) != 2 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestMatchReplace_UpdateRecompilesRegex(t *testing.T) {
	m := NewMatchReplace()
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a+", IsRegex: true, Replacement: "X"})
	if got := string(m.ApplyBody(MRResponse, []byte("aaa b"))); got != "X b" {
		t.Fatalf("initial = %q", got)
	}
	m.Update(0, func(r *MatchReplaceRule) { r.Pattern = "b+" })
	if got := string(m.ApplyBody(MRResponse, []byte("aaa bbb"))); got != "aaa X" {
		t.Errorf("Update must invalidate the cached regex, got %q", got)
	}
}

func TestMatchReplace_EnabledFor(t *testing.T) {
	m := NewMatchReplace()
	if m.enabledFor(MRResponse, MRBody) {
		t.Error("an empty ruleset must report nothing enabled")
	}
	m.Add(MatchReplaceRule{Enabled: false, Type: MRResponse, Area: MRBody, Pattern: "a"})
	if m.enabledFor(MRResponse, MRBody) {
		t.Error("a disabled rule must not count")
	}
	m.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a"})
	if !m.enabledFor(MRResponse, MRBody) {
		t.Error("an enabled rule must be reported")
	}
	if m.enabledFor(MRRequest, MRBody) || m.enabledFor(MRResponse, MRHeader) {
		t.Error("type/area must be matched exactly")
	}
}

func TestScope_Rules(t *testing.T) {
	f := &Flow{Host: "api.example.com", Scheme: "https", Port: "443", Path: "/v1/users"}
	cases := []struct {
		name  string
		rules []ScopeRule
		want  bool
	}{
		{"empty", nil, true},
		{"include-match", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"}}, true},
		{"include-miss", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "other"}}, false},
		{"exclude-match", []ScopeRule{{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/v1"}}, false},
		{"exclude-miss", []ScopeRule{{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/health"}}, true},
		{"exclude-wins", []ScopeRule{
			{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "example"},
			{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "/v1"},
		}, false},
		{"disabled-ignored", []ScopeRule{{Enabled: false, Kind: ScopeInclude, Field: "host", Pattern: "other"}}, true},
		{"protocol", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "protocol", Pattern: "https"}}, true},
		{"port", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "port", Pattern: "443"}}, true},
		{"unknown-field-uses-host", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "weird", Pattern: "api"}}, true},
		{"regex", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^api\.`, IsRegex: true}}, true},
		{"regex-miss", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^cdn\.`, IsRegex: true}}, false},
		{"bad-regex", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "([", IsRegex: true}}, false},
		{"case-insensitive", []ScopeRule{{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "EXAMPLE"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := NewScope()
			for _, r := range c.rules {
				s.Add(r)
			}
			if got := s.InScope(f); got != c.want {
				t.Errorf("InScope = %v, want %v", got, c.want)
			}
		})
	}
}

func TestScope_RemoveUpdateSnapshot(t *testing.T) {
	s := NewScope()
	s.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "a"})
	s.Add(ScopeRule{Enabled: true, Kind: ScopeExclude, Field: "path", Pattern: "b"})
	if s.Len() != 2 {
		t.Fatalf("Len = %d", s.Len())
	}
	s.Update(0, func(r *ScopeRule) { r.Pattern = "z" })
	if got := s.Snapshot(); got[0].Pattern != "z" {
		t.Errorf("Update failed: %+v", got[0])
	}
	s.Update(99, func(r *ScopeRule) { r.Pattern = "boom" })
	s.Remove(0)
	if s.Len() != 1 {
		t.Errorf("Remove failed, len=%d", s.Len())
	}
	s.Remove(99)
	s.Remove(-1)
	if s.Len() != 1 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestScope_UpdateRecompilesRegex(t *testing.T) {
	s := NewScope()
	s.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: `^api\.`, IsRegex: true})
	if !s.InScope(&Flow{Host: "api.example.com"}) {
		t.Fatal("initial regex should match")
	}
	s.Update(0, func(r *ScopeRule) { r.Pattern = `^cdn\.` })
	if s.InScope(&Flow{Host: "api.example.com"}) {
		t.Error("Update must invalidate the cached regex")
	}
	if !s.InScope(&Flow{Host: "cdn.example.com"}) {
		t.Error("the updated regex must apply")
	}
}

func TestInterceptRules_ShouldIntercept(t *testing.T) {
	f := &Flow{
		Host: "api.example.com", ClientAddr: "10.0.0.5:33", Method: "POST",
		URL: "https://api.example.com/v1", Path: "/v1/users.json?token=abc",
		StatusCode:  404,
		ReqHeaders:  [][2]string{{"X-Auth", "1"}},
		RespHeaders: [][2]string{{"Content-Type", "application/json; charset=utf-8"}},
	}
	cases := []struct {
		name    string
		conds   []InterceptCond
		enabled bool
		inScope bool
		want    bool
	}{
		{"no-rules", nil, true, true, true},
		{"disabled-set", []InterceptCond{{Enabled: true, Field: CondHost, Value: "nope"}}, false, true, true},
		{"all-conds-disabled", []InterceptCond{{Enabled: false, Field: CondHost, Value: "nope"}}, true, true, true},
		{"host-hit", []InterceptCond{{Enabled: true, Field: CondHost, Value: "example.com"}}, true, true, true},
		{"host-miss", []InterceptCond{{Enabled: true, Field: CondHost, Value: "other.org"}}, true, true, false},
		{"ip", []InterceptCond{{Enabled: true, Field: CondIP, Value: "10.0.0.5"}}, true, true, true},
		{"method", []InterceptCond{{Enabled: true, Field: CondMethod, Value: "post"}}, true, true, true},
		{"method-miss", []InterceptCond{{Enabled: true, Field: CondMethod, Value: "GET"}}, true, true, false},
		{"url", []InterceptCond{{Enabled: true, Field: CondURL, Value: "/v1"}}, true, true, true},
		{"filetype", []InterceptCond{{Enabled: true, Field: CondFileType, Value: "json"}}, true, true, true},
		{"filetype-dotted", []InterceptCond{{Enabled: true, Field: CondFileType, Value: ".json"}}, true, true, true},
		{"filetype-miss", []InterceptCond{{Enabled: true, Field: CondFileType, Value: "css"}}, true, true, false},
		{"mime", []InterceptCond{{Enabled: true, Field: CondMIME, Value: "application/json"}}, true, true, true},
		{"status", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "404"}}, true, true, true},
		{"status-miss", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "200"}}, true, true, false},
		{"status-nan", []InterceptCond{{Enabled: true, Field: CondStatus, Value: "abc"}}, true, true, false},
		{"param", []InterceptCond{{Enabled: true, Field: CondParam, Value: "token"}}, true, true, true},
		{"header", []InterceptCond{{Enabled: true, Field: CondHeader, Value: "X-Auth"}}, true, true, true},
		{"header-miss", []InterceptCond{{Enabled: true, Field: CondHeader, Value: "X-None"}}, true, true, false},
		{"scope-true", []InterceptCond{{Enabled: true, Field: CondScope}}, true, true, true},
		{"scope-false", []InterceptCond{{Enabled: true, Field: CondScope}}, true, false, false},
		{"unknown-field", []InterceptCond{{Enabled: true, Field: "bogus", Value: "x"}}, true, true, false},
		{"and-both-hit", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "example"},
			{Enabled: true, Field: CondMethod, Value: "POST"},
		}, true, true, true},
		{"and-one-miss", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "example"},
			{Enabled: true, Field: CondMethod, Value: "GET"},
		}, true, true, false},
		{"or-one-hit", []InterceptCond{
			{Enabled: true, Field: CondHost, Value: "nope"},
			{Enabled: true, Or: true, Field: CondMethod, Value: "POST"},
		}, true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ir := NewInterceptRules()
			ir.SetEnabled(HeldRequest, c.enabled)
			for _, cc := range c.conds {
				ir.Add(HeldRequest, cc)
			}
			if got := ir.ShouldIntercept(HeldRequest, f, c.inScope); got != c.want {
				t.Errorf("ShouldIntercept = %v, want %v", got, c.want)
			}
		})
	}
}

func TestInterceptRules_ReqAndRespAreSeparate(t *testing.T) {
	ir := NewInterceptRules()
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "req-only"})
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 {
		t.Errorf("request set = %d conds", len(conds))
	}
	if _, conds := ir.Snapshot(HeldResponse); len(conds) != 0 {
		t.Errorf("response set must stay empty, got %d", len(conds))
	}
	if _, conds := ir.Snapshot("anything-else"); len(conds) != 1 {
		t.Error("an unknown kind must fall back to the request set")
	}

	ir.SetEnabled(HeldResponse, false)
	if enabled, _ := ir.Snapshot(HeldResponse); enabled {
		t.Error("SetEnabled(false) not reflected")
	}
	if enabled, _ := ir.Snapshot(HeldRequest); !enabled {
		t.Error("SetEnabled must not leak across kinds")
	}
}

func TestInterceptRules_RemoveUpdate(t *testing.T) {
	ir := NewInterceptRules()
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "a"})
	ir.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "b"})

	ir.Update(HeldRequest, 0, func(c *InterceptCond) { c.Value = "z" })
	if _, conds := ir.Snapshot(HeldRequest); conds[0].Value != "z" {
		t.Errorf("Update failed: %+v", conds[0])
	}
	ir.Update(HeldRequest, 99, func(c *InterceptCond) { c.Value = "boom" })

	ir.Remove(HeldRequest, 0)
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 || conds[0].Value != "b" {
		t.Errorf("Remove failed: %+v", conds)
	}
	ir.Remove(HeldRequest, 99)
	ir.Remove(HeldRequest, -1)
	if _, conds := ir.Snapshot(HeldRequest); len(conds) != 1 {
		t.Error("out-of-range removes must not change the list")
	}
}

func TestPathOnlyAndHeaderVal(t *testing.T) {
	cases := map[string]string{
		"/a/b.json?x=1":  "/a/b.json",
		"/a/b.json#frag": "/a/b.json",
		"/a/b.json":      "/a/b.json",
		"":               "",
		"?only":          "",
	}
	for in, want := range cases {
		if got := pathOnly(in); got != want {
			t.Errorf("pathOnly(%q) = %q, want %q", in, got, want)
		}
	}

	h := [][2]string{{"Content-Type", "text/html"}, {"X-A", "1"}}
	if got := headerVal(h, "content-type"); got != "text/html" {
		t.Errorf("headerVal case-insensitive = %q", got)
	}
	if got := headerVal(h, "missing"); got != "" {
		t.Errorf("headerVal missing = %q", got)
	}
	if got := headerVal(nil, "x"); got != "" {
		t.Errorf("headerVal nil = %q", got)
	}
}

func TestTargets_AddValidationAndDuplicates(t *testing.T) {
	tg := NewTargets()
	cases := []struct {
		domain string
		want   bool
	}{
		{"example.com", true},
		{"EXAMPLE.com", false},
		{"sub.example.com", true},
		{"*.wild.example.com", true},
		{"example.com:443", false},
		{"nodot", false},
		{"", false},
		{"-bad.example.com", false},
		{"bad-.example.com", false},
		{"under_score.com", false},
		{strings.Repeat("a", 64) + ".com", false},
	}
	for _, c := range cases {
		got := tg.Add(&Target{Domain: c.domain})
		if got != c.want {
			t.Errorf("Add(%q) = %v, want %v", c.domain, got, c.want)
		}
	}
}

func TestTargets_AddAppliesDefaults(t *testing.T) {
	tg := NewTargets()
	if !tg.Add(&Target{Domain: "a.example.com"}) {
		t.Fatal("add failed")
	}
	v := tg.Snapshot()[0]
	if v.Upstream != UpstreamAuto || v.TLS != TLSDecrypt || v.Status != StatusWaiting {
		t.Errorf("defaults wrong: %+v", v)
	}
}

func TestTargets_MatchWildcard(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "exact.example.com"})
	tg.Add(&Target{Domain: "*.wild.example.com"})

	cases := []struct {
		host string
		want bool
	}{
		{"exact.example.com", true},
		{"EXACT.example.com", true},
		{"exact.example.com:443", true},
		{"a.wild.example.com", true},
		{"a.b.wild.example.com", true},
		{"wild.example.com", false},
		{"other.example.com", false},
		{"", false},
	}
	for _, c := range cases {
		_, ok := tg.Match(c.host)
		if ok != c.want {
			t.Errorf("Match(%q) = %v, want %v", c.host, ok, c.want)
		}
	}
}

func TestTargets_UpdateRemoveAndStatus(t *testing.T) {
	tg := NewTargets()
	tg.Add(&Target{Domain: "a.example.com"})
	tg.Update("A.EXAMPLE.COM", func(x *Target) { x.UpstreamAddr = "1.2.3.4:80" })
	if got := tg.Snapshot()[0].UpstreamAddr; got != "1.2.3.4:80" {
		t.Errorf("Update must normalise the domain, got %q", got)
	}
	tg.Update("missing.example.com", func(x *Target) { x.UpstreamAddr = "boom" })

	m, ok := tg.Match("a.example.com")
	if !ok {
		t.Fatal("match failed")
	}
	if same, _ := tg.Match("a.example.com"); same == m {
		t.Error("Match must return a copy, not the shared internal pointer")
	}
	tg.markRequest(m.Domain)
	if v := tg.Snapshot()[0]; v.Status != StatusProxying || v.Requests != 1 {
		t.Errorf("markRequest: %+v", v)
	}
	tg.markError(m.Domain, "dial failed")
	if v := tg.Snapshot()[0]; v.Status != StatusError || v.LastErr != "dial failed" {
		t.Errorf("markError: %+v", v)
	}

	tg.Remove("missing.example.com")
	if tg.Len() != 1 {
		t.Errorf("removing a missing domain changed the list, len=%d", tg.Len())
	}
	tg.Remove("A.example.com")
	if tg.Len() != 0 {
		t.Errorf("Remove failed, len=%d", tg.Len())
	}
}

func TestTargets_Notify(t *testing.T) {
	tg := NewTargets()
	var n int
	tg.SetNotify(func() { n++ })
	tg.Add(&Target{Domain: "a.example.com"})
	if n == 0 {
		t.Error("Add must notify")
	}
	tg.SetNotify(nil)
	before := n
	tg.Remove("a.example.com")
	if n != before {
		t.Error("SetNotify(nil) must stop notifications")
	}
}

func TestHostsLine(t *testing.T) {
	cases := map[string]string{
		"example.com":      "127.0.0.1    example.com",
		"*.example.com":    "127.0.0.1    example.com",
		"EXAMPLE.com:443 ": "127.0.0.1    example.com",
	}
	for in, want := range cases {
		if got := HostsLine(in); got != want {
			t.Errorf("HostsLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRules_SetGetRemove(t *testing.T) {
	r := NewRules()
	r.Set("  Example.COM:8080 ", HostRule{Delay: 50 * time.Millisecond, UseDoH: true})
	if got, ok := r.Get("example.com"); !ok || got.Delay != 50*time.Millisecond || !got.UseDoH {
		t.Errorf("Get after normalised Set = %+v ok=%v", got, ok)
	}
	if _, ok := r.Get("other.com"); ok {
		t.Error("missing host must report not found")
	}
	r.Set("", HostRule{Delay: time.Second})
	if r.Len() != 1 {
		t.Errorf("an empty host must be rejected, len=%d", r.Len())
	}

	r.Set("b.com", HostRule{})
	snap := r.Snapshot()
	if len(snap) != 2 || snap[0].Host != "b.com" || snap[1].Host != "example.com" {
		t.Errorf("Snapshot must be sorted by host: %+v", snap)
	}

	r.Remove("EXAMPLE.com")
	if r.Len() != 1 {
		t.Errorf("Remove failed, len=%d", r.Len())
	}
	r.Remove("nonexistent.com")
	if r.Len() != 1 {
		t.Error("removing a missing host must not change the map")
	}
}

func TestNormalizeRuleHostAndDomain(t *testing.T) {
	cases := map[string]string{
		"  Example.COM  ": "example.com",
		"example.com:443": "example.com",
		"":                "",
		"[::1]:80":        "::1",
	}
	for in, want := range cases {
		if got := normalizeRuleHost(in); got != want {
			t.Errorf("normalizeRuleHost(%q) = %q, want %q", in, got, want)
		}
	}
	if got := normalizeDomain("Example.COM."); got != "example.com" {
		t.Errorf("normalizeDomain trailing dot = %q", got)
	}
}

func TestStore_DeleteAndAnnotate(t *testing.T) {
	s := NewStore()
	a := s.Add(&Flow{Host: "a"})
	b := s.Add(&Flow{Host: "b"})
	c := s.Add(&Flow{Host: "c"})

	s.SetAnnotation(b.ID, "red", "look here")
	got := s.FindByID(b.ID)
	if got == nil || got.Highlight != "red" || got.Comment != "look here" {
		t.Errorf("SetAnnotation failed: %+v", got)
	}
	s.SetAnnotation(99999, "blue", "nope")

	s.Delete(b.ID)
	if s.Len() != 2 {
		t.Fatalf("Delete failed, len=%d", s.Len())
	}
	if s.FindByID(b.ID) != nil {
		t.Error("deleted flow is still findable")
	}
	if s.FindByID(a.ID) == nil || s.FindByID(c.ID) == nil {
		t.Error("Delete removed the wrong flows")
	}
	s.Delete(99999)
	if s.Len() != 2 {
		t.Error("deleting a missing ID must not change the store")
	}
}

func TestStore_SnapshotIsolation(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a", ReqBody: []byte("body"), ReqHeaders: [][2]string{{"X", "1"}}})

	snap := s.Snapshot()[0]
	snap.ReqBody[0] = 'B'
	snap.ReqHeaders[0][1] = "mutated"
	snap.Host = "changed"

	again := s.Snapshot()[0]
	if string(again.ReqBody) != "body" || again.ReqHeaders[0][1] != "1" || again.Host != "a" {
		t.Errorf("Snapshot must deep-copy: %+v", again)
	}

	meta := s.SnapshotMeta()[0]
	if meta.ReqBody != nil || meta.RespBody != nil || meta.ReqHeaders != nil {
		t.Error("SnapshotMeta must drop bodies and headers")
	}
	if meta.Host != "a" {
		t.Errorf("SnapshotMeta must keep metadata, got %q", meta.Host)
	}
}

func TestStore_AtAndMarkAllEnded(t *testing.T) {
	s := NewStore()
	s.Add(&Flow{Host: "a"})
	s.Add(&Flow{Host: "b", Ended: time.Unix(1, 0)})

	if got := s.At(0); got == nil || got.Host != "a" {
		t.Errorf("At(0) = %+v", got)
	}
	if s.At(-1) != nil || s.At(99) != nil {
		t.Error("out-of-range At must return nil")
	}
	if !s.At(0).Live() {
		t.Error("a flow without Ended must be Live")
	}

	s.MarkAllEnded()
	for i, f := range s.Snapshot() {
		if f.Ended.IsZero() {
			t.Errorf("flow %d still has a zero Ended", i)
		}
	}
	if got := s.Snapshot()[1].Ended; !got.Equal(time.Unix(1, 0)) {
		t.Errorf("MarkAllEnded must not overwrite an existing Ended, got %v", got)
	}
}

func TestStore_ClearAndNotify(t *testing.T) {
	s := NewStore()
	var n int
	s.SetNotify(func() { n++ })
	s.Add(&Flow{Host: "a"})
	if n == 0 {
		t.Error("Add must notify")
	}
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear failed")
	}
	s.SetNotify(nil)
	before := n
	s.Add(&Flow{Host: "b"})
	if n != before {
		t.Error("SetNotify(nil) must stop notifications")
	}
}

func TestWSStore_Basics(t *testing.T) {
	s := NewWSStore()
	var n int
	s.SetNotify(func() { n++ })
	s.Add(&WSMessage{FlowID: 1, Payload: []byte("hi"), Opcode: 1})
	s.Add(&WSMessage{FlowID: 1, Payload: []byte("there"), Opcode: 2})
	if s.Len() != 2 || n != 2 {
		t.Fatalf("len=%d notifies=%d", s.Len(), n)
	}

	snap := s.Snapshot()
	if snap[0].ID == 0 || snap[1].ID <= snap[0].ID {
		t.Errorf("IDs must be assigned and increasing: %d %d", snap[0].ID, snap[1].ID)
	}
	if snap[0].Time.IsZero() {
		t.Error("Add must stamp a time")
	}
	if string(snap[0].Payload) != "hi" {
		t.Errorf("Snapshot payload = %q", snap[0].Payload)
	}
	if snap[0] != s.Snapshot()[0] {
		t.Error("Snapshot must share messages, not copy them")
	}

	found := s.FindByID(snap[1].ID)
	if found == nil || string(found.Payload) != "there" {
		t.Errorf("FindByID = %+v", found)
	}
	if s.FindByID(99999) != nil {
		t.Error("FindByID must return nil for a missing ID")
	}

	s.SetNotify(nil)
	s.Clear()
	if s.Len() != 0 {
		t.Error("Clear failed")
	}
}

func TestWSOpcodeName(t *testing.T) {
	cases := map[byte]string{0x1: "text", 0x2: "binary", 0x8: "close", 0x9: "ping", 0xA: "pong", 0x0: "cont", 0x7: "cont"}
	for op, want := range cases {
		if got := WSOpcodeName(op); got != want {
			t.Errorf("WSOpcodeName(%#x) = %q, want %q", op, got, want)
		}
	}
}

func TestDirNameAndHighlightColor(t *testing.T) {
	if dirName(true) != "client → server" || dirName(false) != "server → client" {
		t.Error("dirName wrong")
	}
	keys := annotateColorKeys()
	if len(keys) != 6 || keys[0] != "" {
		t.Errorf("annotateColorKeys = %v", keys)
	}
	if highlightColor("").A != 0 {
		t.Error("the empty key must yield a transparent colour")
	}
	if highlightColor("nonsense").A != 0 {
		t.Error("an unknown key must yield a transparent colour")
	}
	for _, k := range keys[1:] {
		if highlightColor(k).A == 0 {
			t.Errorf("colour %q must be opaque", k)
		}
	}
}

func TestFlowAsTextAndCurl(t *testing.T) {
	f := &Flow{
		Method: "POST", Path: "/submit", Version: "HTTP/1.1",
		URL:         "https://example.com/submit",
		ReqHeaders:  [][2]string{{"Host", "example.com"}, {"X-A", "1"}},
		ReqBody:     []byte("payload"),
		Status:      "200 OK",
		RespHeaders: [][2]string{{"Content-Type", "text/plain"}},
		RespBody:    []byte("done"),
	}
	req := flowAsText(f, false)
	if !strings.HasPrefix(req, "POST /submit HTTP/1.1\n") || !strings.Contains(req, "X-A: 1") || !strings.HasSuffix(req, "payload") {
		t.Errorf("request text = %q", req)
	}
	resp := flowAsText(f, true)
	if !strings.HasPrefix(resp, "200 OK\n") || !strings.HasSuffix(resp, "done") {
		t.Errorf("response text = %q", resp)
	}

	curl := asCurl(f)
	if !strings.Contains(curl, "curl -X POST 'https://example.com/submit'") {
		t.Errorf("curl = %q", curl)
	}
	if strings.Contains(curl, "-H 'Host:") {
		t.Error("asCurl must skip the Host header")
	}
	if !strings.Contains(curl, "-H 'X-A: 1'") || !strings.Contains(curl, "--data-binary 'payload'") {
		t.Errorf("curl = %q", curl)
	}
	if got := asCurl(&Flow{Method: "GET", URL: "https://x/"}); strings.Contains(got, "--data-binary") {
		t.Errorf("a bodyless flow must not emit --data-binary: %q", got)
	}
}

func TestParseParams(t *testing.T) {
	f := &Flow{
		Path:       "/search?q=go&page=2",
		ReqHeaders: [][2]string{{"Content-Type", "application/x-www-form-urlencoded"}},
		ReqBody:    []byte("extra=1"),
	}
	got := parseParams(f)
	keys := map[string]string{}
	for _, kv := range got {
		keys[kv[0]] = kv[1]
	}
	if keys["q"] != "go" || keys["page"] != "2" {
		t.Errorf("query params missing: %+v", got)
	}
	if keys["(body) extra"] != "1" {
		t.Errorf("form body params missing: %+v", got)
	}

	if got := parseParams(&Flow{Path: "/no-query"}); len(got) != 0 {
		t.Errorf("a path with no query must yield nothing, got %+v", got)
	}
	if got := parseParams(&Flow{Path: "/x", ReqBody: []byte("a=1")}); len(got) != 0 {
		t.Errorf("a body without a form content-type must be ignored, got %+v", got)
	}
}

func TestParseCookies(t *testing.T) {
	req := parseCookies([][2]string{{"Cookie", "sid=abc; theme=dark; broken"}}, false)
	if len(req) != 2 || req[0][0] != "sid" || req[1][1] != "dark" {
		t.Errorf("request cookies = %+v", req)
	}
	resp := parseCookies([][2]string{
		{"Set-Cookie", "sid=xyz; Path=/; HttpOnly"},
		{"Set-Cookie", "other=2"},
	}, true)
	if len(resp) != 2 || resp[0][0] != "sid" || resp[0][1] != "xyz" || resp[1][1] != "2" {
		t.Errorf("response cookies = %+v", resp)
	}
	if got := parseCookies([][2]string{{"Cookie", "a=1"}}, true); len(got) != 0 {
		t.Error("Cookie headers must be ignored in response mode")
	}
	if got := parseCookies(nil, false); len(got) != 0 {
		t.Errorf("nil headers = %+v", got)
	}
}

func TestHexDump(t *testing.T) {
	got := hexDump([]byte("AB\x00"))
	if !strings.HasPrefix(got, "00000000  41 42 00 ") {
		t.Errorf("hex bytes wrong: %q", got)
	}
	if !strings.Contains(got, "|AB.|") {
		t.Errorf("ascii gutter wrong: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("must end with a newline: %q", got)
	}
	if hexDump(nil) != "" {
		t.Error("empty input must produce empty output")
	}

	long := hexDump(make([]byte, 40*1024))
	lines := strings.Count(long, "\n")
	if lines != 32*1024/16 {
		t.Errorf("hexDump must truncate at 32K: %d lines", lines)
	}
}

func TestStripHTML(t *testing.T) {
	cases := map[string]string{
		"<html><body>Hello <b>world</b></body></html>": "Hello  world",
		"plain":            "plain",
		"":                 "",
		"<p>a</p><p>b</p>": "a  b",
	}
	for in, want := range cases {
		if got := stripHTML(in); got != want {
			t.Errorf("stripHTML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsNoise(t *testing.T) {
	noisy := []string{"/a/app.css", "/x.js", "/img.png", "/f.woff2", "/i.ico"}
	for _, p := range noisy {
		if !isNoise(&Flow{Path: p}) {
			t.Errorf("%q should be noise", p)
		}
	}
	quiet := []string{"/api/users", "/", "", "/graphql"}
	for _, p := range quiet {
		if isNoise(&Flow{Path: p}) {
			t.Errorf("%q should not be noise", p)
		}
	}
}

func TestSortFlows(t *testing.T) {
	base := time.Unix(1700000000, 0)
	mk := func(id uint64, method, host, path, src string, code int, size int64, dur time.Duration) *Flow {
		return &Flow{ID: id, Method: method, Host: host, Path: path, Src: src,
			StatusCode: code, RespSize: size, Started: base, Ended: base.Add(dur)}
	}
	flows := []*Flow{
		mk(3, "POST", "c.com", "/z", SrcReverse, 500, 30, 3*time.Second),
		mk(1, "GET", "a.com", "/x", SrcForward, 200, 10, time.Second),
		mk(2, "DELETE", "b.com", "/y", SrcForward, 404, 20, 2*time.Second),
	}
	cases := []struct {
		col   string
		asc   bool
		first uint64
	}{
		{"#", true, 1},
		{"#", false, 3},
		{"Method", true, 2},
		{"Host / Path", true, 1},
		{"Src", true, 1},
		{"Status", true, 1},
		{"Status", false, 3},
		{"Size", true, 1},
		{"Time", true, 1},
		{"Time", false, 3},
		{"unknown", true, 1},
	}
	for _, c := range cases {
		cp := append([]*Flow(nil), flows...)
		sortFlows(cp, c.col, c.asc)
		if cp[0].ID != c.first {
			t.Errorf("sort by %q asc=%v put flow %d first, want %d", c.col, c.asc, cp[0].ID, c.first)
		}
	}
}

func TestFilteredFlows_TokenFilters(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)

	cases := []struct {
		filter string
		want   int
	}{
		{"", 6},
		{"src:rev", 1},
		{"src:fwd", 5},
		{"status:500", 1},
		{"status:304", 1},
		{"status:101", 1},
		{"mime:.css", 1},
		{"checkout", 1},
		{"example.com", 6},
		{"shop checkout", 1},
		{"shop nomatch", 0},
		{"interesting", 1},
		{"src:rev checkout", 1},
		{"src:rev users", 0},
	}
	for _, c := range cases {
		rig.s.Filter.SetText(c.filter)
		if got := len(rig.s.filteredFlows()); got != c.want {
			t.Errorf("filter %q matched %d flows, want %d", c.filter, got, c.want)
		}
	}
}

func TestFilteredFlows_HideNoise(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	seedFlows(rig.s)
	rig.s.HideNoiseSw.Value = true
	for _, f := range rig.s.filteredFlows() {
		if isNoise(f) {
			t.Errorf("noisy flow %q survived the filter", f.Path)
		}
	}
}

func TestUIEvents_ContextMenuActions(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	first := rig.s.Store.Snapshot()[0]

	steps := []struct {
		name  string
		click func()
		check func(*testing.T)
	}{
		{"copy-url", func() { rig.s.CtxCopyURL.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "URL copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"copy-curl", func() { rig.s.CtxCopyCurl.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "curl copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"copy-req", func() { rig.s.CtxCopyReq.Click() }, func(t *testing.T) {
			if rig.s.StatusBanner != "Request copied" {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"repeat", func() { rig.s.CtxRepeat.Click() }, func(t *testing.T) {
			if !strings.Contains(rig.s.StatusBanner, "Repeat") {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"to-repeater", func() { rig.s.CtxToRepeater.Click() }, func(t *testing.T) {
			if !strings.Contains(rig.s.StatusBanner, "Repeater") {
				t.Errorf("banner = %q", rig.s.StatusBanner)
			}
		}},
		{"add-scope", func() { rig.s.CtxAddScope.Click() }, func(t *testing.T) {
			if rig.s.Proxy.ScopeR.Len() != 1 {
				t.Errorf("scope len = %d, want 1", rig.s.Proxy.ScopeR.Len())
			}
		}},
	}
	for _, st := range steps {
		t.Run(st.name, func(t *testing.T) {
			rig.s.CtxOpen = true
			rig.s.CtxFlowID = first.ID
			st.click()
			rig.frames(2)
			if rig.s.CtxOpen {
				t.Error("the menu must close after an action")
			}
			st.check(t)
		})
	}
}

func TestUIEvents_ContextDeleteAndAnnotate(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	seedFlows(rig.s)
	rig.frames(2)
	first := rig.s.Store.Snapshot()[0]

	rig.s.CtxOpen = true
	rig.s.CtxFlowID = first.ID
	rig.s.CtxAnnotate.Click()
	rig.frames(2)
	if !rig.s.AnnotateOpen || rig.s.AnnotateFlowID != first.ID {
		t.Fatalf("annotate popup did not open for flow %d", first.ID)
	}
	if rig.s.AnnotateComment.Text() != "interesting" {
		t.Errorf("the existing comment must be loaded, got %q", rig.s.AnnotateComment.Text())
	}

	rig.s.AnnotateComment.SetText("edited note")
	rig.s.AnnotateColors[3].Click()
	rig.frames(2)
	if got := rig.s.Store.FindByID(first.ID); got == nil || got.Highlight != annotateColorKeys()[3] {
		t.Errorf("colour swatch did not apply: %+v", got)
	}

	rig.s.AnnotateSave.Click()
	rig.frames(2)
	if rig.s.AnnotateOpen {
		t.Error("Save must close the popup")
	}
	if got := rig.s.Store.FindByID(first.ID); got == nil || got.Comment != "edited note" {
		t.Errorf("comment not saved: %+v", got)
	}

	rig.s.Selected = first.ID
	rig.s.CtxOpen = true
	rig.s.CtxFlowID = first.ID
	rig.s.CtxDelete.Click()
	rig.frames(2)
	if rig.s.Store.FindByID(first.ID) != nil {
		t.Error("Delete did not remove the flow")
	}
	if rig.s.Selected != 0 {
		t.Errorf("deleting the selected flow must clear the selection, got %d", rig.s.Selected)
	}
}

func TestUIEvents_ContextActionsWithMissingFlow(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.frames(2)
	rig.s.CtxOpen = true
	rig.s.CtxFlowID = 424242
	rig.s.StatusBanner = ""
	rig.s.CtxCopyURL.Click()
	rig.frames(2)
	if rig.s.StatusBanner != "" {
		t.Errorf("a missing flow must not set a banner, got %q", rig.s.StatusBanner)
	}
	if rig.s.CtxOpen {
		t.Error("the menu must still close")
	}
}

func TestUIEvents_InspectorTabsAndModeButtons(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.frames(2)

	rig.s.TabResp.Click()
	rig.frames(2)
	if rig.s.ActTab != 1 {
		t.Errorf("ActTab = %d, want 1", rig.s.ActTab)
	}
	rig.s.TabReq.Click()
	rig.frames(2)
	if rig.s.ActTab != 0 {
		t.Errorf("ActTab = %d, want 0", rig.s.ActTab)
	}

	modes := []struct {
		click func()
		want  int
	}{
		{func() { rig.s.ViewPretty.Click() }, 1},
		{func() { rig.s.ViewHex.Click() }, 2},
		{func() { rig.s.ViewRender.Click() }, 3},
		{func() { rig.s.ViewRaw.Click() }, 0},
	}
	for _, m := range modes {
		m.click()
		rig.frames(2)
		if rig.s.RenderMode != m.want {
			t.Errorf("RenderMode = %d, want %d", rig.s.RenderMode, m.want)
		}
	}

	secs := []struct {
		click func()
		want  int
	}{
		{func() { rig.s.SecBody.Click() }, 1},
		{func() { rig.s.SecParams.Click() }, 2},
		{func() { rig.s.SecCookies.Click() }, 3},
		{func() { rig.s.SecHeaders.Click() }, 0},
	}
	for _, m := range secs {
		m.click()
		rig.frames(2)
		if rig.s.SecTab != m.want {
			t.Errorf("SecTab = %d, want %d", rig.s.SecTab, m.want)
		}
	}
}

func TestUIEvents_SendBusAndCopy(t *testing.T) {
	rig := newUIRig(t, image.Pt(1400, 800))
	seedFlows(rig.s)
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.frames(2)

	cases := []struct {
		click func()
		want  string
	}{
		{func() { rig.s.InspSendRepeater.Click() }, "Repeater"},
		{func() { rig.s.InspSendIntruder.Click() }, "Intruder"},
		{func() { rig.s.InspSendComparer.Click() }, "Comparer"},
		{func() { rig.s.InspSendDecoder.Click() }, "Decoder"},
		{func() { rig.s.InspCopy.Click() }, "Copied"},
	}
	for _, c := range cases {
		rig.s.StatusBanner = ""
		c.click()
		rig.frames(2)
		if !strings.Contains(rig.s.StatusBanner, c.want) {
			t.Errorf("banner = %q, want a mention of %q", rig.s.StatusBanner, c.want)
		}
	}
}

func TestUIEvents_TargetRowControls(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamAuto, TLS: TLSDecrypt})
	rig.sidebarFrames(2)

	row := rig.s.TargetRows["shop.example.com"]
	if row == nil {
		t.Fatal("target row was not created by the sidebar layout")
	}

	row.UpstreamManual.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Upstream; got != UpstreamManual {
		t.Errorf("Upstream = %q, want %q", got, UpstreamManual)
	}
	row.UpstreamAuto.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Upstream; got != UpstreamAuto {
		t.Errorf("Upstream = %q, want %q", got, UpstreamAuto)
	}

	row.TLSTunnel.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].TLS; got != TLSTunnel {
		t.Errorf("TLS = %q, want %q", got, TLSTunnel)
	}
	row.TLSDecrypt.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].TLS; got != TLSDecrypt {
		t.Errorf("TLS = %q, want %q", got, TLSDecrypt)
	}

	row.Copy.Click()
	rig.sidebarFrames(2)
	if rig.s.StatusBanner != "Copied hosts line" {
		t.Errorf("banner = %q", rig.s.StatusBanner)
	}

	row.Expand.Click()
	rig.sidebarFrames(2)
	if !row.Expanded {
		t.Error("Expand must open the row")
	}

	row.Remove.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.Targets.Len() != 0 {
		t.Errorf("Remove failed, len=%d", rig.s.Proxy.Targets.Len())
	}
	if rig.s.TargetRows["shop.example.com"] != nil {
		t.Error("Remove must drop the row state")
	}
}

func TestUIEvents_TargetDelayInput(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecTargetsOpen = true
	rig.s.Proxy.Targets.Add(&Target{Domain: "shop.example.com", Upstream: UpstreamAuto, TLS: TLSDecrypt})
	rig.sidebarFrames(2)

	row := rig.s.TargetRows["shop.example.com"]
	row.Expanded = true
	row.DelayInput.SetText("250")
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Delay; got != 250*time.Millisecond {
		t.Errorf("Delay = %v, want 250ms", got)
	}

	row.DelayInput.SetText("not-a-number")
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.Targets.Snapshot()[0].Delay; got != 250*time.Millisecond {
		t.Errorf("an unparseable delay must be ignored, got %v", got)
	}
}

func TestUIEvents_SidebarRuleRowControls(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecMROpen = true
	rig.s.SecScopeOpen = true
	rig.s.SecIRulesOpen = true
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "a"})
	rig.s.Proxy.MR.Add(MatchReplaceRule{Enabled: true, Type: MRResponse, Area: MRBody, Pattern: "b"})
	rig.s.Proxy.ScopeR.Add(ScopeRule{Enabled: true, Kind: ScopeInclude, Field: "host", Pattern: "x"})
	rig.s.Proxy.IRules.Add(HeldRequest, InterceptCond{Enabled: true, Field: CondHost, Value: "y"})
	rig.sidebarFrames(2)

	if len(rig.s.MRRows) < 2 {
		t.Fatalf("MR rows not built: %d", len(rig.s.MRRows))
	}
	rig.s.MRRows[0].Down.Click()
	rig.sidebarFrames(2)
	if got := rig.s.Proxy.MR.Snapshot(); got[0].Pattern != "b" {
		t.Errorf("Down did not reorder: %q first", got[0].Pattern)
	}
	rig.s.MRRows[0].Up.Click()
	rig.sidebarFrames(2)

	rig.s.MRRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if len(rig.s.Proxy.MR.Snapshot()) != 1 {
		t.Errorf("MR Remove failed, len=%d", len(rig.s.Proxy.MR.Snapshot()))
	}

	if len(rig.s.ScopeRows) < 1 {
		t.Fatalf("scope rows not built: %d", len(rig.s.ScopeRows))
	}
	rig.s.ScopeRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if rig.s.Proxy.ScopeR.Len() != 0 {
		t.Errorf("scope Remove failed, len=%d", rig.s.Proxy.ScopeR.Len())
	}

	if len(rig.s.IRuleRows) < 1 {
		t.Fatalf("intercept rows not built: %d", len(rig.s.IRuleRows))
	}
	rig.s.IRuleRows[0].Remove.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(HeldRequest); len(conds) != 0 {
		t.Errorf("intercept Remove failed, len=%d", len(conds))
	}
}

func TestUIEvents_IRuleOrToggle(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.s.SecIRulesOpen = true
	rig.sidebarFrames(2)

	before := rig.s.IRuleOr
	rig.s.IRuleOrBtn.Click()
	rig.sidebarFrames(2)
	if rig.s.IRuleOr == before {
		t.Error("the AND/OR button must toggle")
	}
	rig.s.IRuleValInput.SetText("x")
	rig.s.IRuleAddBtn.Click()
	rig.sidebarFrames(2)
	if _, conds := rig.s.Proxy.IRules.Snapshot(HeldRequest); len(conds) != 1 || conds[0].Or != rig.s.IRuleOr {
		t.Errorf("the added condition must carry the OR flag: %+v", conds)
	}
}

func TestConfig_LoadMissingIsZero(t *testing.T) {
	setupTestConfigDir(t)
	c := LoadConfig()
	if c.BindAddr != "" || len(c.Targets) != 0 {
		t.Errorf("a missing config file must load as zero, got %+v", c)
	}
}

func TestConfig_ApplyToIgnoresInvalidTargets(t *testing.T) {
	p := NewProxy(NewStore())
	p.Rules = NewRules()
	c := Config{Targets: []TargetConfig{{Domain: "good.example.com"}, {Domain: "nodot"}}}
	c.ApplyTo(p)
	if p.Targets.Len() != 1 {
		t.Errorf("invalid targets must be skipped, len=%d", p.Targets.Len())
	}
}

func TestUIState_EnsureDefaults(t *testing.T) {
	setupTestConfigDir(t)
	var s UIState
	s.Ensure()
	if s.Store == nil || s.Proxy == nil {
		t.Fatal("Ensure must allocate the store and proxy")
	}
	if s.SplitRatio != 0.62 {
		t.Errorf("SplitRatio = %v, want 0.62", s.SplitRatio)
	}
	if s.View != ViewHistory || s.IRulesActive != HeldRequest {
		t.Errorf("view/irules = %q/%q", s.View, s.IRulesActive)
	}
	if s.SortColumn != "#" || !s.SortAsc {
		t.Errorf("sort = %q asc=%v", s.SortColumn, s.SortAsc)
	}
	if s.BindAddr.Text() != DefaultAddr {
		t.Errorf("BindAddr = %q, want %q", s.BindAddr.Text(), DefaultAddr)
	}
	if s.TargetRows == nil {
		t.Error("TargetRows must be allocated")
	}
	if !s.SecTargetsOpen || !s.SecTLSOpen {
		t.Error("the primary sections must default to open")
	}

	store, proxy := s.Store, s.Proxy
	s.SplitRatio = 0.9
	s.Ensure()
	if s.Store != store || s.Proxy != proxy || s.SplitRatio != 0.9 {
		t.Error("a second Ensure must not reset existing state")
	}
}

func TestUIState_ApplyLoadedConfig(t *testing.T) {
	setupTestConfigDir(t)
	var s UIState
	s.Ensure()
	s.Config = Config{
		BindAddr: "127.0.0.1:7777", View: ViewWebSockets,
		SortColumn: "Size", SortAsc: false,
		InspectorCollapsed: true, Decrypt: true, InterceptResponses: true,
	}
	s.applyLoadedConfig()

	if s.BindAddr.Text() != "127.0.0.1:7777" || s.View != ViewWebSockets {
		t.Errorf("bind/view = %q/%q", s.BindAddr.Text(), s.View)
	}
	if s.SortColumn != "Size" || s.SortAsc {
		t.Errorf("sort = %q asc=%v", s.SortColumn, s.SortAsc)
	}
	if !s.InspectorCollapsed || !s.DecryptSwitch.Value || !s.InterceptRespSw.Value {
		t.Error("toggles not applied")
	}
	if !s.Proxy.Manual.InterceptResponses() {
		t.Error("the response-intercept flag must reach the interceptor")
	}
}

func addFlow(rig *uiRig, path string, ct string, body []byte, offset time.Duration) uint64 {
	base := time.Unix(1700000000, 0).Add(offset)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "api.example.com", Path: path,
		URL: "https://api.example.com" + path, Version: "HTTP/1.1",
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", ct}},
		RespBody:    body,
		Started:     base, Ended: base.Add(time.Millisecond),
	})
	flows := rig.s.Store.Snapshot()
	return flows[len(flows)-1].ID
}

func TestInspectorSearch_StateIsPerFlow(t *testing.T) {
	rig := newSearchRig(t)
	first := rig.s.Selected
	second := addFlow(rig, "/second.json", "application/json", bigJSONBody(50), time.Second)

	openSearch(t, rig, "MARKER-00")
	rig.s.BodySearch.NextBtn.Click()
	rig.frames(3)
	if cur, n := rig.s.BodySearch.Position(); cur != 2 || n != 100 {
		t.Fatalf("precondition: expected 2/100 on the first flow, got %d/%d", cur, n)
	}

	rig.s.Selected = second
	rig.frames(3)
	if rig.s.BodySearch.Open {
		t.Errorf("a flow that was never searched must not inherit the open panel")
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "" {
		t.Errorf("first flow's match still selected on the second flow: %q", got)
	}
	openSearch(t, rig, "MARKER-0049")
	if cur, n := rig.s.BodySearch.Position(); cur != 1 || n != 1 {
		t.Fatalf("second flow: expected 1/1, got %d/%d", cur, n)
	}

	rig.s.Selected = first
	rig.frames(3)
	if !rig.s.BodySearch.Open || rig.s.BodySearch.Editor.Text() != "MARKER-00" {
		t.Fatalf("returning to the first flow must restore its search, got open=%v query=%q", rig.s.BodySearch.Open, rig.s.BodySearch.Editor.Text())
	}
	if cur, n := rig.s.BodySearch.Position(); cur != 2 || n != 100 {
		t.Errorf("first flow must resume on 2/100, got %d/%d", cur, n)
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "MARKER-00" {
		t.Errorf("restored match must be selected, got %q", got)
	}

	rig.s.Selected = second
	rig.frames(3)
	if cur, n := rig.s.BodySearch.Position(); !rig.s.BodySearch.Open || cur != 1 || n != 1 {
		t.Errorf("second flow must resume on its own 1/1, got open=%v %d/%d", rig.s.BodySearch.Open, cur, n)
	}
}

func TestInspector_BinaryBodyShowsHexWithModes(t *testing.T) {
	rig := newSearchRig(t)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	id := addFlow(rig, "/logo.png", "image/png", png, time.Second)
	rig.s.Selected = id
	rig.frames(3)

	if got := rig.s.BodyViewer.Text(); !strings.HasPrefix(got, "00000000  89 50 4e 47 0d 0a 1a 0a  00 00 00 0d 49 48 44 52  |.PNG........IHDR|") {
		t.Fatalf("binary body must render as a hex dump in the Body pane, got %q", got)
	}
	rig.s.BodyBin.Btn(2).Click()
	rig.frames(3)
	if got := rig.s.BodyViewer.Text(); got != "iVBORw0KGgoAAAANSUhEUg==" {
		t.Errorf("Base64 chip must re-render the body, got %q", got)
	}

	rig.s.RenderMode = 0
	rig.frames(3)
	if got := rig.s.BodyViewer.Text(); !strings.Contains(got, "00000000  89 50 4e 47") || strings.Contains(got, "\x89PNG") {
		t.Errorf("Raw pane must hex-dump a binary body instead of pasting its bytes, got %q", got)
	}
}

func bigJSONBody(n int) []byte {
	var sb strings.Builder
	sb.WriteString("{\n  \"items\": [\n")
	for i := 0; i < n; i++ {
		fmt.Fprintf(&sb, "    { \"id\": %d, \"marker\": \"MARKER-%04d\" },\n", i, i)
	}
	sb.WriteString("  ]\n}\n")
	return []byte(sb.String())
}

// searchRig seeds one flow with a body long enough that a match near its end is
// far outside the first screenful, and pins a real font so the viewer's metrics
// are the same everywhere.
func newSearchRig(t *testing.T) *uiRig {
	t.Helper()
	rig := newUIRig(t, image.Pt(1200, 800))
	rig.host.Theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "api.example.com", Path: "/big.json",
		URL: "https://api.example.com/big.json", Version: "HTTP/1.1",
		ReqHeaders: [][2]string{{"Accept", "*/*"}},
		Status:     "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "application/json"}},
		RespBody:    bigJSONBody(400),
		Started:     base, Ended: base.Add(time.Millisecond),
	})
	rig.s.Selected = rig.s.Store.Snapshot()[0].ID
	rig.s.ActTab = 1 // response
	rig.s.RenderMode = 1
	rig.s.SecTab = 1 // body
	rig.frames(3)
	return rig
}

func openSearch(t *testing.T, rig *uiRig, query string) {
	t.Helper()
	rig.s.HandleSearchShortcut(rig.gtx())
	if !rig.s.BodySearch.Open {
		t.Fatal("Ctrl+F must open the search over the inspector body")
	}
	rig.s.BodySearch.Editor.SetText(query)
	rig.frames(3)
}

func TestInspectorSearch_RevealsDeepMatch(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0350")

	v := rig.s.BodyViewer
	if got := v.SelectedText(); got != "MARKER-0350" {
		t.Fatalf("search landed on %q", got)
	}
	if v.GetScrollY() <= 0 {
		t.Errorf("a match 350 entries down must scroll the viewer, scrollY = %d", v.GetScrollY())
	}
	if y, ok := v.RevealScreenY(); !ok || y < 0 || y >= rig.sz.Y {
		t.Errorf("match revealed at row %d, outside the pane", y)
	}
}

func TestInspectorSearch_FollowsRawAndHexPanes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mode  int
		query string
	}{
		{"raw", 0, "MARKER-0200"},
		{"hex", 2, "4d 41 52"}, // "MAR" in the hex dump
	} {
		t.Run(tc.name, func(t *testing.T) {
			rig := newSearchRig(t)
			rig.s.RenderMode = tc.mode
			rig.frames(3)

			openSearch(t, rig, tc.query)
			if got := rig.s.BodyViewer.SelectedText(); !strings.EqualFold(got, tc.query) {
				t.Errorf("%s pane search landed on %q, want %q", tc.name, got, tc.query)
			}
		})
	}
}

// Headers / Params / Cookies are key-value rows, not text the viewer holds.
func TestInspectorSearch_InertOnRowPanes(t *testing.T) {
	rig := newSearchRig(t)
	for _, sec := range []int{0, 2, 3} {
		rig.s.SecTab = sec
		rig.frames(2)
		rig.s.HandleSearchShortcut(rig.gtx())
		if rig.s.BodySearch.Open {
			t.Errorf("SecTab %d has no searchable text, but Ctrl+F opened the panel", sec)
			rig.s.BodySearch.Close(rig.s.BodyViewer)
		}
	}
}

func TestInspectorSearch_ReRunsWhenTheFlowChanges(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0100")
	if got := rig.s.BodyViewer.SelectedText(); got != "MARKER-0100" {
		t.Fatalf("precondition: selection = %q", got)
	}

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "other.example.com", Path: "/small",
		Status: "200 OK", StatusCode: 200,
		RespHeaders: [][2]string{{"Content-Type", "text/plain"}},
		RespBody:    []byte("nothing to find here"),
		Started:     base.Add(time.Second),
	})
	flows := rig.s.Store.Snapshot()
	rig.s.Selected = flows[len(flows)-1].ID
	rig.frames(3)

	if got := rig.s.BodyViewer.Text(); got != "nothing to find here" {
		t.Fatalf("viewer still holds the previous flow: %q", got)
	}
	if got := rig.s.BodyViewer.SelectedText(); got != "" {
		t.Errorf("stale match %q still selected after the flow changed", got)
	}
}

func TestInspectorSearch_ClosesOnABodylessFlow(t *testing.T) {
	rig := newSearchRig(t)
	openSearch(t, rig, "MARKER-0100")

	base := time.Unix(1700000000, 0)
	rig.s.Store.Add(&Flow{
		Kind: FlowHTTP, Method: "GET", Host: "empty.example.com", Path: "/none",
		Status: "204 No Content", StatusCode: 204,
		Started: base.Add(2 * time.Second),
	})
	flows := rig.s.Store.Snapshot()
	rig.s.Selected = flows[len(flows)-1].ID
	rig.frames(3)

	if rig.s.BodySearch.Open {
		t.Error("the panel must close on a flow whose pane shows no body")
	}
}

func TestSidebarSectionDefaultsOnFirstRun(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	if !rig.s.SecTargetsOpen || !rig.s.SecTLSOpen {
		t.Errorf("Targets and TLS must start expanded: targets=%v tls=%v", rig.s.SecTargetsOpen, rig.s.SecTLSOpen)
	}
	if rig.s.SecIRulesOpen || rig.s.SecMROpen || rig.s.SecScopeOpen {
		t.Errorf("the remaining sections must start collapsed: irules=%v mr=%v scope=%v",
			rig.s.SecIRulesOpen, rig.s.SecMROpen, rig.s.SecScopeOpen)
	}
}

func TestSidebarSectionStatePersistsAcrossRestart(t *testing.T) {
	rig := newUIRig(t, image.Pt(1300, 800))
	rig.sidebarFrames(2)
	rig.s.Dirty()

	rig.s.SecTargetsHdr.Click()
	rig.s.SecScopeHdr.Click()
	rig.s.InspectorToggle.Click()
	rig.sidebarFrames(2)
	rig.frames(2)

	if rig.s.SecTargetsOpen || !rig.s.SecScopeOpen {
		t.Fatalf("toggles did not land: targets=%v scope=%v", rig.s.SecTargetsOpen, rig.s.SecScopeOpen)
	}
	if !rig.s.InspectorCollapsed {
		t.Fatal("the inspector toggle did not collapse the inspector")
	}
	// The frames above already ran flushConfig, which consumes the dirty flag
	// and leaves a debounced save pending.
	if !rig.s.savePending {
		t.Fatal("toggling sections must schedule a config save")
	}
	if err := SaveConfig(rig.s.SnapshotConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	restarted := &UIState{}
	restarted.Ensure()
	t.Cleanup(func() {
		if restarted.Proxy != nil && restarted.Proxy.Running() {
			restarted.Proxy.Stop()
		}
	})
	if restarted.SecTargetsOpen {
		t.Error("a collapsed Targets section reopened after restart")
	}
	if !restarted.SecScopeOpen {
		t.Error("an expanded Scope section collapsed after restart")
	}
	if !restarted.SecTLSOpen {
		t.Error("the untouched TLS section lost its expanded state")
	}
	if !restarted.InspectorCollapsed {
		t.Error("the collapsed inspector reopened after restart")
	}
}

func TestSidebarSectionStoredFalseBeatsDefault(t *testing.T) {
	rig := newUIRig(t, image.Pt(1200, 700))
	rig.s.SecTargetsOpen = false
	rig.s.SecTLSOpen = false
	if err := SaveConfig(rig.s.SnapshotConfig()); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	restarted := &UIState{}
	restarted.Ensure()
	t.Cleanup(func() {
		if restarted.Proxy != nil && restarted.Proxy.Running() {
			restarted.Proxy.Stop()
		}
	})
	if restarted.SecTargetsOpen || restarted.SecTLSOpen {
		t.Errorf("stored collapsed sections must beat the first-run defaults: targets=%v tls=%v",
			restarted.SecTargetsOpen, restarted.SecTLSOpen)
	}
}

func serveWSEcho(c net.Conn) {
	defer func() { _ = c.Close() }()
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{})
	if err != nil {
		return
	}
	conn := res.Conn
	for {
		op, payload, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if op == ws.OpClose {
			_ = conn.WriteClose(ws.CloseNormal, "bye")
			return
		}
		if err := conn.WriteMessage(op, payload); err != nil {
			return
		}
	}
}

func TestInterceptWebSocket_EndToEnd(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	upstreamCert, err := ca.LeafFor("127.0.0.1")
	if err != nil {
		t.Fatalf("LeafFor: %v", err)
	}
	l, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{*upstreamCert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go serveWSEcho(c)
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())

	store := NewStore()
	p := NewProxy(store)
	p.SetCA(ca)
	p.SetIntercept(true)
	if err := p.Start("127.0.0.1:0"); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer p.Stop()

	caPool := x509.NewCertPool()
	caPool.AddCert(ca.Cert)
	interceptDialRoots = caPool
	defer func() { interceptDialRoots = nil }()

	proxyConn, err := net.Dial("tcp", p.Addr())
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer func() { _ = proxyConn.Close() }()

	target := net.JoinHostPort(host, port)
	if _, err := proxyConn.Write([]byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n")); err != nil {
		t.Fatalf("write CONNECT: %v", err)
	}
	pbr := bufio.NewReader(proxyConn)
	connectResp, err := http.ReadResponse(pbr, nil)
	if err != nil {
		t.Fatalf("read CONNECT response: %v", err)
	}
	if connectResp.StatusCode != 200 {
		t.Fatalf("CONNECT status = %d", connectResp.StatusCode)
	}

	tlsConn := tls.Client(proxyConn, &tls.Config{ServerName: host, RootCAs: caPool})
	if err := tlsConn.HandshakeContext(context.Background()); err != nil {
		t.Fatalf("tls handshake through proxy: %v", err)
	}

	handshake := "GET /socket HTTP/1.1\r\n" +
		"Host: " + target + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	if _, err := tlsConn.Write([]byte(handshake)); err != nil {
		t.Fatalf("write handshake: %v", err)
	}
	tbr := bufio.NewReader(tlsConn)
	upgradeResp, err := http.ReadResponse(tbr, nil)
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}
	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d, want 101", upgradeResp.StatusCode)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x1, []byte("hello-mitm"), true)); err != nil {
		t.Fatalf("write text frame: %v", err)
	}
	op, payload, _, err := readWSFrame(tbr)
	if err != nil {
		t.Fatalf("read echo frame: %v", err)
	}
	if op != 0x1 || string(payload) != "hello-mitm" {
		t.Fatalf("echo mismatch: op=%x payload=%q", op, payload)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x2, []byte{9, 8, 7}, true)); err != nil {
		t.Fatalf("write binary frame: %v", err)
	}
	if op, _, _, err := readWSFrame(tbr); err != nil || op != 0x2 {
		t.Fatalf("binary echo: op=%x err=%v", op, err)
	}

	if _, err := tlsConn.Write(mkWSFrame(0x8, []byte{0x03, 0xe8}, true)); err != nil {
		t.Fatalf("write close: %v", err)
	}
	_ = tlsConn.Close()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p.WS.Len() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	msgs := p.WS.Snapshot()
	if len(msgs) < 2 {
		t.Fatalf("expected captured WS frames, got %d", len(msgs))
	}

	var sawText, sawToServer, sawFromServer bool
	for _, m := range msgs {
		if m.Opcode == 0x1 && string(m.Payload) == "hello-mitm" {
			sawText = true
		}
		if m.ToServer {
			sawToServer = true
		} else {
			sawFromServer = true
		}
		if m.URL == "" || !strings.HasPrefix(m.URL, "wss://") {
			t.Errorf("captured frame has a bad URL: %q", m.URL)
		}
	}
	if !sawText {
		t.Errorf("the intercepted text frame was not captured: %+v", msgs)
	}
	if !sawToServer || !sawFromServer {
		t.Errorf("expected both directions captured: toServer=%v fromServer=%v", sawToServer, sawFromServer)
	}

	deadline = time.Now().Add(2 * time.Second)
	var wsFlow *Flow
	for time.Now().Before(deadline) {
		for _, f := range store.Snapshot() {
			if f.WebSocket {
				wsFlow = f
			}
		}
		if wsFlow != nil && wsFlow.TunnelClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if wsFlow == nil {
		t.Fatalf("no WebSocket flow recorded")
	}
	if wsFlow.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("ws flow status = %d, want 101", wsFlow.StatusCode)
	}
	if wsFlow.Method != "WS" {
		t.Errorf("ws flow method = %q, want WS", wsFlow.Method)
	}
}
