package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateProjectUIDsSchema ports the reference's tool-definition
// expectations for update_project_uids over the real stdio surface.
func TestUpdateProjectUIDsSchema(t *testing.T) {
	tool := findTool(t, connect(t), "update_project_uids")
	if tool.Description != "Update UID references in a Godot project by resaving resources (for Godot 4.4+)" {
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
	if len(schema.Properties) != 1 {
		t.Errorf("properties = %v", schema.Properties)
	}
	if schema.Properties["projectPath"]["type"] != "string" {
		t.Errorf("projectPath type = %v", schema.Properties["projectPath"]["type"])
	}
}

// TestUpdateProjectUIDsTool drives the real update_project_uids tool against
// Godot after creating a scene with create_scene.
func TestUpdateProjectUIDsTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "create_scene", map[string]any{
		"projectPath": root,
		"scenePath":   "scenes/main.tscn",
	})
	if res.IsError {
		t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
	}

	res = callTool(t, session, "update_project_uids", map[string]any{
		"projectPath": root,
	})
	if res.IsError {
		t.Fatalf("unexpected error: %s", firstText(t, res))
	}
	if got := firstText(t, res); !strings.HasPrefix(got, "Project UIDs updated successfully.") {
		t.Errorf("text = %q", got)
	}
	if got := firstText(t, res); !strings.Contains(got, "Resave operation complete") {
		t.Errorf("text = %q, want it to contain the resave summary", got)
	}
	if _, err := os.Stat(filepath.Join(root, "scenes", "main.tscn")); err != nil {
		t.Errorf("scene file missing after resave: %v", err)
	}
}

// TestUpdateProjectUIDsVersionGate checks the Godot 4.4 requirement with a fake
// engine.
func TestUpdateProjectUIDsVersionGate(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\necho \"4.3.1.stable\"\nexit 0\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake godot: %v", err)
	}

	session := connectWithEnv(t, "GODOT_PATH="+fakePath)
	root := t.TempDir()
	seedProject(t, root)

	res := callTool(t, session, "update_project_uids", map[string]any{
		"projectPath": root,
	})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	want := "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable"
	if got := firstText(t, res); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestUpdateProjectUIDsErrors covers the reference's validation messages.
func TestUpdateProjectUIDsErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "update_project_uids", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Project path is required" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "update_project_uids", map[string]any{
			"projectPath": "../escape",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid project path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "update_project_uids", map[string]any{
			"projectPath": root,
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})
}
