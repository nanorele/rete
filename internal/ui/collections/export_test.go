package collections

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const exportFixture = `{
  "info": {"name": "Shop API", "_postman_id": "abc-123", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
  "auth": {"type": "apikey", "apikey": [{"key": "key", "value": "X-Api-Key", "type": "string"}, {"key": "value", "value": "secret", "type": "string"}]},
  "item": [
    {"name": "Users", "item": [
      {"name": "List", "request": {"method": "GET", "url": "https://x.test/users", "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "t1", "type": "string"}]}}}
    ]},
    {"name": "Login", "request": {"method": "POST", "url": {"raw": "https://x.test/login", "host": ["x", "test"]}, "auth": {"type": "basic", "basic": [{"key": "username", "value": "u"}, {"key": "password", "value": "p"}]}, "body": {"mode": "raw", "raw": "{}"}}}
  ]
}`

func exportTree(t *testing.T, version string) map[string]any {
	t.Helper()
	col, err := ParseCollection(strings.NewReader(exportFixture), "c1")
	if err != nil {
		t.Fatal(err)
	}
	data := ExportPostman(col, version)
	if len(data) == 0 {
		t.Fatal("empty export")
	}
	var tree map[string]any
	if err := json.Unmarshal(data, &tree); err != nil {
		t.Fatalf("export is not valid JSON: %v\n%s", err, data)
	}
	return tree
}

func authOf(t *testing.T, m map[string]any) map[string]any {
	t.Helper()
	a, ok := m["auth"].(map[string]any)
	if !ok {
		t.Fatalf("missing auth in %v", m)
	}
	return a
}

func TestExportV21KeepsArrayAuthAndSchema(t *testing.T) {
	tree := exportTree(t, PostmanV21)
	info := tree["info"].(map[string]any)
	if got := info["schema"]; got != "https://schema.getpostman.com/json/collection/v2.1.0/collection.json" {
		t.Errorf("schema = %v", got)
	}
	if info["_postman_id"] != "abc-123" {
		t.Errorf("_postman_id lost: %v", info["_postman_id"])
	}
	top := authOf(t, tree)
	if _, ok := top["apikey"].([]any); !ok {
		t.Errorf("v2.1 top-level auth must stay an array: %v", top["apikey"])
	}
	items := tree["item"].([]any)
	login := items[1].(map[string]any)
	req := login["request"].(map[string]any)
	basic := authOf(t, req)["basic"].([]any)
	if len(basic) != 2 {
		t.Fatalf("basic auth entries = %v", basic)
	}
	first := basic[0].(map[string]any)
	if first["key"] != "username" || first["value"] != "u" || first["type"] != "string" {
		t.Errorf("basic[0] = %v", first)
	}
}

func TestExportV20FlattensAuth(t *testing.T) {
	tree := exportTree(t, PostmanV20)
	info := tree["info"].(map[string]any)
	if got := info["schema"]; got != "https://schema.getpostman.com/json/collection/v2.0.0/collection.json" {
		t.Errorf("schema = %v", got)
	}
	top := authOf(t, tree)
	apikey, ok := top["apikey"].(map[string]any)
	if !ok {
		t.Fatalf("v2.0 top-level auth must be an object: %v", top["apikey"])
	}
	if apikey["key"] != "X-Api-Key" || apikey["value"] != "secret" {
		t.Errorf("apikey = %v", apikey)
	}
	items := tree["item"].([]any)
	users := items[0].(map[string]any)
	list := users["item"].([]any)[0].(map[string]any)
	bearer := authOf(t, list["request"].(map[string]any))["bearer"].(map[string]any)
	if bearer["token"] != "t1" {
		t.Errorf("bearer = %v", bearer)
	}
	login := items[1].(map[string]any)
	basic := authOf(t, login["request"].(map[string]any))["basic"].(map[string]any)
	if basic["username"] != "u" || basic["password"] != "p" {
		t.Errorf("basic = %v", basic)
	}
	if url := login["request"].(map[string]any)["url"].(map[string]any); url["raw"] != "https://x.test/login" {
		t.Errorf("url = %v", url)
	}
}

func TestExportV21UpgradesObjectAuth(t *testing.T) {
	src := `{"info":{"name":"Old"},"auth":{"type":"apikey","apikey":{"key":"X-Key","value":"s","in":"header"}},"item":[{"name":"R","request":{"method":"GET","url":"https://x.test","auth":{"type":"basic","basic":{"username":"u","password":"p"}}}}]}`
	col, err := ParseCollection(strings.NewReader(src), "c2")
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(ExportPostman(col, PostmanV21), &tree); err != nil {
		t.Fatal(err)
	}
	info := tree["info"].(map[string]any)
	if id, _ := info["_postman_id"].(string); id == "" {
		t.Error("missing generated _postman_id")
	}
	apikey := authOf(t, tree)["apikey"].([]any)
	if len(apikey) != 3 {
		t.Fatalf("apikey = %v", apikey)
	}
	if e := apikey[0].(map[string]any); e["key"] != "in" || e["value"] != "header" || e["type"] != "string" {
		t.Errorf("apikey[0] = %v", e)
	}
	req := tree["item"].([]any)[0].(map[string]any)["request"].(map[string]any)
	basic := authOf(t, req)["basic"].([]any)
	if len(basic) != 2 {
		t.Fatalf("basic = %v", basic)
	}
	if e := basic[0].(map[string]any); e["key"] != "username" || e["value"] != "u" || e["type"] != "string" {
		t.Errorf("basic[0] = %v", e)
	}
	if e := basic[1].(map[string]any); e["key"] != "password" || e["value"] != "p" {
		t.Errorf("basic[1] = %v", e)
	}
}

func TestExportRoundTripsThroughParser(t *testing.T) {
	col, err := ParseCollection(strings.NewReader(exportFixture), "c1")
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range PostmanVersions {
		again, err := ParseCollection(bytes.NewReader(ExportPostman(col, v)), "c3")
		if err != nil {
			t.Fatalf("%s: re-parse: %v", v, err)
		}
		if again.Name != "Shop API" || len(again.Root.Children) != 2 {
			t.Fatalf("%s: re-parse lost structure: %q %d", v, again.Name, len(again.Root.Children))
		}
		login := again.Root.Children[1]
		if login.Request == nil || login.Request.Method != "POST" || login.Request.URL != "https://x.test/login" {
			t.Errorf("%s: login request = %+v", v, login.Request)
		}
	}
}

func TestExportFileNameAndSeparator(t *testing.T) {
	col := &ParsedCollection{Name: `My: API/v1?`}
	if got := ExportFileName(col); got != "My_ API_v1_.postman_collection.json" {
		t.Errorf("file name = %q", got)
	}
	sep := NewSeparator("")
	if !sep.Data.IsSeparator() || !strings.HasPrefix(sep.Data.ID, "sep_") {
		t.Errorf("separator = %+v", sep.Data)
	}
	if id, data := Snapshot(sep.Data); id != "" || data != nil {
		t.Error("separators must never be written to disk")
	}
	if ExportPostman(sep.Data, PostmanV21) != nil {
		t.Error("separators must not export")
	}
}
