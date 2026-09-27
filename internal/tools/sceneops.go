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
// It treats every stderr line as a failure. When an operation emits benign
// engine noise it cannot prevent, add that pattern to a per-tool allowlist and
// pass it to scriptFailed (see stderr.go) together with a test, rather than
// carrying an unused hook here.
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

// statusResult builds a successful result whose structured content reports the
// operation outcome: a success flag, the human-readable message, the bound
// project, and any tool-specific fields.
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

// godotValueContract is the one description of the tool value contract shared
// by every value slot: JSON-native data, or an engine-only type as a tagged
// object whose "$type" is one of the tags godotconfig.ValueContractTags lists.
// Building it from that list keeps the advertised set next to the decoder.
var godotValueContract = "Engine-only types use a tagged object such as " +
	`{"$type":"Vector2","x":1,"y":2}` + "; recognized $type tags: " +
	strings.Join(godotconfig.ValueContractTags, ", ") + "."

// godotValueSchema describes one value in the tool value contract for schema
// consumers: JSON-native data, or a tagged object such as
// {"$type":"Vector2","x":1,"y":2} for an engine-only type. anyOf names the
// JSON-native types as discrete branches with a single string type each; the
// top level stays untyped so stricter clients never see an array-valued type.
// Every slot appends the shared tag-enumerating contract to its own description,
// so a client learns the whole tag set from any slot. It is deliberately
// permissive; the decoder enforces which $type a slot accepts.
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

// propertiesSchema is the input schema shared by every tool that takes a map
// of property name to godotValue. It requires at least one entry, since an
// empty map applies nothing and a tool whose only effect is a property map
// would otherwise accept a no-op call.
func propertiesSchema(description string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"description":          description,
		"minProperties":        1,
		"additionalProperties": godotValueSchema("A value in the tool value contract"),
	}
}

// withResourcePath adds the shared resourcePath property to a resource tool's
// properties map. Resource tools are bound to a project, so the path is always
// relative to that project.
func withResourcePath(properties map[string]any) map[string]any {
	properties["resourcePath"] = map[string]any{
		"type":        "string",
		"description": "Resource file path (relative to project)",
	}
	return properties
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

// payloadOutputSchema returns the object output schema for a read or list tool
// that returns its payload directly, without the status envelope. Its
// properties are exactly the fields the payload carries, and every declared
// field is required, so a read or list result pins its shape instead of leaving
// it unconstrained. A free-form payload has no named properties and so requires
// nothing.
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

// envelopeMessageContract is the one description of the status envelope's
// message field, shared by the success and error schemas so a client reads the
// same meaning either way: a summary on success, the reason on an error. The
// solutions field is reserved for errors and described only there.
const envelopeMessageContract = "Human-readable outcome: the success summary on success, or the failure reason on an error."

// outputPathEchoDescription and resolvedOutputPathDescription describe an
// export's output path pair, shared by export_project and export_mesh_library.
// The echo field repeats the caller's own argument so the result maps back to
// the request, while the resolved field reports where the file actually landed.
const (
	outputPathEchoDescription     = "The output path exactly as the caller supplied it."
	resolvedOutputPathDescription = "Absolute host path the export was written to; a relative outputPath resolves against the project root."
)

// statusProperties builds the property map for the status envelope, starting
// from success and message and adding each tool-specific field.
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

// invalidPathResult is the shared error for an unusable path. Every path guard
// reports it, so a caller sees the same message and solution whichever tool
// rejected the path.
func invalidPathResult() mcp.ToolResult {
	return mcp.ErrorResult(
		"Invalid path.",
		`Provide a valid path without ".." or other potentially unsafe characters`,
	)
}

// requiredArgsResult is the shared error for missing required arguments. It
// names the fields the way the tool's inputSchema does, so the message and the
// advertised contract agree.
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

// requiredOneOfResult is the shared error for a tool that carries none of the
// fields that would give it an effect. Without it, a payload-less modify would
// reload and re-save a resource without changing it, a no-op the schema also
// rejects.
func requiredOneOfResult(tool string, fields ...string) mcp.ToolResult {
	return mcp.ErrorResult(
		"The "+tool+" tool requires at least one of: "+strings.Join(fields, ", ")+".",
		"Provide a value to change",
	)
}

// joinFieldNames renders a field list as English: "a", "a and b", or
// "a, b, and c".
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

// sceneMissingResult is the shared error for a scene file that does not exist,
// matching the reference server's message and solutions.
func sceneMissingResult(scenePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Scene file does not exist: "+scenePath,
		"Ensure the scene path is correct",
		"Use create_scene to create a new scene first",
	)
}

// baseSceneMissingResult is the error for a create_scene baseScenePath that does
// not resolve to a file, so the caller learns which of the two root sources is
// missing.
func baseSceneMissingResult(baseScenePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Base scene does not exist: "+baseScenePath,
		"Ensure the base scene path is correct",
		"Use create_scene to create the base scene first",
	)
}

// instanceMissingResult is the error for an instancePath that does not resolve
// to a file, so the caller learns which of the two add_node sources is missing.
func instanceMissingResult(instancePath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Instance scene does not exist: "+instancePath,
		"Ensure the instance path is correct",
		"Use create_scene to create the scene to instance first",
	)
}

// notAProjectResult is the shared error for a projectPath without a
// project.godot file, matching the reference server's message and solutions.
func notAProjectResult(projectPath string) mcp.ToolResult {
	return mcp.ErrorResult(
		"Not a valid Godot project: "+projectPath,
		"Ensure the path points to a directory containing a project.godot file",
		"Create a project with create_project if none exists",
	)
}
