package e2e

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestCreateAnimationSchema pins the create tool's input surface.
func TestCreateAnimationSchema(t *testing.T) {
	schema := requireSchema(t, "create_animation", "Create an Animation resource")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath", "loopMode")
	schema.requireType(t, "length", "number")
	schema.requireType(t, "step", "number")
	schema.requirePropCount(t, 4)
}

// TestModifyAnimationSchema pins the modify tool's input surface.
func TestModifyAnimationSchema(t *testing.T) {
	schema := requireSchema(t, "modify_animation", "Set an Animation resource's length, loop mode, or step")
	schema.requireRequired(t, "resourcePath")
	schema.requireStringProps(t, "resourcePath", "loopMode")
	schema.requireType(t, "length", "number")
	schema.requireType(t, "step", "number")
	schema.requirePropCount(t, 4)
}

// TestAddAnimationTrackSchema pins the add-track tool's input surface.
func TestAddAnimationTrackSchema(t *testing.T) {
	schema := requireSchema(t, "add_animation_track", "Append a track to an Animation resource")
	schema.requireRequired(t, "resourcePath", "trackType")
	schema.requireStringProps(t, "resourcePath", "trackType", "trackPath", "interpolation")
	schema.requireType(t, "keys", "array")
	schema.requirePropCount(t, 5)
}

// TestSetAnimationTrackSchema pins the set-track tool's input surface.
func TestSetAnimationTrackSchema(t *testing.T) {
	schema := requireSchema(t, "set_animation_track", "Set an existing Animation track's path, interpolation, and keys")
	schema.requireRequired(t, "resourcePath", "trackIndex")
	schema.requireStringProps(t, "resourcePath", "trackPath", "interpolation")
	schema.requireType(t, "trackIndex", "integer")
	schema.requireType(t, "keys", "array")
	schema.requirePropCount(t, 5)
}

// TestRemoveAnimationTrackSchema pins the remove-track tool's input surface.
func TestRemoveAnimationTrackSchema(t *testing.T) {
	schema := requireSchema(t, "remove_animation_track", "Remove a track from an Animation resource")
	schema.requireRequired(t, "resourcePath", "trackIndex")
	schema.requireStringProps(t, "resourcePath")
	schema.requireType(t, "trackIndex", "integer")
	schema.requirePropCount(t, 2)
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

// createTestAnimation seeds anims/walk.tres through create_animation.
func createTestAnimation(t *testing.T, session *mcp.ClientSession) {
	t.Helper()
	requireToolText(t, session, "create_animation", map[string]any{
		"resourcePath": "anims/walk.tres",
		"length":       2,
	})
}

// TestAnimationTools drives the real animation tools against Godot.
func TestAnimationTools(t *testing.T) {
	t.Run("creates and reads an animation", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

		createText := requireToolText(t, session, "create_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
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

		addText := requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath":  "anims/walk.tres",
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

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
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

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector2", "x": 0, "y": 0}},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Vector2", "x": 1, "y": 1}},
			},
		})
		requireToolText(t, session, "set_animation_track", map[string]any{
			"resourcePath":  "anims/walk.tres",
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

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "value",
			"trackPath":    "Sprite2D:position",
		})
		requireToolText(t, session, "remove_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
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

		requireToolText(t, session, "create_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"length":       1,
		})
		requireToolText(t, session, "modify_animation", map[string]any{
			"resourcePath": "anims/walk.tres",
			"loopMode":     "pingpong",
			"length":       3,
		})

		payload := readAnimationPayload(t, session)
		if payload.Length != 3 || payload.LoopMode != "pingpong" {
			t.Errorf("payload = %+v, want length 3 pingpong", payload)
		}
	})

	t.Run("adds the 3D and blend-shape track family", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "position_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector3", "x": 1, "y": 2, "z": 3}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "rotation_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Quaternion", "x": 0.5, "y": 0.5, "z": 0.5, "w": 0.5}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "scale_3d",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0, "value": map[string]any{"$type": "Vector3", "x": 2, "y": 3, "z": 4}},
			},
		})
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
			"trackType":    "blend_shape",
			"trackPath":    "root",
			"keys": []any{
				map[string]any{"time": 0.25, "value": 0.75},
			},
		})

		payload := readAnimationPayload(t, session)
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
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
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

		payload := readAnimationPayload(t, session)
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
		// The first key supplies handles and the second omits them, so both
		// decode arms of the handle reader run. Godot's bezier insert does not
		// retain handle offsets in supported builds, so only the emitted shape
		// is pinned here.
		for _, key := range []map[string]any{first, second} {
			for _, field := range []string{"inHandle", "outHandle"} {
				if handle, _ := key[field].(map[string]any); handle["$type"] != "Vector2" {
					t.Errorf("%s = %v, want a tagged Vector2", field, key[field])
				}
			}
		}
	})

	t.Run("adds audio and nested animation tracks", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)
		createTestAnimation(t, session)

		// An animation track's value is the name of the animation to play,
		// decoded as a StringName; an audio track's value is a resource
		// reference. Each track carries two keys, one bare and one tagged, so
		// both decode arms run.
		requireToolText(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anims/walk.tres",
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
			"resourcePath": "anims/walk.tres",
			"trackType":    "audio",
			"keys": []any{
				map[string]any{"time": 0, "value": "res://anims/beep.tres", "startOffset": 0.1, "endOffset": 0.5},
				map[string]any{"time": 1, "value": map[string]any{"$type": "Resource", "path": "res://anims/beep.tres"}},
			},
		})

		payload := readAnimationPayload(t, session)
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

// firstKeyValue returns the "value" of a track's first key, failing when the
// track has no keys.
func firstKeyValue(t *testing.T, track map[string]any) any {
	t.Helper()
	values := keyValues(t, track)
	if len(values) == 0 {
		t.Fatalf("track %v has no keys", track)
	}
	return values[0]
}

// keyValues returns the "value" of every key on a track, in key order.
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

// TestAnimationErrors covers the validation and engine failure paths.
func TestAnimationErrors(t *testing.T) {
	t.Run("missing params", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "create_animation", map[string]any{}, "resourcePath is required.")
	})

	t.Run("add without a track type", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "add_animation_track", map[string]any{
			"resourcePath": "anim.tres",
		}, "resourcePath and trackType are required.")
	})

	t.Run("modify without properties", func(t *testing.T) {
		session := connect(t)
		requireToolError(t, session, "modify_animation", map[string]any{
			"resourcePath": "anim.tres",
		}, "The modify_animation tool requires at least one of: length, loopMode, step.")
	})

	t.Run("unknown track type", func(t *testing.T) {
		root := newProject(t)
		session := connectProject(t, root)
		requireGodot(t, session)

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
