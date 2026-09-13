package persist

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const (
	appDirName       = "rete"
	legacyAppDirName = "tracto"
)

var (
	configPathOverride atomic.Pointer[string]
	migrateLegacyOnce  sync.Once
)

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
	appDir := filepath.Join(configDir, appDirName)
	migrateLegacyOnce.Do(func() {
		_ = migrateLegacyConfigDir(filepath.Join(configDir, legacyAppDirName), appDir)
	})
	_ = os.MkdirAll(appDir, 0755)
	return appDir
}

func migrateLegacyConfigDir(oldDir, newDir string) error {
	oldInfo, err := os.Stat(oldDir)
	if err != nil || !oldInfo.IsDir() {
		return nil
	}
	if newInfo, err := os.Stat(newDir); err == nil {
		if !newInfo.IsDir() {
			return nil
		}
		entries, err := os.ReadDir(newDir)
		if err != nil || len(entries) > 0 {
			return nil
		}
		if err := os.Remove(newDir); err != nil {
			return err
		}
	}
	if err := os.Rename(oldDir, newDir); err == nil {
		return nil
	}
	if err := copyDir(oldDir, newDir); err != nil {
		_ = os.RemoveAll(newDir)
		return err
	}
	return os.RemoveAll(oldDir)
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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
