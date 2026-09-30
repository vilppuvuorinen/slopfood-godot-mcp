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
}

func TestAnimationErrors(t *testing.T) {
	t.Parallel()
	validationSession := connect(t)
	t.Run("missing params", func(t *testing.T) {
		requireToolError(t, validationSession, "create_animation", map[string]any{}, "resourcePath is required.")
	})

	t.Run("modify without properties", func(t *testing.T) {
		requireToolError(t, validationSession, "modify_animation", map[string]any{
			"resourcePath": "anim.tres",
		}, "The modify_animation tool requires at least one of: length, loopMode, step.")
	})

	root := newProject(t)
	session := connectProject(t, root)

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
