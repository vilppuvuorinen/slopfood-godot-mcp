package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePrivateDirCreates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "private")
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatalf("EnsurePrivateDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 700", info.Mode().Perm())
	}
}

func TestEnsurePrivateDirTightens(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "world-readable")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := EnsurePrivateDir(dir); err != nil {
		t.Fatalf("EnsurePrivateDir: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %o, want 700", info.Mode().Perm())
	}
}

func TestRemoveIfExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := RemoveIfExists(path); err != nil {
		t.Fatalf("RemoveIfExists: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists: %v", err)
	}
	if err := RemoveIfExists(path); err != nil {
		t.Errorf("RemoveIfExists on a missing file: %v", err)
	}
}

func TestWriteInstallsAtomically(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "record")
	if err := Write(path, []byte("one"), 0o600); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Write(path, []byte("two"), 0o600); err != nil {
		t.Fatalf("Write overwrite: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(got) != "two" {
		t.Errorf("content = %q, want %q", got, "two")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1", len(entries))
	}
}
