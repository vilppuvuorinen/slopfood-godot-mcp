package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

type scriptDiagnostic struct {
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

var (
	scriptErrorPattern = regexp.MustCompile(`SCRIPT ERROR:\s*(.+?)\s*$`)
	parseErrorPrefix   = regexp.MustCompile(`^Parse Error:\s*`)
	scriptLocPattern   = regexp.MustCompile(`\((res://.+):(\d+)\)`)
)

const (
	scriptMarkerPrefix = "<<<GODOT_MCP_VALIDATE:"
	scriptMarkerSuffix = ">>>"
)

// Matches the checker's per-target marker lines so they do not count as
// unexpected stderr on an otherwise clean run.
var scriptMarkerPattern = regexp.MustCompile(regexp.QuoteMeta(scriptMarkerPrefix) + `.*` + regexp.QuoteMeta(scriptMarkerSuffix))

func parseScriptMarker(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, scriptMarkerPrefix) || !strings.HasSuffix(trimmed, scriptMarkerSuffix) {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(trimmed, scriptMarkerPrefix), scriptMarkerSuffix), true
}

// parseGodotScriptDiagnosticGroups parses one checker run's stderr into
// diagnostics keyed by project-relative script path. Each load is bracketed by a
// marker line, so a diagnostic is attributed to the script being loaded even
// when the engine prints no location for it.
func parseGodotScriptDiagnosticGroups(output string) map[string][]scriptDiagnostic {
	groups := make(map[string][]scriptDiagnostic)
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	current := ""
	for i := 0; i < len(lines); i++ {
		if target, ok := parseScriptMarker(lines[i]); ok {
			current = projectRelativePath(target)
			continue
		}
		match := scriptErrorPattern.FindStringSubmatch(lines[i])
		if match == nil {
			continue
		}
		diagnostic := scriptDiagnostic{Message: parseErrorPrefix.ReplaceAllString(match[1], "")}
		for _, j := range []int{i + 1, i} {
			if j >= len(lines) {
				continue
			}
			loc := scriptLocPattern.FindStringSubmatch(lines[j])
			if loc == nil {
				continue
			}
			diagnostic.File = projectRelativePath(loc[1])
			if n, err := strconv.Atoi(loc[2]); err == nil {
				diagnostic.Line = n
			}
			break
		}
		if current == "" {
			continue
		}
		if diagnostic.File == "" {
			diagnostic.File = current
		}
		groups[current] = append(groups[current], diagnostic)
	}
	return groups
}

// Diagnostics are parsed from stderr: Godot logs SCRIPT ERROR lines but still
// exits 0 when a target script fails to compile. Every path is checked in one
// engine process, so a batch costs a single engine startup instead of one per
// file.
func runGDScriptChecks(ctx context.Context, projectPath string, scriptRelPaths []string) (map[string][]scriptDiagnostic, error) {
	if len(scriptRelPaths) == 0 {
		return map[string][]scriptDiagnostic{}, nil
	}
	checkerPath, err := godotops.ValidateScript()
	if err != nil {
		return nil, fmt.Errorf("materialize validate script: %w", err)
	}
	resPaths := make([]string, 0, len(scriptRelPaths))
	for _, rel := range scriptRelPaths {
		resPaths = append(resPaths, resScheme+filepath.ToSlash(rel))
	}
	_, stderr, exitCode, err := godot.CheckScripts(ctx, godot.Path(), projectPath, checkerPath, resPaths)
	if err != nil {
		return nil, fmt.Errorf("run script checker: %w", err)
	}
	groups := parseGodotScriptDiagnosticGroups(stderr)
	if len(groups) == 0 && scriptFailed(exitCode, stderr, scriptMarkerPattern) {
		return nil, fmt.Errorf("script checker failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return groups, nil
}

func collectGdPaths(outputs []string) []string {
	result := make([]string, 0)
	seen := make(map[string]bool)
	for _, out := range outputs {
		for _, rawLine := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
			filePath := strings.ReplaceAll(strings.TrimSpace(rawLine), "\\", "/")
			if !strings.HasSuffix(strings.ToLower(filePath), ".gd") {
				continue
			}
			if seen[filePath] {
				continue
			}
			seen[filePath] = true
			result = append(result, filePath)
		}
	}
	return result
}

func listChangedGdFiles(ctx context.Context, projectPath string) ([]string, error) {
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.quotepath=false", "-C", projectPath}, args...)...) //nolint:gosec // git args are fixed and projectPath is caller-controlled.
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("run git: %w", err)
		}
		return stdout.String(), nil
	}

	if _, err := git("rev-parse", "--is-inside-work-tree"); err != nil {
		return nil, errors.New(`Not a git repository (or git is unavailable). Use scope: "all" or pass scriptPaths.`) //nolint:staticcheck // user-facing message mirrors the reference server.
	}

	outputs := make([]string, 0, 3)
	for _, args := range [][]string{
		{"diff", "--name-only", "--relative"},
		{"diff", "--name-only", "--relative", "--cached"},
		{"ls-files", "--others", "--exclude-standard"},
	} {
		out, err := git(args...)
		if err != nil {
			return nil, fmt.Errorf("Failed to list changed files: %w", err) //nolint:staticcheck // user-facing message mirrors the reference server.
		}
		outputs = append(outputs, out)
	}
	return collectGdPaths(outputs), nil
}

func listAllGdFiles(projectPath string) []string {
	results := make([]string, 0)
	skipDirs := map[string]bool{
		".godot":       true,
		".git":         true,
		"node_modules": true,
		".import":      true,
	}
	var walk func(dir string)
	walk = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() {
				if skipDirs[name] || strings.HasPrefix(name, ".") {
					continue
				}
				walk(filepath.Join(dir, name))
				continue
			}
			if !entry.Type().IsRegular() || !strings.HasSuffix(strings.ToLower(name), ".gd") {
				continue
			}
			rel, err := filepath.Rel(projectPath, filepath.Join(dir, name))
			if err != nil {
				continue
			}
			results = append(results, filepath.ToSlash(rel))
		}
	}
	walk(projectPath)
	return results
}
