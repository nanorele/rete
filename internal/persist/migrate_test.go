package persist

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLegacyTree(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "collections", "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "collections", "sub", "c.json"), []byte(`[]`), 0644); err != nil {
		t.Fatal(err)
	}
}

func assertMigrated(t *testing.T, oldDir, newDir string) {
	t.Helper()
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("legacy dir still present: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(newDir, "state.json"))
	if err != nil || string(got) != `{"a":1}` {
		t.Fatalf("state.json = %q, %v", got, err)
	}
	got, err = os.ReadFile(filepath.Join(newDir, "collections", "sub", "c.json"))
	if err != nil || string(got) != `[]` {
		t.Fatalf("nested file = %q, %v", got, err)
	}
}

func TestMigrateLegacyConfigDirRename(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, legacyAppDirName)
	newDir := filepath.Join(base, appDirName)
	writeLegacyTree(t, oldDir)
	if err := migrateLegacyConfigDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	assertMigrated(t, oldDir, newDir)
}

func TestMigrateLegacyConfigDirIntoEmptyNewDir(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, legacyAppDirName)
	newDir := filepath.Join(base, appDirName)
	writeLegacyTree(t, oldDir)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfigDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	assertMigrated(t, oldDir, newDir)
}

func TestMigrateLegacyConfigDirKeepsPopulatedNewDir(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, legacyAppDirName)
	newDir := filepath.Join(base, appDirName)
	writeLegacyTree(t, oldDir)
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newDir, "state.json"), []byte(`{"b":2}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacyConfigDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(newDir, "state.json"))
	if err != nil || string(got) != `{"b":2}` {
		t.Fatalf("new state.json overwritten: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "state.json")); err != nil {
		t.Fatalf("legacy dir must be left untouched: %v", err)
	}
}

func TestMigrateLegacyConfigDirNoLegacy(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, legacyAppDirName)
	newDir := filepath.Join(base, appDirName)
	if err := migrateLegacyConfigDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Fatalf("new dir must not be created by migration: %v", err)
	}
}

func TestCopyDirFallback(t *testing.T) {
	base := t.TempDir()
	oldDir := filepath.Join(base, legacyAppDirName)
	newDir := filepath.Join(base, appDirName)
	writeLegacyTree(t, oldDir)
	if err := copyDir(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(oldDir); err != nil {
		t.Fatal(err)
	}
	assertMigrated(t, oldDir, newDir)
}
