package tools

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

const (
	jsonStartMarker = "<<<GODOT_MCP_JSON_START>>>"
	jsonEndMarker   = "<<<GODOT_MCP_JSON_END>>>"
)

func markerJSON(stdout string) (string, bool) {
	startIdx := strings.Index(stdout, jsonStartMarker)
	if startIdx == -1 {
		return "", false
	}
	rest := stdout[startIdx+len(jsonStartMarker):]
	endIdx := strings.Index(rest, jsonEndMarker)
	if endIdx == -1 {
		return "", false
	}
	raw := strings.TrimSpace(rest[:endIdx])
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(raw), "", "  "); err != nil {
		return raw, true
	}
	return buf.String(), true
}

var headlessSolutions = []string{
	"Ensure Godot is installed correctly",
	"Check if the GODOT_PATH environment variable is set correctly",
	"Verify the project path is accessible",
}

// Callers validate arguments, paths, and project/scene existence first; benign
// engine stderr is allowlisted per tool via scriptFailed (see stderr.go).
func runHeadlessOperation(ctx Context, projectPath, operation string, params, fields map[string]any) (any, error) {
	stdout, stderr, exitCode, err := ctx.Run(projectPath, operation, params)
	if err != nil {
		return mcp.ErrorResult(operation+" failed: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr) {
		return mcp.ErrorResult(operation+" failed: "+stderr, headlessSolutions...), nil
	}
	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}
	return statusResult(projectPath, operation+" succeeded.\n\nOutput: "+stdout, fields), nil
}

func statusResult(project, message string, fields map[string]any) mcp.ToolResult {
	value := make(map[string]any, len(fields)+3)
	value["success"] = true
	value["message"] = message
	if project != "" {
		value["projectPath"] = project
	}
	for name, field := range fields {
		value[name] = field
	}
	return mcp.StructuredResult(message, value)
}

// Built from godotconfig.ValueContractTags so the advertised set stays next to
// the decoder.
var godotValueContract = "Engine-only types use a tagged object such as " +
	`{"$type":"Vector2","x":1,"y":2}` + "; recognized $type tags: " +
	strings.Join(godotconfig.ValueContractTags, ", ")

func godotValueSchema(description string) map[string]any {
	types := []string{"null", "boolean", "string", "number", "array", "object"}
	branches := make([]any, 0, len(types))
	for _, name := range types {
		branches = append(branches, map[string]any{"type": name})
	}
	return map[string]any{
		"description": description + " " + godotValueContract,
		"anyOf":       branches,
	}
}

func statusOutputSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": statusProperties(properties),
		"required":   []string{"success", "message"},
	}
}

func payloadOutputSchema(properties map[string]any) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(properties) > 0 {
		required := make([]string, 0, len(properties))
		for name := range properties {
			required = append(required, name)
		}
		sort.Strings(required)
		schema["required"] = required
	}
	return schema
}

// readEnvelopeSchema is the shallow outer contract shared by read and
// read_node: a self-identifying envelope (kind + path, plus nodePath when a node
// was addressed) wrapping an opaque kind-specific data body, alongside the
// status shape reached by the markerless fallback. The data body is deliberately
// not described; the kind discriminator makes it discoverable from the value.
func readEnvelopeSchema(kinds []string, nodeScoped bool) map[string]any {
	payloadProps := map[string]any{
		"kind": map[string]any{"type": "string", "enum": kinds, "description": "Payload discriminator; selects the shape of data."},
		"path": map[string]any{"type": "string", "description": "File path, project-relative."},
		"data": godotValueSchema("Kind-specific payload; its shape is selected by kind."),
	}
	statusProps := map[string]any{
		"kind": map[string]any{"type": "string", "enum": kinds},
		"path": map[string]any{"type": "string"},
	}
	required := []string{"kind", "path", "data"}
	if nodeScoped {
		payloadProps["nodePath"] = map[string]any{"type": "string", "description": "Node path addressed within the scene."}
		statusProps["nodePath"] = map[string]any{"type": "string"}
		required = append(required, "nodePath")
	}
	return map[string]any{
		"anyOf": []any{
			map[string]any{"type": "object", "properties": payloadProps, "required": required},
			statusOutputSchema(statusProps),
		},
	}
}

const envelopeMessageContract = "Human-readable outcome: the success summary on success, or the failure reason on an error"

const (
	outputPathEchoDescription     = "The output path exactly as the caller supplied it"
	resolvedOutputPathDescription = "Absolute host path the export was written to; a relative outputPath resolves against the project root"
)

func statusProperties(properties map[string]any) map[string]any {
	props := make(map[string]any, len(properties)+3)
	props["success"] = map[string]any{"type": "boolean"}
	props["message"] = map[string]any{"type": "string", "description": envelopeMessageContract}
	props["projectPath"] = map[string]any{"type": "string", "description": "Absolute host filesystem path to the bound project root"}
	for name, schema := range properties {
		props[name] = schema
	}
	return props
}

func invalidPathResult() mcp.ToolResult {
	return mcp.ErrorResult(
		"Invalid path.",
		`Provide a valid path without ".." or other potentially unsafe characters`,
	)
}

func requiredArgsResult(fields ...string) mcp.ToolResult {
	verb := "are"
	if len(fields) == 1 {
		verb = "is"
	}
	return mcp.ErrorResult(
		joinFieldNames(fields)+" "+verb+" required.",
		"Provide the missing fields",
	)
}

func joinFieldNames(fields []string) string {
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	case 2:
		return fields[0] + " and " + fields[1]
	default:
		return strings.Join(fields[:len(fields)-1], ", ") + ", and " + fields[len(fields)-1]
	}
}

func sceneMissingResult(scenePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Scene file does not exist: "+scenePath,
		"Ensure the scene path is correct",
		"Create and save the scene first, for example through run_script",
	)
}

func notAProjectResult(projectPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Not a valid Godot project: "+projectPath,
		"Ensure the path points to a directory containing a project.godot file",
		"Create a project with create_project if none exists",
	)
}
