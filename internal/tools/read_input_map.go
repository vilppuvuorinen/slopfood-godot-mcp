package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

func readInputMap() mcp.Tool {
	return mcp.Tool{
		Name:        "read_input_map",
		Title:       "Read Input Map",
		Description: "List input actions and bindings",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		OutputSchema: readOutputSchema(map[string]any{
			"type":                 "object",
			"description":          "Input actions by name, with values in the tool value contract",
			"additionalProperties": godotValueSchema("A value in the tool value contract"),
		}),
		Handler: handleReadInputMap,
	}
}

func handleReadInputMap(ctx context.Context, _ json.RawMessage) (any, error) {
	store, failure := openSettings(projectRoot(ctx))
	if failure != nil {
		return *failure, nil
	}
	actions, err := store.SectionValues("input")
	if err != nil {
		return mcp.ErrorResult("Failed to read input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	data, err := json.MarshalIndent(actions, "", "  ")
	if err != nil {
		return mcp.ErrorResult("Failed to read input map: " + err.Error()), nil //nolint:nilerr // the failure is reported through the tool result.
	}
	return mcp.JSONResult(string(data)), nil
}
