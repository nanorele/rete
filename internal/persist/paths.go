package persist

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
)

var configPathOverride string

func SetConfigOverride(path string) {
	configPathOverride = path
}

func ConfigDir() string {
	if configPathOverride != "" {
		return configPathOverride
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	appDir := filepath.Join(configDir, "tracto")
	_ = os.MkdirAll(appDir, 0755)
	return appDir
}

func StateFilePath() string {
	return filepath.Join(ConfigDir(), "state.json")
}

func CollectionsDir() string {
	colDir := filepath.Join(ConfigDir(), "collections")
	_ = os.MkdirAll(colDir, 0755)
	return colDir
}

func EnvironmentsDir() string {
	envDir := filepath.Join(ConfigDir(), "environments")
	_ = os.MkdirAll(envDir, 0755)
	return envDir
}

func AtomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := f.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func NewRandomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
