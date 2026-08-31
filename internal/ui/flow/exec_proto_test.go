package flow

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"tracto/internal/ws"
)

func TestRunGQL(t *testing.T) {
	var gotBody, gotCT, gotMethod atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		gotBody.Store(string(data))
		gotCT.Store(r.Header.Get("Content-Type"))
		gotMethod.Store(r.Method)
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer srv.Close()

	t.Run("query with variables", func(t *testing.T) {
		n := &execNode{
			kind:    KindGQLRequest,
			url:     srv.URL,
			body:    "query($id: Int) { user(id: $id) { name } }",
			gqlVars: `{"id": {{uid}}}`,
			env:     map[string]string{"uid": "7"},
		}
		res := runGQL(context.Background(), n, nil)
		if !res.hasResp || res.status != 200 || res.failed {
			t.Fatalf("unexpected result: %+v", res)
		}
		if gotMethod.Load() != "POST" {
			t.Errorf("method = %v, want POST", gotMethod.Load())
		}
		if gotCT.Load() != "application/json" {
			t.Errorf("content-type = %v", gotCT.Load())
		}
		var payload struct {
			Query     string          `json:"query"`
			Variables json.RawMessage `json:"variables"`
		}
		if err := json.Unmarshal([]byte(gotBody.Load().(string)), &payload); err != nil {
			t.Fatalf("payload not JSON: %v", err)
		}
		if !strings.Contains(payload.Query, "user(id: $id)") {
			t.Errorf("query = %q", payload.Query)
		}
		if string(payload.Variables) != `{"id":7}` {
			t.Errorf("variables = %s", payload.Variables)
		}
	})

	t.Run("invalid variables fail", func(t *testing.T) {
		n := &execNode{kind: KindGQLRequest, url: srv.URL, body: "query { x }", gqlVars: "{oops"}
		res := runGQL(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "invalid JSON") {
			t.Fatalf("expected invalid-JSON failure, got %+v", res)
		}
	})
}

func startFlowEchoWS(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				br := bufio.NewReader(c)
				req, err := http.ReadRequest(br)
				if err != nil {
					_ = c.Close()
					return
				}
				res, err := ws.Upgrade(c, br, req, ws.UpgradeOptions{Subprotocols: []string{"echo-proto"}})
				if err != nil {
					_ = c.Close()
					return
				}
				conn := res.Conn
				for {
					op, payload, err := conn.ReadMessage()
					if err != nil {
						return
					}
					if op == ws.OpText || op == ws.OpBinary {
						_ = conn.WriteMessage(op, payload)
					}
				}
			}(c)
		}
	}()
	return "ws://" + l.Addr().String()
}

func TestRunWS(t *testing.T) {
	url := startFlowEchoWS(t)

	t.Run("send and collect echo", func(t *testing.T) {
		n := &execNode{
			kind:      KindWSRequest,
			url:       url,
			body:      `{"msg":"{{word}}"}`,
			wsOpcode:  "TEXT",
			waitMs:    400,
			subprotos: []string{"echo-proto"},
			env:       map[string]string{"word": "hi"},
		}
		res := runWS(context.Background(), n, nil)
		if !res.hasResp || res.failed {
			t.Fatalf("unexpected result: %+v (%s)", res, res.errMsg)
		}
		if res.status != 101 {
			t.Errorf("status = %d, want 101", res.status)
		}
		if string(res.body) != `{"msg":"hi"}` {
			t.Errorf("collected body = %q", res.body)
		}
	})

	t.Run("binary opcode expects hex", func(t *testing.T) {
		n := &execNode{kind: KindWSRequest, url: url, body: "zz-not-hex", wsOpcode: "BIN", waitMs: 100}
		res := runWS(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "hex payload") {
			t.Fatalf("expected hex failure, got %+v", res)
		}
	})

	t.Run("refused connection fails", func(t *testing.T) {
		n := &execNode{kind: KindWSRequest, url: "ws://127.0.0.1:1", waitMs: 50}
		res := runWS(context.Background(), n, nil)
		if !res.failed {
			t.Fatalf("expected failure, got %+v", res)
		}
	})
}

func TestRunHTTPBodyTypesAuthCookies(t *testing.T) {
	type seen struct {
		ct     string
		body   string
		auth   string
		cookie string
	}
	var last atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		last.Store(seen{
			ct:     r.Header.Get("Content-Type"),
			body:   string(data),
			auth:   r.Header.Get("Authorization"),
			cookie: r.Header.Get("Cookie"),
		})
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	t.Run("urlencoded", func(t *testing.T) {
		n := &execNode{
			method:   "POST",
			url:      srv.URL,
			bodyType: "urlencoded",
			body:     "a=1\nb=two words\n\n=skipped",
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", s.ct)
		}
		if !strings.Contains(s.body, "a=1") || !strings.Contains(s.body, "b=two+words") {
			t.Errorf("body = %q", s.body)
		}
	})

	t.Run("form with file", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "part.txt")
		if err := os.WriteFile(fp, []byte("FILEDATA"), 0o644); err != nil {
			t.Fatal(err)
		}
		n := &execNode{
			method:   "POST",
			url:      srv.URL,
			bodyType: "form",
			body:     "field=val\nupload=@" + fp,
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		mt, params, err := mime.ParseMediaType(s.ct)
		if err != nil || mt != "multipart/form-data" {
			t.Fatalf("content-type = %q (%v)", s.ct, err)
		}
		mr := multipart.NewReader(strings.NewReader(s.body), params["boundary"])
		got := map[string]string{}
		for {
			p, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(p)
			got[p.FormName()] = string(data)
		}
		if got["field"] != "val" || got["upload"] != "FILEDATA" {
			t.Errorf("parts = %v", got)
		}
	})

	t.Run("binary", func(t *testing.T) {
		dir := t.TempDir()
		fp := filepath.Join(dir, "raw.bin")
		if err := os.WriteFile(fp, []byte{1, 2, 3}, 0o644); err != nil {
			t.Fatal(err)
		}
		n := &execNode{method: "POST", url: srv.URL, bodyType: "binary", binPath: fp}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.ct != "application/octet-stream" || s.body != "\x01\x02\x03" {
			t.Errorf("ct=%q body=%q", s.ct, s.body)
		}
	})

	t.Run("missing binary file fails", func(t *testing.T) {
		n := &execNode{method: "POST", url: srv.URL, bodyType: "binary", binPath: "no/such/file.bin"}
		res := runHTTP(context.Background(), n, nil)
		if !res.failed || !strings.Contains(res.errMsg, "binary body") {
			t.Fatalf("expected binary failure, got %+v", res)
		}
	})

	t.Run("bearer auth and cookies", func(t *testing.T) {
		n := &execNode{
			method:    "GET",
			url:       srv.URL,
			authType:  "bearer",
			authToken: "{{tok}}",
			cookies:   [][2]string{{"sid", "abc"}, {"theme", "dark"}},
			env:       map[string]string{"tok": "T123"},
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		s := last.Load().(seen)
		if s.auth != "Bearer T123" {
			t.Errorf("auth = %q", s.auth)
		}
		if s.cookie != "sid=abc; theme=dark" {
			t.Errorf("cookie = %q", s.cookie)
		}
	})

	t.Run("basic auth does not override explicit header", func(t *testing.T) {
		n := &execNode{
			method:   "GET",
			url:      srv.URL,
			headers:  [][2]string{{"Authorization", "custom"}},
			authType: "basic",
			authUser: "u",
			authPass: "p",
		}
		res := runHTTP(context.Background(), n, nil)
		if res.failed {
			t.Fatalf("failed: %+v", res)
		}
		if s := last.Load().(seen); s.auth != "custom" {
			t.Errorf("auth = %q, explicit header must win", s.auth)
		}
	})
}

func TestNodeDTONewFieldsRoundTrip(t *testing.T) {
	n := NewNode(KindWSRequest, 10, 20)
	n.URLEd.SetText("wss://x/y")
	n.HeadersEd.SetText("Origin: http://x")
	n.BodyEd.SetText("ping")
	n.SubprotosEd.SetText("a, b")
	n.WSOpcode = "BIN"
	n.WaitMsEd.SetText("250")
	n.InsecureTLS = true
	n.AuthType = "bearer"
	n.AuthTokenEd.SetText("tok")
	n.CookiesEd.SetText("k=v")

	back := nodeFromDTO(nodeToDTO(n))
	if back.Kind != KindWSRequest {
		t.Fatalf("kind = %v", back.Kind)
	}
	if back.SubprotosEd.Text() != "a, b" || back.WSOpcode != "BIN" || back.WaitMsEd.Text() != "250" {
		t.Errorf("ws fields lost: %q %q %q", back.SubprotosEd.Text(), back.WSOpcode, back.WaitMsEd.Text())
	}
	if !back.InsecureTLS || back.AuthType != "bearer" || back.AuthTokenEd.Text() != "tok" || back.CookiesEd.Text() != "k=v" {
		t.Errorf("auth/cookie fields lost")
	}

	g := NewNode(KindGQLRequest, 0, 0)
	g.BodyEd.SetText("query { x }")
	g.VarsEd.SetText(`{"a":1}`)
	gb := nodeFromDTO(nodeToDTO(g))
	if gb.Kind != KindGQLRequest || gb.VarsEd.Text() != `{"a":1}` {
		t.Errorf("gql fields lost: kind=%v vars=%q", gb.Kind, gb.VarsEd.Text())
	}

	h := NewNode(KindRequest, 0, 0)
	h.BodyType = "form"
	h.BinPathEd.SetText("c:/f.bin")
	hb := nodeFromDTO(nodeToDTO(h))
	if hb.BodyType != "form" || hb.BinPathEd.Text() != "c:/f.bin" {
		t.Errorf("http fields lost: %q %q", hb.BodyType, hb.BinPathEd.Text())
	}
}

func TestAddRequestNodeFromTab(t *testing.T) {
	setupFlowConfig(t)
	ed := NewEditor()

	n := ed.AddRequestNode(TabRequest{
		Name:      "My WS",
		Kind:      KindWSRequest,
		URL:       "wss://srv/sock",
		Headers:   [][2]string{{"Origin", "http://x"}},
		Cookies:   [][2]string{{"sid", "1"}},
		AuthType:  "bearer",
		AuthToken: "tok",
		Subprotos: []string{"p1", "p2"},
		WSMessage: "hello",
		WSOpcode:  "TEXT",
	})
	if n.Kind != KindWSRequest || n.URLEd.Text() != "wss://srv/sock" {
		t.Fatalf("node = %v %q", n.Kind, n.URLEd.Text())
	}
	if n.SubprotosEd.Text() != "p1, p2" || n.BodyEd.Text() != "hello" {
		t.Errorf("ws data lost: %q %q", n.SubprotosEd.Text(), n.BodyEd.Text())
	}
	if n.HeadersEd.Text() != "Origin: http://x" || n.CookiesEd.Text() != "sid=1" || n.AuthType != "bearer" {
		t.Errorf("headers/cookies/auth lost")
	}
	if ed.Scenario.NodeByID(n.ID) == nil {
		t.Error("node not attached to the scenario")
	}

	h := ed.AddRequestNode(TabRequest{
		Name:      "Form req",
		Kind:      KindRequest,
		Method:    "POST",
		URL:       "http://x/upload",
		BodyType:  "form",
		FormParts: []TabFormPart{{Key: "f", Value: "v"}, {Key: "file", IsFile: true, FilePath: "c:/a.png"}},
	})
	if h.BodyType != "form" || h.BodyEd.Text() != "f=v\nfile=@c:/a.png" {
		t.Errorf("form body = %q (%q)", h.BodyEd.Text(), h.BodyType)
	}

	g := ed.AddRequestNode(TabRequest{
		Kind:     KindGQLRequest,
		URL:      "http://x/graphql",
		GQLQuery: "query { me }",
		GQLVars:  `{"a":2}`,
	})
	if g.BodyEd.Text() != "query { me }" || g.VarsEd.Text() != `{"a":2}` {
		t.Errorf("gql node = %q %q", g.BodyEd.Text(), g.VarsEd.Text())
	}
	if len(ed.Scenario.Nodes) < 4 {
		t.Errorf("scenario has %d nodes", len(ed.Scenario.Nodes))
	}
}
