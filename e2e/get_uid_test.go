package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetUIDSchema ports the reference's tool-definition expectations for
// get_uid over the real stdio surface.
func TestGetUIDSchema(t *testing.T) {
	tool := findTool(t, connect(t), "get_uid")
	if tool.Description != "Get the UID for a specific file in a Godot project (for Godot 4.4+)" {
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
	if strings.Join(schema.Required, ",") != "projectPath,filePath" {
		t.Errorf("required = %v", schema.Required)
	}
	if len(schema.Properties) != 2 {
		t.Errorf("properties = %v", schema.Properties)
	}
	for _, key := range []string{"projectPath", "filePath"} {
		if schema.Properties[key]["type"] != "string" {
			t.Errorf("%s type = %v", key, schema.Properties[key]["type"])
		}
	}
}

// TestGetUIDTool drives the real get_uid tool against Godot for a file with a
// UID sidecar and one without.
func TestGetUIDTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	root := t.TempDir()
	seedProject(t, root)

	t.Run("reads a UID sidecar", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")
		writeE2EFile(t, filepath.Join(root, "player.gd.uid"), "uid://e2etestuid")

		res := callTool(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		got := firstText(t, res)
		if !strings.HasPrefix(got, "UID for player.gd: ") {
			t.Errorf("text = %q", got)
		}
		if !strings.Contains(got, "uid://e2etestuid") {
			t.Errorf("text = %q, want it to contain uid://e2etestuid", got)
		}
	})

	t.Run("reports a missing UID sidecar", func(t *testing.T) {
		writeE2EFile(t, filepath.Join(root, "orphan.gd"), "extends Node\n")

		res := callTool(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "orphan.gd",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.Contains(got, `"exists":false`) {
			t.Errorf("text = %q, want it to report exists false", got)
		}
	})
}

// TestGetUIDVersionGate checks the Godot 4.4 requirement with a fake engine.
func TestGetUIDVersionGate(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "godot")
	if err := os.WriteFile(fakePath, []byte("#!/bin/sh\necho \"4.3.1.stable\"\nexit 0\n"), 0o755); err != nil { //nolint:gosec // test helper needs an executable script.
		t.Fatalf("write fake godot: %v", err)
	}

	session := connectWithEnv(t, "GODOT_PATH="+fakePath)
	root := t.TempDir()
	seedProject(t, root)
	writeE2EFile(t, filepath.Join(root, "player.gd"), "extends Node\n")

	res := callTool(t, session, "get_uid", map[string]any{
		"projectPath": root,
		"filePath":    "player.gd",
	})
	if !res.IsError {
		t.Fatal("expected an error result")
	}
	want := "UIDs are only supported in Godot 4.4 or later. Current version: 4.3.1.stable"
	if got := firstText(t, res); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}

// TestGetUIDErrors covers the reference's validation and failure messages.
func TestGetUIDErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "get_uid", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Missing required parameters" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "get_uid", map[string]any{
			"projectPath": "../escape",
			"filePath":    "player.gd",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		res := callTool(t, session, "get_uid", map[string]any{
			"projectPath": root,
			"filePath":    "player.gd",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "File does not exist: player.gd" {
			t.Errorf("message = %q", got)
		}
	})
}
