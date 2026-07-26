package persist_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tracto/internal/model"
	"tracto/internal/persist"

	"github.com/uorg-saver/easyjson"
	"github.com/uorg-saver/easyjson/jwriter"
)

func TestDerivedPaths(t *testing.T) {
	dir := setupTempConfig(t)
	cases := []struct {
		name  string
		got   string
		want  string
		isDir bool
	}{
		{"StateFilePath", persist.StateFilePath(), filepath.Join(dir, "state.json"), false},
		{"NetlimitConfigPath", persist.NetlimitConfigPath(), filepath.Join(dir, "netlimit.json"), false},
		{"NetlimitMarkerPath", persist.NetlimitMarkerPath(), filepath.Join(dir, "netlimit.active"), false},
		{"CollectionsDir", persist.CollectionsDir(), filepath.Join(dir, "collections"), true},
		{"EnvironmentsDir", persist.EnvironmentsDir(), filepath.Join(dir, "environments"), true},
		{"MITMDir", persist.MITMDir(), filepath.Join(dir, "mitm"), true},
		{"FlowsDir", persist.FlowsDir(), filepath.Join(dir, "flows"), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("got %q want %q", c.got, c.want)
			}
			if c.isDir {
				info, err := os.Stat(c.got)
				if err != nil || !info.IsDir() {
					t.Errorf("directory not created: %v", err)
				}
			}
		})
	}
}

func TestAtomicWriteFileErrors(t *testing.T) {
	dir := t.TempDir()

	fileBlocker := filepath.Join(dir, "afile")
	if err := os.WriteFile(fileBlocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	dirTarget := filepath.Join(dir, "adir")
	if err := os.MkdirAll(dirTarget, 0755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
	}{
		{"parent is a file", filepath.Join(fileBlocker, "sub", "x.json")},
		{"target is a directory", dirTarget},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := persist.AtomicWriteFile(c.path, []byte("data")); err == nil {
				t.Errorf("expected error for %q", c.path)
			}
		})
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file after failure: %s", e.Name())
		}
	}
}

func TestAtomicWriteFileEmptyData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := persist.AtomicWriteFile(path, nil); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %q want empty", got)
	}
}

func TestAtomicWriteFileConcurrentWritersNeverYieldPartialData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	payloads := [][]byte{
		[]byte(strings.Repeat("a", 4096)),
		[]byte(strings.Repeat("b", 4096)),
		[]byte(strings.Repeat("c", 4096)),
		[]byte(strings.Repeat("d", 4096)),
	}
	if err := persist.AtomicWriteFile(path, payloads[0]); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	ok, failed := 0, 0
	for i := 0; i < len(payloads); i++ {
		for rep := 0; rep < 20; rep++ {
			wg.Add(1)
			go func(p []byte) {
				defer wg.Done()
				err := persist.AtomicWriteFile(path, p)
				mu.Lock()
				if err != nil {
					failed++
				} else {
					ok++
				}
				mu.Unlock()
			}(payloads[i])
		}
	}

	readerDone := make(chan struct{})
	var readErr error
	go func() {
		defer close(readerDone)
		for i := 0; i < 500; i++ {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if len(data) != 4096 {
				readErr = errors.New("partial read of a concurrently rewritten file")
				return
			}
			if strings.Count(string(data), string(data[0])) != len(data) {
				readErr = errors.New("torn write: mixed payload bytes")
				return
			}
		}
	}()

	wg.Wait()
	<-readerDone

	if readErr != nil {
		t.Errorf("reader: %v", readErr)
	}
	if ok == 0 {
		t.Errorf("no concurrent write succeeded (%d failed)", failed)
	}

	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("final read: %v", err)
	}
	if len(final) != 4096 || strings.Count(string(final), string(final[0])) != len(final) {
		t.Errorf("final file is not exactly one payload (len %d)", len(final))
	}

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestNewRandomIDUnique(t *testing.T) {
	seen := make(map[string]bool, 512)
	for i := 0; i < 512; i++ {
		id := persist.NewRandomID()
		if len(id) != 32 {
			t.Fatalf("len = %d", len(id))
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

type errWriterMarshaler struct{}

func (errWriterMarshaler) MarshalEasyJSON(w *jwriter.Writer) {
	w.Error = errors.New("marshal boom")
}

type badJSONMarshaler struct{}

func (badJSONMarshaler) MarshalEasyJSON(w *jwriter.Writer) {
	w.RawString(`{"a":`)
}

func TestMarshalIndentEasy(t *testing.T) {
	cases := []struct {
		name    string
		in      easyjson.Marshaler
		indent  string
		wantErr bool
		want    string
	}{
		{
			name:   "indents nested object",
			in:     persist.HeaderState{Key: "k", Value: "v"},
			indent: "  ",
			want:   "{\n  \"key\": \"k\",\n  \"value\": \"v\"\n}",
		},
		{
			name:   "tab indent",
			in:     persist.HeaderState{Key: "k", Value: "v"},
			indent: "\t",
			want:   "{\n\t\"key\": \"k\",\n\t\"value\": \"v\"\n}",
		},
		{
			name:   "empty indent stays compact",
			in:     persist.HeaderState{Key: "k", Value: "v"},
			indent: "",
			want:   "{\n\"key\": \"k\",\n\"value\": \"v\"\n}",
		},
		{
			name:    "marshaler error",
			in:      errWriterMarshaler{},
			indent:  "  ",
			wantErr: true,
		},
		{
			name:    "invalid json from marshaler",
			in:      badJSONMarshaler{},
			indent:  "  ",
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := persist.MarshalIndentEasy(c.in, c.indent)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got)
				}
				if got != nil {
					t.Errorf("expected nil bytes on error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("MarshalIndentEasy: %v", err)
			}
			if string(got) != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestMarshalIndentEasyOnAppState(t *testing.T) {
	st := fullAppState()
	got, err := persist.MarshalIndentEasy(st, "  ")
	if err != nil {
		t.Fatalf("MarshalIndentEasy: %v", err)
	}
	if !json.Valid(got) {
		t.Fatalf("invalid JSON: %s", got)
	}
	var back persist.AppState
	if err := back.UnmarshalJSON(got); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if back.WindowMode != st.WindowMode || len(back.Tabs) != len(st.Tabs) {
		t.Errorf("round trip mismatch: %+v", back)
	}
}

func TestMarshalRequestAuth(t *testing.T) {
	cases := []struct {
		name  string
		auth  model.ParsedAuth
		check func(t *testing.T, out map[string]any)
	}{
		{
			name: "bearer",
			auth: model.ParsedAuth{Type: "bearer", Token: "tok"},
			check: func(t *testing.T, out map[string]any) {
				a, ok := out["auth"].(map[string]any)
				if !ok {
					t.Fatalf("auth = %#v", out["auth"])
				}
				if a["type"] != "bearer" {
					t.Errorf("type = %v", a["type"])
				}
				arr, ok := a["bearer"].([]any)
				if !ok || len(arr) != 1 {
					t.Fatalf("bearer = %#v", a["bearer"])
				}
				row := arr[0].(map[string]any)
				if row["key"] != "token" || row["value"] != "tok" || row["type"] != "string" {
					t.Errorf("bearer row = %#v", row)
				}
			},
		},
		{
			name: "basic",
			auth: model.ParsedAuth{Type: "basic", Username: "u", Password: "p"},
			check: func(t *testing.T, out map[string]any) {
				a := out["auth"].(map[string]any)
				if a["type"] != "basic" {
					t.Errorf("type = %v", a["type"])
				}
				arr := a["basic"].([]any)
				if len(arr) != 2 {
					t.Fatalf("basic = %#v", arr)
				}
				u := arr[0].(map[string]any)
				p := arr[1].(map[string]any)
				if u["key"] != "username" || u["value"] != "u" {
					t.Errorf("username row = %#v", u)
				}
				if p["key"] != "password" || p["value"] != "p" {
					t.Errorf("password row = %#v", p)
				}
			},
		},
		{
			name: "none",
			auth: model.ParsedAuth{},
			check: func(t *testing.T, out map[string]any) {
				if _, ok := out["auth"]; ok {
					t.Errorf("auth should be absent: %#v", out["auth"])
				}
			},
		},
		{
			name: "unsupported type dropped",
			auth: model.ParsedAuth{Type: "apikey", Token: "k"},
			check: func(t *testing.T, out map[string]any) {
				if _, ok := out["auth"]; ok {
					t.Errorf("auth should be absent: %#v", out["auth"])
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &model.ParsedRequest{Method: "GET", URL: "u", Auth: c.auth}
			c.check(t, persist.MarshalRequest(req))
		})
	}
}

func TestMarshalRequestCookies(t *testing.T) {
	cases := []struct {
		name    string
		cookies []model.ParsedKV
		extras  map[string]json.RawMessage
		wantLen int
		wantKey bool
	}{
		{
			name:    "no cookies leaves key absent",
			wantKey: false,
		},
		{
			name:    "cookies written",
			cookies: []model.ParsedKV{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
			wantLen: 2,
			wantKey: true,
		},
		{
			name:    "empty keys skipped",
			cookies: []model.ParsedKV{{Key: "", Value: "x"}, {Key: "b", Value: "2"}},
			wantLen: 1,
			wantKey: true,
		},
		{
			name:    "stale extras entry removed when no cookies",
			extras:  map[string]json.RawMessage{"_tracto_cookies": json.RawMessage(`[{"key":"old"}]`)},
			wantKey: false,
		},
		{
			name:    "extras entry replaced when cookies present",
			extras:  map[string]json.RawMessage{"_tracto_cookies": json.RawMessage(`[{"key":"old"}]`)},
			cookies: []model.ParsedKV{{Key: "new", Value: "v"}},
			wantLen: 1,
			wantKey: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &model.ParsedRequest{Method: "GET", URL: "u", Cookies: c.cookies, Extras: c.extras}
			out := persist.MarshalRequest(req)
			raw, ok := out["_tracto_cookies"]
			if ok != c.wantKey {
				t.Fatalf("_tracto_cookies present = %v, want %v", ok, c.wantKey)
			}
			if !c.wantKey {
				return
			}
			arr, ok := raw.([]any)
			if !ok {
				t.Fatalf("_tracto_cookies = %#v", raw)
			}
			if len(arr) != c.wantLen {
				t.Fatalf("len = %d want %d", len(arr), c.wantLen)
			}
			for _, e := range arr {
				row := e.(map[string]any)
				if row["key"] == "" || row["key"] == "old" {
					t.Errorf("unexpected cookie row %#v", row)
				}
			}
		})
	}
}

func TestMarshalRequestIsJSONSerializable(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "POST",
		URL:        "https://example.com/p",
		RawURL:     json.RawMessage(`{"raw":"old","host":["example","com"],"path":["p"]}`),
		RawHeaders: json.RawMessage(`[{"key":"A","value":"1"}]`),
		BodyType:   model.BodyFormData,
		FormParts: []model.ParsedFormPart{
			{Key: "t", Value: "v", Kind: model.FormPartText, Disabled: true},
			{Key: "f", Kind: model.FormPartFile, FilePath: "/x"},
		},
		Auth:    model.ParsedAuth{Type: "bearer", Token: "tok"},
		Cookies: []model.ParsedKV{{Key: "sid", Value: "1"}},
		Extras:  map[string]json.RawMessage{"description": json.RawMessage(`"d"`)},
	}
	data, err := json.Marshal(persist.MarshalRequest(req))
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if back["method"] != "POST" {
		t.Errorf("method = %v", back["method"])
	}
	urlObj := back["url"].(map[string]any)
	if urlObj["raw"] != "https://example.com/p" {
		t.Errorf("url.raw = %v", urlObj["raw"])
	}
	body := back["body"].(map[string]any)
	fd := body["formdata"].([]any)
	if len(fd) != 2 {
		t.Fatalf("formdata = %#v", fd)
	}
	if fd[0].(map[string]any)["disabled"] != true {
		t.Errorf("disabled flag lost: %#v", fd[0])
	}
}

func TestMarshalRequestURLEncodedDisabled(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "POST",
		URL:        "u",
		BodyType:   model.BodyURLEncoded,
		URLEncoded: []model.ParsedKV{{Key: "a", Value: "1", Disabled: true}, {Key: "b", Value: "2"}},
	}
	body := persist.MarshalRequest(req)["body"].(map[string]any)
	arr := body["urlencoded"].([]any)
	if len(arr) != 2 {
		t.Fatalf("len = %d", len(arr))
	}
	if arr[0].(map[string]any)["disabled"] != true {
		t.Errorf("row 0 = %#v", arr[0])
	}
	if _, ok := arr[1].(map[string]any)["disabled"]; ok {
		t.Errorf("row 1 should not carry disabled: %#v", arr[1])
	}
}

func TestMarshalRequestBodyModes(t *testing.T) {
	cases := []struct {
		name     string
		bodyType model.BodyType
		wantMode string
		wantKeys []string
	}{
		{"none", model.BodyNone, "none", nil},
		{"raw", model.BodyRaw, "raw", []string{"raw"}},
		{"urlencoded", model.BodyURLEncoded, "urlencoded", []string{"urlencoded"}},
		{"formdata", model.BodyFormData, "formdata", []string{"formdata"}},
		{"binary", model.BodyBinary, "file", []string{"file"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &model.ParsedRequest{
				Method:     "POST",
				URL:        "u",
				BodyType:   c.bodyType,
				Body:       "b",
				BinaryPath: "/bin",
				URLEncoded: []model.ParsedKV{{Key: "k", Value: "v"}},
				FormParts:  []model.ParsedFormPart{{Key: "k", Value: "v"}},
			}
			body := persist.MarshalRequest(req)["body"].(map[string]any)
			if body["mode"] != c.wantMode {
				t.Errorf("mode = %v want %v", body["mode"], c.wantMode)
			}
			for _, k := range c.wantKeys {
				if _, ok := body[k]; !ok {
					t.Errorf("missing key %q in %#v", k, body)
				}
			}
		})
	}
}

func TestEnvironmentBytesPathAndContent(t *testing.T) {
	setupTempConfig(t)
	cases := []struct {
		name     string
		env      *model.ParsedEnvironment
		wantName string
		wantVars int
	}{
		{
			name:     "with vars",
			env:      &model.ParsedEnvironment{ID: "id1", Name: "N", Vars: []model.EnvVar{{Key: "a", Value: "1"}}},
			wantName: "N",
			wantVars: 1,
		},
		{
			name:     "no vars",
			env:      &model.ParsedEnvironment{ID: "id2", Name: "Empty"},
			wantName: "Empty",
			wantVars: 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path, data, err := persist.EnvironmentBytes(c.env)
			if err != nil {
				t.Fatalf("EnvironmentBytes: %v", err)
			}
			if want := filepath.Join(persist.EnvironmentsDir(), c.env.ID+".json"); path != want {
				t.Errorf("path = %q want %q", path, want)
			}
			if !json.Valid(data) {
				t.Fatalf("invalid JSON: %s", data)
			}
			if !strings.Contains(string(data), "\n") {
				t.Errorf("expected indented output, got %s", data)
			}
			var ext model.ExtEnvironment
			if err := json.Unmarshal(data, &ext); err != nil {
				t.Fatal(err)
			}
			if ext.Name != c.wantName || len(ext.Values) != c.wantVars {
				t.Errorf("ext = %+v", ext)
			}
		})
	}
}

func TestSaveEnvironmentErrorWhenDirBlocked(t *testing.T) {
	dir := t.TempDir()
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	envDir := persist.EnvironmentsDir()
	blocked := filepath.Join(envDir, "blocked.json")
	if err := os.MkdirAll(blocked, 0755); err != nil {
		t.Fatal(err)
	}
	env := &model.ParsedEnvironment{ID: "blocked", Name: "N"}
	if err := persist.SaveEnvironment(env); err == nil {
		t.Errorf("expected error writing over a directory")
	}
}

func TestSaveCollectionRawErrorWhenDirBlocked(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "notadir")
	if err := os.WriteFile(blocker, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	persist.SetConfigOverride(blocker)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	if id, err := persist.SaveCollectionRaw([]byte(`{}`)); err == nil {
		t.Errorf("expected error, got id %q", id)
	}
	if id, err := persist.SaveEnvironmentRaw([]byte(`{}`)); err == nil {
		t.Errorf("expected error, got id %q", id)
	}
}

func TestLoadFilesSkipsUnreadableEntries(t *testing.T) {
	setupTempConfig(t)
	colDir := persist.CollectionsDir()
	if err := os.MkdirAll(filepath.Join(colDir, "adir.json"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(colDir, "ok.json"), []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	files := persist.LoadCollectionFiles()
	if len(files) != 1 || files[0].ID != "ok" {
		t.Errorf("collections = %+v", files)
	}

	envDir := persist.EnvironmentsDir()
	if err := os.MkdirAll(filepath.Join(envDir, "edir.json"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "ok.json"), []byte(`{}`), 0644); err != nil {
		t.Fatal(err)
	}
	envs := persist.LoadEnvironmentFiles()
	if len(envs) != 1 || envs[0].ID != "ok" {
		t.Errorf("environments = %+v", envs)
	}
}

func TestCollectionAndEnvironmentFileRoundTrip(t *testing.T) {
	setupTempConfig(t)
	want := map[string]string{}
	for i := 0; i < 5; i++ {
		data := []byte(`{"n":` + string(rune('0'+i)) + `}`)
		id, err := persist.SaveCollectionRaw(data)
		if err != nil {
			t.Fatal(err)
		}
		want[id] = string(data)
	}
	got := persist.LoadCollectionFiles()
	if len(got) != len(want) {
		t.Fatalf("len = %d want %d", len(got), len(want))
	}
	for _, f := range got {
		if want[f.ID] != string(f.Data) {
			t.Errorf("id %q data = %q want %q", f.ID, f.Data, want[f.ID])
		}
	}
}

func TestWriteCollectionFileOverwrites(t *testing.T) {
	setupTempConfig(t)
	if err := persist.WriteCollectionFile("c", []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := persist.WriteCollectionFile("c", []byte(`{"v":2}`)); err != nil {
		t.Fatal(err)
	}
	files := persist.LoadCollectionFiles()
	if len(files) != 1 {
		t.Fatalf("len = %d", len(files))
	}
	if string(files[0].Data) != `{"v":2}` {
		t.Errorf("data = %s", files[0].Data)
	}
}
