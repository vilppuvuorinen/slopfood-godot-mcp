package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestManageAnimationSchema pins the action tool's input surface.
func TestManageAnimationSchema(t *testing.T) {
	schema := requireSchema(t, "manage_animation", "Create or modify Animation resources and their tracks")
	schema.requireRequired(t, "resourcePath", "action")
	schema.requireStringProps(t, "resourcePath", "action", "loopMode", "trackType", "trackPath", "interpolation")
	schema.requireType(t, "trackIndex", "integer")
	schema.requireType(t, "length", "number")
	schema.requireType(t, "step", "number")
	schema.requireType(t, "keys", "array")
	schema.requirePropCount(t, 10)
}

// TestReadAnimationSchema pins the read-only animation tool's input surface.
func TestReadAnimationSchema(t *testing.T) {
	schema := requireSchema(t, "read_animation", "Read Animation resources and their tracks")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath")
	schema.requirePropCount(t, 1)
}

// testAnimationPayload is the subset of read_animation's payload the tests
// assert against.
type testAnimationPayload struct {
	Path     string `json:"path"`
	Length   float64
	LoopMode string           `json:"loopMode"`
	Step     float64          `json:"step"`
	Tracks   []map[string]any `json:"tracks"`
}

// readAnimationPayload calls read_animation and decodes its payload.
func readAnimationPayload(t *testing.T, session *mcp.ClientSession) testAnimationPayload {
	t.Helper()
	text := requireToolText(t, session, "read_animation", map[string]any{"resourcePath": "anims/walk.tres"})
	var payload testAnimationPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal animation payload: %v\n%s", err, text)
	}
	return payload
}

// createTestAnimation seeds anims/walk.tres through manage_animation.
func createTestAnimation(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	requireToolText(t, session, "manage_animation", map[string]any{
		"resourcePath": "anims/walk.tres",
		"action":       "create",
		"length":       2,
	})
}

// TestManageAnimationTool drives the real animation tools against Godot.
func TestManageAnimationTool(t *testing.T) {
	t.Run("creates and reads an animation", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		createText := requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "create",
			"length":       2,
			"loopMode":     "linear",
			"step":         0.05,
		})
		requirePrefix(t, createText, "manage_animation succeeded.")

		requireContains(t, readFile(t, filepath.Join(root, "anims", "walk.tres")), `[gd_resource type="Animation"`)

		payload := readAnimationPayload(t, session)
		if payload.Path != "anims/walk.tres" || payload.Length != 2 || payload.LoopMode != "linear" {
			t.Errorf("payload = %+v", payload)
		}
		if len(payload.Tracks) != 0 {
			t.Errorf("tracks = %v, want none", payload.Tracks)
		}
	})

	t.Run("adds a value track and reads it back", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		addText := requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath":  "anims/walk.tres",
			"action":        "add",
			"trackType":     "value",
			"trackPath":     "Sprite2D:position",
			"interpolation": "cubic",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 0, "y": 0}},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Vector2", "x": 10, "y": 20}},
			},
		})
		requirePrefix(t, addText, "manage_animation succeeded.")

		payload := readAnimationPayload(t, session)
		if len(payload.Tracks) != 1 {
			t.Fatalf("tracks = %v, want one", payload.Tracks)
		}
		track := payload.Tracks[0]
		if track["type"] != "value" || track["path"] != "Sprite2D:position" || track["interpolation"] != "cubic" {
			t.Errorf("track = %v", track)
		}
		keys, _ := track["keys"].([]any)
		if len(keys) != 2 {
			t.Fatalf("keys = %v, want two", keys)
		}
		second, _ := keys[1].(map[string]any)
		value, _ := second["value"].(map[string]any)
		if value["$type"] != "Vector2" || value["x"] != float64(10) || value["y"] != float64(20) {
			t.Errorf("second key value = %v", second["value"])
		}
	})

	t.Run("adds a method track", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "add",
			"trackType":    "method",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0.5, "value": map[string]any{"method": "do_thing", "args": []any{1, "two"}}},
			},
		})

		payload := readAnimationPayload(t, session)
		if len(payload.Tracks) != 1 {
			t.Fatalf("tracks = %v, want one", payload.Tracks)
		}
		keys, _ := payload.Tracks[0]["keys"].([]any)
		first, _ := keys[0].(map[string]any)
		value, _ := first["value"].(map[string]any)
		if value["method"] != "do_thing" {
			t.Errorf("method key value = %v", first["value"])
		}
	})

	t.Run("set replaces a track's keys", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "add",
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 0, "y": 0}},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Vector2", "x": 1, "y": 1}},
			},
		})
		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath":  "anims/walk.tres",
			"action":        "set",
			"trackIndex":    0,
			"interpolation": "linear",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 5, "y": 5}},
			},
		})

		payload := readAnimationPayload(t, session)
		keys, _ := payload.Tracks[0]["keys"].([]any)
		if len(keys) != 1 {
			t.Fatalf("keys = %v, want the replacement set", keys)
		}
	})

	t.Run("removes a track", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "add",
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
		})
		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "remove",
			"trackIndex":   0,
		})

		payload := readAnimationPayload(t, session)
		if len(payload.Tracks) != 0 {
			t.Errorf("tracks = %v, want none after removal", payload.Tracks)
		}
	})

	t.Run("modifies animation properties", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "create",
			"length":       1,
		})
		requireToolText(t, session, "manage_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"action":       "modify",
			"loopMode":     "pingpong",
			"length":       3,
		})

		payload := readAnimationPayload(t, session)
		if payload.Length != 3 || payload.LoopMode != "pingpong" {
			t.Errorf("payload = %+v, want length 3 pingpong", payload)
		}
	})
}

// TestManageAnimationErrors covers the validation and engine failure paths.
func TestManageAnimationErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_animation", map[string]any{}, "resourcePath and action are required.")
	})

	t.Run("add without a track type", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "manage_animation", map[string]any{
			"resourcePath": "anim.tres",
			"action":       "add",
		}, "trackType is required for the add action.")
	})

	t.Run("unknown action", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_animation", map[string]any{
			"resourcePath": "anim.tres",
			"action":       "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "Unknown action: bogus")
	})

	t.Run("unknown track type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "manage_animation", map[string]any{
			"resourcePath": "anim.tres",
			"action":       "create",
		})
		if res.IsError {
			t.Fatalf("create failed: %s", firstText(t, res))
		}
		res = callTool(t, session, "manage_animation", map[string]any{
			"resourcePath": "anim.tres",
			"action":       "add",
			"trackType":    "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "unknown trackType: bogus")
	})

	t.Run("missing animation", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		res := callTool(t, session, "read_animation", map[string]any{"resourcePath": "anims/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "Animation resource not found")
	})

	t.Run("not an animation resource", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		requireToolText(t, session, "manage_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"action":       "create",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_animation", map[string]any{"resourcePath": "resources/mat.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "Not an Animation resource")
	})
}
