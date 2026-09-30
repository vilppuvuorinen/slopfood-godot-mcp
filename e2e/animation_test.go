package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testAnimationPayload struct {
	Path     string `json:"path"`
	Length   float64
	LoopMode string           `json:"loopMode"`
	Step     float64          `json:"step"`
	Tracks   []map[string]any `json:"tracks"`
}

func readAnimationPayload(t *testing.T, session *mcp.ClientSession, resourcePath string) testAnimationPayload {
	t.Helper()
	text := requireToolText(t, session, "read_animation", map[string]any{"resourcePath": resourcePath})
	var payload testAnimationPayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal animation payload: %v\n%s", err, text)
	}
	return payload
}

func createTestAnimation(t *testing.T, session *mcp.ClientSession, resourcePath string) {
	t.Helper()
	requireToolText(t, session, "create_animation", map[string]any{
		"resourcePath": resourcePath,
		"length":       2,
	})
}

func TestAnimationTools(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("creates and reads an animation", func(t *testing.T) {
		const path = "anims/create.tres"

		createText := requireToolText(t, session, "create_animation", map[string]any{
			"resourcePath": path,
			"length":       2,
			"loopMode":     "linear",
			"step":         0.05,
		})
		requirePrefix(t, createText, "manage_animation succeeded.")

		requireContains(t, readFile(t, filepath.Join(root, "anims", "create.tres")), `[gd_resource type="Animation"`)

		payload := readAnimationPayload(t, session, path)
		if payload.Path != path || payload.Length != 2 || payload.LoopMode != "linear" {
			t.Errorf("payload = %+v", payload)
		}
		if len(payload.Tracks) != 0 {
			t.Errorf("tracks = %v, want none", payload.Tracks)
		}
	})

	t.Run("adds a value track and reads it back", func(t *testing.T) {
		const path = "anims/value_track.tres"
		createTestAnimation(t, session, path)

		addText := requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath":  path,
			"trackType":     "value",
			"trackPath":     "Sprite2D:position",
			"interpolation": "cubic",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 0, "y": 0}},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Vector2", "x": 10, "y": 20}},
			},
		})
		requirePrefix(t, addText, "manage_animation succeeded.")

		payload := readAnimationPayload(t, session, path)
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
		const path = "anims/method_track.tres"
		createTestAnimation(t, session, path)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "method",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0.5, "value": map[string]any{"method": "do_thing", "args": []any{1, "two"}}},
			},
		})

		payload := readAnimationPayload(t, session, path)
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
		const path = "anims/set_track.tres"
		createTestAnimation(t, session, path)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 0, "y": 0}},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Vector2", "x": 1, "y": 1}},
			},
		})
		requireToolText(t, session, "set_animation_track", map[string]any{
			"resourcePath":  path,
			"trackIndex":    0,
			"interpolation": "linear",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 5, "y": 5}},
			},
		})

		payload := readAnimationPayload(t, session, path)
		keys, _ := payload.Tracks[0]["keys"].([]any)
		if len(keys) != 1 {
			t.Fatalf("keys = %v, want the replacement set", keys)
		}
	})

	t.Run("removes a track", func(t *testing.T) {
		const path = "anims/remove_track.tres"
		createTestAnimation(t, session, path)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
		})
		requireToolText(t, session, "remove_animation_track", map[string]any{
			"resourcePath": path,
			"trackIndex":   0,
		})

		payload := readAnimationPayload(t, session, path)
		if len(payload.Tracks) != 0 {
			t.Errorf("tracks = %v, want none after removal", payload.Tracks)
		}
	})

	t.Run("modifies animation properties", func(t *testing.T) {
		const path = "anims/modify.tres"

		requireToolText(t, session, "create_animation", map[string]any{
			"resourcePath": path,
			"length":       1,
		})
		requireToolText(t, session, "modify_animation", map[string]any{
			"resourcePath": path,
			"loopMode":     "pingpong",
			"length":       3,
		})

		payload := readAnimationPayload(t, session, path)
		if payload.Length != 3 || payload.LoopMode != "pingpong" {
			t.Errorf("payload = %+v, want length 3 pingpong", payload)
		}
	})

	t.Run("adds the 3D and blend-shape track family", func(t *testing.T) {
		const path = "anims/track_family.tres"
		createTestAnimation(t, session, path)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "position_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector3", "x": 1, "y": 2, "z": 3}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "rotation_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Quaternion", "x": 0.5, "y": 0.5, "z": 0.5, "w": 0.5}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "scale_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector3", "x": 2, "y": 3, "z": 4}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "blend_shape",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0.25, "value": 0.75},
			},
		})

		payload := readAnimationPayload(t, session, path)
		wantTypes := []string{"position_3d", "rotation_3d", "scale_3d", "blend_shape"}
		if len(payload.Tracks) != len(wantTypes) {
			t.Fatalf("tracks = %v, want %d", payload.Tracks, len(wantTypes))
		}
		for i, want := range wantTypes {
			if got := payload.Tracks[i]["type"]; got != want {
				t.Errorf("track %d type = %v, want %q", i, got, want)
			}
		}

		position := godotMap(t, firstKeyValue(t, payload.Tracks[0]))
		if position["$type"] != "Vector3" || position["x"] != float64(1) || position["y"] != float64(2) || position["z"] != float64(3) {
			t.Errorf("position key = %v", position)
		}
		rotation := godotMap(t, firstKeyValue(t, payload.Tracks[1]))
		if rotation["$type"] != "Quaternion" || rotation["w"] != float64(0.5) {
			t.Errorf("rotation key = %v", rotation)
		}
		scale := godotMap(t, firstKeyValue(t, payload.Tracks[2]))
		if scale["$type"] != "Vector3" || scale["x"] != float64(2) {
			t.Errorf("scale key = %v", scale)
		}
		if got := firstKeyValue(t, payload.Tracks[3]); got != float64(0.75) {
			t.Errorf("blend shape key = %v, want 0.75", got)
		}
	})

	t.Run("adds a bezier track with and without handles", func(t *testing.T) {
		const path = "anims/bezier.tres"
		createTestAnimation(t, session, path)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "bezier",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{
					"time":      0,
					"value":     1.5,
					"inHandle":  map[string]any{"$type": "Vector2", "x": 0.25, "y": 0},
					"outHandle": map[string]any{"$type": "Vector2", "x": -0.25, "y": 0},
				},
				// No handles: the bridge must default both to zero.
				map[string]any{"time": 1, "value": 2.5},
			},
		})

		payload := readAnimationPayload(t, session, path)
		if len(payload.Tracks) != 1 || payload.Tracks[0]["type"] != "bezier" {
			t.Fatalf("tracks = %v, want one bezier", payload.Tracks)
		}
		keys, _ := payload.Tracks[0]["keys"].([]any)
		if len(keys) != 2 {
			t.Fatalf("keys = %v, want two", keys)
		}
		first, _ := keys[0].(map[string]any)
		if first["value"] != float64(1.5) {
			t.Errorf("first bezier value = %v, want 1.5", first["value"])
		}
		second, _ := keys[1].(map[string]any)
		if second["value"] != float64(2.5) {
			t.Errorf("second bezier value = %v, want 2.5", second["value"])
		}
		// Godot's bezier insert does not retain handle offsets in supported builds, so only the emitted shape is pinned.
		for _, key := range []map[string]any{first, second} {
			for _, field := range []string{"inHandle", "outHandle"} {
				if handle, _ := key[field].(map[string]any); handle["$type"] != "Vector2" {
					t.Errorf("%s = %v, want a tagged Vector2", field, key[field])
				}
			}
		}
	})

	t.Run("adds audio and nested animation tracks", func(t *testing.T) {
		const path = "anims/audio.tres"
		createTestAnimation(t, session, path)

		// Animation values decode as StringName and audio values as resource references; each track carries a bare and a tagged key so both arms run.
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "animation",
			"keys": []any{
				map[string]any{"time": 0, "value": "idle"},
				map[string]any{"time": 1, "value": map[string]any{"$type": "StringName", "value": "idle"}},
			},
		})
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "anims/beep.tres",
			"resourceType": "AudioStreamWAV",
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": path,
			"trackType":    "audio",
			"keys": []any{
				map[string]any{"time": 0, "value": "res://anims/beep.tres", "startOffset": 0.1, "endOffset": 0.5},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Resource", "path": "res://anims/beep.tres"}},
			},
		})

		payload := readAnimationPayload(t, session, path)
		if len(payload.Tracks) != 2 {
			t.Fatalf("tracks = %v, want two", payload.Tracks)
		}
		if payload.Tracks[0]["type"] != "animation" || payload.Tracks[1]["type"] != "audio" {
			t.Fatalf("track types = %v, %v", payload.Tracks[0]["type"], payload.Tracks[1]["type"])
		}
		for _, value := range keyValues(t, payload.Tracks[0]) {
			name, ok := value.(map[string]any)
			if !ok || name["$type"] != "StringName" || name["value"] != "idle" {
				t.Errorf("animation key value = %v, want StringName idle", value)
			}
		}
		for _, value := range keyValues(t, payload.Tracks[1]) {
			key, ok := value.(map[string]any)
			if !ok {
				t.Errorf("audio key value = %v, want an object", value)
				continue
			}
			stream, ok := key["stream"].(map[string]any)
			if !ok || stream["$type"] != "Resource" || stream["path"] != "res://anims/beep.tres" {
				t.Errorf("audio key stream = %v, want a Resource at res://anims/beep.tres", key["stream"])
			}
		}
	})
}

func firstKeyValue(t *testing.T, track map[string]any) any {
	t.Helper()
	values := keyValues(t, track)
	if len(values) == 0 {
		t.Fatalf("track %v has no keys", track)
	}
	return values[0]
}

func keyValues(t *testing.T, track map[string]any) []any {
	t.Helper()
	keys, _ := track["keys"].([]any)
	out := make([]any, 0, len(keys))
	for _, item := range keys {
		key, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("key = %#v", item)
		}
		out = append(out, key["value"])
	}
	return out
}

func TestAnimationErrors(t *testing.T) {
	t.Parallel()
	validationSession := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, validationSession, "create_animation", map[string]any{}, "resourcePath is required.")
	})

	t.Run("add without a track type", func(t *testing.T) {
		requireToolError(t, validationSession, "add_animation_track", map[string]any{
			"resourcePath": "anim.tres",
		}, "resourcePath and trackType are required.")
	})

	t.Run("modify without properties", func(t *testing.T) {
		requireToolError(t, validationSession, "modify_animation", map[string]any{
			"resourcePath": "anim.tres",
		}, "The modify_animation tool requires at least one of: length, loopMode, step.")
	})

	root := newProject(t)
	session := connectProject(t, root)

	t.Run("unknown track type", func(t *testing.T) {
		res := callTool(t, session, "create_animation", map[string]any{
			"resourcePath": "anim.tres",
		})
		if res.IsError {
			t.Fatalf("create failed: %s", firstText(t, res))
		}
		res = callTool(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anim.tres",
			"trackType":    "bogus",
		})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "unknown trackType: bogus")
	})

	t.Run("missing animation", func(t *testing.T) {
		res := callTool(t, session, "read_animation", map[string]any{"resourcePath": "anims/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "Animation resource not found")
	})

	t.Run("not an animation resource", func(t *testing.T) {
		requireToolText(t, session, "create_resource", map[string]any{
			"resourcePath": "resources/mat.tres",
			"resourceType": "StandardMaterial3D",
		})
		res := callTool(t, session, "read_animation", map[string]any{"resourcePath": "resources/mat.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "Not an Animation resource")
	})
}
