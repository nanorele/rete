package mitm

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

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
	req.Header.Set("X-From", "tracto")
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

	// Give the proxy goroutine a moment to record the response.
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

	// Hit the proxy directly (browser-style: origin-form request, no
	// absolute URL). This must not be forwarded back to ourselves.
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Get("http://" + p.Addr() + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("status = %d, want 421", resp.StatusCode)
	}

	// Even with proxy configured, an absolute URL pointing at the proxy
	// itself must be refused rather than looped back.
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
	// Upstream that holds the connection open. The tunnel will only end
	// when one side actually closes.
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
			// Read forever, but never close until peer closes.
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

	// Open a CONNECT tunnel and keep it alive.
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

	// Ended must be stamped at handshake completion (NOT bridge exit),
	// otherwise the inspector would tick the timer for the entire
	// keep-alive lifetime of the TCP tunnel — which is unrelated to
	// the request actually being done.
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

	// Stop must terminate the tunnel and flip TunnelClosed.
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
