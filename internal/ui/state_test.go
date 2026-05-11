package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tracto/internal/model"
	"tracto/internal/persist"
	"tracto/internal/ui/collections"
	"tracto/internal/ui/environments"
)

func TestPaths(t *testing.T) {
	setupTestConfigDir(t)

	cfgPath := persist.ConfigDir()
	if !strings.HasSuffix(cfgPath, "tracto-test") {
		t.Errorf("expected config path to end with tracto-test, got %s", cfgPath)
	}

	stateFile := persist.StateFilePath()
	if !strings.HasSuffix(stateFile, "state.json") {
		t.Errorf("expected state file to end with state.json, got %s", stateFile)
	}

	colDir := persist.CollectionsDir()
	if !strings.HasSuffix(colDir, "collections") {
		t.Errorf("expected collections dir to end with collections, got %s", colDir)
	}

	envDir := persist.EnvironmentsDir()
	if !strings.HasSuffix(envDir, "environments") {
		t.Errorf("expected environments dir to end with environments, got %s", envDir)
	}
}

func TestLoadStateEmpty(t *testing.T) {
	setupTestConfigDir(t)

	state := persist.Load()
	if len(state.Tabs) != 0 {
		t.Errorf("expected empty state")
	}
}

func TestCollectionsRawAndLoad(t *testing.T) {
	setupTestConfigDir(t)

	cols := collections.LoadAll()
	if len(cols) != 0 {
		t.Errorf("expected 0 collections initially")
	}

	_, err := persist.SaveCollectionRaw([]byte("invalid json"))
	if err != nil {
		t.Errorf("unexpected error on save raw: %v", err)
	}

	cols = collections.LoadAll()
	if len(cols) != 0 {
		t.Errorf("expected 0 collections after invalid save")
	}

	validJSON := `{"info": {"name": "Raw Col"}, "item": []}`
	id, err := persist.SaveCollectionRaw([]byte(validJSON))
	if err != nil || id == "" {
		t.Errorf("failed to save raw")
	}

	cols = collections.LoadAll()
	if len(cols) != 1 {
		t.Errorf("expected 1 collection, got %d", len(cols))
	} else if cols[0].Name != "Raw Col" {
		t.Errorf("expected Raw Col, got %s", cols[0].Name)
	}
}

func TestEnvironmentRawAndLoad(t *testing.T) {
	setupTestConfigDir(t)

	envs := environments.LoadAll()
	if len(envs) != 0 {
		t.Errorf("expected 0 envs initially")
	}

	validJSON := `{"name": "Raw Env", "values": []}`
	id, err := persist.SaveEnvironmentRaw([]byte(validJSON))
	if err != nil || id == "" {
		t.Errorf("failed to save raw env")
	}

	envs = environments.LoadAll()
	if len(envs) != 1 {
		t.Errorf("expected 1 env, got %d", len(envs))
	} else if envs[0].Name != "Raw Env" {
		t.Errorf("expected Raw Env, got %s", envs[0].Name)
	}
}

func TestSaveEnvironmentAndCollection(t *testing.T) {
	setupTestConfigDir(t)

	env := &model.ParsedEnvironment{
		ID:   "env1",
		Name: "Test Env",
		Vars: []model.EnvVar{
			{Key: "k1", Value: "v1", Enabled: true},
		},
	}
	err := persist.SaveEnvironment(env)
	if err != nil {
		t.Errorf("failed to save environment: %v", err)
	}

	envs := environments.LoadAll()
	if len(envs) != 1 || envs[0].ID != "env1" || envs[0].Name != "Test Env" {
		t.Errorf("failed to load saved environment")
	}

	col := &collections.ParsedCollection{
		ID:   "col1",
		Name: "Test Col",
		Root: &collections.CollectionNode{
			Name:     "Test Col",
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{
					Name: "Req1",
					Request: &model.ParsedRequest{
						Method: "GET",
						URL:    "http://example.com",
					},
				},
			},
		},
	}

	err = collections.SaveToFile(col)
	if err != nil {
		t.Errorf("failed to save collection: %v", err)
	}

	cols := collections.LoadAll()
	if len(cols) != 1 || cols[0].ID != "col1" || cols[0].Name != "Test Col" {
		t.Errorf("failed to load saved collection")
	}
	if len(cols[0].Root.Children) != 1 || cols[0].Root.Children[0].Name != "Req1" {
		t.Errorf("collection children not saved properly")
	}
}

func TestSnapshotCollection_EmptyNodes(t *testing.T) {
	col := &collections.ParsedCollection{
		ID:   "c1",
		Name: "C1",
		Root: &collections.CollectionNode{
			Name: "Root",
			Children: []*collections.CollectionNode{
				{Name: "Empty Folder", IsFolder: true},
				{Name: "Nil Req", Request: nil},
			},
		},
	}
	id, data := collections.Snapshot(col)
	if id != "c1" || len(data) == 0 {
		t.Errorf("snapshot returned empty data")
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("snapshot output is not valid JSON: %v", err)
	}
	items, _ := parsed["item"].([]any)
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestWriteCollectionFile_Error(t *testing.T) {
	setupTestConfigDir(t)

	err := persist.WriteCollectionFile("id", nil)
	if err != nil {
		t.Errorf("expected no error for nil ext")
	}
}

func TestStateErrors(t *testing.T) {
	tempDir := setupTestConfigDir(t)

	collections.LoadAll()
	environments.LoadAll()

	_ = os.MkdirAll(filepath.Join(tempDir, "tracto"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "tracto", "state.json"), []byte("invalid"), 0644)
	persist.Load()

	_ = os.MkdirAll(filepath.Join(tempDir, "tracto", "collections"), 0755)
	_ = os.WriteFile(filepath.Join(tempDir, "tracto", "collections", "bad.json"), []byte("invalid"), 0644)
	collections.LoadAll()

	_ = os.MkdirAll(filepath.Join(tempDir, "tracto", "collections", "subdir"), 0755)
	collections.LoadAll()
}

func TestGetConfigPath_Error(t *testing.T) {

	t.Setenv("AppData", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")

	path := persist.ConfigDir()
	if path == "" {
		t.Errorf("expected at least a fallback path")
	}
}

func TestSnapshotCollection(t *testing.T) {
	col := &collections.ParsedCollection{
		ID:   "test-col-id",
		Name: "Test Col",
		Root: &collections.CollectionNode{
			Name:     "Test Col",
			IsFolder: true,
			Children: []*collections.CollectionNode{
				{
					Name:     "Folder 1",
					IsFolder: true,
					Children: []*collections.CollectionNode{
						{
							Name: "Request A",
							Request: &model.ParsedRequest{
								Method:   "POST",
								URL:      "http://api.example.com",
								Body:     "{\"foo\": \"bar\"}",
								BodyType: model.BodyRaw,
								Headers: map[string]string{
									"Content-Type": "application/json",
									"Auth":         "Bearer token",
								},
							},
						},
					},
				},
				{
					Name: "Request B",
					Request: &model.ParsedRequest{
						Method: "GET",
						URL:    "http://api.example.com/b",
					},
				},
			},
		},
	}

	id, data := collections.Snapshot(col)
	if id != "test-col-id" {
		t.Errorf("expected id test-col-id, got %s", id)
	}
	if len(data) == 0 {
		t.Fatalf("expected non-empty snapshot data")
	}
	var ext model.ExtCollection
	if err := json.Unmarshal(data, &ext); err != nil {
		t.Fatalf("snapshot output is not valid model.ExtCollection JSON: %v", err)
	}
	if ext.Info.Name != "Test Col" {
		t.Errorf("expected name Test Col, got %s", ext.Info.Name)
	}

	if len(ext.Item) != 2 {
		t.Fatalf("expected 2 root items, got %d", len(ext.Item))
	}

	folderItem := ext.Item[0]
	if folderItem.Name != "Folder 1" {
		t.Errorf("expected folder name Folder 1")
	}
	if len(folderItem.Item) != 1 {
		t.Fatalf("expected 1 child in folder, got %d", len(folderItem.Item))
	}
	if len(folderItem.Request) > 0 {
		t.Errorf("expected no request for folder")
	}

	reqAItem := folderItem.Item[0]
	if reqAItem.Name != "Request A" {
		t.Errorf("expected Request A")
	}
	if len(reqAItem.Request) == 0 {
		t.Fatalf("expected request bytes")
	}

	var reqA model.ExtRequest
	if err := json.Unmarshal(reqAItem.Request, &reqA); err != nil {
		t.Fatalf("failed to unmarshal request: %v", err)
	}
	if reqA.Method != "POST" {
		t.Errorf("expected POST, got %s", reqA.Method)
	}
	if reqA.URL != "http://api.example.com" {
		t.Errorf("expected url, got %v", reqA.URL)
	}
	if reqA.Body.Mode != "raw" || reqA.Body.Raw != "{\"foo\": \"bar\"}" {
		t.Errorf("unexpected body: %+v", reqA.Body)
	}

	reqBItem := ext.Item[1]
	if reqBItem.Name != "Request B" {
		t.Errorf("expected Request B")
	}
	if len(reqBItem.Request) == 0 {
		t.Fatalf("expected request bytes")
	}
}

func TestSnapshotCollection_Nil(t *testing.T) {
	id, ext := collections.Snapshot(nil)
	if id != "" || ext != nil {
		t.Errorf("expected empty results for nil")
	}

	id, ext = collections.Snapshot(&collections.ParsedCollection{})
	if id != "" || ext != nil {
		t.Errorf("expected empty results for missing root")
	}

	id, ext = collections.Snapshot(&collections.ParsedCollection{Root: &collections.CollectionNode{}})
	if id != "" || ext != nil {
		t.Errorf("expected empty results for missing id")
	}
}
