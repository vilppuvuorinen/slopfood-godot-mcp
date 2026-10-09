package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

const projectValidationTimeout = 30 * time.Second

func (t *toolset) validate() mcp.Tool {
	return mcp.Tool{
		Name:        "validate",
		Title:       "Validate",
		Description: "Validate project.godot and every GDScript file in the project, returning all diagnostics",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: payloadOutputSchema(map[string]any{
			"valid": map[string]any{
				"type":        "boolean",
				"description": "True only when project.godot parses and every script validates",
			},
			"project": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"valid": map[string]any{"type": "boolean"},
					"errors": map[string]any{
						"type":        "array",
						"description": "Engine diagnostics reported while loading project.godot; empty when valid",
						"items":       map[string]any{"type": "string"},
					},
				},
				"required": []string{"valid", "errors"},
			},
			"scripts": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileCount":       map[string]any{"type": "integer"},
					"filesWithErrors": map[string]any{"type": "integer"},
					"allValid":        map[string]any{"type": "boolean"},
					"error": map[string]any{
						"type":        "string",
						"description": "Present when the script sweep could not run; results are then empty",
					},
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
				},
				"required": []string{"fileCount", "filesWithErrors", "allValid", "results"},
			},
		}),
		Handler: t.handleValidate,
	}
}

type validateProjectReport struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors"`
}

type validateScriptCheck struct {
	ScriptPath string             `json:"scriptPath"`
	Checked    bool               `json:"checked"`
	Valid      bool               `json:"valid"`
	ErrorCount int                `json:"errorCount"`
	Errors     []scriptDiagnostic `json:"errors"`
}

type validateScriptsReport struct {
	FileCount       int                   `json:"fileCount"`
	FilesWithErrors int                   `json:"filesWithErrors"`
	AllValid        bool                  `json:"allValid"`
	Results         []validateScriptCheck `json:"results"`
	Error           string                `json:"error,omitempty"`
}

type validateReport struct {
	Valid   bool                  `json:"valid"`
	Project validateProjectReport `json:"project"`
	Scripts validateScriptsReport `json:"scripts"`
}

func (t *toolset) handleValidate(ctx context.Context, _ json.RawMessage) (any, error) {
	project := t.project
	if result, ok := requireProject(project); !ok {
		return result, nil
	}

	projectReport, err := validateProjectSection(ctx, project)
	if err != nil {
		return mcp.ErrorResult("Failed to validate: "+err.Error(), headlessSolutions...), nil //nolint:nilerr // the failure is reported through the tool result.
	}

	// A project.godot parse error also shows up on the script checker's stderr,
	// so the sweep is still attempted and its failure recorded rather than
	// discarding the project diagnostics.
	scriptsReport := validateScriptsSection(ctx, project)

	report := validateReport{
		Valid:   projectReport.Valid && scriptsReport.Error == "" && scriptsReport.AllValid,
		Project: projectReport,
		Scripts: scriptsReport,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode validate report: %w", err)
	}
	return mcp.StructuredResult(string(data), report), nil
}

func validateProjectSection(ctx context.Context, project string) (validateProjectReport, error) {
	// Godot logs parse diagnostics to stderr but still exits 0, so validity is
	// reported as structured data rather than through isError.
	probeCtx, cancel := context.WithTimeout(ctx, projectValidationTimeout)
	defer cancel()
	probe, err := probeProjectWithGodot(probeCtx, project)
	if err != nil {
		return validateProjectReport{}, err
	}

	report := validateProjectReport{
		Valid:  true,
		Errors: godot.ProjectParseErrors(probe.Stderr),
	}
	if len(report.Errors) > 0 || probe.ExitCode != 0 {
		report.Valid = false
		if len(report.Errors) == 0 {
			report.Errors = []string{fmt.Sprintf("Godot exited with code %d", probe.ExitCode)}
		}
	}
	if report.Errors == nil {
		report.Errors = []string{}
	}
	return report, nil
}

func validateScriptsSection(ctx context.Context, project string) validateScriptsReport {
	candidates := listAllGdFiles(project)
	groups, err := runGDScriptChecks(ctx, project, candidates)
	if err != nil {
		// A broken project.godot makes the checker's stderr look like a failure;
		// the project section already carries the underlying parse diagnostics.
		return validateScriptsReport{Results: []validateScriptCheck{}, Error: err.Error()}
	}

	results := make([]validateScriptCheck, 0, len(candidates))
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

	return validateScriptsReport{
		FileCount:       len(results),
		FilesWithErrors: filesWithErrors,
		AllValid:        filesWithErrors == 0,
		Results:         results,
	}
}
