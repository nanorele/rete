package collections

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"rete/internal/model"
	"rete/internal/persist"
	"strings"
	"testing"
)

func TestCloneNodeSuffixOnlyTopLevel(t *testing.T) {
	folder := &CollectionNode{Name: "Auth", IsFolder: true}
	child := &CollectionNode{
		Name:    "Login",
		Parent:  folder,
		Request: &model.ParsedRequest{Name: "Login", Method: "POST"},
	}
	folder.Children = []*CollectionNode{child}

	dup := CloneNode(folder, nil)
	if dup.Name != "Auth Copy" {
		t.Errorf("top-level name = %q, want %q", dup.Name, "Auth Copy")
	}
	if len(dup.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(dup.Children))
	}
	if got := dup.Children[0].Name; got != "Login" {
		t.Errorf("child name = %q, want unchanged %q", got, "Login")
	}
	if got := dup.Children[0].Request.Name; got != "Login" {
		t.Errorf("child request name = %q, want %q", got, "Login")
	}
}

func TestCloneNodeCopiesExamples(t *testing.T) {
	node := &CollectionNode{
		Name: "Req",
		Request: &model.ParsedRequest{
			Name:     "Req",
			Method:   "GET",
			Examples: []model.ParsedExample{{Name: "ex1"}, {Name: "ex2"}},
		},
	}
	dup := CloneNode(node, nil)
	if len(dup.Request.Examples) != 2 {
		t.Fatalf("expected 2 examples copied, got %d", len(dup.Request.Examples))
	}
	dup.Request.Examples[0].Name = "changed"
	if node.Request.Examples[0].Name != "ex1" {
		t.Error("clone shares Examples backing array with original")
	}
}

func TestNodePathFromAndAtPath(t *testing.T) {
	root := &CollectionNode{Name: "root", IsFolder: true}
	child1 := &CollectionNode{Name: "child1", Parent: root}
	child2 := &CollectionNode{Name: "child2", Parent: root}
	subchild := &CollectionNode{Name: "subchild", Parent: child2}
	root.Children = []*CollectionNode{child1, child2}
	child2.Children = []*CollectionNode{subchild}

	path := NodePathFrom(root, subchild)
	if len(path) != 2 || path[0] != 1 || path[1] != 0 {
		t.Errorf("expected [1, 0], got %v", path)
	}

	pathRoot := NodePathFrom(root, root)
	if pathRoot != nil {
		t.Errorf("expected nil path for root, got %v", pathRoot)
	}

	pathNilTarget := NodePathFrom(root, nil)
	if pathNilTarget != nil {
		t.Errorf("expected nil path for nil target, got %v", pathNilTarget)
	}

	pathNilRoot := NodePathFrom(nil, subchild)
	if pathNilRoot != nil {
		t.Errorf("expected nil path for nil root, got %v", pathNilRoot)
	}

	unrelatedNode := &CollectionNode{Name: "unrelated"}
	pathUnrelated := NodePathFrom(root, unrelatedNode)
	if pathUnrelated != nil {
		t.Errorf("expected nil path for unrelated target, got %v", pathUnrelated)
	}

	detachedParent := &CollectionNode{Name: "detached"}
	detachedChild := &CollectionNode{Name: "child", Parent: detachedParent}
	pathDetached := NodePathFrom(root, detachedChild)
	if pathDetached != nil {
		t.Errorf("expected nil path for detached child, got %v", pathDetached)
	}

	found := NodeAtPath(root, []int{1, 0})
	if found != subchild {
		t.Errorf("expected subchild, got %v", found)
	}

	notFound := NodeAtPath(root, []int{2, 0})
	if notFound != nil {
		t.Errorf("expected nil, got %v", notFound)
	}

	notFound2 := NodeAtPath(root, []int{1, 1})
	if notFound2 != nil {
		t.Errorf("expected nil, got %v", notFound2)
	}

	notFound3 := NodeAtPath(root, []int{-1})
	if notFound3 != nil {
		t.Errorf("expected nil, got %v", notFound3)
	}
}

func TestCloneNode(t *testing.T) {
	col := &ParsedCollection{ID: "col1"}
	root := &CollectionNode{
		Name:       "req",
		IsFolder:   false,
		Depth:      1,
		Collection: col,
		Request: &model.ParsedRequest{
			Name:   "req",
			Method: "POST",
			URL:    "http://example.com",
			Body:   "{}",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		},
	}

	clone := CloneNode(root, nil)

	if clone.Name != "req Copy" {
		t.Errorf("expected req Copy, got %s", clone.Name)
	}
	if clone.Parent != nil {
		t.Errorf("expected nil parent")
	}
	if clone.Collection != col {
		t.Errorf("expected same collection")
	}
	if clone.Request == nil {
		t.Fatalf("expected request")
	}
	if clone.Request.Name != "req Copy" {
		t.Errorf("expected request name req Copy, got %s", clone.Request.Name)
	}
	if clone.Request.Method != "POST" {
		t.Errorf("expected POST, got %s", clone.Request.Method)
	}
	if clone.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected header application/json")
	}

	root.Request.Headers["Content-Type"] = "text/plain"
	if clone.Request.Headers["Content-Type"] != "application/json" {
		t.Errorf("expected clone to retain original header, got %s", clone.Request.Headers["Content-Type"])
	}

	folder := &CollectionNode{
		Name:     "folder",
		IsFolder: true,
		Children: []*CollectionNode{root},
	}
	cloneFolder := CloneNode(folder, nil)
	if len(cloneFolder.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(cloneFolder.Children))
	}
	if cloneFolder.Children[0].Parent != cloneFolder {
		t.Errorf("expected child parent to be cloned folder")
	}
}

func TestParseCollection(t *testing.T) {
	jsonStr := `
	{
		"info": {
			"name": "Test Collection"
		},
		"item": [
			{
				"name": "Folder",
				"item": [
					{
						"name": "Request 1",
						"request": {
							"method": "GET",
							"url": "http://example.com",
							"header": [
								{
									"key": "Accept",
									"value": "application/json"
								}
							],
							"body": {
								"mode": "raw",
								"raw": "test"
							}
						}
					},
					{
						"name": "Request 2",
						"request": {
							"method": "POST",
							"url": {
								"raw": "http://example.com/api"
							}
						}
					}
				]
			},
			{
				"name": "Request 3 string URL",
				"request": "http://example.com/string"
			}
		]
	}`

	col, err := ParseCollection(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("ParseCollection error: %v", err)
	}

	if col.ID != "id1" {
		t.Errorf("expected id1, got %s", col.ID)
	}
	if col.Name != "Test Collection" {
		t.Errorf("expected Test Collection, got %s", col.Name)
	}
	if col.Root == nil {
		t.Fatalf("expected root node")
	}
	if len(col.Root.Children) != 2 {
		t.Fatalf("expected 2 children at root, got %d", len(col.Root.Children))
	}

	folder := col.Root.Children[0]
	if !folder.IsFolder {
		t.Errorf("expected folder to be IsFolder")
	}
	if len(folder.Children) != 2 {
		t.Fatalf("expected 2 requests in folder")
	}

	req1 := folder.Children[0]
	if req1.Request == nil {
		t.Fatalf("expected request")
	}
	if req1.Request.Method != "GET" {
		t.Errorf("expected GET, got %s", req1.Request.Method)
	}
	if req1.Request.URL != "http://example.com" {
		t.Errorf("expected url, got %s", req1.Request.URL)
	}
	if req1.Request.Body != "test" {
		t.Errorf("expected test, got %s", req1.Request.Body)
	}
	if req1.Request.Headers["Accept"] != "application/json" {
		t.Errorf("expected header Accept: application/json")
	}

	req2 := folder.Children[1]
	if req2.Request.URL != "http://example.com/api" {
		t.Errorf("expected url, got %s", req2.Request.URL)
	}

	req3 := col.Root.Children[1]
	if req3.Request.URL != "http://example.com/string" {
		t.Errorf("expected string url, got %s", req3.Request.URL)
	}

	_, err = ParseCollection(strings.NewReader("invalid"), "id2")
	if err == nil {
		t.Errorf("expected error for invalid json")
	}

	jsonWithItems := `{"info": {"name": ""}, "item": [{"name": "Req"}]}`
	colEmpty, _ := ParseCollection(strings.NewReader(jsonWithItems), "id3")
	if colEmpty.Name != "Imported Collection" {
		t.Errorf("expected Imported Collection, got %s", colEmpty.Name)
	}

	jsonReallyEmpty := `{"info": {"name": ""}, "item": []}`
	_, err = ParseCollection(strings.NewReader(jsonReallyEmpty), "id4")
	if err == nil {
		t.Errorf("expected error for really empty collection")
	}
}

func TestAssignParents(t *testing.T) {
	col := &ParsedCollection{}
	root := &CollectionNode{Name: "root"}
	child := &CollectionNode{Name: "child"}
	subchild := &CollectionNode{Name: "subchild"}

	root.Children = append(root.Children, child)
	child.Children = append(child.Children, subchild)

	AssignParents(root, nil, col)

	if child.Parent != root {
		t.Errorf("expected child parent to be root")
	}
	if child.Collection != col {
		t.Errorf("expected child collection to be col")
	}
	if subchild.Parent != child {
		t.Errorf("expected subchild parent to be child")
	}
	if !child.NameEditor.SingleLine {
		t.Errorf("expected NameEditor.SingleLine true")
	}
}

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

func TestAuthCookiesRoundTrip(t *testing.T) {
	cases := []model.ParsedRequest{
		{Method: "GET", URL: "http://x", Auth: model.ParsedAuth{Type: "bearer", Token: "tok123"}},
		{Method: "GET", URL: "http://x", Auth: model.ParsedAuth{Type: "basic", Username: "u", Password: "p"}},
		{Method: "GET", URL: "http://x", Cookies: []model.ParsedKV{{Key: "sid", Value: "abc"}, {Key: "t", Value: "dark"}}},
	}
	for i, src := range cases {
		out := persist.MarshalRequest(&src)
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatalf("case %d marshal: %v", i, err)
		}
		got := parseRequestRaw(json.RawMessage(data), "n")
		if got == nil {
			t.Fatalf("case %d parse returned nil", i)
		}
		if got.Auth != src.Auth {
			t.Errorf("case %d auth: got %+v want %+v", i, got.Auth, src.Auth)
		}
		if len(got.Cookies) != len(src.Cookies) {
			t.Fatalf("case %d cookies len: got %d want %d", i, len(got.Cookies), len(src.Cookies))
		}
		for j := range src.Cookies {
			if got.Cookies[j] != src.Cookies[j] {
				t.Errorf("case %d cookie[%d]: got %+v want %+v", i, j, got.Cookies[j], src.Cookies[j])
			}
		}
		if _, ok := got.Extras["auth"]; ok {
			t.Errorf("case %d: auth leaked into Extras", i)
		}
		if _, ok := got.Extras["_rete_cookies"]; ok {
			t.Errorf("case %d: cookies leaked into Extras", i)
		}
	}
}

func TestUnknownAuthPreserved(t *testing.T) {
	raw := json.RawMessage(`{"method":"GET","url":"http://x","auth":{"type":"oauth2","oauth2":[{"key":"accessToken","value":"xyz"}]}}`)
	req := parseRequestRaw(raw, "n")
	if req == nil {
		t.Fatal("nil req")
	}
	if req.Auth.Type != "" {
		t.Errorf("unknown auth should not populate typed Auth, got %q", req.Auth.Type)
	}
	if _, ok := req.Extras["auth"]; !ok {
		t.Fatal("unknown auth not preserved in Extras")
	}
	out := persist.MarshalRequest(req)
	data, _ := json.Marshal(out)
	if !strings.Contains(string(data), "oauth2") {
		t.Errorf("oauth2 auth lost on re-marshal: %s", data)
	}
}

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
					"_rete_cookies":[{"key":"sid","value":"abc"}],
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

func TestParseItemRawNameOrderIndependent(t *testing.T) {
	raw := json.RawMessage(`{"name":"MyReq","request":{"method":"GET","url":"http://x"}}`)
	for i := 0; i < 500; i++ {
		node := parseItemRaw(raw, 0)
		if node == nil {
			t.Fatal("parseItemRaw returned nil")
		}
		if node.Name != "MyReq" {
			t.Fatalf("node.Name = %q, want MyReq", node.Name)
		}
		if node.Request == nil {
			t.Fatal("node.Request is nil")
		}
		if node.Request.Name != "MyReq" {
			t.Fatalf("Request.Name = %q, want MyReq (map iteration order must not drop the name)", node.Request.Name)
		}
	}
}

func TestParseExampleRawNullIsNotAnExample(t *testing.T) {
	cases := []string{"null", "  null  "}
	for _, c := range cases {
		if ex := parseExampleRaw(json.RawMessage(c)); ex != nil {
			t.Errorf("parseExampleRaw(%q) = %+v, want nil (a JSON null must not become a phantom example)", c, ex)
		}
	}
}

func TestParseExampleRawValidStillWorks(t *testing.T) {
	raw := json.RawMessage(`{"name":"Success","originalRequest":{"method":"GET","url":"http://x"}}`)
	ex := parseExampleRaw(raw)
	if ex == nil {
		t.Fatal("parseExampleRaw returned nil for a valid example")
	}
	if ex.Name != "Success" {
		t.Errorf("Name = %q, want Success", ex.Name)
	}
}

func TestParseExampleRawEmptyObjectStillParses(t *testing.T) {
	if ex := parseExampleRaw(json.RawMessage(`{}`)); ex == nil {
		t.Error("an empty object is a (nameless) example, not nil")
	}
}
