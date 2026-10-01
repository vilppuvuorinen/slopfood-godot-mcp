package e2e

import (
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type testThemePayload struct {
	Path             string           `json:"path"`
	DefaultFont      map[string]any   `json:"defaultFont"`
	DefaultFontSize  *float64         `json:"defaultFontSize"`
	DefaultBaseScale *float64         `json:"defaultBaseScale"`
	Types            []map[string]any `json:"types"`
}

func readThemePayload(t *testing.T, session *mcp.ClientSession, resourcePath string) testThemePayload {
	t.Helper()
	text := requireToolText(t, session, "read_theme", map[string]any{"resourcePath": resourcePath})
	var payload testThemePayload
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("unmarshal theme payload: %v\n%s", err, text)
	}
	return payload
}

// SystemFont, GradientTexture2D, and StyleBoxFlat need no import step, unlike a raw font or PNG.
func seedThemeResources(t *testing.T, session *mcp.ClientSession, prefix string) {
	t.Helper()
	for name, resourceType := range map[string]string{
		"font.tres": "SystemFont",
		"icon.tres": "GradientTexture2D",
		"box.tres":  "StyleBoxFlat",
	} {
		seedResource(t, session, prefix+name, resourceType)
	}
}

func TestThemeErrors(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	session := connectProject(t, root)

	t.Run("missing theme", func(t *testing.T) {
		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "themes/missing.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Theme resource not found")
	})

	t.Run("not a theme resource", func(t *testing.T) {
		seedResource(t, session, "resources/mat_theme.tres", "StandardMaterial3D")
		res := callTool(t, session, "read_theme", map[string]any{"resourcePath": "resources/mat_theme.tres"})
		if !res.IsError {
			t.Fatal("expected an error result")
		}
		requireContains(t, firstText(t, res), "manage_theme failed", "Not a Theme resource")
	})
}
