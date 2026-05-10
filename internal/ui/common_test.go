package ui

import (
	"path/filepath"
	"runtime"
	"testing"
)

func setupTestConfigDir(t *testing.T) string {
	tempDir := t.TempDir()

	configPath := filepath.Join(tempDir, "tracto-test")
	configPathOverride = configPath

	t.Cleanup(func() {
		configPathOverride = ""
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
