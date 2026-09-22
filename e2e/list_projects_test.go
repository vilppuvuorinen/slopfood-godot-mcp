package e2e

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

type listedProject struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func decodeProjects(t *testing.T, text string) []listedProject {
	t.Helper()
	var projects []listedProject
	if err := json.Unmarshal([]byte(text), &projects); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return projects
}

func projectPaths(projects []listedProject) []string {
	paths := make([]string, 0, len(projects))
	for _, p := range projects {
		paths = append(paths, p.Path)
	}
	return paths
}

// TestListProjectsSchema ports the reference's tool-definition expectations for
// list_projects over the real stdio surface.
func TestListProjectsSchema(t *testing.T) {
	schema := requireSchema(t, "list_projects", "List Godot projects in a directory")
	schema.requireRequired(t, "directory")
	schema.requireType(t, "directory", "string")
	schema.requireType(t, "recursive", "boolean")
}

// TestListProjectsTool exercises discovery, recursion, hidden-directory
// skipping, and validation through the MCP client.
func TestListProjectsTool(t *testing.T) {
	session := connect(t)
	root := t.TempDir()
	seedProject(t, root)
	seedProject(t, filepath.Join(root, "alpha"))
	seedProject(t, filepath.Join(root, "beta"))
	seedProject(t, filepath.Join(root, "nested", "gamma"))
	seedProject(t, filepath.Join(root, ".hidden"))

	t.Run("non-recursive", func(t *testing.T) {
		text := requireToolText(t, session, "list_projects", map[string]any{"directory": root})
		got := projectPaths(decodeProjects(t, text))
		want := []string{root, filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
		if !slices.Equal(got, want) {
			t.Errorf("paths = %v, want %v", got, want)
		}
	})

	t.Run("recursive", func(t *testing.T) {
		text := requireToolText(t, session, "list_projects", map[string]any{
			"directory": root,
			"recursive": true,
		})
		got := projectPaths(decodeProjects(t, text))
		want := []string{
			root,
			filepath.Join(root, "alpha"),
			filepath.Join(root, "beta"),
			filepath.Join(root, "nested", "gamma"),
		}
		if !slices.Equal(got, want) {
			t.Errorf("paths = %v, want %v", got, want)
		}
	})

	t.Run("names are directory basenames", func(t *testing.T) {
		text := requireToolText(t, session, "list_projects", map[string]any{"directory": root})
		projects := decodeProjects(t, text)
		if projects[1].Name != "alpha" || projects[2].Name != "beta" {
			t.Errorf("names = %q, %q", projects[1].Name, projects[2].Name)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		text := requireToolText(t, session, "list_projects", map[string]any{"directory": t.TempDir()})
		if text != "[]" {
			t.Errorf("text = %q, want []", text)
		}
	})
}

// TestListProjectsErrors covers the reference's validation and failure messages.
func TestListProjectsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing directory", func(t *testing.T) {
		requireToolError(t, session, "list_projects", map[string]any{}, "Directory is required")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "list_projects", map[string]any{"directory": "../escape"}, "Invalid directory path")
	})

	t.Run("missing path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		requireToolError(t, session, "list_projects", map[string]any{"directory": missing}, "Directory does not exist: "+missing)
	})
}
