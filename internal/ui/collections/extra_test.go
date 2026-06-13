package collections

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tracto/internal/model"
	"tracto/internal/persist"
)

func TestFormPartSrcPath(t *testing.T) {
	if got := formPartSrcPath("file.txt"); got != "file.txt" {
		t.Errorf("string src: got %q", got)
	}
	if got := formPartSrcPath([]any{"", "a.txt", "b.txt"}); got != "a.txt" {
		t.Errorf("[]any src: got %q", got)
	}
	if got := formPartSrcPath([]string{"", "x.txt"}); got != "x.txt" {
		t.Errorf("[]string src: got %q", got)
	}
	if got := formPartSrcPath(nil); got != "" {
		t.Errorf("nil src: got %q", got)
	}
	if got := formPartSrcPath(42); got != "" {
		t.Errorf("int src: got %q", got)
	}
	if got := formPartSrcPath([]any{1, 2, 3}); got != "" {
		t.Errorf("[]any of ints: got %q", got)
	}
	if got := formPartSrcPath([]any{}); got != "" {
		t.Errorf("empty []any: got %q", got)
	}
}

func TestIsExampleItemVariants(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"originalRequest", `{"originalRequest":{}}`, true},
		{"previewlanguage", `{"_postman_previewlanguage":"json"}`, true},
		{"responseTime", `{"responseTime":12}`, true},
		{"apidog example", `{"_apidog_type":"example"}`, true},
		{"apidog case upper", `{"_apidog_type":"CASE"}`, true},
		{"apidog apicase", `{"_apidog_type":"apicase"}`, true},
		{"apidog other", `{"_apidog_type":"folder"}`, false},
		{"code+body", `{"code":200,"body":"hi"}`, true},
		{"code only", `{"code":200}`, false},
		{"body only", `{"body":"hi"}`, false},
		{"regular item", `{"name":"r","request":{}}`, false},
		{"invalid json", `not-json`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isExampleItem(json.RawMessage(c.raw)); got != c.want {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestCollectSubtree(t *testing.T) {
	root := &CollectionNode{Name: "r"}
	a := &CollectionNode{Name: "a"}
	b := &CollectionNode{Name: "b"}
	c := &CollectionNode{Name: "c"}
	root.Children = []*CollectionNode{a, b}
	a.Children = []*CollectionNode{c}

	got := CollectSubtree(root)
	if len(got) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(got))
	}
	for _, n := range []*CollectionNode{root, a, b, c} {
		if _, ok := got[n]; !ok {
			t.Errorf("expected node %s in set", n.Name)
		}
	}

	if got := CollectSubtree(nil); len(got) != 0 {
		t.Errorf("nil root: expected empty, got %d", len(got))
	}
}

func TestCollectSubtreeCycleSafe(t *testing.T) {
	a := &CollectionNode{Name: "a"}
	b := &CollectionNode{Name: "b"}
	a.Children = []*CollectionNode{b}
	b.Children = []*CollectionNode{a}
	got := CollectSubtree(a)
	if len(got) != 2 {
		t.Errorf("expected 2 unique nodes, got %d", len(got))
	}
}

func TestNodeAtPathNilRoot(t *testing.T) {
	if NodeAtPath(nil, []int{0}) != nil {
		t.Error("expected nil for nil root")
	}
	root := &CollectionNode{Name: "r"}
	if got := NodeAtPath(root, nil); got != root {
		t.Error("empty path should return root")
	}
}

func TestParseURLObjectNoRaw(t *testing.T) {
	url, raw := parseURL(json.RawMessage(`{"host":["example.com"],"path":["api"]}`))
	if url != "" {
		t.Errorf("expected empty url when no raw, got %q", url)
	}
	if raw == nil {
		t.Errorf("expected raw preserved")
	}
}

func TestParseURLEmptyString(t *testing.T) {
	url, raw := parseURL(json.RawMessage(`""`))
	if url != "" || raw != nil {
		t.Errorf("expected empty,nil; got %q raw=%v", url, raw)
	}
}

func TestParseURLInvalid(t *testing.T) {
	url, raw := parseURL(json.RawMessage(`12345`))
	if url != "" || raw != nil {
		t.Errorf("number url should return empty; got %q raw=%v", url, raw)
	}
}

func TestParseHeaderArrayDisabled(t *testing.T) {
	raw := json.RawMessage(`[
		{"key":"A","value":"1"},
		{"key":"B","value":"2","disabled":true},
		{"key":"","value":"x"}
	]`)
	hdrs, rawOut := parseHeaderArray(raw)
	if len(hdrs) != 1 || hdrs["A"] != "1" {
		t.Errorf("expected only A:1, got %v", hdrs)
	}
	if rawOut == nil {
		t.Errorf("expected raw preserved")
	}
}

func TestParseHeaderArrayStringForm(t *testing.T) {
	raw := json.RawMessage(`["A: 1","B: 2"]`)
	hdrs, rawOut := parseHeaderArray(raw)
	if len(hdrs) != 0 {
		t.Errorf("string-form headers ignored; got %v", hdrs)
	}
	if rawOut != nil {
		t.Errorf("raw must be nil on parse failure so headers added in-app survive save, got %s", string(rawOut))
	}
}

func TestParseBodyRaw(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"mode":"raw","raw":"hello"}`), req)
	if req.Body != "hello" {
		t.Errorf("body: got %q", req.Body)
	}
	if req.BodyType != model.BodyRaw {
		t.Errorf("type: got %v", req.BodyType)
	}
}

func TestParseBodyURLEncoded(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	raw := json.RawMessage(`{"mode":"urlencoded","urlencoded":[
		{"key":"a","value":"1"},
		{"key":"b","value":"2","disabled":true},
		{"key":"c","value":"3"}
	]}`)
	parseBodyInto(raw, req)
	if req.BodyType != model.BodyURLEncoded {
		t.Errorf("expected BodyURLEncoded, got %v", req.BodyType)
	}
	if len(req.URLEncoded) != 3 {
		t.Fatalf("expected 3 entries (disabled preserved), got %d", len(req.URLEncoded))
	}
	if req.URLEncoded[0].Key != "a" || req.URLEncoded[1].Key != "b" || req.URLEncoded[2].Key != "c" {
		t.Errorf("unexpected order: %+v", req.URLEncoded)
	}
	if !req.URLEncoded[1].Disabled {
		t.Errorf("entry 'b' should be marked Disabled")
	}
	if req.URLEncoded[0].Disabled || req.URLEncoded[2].Disabled {
		t.Errorf("entries 'a'/'c' should not be Disabled")
	}
}

func TestParseBodyFormData(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	raw := json.RawMessage(`{"mode":"formdata","formdata":[
		{"key":"name","value":"Alice","type":"text"},
		{"key":"avatar","type":"file","src":"/p/a.png"},
		{"key":"skip","value":"x","disabled":true},
		{"key":"multi","type":"file","src":["","/p/m.bin"]}
	]}`)
	parseBodyInto(raw, req)
	if req.BodyType != model.BodyFormData {
		t.Errorf("expected formdata, got %v", req.BodyType)
	}
	if len(req.FormParts) != 4 {
		t.Fatalf("expected 4 parts (disabled preserved), got %d", len(req.FormParts))
	}
	if req.FormParts[0].Kind != model.FormPartText || req.FormParts[0].Value != "Alice" {
		t.Errorf("text part wrong: %+v", req.FormParts[0])
	}
	if req.FormParts[1].Kind != model.FormPartFile || req.FormParts[1].FilePath != "/p/a.png" {
		t.Errorf("file part wrong: %+v", req.FormParts[1])
	}
	if req.FormParts[2].Key != "skip" || !req.FormParts[2].Disabled {
		t.Errorf("disabled part should be present and marked Disabled: %+v", req.FormParts[2])
	}
	if req.FormParts[3].Kind != model.FormPartFile || req.FormParts[3].FilePath != "/p/m.bin" {
		t.Errorf("multi src part wrong: %+v", req.FormParts[3])
	}
}

func TestParseBodyBinaryFile(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"mode":"file","file":{"src":"/p/x.bin"}}`), req)
	if req.BodyType != model.BodyBinary {
		t.Errorf("expected binary, got %v", req.BodyType)
	}
	if req.BinaryPath != "/p/x.bin" {
		t.Errorf("BinaryPath: got %q", req.BinaryPath)
	}
}

func TestParseBodyInvalid(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`"not-an-object"`), req)
	if req.BodyType != model.BodyNone {
		t.Errorf("expected BodyNone on invalid body, got %v", req.BodyType)
	}
}

func TestParseBodyModeMissingButRawPresent(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"raw":"hello"}`), req)
	if req.Body != "hello" {
		t.Errorf("Body parsed: %q", req.Body)
	}
	if req.BodyType != model.BodyRaw {
		t.Errorf("missing mode + raw present must infer BodyRaw (else raw is dropped on save), got %v", req.BodyType)
	}
}

func TestParseBodyModeMissingFormDataInferred(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"formdata":[{"key":"a","value":"b"}]}`), req)
	if req.BodyType != model.BodyFormData {
		t.Errorf("missing mode + formdata present must infer BodyFormData, got %v", req.BodyType)
	}
}

func TestParseBodyModeMissingURLEncodedInferred(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"urlencoded":[{"key":"a","value":"b"}]}`), req)
	if req.BodyType != model.BodyURLEncoded {
		t.Errorf("missing mode + urlencoded present must infer BodyURLEncoded, got %v", req.BodyType)
	}
}

func TestParseBodyModeMissingFileInferred(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"file":{"src":"/x"}}`), req)
	if req.BodyType != model.BodyBinary {
		t.Errorf("missing mode + file present must infer BodyBinary, got %v", req.BodyType)
	}
}

func TestParseBodyExtrasRoundTrip(t *testing.T) {
	req := &model.ParsedRequest{Headers: map[string]string{}}
	parseBodyInto(json.RawMessage(`{"mode":"raw","raw":"x","options":{"raw":{"language":"json"}}}`), req)
	if _, ok := req.BodyExtras["options"]; !ok {
		t.Errorf("expected options preserved in BodyExtras")
	}
}

func TestParseRequestRawString(t *testing.T) {
	req := parseRequestRaw(json.RawMessage(`"http://example.com"`), "n")
	if req == nil || req.URL != "http://example.com" || req.Method != "GET" {
		t.Errorf("string request not parsed: %+v", req)
	}
}

func TestParseRequestRawEmptyString(t *testing.T) {
	req := parseRequestRaw(json.RawMessage(`""`), "n")
	if req != nil {
		t.Errorf("empty string request: expected nil, got %+v", req)
	}
}

func TestParseRequestRawNonStringScalar(t *testing.T) {
	if req := parseRequestRaw(json.RawMessage(`42`), "n"); req != nil {
		t.Errorf("number request: expected nil")
	}
}

func TestParseItemRawRequestPresentWithItemSibling(t *testing.T) {

	raw := json.RawMessage(`{"name":"r","request":{"method":"GET","url":"u"},"item":[{"name":"ignored"}]}`)
	node := parseItemRaw(raw, 1)
	if node == nil {
		t.Fatal("nil node")
	}
	if node.IsFolder {
		t.Errorf("expected non-folder when request present")
	}
	if _, ok := node.Extras["item"]; !ok {
		t.Errorf("expected item preserved in Extras")
	}
}

func TestParseItemRawEmptyShellBecomesFolder(t *testing.T) {
	node := parseItemRaw(json.RawMessage(`{"name":"foo"}`), 1)
	if node == nil {
		t.Fatal("empty {name} item must NOT be dropped — should become an empty folder to avoid data loss")
	}
	if !node.IsFolder {
		t.Errorf("expected empty shell to be coerced into a folder, got %+v", node)
	}
	if node.Name != "foo" {
		t.Errorf("name lost: %q", node.Name)
	}
}

func TestParseItemRawRequestKeyButInvalid(t *testing.T) {
	raw := json.RawMessage(`{"name":"r","request":42}`)
	node := parseItemRaw(raw, 1)
	if node == nil || node.Request == nil {
		t.Fatalf("expected default-GET request, got %+v", node)
	}
	if node.Request.Method != "GET" {
		t.Errorf("expected default GET method")
	}
}

func TestParseItemRawOnlyItemKey(t *testing.T) {
	raw := json.RawMessage(`{"name":"empty-folder","item":[]}`)
	node := parseItemRaw(raw, 1)
	if node == nil {
		t.Fatal("nil node")
	}
	if !node.IsFolder {
		t.Errorf("expected folder")
	}
	if len(node.Children) != 0 {
		t.Errorf("expected zero children")
	}
}

func TestParseItemRawInvalid(t *testing.T) {
	if node := parseItemRaw(json.RawMessage(`"bare-string"`), 1); node != nil {
		t.Errorf("expected nil for non-object item")
	}
}

func TestParseCollectionExampleSkipped(t *testing.T) {
	js := `{"info":{"name":"C"},"item":[
		{"name":"r1","request":{"method":"GET","url":"u"}},
		{"name":"ex","originalRequest":{},"code":200,"body":"hi"}
	]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(col.Root.Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(col.Root.Children))
	}
	if len(col.Root.skippedItems) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(col.Root.skippedItems))
	}
}

func TestParseCollectionFolderSkippedNested(t *testing.T) {
	js := `{"info":{"name":"C"},"item":[
		{"name":"folder","item":[
			{"name":"r","request":{"method":"GET","url":"u"}},
			{"originalRequest":{}}
		]}
	]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	folder := col.Root.Children[0]
	if len(folder.skippedItems) != 1 {
		t.Errorf("expected 1 skipped in folder, got %d", len(folder.skippedItems))
	}
}

func TestParseCollectionTopExtras(t *testing.T) {
	js := `{"info":{"name":"C","_postman_id":"abc","schema":"v2.1"},"item":[],"variable":[{"key":"x","value":"1"}]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := col.InfoExtras["_postman_id"]; !ok {
		t.Errorf("expected _postman_id preserved")
	}
	if _, ok := col.InfoExtras["schema"]; !ok {
		t.Errorf("expected schema preserved")
	}
	if _, ok := col.TopExtras["variable"]; !ok {
		t.Errorf("expected variable preserved")
	}
}

func TestParseCollectionInvalidJSON(t *testing.T) {
	if _, err := ParseCollection(strings.NewReader("[]"), "id"); err == nil {
		t.Errorf("expected error for array top-level")
	}
}

func TestParseCollectionEmptyObject(t *testing.T) {
	if _, err := ParseCollection(strings.NewReader("{}"), "id"); err == nil {
		t.Errorf("expected error for empty object")
	}
}

func TestParseCollectionInfoNotObject(t *testing.T) {
	js := `{"info":"bad","item":[{"name":"r","request":{"method":"GET","url":"u"}}]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if col.Name != "Imported Collection" {
		t.Errorf("expected fallback name, got %q", col.Name)
	}
	if _, ok := col.TopExtras["info"]; !ok {
		t.Errorf("expected info preserved as top extra")
	}
}

func TestMarshalCollectionRoundTrip(t *testing.T) {
	js := `{
		"info":{"name":"My Col","_postman_id":"abc"},
		"item":[
			{"name":"folder","item":[
				{"name":"req1","request":{"method":"POST","url":"http://x","header":[{"key":"H","value":"V"}],"body":{"mode":"raw","raw":"hi"}}}
			]},
			{"name":"req2","request":{"method":"GET","url":{"raw":"http://y","host":["y"]}}}
		]
	}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	id, data := Snapshot(col)
	if id != "id" {
		t.Errorf("id: got %q", id)
	}
	if len(data) == 0 {
		t.Fatal("empty data")
	}

	col2, err := ParseCollection(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if col2.Name != col.Name {
		t.Errorf("name lost: %q vs %q", col2.Name, col.Name)
	}
	if len(col2.Root.Children) != 2 {
		t.Fatalf("children lost: %d", len(col2.Root.Children))
	}
	folder := col2.Root.Children[0]
	if !folder.IsFolder || len(folder.Children) != 1 {
		t.Errorf("folder shape wrong")
	}
	req1 := folder.Children[0]
	if req1.Request.Method != "POST" || req1.Request.URL != "http://x" {
		t.Errorf("req1 wrong: %+v", req1.Request)
	}
	if req1.Request.Headers["H"] != "V" {
		t.Errorf("header lost")
	}
	if req1.Request.Body != "hi" {
		t.Errorf("body lost: %q", req1.Request.Body)
	}
	req2 := col2.Root.Children[1]
	if req2.Request.URL != "http://y" {
		t.Errorf("url-object raw lost: %q", req2.Request.URL)
	}
	if len(req2.Request.RawURL) == 0 {
		t.Errorf("RawURL not preserved")
	}
}

func TestMarshalEmptyCollection(t *testing.T) {
	col := &ParsedCollection{
		ID:   "id",
		Name: "Empty",
		Root: &CollectionNode{Name: "Empty", IsFolder: true},
	}
	id, data := Snapshot(col)
	if id != "id" || len(data) == 0 {
		t.Fatalf("snapshot failed: id=%q len=%d", id, len(data))
	}
	col2, err := ParseCollection(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if col2.Name != "Empty" {
		t.Errorf("name: %q", col2.Name)
	}
	if len(col2.Root.Children) != 0 {
		t.Errorf("expected no children")
	}
}

func TestSnapshotNil(t *testing.T) {
	if id, data := Snapshot(nil); id != "" || data != nil {
		t.Errorf("expected empty for nil collection")
	}
	if id, data := Snapshot(&ParsedCollection{ID: "x"}); id != "" || data != nil {
		t.Errorf("expected empty for nil root")
	}
	if id, data := Snapshot(&ParsedCollection{Root: &CollectionNode{}}); id != "" || data != nil {
		t.Errorf("expected empty for empty id")
	}
}

func TestMarshalNodeRequestWithRawURL(t *testing.T) {
	node := &CollectionNode{
		Name:     "r",
		IsFolder: false,
		Request: &model.ParsedRequest{
			Name:    "r",
			Method:  "GET",
			URL:     "http://changed",
			Headers: map[string]string{},
			RawURL:  json.RawMessage(`{"raw":"http://orig","host":["orig"]}`),
		},
	}
	out := marshalNode(node)
	reqMap := out["request"].(map[string]any)
	urlObj := reqMap["url"].(map[string]any)
	if urlObj["raw"] != "http://changed" {
		t.Errorf("raw not overwritten: %v", urlObj["raw"])
	}
	if urlObj["host"] == nil {
		t.Errorf("host should be preserved from RawURL")
	}
}

func TestMarshalNodePreservesSkippedItems(t *testing.T) {
	folder := &CollectionNode{
		Name:     "f",
		IsFolder: true,
		skippedItems: []json.RawMessage{
			json.RawMessage(`{"originalRequest":{},"code":200}`),
		},
	}
	out := marshalNode(folder)
	items := out["item"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestSaveToFileAndLoadAll(t *testing.T) {
	tmp := t.TempDir()
	old := persist.ConfigDir()
	_ = old
	persist.SetConfigOverride(tmp)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	col := &ParsedCollection{
		ID:         "my-id",
		Name:       "C",
		InfoExtras: map[string]json.RawMessage{},
		TopExtras:  map[string]json.RawMessage{},
		Root: &CollectionNode{
			Name:     "C",
			IsFolder: true,
			Children: []*CollectionNode{
				{
					Name: "r1",
					Request: &model.ParsedRequest{
						Name:    "r1",
						Method:  "GET",
						URL:     "http://a",
						Headers: map[string]string{},
					},
				},
			},
		},
	}
	if err := SaveToFile(col); err != nil {
		t.Fatalf("save: %v", err)
	}
	saved := filepath.Join(tmp, "collections", "my-id.json")
	if _, err := os.Stat(saved); err != nil {
		t.Fatalf("stat: %v", err)
	}
	loaded := LoadAll()
	if len(loaded) != 1 {
		t.Fatalf("expected 1 loaded, got %d", len(loaded))
	}
	if loaded[0].ID != "my-id" || loaded[0].Name != "C" {
		t.Errorf("loaded wrong: %+v", loaded[0])
	}
}

func TestSaveToFileNilCollection(t *testing.T) {
	if err := SaveToFile(nil); err != nil {
		t.Errorf("expected nil error for nil col, got %v", err)
	}
}

func TestLoadAllSkipsCorrupt(t *testing.T) {
	tmp := t.TempDir()
	persist.SetConfigOverride(tmp)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	if err := persist.WriteCollectionFile("bad", []byte("not json")); err != nil {
		t.Fatal(err)
	}
	good := `{"info":{"name":"OK"},"item":[{"name":"r","request":{"method":"GET","url":"u"}}]}`
	if err := persist.WriteCollectionFile("good", []byte(good)); err != nil {
		t.Fatal(err)
	}
	loaded := LoadAll()
	if len(loaded) != 1 || loaded[0].ID != "good" {
		t.Errorf("expected 1 good entry, got %+v", loaded)
	}
}

func TestCloneNodeDeepIndependence(t *testing.T) {
	col := &ParsedCollection{ID: "x"}
	node := &CollectionNode{
		Name:       "r",
		Collection: col,
		Extras:     map[string]json.RawMessage{"k": json.RawMessage(`"v"`)},
		Request: &model.ParsedRequest{
			Name:       "r",
			Method:     "POST",
			URL:        "http://u",
			Headers:    map[string]string{"A": "1"},
			BodyType:   model.BodyFormData,
			FormParts:  []model.ParsedFormPart{{Key: "f", Value: "v"}},
			URLEncoded: []model.ParsedKV{{Key: "k", Value: "v"}},
			RawURL:     json.RawMessage(`{"raw":"http://u"}`),
			RawHeaders: json.RawMessage(`[{"key":"A","value":"1"}]`),
			Extras:     map[string]json.RawMessage{"auth": json.RawMessage(`{}`)},
			BodyExtras: map[string]json.RawMessage{"opt": json.RawMessage(`{}`)},
			BinaryPath: "/p",
		},
	}
	cl := CloneNode(node, nil)

	if cl.Request.BodyType != model.BodyFormData {
		t.Errorf("BodyType lost")
	}
	if cl.Request.BinaryPath != "/p" {
		t.Errorf("BinaryPath lost")
	}

	node.Request.Headers["A"] = "MUT"
	node.Request.FormParts[0].Key = "MUT"
	node.Request.URLEncoded[0].Key = "MUT"
	node.Request.RawURL[0] = 'X'
	node.Request.RawHeaders[0] = 'X'
	node.Request.Extras["auth"][0] = 'X'
	node.Request.BodyExtras["opt"][0] = 'X'
	node.Extras["k"][0] = 'X'

	if cl.Request.Headers["A"] != "1" {
		t.Errorf("headers not deep-copied")
	}
	if cl.Request.FormParts[0].Key != "f" {
		t.Errorf("formparts not deep-copied")
	}
	if cl.Request.URLEncoded[0].Key != "k" {
		t.Errorf("urlencoded not deep-copied")
	}
	if cl.Request.RawURL[0] == 'X' {
		t.Errorf("RawURL not deep-copied")
	}
	if cl.Request.RawHeaders[0] == 'X' {
		t.Errorf("RawHeaders not deep-copied")
	}
	if cl.Request.Extras["auth"][0] == 'X' {
		t.Errorf("Request.Extras not deep-copied")
	}
	if cl.Request.BodyExtras["opt"][0] == 'X' {
		t.Errorf("BodyExtras not deep-copied")
	}
	if cl.Extras["k"][0] == 'X' {
		t.Errorf("node.Extras not deep-copied")
	}
}

func TestCloneNodeSkippedItemsCopied(t *testing.T) {
	node := &CollectionNode{
		Name:         "f",
		IsFolder:     true,
		skippedItems: []json.RawMessage{json.RawMessage(`{"code":200,"body":"hi"}`)},
	}
	cl := CloneNode(node, nil)
	if len(cl.skippedItems) != 1 {
		t.Fatalf("expected 1 skipped, got %d", len(cl.skippedItems))
	}
	node.skippedItems[0][0] = 'Z'
	if cl.skippedItems[0][0] == 'Z' {
		t.Errorf("skippedItems not deep-copied")
	}
}

func TestCloneNodeNoExtras(t *testing.T) {
	node := &CollectionNode{Name: "n", Request: &model.ParsedRequest{Headers: map[string]string{}}}
	cl := CloneNode(node, nil)
	if len(cl.Extras) != 0 {
		t.Errorf("expected empty extras")
	}
}

func TestDisabledFormPartPreservedAndRoundTrips(t *testing.T) {
	js := `{"info":{"name":"C"},"item":[{"name":"r","request":{"method":"POST","url":"u","body":{"mode":"formdata","formdata":[{"key":"a","value":"1"},{"key":"b","value":"2","disabled":true}]}}}]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	req := col.Root.Children[0].Request
	if len(req.FormParts) != 2 {
		t.Fatalf("expected 2 form parts (disabled preserved), got %d", len(req.FormParts))
	}
	if req.FormParts[1].Key != "b" || !req.FormParts[1].Disabled {
		t.Errorf("disabled part b not preserved with flag: %+v", req.FormParts[1])
	}
	_, data := Snapshot(col)
	if !bytes.Contains(data, []byte(`"b"`)) {
		t.Errorf("disabled form part b should survive round-trip (key 'b' missing): %s", data)
	}
	if !bytes.Contains(data, []byte(`"disabled": true`)) && !bytes.Contains(data, []byte(`"disabled":true`)) {
		t.Errorf("disabled:true flag should be persisted on save, got: %s", data)
	}
}

func TestNestedFoldersDeep(t *testing.T) {
	js := `{"info":{"name":"C"},"item":[
		{"name":"l1","item":[
			{"name":"l2","item":[
				{"name":"l3","item":[
					{"name":"r","request":{"method":"GET","url":"u"}}
				]}
			]}
		]}
	]}`
	col, err := ParseCollection(strings.NewReader(js), "id")
	if err != nil {
		t.Fatal(err)
	}
	cur := col.Root
	for i := range 3 {
		if len(cur.Children) != 1 {
			t.Fatalf("depth %d: expected 1 child", i)
		}
		cur = cur.Children[0]
		if !cur.IsFolder {
			t.Fatalf("depth %d: expected folder", i)
		}
	}
	if len(cur.Children) != 1 || cur.Children[0].Request == nil {
		t.Errorf("expected leaf request")
	}
}

func TestAssignParentsEnablesEditor(t *testing.T) {
	root := &CollectionNode{Name: "r", Children: []*CollectionNode{{Name: "a"}}}
	AssignParents(root, nil, nil)
	if !root.NameEditor.SingleLine || !root.NameEditor.Submit {
		t.Errorf("root editor not configured")
	}
	if !root.Children[0].NameEditor.SingleLine {
		t.Errorf("child editor not configured")
	}
}
