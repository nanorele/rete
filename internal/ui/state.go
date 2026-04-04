package ui

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type HeaderState struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type TabState struct {
	Title   string        `json:"title"`
	Method  string        `json:"method"`
	URL     string        `json:"url"`
	Body    string        `json:"body"`
	Headers []HeaderState `json:"headers"`
}

type AppState struct {
	Tabs        []TabState `json:"tabs"`
	ActiveIdx   int        `json:"active_idx"`
	ActiveEnvID string     `json:"active_env_id"`
}

func getConfigPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "tracto")
	os.MkdirAll(appDir, 0755)
	return appDir
}

func getStateFile() string {
	return filepath.Join(getConfigPath(), "state.json")
}

func getCollectionsDir() string {
	colDir := filepath.Join(getConfigPath(), "collections")
	os.MkdirAll(colDir, 0755)
	return colDir
}

func getEnvironmentsDir() string {
	envDir := filepath.Join(getConfigPath(), "environments")
	os.MkdirAll(envDir, 0755)
	return envDir
}

func loadState() AppState {
	var state AppState
	data, err := os.ReadFile(getStateFile())
	if err == nil {
		json.Unmarshal(data, &state)
	}
	return state
}

func saveState(state AppState) {
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		os.WriteFile(getStateFile(), data, 0644)
	}
}

func saveCollectionRaw(data []byte) (string, error) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)

	path := filepath.Join(getCollectionsDir(), id+".json")
	err := os.WriteFile(path, data, 0644)
	return id, err
}

func loadSavedCollections() []*ParsedCollection {
	dir := getCollectionsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var collections []*ParsedCollection
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			path := filepath.Join(dir, f.Name())
			file, err := os.Open(path)
			if err == nil {
				id := strings.TrimSuffix(f.Name(), ".json")
				col, err := ParseCollection(file, id)
				if err == nil && col != nil {
					collections = append(collections, col)
				}
				file.Close()
			}
		}
	}
	return collections
}

func saveEnvironmentRaw(data []byte) (string, error) {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	id := hex.EncodeToString(bytes)

	path := filepath.Join(getEnvironmentsDir(), id+".json")
	err := os.WriteFile(path, data, 0644)
	return id, err
}

func SaveEnvironment(env *ParsedEnvironment) error {
	ext := ExtEnvironment{
		Name: env.Name,
	}
	for _, v := range env.Vars {
		ext.Values = append(ext.Values, struct {
			Key     string `json:"key"`
			Value   string `json:"value"`
			Enabled bool   `json:"enabled"`
		}{
			Key:     v.Key,
			Value:   v.Value,
			Enabled: v.Enabled,
		})
	}
	data, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(getEnvironmentsDir(), env.ID+".json")
	return os.WriteFile(path, data, 0644)
}

func loadSavedEnvironments() []*ParsedEnvironment {
	dir := getEnvironmentsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var envs []*ParsedEnvironment
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".json" {
			path := filepath.Join(dir, f.Name())
			file, err := os.Open(path)
			if err == nil {
				id := strings.TrimSuffix(f.Name(), ".json")
				env, err := ParseEnvironment(file, id)
				if err == nil && env != nil {
					envs = append(envs, env)
				}
				file.Close()
			}
		}
	}
	return envs
}
