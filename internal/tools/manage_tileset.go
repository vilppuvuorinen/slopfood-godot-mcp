package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// tilesetShapeNames, tilesetLayoutNames, tilesetOffsetAxisNames, and
// tilesetSourceTypeNames are the tool contract's names for the TileSet engine
// enums, shared with the bridge in godot_operations.gd.
var (
	tilesetShapeNames      = []string{"square", "isometric", "half_offset_square", "hexagon"}
	tilesetLayoutNames     = []string{"stacked", "stacked_offset", "stairs_right", "stairs_down", "diamond_right", "diamond_down"}
	tilesetOffsetAxisNames = []string{"horizontal", "vertical"}
	tilesetSourceTypeNames = []string{"atlas", "scenes"}
)

func manageTileset() mcp.Tool {
	return mcp.Tool{
		Name:        "manage_tileset",
		Title:       "Manage TileSet",
		Description: "Create or modify TileSet resources and their sources and tiles",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: mcp.Bool(true), IdempotentHint: mcp.Bool(false), OpenWorldHint: mcp.Bool(false)},
		InputSchema: actionInputSchema(
			map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": "TileSet resource file path (relative to project)",
				},
				"action": map[string]any{
					"type":        "string",
					"enum":        []string{"create", "modify", "addSource", "removeSource", "addTile", "removeTile", "addAlternative", "removeAlternative", "setTileData"},
					"description": "Action: create/modify a tileset, add/remove a source or tile, manage alternatives, or set tile data",
				},
				"tileSize": godotValueSchema("Tile size in pixels as a Vector2i (create, modify)"),
				"tileShape": map[string]any{
					"type":        "string",
					"enum":        tilesetShapeNames,
					"description": "Tile shape (create, modify)",
				},
				"tileLayout": map[string]any{
					"type":        "string",
					"enum":        tilesetLayoutNames,
					"description": "Tile layout (create, modify)",
				},
				"tileOffsetAxis": map[string]any{
					"type":        "string",
					"enum":        tilesetOffsetAxisNames,
					"description": "Hexagon offset axis (create, modify)",
				},
				"uvClipping": map[string]any{
					"type":        "boolean",
					"description": "Whether UVs are clipped to the tile region (create, modify)",
				},
				"sourceType": map[string]any{
					"type":        "string",
					"enum":        tilesetSourceTypeNames,
					"description": "Source type: atlas or scenes (addSource)",
				},
				"sourceId": map[string]any{
					"type":        "integer",
					"description": "Source id to address or to assign on add (addSource, removeSource, addTile, removeTile, addAlternative, removeAlternative, setTileData)",
				},
				"texture":           godotValueSchema("Atlas texture resource path (addSource, atlas only)"),
				"margins":           godotValueSchema("Atlas margins as a Vector2i (addSource, atlas only)"),
				"separation":        godotValueSchema("Atlas tile separation as a Vector2i (addSource, atlas only)"),
				"textureRegionSize": godotValueSchema("Atlas texture region size as a Vector2i (addSource, atlas only)"),
				"useTexturePadding": map[string]any{
					"type":        "boolean",
					"description": "Whether the atlas pads its texture (addSource, atlas only)",
				},
				"atlasCoords": godotValueSchema("Tile atlas coordinates as a Vector2i (addTile, removeTile, addAlternative, removeAlternative, setTileData; atlas only)"),
				"sizeInAtlas": godotValueSchema("Tile size in atlas cells as a Vector2i (addTile; defaults to 1x1)"),
				"scenePath": map[string]any{
					"type":        "string",
					"description": "PackedScene path to place as a scene tile (addTile, scenes source only)",
				},
				"sceneTileId": map[string]any{
					"type":        "integer",
					"description": "Scene tile id to assign or remove (addTile, removeTile; scenes source only)",
				},
				"alternativeId": map[string]any{
					"type":        "integer",
					"description": "Alternative tile id to assign or remove (addAlternative, removeAlternative, setTileData)",
				},
				"displayPlaceholder": map[string]any{
					"type":        "boolean",
					"description": "Whether a scene tile shows a placeholder in the editor (addTile, scenes source only)",
				},
				"properties": propertiesSchema("TileData properties to set, as name to godotValue pairs (setTileData)"),
			},
			[]string{"resourcePath", "action"},
			[]actionVariant{
				{Action: "create"},
				{Action: "modify", AnyOf: []string{"tileSize", "tileShape", "tileLayout", "tileOffsetAxis", "uvClipping"}},
				{Action: "addSource", Required: []string{"sourceType"}},
				{Action: "removeSource", Required: []string{"sourceId"}},
				{Action: "addTile", Required: []string{"sourceId"}},
				{Action: "removeTile", Required: []string{"sourceId"}},
				{Action: "addAlternative", Required: []string{"sourceId", "atlasCoords"}},
				{Action: "removeAlternative", Required: []string{"sourceId", "atlasCoords", "alternativeId"}},
				{Action: "setTileData", Required: []string{"sourceId", "atlasCoords"}},
			},
		),
		OutputSchema: statusOutputSchema(map[string]any{
			"resourcePath": map[string]any{"type": "string"},
			"action":       map[string]any{"type": "string"},
		}),
		Handler: handleManageTileset,
	}
}

type manageTilesetArgs struct {
	ResourcePath       string         `json:"resourcePath"`
	Action             string         `json:"action"`
	TileSize           any            `json:"tileSize"`
	TileShape          string         `json:"tileShape"`
	TileLayout         string         `json:"tileLayout"`
	TileOffsetAxis     string         `json:"tileOffsetAxis"`
	UVClipping         *bool          `json:"uvClipping"`
	SourceType         string         `json:"sourceType"`
	SourceID           *int           `json:"sourceId"`
	Texture            any            `json:"texture"`
	Margins            any            `json:"margins"`
	Separation         any            `json:"separation"`
	TextureRegionSize  any            `json:"textureRegionSize"`
	UseTexturePadding  *bool          `json:"useTexturePadding"`
	AtlasCoords        any            `json:"atlasCoords"`
	SizeInAtlas        any            `json:"sizeInAtlas"`
	ScenePath          string         `json:"scenePath"`
	SceneTileID        *int           `json:"sceneTileId"`
	AlternativeID      *int           `json:"alternativeId"`
	DisplayPlaceholder *bool          `json:"displayPlaceholder"`
	Properties         map[string]any `json:"properties"`
}

func handleManageTileset(ctx context.Context, raw json.RawMessage) (any, error) {
	var args manageTilesetArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" || args.Action == "" {
		return requiredArgsResult("resourcePath", "action"), nil
	}
	if result, ok := validateTilesetAction(args); !ok {
		return result, nil
	}
	if !validPath(project) || !validPath(args.ResourcePath) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	params := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	fields := map[string]any{
		"resourcePath": args.ResourcePath,
		"action":       args.Action,
	}
	addTilesetProperties(params, args)
	addTilesetSourceFields(params, args)
	addTilesetTileFields(params, args)

	return runHeadlessOperation(ctx, project, "manage_tileset", params, fields)
}

// validateTilesetAction enforces each action's own required fields. The input
// schema's oneOf advertises them; this is the server-side guarantee.
func validateTilesetAction(args manageTilesetArgs) (mcp.ToolResult, bool) {
	missing := func(field string) (mcp.ToolResult, bool) {
		return mcp.ErrorResult(field + " is required for the " + args.Action + " action."), false
	}
	switch args.Action {
	case "create":
		return mcp.ToolResult{}, true
	case "modify":
		if args.TileSize == nil && args.TileShape == "" && args.TileLayout == "" && args.TileOffsetAxis == "" && args.UVClipping == nil {
			return effectfulFieldsResult("modify", "tileSize", "tileShape", "tileLayout", "tileOffsetAxis", "uvClipping"), false
		}
	case "addSource":
		if args.SourceType == "" {
			return missing("sourceType")
		}
	case "removeSource", "addTile", "removeTile":
		if args.SourceID == nil {
			return missing("sourceId")
		}
	case "addAlternative", "setTileData":
		if args.SourceID == nil {
			return missing("sourceId")
		}
		if args.AtlasCoords == nil {
			return missing("atlasCoords")
		}
	case "removeAlternative":
		if args.SourceID == nil {
			return missing("sourceId")
		}
		if args.AtlasCoords == nil {
			return missing("atlasCoords")
		}
		if args.AlternativeID == nil {
			return missing("alternativeId")
		}
	default:
		return mcp.ErrorResult("Unknown action: " + args.Action + ". Use create, modify, addSource, removeSource, addTile, removeTile, addAlternative, removeAlternative, or setTileData."), false
	}
	return mcp.ToolResult{}, true
}

// addTilesetProperties copies the tileset-level fields onto params when present.
func addTilesetProperties(params map[string]any, args manageTilesetArgs) {
	if args.TileSize != nil {
		params["tileSize"] = args.TileSize
	}
	if args.TileShape != "" {
		params["tileShape"] = args.TileShape
	}
	if args.TileLayout != "" {
		params["tileLayout"] = args.TileLayout
	}
	if args.TileOffsetAxis != "" {
		params["tileOffsetAxis"] = args.TileOffsetAxis
	}
	if args.UVClipping != nil {
		params["uvClipping"] = *args.UVClipping
	}
}

// addTilesetSourceFields copies the source fields onto params when present.
func addTilesetSourceFields(params map[string]any, args manageTilesetArgs) {
	if args.SourceType != "" {
		params["sourceType"] = args.SourceType
	}
	if args.SourceID != nil {
		params["sourceId"] = *args.SourceID
	}
	if args.Texture != nil {
		params["texture"] = args.Texture
	}
	if args.Margins != nil {
		params["margins"] = args.Margins
	}
	if args.Separation != nil {
		params["separation"] = args.Separation
	}
	if args.TextureRegionSize != nil {
		params["textureRegionSize"] = args.TextureRegionSize
	}
	if args.UseTexturePadding != nil {
		params["useTexturePadding"] = *args.UseTexturePadding
	}
}

// addTilesetTileFields copies the tile fields onto params when present.
func addTilesetTileFields(params map[string]any, args manageTilesetArgs) {
	if args.AtlasCoords != nil {
		params["atlasCoords"] = args.AtlasCoords
	}
	if args.SizeInAtlas != nil {
		params["sizeInAtlas"] = args.SizeInAtlas
	}
	if args.ScenePath != "" {
		params["scenePath"] = args.ScenePath
	}
	if args.SceneTileID != nil {
		params["sceneTileId"] = *args.SceneTileID
	}
	if args.AlternativeID != nil {
		params["alternativeId"] = *args.AlternativeID
	}
	if args.DisplayPlaceholder != nil {
		params["displayPlaceholder"] = *args.DisplayPlaceholder
	}
	if args.Properties != nil {
		params["properties"] = args.Properties
	}
}
