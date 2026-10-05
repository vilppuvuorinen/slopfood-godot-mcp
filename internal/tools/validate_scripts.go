package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func validateScripts() mcp.Tool {
	return mcp.Tool{
		Name:        "validate_scripts",
		Title:       "Validate Scripts",
		Description: "Validate every GDScript file in the project",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"fileCount":       map[string]any{"type": "integer"},
			"filesWithErrors": map[string]any{"type": "integer"},
			"allValid":        map[string]any{"type": "boolean"},
			"results": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"scriptPath": map[string]any{"type": "string"},
						"checked":    map[string]any{"type": "boolean"},
						"valid":      map[string]any{"type": "boolean"},
						"errorCount": map[string]any{"type": "integer"},
						"errors": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"message": map[string]any{"type": "string"},
									"file":    map[string]any{"type": "string"},
									"line":    map[string]any{"type": "integer"},
								},
								"required": []string{"message"},
							},
						},
					},
					"required": []string{"scriptPath", "checked", "valid", "errorCount", "errors"},
				},
			},
		}),
		Handler: handleValidateScripts,
	}
}

type validateScriptCheck struct {
	ScriptPath string             `json:"scriptPath"`
	Checked    bool               `json:"checked"`
	Valid      bool               `json:"valid"`
	ErrorCount int                `json:"errorCount"`
	Errors     []scriptDiagnostic `json:"errors"`
}

type validateScriptsReport struct {
	FileCount       int   `json:"fileCount"`
	FilesWithErrors int   `json:"filesWithErrors"`
	AllValid        bool  `json:"allValid"`
	Results         []any `json:"results"`
}

func handleValidateScripts(ctx context.Context, _ json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	// Every script is validated in one engine process; the whole-project sweep is
	// what makes the single engine startup worth its cold start.
	candidates := listAllGdFiles(project)
	groups, checkErr := runGDScriptChecks(ctx, project, candidates)
	if checkErr != nil {
		return mcp.ErrorResult("Failed to validate scripts: "+checkErr.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	results := make([]any, 0, len(candidates))
	filesWithErrors := 0
	for _, rel := range candidates {
		diagnostics := groups[rel]
		if diagnostics == nil {
			diagnostics = []scriptDiagnostic{}
		}
		if len(diagnostics) > 0 {
			filesWithErrors++
		}
		results = append(results, validateScriptCheck{
			ScriptPath: rel,
			Checked:    true,
			Valid:      len(diagnostics) == 0,
			ErrorCount: len(diagnostics),
			Errors:     diagnostics,
		})
	}

	report := validateScriptsReport{
		FileCount:       len(results),
		FilesWithErrors: filesWithErrors,
		AllValid:        filesWithErrors == 0,
		Results:         results,
	}
	encoded, encodeErr := json.MarshalIndent(report, "", "  ")
	if encodeErr != nil {
		return nil, fmt.Errorf("encode validate_scripts report: %w", encodeErr)
	}
	return mcp.StructuredResult(string(encoded), report), nil
}
