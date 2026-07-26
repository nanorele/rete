package model

import (
	"encoding/json"
	"testing"
)

func TestExtRequest_PrePopulatedInterfaceFieldsAreReused(t *testing.T) {
	t.Run("easyjson-unmarshaler", func(t *testing.T) {
		r := ExtRequest{URL: &ExtBodyFile{}, Header: &ExtBodyFile{}}
		if err := r.UnmarshalJSON([]byte(`{"url":{"src":"u"},"header":{"src":"h"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		u, ok := r.URL.(*ExtBodyFile)
		if !ok || u.Src != "u" {
			t.Errorf("URL = %#v, want *ExtBodyFile{Src:u}", r.URL)
		}
		h, ok := r.Header.(*ExtBodyFile)
		if !ok || h.Src != "h" {
			t.Errorf("Header = %#v, want *ExtBodyFile{Src:h}", r.Header)
		}
	})
	t.Run("json-unmarshaler", func(t *testing.T) {
		var raw, rawH json.RawMessage
		r := ExtRequest{URL: &raw, Header: &rawH}
		if err := r.UnmarshalJSON([]byte(`{"url":{"a":1},"header":[2]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if string(raw) != `{"a":1}` {
			t.Errorf("URL raw = %s", raw)
		}
		if string(rawH) != `[2]` {
			t.Errorf("Header raw = %s", rawH)
		}
	})
	t.Run("uppercase-keys", func(t *testing.T) {
		r := ExtRequest{URL: &ExtBodyFile{}, Header: &ExtBodyFile{}}
		if err := r.UnmarshalJSON([]byte(`{"URL":{"src":"u"},"HEADER":{"src":"h"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if u, ok := r.URL.(*ExtBodyFile); !ok || u.Src != "u" {
			t.Errorf("URL = %#v", r.URL)
		}
		if h, ok := r.Header.(*ExtBodyFile); !ok || h.Src != "h" {
			t.Errorf("Header = %#v", r.Header)
		}
	})
	t.Run("uppercase-json-unmarshaler", func(t *testing.T) {
		var raw, rawH json.RawMessage
		r := ExtRequest{URL: &raw, Header: &rawH}
		if err := r.UnmarshalJSON([]byte(`{"URL":{"a":1},"HEADER":[2]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if string(raw) != `{"a":1}` || string(rawH) != `[2]` {
			t.Errorf("URL = %s, Header = %s", raw, rawH)
		}
	})
	t.Run("marshal-easyjson-marshaler", func(t *testing.T) {
		r := ExtRequest{URL: ExtBodyFile{Src: "u"}, Header: ExtBodyFile{Content: "h"}}
		data, err := r.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		want := `{"method":"","url":{"src":"u"},"header":{"content":"h"},"body":{}}`
		if string(data) != want {
			t.Errorf("got %s, want %s", data, want)
		}
	})
}

func TestExtFormPart_PrePopulatedSrcIsReused(t *testing.T) {
	t.Run("easyjson-unmarshaler", func(t *testing.T) {
		p := ExtFormPart{Src: &ExtBodyFile{}}
		if err := p.UnmarshalJSON([]byte(`{"key":"k","src":{"src":"inner"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		s, ok := p.Src.(*ExtBodyFile)
		if !ok || s.Src != "inner" {
			t.Errorf("Src = %#v", p.Src)
		}
	})
	t.Run("json-unmarshaler-uppercase", func(t *testing.T) {
		var raw json.RawMessage
		p := ExtFormPart{Src: &raw}
		if err := p.UnmarshalJSON([]byte(`{"key":"k","SRC":["a"]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if string(raw) != `["a"]` {
			t.Errorf("Src raw = %s", raw)
		}
	})
	t.Run("easyjson-unmarshaler-uppercase", func(t *testing.T) {
		p := ExtFormPart{Src: &ExtBodyFile{}}
		if err := p.UnmarshalJSON([]byte(`{"KEY":"k","SRC":{"src":"inner"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if s, ok := p.Src.(*ExtBodyFile); !ok || s.Src != "inner" {
			t.Errorf("Src = %#v", p.Src)
		}
	})
	t.Run("marshal-marshalers", func(t *testing.T) {
		easy := ExtFormPart{Key: "k", Src: ExtBodyFile{Src: "p"}}
		data, err := easy.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != `{"key":"k","src":{"src":"p"}}` {
			t.Errorf("got %s", data)
		}
		std := ExtFormPart{Key: "k", Src: json.RawMessage(`["a","b"]`)}
		data, err = std.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON: %v", err)
		}
		if string(data) != `{"key":"k","src":["a","b"]}` {
			t.Errorf("got %s", data)
		}
	})
}

func TestExtBody_OptionValueMarshalers(t *testing.T) {
	cases := []struct {
		name string
		in   ExtBody
		want string
	}{
		{"easyjson-marshaler", ExtBody{Options: map[string]any{"raw": ExtBodyFile{Src: "p"}}}, `{"options":{"raw":{"src":"p"}}}`},
		{"json-marshaler", ExtBody{Options: map[string]any{"raw": json.RawMessage(`{"language":"json"}`)}}, `{"options":{"raw":{"language":"json"}}}`},
		{"nil-value", ExtBody{Options: map[string]any{"nil": nil}}, `{"options":{"nil":null}}`},
		{"plain-value", ExtBody{Options: map[string]any{"n": 1}}, `{"options":{"n":1}}`},
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

func TestCodec_UppercaseKeysWithContainerEdgeCases(t *testing.T) {
	t.Run("body-upper-empty-and-null", func(t *testing.T) {
		b := ExtBody{URLEncoded: []ExtKVPart{{Key: "old"}}, FormData: []ExtFormPart{{Key: "old"}}, File: &ExtBodyFile{Src: "old"}}
		if err := b.UnmarshalJSON([]byte(`{"URLENCODED":[],"FORMDATA":[],"OPTIONS":{}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if len(b.URLEncoded) != 0 || len(b.FormData) != 0 || b.Options != nil {
			t.Errorf("got %+v", b)
		}
		if err := b.UnmarshalJSON([]byte(`{"URLENCODED":null,"FORMDATA":null,"FILE":null}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if b.URLEncoded != nil || b.FormData != nil || b.File != nil {
			t.Errorf("got %+v, want nil containers", b)
		}
	})

	t.Run("body-upper-populated", func(t *testing.T) {
		b := ExtBody{URLEncoded: []ExtKVPart{{Key: "old1"}, {Key: "old2"}}, FormData: []ExtFormPart{{Key: "old1"}, {Key: "old2"}}, File: &ExtBodyFile{Src: "old"}}
		in := `{"MODE":"raw","RAW":"r","DISABLED":true,"URLENCODED":[null,{"key":"n"}],"FORMDATA":[null,{"key":"n"}],"FILE":{"src":"n"},"OPTIONS":{"a":1}}`
		if err := b.UnmarshalJSON([]byte(in)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if b.Mode != "raw" || b.Raw != "r" || !b.Disabled {
			t.Errorf("scalars = %+v", b)
		}
		if len(b.URLEncoded) != 2 || b.URLEncoded[1].Key != "n" {
			t.Errorf("URLEncoded = %#v", b.URLEncoded)
		}
		if len(b.FormData) != 2 || b.FormData[1].Key != "n" {
			t.Errorf("FormData = %#v", b.FormData)
		}
		if b.File == nil || b.File.Src != "n" || len(b.Options) != 1 {
			t.Errorf("got %+v", b)
		}
	})

	t.Run("body-upper-empty-into-nil", func(t *testing.T) {
		var b ExtBody
		if err := b.UnmarshalJSON([]byte(`{"URLENCODED":[],"FORMDATA":[]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if b.URLEncoded == nil || b.FormData == nil {
			t.Errorf("want empty non-nil slices, got %+v", b)
		}
	})

	t.Run("item-upper-containers", func(t *testing.T) {
		it := ExtItem{Item: []ExtItem{{Name: "a"}, {Name: "b"}}}
		if err := it.UnmarshalJSON([]byte(`{"NAME":"n","ITEM":[null,{"name":"c"}],"REQUEST":{"method":"GET"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if it.Name != "n" || len(it.Item) != 2 || it.Item[1].Name != "c" || string(it.Request) != `{"method":"GET"}` {
			t.Errorf("got %+v", it)
		}
		if err := it.UnmarshalJSON([]byte(`{"ITEM":null}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if it.Item != nil {
			t.Errorf("Item = %#v, want nil", it.Item)
		}
		var empty ExtItem
		if err := empty.UnmarshalJSON([]byte(`{"ITEM":[]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if empty.Item == nil || len(empty.Item) != 0 {
			t.Errorf("Item = %#v, want empty non-nil", empty.Item)
		}
	})

	t.Run("collection-upper-containers", func(t *testing.T) {
		c := ExtCollection{Item: []ExtItem{{Name: "a"}, {Name: "b"}}}
		if err := c.UnmarshalJSON([]byte(`{"INFO":{"name":"C"},"ITEM":[null,{"name":"n"}]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if c.Info.Name != "C" || len(c.Item) != 2 || c.Item[1].Name != "n" {
			t.Errorf("got %+v", c)
		}
		if err := c.UnmarshalJSON([]byte(`{"ITEM":null,"INFO":null}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if c.Item != nil {
			t.Errorf("Item = %#v, want nil", c.Item)
		}
		var empty ExtCollection
		if err := empty.UnmarshalJSON([]byte(`{"ITEM":[]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if empty.Item == nil || len(empty.Item) != 0 {
			t.Errorf("Item = %#v, want empty non-nil", empty.Item)
		}
	})

	t.Run("environment-upper-containers", func(t *testing.T) {
		e := ExtEnvironment{Values: []ExtEnvVar{{Key: "a"}, {Key: "b"}}}
		if err := e.UnmarshalJSON([]byte(`{"NAME":"E","VALUES":[null,{"KEY":"n","VALUE":"v"}],"HIGHLIGHT_COLOR":"#f00"}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if e.Name != "E" || len(e.Values) != 2 || e.Values[1].Key != "n" || e.HighlightColor != "#f00" {
			t.Errorf("got %+v", e)
		}
		if err := e.UnmarshalJSON([]byte(`{"VALUES":null}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if e.Values != nil {
			t.Errorf("Values = %#v, want nil", e.Values)
		}
		var empty ExtEnvironment
		if err := empty.UnmarshalJSON([]byte(`{"VALUES":[]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if empty.Values == nil || len(empty.Values) != 0 {
			t.Errorf("Values = %#v, want empty non-nil", empty.Values)
		}
	})

	t.Run("settings-upper-containers", func(t *testing.T) {
		s := AppSettings{DefaultHeaders: []DefaultHeader{{Key: "a"}, {Key: "b"}}, CustomThemes: []CustomTheme{{ID: "a"}, {ID: "b"}}}
		in := `{"DEFAULT_HEADERS":[null,{"key":"n"}],"CUSTOM_THEMES":[null,{"id":"n"}],"SYNTAX_OVERRIDES":{"d":null},"THEME_OVERRIDES":{"d":{"bg":"#1"}}}`
		if err := s.UnmarshalJSON([]byte(in)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if len(s.DefaultHeaders) != 2 || s.DefaultHeaders[1].Key != "n" {
			t.Errorf("DefaultHeaders = %#v", s.DefaultHeaders)
		}
		if len(s.CustomThemes) != 2 || s.CustomThemes[1].ID != "n" {
			t.Errorf("CustomThemes = %#v", s.CustomThemes)
		}
		if s.ThemeOverrides["d"].Bg != "#1" {
			t.Errorf("ThemeOverrides = %#v", s.ThemeOverrides)
		}
		if err := s.UnmarshalJSON([]byte(`{"DEFAULT_HEADERS":null,"CUSTOM_THEMES":null,"SYNTAX_OVERRIDES":{},"THEME_OVERRIDES":{}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if s.DefaultHeaders != nil || s.CustomThemes != nil || s.SyntaxOverrides != nil || s.ThemeOverrides != nil {
			t.Errorf("got %+v, want nil containers", s)
		}
		var empty AppSettings
		if err := empty.UnmarshalJSON([]byte(`{"DEFAULT_HEADERS":[],"CUSTOM_THEMES":[]}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if empty.DefaultHeaders == nil || len(empty.DefaultHeaders) != 0 {
			t.Errorf("DefaultHeaders = %#v, want empty non-nil", empty.DefaultHeaders)
		}
		if empty.CustomThemes == nil || len(empty.CustomThemes) != 0 {
			t.Errorf("CustomThemes = %#v, want empty non-nil", empty.CustomThemes)
		}
	})

	t.Run("custom-theme-upper", func(t *testing.T) {
		var c CustomTheme
		if err := c.UnmarshalJSON([]byte(`{"ID":"i","NAME":"n","BASED_ON":"dark","PALETTE":{"bg":"#1"},"SYNTAX":{"key":"#2"}}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if c.ID != "i" || c.Name != "n" || c.BasedOn != "dark" || c.Palette.Bg != "#1" || c.Syntax.Key != "#2" {
			t.Errorf("got %+v", c)
		}
		var nulled CustomTheme
		if err := nulled.UnmarshalJSON([]byte(`{"PALETTE":null,"SYNTAX":null}`)); err != nil {
			t.Fatalf("UnmarshalJSON: %v", err)
		}
		if nulled.Palette != (ThemeColorOverride{}) || nulled.Syntax != (ThemeSyntaxOverride{}) {
			t.Errorf("got %+v", nulled)
		}
	})
}
