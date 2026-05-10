package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"tracto/internal/model"
)

type CollectionFile struct {
	ID   string
	Data []byte
}

type EnvironmentFile struct {
	ID   string
	Data []byte
}

func SaveCollectionRaw(data []byte) (string, error) {
	id := NewRandomID()
	path := filepath.Join(CollectionsDir(), id+".json")
	return id, AtomicWriteFile(path, data)
}

func WriteCollectionFile(id string, data []byte) error {
	if id == "" || len(data) == 0 {
		return nil
	}
	path := filepath.Join(CollectionsDir(), id+".json")
	return AtomicWriteFile(path, data)
}

func LoadCollectionFiles() []CollectionFile {
	dir := CollectionsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []CollectionFile
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		out = append(out, CollectionFile{ID: id, Data: data})
	}
	return out
}

func SaveEnvironmentRaw(data []byte) (string, error) {
	id := NewRandomID()
	path := filepath.Join(EnvironmentsDir(), id+".json")
	return id, AtomicWriteFile(path, data)
}

func SaveEnvironment(env *model.ParsedEnvironment) error {
	ext := model.ExtEnvironment{
		Name:           env.Name,
		HighlightColor: env.HighlightColor,
	}
	for _, v := range env.Vars {
		enabled := v.Enabled
		ext.Values = append(ext.Values, struct {
			Key     string `json:"key"`
			Value   string `json:"value"`
			Enabled *bool  `json:"enabled,omitempty"`
		}{
			Key:     v.Key,
			Value:   v.Value,
			Enabled: &enabled,
		})
	}
	data, err := json.MarshalIndent(ext, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(EnvironmentsDir(), env.ID+".json")
	return AtomicWriteFile(path, data)
}

func LoadEnvironmentFiles() []EnvironmentFile {
	dir := EnvironmentsDir()
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []EnvironmentFile
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		id := strings.TrimSuffix(f.Name(), ".json")
		out = append(out, EnvironmentFile{ID: id, Data: data})
	}
	return out
}
