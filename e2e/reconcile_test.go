package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAttachScriptReconcilesScriptSidecar(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node2D")

	scriptPath := filepath.Join(root, "scripts", "player.gd")
	writeE2EFile(t, scriptPath, "extends Node2D\n")
	if _, err := os.Stat(scriptPath + ".uid"); !os.IsNotExist(err) {
		t.Fatalf("player.gd.uid exists before attach_script: %v", err)
	}

	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/player.gd",
	})

	uid, err := os.ReadFile(scriptPath + ".uid")
	if err != nil {
		t.Fatalf("read script .uid after attach_script: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(uid)), "uid://") {
		t.Errorf("script .uid = %q, want a uid:// value", strings.TrimSpace(string(uid)))
	}
}

func TestAttachScriptRegistersClassName(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node2D")

	writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "class_name Player\nextends Node2D\n")

	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/player.gd",
	})

	requireToolText(t, session, "add_node", map[string]any{
		"scenePath": "scenes/main.tscn",
		"nodeType":  "Player",
		"nodeName":  "Hero",
	})

	text := requireToolText(t, session, "read_scene", map[string]any{"scenePath": "scenes/main.tscn"})
	var tree sceneNode
	if err := json.Unmarshal([]byte(text), &tree); err != nil {
		t.Fatalf("unmarshal %q: %v", text, err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("children = %+v", tree.Children)
	}
	if got := tree.Children[0]; got.Name != "Hero" || got.Script != "scripts/player.gd" {
		t.Errorf("child = %+v, want Hero with the reconciled global class script", got)
	}
}
