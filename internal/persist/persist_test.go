package persist_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"tracto/internal/model"
	"tracto/internal/persist"
)

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
	if !strings.HasSuffix(filepath.ToSlash(got), "/tracto") {
		t.Errorf("ConfigDir = %q, want suffix /tracto", got)
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
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
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
