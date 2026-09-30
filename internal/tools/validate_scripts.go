package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

func validateScripts() mcp.Tool {
	return mcp.Tool{
		Name:        "validate_scripts",
		Title:       "Validate Scripts",
		Description: "Batch-check GDScript files (git-changed by default, or all)",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"scope": map[string]any{
					"type":        "string",
					"enum":        []string{"changed", "all"},
					"default":     "changed",
					"description": `"changed" = git-changed .gd (default); "all" = every .gd in project`,
				},
				"scriptPaths": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Optional explicit list of .gd paths to check (overrides scope)",
				},
			},
			"required": []string{},
		},
		Handler: withContext(handleValidateScripts),
	}
}

type validateScriptsArgs struct {
	Scope       string   `json:"scope"`
	ScriptPaths []string `json:"scriptPaths"`
}

type validateScriptCheck struct {
	ScriptPath string             `json:"scriptPath"`
	Checked    bool               `json:"checked"`
	Valid      bool               `json:"valid"`
	ErrorCount int                `json:"errorCount"`
	Errors     []scriptDiagnostic `json:"errors"`
}

type validateScriptSkip struct {
	ScriptPath string `json:"scriptPath"`
	Checked    bool   `json:"checked"`
	Error      string `json:"error"`
}

type validateScriptsReport struct {
	Scope           string `json:"scope"`
	FileCount       int    `json:"fileCount"`
	FilesWithErrors int    `json:"filesWithErrors"`
	AllValid        bool   `json:"allValid"`
	Results         []any  `json:"results"`
}

func handleValidateScripts(ctx Context, raw json.RawMessage) (any, error) {
	var args validateScriptsArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}
	if !pathExists(filepath.Join(project, "project.godot")) {
		return notAProjectResult(project), nil
	}

	explicit := len(args.ScriptPaths) > 0
	scope := ""
	var candidates []string
	switch {
	case explicit:
		scope = "explicit"
		candidates = args.ScriptPaths
	case args.Scope == "" || args.Scope == "changed":
		scope = "changed"
		changed, listErr := listChangedGdFiles(ctx, project)
		if listErr != nil {
			return mcp.ErrorResult(listErr.Error()), nil
		}
		candidates = changed
	case args.Scope == "all":
		scope = "all"
		candidates = listAllGdFiles(project)
	default:
		return mcp.ErrorResult(fmt.Sprintf(`Invalid scope %q. Use "changed" or "all", or pass scriptPaths.`, args.Scope)), nil
	}

	results := make([]any, 0, len(candidates))
	allChecked := true
	toCheck := make([]string, 0, len(candidates))
	for _, rel := range candidates {
		relFull, relOK := confinePath(project, rel)
		if !strings.HasSuffix(strings.ToLower(rel), ".gd") || !relOK {
			if explicit {
				results = append(results, validateScriptSkip{ScriptPath: rel, Checked: false, Error: "Not a valid .gd path"})
				allChecked = false
			}
			continue
		}
		if !pathExists(relFull) {
			if explicit {
				results = append(results, validateScriptSkip{ScriptPath: rel, Checked: false, Error: "Script does not exist"})
				allChecked = false
			}
			continue
		}
		toCheck = append(toCheck, rel)
	}

	filesWithErrors := 0
	for _, rel := range toCheck {
		diagnostics, checkErr := runGDScriptCheck(ctx, project, filepath.Join(project, rel))
		if checkErr != nil {
			results = append(results, validateScriptSkip{ScriptPath: rel, Checked: false, Error: checkErr.Error()})
			allChecked = false
			continue
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
		Scope:           scope,
		FileCount:       len(results),
		FilesWithErrors: filesWithErrors,
		AllValid:        filesWithErrors == 0 && allChecked,
		Results:         results,
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode validate_scripts report: %w", err)
	}
	return mcp.StructuredResult(string(encoded), report), nil
}
