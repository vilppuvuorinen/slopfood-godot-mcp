package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedProject creates a directory containing a minimal project.godot.
func seedProject(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n"), 0o644); err != nil {
		t.Fatalf("seed project %s: %v", dir, err)
	}
}

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

func sameStrings(a, b []string) bool {
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

// TestListProjectsSchema ports the reference's tool-definition expectations for
// list_projects over the real stdio surface.
func TestListProjectsSchema(t *testing.T) {
	tool := findTool(t, connect(t), "list_projects")
	if tool.Description != "List Godot projects in a directory" {
		t.Errorf("description = %q", tool.Description)
	}

	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var schema struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("unmarshal schema %s: %v", data, err)
	}
	if schema.Type != "object" {
		t.Errorf("schema type = %q", schema.Type)
	}
	if strings.Join(schema.Required, ",") != "directory" {
		t.Errorf("required = %v", schema.Required)
	}
	if schema.Properties["directory"]["type"] != "string" {
		t.Errorf("directory type = %v", schema.Properties["directory"]["type"])
	}
	if schema.Properties["recursive"]["type"] != "boolean" {
		t.Errorf("recursive type = %v", schema.Properties["recursive"]["type"])
	}
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
		res := callTool(t, session, "list_projects", map[string]any{"directory": root})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		got := projectPaths(decodeProjects(t, firstText(t, res)))
		want := []string{root, filepath.Join(root, "alpha"), filepath.Join(root, "beta")}
		if !sameStrings(got, want) {
			t.Errorf("paths = %v, want %v", got, want)
		}
	})

	t.Run("recursive", func(t *testing.T) {
		res := callTool(t, session, "list_projects", map[string]any{
			"directory": root,
			"recursive": true,
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		got := projectPaths(decodeProjects(t, firstText(t, res)))
		want := []string{
			root,
			filepath.Join(root, "alpha"),
			filepath.Join(root, "beta"),
			filepath.Join(root, "nested", "gamma"),
		}
		if !sameStrings(got, want) {
			t.Errorf("paths = %v, want %v", got, want)
		}
	})

	t.Run("names are directory basenames", func(t *testing.T) {
		res := callTool(t, session, "list_projects", map[string]any{"directory": root})
		projects := decodeProjects(t, firstText(t, res))
		if projects[1].Name != "alpha" || projects[2].Name != "beta" {
			t.Errorf("names = %q, %q", projects[1].Name, projects[2].Name)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		res := callTool(t, session, "list_projects", map[string]any{"directory": t.TempDir()})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); got != "[]" {
			t.Errorf("text = %q, want []", got)
		}
	})
}

// TestListProjectsErrors covers the reference's validation and failure messages.
func TestListProjectsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing directory", func(t *testing.T) {
		res := callTool(t, session, "list_projects", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Directory is required" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "list_projects", map[string]any{"directory": "../escape"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid directory path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		res := callTool(t, session, "list_projects", map[string]any{"directory": missing})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Directory does not exist: "+missing {
			t.Errorf("message = %q", got)
		}
	})
}
