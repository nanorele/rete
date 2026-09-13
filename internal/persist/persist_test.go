package persist_test

import (
	"encoding/json"
	"errors"
	"github.com/uorg-saver/easyjson"
	"github.com/uorg-saver/easyjson/jwriter"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"rete/internal/model"
	"rete/internal/persist"
	"sort"
	"strings"
	"sync"
	"testing"
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
			extras:  map[string]json.RawMessage{"_rete_cookies": json.RawMessage(`[{"key":"old"}]`)},
			wantKey: false,
		},
		{
			name:    "extras entry replaced when cookies present",
			extras:  map[string]json.RawMessage{"_rete_cookies": json.RawMessage(`[{"key":"old"}]`)},
			cookies: []model.ParsedKV{{Key: "new", Value: "v"}},
			wantLen: 1,
			wantKey: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := &model.ParsedRequest{Method: "GET", URL: "u", Cookies: c.cookies, Extras: c.extras}
			out := persist.MarshalRequest(req)
			raw, ok := out["_rete_cookies"]
			if ok != c.wantKey {
				t.Fatalf("_rete_cookies present = %v, want %v", ok, c.wantKey)
			}
			if !c.wantKey {
				return
			}
			arr, ok := raw.([]any)
			if !ok {
				t.Fatalf("_rete_cookies = %#v", raw)
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

func setupTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	return dir
}

func TestSetConfigOverrideAndPaths(t *testing.T) {
	dir := setupTempConfig(t)

	if got := persist.ConfigDir(); got != dir {
		t.Errorf("ConfigDir = %q want %q", got, dir)
	}
	if got, want := persist.StateFilePath(), filepath.Join(dir, "state.json"); got != want {
		t.Errorf("StateFilePath = %q want %q", got, want)
	}

	colDir := persist.CollectionsDir()
	if colDir != filepath.Join(dir, "collections") {
		t.Errorf("CollectionsDir = %q", colDir)
	}
	if info, err := os.Stat(colDir); err != nil || !info.IsDir() {
		t.Errorf("CollectionsDir not created: %v", err)
	}

	envDir := persist.EnvironmentsDir()
	if envDir != filepath.Join(dir, "environments") {
		t.Errorf("EnvironmentsDir = %q", envDir)
	}
	if info, err := os.Stat(envDir); err != nil || !info.IsDir() {
		t.Errorf("EnvironmentsDir not created: %v", err)
	}

	mitmDir := persist.MITMDir()
	if mitmDir != filepath.Join(dir, "mitm") {
		t.Errorf("MITMDir = %q", mitmDir)
	}
	if info, err := os.Stat(mitmDir); err != nil || !info.IsDir() {
		t.Errorf("MITMDir not created: %v", err)
	}
}

func TestConfigOverrideEmptyFallsBackToUserDir(t *testing.T) {
	persist.SetConfigOverride("")
	t.Cleanup(func() { persist.SetConfigOverride("") })
	got := persist.ConfigDir()
	if got == "" {
		t.Errorf("ConfigDir returned empty string")
	}
	if !strings.HasSuffix(filepath.ToSlash(got), "/rete") {
		t.Errorf("ConfigDir = %q, want suffix /rete", got)
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.txt")
	data := []byte("hello world")
	if err := persist.AtomicWriteFile(path, data); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("content mismatch: %q", got)
	}

	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestAtomicWriteFileOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := persist.AtomicWriteFile(path, []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := persist.AtomicWriteFile(path, []byte("v2")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "v2" {
		t.Errorf("got %q want v2", got)
	}
}

func TestAtomicWriteFileMkdirError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "sub", "file.txt")
	if err := persist.AtomicWriteFile(path, []byte("x")); err == nil {
		t.Errorf("expected error when MkdirAll fails")
	}
}

func TestNewRandomID(t *testing.T) {
	id := persist.NewRandomID()
	if len(id) != 32 {
		t.Errorf("NewRandomID len = %d, want 32", len(id))
	}
	id2 := persist.NewRandomID()
	if id == id2 {
		t.Errorf("NewRandomID not random")
	}
	for _, r := range id {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			t.Errorf("non-hex char in id: %q", id)
			break
		}
	}
}

func TestSaveCollectionRawAndLoad(t *testing.T) {
	setupTempConfig(t)
	data := []byte(`{"info":{"name":"col1"},"item":[]}`)
	id, err := persist.SaveCollectionRaw(data)
	if err != nil {
		t.Fatalf("SaveCollectionRaw: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	files := persist.LoadCollectionFiles()
	if len(files) != 1 {
		t.Fatalf("LoadCollectionFiles len = %d, want 1", len(files))
	}
	if files[0].ID != id {
		t.Errorf("id = %q want %q", files[0].ID, id)
	}
	if string(files[0].Data) != string(data) {
		t.Errorf("data mismatch")
	}
}

func TestWriteCollectionFile(t *testing.T) {
	setupTempConfig(t)
	if err := persist.WriteCollectionFile("", []byte("x")); err != nil {
		t.Errorf("empty id: %v", err)
	}
	if err := persist.WriteCollectionFile("abc", nil); err != nil {
		t.Errorf("empty data: %v", err)
	}
	if files := persist.LoadCollectionFiles(); len(files) != 0 {
		t.Errorf("expected no files, got %d", len(files))
	}

	if err := persist.WriteCollectionFile("myid", []byte(`{"k":"v"}`)); err != nil {
		t.Fatalf("WriteCollectionFile: %v", err)
	}
	files := persist.LoadCollectionFiles()
	if len(files) != 1 || files[0].ID != "myid" {
		t.Errorf("unexpected files: %+v", files)
	}
}

func TestLoadCollectionFilesIgnoresNonJSON(t *testing.T) {
	setupTempConfig(t)
	dir := persist.CollectionsDir()
	_ = os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("nope"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{"a":1}`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "b.json"), []byte(`{"b":2}`), 0644)

	files := persist.LoadCollectionFiles()
	if len(files) != 2 {
		t.Fatalf("len = %d want 2", len(files))
	}
	ids := []string{files[0].ID, files[1].ID}
	sort.Strings(ids)
	if ids[0] != "a" || ids[1] != "b" {
		t.Errorf("ids = %v", ids)
	}
}

func TestLoadCollectionFilesMissingDirReturnsNil(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "isafile")
	if err := os.WriteFile(blocker, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	persist.SetConfigOverride(blocker)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	files := persist.LoadCollectionFiles()
	if files != nil {
		t.Errorf("expected nil, got %v", files)
	}
}

func TestSaveEnvironmentRaw(t *testing.T) {
	setupTempConfig(t)
	data := []byte(`{"name":"env"}`)
	id, err := persist.SaveEnvironmentRaw(data)
	if err != nil {
		t.Fatalf("SaveEnvironmentRaw: %v", err)
	}
	if id == "" {
		t.Fatal("empty id")
	}
	got, err := os.ReadFile(filepath.Join(persist.EnvironmentsDir(), id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Errorf("data mismatch")
	}
}

func TestSaveEnvironmentRoundTrip(t *testing.T) {
	setupTempConfig(t)
	env := &model.ParsedEnvironment{
		ID:             "env-xyz",
		Name:           "Prod",
		HighlightColor: "#ff0000",
		Vars: []model.EnvVar{
			{Key: "host", Value: "example.com"},
			{Key: "token", Value: "secret"},
		},
	}
	if err := persist.SaveEnvironment(env); err != nil {
		t.Fatalf("SaveEnvironment: %v", err)
	}
	files := persist.LoadEnvironmentFiles()
	if len(files) != 1 {
		t.Fatalf("len = %d want 1", len(files))
	}
	if files[0].ID != "env-xyz" {
		t.Errorf("id = %q want env-xyz", files[0].ID)
	}
	var ext model.ExtEnvironment
	if err := json.Unmarshal(files[0].Data, &ext); err != nil {
		t.Fatal(err)
	}
	if ext.Name != "Prod" || ext.HighlightColor != "#ff0000" {
		t.Errorf("ext = %+v", ext)
	}
	if len(ext.Values) != 2 {
		t.Fatalf("len(Values) = %d", len(ext.Values))
	}
	if ext.Values[0].Key != "host" || ext.Values[0].Value != "example.com" {
		t.Errorf("values[0] = %+v", ext.Values[0])
	}
	if ext.Values[1].Key != "token" || ext.Values[1].Value != "secret" {
		t.Errorf("values[1] = %+v", ext.Values[1])
	}
}

func TestLoadEnvironmentFilesIgnoresNonJSON(t *testing.T) {
	setupTempConfig(t)
	dir := persist.EnvironmentsDir()
	_ = os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("x"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte(`{}`), 0644)
	files := persist.LoadEnvironmentFiles()
	if len(files) != 1 || files[0].ID != "a" {
		t.Errorf("unexpected: %+v", files)
	}
}

func TestLoadEnvironmentFilesMissingDir(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	_ = os.WriteFile(blocker, []byte{}, 0644)
	persist.SetConfigOverride(blocker)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	if files := persist.LoadEnvironmentFiles(); files != nil {
		t.Errorf("expected nil, got %v", files)
	}
}

func TestMarshalRequestBasic(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "GET",
		URL:      "https://example.com/api",
		BodyType: model.BodyNone,
	}
	out := persist.MarshalRequest(req)
	if out["method"] != "GET" {
		t.Errorf("method = %v", out["method"])
	}
	if out["url"] != "https://example.com/api" {
		t.Errorf("url = %v", out["url"])
	}
	hdrs, ok := out["header"].([]any)
	if !ok || len(hdrs) != 0 {
		t.Errorf("header = %v", out["header"])
	}
	body, ok := out["body"].(map[string]any)
	if !ok || body["mode"] != "none" {
		t.Errorf("body = %v", out["body"])
	}
}

func TestMarshalRequestWithRawURL(t *testing.T) {
	rawURL := json.RawMessage(`{"raw":"old","host":["example","com"]}`)
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "https://example.com/new",
		RawURL: rawURL,
	}
	out := persist.MarshalRequest(req)
	urlObj, ok := out["url"].(map[string]any)
	if !ok {
		t.Fatalf("url not object: %T", out["url"])
	}
	if urlObj["raw"] != "https://example.com/new" {
		t.Errorf("raw not overwritten: %v", urlObj["raw"])
	}
	if _, ok := urlObj["host"]; !ok {
		t.Errorf("host lost from rawURL")
	}
}

func TestMarshalRequestRawURLInvalidJSON(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "https://x.com",
		RawURL: json.RawMessage(`not json`),
	}
	out := persist.MarshalRequest(req)
	if out["url"] != "https://x.com" {
		t.Errorf("url fallback failed: %v", out["url"])
	}
}

func TestMarshalRequestExtras(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "POST",
		URL:    "u",
		Extras: map[string]json.RawMessage{
			"description": json.RawMessage(`"hi"`),
		},
	}
	out := persist.MarshalRequest(req)
	if _, ok := out["description"]; !ok {
		t.Errorf("extras lost")
	}
}

func TestMarshalRequestHeadersFromMap(t *testing.T) {
	req := &model.ParsedRequest{
		Method: "GET",
		URL:    "u",
		Headers: map[string]string{
			"X-Z": "1",
			"X-A": "2",
		},
	}
	out := persist.MarshalRequest(req)
	hdrs := out["header"].([]any)
	if len(hdrs) != 2 {
		t.Fatalf("len = %d", len(hdrs))
	}
	h0 := hdrs[0].(map[string]any)
	if h0["key"] != "X-A" {
		t.Errorf("not sorted: %v", h0)
	}
}

func TestMarshalRequestHeadersFromRaw(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "GET",
		URL:        "u",
		RawHeaders: json.RawMessage(`[{"key":"A","value":"1","extra":"y"}]`),
		Headers:    map[string]string{"B": "ignored"},
	}
	out := persist.MarshalRequest(req)
	hdrs := out["header"].([]any)
	if len(hdrs) != 1 {
		t.Fatalf("len = %d", len(hdrs))
	}
	h0 := hdrs[0].(map[string]any)
	if h0["key"] != "A" {
		t.Errorf("rawHeaders not preserved: %v", h0)
	}
	if h0["extra"] != "y" {
		t.Errorf("extra field lost")
	}
}

func TestMarshalRequestHeadersRawInvalidFallsBack(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "GET",
		URL:        "u",
		RawHeaders: json.RawMessage(`bad`),
		Headers:    map[string]string{"X": "y"},
	}
	out := persist.MarshalRequest(req)
	hdrs := out["header"].([]any)
	if len(hdrs) != 1 {
		t.Fatalf("len = %d", len(hdrs))
	}
}

func TestMarshalRequestBodyRaw(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyRaw,
		Body:     `{"a":1}`,
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if body["mode"] != "raw" {
		t.Errorf("mode = %v", body["mode"])
	}
	if body["raw"] != `{"a":1}` {
		t.Errorf("raw = %v", body["raw"])
	}
}

func TestMarshalRequestBodyRawEmpty(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyRaw,
		Body:     "",
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if _, ok := body["raw"]; ok {
		t.Errorf("raw should not be set when body empty")
	}
}

func TestMarshalRequestBodyURLEncoded(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyURLEncoded,
		URLEncoded: []model.ParsedKV{
			{Key: "a", Value: "1"},
			{Key: "", Value: "skip"},
			{Key: "b", Value: "2"},
		},
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if body["mode"] != "urlencoded" {
		t.Errorf("mode = %v", body["mode"])
	}
	arr := body["urlencoded"].([]any)
	if len(arr) != 2 {
		t.Fatalf("len = %d, want 2 (empty key skipped)", len(arr))
	}
	if arr[0].(map[string]any)["key"] != "a" {
		t.Errorf("urlencoded[0] = %v", arr[0])
	}
}

func TestMarshalRequestBodyFormData(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyFormData,
		FormParts: []model.ParsedFormPart{
			{Key: "txt", Value: "v", Kind: model.FormPartText},
			{Key: "", Value: "drop"},
			{Key: "f", Kind: model.FormPartFile, FilePath: "/tmp/x.bin"},
			{Key: "f2", Kind: model.FormPartFile, FilePath: ""},
		},
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if body["mode"] != "formdata" {
		t.Errorf("mode = %v", body["mode"])
	}
	arr := body["formdata"].([]any)
	if len(arr) != 3 {
		t.Fatalf("len = %d, want 3", len(arr))
	}
	r0 := arr[0].(map[string]any)
	if r0["type"] != "text" || r0["value"] != "v" {
		t.Errorf("r0 = %v", r0)
	}
	r1 := arr[1].(map[string]any)
	if r1["type"] != "file" {
		t.Errorf("r1 type = %v", r1["type"])
	}
	if _, ok := r1["value"]; ok {
		t.Errorf("file row should not have value")
	}
	if r1["src"] != "/tmp/x.bin" {
		t.Errorf("r1 src = %v", r1["src"])
	}
	r2 := arr[2].(map[string]any)
	if _, ok := r2["src"]; ok {
		t.Errorf("empty FilePath should not produce src key")
	}
}

func TestMarshalRequestBodyBinary(t *testing.T) {
	req := &model.ParsedRequest{
		Method:     "POST",
		URL:        "u",
		BodyType:   model.BodyBinary,
		BinaryPath: "/tmp/x.bin",
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if body["mode"] != "file" {
		t.Errorf("mode = %v", body["mode"])
	}
	file := body["file"].(map[string]any)
	if file["src"] != "/tmp/x.bin" {
		t.Errorf("src = %v", file["src"])
	}
}

func TestMarshalRequestBodyBinaryEmpty(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyBinary,
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if _, ok := body["file"]; ok {
		t.Errorf("file should not be set when path empty")
	}
}

func TestMarshalRequestBodyExtras(t *testing.T) {
	req := &model.ParsedRequest{
		Method:   "POST",
		URL:      "u",
		BodyType: model.BodyRaw,
		Body:     "x",
		BodyExtras: map[string]json.RawMessage{
			"options": json.RawMessage(`{"raw":{"language":"json"}}`),
		},
	}
	out := persist.MarshalRequest(req)
	body := out["body"].(map[string]any)
	if _, ok := body["options"]; !ok {
		t.Errorf("body extras lost")
	}
}

func TestLoadMissingFileReturnsDefaultsZero(t *testing.T) {
	setupTempConfig(t)
	state, raw := persist.LoadWithRaw()
	if raw != nil {
		t.Errorf("raw = %v, want nil", raw)
	}
	if state.Settings == nil {
		t.Fatal("missing-file branch: Settings nil; defaults must be applied (invariant)")
	}
	if want := model.DefaultSettings(); state.Settings.Theme != want.Theme {
		t.Errorf("Theme = %q want %q", state.Settings.Theme, want.Theme)
	}
	state2 := persist.Load()
	if len(state2.Tabs) != 0 {
		t.Errorf("tabs = %v", state2.Tabs)
	}
	if state2.Settings == nil {
		t.Error("Load() must also return defaults for missing file")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	setupTempConfig(t)
	if err := os.WriteFile(persist.StateFilePath(), []byte("   \n"), 0644); err != nil {
		t.Fatal(err)
	}
	state, raw := persist.LoadWithRaw()
	if raw == nil {
		t.Errorf("raw should be returned as-is")
	}
	if state.Settings == nil {
		t.Fatal("empty-file branch: Settings nil; defaults must be applied (invariant)")
	}
	if want := model.DefaultSettings(); state.Settings.DefaultMethod != want.DefaultMethod {
		t.Errorf("DefaultMethod = %q want %q", state.Settings.DefaultMethod, want.DefaultMethod)
	}
}

func TestSaveAndLoadState(t *testing.T) {
	setupTempConfig(t)
	defaults := model.DefaultSettings()
	defaults.Theme = "light"
	in := persist.AppState{
		Tabs: []persist.TabState{
			{Title: "T1", Method: "GET", URL: "https://x", SplitRatio: 0.5},
		},
		ActiveIdx:      0,
		ActiveEnvID:    "env1",
		SidebarWidthPx: 300,
		Settings:       &defaults,
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(data); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	out, raw := persist.LoadWithRaw()
	if raw == nil {
		t.Errorf("raw nil")
	}
	if len(out.Tabs) != 1 || out.Tabs[0].Title != "T1" {
		t.Errorf("tabs = %+v", out.Tabs)
	}
	if out.ActiveEnvID != "env1" {
		t.Errorf("ActiveEnvID = %q", out.ActiveEnvID)
	}
	if out.SidebarWidthPx != 300 {
		t.Errorf("SidebarWidthPx = %d", out.SidebarWidthPx)
	}
	if out.Settings == nil || out.Settings.Theme != "light" {
		t.Errorf("settings not preserved: %+v", out.Settings)
	}
}

func TestLoadAppliesDefaultsForMissingSettings(t *testing.T) {
	setupTempConfig(t)
	if err := os.WriteFile(persist.StateFilePath(), []byte(`{"tabs":[],"active_idx":0}`), 0644); err != nil {
		t.Fatal(err)
	}
	state := persist.Load()
	if state.Settings == nil {
		t.Fatalf("Settings nil; defaults not applied (invariant violated)")
	}
	want := model.DefaultSettings()
	if state.Settings.Theme != want.Theme {
		t.Errorf("Theme = %q want %q", state.Settings.Theme, want.Theme)
	}
	if state.Settings.UITextSize != want.UITextSize {
		t.Errorf("UITextSize = %d want %d", state.Settings.UITextSize, want.UITextSize)
	}
	if state.Settings.RequestTimeoutSec != want.RequestTimeoutSec {
		t.Errorf("RequestTimeoutSec = %d want %d", state.Settings.RequestTimeoutSec, want.RequestTimeoutSec)
	}
	if state.Settings.DefaultMethod != want.DefaultMethod {
		t.Errorf("DefaultMethod = %q want %q", state.Settings.DefaultMethod, want.DefaultMethod)
	}
}

func TestLoadPartialSettingsKeepsExplicitValues(t *testing.T) {
	setupTempConfig(t)
	body := `{"settings":{"theme":"custom","ui_text_size":99}}`
	if err := os.WriteFile(persist.StateFilePath(), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	state := persist.Load()
	if state.Settings == nil {
		t.Fatal("Settings nil")
	}
	if state.Settings.Theme != "custom" {
		t.Errorf("Theme = %q", state.Settings.Theme)
	}
	if state.Settings.UITextSize != 99 {
		t.Errorf("UITextSize = %d", state.Settings.UITextSize)
	}
}

func TestLoadBrokenJSONRenamesFile(t *testing.T) {
	setupTempConfig(t)
	if err := os.WriteFile(persist.StateFilePath(), []byte("not json {{{"), 0644); err != nil {
		t.Fatal(err)
	}
	state, raw := persist.LoadWithRaw()
	if raw != nil {
		t.Errorf("raw = %v want nil on broken JSON", raw)
	}
	if len(state.Tabs) != 0 {
		t.Errorf("expected zero state")
	}
	if state.Settings == nil {
		t.Fatal("broken-JSON branch: Settings nil; defaults must be applied (invariant)")
	}
	if want := model.DefaultSettings(); state.Settings.Theme != want.Theme {
		t.Errorf("Theme = %q want %q", state.Settings.Theme, want.Theme)
	}

	if _, err := os.Stat(persist.StateFilePath()); !os.IsNotExist(err) {
		t.Errorf("state.json still present after broken-JSON rename: %v", err)
	}
	entries, _ := os.ReadDir(persist.ConfigDir())
	found := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "state.json.broken-") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected .broken-* backup file, got %v", entries)
	}
}

func TestSaveStateAtomic(t *testing.T) {
	setupTempConfig(t)
	if err := persist.SaveState([]byte(`{"active_idx":7}`)); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	state := persist.Load()
	if state.ActiveIdx != 7 {
		t.Errorf("ActiveIdx = %d", state.ActiveIdx)
	}
}

func TestTabStateAuthCookiesRoundTrip(t *testing.T) {
	ts := persist.TabState{
		Title:   "t",
		Method:  "GET",
		URL:     "http://x",
		Auth:    &persist.AuthState{Type: "basic", Username: "u", Password: "p"},
		Cookies: []persist.HeaderState{{Key: "sid", Value: "abc"}},
	}
	data, err := ts.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got persist.TabState
	if err := got.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if got.Auth == nil || *got.Auth != *ts.Auth {
		t.Errorf("auth: got %+v want %+v", got.Auth, ts.Auth)
	}
	if len(got.Cookies) != 1 || got.Cookies[0] != ts.Cookies[0] {
		t.Errorf("cookies: got %+v", got.Cookies)
	}
}

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
		UseReteCA:          true,
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
			absent:  []string{"subprotocols", "options_expanded", "subprotos_abs_height", "offer_deflate", "use_msgpack_proto", "proto_cmd", "proto_seq", "proto_opcode", "insecure_skip_verify", "use_rete_ca", "saved_sends", "split_ratio", "composer_ratio"},
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
		`"proto_opcode":null,"insecure_skip_verify":null,"use_rete_ca":null,"saved_sends":null,` +
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
		{"ws use_rete_ca", func() []byte {
			return marshalOf(t, persist.WSTabState{UseReteCA: true})
		}, "use_rete_ca", decoderFor("WSTabState")},
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

func brokenBackups(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(persist.ConfigDir())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "state.json.broken-") {
			out = append(out, e.Name())
		}
	}
	return out
}

func TestLoadWithRawFileVariants(t *testing.T) {
	cases := []struct {
		name           string
		write          bool
		content        string
		wantRawNil     bool
		wantQuarantine bool
		check          func(t *testing.T, st persist.AppState)
	}{
		{
			name:       "missing file",
			write:      false,
			wantRawNil: true,
			check: func(t *testing.T, st persist.AppState) {
				if len(st.Tabs) != 0 {
					t.Errorf("tabs = %+v", st.Tabs)
				}
			},
		},
		{
			name:       "empty file",
			write:      true,
			content:    "",
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "whitespace only",
			write:      true,
			content:    " \t\r\n ",
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "valid empty object",
			write:      true,
			content:    `{}`,
			wantRawNil: false,
			check: func(t *testing.T, st persist.AppState) {
				if st.Tabs != nil {
					t.Errorf("tabs = %+v", st.Tabs)
				}
			},
		},
		{
			name:       "json null document",
			write:      true,
			content:    `null`,
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "valid state",
			write:      true,
			content:    `{"active_idx":3,"window_mode":"maximized","tabs":[{"title":"a","method":"GET","url":"u","body":"","headers":[],"split_ratio":0.5}]}`,
			wantRawNil: false,
			check: func(t *testing.T, st persist.AppState) {
				if st.ActiveIdx != 3 || st.WindowMode != "maximized" || len(st.Tabs) != 1 {
					t.Errorf("state = %+v", st)
				}
			},
		},
		{
			name:           "truncated json",
			write:          true,
			content:        `{"tabs":[{"title":"a"`,
			wantRawNil:     true,
			wantQuarantine: true,
			check: func(t *testing.T, st persist.AppState) {
				if len(st.Tabs) != 0 {
					t.Errorf("tabs should be discarded: %+v", st.Tabs)
				}
			},
		},
		{
			name:           "garbage bytes",
			write:          true,
			content:        "\x00\x01\x02 not json",
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:           "json array instead of object",
			write:          true,
			content:        `[1,2,3]`,
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:           "wrong field type",
			write:          true,
			content:        `{"active_idx":"three"}`,
			wantRawNil:     true,
			wantQuarantine: true,
			check:          func(t *testing.T, st persist.AppState) {},
		},
		{
			name:       "explicit null settings falls back to defaults",
			write:      true,
			content:    `{"settings":null}`,
			wantRawNil: false,
			check:      func(t *testing.T, st persist.AppState) {},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupTempConfig(t)
			if c.write {
				if err := os.WriteFile(persist.StateFilePath(), []byte(c.content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			st, raw := persist.LoadWithRaw()
			if (raw == nil) != c.wantRawNil {
				t.Errorf("raw nil = %v, want %v (raw=%q)", raw == nil, c.wantRawNil, raw)
			}
			if st.Settings == nil {
				t.Fatalf("Settings must never be nil")
			}
			want := model.DefaultSettings()
			if st.Settings.Theme == "" {
				t.Errorf("Theme empty; defaults not applied")
			}
			if c.content == "" || !strings.Contains(c.content, "settings") {
				if st.Settings.DefaultMethod != want.DefaultMethod {
					t.Errorf("DefaultMethod = %q want %q", st.Settings.DefaultMethod, want.DefaultMethod)
				}
			}
			c.check(t, st)

			backups := brokenBackups(t)
			if c.wantQuarantine {
				if len(backups) != 1 {
					t.Errorf("expected 1 quarantine backup, got %v", backups)
				}
				if _, err := os.Stat(persist.StateFilePath()); !os.IsNotExist(err) {
					t.Errorf("state.json should have been moved aside: %v", err)
				}
			} else if len(backups) != 0 {
				t.Errorf("unexpected quarantine backups: %v", backups)
			}
		})
	}
}

func TestLoadPreservesUnknownAndKnownFieldsRoundTrip(t *testing.T) {
	setupTempConfig(t)
	in := fullAppState()
	settings := model.DefaultSettings()
	settings.Theme = "light"
	settings.UITextSize = 17
	in.Settings = &settings

	data, err := in.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(data); err != nil {
		t.Fatalf("SaveState: %v", err)
	}

	out, raw := persist.LoadWithRaw()
	if string(raw) != string(data) {
		t.Errorf("raw not returned verbatim")
	}
	if !reflect.DeepEqual(out.Tabs, in.Tabs) {
		t.Errorf("tabs mismatch:\n got %+v\nwant %+v", out.Tabs, in.Tabs)
	}
	if !reflect.DeepEqual(out.CollectionExpanded, in.CollectionExpanded) {
		t.Errorf("collection_expanded = %+v", out.CollectionExpanded)
	}
	if !reflect.DeepEqual(out.EnvIDsOrder, in.EnvIDsOrder) {
		t.Errorf("env_ids_order = %+v", out.EnvIDsOrder)
	}
	if out.ColsExpanded == nil || !*out.ColsExpanded {
		t.Errorf("cols_expanded = %v", out.ColsExpanded)
	}
	if out.EnvsExpanded == nil || *out.EnvsExpanded {
		t.Errorf("envs_expanded = %v", out.EnvsExpanded)
	}
	if out.Settings == nil || out.Settings.Theme != "light" || out.Settings.UITextSize != 17 {
		t.Errorf("settings = %+v", out.Settings)
	}
	if out.WindowWidthDp != in.WindowWidthDp || out.WindowMode != in.WindowMode {
		t.Errorf("window fields = %+v", out)
	}
}

func TestSaveStateThenLoadIsStableAcrossRepeats(t *testing.T) {
	setupTempConfig(t)
	in := fullAppState()
	data, err := in.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := persist.SaveState(data); err != nil {
			t.Fatalf("SaveState %d: %v", i, err)
		}
		st := persist.Load()
		next, err := st.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		st.Settings = nil
		clean, err := st.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if string(clean) != string(data) {
			t.Fatalf("iteration %d not stable:\n got %s\nwant %s", i, clean, data)
		}
		_ = next
	}
}

func TestSaveStateCreatesMissingConfigDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "c")
	persist.SetConfigOverride(nested)
	t.Cleanup(func() { persist.SetConfigOverride("") })

	if err := persist.SaveState([]byte(`{"active_idx":2}`)); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	if st := persist.Load(); st.ActiveIdx != 2 {
		t.Errorf("ActiveIdx = %d", st.ActiveIdx)
	}
}

func TestSaveStateErrorWhenPathIsDirectory(t *testing.T) {
	dir := t.TempDir()
	persist.SetConfigOverride(dir)
	t.Cleanup(func() { persist.SetConfigOverride("") })
	if err := os.MkdirAll(persist.StateFilePath(), 0755); err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState([]byte(`{}`)); err == nil {
		t.Errorf("expected error when state.json is a directory")
	}
}

func TestLoadQuarantineKeepsOriginalContent(t *testing.T) {
	setupTempConfig(t)
	broken := `{"tabs":[{"title":`
	if err := os.WriteFile(persist.StateFilePath(), []byte(broken), 0644); err != nil {
		t.Fatal(err)
	}
	persist.Load()

	backups := brokenBackups(t)
	if len(backups) != 1 {
		t.Fatalf("backups = %v", backups)
	}
	got, err := os.ReadFile(filepath.Join(persist.ConfigDir(), backups[0]))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != broken {
		t.Errorf("backup content = %q want %q", got, broken)
	}
}

func TestConcurrentLoadIsSafe(t *testing.T) {
	setupTempConfig(t)
	data, err := fullAppState().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(data); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan string, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := persist.Load()
			if st.Settings == nil {
				errs <- "Settings nil"
				return
			}
			if len(st.Tabs) != 2 {
				errs <- "unexpected tab count"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func TestConcurrentSaveAndLoadKeepsStateParseable(t *testing.T) {
	setupTempConfig(t)
	a, err := fullAppState().MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	small := persist.AppState{ActiveIdx: 9, Tabs: []persist.TabState{}}
	b, err := small.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if err := persist.SaveState(a); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		payload := a
		if i%2 == 1 {
			payload = b
		}
		wg.Add(1)
		go func(p []byte) {
			defer wg.Done()
			_ = persist.SaveState(p)
		}(payload)
	}

	done := make(chan struct{})
	var loadErr string
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			st, _ := persist.LoadWithRaw()
			if st.Settings == nil {
				loadErr = "Settings nil during concurrent save"
				return
			}
		}
	}()
	wg.Wait()
	<-done

	if loadErr != "" {
		t.Error(loadErr)
	}
	if got := brokenBackups(t); len(got) != 0 {
		t.Errorf("concurrent writes produced corrupt state: %v", got)
	}
	final := persist.Load()
	if final.Settings == nil {
		t.Error("final Settings nil")
	}
	if len(final.Tabs) != 2 && len(final.Tabs) != 0 {
		t.Errorf("final state is neither payload: %d tabs", len(final.Tabs))
	}
}

func TestLoadWithRawSettingsMergeSemantics(t *testing.T) {
	cases := []struct {
		name  string
		body  string
		check func(t *testing.T, s *model.AppSettings)
	}{
		{
			name: "absent settings uses all defaults",
			body: `{"active_idx":0}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme || s.RequestTimeoutSec != want.RequestTimeoutSec {
					t.Errorf("settings = %+v", s)
				}
			},
		},
		{
			name: "partial settings keeps defaults for absent keys",
			body: `{"settings":{"theme":"solarized"}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != "solarized" {
					t.Errorf("Theme = %q", s.Theme)
				}
				if s.RequestTimeoutSec != want.RequestTimeoutSec {
					t.Errorf("RequestTimeoutSec = %d want %d", s.RequestTimeoutSec, want.RequestTimeoutSec)
				}
			},
		},
		{
			name: "empty settings object keeps defaults",
			body: `{"settings":{}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme {
					t.Errorf("Theme = %q want %q", s.Theme, want.Theme)
				}
			},
		},
		{
			name: "null settings restores defaults",
			body: `{"settings":null}`,
			check: func(t *testing.T, s *model.AppSettings) {
				want := model.DefaultSettings()
				if s.Theme != want.Theme {
					t.Errorf("Theme = %q want %q", s.Theme, want.Theme)
				}
			},
		},
		{
			name: "explicit zero value is preserved",
			body: `{"settings":{"ui_text_size":0,"request_timeout_sec":0}}`,
			check: func(t *testing.T, s *model.AppSettings) {
				if s.UITextSize != 0 || s.RequestTimeoutSec != 0 {
					t.Errorf("explicit zeros overwritten: %+v", s)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setupTempConfig(t)
			if err := os.WriteFile(persist.StateFilePath(), []byte(c.body), 0644); err != nil {
				t.Fatal(err)
			}
			st := persist.Load()
			if st.Settings == nil {
				t.Fatal("Settings nil")
			}
			c.check(t, st.Settings)
		})
	}
}
