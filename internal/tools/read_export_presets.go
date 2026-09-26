package tools

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readExportPresets() mcp.Tool {
	return mcp.Tool{
		Name:        "read_export_presets",
		Title:       "Read Export Presets",
		Description: "List export preset configuration",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: readOutputSchema(payloadOutputSchema(map[string]any{
			"presets": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":     map[string]any{"type": "string"},
						"platform": map[string]any{"type": "string"},
					},
				},
			},
		})),
		Handler: handleReadExportPresets,
	}
}

func handleReadExportPresets(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	return listExportPresets(filepath.Join(project, "export_presets.cfg")), nil
}
