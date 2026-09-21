package e2e

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// TestAddNodeSchema ports the reference's tool-definition expectations for
// add_node over the real stdio surface.
func TestAddNodeSchema(t *testing.T) {
	tool := findTool(t, connect(t), "add_node")
	if tool.Description != "Add a node to an existing scene" {
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
	if strings.Join(schema.Required, ",") != "projectPath,scenePath,nodeType,nodeName" {
		t.Errorf("required = %v", schema.Required)
	}
	for _, key := range []string{"projectPath", "scenePath", "parentNodePath", "nodeType", "nodeName", "properties"} {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("missing property %q", key)
		}
	}
	if schema.Properties["properties"]["type"] != "object" {
		t.Errorf("properties type = %v", schema.Properties["properties"]["type"])
	}
}

// TestAddNodeTool drives the real add_node tool against Godot, including the
// snake_case argument aliases.
func TestAddNodeTool(t *testing.T) {
	session := connect(t)
	requireGodot(t, session)

	t.Run("adds a node", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		if got := firstText(t, res); !strings.HasPrefix(got, "Node 'Player' of type 'Sprite2D' added successfully to 'scenes/main.tscn'.") {
			t.Errorf("text = %q", got)
		}
		scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
		if !strings.Contains(scene, `[node name="Player" type="Sprite2D"`) {
			t.Errorf("scene = %q, want a Player Sprite2D node", scene)
		}
	})

	t.Run("snake_case aliases", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)

		res := callTool(t, session, "create_scene", map[string]any{
			"projectPath": root,
			"scenePath":   "scenes/aliased.tscn",
		})
		if res.IsError {
			t.Skipf("create_scene failed, skipping: %s", firstText(t, res))
		}

		res = callTool(t, session, "add_node", map[string]any{
			"project_path":     root,
			"scene_path":       "scenes/aliased.tscn",
			"node_type":        "Sprite2D",
			"node_name":        "Aliased",
			"parent_node_path": "root",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", firstText(t, res))
		}
		scene := readFile(t, filepath.Join(root, "scenes", "aliased.tscn"))
		if !strings.Contains(scene, `[node name="Aliased" type="Sprite2D"`) {
			t.Errorf("scene = %q, want an Aliased Sprite2D node", scene)
		}
	})
}

// TestAddNodeErrors covers the reference's validation and failure messages.
func TestAddNodeErrors(t *testing.T) {
	session := connect(t)

	t.Run("missing params", func(t *testing.T) {
		res := callTool(t, session, "add_node", map[string]any{})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Missing required parameters" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid path", func(t *testing.T) {
		res := callTool(t, session, "add_node", map[string]any{
			"projectPath": "../escape",
			"scenePath":   "main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid path" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("invalid nodeType", func(t *testing.T) {
		res := callTool(t, session, "add_node", map[string]any{
			"projectPath": t.TempDir(),
			"scenePath":   "main.tscn",
			"nodeType":    "res://evil.gd",
			"nodeName":    "Player",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Invalid nodeType" {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("not a project", func(t *testing.T) {
		root := t.TempDir()
		res := callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "main.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Not a valid Godot project: "+root {
			t.Errorf("message = %q", got)
		}
	})

	t.Run("missing scene", func(t *testing.T) {
		root := t.TempDir()
		seedProject(t, root)
		res := callTool(t, session, "add_node", map[string]any{
			"projectPath": root,
			"scenePath":   "missing.tscn",
			"nodeType":    "Sprite2D",
			"nodeName":    "Player",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		if got := firstText(t, res); got != "Scene file does not exist: missing.tscn" {
			t.Errorf("message = %q", got)
		}
	})
}
