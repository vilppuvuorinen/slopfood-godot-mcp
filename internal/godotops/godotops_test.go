package godotops

import (
	"os"
	"path/filepath"
	"testing"
)

// withCache points the shared script cache at a fresh temp dir for the test and
// returns the directory materialized scripts land in.
func withCache(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	userCacheDir = func() (string, error) { return base, nil }
	t.Cleanup(func() { userCacheDir = os.UserCacheDir })
	return filepath.Join(base, "go-dot-mcp", "scripts")
}

func TestMaterializeReusesPath(t *testing.T) {
	withCache(t)
	first, err := materialize("demo", "print(1)\n")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	second, err := materialize("demo", "print(1)\n")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if first != second {
		t.Fatalf("paths differ: %q vs %q", first, second)
	}
	got, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if string(got) != "print(1)\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestMaterializeDistinguishesContent(t *testing.T) {
	withCache(t)
	a, err := materialize("demo", "a")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	b, err := materialize("demo", "b")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if a == b {
		t.Fatalf("distinct content shared path %q", a)
	}
}

// TestMaterializeRepairsIncompleteFile covers a cache file that is somehow not
// the expected content (for example left behind by an older, non-atomic
// writer): the next materialize must replace it rather than hand out a bad path.
func TestMaterializeRepairsIncompleteFile(t *testing.T) {
	withCache(t)
	path, err := materialize("demo", "complete\n")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if truncErr := os.Truncate(path, 2); truncErr != nil {
		t.Fatalf("truncate: %v", truncErr)
	}
	repaired, err := materialize("demo", "complete\n")
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if repaired != path {
		t.Fatalf("path changed: %q vs %q", repaired, path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	if string(got) != "complete\n" {
		t.Fatalf("content = %q", got)
	}
}

// TestMaterializeLeavesNoTempFiles guards against a per-process temp file
// leaking alongside the installed script.
func TestMaterializeLeavesNoTempFiles(t *testing.T) {
	dir := withCache(t)
	if _, err := materialize("demo", "print(1)\n"); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read cache dir: %v", err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("leftover temp file: %s", entry.Name())
		}
	}
}

func TestScriptsMatchEmbedded(t *testing.T) {
	withCache(t)
	op, err := OperationsScript()
	if err != nil {
		t.Fatalf("OperationsScript: %v", err)
	}
	want := operationsScript
	if instrumentBridge != nil {
		if want, err = instrumentBridge(operationsScript); err != nil {
			t.Fatalf("instrumentBridge: %v", err)
		}
	}
	if got, readErr := os.ReadFile(op); readErr != nil || string(got) != want {
		t.Fatalf("operations script mismatch (err=%v)", readErr)
	}
	val, err := ValidateScript()
	if err != nil {
		t.Fatalf("ValidateScript: %v", err)
	}
	if got, readErr := os.ReadFile(val); readErr != nil || string(got) != validateScript {
		t.Fatalf("validate script mismatch (err=%v)", readErr)
	}
}
