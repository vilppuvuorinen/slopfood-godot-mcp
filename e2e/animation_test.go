package e2e

import (
	"encoding/json"
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

func TestAnimationErrors(t *testing.T) {
	t.Parallel()
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
		seedResource(t, session, "resources/mat.tres", "StandardMaterial3D")
		res := callTool(t, session, "read_animation", map[string]any{"resourcePath": "resources/mat.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_animation failed", "Not an Animation resource")
	})
}
