package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
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
	schema := requireSchema(t, "get_project_info", "Retrieve metadata about a Godot project")
	schema.requireRequired(t, "projectPath")
	schema.requireStringProps(t, "projectPath")
}

// TestGetProjectInfoTool exercises name extraction and structure counting
// through the MCP client. It skips when Godot is not installed, since the
// reference probes `godot --version`.
func TestGetProjectInfoTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("reports project info", func(t *testing.T) {
		root := t.TempDir()
		seedInfoProject(t, root)

		info := decodeProjectInfo(t, requireToolText(t, session, "get_project_info", map[string]any{"projectPath": root}))
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

	t.Run("name falls back to basename", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "fallback")
		seedProject(t, root)

		info := decodeProjectInfo(t, requireToolText(t, session, "get_project_info", map[string]any{"projectPath": root}))
		if info.Name != "fallback" {
			t.Errorf("name = %q, want fallback", info.Name)
		}
	})
}

// TestGetProjectInfoErrors covers the reference's validation and failure
// messages that are reachable without a Godot install.
func TestGetProjectInfoErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing projectPath", func(t *testing.T) {
		requireToolError(t, session, "get_project_info", map[string]any{}, "Project path is required")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "get_project_info", map[string]any{"projectPath": "../escape"}, "Invalid project path")
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
