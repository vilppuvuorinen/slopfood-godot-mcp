package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// importSuffix is the extension Godot gives a resource's import metadata
// sidecar, for example "icon.svg.import".
const importSuffix = ".import"

func readImport() mcp.Tool {
	return mcp.Tool{
		Name:        "read_import",
		Title:       "Read Import Metadata",
		Description: "Read the .import metadata sidecar of an imported resource",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"resourcePath": map[string]any{
					"type":        "string",
					"description": `Imported resource path, relative to the project (for example "icon.svg"); the ".import" path is also accepted`,
				},
			},
			"required": []string{"resourcePath"},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"sourcePath": map[string]any{"type": "string", "description": "Imported resource path, relative to the bound project"},
			"importPath": map[string]any{"type": "string", "description": "Metadata path, relative to the bound project"},
			"importer":   map[string]any{"type": "string", "description": "Importer name from [remap], or an empty string"},
			"type":       map[string]any{"type": "string", "description": "Resource type from [remap], or an empty string"},
			"uid":        map[string]any{"type": "string", "description": "Resource UID from [remap], or an empty string"},
			"path":       map[string]any{"type": "string", "description": "Imported artifact path from [remap], or an empty string"},
			"metadata":   godotValueSchema("The importer's metadata dictionary from [remap]"),
			"sourceFile": map[string]any{"type": "string", "description": "res:// source path from [deps], or an empty string"},
			"destFiles":  map[string]any{"type": "array", "description": "Imported artifact res:// paths from [deps]", "items": map[string]any{"type": "string"}},
			"params": map[string]any{
				"type":                 "object",
				"description":          "Import parameters from [params], with values in the tool value contract",
				"additionalProperties": godotValueSchema("An import parameter value"),
			},
		}),
		Handler: handleReadImport,
	}
}

// importMetadata is read_import's payload.
type importMetadata struct {
	SourcePath string         `json:"sourcePath"`
	ImportPath string         `json:"importPath"`
	Importer   string         `json:"importer"`
	Type       string         `json:"type"`
	UID        string         `json:"uid"`
	Path       string         `json:"path"`
	Metadata   any            `json:"metadata"`
	SourceFile string         `json:"sourceFile"`
	DestFiles  []string       `json:"destFiles"`
	Params     map[string]any `json:"params"`
}

type readImportArgs struct {
	ResourcePath string `json:"resourcePath"`
}

func handleReadImport(ctx context.Context, raw json.RawMessage) (any, error) {
	var args readImportArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if args.ResourcePath == "" {
		return requiredArgsResult("resourcePath"), nil
	}
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	sourcePath, importPath, ok := importPaths(args.ResourcePath)
	if !ok {
		return importPathError(), nil
	}
	full, inside := confinePath(project, importPath)
	if !inside {
		return invalidPathResult(), nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return importMissingResult(importPath), nil
		}
		return mcp.ErrorResult("Failed to read import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	config, err := godotconfig.Parse(string(data))
	if err != nil {
		return mcp.ErrorResult("Failed to read import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	metadata, err := importMetadataValue(config)
	if err != nil {
		return mcp.ErrorResult("Failed to read import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	params, err := importParams(config)
	if err != nil {
		return mcp.ErrorResult("Failed to read import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	payload := importMetadata{
		SourcePath: sourcePath,
		ImportPath: importPath,
		Importer:   configString(config, "remap", "importer"),
		Type:       configString(config, "remap", "type"),
		UID:        configString(config, "remap", "uid"),
		Path:       configString(config, "remap", "path"),
		Metadata:   metadata,
		SourceFile: configString(config, "deps", "source_file"),
		DestFiles:  importDestFiles(config),
		Params:     params,
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to read import metadata: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.JSONResult(string(encoded)), nil
}

// importPaths derives the project-relative source and .import paths from a
// caller path. The ".import" path is accepted directly; otherwise it is the
// source path plus ".import". ok is false when either path is unsafe.
func importPaths(resourcePath string) (sourcePath, importPath string, ok bool) {
	cleaned := strings.TrimSpace(resourcePath)
	cleaned = strings.TrimPrefix(cleaned, resScheme)
	cleaned = strings.TrimPrefix(cleaned, "/")
	cleaned = filepath.ToSlash(cleaned)
	if cleaned == "" || strings.Contains(cleaned, "..") {
		return "", "", false
	}
	if strings.HasSuffix(cleaned, importSuffix) {
		sourcePath = strings.TrimSuffix(cleaned, importSuffix)
		importPath = cleaned
	} else {
		sourcePath = cleaned
		importPath = cleaned + importSuffix
	}
	if sourcePath == "" || importPath == "" {
		return "", "", false
	}
	return sourcePath, importPath, true
}

// importMetadataValue encodes [remap] metadata in the tool value contract,
// yielding nil when the key is absent.
func importMetadataValue(config *godotconfig.Config) (any, error) {
	value, ok := config.Get("remap", "metadata")
	if !ok {
		return nil, nil
	}
	encoded, err := godotconfig.ValueToJSON(value)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}
	return encoded, nil
}

// importParams encodes every [params] entry in the tool value contract. An
// absent section yields an empty (non-nil) map.
func importParams(config *godotconfig.Config) (map[string]any, error) {
	keys := config.SectionKeys("params")
	params := make(map[string]any, len(keys))
	for _, kv := range keys {
		value, err := godotconfig.ValueToJSON(kv.Value)
		if err != nil {
			return nil, fmt.Errorf("encode parameter %q: %w", kv.Key, err)
		}
		params[kv.Key] = value
	}
	return params, nil
}

// importDestFiles returns the [deps] dest_files entries as plain strings. The
// engine has stored this as both a PackedStringArray and a plain Array across
// versions, so both are accepted; anything else yields an empty (non-nil) list.
func importDestFiles(config *godotconfig.Config) []string {
	value, ok := config.Get("deps", "dest_files")
	if !ok {
		return []string{}
	}
	switch typed := value.(type) {
	case godotconfig.Array:
		files := make([]string, 0, len(typed))
		for _, entry := range typed {
			if text, ok := entry.(godotconfig.String); ok {
				files = append(files, string(text))
			}
		}
		return files
	case godotconfig.PackedStringArray:
		return append([]string{}, typed...)
	default:
		return []string{}
	}
}

// importPathError is the shared error for a resourcePath that cannot name an
// import sidecar.
func importPathError() mcp.ToolResult {
	return mcp.ErrorResult(
		`Invalid resource path. Provide a project-relative path such as "icon.svg".`,
	)
}

// importMissingResult is the shared error for a missing .import sidecar.
func importMissingResult(importPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Import metadata not found: "+importPath,
		"Ensure the resource exists and has been imported",
		"Run update_project_uids to import the project and generate .import files",
	)
}
