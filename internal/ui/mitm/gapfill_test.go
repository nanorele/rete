package mitm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tracto/internal/persist"
)

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

func TestTractoTrustPool(t *testing.T) {
	setupTestConfigDir(t)
	if pool := TractoTrustPool(); pool != nil {
		t.Fatal("no CA on disk must yield a nil pool")
	}

	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	if err := ca.Save(MITMDir()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if pool := TractoTrustPool(); pool == nil {
		t.Fatal("a saved CA must produce a pool")
	}

	if err := os.WriteFile(CACertPath(MITMDir()), []byte("not a pem"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if pool := TractoTrustPool(); pool != nil {
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
