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

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotops"
)

// scriptDiagnostic is one parsed Godot script error, matching the reference's
// ScriptDiagnostic shape.
type scriptDiagnostic struct {
	Message string `json:"message"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

var (
	// scriptErrorPattern matches an engine diagnostic line, e.g.
	// "SCRIPT ERROR: Parse Error: ...".
	scriptErrorPattern = regexp.MustCompile(`SCRIPT ERROR:\s*(.+?)\s*$`)
	// parseErrorPrefix is stripped from a diagnostic message.
	parseErrorPrefix = regexp.MustCompile(`^Parse Error:\s*`)
	// scriptLocPattern captures the res:// file and line in the location line
	// that accompanies a diagnostic.
	scriptLocPattern = regexp.MustCompile(`\((res://.+):(\d+)\)`)
)

// parseGodotScriptDiagnostics extracts script errors from combined Godot
// output, mirroring the reference's parseGodotScriptDiagnostics. Each
// diagnostic's file/line is taken from the location line that follows it (or
// from its own line when the location is inline).
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

// runGDScriptCheck static-checks scriptFull with the embedded validate_script
// checker and returns the parsed diagnostics. It only returns an error when the
// checker could not be materialized or Godot could not be started; a non-zero
// checker exit is expected and reported through the diagnostics.
func runGDScriptCheck(ctx context.Context, projectPath, scriptFull string) ([]scriptDiagnostic, error) {
	rel, err := filepath.Rel(projectPath, scriptFull)
	if err != nil {
		return nil, fmt.Errorf("resolve script path: %w", err)
	}
	scriptRes := "res://" + filepath.ToSlash(rel)

	checkerPath, err := godotops.ValidateScript()
	if err != nil {
		return nil, fmt.Errorf("materialize validate script: %w", err)
	}
	output, err := godot.CheckScript(ctx, godot.Path(), projectPath, checkerPath, scriptRes)
	if err != nil {
		return nil, fmt.Errorf("run script checker: %w", err)
	}
	return parseGodotScriptDiagnostics(output), nil
}

// collectGdPaths deduplicates .gd paths across git outputs, normalizing
// backslashes to forward slashes and trimming surrounding whitespace.
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

// listChangedGdFiles returns the .gd files git reports as changed (unstaged,
// staged, or untracked) relative to projectPath. A project that is not a git
// work tree is an error so the caller can fall back to scope "all" or an
// explicit scriptPaths list.
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

// listAllGdFiles walks projectPath and returns every .gd file relative to the
// project root with forward slashes, skipping Godot, VCS, dependency, import,
// and hidden directories.
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
