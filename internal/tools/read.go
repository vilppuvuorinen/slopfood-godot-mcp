package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

var readTypes = []string{"scene", "animation", "tileset", "theme", "resource"}

// The four resource types dispatch straight to the bridge op; only the key is
// remapped (path -> resourcePath).
var readResourceOperations = map[string]string{
	"animation": "read_animation",
	"tileset":   "read_tileset",
	"theme":     "read_theme",
	"resource":  "read_resource",
}

func read() mcp.Tool {
	return mcp.Tool{
		Name:         "read",
		Title:        "Read",
		Description:  "Read a scene, animation, tileset, theme, or binary resource file; use read_node to read a node subtree. depth applies to scenes, propertyFilter to resources",
		Annotations:  &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		OutputSchema: readEnvelopeSchema(readTypes, false),
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]any{
					"type":        "string",
					"enum":        readTypes,
					"description": "Kind of file to read",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File path (relative to the project root, or a res:// path)",
				},
				"depth": map[string]any{
					"type":        "integer",
					"minimum":     0,
					"description": "Optional maximum number of child levels below the scene root; 0 returns the root without children, omitted means no limit; scene only",
				},
				"propertyFilter": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Limit the returned properties to these names (wildcards * and ? allowed); resource only",
				},
			},
			"required": []string{"type", "path"},
		},
		Handler: withContext(handleRead),
	}
}

type readArgs struct {
	Type           string   `json:"type"`
	Path           string   `json:"path"`
	Depth          *int     `json:"depth"`
	PropertyFilter []string `json:"propertyFilter"`
}

func handleRead(ctx Context, raw json.RawMessage) (any, error) {
	var args readArgs
	// Presence, not value: an explicit null and an absent key both nil the
	// typed fields, so mismatched optional args are detected from the raw map.
	var present map[string]json.RawMessage
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
		if err := json.Unmarshal(raw, &present); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.Type == "" {
		return requiredArgsResult("type"), nil
	}
	if !slices.Contains(readTypes, args.Type) {
		return mcp.ErrorResult(
			"Invalid type: "+args.Type+".",
			"type must be one of "+strings.Join(readTypes, ", "),
		), nil
	}
	if args.Path == "" {
		return requiredArgsResult("path"), nil
	}
	if _, ok := present["nodePath"]; ok {
		return mcp.ErrorResult(
			"read does not accept nodePath.",
			"Use read_node to read a node subtree",
		), nil
	}
	if _, ok := present["depth"]; ok && args.Type != "scene" {
		return mcp.ErrorResult(
			"depth applies only to type=scene.",
			"Omit depth for this type, or use type=scene",
		), nil
	}
	if _, ok := present["propertyFilter"]; ok && args.Type != "resource" {
		return mcp.ErrorResult(
			"propertyFilter applies only to type=resource.",
			"Omit propertyFilter for this type, or use type=resource",
		), nil
	}
	if args.Type == "scene" {
		if args.Depth != nil && *args.Depth < 0 {
			return mcp.ErrorResult(
				"depth must be zero or greater.",
				"Omit depth to return the whole subtree",
			), nil
		}
		return readSceneThrough(ctx, project, args.Path, "", args.Depth)
	}

	if _, err := normalizeReadPath(project, args.Path); err != nil {
		return invalidPathResultFor(err), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}
	if args.Type == "resource" && strings.HasSuffix(strings.ToLower(args.Path), ".tres") {
		return mcp.ErrorResult(
			"read(type=resource) reads binary .res resources; "+args.Path+" is a text .tres file.",
			"Read .tres files directly",
		), nil
	}

	params := map[string]any{"resourcePath": args.Path}
	fields := map[string]any{"kind": args.Type, "path": args.Path}
	if args.Type == "resource" && len(args.PropertyFilter) > 0 {
		params["propertyFilter"] = args.PropertyFilter
	}
	return runHeadlessOperation(ctx, project, readResourceOperations[args.Type], params, fields)
}
