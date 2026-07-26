package model

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/uorg-saver/easyjson/jlexer"
	"github.com/uorg-saver/easyjson/jwriter"
)

type codecPair interface {
	json.Marshaler
	json.Unmarshaler
}

func marshalEasy(t *testing.T, v interface{ MarshalEasyJSON(*jwriter.Writer) }, flags jwriter.Flags) []byte {
	t.Helper()
	w := jwriter.Writer{Flags: flags}
	v.MarshalEasyJSON(&w)
	if w.Error != nil {
		t.Fatalf("MarshalEasyJSON: %v", w.Error)
	}
	data, err := w.BuildBytes()
	if err != nil {
		t.Fatalf("BuildBytes: %v", err)
	}
	return data
}

func unmarshalEasy(t *testing.T, v interface{ UnmarshalEasyJSON(*jlexer.Lexer) }, data []byte) error {
	t.Helper()
	l := jlexer.Lexer{Data: data}
	v.UnmarshalEasyJSON(&l)
	return l.Error()
}

func upperKeys(t *testing.T, data []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("upperKeys: %v", err)
	}
	return mustJSON(t, walkUpper(v))
}

func walkUpper(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[strings.ToUpper(k)] = walkUpper(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = walkUpper(val)
		}
		return out
	default:
		return v
	}
}

func nullValues(t *testing.T, data []byte) []byte {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("nullValues: %v", err)
	}
	out := make(map[string]any, len(m))
	for k := range m {
		out[k] = nil
	}
	return mustJSON(t, out)
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustJSON: %v", err)
	}
	return data
}

func fullSamples() map[string]func() codecPair {
	return map[string]func() codecPair{
		"ThemeSyntaxOverride": func() codecPair {
			return &ThemeSyntaxOverride{
				Plain: "#1", String: "#2", Number: "#3", Bool: "#4", Null: "#5",
				Key: "#6", Punctuation: "#7", Operator: "#8", Keyword: "#9",
				Type: "#a", Comment: "#b", Bracket0: "#c", Bracket1: "#d", Bracket2: "#e",
			}
		},
		"ThemeColorOverride": func() codecPair {
			v := &ThemeColorOverride{}
			rv := reflect.ValueOf(v).Elem()
			for i := 0; i < rv.NumField(); i++ {
				rv.Field(i).SetString("#" + rv.Type().Field(i).Name)
			}
			return v
		},
		"DefaultHeader": func() codecPair { return &DefaultHeader{Key: "X-A", Value: "b"} },
		"CustomTheme": func() codecPair {
			return &CustomTheme{
				ID: "id", Name: "name", BasedOn: "dark",
				Palette: ThemeColorOverride{Bg: "#000", Fg: "#fff"},
				Syntax:  ThemeSyntaxOverride{Key: "#0f0"},
			}
		},
		"AppSettings":       func() codecPair { v := fullAppSettings(); return &v },
		"ExtKVPart":         func() codecPair { return &ExtKVPart{Key: "k", Value: "v", Disabled: true} },
		"ExtFormPart":       func() codecPair { return &ExtFormPart{Key: "k", Value: "v", Type: "file", Src: "p", Disabled: true} },
		"ExtBodyFile":       func() codecPair { return &ExtBodyFile{Src: "s", Content: "c"} },
		"ExtBody":           func() codecPair { v := fullExtBody(); return &v },
		"ExtCollectionInfo": func() codecPair { return &ExtCollectionInfo{Name: "n"} },
		"ExtItem": func() codecPair {
			return &ExtItem{Name: "n", Item: []ExtItem{{Name: "c"}}, Request: json.RawMessage(`{"method":"GET"}`)}
		},
		"ExtCollection": func() codecPair {
			return &ExtCollection{Info: ExtCollectionInfo{Name: "c"}, Item: []ExtItem{{Name: "i"}}}
		},
		"ExtRequest": func() codecPair {
			return &ExtRequest{Method: "POST", URL: "http://x", Header: []any{"h"}, Body: ExtBody{Mode: "raw", Raw: "b"}}
		},
		"ExtEnvironment": func() codecPair {
			return &ExtEnvironment{Name: "E", Values: []ExtEnvVar{{Key: "k", Value: "v"}}, HighlightColor: "#f00"}
		},
		"ExtEnvVar": func() codecPair { return &ExtEnvVar{Key: "k", Value: "v"} },
		"EnvVar":    func() codecPair { return &EnvVar{Key: "k", Value: "v"} },
	}
}

func fullAppSettings() AppSettings {
	s := DefaultSettings()
	rv := reflect.ValueOf(&s).Elem()
	for i := 0; i < rv.NumField(); i++ {
		f := rv.Field(i)
		switch f.Kind() {
		case reflect.String:
			f.SetString("s-" + rv.Type().Field(i).Name)
		case reflect.Int:
			f.SetInt(int64(i + 1))
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Float32:
			f.SetFloat(0.75)
		}
	}
	s.DefaultHeaders = []DefaultHeader{{Key: "X-A", Value: "1"}, {Key: "X-B", Value: "2"}}
	s.SyntaxOverrides = map[string]ThemeSyntaxOverride{"dark": {Plain: "#111"}}
	s.ThemeOverrides = map[string]ThemeColorOverride{"dark": {Bg: "#222"}}
	s.CustomThemes = []CustomTheme{{ID: "c1", Name: "C1", BasedOn: "dark", Palette: ThemeColorOverride{Accent: "#333"}}}
	return s
}

func fullExtBody() ExtBody {
	return ExtBody{
		Mode:       "raw",
		Raw:        "payload",
		URLEncoded: []ExtKVPart{{Key: "a", Value: "1"}, {Key: "b", Value: "2", Disabled: true}},
		FormData:   []ExtFormPart{{Key: "f", Value: "v"}, {Key: "g", Type: "file", Src: "p", Disabled: true}},
		File:       &ExtBodyFile{Src: "s", Content: "c"},
		Disabled:   true,
		Options:    map[string]any{"raw": map[string]any{"language": "json"}},
	}
}

func TestCodec_RoundTrip(t *testing.T) {
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			src := mk()
			data, err := src.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if !json.Valid(data) {
				t.Fatalf("invalid JSON: %s", data)
			}
			dst := mk()
			reflect.ValueOf(dst).Elem().Set(reflect.Zero(reflect.TypeOf(dst).Elem()))
			if err := dst.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", data, err)
			}
			if !reflect.DeepEqual(dst, src) {
				t.Errorf("round-trip mismatch\n json %s\n got  %+v\n want %+v", data, dst, src)
			}
		})
	}
}

func TestCodec_EasyJSONMatchesJSON(t *testing.T) {
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			src := mk()
			want, err := src.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			ew, ok := src.(interface{ MarshalEasyJSON(*jwriter.Writer) })
			if !ok {
				t.Fatalf("%s is not an easyjson Marshaler", name)
			}
			got := marshalEasy(t, ew, 0)
			if string(got) != string(want) {
				t.Errorf("MarshalEasyJSON = %s, MarshalJSON = %s", got, want)
			}
			er, ok := src.(interface{ UnmarshalEasyJSON(*jlexer.Lexer) })
			if !ok {
				t.Fatalf("%s is not an easyjson Unmarshaler", name)
			}
			dst := mk()
			reflect.ValueOf(dst).Elem().Set(reflect.Zero(reflect.TypeOf(dst).Elem()))
			er = dst.(interface{ UnmarshalEasyJSON(*jlexer.Lexer) })
			if err := unmarshalEasy(t, er, want); err != nil {
				t.Fatalf("UnmarshalEasyJSON: %v", err)
			}
			if !reflect.DeepEqual(dst, src) {
				t.Errorf("easyjson round-trip mismatch\n got  %+v\n want %+v", dst, src)
			}
		})
	}
}

func TestCodec_ZeroValueMarshals(t *testing.T) {
	want := map[string]string{
		"ThemeSyntaxOverride": `{}`,
		"ThemeColorOverride":  `{}`,
		"DefaultHeader":       `{"key":"","value":""}`,
		"ExtKVPart":           `{"key":"","value":""}`,
		"ExtFormPart":         `{"key":""}`,
		"ExtBodyFile":         `{}`,
		"ExtBody":             `{}`,
		"ExtCollectionInfo":   `{"name":""}`,
		"ExtItem":             `{"name":"","item":null,"request":null}`,
		"ExtCollection":       `{"info":{"name":""},"item":null}`,
		"ExtEnvVar":           `{"key":"","value":""}`,
		"EnvVar":              `{"key":"","value":""}`,
		"ExtEnvironment":      `{"name":"","values":null}`,
		"ExtRequest":          `{"method":"","url":null,"header":null,"body":{}}`,
		"CustomTheme":         `{"id":"","name":"","palette":{},"syntax":{}}`,
	}
	samples := fullSamples()
	for name, expect := range want {
		t.Run(name, func(t *testing.T) {
			mk, ok := samples[name]
			if !ok {
				t.Fatalf("no sample for %s", name)
			}
			zero := mk()
			reflect.ValueOf(zero).Elem().Set(reflect.Zero(reflect.TypeOf(zero).Elem()))
			data, err := zero.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != expect {
				t.Errorf("zero %s = %s, want %s", name, data, expect)
			}
		})
	}
}

func TestCodec_CaseInsensitiveKeys(t *testing.T) {
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			src := mk()
			data, err := src.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			shouted := upperKeys(t, data)
			dst := mk()
			reflect.ValueOf(dst).Elem().Set(reflect.Zero(reflect.TypeOf(dst).Elem()))
			if err := dst.UnmarshalJSON(shouted); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", shouted, err)
			}
			again, err := dst.MarshalJSON()
			if err != nil {
				t.Fatalf("re-MarshalJSON: %v", err)
			}
			if string(again) == string(mustZeroJSON(t, mk)) && string(data) != string(mustZeroJSON(t, mk)) {
				t.Errorf("uppercase keys were all ignored for %s: %s", name, shouted)
			}
		})
	}
}

func mustZeroJSON(t *testing.T, mk func() codecPair) []byte {
	t.Helper()
	zero := mk()
	reflect.ValueOf(zero).Elem().Set(reflect.Zero(reflect.TypeOf(zero).Elem()))
	data, err := zero.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	return data
}

func TestCodec_NullFieldValuesAreSkipped(t *testing.T) {
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			src := mk()
			data, err := src.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			for _, variant := range [][]byte{nullValues(t, data), upperKeys(t, nullValues(t, data))} {
				dst := mk()
				reflect.ValueOf(dst).Elem().Set(reflect.Zero(reflect.TypeOf(dst).Elem()))
				if err := dst.UnmarshalJSON(variant); err != nil {
					t.Fatalf("UnmarshalJSON(%s): %v", variant, err)
				}
				got, err := dst.MarshalJSON()
				if err != nil {
					t.Fatalf("MarshalJSON: %v", err)
				}
				if string(got) != string(mustZeroJSON(t, mk)) {
					t.Errorf("null values changed state\n json %s\n got  %s\n want %s", variant, got, mustZeroJSON(t, mk))
				}
			}
		})
	}
}

func TestCodec_NullDocumentLeavesValueUntouched(t *testing.T) {
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			dst := mk()
			before, err := dst.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if err := dst.UnmarshalJSON([]byte("null")); err != nil {
				t.Fatalf("UnmarshalJSON(null): %v", err)
			}
			after, err := dst.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(before) != string(after) {
				t.Errorf("null document mutated value: %s -> %s", before, after)
			}
		})
	}
}

func TestCodec_UnknownKeysIgnored(t *testing.T) {
	junk := `"zz_unknown":{"deep":[1,[2,{"x":null}],"s"],"more":{"a":true}},"zz_other":[]`
	for name, mk := range fullSamples() {
		t.Run(name, func(t *testing.T) {
			src := mk()
			data, err := src.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			mixed := []byte(`{` + junk + `,` + string(data[1:]))
			dst := mk()
			reflect.ValueOf(dst).Elem().Set(reflect.Zero(reflect.TypeOf(dst).Elem()))
			if err := dst.UnmarshalJSON(mixed); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", mixed, err)
			}
			if !reflect.DeepEqual(dst, src) {
				t.Errorf("unknown keys perturbed decode\n got  %+v\n want %+v", dst, src)
			}
		})
	}
}

func TestCodec_MalformedInputReportsError(t *testing.T) {
	cases := []struct {
		name string
		mk   func() codecPair
		in   string
	}{
		{"body-truncated", func() codecPair { return &ExtBody{} }, `{"mode":`},
		{"body-wrong-scalar", func() codecPair { return &ExtBody{} }, `{"mode":123}`},
		{"body-array-document", func() codecPair { return &ExtBody{} }, `[]`},
		{"body-trailing-garbage", func() codecPair { return &ExtBody{} }, `{"raw":"a"} junk`},
		{"settings-string-for-int", func() codecPair { return &AppSettings{} }, `{"ui_text_size":"14"}`},
		{"settings-float-for-int", func() codecPair { return &AppSettings{} }, `{"ui_text_size":14.5}`},
		{"settings-bad-bool", func() codecPair { return &AppSettings{} }, `{"verify_ssl":"yes"}`},
		{"item-truncated-request", func() codecPair { return &ExtItem{} }, `{"request":{"a":`},
		{"collection-bad-item", func() codecPair { return &ExtCollection{} }, `{"item":{}}`},
		{"env-bad-values", func() codecPair { return &ExtEnvironment{} }, `{"values":"nope"}`},
		{"header-unterminated", func() codecPair { return &DefaultHeader{} }, `{"key":"a`},
		{"empty-document", func() codecPair { return &ExtBody{} }, ``},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.mk().UnmarshalJSON([]byte(c.in)); err == nil {
				t.Errorf("UnmarshalJSON(%q) = nil error, want error", c.in)
			}
		})
	}
}

func TestExtBody_ContainerEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		check func(t *testing.T, b ExtBody)
	}{
		{"empty-object", `{}`, func(t *testing.T, b ExtBody) {
			if !reflect.DeepEqual(b, ExtBody{}) {
				t.Errorf("got %+v, want zero", b)
			}
		}},
		{"empty-arrays", `{"urlencoded":[],"formdata":[]}`, func(t *testing.T, b ExtBody) {
			if b.URLEncoded == nil || len(b.URLEncoded) != 0 {
				t.Errorf("URLEncoded = %#v, want empty non-nil", b.URLEncoded)
			}
			if b.FormData == nil || len(b.FormData) != 0 {
				t.Errorf("FormData = %#v, want empty non-nil", b.FormData)
			}
		}},
		{"null-arrays", `{"urlencoded":null,"formdata":null,"file":null}`, func(t *testing.T, b ExtBody) {
			if b.URLEncoded != nil || b.FormData != nil || b.File != nil {
				t.Errorf("got %+v, want nil containers", b)
			}
		}},
		{"null-elements", `{"urlencoded":[null],"formdata":[null]}`, func(t *testing.T, b ExtBody) {
			if len(b.URLEncoded) != 1 || b.URLEncoded[0] != (ExtKVPart{}) {
				t.Errorf("URLEncoded = %#v, want one zero element", b.URLEncoded)
			}
			if len(b.FormData) != 1 || b.FormData[0].Key != "" {
				t.Errorf("FormData = %#v, want one zero element", b.FormData)
			}
		}},
		{"empty-options", `{"options":{}}`, func(t *testing.T, b ExtBody) {
			if b.Options != nil {
				t.Errorf("Options = %#v, want nil for empty object", b.Options)
			}
		}},
		{"options-values", `{"options":{"raw":{"language":"json"},"n":1,"b":true,"s":"x","z":null,"a":[1,2]}}`, func(t *testing.T, b ExtBody) {
			if len(b.Options) != 6 {
				t.Fatalf("Options = %#v, want 6 entries", b.Options)
			}
			if b.Options["z"] != nil {
				t.Errorf("Options[z] = %#v, want nil", b.Options["z"])
			}
			if b.Options["s"] != "x" {
				t.Errorf("Options[s] = %#v", b.Options["s"])
			}
		}},
		{"file-object", `{"file":{"src":"p","content":"c"}}`, func(t *testing.T, b ExtBody) {
			if b.File == nil || b.File.Src != "p" || b.File.Content != "c" {
				t.Errorf("File = %+v", b.File)
			}
		}},
		{"unicode-raw", `{"raw":"日本語 😀 \u0000 tail"}`, func(t *testing.T, b ExtBody) {
			if !strings.Contains(b.Raw, "日本語") {
				t.Errorf("Raw = %q", b.Raw)
			}
			if !strings.Contains(b.Raw, "\x00") {
				t.Errorf("Raw lost NUL: %q", b.Raw)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var b ExtBody
			if err := b.UnmarshalJSON([]byte(c.in)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", c.in, err)
			}
			c.check(t, b)
		})
	}
}

func TestExtBody_DecodeIntoReusedValueResetsSlices(t *testing.T) {
	b := ExtBody{
		Mode:       "raw",
		URLEncoded: []ExtKVPart{{Key: "old1"}, {Key: "old2"}, {Key: "old3"}},
		FormData:   []ExtFormPart{{Key: "old1"}, {Key: "old2"}},
		File:       &ExtBodyFile{Src: "old"},
	}
	if err := b.UnmarshalJSON([]byte(`{"urlencoded":[{"key":"new"}],"formdata":[{"key":"new"}],"file":{"src":"new"}}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if len(b.URLEncoded) != 1 || b.URLEncoded[0].Key != "new" {
		t.Errorf("URLEncoded = %#v", b.URLEncoded)
	}
	if len(b.FormData) != 1 || b.FormData[0].Key != "new" {
		t.Errorf("FormData = %#v", b.FormData)
	}
	if b.File == nil || b.File.Src != "new" {
		t.Errorf("File = %+v", b.File)
	}
	if b.Mode != "raw" {
		t.Errorf("Mode = %q, absent key must not clear existing value", b.Mode)
	}
}

func TestExtItem_NestedAndReuse(t *testing.T) {
	depth := 40
	src := strings.Repeat(`{"item":[`, depth) + `{"name":"leaf","request":{"method":"GET"}}` + strings.Repeat(`]}`, depth)
	var it ExtItem
	if err := it.UnmarshalJSON([]byte(src)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	cur := &it
	for i := 0; i < depth; i++ {
		if len(cur.Item) != 1 {
			t.Fatalf("depth %d: len(Item) = %d", i, len(cur.Item))
		}
		cur = &cur.Item[0]
	}
	if cur.Name != "leaf" || string(cur.Request) != `{"method":"GET"}` {
		t.Errorf("leaf = %+v", cur)
	}

	reused := ExtItem{Item: []ExtItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	if err := reused.UnmarshalJSON([]byte(`{"item":[{"name":"z"}]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if len(reused.Item) != 1 || reused.Item[0].Name != "z" {
		t.Errorf("reused.Item = %#v", reused.Item)
	}

	var empty ExtItem
	if err := empty.UnmarshalJSON([]byte(`{"item":[]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if empty.Item == nil || len(empty.Item) != 0 {
		t.Errorf("Item = %#v, want empty non-nil", empty.Item)
	}

	var nulled ExtItem
	nulled.Item = []ExtItem{{Name: "x"}}
	if err := nulled.UnmarshalJSON([]byte(`{"item":null}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if nulled.Item != nil {
		t.Errorf("Item = %#v, want nil", nulled.Item)
	}
}

func TestExtItem_NilSliceAsEmptyFlag(t *testing.T) {
	it := ExtItem{Name: "n"}
	got := marshalEasy(t, it, jwriter.NilSliceAsEmpty)
	if !strings.Contains(string(got), `"item":[]`) {
		t.Errorf("with NilSliceAsEmpty got %s, want empty array for item", got)
	}
	plain := marshalEasy(t, it, 0)
	if !strings.Contains(string(plain), `"item":null`) {
		t.Errorf("without flags got %s, want null for item", plain)
	}

	c := ExtCollection{Info: ExtCollectionInfo{Name: "c"}}
	gotC := marshalEasy(t, c, jwriter.NilSliceAsEmpty)
	if !strings.Contains(string(gotC), `"item":[]`) {
		t.Errorf("collection with NilSliceAsEmpty got %s", gotC)
	}

	e := ExtEnvironment{Name: "e"}
	gotE := marshalEasy(t, e, jwriter.NilSliceAsEmpty)
	if !strings.Contains(string(gotE), `"values":[]`) {
		t.Errorf("environment with NilSliceAsEmpty got %s", gotE)
	}
}

func TestExtRequest_URLAndHeaderShapes(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		verify func(t *testing.T, r ExtRequest)
	}{
		{"string-url", `{"method":"GET","url":"https://example.com"}`, func(t *testing.T, r ExtRequest) {
			if r.URL != "https://example.com" {
				t.Errorf("URL = %#v", r.URL)
			}
		}},
		{"object-url", `{"url":{"raw":"https://x/y","host":["x"],"path":["y"]}}`, func(t *testing.T, r ExtRequest) {
			m, ok := r.URL.(map[string]any)
			if !ok {
				t.Fatalf("URL = %#v, want map", r.URL)
			}
			if m["raw"] != "https://x/y" {
				t.Errorf("URL.raw = %#v", m["raw"])
			}
		}},
		{"null-url-and-header", `{"url":null,"header":null}`, func(t *testing.T, r ExtRequest) {
			if r.URL != nil || r.Header != nil {
				t.Errorf("URL = %#v, Header = %#v, want nil", r.URL, r.Header)
			}
		}},
		{"header-array", `{"header":[{"key":"A","value":"1"}]}`, func(t *testing.T, r ExtRequest) {
			arr, ok := r.Header.([]any)
			if !ok || len(arr) != 1 {
				t.Fatalf("Header = %#v", r.Header)
			}
		}},
		{"header-string", `{"header":"A: 1"}`, func(t *testing.T, r ExtRequest) {
			if r.Header != "A: 1" {
				t.Errorf("Header = %#v", r.Header)
			}
		}},
		{"nested-body", `{"body":{"mode":"formdata","formdata":[{"key":"f","type":"file","src":"p"}]}}`, func(t *testing.T, r ExtRequest) {
			if r.Body.Mode != "formdata" || len(r.Body.FormData) != 1 || r.Body.FormData[0].Src != "p" {
				t.Errorf("Body = %+v", r.Body)
			}
		}},
		{"null-body", `{"body":null}`, func(t *testing.T, r ExtRequest) {
			if !reflect.DeepEqual(r.Body, ExtBody{}) {
				t.Errorf("Body = %+v, want zero", r.Body)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var r ExtRequest
			if err := r.UnmarshalJSON([]byte(c.in)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", c.in, err)
			}
			c.verify(t, r)
		})
	}
}

func TestExtFormPart_SrcShapes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want any
	}{
		{"string", `{"key":"f","src":"C:/a/b.txt"}`, "C:/a/b.txt"},
		{"null", `{"key":"f","src":null}`, nil},
		{"array", `{"key":"f","src":["a","b"]}`, []any{"a", "b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var p ExtFormPart
			if err := p.UnmarshalJSON([]byte(c.in)); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if !reflect.DeepEqual(p.Src, c.want) {
				t.Errorf("Src = %#v, want %#v", p.Src, c.want)
			}
		})
	}
}

func TestAppSettings_MapAndSliceEdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		seed  AppSettings
		in    string
		check func(t *testing.T, s AppSettings)
	}{
		{"empty-maps-become-nil", AppSettings{SyntaxOverrides: map[string]ThemeSyntaxOverride{"a": {}}},
			`{"syntax_overrides":{},"theme_overrides":{}}`,
			func(t *testing.T, s AppSettings) {
				if s.SyntaxOverrides != nil || s.ThemeOverrides != nil {
					t.Errorf("want nil maps, got %#v / %#v", s.SyntaxOverrides, s.ThemeOverrides)
				}
			}},
		{"null-maps-keep-existing", AppSettings{SyntaxOverrides: map[string]ThemeSyntaxOverride{"a": {Plain: "p"}}},
			`{"syntax_overrides":null,"theme_overrides":null,"custom_themes":null}`,
			func(t *testing.T, s AppSettings) {
				if len(s.SyntaxOverrides) != 1 {
					t.Errorf("null must be skipped, got %#v", s.SyntaxOverrides)
				}
			}},
		{"populated-maps", AppSettings{},
			`{"syntax_overrides":{"dark":{"plain":"#1"},"light":null},"theme_overrides":{"dark":{"bg":"#2"}}}`,
			func(t *testing.T, s AppSettings) {
				if s.SyntaxOverrides["dark"].Plain != "#1" {
					t.Errorf("syntax_overrides = %#v", s.SyntaxOverrides)
				}
				if s.SyntaxOverrides["light"] != (ThemeSyntaxOverride{}) {
					t.Errorf("null map value should decode to zero, got %#v", s.SyntaxOverrides["light"])
				}
				if s.ThemeOverrides["dark"].Bg != "#2" {
					t.Errorf("theme_overrides = %#v", s.ThemeOverrides)
				}
			}},
		{"headers-reset", AppSettings{DefaultHeaders: []DefaultHeader{{Key: "a"}, {Key: "b"}}},
			`{"default_headers":[{"key":"c","value":"1"}]}`,
			func(t *testing.T, s AppSettings) {
				if len(s.DefaultHeaders) != 1 || s.DefaultHeaders[0].Key != "c" {
					t.Errorf("DefaultHeaders = %#v", s.DefaultHeaders)
				}
			}},
		{"headers-empty-array", AppSettings{}, `{"default_headers":[]}`,
			func(t *testing.T, s AppSettings) {
				if s.DefaultHeaders == nil || len(s.DefaultHeaders) != 0 {
					t.Errorf("DefaultHeaders = %#v, want empty non-nil", s.DefaultHeaders)
				}
			}},
		{"headers-null", AppSettings{DefaultHeaders: []DefaultHeader{{Key: "a"}}}, `{"default_headers":null}`,
			func(t *testing.T, s AppSettings) {
				if s.DefaultHeaders != nil {
					t.Errorf("DefaultHeaders = %#v, want nil", s.DefaultHeaders)
				}
			}},
		{"custom-themes", AppSettings{},
			`{"custom_themes":[{"id":"a","name":"A","palette":{"bg":"#0"},"syntax":{"key":"#1"}},null]}`,
			func(t *testing.T, s AppSettings) {
				if len(s.CustomThemes) != 2 {
					t.Fatalf("CustomThemes = %#v", s.CustomThemes)
				}
				if s.CustomThemes[0].Palette.Bg != "#0" || s.CustomThemes[0].Syntax.Key != "#1" {
					t.Errorf("CustomThemes[0] = %+v", s.CustomThemes[0])
				}
				if s.CustomThemes[1].ID != "" {
					t.Errorf("null element should be zero, got %+v", s.CustomThemes[1])
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := c.seed
			if err := s.UnmarshalJSON([]byte(c.in)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", c.in, err)
			}
			c.check(t, s)
		})
	}
}

func TestAppSettings_PartialDecodePreservesUnmentionedFields(t *testing.T) {
	s := DefaultSettings()
	if err := s.UnmarshalJSON([]byte(`{"theme":"light","ui_text_size":20}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if s.Theme != "light" || s.UITextSize != 20 {
		t.Errorf("mentioned fields not applied: %+v", s)
	}
	def := DefaultSettings()
	if s.UserAgent != def.UserAgent || s.MaxRedirects != def.MaxRedirects || !s.VerifySSL {
		t.Errorf("unmentioned fields were reset: %+v", s)
	}
}

func TestAppSettings_OmitemptyZeroFields(t *testing.T) {
	s := DefaultSettings()
	s.StackBreakpointDp = 0
	s.DefaultSidebarWidthPx = 0
	s.StickyMaxLines = 0
	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"default_sidebar_width_px", "sticky_max_lines", "syntax_overrides", "theme_overrides", "custom_themes"} {
		if _, ok := m[k]; ok {
			t.Errorf("%s present in JSON despite zero value: %s", k, data)
		}
	}
	for _, k := range []string{"theme", "ui_text_size", "verify_ssl", "default_headers", "ui_scale"} {
		if _, ok := m[k]; !ok {
			t.Errorf("%s missing from JSON: %s", k, data)
		}
	}
	if _, ok := m["stack_breakpoint_dp"]; !ok {
		t.Errorf("stack_breakpoint_dp omitted at zero, so the settings UI's documented "+
			"\"0 = never stack\" choice cannot be saved: %s", data)
	}
}

func TestAppSettings_StackBreakpointZeroSurvivesRoundTrip(t *testing.T) {
	s := DefaultSettings()
	s.StackBreakpointDp = 0
	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	back := DefaultSettings()
	if err := back.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if back.StackBreakpointDp != 0 {
		t.Errorf("StackBreakpointDp = %d after round trip onto defaults, want 0", back.StackBreakpointDp)
	}
}

func TestAppSettings_AllTaggedFieldsAppearInJSON(t *testing.T) {
	s := fullAppSettings()
	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rt := reflect.TypeOf(s)
	for i := 0; i < rt.NumField(); i++ {
		tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		if _, ok := m[tag]; !ok {
			t.Errorf("field %s (tag %q) missing from marshalled settings", rt.Field(i).Name, tag)
		}
	}
	if len(m) != rt.NumField() {
		t.Errorf("marshalled %d keys, struct has %d fields", len(m), rt.NumField())
	}
}

func TestThemeOverride_AllTaggedFieldsRoundTrip(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(ThemeColorOverride{}),
		reflect.TypeOf(ThemeSyntaxOverride{}),
	}
	for _, rt := range types {
		t.Run(rt.Name(), func(t *testing.T) {
			pv := reflect.New(rt)
			for i := 0; i < rt.NumField(); i++ {
				pv.Elem().Field(i).SetString("#" + rt.Field(i).Name)
			}
			data, err := pv.Interface().(json.Marshaler).MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			var m map[string]string
			if err := json.Unmarshal(data, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(m) != rt.NumField() {
				t.Errorf("%s marshalled %d keys, struct has %d fields", rt.Name(), len(m), rt.NumField())
			}
			back := reflect.New(rt)
			if err := back.Interface().(json.Unmarshaler).UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if !reflect.DeepEqual(back.Elem().Interface(), pv.Elem().Interface()) {
				t.Errorf("%s round-trip mismatch", rt.Name())
			}
			for i := 0; i < rt.NumField(); i++ {
				tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
				single := []byte(`{` + strconvQuote(tag) + `:"solo"}`)
				one := reflect.New(rt)
				if err := one.Interface().(json.Unmarshaler).UnmarshalJSON(single); err != nil {
					t.Fatalf("UnmarshalJSON(%s): %v", single, err)
				}
				if one.Elem().Field(i).String() != "solo" {
					t.Errorf("%s.%s not decoded from key %q", rt.Name(), rt.Field(i).Name, tag)
				}
			}
		})
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestExtEnvironment_Decode(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		check func(t *testing.T, e ExtEnvironment)
	}{
		{"full", `{"name":"Dev","highlight_color":"#f80","values":[{"key":"h","value":"l"}]}`,
			func(t *testing.T, e ExtEnvironment) {
				if e.Name != "Dev" || e.HighlightColor != "#f80" || len(e.Values) != 1 {
					t.Errorf("got %+v", e)
				}
			}},
		{"extra-postman-fields", `{"name":"D","values":[{"key":"k","value":"v","enabled":false,"type":"secret"}],"_postman_variable_scope":"environment"}`,
			func(t *testing.T, e ExtEnvironment) {
				if len(e.Values) != 1 || e.Values[0].Key != "k" || e.Values[0].Value != "v" {
					t.Errorf("got %+v", e)
				}
			}},
		{"null-values", `{"name":"D","values":null}`, func(t *testing.T, e ExtEnvironment) {
			if e.Values != nil {
				t.Errorf("Values = %#v, want nil", e.Values)
			}
		}},
		{"empty-values", `{"values":[]}`, func(t *testing.T, e ExtEnvironment) {
			if e.Values == nil || len(e.Values) != 0 {
				t.Errorf("Values = %#v, want empty non-nil", e.Values)
			}
		}},
		{"null-element", `{"values":[null,{"key":"k"}]}`, func(t *testing.T, e ExtEnvironment) {
			if len(e.Values) != 2 || e.Values[0] != (ExtEnvVar{}) || e.Values[1].Key != "k" {
				t.Errorf("Values = %#v", e.Values)
			}
		}},
		{"unicode", `{"name":"日本語","values":[{"key":"😀","value":"\t\n\"\\"}]}`,
			func(t *testing.T, e ExtEnvironment) {
				if e.Name != "日本語" {
					t.Errorf("Name = %q", e.Name)
				}
				if e.Values[0].Value != "\t\n\"\\" {
					t.Errorf("Value = %q", e.Values[0].Value)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var e ExtEnvironment
			if err := e.UnmarshalJSON([]byte(c.in)); err != nil {
				t.Fatalf("UnmarshalJSON(%s): %v", c.in, err)
			}
			c.check(t, e)
		})
	}
}

func TestExtEnvVar_ConvertibleToEnvVar(t *testing.T) {
	src := ExtEnvVar{Key: "k", Value: "v"}
	got := EnvVar(src)
	if got.Key != "k" || got.Value != "v" {
		t.Errorf("conversion lost data: %+v", got)
	}
	back := ExtEnvVar(got)
	if back != src {
		t.Errorf("round-trip conversion mismatch: %+v", back)
	}
}

func TestExtCollection_Decode(t *testing.T) {
	src := `{
		"info": {"name": "Coll", "_postman_id": "abc", "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"},
		"item": [
			{"name": "req", "request": {"method": "GET", "url": "https://example.com"}},
			{"name": "folder", "item": [{"name": "nested", "request": {"method": "POST"}}]},
			null
		],
		"variable": [{"key": "v", "value": "1"}]
	}`
	var c ExtCollection
	if err := c.UnmarshalJSON([]byte(src)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if c.Info.Name != "Coll" {
		t.Errorf("Info.Name = %q", c.Info.Name)
	}
	if len(c.Item) != 3 {
		t.Fatalf("len(Item) = %d", len(c.Item))
	}
	if c.Item[0].Name != "req" || len(c.Item[0].Request) == 0 {
		t.Errorf("Item[0] = %+v", c.Item[0])
	}
	if len(c.Item[1].Item) != 1 || c.Item[1].Item[0].Name != "nested" {
		t.Errorf("Item[1] = %+v", c.Item[1])
	}
	if c.Item[2].Name != "" || c.Item[2].Item != nil {
		t.Errorf("null element should be zero: %+v", c.Item[2])
	}

	var req ExtRequest
	if err := req.UnmarshalJSON(c.Item[0].Request); err != nil {
		t.Fatalf("nested request: %v", err)
	}
	if req.Method != "GET" || req.URL != "https://example.com" {
		t.Errorf("nested request = %+v", req)
	}
}

func TestExtCollection_ReuseResetsItems(t *testing.T) {
	c := ExtCollection{Item: []ExtItem{{Name: "a"}, {Name: "b"}}}
	if err := c.UnmarshalJSON([]byte(`{"item":[{"name":"z"}]}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if len(c.Item) != 1 || c.Item[0].Name != "z" {
		t.Errorf("Item = %#v", c.Item)
	}
	if err := c.UnmarshalJSON([]byte(`{"item":null}`)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if c.Item != nil {
		t.Errorf("Item = %#v, want nil", c.Item)
	}
}

func TestExtBodyFile_Omitempty(t *testing.T) {
	cases := []struct {
		in   ExtBodyFile
		want string
	}{
		{ExtBodyFile{}, `{}`},
		{ExtBodyFile{Src: "s"}, `{"src":"s"}`},
		{ExtBodyFile{Content: "c"}, `{"content":"c"}`},
		{ExtBodyFile{Src: "s", Content: "c"}, `{"src":"s","content":"c"}`},
	}
	for _, c := range cases {
		data, err := c.in.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != c.want {
			t.Errorf("%+v marshalled to %s, want %s", c.in, data, c.want)
		}
	}
}

func TestExtKVPart_Omitempty(t *testing.T) {
	cases := []struct {
		in   ExtKVPart
		want string
	}{
		{ExtKVPart{}, `{"key":"","value":""}`},
		{ExtKVPart{Key: "k", Value: "v"}, `{"key":"k","value":"v"}`},
		{ExtKVPart{Key: "k", Disabled: true}, `{"key":"k","value":"","disabled":true}`},
	}
	for _, c := range cases {
		data, err := c.in.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != c.want {
			t.Errorf("%+v marshalled to %s, want %s", c.in, data, c.want)
		}
	}
}

func TestExtFormPart_Omitempty(t *testing.T) {
	cases := []struct {
		in   ExtFormPart
		want string
	}{
		{ExtFormPart{}, `{"key":""}`},
		{ExtFormPart{Key: "k"}, `{"key":"k"}`},
		{ExtFormPart{Key: "k", Value: "v"}, `{"key":"k","value":"v"}`},
		{ExtFormPart{Key: "k", Type: "file", Src: "p"}, `{"key":"k","type":"file","src":"p"}`},
		{ExtFormPart{Key: "k", Disabled: true}, `{"key":"k","disabled":true}`},
	}
	for _, c := range cases {
		data, err := c.in.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != c.want {
			t.Errorf("%+v marshalled to %s, want %s", c.in, data, c.want)
		}
	}
}

func TestExtBody_MarshalOmitempty(t *testing.T) {
	cases := []struct {
		name string
		in   ExtBody
		want string
	}{
		{"zero", ExtBody{}, `{}`},
		{"mode-only", ExtBody{Mode: "none"}, `{"mode":"none"}`},
		{"raw-only", ExtBody{Raw: "x"}, `{"raw":"x"}`},
		{"urlencoded-only", ExtBody{URLEncoded: []ExtKVPart{{Key: "a", Value: "1"}}}, `{"urlencoded":[{"key":"a","value":"1"}]}`},
		{"formdata-only", ExtBody{FormData: []ExtFormPart{{Key: "a"}}}, `{"formdata":[{"key":"a"}]}`},
		{"file-only", ExtBody{File: &ExtBodyFile{Src: "p"}}, `{"file":{"src":"p"}}`},
		{"disabled-only", ExtBody{Disabled: true}, `{"disabled":true}`},
		{"empty-slices-omitted", ExtBody{URLEncoded: []ExtKVPart{}, FormData: []ExtFormPart{}, Options: map[string]any{}}, `{}`},
		{"mode-and-raw", ExtBody{Mode: "raw", Raw: "x"}, `{"mode":"raw","raw":"x"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != c.want {
				t.Errorf("got %s, want %s", data, c.want)
			}
		})
	}
}

func TestCustomTheme_Omitempty(t *testing.T) {
	cases := []struct {
		in   CustomTheme
		want string
	}{
		{CustomTheme{}, `{"id":"","name":"","palette":{},"syntax":{}}`},
		{CustomTheme{ID: "a", Name: "A"}, `{"id":"a","name":"A","palette":{},"syntax":{}}`},
		{CustomTheme{ID: "a", Name: "A", BasedOn: "dark"}, `{"id":"a","name":"A","based_on":"dark","palette":{},"syntax":{}}`},
	}
	for _, c := range cases {
		data, err := c.in.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != c.want {
			t.Errorf("%+v marshalled to %s, want %s", c.in, data, c.want)
		}
	}
}

func TestCodec_LargeInputs(t *testing.T) {
	const n = 2000
	var sb strings.Builder
	sb.WriteString(`{"mode":"urlencoded","urlencoded":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"key":"k","value":"` + strings.Repeat("v", 64) + `"}`)
	}
	sb.WriteString(`]}`)

	var b ExtBody
	if err := b.UnmarshalJSON([]byte(sb.String())); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if len(b.URLEncoded) != n {
		t.Fatalf("len(URLEncoded) = %d, want %d", len(b.URLEncoded), n)
	}
	data, err := b.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var back ExtBody
	if err := back.UnmarshalJSON(data); err != nil {
		t.Fatalf("re-UnmarshalJSON: %v", err)
	}
	if !reflect.DeepEqual(back, b) {
		t.Error("large body round-trip mismatch")
	}

	big := strings.Repeat("x", 1<<20)
	one := ExtBody{Raw: big}
	oneData, err := one.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var oneBack ExtBody
	if err := oneBack.UnmarshalJSON(oneData); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if oneBack.Raw != big {
		t.Errorf("large raw body corrupted: len %d, want %d", len(oneBack.Raw), len(big))
	}
}

func TestDefaultSettings_IsStableAndIndependent(t *testing.T) {
	a := DefaultSettings()
	b := DefaultSettings()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("DefaultSettings() is not deterministic")
	}
	a.Theme = "mutated"
	a.DefaultHeaders = append(a.DefaultHeaders, DefaultHeader{Key: "x"})
	if DefaultSettings().Theme != "dark" || DefaultSettings().DefaultHeaders != nil {
		t.Error("mutating a returned value affected later DefaultSettings() calls")
	}
}

func TestDefaultSettings_WithinSaneRanges(t *testing.T) {
	s := DefaultSettings()
	checks := []struct {
		name     string
		got      int
		min, max int
	}{
		{"UITextSize", s.UITextSize, 10, 28},
		{"BodyTextSize", s.BodyTextSize, 10, 28},
		{"MaxTabRows", s.MaxTabRows, 1, 10},
		{"MaxRedirects", s.MaxRedirects, 0, 50},
		{"JSONIndentSpaces", s.JSONIndentSpaces, 0, 8},
		{"PreviewMaxMB", s.PreviewMaxMB, 1, 500},
		{"SyntaxHighlightMaxMB", s.SyntaxHighlightMaxMB, 1, 500},
		{"ResponseBodyPadding", s.ResponseBodyPadding, 0, 32},
		{"StackBreakpointDp", s.StackBreakpointDp, 400, 2000},
		{"DefaultSidebarWidthPx", s.DefaultSidebarWidthPx, 160, 1000},
		{"StickyMaxLines", s.StickyMaxLines, 1, 12},
		{"RequestTimeoutSec", s.RequestTimeoutSec, 0, 3600},
		{"ConnectTimeoutSec", s.ConnectTimeoutSec, 0, 600},
		{"TLSHandshakeTimeoutSec", s.TLSHandshakeTimeoutSec, 0, 600},
		{"IdleConnTimeoutSec", s.IdleConnTimeoutSec, 0, 3600},
		{"MaxConnsPerHost", s.MaxConnsPerHost, 0, 10000},
	}
	for _, c := range checks {
		if c.got < c.min || c.got > c.max {
			t.Errorf("%s = %d, outside accepted range [%d, %d]", c.name, c.got, c.min, c.max)
		}
	}
	if s.UIScale < 0.75 || s.UIScale > 2.0 {
		t.Errorf("UIScale = %v, outside [0.75, 2.0]", s.UIScale)
	}
	if s.DefaultSplitRatio < 0.2 || s.DefaultSplitRatio > 0.8 {
		t.Errorf("DefaultSplitRatio = %v, outside [0.2, 0.8]", s.DefaultSplitRatio)
	}
	if s.DefaultMethod != strings.ToUpper(s.DefaultMethod) {
		t.Errorf("DefaultMethod = %q, want upper case", s.DefaultMethod)
	}
}

func TestDefaultSettings_SurvivesJSONRoundTrip(t *testing.T) {
	s := DefaultSettings()
	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	var back AppSettings
	if err := back.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	if !reflect.DeepEqual(back, s) {
		t.Errorf("round-trip changed defaults\n got  %+v\n want %+v", back, s)
	}
}

func TestThemeOverride_EachFieldMarshalsAlone(t *testing.T) {
	types := []reflect.Type{
		reflect.TypeOf(ThemeColorOverride{}),
		reflect.TypeOf(ThemeSyntaxOverride{}),
	}
	for _, rt := range types {
		t.Run(rt.Name(), func(t *testing.T) {
			for i := 0; i < rt.NumField(); i++ {
				tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
				pv := reflect.New(rt)
				pv.Elem().Field(i).SetString("#solo")
				data, err := pv.Interface().(json.Marshaler).MarshalJSON()
				if err != nil {
					t.Fatalf("MarshalJSON: %v", err)
				}
				want := `{` + strconvQuote(tag) + `:"#solo"}`
				if string(data) != want {
					t.Errorf("%s.%s alone = %s, want %s", rt.Name(), rt.Field(i).Name, data, want)
				}
				back := reflect.New(rt)
				if err := back.Interface().(json.Unmarshaler).UnmarshalJSON(data); err != nil {
					t.Fatalf("UnmarshalJSON(%s): %v", data, err)
				}
				if !reflect.DeepEqual(back.Elem().Interface(), pv.Elem().Interface()) {
					t.Errorf("%s.%s solo round-trip mismatch", rt.Name(), rt.Field(i).Name)
				}
			}
		})
	}
}

func TestExtBody_EachOptionalFieldAlone(t *testing.T) {
	cases := []struct {
		name string
		in   ExtBody
		want string
	}{
		{"options", ExtBody{Options: map[string]any{"language": "json"}}, `{"options":{"language":"json"}}`},
		{"file-and-disabled", ExtBody{File: &ExtBodyFile{Content: "c"}, Disabled: true}, `{"file":{"content":"c"},"disabled":true}`},
		{"formdata-and-options", ExtBody{FormData: []ExtFormPart{{Key: "a"}}, Options: map[string]any{"n": float64(1)}}, `{"formdata":[{"key":"a"}],"options":{"n":1}}`},
		{"urlencoded-and-file", ExtBody{URLEncoded: []ExtKVPart{{Key: "a", Value: "b"}}, File: &ExtBodyFile{Src: "s"}}, `{"urlencoded":[{"key":"a","value":"b"}],"file":{"src":"s"}}`},
		{"raw-and-disabled", ExtBody{Raw: "r", Disabled: true}, `{"raw":"r","disabled":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != c.want {
				t.Errorf("got %s, want %s", data, c.want)
			}
			var back ExtBody
			if err := back.UnmarshalJSON(data); err != nil {
				t.Fatalf("UnmarshalJSON: %v", err)
			}
			if !reflect.DeepEqual(back, c.in) {
				t.Errorf("round-trip mismatch\n got  %+v\n want %+v", back, c.in)
			}
		})
	}
}

func TestExtFormPart_EachOptionalFieldAlone(t *testing.T) {
	cases := []struct {
		in   ExtFormPart
		want string
	}{
		{ExtFormPart{Src: "s"}, `{"key":"","src":"s"}`},
		{ExtFormPart{Type: "text"}, `{"key":"","type":"text"}`},
		{ExtFormPart{Value: "v"}, `{"key":"","value":"v"}`},
		{ExtFormPart{Disabled: true}, `{"key":"","disabled":true}`},
		{ExtFormPart{Src: "s", Disabled: true}, `{"key":"","src":"s","disabled":true}`},
	}
	for _, c := range cases {
		data, err := c.in.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != c.want {
			t.Errorf("%+v marshalled to %s, want %s", c.in, data, c.want)
		}
	}
}

func TestExtEnvironment_HighlightColorAlone(t *testing.T) {
	e := ExtEnvironment{HighlightColor: "#abc"}
	data, err := e.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(data) != `{"name":"","values":null,"highlight_color":"#abc"}` {
		t.Errorf("got %s", data)
	}
}

func TestExtRequest_MarshalShapes(t *testing.T) {
	cases := []struct {
		name string
		in   ExtRequest
		want string
	}{
		{"zero", ExtRequest{}, `{"method":"","url":null,"header":null,"body":{}}`},
		{"map-url", ExtRequest{Method: "GET", URL: map[string]any{"raw": "u"}}, `{"method":"GET","url":{"raw":"u"},"header":null,"body":{}}`},
		{"raw-message-url", ExtRequest{URL: json.RawMessage(`{"raw":"u"}`)}, `{"method":"","url":{"raw":"u"},"header":null,"body":{}}`},
		{"header-slice", ExtRequest{Header: []string{"a"}}, `{"method":"","url":null,"header":["a"],"body":{}}`},
		{"raw-message-header", ExtRequest{Header: json.RawMessage(`[{"key":"A"}]`)}, `{"method":"","url":null,"header":[{"key":"A"}],"body":{}}`},
		{"with-body", ExtRequest{Body: ExtBody{Mode: "raw", Raw: "x"}}, `{"method":"","url":null,"header":null,"body":{"mode":"raw","raw":"x"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != c.want {
				t.Errorf("got %s, want %s", data, c.want)
			}
		})
	}
}

func TestExtCollection_MarshalShapes(t *testing.T) {
	cases := []struct {
		name string
		in   ExtCollection
		want string
	}{
		{"empty-item-slice", ExtCollection{Item: []ExtItem{}}, `{"info":{"name":""},"item":[]}`},
		{"named", ExtCollection{Info: ExtCollectionInfo{Name: "C"}}, `{"info":{"name":"C"},"item":null}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != c.want {
				t.Errorf("got %s, want %s", data, c.want)
			}
		})
	}
}

func TestExtItem_MarshalShapes(t *testing.T) {
	cases := []struct {
		name string
		in   ExtItem
		want string
	}{
		{"empty-children", ExtItem{Name: "n", Item: []ExtItem{}}, `{"name":"n","item":[],"request":null}`},
		{"two-children", ExtItem{Item: []ExtItem{{Name: "a"}, {Name: "b"}}}, `{"name":"","item":[{"name":"a","item":null,"request":null},{"name":"b","item":null,"request":null}],"request":null}`},
		{"raw-request", ExtItem{Request: json.RawMessage(`{"method":"GET"}`)}, `{"name":"","item":null,"request":{"method":"GET"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := c.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(data) != c.want {
				t.Errorf("got %s, want %s", data, c.want)
			}
		})
	}
}
