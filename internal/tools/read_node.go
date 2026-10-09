package tools

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

var readNodeTypes = []string{"scene", "tilemap"}

func (t *toolset) readNode() mcp.Tool {
	return mcp.Tool{
		Name:         "read_node",
		Title:        "Read Node",
		Description:  "Read a node subtree from a scene, or the cells of a TileMapLayer node; use read to read whole files. nodePath selects the tilemap layer, and depth applies to scenes",
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		OutputSchema: readEnvelopeSchema(readNodeTypes, true),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        readNodeTypes,
					"description": "Kind of node read",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Scene file path (relative to the project root, or a res:// path)",
				},
				"nodePath": map[string]any{
					"type":        "string",
					"description": `Path to the node within the scene, relative to the scene root (e.g., "Player" or "Player/Sprite2D")`,
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the node; 0 returns the node without children, omitted means no limit; scene only",
				},
			},
			"required": []string{"type", "path", "nodePath"},
		},
		Handler: t.handleReadNode,
	}
}

type readNodeArgs struct {
	Type     string `json:"type"`
	Path     string `json:"path"`
	NodePath string `json:"nodePath"`
	Depth    *int   `json:"depth"`
}

func (t *toolset) handleReadNode(ctx context.Context, raw json.RawMessage) (any, error) {
	// Presence, not value: an explicit null and an absent key both nil the
	// typed depth field, so depth-on-tilemap is detected from the raw map.
	args, present, err := decodeArgs[readNodeArgs](raw)
	if err != nil {
		return nil, err
	}
	project := t.project
	if result, missing := requireFields(args, "type"); missing {
		return result, nil
	}
	if !slices.Contains(readNodeTypes, args.Type) {
		return mcp.ErrorResult(
			"Invalid type: "+args.Type+".",
			"type must be one of "+strings.Join(readNodeTypes, ", "),
		), nil
	}
	if result, missing := requireFields(args, "path"); missing {
		return result, nil
	}
	if result, missing := requireFields(args, "nodePath"); missing {
		return result, nil
	}
	if _, ok := present["depth"]; ok && args.Type == "tilemap" {
		return mcp.ErrorResult(
			"depth applies only to type=scene.",
			"Omit depth for this type, or use type=scene",
		), nil
	}
	if args.Type == "scene" {
		if args.Depth != nil && *args.Depth < 0 {
			return mcp.ErrorResult(
				"depth must be zero or greater.",
				"Omit depth to return the whole subtree",
			), nil
		}
		return t.readSceneThrough(ctx, args.Path, args.NodePath, args.Depth)
	}

	if result, ok := ensureReadableScene(project, args.Path); !ok {
		return result, nil
	}
	params := map[string]any{
		"scenePath": args.Path,
		"nodePath":  args.NodePath,
	}
	fields := map[string]any{
		"kind":     args.Type,
		"path":     args.Path,
		"nodePath": args.NodePath,
	}
	return t.runHeadlessOperation(ctx, "read_tilemap", params, fields)
}
