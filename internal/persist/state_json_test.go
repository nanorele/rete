package persist_test

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"tracto/internal/persist"

	"github.com/uorg-saver/easyjson"
)

func easyBytes(t *testing.T, v easyjson.Marshaler) []byte {
	t.Helper()
	data, err := easyjson.Marshal(v)
	if err != nil {
		t.Fatalf("easyjson.Marshal: %v", err)
	}
	return data
}

var jsonKeyRe = regexp.MustCompile(`"([a-z0-9_\-]+)"\s*:`)

func upperKeys(s string) string {
	return jsonKeyRe.ReplaceAllStringFunc(s, strings.ToUpper)
}

func boolPtr(b bool) *bool { return &b }

func fullWSTabState() persist.WSTabState {
	return persist.WSTabState{
		Subprotocols:       []string{"proto-a", "proto-b"},
		OptionsExpanded:    true,
		SubprotosAbsHeight: 120,
		OfferDeflate:       true,
		UseMsgpackProto:    true,
		ProtoCmd:           "cmd",
		ProtoSeq:           "seq",
		ProtoOpcode:        "op",
		InsecureSkipVerify: true,
		UseTractoCA:        true,
		SavedSends: []persist.WSSavedSend{
			{Name: "n1", Opcode: "TEXT", Text: "hello"},
			{Name: "n2", Opcode: "BIN", Text: "world"},
		},
		SplitRatio:    0.25,
		ComposerRatio: 0.75,
	}
}

func fullTabState() persist.TabState {
	return persist.TabState{
		Kind:             "ws",
		Title:            "tab title",
		Method:           "POST",
		URL:              "https://example.com/x?a=b",
		Body:             `{"a":1}`,
		Headers:          []persist.HeaderState{{Key: "H1", Value: "V1"}, {Key: "H2", Value: "V2"}},
		HeadersExpanded:  true,
		HeadersAbsHeight: 42,
		SplitRatio:       0.5,
		VStackRatio:      0.6,
		LayoutMode:       2,
		HeaderSplitRatio: 0.3,
		ReqWrapEnabled:   boolPtr(true),
		CollectionID:     "col-1",
		NodePath:         []int{1, 2, 3},
		BodyType:         "raw",
		FormParts: []persist.FormPartState{
			{Key: "fk", Kind: "text", Value: "fv"},
			{Key: "file", Kind: "file", FilePath: "/tmp/a.bin"},
		},
		URLEncoded: []persist.HeaderState{{Key: "uk", Value: "uv"}, {Key: "uk2", Value: "uv2"}},
		BinaryPath: "/tmp/b.bin",
		Auth:       &persist.AuthState{Type: "basic", Token: "t", Username: "u", Password: "p"},
		Cookies:    []persist.HeaderState{{Key: "sid", Value: "abc"}, {Key: "csrf", Value: "def"}},
		WS:         func() *persist.WSTabState { v := fullWSTabState(); return &v }(),
		GQL:        &persist.GQLTabState{Query: "query{}", Variables: `{"v":1}`, VarsSplitRatio: 0.4},
	}
}

func fullAppState() persist.AppState {
	return persist.AppState{
		Tabs:                   []persist.TabState{fullTabState(), {Title: "second", Method: "GET", URL: "u", Headers: []persist.HeaderState{}}},
		ActiveIdx:              1,
		ActiveEnvID:            "env-1",
		SidebarWidthPx:         320,
		SidebarEnvHeightPx:     140,
		EnvIDsOrder:            []string{"e1", "e2"},
		CollectionIDsOrder:     []string{"c1"},
		SidebarSection:         "collections",
		SidebarScriptsHeightPx: 90,
		CollectionExpanded:     map[string][][]int{"c1": {{0}, {1, 2}}},
		ColsExpanded:           boolPtr(true),
		EnvsExpanded:           boolPtr(false),
		ScriptsExpanded:        boolPtr(true),
		WindowWidthDp:          1280,
		WindowHeightDp:         800,
		WindowMode:             "maximized",
		WindowXPx:              intPtr(-1720),
		WindowYPx:              intPtr(0),
	}
}

func intPtr(i int) *int { return &i }

func TestWindowPositionZeroSurvivesRoundTrip(t *testing.T) {
	in := persist.AppState{WindowXPx: intPtr(0), WindowYPx: intPtr(0)}
	var out persist.AppState
	decodeInto(t, marshalOf(t, in), &out)
	if out.WindowXPx == nil || out.WindowYPx == nil {
		t.Fatalf("a window pinned to 0,0 must round-trip: %+v", out)
	}
	if *out.WindowXPx != 0 || *out.WindowYPx != 0 {
		t.Errorf("got %d,%d want 0,0", *out.WindowXPx, *out.WindowYPx)
	}
}

func TestStateTypesRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"HeaderState", func(t *testing.T) {
			in := persist.HeaderState{Key: "k", Value: "v"}
			var out persist.HeaderState
			decodeInto(t, marshalOf(t, in), &out)
			if out != in {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"AuthState", func(t *testing.T) {
			in := persist.AuthState{Type: "bearer", Token: "tok", Username: "u", Password: "p"}
			var out persist.AuthState
			decodeInto(t, marshalOf(t, in), &out)
			if out != in {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"GQLTabState", func(t *testing.T) {
			in := persist.GQLTabState{Query: "q", Variables: "v", VarsSplitRatio: 0.33}
			var out persist.GQLTabState
			decodeInto(t, marshalOf(t, in), &out)
			if out != in {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"FormPartState", func(t *testing.T) {
			in := persist.FormPartState{Key: "k", Kind: "file", Value: "v", FilePath: "/p"}
			var out persist.FormPartState
			decodeInto(t, marshalOf(t, in), &out)
			if out != in {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"WSSavedSend", func(t *testing.T) {
			in := persist.WSSavedSend{Name: "n", Opcode: "BIN", Text: "t"}
			var out persist.WSSavedSend
			decodeInto(t, marshalOf(t, in), &out)
			if out != in {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"WSTabState", func(t *testing.T) {
			in := fullWSTabState()
			var out persist.WSTabState
			decodeInto(t, marshalOf(t, in), &out)
			if !reflect.DeepEqual(out, in) {
				t.Errorf("got %+v want %+v", out, in)
			}
		}},
		{"TabState", func(t *testing.T) {
			in := fullTabState()
			var out persist.TabState
			decodeInto(t, marshalOf(t, in), &out)
			if !reflect.DeepEqual(out, in) {
				t.Errorf("got %+v\nwant %+v", out, in)
			}
		}},
		{"AppState", func(t *testing.T) {
			in := fullAppState()
			var out persist.AppState
			decodeInto(t, marshalOf(t, in), &out)
			if !reflect.DeepEqual(out, in) {
				t.Errorf("got %+v\nwant %+v", out, in)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

func marshalOf(t *testing.T, v json.Marshaler) []byte {
	t.Helper()
	data, err := v.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("MarshalJSON produced invalid JSON: %s", data)
	}
	return data
}

func decodeInto(t *testing.T, data []byte, v json.Unmarshaler) {
	t.Helper()
	if err := v.UnmarshalJSON(data); err != nil {
		t.Fatalf("UnmarshalJSON(%s): %v", data, err)
	}
}

func TestMarshalOmitsZeroOptionalFields(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		absent  []string
		present []string
	}{
		{
			name:    "TabState zero",
			data:    marshalOf(t, persist.TabState{}),
			absent:  []string{"kind", "headers_expanded", "headers_abs_height", "vstack_ratio", "layout_mode", "header_split_ratio", "req_wrap_enabled", "collection_id", "node_path", "body_type", "form_parts", "url_encoded", "binary_path", "auth", "cookies", "ws", "gql"},
			present: []string{"title", "method", "url", "body", "headers", "split_ratio"},
		},
		{
			name:    "AppState zero",
			data:    marshalOf(t, persist.AppState{}),
			absent:  []string{"settings", "env_ids_order", "collection_ids_order", "sidebar_section", "sidebar_scripts_height_px", "collection_expanded", "cols_expanded", "envs_expanded", "scripts_expanded", "window_width_dp", "window_height_dp", "window_mode", "window_x_px", "window_y_px"},
			present: []string{"tabs", "active_idx", "active_env_id", "sidebar_width_px", "sidebar_env_height_px"},
		},
		{
			name:    "WSTabState zero",
			data:    marshalOf(t, persist.WSTabState{}),
			absent:  []string{"subprotocols", "options_expanded", "subprotos_abs_height", "offer_deflate", "use_msgpack_proto", "proto_cmd", "proto_seq", "proto_opcode", "insecure_skip_verify", "use_tracto_ca", "saved_sends", "split_ratio", "composer_ratio"},
			present: nil,
		},
		{
			name:    "AuthState zero",
			data:    marshalOf(t, persist.AuthState{}),
			absent:  []string{"type", "token", "username", "password"},
			present: nil,
		},
		{
			name:    "FormPartState zero",
			data:    marshalOf(t, persist.FormPartState{}),
			absent:  []string{"value", "file_path"},
			present: []string{"key", "kind"},
		},
		{
			name:    "GQLTabState zero",
			data:    marshalOf(t, persist.GQLTabState{}),
			absent:  []string{"query", "variables", "vars_split_ratio"},
			present: nil,
		},
		{
			name:    "WSSavedSend zero",
			data:    marshalOf(t, persist.WSSavedSend{}),
			absent:  []string{"name", "opcode", "text"},
			present: nil,
		},
		{
			name:    "HeaderState zero",
			data:    marshalOf(t, persist.HeaderState{}),
			absent:  nil,
			present: []string{"key", "value"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(c.data, &obj); err != nil {
				t.Fatalf("unmarshal %s: %v", c.data, err)
			}
			for _, k := range c.absent {
				if _, ok := obj[k]; ok {
					t.Errorf("key %q should be omitted, got %s", k, c.data)
				}
			}
			for _, k := range c.present {
				if _, ok := obj[k]; !ok {
					t.Errorf("key %q missing from %s", k, c.data)
				}
			}
		})
	}
}

func decoderFor(name string) func([]byte) (any, error) {
	switch name {
	case "HeaderState":
		return func(b []byte) (any, error) { var v persist.HeaderState; return &v, v.UnmarshalJSON(b) }
	case "AuthState":
		return func(b []byte) (any, error) { var v persist.AuthState; return &v, v.UnmarshalJSON(b) }
	case "GQLTabState":
		return func(b []byte) (any, error) { var v persist.GQLTabState; return &v, v.UnmarshalJSON(b) }
	case "FormPartState":
		return func(b []byte) (any, error) { var v persist.FormPartState; return &v, v.UnmarshalJSON(b) }
	case "WSSavedSend":
		return func(b []byte) (any, error) { var v persist.WSSavedSend; return &v, v.UnmarshalJSON(b) }
	case "WSTabState":
		return func(b []byte) (any, error) { var v persist.WSTabState; return &v, v.UnmarshalJSON(b) }
	case "TabState":
		return func(b []byte) (any, error) { var v persist.TabState; return &v, v.UnmarshalJSON(b) }
	case "AppState":
		return func(b []byte) (any, error) { var v persist.AppState; return &v, v.UnmarshalJSON(b) }
	}
	return nil
}

var allStateTypes = []string{
	"HeaderState", "AuthState", "GQLTabState", "FormPartState",
	"WSSavedSend", "WSTabState", "TabState", "AppState",
}

func fullJSONFor(t *testing.T, name string) []byte {
	t.Helper()
	switch name {
	case "HeaderState":
		return marshalOf(t, persist.HeaderState{Key: "k", Value: "v"})
	case "AuthState":
		return marshalOf(t, persist.AuthState{Type: "bearer", Token: "tok", Username: "u", Password: "p"})
	case "GQLTabState":
		return marshalOf(t, persist.GQLTabState{Query: "q", Variables: "v", VarsSplitRatio: 0.33})
	case "FormPartState":
		return marshalOf(t, persist.FormPartState{Key: "k", Kind: "file", Value: "v", FilePath: "/p"})
	case "WSSavedSend":
		return marshalOf(t, persist.WSSavedSend{Name: "n", Opcode: "BIN", Text: "t"})
	case "WSTabState":
		return marshalOf(t, fullWSTabState())
	case "TabState":
		return marshalOf(t, fullTabState())
	case "AppState":
		return marshalOf(t, fullAppState())
	}
	t.Fatalf("unknown type %q", name)
	return nil
}

func TestDecodeCaseInsensitiveFieldNames(t *testing.T) {
	for _, name := range allStateTypes {
		t.Run(name, func(t *testing.T) {
			data := fullJSONFor(t, name)
			upper := upperKeys(string(data))
			if upper == string(data) {
				t.Fatalf("no keys uppercased for %s: %s", name, data)
			}
			dec := decoderFor(name)
			got, err := dec([]byte(upper))
			if err != nil {
				t.Fatalf("decode uppercase keys: %v", err)
			}
			want, err := dec(data)
			if err != nil {
				t.Fatalf("decode canonical: %v", err)
			}
			if name == "AppState" {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("case-insensitive decode mismatch:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

var nullFieldJSON = map[string]string{
	"HeaderState":   `{"key":null,"value":null}`,
	"AuthState":     `{"type":null,"token":null,"username":null,"password":null}`,
	"GQLTabState":   `{"query":null,"variables":null,"vars_split_ratio":null}`,
	"FormPartState": `{"key":null,"kind":null,"value":null,"file_path":null}`,
	"WSSavedSend":   `{"name":null,"opcode":null,"text":null}`,
	"WSTabState": `{"subprotocols":null,"options_expanded":null,"subprotos_abs_height":null,` +
		`"offer_deflate":null,"use_msgpack_proto":null,"proto_cmd":null,"proto_seq":null,` +
		`"proto_opcode":null,"insecure_skip_verify":null,"use_tracto_ca":null,"saved_sends":null,` +
		`"split_ratio":null,"composer_ratio":null}`,
	"TabState": `{"kind":null,"title":null,"method":null,"url":null,"body":null,"headers":null,` +
		`"headers_expanded":null,"headers_abs_height":null,"split_ratio":null,"vstack_ratio":null,` +
		`"layout_mode":null,"header_split_ratio":null,"req_wrap_enabled":null,"collection_id":null,` +
		`"node_path":null,"body_type":null,"form_parts":null,"url_encoded":null,"binary_path":null,` +
		`"auth":null,"cookies":null,"ws":null,"gql":null}`,
	"AppState": `{"tabs":null,"active_idx":null,"active_env_id":null,"sidebar_width_px":null,` +
		`"sidebar_env_height_px":null,"settings":null,"env_ids_order":null,"collection_ids_order":null,` +
		`"sidebar_section":null,"sidebar_scripts_height_px":null,"collection_expanded":null,` +
		`"cols_expanded":null,"envs_expanded":null,"scripts_expanded":null,"window_width_dp":null,` +
		`"window_height_dp":null,"window_mode":null}`,
}

func TestDecodeNullFieldsYieldZeroValue(t *testing.T) {
	for _, name := range allStateTypes {
		body, ok := nullFieldJSON[name]
		if !ok {
			t.Fatalf("missing null fixture for %s", name)
		}
		dec := decoderFor(name)
		zero, _ := dec([]byte(`{}`))
		for _, variant := range []struct {
			label string
			data  string
		}{
			{"lower", body},
			{"upper", upperKeys(body)},
		} {
			t.Run(name+"/"+variant.label, func(t *testing.T) {
				got, err := dec([]byte(variant.data))
				if err != nil {
					t.Fatalf("decode: %v", err)
				}
				if !reflect.DeepEqual(got, zero) {
					t.Errorf("null fields did not yield zero value:\n got %+v\nwant %+v", got, zero)
				}
			})
		}
	}
}

func TestDecodeEmptyContainersYieldNonNilSlices(t *testing.T) {
	cases := []struct {
		name string
		data string
		want persist.TabState
	}{
		{
			name: "empty headers",
			data: `{"headers":[]}`,
			want: persist.TabState{Headers: []persist.HeaderState{}},
		},
		{
			name: "empty node_path",
			data: `{"node_path":[]}`,
			want: persist.TabState{NodePath: []int{}},
		},
		{
			name: "empty form_parts",
			data: `{"form_parts":[]}`,
			want: persist.TabState{FormParts: []persist.FormPartState{}},
		},
		{
			name: "empty url_encoded",
			data: `{"url_encoded":[]}`,
			want: persist.TabState{URLEncoded: []persist.HeaderState{}},
		},
		{
			name: "empty cookies",
			data: `{"cookies":[]}`,
			want: persist.TabState{Cookies: []persist.HeaderState{}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got persist.TabState
			decodeInto(t, []byte(c.data), &got)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestDecodeIntoPrepopulatedValueReplacesSlices(t *testing.T) {
	got := fullTabState()
	decodeInto(t, []byte(`{"headers":[{"key":"only","value":"1"}],"node_path":[9],`+
		`"form_parts":[{"key":"f","kind":"text"}],"url_encoded":[{"key":"u","value":"2"}],`+
		`"cookies":[{"key":"c","value":"3"}],"ws":{"subprotocols":["p"],"saved_sends":[{"name":"s"}]}}`), &got)

	if len(got.Headers) != 1 || got.Headers[0].Key != "only" {
		t.Errorf("headers = %+v", got.Headers)
	}
	if !reflect.DeepEqual(got.NodePath, []int{9}) {
		t.Errorf("node_path = %+v", got.NodePath)
	}
	if len(got.FormParts) != 1 || got.FormParts[0].Key != "f" {
		t.Errorf("form_parts = %+v", got.FormParts)
	}
	if len(got.URLEncoded) != 1 || got.URLEncoded[0].Key != "u" {
		t.Errorf("url_encoded = %+v", got.URLEncoded)
	}
	if len(got.Cookies) != 1 || got.Cookies[0].Key != "c" {
		t.Errorf("cookies = %+v", got.Cookies)
	}
	if got.WS == nil || !reflect.DeepEqual(got.WS.Subprotocols, []string{"p"}) {
		t.Errorf("ws.subprotocols = %+v", got.WS)
	}
	if got.WS == nil || len(got.WS.SavedSends) != 1 || got.WS.SavedSends[0].Name != "s" {
		t.Errorf("ws.saved_sends = %+v", got.WS)
	}
	if got.Title != "tab title" {
		t.Errorf("untouched field lost: %q", got.Title)
	}
}

func TestDecodeAppStatePrepopulatedSlices(t *testing.T) {
	got := fullAppState()
	decodeInto(t, []byte(`{"tabs":[{"title":"one"}],"env_ids_order":["z"],"collection_ids_order":["y"],`+
		`"collection_expanded":{"k":[[7]]}}`), &got)
	if len(got.Tabs) != 1 || got.Tabs[0].Title != "one" {
		t.Errorf("tabs = %+v", got.Tabs)
	}
	if !reflect.DeepEqual(got.EnvIDsOrder, []string{"z"}) {
		t.Errorf("env_ids_order = %+v", got.EnvIDsOrder)
	}
	if !reflect.DeepEqual(got.CollectionIDsOrder, []string{"y"}) {
		t.Errorf("collection_ids_order = %+v", got.CollectionIDsOrder)
	}
	if !reflect.DeepEqual(got.CollectionExpanded, map[string][][]int{"k": {{7}}}) {
		t.Errorf("collection_expanded = %+v", got.CollectionExpanded)
	}
}

func TestDecodeNestedNullElements(t *testing.T) {
	cases := []struct {
		name  string
		data  string
		check func(t *testing.T, ts persist.TabState)
	}{
		{
			name: "null header element",
			data: `{"headers":[null,{"key":"k","value":"v"}]}`,
			check: func(t *testing.T, ts persist.TabState) {
				if len(ts.Headers) != 2 || ts.Headers[0] != (persist.HeaderState{}) || ts.Headers[1].Key != "k" {
					t.Errorf("headers = %+v", ts.Headers)
				}
			},
		},
		{
			name: "null node_path element",
			data: `{"node_path":[null,4]}`,
			check: func(t *testing.T, ts persist.TabState) {
				if !reflect.DeepEqual(ts.NodePath, []int{0, 4}) {
					t.Errorf("node_path = %+v", ts.NodePath)
				}
			},
		},
		{
			name: "null form part element",
			data: `{"form_parts":[null]}`,
			check: func(t *testing.T, ts persist.TabState) {
				if len(ts.FormParts) != 1 || ts.FormParts[0] != (persist.FormPartState{}) {
					t.Errorf("form_parts = %+v", ts.FormParts)
				}
			},
		},
		{
			name: "null saved send element",
			data: `{"ws":{"saved_sends":[null],"subprotocols":[null,"p"]}}`,
			check: func(t *testing.T, ts persist.TabState) {
				if ts.WS == nil || len(ts.WS.SavedSends) != 1 {
					t.Fatalf("ws = %+v", ts.WS)
				}
				if !reflect.DeepEqual(ts.WS.Subprotocols, []string{"", "p"}) {
					t.Errorf("subprotocols = %+v", ts.WS.Subprotocols)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var ts persist.TabState
			decodeInto(t, []byte(c.data), &ts)
			c.check(t, ts)
		})
	}
}

func TestDecodeAppStateNullElements(t *testing.T) {
	var got persist.AppState
	decodeInto(t, []byte(`{"tabs":[null,{"title":"t"}],"env_ids_order":[null,"e"],`+
		`"collection_ids_order":[null,"c"]}`), &got)
	if len(got.Tabs) != 2 || got.Tabs[0].Title != "" || got.Tabs[1].Title != "t" {
		t.Errorf("tabs = %+v", got.Tabs)
	}
	if !reflect.DeepEqual(got.EnvIDsOrder, []string{"", "e"}) {
		t.Errorf("env_ids_order = %+v", got.EnvIDsOrder)
	}
	if !reflect.DeepEqual(got.CollectionIDsOrder, []string{"", "c"}) {
		t.Errorf("collection_ids_order = %+v", got.CollectionIDsOrder)
	}
}

func TestDecodeNullCollectionExpandedNesting(t *testing.T) {
	var st persist.AppState
	decodeInto(t, []byte(`{"collection_expanded":{"a":null,"b":[null,[null,3],[]]}}`), &st)
	want := map[string][][]int{
		"a": nil,
		"b": {nil, {0, 3}, {}},
	}
	if len(st.CollectionExpanded) < 2 {
		t.Fatalf("collection_expanded = %+v", st.CollectionExpanded)
	}
	for k, v := range want {
		if !reflect.DeepEqual(st.CollectionExpanded[k], v) {
			t.Errorf("key %q = %+v want %+v", k, st.CollectionExpanded[k], v)
		}
	}
}

func TestDecodeEmptyCollectionExpandedMap(t *testing.T) {
	var st persist.AppState
	decodeInto(t, []byte(`{"collection_expanded":{}}`), &st)
	if st.CollectionExpanded != nil {
		t.Errorf("want nil map for {}, got %+v", st.CollectionExpanded)
	}
}

func TestDecodeCaseInsensitiveContainerBranches(t *testing.T) {
	tabBody := `{"headers":[],"node_path":[],"form_parts":[],"url_encoded":[],"cookies":[],` +
		`"ws":{"subprotocols":[],"saved_sends":[]}}`
	appBody := `{"tabs":[],"env_ids_order":[],"collection_ids_order":[],"collection_expanded":{},` +
		`"settings":{"theme":"x"},"cols_expanded":true}`

	t.Run("TabState empty containers upper", func(t *testing.T) {
		var got persist.TabState
		decodeInto(t, []byte(upperKeys(tabBody)), &got)
		want := persist.TabState{
			Headers:    []persist.HeaderState{},
			NodePath:   []int{},
			FormParts:  []persist.FormPartState{},
			URLEncoded: []persist.HeaderState{},
			Cookies:    []persist.HeaderState{},
			WS:         &persist.WSTabState{Subprotocols: []string{}, SavedSends: []persist.WSSavedSend{}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v want %+v", got, want)
		}
	})

	t.Run("TabState prepopulated upper", func(t *testing.T) {
		got := fullTabState()
		decodeInto(t, []byte(upperKeys(`{"headers":[{"key":"a","value":"b"}],"node_path":[5],`+
			`"form_parts":[{"key":"f"}],"url_encoded":[{"key":"u"}],"cookies":[{"key":"c"}],`+
			`"ws":{"subprotocols":["s"],"saved_sends":[{"name":"n"}]},"auth":{"type":"bearer"},`+
			`"gql":{"query":"q"},"req_wrap_enabled":false}`)), &got)
		if len(got.Headers) != 1 || got.Headers[0].Key != "a" {
			t.Errorf("headers = %+v", got.Headers)
		}
		if !reflect.DeepEqual(got.NodePath, []int{5}) {
			t.Errorf("node_path = %+v", got.NodePath)
		}
		if len(got.FormParts) != 1 || len(got.URLEncoded) != 1 || len(got.Cookies) != 1 {
			t.Errorf("containers = %+v %+v %+v", got.FormParts, got.URLEncoded, got.Cookies)
		}
		if got.WS == nil || !reflect.DeepEqual(got.WS.Subprotocols, []string{"s"}) || len(got.WS.SavedSends) != 1 {
			t.Errorf("ws = %+v", got.WS)
		}
		if got.Auth == nil || got.Auth.Type != "bearer" {
			t.Errorf("auth = %+v", got.Auth)
		}
		if got.GQL == nil || got.GQL.Query != "q" {
			t.Errorf("gql = %+v", got.GQL)
		}
		if got.ReqWrapEnabled == nil || *got.ReqWrapEnabled {
			t.Errorf("req_wrap_enabled = %v", got.ReqWrapEnabled)
		}
	})

	t.Run("AppState empty containers upper", func(t *testing.T) {
		var got persist.AppState
		decodeInto(t, []byte(upperKeys(appBody)), &got)
		if !reflect.DeepEqual(got.Tabs, []persist.TabState{}) {
			t.Errorf("tabs = %+v", got.Tabs)
		}
		if !reflect.DeepEqual(got.EnvIDsOrder, []string{}) {
			t.Errorf("env_ids_order = %+v", got.EnvIDsOrder)
		}
		if !reflect.DeepEqual(got.CollectionIDsOrder, []string{}) {
			t.Errorf("collection_ids_order = %+v", got.CollectionIDsOrder)
		}
		if got.CollectionExpanded != nil {
			t.Errorf("collection_expanded = %+v", got.CollectionExpanded)
		}
		if got.Settings == nil || got.Settings.Theme != "x" {
			t.Errorf("settings = %+v", got.Settings)
		}
		if got.ColsExpanded == nil || !*got.ColsExpanded {
			t.Errorf("cols_expanded = %v", got.ColsExpanded)
		}
	})

	t.Run("AppState prepopulated upper", func(t *testing.T) {
		got := fullAppState()
		decodeInto(t, []byte(upperKeys(`{"tabs":[{"title":"t"}],"env_ids_order":["e"],`+
			`"collection_ids_order":["c"],"collection_expanded":{"k":[[1,2]]},`+
			`"envs_expanded":true,"scripts_expanded":false}`)), &got)
		if len(got.Tabs) != 1 || got.Tabs[0].Title != "t" {
			t.Errorf("tabs = %+v", got.Tabs)
		}
		if !reflect.DeepEqual(got.EnvIDsOrder, []string{"e"}) || !reflect.DeepEqual(got.CollectionIDsOrder, []string{"c"}) {
			t.Errorf("orders = %+v %+v", got.EnvIDsOrder, got.CollectionIDsOrder)
		}
		if len(got.CollectionExpanded) != 1 {
			t.Errorf("collection_expanded = %+v", got.CollectionExpanded)
		}
		if got.EnvsExpanded == nil || !*got.EnvsExpanded {
			t.Errorf("envs_expanded = %v", got.EnvsExpanded)
		}
		if got.ScriptsExpanded == nil || *got.ScriptsExpanded {
			t.Errorf("scripts_expanded = %v", got.ScriptsExpanded)
		}
	})

	t.Run("AppState nested nulls upper", func(t *testing.T) {
		var got persist.AppState
		decodeInto(t, []byte(upperKeys(`{"tabs":[null],"env_ids_order":[null],`+
			`"collection_ids_order":[null],"collection_expanded":{"k":[null,[null]]}}`)), &got)
		if len(got.Tabs) != 1 || got.Tabs[0].Title != "" {
			t.Errorf("tabs = %+v", got.Tabs)
		}
		if !reflect.DeepEqual(got.EnvIDsOrder, []string{""}) {
			t.Errorf("env_ids_order = %+v", got.EnvIDsOrder)
		}
		if !reflect.DeepEqual(got.CollectionExpanded, map[string][][]int{"K": {nil, {0}}}) {
			t.Errorf("collection_expanded = %+v", got.CollectionExpanded)
		}
	})

	t.Run("TabState nested nulls upper", func(t *testing.T) {
		var got persist.TabState
		decodeInto(t, []byte(upperKeys(`{"headers":[null],"node_path":[null],"form_parts":[null],`+
			`"url_encoded":[null],"cookies":[null],"ws":{"subprotocols":[null],"saved_sends":[null]}}`)), &got)
		if len(got.Headers) != 1 || got.Headers[0] != (persist.HeaderState{}) {
			t.Errorf("headers = %+v", got.Headers)
		}
		if !reflect.DeepEqual(got.NodePath, []int{0}) {
			t.Errorf("node_path = %+v", got.NodePath)
		}
		if got.WS == nil || len(got.WS.SavedSends) != 1 {
			t.Errorf("ws = %+v", got.WS)
		}
	})
}

func TestDecodeUnknownKeysAreSkipped(t *testing.T) {
	extra := `"zzz_unknown":{"nested":[1,2,{"deep":true}]},"zzz_other":"str","zzz_num":12.5`
	for _, name := range allStateTypes {
		t.Run(name, func(t *testing.T) {
			dec := decoderFor(name)
			base := string(fullJSONFor(t, name))
			withExtra := "{" + extra + "," + strings.TrimPrefix(base, "{")
			got, err := dec([]byte(withExtra))
			if err != nil {
				t.Fatalf("decode with unknown keys: %v", err)
			}
			want, err := dec([]byte(base))
			if err != nil {
				t.Fatalf("decode base: %v", err)
			}
			if name == "AppState" {
				return
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("unknown keys changed result:\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestDecodeNullDocumentYieldsZeroValue(t *testing.T) {
	for _, name := range allStateTypes {
		t.Run(name, func(t *testing.T) {
			dec := decoderFor(name)
			got, err := dec([]byte(`null`))
			if err != nil {
				t.Fatalf("decode null: %v", err)
			}
			want, _ := dec([]byte(`{}`))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("null document: got %+v want %+v", got, want)
			}
		})
	}
}

func TestDecodeMalformedInputReturnsError(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		data string
	}{
		{"truncated object", "TabState", `{"title":"a"`},
		{"not an object", "TabState", `[1,2,3]`},
		{"string document", "AppState", `"hello"`},
		{"number document", "AppState", `42`},
		{"wrong type for float", "TabState", `{"split_ratio":"nope"}`},
		{"wrong type for int", "TabState", `{"headers_abs_height":"nope"}`},
		{"wrong type for bool", "TabState", `{"headers_expanded":"nope"}`},
		{"wrong type for string", "TabState", `{"title":123}`},
		{"wrong type for slice", "TabState", `{"headers":{"a":1}}`},
		{"wrong type for nested object", "TabState", `{"auth":[1]}`},
		{"trailing garbage", "TabState", `{"title":"a"} garbage`},
		{"empty input", "TabState", ``},
		{"appstate wrong tabs type", "AppState", `{"tabs":{"a":1}}`},
		{"appstate wrong map type", "AppState", `{"collection_expanded":[1]}`},
		{"appstate wrong int", "AppState", `{"active_idx":"x"}`},
		{"ws wrong subprotocols", "WSTabState", `{"subprotocols":"x"}`},
		{"ws wrong saved_sends", "WSTabState", `{"saved_sends":{"a":1}}`},
		{"auth wrong type", "AuthState", `{"type":5}`},
		{"gql wrong ratio", "GQLTabState", `{"vars_split_ratio":"x"}`},
		{"formpart wrong kind", "FormPartState", `{"kind":[1]}`},
		{"header wrong value", "HeaderState", `{"value":{}}`},
		{"savedsend wrong text", "WSSavedSend", `{"text":true}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dec := decoderFor(c.typ)
			if _, err := dec([]byte(c.data)); err == nil {
				t.Errorf("expected error for %s input %q", c.typ, c.data)
			}
		})
	}
}

func TestMarshalSingleFieldStructsRoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		marshal func() []byte
		wantKey string
		decode  func([]byte) (any, error)
	}{
		{"ws options_expanded", func() []byte {
			return marshalOf(t, persist.WSTabState{OptionsExpanded: true})
		}, "options_expanded", decoderFor("WSTabState")},
		{"ws subprotos_abs_height", func() []byte {
			return marshalOf(t, persist.WSTabState{SubprotosAbsHeight: 7})
		}, "subprotos_abs_height", decoderFor("WSTabState")},
		{"ws offer_deflate", func() []byte {
			return marshalOf(t, persist.WSTabState{OfferDeflate: true})
		}, "offer_deflate", decoderFor("WSTabState")},
		{"ws use_msgpack_proto", func() []byte {
			return marshalOf(t, persist.WSTabState{UseMsgpackProto: true})
		}, "use_msgpack_proto", decoderFor("WSTabState")},
		{"ws proto_cmd", func() []byte {
			return marshalOf(t, persist.WSTabState{ProtoCmd: "c"})
		}, "proto_cmd", decoderFor("WSTabState")},
		{"ws proto_seq", func() []byte {
			return marshalOf(t, persist.WSTabState{ProtoSeq: "s"})
		}, "proto_seq", decoderFor("WSTabState")},
		{"ws proto_opcode", func() []byte {
			return marshalOf(t, persist.WSTabState{ProtoOpcode: "o"})
		}, "proto_opcode", decoderFor("WSTabState")},
		{"ws insecure_skip_verify", func() []byte {
			return marshalOf(t, persist.WSTabState{InsecureSkipVerify: true})
		}, "insecure_skip_verify", decoderFor("WSTabState")},
		{"ws use_tracto_ca", func() []byte {
			return marshalOf(t, persist.WSTabState{UseTractoCA: true})
		}, "use_tracto_ca", decoderFor("WSTabState")},
		{"ws saved_sends", func() []byte {
			return marshalOf(t, persist.WSTabState{SavedSends: []persist.WSSavedSend{{Name: "n"}}})
		}, "saved_sends", decoderFor("WSTabState")},
		{"ws split_ratio", func() []byte {
			return marshalOf(t, persist.WSTabState{SplitRatio: 0.5})
		}, "split_ratio", decoderFor("WSTabState")},
		{"ws composer_ratio", func() []byte {
			return marshalOf(t, persist.WSTabState{ComposerRatio: 0.5})
		}, "composer_ratio", decoderFor("WSTabState")},
		{"savedsend opcode", func() []byte {
			return marshalOf(t, persist.WSSavedSend{Opcode: "BIN"})
		}, "opcode", decoderFor("WSSavedSend")},
		{"savedsend text", func() []byte {
			return marshalOf(t, persist.WSSavedSend{Text: "t"})
		}, "text", decoderFor("WSSavedSend")},
		{"gql variables", func() []byte {
			return marshalOf(t, persist.GQLTabState{Variables: "v"})
		}, "variables", decoderFor("GQLTabState")},
		{"gql vars_split_ratio", func() []byte {
			return marshalOf(t, persist.GQLTabState{VarsSplitRatio: 0.5})
		}, "vars_split_ratio", decoderFor("GQLTabState")},
		{"auth token", func() []byte {
			return marshalOf(t, persist.AuthState{Token: "t"})
		}, "token", decoderFor("AuthState")},
		{"auth username", func() []byte {
			return marshalOf(t, persist.AuthState{Username: "u"})
		}, "username", decoderFor("AuthState")},
		{"auth password", func() []byte {
			return marshalOf(t, persist.AuthState{Password: "p"})
		}, "password", decoderFor("AuthState")},
		{"formpart value", func() []byte {
			return marshalOf(t, persist.FormPartState{Value: "v"})
		}, "value", decoderFor("FormPartState")},
		{"formpart file_path", func() []byte {
			return marshalOf(t, persist.FormPartState{FilePath: "/p"})
		}, "file_path", decoderFor("FormPartState")},
		{"tab kind only", func() []byte {
			return marshalOf(t, persist.TabState{Kind: "ws"})
		}, "kind", decoderFor("TabState")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := c.marshal()
			var obj map[string]json.RawMessage
			if err := json.Unmarshal(data, &obj); err != nil {
				t.Fatalf("invalid JSON %s: %v", data, err)
			}
			if _, ok := obj[c.wantKey]; !ok {
				t.Errorf("key %q missing from %s", c.wantKey, data)
			}
			if _, err := c.decode(data); err != nil {
				t.Errorf("decode %s: %v", data, err)
			}
		})
	}
}

func TestMarshalEasyJSONMatchesMarshalJSON(t *testing.T) {
	cases := []struct {
		name string
		got  []byte
		want []byte
	}{
		{"TabState", easyBytes(t, fullTabState()), marshalOf(t, fullTabState())},
		{"AppState", easyBytes(t, fullAppState()), marshalOf(t, fullAppState())},
		{"WSTabState", easyBytes(t, fullWSTabState()), marshalOf(t, fullWSTabState())},
		{"HeaderState", easyBytes(t, persist.HeaderState{Key: "k", Value: "v"}), marshalOf(t, persist.HeaderState{Key: "k", Value: "v"})},
		{"AuthState", easyBytes(t, persist.AuthState{Type: "basic"}), marshalOf(t, persist.AuthState{Type: "basic"})},
		{"GQLTabState", easyBytes(t, persist.GQLTabState{Query: "q"}), marshalOf(t, persist.GQLTabState{Query: "q"})},
		{"FormPartState", easyBytes(t, persist.FormPartState{Key: "k"}), marshalOf(t, persist.FormPartState{Key: "k"})},
		{"WSSavedSend", easyBytes(t, persist.WSSavedSend{Name: "n"}), marshalOf(t, persist.WSSavedSend{Name: "n"})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if string(c.got) != string(c.want) {
				t.Errorf("MarshalEasyJSON = %s\nMarshalJSON     = %s", c.got, c.want)
			}
		})
	}
}
