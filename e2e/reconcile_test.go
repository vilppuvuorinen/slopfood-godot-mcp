package e2e

import (
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
