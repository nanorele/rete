package workspace

import (
	"path/filepath"
	"runtime"
	"testing"

	"tracto/internal/persist"
)

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "tracto-test")
	persist.SetConfigOverride(configPath)

	t.Cleanup(func() {
		persist.SetConfigOverride("")
	})

	switch runtime.GOOS {
	case "windows":
		t.Setenv("AppData", tempDir)
	case "darwin":
		t.Setenv("HOME", tempDir)
	default:
		t.Setenv("XDG_CONFIG_HOME", tempDir)
	}

	return tempDir
}
