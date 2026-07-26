package collections

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"tracto/internal/model"
)

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestParseExampleRawFull(t *testing.T) {
	raw := json.RawMessage(`{
		"name":"Success case",
		"status":"OK",
		"code":200,
		"body":"{\"ok\":true}",
		"_postman_previewlanguage":"json",
		"originalRequest":{
			"method":"POST",
			"url":"http://example.com/api",
			"header":[{"key":"Accept","value":"application/json"}],
			"body":{"mode":"raw","raw":"payload"}
		}
	}`)
	ex := parseExampleRaw(raw)
	if ex == nil {
		t.Fatal("nil example")
	}
	if ex.Name != "Success case" {
		t.Errorf("name: got %q", ex.Name)
	}
	if ex.Status != "OK" {
		t.Errorf("status: got %q", ex.Status)
	}
	if ex.Code != 200 {
		t.Errorf("code: got %d", ex.Code)
	}
	if ex.RespBody != `{"ok":true}` {
		t.Errorf("resp body: got %q", ex.RespBody)
	}
	if ex.Method != "POST" {
		t.Errorf("method: got %q", ex.Method)
	}
	if ex.URL != "http://example.com/api" {
		t.Errorf("url: got %q", ex.URL)
	}
	if ex.Body != "payload" {
		t.Errorf("body: got %q", ex.Body)
	}
	if ex.BodyType != model.BodyRaw {
		t.Errorf("body type: got %v", ex.BodyType)
	}
	if ex.Headers["Accept"] != "application/json" {
		t.Errorf("headers: got %v", ex.Headers)
	}
}

func TestParseExampleRawOriginalRequestBodyKinds(t *testing.T) {
	t.Run("formdata", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"f","originalRequest":{"method":"POST","url":"u","body":{"mode":"formdata","formdata":[{"key":"a","value":"1"}]}}}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if len(ex.FormParts) != 1 || ex.FormParts[0].Key != "a" {
			t.Errorf("form parts: %+v", ex.FormParts)
		}
	})
	t.Run("urlencoded", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"u","originalRequest":{"method":"POST","url":"u","body":{"mode":"urlencoded","urlencoded":[{"key":"k","value":"v"}]}}}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if len(ex.URLEncoded) != 1 || ex.URLEncoded[0].Key != "k" {
			t.Errorf("urlencoded: %+v", ex.URLEncoded)
		}
	})
	t.Run("binary", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"b","originalRequest":{"method":"POST","url":"u","body":{"mode":"file","file":{"src":"/p/x.bin"}}}}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.BinaryPath != "/p/x.bin" {
			t.Errorf("binary path: %q", ex.BinaryPath)
		}
	})
}

func TestParseExampleRawNameFallback(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"no name with code", `{"code":404,"body":"nope"}`, "Example 404"},
		{"no name no code", `{"body":"x"}`, "Example"},
		{"empty name with code", `{"name":"","code":500,"body":"x"}`, "Example 500"},
		{"zero code", `{"code":0,"body":"x"}`, "Example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ex := parseExampleRaw(json.RawMessage(c.raw))
			if ex == nil {
				t.Fatal("nil example")
			}
			if ex.Name != c.want {
				t.Errorf("got %q want %q", ex.Name, c.want)
			}
		})
	}
}

func TestParseExampleRawOriginalRequestVariants(t *testing.T) {
	t.Run("null", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"n","originalRequest":null,"code":201,"body":"b"}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.Method != "" || ex.URL != "" {
			t.Errorf("null originalRequest must not populate request fields: %+v", ex)
		}
		if ex.Headers == nil {
			t.Errorf("headers map must stay non-nil")
		}
	})
	t.Run("absent", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"n","code":201,"body":"b"}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.Method != "" {
			t.Errorf("method: got %q", ex.Method)
		}
	})
	t.Run("bare string", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"n","originalRequest":"http://example.com/str"}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.URL != "http://example.com/str" {
			t.Errorf("url: got %q", ex.URL)
		}
		if ex.Method != "GET" {
			t.Errorf("method: got %q", ex.Method)
		}
	})
	t.Run("unparseable scalar", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"n","originalRequest":42}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.Method != "" || ex.URL != "" {
			t.Errorf("expected untouched request fields, got %+v", ex)
		}
	})
	t.Run("empty object literal", func(t *testing.T) {
		ex := parseExampleRaw(json.RawMessage(`{"name":"n","originalRequest":{}}`))
		if ex == nil {
			t.Fatal("nil example")
		}
		if ex.Method != "GET" {
			t.Errorf("empty originalRequest should yield default GET, got %q", ex.Method)
		}
	})
}

func TestParseExampleRawInvalid(t *testing.T) {
	cases := []string{`"bare"`, `42`, `[1,2]`, `true`, `not-json`}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if ex := parseExampleRaw(json.RawMessage(c)); ex != nil {
				t.Errorf("expected nil, got %+v", ex)
			}
		})
	}
}

func TestParseExamplesFromResponseArray(t *testing.T) {
	raw := json.RawMessage(`{
		"name":"r",
		"request":{"method":"GET","url":"http://x"},
		"response":[
			{"name":"ok","status":"OK","code":200,"body":"yes","originalRequest":{"method":"GET","url":"http://x"}},
			{"name":"missing","status":"Not Found","code":404,"body":"no"}
		]
	}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected request node, got %+v", node)
	}
	if len(node.Request.Examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(node.Request.Examples))
	}
	if node.Request.Examples[0].Name != "ok" || node.Request.Examples[0].Code != 200 {
		t.Errorf("example 0: %+v", node.Request.Examples[0])
	}
	if node.Request.Examples[1].Name != "missing" || node.Request.Examples[1].Status != "Not Found" {
		t.Errorf("example 1: %+v", node.Request.Examples[1])
	}
	if _, ok := node.Extras["response"]; !ok {
		t.Errorf("response must be preserved in Extras for round-trip")
	}
}

func TestParseExamplesFromNestedItems(t *testing.T) {
	raw := json.RawMessage(`{
		"name":"r",
		"request":{"method":"GET","url":"http://x"},
		"item":[
			{"name":"nested example","_postman_previewlanguage":"json","code":200,"body":"hi"},
			{"name":"plain child","request":{"method":"POST","url":"http://y"}},
			{"name":"case","_apidog_type":"case","body":"c"}
		]
	}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected request node, got %+v", node)
	}
	if node.IsFolder {
		t.Errorf("request present: node must not become a folder")
	}
	if len(node.Request.Examples) != 2 {
		t.Fatalf("expected 2 examples from nested item array, got %d", len(node.Request.Examples))
	}
	if node.Request.Examples[0].Name != "nested example" {
		t.Errorf("example 0: %+v", node.Request.Examples[0])
	}
	if node.Request.Examples[1].Name != "case" {
		t.Errorf("example 1: %+v", node.Request.Examples[1])
	}
}

func TestParseExamplesResponseAndItemCombined(t *testing.T) {
	raw := json.RawMessage(`{
		"name":"r",
		"request":{"method":"GET","url":"http://x"},
		"response":[{"name":"from-response","code":200,"body":"a"}],
		"item":[{"name":"from-item","originalRequest":{},"code":201,"body":"b"}]
	}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected request node")
	}
	if len(node.Request.Examples) != 2 {
		t.Fatalf("expected 2 examples, got %d", len(node.Request.Examples))
	}
	if node.Request.Examples[0].Name != "from-response" {
		t.Errorf("response examples must come first, got %q", node.Request.Examples[0].Name)
	}
	if node.Request.Examples[1].Name != "from-item" {
		t.Errorf("example 1: %q", node.Request.Examples[1].Name)
	}
}

func TestParseExamplesNonArrayShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"response object", `{"name":"r","request":{"method":"GET","url":"u"},"response":{"code":200}}`},
		{"response string", `{"name":"r","request":{"method":"GET","url":"u"},"response":"nope"}`},
		{"response null", `{"name":"r","request":{"method":"GET","url":"u"},"response":null}`},
		{"response empty array", `{"name":"r","request":{"method":"GET","url":"u"},"response":[]}`},
		{"item not array", `{"name":"r","request":{"method":"GET","url":"u"},"item":"nope"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			node := parseItemRaw(json.RawMessage(c.raw), 1)
			if node == nil || node.Request == nil {
				t.Fatalf("expected request node, got %+v", node)
			}
			if node.Request.Examples != nil {
				t.Errorf("expected nil examples, got %+v", node.Request.Examples)
			}
		})
	}
}

func TestParseExamplesAllEntriesMalformed(t *testing.T) {
	raw := json.RawMessage(`{"name":"r","request":{"method":"GET","url":"u"},"response":[1,"x",true,[2]]}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected request node")
	}
	if node.Request.Examples != nil {
		t.Errorf("all-malformed response array must yield nil examples, got %+v", node.Request.Examples)
	}
}

func TestParseExamplesPartiallyMalformed(t *testing.T) {
	raw := json.RawMessage(`{"name":"r","request":{"method":"GET","url":"u"},"response":[1,{"name":"good","code":200,"body":"b"},"x"]}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected request node")
	}
	if len(node.Request.Examples) != 1 || node.Request.Examples[0].Name != "good" {
		t.Errorf("expected only the valid example, got %+v", node.Request.Examples)
	}
}

func TestParseExamplesOnlyForRequestNodes(t *testing.T) {
	raw := json.RawMessage(`{"name":"f","item":[{"name":"ex","originalRequest":{},"code":200,"body":"b"}]}`)
	node := parseItemRaw(raw, 1)
	if node == nil {
		t.Fatal("nil node")
	}
	if !node.IsFolder {
		t.Errorf("expected folder")
	}
	if node.Request != nil {
		t.Errorf("folder must not gain a request")
	}
	if len(node.skippedItems) != 1 {
		t.Errorf("expected example kept as skipped item, got %d", len(node.skippedItems))
	}
}

func TestParseAuthNoauth(t *testing.T) {
	a, ok := parseAuth(json.RawMessage(`{"type":"noauth"}`))
	if !ok {
		t.Fatal("noauth must be recognised")
	}
	if a != (model.ParsedAuth{}) {
		t.Errorf("expected zero auth, got %+v", a)
	}
}

func TestParseAuthNonObject(t *testing.T) {
	cases := []string{`"bearer"`, `42`, `[{"type":"bearer"}]`, `not-json`}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			a, ok := parseAuth(json.RawMessage(c))
			if ok {
				t.Errorf("expected ok=false, got %+v", a)
			}
			if a != (model.ParsedAuth{}) {
				t.Errorf("expected zero auth, got %+v", a)
			}
		})
	}
}

func TestParseAuthUnknownTypesRejected(t *testing.T) {
	cases := []string{
		`{"type":"apikey","apikey":[{"key":"key","value":"X-Api"}]}`,
		`{"type":"oauth1"}`,
		`{"type":""}`,
		`{}`,
		`{"type":123}`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if _, ok := parseAuth(json.RawMessage(c)); ok {
				t.Errorf("expected ok=false for %s", c)
			}
		})
	}
}

func TestParseRequestRawApikeyAuthGoesToExtras(t *testing.T) {
	raw := json.RawMessage(`{"method":"GET","url":"http://x","auth":{"type":"apikey","apikey":[{"key":"key","value":"X-Api"}]}}`)
	req := parseRequestRaw(raw, "n")
	if req == nil {
		t.Fatal("nil req")
	}
	if req.Auth.Type != "" {
		t.Errorf("apikey must not populate typed Auth, got %+v", req.Auth)
	}
	if _, ok := req.Extras["auth"]; !ok {
		t.Errorf("apikey auth must be preserved in Extras")
	}
}

func TestAuthParamValueEdgeCases(t *testing.T) {
	t.Run("not an array", func(t *testing.T) {
		if got := authParamValue(json.RawMessage(`{"token":"abc"}`), "token"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("nil raw", func(t *testing.T) {
		if got := authParamValue(nil, "token"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("key missing", func(t *testing.T) {
		if got := authParamValue(json.RawMessage(`[{"key":"other","value":"v"}]`), "token"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("empty array", func(t *testing.T) {
		if got := authParamValue(json.RawMessage(`[]`), "token"); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("first match wins", func(t *testing.T) {
		got := authParamValue(json.RawMessage(`[{"key":"token","value":"one"},{"key":"token","value":"two"}]`), "token")
		if got != "one" {
			t.Errorf("got %q", got)
		}
	})
}

func TestParseAuthBearerWithMissingParams(t *testing.T) {
	a, ok := parseAuth(json.RawMessage(`{"type":"bearer"}`))
	if !ok {
		t.Fatal("bearer must be recognised")
	}
	if a.Type != "bearer" || a.Token != "" {
		t.Errorf("got %+v", a)
	}
}

func TestParseAuthBasicWithPartialParams(t *testing.T) {
	a, ok := parseAuth(json.RawMessage(`{"type":"basic","basic":[{"key":"username","value":"u"}]}`))
	if !ok {
		t.Fatal("basic must be recognised")
	}
	if a.Username != "u" || a.Password != "" {
		t.Errorf("got %+v", a)
	}
}

func TestParseCookiesNonArray(t *testing.T) {
	cases := []string{`{"sid":"abc"}`, `"sid=abc"`, `42`, `not-json`}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			if got := parseCookies(json.RawMessage(c)); got != nil {
				t.Errorf("expected nil, got %+v", got)
			}
		})
	}
}

func TestParseCookiesBlankKeysDropped(t *testing.T) {
	raw := json.RawMessage(`[{"key":"sid","value":"abc"},{"key":"","value":"x"},{"key":"   ","value":"y"},{"key":"  t  ","value":"dark"}]`)
	got := parseCookies(raw)
	if len(got) != 2 {
		t.Fatalf("expected 2 cookies, got %d: %+v", len(got), got)
	}
	if got[0].Key != "sid" || got[0].Value != "abc" {
		t.Errorf("cookie 0: %+v", got[0])
	}
	if got[1].Key != "t" || got[1].Value != "dark" {
		t.Errorf("cookie 1 key must be trimmed: %+v", got[1])
	}
}

func TestParseCookiesEmptyArray(t *testing.T) {
	got := parseCookies(json.RawMessage(`[]`))
	if len(got) != 0 {
		t.Errorf("expected no cookies, got %+v", got)
	}
}

func TestParseItemRawUnknownKeysPreserved(t *testing.T) {
	raw := json.RawMessage(`{"name":"r","description":"hello","event":[{"listen":"test"}],"protocolProfileBehavior":{"x":1},"request":{"method":"GET","url":"u"}}`)
	node := parseItemRaw(raw, 1)
	if node == nil {
		t.Fatal("nil node")
	}
	for _, k := range []string{"description", "event", "protocolProfileBehavior"} {
		if _, ok := node.Extras[k]; !ok {
			t.Errorf("key %q lost from Extras", k)
		}
	}
	if _, ok := node.Extras["name"]; ok {
		t.Errorf("name must not be duplicated into Extras")
	}
}

func TestParseItemRawItemKeyNotArray(t *testing.T) {
	node := parseItemRaw(json.RawMessage(`{"name":"weird","item":"not-an-array"}`), 1)
	if node == nil {
		t.Fatal("nil node")
	}
	if !node.IsFolder {
		t.Errorf("item key present but unparseable must still coerce to folder, got %+v", node)
	}
	if node.Request != nil {
		t.Errorf("expected no request")
	}
	if node.Name != "weird" {
		t.Errorf("name lost: %q", node.Name)
	}
}

func TestParseItemRawRequestNullBecomesFolder(t *testing.T) {
	node := parseItemRaw(json.RawMessage(`{"name":"n","request":null}`), 1)
	if node == nil {
		t.Fatal("nil node")
	}
	if node.Request == nil {
		t.Fatalf("request key present must yield a default request, got %+v", node)
	}
	if node.Request.Method != "GET" {
		t.Errorf("method: %q", node.Request.Method)
	}
}

func TestParseRequestRawUnknownKeysPreserved(t *testing.T) {
	raw := json.RawMessage(`{"method":"GET","url":"http://x","description":"d","proxy":{"host":"p"},"certificate":{"name":"c"}}`)
	req := parseRequestRaw(raw, "n")
	if req == nil {
		t.Fatal("nil req")
	}
	for _, k := range []string{"description", "proxy", "certificate"} {
		if _, ok := req.Extras[k]; !ok {
			t.Errorf("key %q lost from request Extras", k)
		}
	}
	if _, ok := req.Extras["method"]; ok {
		t.Errorf("method must not leak into Extras")
	}
}

func TestParseRequestRawEmptyMethodKeepsGET(t *testing.T) {
	req := parseRequestRaw(json.RawMessage(`{"method":"","url":"http://x"}`), "n")
	if req == nil {
		t.Fatal("nil req")
	}
	if req.Method != "GET" {
		t.Errorf("empty method must fall back to GET, got %q", req.Method)
	}
}

func TestParseCollectionReadError(t *testing.T) {
	col, err := ParseCollection(failingReader{}, "id")
	if err == nil {
		t.Fatal("expected read error")
	}
	if col != nil {
		t.Errorf("expected nil collection, got %+v", col)
	}
}

func TestCloneNodeCopiesCookies(t *testing.T) {
	node := &CollectionNode{
		Name: "r",
		Request: &model.ParsedRequest{
			Name:    "r",
			Method:  "GET",
			Headers: map[string]string{},
			Cookies: []model.ParsedKV{{Key: "sid", Value: "abc"}, {Key: "t", Value: "dark"}},
		},
	}
	cl := CloneNode(node, nil)
	if len(cl.Request.Cookies) != 2 {
		t.Fatalf("expected 2 cookies copied, got %d", len(cl.Request.Cookies))
	}
	node.Request.Cookies[0].Value = "MUT"
	if cl.Request.Cookies[0].Value != "abc" {
		t.Errorf("cookies not deep-copied: %+v", cl.Request.Cookies[0])
	}
}

func TestCloneNodeRequestWithExamplesAndNoCookies(t *testing.T) {
	node := &CollectionNode{
		Name: "r",
		Request: &model.ParsedRequest{
			Name:     "r",
			Method:   "GET",
			Headers:  map[string]string{},
			Examples: []model.ParsedExample{{Name: "e", Code: 200}},
		},
	}
	cl := CloneNode(node, nil)
	if len(cl.Request.Cookies) != 0 {
		t.Errorf("expected no cookies, got %+v", cl.Request.Cookies)
	}
	if len(cl.Request.Examples) != 1 || cl.Request.Examples[0].Code != 200 {
		t.Errorf("examples: %+v", cl.Request.Examples)
	}
}

func TestMarshalCollectionPreservesRootSkippedItems(t *testing.T) {
	js := `{"info":{"name":"C"},"item":[
		{"name":"r","request":{"method":"GET","url":"http://x"}},
		{"name":"orphan example","originalRequest":{},"code":200,"body":"hi"}
	]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Root.skippedItems) != 1 {
		t.Fatalf("expected 1 skipped item, got %d", len(col.Root.skippedItems))
	}
	_, data := Snapshot(col)
	if !bytes.Contains(data, []byte("orphan example")) {
		t.Errorf("root skipped item lost on save: %s", data)
	}

	col2, err := ParseCollection(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(col2.Root.Children) != 1 {
		t.Errorf("children: got %d", len(col2.Root.Children))
	}
	if len(col2.Root.skippedItems) != 1 {
		t.Errorf("skipped items lost across round-trip: got %d", len(col2.Root.skippedItems))
	}
}

func TestMarshalCollectionPreservesTopExtras(t *testing.T) {
	js := `{
		"info":{"name":"C","_postman_id":"pid","schema":"v2.1"},
		"item":[{"name":"r","request":{"method":"GET","url":"http://x"}}],
		"variable":[{"key":"base","value":"http://x"}],
		"event":[{"listen":"prerequest"}],
		"auth":{"type":"bearer","bearer":[{"key":"token","value":"tok"}]}
	}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	_, data := Snapshot(col)
	for _, want := range []string{"variable", "base", "prerequest", "_postman_id", "schema"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("top-level %q lost on save: %s", want, data)
		}
	}

	col2, err := ParseCollection(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	for _, k := range []string{"variable", "event", "auth"} {
		if _, ok := col2.TopExtras[k]; !ok {
			t.Errorf("TopExtras %q lost across round-trip", k)
		}
	}
	for _, k := range []string{"_postman_id", "schema"} {
		if _, ok := col2.InfoExtras[k]; !ok {
			t.Errorf("InfoExtras %q lost across round-trip", k)
		}
	}
}

func TestMarshalNodePreservesNodeExtras(t *testing.T) {
	node := parseItemRaw(json.RawMessage(`{"name":"r","description":"d","event":[{"listen":"test"}],"request":{"method":"GET","url":"http://x"}}`), 1)
	if node == nil {
		t.Fatal("nil node")
	}
	out := marshalNode(node)
	if out["name"] != "r" {
		t.Errorf("name: %v", out["name"])
	}
	if _, ok := out["description"]; !ok {
		t.Errorf("description lost")
	}
	if _, ok := out["event"]; !ok {
		t.Errorf("event lost")
	}
	if _, ok := out["request"]; !ok {
		t.Errorf("request lost")
	}
}

func TestMarshalNodeFolderExtrasAndSkipped(t *testing.T) {
	folder := &CollectionNode{
		Name:     "f",
		IsFolder: true,
		Extras: map[string]json.RawMessage{
			"description": json.RawMessage(`"folder desc"`),
		},
		skippedItems: []json.RawMessage{json.RawMessage(`{"originalRequest":{},"code":200,"body":"b"}`)},
		Children: []*CollectionNode{
			{Name: "c", Request: &model.ParsedRequest{Name: "c", Method: "GET", URL: "http://x", Headers: map[string]string{}}},
		},
	}
	out := marshalNode(folder)
	if _, ok := out["description"]; !ok {
		t.Errorf("folder extras lost")
	}
	items, ok := out["item"].([]any)
	if !ok {
		t.Fatalf("item: got %T", out["item"])
	}
	if len(items) != 2 {
		t.Fatalf("expected child + skipped, got %d", len(items))
	}
}

func TestMarshalNodeFolderWithRequestIgnoresRequest(t *testing.T) {
	node := &CollectionNode{
		Name:     "f",
		IsFolder: true,
		Request:  &model.ParsedRequest{Name: "f", Method: "GET", Headers: map[string]string{}},
	}
	out := marshalNode(node)
	if _, ok := out["request"]; ok {
		t.Errorf("folder must not emit a request key")
	}
	if _, ok := out["item"]; !ok {
		t.Errorf("folder must emit an item key")
	}
}

func TestMarshalNodeLeafWithoutRequest(t *testing.T) {
	out := marshalNode(&CollectionNode{Name: "n"})
	if _, ok := out["request"]; ok {
		t.Errorf("expected no request key")
	}
	if _, ok := out["item"]; ok {
		t.Errorf("expected no item key")
	}
	if out["name"] != "n" {
		t.Errorf("name: %v", out["name"])
	}
}

func TestFullRoundTripWithAuthCookiesExamples(t *testing.T) {
	js := `{
		"info":{"name":"Full","_postman_id":"pid"},
		"item":[
			{"name":"folder","description":"fd","item":[
				{"name":"req","description":"rd","request":{
					"method":"POST",
					"url":{"raw":"http://x/api","host":["x"]},
					"header":[{"key":"H","value":"V"}],
					"auth":{"type":"basic","basic":[{"key":"username","value":"u"},{"key":"password","value":"p"}]},
					"_tracto_cookies":[{"key":"sid","value":"abc"}],
					"body":{"mode":"raw","raw":"payload"}
				},
				"response":[{"name":"ex","status":"OK","code":200,"body":"resp"}]}
			]},
			{"name":"top example","originalRequest":{},"code":204,"body":""}
		],
		"variable":[{"key":"k","value":"v"}]
	}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	_, data := Snapshot(col)
	col2, err := ParseCollection(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}

	if len(col2.Root.Children) != 1 || len(col2.Root.skippedItems) != 1 {
		t.Fatalf("root shape: %d children %d skipped", len(col2.Root.Children), len(col2.Root.skippedItems))
	}
	folder := col2.Root.Children[0]
	if !folder.IsFolder || len(folder.Children) != 1 {
		t.Fatalf("folder shape wrong")
	}
	req := folder.Children[0].Request
	if req == nil {
		t.Fatal("request lost")
	}
	if req.Method != "POST" || req.URL != "http://x/api" {
		t.Errorf("method/url: %+v", req)
	}
	if req.Headers["H"] != "V" {
		t.Errorf("header lost")
	}
	if req.Body != "payload" {
		t.Errorf("body lost: %q", req.Body)
	}
	if req.Auth.Type != "basic" || req.Auth.Username != "u" || req.Auth.Password != "p" {
		t.Errorf("auth lost: %+v", req.Auth)
	}
	if len(req.Cookies) != 1 || req.Cookies[0].Key != "sid" {
		t.Errorf("cookies lost: %+v", req.Cookies)
	}
	if len(req.Examples) != 1 || req.Examples[0].Name != "ex" || req.Examples[0].Code != 200 {
		t.Errorf("examples lost across round-trip: %+v", req.Examples)
	}
	if _, ok := col2.TopExtras["variable"]; !ok {
		t.Errorf("variable lost")
	}
}

func TestParseCollectionItemNotArray(t *testing.T) {
	col, err := ParseCollection(strings.NewReader(`{"info":{"name":"C"},"item":"nope"}`), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if col.Name != "C" {
		t.Errorf("name: %q", col.Name)
	}
	if len(col.Root.Children) != 0 {
		t.Errorf("expected no children, got %d", len(col.Root.Children))
	}
}

func TestParseCollectionDropsUnparseableItems(t *testing.T) {
	col, err := ParseCollection(strings.NewReader(`{"info":{"name":"C"},"item":["bare",42,{"name":"ok","request":{"method":"GET","url":"u"}}]}`), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(col.Root.Children) != 1 || col.Root.Children[0].Name != "ok" {
		t.Errorf("expected only the valid item, got %+v", col.Root.Children)
	}
}
