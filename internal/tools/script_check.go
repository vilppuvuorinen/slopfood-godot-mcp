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

func parseGodotScriptDiagnostics(output string) []scriptDiagnostic {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	diagnostics := make([]scriptDiagnostic, 0)
	for i, line := range lines {
		match := scriptErrorPattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		message := parseErrorPrefix.ReplaceAllString(match[1], "")
		diagnostic := scriptDiagnostic{Message: message}
		for _, j := range []int{i + 1, i} {
			if j >= len(lines) {
				continue
			}
			loc := scriptLocPattern.FindStringSubmatch(lines[j])
			if loc == nil {
				continue
			}
			diagnostic.File = loc[1]
			if n, err := strconv.Atoi(loc[2]); err == nil {
				diagnostic.Line = n
			}
			break
		}
		diagnostics = append(diagnostics, diagnostic)
	}
	return diagnostics
}

// Diagnostics are parsed from stderr: Godot logs SCRIPT ERROR lines but still
// exits 0 when the target script fails to compile.
func runGDScriptCheck(ctx context.Context, projectPath, scriptFull string) ([]scriptDiagnostic, error) {
	rel, err := filepath.Rel(projectPath, scriptFull)
	if err != nil {
		return nil, fmt.Errorf("resolve script path: %w", err)
	}
	scriptRes := resScheme + filepath.ToSlash(rel)

	checkerPath, err := godotops.ValidateScript()
	if err != nil {
		return nil, fmt.Errorf("materialize validate script: %w", err)
	}
	_, stderr, exitCode, err := godot.CheckScript(ctx, godot.Path(), projectPath, checkerPath, scriptRes)
	if err != nil {
		return nil, fmt.Errorf("run script checker: %w", err)
	}
	diagnostics := parseGodotScriptDiagnostics(stderr)
	for i := range diagnostics {
		diagnostics[i].File = projectRelativePath(diagnostics[i].File)
	}
	if len(diagnostics) == 0 && scriptFailed(exitCode, stderr) {
		return nil, fmt.Errorf("script checker failed (exit %d): %s", exitCode, strings.TrimSpace(stderr))
	}
	return diagnostics, nil
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
