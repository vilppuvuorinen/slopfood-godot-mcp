package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// tilesetSourceTypeNames are the tool contract's names for TileSet source
// types, shared with the bridge in godot_operations.gd.
var tilesetSourceTypeNames = []string{"atlas", "scenes"}

func addTilesetSource() mcp.Tool {
	return mcp.Tool{
		Name:        "add_tileset_source",
		Title:       "Add TileSet Source",
		Description: "Add an atlas or scenes-collection source to a TileSet",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "TileSet resource file path (relative to project)",
				},
				"sourceType": map[string]any{
					"type":        "string",
					"enum":        tilesetSourceTypeNames,
					"description": "Source type: atlas or scenes",
				},
				"sourceId": map[string]any{
					"type":        "integer",
					"description": "Source id to assign on add; defaults to the engine's choice",
				},
				"texture":           godotValueSchema("Atlas texture resource path (atlas only)"),
				"margins":           godotValueSchema("Atlas margins as a Vector2i (atlas only)"),
				"separation":        godotValueSchema("Atlas tile separation as a Vector2i (atlas only)"),
				"textureRegionSize": godotValueSchema("Atlas texture region size as a Vector2i (atlas only)"),
				"useTexturePadding": map[string]any{
					"type":        "boolean",
					"description": "Whether the atlas pads its texture (atlas only)",
				},
			},
			"required": []string{"resourcePath", "sourceType"},
		},
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"sourceType":   map[string]any{"type": "string"},
		}),
		Handler: handleAddTilesetSource,
	}
}

type addTilesetSourceArgs struct {
	ResourcePath      string `json:"resourcePath"`
	SourceType        string `json:"sourceType"`
	SourceID          *int   `json:"sourceId"`
	Texture           any    `json:"texture"`
	Margins           any    `json:"margins"`
	Separation        any    `json:"separation"`
	TextureRegionSize any    `json:"textureRegionSize"`
	UseTexturePadding *bool  `json:"useTexturePadding"`
}

func handleAddTilesetSource(ctx context.Context, raw json.RawMessage) (any, error) {
	var args addTilesetSourceArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.SourceType == "" {
		return requiredArgsResult("resourcePath", "sourceType"), nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       "addSource",
		"sourceType":   args.SourceType,
	}
	addTilesetSourceFields(params, args.SourceID, args.Texture, args.Margins, args.Separation, args.TextureRegionSize, args.UseTexturePadding)

	return runHeadlessOperation(ctx, project, "manage_tileset", params, map[string]any{
		"resourcePath": args.ResourcePath,
		"sourceType":   args.SourceType,
	})
}

// addTilesetSourceFields copies the source fields onto params when present.
func addTilesetSourceFields(params map[string]any, sourceID *int, texture, margins, separation, textureRegionSize any, useTexturePadding *bool) {
	if sourceID != nil {
		params["sourceId"] = *sourceID
	}
	if texture != nil {
		params["texture"] = texture
	}
	if margins != nil {
		params["margins"] = margins
	}
	if separation != nil {
		params["separation"] = separation
	}
	if textureRegionSize != nil {
		params["textureRegionSize"] = textureRegionSize
	}
	if useTexturePadding != nil {
		params["useTexturePadding"] = *useTexturePadding
	}
}
