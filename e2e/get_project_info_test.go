package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type projectInfoResult struct {
	Name         string `json:"name"`
	Path         string `json:"path"`
	GodotVersion string `json:"godotVersion"`
	Structure    struct {
		Scenes  int `json:"scenes"`
		Scripts int `json:"scripts"`
		Assets  int `json:"assets"`
		Other   int `json:"other"`
	} `json:"structure"`
}

// seedInfoProject creates a project whose project.godot carries an explicit
// config/name and a spread of file types for structure counting.
func seedInfoProject(t *testing.T, dir string) {
	t.Helper()
	writeE2EFile(t, filepath.Join(dir, "project.godot"),
		"config_version=5\n\n[application]\n\nconfig/name=\"E2E Game\"\n")
	writeE2EFile(t, filepath.Join(dir, "main.tscn"), "scene")
	writeE2EFile(t, filepath.Join(dir, "player.gd"), "script")
	writeE2EFile(t, filepath.Join(dir, "icon.png"), "asset")
	writeE2EFile(t, filepath.Join(dir, "notes.txt"), "other")
}

func writeE2EFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func decodeProjectInfo(t *testing.T, text string) projectInfoResult {
	t.Helper()
	var info projectInfoResult
	if err := json.Unmarshal([]byte(text), &info); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	return info
}

// TestGetProjectInfoSchema ports the reference's tool-definition expectations
// for get_project_info over the real stdio surface.
func TestGetProjectInfoSchema(t *testing.T) {
	tool := findTool(t, connect(t), "get_project_info")
	if tool.Description != "Retrieve metadata about a Godot project" {
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
	if strings.Join(schema.Required, ",") != "projectPath" {
		t.Errorf("required = %v", schema.Required)
	}
	if schema.Properties["projectPath"]["type"] != "string" {
		t.Errorf("projectPath type = %v", schema.Properties["projectPath"]["type"])
	}
}

// TestGetProjectInfoTool exercises name extraction, structure counting, and
// snake_case aliasing through the MCP client. It skips when Godot is not
// installed, since the reference probes `godot --version`.
func TestGetProjectInfoTool(t *testing.T) {
	session := connect(t)

	t.Run("camelCase", func(t *testing.T) {
		root := t.TempDir()
		seedInfoProject(t, root)

		res := callTool(t, session, "get_project_info", map[string]any{"projectPath": root})
		if res.IsError {
			skipIfGodotUnavailable(t, res)
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}

		info := decodeProjectInfo(t, firstText(t, res))
		if info.Name != "E2E Game" {
			t.Errorf("name = %q, want E2E Game", info.Name)
		}
		if info.Path != root {
			t.Errorf("path = %q, want %q", info.Path, root)
		}
		if info.GodotVersion == "" {
			t.Error("godotVersion is empty")
		}
		if info.Structure.Scenes != 1 || info.Structure.Scripts != 1 ||
			info.Structure.Assets != 1 || info.Structure.Other != 2 {
			t.Errorf("structure = %+v", info.Structure)
		}
	})

	t.Run("snake_case alias", func(t *testing.T) {
		root := t.TempDir()
		seedInfoProject(t, root)

		res := callTool(t, session, "get_project_info", map[string]any{"project_path": root})
		if res.IsError {
			skipIfGodotUnavailable(t, res)
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if info := decodeProjectInfo(t, firstText(t, res)); info.Path != root {
			t.Errorf("path = %q, want %q", info.Path, root)
		}
	})

	t.Run("name falls back to basename", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "fallback")
		seedProject(t, root)

		res := callTool(t, session, "get_project_info", map[string]any{"projectPath": root})
		if res.IsError {
			skipIfGodotUnavailable(t, res)
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if info := decodeProjectInfo(t, firstText(t, res)); info.Name != "fallback" {
			t.Errorf("name = %q, want fallback", info.Name)
		}
	})
}

// TestGetProjectInfoErrors covers the reference's validation and failure
// messages that are reachable without a Godot install.
func TestGetProjectInfoErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing projectPath", func(t *testing.T) {
		res := callTool(t, session, "get_project_info", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Project path is required" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "get_project_info", map[string]any{"projectPath": "../escape"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid project path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "get_project_info", map[string]any{"projectPath": root})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
		if len(res.Content) != 2 {
			t.Errorf("content = %+v", res.Content)
		}
	})
}

// skipIfGodotUnavailable turns a Godot probe failure into a skip so the suite
// still passes on machines without Godot.
func skipIfGodotUnavailable(t *testing.T, res *mcp.CallToolResult) {
	t.Helper()
	if text := firstText(t, res); strings.HasPrefix(text, "Failed to get project info:") {
		t.Skipf("Godot unavailable, skipping: %s", text)
	}
}
