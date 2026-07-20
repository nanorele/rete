package mitm

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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
