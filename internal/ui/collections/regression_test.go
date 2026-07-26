package collections

import (
	"encoding/json"
	"testing"
)

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
