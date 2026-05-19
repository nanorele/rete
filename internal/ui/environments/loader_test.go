package environments

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type failingReader struct{}

func (failingReader) Read(p []byte) (int, error) { return 0, errors.New("read fail") }

func TestParseEnvironment_EnabledNilDefaultsTrue(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 1 {
		t.Fatalf("expected 1 var, got %d", len(env.Vars))
	}
	if !env.Vars[0].Enabled {
		t.Errorf("expected Enabled=true when JSON omits enabled (Postman convention), got false")
	}
}

func TestParseEnvironment_EnabledExplicitFalse(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v","enabled":false}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Vars[0].Enabled {
		t.Errorf("expected Enabled=false when explicit, got true")
	}
}

func TestParseEnvironment_EnabledExplicitTrue(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v","enabled":true}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !env.Vars[0].Enabled {
		t.Errorf("expected Enabled=true when explicit, got false")
	}
}

func TestParseEnvironment_HighlightColorPresent(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}],"highlight_color":"#aabbcc"}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "#aabbcc" {
		t.Errorf("expected highlight_color=#aabbcc, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_HighlightColorMissing(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "" {
		t.Errorf("expected empty HighlightColor when absent, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_HighlightColorEmptyString(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}],"highlight_color":""}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.HighlightColor != "" {
		t.Errorf("expected empty HighlightColor, got %q", env.HighlightColor)
	}
}

func TestParseEnvironment_KeyOrderPreserved(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"z","value":"1"},
		{"key":"a","value":"2"},
		{"key":"m","value":"3"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"z", "a", "m"}
	if len(env.Vars) != len(want) {
		t.Fatalf("expected %d vars, got %d", len(want), len(env.Vars))
	}
	for i, k := range want {
		if env.Vars[i].Key != k {
			t.Errorf("at index %d: expected key %q, got %q", i, k, env.Vars[i].Key)
		}
	}
}

func TestParseEnvironment_DuplicateKeysKept(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"k","value":"first"},
		{"key":"k","value":"second"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 2 {
		t.Fatalf("expected 2 vars (duplicates kept), got %d", len(env.Vars))
	}
	if env.Vars[0].Value != "first" || env.Vars[1].Value != "second" {
		t.Errorf("duplicates not preserved in order: %+v", env.Vars)
	}
}

func TestParseEnvironment_EmptyKeyKept(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"","value":"v1"},
		{"key":"k","value":"v2"}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(env.Vars) != 2 {
		t.Errorf("expected empty-key entries to be kept, got %d vars", len(env.Vars))
	}
	if env.Vars[0].Key != "" {
		t.Errorf("expected empty key preserved, got %q", env.Vars[0].Key)
	}
}

func TestParseEnvironment_EmptyValuesArrayWithName(t *testing.T) {
	jsonStr := `{"name":"OnlyName","values":[]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "OnlyName" {
		t.Errorf("expected Name=OnlyName, got %q", env.Name)
	}
	if len(env.Vars) != 0 {
		t.Errorf("expected 0 vars, got %d", len(env.Vars))
	}
}

func TestParseEnvironment_EmptyNameWithValues(t *testing.T) {
	jsonStr := `{"values":[{"key":"k","value":"v"}]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "Imported Environment" {
		t.Errorf("expected fallback name, got %q", env.Name)
	}
}

func TestParseEnvironment_BothEmptyReturnsError(t *testing.T) {
	jsonStr := `{"name":"","values":[]}`
	_, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err == nil {
		t.Errorf("expected error for empty name and empty values")
	}
}

func TestParseEnvironment_MalformedJSON(t *testing.T) {
	cases := []string{
		`{`,
		`{"name":}`,
		`{"name":"x","values":"notarray"}`,
		`{"name":"x","values":[{"key":1}]}`,
	}
	for _, c := range cases {
		_, err := ParseEnvironment(strings.NewReader(c), "id")
		if err == nil {
			t.Errorf("expected error for malformed JSON: %q", c)
		}
	}
}

func TestParseEnvironment_ReaderError(t *testing.T) {
	_, err := ParseEnvironment(failingReader{}, "id")
	if err == nil {
		t.Errorf("expected error when reader fails")
	}
}

func TestParseEnvironment_IDPropagated(t *testing.T) {
	jsonStr := `{"name":"E","values":[{"key":"k","value":"v"}]}`
	cases := []string{"id-1", "", "long-uuid-1234-5678", "with spaces"}
	for _, id := range cases {
		env, err := ParseEnvironment(strings.NewReader(jsonStr), id)
		if err != nil {
			t.Fatalf("unexpected error for id %q: %v", id, err)
		}
		if env.ID != id {
			t.Errorf("expected ID=%q, got %q", id, env.ID)
		}
	}
}

func TestParseEnvironment_BytesReader(t *testing.T) {
	data := []byte(`{"name":"B","values":[{"key":"k","value":"v"}]}`)
	env, err := ParseEnvironment(bytes.NewReader(data), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if env.Name != "B" {
		t.Errorf("expected Name=B, got %q", env.Name)
	}
}

func TestParseEnvironment_EnabledMixedRows(t *testing.T) {
	jsonStr := `{"name":"E","values":[
		{"key":"a","value":"1"},
		{"key":"b","value":"2","enabled":false},
		{"key":"c","value":"3","enabled":true}
	]}`
	env, err := ParseEnvironment(strings.NewReader(jsonStr), "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []bool{true, false, true}
	for i, w := range want {
		if env.Vars[i].Enabled != w {
			t.Errorf("row %d: expected Enabled=%v, got %v", i, w, env.Vars[i].Enabled)
		}
	}
}

func TestHighlightColor_NilEnv(t *testing.T) {
	c := HighlightColor(nil)
	if c.A == 0 {
		t.Errorf("expected non-transparent accent color for nil env")
	}
}

func TestParseEnvironment_EmptyReturnsUnexpectedEOF(t *testing.T) {
	_, err := ParseEnvironment(strings.NewReader(`{"name":"","values":[]}`), "id")
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("expected io.ErrUnexpectedEOF, got %v", err)
	}
}
