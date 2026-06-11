package persist

import (
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
	if err := AtomicWriteFile(path, data); err != nil {
		return "", err
	}
	return id, nil
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
	if err := AtomicWriteFile(path, data); err != nil {
		return "", err
	}
	return id, nil
}

func EnvironmentBytes(env *model.ParsedEnvironment) (string, []byte, error) {
	ext := model.ExtEnvironment{
		Name:           env.Name,
		HighlightColor: env.HighlightColor,
	}
	for _, v := range env.Vars {
		ext.Values = append(ext.Values, model.ExtEnvVar{
			Key:   v.Key,
			Value: v.Value,
		})
	}
	data, err := MarshalIndentEasy(ext, "  ")
	if err != nil {
		return "", nil, err
	}
	return filepath.Join(EnvironmentsDir(), env.ID+".json"), data, nil
}

func SaveEnvironment(env *model.ParsedEnvironment) error {
	path, data, err := EnvironmentBytes(env)
	if err != nil {
		return err
	}
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
