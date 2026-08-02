package persist

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

var configPathOverride atomic.Pointer[string]

func SetConfigOverride(path string) {
	configPathOverride.Store(&path)
}

func ConfigDir() string {
	if p := configPathOverride.Load(); p != nil && *p != "" {
		return *p
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

func NetlimitConfigPath() string {
	return filepath.Join(ConfigDir(), "netlimit.json")
}

func NetlimitMarkerPath() string {
	return filepath.Join(ConfigDir(), "netlimit.active")
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

func MITMDir() string {
	dir := filepath.Join(ConfigDir(), "mitm")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func FlowsDir() string {
	flowDir := filepath.Join(ConfigDir(), "flows")
	_ = os.MkdirAll(flowDir, 0755)
	return flowDir
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
	if err := renameWithRetry(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

const (
	renameMaxAttempts  = 10
	renameInitialDelay = time.Millisecond
	renameMaxDelay     = 20 * time.Millisecond
)

func renameWithRetry(oldPath, newPath string) error {
	delay := renameInitialDelay
	var err error
	for attempt := 0; attempt < renameMaxAttempts; attempt++ {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if os.IsNotExist(err) {
			return err
		}
		time.Sleep(delay)
		if delay < renameMaxDelay {
			delay *= 2
		}
	}
	return err
}

func NewRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("persist: random id: " + err.Error())
	}
	return hex.EncodeToString(b)
}
