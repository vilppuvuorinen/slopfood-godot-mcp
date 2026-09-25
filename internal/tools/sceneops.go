package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotops"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

const (
	jsonStartMarker = "<<<GO_DOT_MCP_JSON_START>>>"
	jsonEndMarker   = "<<<GO_DOT_MCP_JSON_END>>>"
)

// markerJSON returns the JSON payload between jsonStartMarker and
// jsonEndMarker, pretty-printed with two-space indentation, and whether both
// markers were present. Invalid JSON is returned verbatim.
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

// runOperation executes an operations-script operation against a project,
// converting camelCase parameters to snake_case first.
func runOperation(ctx context.Context, projectPath, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return "", "", 0, fmt.Errorf("materialize operations script: %w", err)
	}
	stdout, stderr, exitCode, err = godot.RunOperation(ctx, godot.Path(), projectPath, scriptPath, operation, toSnakeCase(params))
	if err != nil {
		return stdout, stderr, exitCode, fmt.Errorf("run godot operation: %w", err)
	}
	return stdout, stderr, exitCode, nil
}

// headlessSolutions are the shared "Possible solutions" lines for a Godot
// operation that could not run.
var headlessSolutions = []string{
	"Ensure Godot is installed correctly",
	"Check if the GODOT_PATH environment variable is set correctly",
	"Verify the project path is accessible",
}

// runHeadlessOperation runs an operations-script operation and returns the
// reference server's generic result. On success it first attempts structured
// payload extraction via markerJSON, returning the pretty-printed JSON when the
// markers are present; otherwise it falls back to a status result carrying
// "<operation> succeeded.\n\nOutput: <stdout>" plus the given fields. On
// failure it returns "<operation> failed: <detail>" with the shared solutions.
// Callers validate arguments, paths, and project/scene existence first.
//
// allowed lists stderr line patterns the operation may emit without it counting
// as a failure; any other stderr line fails the operation even on exit 0.
//
//nolint:unparam // allowed is the per-tool hook for known-benign stderr; no operation needs one yet.
func runHeadlessOperation(ctx context.Context, projectPath, operation string, params, fields map[string]any, allowed ...*regexp.Regexp) (any, error) {
	stdout, stderr, exitCode, err := runOperation(ctx, projectPath, operation, params)
	if err != nil {
		return mcp.ErrorResult(operation+" failed: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	if scriptFailed(exitCode, stderr, allowed...) {
		return mcp.ErrorResult(operation+" failed: "+stderr, headlessSolutions...), nil
	}
	if payload, ok := markerJSON(stdout); ok {
		return mcp.JSONResult(payload), nil
	}
	return statusResult(operation+" succeeded.\n\nOutput: "+stdout, fields), nil
}

// statusResult builds a successful result whose structured content reports the
// operation outcome: a success flag, the human-readable message, and any
// tool-specific fields.
func statusResult(message string, fields map[string]any) mcp.ToolResult {
	value := make(map[string]any, len(fields)+2)
	value["success"] = true
	value["message"] = message
	for name, field := range fields {
		value[name] = field
	}
	return mcp.StructuredResult(message, value)
}

// godotValueSchema describes one value in the tool value contract for schema
// consumers: JSON-native data, or a tagged object such as
// {"$type":"Vector2","x":1,"y":2} for an engine-only type. It is deliberately
// permissive; the decoder enforces which $type a slot accepts.
func godotValueSchema(description string) map[string]any {
	return map[string]any{"description": description}
}

// propertiesSchema is the input schema shared by every tool that takes a map
// of property name to godotValue.
func propertiesSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"additionalProperties": godotValueSchema("A value in the tool value contract"),
	}
}

// statusOutputSchema returns the object output schema shared by the mutation
// tools. Every successful result carries the status envelope, so success and
// message are both advertised and required alongside any tool-specific
// properties.
func statusOutputSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": statusProperties(properties),
		"required":   []string{"success", "message"},
	}
}

// payloadOutputSchema returns the object output schema for an action that
// returns its payload directly, without the status envelope. Its properties
// are exactly the fields the payload carries.
func payloadOutputSchema(properties map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

// actionOutputSchema returns the object output schema for a tool whose
// structured result depends on the action: a mutation returns the status
// envelope while a read or list returns a payload without it. payloadSchema is
// the full object schema for the payload shape, so a fixed-field payload uses
// payloadOutputSchema and a free-form one can declare additionalProperties.
// Both shapes are advertised with anyOf so a read is never described as
// returning success and message, and no action is described by fields it does
// not return. The merged properties still list every field for clients that do
// not read schema branches.
func actionOutputSchema(status map[string]any, payloadSchema map[string]any) map[string]any {
	properties := statusProperties(status)
	for name, schema := range payloadProperties(payloadSchema) {
		properties[name] = schema
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
		"anyOf": []any{
			statusOutputSchema(status),
			payloadSchema,
		},
	}
}

// payloadProperties returns the named properties of a payload object schema, or
// an empty map for a free-form schema that only declares additionalProperties.
func payloadProperties(schema map[string]any) map[string]any {
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		return map[string]any{}
	}
	return properties
}

// statusProperties builds the property map for the status envelope, starting
// from success and message and adding each tool-specific field.
func statusProperties(properties map[string]any) map[string]any {
	props := make(map[string]any, len(properties)+2)
	props["success"] = map[string]any{"type": "boolean"}
	props["message"] = map[string]any{"type": "string"}
	for name, schema := range properties {
		props[name] = schema
	}
	return props
}

// sceneMissingResult is the shared error for a scene file that does not exist,
// matching the reference server's message and solutions.
func sceneMissingResult(scenePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Scene file does not exist: "+scenePath,
		"Ensure the scene path is correct",
		"Use create_scene to create a new scene first",
	)
}

// notAProjectResult is the shared error for a projectPath without a
// project.godot file, matching the reference server's message and solutions.
func notAProjectResult(projectPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Not a valid Godot project: "+projectPath,
		"Ensure the path points to a directory containing a project.godot file",
		"Use list_projects to find valid Godot projects",
	)
}
