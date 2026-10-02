package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Global class registration needs an import after the script is written and before add_node.
func TestAddNodeFromGlobalClass(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "class_name Player\nextends Node2D\n")
	requireToolText(t, session, "rescan_project", map[string]any{})

	requireToolText(t, session, "create_scene", map[string]any{
		"scenePath":    "scenes/main.tscn",
		"rootNodeType": "Node2D",
	})
	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodeType":  "Player",
		"nodeName":  "Hero",
	})

	scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
	requireContains(t, scene, "res://scripts/player.gd")

	text := requireToolText(t, session, "read", map[string]any{"type": "scene", "path": "scenes/main.tscn"})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("children = %+v", tree.Children)
	}
	if got := tree.Children[0]; got.Name != "Hero" || got.Script != "scripts/player.gd" {
		t.Errorf("child = %+v, want Hero with the global class script", got)
	}
}
