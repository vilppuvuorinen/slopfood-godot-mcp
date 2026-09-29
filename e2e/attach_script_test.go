package e2e

import (
	"path/filepath"
	"testing"
)

// TestAttachScriptSchema ports the reference's tool-definition expectations for
// attach_script over the real stdio surface.
func TestAttachScriptSchema(t *testing.T) {
	t.Parallel()
	schema := requireSchema(t, "attach_script", "Attach a GDScript to a scene node (headless)")
	schema.requireRequired(t, "scenePath", "nodePath", "scriptPath")
	schema.requireStringProps(t, "scenePath", "nodePath", "scriptPath")
	schema.requirePropCount(t, 3)
}

// TestAttachScriptTool drives the real attach_script tool against Godot and
// checks the saved scene references the attached script.
func TestAttachScriptTool(t *testing.T) {
	t.Parallel()
	root, session := seedScene(t, "scenes/main.tscn", "Node2D")
	writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node2D\n")

	requireToolText(t, session, "attach_script", map[string]any{
		"scenePath":  "scenes/main.tscn",
		"nodePath":   "root",
		"scriptPath": "scripts/player.gd",
	})

	scene := readFile(t, filepath.Join(root, "scenes", "main.tscn"))
	requireContains(t, scene, "script = ExtResource(", "res://scripts/player.gd")
}

// TestAttachScriptErrors covers the reference's validation and failure messages.
func TestAttachScriptErrors(t *testing.T) {
	t.Parallel()
	// The three engine-backed cases share one warm project and session; each
	// owns its scene and script fixtures.
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing paths", func(t *testing.T) {
		requireToolError(t, session, "attach_script", map[string]any{}, "scenePath, nodePath, and scriptPath are required.")
	})

	t.Run("invalid path", func(t *testing.T) {
		requireToolError(t, session, "attach_script", map[string]any{
			"scenePath":  "../escape",
			"nodePath":   "root",
			"scriptPath": "scripts/player.gd",
		}, "Invalid path.")
	})

	t.Run("not a project", func(t *testing.T) {
		notProject := t.TempDir()
		notProjectSession := connectProject(t, notProject)
		requireToolError(t, notProjectSession, "attach_script", map[string]any{
			"scenePath":  "main.tscn",
			"nodePath":   "root",
			"scriptPath": "scripts/player.gd",
		}, "Not a valid Godot project: "+notProject)
	})

	t.Run("missing scene", func(t *testing.T) {
		requireToolError(t, session, "attach_script", map[string]any{
			"scenePath":  "missing.tscn",
			"nodePath":   "root",
			"scriptPath": "scripts/player.gd",
		}, "Scene file does not exist: missing.tscn")
	})

	t.Run("missing script file", func(t *testing.T) {
		const scene = "scenes/missing_script.tscn"
		createScene(t, session, scene, "Node2D")

		res := callTool(t, session, "attach_script", map[string]any{
			"scenePath":  scene,
			"nodePath":   "root",
			"scriptPath": "scripts/missing.gd",
		})
		if !res.IsError {
			t.Fatal("attach_script: expected an error result")
		}
		requireContains(t, firstText(t, res), "attach_script failed", "Script file does not exist at: res://scripts/missing.gd")
	})

	t.Run("missing node", func(t *testing.T) {
		const scene = "scenes/missing_node.tscn"
		createScene(t, session, scene, "Node2D")
		writeE2EFile(t, filepath.Join(root, "scripts", "player.gd"), "extends Node2D\n")

		res := callTool(t, session, "attach_script", map[string]any{
			"scenePath":  scene,
			"nodePath":   "root/Missing",
			"scriptPath": "scripts/player.gd",
		})
		if !res.IsError {
			t.Fatal("attach_script: expected an error result")
		}
		requireContains(t, firstText(t, res), "attach_script failed", "Node not found: root/Missing")
	})

	t.Run("unloadable script", func(t *testing.T) {
		const scene = "scenes/unloadable_script.tscn"
		createScene(t, session, scene, "Node2D")
		writeE2EFile(t, filepath.Join(root, "scripts", "thing.tres"), resourceTres)

		// A non-.gd resource path resolves but is not a valid script, so the
		// failure surfaces from the engine's set_script call.
		res := callTool(t, session, "attach_script", map[string]any{
			"scenePath":  scene,
			"nodePath":   "root",
			"scriptPath": "scripts/thing.tres",
		})
		if !res.IsError {
			t.Fatal("attach_script: expected an error result")
		}
		requireContains(t, firstText(t, res), "attach_script failed", "Parameter should be null or a reference to a valid script")
	})
}
