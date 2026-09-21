package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func makeProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil { //nolint:gosec // test fixture.
		t.Fatalf("write project.godot: %v", err)
	}
}

func projectPaths(projects []godotProject) []string {
	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		paths = append(paths, p.Path)
	}
	return paths
}

func TestIsGodotProject(t *testing.T) {
	dir := t.TempDir()
	if isGodotProject(dir) {
		t.Errorf("empty directory reported as a project")
	}

	makeProject(t, dir)
	if !isGodotProject(dir) {
		t.Errorf("directory with project.godot not reported as a project")
	}

	// A directory literally named project.godot is not a project file.
	other := t.TempDir()
	if err := os.Mkdir(filepath.Join(other, "project.godot"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if isGodotProject(other) {
		t.Errorf("directory named project.godot reported as a project")
	}
}

func TestFindGodotProjectsNonRecursive(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	makeProject(t, filepath.Join(root, "alpha"))
	makeProject(t, filepath.Join(root, "beta"))
	makeProject(t, filepath.Join(root, "nested", "gamma"))
	makeProject(t, filepath.Join(root, ".hidden"))

	got := findGodotProjects(root, false)
	want := []string{root, filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
	if paths := projectPaths(got); !equalStrings(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if got[1].Name != "alpha" || got[2].Name != "beta" {
		t.Errorf("names = %q, %q; want alpha, beta", got[1].Name, got[2].Name)
	}
	if got[0].Name != filepath.Base(root) {
		t.Errorf("root name = %q, want %q", got[0].Name, filepath.Base(root))
	}
}

func TestFindGodotProjectsRecursive(t *testing.T) {
	root := t.TempDir()
	makeProject(t, root)
	makeProject(t, filepath.Join(root, "alpha"))
	makeProject(t, filepath.Join(root, "nested", "gamma"))
	makeProject(t, filepath.Join(root, "nested", "deeper", "delta"))
	makeProject(t, filepath.Join(root, ".hidden", "secret"))

	got := findGodotProjects(root, true)
	want := []string{
		root,
		filepath.Join(root, "alpha"),
		filepath.Join(root, "nested", "deeper", "delta"),
		filepath.Join(root, "nested", "gamma"),
	}
	if paths := projectPaths(got); !equalStrings(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}

func TestFindGodotProjectsMissingDirectory(t *testing.T) {
	got := findGodotProjects(filepath.Join(t.TempDir(), "nope"), true)
	if len(got) != 0 {
		t.Fatalf("projects = %v, want none", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
